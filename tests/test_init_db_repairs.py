from pathlib import Path

from src.config.constants import AccountStatus
from src.database.init_db import repair_partial_account_statuses
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


def test_repair_partial_account_statuses_reclassifies_active_partial_accounts():
    manager = _build_manager("repair_partial_accounts.db")

    with manager.session_scope() as session:
        session.add_all([
            Account(
                email="pending@example.com",
                password="known-pass",
                refresh_token="",
                email_service="outlook",
                status=AccountStatus.ACTIVE.value,
                source="register",
                extra_data={},
            ),
            Account(
                email="login@example.com",
                password="",
                refresh_token="",
                email_service="outlook",
                status=AccountStatus.ACTIVE.value,
                source="login",
                extra_data={"existing_account_detected": True},
            ),
            Account(
                email="healthy@example.com",
                password="healthy-pass",
                refresh_token="refresh-token",
                email_service="outlook",
                status=AccountStatus.ACTIVE.value,
                source="register",
                extra_data={},
            ),
        ])

    summary = repair_partial_account_statuses(manager.database_url)

    assert summary["updated"] == 2
    assert summary["token_pending"] == 1
    assert summary["login_incomplete"] == 1

    with manager.session_scope() as session:
        pending = session.query(Account).filter(Account.email == "pending@example.com").one()
        login = session.query(Account).filter(Account.email == "login@example.com").one()
        healthy = session.query(Account).filter(Account.email == "healthy@example.com").one()

        assert pending.status == AccountStatus.TOKEN_PENDING.value
        assert pending.extra_data["account_status_reason"] == "repair_missing_refresh_token"
        assert login.status == AccountStatus.LOGIN_INCOMPLETE.value
        assert login.extra_data["account_status_reason"] == "repair_missing_login_password"
        assert healthy.status == AccountStatus.ACTIVE.value
