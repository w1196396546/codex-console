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
