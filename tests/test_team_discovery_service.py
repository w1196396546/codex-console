import asyncio
from pathlib import Path

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.database.team_models import Team
from src.services.team.client import TeamClient
from src.services.team.discovery import discover_teams_from_local_accounts


def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)

    db_path = runtime_dir / db_name
    if db_path.exists():
        db_path.unlink()

    manager = DatabaseSessionManager(f"sqlite:///{db_path}")
    Base.metadata.create_all(bind=manager.engine)
    return manager.SessionLocal()


def test_discover_teams_from_local_accounts_persists_matching_owner_account():
    session = _build_session("team_discovery_service_accounts.db")
    try:
        owner = Account(
            email="owner@example.com",
            password="pass-1",
            email_service="outlook",
            account_id="acct-team-owner",
            access_token="access-owner",
            subscription_type="team",
            status="active",
        )
        member_without_token = Account(
            email="member@example.com",
            password="pass-2",
            email_service="outlook",
            account_id="acct-member",
            access_token="",
            subscription_type="team",
            status="active",
        )
        session.add_all([owner, member_without_token])
        session.commit()
        session.refresh(owner)

        async def fake_transport(*, method, path, access_token="", **kwargs):
            assert method == "GET"
            assert path == "/backend-api/accounts/check/v4-2023-04-27"
            assert access_token == "access-owner"
            return {
                "accounts": {
                    "acct-team-owner": {
                        "account": {
                            "plan_type": "team",
                            "name": "Alpha Team",
                            "account_user_role": "account-owner",
                        },
                        "entitlement": {
                            "subscription_plan": "chatgpt-team",
                            "expires_at": "2026-12-31T00:00:00Z",
                        },
                    },
                    "acct-personal": {
                        "account": {
                            "plan_type": "personal",
                            "name": "Personal",
                        },
                        "entitlement": {
                            "subscription_plan": "free",
                        },
                    },
                }
            }

        summary = asyncio.run(
            discover_teams_from_local_accounts(
                session,
                client=TeamClient(transport=fake_transport),
            )
        )

        teams = session.query(Team).order_by(Team.id.asc()).all()
        assert len(teams) == 1
        assert teams[0].owner_account_id == owner.id
        assert teams[0].upstream_account_id == "acct-team-owner"
        assert teams[0].team_name == "Alpha Team"
        assert teams[0].subscription_plan == "chatgpt-team"
        assert teams[0].account_role_snapshot == "account-owner"
        assert teams[0].sync_status == "synced"
        assert summary == {
            "accounts_scanned": 1,
            "teams_found": 1,
            "teams_persisted": 1,
        }
    finally:
        session.close()


def test_discover_teams_from_local_accounts_persists_multiple_owner_team_accounts():
    session = _build_session("team_discovery_service_multi_owner_accounts.db")
    try:
        owner = Account(
            email="owner-multi@example.com",
            password="pass-1",
            email_service="outlook",
            account_id="acct-root-owner",
            access_token="access-owner",
            subscription_type="team",
            status="active",
        )
        session.add(owner)
        session.commit()
        session.refresh(owner)

        async def fake_transport(*, method, path, access_token="", **kwargs):
            assert method == "GET"
            assert access_token == "access-owner"
            return {
                "accounts": {
                    "acct-team-a": {
                        "account": {
                            "plan_type": "team",
                            "name": "Alpha Team",
                            "account_user_role": "account-owner",
                        },
                        "entitlement": {
                            "subscription_plan": "chatgpt-team",
                            "expires_at": "2026-12-31T00:00:00Z",
                        },
                    },
                    "acct-team-b": {
                        "account": {
                            "plan_type": "team",
                            "name": "Beta Team",
                            "account_user_role": "account-owner",
                        },
                        "entitlement": {
                            "subscription_plan": "chatgpt-team",
                            "expires_at": "2027-01-31T00:00:00Z",
                        },
                    },
                }
            }

        summary = asyncio.run(
            discover_teams_from_local_accounts(
                session,
                client=TeamClient(transport=fake_transport),
            )
        )

        teams = session.query(Team).order_by(Team.upstream_account_id.asc()).all()
        assert [team.upstream_account_id for team in teams] == ["acct-team-a", "acct-team-b"]
        assert summary == {
            "accounts_scanned": 1,
            "teams_found": 2,
            "teams_persisted": 2,
        }
    finally:
        session.close()


def test_discover_teams_from_local_accounts_skips_team_accounts_without_owner_role():
    session = _build_session("team_discovery_service_non_owner.db")
    try:
        account = Account(
            email="admin@example.com",
            password="pass-3",
            email_service="outlook",
            account_id="acct-team-admin",
            access_token="access-admin",
            subscription_type="team",
            status="active",
        )
        session.add(account)
        session.commit()

        async def fake_transport(*, method, path, access_token="", **kwargs):
            assert method == "GET"
            assert path == "/backend-api/accounts/check/v4-2023-04-27"
            assert access_token == "access-admin"
            return {
                "accounts": {
                    "acct-team-admin": {
                        "account": {
                            "plan_type": "team",
                            "name": "Beta Team",
                            "account_user_role": "admin",
                        },
                        "entitlement": {
                            "subscription_plan": "chatgpt-team",
                            "expires_at": "2026-12-31T00:00:00Z",
                        },
                    }
                }
            }

        summary = asyncio.run(
            discover_teams_from_local_accounts(
                session,
                client=TeamClient(transport=fake_transport),
            )
        )

        teams = session.query(Team).order_by(Team.id.asc()).all()
        assert teams == []
        assert summary == {
            "accounts_scanned": 1,
            "teams_found": 1,
            "teams_persisted": 0,
        }
    finally:
        session.close()


def test_discover_teams_from_local_accounts_skips_accounts_without_access_token():
    session = _build_session("team_discovery_service_missing_access_token.db")
    try:
        account = Account(
            email="missing-token@example.com",
            password="pass-4",
            email_service="outlook",
            account_id="acct-missing-token",
            access_token="",
            subscription_type="team",
            status="active",
        )
        session.add(account)
        session.commit()

        async def fake_transport(**kwargs):
            raise AssertionError("transport should not be called for accounts without access token")

        summary = asyncio.run(
            discover_teams_from_local_accounts(
                session,
                client=TeamClient(transport=fake_transport),
            )
        )

        teams = session.query(Team).order_by(Team.id.asc()).all()
        assert teams == []
        assert summary == {
            "accounts_scanned": 0,
            "teams_found": 0,
            "teams_persisted": 0,
        }
    finally:
        session.close()


def test_discover_teams_from_local_accounts_skips_failed_accounts():
    session = _build_session("team_discovery_service_failed_status.db")
    try:
        account = Account(
            email="failed@example.com",
            password="pass-5",
            email_service="outlook",
            account_id="acct-failed",
            access_token="access-failed",
            subscription_type="team",
            status="failed",
        )
        session.add(account)
        session.commit()

        async def fake_transport(**kwargs):
            raise AssertionError("transport should not be called for failed accounts")

        summary = asyncio.run(
            discover_teams_from_local_accounts(
                session,
                client=TeamClient(transport=fake_transport),
            )
        )

        assert session.query(Team).count() == 0
        assert summary == {
            "accounts_scanned": 0,
            "teams_found": 0,
            "teams_persisted": 0,
        }
    finally:
        session.close()
