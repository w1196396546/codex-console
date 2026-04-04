from src.core.anyauto.oauth_client import OAuthClient


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
