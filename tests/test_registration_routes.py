import asyncio
import sqlite3
from contextlib import contextmanager
from collections import Counter
from pathlib import Path
from types import SimpleNamespace

from fastapi import BackgroundTasks
from sqlalchemy.exc import OperationalError
from sqlalchemy import event

from src.core import dynamic_proxy as dynamic_proxy_module
from src.database import crud as database_crud
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
    assert 'textarea\n                                                id="outlook-account-search"' in template
    assert "多个邮箱可按回车分隔" in template
    assert "也支持直接粘贴 Outlook 导入行" in template
    assert "筛出多个结果后可继续多选" in template
    assert '<option value="all">全部</option>' in template
    assert '<option value="unregistered">未注册</option>' in template
    assert '<option value="registered_needs_token_refresh">已注册待补Token</option>' in template
    assert '<option value="registered_complete">注册已完成</option>' in template
    assert 'onclick="selectExecutableOutlookAccounts()"' in template
    assert 'id="outlook-skip-registered"' not in template


def test_registration_template_exposes_pause_resume_button():
    template = Path("templates/index.html").read_text(encoding="utf-8")

    assert 'id="pause-btn"' in template
    assert "暂停任务" in template


def test_registration_frontend_status_map_supports_paused():
    script = Path("static/js/app.js").read_text(encoding="utf-8")

    assert "paused: { text: '已暂停'" in script
    assert "function handlePauseResumeTask()" in script


def test_registration_template_exposes_chatgpt_registration_mode_selector():
    template = Path("templates/index.html").read_text(encoding="utf-8")

    assert 'id="chatgpt-registration-mode"' in template
    assert '<option value="refresh_token">有 RT（推荐）</option>' in template
    assert '<option value="access_token_only">无 RT（兼容）</option>' in template


def test_registration_frontend_submits_chatgpt_registration_mode():
    script = Path("static/js/app.js").read_text(encoding="utf-8")

    assert "chatgpt_registration_mode: elements.chatgptRegistrationMode.value" in script


def test_get_proxy_for_registration_prefers_dynamic_proxy_over_proxy_pool(monkeypatch):
    monkeypatch.setattr(
        registration_module,
        "get_settings",
        lambda: SimpleNamespace(
            proxy_dynamic_enabled=True,
            proxy_dynamic_api_url="https://proxy.example.com/get",
            proxy_dynamic_api_key=None,
            proxy_dynamic_api_key_header="X-API-Key",
            proxy_dynamic_result_field="",
            proxy_url="http://static.proxy:8080",
        ),
    )
    monkeypatch.setattr(
        dynamic_proxy_module,
        "fetch_dynamic_proxy",
        lambda **kwargs: "http://dynamic.proxy:9000",
    )
    monkeypatch.setattr(
        registration_module.crud,
        "get_random_proxy",
        lambda db: SimpleNamespace(id=42, proxy_url="http://pool.proxy:8000"),
    )

    proxy_url, proxy_id = registration_module.get_proxy_for_registration(db=None)

    assert proxy_url == "http://dynamic.proxy:9000"
    assert proxy_id is None


def test_get_proxy_for_registration_falls_back_to_proxy_pool_when_dynamic_proxy_unavailable(monkeypatch):
    monkeypatch.setattr(
        registration_module,
        "get_settings",
        lambda: SimpleNamespace(
            proxy_dynamic_enabled=True,
            proxy_dynamic_api_url="https://proxy.example.com/get",
            proxy_dynamic_api_key=None,
            proxy_dynamic_api_key_header="X-API-Key",
            proxy_dynamic_result_field="",
            proxy_url="http://static.proxy:8080",
        ),
    )
    monkeypatch.setattr(dynamic_proxy_module, "fetch_dynamic_proxy", lambda **kwargs: None)
    monkeypatch.setattr(
        registration_module.crud,
        "get_random_proxy",
        lambda db: SimpleNamespace(id=7, proxy_url="http://pool.proxy:8000"),
    )

    proxy_url, proxy_id = registration_module.get_proxy_for_registration(db=None)

    assert proxy_url == "http://pool.proxy:8000"
    assert proxy_id == 7


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


