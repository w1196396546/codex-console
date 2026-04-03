"""Auth-specific Cloudflare browser warmup helpers."""

from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)

_AUTH_CF_COOKIE_NAMES = {"cf_clearance", "__cf_bm"}
_AUTH_COOKIE_DOMAIN_SUFFIXES = (
    "auth.openai.com",
    ".auth.openai.com",
    "openai.com",
    ".openai.com",
    "chatgpt.com",
    ".chatgpt.com",
)


def _is_supported_cookie_domain(domain: str) -> bool:
    text = str(domain or "").strip().lower()
    if not text:
        return False
    return any(text == suffix or text.endswith(suffix.lstrip(".")) for suffix in _AUTH_COOKIE_DOMAIN_SUFFIXES)


def _build_browser_cookie_items(session: Any, device_id: str) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()

    cookie_jar = getattr(getattr(session, "cookies", None), "jar", None) or []
    for cookie in cookie_jar:
        try:
            name = str(getattr(cookie, "name", "") or "").strip()
            value = str(getattr(cookie, "value", "") or "").strip()
            domain = str(getattr(cookie, "domain", "") or "").strip()
            path = str(getattr(cookie, "path", "/") or "/").strip() or "/"
        except Exception:
            continue
        if not name or not value or not _is_supported_cookie_domain(domain):
            continue
        marker = (name, domain)
        if marker in seen:
            continue
        seen.add(marker)
        items.append(
            {
                "name": name,
                "value": value,
                "domain": domain,
                "path": path,
                "secure": bool(getattr(cookie, "secure", True)),
            }
        )

    did_text = str(device_id or "").strip()
    if did_text and ("oai-did", "auth.openai.com") not in seen:
        items.append(
            {
                "name": "oai-did",
                "value": did_text,
                "domain": "auth.openai.com",
                "path": "/",
                "secure": True,
            }
        )

    return items


def _merge_cookie_into_session(session: Any, name: str, value: str, domain: str, path: str = "/") -> None:
    if not session or not hasattr(session, "cookies"):
        return
    try:
        session.cookies.set(name, value, domain=domain, path=path or "/")
    except Exception as exc:
        logger.warning("回灌 auth cookie 失败: name=%s domain=%s error=%s", name, domain, exc)


def try_warm_auth_cloudflare_cookies(
    session: Any,
    auth_url: str,
    *,
    device_id: str,
    proxy: Optional[str] = None,
    user_agent: Optional[str] = None,
    timeout_seconds: int = 45,
    headless: bool = False,
) -> bool:
    """Warm auth.openai.com Cloudflare cookies through a browser fallback."""
    target_url = str(auth_url or "").strip()
    if not target_url or session is None:
        return False

    try:
        from playwright.sync_api import sync_playwright
    except Exception:
        logger.warning("auth Cloudflare warmup 跳过: Playwright 不可用")
        return False

    from .browser_bind import _wait_for_cloudflare

    launch_kwargs: Dict[str, Any] = {"headless": bool(headless)}
    proxy_server = str(proxy or "").strip()
    if proxy_server:
        launch_kwargs["proxy"] = {"server": proxy_server}

    cookie_items = _build_browser_cookie_items(session, device_id=device_id)

    with sync_playwright() as p:
        browser = p.chromium.launch(**launch_kwargs)
        try:
            context_kwargs: Dict[str, Any] = {}
            if user_agent:
                context_kwargs["user_agent"] = user_agent
            context = browser.new_context(**context_kwargs)
            if cookie_items:
                context.add_cookies(cookie_items)

            page = context.new_page()
            page.goto(target_url, wait_until="domcontentloaded", timeout=60000)
            passed, note = _wait_for_cloudflare(page, max_wait_seconds=max(15, int(timeout_seconds or 45)))
            if not passed:
                logger.warning("auth Cloudflare warmup 未通过: url=%s note=%s", target_url[:160], note)
                return False

            merged = False
            for cookie in context.cookies():
                name = str(cookie.get("name") or "").strip()
                if name not in _AUTH_CF_COOKIE_NAMES:
                    continue
                value = str(cookie.get("value") or "").strip()
                domain = str(cookie.get("domain") or "auth.openai.com").strip() or "auth.openai.com"
                path = str(cookie.get("path") or "/").strip() or "/"
                if not value:
                    continue
                _merge_cookie_into_session(session, name, value, domain, path)
                merged = True

            return merged
        finally:
            try:
                browser.close()
            except Exception:
                pass
