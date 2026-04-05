from contextlib import contextmanager
from types import SimpleNamespace
import threading
import time

import pytest

import src.core.anyauto.register_flow as register_flow_module
from src.core.anyauto.register_flow import AnyAutoRegistrationEngine, EmailServiceAdapter


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

    def __init__(self, proxy=None, verbose=True, browser_mode="protocol", check_cancelled=None):
        self.proxy = proxy
        self.verbose = verbose
        self.browser_mode = browser_mode
        self.check_cancelled = check_cancelled
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
    passwordless_plans = []
    call_count = 0
    passwordless_call_count = 0
    last_login_email = None
    last_login_password = None
    last_passwordless_profile = None

    def __init__(self, config, proxy=None, verbose=True, browser_mode="protocol", check_cancelled=None):
        self.config = config
        self.proxy = proxy
        self.verbose = verbose
        self.browser_mode = browser_mode
        self.check_cancelled = check_cancelled
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
        first_name=None,
        last_name=None,
        birthdate=None,
    ):
        FakeOAuthClient.passwordless_call_count += 1
        FakeOAuthClient.last_passwordless_profile = {
            "email": email,
            "device_id": device_id,
            "user_agent": user_agent,
            "sec_ch_ua": sec_ch_ua,
            "impersonate": impersonate,
            "first_name": first_name,
            "last_name": last_name,
            "birthdate": birthdate,
        }
        if not FakeOAuthClient.passwordless_plans:
            raise AssertionError("missing passwordless OAuth plan")
        plan = FakeOAuthClient.passwordless_plans.pop(0)
        self.last_error = plan.get("last_error", "")
        return plan.get("tokens")

    def _decode_oauth_session_cookie(self):
        return {}


def _settings():
    return SimpleNamespace(
        registration_default_password_length=12,
        openai_auth_url="https://auth.openai.com",
        openai_client_id="client-1",
        openai_redirect_uri="http://localhost:1455/auth/callback",
    )


def _build_engine(monkeypatch, *, reset_global_window=True):
    monkeypatch.setattr("src.core.anyauto.register_flow.get_settings", _settings)
    monkeypatch.setattr("src.core.anyauto.register_flow.ChatGPTClient", FakeChatGPTClient)
    monkeypatch.setattr("src.core.anyauto.register_flow.OAuthClient", FakeOAuthClient)
    AnyAutoRegistrationEngine._refresh_token_slots = set()
    AnyAutoRegistrationEngine._refresh_token_cooldowns = {}
    AnyAutoRegistrationEngine._refresh_token_global_inflight = 0
    AnyAutoRegistrationEngine._refresh_token_global_spacing_seconds = 0.0
    if reset_global_window:
        AnyAutoRegistrationEngine._refresh_token_global_next_allowed_at = 0.0
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


def _patch_saved_account_with_metadata(monkeypatch, password, extra_data):
    @contextmanager
    def fake_get_db():
        yield object()

    account = SimpleNamespace(password=password, extra_data=extra_data)
    monkeypatch.setattr(register_flow_module, "get_db", fake_get_db, raising=False)
    monkeypatch.setattr(
        register_flow_module,
        "crud",
        SimpleNamespace(get_account_by_email=lambda db, email: account),
        raising=False,
    )


def test_email_service_adapter_wait_for_verification_code_honors_cancellation_between_poll_slices():
    calls = []
    cancelled = {"value": False}

    class SlowEmailService:
        def get_verification_code(self, **kwargs):
            calls.append(kwargs["timeout"])
            cancelled["value"] = True
            return None

    adapter = EmailServiceAdapter(
        SlowEmailService(),
        "tester@example.com",
        "mailbox-1",
        lambda _msg: None,
        check_cancelled=lambda: cancelled["value"],
    )

    with pytest.raises(RuntimeError, match="任务已取消"):
        adapter.wait_for_verification_code("tester@example.com", timeout=30)

    assert calls == [5]


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


