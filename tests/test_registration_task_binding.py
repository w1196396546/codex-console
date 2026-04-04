import sqlite3
import threading
from contextlib import contextmanager
from pathlib import Path
from types import SimpleNamespace

from sqlalchemy.exc import OperationalError

from src.database.models import Base, EmailService, RegistrationTask
from src.database.session import DatabaseSessionManager
from src.web.routes import registration as registration_routes
from src.web.task_manager import task_manager


def _build_manager(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager


def test_sync_registration_task_prefers_bound_email_service_id(monkeypatch):
    manager = _build_manager("registration_task_binding.db")

    with manager.session_scope() as session:
        first = EmailService(
            service_type="outlook",
            name="first@example.com",
            config={"email": "first@example.com", "password": "pass-1"},
            enabled=True,
            priority=0,
        )
        second = EmailService(
            service_type="outlook",
            name="second@example.com",
            config={"email": "second@example.com", "password": "pass-2"},
            enabled=True,
            priority=1,
        )
        session.add_all([first, second])
        session.flush()

        task = RegistrationTask(
            task_uuid="task-bound-service",
            status="pending",
            email_service_id=second.id,
        )
        session.add(task)

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    captured = {}

    def fake_create(service_type, config):
        captured["service_type"] = service_type.value if hasattr(service_type, "value") else str(service_type)
        captured["email"] = config.get("email")
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None):
            self.email_service = email_service

        def run(self):
            return SimpleNamespace(
                success=False,
                email="",
                error_message="stop after selection",
                to_dict=lambda: {},
            )

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(registration_routes, "get_settings", lambda: SimpleNamespace())
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(registration_routes.task_manager, "update_status", lambda *args, **kwargs: None)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes._run_sync_registration_task(
        task_uuid="task-bound-service",
        email_service_type="outlook",
        proxy=None,
        email_service_config=None,
        email_service_id=None,
    )

    assert captured["service_type"] == "outlook"
    assert captured["email"] == "second@example.com"


