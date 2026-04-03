from datetime import datetime, timedelta
from pathlib import Path

from src.database.models import Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import (
    list_team_task_items,
    list_team_tasks,
    upsert_team,
    upsert_team_membership,
    upsert_team_task,
    upsert_team_task_item,
)
from src.database.team_models import Team, TeamMembership, TeamTask, TeamTaskItem


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def test_upsert_team_uses_owner_account_and_upstream_account_as_stable_key():
    session = _build_session("team_crud_team_key.db")
    try:
        created_expires_at = datetime(2026, 12, 31, 0, 0, 0)
        updated_expires_at = created_expires_at + timedelta(days=31)
        created = upsert_team(
            session,
            owner_account_id=101,
            upstream_account_id="acct-upstream-1",
            upstream_team_id="team-upstream-1",
            team_name="Alpha Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="active",
            current_members=3,
            max_members=10,
            seats_available=7,
            expires_at=created_expires_at,
            sync_status="synced",
        )
        assert created.expires_at == created_expires_at

        merged = upsert_team(
            session,
            owner_account_id=101,
            upstream_account_id="acct-upstream-1",
            upstream_team_id="team-upstream-2",
            team_name="Alpha Team Renamed",
            plan_type="team",
            subscription_plan="chatgpt-team-v2",
            account_role_snapshot="admin",
            status="paused",
            current_members=4,
            max_members=12,
            seats_available=8,
            expires_at=updated_expires_at,
            sync_status="running",
            sync_error="temporary upstream lag",
        )

        assert merged.id == created.id
        assert merged.team_name == "Alpha Team Renamed"
        assert merged.upstream_team_id == "team-upstream-2"
        assert merged.subscription_plan == "chatgpt-team-v2"
        assert merged.account_role_snapshot == "admin"
        assert merged.status == "paused"
        assert merged.current_members == 4
        assert merged.max_members == 12
        assert merged.seats_available == 8
        assert merged.expires_at == updated_expires_at
        assert merged.sync_status == "running"
        assert merged.sync_error == "temporary upstream lag"
        assert session.query(Team).count() == 1
    finally:
        session.close()


def test_upsert_team_membership_deduplicates_by_normalized_email():
    session = _build_session("team_crud_membership_email.db")
    try:
        team = upsert_team(
            session,
            owner_account_id=202,
            upstream_account_id="acct-upstream-2",
            team_name="Beta Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="active",
        )

        created = upsert_team_membership(
            session,
            team_id=team.id,
            local_account_id=9001,
            member_email="  Member@Example.COM ",
            upstream_user_id="user-1",
            member_role="member",
            membership_status="invited",
            source="sync",
        )

        merged = upsert_team_membership(
            session,
            team_id=team.id,
            local_account_id=9002,
            member_email="Display Name <member@example.com>",
            upstream_user_id="user-2",
            member_role="admin",
            membership_status="already_member",
            source="manual",
            sync_error="none",
        )

        assert merged.id == created.id
        assert merged.member_email == "member@example.com"
        assert merged.local_account_id == 9002
        assert merged.upstream_user_id == "user-2"
        assert merged.member_role == "admin"
        assert merged.membership_status == "already_member"
        assert merged.source == "manual"
        assert merged.sync_error == "none"
        assert session.query(TeamMembership).count() == 1
    finally:
        session.close()


def test_upsert_team_membership_deduplicates_wrapped_email():
    session = _build_session("team_crud_membership_wrapped_email.db")
    try:
        team = upsert_team(
            session,
            owner_account_id=203,
            upstream_account_id="acct-upstream-2b",
            team_name="Wrapped Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="pending",
        )

        created = upsert_team_membership(
            session,
            team_id=team.id,
            member_email="Name <Foo@Example.com>",
            member_role="member",
            membership_status="invited",
            source="sync",
        )

        merged = upsert_team_membership(
            session,
            team_id=team.id,
            member_email="foo@example.com",
            member_role="admin",
            membership_status="joined",
            source="manual",
        )

        assert merged.id == created.id
        assert merged.member_email == "foo@example.com"
        assert merged.member_role == "admin"
        assert merged.membership_status == "joined"
        assert session.query(TeamMembership).count() == 1
    finally:
        session.close()


def test_upsert_team_task_supports_owner_scoped_discovery_without_team():
    session = _build_session("team_crud_discovery_task.db")
    try:
        created = upsert_team_task(
            session,
            team_id=None,
            owner_account_id=404,
            task_uuid="task-discovery-1",
            task_type="discover_team",
            status="pending",
            request_payload={"mode": "discovery"},
            logs="queued",
        )

        merged = upsert_team_task(
            session,
            team_id=None,
            owner_account_id=404,
            task_uuid="task-discovery-1",
            task_type="discover_team",
            status="completed",
            request_payload={"mode": "discovery", "retry": 1},
            result_payload={"teams_found": 2},
            logs="done",
        )

        assert merged.id == created.id
        assert merged.team_id is None
        assert merged.owner_account_id == 404
        assert merged.status == "completed"
        assert merged.request_payload == {"mode": "discovery", "retry": 1}
        assert merged.result_payload == {"teams_found": 2}
        assert session.query(TeamTask).count() == 1
    finally:
        session.close()


