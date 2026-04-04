from pathlib import Path

import pytest

from src.database.models import Base
from src.database.team_crud import upsert_team_task
from src.database.session import DatabaseSessionManager
from src.database.team_models import TeamTask
from src.services.team.tasks import (
    build_accepted_payload_from_task,
    complete_team_task,
    enqueue_team_task,
    find_active_team_task,
)
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


def test_enqueue_team_task_persists_record_and_returns_accepted_payload():
    session = _build_session("team_task_service_enqueue.db")
    try:
        payload = enqueue_team_task(
            session,
            task_uuid="team-task-1",
            task_type="discover_owner_teams",
            owner_account_id=101,
            request_payload={"mode": "discovery"},
        )

        persisted = session.query(TeamTask).filter(TeamTask.task_uuid == "team-task-1").one()

        assert persisted.owner_account_id == 101
        assert persisted.team_id is None
        assert persisted.task_type == "discover_owner_teams"
        assert persisted.status == "pending"
        assert persisted.request_payload == {"mode": "discovery"}
        assert payload == {
            "success": True,
            "task_uuid": "team-task-1",
            "task_type": "discover_owner_teams",
            "status": "pending",
            "owner_account_id": 101,
            "scope_type": "owner",
            "scope_id": "101",
            "ws_channel": "/api/ws/task/team-task-1",
        }
        assert task_manager.get_status("team-task-1") == {"status": "pending"}
    finally:
        session.close()
        task_manager.cleanup_task("team-task-1")


def test_enqueue_team_task_includes_team_scope_when_team_id_is_present():
    session = _build_session("team_task_service_team_scope.db")
    try:
        payload = enqueue_team_task(
            session,
            task_uuid="team-task-3",
            task_type="discover_owner_teams",
            team_id=303,
            request_payload={"mode": "sync"},
        )

        assert payload == {
            "success": True,
            "task_uuid": "team-task-3",
            "task_type": "discover_owner_teams",
            "status": "pending",
            "team_id": 303,
            "scope_type": "team",
            "scope_id": "303",
            "ws_channel": "/api/ws/task/team-task-3",
        }
    finally:
        session.close()
        task_manager.cleanup_task("team-task-3")


def test_enqueue_team_task_team_scope_keeps_owner_account_id_in_payload_and_record():
    session = _build_session("team_task_service_team_scope_with_owner.db")
    try:
        payload = enqueue_team_task(
            session,
            task_uuid="team-task-4",
            task_type="discover_owner_teams",
            team_id=404,
            owner_account_id=1404,
            request_payload={"mode": "sync"},
        )

        persisted = session.query(TeamTask).filter(TeamTask.task_uuid == "team-task-4").one()

        assert persisted.team_id == 404
        assert persisted.owner_account_id == 1404
        assert persisted.scope_type == "team"
        assert persisted.scope_id == "404"
        assert persisted.active_scope_key == "team:404"
        assert payload == {
            "success": True,
            "task_uuid": "team-task-4",
            "task_type": "discover_owner_teams",
            "status": "pending",
            "team_id": 404,
            "owner_account_id": 1404,
            "scope_type": "team",
            "scope_id": "404",
            "ws_channel": "/api/ws/task/team-task-4",
        }
    finally:
        session.close()
        task_manager.cleanup_task("team-task-4")


def test_complete_team_task_updates_persisted_result_and_runtime_status():
    session = _build_session("team_task_service_complete.db")
    try:
        enqueue_team_task(
            session,
            task_uuid="team-task-2",
            task_type="discover_owner_teams",
            owner_account_id=202,
            request_payload={"mode": "discovery"},
        )

        completed = complete_team_task(
            session,
            task_uuid="team-task-2",
            status="completed",
            result_payload={"teams_found": 2},
        )

        assert completed.status == "completed"
        assert completed.result_payload == {"teams_found": 2}
        assert task_manager.get_status("team-task-2") == {
            "status": "completed",
            "result_payload": {"teams_found": 2},
        }
    finally:
        session.close()
        task_manager.cleanup_task("team-task-2")


def test_complete_team_task_requires_existing_task():
    session = _build_session("team_task_service_missing_task.db")
    try:
        with pytest.raises(LookupError):
            complete_team_task(
                session,
                task_uuid="missing-task",
                status="completed",
                result_payload={"teams_found": 0},
            )
    finally:
        session.close()
        task_manager.cleanup_task("missing-task")


def test_complete_team_task_rejects_missing_task_uuid():
    session = _build_session("team_task_service_missing_uuid.db")
    try:
        with pytest.raises(ValueError, match="task_uuid"):
            complete_team_task(
                session,
                task_uuid="",
                status="completed",
                result_payload={"teams_found": 0},
            )
    finally:
        session.close()


