import sqlite3
from contextlib import contextmanager
from pathlib import Path

from src.core.upload import sub2api_upload
from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def test_batch_upload_to_sub2api_marks_accounts_uploaded(monkeypatch):
    seed_session = _build_session("sub2api_upload_marks_uploaded.db")
    manager = seed_session.bind

    account = Account(
        email="sub2api@example.com",
        password="pass-1",
        email_service="outlook",
        status="active",
        access_token="access-token",
    )
    seed_session.add(account)
    seed_session.commit()
    seed_session.refresh(account)
    account_id = account.id
    seed_session.close()

    @contextmanager
    def fake_get_db():
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    monkeypatch.setattr(sub2api_upload, "get_db", fake_get_db)
    monkeypatch.setattr(
        sub2api_upload,
        "upload_to_sub2api",
        lambda accounts, api_url, api_key, concurrency=3, priority=50, target_type="sub2api": (True, "成功上传 1 个账号"),
    )

    result = sub2api_upload.batch_upload_to_sub2api(
        [account_id],
        "https://sub2api.example",
        "key",
    )

    assert result["success_count"] == 1

    with fake_get_db() as db:
        saved = db.query(Account).filter(Account.id == account_id).first()
        assert saved.sub2api_uploaded is True
        assert saved.sub2api_uploaded_at is not None


def test_sqlite_migrate_tables_adds_sub2api_upload_columns():
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / "sub2api_upload_migration.db"
    if db_path.exists():
        db_path.unlink()

    conn = sqlite3.connect(db_path)
    conn.execute(
        """
        CREATE TABLE accounts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            email VARCHAR(255) NOT NULL UNIQUE,
            email_service VARCHAR(50) NOT NULL
        )
        """
    )
    conn.commit()
    conn.close()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    manager.migrate_tables()

    conn = sqlite3.connect(db_path)
    columns = {row[1] for row in conn.execute("PRAGMA table_info('accounts')").fetchall()}
    conn.close()

    assert "sub2api_uploaded" in columns
    assert "sub2api_uploaded_at" in columns
