import asyncio
from contextlib import contextmanager
from types import SimpleNamespace

import pytest

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
    assert 'id="database-mode-note"' in template


def test_settings_script_reads_and_writes_token_completion_cap():
    script = open("static/js/settings.js", "r", encoding="utf-8").read()

    assert "token-completion-max-concurrency" in script
    assert "token_completion_max_concurrency" in script
    assert "database_supports_file_backup" in script
    assert "database-mode-note" in script


def test_get_database_info_marks_postgresql_as_remote(monkeypatch):
    monkeypatch.setattr(
        settings_module,
        "get_settings",
        lambda: SimpleNamespace(database_url="postgresql+psycopg://codex:test@localhost:5432/codex"),
    )

    class FakeQuery:
        def __init__(self, count):
            self._count = count

        def count(self):
            return self._count

    class FakeDB:
        def query(self, model):
            counts = {
                "Account": 12,
                "EmailService": 3,
                "RegistrationTask": 5,
            }
            return FakeQuery(counts[model.__name__])

    @contextmanager
    def fake_get_db():
        yield FakeDB()

    monkeypatch.setattr(settings_module, "get_db", fake_get_db)

    payload = asyncio.run(settings_module.get_database_info())

    assert payload["database_engine"] == "postgresql"
    assert payload["database_size_bytes"] is None
    assert payload["database_size_display"] == "远程数据库"
    assert payload["database_supports_file_backup"] is False
    assert payload["database_supports_file_import"] is False
    assert payload["accounts_count"] == 12


def test_backup_database_rejects_non_sqlite(monkeypatch):
    monkeypatch.setattr(
        settings_module,
        "get_settings",
        lambda: SimpleNamespace(database_url="postgresql+psycopg://codex:test@localhost:5432/codex"),
    )

    with pytest.raises(settings_module.HTTPException) as exc_info:
        asyncio.run(settings_module.backup_database())

    assert exc_info.value.status_code == 400
    assert "SQLite 文件备份" in exc_info.value.detail
