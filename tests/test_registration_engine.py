import base64
import json
from types import SimpleNamespace

from src.config.constants import EmailServiceType, OPENAI_API_ENDPOINTS, OPENAI_PAGE_TYPES
from src.core.http_client import OpenAIHTTPClient
from src.core.openai.oauth import OAuthStart
from src.core import register as register_module
from src.core.register import RegistrationEngine
from src.services.base import BaseEmailService


class DummyResponse:
    def __init__(self, status_code=200, payload=None, text="", headers=None, on_return=None):
        self.status_code = status_code
        self._payload = payload
        self.text = text
        self.headers = headers or {}
        self.on_return = on_return

    def json(self):
        if self._payload is None:
            raise ValueError("no json payload")
        return self._payload


class QueueSession:
    def __init__(self, steps):
        self.steps = list(steps)
        self.calls = []
        self.cookies = {}

    def get(self, url, **kwargs):
        return self._request("GET", url, **kwargs)

    def post(self, url, **kwargs):
        return self._request("POST", url, **kwargs)

    def request(self, method, url, **kwargs):
        return self._request(method.upper(), url, **kwargs)

    def close(self):
        return None

    def _request(self, method, url, **kwargs):
        self.calls.append({
            "method": method,
            "url": url,
            "kwargs": kwargs,
        })
        if not self.steps:
            raise AssertionError(f"unexpected request: {method} {url}")
        expected_method, expected_url, response = self.steps.pop(0)
        assert method == expected_method
        assert url == expected_url
        if callable(response):
            response = response(self)
        if response.on_return:
            response.on_return(self)
        return response


class FakeEmailService(BaseEmailService):
    def __init__(self, codes):
        super().__init__(EmailServiceType.TEMPMAIL)
        self.codes = list(codes)
        self.otp_requests = []

    def create_email(self, config=None):
        return {
            "email": "tester@example.com",
            "service_id": "mailbox-1",
        }

    def get_verification_code(self, email, email_id=None, timeout=120, pattern=r"(?<!\d)(\d{6})(?!\d)", otp_sent_at=None):
        self.otp_requests.append({
            "email": email,
            "email_id": email_id,
            "otp_sent_at": otp_sent_at,
        })
        if not self.codes:
            raise AssertionError("no verification code queued")
        return self.codes.pop(0)

    def list_emails(self, **kwargs):
        return []

    def delete_email(self, email_id):
        return True

    def check_health(self):
        return True


class FakeOAuthManager:
    def __init__(self):
        self.start_calls = 0
        self.callback_calls = []

    def start_oauth(self):
        self.start_calls += 1
        return OAuthStart(
            auth_url=f"https://auth.example.test/flow/{self.start_calls}",
            state=f"state-{self.start_calls}",
            code_verifier=f"verifier-{self.start_calls}",
            redirect_uri="http://localhost:1455/auth/callback",
        )

    def handle_callback(self, callback_url, expected_state, code_verifier):
        self.callback_calls.append({
            "callback_url": callback_url,
            "expected_state": expected_state,
            "code_verifier": code_verifier,
        })
        return {
            "account_id": "acct-1",
            "access_token": "access-1",
            "refresh_token": "refresh-1",
            "id_token": "id-1",
        }


class FakeOpenAIClient:
    def __init__(self, sessions, sentinel_tokens):
        self._sessions = list(sessions)
        self._session_index = 0
        self._session = self._sessions[0]
        self._sentinel_tokens = list(sentinel_tokens)

    @property
    def session(self):
        return self._session

    def check_ip_location(self):
        return True, "US"

    def check_sentinel(self, did):
        if not self._sentinel_tokens:
            raise AssertionError("no sentinel token queued")
        return self._sentinel_tokens.pop(0)

    def build_sentinel_header_token(self, did, challenge_token, *, flow):
        return json.dumps({
            "p": "fake-pow",
            "t": "",
            "c": challenge_token,
            "id": did,
            "flow": flow,
        })

    def close(self):
        if self._session_index + 1 < len(self._sessions):
            self._session_index += 1
            self._session = self._sessions[self._session_index]