def test_run_existing_account_without_saved_password_uses_passwordless_oauth(monkeypatch):
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
    FakeOAuthClient.plans = []
    FakeOAuthClient.passwordless_plans = [
        {
            "tokens": {
                "access_token": "oauth-access-passwordless",
                "refresh_token": "refresh-passwordless",
                "id_token": "id-passwordless",
                "account_id": "acct-passwordless",
                "workspace_id": "ws-passwordless",
            }
        }
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.passwordless_call_count = 0
    _patch_saved_account(monkeypatch, None)

    engine = _build_engine(monkeypatch)
    result = engine.run()

    assert result["success"] is True
    assert result["source"] == "login"
    assert result["refresh_token"] == "refresh-passwordless"
    assert result["metadata"]["refresh_token_source"] == "oauth_passwordless"
    assert result["metadata"]["existing_account_detected"] is True
    assert FakeOAuthClient.call_count == 0
    assert FakeOAuthClient.passwordless_call_count == 1


def test_run_existing_account_without_saved_password_fails_when_passwordless_oauth_still_missing_rt(monkeypatch):
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
    FakeOAuthClient.plans = []
    FakeOAuthClient.passwordless_plans = [
        {
            "tokens": None,
            "last_error": "提交邮箱失败: 429 - rate limit exceeded",
        },
        {
            "tokens": None,
            "last_error": "提交邮箱失败: 429 - rate limit exceeded",
        },
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.passwordless_call_count = 0
    _patch_saved_account(monkeypatch, None)

    engine = _build_engine(monkeypatch, reset_global_window=False)
    result = engine.run()

    assert result["success"] is False
    assert "缺少历史密码" in result["error_message"]
    assert FakeOAuthClient.call_count == 0
    assert FakeOAuthClient.passwordless_call_count == 2


def test_run_skips_oauth_completion_when_cooldown_is_active(monkeypatch):
    FakeChatGPTClient.reuse_result = (
        True,
        {
            "access_token": "session-access-cooldown",
            "session_token": "session-token-cooldown",
            "account_id": "acct-session-cooldown",
            "workspace_id": "ws-session-cooldown",
        },
    )
    FakeChatGPTClient.create_account_refresh_token = ""
    FakeChatGPTClient.create_account_account_id = ""
    FakeChatGPTClient.create_account_workspace_id = ""
    FakeOAuthClient.plans = []
    FakeOAuthClient.call_count = 0
    _patch_saved_account_with_metadata(
        monkeypatch,
        "known-pass",
        {"refresh_token_cooldown_until": "2999-01-01T00:00:00"},
    )

    engine = _build_engine(monkeypatch, reset_global_window=False)
    result = engine.run()

    assert result["success"] is True
    assert result["refresh_token"] == ""
    assert result["metadata"]["refresh_token_retry_deferred"] is True
    assert "冷却中" in result["metadata"]["refresh_token_error"]
    assert FakeOAuthClient.call_count == 0


def test_oauth_completion_slot_rejects_duplicate_inflight_attempts(monkeypatch):
    engine = _build_engine(monkeypatch, reset_global_window=False)
    email = "dedupe@example.com"

    first = engine._try_acquire_refresh_token_completion_slot(email)
    second = engine._try_acquire_refresh_token_completion_slot(email)

    try:
        assert first is True
        assert second is False
    finally:
        engine._release_refresh_token_completion_slot(email)


def test_oauth_completion_waits_for_global_refresh_token_window(monkeypatch):
    FakeOAuthClient.plans = [
        {
            "tokens": {
                "access_token": "oauth-access-window",
                "refresh_token": "refresh-window",
            },
            "last_error": "",
        }
    ]
    FakeOAuthClient.call_count = 0
    AnyAutoRegistrationEngine._refresh_token_slots = set()
    AnyAutoRegistrationEngine._refresh_token_cooldowns = {}
    AnyAutoRegistrationEngine._refresh_token_global_next_allowed_at = 105.0

    sleeps = []
    monkeypatch.setattr(register_flow_module.time, "time", lambda: 100.0)
    monkeypatch.setattr(register_flow_module.time, "sleep", sleeps.append)

    engine = _build_engine(monkeypatch, reset_global_window=False)
    chatgpt_client = FakeChatGPTClient()

    result = engine._run_oauth_token_completion(
        chatgpt_client,
        "second@example.com",
        "known-pass",
        DummyEmailService(),
        {"oauth_client_id": "client-1"},
        reason="补齐窗口",
    )

    assert result["tokens"]["refresh_token"] == "refresh-window"
    assert sleeps == [5.0]
    assert FakeOAuthClient.call_count == 1


def test_oauth_completion_without_password_uses_passwordless_flow_under_global_window(monkeypatch):
    FakeOAuthClient.plans = []
    FakeOAuthClient.passwordless_plans = [
        {
            "tokens": {
                "access_token": "oauth-access-passwordless-window",
                "refresh_token": "refresh-passwordless-window",
            },
            "last_error": "",
        }
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.passwordless_call_count = 0
    AnyAutoRegistrationEngine._refresh_token_slots = set()
    AnyAutoRegistrationEngine._refresh_token_cooldowns = {}
    AnyAutoRegistrationEngine._refresh_token_global_next_allowed_at = 105.0

    sleeps = []
    monkeypatch.setattr(register_flow_module.time, "time", lambda: 100.0)
    monkeypatch.setattr(register_flow_module.time, "sleep", sleeps.append)

    engine = _build_engine(monkeypatch, reset_global_window=False)
    chatgpt_client = FakeChatGPTClient()

    result = engine._run_oauth_token_completion(
        chatgpt_client,
        "passwordless@example.com",
        "",
        DummyEmailService(),
        {"oauth_client_id": "client-1"},
        reason="无密码补齐",
    )

    assert result["tokens"]["refresh_token"] == "refresh-passwordless-window"
    assert result["source"] == "oauth_passwordless"
    assert sleeps == [5.0]
    assert FakeOAuthClient.call_count == 0
    assert FakeOAuthClient.passwordless_call_count == 1


def test_oauth_completion_without_password_forwards_signup_profile_to_passwordless_flow(monkeypatch):
    FakeOAuthClient.plans = []
    FakeOAuthClient.passwordless_plans = [
        {
            "tokens": {
                "access_token": "oauth-access-passwordless-profile",
                "refresh_token": "refresh-passwordless-profile",
            },
            "last_error": "",
        }
    ]
    FakeOAuthClient.call_count = 0
    FakeOAuthClient.passwordless_call_count = 0
    FakeOAuthClient.last_passwordless_profile = None

    engine = _build_engine(monkeypatch)
    chatgpt_client = FakeChatGPTClient()

    result = engine._run_oauth_token_completion(
        chatgpt_client,
        "profiled@example.com",
        "",
        DummyEmailService(),
        {"oauth_client_id": "client-1"},
        reason="无密码补齐并透传注册资料",
        first_name="Alice",
        last_name="Walker",
        birthdate="1999-02-03",
    )

    assert result["tokens"]["refresh_token"] == "refresh-passwordless-profile"
    assert result["source"] == "oauth_passwordless"
    assert FakeOAuthClient.last_passwordless_profile == {
        "email": "profiled@example.com",
        "device_id": "device-1",
        "user_agent": "ua",
        "sec_ch_ua": "sec",
        "impersonate": "chrome",
        "first_name": "Alice",
        "last_name": "Walker",
        "birthdate": "1999-02-03",
    }


def test_oauth_completion_retries_after_global_backoff_until_success(monkeypatch):
    FakeOAuthClient.plans = [
        {
            "tokens": None,
            "last_error": "提交邮箱失败: 429 - rate limit exceeded",
        },
        {
            "tokens": None,
            "last_error": "提交邮箱失败: 429 - rate limit exceeded",
        },
        {
            "tokens": {
                "access_token": "oauth-access-after-backoff",
                "refresh_token": "refresh-after-backoff",
            },
            "last_error": "",
        },
    ]
    FakeOAuthClient.call_count = 0
    AnyAutoRegistrationEngine._refresh_token_slots = set()
    AnyAutoRegistrationEngine._refresh_token_cooldowns = {}
    AnyAutoRegistrationEngine._refresh_token_global_next_allowed_at = 0.0
    AnyAutoRegistrationEngine._refresh_token_global_rate_limit_backoff_seconds = 45.0

    sleeps = []
    monkeypatch.setattr(register_flow_module.time, "time", lambda: 100.0)
    monkeypatch.setattr(register_flow_module.time, "sleep", sleeps.append)

    engine = _build_engine(monkeypatch)
    chatgpt_client = FakeChatGPTClient()

    result = engine._run_oauth_token_completion(
        chatgpt_client,
        "rate-limit@example.com",
        "known-pass",
        DummyEmailService(),
        {"oauth_client_id": "client-1"},
        reason="命中限流",
    )

    assert result["tokens"]["refresh_token"] == "refresh-after-backoff"
    assert 45.0 in sleeps
    assert FakeOAuthClient.call_count == 3


def test_oauth_completion_allows_parallel_inflight_when_configured(monkeypatch):
    AnyAutoRegistrationEngine._refresh_token_slots = set()
    AnyAutoRegistrationEngine._refresh_token_cooldowns = {}
    AnyAutoRegistrationEngine._refresh_token_global_next_allowed_at = 0.0

    in_flight = {"count": 0, "max": 0}
    counter_lock = threading.Lock()
    first_entered = threading.Event()
    release = threading.Event()

    def fake_passwordless(self, chatgpt_client, email, skymail_adapter, oauth_config):
        with counter_lock:
            in_flight["count"] += 1
            in_flight["max"] = max(in_flight["max"], in_flight["count"])
            if in_flight["count"] == 1:
                first_entered.set()
        release.wait(timeout=0.5)
        with counter_lock:
            in_flight["count"] -= 1
        return {
            "access_token": f"access-{email}",
            "refresh_token": f"refresh-{email}",
        }

    monkeypatch.setattr(
        AnyAutoRegistrationEngine,
        "_passwordless_oauth_reauth",
        fake_passwordless,
    )

    engine1 = AnyAutoRegistrationEngine(
        email_service=DummyEmailService(),
        callback_logger=lambda _msg: None,
        extra_config={"token_completion_concurrency": 2},
    )
    engine2 = AnyAutoRegistrationEngine(
        email_service=DummyEmailService(),
        callback_logger=lambda _msg: None,
        extra_config={"token_completion_concurrency": 2},
    )
    chatgpt_client = FakeChatGPTClient()

    results = []

    def run(engine, email):
        results.append(
            engine._run_oauth_token_completion(
                chatgpt_client,
                email,
                "",
                DummyEmailService(),
                {"oauth_client_id": "client-1"},
                reason="并发补齐",
            )
        )

    t1 = threading.Thread(target=run, args=(engine1, "first@example.com"))
    t2 = threading.Thread(target=run, args=(engine2, "second@example.com"))

    t1.start()
    assert first_entered.wait(timeout=0.2), "first token completion did not start in time"
    t2.start()
    time.sleep(0.1)
    release.set()
    t1.join()
    t2.join()

    assert len(results) == 2
    assert in_flight["max"] == 2
