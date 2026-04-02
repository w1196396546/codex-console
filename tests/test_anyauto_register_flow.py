from contextlib import contextmanager
from types import SimpleNamespace

import src.core.anyauto.register_flow as register_flow_module
from src.core.anyauto.register_flow import AnyAutoRegistrationEngine


class DummyEmailService:
    def create_email(self):
        return {
            "email": "tester@example.com",
            "service_id": "mailbox-1",
        }

    def get_verification_code(self, **kwargs):
        return "123456"


class FakeChatGPTClient:
    reuse_result = (True, {})
    register_result = (True, "注册成功")
    existing_account_detected = False
    create_account_refresh_token = ""
    create_account_account_id = ""
    create_account_workspace_id = ""
    create_account_callback_url = ""
    create_account_continue_url = ""

    def __init__(self, proxy=None, verbose=True, browser_mode="protocol"):
        self.proxy = proxy
        self.verbose = verbose
        self.browser_mode = browser_mode
        self.device_id = "device-1"
        self.ua = "ua"
        self.sec_ch_ua = "sec"
        self.impersonate = "chrome"
        self.session = SimpleNamespace(cookies=SimpleNamespace(jar=[]))
        self.last_registration_state = SimpleNamespace(
            continue_url="https://chatgpt.com/",
            current_url="https://chatgpt.com/",
            page_type="chatgpt_home",
        )
        self.last_existing_account_detected = self.existing_account_detected
        self.last_create_account_refresh_token = self.create_account_refresh_token
        self.last_create_account_account_id = self.create_account_account_id
        self.last_create_account_workspace_id = self.create_account_workspace_id
        self.last_create_account_callback_url = self.create_account_callback_url
        self.last_create_account_continue_url = self.create_account_continue_url
        self.last_create_account_data = {}

    def _log(self, message):
        return None

    def register_complete_flow(self, email, password, first_name, last_name, birthdate, skymail_client):
        return self.register_result

    def reuse_session_and_get_tokens(self):
        return self.reuse_result


class FakeOAuthClient:
    plans = []
    call_count = 0
    last_login_email = None
    last_login_password = None

    def __init__(self, config, proxy=None, verbose=True, browser_mode="protocol"):
        self.config = config
        self.proxy = proxy
        self.verbose = verbose
        self.browser_mode = browser_mode
        self.session = SimpleNamespace(cookies=SimpleNamespace(jar=[]))
        self.last_error = ""

    def _log(self, message):
        return None

    def login_and_get_tokens(
        self,
        email,
        password,
        device_id,
        user_agent=None,
        sec_ch_ua=None,
        impersonate=None,
        skymail_client=None,
    ):
        FakeOAuthClient.call_count += 1
        FakeOAuthClient.last_login_email = email
        FakeOAuthClient.last_login_password = password
        if not FakeOAuthClient.plans:
            raise AssertionError("missing OAuth plan")
        plan = FakeOAuthClient.plans.pop(0)
        self.last_error = plan.get("last_error", "")
        return plan.get("tokens")

    def login_passwordless_and_get_tokens(
        self,
        email,
        device_id,
        user_agent=None,
        sec_ch_ua=None,
        impersonate=None,
        skymail_client=None,
    ):
        raise AssertionError("passwordless flow should not run in these tests")

    def _decode_oauth_session_cookie(self):
        return {}


def _settings():
    return SimpleNamespace(
        registration_default_password_length=12,
        openai_auth_url="https://auth.openai.com",
        openai_client_id="client-1",
        openai_redirect_uri="http://localhost:1455/auth/callback",
    )


def _build_engine(monkeypatch):
    monkeypatch.setattr("src.core.anyauto.register_flow.get_settings", _settings)
    monkeypatch.setattr("src.core.anyauto.register_flow.ChatGPTClient", FakeChatGPTClient)
    monkeypatch.setattr("src.core.anyauto.register_flow.OAuthClient", FakeOAuthClient)
    return AnyAutoRegistrationEngine(email_service=DummyEmailService(), callback_logger=lambda _msg: None)


def _patch_saved_account(monkeypatch, password):
    @contextmanager
    def fake_get_db():
        yield object()

    account = SimpleNamespace(password=password) if password is not None else None
    monkeypatch.setattr(register_flow_module, "get_db", fake_get_db, raising=False)
    monkeypatch.setattr(
        register_flow_module,
        "crud",
        SimpleNamespace(get_account_by_email=lambda db, email: account),
        raising=False,
    )


def test_run_uses_create_account_refresh_token_before_oauth(monkeypatch):
    FakeChatGPTClient.reuse_result = (
        True,
        {
            "access_token": "session-access-1",
            "session_token": "session-token-1",
            "account_id": "acct-session-1",
            "workspace_id": "ws-session-1",
        },
    )
    FakeChatGPTClient.create_account_refresh_token = "refresh-from-create-account"
    FakeChatGPTClient.create_account_account_id = "acct-create-1"
    FakeChatGPTClient.create_account_workspace_id = "ws-create-1"
    FakeOAuthClient.plans = []
    FakeOAuthClient.call_count = 0

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["access_token"] == "session-access-1"
    assert result["refresh_token"] == "refresh-from-create-account"
    assert result["session_token"] == "session-token-1"
    assert result["metadata"]["refresh_token_source"] == "create_account"
    assert FakeOAuthClient.call_count == 0


