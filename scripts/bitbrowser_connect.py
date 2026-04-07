#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from typing import Any
from urllib.parse import urlparse


DEFAULT_BASE_URL = "http://127.0.0.1:54345"


class BitBrowserError(RuntimeError):
    """BitBrowser 本地服务调用失败。"""


def normalize_base_url(base_url: str) -> str:
    return base_url.rstrip("/")


def request_json(method: str, url: str, payload: dict[str, Any] | None, timeout: float) -> Any:
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise BitBrowserError(f"{method} {url} 返回 HTTP {exc.code}: {detail}") from exc
    except urllib.error.URLError as exc:
        raise BitBrowserError(f"{method} {url} 连接失败: {exc.reason}") from exc

    try:
        return json.loads(body)
    except json.JSONDecodeError as exc:
        raise BitBrowserError(f"{method} {url} 返回了非 JSON 内容: {body[:200]}") from exc


def post_json(base_url: str, path: str, payload: dict[str, Any], timeout: float) -> Any:
    return request_json("POST", f"{normalize_base_url(base_url)}{path}", payload, timeout)


def get_json(url: str, timeout: float) -> Any:
    return request_json("GET", url, None, timeout)


def expect_success(response: Any, path: str) -> Any:
    if not isinstance(response, dict):
        raise BitBrowserError(f"{path} 返回格式异常: {response!r}")
    if not response.get("success"):
        raise BitBrowserError(f"{path} 调用失败: {json.dumps(response, ensure_ascii=False)}")
    return response.get("data")


def normalize_http_endpoint(endpoint: str) -> str:
    value = endpoint.rstrip("/")
    if value.startswith(("http://", "https://")):
        return value
    if value.startswith(("ws://", "wss://")):
        parsed = urlparse(value)
        if not parsed.netloc:
            raise BitBrowserError(f"无法从 ws 地址推导 HTTP 地址: {value}")
        return f"http://{parsed.netloc}"
    return f"http://{value}"


def list_opened(base_url: str, timeout: float) -> dict[str, Any]:
    opened_ids = expect_success(post_json(base_url, "/browser/opened/ids", {}, timeout), "/browser/opened/ids")
    ports = expect_success(post_json(base_url, "/browser/ports", {}, timeout), "/browser/ports")
    return {
        "base_url": normalize_base_url(base_url),
        "opened_ids": opened_ids or [],
        "ports": ports or {},
        "count": len(opened_ids or []),
    }


def health(base_url: str, timeout: float) -> dict[str, Any]:
    data = expect_success(post_json(base_url, "/health", {}, timeout), "/health")
    return {
        "base_url": normalize_base_url(base_url),
        "ok": True,
        "message": data,
    }


def select_browser_id(opened_ids: list[str], browser_id: str | None) -> str:
    if browser_id:
        return browser_id
    if len(opened_ids) == 1:
        return opened_ids[0]
    if not opened_ids:
        raise ValueError("当前没有已打开实例，请传入 --browser-id，并结合 --open 打开目标环境。")
    raise ValueError("当前存在多个已打开实例，请显式传入 --browser-id。")


def collect_connect_info(
    base_url: str,
    browser_id: str | None = None,
    auto_open: bool = False,
    timeout: float = 5.0,
) -> dict[str, Any]:
    opened = list_opened(base_url, timeout)
    opened_ids = opened["opened_ids"]
    ports = opened["ports"]
    selected_browser_id = select_browser_id(opened_ids, browser_id)

    open_data = None
    if selected_browser_id not in ports:
        if not auto_open:
            raise ValueError(f"实例 {selected_browser_id} 当前未打开，请增加 --open 以自动打开。")
        open_data = expect_success(
            post_json(base_url, "/browser/open", {"id": selected_browser_id}, timeout),
            "/browser/open",
        )

    debug_http_url = ""
    if open_data and open_data.get("http"):
        debug_http_url = normalize_http_endpoint(str(open_data["http"]))
    elif selected_browser_id in ports:
        debug_http_url = f"http://127.0.0.1:{ports[selected_browser_id]}"
    else:
        raise BitBrowserError(f"实例 {selected_browser_id} 未返回调试端口。")

    version_info = get_json(f"{debug_http_url}/json/version", timeout)
    targets = get_json(f"{debug_http_url}/json/list", timeout)
    pages = [
        {
            "id": item.get("id"),
            "title": item.get("title"),
            "url": item.get("url"),
        }
        for item in targets
        if isinstance(item, dict) and item.get("type") == "page"
    ]

    return {
        "base_url": normalize_base_url(base_url),
        "browser_id": selected_browser_id,
        "was_opened_by_script": bool(open_data),
        "debug_http_url": debug_http_url,
        "browser_ws_endpoint": version_info.get("webSocketDebuggerUrl") or (open_data or {}).get("ws"),
        "browser_version": version_info.get("Browser"),
        "page_count": len(pages),
        "pages": pages,
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="获取 BitBrowser 本地连接信息，输出 JSON。")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL, help=f"BitBrowser 本地 API 地址，默认 {DEFAULT_BASE_URL}")
    parser.add_argument("--timeout", type=float, default=5.0, help="接口超时秒数，默认 5")
    parser.add_argument("--pretty", action="store_true", help="是否格式化输出 JSON")

    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("health", help="检查本地服务是否可用")
    subparsers.add_parser("opened", help="列出当前已打开实例及调试端口")

    connect_parser = subparsers.add_parser("connect-info", help="返回某个实例的可连接 CDP 信息")
    connect_parser.add_argument("--browser-id", help="目标环境 ID；不传时仅在已打开实例恰好为 1 个时自动选中")
    connect_parser.add_argument("--open", action="store_true", help="如果目标实例尚未打开，则先调用 /browser/open")
    return parser


def dump_json(payload: dict[str, Any], pretty: bool) -> None:
    if pretty:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return
    print(json.dumps(payload, ensure_ascii=False))


def run_command(args: argparse.Namespace) -> dict[str, Any]:
    if args.command == "health":
        return health(args.base_url, args.timeout)
    if args.command == "opened":
        return list_opened(args.base_url, args.timeout)
    if args.command == "connect-info":
        return collect_connect_info(
            args.base_url,
            browser_id=args.browser_id,
            auto_open=args.open,
            timeout=args.timeout,
        )
    raise AssertionError(f"unknown command {args.command}")


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        payload = run_command(args)
    except (BitBrowserError, ValueError) as exc:
        dump_json({"ok": False, "error": str(exc)}, args.pretty)
        return 1
    dump_json(payload, args.pretty)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