def test_upsert_team_task_and_items_use_spec_field_names():
    session = _build_session("team_crud_task_fields.db")
    try:
        team = upsert_team(
            session,
            owner_account_id=303,
            upstream_account_id="acct-upstream-3",
            team_name="Gamma Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="pending",
        )

        task = upsert_team_task(
            session,
            team_id=team.id,
            task_uuid="task-uuid-1",
            task_type="sync_members",
            status="pending",
            request_payload={"page": 1},
            result_payload={"imported": 0},
            logs="boot",
        )
        item = upsert_team_task_item(
            session,
            task_id=task.id,
            target_email="alice@example.com",
            item_status="pending",
            before={"member_role": "member"},
            after={"member_role": "member"},
            message="queued",
        )

        updated_task = upsert_team_task(
            session,
            team_id=team.id,
            task_uuid="task-uuid-1",
            task_type="sync_members",
            status="completed",
            request_payload={"page": 2},
            result_payload={"imported": 1},
            error_message="",
            logs="done",
        )
        updated_item = upsert_team_task_item(
            session,
            task_id=task.id,
            target_email="alice@example.com",
            item_status="completed",
            before={"member_role": "member"},
            after={"member_role": "admin"},
            message="upgraded",
            error_message="",
        )

        assert updated_task.id == task.id
        assert updated_task.request_payload == {"page": 2}
        assert updated_task.result_payload == {"imported": 1}
        assert updated_task.logs == "done"

        assert updated_item.id == item.id
        assert updated_item.item_status == "completed"
        assert updated_item.after_ == {"member_role": "admin"}
        assert updated_item.message == "upgraded"

        tasks = list_team_tasks(session, team_id=team.id)
        items = list_team_task_items(session, task_id=task.id)

        assert session.query(TeamTask).count() == 1
        assert session.query(TeamTaskItem).count() == 1
        assert [row.task_uuid for row in tasks] == ["task-uuid-1"]
        assert [row.target_email for row in items] == ["alice@example.com"]
    finally:
        session.close()


def test_upsert_team_patch_semantics_preserve_omitted_values_and_clear_explicit_none():
    session = _build_session("team_crud_team_patch_semantics.db")
    try:
        created = upsert_team(
            session,
            owner_account_id=501,
            upstream_account_id="acct-upstream-5",
            upstream_team_id="team-upstream-5",
            team_name="Delta Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="active",
            sync_status="failed",
            sync_error="stale snapshot",
        )

        omitted = upsert_team(
            session,
            owner_account_id=501,
            upstream_account_id="acct-upstream-5",
            upstream_team_id="team-upstream-5b",
            team_name="Delta Team Renamed",
            plan_type="team",
            subscription_plan="chatgpt-team-v2",
            account_role_snapshot="admin",
            status="paused",
            sync_status="synced",
        )

        assert omitted.id == created.id
        assert omitted.sync_error == "stale snapshot"

        cleared = upsert_team(
            session,
            owner_account_id=501,
            upstream_account_id="acct-upstream-5",
            upstream_team_id="team-upstream-5c",
            team_name="Delta Team Final",
            plan_type="team",
            subscription_plan="chatgpt-team-v3",
            account_role_snapshot="admin",
            status="active",
            sync_status="synced",
            sync_error=None,
        )

        assert cleared.id == created.id
        assert cleared.sync_error is None
    finally:
        session.close()


def test_upsert_team_membership_patch_semantics_preserve_omitted_values_and_clear_explicit_none():
    session = _build_session("team_crud_membership_patch_semantics.db")
    try:
        team = upsert_team(
            session,
            owner_account_id=502,
            upstream_account_id="acct-upstream-6",
            team_name="Epsilon Team",
            plan_type="team",
            subscription_plan="chatgpt-team",
            account_role_snapshot="owner",
            status="active",
        )

        created = upsert_team_membership(
            session,
            team_id=team.id,
            local_account_id=7001,
            member_email="member@example.com",
            upstream_user_id="user-7001",
            member_role="member",
            membership_status="joined",
            source="sync",
            sync_error="upstream mismatch",
        )

        omitted = upsert_team_membership(
            session,
            team_id=team.id,
            member_email="member@example.com",
            upstream_user_id="user-7002",
            member_role="admin",
            membership_status="already_member",
            source="manual",
        )

        assert omitted.id == created.id
        assert omitted.local_account_id == 7001
        assert omitted.sync_error == "upstream mismatch"

        cleared = upsert_team_membership(
            session,
            team_id=team.id,
            local_account_id=None,
            member_email="member@example.com",
            upstream_user_id="user-7003",
            member_role="owner",
            membership_status="already_member",
            source="manual",
            sync_error=None,
        )

        assert cleared.id == created.id
        assert cleared.local_account_id is None
        assert cleared.sync_error is None
    finally:
        session.close()


def test_upsert_team_task_patch_semantics_preserve_omitted_values_and_clear_explicit_none():
    session = _build_session("team_crud_task_patch_semantics.db")
    try:
        completed_at = datetime(2026, 4, 1, 12, 30, 0)
        created = upsert_team_task(
            session,
            team_id=None,
            owner_account_id=503,
            task_uuid="task-patch-semantics-1",
            task_type="sync_members",
            status="failed",
            request_payload={"page": 1},
            result_payload={"imported": 0},
            error_message="request timeout",
            logs="retrying",
            completed_at=completed_at,
        )

        omitted = upsert_team_task(
            session,
            team_id=None,
            owner_account_id=503,
            task_uuid="task-patch-semantics-1",
            task_type="sync_members",
            status="running",
            request_payload={"page": 2},
            result_payload={"imported": 1},
            logs="progress",
        )

        assert omitted.id == created.id
        assert omitted.error_message == "request timeout"
        assert omitted.completed_at == completed_at

        cleared = upsert_team_task(
            session,
            team_id=None,
            owner_account_id=503,
            task_uuid="task-patch-semantics-1",
            task_type="sync_members",
            status="completed",
            request_payload={"page": 3},
            result_payload={"imported": 2},
            error_message=None,
            logs="done",
            completed_at=None,
        )

        assert cleared.id == created.id
        assert cleared.error_message is None
        assert cleared.completed_at is None
    finally:
        session.close()