def test_run_session_reuse_success_still_fetches_refresh_token_via_oauth(monkeypatch):
    FakeChatGPTClient.reuse_result = (
        True,
        {
            "access_token": "session-access-2",
            "session_token": "session-token-2",
            "account_id": "acct-session-2",
            "workspace_id": "ws-session-2",
        },
    )
    FakeChatGPTClient.create_account_refresh_token = ""
    FakeChatGPTClient.create_account_account_id = ""
    FakeChatGPTClient.create_account_workspace_id = ""
    FakeOAuthClient.plans = [
        {
            "tokens": {
                "access_token": "oauth-access-2",
                "refresh_token": "refresh-2",
                "id_token": "id-2",
                "account_id": "acct-oauth-2",
                "workspace_id": "ws-oauth-2",
            }
        }
    ]
    FakeOAuthClient.call_count = 0

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["access_token"] == "oauth-access-2"
    assert result["refresh_token"] == "refresh-2"
    assert result["id_token"] == "id-2"
    assert result["session_token"] == "session-token-2"
    assert result["account_id"] == "acct-oauth-2"
    assert result["workspace_id"] == "ws-oauth-2"
    assert result["metadata"]["refresh_token_source"] == "oauth_password"
    assert FakeOAuthClient.call_count == 1


def test_run_session_reuse_success_allows_oauth_retry_to_fill_refresh_token(monkeypatch):
    FakeChatGPTClient.reuse_result = (
        True,
        {
            "access_token": "session-access-3",
            "session_token": "session-token-3",
            "account_id": "acct-session-3",
            "workspace_id": "ws-session-3",
        },
    )
    FakeChatGPTClient.create_account_refresh_token = ""
    FakeChatGPTClient.create_account_account_id = ""
    FakeChatGPTClient.create_account_workspace_id = ""
    FakeOAuthClient.plans = [
        {
            "tokens": None,
            "last_error": "temporary oauth failure",
        },
        {
            "tokens": {
                "access_token": "oauth-access-3",
                "refresh_token": "refresh-3",
                "id_token": "id-3",
                "account_id": "acct-oauth-3",
                "workspace_id": "ws-oauth-3",
            }
        },
    ]
    FakeOAuthClient.call_count = 0

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["refresh_token"] == "refresh-3"
    assert result["session_token"] == "session-token-3"
    assert result["metadata"]["refresh_token_source"] == "oauth_password"
    assert FakeOAuthClient.call_count == 2


def test_run_existing_account_uses_saved_password_and_switches_to_login(monkeypatch):
    FakeChatGPTClient.register_result = (False, "注册失败: HTTP 400: user_exists")
    FakeChatGPTClient.create_account_refresh_token = ""
    FakeChatGPTClient.create_account_account_id = ""
    FakeChatGPTClient.create_account_workspace_id = ""
    FakeOAuthClient.plans = [
        {
            "tokens": {
                "access_token": "oauth-access-existing",
                "refresh_token": "refresh-existing",
                "id_token": "id-existing",
                "account_id": "acct-existing",
                "workspace_id": "ws-existing",
            }
        }
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.last_login_email = None
    FakeOAuthClient.last_login_password = None
    _patch_saved_account(monkeypatch, "known-pass")

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["source"] == "login"
    assert result["access_token"] == "oauth-access-existing"
    assert result["refresh_token"] == "refresh-existing"
    assert engine.password == "known-pass"
    assert FakeOAuthClient.last_login_email == "tester@example.com"
    assert FakeOAuthClient.last_login_password == "known-pass"
    assert result["metadata"]["existing_account_detected"] is True
    assert result["metadata"]["login_password_source"] == "database"


def test_run_existing_account_direct_otp_uses_saved_password_for_oauth(monkeypatch):
    FakeChatGPTClient.register_result = (True, "注册成功")
    FakeChatGPTClient.existing_account_detected = True
    FakeChatGPTClient.reuse_result = (
        True,
        {
            "access_token": "session-access-existing",
            "session_token": "session-token-existing",
            "account_id": "acct-session-existing",
            "workspace_id": "ws-session-existing",
        },
    )
    FakeChatGPTClient.create_account_refresh_token = ""
    FakeChatGPTClient.create_account_account_id = ""
    FakeChatGPTClient.create_account_workspace_id = ""
    FakeOAuthClient.plans = [
        {
            "tokens": {
                "access_token": "oauth-access-existing-otp",
                "refresh_token": "refresh-existing-otp",
                "id_token": "id-existing-otp",
                "account_id": "acct-existing-otp",
                "workspace_id": "ws-existing-otp",
            }
        }
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.last_login_email = None
    FakeOAuthClient.last_login_password = None
    _patch_saved_account(monkeypatch, "known-pass-otp")

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["source"] == "login"
    assert result["refresh_token"] == "refresh-existing-otp"
    assert engine.password == "known-pass-otp"
    assert FakeOAuthClient.last_login_password == "known-pass-otp"
    assert result["metadata"]["existing_account_detected"] is True
    assert result["metadata"]["login_password_source"] == "database"
