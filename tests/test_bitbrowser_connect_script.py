from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest


SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "bitbrowser_connect.py"


def load_module():
    spec = importlib.util.spec_from_file_location("bitbrowser_connect", SCRIPT_PATH)
    if spec is None or spec.loader is None:
        raise AssertionError(f"unable to load {SCRIPT_PATH}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_collect_connect_info_uses_single_opened_browser_and_port(monkeypatch: pytest.MonkeyPatch):
    module = load_module()

    def fake_post_json(base_url: str, path: str, payload: dict, timeout: float):
        assert base_url == "http://127.0.0.1:54345"
        assert timeout == 3
        if path == "/browser/opened/ids":
            return {"success": True, "data": ["browser-1"]}
        if path == "/browser/ports":
            return {"success": True, "data": {"browser-1": "54937"}}
        raise AssertionError(f"unexpected path {path}")

    def fake_get_json(url: str, timeout: float):
        assert timeout == 3
        if url == "http://127.0.0.1:54937/json/version":
            return {
                "Browser": "Chrome/140.0.7339.265",
                "webSocketDebuggerUrl": "ws://127.0.0.1:54937/devtools/browser/browser-socket",
            }
        if url == "http://127.0.0.1:54937/json/list":
            return [
                {"id": "page-1", "type": "page", "title": "ChatGPT", "url": "https://chatgpt.com/"},
                {"id": "iframe-1", "type": "iframe", "title": "", "url": "https://example.com/frame"},
            ]
        raise AssertionError(f"unexpected url {url}")

    monkeypatch.setattr(module, "post_json", fake_post_json)
    monkeypatch.setattr(module, "get_json", fake_get_json)

    result = module.collect_connect_info("http://127.0.0.1:54345", timeout=3)

    assert result["browser_id"] == "browser-1"
    assert result["debug_http_url"] == "http://127.0.0.1:54937"
    assert result["browser_ws_endpoint"] == "ws://127.0.0.1:54937/devtools/browser/browser-socket"
    assert result["page_count"] == 1
    assert result["pages"] == [{"id": "page-1", "title": "ChatGPT", "url": "https://chatgpt.com/"}]


def test_collect_connect_info_requires_browser_id_when_multiple_opened(monkeypatch: pytest.MonkeyPatch):
    module = load_module()

    def fake_post_json(base_url: str, path: str, payload: dict, timeout: float):
        if path == "/browser/opened/ids":
            return {"success": True, "data": ["browser-1", "browser-2"]}
        if path == "/browser/ports":
            return {"success": True, "data": {"browser-1": "54937", "browser-2": "54938"}}
        raise AssertionError(f"unexpected path {path}")

    monkeypatch.setattr(module, "post_json", fake_post_json)

    with pytest.raises(ValueError, match="多个已打开实例"):
        module.collect_connect_info("http://127.0.0.1:54345", timeout=3)


def test_collect_connect_info_can_open_browser_when_requested(monkeypatch: pytest.MonkeyPatch):
    module = load_module()
    calls: list[tuple[str, dict]] = []

    def fake_post_json(base_url: str, path: str, payload: dict, timeout: float):
        calls.append((path, payload))
        if path == "/browser/opened/ids":
            return {"success": True, "data": []}
        if path == "/browser/ports":
            return {"success": True, "data": {}}
        if path == "/browser/open":
            assert payload == {"id": "browser-9"}
            return {
                "success": True,
                "data": {
                    "id": "browser-9",
                    "http": "127.0.0.1:60001",
                    "ws": "ws://127.0.0.1:60001/devtools/browser/browser-9",
                },
            }
        raise AssertionError(f"unexpected path {path}")

    def fake_get_json(url: str, timeout: float):
        if url == "http://127.0.0.1:60001/json/version":
            return {
                "Browser": "Chrome/140.0.7339.265",
                "webSocketDebuggerUrl": "ws://127.0.0.1:60001/devtools/browser/browser-9",
            }
        if url == "http://127.0.0.1:60001/json/list":
            return [{"id": "page-9", "type": "page", "title": "工作台", "url": "https://console.bitbrowser.net/"}]
        raise AssertionError(f"unexpected url {url}")

    monkeypatch.setattr(module, "post_json", fake_post_json)
    monkeypatch.setattr(module, "get_json", fake_get_json)

    result = module.collect_connect_info(
        "http://127.0.0.1:54345",
        browser_id="browser-9",
        auto_open=True,
        timeout=3,
    )

    assert calls[-1] == ("/browser/open", {"id": "browser-9"})
    assert result["browser_id"] == "browser-9"
    assert result["debug_http_url"] == "http://127.0.0.1:60001"
    assert result["page_count"] == 1
