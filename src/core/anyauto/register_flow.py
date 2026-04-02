"""
Any-auto-register 风格注册流程（V2）。
以状态机 + Session 复用为主，必要时回退 OAuth。
"""

from __future__ import annotations

import secrets
import time
from datetime import datetime
from typing import Optional, Callable, Dict, Any

from .chatgpt_client import ChatGPTClient
from .oauth_client import OAuthClient
from .utils import generate_random_name, generate_random_birthday, decode_jwt_payload
from ...config.constants import PASSWORD_CHARSET, DEFAULT_PASSWORD_LENGTH
from ...config.settings import get_settings
from ...database import crud
from ...database.session import get_db


class EmailServiceAdapter:
    """将 codex-console 邮箱服务适配成 any-auto-register 预期接口。"""

    def __init__(self, email_service, email: str, email_id: Optional[str], log_fn: Callable[[str], None]):
        self.es = email_service
        self.email = email
        self.email_id = email_id
        self.log_fn = log_fn or (lambda _msg: None)
        self._used_codes: set[str] = set()

    def wait_for_verification_code(self, email, timeout=60, otp_sent_at=None, exclude_codes=None):
        exclude = set(exclude_codes or [])
        exclude.update(self._used_codes)
        deadline = time.time() + max(1, int(timeout))
        sent_at = otp_sent_at or time.time()

        while time.time() < deadline:
            remaining = max(1, int(deadline - time.time()))
            code = self.es.get_verification_code(
                email=email,
                email_id=self.email_id,
                timeout=remaining,
                otp_sent_at=sent_at,
            )
            if not code:
                return None
            if code in exclude:
                exclude.add(code)
                continue
            self._used_codes.add(code)
            self.log_fn(f"成功获取验证码: {code}")
            return code
        return None


