from __future__ import annotations

from typing import Any, Callable, Iterable, List, Optional

try:
    from curl_cffi import requests as cffi_requests
except ImportError:  # pragma: no cover - 依赖由运行环境保证，测试可打桩覆盖
    cffi_requests = None


_TEAM_API_BASE_URL = "https://chatgpt.com"


class TeamClientError(Exception):
    """上游 Team API 基础异常。"""


class TeamAuthenticationError(TeamClientError):
    """鉴权失败。"""


class TeamPermissionError(TeamClientError):
    """权限不足。"""


class TeamNotFoundError(TeamClientError):
    """资源不存在。"""


class TeamRateLimitError(TeamClientError):
    """命中限流。"""


class TeamResponseFormatError(TeamClientError):
    """响应结构不符合预期。"""




class TeamAccount(dict):
    """兼容旧导出的占位类型。"""


class TeamUser(dict):
    """兼容旧导出的占位类型。"""


class TeamMember(dict):
    """兼容旧导出的占位类型。"""


class TeamInvite(dict):
    """兼容旧导出的占位类型。"""

class TeamClient:
    """上游 Team API 合同层。"""

    def __init__(self, transport: Optional[Callable[..., Any]] = None):
        self._transport = transport or self._default_transport

    async def get_team_accounts(self, access_token: str) -> dict:
        return await self._call_transport(
            "GET",
            "/backend-api/accounts/check/v4-2023-04-27",
            access_token=access_token,
        )

    async def list_members(
        self,
        access_token: str,
        upstream_account_id: str,
        *,
        limit: int = 100,
        offset: int = 0,
    ) -> dict:
        return await self._call_transport(
            "GET",
            f"/backend-api/accounts/{upstream_account_id}/users",
            access_token=access_token,
            params={"limit": limit, "offset": offset},
        )

    async def list_invites(self, access_token: str, upstream_account_id: str) -> dict:
        return await self._call_transport(
            "GET",
            f"/backend-api/accounts/{upstream_account_id}/invites",
            access_token=access_token,
        )

    def parse_team_accounts(self, payload: Any) -> List[dict]:
        body = self._ensure_dict(payload, "team accounts")
        accounts = body.get("accounts")
        if not isinstance(accounts, dict):
            raise TeamResponseFormatError("Team accounts 响应缺少 accounts 映射")

        parsed: List[dict] = []
        for upstream_account_id, raw_item in accounts.items():
            if not isinstance(raw_item, dict):
                continue

            account = raw_item.get("account") if isinstance(raw_item.get("account"), dict) else {}
            entitlement = raw_item.get("entitlement") if isinstance(raw_item.get("entitlement"), dict) else {}

            plan_type = self._as_text(account.get("plan_type")) or "unknown"
            subscription_plan = self._as_text(entitlement.get("subscription_plan")) or "unknown"
            if not self._is_team_account(plan_type=plan_type, subscription_plan=subscription_plan):
                continue

            upstream_account_id = self._as_text(upstream_account_id)
            if not upstream_account_id:
                continue

            parsed.append(
                {
                    "upstream_account_id": upstream_account_id,
                    "team_name": self._as_text(account.get("name")) or f"Team-{upstream_account_id[:8]}",
                    "plan_type": plan_type,
                    "account_role_snapshot": self._as_text(account.get("account_user_role")) or "unknown",
                    "subscription_plan": subscription_plan,
                    "expires_at": self._as_text(entitlement.get("expires_at")),
                }
            )

        return parsed

    def parse_members(self, payload: Any) -> List[dict]:
        items = self._extract_items(payload, context="members")
        parsed: List[dict] = []
        for item in items:
            if not isinstance(item, dict):
                raise TeamResponseFormatError("Team members.items 列表项不是对象")

            upstream_user_id = self._as_text(item.get("id"))
            if not upstream_user_id:
                raise TeamResponseFormatError("Team members.items[].id 缺失")

            parsed.append(
                {
                    "upstream_user_id": upstream_user_id,
                    "email": self._as_text(item.get("email")),
                    "name": self._as_text(item.get("name")),
                    "role": self._as_text(item.get("role")),
                    "created_time": self._as_text(item.get("created_time")),
                }
            )

        return parsed

    def parse_invites(self, payload: Any) -> List[dict]:
        items = self._extract_items(payload, context="invites")
        parsed: List[dict] = []
        for item in items:
            if not isinstance(item, dict):
                raise TeamResponseFormatError("Team invites.items 列表项不是对象")

            email_address = self._as_text(item.get("email_address"))
            if not email_address:
                raise TeamResponseFormatError("Team invites.items[].email_address 缺失")

            parsed.append(
                {
                    "email_address": email_address,
                    "role": self._as_text(item.get("role")),
                    "created_time": self._as_text(item.get("created_time")),
                }
            )

        return parsed

    def raise_for_error(self, *, status_code: int, payload: Any = None, text: str = "") -> None:
        if status_code < 400:
            return

        detail = self._extract_error_message(payload, text=text)
        normalized = detail.lower()

        if status_code == 401 or any(token in normalized for token in ("invalid", "unauthorized", "authentication", "token")):
            raise TeamAuthenticationError(detail)
        if status_code == 403 or any(token in normalized for token in ("forbidden", "permission")):
            raise TeamPermissionError(detail)
        if status_code == 404 or any(token in normalized for token in ("missing", "not found", "not_found")):
            raise TeamNotFoundError(detail)
        if status_code == 429 or "rate_limit" in normalized:
            raise TeamRateLimitError(detail)
        raise TeamClientError(detail)

    def collect_items_from_pages(self, *, pages: Iterable[Any], parser: Callable[[Any], List[dict]]) -> List[dict]:
        collected: List[dict] = []

        for payload in pages:
            body = self._ensure_dict(payload, "paged response")
            items = self._extract_items(body, context="paged response")
            parsed_items = parser(body)
            collected.extend(parsed_items)

            if not items:
                break

            total = self._as_int(body.get("total"))
            if total is not None and len(collected) >= total:
                break

            limit = self._as_int(body.get("limit"))
            if limit is not None and len(items) < limit:
                break

        return collected

    async def _call_transport(self, method: str, path: str, **kwargs) -> dict:
        result = self._transport(method=method, path=path, **kwargs)
        if hasattr(result, "__await__"):
            return await result
        return result

    def _default_transport(self, *, method: str, path: str, access_token: str = "", **kwargs) -> dict:
        if cffi_requests is None:
            raise TeamClientError("curl_cffi 不可用，无法创建默认 Team API transport")

        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {access_token}",
            "Origin": _TEAM_API_BASE_URL,
            "Referer": f"{_TEAM_API_BASE_URL}/",
        }
        url = f"{_TEAM_API_BASE_URL}{path}"

        try:
            response = cffi_requests.request(method, url, headers=headers, impersonate="chrome", **kwargs)
        except Exception as exc:  # pragma: no cover - 网络异常分支通过运行时保护
            raise TeamClientError(f"Team API 请求失败: {exc}") from exc

        payload = response.json()
        self.raise_for_error(
            status_code=getattr(response, "status_code", 200),
            payload=payload,
            text=self._as_text(getattr(response, "text", "")),
        )
        return self._ensure_dict(payload, "transport")

    @staticmethod
    def _is_team_account(*, plan_type: str, subscription_plan: str) -> bool:
        return plan_type.lower() == "team" or subscription_plan.lower() == "team"

    @staticmethod
    def _ensure_dict(payload: Any, context: str) -> dict:
        if not isinstance(payload, dict):
            raise TeamResponseFormatError(f"Team {context} 响应不是对象")
        return payload

    def _extract_items(self, payload: Any, *, context: str) -> List[Any]:
        body = self._ensure_dict(payload, context)
        items = body.get("items")
        if not isinstance(items, list):
            raise TeamResponseFormatError(f"Team {context} 响应中的 items 不是列表")
        return items

    def _extract_error_message(self, payload: Any, *, text: str) -> str:
        if isinstance(payload, dict):
            detail = self._as_text(payload.get("detail"))
            if detail:
                return detail

            error = payload.get("error")
            if isinstance(error, str):
                error_text = self._as_text(error)
                if error_text:
                    return error_text
            elif isinstance(error, dict):
                error_message = self._as_text(error.get("message"))
                if error_message:
                    return error_message

                error_code = self._as_text(error.get("code"))
                if error_code:
                    return error_code

        return self._as_text(text) or "上游 Team API 请求失败"

    @staticmethod
    def _as_text(value: Any) -> str:
        return str(value or "").strip()

    @staticmethod
    def _as_int(value: Any) -> Optional[int]:
        if value in (None, ""):
            return None
        try:
            return int(value)
        except (TypeError, ValueError):
            return None
