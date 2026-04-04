import asyncio
from contextlib import contextmanager
from pathlib import Path
from types import SimpleNamespace

from fastapi import BackgroundTasks

from src.database.models import Base, Account, EmailService
from src.database.session import DatabaseSessionManager
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


def _build_manager(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager


def test_derive_outlook_execution_state_matches_spec_priority():
    assert registration_module._derive_outlook_execution_state(None) == "unregistered"
    assert registration_module._derive_outlook_execution_state(
        SimpleNamespace(refresh_token="refresh-token")
    ) == "registered_complete"
    assert registration_module._derive_outlook_execution_state(
        SimpleNamespace(refresh_token="")
    ) == "registered_needs_token_refresh"


def test_derive_outlook_execution_state_currently_ignores_account_status():
    assert registration_module._derive_outlook_execution_state(
        SimpleNamespace(status="expired", refresh_token="")
    ) == "registered_needs_token_refresh"
    assert registration_module._derive_outlook_execution_state(
        SimpleNamespace(status="failed", refresh_token="refresh-token")
    ) == "registered_complete"


def test_registration_template_outlook_filter_contract_matches_frontend_helper():
    template = Path("templates/index.html").read_text(encoding="utf-8")

    assert 'id="outlook-account-status-filter"' in template
    assert '<option value="all">全部</option>' in template
    assert '<option value="unregistered">未注册</option>' in template
    assert '<option value="registered_needs_token_refresh">已注册待补Token</option>' in template
    assert '<option value="registered_complete">注册已完成</option>' in template
    assert 'onclick="selectExecutableOutlookAccounts()"' in template
    assert 'id="outlook-skip-registered"' not in template


def test_start_outlook_batch_registration_allows_registered_complete_accounts(monkeypatch):
    manager = _build_manager("registration_routes_skip_semantics.db")

    with manager.session_scope() as session:
        pending_service = EmailService(
            service_type="outlook",
            name="pending-refresh@outlook.com",
            config={"email": "pending-refresh@outlook.com"},
            enabled=True,
            priority=0,
        )
        complete_service = EmailService(
            service_type="outlook",
            name="complete@outlook.com",
            config={"email": "complete@outlook.com"},
            enabled=True,
            priority=1,
        )
        session.add_all([pending_service, complete_service])
        session.flush()

        session.add_all([
            Account(
                email="pending-refresh@outlook.com",
                password="pass-1",
                refresh_token="",
                email_service="outlook",
                status="active",
            ),
            Account(
                email="complete@outlook.com",
                password="pass-2",
                refresh_token="refresh-token-2",
                email_service="outlook",
                status="active",
            ),
        ])

        pending_id = pending_service.id
        complete_id = complete_service.id

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(registration_module, "get_db", fake_get_db)
    monkeypatch.setattr(registration_module.uuid, "uuid4", lambda: "batch-id-1")
    registration_module.batch_tasks.clear()

    request = registration_module.OutlookBatchRegistrationRequest(
        service_ids=[pending_id, complete_id],
        interval_min=1,
        interval_max=2,
    )

    response = asyncio.run(
        registration_module.start_outlook_batch_registration(request, BackgroundTasks())
    )

    assert response.batch_id == "batch-id-1"
    assert response.total == 2
    assert response.skipped == 0
    assert response.to_register == 2
    assert response.service_ids == [pending_id, complete_id]


def test_cancel_task_marks_runtime_cancel_flag_and_cancelling_status(monkeypatch):
    manager = _build_manager("registration_routes_cancel_single.db")

    with manager.session_scope() as session:
        session.add(
            registration_module.RegistrationTask(
                task_uuid="task-cancel-route",
                status="running",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(registration_module, "get_db", fake_get_db)
    registration_module.task_manager.cleanup_task("task-cancel-route")

    response = asyncio.run(registration_module.cancel_task("task-cancel-route"))

    with manager.session_scope() as session:
        task = registration_module.crud.get_registration_task(session, "task-cancel-route")
        task_status = task.status if task is not None else None

    assert response["success"] is True
    assert task is not None
    assert task_status == "cancelling"
    assert registration_module.task_manager.is_cancelled("task-cancel-route") is True


def test_cancel_batch_cascades_to_child_tasks():
    batch_id = "batch-cancel-cascade"
    child_task_ids = ["batch-child-1", "batch-child-2"]
    registration_module.batch_tasks[batch_id] = {
        "total": 2,
        "completed": 0,
        "success": 0,
        "failed": 0,
        "cancelled": False,
        "task_uuids": child_task_ids,
        "current_index": 0,
        "logs": [],
        "finished": False,
    }

    try:
        response = asyncio.run(registration_module.cancel_batch(batch_id))

        assert response["success"] is True
        assert registration_module.batch_tasks[batch_id]["cancelled"] is True
        assert registration_module.task_manager.is_batch_cancelled(batch_id) is True
        assert registration_module.task_manager.is_cancelled("batch-child-1") is True
        assert registration_module.task_manager.is_cancelled("batch-child-2") is True
    finally:
        registration_module.batch_tasks.pop(batch_id, None)
        registration_module.task_manager.cleanup_task("batch-child-1")
        registration_module.task_manager.cleanup_task("batch-child-2")


def test_get_task_logs_prefers_runtime_log_queue(monkeypatch):
    manager = _build_manager("registration_routes_runtime_logs.db")

    with manager.session_scope() as session:
        session.add(
            registration_module.RegistrationTask(
                task_uuid="task-runtime-log",
                status="running",
                logs="persisted-log",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(registration_module, "get_db", fake_get_db)
    registration_module.task_manager.add_log("task-runtime-log", "live-log-1")

    try:
        payload = asyncio.run(registration_module.get_task_logs("task-runtime-log"))
    finally:
        registration_module.task_manager.cleanup_task("task-runtime-log")

    assert payload["logs"] == ["live-log-1"]