@pytest.mark.parametrize("existing_status", ["pending", "running"])
def test_enqueue_team_task_rejects_second_write_task_for_same_team(existing_status):
    session = _build_session(f"team_task_service_conflict_{existing_status}.db")
    try:
        enqueue_team_task(
            session,
            task_uuid=f"team-task-existing-{existing_status}",
            task_type="discover_owner_teams",
            team_id=909,
            request_payload={"mode": "sync"},
        )
        if existing_status == "running":
            complete_team_task(
                session,
                task_uuid="team-task-existing-running",
                status="running",
            )

        with pytest.raises(RuntimeError, match="409"):
            enqueue_team_task(
                session,
                task_uuid="team-task-conflict",
                task_type="discover_owner_teams",
                team_id=909,
                request_payload={"mode": "sync-again"},
            )

        persisted = session.query(TeamTask).order_by(TeamTask.id.asc()).all()
        assert [task.task_uuid for task in persisted] == [f"team-task-existing-{existing_status}"]
    finally:
        session.close()
        task_manager.cleanup_task(f"team-task-existing-{existing_status}")
        task_manager.cleanup_task("team-task-conflict")


def test_enqueue_team_task_rejects_second_write_task_for_same_owner_scope():
    session = _build_session("team_task_service_owner_conflict.db")
    try:
        enqueue_team_task(
            session,
            task_uuid="team-task-owner-1",
            task_type="discover_owner_teams",
            owner_account_id=808,
            request_payload={"mode": "discovery"},
        )

        with pytest.raises(RuntimeError, match="409"):
            enqueue_team_task(
                session,
                task_uuid="team-task-owner-2",
                task_type="discover_owner_teams",
                owner_account_id=808,
                request_payload={"mode": "discovery-again"},
            )
    finally:
        session.close()
        task_manager.cleanup_task("team-task-owner-1")
        task_manager.cleanup_task("team-task-owner-2")


def test_find_active_team_task_returns_existing_owner_task_payload():
    session = _build_session("team_task_service_owner_existing.db")
    try:
        enqueue_team_task(
            session,
            task_uuid="team-task-owner-existing",
            task_type="discover_owner_teams",
            owner_account_id=818,
            request_payload={"mode": "discovery"},
        )

        existing = find_active_team_task(
            session,
            owner_account_id=818,
            task_type="discover_owner_teams",
        )

        assert existing is not None
        assert existing.task_uuid == "team-task-owner-existing"
        assert build_accepted_payload_from_task(existing) == {
            "success": True,
            "task_uuid": "team-task-owner-existing",
            "task_type": "discover_owner_teams",
            "status": "pending",
            "owner_account_id": 818,
            "scope_type": "owner",
            "scope_id": "818",
            "ws_channel": "/api/ws/task/team-task-owner-existing",
        }
    finally:
        session.close()
        task_manager.cleanup_task("team-task-owner-existing")


def test_build_accepted_response_payload_infers_scope_from_owner_account():
    payload = task_manager.build_accepted_response_payload(
        "team-task-owner-direct",
        task_type="discover_owner_teams",
        owner_account_id=818,
    )

    assert payload == {
        "success": True,
        "task_uuid": "team-task-owner-direct",
        "task_type": "discover_owner_teams",
        "status": "pending",
        "owner_account_id": 818,
        "scope_type": "owner",
        "scope_id": "818",
        "ws_channel": "/api/ws/task/team-task-owner-direct",
    }


def test_upsert_team_task_infers_owner_scope_fields_for_discovery_tasks():
    session = _build_session("team_task_service_crud_owner_scope.db")
    try:
        created = upsert_team_task(
            session,
            task_uuid="team-task-owner-crud",
            task_type="discover_owner_teams",
            owner_account_id=919,
            status="pending",
            request_payload={"mode": "discovery"},
        )

        assert created.scope_type == "owner"
        assert created.scope_id == "919"
        assert created.active_scope_key == "owner:919"
    finally:
        session.close()


def test_upsert_team_task_prefers_team_scope_key_when_team_and_owner_are_present():
    session = _build_session("team_task_service_crud_team_scope_with_owner.db")
    try:
        created = upsert_team_task(
            session,
            task_uuid="team-task-team-crud",
            task_type="discover_owner_teams",
            team_id=929,
            owner_account_id=1929,
            status="pending",
            request_payload={"mode": "sync"},
        )

        assert created.team_id == 929
        assert created.owner_account_id == 1929
        assert created.scope_type == "team"
        assert created.scope_id == "929"
        assert created.active_scope_key == "team:929"
    finally:
        session.close()
