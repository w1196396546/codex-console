import logging
import sqlite3
from contextlib import contextmanager
from pathlib import Path

from sqlalchemy.exc import OperationalError

from src.core import db_logs as db_logs_module
from src.database.session import DatabaseSessionManager


def _build_sqlite_manager(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    return DatabaseSessionManager(f"sqlite:///{db_path}")


def test_sqlite_session_manager_enables_wal_and_busy_timeout():
    manager = _build_sqlite_manager("sqlite_pragmas.db")

    with manager.engine.connect() as conn:
        journal_mode = conn.exec_driver_sql("PRAGMA journal_mode").scalar()
        busy_timeout = conn.exec_driver_sql("PRAGMA busy_timeout").scalar()
        synchronous = conn.exec_driver_sql("PRAGMA synchronous").scalar()

    assert str(journal_mode).lower() == "wal"
    assert int(busy_timeout) >= 15000
    assert int(synchronous) in {1, 2}


def test_database_log_handler_drops_locked_write_without_handle_error(monkeypatch):
    handler = db_logs_module.DatabaseLogHandler()
    errors = []

    class FakeDB:
        def add(self, _obj):
            return None

        def commit(self):
            raise OperationalError(
                statement="INSERT INTO app_logs ...",
                params={},
                orig=sqlite3.OperationalError("database is locked"),
            )

        def rollback(self):
            return None

    @contextmanager
    def fake_get_db():
        yield FakeDB()

    monkeypatch.setattr(db_logs_module, "get_db", fake_get_db)
    monkeypatch.setattr(handler, "handleError", lambda record: errors.append(record))

    record = logging.LogRecord(
        name="test.locked",
        level=logging.INFO,
        pathname=__file__,
        lineno=42,
        msg="locked message",
        args=(),
        exc_info=None,
    )

    handler.emit(record)

    assert errors == []
