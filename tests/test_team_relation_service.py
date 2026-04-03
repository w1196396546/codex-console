from pathlib import Path

from src.database import crud
from src.database.models import Account, AppLog, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_membership
from src.database.team_models import TeamMembership
from src.services.team.relation import bind_local_account, relink_account_memberships


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def _make_membership(session, *, member_email: str, local_account_id=None, source: str = "sync"):
    team = upsert_team(
        session,
        owner_account_id=501,
        upstream_account_id="acct-upstream-team-relation",
        team_name="Relation Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="owner",
        status="active",
    )
    return upsert_team_membership(
        session,
        team_id=team.id,
        local_account_id=local_account_id,
        member_email=member_email,
        member_role="member",
        membership_status="invited",
        source=source,
    )


def _make_account(session, *, email: str) -> Account:
    account = Account(
        email=email,
        password="known-pass",
        email_service="outlook",
        status="active",
        extra_data={},
        source="register",
    )
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def test_relink_account_memberships_backfills_local_account_id():
    session = _build_session("team_relation_backfill.db")
    try:
        membership = _make_membership(
            session,
            member_email=" Foo@Example.com ",
            local_account_id=None,
        )
        account = _make_account(session, email="foo@example.com")

        updated = relink_account_memberships(session, account.id, account.email)
        session.refresh(membership)

        assert updated == 1
        assert membership.local_account_id == account.id
    finally:
        session.close()


def test_manual_bind_is_not_overwritten_by_auto_relink():
    session = _build_session("team_relation_manual_priority.db")
    try:
        membership = _make_membership(
            session,
            member_email="foo@example.com",
            local_account_id=99,
            source="manual_bind",
        )
        account = _make_account(session, email="foo@example.com")

        updated = relink_account_memberships(session, account.id, account.email)
        session.refresh(membership)

        assert updated == 0
        assert membership.local_account_id == 99
        assert membership.source == "manual_bind"
    finally:
        session.close()


def test_bind_local_account_allows_same_email_binding_and_audits():
    session = _build_session("team_relation_bind_same_email.db")
    try:
        membership = _make_membership(
            session,
            member_email="Display Name <foo@example.com>",
            local_account_id=None,
        )
        account = _make_account(session, email=" Foo@Example.com ")

        result = bind_local_account(session, membership.id, account.id)
        session.refresh(membership)
        audit_log = session.query(AppLog).order_by(AppLog.id.desc()).first()

        assert result["success"] is True
        assert result["membership_id"] == membership.id
        assert membership.local_account_id == account.id
        assert membership.source == "manual_bind"
        assert audit_log is not None
        assert "manual bind" in audit_log.message.lower()
    finally:
        session.close()


def test_bind_local_account_changes_and_audit_are_rolled_back_with_caller_transaction():
    db_name = "team_relation_bind_rollback.db"
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)
    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    session = manager.SessionLocal()
    try:
        membership = _make_membership(
            session,
            member_email="foo@example.com",
            local_account_id=None,
        )
        account = _make_account(session, email="foo@example.com")
        membership_id = membership.id

        result = bind_local_account(session, membership_id, account.id)
        audit_log = session.query(AppLog).order_by(AppLog.id.desc()).first()

        assert result["success"] is True
        assert membership.local_account_id == account.id
        assert membership.source == "manual_bind"
        assert audit_log is not None
        assert "manual bind" in audit_log.message.lower()

        session.rollback()
        session.close()

        verification_manager = DatabaseSessionManager(f"sqlite:///{db_path}")
        verification_session = verification_manager.SessionLocal()
        try:
            persisted_membership = (
                verification_session.query(TeamMembership)
                .filter(TeamMembership.id == membership_id)
                .first()
            )
            persisted_audit = verification_session.query(AppLog).order_by(AppLog.id.desc()).first()

            assert persisted_membership is not None
            assert persisted_membership.local_account_id is None
            assert persisted_membership.source == "sync"
            assert persisted_audit is None
        finally:
            verification_session.close()
    finally:
        if session.is_active:
            session.close()


def test_bind_local_account_rejects_cross_email_without_confirmation():
    session = _build_session("team_relation_bind_cross_email.db")
    try:
        membership = _make_membership(
            session,
            member_email="foo@example.com",
            local_account_id=None,
        )
        account = crud.create_account(
            session,
            email="bar@example.com",
            password="known-pass",
            email_service="outlook",
            status="active",
        )

        result = bind_local_account(session, membership.id, account.id)
        session.refresh(membership)

        assert result["success"] is False
        assert result["error_code"] == 400
        assert membership.local_account_id is None
        assert membership.source == "sync"
    finally:
        session.close()


def test_bind_local_account_rejects_overwriting_existing_binding_to_other_account():
    session = _build_session("team_relation_bind_conflict.db")
    try:
        current_account = crud.create_account(
            session,
            email="foo@example.com",
            password="known-pass",
            email_service="outlook",
            status="active",
        )
        other_account = crud.create_account(
            session,
            email="foo@example.com ",
            password="new-pass",
            email_service="outlook",
            status="active",
            if_exists="return",
        )
        membership = _make_membership(
            session,
            member_email="foo@example.com",
            local_account_id=current_account.id + 100,
            source="sync",
        )

        result = bind_local_account(session, membership.id, other_account.id)
        session.refresh(membership)

        assert result["success"] is False
        assert result["error_code"] == 409
        assert membership.local_account_id == current_account.id + 100
        assert membership.source == "sync"
    finally:
        session.close()
