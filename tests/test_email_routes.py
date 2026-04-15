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


async def _read_streaming_response_body(response) -> str:
    chunks = []
    async for chunk in response.body_iterator:
        chunks.append(chunk)
    return b"".join(chunks).decode("utf-8")


def test_list_email_services_supports_outlook_search_status_and_pagination(monkeypatch):
    seed_session = _build_session("email_routes_outlook_filter_pagination.db")
    manager = seed_session.bind

    registered = _seed_account(seed_session, email="matched@example.com")
    registered_id = registered.id
    _seed_email_service(
        seed_session,
        name="Matched Registered",
        config={"email": "Matched@Example.com", "password": "secret-1"},
        priority=0,
    )
    _seed_email_service(
        seed_session,
        name="Matched Pending",
        config={"email": "pending@example.com", "password": "secret-2"},
        priority=1,
    )
    _seed_email_service(
        seed_session,
        name="Skipped Outlook",
        config={"email": "skipped@example.com", "password": "secret-3"},
        priority=2,
    )
    seed_session.close()

    @contextmanager
    def fake_get_db():
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    monkeypatch.setattr(email_module, "get_db", fake_get_db)

    page_one = asyncio.run(
        email_module.list_email_services(
            service_type="outlook",
            enabled_only=False,
            search="Matched@Example.com----secret-1\npending@example.com----secret-2",
            registration_status="all",
            page=1,
            page_size=1,
        )
    )
    page_two = asyncio.run(
        email_module.list_email_services(
            service_type="outlook",
            enabled_only=False,
            search="Matched@Example.com----secret-1\npending@example.com----secret-2",
            registration_status="all",
            page=2,
            page_size=1,
        )
    )
    registered_only = asyncio.run(
        email_module.list_email_services(
            service_type="outlook",
            enabled_only=False,
            search=None,
            registration_status="registered",
            page=1,
            page_size=20,
        )
    )

    assert page_one.total == 2
    assert page_one.page == 1
    assert page_one.page_size == 1
    assert page_one.total_pages == 2
    assert [service.name for service in page_one.services] == ["Matched Registered"]
    assert [service.name for service in page_two.services] == ["Matched Pending"]
    assert registered_only.total == 1
    assert [service.name for service in registered_only.services] == ["Matched Registered"]
    assert registered_only.services[0].registered_account_id == registered_id


def test_export_outlook_services_returns_import_compatible_lines(monkeypatch):
    seed_session = _build_session("email_routes_outlook_export.db")
    manager = seed_session.bind

    plain = _seed_email_service(
        seed_session,
        name="plain@example.com",
        config={"email": "plain@example.com", "password": "plain-secret"},
        priority=1,
    )
    plain_id = plain.id
    oauth = _seed_email_service(
        seed_session,
        name="oauth@example.com",
        config={
            "email": "oauth@example.com",
            "password": "oauth-secret",
            "client_id": "client-id-1",
            "refresh_token": "refresh-token-1",
        },
        priority=0,
    )
    oauth_id = oauth.id
    seed_session.close()

    @contextmanager
    def fake_get_db():
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    monkeypatch.setattr(email_module, "get_db", fake_get_db)

    response = asyncio.run(
        email_module.export_outlook_services(
            email_module.OutlookExportRequest(service_ids=[plain_id, oauth_id])
        )
    )
    body = asyncio.run(_read_streaming_response_body(response))

    assert response.headers["content-type"].startswith("text/plain")
    assert "attachment;" in response.headers["content-disposition"]
    assert body.splitlines() == [
        "oauth@example.com----oauth-secret----client-id-1----refresh-token-1",
        "plain@example.com----plain-secret",
    ]