def test_sync_registration_task_skips_outlook_services_claimed_by_other_tasks(monkeypatch):
    manager = _build_manager("registration_task_claimed_service.db")

    with manager.session_scope() as session:
        first = EmailService(
            service_type="outlook",
            name="first@example.com",
            config={"email": "first@example.com", "password": "pass-1"},
            enabled=True,
            priority=0,
        )
        second = EmailService(
            service_type="outlook",
            name="second@example.com",
            config={"email": "second@example.com", "password": "pass-2"},
            enabled=True,
            priority=1,
        )
        session.add_all([first, second])
        session.flush()

        session.add_all([
            RegistrationTask(
                task_uuid="task-claimed-service",
                status="running",
                email_service_id=first.id,
            ),
            RegistrationTask(
                task_uuid="task-auto-pick",
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

    captured = {}

    def fake_create(service_type, config):
        captured["service_type"] = service_type.value if hasattr(service_type, "value") else str(service_type)
        captured["email"] = config.get("email")
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None):
            self.email_service = email_service

        def run(self):
            return SimpleNamespace(
                success=False,
                email="",
                error_message="stop after selection",
                to_dict=lambda: {},
            )

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(registration_routes, "get_settings", lambda: SimpleNamespace())
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(registration_routes.task_manager, "update_status", lambda *args, **kwargs: None)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes._run_sync_registration_task(
        task_uuid="task-auto-pick",
        email_service_type="outlook",
        proxy=None,
        email_service_config=None,
        email_service_id=None,
    )

    assert captured["service_type"] == "outlook"
    assert captured["email"] == "second@example.com"


def test_sync_registration_task_marks_cancelled_when_engine_detects_runtime_cancel(monkeypatch):
    manager = _build_manager("registration_task_runtime_cancel.db")

    with manager.session_scope() as session:
        session.add(
            RegistrationTask(
                task_uuid="task-runtime-cancel",
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

    captured = {}

    def fake_create(service_type, config):
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(
            self,
            email_service,
            proxy_url=None,
            callback_logger=None,
            task_uuid=None,
            check_cancelled=None,
        ):
            captured["task_uuid"] = task_uuid
            captured["check_cancelled"] = check_cancelled

        def run(self):
            assert callable(captured["check_cancelled"])
            registration_routes.task_manager.cancel_task(captured["task_uuid"])
            captured["check_cancelled"]()

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_routes,
        "get_settings",
        lambda: SimpleNamespace(
            tempmail_enabled=True,
            tempmail_base_url="https://mail.test",
            tempmail_timeout=10,
            tempmail_max_retries=1,
        ),
    )
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(registration_routes.task_manager, "update_status", lambda *args, **kwargs: None)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes._run_sync_registration_task(
        task_uuid="task-runtime-cancel",
        email_service_type="tempmail",
        proxy=None,
        email_service_config=None,
        email_service_id=None,
    )

    with manager.session_scope() as session:
        task = registration_routes.crud.get_registration_task(session, "task-runtime-cancel")
        task_status = task.status if task is not None else None

    assert task is not None
    assert task_status == "cancelled"


def test_sync_registration_task_blocks_while_paused_until_resumed(monkeypatch):
    manager = _build_manager("registration_task_pause_resume.db")

    with manager.session_scope() as session:
        session.add(
            RegistrationTask(
                task_uuid="task-runtime-pause",
                status="pending",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    started = threading.Event()
    finished = threading.Event()
    released = {"ran": False}

    def fake_create(service_type, config):
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None, check_cancelled=None):
            self.check_cancelled = check_cancelled

        def run(self):
            started.set()
            released["ran"] = True
            finished.set()
            return SimpleNamespace(
                success=False,
                email="",
                error_message="pause-finished",
                to_dict=lambda: {},
            )

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_routes,
        "get_settings",
        lambda: SimpleNamespace(
            tempmail_enabled=True,
            tempmail_base_url="https://mail.test",
            tempmail_timeout=10,
            tempmail_max_retries=1,
        ),
    )
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes.task_manager.pause_task("task-runtime-pause")
    worker = threading.Thread(
        target=registration_routes._run_sync_registration_task,
        kwargs={
            "task_uuid": "task-runtime-pause",
            "email_service_type": "tempmail",
            "proxy": None,
            "email_service_config": None,
            "email_service_id": None,
        },
        daemon=True,
    )
    worker.start()

    assert started.wait(0.2) is False
    assert released["ran"] is False

    registration_routes.task_manager.resume_task("task-runtime-pause")

    worker.join(timeout=1.0)
    assert worker.is_alive() is False
    assert finished.is_set() is True
    assert released["ran"] is True


def test_sync_registration_task_cancel_breaks_pause_wait(monkeypatch):
    manager = _build_manager("registration_task_pause_cancel.db")

    with manager.session_scope() as session:
        session.add(
            RegistrationTask(
                task_uuid="task-runtime-pause-cancel",
                status="pending",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    executed = {"ran": False}

    def fake_create(service_type, config):
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None, check_cancelled=None):
            return None

        def run(self):
            executed["ran"] = True
            return SimpleNamespace(
                success=False,
                email="",
                error_message="should-not-run",
                to_dict=lambda: {},
            )

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_routes,
        "get_settings",
        lambda: SimpleNamespace(
            tempmail_enabled=True,
            tempmail_base_url="https://mail.test",
            tempmail_timeout=10,
            tempmail_max_retries=1,
        ),
    )
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes.task_manager.pause_task("task-runtime-pause-cancel")
    worker = threading.Thread(
        target=registration_routes._run_sync_registration_task,
        kwargs={
            "task_uuid": "task-runtime-pause-cancel",
            "email_service_type": "tempmail",
            "proxy": None,
            "email_service_config": None,
            "email_service_id": None,
        },
        daemon=True,
    )
    worker.start()

    registration_routes.task_manager.cancel_task("task-runtime-pause-cancel")
    worker.join(timeout=1.0)

    with manager.session_scope() as session:
        task = registration_routes.crud.get_registration_task(session, "task-runtime-pause-cancel")
        task_status = task.status if task is not None else None

    assert worker.is_alive() is False
    assert executed["ran"] is False
    assert task_status == "cancelled"


def test_update_registration_task_retries_when_sqlite_is_temporarily_locked(monkeypatch):
    db_task = RegistrationTask(task_uuid="task-retry", status="pending")

    class DummySession:
        def __init__(self):
            self.commit_attempts = 0
            self.refresh_calls = 0

        def commit(self):
            self.commit_attempts += 1
            if self.commit_attempts < 3:
                raise OperationalError("UPDATE registration_tasks", {}, Exception("database is locked"))

        def rollback(self):
            return None

        def refresh(self, obj):
            self.refresh_calls += 1

    session = DummySession()
    monkeypatch.setattr(registration_routes.crud, "get_registration_task_by_uuid", lambda db, task_uuid: db_task)

    original_status = db_task.status
    updated = registration_routes.crud.update_registration_task(
        session,
        "task-retry",
        status="running",
    )

    assert updated is db_task
    assert original_status == "pending"
    assert db_task.status == "running"
    assert session.commit_attempts == 3
    assert session.refresh_calls == 1


def test_sync_registration_task_continues_when_proxy_update_hits_sqlite_lock(monkeypatch):
    manager = _build_manager("registration_task_proxy_lock.db")

    with manager.session_scope() as session:
        session.add(
            RegistrationTask(
                task_uuid="task-proxy-lock",
                status="pending",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    def fake_create(service_type, config):
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(self, email_service, proxy_url=None, callback_logger=None, task_uuid=None, check_cancelled=None):
            self.proxy_url = proxy_url

        def run(self):
            return SimpleNamespace(
                success=False,
                email="",
                error_message="engine-finished",
                to_dict=lambda: {},
            )

    original_update = registration_routes.crud.update_registration_task

    def flaky_update(db, task_uuid, **kwargs):
        if kwargs.get("proxy") == "http://proxy.test":
            raise OperationalError(
                "UPDATE registration_tasks SET proxy=? WHERE registration_tasks.id = ?",
                {"proxy": "http://proxy.test"},
                sqlite3.OperationalError("database is locked"),
            )
        return original_update(db, task_uuid, **kwargs)

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_routes,
        "get_settings",
        lambda: SimpleNamespace(
            tempmail_enabled=True,
            tempmail_base_url="https://mail.test",
            tempmail_timeout=10,
            tempmail_max_retries=1,
        ),
    )
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: ("http://proxy.test", None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(registration_routes.crud, "update_registration_task", flaky_update)
    monkeypatch.setattr(registration_routes.task_manager, "update_status", lambda *args, **kwargs: None)
    monkeypatch.setattr(
        registration_routes.task_manager,
        "create_log_callback",
        lambda *args, **kwargs: (lambda message: None),
    )

    registration_routes._run_sync_registration_task(
        task_uuid="task-proxy-lock",
        email_service_type="tempmail",
        proxy=None,
        email_service_config=None,
        email_service_id=None,
    )

    with manager.session_scope() as session:
        task = registration_routes.crud.get_registration_task(session, "task-proxy-lock")
        task_status = task.status if task is not None else None
        task_error = task.error_message if task is not None else None

    assert task_status == "failed"
    assert task_error == "engine-finished"


def test_create_log_callback_forwards_child_logs_to_batch_channel(monkeypatch):
    captured = {"task": [], "batch": []}

    monkeypatch.setattr(task_manager, "add_log", lambda task_uuid, message: captured["task"].append((task_uuid, message)))
    monkeypatch.setattr(task_manager, "add_batch_log", lambda batch_id, message: captured["batch"].append((batch_id, message)))

    callback = task_manager.create_log_callback(
        "task-log-forward",
        prefix="[任务1]",
        batch_id="batch-log-forward",
    )

    callback("开始注册...")

    assert captured["task"] == [("task-log-forward", "[任务1] 开始注册...")]
    assert captured["batch"] == [("batch-log-forward", "[任务1] 开始注册...")]


def test_registration_log_callback_persists_child_logs_to_batch_snapshot():
    task_uuid = "task-batch-snapshot"
    batch_id = "batch-snapshot"
    registration_routes.batch_tasks[batch_id] = {"logs": []}

    callback = registration_routes._create_registration_log_callback(
        task_uuid,
        log_prefix="[任务1]",
        batch_id=batch_id,
    )

    try:
        callback("开始注册...")

        assert task_manager.get_logs(task_uuid)[-1] == "[任务1] 开始注册..."
        assert task_manager.get_batch_logs(batch_id)[-1] == "[任务1] 开始注册..."
        assert registration_routes.batch_tasks[batch_id]["logs"][-1] == "[任务1] 开始注册..."
    finally:
        from src.web.task_manager import _batch_logs, _log_queues  # type: ignore

        registration_routes.batch_tasks.pop(batch_id, None)
        _batch_logs.pop(batch_id, None)
        _log_queues.pop(task_uuid, None)


def test_sync_registration_task_passes_token_completion_concurrency_to_engine(monkeypatch):
    manager = _build_manager("registration_task_token_completion_concurrency.db")

    with manager.session_scope() as session:
        session.add(
            RegistrationTask(
                task_uuid="task-token-completion-concurrency",
                status="pending",
            )
        )

    @contextmanager
    def fake_get_db():
        session = manager.SessionLocal()
        try:
            yield session
        finally:
            session.close()

    captured = {}

    def fake_create(service_type, config):
        return SimpleNamespace(service_type=service_type, config=config)

    class FakeEngine:
        def __init__(
            self,
            email_service,
            proxy_url=None,
            callback_logger=None,
            task_uuid=None,
            check_cancelled=None,
            extra_config=None,
        ):
            captured["extra_config"] = dict(extra_config or {})

        def run(self):
            return SimpleNamespace(
                success=False,
                email="",
                error_message="stop after config capture",
                to_dict=lambda: {},
            )

    monkeypatch.setattr(registration_routes, "get_db", fake_get_db)
    monkeypatch.setattr(
        registration_routes,
        "get_settings",
        lambda: SimpleNamespace(
            registration_token_completion_max_concurrency=0,
            tempmail_enabled=True,
            tempmail_base_url="https://example.test",
            tempmail_timeout=30,
            tempmail_max_retries=1,
        ),
    )
    monkeypatch.setattr(registration_routes, "get_proxy_for_registration", lambda db: (None, None))
    monkeypatch.setattr(registration_routes.EmailServiceFactory, "create", fake_create)
    monkeypatch.setattr(registration_routes, "RegistrationEngine", FakeEngine)
    monkeypatch.setattr(registration_routes.task_manager, "update_status", lambda *args, **kwargs: None)

    registration_routes._run_sync_registration_task(
        task_uuid="task-token-completion-concurrency",
        email_service_type="tempmail",
        proxy=None,
        email_service_config=None,
        token_completion_concurrency=4,
    )

    assert captured["extra_config"]["token_completion_concurrency"] == 4
    assert captured["extra_config"]["token_completion_max_concurrency"] == 0


def test_add_batch_log_caps_history_to_recent_window():
    batch_id = "batch-log-cap"
    original_logs = task_manager.get_batch_logs(batch_id)

    try:
        for index in range(0, 1205):
            task_manager.add_batch_log(batch_id, f"log-{index}")

        logs = task_manager.get_batch_logs(batch_id)

        assert len(logs) <= 1000
        assert logs[0] == "log-205"
        assert logs[-1] == "log-1204"
    finally:
        # 直接清理测试批量日志，避免污染其他用例
        from src.web.task_manager import _batch_logs  # type: ignore
        _batch_logs.pop(batch_id, None)
