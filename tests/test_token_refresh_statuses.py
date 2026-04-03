from pathlib import Path
from contextlib import contextmanager

from src.config.constants import AccountStatus
from src.core.openai import token_refresh as token_refresh_module
from src.core.openai.token_refresh import TokenRefreshResult
from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager


def _build_manager(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager


def test_validate_account_token_keeps_partial_status_when_refresh_token_is_missing(monkeypatch):
    manager = _build_manager("token_refresh_validate_partial.db")

    with manager.session_scope() as session:
        account = Account(
            email="pending@example.com",
            password="known-pass",
            access_token="access-token",
            refresh_token="",
            email_service="outlook",
            status=AccountStatus.TOKEN_PENDING.value,
            source="register",
        )
        session.add(account)
        session.flush()
        account_id = account.id

    class FakeManager:
        def __init__(self, proxy_url=None):
            self.proxy_url = proxy_url

        def validate_token(self, access_token):
            return True, None

    monkeypatch.setattr(token_refresh_module, "TokenRefreshManager", FakeManager)

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(token_refresh_module, "get_db", fake_get_db)

    is_valid, error = token_refresh_module.validate_account_token(account_id)

    assert is_valid is True
    assert error is None

    with manager.session_scope() as session:
        refreshed = session.query(Account).filter(Account.id == account_id).one()
        assert refreshed.status == AccountStatus.TOKEN_PENDING.value


def test_refresh_account_token_promotes_partial_status_after_refresh_token_is_filled(monkeypatch):
    manager = _build_manager("token_refresh_promote_active.db")

    with manager.session_scope() as session:
        account = Account(
            email="pending@example.com",
            password="known-pass",
            access_token="access-old",
            refresh_token="",
            email_service="outlook",
            status=AccountStatus.TOKEN_PENDING.value,
            source="register",
        )
        session.add(account)
        session.flush()
        account_id = account.id

    class FakeManager:
        def __init__(self, proxy_url=None):
            self.proxy_url = proxy_url

        def refresh_account(self, account):
            return TokenRefreshResult(
                success=True,
                access_token="access-new",
                refresh_token="refresh-new",
            )

    monkeypatch.setattr(token_refresh_module, "TokenRefreshManager", FakeManager)

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(token_refresh_module, "get_db", fake_get_db)

    result = token_refresh_module.refresh_account_token(account_id)

    assert result.success is True

    with manager.session_scope() as session:
        refreshed = session.query(Account).filter(Account.id == account_id).one()
        assert refreshed.access_token == "access-new"
        assert refreshed.refresh_token == "refresh-new"
        assert refreshed.status == AccountStatus.ACTIVE.value
