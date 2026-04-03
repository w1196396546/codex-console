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
