from types import SimpleNamespace

from src.web.routes import registration as registration_module


def test_is_account_registration_complete_requires_refresh_token():
    account = SimpleNamespace(status="active", refresh_token="")

    assert registration_module._is_account_registration_complete(account) is False


def test_is_account_registration_complete_with_refresh_token_is_skippable():
    account = SimpleNamespace(status="active", refresh_token="refresh-token-1")

    assert registration_module._is_account_registration_complete(account) is True


def test_needs_token_refresh_for_registered_account_without_refresh_token():
    account = SimpleNamespace(status="active", refresh_token="")

    assert registration_module._needs_token_refresh(account) is True
