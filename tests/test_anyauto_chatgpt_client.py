import json

import src.core.anyauto.chatgpt_client as chatgpt_client_module
from src.core.anyauto.chatgpt_client import ChatGPTClient
from src.core.anyauto.utils import FlowState


def test_state_requires_navigation_for_workspace_api_state():
    client = ChatGPTClient.__new__(ChatGPTClient)
    state = FlowState(
        page_type="workspace",
        current_url="https://auth.openai.com/workspace",
        continue_url="",
        method="GET",
        source="api",
    )

    assert client._state_requires_navigation(state) is True


def test_register_complete_flow_treats_workspace_state_as_success(monkeypatch):
    client = ChatGPTClient(proxy=None, verbose=False, browser_mode="protocol")

    monkeypatch.setattr(client, "visit_homepage", lambda: True)
    monkeypatch.setattr(client, "get_csrf_token", lambda: "csrf-token")
    monkeypatch.setattr(client, "signin", lambda email, csrf_token: "https://auth.example.test/authorize")
    monkeypatch.setattr(client, "authorize", lambda url: "https://auth.openai.com/email-verification")

    workspace_state = FlowState(
        page_type="workspace",
        current_url="https://auth.openai.com/workspace",
        continue_url="",
        method="GET",
        source="api",
    )

    monkeypatch.setattr(
        client,
        "verify_email_otp",
        lambda otp_code, return_state=False: (True, workspace_state) if return_state else (True, "验证成功"),
    )

    class DummySkyMail:
        def wait_for_verification_code(self, email, timeout=90, exclude_codes=None):
            return "123456"

    success, message = client.register_complete_flow(
        "tester@example.com",
        "Password123!",
        "Test",
        "User",
        "2000-01-01",
        DummySkyMail(),
    )

    assert success is True
    assert message == "workspace_required"
    assert client.last_registration_state.page_type == "workspace"


def test_register_user_sends_device_id_and_sentinel_headers(monkeypatch):
    client = ChatGPTClient(proxy=None, verbose=False, browser_mode="protocol")
    captured = {}

    sentinel_payload = {
        "p": "proof-token",
        "c": "challenge-token",
        "id": client.device_id,
        "flow": "username_password_create",
    }
    sentinel_token = json.dumps(sentinel_payload)

    def fake_build_sentinel_token(session, device_id, flow, user_agent, sec_ch_ua, impersonate):
        captured["builder_args"] = {
            "session": session,
            "device_id": device_id,
            "flow": flow,
            "user_agent": user_agent,
            "sec_ch_ua": sec_ch_ua,
            "impersonate": impersonate,
        }
        return sentinel_token

    class DummyResponse:
        status_code = 200
        text = ""

        @staticmethod
        def json():
            return {"ok": True}

    class DummySession:
        def post(self, url, json=None, headers=None, timeout=None):
            captured["url"] = url
            captured["payload"] = json
            captured["headers"] = headers
            captured["timeout"] = timeout
            return DummyResponse()

    monkeypatch.setattr(chatgpt_client_module, "build_sentinel_token", fake_build_sentinel_token)
    monkeypatch.setattr(client, "_browser_pause", lambda *args, **kwargs: None)
    client.session = DummySession()

    success, message = client.register_user("tester@example.com", "Password123!")

    assert success is True
    assert message == "注册成功"
    assert captured["builder_args"]["session"] is client.session
    assert captured["builder_args"]["device_id"] == client.device_id
    assert captured["builder_args"]["flow"] == "username_password_create"

    headers = captured["headers"]
    assert headers["oai-device-id"] == client.device_id
    assert headers["openai-sentinel-token"] == sentinel_token

    parsed = json.loads(headers["openai-sentinel-token"])
    assert parsed["p"] == "proof-token"
    assert parsed["c"] == "challenge-token"
    assert parsed["id"] == client.device_id
    assert parsed["flow"] == "username_password_create"


def test_authorize_retries_after_browser_warmup_on_cloudflare(monkeypatch):
    client = ChatGPTClient(proxy="http://127.0.0.1:7890", verbose=False, browser_mode="protocol")
    state = {
        "warmed": False,
        "warmup_calls": [],
        "reset_count": 0,
    }

    blocked_url = "https://auth.openai.com/api/accounts/authorize"
    success_url = "https://auth.openai.com/create-account/password"

    class DummyResponse:
        def __init__(self, url):
            self.url = url

    class DummySession:
        def get(self, url, headers=None, allow_redirects=True, timeout=None):
            current_url = success_url if state["warmed"] else blocked_url
            return DummyResponse(current_url)

    def fake_warmup(session, auth_url, *, device_id, proxy=None, user_agent=None):
        state["warmup_calls"].append(
            {
                "session": session,
                "auth_url": auth_url,
                "device_id": device_id,
                "proxy": proxy,
                "user_agent": user_agent,
            }
        )
        state["warmed"] = True
        return True

    def fake_reset_session():
        state["reset_count"] += 1
        state["warmed"] = False

    monkeypatch.setattr(
        chatgpt_client_module,
        "try_warm_auth_cloudflare_cookies",
        fake_warmup,
        raising=False,
    )
    monkeypatch.setattr(client, "_browser_pause", lambda *args, **kwargs: None)
    monkeypatch.setattr(client, "_reset_session", fake_reset_session)
    client.session = DummySession()

    final_url = client.authorize("https://auth.example.test/authorize", max_retries=2)

    assert final_url == success_url
    assert state["reset_count"] == 0
    assert len(state["warmup_calls"]) == 1
    assert state["warmup_calls"][0]["session"] is client.session
    assert state["warmup_calls"][0]["auth_url"] == "https://auth.example.test/authorize"
    assert state["warmup_calls"][0]["device_id"] == client.device_id
    assert state["warmup_calls"][0]["proxy"] == client.proxy
    assert state["warmup_calls"][0]["user_agent"] == client.ua
