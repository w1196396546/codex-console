import asyncio
from pathlib import Path

import pytest

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_membership
from src.database.team_models import Team, TeamMembership
from src.services.team.client import TeamClient
from src.services.team.sync import TeamSyncNotFoundError, sync_team_memberships


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def _make_account(
    session,
    *,
    email: str,
    access_token: str = "",
    account_id: str | None = None,
) -> Account:
    account = Account(
        email=email,
        password="known-pass",
        email_service="outlook",
        access_token=access_token,
        account_id=account_id,
        status="active",
        source="register",
        extra_data={},
    )
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def _make_team(session, *, owner_account_id: int, upstream_account_id: str, max_members: int = 6) -> Team:
    return upsert_team(
        session,
        owner_account_id=owner_account_id,
        upstream_account_id=upstream_account_id,
        team_name="Sync Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="owner",
        status="active",
        max_members=max_members,
        current_members=0,
        seats_available=max_members,
        sync_status="pending",
    )


class FakeTeamSyncClient:
    def __init__(self, *, members_payload=None, invites_payload=None, error: Exception | None = None):
        self.members_payload = members_payload or {"items": []}
        self.invites_payload = invites_payload or {"items": []}
        self.error = error
        self._parser = TeamClient(transport=lambda **_: {"items": []})

    async def list_members(self, access_token: str, upstream_account_id: str, *, limit: int = 100, offset: int = 0):
        if self.error is not None:
            raise self.error
        return self.members_payload

    async def list_invites(self, access_token: str, upstream_account_id: str):
        if self.error is not None:
            raise self.error
        return self.invites_payload

    def parse_members(self, payload):
        return self._parser.parse_members(payload)

    def parse_invites(self, payload):
        return self._parser.parse_invites(payload)

    def collect_items_from_pages(self, *, pages, parser):
        return self._parser.collect_items_from_pages(pages=pages, parser=parser)


def test_sync_team_memberships_merges_member_and_invite_for_same_email_as_joined():
    session = _build_session("team_sync_merge_joined.db")
    try:
        owner = _make_account(
            session,
            email="owner@example.com",
            access_token="owner-token",
            account_id="acct-owner",
        )
        team = _make_team(session, owner_account_id=owner.id, upstream_account_id="acct-team-sync", max_members=5)
        client = FakeTeamSyncClient(
            members_payload={
                "items": [
                    {
                        "id": "user-1",
                        "email": " Foo@Example.com ",
                        "role": "member",
                        "created_time": "2026-04-03T00:00:00Z",
                    }
                ]
            },
            invites_payload={
                "items": [
                    {
                        "email_address": "foo@example.com",
                        "role": "member",
                        "created_time": "2026-04-02T00:00:00Z",
                    }
                ]
            },
        )

        summary = asyncio.run(sync_team_memberships(session, team_id=team.id, client=client))

        membership = session.query(TeamMembership).filter(TeamMembership.team_id == team.id).one()
        session.refresh(team)

        assert summary["active_member_count"] == 1
        assert summary["joined_count"] == 1
        assert summary["invited_count"] == 0
        assert membership.member_email == "foo@example.com"
        assert membership.membership_status == "joined"
        assert team.current_members == 1
        assert team.seats_available == 4
        assert team.sync_status == "synced"
        assert team.sync_error is None
    finally:
        session.close()


def test_sync_team_memberships_backfills_local_account_id_by_normalized_email():
    session = _build_session("team_sync_backfill_account.db")
    try:
        owner = _make_account(
            session,
            email="owner@example.com",
            access_token="owner-token",
            account_id="acct-owner",
        )
        linked_account = _make_account(session, email=" Linked@Example.com ")
        team = _make_team(session, owner_account_id=owner.id, upstream_account_id="acct-team-sync-backfill")
        client = FakeTeamSyncClient(
            members_payload={
                "items": [
                    {
                        "id": "user-2",
                        "email": "Display Name <linked@example.com>",
                        "role": "admin",
                        "created_time": "2026-04-03T00:00:00Z",
                    }
                ]
            }
        )

        asyncio.run(sync_team_memberships(session, team_id=team.id, client=client))

        membership = session.query(TeamMembership).filter(TeamMembership.team_id == team.id).one()

        assert membership.member_email == "linked@example.com"
        assert membership.local_account_id == linked_account.id
    finally:
        session.close()


def test_sync_team_memberships_raises_clear_error_when_team_missing():
    session = _build_session("team_sync_missing_team.db")
    try:
        with pytest.raises(TeamSyncNotFoundError, match="team 999 not found"):
            asyncio.run(sync_team_memberships(session, team_id=999, client=FakeTeamSyncClient()))
    finally:
        session.close()


def test_sync_team_memberships_marks_team_failed_when_upstream_fetch_errors():
    session = _build_session("team_sync_upstream_error.db")
    try:
        owner = _make_account(
            session,
            email="owner@example.com",
            access_token="owner-token",
            account_id="acct-owner",
        )
        team = _make_team(session, owner_account_id=owner.id, upstream_account_id="acct-team-sync-error")

        with pytest.raises(RuntimeError, match="upstream exploded"):
            asyncio.run(
                sync_team_memberships(
                    session,
                    team_id=team.id,
                    client=FakeTeamSyncClient(error=RuntimeError("upstream exploded")),
                )
            )

        session.refresh(team)
        assert team.sync_status == "failed"
        assert "upstream exploded" in (team.sync_error or "")
        assert team.last_sync_at is not None
    finally:
        session.close()


def test_sync_team_memberships_counts_joined_and_already_member_as_active_members():
    session = _build_session("team_sync_active_counts.db")
    try:
        owner = _make_account(
            session,
            email="owner@example.com",
            access_token="owner-token",
            account_id="acct-owner",
        )
        team = _make_team(session, owner_account_id=owner.id, upstream_account_id="acct-team-sync-active", max_members=3)
        upsert_team_membership(
            session,
            team_id=team.id,
            member_email="existing@example.com",
            member_role="member",
            membership_status="already_member",
            source="sync",
        )
        client = FakeTeamSyncClient(
            members_payload={
                "items": [
                    {
                        "id": "user-3",
                        "email": "joined@example.com",
                        "role": "member",
                        "created_time": "2026-04-03T00:00:00Z",
                    }
                ]
            }
        )

        summary = asyncio.run(sync_team_memberships(session, team_id=team.id, client=client))
        memberships = (
            session.query(TeamMembership)
            .filter(TeamMembership.team_id == team.id)
            .order_by(TeamMembership.member_email.asc())
            .all()
        )
        session.refresh(team)

        assert summary["active_member_count"] == 2
        assert summary["joined_count"] == 1
        assert summary["invited_count"] == 0
        assert [(item.member_email, item.membership_status) for item in memberships] == [
            ("existing@example.com", "already_member"),
            ("joined@example.com", "joined"),
        ]
        assert team.current_members == 2
        assert team.seats_available == 1
    finally:
        session.close()
