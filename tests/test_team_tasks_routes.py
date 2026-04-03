from contextlib import contextmanager
from pathlib import Path

from fastapi.testclient import TestClient

from src.database.models import Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team_task, upsert_team_task_item
from src.web.app import create_app
from src.web.routes import team_tasks as team_tasks_module
from src.web.task_manager import task_manager


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

    monkeypatch.setattr(team_tasks_module, "get_db", fake_get_db)
    return TestClient(create_app())


def test_get_team_task_includes_guard_logs(monkeypatch):
    client = _create_client(monkeypatch, "team_tasks_routes_guard_logs.db")
    task_uuid = "task-team-guard-logs"

    with team_tasks_module.get_db() as db:
        task = upsert_team_task(
            db,
            task_uuid=task_uuid,
            task_type="invite_emails",
            team_id=101,
            status="running",
            logs="未触发子号自动刷新 RT\n未触发子号自动注册",
        )
        upsert_team_task_item(
            db,
            task_id=task.id,
            target_email="invitee@example.com",
            item_status="success",
            before={"membership_status": "pending"},
            after={"membership_status": "invited"},
            message="ok",
        )

    task_manager.update_status(task_uuid, "running", source="team")
    task_manager.add_log(task_uuid, "未触发子号自动刷新 RT")

    response = client.get(f"/api/team/tasks/{task_uuid}")

    assert response.status_code == 200
    payload = response.json()
    assert payload["task_uuid"] == task_uuid
    assert "未触发子号自动注册" in payload["logs"]
    assert "未触发子号自动刷新 RT" in payload["guard_logs"]
    assert payload["items"][0]["target_email"] == "invitee@example.com"