def _workspace_cookie(workspace_id):
    payload = base64.urlsafe_b64encode(
        json.dumps({"workspaces": [{"id": workspace_id}]}).encode("utf-8")
    ).decode("ascii").rstrip("=")
    return f"{payload}.sig"


def _response_with_did(did):
    return DummyResponse(
        status_code=200,
        text="ok",
        on_return=lambda session: session.cookies.__setitem__("oai-did", did),
    )


def _response_with_login_cookies(workspace_id="ws-1", session_token="session-1"):
    def setter(session):
        session.cookies["oai-client-auth-session"] = _workspace_cookie(workspace_id)
        session.cookies["__Secure-next-auth.session-token"] = session_token

    return DummyResponse(status_code=200, payload={}, on_return=setter)


def test_check_sentinel_sends_non_empty_pow(monkeypatch):
    session = QueueSession([
        ("POST", OPENAI_API_ENDPOINTS["sentinel"], DummyResponse(payload={"token": "sentinel-token"})),
    ])
    client = OpenAIHTTPClient()
    client._session = session

    monkeypatch.setattr(
        "src.core.http_client.build_sentinel_pow_token",
        lambda user_agent: "gAAAAACpow-token",
    )

    token = client.check_sentinel("device-1")

    assert token == "sentinel-token"
    body = json.loads(session.calls[0]["kwargs"]["data"])
    assert body["id"] == "device-1"
    assert body["flow"] == "authorize_continue"
    assert body["p"] == "gAAAAACpow-token"


def _assert_complete_sentinel_header(headers, *, expected_device_id, expected_flow, expected_challenge):
    sentinel_header = headers.get("openai-sentinel-token")
    assert sentinel_header
    sentinel_payload = json.loads(sentinel_header)
    assert sentinel_payload == {
        "p": "gAAAAACpow-token",
        "t": "",
        "c": expected_challenge,
        "id": expected_device_id,
        "flow": expected_flow,
    }


def test_native_auth_start_sends_complete_sentinel_header(monkeypatch):
    session = QueueSession([
        (
            "POST",
            OPENAI_API_ENDPOINTS["signup"],
            DummyResponse(payload={"page": {"type": OPENAI_PAGE_TYPES["PASSWORD_REGISTRATION"]}}),
        ),
        (
            "POST",
            OPENAI_API_ENDPOINTS["signup"],
            DummyResponse(payload={"page": {"type": OPENAI_PAGE_TYPES["LOGIN_PASSWORD"]}}),
        ),
    ])
    engine = RegistrationEngine(FakeEmailService([]))
    engine.session = session
    engine.email = "tester@example.com"

    monkeypatch.setattr(
        "src.core.http_client.build_sentinel_pow_token",
        lambda user_agent: "gAAAAACpow-token",
    )

    signup_result = engine._submit_signup_form("device-auth", "challenge-signup")
    login_result = engine._submit_login_start("device-auth", "challenge-login")

    assert signup_result.success is True
    assert login_result.success is True

    signup_headers = session.calls[0]["kwargs"]["headers"]
    login_headers = session.calls[1]["kwargs"]["headers"]
    _assert_complete_sentinel_header(
        signup_headers,
        expected_device_id="device-auth",
        expected_flow="authorize_continue",
        expected_challenge="challenge-signup",
    )
    _assert_complete_sentinel_header(
        login_headers,
        expected_device_id="device-auth",
        expected_flow="authorize_continue",
        expected_challenge="challenge-login",
    )


def test_native_register_password_sends_device_id_and_complete_sentinel_header(monkeypatch):
    session = QueueSession([
        ("POST", OPENAI_API_ENDPOINTS["register"], DummyResponse(payload={})),
    ])
    engine = RegistrationEngine(FakeEmailService([]))
    engine.session = session
    engine.email = "tester@example.com"

    monkeypatch.setattr(engine, "_generate_password", lambda: "Passw0rd!123")
    monkeypatch.setattr(
        "src.core.http_client.build_sentinel_pow_token",
        lambda user_agent: "gAAAAACpow-token",
    )

    success, password = engine._register_password("device-register", "challenge-register")

    assert success is True
    assert password == "Passw0rd!123"

    headers = session.calls[0]["kwargs"]["headers"]
    assert headers["oai-device-id"] == "device-register"
    _assert_complete_sentinel_header(
        headers,
        expected_device_id="device-register",
        expected_flow="username_password_create",
        expected_challenge="challenge-register",
    )


