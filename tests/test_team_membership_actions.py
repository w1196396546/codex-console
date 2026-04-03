import asyncio
from pathlib import Path

import pytest

from src.database import crud
from src.database.models import Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_membership
from src.database.team_models import Team, TeamMembership
from src.services.team.client import TeamClient
from src.services.team.membership_actions import apply_membership_action


def _build_session_factory(db_name: str) -> DatabaseSessionManager:
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager


def _build_session(db_name: str):
    return _build_session_factory(db_name).SessionLocal()


def _make_owner_and_team(session):
    owner = crud.create_account(
        session,
        email="owner-membership@example.com",
        password="owner-pass",
        email_service="outlook",
        access_token="owner-membership-token",
        account_id="acct-owner-membership",
        status="active",
        if_exists="raise",
    )
    team = upsert_team(
        session,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-membership",
        team_name="Membership Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
        current_members=1,
        max_members=6,
        seats_available=5,
    )
    return owner, team


def _make_membership(session, *, team_id: int, email: str, status: str, upstream_user_id: str | None = None):
    return upsert_team_membership(
        session,
        team_id=team_id,
        member_email=email,
        upstream_user_id=upstream_user_id,
        member_role="standard-user",
        membership_status=status,
        source="sync",
    )


def test_revoke_action_persists_updates_team_aggregate_and_requires_refresh():
    manager = _build_session_factory("team_membership_action_revoke_persist.db")
    session = manager.SessionLocal()
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="invitee-persist@example.com",
            status="invited",
        )
        _make_membership(
            session,
            team_id=team.id,
            email="still-active@example.com",
            status="joined",
            upstream_user_id="user-still-active",
        )
        team.current_members = 99
        team.seats_available = 0
        team_id = team.id
        membership_id = membership.id
        session.commit()

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            assert method == "DELETE"
            assert path == "/backend-api/accounts/acct-team-membership/invites"
            assert access_token == "owner-membership-token"
            assert json == {"email_address": "invitee-persist@example.com"}
            return {}

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership_id,
                action="revoke",
                client=TeamClient(transport=fake_transport),
            )
        )

        assert result["success"] is True
        assert result["refresh_required"] is True
        session.close()

        fresh_session = manager.SessionLocal()
        try:
            persisted_membership = (
                fresh_session.query(TeamMembership)
                .filter(TeamMembership.id == membership_id)
                .first()
            )
            persisted_team = fresh_session.query(Team).filter(Team.id == team_id).first()

            assert persisted_membership is not None
            assert persisted_membership.membership_status == "revoked"
            assert persisted_team is not None
            assert persisted_team.current_members == 1
            assert persisted_team.seats_available == 5
        finally:
            fresh_session.close()
    finally:
        session.close()


def test_remove_action_persists_updates_team_aggregate_and_requires_refresh():
    manager = _build_session_factory("team_membership_action_remove_persist.db")
    session = manager.SessionLocal()
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="joined-remove@example.com",
            status="joined",
            upstream_user_id="user-remove-persist",
        )
        _make_membership(
            session,
            team_id=team.id,
            email="invite-stays@example.com",
            status="invited",
        )
        team.current_members = 77
        team.seats_available = 0
        team_id = team.id
        membership_id = membership.id
        session.commit()

        async def fake_transport(*, method, path, access_token="", **kwargs):
            assert method == "DELETE"
            assert path == "/backend-api/accounts/acct-team-membership/users/user-remove-persist"
            assert access_token == "owner-membership-token"
            return {}

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership_id,
                action="remove",
                client=TeamClient(transport=fake_transport),
            )
        )

        assert result["success"] is True
        assert result["refresh_required"] is True
        session.close()

        fresh_session = manager.SessionLocal()
        try:
            persisted_membership = (
                fresh_session.query(TeamMembership)
                .filter(TeamMembership.id == membership_id)
                .first()
            )
            persisted_team = fresh_session.query(Team).filter(Team.id == team_id).first()

            assert persisted_membership is not None
            assert persisted_membership.membership_status == "removed"
            assert persisted_team is not None
            assert persisted_team.current_members == 1
            assert persisted_team.seats_available == 5
        finally:
            fresh_session.close()
    finally:
        session.close()


