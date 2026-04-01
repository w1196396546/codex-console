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
