from src.core.anyauto.oauth_client import OAuthClient
from src.core.anyauto.utils import FlowState


def test_oauth_client_normalizes_full_authorize_url_to_issuer():
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com/oauth/authorize",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    assert client.oauth_issuer == "https://auth.openai.com"


def test_oauth_client_request_with_rate_limit_retry_honors_retry_after(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    sleeps = []

    class FakeResponse:
        def __init__(self, status_code, headers=None):
            self.status_code = status_code
            self.headers = headers or {}

    responses = [
        FakeResponse(429, {"Retry-After": "7"}),
        FakeResponse(200, {}),
    ]

    class FakeSession:
        def __init__(self):
            self.calls = 0

        def post(self, url, **kwargs):
            self.calls += 1
            return responses.pop(0)

    client.session = FakeSession()
    monkeypatch.setattr("src.core.anyauto.oauth_client.time.sleep", sleeps.append)

    response = client._request_with_rate_limit_retry(
        "post",
        "https://auth.openai.com/api/accounts/authorize/continue",
        request_label="提交邮箱",
        max_attempts=2,
        json={"username": {"kind": "email", "value": "demo@example.com"}},
    )

    assert response.status_code == 200
    assert client.session.calls == 2
    assert sleeps == [7.0]


def test_bootstrap_oauth_session_falls_back_to_direct_session_on_transport_error(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        proxy="http://proxy.example:8080",
        verbose=False,
    )

    class ProxySession:
        def __init__(self):
            self.cookies = []

        def get(self, url, **kwargs):
            raise RuntimeError("Proxy CONNECT aborted")

    class DirectSession:
        def __init__(self):
            self.cookies = [type("Cookie", (), {"name": "login_session"})()]

        def get(self, url, **kwargs):
            return type(
                "Response",
                (),
                {
                    "url": "https://auth.openai.com/log-in",
                    "status_code": 200,
                    "history": [],
                },
            )()

    client.session = ProxySession()
    monkeypatch.setattr(client, "_browser_pause", lambda *args, **kwargs: None)
    monkeypatch.setattr(
        client,
        "_create_direct_session",
        lambda: DirectSession(),
        raising=False,
    )

    final_url = client._bootstrap_oauth_session(
        "https://auth.openai.com/oauth/authorize",
        {"client_id": "client-1"},
        device_id="device-1",
        user_agent="ua",
        sec_ch_ua="sec",
        impersonate="chrome120",
    )

    assert final_url == "https://auth.openai.com/log-in"
    assert type(client.session).__name__ == "DirectSession"


def test_login_passwordless_completes_about_you_before_token_exchange(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    captured = {}

    monkeypatch.setattr(client, "_bootstrap_oauth_session", lambda *args, **kwargs: "https://auth.openai.com/log-in")
    monkeypatch.setattr(
        client,
        "_submit_authorize_continue",
        lambda *args, **kwargs: FlowState(
            page_type="login_password",
            current_url="https://auth.openai.com/log-in/password",
        ),
    )
    monkeypatch.setattr(client, "_send_email_otp", lambda *args, **kwargs: (True, ""))
    monkeypatch.setattr(
        client,
        "_handle_otp_verification",
        lambda *args, **kwargs: FlowState(
            page_type="about_you",
            current_url="https://auth.openai.com/about-you",
        ),
    )

    def fake_complete_about_you_profile(state, device_id, user_agent=None, sec_ch_ua=None, impersonate=None, first_name=None, last_name=None, birthdate=None):
        captured.update(
            {
                "state": state.page_type,
                "device_id": device_id,
                "user_agent": user_agent,
                "sec_ch_ua": sec_ch_ua,
                "impersonate": impersonate,
                "first_name": first_name,
                "last_name": last_name,
                "birthdate": birthdate,
            }
        )
        return FlowState(
            page_type="oauth_callback",
            current_url="http://localhost:1455/auth/callback?code=passwordless-code",
        )

    monkeypatch.setattr(client, "_complete_about_you_profile", fake_complete_about_you_profile)
    monkeypatch.setattr(
        client,
        "_exchange_code_for_tokens",
        lambda code, code_verifier, user_agent=None, impersonate=None: {
            "access_token": "access-from-about-you",
            "refresh_token": "refresh-from-about-you",
            "code": code,
        },
    )

    tokens = client.login_passwordless_and_get_tokens(
        "tester@example.com",
        "device-about-you",
        user_agent="ua-about-you",
        sec_ch_ua="sec-about-you",
        impersonate="chrome-about-you",
        skymail_client=object(),
        first_name="Alice",
        last_name="Walker",
        birthdate="1999-02-03",
    )

    assert tokens["refresh_token"] == "refresh-from-about-you"
    assert captured == {
        "state": "about_you",
        "device_id": "device-about-you",
        "user_agent": "ua-about-you",
        "sec_ch_ua": "sec-about-you",
        "impersonate": "chrome-about-you",
        "first_name": "Alice",
        "last_name": "Walker",
        "birthdate": "1999-02-03",
    }


def test_login_and_get_tokens_uses_codex_simplified_authorize_flags(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    captured = {}

    def fake_bootstrap(authorize_url, authorize_params, **kwargs):
        captured["authorize_url"] = authorize_url
        captured["authorize_params"] = dict(authorize_params)
        return "https://auth.openai.com/log-in"

    monkeypatch.setattr(client, "_bootstrap_oauth_session", fake_bootstrap)
    monkeypatch.setattr(
        client,
        "_submit_authorize_continue",
        lambda *args, **kwargs: FlowState(
            page_type="oauth_callback",
            current_url="http://localhost:1455/auth/callback?code=oauth-code",
        ),
    )
    monkeypatch.setattr(
        client,
        "_exchange_code_for_tokens",
        lambda code, code_verifier, user_agent=None, impersonate=None: {
            "access_token": "access-token",
            "refresh_token": "refresh-token",
            "code": code,
        },
    )

    tokens = client.login_and_get_tokens(
        "tester@example.com",
        "known-pass",
        "device-1",
        user_agent="ua",
        sec_ch_ua="sec",
        impersonate="chrome",
        skymail_client=object(),
    )

    assert tokens["refresh_token"] == "refresh-token"
    assert captured["authorize_url"] == "https://auth.openai.com/oauth/authorize"
    assert captured["authorize_params"]["id_token_add_organizations"] == "true"
    assert captured["authorize_params"]["codex_cli_simplified_flow"] == "true"


def test_login_passwordless_uses_codex_simplified_authorize_flags(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    captured = {}

    def fake_bootstrap(authorize_url, authorize_params, **kwargs):
        captured["authorize_url"] = authorize_url
        captured["authorize_params"] = dict(authorize_params)
        return "https://auth.openai.com/log-in"

    monkeypatch.setattr(client, "_bootstrap_oauth_session", fake_bootstrap)
    monkeypatch.setattr(
        client,
        "_submit_authorize_continue",
        lambda *args, **kwargs: FlowState(
            page_type="login_password",
            current_url="https://auth.openai.com/log-in/password",
        ),
    )
    monkeypatch.setattr(client, "_send_email_otp", lambda *args, **kwargs: (True, ""))
    monkeypatch.setattr(
        client,
        "_handle_otp_verification",
        lambda *args, **kwargs: FlowState(
            page_type="oauth_callback",
            current_url="http://localhost:1455/auth/callback?code=passwordless-code",
        ),
    )
    monkeypatch.setattr(
        client,
        "_exchange_code_for_tokens",
        lambda code, code_verifier, user_agent=None, impersonate=None: {
            "access_token": "access-token",
            "refresh_token": "refresh-token",
            "code": code,
        },
    )

    tokens = client.login_passwordless_and_get_tokens(
        "tester@example.com",
        "device-1",
        user_agent="ua",
        sec_ch_ua="sec",
        impersonate="chrome",
        skymail_client=object(),
        first_name="Alice",
        last_name="Walker",
        birthdate="1999-02-03",
    )

    assert tokens["refresh_token"] == "refresh-token"
    assert captured["authorize_url"] == "https://auth.openai.com/oauth/authorize"
    assert captured["authorize_params"]["id_token_add_organizations"] == "true"
    assert captured["authorize_params"]["codex_cli_simplified_flow"] == "true"


def test_state_supports_workspace_resolution_requires_explicit_consent_markers(monkeypatch):
    client = OAuthClient(
        config={
            "oauth_issuer": "https://auth.openai.com",
            "oauth_client_id": "client-1",
            "oauth_redirect_uri": "http://localhost:1455/auth/callback",
        },
        verbose=False,
    )

    monkeypatch.setattr(
        client,
        "_decode_oauth_session_cookie",
        lambda: {"workspaces": [{"id": "ws-1"}]},
    )

    assert client._state_supports_workspace_resolution(
        FlowState(
            page_type="login_password",
            current_url="https://auth.openai.com/log-in/password",
        )
    ) is False
    assert client._state_supports_workspace_resolution(
        FlowState(
            page_type="consent",
            current_url="https://auth.openai.com/sign-in-with-chatgpt/codex/consent",
        )
    ) is True