def test_bind_local_account_persists_and_recomputes_team_aggregate():
    manager = _build_session_factory("team_membership_action_bind_persist.db")
    session = manager.SessionLocal()
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="bind-success@example.com",
            status="invited",
        )
        local_account = crud.create_account(
            session,
            email="bind-success@example.com",
            password="bind-pass",
            email_service="outlook",
            status="active",
            if_exists="raise",
        )
        team.current_members = 42
        team.seats_available = 0
        team_id = team.id
        membership_id = membership.id
        local_account_id = local_account.id
        session.commit()

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership_id,
                action="bind-local-account",
                account_id=local_account_id,
            )
        )

        assert result["success"] is True
        assert result["refresh_required"] is True
        session.close()

        fresh_session = manager.SessionLocal()
        try:
            persisted_membership = (
                fresh_session.query(TeamMembership)
                .filter(TeamMembership.id == membership_id)
                .first()
            )
            persisted_team = fresh_session.query(Team).filter(Team.id == team_id).first()

            assert persisted_membership is not None
            assert persisted_membership.local_account_id == local_account_id
            assert persisted_membership.source == "manual_bind"
            assert persisted_team is not None
            assert persisted_team.current_members == 1
            assert persisted_team.seats_available == 5
        finally:
            fresh_session.close()
    finally:
        session.close()


def test_invited_membership_can_be_revoked_and_updates_status():
    session = _build_session("team_membership_action_revoke.db")
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="invitee@example.com",
            status="invited",
        )

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            assert method == "DELETE"
            assert path == "/backend-api/accounts/acct-team-membership/invites"
            assert access_token == "owner-membership-token"
            assert json == {"email_address": "invitee@example.com"}
            return {}

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership.id,
                action="revoke",
                client=TeamClient(transport=fake_transport),
            )
        )

        session.refresh(membership)
        assert result["success"] is True
        assert result["team_id"] == team.id
        assert result["membership_id"] == membership.id
        assert result["next_status"] == "revoked"
        assert result["refresh_required"] is True
        assert membership.membership_status == "revoked"
    finally:
        session.close()


@pytest.mark.parametrize("status", ["joined", "already_member"])
def test_joined_or_existing_membership_can_be_removed_and_updates_status(status):
    session = _build_session(f"team_membership_action_remove_{status}.db")
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email=f"{status}@example.com",
            status=status,
            upstream_user_id=f"user-{status}",
        )

        async def fake_transport(*, method, path, access_token="", **kwargs):
            assert method == "DELETE"
            assert path == f"/backend-api/accounts/acct-team-membership/users/user-{status}"
            assert access_token == "owner-membership-token"
            return {}

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership.id,
                action="remove",
                client=TeamClient(transport=fake_transport),
            )
        )

        session.refresh(membership)
        assert result["success"] is True
        assert result["next_status"] == "removed"
        assert result["refresh_required"] is True
        assert membership.membership_status == "removed"
    finally:
        session.close()


@pytest.mark.parametrize("status", ["joined", "already_member"])
def test_joined_or_existing_membership_cannot_be_revoked(status):
    session = _build_session(f"team_membership_action_invalid_revoke_{status}.db")
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email=f"no-revoke-{status}@example.com",
            status=status,
            upstream_user_id=f"user-no-revoke-{status}",
        )

        async def fake_transport(**kwargs):
            raise AssertionError("invalid revoke must not call upstream")

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership.id,
                action="revoke",
                client=TeamClient(transport=fake_transport),
            )
        )

        session.refresh(membership)
        assert result["success"] is False
        assert result["error_code"] == 400
        assert membership.membership_status == status
    finally:
        session.close()


def test_invited_membership_cannot_be_removed():
    session = _build_session("team_membership_action_invalid_remove_invited.db")
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="invite-only@example.com",
            status="invited",
        )

        async def fake_transport(**kwargs):
            raise AssertionError("invalid remove must not call upstream")

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership.id,
                action="remove",
                client=TeamClient(transport=fake_transport),
            )
        )

        session.refresh(membership)
        assert result["success"] is False
        assert result["error_code"] == 400
        assert membership.membership_status == "invited"
    finally:
        session.close()


def test_bind_local_account_action_reuses_same_email_guard():
    session = _build_session("team_membership_action_bind_cross_email.db")
    try:
        _, team = _make_owner_and_team(session)
        membership = _make_membership(
            session,
            team_id=team.id,
            email="bind-me@example.com",
            status="invited",
        )
        other_account = crud.create_account(
            session,
            email="other@example.com",
            password="other-pass",
            email_service="outlook",
            status="active",
            if_exists="raise",
        )

        result = asyncio.run(
            apply_membership_action(
                session,
                membership_id=membership.id,
                action="bind-local-account",
                account_id=other_account.id,
            )
        )

        session.refresh(membership)
        assert result["success"] is False
        assert result["error_code"] == 400
        assert membership.local_account_id is None
    finally:
        session.close()