class AnyAutoRegistrationEngine:
    def __init__(
        self,
        email_service,
        proxy_url: Optional[str] = None,
        callback_logger: Optional[Callable[[str], None]] = None,
        max_retries: int = 3,
        browser_mode: str = "protocol",
        extra_config: Optional[Dict[str, Any]] = None,
    ):
        self.email_service = email_service
        self.proxy_url = proxy_url
        self.callback_logger = callback_logger or (lambda _msg: None)
        self.max_retries = max(1, int(max_retries or 1))
        self.browser_mode = browser_mode or "protocol"
        self.extra_config = dict(extra_config or {})

        self.email: Optional[str] = None
        self.inbox_email: Optional[str] = None
        self.email_info: Optional[Dict[str, Any]] = None
        self.password: Optional[str] = None
        self.session = None
        self.device_id: Optional[str] = None

    def _log(self, message: str):
        if self.callback_logger:
            self.callback_logger(message)

    @staticmethod
    def _build_password(length: int) -> str:
        length = max(8, int(length or DEFAULT_PASSWORD_LENGTH))
        return "".join(secrets.choice(PASSWORD_CHARSET) for _ in range(length))

    @staticmethod
    def _should_retry(message: str) -> bool:
        text = str(message or "").lower()
        retriable_markers = [
            "tls",
            "ssl",
            "curl: (35)",
            "预授权被拦截",
            "authorize",
            "registration_disallowed",
            "http 400",
            "创建账号失败",
            "未获取到 authorization code",
            "consent",
            "workspace",
            "organization",
            "otp",
            "验证码",
            "session",
            "accesstoken",
            "next-auth",
        ]
        return any(marker.lower() in text for marker in retriable_markers)

    @staticmethod
    def _is_existing_account_error(message: str) -> bool:
        text = str(message or "").lower()
        return any(
            marker in text
            for marker in (
                "user_exists",
                "already registered",
                "already exists",
                "email already exists",
                "该邮箱已存在",
                "邮箱已注册",
                "已注册账号",
            )
        )

    def _resolve_existing_account_password(self, email: str) -> tuple[str, str]:
        configured_password = str(
            self.extra_config.get("existing_account_password")
            or self.extra_config.get("login_password")
            or ""
        ).strip()
        if configured_password:
            return configured_password, "extra_config"

        try:
            with get_db() as db:
                account = crud.get_account_by_email(db, email)
                stored_password = str(getattr(account, "password", "") or "").strip() if account else ""
                if stored_password:
                    return stored_password, "database"
        except Exception as exc:
            self._log(f"回填已注册账号密码失败: {exc}")

        return "", ""

    @staticmethod
    def _extract_account_id_from_token(token: str) -> str:
        payload = decode_jwt_payload(token)
        if not isinstance(payload, dict):
            return ""
        auth_claims = payload.get("https://api.openai.com/auth") or {}
        for key in ("chatgpt_account_id", "account_id", "workspace_id"):
            value = str(auth_claims.get(key) or payload.get(key) or "").strip()
            if value:
                return value
        return ""

    @staticmethod
    def _is_phone_required_error(message: str) -> bool:
        text = str(message or "").lower()
        return any(
            marker in text
            for marker in (
                "add_phone",
                "add-phone",
                "phone",
                "phone required",
                "phone verification",
                "手机号",
            )
        )

    def _passwordless_oauth_reauth(
        self,
        chatgpt_client: ChatGPTClient,
        email: str,
        skymail_adapter: EmailServiceAdapter,
        oauth_config: Dict[str, Any],
    ) -> Optional[Dict[str, Any]]:
        self._log("检测到 add_phone，尝试 passwordless OTP 登录补全 workspace...")
        oauth_client = OAuthClient(
            config=oauth_config,
            proxy=self.proxy_url,
            verbose=False,
            browser_mode=self.browser_mode,
        )
        oauth_client._log = self._log
        oauth_client.session = chatgpt_client.session

        tokens = oauth_client.login_passwordless_and_get_tokens(
            email,
            chatgpt_client.device_id,
            chatgpt_client.ua,
            chatgpt_client.sec_ch_ua,
            chatgpt_client.impersonate,
            skymail_adapter,
        )
        if tokens and tokens.get("access_token"):
            return {
                "access_token": tokens.get("access_token", ""),
                "refresh_token": tokens.get("refresh_token", ""),
                "id_token": tokens.get("id_token", ""),
                "session": oauth_client.session,
            }

        if oauth_client.last_error:
            self._log(f"Passwordless OAuth 失败: {oauth_client.last_error}")
        return None

    @staticmethod
    def _extract_create_account_result(chatgpt_client: ChatGPTClient) -> Dict[str, Any]:
        return {
            "refresh_token": str(getattr(chatgpt_client, "last_create_account_refresh_token", "") or "").strip(),
            "callback_url": str(getattr(chatgpt_client, "last_create_account_callback_url", "") or "").strip(),
            "continue_url": str(getattr(chatgpt_client, "last_create_account_continue_url", "") or "").strip(),
            "account_id": str(getattr(chatgpt_client, "last_create_account_account_id", "") or "").strip(),
            "workspace_id": str(getattr(chatgpt_client, "last_create_account_workspace_id", "") or "").strip(),
            "raw": getattr(chatgpt_client, "last_create_account_data", {}) or {},
        }

    def _run_oauth_token_completion(
        self,
        chatgpt_client: ChatGPTClient,
        email: str,
        password: str,
        skymail_adapter: EmailServiceAdapter,
        oauth_config: Dict[str, Any],
        *,
        reason: str,
        allow_passwordless: bool = True,
    ) -> Dict[str, Any]:
        last_error = ""
        last_session = chatgpt_client.session

        for oauth_attempt in range(2):
            if oauth_attempt > 0:
                self._log(f"{reason} 第 {oauth_attempt + 1}/2 次重试...")
                time.sleep(1)

            oauth_client = OAuthClient(
                config=oauth_config,
                proxy=self.proxy_url,
                verbose=False,
                browser_mode=self.browser_mode,
            )
            oauth_client._log = self._log
            oauth_client.session = last_session

            tokens = oauth_client.login_and_get_tokens(
                email,
                password,
                chatgpt_client.device_id,
                chatgpt_client.ua,
                chatgpt_client.sec_ch_ua,
                chatgpt_client.impersonate,
                skymail_adapter,
            )
            last_session = oauth_client.session
            if tokens and tokens.get("access_token"):
                return {
                    "tokens": tokens,
                    "session": oauth_client.session,
                    "last_error": "",
                    "source": "oauth_password",
                }

            last_error = str(getattr(oauth_client, "last_error", "") or "").strip()
            if allow_passwordless and self._is_phone_required_error(last_error):
                self._log(f"{reason} 命中手机号/风控，切换 passwordless OAuth 补齐...")
                pwdless = self._passwordless_oauth_reauth(
                    chatgpt_client,
                    email,
                    skymail_adapter,
                    oauth_config,
                )
                if pwdless and pwdless.get("access_token"):
                    return {
                        "tokens": pwdless,
                        "session": pwdless.get("session") or last_session,
                        "last_error": "",
                        "source": "oauth_passwordless",
                    }

        return {
            "tokens": None,
            "session": last_session,
            "last_error": last_error,
            "source": "",
        }

    def _build_session_success_result(self, session_result: Dict[str, Any]) -> Dict[str, Any]:
        account_id = str(session_result.get("account_id", "") or "").strip()
        if not account_id:
            account_id = str(session_result.get("workspace_id", "") or "").strip()
        if not account_id:
            account_id = self._extract_account_id_from_token(session_result.get("access_token", ""))

        workspace_id = str(session_result.get("workspace_id", "") or "").strip() or account_id
        return {
            "success": True,
            "access_token": session_result.get("access_token", ""),
            "session_token": session_result.get("session_token", ""),
            "account_id": account_id,
            "workspace_id": workspace_id,
            "metadata": {
                "auth_provider": session_result.get("auth_provider", ""),
                "expires": session_result.get("expires", ""),
                "user_id": session_result.get("user_id", ""),
                "user": session_result.get("user") or {},
                "account": session_result.get("account") or {},
                "raw_session": session_result.get("raw_session") or {},
            },
        }

    def _merge_success_result(
        self,
        base_result: Dict[str, Any],
        *,
        create_account_result: Optional[Dict[str, Any]] = None,
        oauth_tokens: Optional[Dict[str, Any]] = None,
        token_source: str = "",
    ) -> Dict[str, Any]:
        merged = dict(base_result or {})
        metadata = dict(merged.get("metadata") or {})
        create_account_result = dict(create_account_result or {})
        oauth_tokens = dict(oauth_tokens or {})
        base_access_token = str(merged.get("access_token") or "").strip()
        base_session_token = str(merged.get("session_token") or "").strip()
        oauth_access_token = str(oauth_tokens.get("access_token") or "").strip()

        access_token = str(
            oauth_access_token
            or base_access_token
            or ""
        ).strip()
        refresh_token = str(
            oauth_tokens.get("refresh_token")
            or create_account_result.get("refresh_token")
            or merged.get("refresh_token")
            or ""
        ).strip()
        id_token = str(
            oauth_tokens.get("id_token")
            or merged.get("id_token")
            or ""
        ).strip()
        session_token = str(
            base_session_token
            or oauth_tokens.get("session_token")
            or ""
        ).strip()

        account_id = str(
            oauth_tokens.get("account_id")
            or create_account_result.get("account_id")
            or merged.get("account_id")
            or self._extract_account_id_from_token(oauth_access_token)
            or self._extract_account_id_from_token(base_access_token)
            or ""
        ).strip()
        if not account_id:
            account_id = self._extract_account_id_from_token(access_token)

        explicit_workspace_id = str(
            oauth_tokens.get("workspace_id")
            or create_account_result.get("workspace_id")
            or ""
        ).strip()
        workspace_id = str(
            explicit_workspace_id
            or merged.get("workspace_id")
            or account_id
            or ""
        ).strip()

        merged.update({
            "access_token": access_token,
            "refresh_token": refresh_token,
            "id_token": id_token,
            "session_token": session_token,
            "account_id": account_id,
            "workspace_id": workspace_id,
        })

        if create_account_result.get("callback_url"):
            metadata["create_account_callback_url"] = create_account_result.get("callback_url")
        if create_account_result.get("continue_url"):
            metadata["create_account_continue_url"] = create_account_result.get("continue_url")
        metadata["access_token_source"] = "oauth" if oauth_access_token else "session_reuse"
        if session_token:
            metadata["session_token_source"] = "session_reuse" if base_session_token else "oauth"
        if token_source:
            metadata["refresh_token_source"] = token_source
        metadata["refresh_token_acquired"] = bool(refresh_token)
        metadata["has_session_token"] = bool(session_token)
        merged["metadata"] = metadata
        return merged

    def _build_oauth_success_result(
        self,
        chatgpt_client: ChatGPTClient,
        oauth_config: Dict[str, Any],
        oauth_completion: Dict[str, Any],
        *,
        create_account_result: Optional[Dict[str, Any]] = None,
        success_log: str,
        source: str = "register",
        metadata_extra: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        tokens = oauth_completion.get("tokens") or {}
        self.session = oauth_completion.get("session") or self.session
        self._log(success_log)

        workspace_id = ""
        session_cookie = ""
        oauth_client = None
        try:
            oauth_client = OAuthClient(
                config=oauth_config,
                proxy=self.proxy_url,
                verbose=False,
                browser_mode=self.browser_mode,
            )
            oauth_client.session = self.session
            session_data = oauth_client._decode_oauth_session_cookie()
            if session_data:
                workspaces = session_data.get("workspaces", [])
                if workspaces:
                    workspace_id = str((workspaces[0] or {}).get("id") or "")
                    if workspace_id:
                        self._log(f"成功萃取 Workspace ID: {workspace_id}")
        except Exception:
            pass

        try:
            cookie_jar = getattr(getattr(oauth_client, "session", None), "cookies", None)
            for cookie in getattr(cookie_jar, "jar", []):
                if cookie.name == "__Secure-next-auth.session-token":
                    session_cookie = cookie.value
                    break
        except Exception:
            pass

        account_id = self._extract_account_id_from_token(tokens.get("access_token", "")) or workspace_id
        merged = self._merge_success_result(
            {
                "success": True,
                "access_token": tokens.get("access_token", ""),
                "account_id": account_id or ("v2_acct_" + chatgpt_client.device_id[:8]),
                "workspace_id": workspace_id or account_id,
                "session_token": session_cookie,
            },
            create_account_result=create_account_result,
            oauth_tokens=tokens,
            token_source=str(oauth_completion.get("source") or "oauth"),
        )
        merged["source"] = source
        if metadata_extra:
            metadata = dict(merged.get("metadata") or {})
            metadata.update(metadata_extra)
            merged["metadata"] = metadata
        return merged

    @staticmethod
    def _mark_existing_account_result(
        result: Dict[str, Any],
        *,
        password_source: str = "",
    ) -> Dict[str, Any]:
        marked = dict(result or {})
        marked["source"] = "login"
        metadata = dict(marked.get("metadata") or {})
        metadata["existing_account_detected"] = True
        if password_source:
            metadata["login_password_source"] = password_source
        marked["metadata"] = metadata
        return marked

    def run(self):
        """
        执行 any-auto-register 风格注册流程。
        返回 dict：包含 result(RegistrationResult 填充所需字段) + 额外上下文。
        """
        last_error = ""
        settings = get_settings()
        password_len = int(getattr(settings, "registration_default_password_length", DEFAULT_PASSWORD_LENGTH) or DEFAULT_PASSWORD_LENGTH)

        oauth_config = dict(self.extra_config or {})
        if not oauth_config:
            oauth_config = {
                "oauth_issuer": str(getattr(settings, "openai_auth_url", "") or "https://auth.openai.com"),
                "oauth_client_id": str(getattr(settings, "openai_client_id", "") or "app_EMoamEEZ73f0CkXaXp7hrann"),
                "oauth_redirect_uri": str(getattr(settings, "openai_redirect_uri", "") or "http://localhost:1455/auth/callback"),
            }

        for attempt in range(self.max_retries):
            try:
                if attempt == 0:
                    self._log("=" * 60)
                    self._log("开始注册流程 V2 (Session 复用直取 AccessToken)")
                    self._log(f"请求模式: {self.browser_mode}")
                    self._log("=" * 60)
                else:
                    self._log(f"整流程重试 {attempt + 1}/{self.max_retries} ...")
                    time.sleep(1)

                # 1. 创建邮箱
                self.email_info = self.email_service.create_email()
                raw_email = str((self.email_info or {}).get("email") or "").strip()
                if not raw_email:
                    last_error = "创建邮箱失败"
                    return {"success": False, "error_message": last_error}

                normalized_email = raw_email.lower()
                self.inbox_email = raw_email
                self.email = normalized_email
                try:
                    self.email_info["email"] = normalized_email
                except Exception:
                    pass

                if raw_email != normalized_email:
                    self._log(f"邮箱规范化: {raw_email} -> {normalized_email}")

                # 2. 生成密码 & 用户信息
                self.password = self.password or self._build_password(password_len)
                first_name, last_name = generate_random_name()
                birthdate = generate_random_birthday()
                self._log(f"邮箱: {normalized_email}, 密码: {self.password}")
                self._log(f"注册信息: {first_name} {last_name}, 生日: {birthdate}")

                # 3. 邮箱适配器
                email_id = (self.email_info or {}).get("service_id")
                skymail_adapter = EmailServiceAdapter(self.email_service, normalized_email, email_id, self._log)

                # 4. 注册状态机
                chatgpt_client = ChatGPTClient(
                    proxy=self.proxy_url,
                    verbose=False,
                    browser_mode=self.browser_mode,
                )
                chatgpt_client._log = self._log

                self._log("步骤 1/2: 执行注册状态机...")
                success, msg = chatgpt_client.register_complete_flow(
                    normalized_email, self.password, first_name, last_name, birthdate, skymail_adapter
                )
                if not success:
                    if self._is_existing_account_error(msg):
                        self._log("检测到邮箱已注册，准备改走登录补 token")
                        self.session = chatgpt_client.session
                        self.device_id = chatgpt_client.device_id
                        create_account_result = self._extract_create_account_result(chatgpt_client)
                        login_password, password_source = self._resolve_existing_account_password(normalized_email)
                        if not login_password:
                            last_error = "检测到邮箱已注册，但当前任务未持有历史密码，无法自动切换登录补 token"
                            return {"success": False, "error_message": last_error}

                        self.password = login_password
                        self._log(f"已切换到登录密码，来源: {password_source}")
                        oauth_completion = self._run_oauth_token_completion(
                            chatgpt_client,
                            normalized_email,
                            self.password,
                            skymail_adapter,
                            oauth_config,
                            reason="已注册账号回退 OAuth 登录",
                            allow_passwordless=True,
                        )
                        tokens = oauth_completion.get("tokens") or {}
                        if tokens.get("access_token"):
                            return self._build_oauth_success_result(
                                chatgpt_client,
                                oauth_config,
                                oauth_completion,
                                create_account_result=create_account_result,
                                success_log="已注册账号 OAuth 登录补全成功！",
                                source="login",
                                metadata_extra={
                                    "existing_account_detected": True,
                                    "login_password_source": password_source,
                                },
                            )

                        if self._is_phone_required_error(oauth_completion.get("last_error")):
                            self._log("已注册账号登录命中手机号验证，按成功返回并标记待补全")
                            return {
                                "success": True,
                                "source": "login",
                                "metadata": {
                                    "existing_account_detected": True,
                                    "login_password_source": password_source,
                                    "phone_verification_required": True,
                                    "token_pending": True,
                                    "oauth_error": oauth_completion.get("last_error"),
                                },
                            }

                        last_error = str(oauth_completion.get("last_error") or "").strip() or "已注册账号登录补 token 失败"
                        return {"success": False, "error_message": last_error}

                    last_error = f"注册流失败: {msg}"
                    if attempt < self.max_retries - 1 and self._should_retry(msg):
                        self._log(f"注册流失败，准备整流程重试: {msg}")
                        continue
                    return {"success": False, "error_message": last_error}

                add_phone_required = "add_phone" in str(msg or "").lower()
                try:
                    state = getattr(chatgpt_client, "last_registration_state", None)
                    if state:
                        target = f"{getattr(state, 'continue_url', '')} {getattr(state, 'current_url', '')}".lower()
                        if "add-phone" in target or "add_phone" in str(getattr(state, "page_type", "")).lower():
                            add_phone_required = True
                except Exception:
                    pass

                # 保存会话与设备
                self.session = chatgpt_client.session
                self.device_id = chatgpt_client.device_id
                create_account_result = self._extract_create_account_result(chatgpt_client)
                existing_account_detected = bool(getattr(chatgpt_client, "last_existing_account_detected", False))
                password_source = ""
                if existing_account_detected:
                    existing_password, password_source = self._resolve_existing_account_password(normalized_email)
                    if existing_password:
                        self.password = existing_password
                        self._log(f"检测到已有账号登录态，OAuth 补 token 改用历史密码，来源: {password_source}")
                    else:
                        self.password = ""
                        self._log("检测到已有账号登录态，但本地未找到历史密码；后续仅保留会话复用结果",)
                if create_account_result.get("refresh_token"):
                    self._log("注册链路在 create_account 阶段已拿到 refresh_token")

                if add_phone_required:
                    self._log("注册态命中 add_phone，仍先尝试复用当前会话；缺 refresh_token 时再走 OAuth 补齐")

                # 5. 复用 session 取 token
                self._log("步骤 2/2: 优先复用注册会话提取 ChatGPT Session / AccessToken...")
                session_ok, session_result = chatgpt_client.reuse_session_and_get_tokens()
                if session_ok:
                    self._log("会话复用成功，先保留 session/access_token，再补齐 refresh_token...")
                    base_result = self._build_session_success_result(session_result)
                    base_result = self._merge_success_result(
                        base_result,
                        create_account_result=create_account_result,
                        token_source="create_account" if create_account_result.get("refresh_token") else "",
                    )
                    if existing_account_detected:
                        base_result = self._mark_existing_account_result(
                            base_result,
                            password_source=password_source,
                        )

                    if base_result.get("refresh_token"):
                        self._log(f"无需额外 OAuth，refresh_token 已补齐，来源: {base_result.get('metadata', {}).get('refresh_token_source') or 'unknown'}")
                        return base_result

                    if existing_account_detected and not self.password:
                        metadata = dict(base_result.get("metadata") or {})
                        metadata["refresh_token_acquired"] = False
                        metadata["refresh_token_error"] = "缺少历史密码，跳过 OAuth 补齐"
                        base_result["metadata"] = metadata
                        self._log("已有账号缺少历史密码，跳过 OAuth 补 refresh_token，保留当前会话结果返回")
                        return base_result

                    oauth_completion = self._run_oauth_token_completion(
                        chatgpt_client,
                        normalized_email,
                        self.password,
                        skymail_adapter,
                        oauth_config,
                        reason="session 成功后的 OAuth 补 rt",
                        allow_passwordless=True,
                    )
                    self.session = oauth_completion.get("session") or self.session
                    tokens = oauth_completion.get("tokens") or {}
                    if tokens.get("access_token"):
                        merged_result = self._merge_success_result(
                            base_result,
                            create_account_result=create_account_result,
                            oauth_tokens=tokens,
                            token_source=str(oauth_completion.get("source") or "oauth"),
                        )
                        self._log(
                            f"OAuth 补齐完成，refresh_token={'已获取' if merged_result.get('refresh_token') else '仍缺失'}，"
                            f"来源: {oauth_completion.get('source') or 'oauth'}"
                        )
                        if existing_account_detected:
                            merged_result = self._mark_existing_account_result(
                                merged_result,
                                password_source=password_source,
                            )
                        return merged_result

                    metadata = dict(base_result.get("metadata") or {})
                    metadata["refresh_token_acquired"] = False
                    metadata["refresh_token_error"] = oauth_completion.get("last_error") or ""
                    if self._is_phone_required_error(oauth_completion.get("last_error")):
                        metadata["phone_verification_required"] = True
                    base_result["metadata"] = metadata
                    self._log("会话复用已成功，但 OAuth 仍未补齐 refresh_token，将保留当前会话结果返回")
                    return base_result

                # 6. OAuth 回退
                self._log(f"复用会话失败，回退到 OAuth 登录补全流程: {session_result}")
                oauth_completion = self._run_oauth_token_completion(
                    chatgpt_client,
                    normalized_email,
                    self.password,
                    skymail_adapter,
                    oauth_config,
                    reason="复用会话失败后的 OAuth 回退",
                    allow_passwordless=True,
                )
                tokens = oauth_completion.get("tokens") or {}
                self.session = oauth_completion.get("session") or self.session

                if tokens and tokens.get("access_token"):
                    return self._build_oauth_success_result(
                        chatgpt_client,
                        oauth_config,
                        oauth_completion,
                        create_account_result=create_account_result,
                        success_log="OAuth 回退补全成功！",
                    )

                # 7. 手机号验证需求：按成功返回，但标记为待补全
                if self._is_phone_required_error(oauth_completion.get("last_error")):
                    self._log("检测到手机号验证需求，按成功返回并标记待补全")
                    return {
                        "success": True,
                        "metadata": {
                            "phone_verification_required": True,
                            "token_pending": True,
                            "oauth_error": oauth_completion.get("last_error"),
                        },
                    }

                last_error = str(oauth_completion.get("last_error") or "").strip() or "获取最终 OAuth Tokens 失败"
                return {"success": False, "error_message": f"账号已创建成功，但 {last_error}"}

            except Exception as attempt_error:
                last_error = str(attempt_error)
                if attempt < self.max_retries - 1 and self._should_retry(last_error):
                    self._log(f"本轮出现异常，准备整流程重试: {last_error}")
                    continue
                return {"success": False, "error_message": last_error}

        return {"success": False, "error_message": last_error or "注册失败"}
