import asyncio
from contextlib import contextmanager
from pathlib import Path

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_task
from src.database.team_models import TeamTask
from src.services.team.runner import run_team_task
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


def test_run_team_task_executes_sync_all_teams_request_ids(monkeypatch):
    seed_session = _build_session("team_task_runner_sync_batch.db")
    manager = seed_session.bind

    owner = Account(
        email="owner@example.com",
        password="pass-1",
        email_service="outlook",
        access_token="owner-token",
        account_id="acct-owner",
        status="active",
    )
    seed_session.add(owner)
    seed_session.commit()
    seed_session.refresh(owner)

    team_a = upsert_team(
        seed_session,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-a",
        team_name="Alpha Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
        current_members=0,
        max_members=5,
        seats_available=5,
        sync_status="pending",
    )
    team_b = upsert_team(
        seed_session,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-b",
        team_name="Beta Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
        current_members=0,
        max_members=5,
        seats_available=5,
        sync_status="pending",
    )
    upsert_team_task(
        seed_session,
        task_uuid="team-task-sync-batch",
        task_type="sync_all_teams",
        team_id=team_a.id,
        status="pending",
        request_payload={"ids": [team_a.id, team_b.id]},
    )
    team_a_id = team_a.id
    team_b_id = team_b.id
    seed_session.close()

    @contextmanager
    def fake_get_db():
        db = DatabaseSessionManager(f"{manager.url}").SessionLocal()
        try:
            yield db
        finally:
            db.close()

    seen_team_ids = []

    async def fake_sync(db, *, team_id, client=None, member_page_limit=100):
        seen_team_ids.append(team_id)
        return {
            "membership_count": 0,
            "active_member_count": 0,
            "joined_count": 0,
            "invited_count": 0,
            "already_member_count": 0,
        }

    from src.services.team import runner as runner_module

    monkeypatch.setattr(runner_module, "get_db", fake_get_db)

    asyncio.run(
        run_team_task(
            "team-task-sync-batch",
            sync_func=fake_sync,
        )
    )

    with fake_get_db() as db:
        task = db.query(TeamTask).filter(TeamTask.task_uuid == "team-task-sync-batch").one()
        assert task.status == "completed"
        assert task.result_payload == {
            "requested_count": 2,
            "processed_count": 2,
            "failed_count": 0,
            "results": [
                {"team_id": team_a_id, "status": "completed"},
                {"team_id": team_b_id, "status": "completed"},
            ],
        }

    assert seen_team_ids == [team_a_id, team_b_id]
    task_manager.cleanup_task("team-task-sync-batch")
