from contextlib import contextmanager
from pathlib import Path

from fastapi.testclient import TestClient

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.web.app import create_app
from src.web.routes import payment as payment_module


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def _create_client(monkeypatch, db_name: str) -> TestClient:
    seed_session = _build_session(db_name)
    manager = seed_session.bind
    seed_session.close()

    @contextmanager
    def fake_get_db():
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    monkeypatch.setattr(payment_module, "get_db", fake_get_db)
    return TestClient(create_app())


def _seed_account(session, **overrides):
    defaults = {
        "email": "payment@example.com",
        "password": "pass-1",
        "email_service": "outlook",
        "status": "active",
        "refresh_token": None,
    }
    defaults.update(overrides)
    account = Account(**defaults)
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def test_batch_check_subscription_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "payment_routes_batch_check_subscription_rt_filter.db")

    with payment_module.get_db() as db:
        first = _seed_account(
            db,
            email="sub-has-1@example.com",
            refresh_token="rt-1",
        )
        second = _seed_account(
            db,
            email="sub-has-2@example.com",
            refresh_token="rt-2",
        )
        first_id = first.id
        second_id = second.id
        _seed_account(
            db,
            email="sub-missing@example.com",
            refresh_token="",
        )

    checked_ids = []

    def fake_check_subscription_detail_with_retry(db, account, proxy, allow_token_refresh):
        checked_ids.append(account.id)
        return {"status": "free", "confidence": "low"}, False

    monkeypatch.setattr(
        payment_module,
        "_check_subscription_detail_with_retry",
        fake_check_subscription_detail_with_retry,
    )

    response = client.post(
        "/api/payment/accounts/batch-check-subscription",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["success_count"] == 2
    assert checked_ids == [first_id, second_id]