def test_native_password_verify_sends_device_id_and_complete_sentinel_header(monkeypatch):
    session = QueueSession([
        (
            "POST",
            OPENAI_API_ENDPOINTS["signup"],
            DummyResponse(payload={"page": {"type": OPENAI_PAGE_TYPES["LOGIN_PASSWORD"]}}),
        ),
        (
            "POST",
            OPENAI_API_ENDPOINTS["password_verify"],
            DummyResponse(payload={"page": {"type": OPENAI_PAGE_TYPES["LOGIN_PASSWORD"]}}),
        ),
    ])
    engine = RegistrationEngine(FakeEmailService([]))
    engine.session = session
    engine.email = "tester@example.com"
    engine.password = "known-pass"
    engine.device_id = "device-login"

    monkeypatch.setattr(
        "src.core.http_client.build_sentinel_pow_token",
        lambda user_agent: "gAAAAACpow-token",
    )

    start_result = engine._submit_login_start("device-login", "challenge-login")
    verify_result = engine._submit_login_password()

    assert start_result.success is True
    assert verify_result.success is True

    headers = session.calls[1]["kwargs"]["headers"]
    assert headers["oai-device-id"] == "device-login"
    _assert_complete_sentinel_header(
        headers,
        expected_device_id="device-login",
        expected_flow="password_verify",
        expected_challenge="challenge-login",
    )


def test_run_registers_then_relogs_to_fetch_token(monkeypatch):
    captured = {}

    class StubAnyAutoRegistrationEngine:
        def __init__(
            self,
            email_service,
            proxy_url=None,
            callback_logger=None,
            check_cancelled=None,
            max_retries=3,
            browser_mode="protocol",
            extra_config=None,
        ):
            captured["check_cancelled"] = check_cancelled
            self.email_info = {"service_id": "mailbox-1"}
            self.email = "tester@example.com"
            self.inbox_email = "tester@example.com"
            self.password = "Passw0rd!123"
            self.session = None
            self.device_id = "device-2"

        def run(self):
            return {
                "success": True,
                "source": "register",
                "access_token": "access-2",
                "refresh_token": "refresh-2",
                "id_token": "id-2",
                "session_token": "session-1",
                "account_id": "acct-1",
                "workspace_id": "ws-1",
                "metadata": {
                    "token_acquired_via_relogin": True,
                },
            }

    monkeypatch.setattr(register_module, "AnyAutoRegistrationEngine", StubAnyAutoRegistrationEngine)
    monkeypatch.setattr(
        register_module,
        "get_settings",
        lambda: SimpleNamespace(
            registration_max_retries=1,
            openai_client_id="client-1",
            openai_auth_url="https://auth.example.test",
            openai_token_url="https://auth.example.test/oauth/token",
            openai_redirect_uri="http://localhost:1455/auth/callback",
            openai_scope="openid profile email offline_access",
        ),
    )

    engine = RegistrationEngine(FakeEmailService(["123456"]))
    result = engine.run()

    assert result.success is True
    assert result.source == "register"
    assert result.workspace_id == "ws-1"
    assert result.session_token == "session-1"
    assert result.password == "Passw0rd!123"
    assert result.metadata["token_acquired_via_relogin"] is True
    assert callable(captured["check_cancelled"])


