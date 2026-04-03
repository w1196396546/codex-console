from contextlib import contextmanager
from pathlib import Path

from fastapi.testclient import TestClient

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_membership
from src.web.app import create_app
from src.web.routes import team as team_module


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

    monkeypatch.setattr(team_module, "get_db", fake_get_db)
    return TestClient(create_app())


def _seed_owner_and_team(db):
    owner = Account(
        email="owner@example.com",
        password="pass-1",
        email_service="outlook",
        access_token="owner-token",
        account_id="acct-owner",
        status="active",
    )
    external = Account(
        email="local@example.com",
        password="pass-2",
        email_service="outlook",
        access_token="local-token",
        account_id="acct-local",
        status="active",
    )
    db.add_all([owner, external])
    db.commit()
    db.refresh(owner)
    db.refresh(external)

    team = upsert_team(
        db,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-alpha",
        team_name="Alpha Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
        current_members=1,
        max_members=6,
        seats_available=5,
        sync_status="synced",
    )
    internal = upsert_team_membership(
        db,
        team_id=team.id,
        local_account_id=external.id,
        member_email="local@example.com",
        membership_status="joined",
        member_role="standard-user",
        source="sync",
    )
    external_membership = upsert_team_membership(
        db,
        team_id=team.id,
        member_email="external@example.com",
        membership_status="invited",
        member_role="standard-user",
        source="sync",
    )
    return owner, team, internal, external_membership


def test_run_team_discovery_returns_accepted_payload_with_ws_channel(monkeypatch):
    client = _create_client(monkeypatch, "team_routes_discovery.db")

    monkeypatch.setattr(
        team_module,
        "refresh_account_token",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError("should not refresh token")),
    )
    monkeypatch.setattr(
        team_module,
        "RegistrationEngine",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError("should not register")),
    )

    response = client.post("/api/team/discovery/run", json={"ids": [1]})

    assert response.status_code == 202
    payload = response.json()
    assert payload["status"] == "pending"
    assert payload["task_uuid"]
    assert payload["ws_channel"] == f"/api/ws/task/{payload['task_uuid']}"


def test_list_team_memberships_includes_id(monkeypatch):
    client = _create_client(monkeypatch, "team_routes_memberships.db")

    with team_module.get_db() as db:
        _, team, _, _ = _seed_owner_and_team(db)
        team_id = team.id

    response = client.get(f"/api/team/teams/{team_id}/memberships")

    assert response.status_code == 200
    payload = response.json()
    assert payload["items"]
    assert "id" in payload["items"][0]


def test_list_team_memberships_supports_external_binding(monkeypatch):
    client = _create_client(monkeypatch, "team_routes_external_binding.db")

    with team_module.get_db() as db:
        _, team, _, external_membership = _seed_owner_and_team(db)
        team_id = team.id

    response = client.get(
        f"/api/team/teams/{team_id}/memberships",
        params={"binding": "external"},
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["items"]
    assert [item["id"] for item in payload["items"]] == [external_membership.id]
    assert all(item["local_account_id"] is None for item in payload["items"])


def test_team_invite_routes_never_trigger_register_or_refresh(monkeypatch):
    client = _create_client(monkeypatch, "team_routes_invite_guard.db")

    monkeypatch.setattr(
        team_module,
        "refresh_account_token",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError("should not refresh token")),
    )
    monkeypatch.setattr(
        team_module,
        "RegistrationEngine",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError("should not register")),
    )

    response = client.post(
        "/api/team/teams/101/invite-emails",
        json={"emails": ["invitee@example.com"]},
    )

    assert response.status_code == 202
    payload = response.json()
    assert payload["status"] == "pending"
    assert payload["accepted_count"] == 1
