from __future__ import annotations

import pytest

from src.database.migrate_to_postgres import migrate_sqlite_to_database
from src.database.models import Account, EmailService, Setting
from src.database.session import DatabaseSessionManager
from src.database.team_models import Team, TeamMembership


def test_migrate_sqlite_to_database_copies_rows_and_related_records(tmp_path):
    source_url = f"sqlite:///{tmp_path / 'source.db'}"
    target_url = f"sqlite:///{tmp_path / 'target.db'}"

    source_manager = DatabaseSessionManager(source_url)
    source_manager.create_tables()

    with source_manager.session_scope() as session:
        email_service = EmailService(
            service_type="tempmail",
            name="Temp Mail",
            config={"base_url": "https://mail.example.com"},
            enabled=True,
            priority=1,
        )
        session.add(email_service)
        session.flush()

        account = Account(
            email="alice@example.com",
            password="secret",
            email_service="tempmail",
            email_service_id=str(email_service.id),
            status="active",
            extra_data={"origin": "sqlite"},
        )
        session.add(account)
        session.flush()

        session.add(Setting(key="database.url", value=source_url, category="database"))

        team = Team(
            owner_account_id=account.id,
            upstream_account_id="upstream-account-1",
            team_name="Alpha Team",
            plan_type="team",
            status="active",
        )
        session.add(team)
        session.flush()

        session.add(
            TeamMembership(
                team_id=team.id,
                local_account_id=account.id,
                member_email="alice@example.com",
                membership_status="active",
            )
        )

    summary = migrate_sqlite_to_database(source_url, target_url)

    assert summary["total_rows"] == 5
    assert summary["tables"]["accounts"] == 1
    assert summary["tables"]["email_services"] == 1
    assert summary["tables"]["settings"] == 1
    assert summary["tables"]["teams"] == 1
    assert summary["tables"]["team_memberships"] == 1

    target_manager = DatabaseSessionManager(target_url)
    with target_manager.session_scope() as session:
        account = session.query(Account).one()
        team = session.query(Team).one()
        membership = session.query(TeamMembership).one()

        assert account.email == "alice@example.com"
        assert account.extra_data == {"origin": "sqlite"}
        assert session.query(EmailService).count() == 1
        assert session.query(Setting).count() == 1
        assert team.owner_account_id == account.id
        assert membership.team_id == team.id


def test_migrate_sqlite_to_database_rejects_non_empty_target_without_replace(tmp_path):
    source_url = f"sqlite:///{tmp_path / 'source.db'}"
    target_url = f"sqlite:///{tmp_path / 'target.db'}"

    source_manager = DatabaseSessionManager(source_url)
    source_manager.create_tables()
    with source_manager.session_scope() as session:
        session.add(
            Account(
                email="alice@example.com",
                password="secret",
                email_service="tempmail",
                status="active",
            )
        )

    target_manager = DatabaseSessionManager(target_url)
    target_manager.create_tables()
    with target_manager.session_scope() as session:
        session.add(
            Account(
                email="existing@example.com",
                password="secret",
                email_service="tempmail",
                status="active",
            )
        )

    with pytest.raises(ValueError, match="目标数据库已存在数据"):
        migrate_sqlite_to_database(source_url, target_url)
