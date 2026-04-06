from fastapi.testclient import TestClient

from src.web.app import create_app


def test_legacy_webui_smoke_routes():
    client = TestClient(create_app())

    login_response = client.get("/login")
    assert login_response.status_code == 200
    assert "访问验证" in login_response.text

    home_response = client.get("/", follow_redirects=False)
    assert home_response.status_code == 302
    assert home_response.headers["location"].startswith("/login?next=/")

    accounts_response = client.get("/accounts", follow_redirects=False)
    assert accounts_response.status_code == 302
    assert accounts_response.headers["location"].startswith("/login?next=/accounts")

    static_response = client.get("/static/css/style.css")
    assert static_response.status_code == 200