def test_get_outlook_accounts_for_registration_avoids_n_plus_one_account_queries(monkeypatch):
    manager = _build_manager("registration_routes_outlook_accounts_batch.db")

    with manager.session_scope() as session:
        session.add_all([
            EmailService(
                service_type="outlook",
                name="first@outlook.com",
                config={"email": "first@outlook.com"},
                enabled=True,
                priority=0,
            ),
            EmailService(
                service_type="outlook",
                name="second@outlook.com",
                config={"email": "second@outlook.com"},
                enabled=True,
                priority=1,
            ),
            EmailService(
                service_type="outlook",
                name="third@outlook.com",
                config={"email": "third@outlook.com"},
                enabled=True,
                priority=2,
            ),
        ])
        session.add(
            Account(
                email="second@outlook.com",
                password="pass-2",
                refresh_token="refresh-token-2",
                email_service="outlook",
                status="active",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    query_counter = Counter()

    def _count_accounts_selects(_conn, _cursor, statement, _params, _context, _executemany):
        normalized = " ".join(str(statement).lower().split())
        if normalized.startswith("select") and " from accounts" in normalized:
            query_counter["accounts_select"] += 1

    event.listen(manager.engine, "before_cursor_execute", _count_accounts_selects)
    monkeypatch.setattr(registration_module, "get_db", fake_get_db)

    try:
        payload = asyncio.run(registration_module.get_outlook_accounts_for_registration())
    finally:
        event.remove(manager.engine, "before_cursor_execute", _count_accounts_selects)

    assert payload.total == 3
    assert payload.registered_count == 1
    assert query_counter["accounts_select"] == 1


def test_get_outlook_accounts_for_registration_matches_registered_accounts_by_normalized_email(monkeypatch):
    manager = _build_manager("registration_routes_outlook_accounts_normalized_match.db")

    with manager.session_scope() as session:
        session.add(
            EmailService(
                service_type="outlook",
                name="MiXeD@Outlook.com",
                config={"email": "MiXeD@Outlook.com"},
                enabled=True,
                priority=0,
            )
        )
        session.add(
            Account(
                email="mixed@outlook.com",
                password="pass-1",
                refresh_token="refresh-token-1",
                email_service="outlook",
                status="active",
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

    payload = asyncio.run(registration_module.get_outlook_accounts_for_registration())

    assert payload.total == 1
    assert payload.registered_count == 1
    assert payload.unregistered_count == 0
    assert payload.accounts[0].is_registered is True
    assert payload.accounts[0].registered_account_id is not None


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


def test_pause_and_resume_task_roundtrip_updates_status(monkeypatch):
    manager = _build_manager("registration_routes_pause_resume_single.db")

    with manager.session_scope() as session:
        session.add(
            registration_module.RegistrationTask(
                task_uuid="task-pause-route",
                status="running",
                started_at=registration_module.datetime.utcnow(),
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
    registration_module.task_manager.cleanup_task("task-pause-route")

    pause_response = asyncio.run(registration_module.pause_task("task-pause-route"))
    resume_response = asyncio.run(registration_module.resume_task("task-pause-route"))

    with manager.session_scope() as session:
        task = registration_module.crud.get_registration_task(session, "task-pause-route")
        task_status = task.status if task is not None else None

    assert pause_response["success"] is True
    assert resume_response["success"] is True
    assert task_status == "running"
    assert registration_module.task_manager.is_paused("task-pause-route") is False


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


def test_pause_and_resume_batch_cascade_to_child_tasks(monkeypatch):
    manager = _build_manager("registration_routes_pause_resume_batch.db")

    with manager.session_scope() as session:
        session.add_all([
            registration_module.RegistrationTask(
                task_uuid="batch-pause-child-1",
                status="running",
                started_at=registration_module.datetime.utcnow(),
            ),
            registration_module.RegistrationTask(
                task_uuid="batch-pause-child-2",
                status="pending",
            ),
        ])

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    monkeypatch.setattr(registration_module, "get_db", fake_get_db)
    batch_id = "batch-pause-route"
    registration_module.batch_tasks[batch_id] = {
        "total": 2,
        "completed": 0,
        "success": 0,
        "failed": 0,
        "cancelled": False,
        "paused": False,
        "task_uuids": ["batch-pause-child-1", "batch-pause-child-2"],
        "current_index": 0,
        "logs": [],
        "finished": False,
    }
    registration_module.task_manager.bind_batch_tasks(batch_id, ["batch-pause-child-1", "batch-pause-child-2"])

    try:
        pause_response = asyncio.run(registration_module.pause_batch(batch_id))
        resume_response = asyncio.run(registration_module.resume_batch(batch_id))

        with manager.session_scope() as session:
            child1 = registration_module.crud.get_registration_task(session, "batch-pause-child-1")
            child2 = registration_module.crud.get_registration_task(session, "batch-pause-child-2")
            child1_status = child1.status if child1 is not None else None
            child2_status = child2.status if child2 is not None else None

        assert pause_response["success"] is True
        assert resume_response["success"] is True
        assert child1_status == "running"
        assert child2_status == "pending"
        assert registration_module.batch_tasks[batch_id]["paused"] is False
        assert registration_module.task_manager.is_paused("batch-pause-child-1") is False
        assert registration_module.task_manager.is_paused("batch-pause-child-2") is False
    finally:
        registration_module.batch_tasks.pop(batch_id, None)
        registration_module.task_manager.cleanup_task("batch-pause-child-1")
        registration_module.task_manager.cleanup_task("batch-pause-child-2")


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


def test_get_task_logs_supports_incremental_offset(monkeypatch):
    manager = _build_manager("registration_routes_incremental_task_logs.db")

    with manager.session_scope() as session:
        session.add(
            registration_module.RegistrationTask(
                task_uuid="task-incremental-log",
                status="running",
                logs="persisted-log-0\npersisted-log-1\npersisted-log-2",
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

    payload = asyncio.run(
        registration_module.get_task_logs("task-incremental-log", offset=2)
    )

    assert payload["logs"] == ["persisted-log-2"]
    assert payload["log_offset"] == 2
    assert payload["log_next_offset"] == 3


def test_get_batch_status_prefers_runtime_batch_logs():
    batch_id = "batch-runtime-logs"
    registration_module.batch_tasks[batch_id] = {
        "total": 2,
        "completed": 1,
        "success": 1,
        "failed": 0,
        "cancelled": False,
        "task_uuids": ["task-a", "task-b"],
        "current_index": 1,
        "logs": [],
        "finished": False,
    }
    registration_module.task_manager.init_batch(batch_id, 2)
    registration_module.task_manager.add_batch_log(batch_id, "[任务1] live-batch-log")

    try:
        payload = asyncio.run(registration_module.get_batch_status(batch_id))
    finally:
        registration_module.batch_tasks.pop(batch_id, None)

    assert payload["logs"] == ["[任务1] live-batch-log"]


def test_get_batch_status_supports_incremental_offset_after_runtime_window_rotates():
    batch_id = "batch-runtime-window-rotates"
    registration_module.batch_tasks[batch_id] = {
        "total": 1205,
        "completed": 1000,
        "success": 1000,
        "failed": 0,
        "cancelled": False,
        "task_uuids": [],
        "current_index": 1000,
        "logs": [],
        "finished": False,
    }
    registration_module.task_manager.init_batch(batch_id, 1205)

    try:
        for index in range(1205):
            registration_module.task_manager.add_batch_log(batch_id, f"log-{index}")

        payload = asyncio.run(
            registration_module.get_batch_status(batch_id, log_offset=1000)
        )
    finally:
        registration_module.batch_tasks.pop(batch_id, None)
        from src.web.task_manager import _batch_logs, _batch_log_start_index  # type: ignore
        _batch_logs.pop(batch_id, None)
        _batch_log_start_index.pop(batch_id, None)

    assert payload["log_base_index"] == 205
    assert payload["log_next_offset"] == 1205
    assert payload["logs"][0] == "log-1000"
    assert payload["logs"][-1] == "log-1204"


def test_get_outlook_batch_status_prefers_runtime_batch_logs():
    batch_id = "outlook-batch-runtime-logs"
    registration_module.batch_tasks[batch_id] = {
        "total": 2,
        "completed": 1,
        "success": 1,
        "failed": 0,
        "skipped": 0,
        "current_index": 1,
        "cancelled": False,
        "finished": False,
        "logs": ["persisted-summary-log"],
    }

    registration_module.task_manager.add_batch_log(batch_id, "[任务1] 实时日志")

    try:
        payload = asyncio.run(registration_module.get_outlook_batch_status(batch_id))
    finally:
        registration_module.batch_tasks.pop(batch_id, None)

    assert payload["logs"] == ["[任务1] 实时日志"]


def test_run_batch_parallel_emits_start_logs_for_each_task(monkeypatch):
    batch_id = "batch-parallel-start-logs"
    task_uuids = ["task-a", "task-b"]

    async def fake_run_registration_task(*args, **kwargs):
        return None

    @contextmanager
    def fake_get_db():
        yield None

    monkeypatch.setattr(registration_module, "run_registration_task", fake_run_registration_task)
    monkeypatch.setattr(registration_module, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_module.crud,
        "get_registration_task",
        lambda db, uuid: SimpleNamespace(status="completed", error_message=None),
    )

    try:
        asyncio.run(
            registration_module.run_batch_parallel(
                batch_id,
                task_uuids,
                "tempmail",
                None,
                None,
                None,
                concurrency=2,
            )
        )
        logs = registration_module.batch_tasks[batch_id]["logs"]
    finally:
        registration_module.batch_tasks.pop(batch_id, None)
        from src.web.task_manager import _batch_logs, _batch_status  # type: ignore

        _batch_logs.pop(batch_id, None)
        _batch_status.pop(batch_id, None)

    assert "[任务1] 开始注册..." in logs
    assert "[任务2] 开始注册..." in logs


def test_update_registration_task_retries_transient_sqlite_lock(monkeypatch):
    task = SimpleNamespace(status="pending", proxy=None)

    class FakeBind:
        dialect = SimpleNamespace(name="sqlite")

    class FakeSession:
        def __init__(self):
            self.bind = FakeBind()
            self.commit_calls = 0
            self.rollback_calls = 0
            self.refresh_calls = 0

        def commit(self):
            self.commit_calls += 1
            if self.commit_calls == 1:
                raise OperationalError(
                    "UPDATE registration_tasks SET proxy=? WHERE registration_tasks.id = ?",
                    {"proxy": "http://retry-proxy"},
                    sqlite3.OperationalError("database is locked"),
                )

        def rollback(self):
            self.rollback_calls += 1

        def refresh(self, _task):
            self.refresh_calls += 1

    fake_db = FakeSession()

    monkeypatch.setattr(
        database_crud,
        "get_registration_task_by_uuid",
        lambda db, task_uuid: task if task_uuid == "task-retry-lock" else None,
    )
    monkeypatch.setattr(database_crud.time, "sleep", lambda seconds: None)

    updated = database_crud.update_registration_task(
        fake_db,
        "task-retry-lock",
        status="running",
        proxy="http://retry-proxy",
    )

    assert updated is task
    assert task.status == "running"
    assert task.proxy == "http://retry-proxy"
    assert fake_db.commit_calls == 2
    assert fake_db.rollback_calls == 1
    assert fake_db.refresh_calls == 1


def test_safe_update_registration_task_skips_transient_sqlite_lock(monkeypatch):
    manager = _build_manager("registration_routes_safe_update_locked.db")

    with manager.session_scope() as session:
        session.add(
            registration_module.RegistrationTask(
                task_uuid="task-safe-update-lock",
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

    original_update = registration_module.crud.update_registration_task

    def flaky_update(db, task_uuid, **kwargs):
        if kwargs.get("proxy") == "http://proxy.test":
            raise OperationalError(
                "UPDATE registration_tasks SET proxy=? WHERE registration_tasks.id = ?",
                {"proxy": "http://proxy.test"},
                sqlite3.OperationalError("database is locked"),
            )
        return original_update(db, task_uuid, **kwargs)

    monkeypatch.setattr(registration_module.crud, "update_registration_task", flaky_update)

    with fake_get_db() as db:
        result = registration_module._safe_update_registration_task(
            db,
            "task-safe-update-lock",
            context="测试 proxy 写入",
            proxy="http://proxy.test",
        )

    assert result is None