def test_existing_account_login_uses_auto_sent_otp_without_manual_send(monkeypatch):
    class StubAnyAutoRegistrationEngine:
        def __init__(
            self,
            email_service,
            proxy_url=None,
            callback_logger=None,
            check_cancelled=None,
            max_retries=3,
            browser_mode="protocol",
            extra_config=None,
        ):
            self.email_info = {"service_id": "mailbox-1"}
            self.email = "tester@example.com"
            self.inbox_email = "tester@example.com"
            self.password = "known-pass"
            self.session = None
            self.device_id = "device-login"

        def run(self):
            return {
                "success": True,
                "source": "login",
                "access_token": "access-login",
                "refresh_token": "refresh-login",
                "id_token": "id-login",
                "session_token": "session-existing",
                "account_id": "acct-existing",
                "workspace_id": "ws-existing",
                "metadata": {
                    "token_acquired_via_relogin": False,
                },
            }

    monkeypatch.setattr(register_module, "AnyAutoRegistrationEngine", StubAnyAutoRegistrationEngine)
    monkeypatch.setattr(
        register_module,
        "get_settings",
        lambda: SimpleNamespace(
            registration_max_retries=1,
            openai_client_id="client-1",
            openai_auth_url="https://auth.example.test",
            openai_token_url="https://auth.example.test/oauth/token",
            openai_redirect_uri="http://localhost:1455/auth/callback",
            openai_scope="openid profile email offline_access",
        ),
    )

    engine = RegistrationEngine(FakeEmailService(["246810"]))
    result = engine.run()

    assert result.success is True
    assert result.source == "login"
    assert result.session_token == "session-existing"
    assert result.metadata["token_acquired_via_relogin"] is False


def test_run_propagates_anyauto_login_source(monkeypatch):
    class StubAnyAutoRegistrationEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, check_cancelled=None, max_retries=3, browser_mode="protocol", extra_config=None):
            self.email_service = email_service
            self.proxy_url = proxy_url
            self.callback_logger = callback_logger
            self.check_cancelled = check_cancelled
            self.max_retries = max_retries
            self.browser_mode = browser_mode
            self.extra_config = extra_config
            self.email_info = {"service_id": "mailbox-1"}
            self.email = "tester@example.com"
            self.inbox_email = "tester@example.com"
            self.password = "known-pass"
            self.session = None
            self.device_id = "device-login"

        def run(self):
            return {
                "success": True,
                "source": "login",
                "access_token": "access-login",
                "refresh_token": "refresh-login",
                "id_token": "id-login",
                "session_token": "session-login",
                "account_id": "acct-login",
                "workspace_id": "ws-login",
                "metadata": {
                    "existing_account_detected": True,
                },
            }

    monkeypatch.setattr(register_module, "AnyAutoRegistrationEngine", StubAnyAutoRegistrationEngine)
    monkeypatch.setattr(
        register_module,
        "get_settings",
        lambda: SimpleNamespace(
            registration_max_retries=1,
            openai_client_id="client-1",
            openai_auth_url="https://auth.example.test",
            openai_token_url="https://auth.example.test/oauth/token",
            openai_redirect_uri="http://localhost:1455/auth/callback",
            openai_scope="openid profile email offline_access",
        ),
    )

    engine = RegistrationEngine(FakeEmailService(["123456"]))
    result = engine.run()

    assert result.success is True
    assert result.source == "login"
    assert result.password == "known-pass"
    assert result.metadata["existing_account_detected"] is True


def test_run_returns_cancelled_error_when_check_cancelled_trips(monkeypatch):
    class StubAnyAutoRegistrationEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, check_cancelled=None, max_retries=3, browser_mode="protocol", extra_config=None):
            self.check_cancelled = check_cancelled
            self.email_info = {"service_id": "mailbox-1"}
            self.email = "tester@example.com"
            self.inbox_email = "tester@example.com"
            self.password = "known-pass"
            self.session = None
            self.device_id = "device-login"

        def run(self):
            self.check_cancelled()
            raise AssertionError("should stop before here")

    monkeypatch.setattr(register_module, "AnyAutoRegistrationEngine", StubAnyAutoRegistrationEngine)
    monkeypatch.setattr(
        register_module,
        "get_settings",
        lambda: SimpleNamespace(
            registration_max_retries=1,
            openai_client_id="client-1",
            openai_auth_url="https://auth.example.test",
            openai_token_url="https://auth.example.test/oauth/token",
            openai_redirect_uri="http://localhost:1455/auth/callback",
            openai_scope="openid profile email offline_access",
        ),
    )

    engine = RegistrationEngine(FakeEmailService(["123456"]), check_cancelled=lambda: True)
    result = engine.run()

    assert result.success is False
    assert result.error_message == "任务已取消，停止继续执行"
