import asyncio
from types import SimpleNamespace

from src.web.routes import settings as settings_module


def test_get_registration_settings_includes_token_completion_cap(monkeypatch):
    monkeypatch.setattr(
        settings_module,
        "get_settings",
        lambda: SimpleNamespace(
            registration_max_retries=3,
            registration_timeout=120,
            registration_default_password_length=12,
            registration_sleep_min=5,
            registration_sleep_max=30,
            registration_entry_flow="native",
            registration_token_completion_max_concurrency=7,
        ),
    )

    payload = asyncio.run(settings_module.get_registration_settings())

    assert payload["token_completion_max_concurrency"] == 7


def test_update_registration_settings_persists_token_completion_cap(monkeypatch):
    captured = {}

    monkeypatch.setattr(
        settings_module,
        "update_settings",
        lambda **kwargs: captured.update(kwargs),
    )

    request = settings_module.RegistrationSettings(
        max_retries=3,
        timeout=120,
        default_password_length=12,
        sleep_min=5,
        sleep_max=30,
        entry_flow="native",
        token_completion_max_concurrency=4,
    )

    payload = asyncio.run(settings_module.update_registration_settings(request))

    assert payload["success"] is True
    assert captured["registration_token_completion_max_concurrency"] == 4


def test_settings_template_exposes_token_completion_cap_field():
    template = open("templates/settings.html", "r", encoding="utf-8").read()

    assert 'id="token-completion-max-concurrency"' in template
    assert "Token 收尾最大并发数" in template


def test_settings_script_reads_and_writes_token_completion_cap():
    script = open("static/js/settings.js", "r", encoding="utf-8").read()

    assert "token-completion-max-concurrency" in script
    assert "token_completion_max_concurrency" in script
