import logging
from pathlib import Path

from src.core.db_logs import _should_skip_record
from src.database.session import DatabaseSessionManager


def _build_manager(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    return DatabaseSessionManager(f"sqlite:///{db_path}")


def test_sqlite_engine_enables_wal_and_busy_timeout():
    manager = _build_manager("sqlite_lock_mitigation_config.db")

    with manager.engine.connect() as conn:
        journal_mode = conn.exec_driver_sql("PRAGMA journal_mode").scalar()
        busy_timeout = conn.exec_driver_sql("PRAGMA busy_timeout").scalar()
        synchronous = conn.exec_driver_sql("PRAGMA synchronous").scalar()

    assert str(journal_mode).lower() == "wal"
    assert int(busy_timeout) >= 5000
    assert int(synchronous) in {1, 2}


def test_high_volume_registration_info_logs_are_skipped_from_db_handler():
    record = logging.LogRecord(
        name="src.web.routes.registration",
        level=logging.INFO,
        pathname=__file__,
        lineno=10,
        msg="high volume info log",
        args=(),
        exc_info=None,
    )

    assert _should_skip_record(record) is True

    task_manager_record = logging.LogRecord(
        name="src.web.task_manager",
        level=logging.INFO,
        pathname=__file__,
        lineno=20,
        msg="task manager info log",
        args=(),
        exc_info=None,
    )

    assert _should_skip_record(task_manager_record) is True
