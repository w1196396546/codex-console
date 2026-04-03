import asyncio
from pathlib import Path

import src.services.team.invite as invite_module
from src.database.models import Account, AppLog, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team
from src.database.team_models import TeamMembership
from src.services.team.client import TeamClient, TeamClientError
from src.services.team.invite import invite_account_ids, invite_manual_emails


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def _make_owner_and_team(session):
    owner = Account(
        email="owner@example.com",
        password="owner-pass",
        email_service="outlook",
        access_token="owner-access-token",
        account_id="acct-owner-upstream",
        subscription_type="team",
        status="active",
    )
    session.add(owner)
    session.commit()
    session.refresh(owner)

    team = upsert_team(
        session,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-upstream",
        team_name="Invite Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
        current_members=0,
        max_members=6,
        seats_available=6,
    )
    return owner, team


def _make_child_account(
    session,
    *,
    email: str,
    access_token: str = "child-access-token",
    refresh_token: str = "child-refresh-token",
    status: str = "token_pending",
):
    account = Account(
        email=email,
        password="child-pass",
        email_service="outlook",
        access_token=access_token,
        refresh_token=refresh_token,
        account_id=f"acct-{email}",
        status=status,
        source="register",
        extra_data={"seed": email},
    )
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def _assert_guard_logs(session):
    messages = [log.message for log in session.query(AppLog).order_by(AppLog.id.asc()).all()]
    assert any("未触发子号自动刷新 RT" in message for message in messages)
    assert any("未触发子号自动注册" in message for message in messages)


def test_invite_account_ids_updates_only_team_membership_and_keeps_child_tokens_untouched(monkeypatch):
    session = _build_session("team_invite_service_account_ids.db")
    try:
        _, team = _make_owner_and_team(session)
        child = _make_child_account(
            session,
            email="child@example.com",
            access_token="child-access-before",
            refresh_token="child-refresh-before",
            status="token_pending",
        )

        def _fail_refresh(*args, **kwargs):
            raise AssertionError("child refresh flow must not be called")

        def _fail_register(*args, **kwargs):
            raise AssertionError("child registration flow must not be called")

        monkeypatch.setattr(invite_module, "refresh_child_account_tokens", _fail_refresh, raising=False)
        monkeypatch.setattr(invite_module, "register_child_account", _fail_register, raising=False)

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            assert method == "POST"
            assert path == "/backend-api/accounts/acct-team-upstream/invites"
            assert access_token == "owner-access-token"
            assert json == {
                "email_addresses": ["child@example.com"],
                "role": "standard-user",
                "resend_emails": True,
            }
            return {"ok": True}

        result = asyncio.run(
            invite_account_ids(
                session,
                team_id=team.id,
                account_ids=[child.id],
                client=TeamClient(transport=fake_transport),
            )
        )

        session.refresh(child)
        membership = (
            session.query(TeamMembership)
            .filter(TeamMembership.team_id == team.id, TeamMembership.member_email == "child@example.com")
            .one()
        )

        assert child.access_token == "child-access-before"
        assert child.refresh_token == "child-refresh-before"
        assert child.status == "token_pending"
        assert membership.local_account_id == child.id
        assert membership.membership_status == "invited"
        assert result["success"] is True
        assert result["child_refresh_triggered"] is False
        assert result["child_registration_triggered"] is False
        assert result["results"][0]["child_refresh_triggered"] is False
        assert result["results"][0]["child_registration_triggered"] is False
        _assert_guard_logs(session)
    finally:
        session.close()


def test_invite_manual_emails_returns_guard_logs_and_does_not_trigger_child_flows(monkeypatch):
    session = _build_session("team_invite_service_manual_emails.db")
    try:
        _, team = _make_owner_and_team(session)

        def _fail_refresh(*args, **kwargs):
            raise AssertionError("manual invite must not trigger child refresh")

        def _fail_register(*args, **kwargs):
            raise AssertionError("manual invite must not trigger child register")

        monkeypatch.setattr(invite_module, "refresh_child_account_tokens", _fail_refresh, raising=False)
        monkeypatch.setattr(invite_module, "register_child_account", _fail_register, raising=False)

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            assert method == "POST"
            assert path == "/backend-api/accounts/acct-team-upstream/invites"
            assert access_token == "owner-access-token"
            assert json == {
                "email_addresses": ["manual@example.com"],
                "role": "standard-user",
                "resend_emails": True,
            }
            return {"ok": True}

        result = asyncio.run(
            invite_manual_emails(
                session,
                team_id=team.id,
                emails=["manual@example.com"],
                client=TeamClient(transport=fake_transport),
            )
        )

        assert result["success"] is True
        assert "未触发子号自动刷新 RT" in "\n".join(result["logs"])
        assert "未触发子号自动注册" in "\n".join(result["logs"])
        assert result["results"][0]["next_status"] == "invited"
        _assert_guard_logs(session)
    finally:
        session.close()


def test_invite_account_ids_treats_already_member_as_success_state():
    session = _build_session("team_invite_service_already_member.db")
    try:
        _, team = _make_owner_and_team(session)
        child = _make_child_account(session, email="already@example.com")

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            raise TeamClientError("already in workspace")

        result = asyncio.run(
            invite_account_ids(
                session,
                team_id=team.id,
                account_ids=[child.id],
                client=TeamClient(transport=fake_transport),
            )
        )

        membership = (
            session.query(TeamMembership)
            .filter(TeamMembership.team_id == team.id, TeamMembership.member_email == "already@example.com")
            .one()
        )

        assert result["success"] is True
        assert result["results"][0]["success"] is True
        assert result["results"][0]["next_status"] == "already_member"
        assert membership.membership_status == "already_member"
    finally:
        session.close()


def test_invite_manual_emails_marks_current_item_failed_and_later_items_skipped_when_team_is_full():
    session = _build_session("team_invite_service_team_full.db")
    try:
        _, team = _make_owner_and_team(session)

        async def fake_transport(*, method, path, access_token="", json=None, **kwargs):
            email = json["email_addresses"][0]
            if email == "one@example.com":
                return {"ok": True}
            if email == "two@example.com":
                raise TeamClientError("maximum number of seats reached")
            raise AssertionError("third invite must be skipped after team is full")

        result = asyncio.run(
            invite_manual_emails(
                session,
                team_id=team.id,
                emails=["one@example.com", "two@example.com", "three@example.com"],
                client=TeamClient(transport=fake_transport),
            )
        )

        results = result["results"]
        assert result["success"] is False
        assert [item["next_status"] for item in results] == ["invited", "failed", "skipped"]
        assert results[1]["success"] is False
        assert "full" in results[1]["message"].lower()
        assert results[2]["message"].lower().startswith("skipped")
    finally:
        session.close()
