import asyncio
from contextlib import contextmanager
from pathlib import Path

from src.database.models import Account, Base, EmailService
from src.database.session import DatabaseSessionManager
from src.web.routes import email as email_module


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def _seed_account(session, **overrides):
    defaults = {
        "email": "user@example.com",
        "password": "pass-1",
        "email_service": "outlook",
        "status": "active",
    }
    defaults.update(overrides)
    account = Account(**defaults)
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def _seed_email_service(session, **overrides):
    defaults = {
        "service_type": "outlook",
        "name": "Outlook Account",
        "config": {"email": "user@example.com", "password": "secret"},
        "enabled": True,
        "priority": 0,
    }
    defaults.update(overrides)
    service = EmailService(**defaults)
    session.add(service)
    session.commit()
    session.refresh(service)
    return service


def test_list_email_services_batches_outlook_registration_lookup(monkeypatch):
    seed_session = _build_session("email_routes_batch_lookup.db")
    manager = seed_session.bind

    registered = _seed_account(seed_session, email="mixedcase@example.com")
    registered_id = registered.id
    _seed_email_service(
        seed_session,
        name="Registered Outlook",
        config={"email": "MixedCase@Example.com", "password": "secret"},
    )
    _seed_email_service(
        seed_session,
        name="Unregistered Outlook",
        config={"email": "missing@example.com", "password": "secret"},
        priority=1,
    )
    _seed_email_service(
        seed_session,
        service_type="moe_mail",
        name="Custom Mail",
        config={"base_url": "https://mail.example.com", "api_key": "secret"},
        priority=2,
    )
    seed_session.close()

    get_db_calls = 0

    @contextmanager
    def fake_get_db():
        nonlocal get_db_calls
        get_db_calls += 1
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    monkeypatch.setattr(email_module, "get_db", fake_get_db)

    result = asyncio.run(email_module.list_email_services(service_type=None, enabled_only=False))

    assert get_db_calls == 1
    assert result.total == 3

    services_by_name = {service.name: service for service in result.services}
    assert services_by_name["Registered Outlook"].registration_status == "registered"
    assert services_by_name["Registered Outlook"].registered_account_id == registered_id
    assert services_by_name["Unregistered Outlook"].registration_status == "unregistered"
    assert services_by_name["Custom Mail"].registration_status is None
