import json
from contextlib import contextmanager
from datetime import datetime
from pathlib import Path
from types import SimpleNamespace

from fastapi.testclient import TestClient

from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.database.team_crud import upsert_team, upsert_team_membership
from src.web.app import create_app
from src.web.routes import accounts as accounts_module


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

    monkeypatch.setattr(accounts_module, "get_db", fake_get_db)
    return TestClient(create_app())


def _seed_account(session, **overrides):
    defaults = {
        "email": "user@example.com",
        "password": "pass-1",
        "email_service": "outlook",
        "status": "active",
        "refresh_token": None,
    }
    defaults.update(overrides)
    account = Account(**defaults)
    session.add(account)
    session.commit()
    session.refresh(account)
    return account


def _seed_rt_accounts(session, *, prefix: str):
    has_first = _seed_account(
        session,
        email=f"{prefix}-has-1@example.com",
        refresh_token="rt-1",
        status="active",
    )
    has_second = _seed_account(
        session,
        email=f"{prefix}-has-2@example.com",
        refresh_token="rt-2",
        status="active",
    )
    missing = _seed_account(
        session,
        email=f"{prefix}-missing@example.com",
        refresh_token="",
        status="active",
    )
    return {
        "has_ids": [has_first.id, has_second.id],
        "missing_id": missing.id,
        "emails": {
            "has": [has_first.email, has_second.email],
            "missing": missing.email,
        },
    }
def _seed_team_relation_accounts(session):
    owner = _seed_account(session, email="owner@example.com")
    member = _seed_account(session, email="member@example.com")
    both = _seed_account(session, email="both@example.com")
    none = _seed_account(session, email="none@example.com")
    host = _seed_account(session, email="host@example.com")

    member_team = upsert_team(
        session,
        owner_account_id=host.id,
        upstream_account_id="acct-team-member",
        team_name="Member Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
    )
    both_team = upsert_team(
        session,
        owner_account_id=both.id,
        upstream_account_id="acct-team-both-owner",
        team_name="Both Owner Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
    )
    upsert_team(
        session,
        owner_account_id=owner.id,
        upstream_account_id="acct-team-owner",
        team_name="Owner Team",
        plan_type="team",
        subscription_plan="chatgpt-team",
        account_role_snapshot="account-owner",
        status="active",
    )
    upsert_team_membership(
        session,
        team_id=member_team.id,
        local_account_id=member.id,
        member_email=member.email,
        membership_status="joined",
        member_role="standard-user",
        source="sync",
    )
    upsert_team_membership(
        session,
        team_id=member_team.id,
        local_account_id=both.id,
        member_email=both.email,
        membership_status="already_member",
        member_role="standard-user",
        source="sync",
    )

    return {
        "owner": owner.id,
        "member": member.id,
        "both": both.id,
        "none": none.id,
        "both_team": both_team.id,
    }


def test_list_accounts_supports_refresh_token_state_and_has_refresh_token(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_list.db")

    with accounts_module.get_db() as db:
        _seed_account(
            db,
            email="has-rt@example.com",
            refresh_token="rt-1",
            status="active",
        )
        _seed_account(
            db,
            email="missing-rt@example.com",
            refresh_token="",
            status="expired",
        )

    has_response = client.get("/api/accounts", params={"refresh_token_state": "has"})
    assert has_response.status_code == 200
    has_payload = has_response.json()
    assert has_payload["total"] == 1
    assert has_payload["accounts"][0]["email"] == "has-rt@example.com"
    assert has_payload["accounts"][0]["has_refresh_token"] is True

    missing_response = client.get("/api/accounts", params={"refresh_token_state": "missing"})
    assert missing_response.status_code == 200
    missing_payload = missing_response.json()
    assert missing_payload["total"] == 1
    assert missing_payload["accounts"][0]["email"] == "missing-rt@example.com"
    assert missing_payload["accounts"][0]["has_refresh_token"] is False


def test_list_accounts_includes_sub2api_upload_state(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_sub2api_state.db")

    uploaded_at = datetime(2026, 4, 3, 12, 0, 0)
    with accounts_module.get_db() as db:
        account = _seed_account(
            db,
            email="sub2api-visible@example.com",
            refresh_token="rt-1",
            status="active",
        )
        account.sub2api_uploaded = True
        account.sub2api_uploaded_at = uploaded_at
        db.commit()

    response = client.get("/api/accounts")

    assert response.status_code == 200
    payload = response.json()
    assert payload["total"] == 1
    assert payload["accounts"][0]["sub2api_uploaded"] is True
    assert payload["accounts"][0]["sub2api_uploaded_at"] == uploaded_at.isoformat()


def test_list_accounts_treats_whitespace_refresh_token_as_missing(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_whitespace_rt.db")

    with accounts_module.get_db() as db:
        _seed_account(
            db,
            email="blank-rt@example.com",
            refresh_token="   ",
            status="active",
        )

    has_response = client.get("/api/accounts", params={"refresh_token_state": "has"})
    assert has_response.status_code == 200
    assert has_response.json()["total"] == 0

    missing_response = client.get("/api/accounts", params={"refresh_token_state": "missing"})
    assert missing_response.status_code == 200
    payload = missing_response.json()
    assert payload["total"] == 1
    assert payload["accounts"][0]["email"] == "blank-rt@example.com"
    assert payload["accounts"][0]["has_refresh_token"] is False


def test_list_accounts_rejects_invalid_refresh_token_state(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_invalid_rt.db")

    response = client.get("/api/accounts", params={"refresh_token_state": "weird"})

    assert response.status_code == 400
    assert response.json()["detail"] == "无效的 refresh_token_state"


def test_list_accounts_supports_combined_filters_with_refresh_token_state(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_combined_filters.db")

    with accounts_module.get_db() as db:
        _seed_account(
            db,
            email="vip-match@example.com",
            account_id="vip-001",
            email_service="outlook",
            refresh_token="rt-1",
            status="active",
        )
        _seed_account(
            db,
            email="vip-wrong-status@example.com",
            account_id="vip-002",
            email_service="outlook",
            refresh_token="rt-2",
            status="expired",
        )
        _seed_account(
            db,
            email="vip-wrong-service@example.com",
            account_id="vip-003",
            email_service="tempmail",
            refresh_token="rt-3",
            status="active",
        )
        _seed_account(
            db,
            email="vip-missing-rt@example.com",
            account_id="vip-004",
            email_service="outlook",
            refresh_token="",
            status="active",
        )

    response = client.get(
        "/api/accounts",
        params={
            "refresh_token_state": "has",
            "status": "active",
            "email_service": "outlook",
            "search": "vip",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["total"] == 1
    assert [item["email"] for item in payload["accounts"]] == ["vip-match@example.com"]


def test_patch_account_updates_single_status(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_patch_status.db")

    with accounts_module.get_db() as db:
        account = _seed_account(db, email="single-status@example.com", status="active")
        account_id = account.id

    response = client.patch(
        f"/api/accounts/{account_id}",
        json={"status": "failed"},
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["id"] == account_id
    assert payload["status"] == "failed"

    with accounts_module.get_db() as db:
        refreshed = db.query(Account).filter(Account.id == account_id).one()
        assert refreshed.status == "failed"


def test_batch_update_accounts_uses_ids_when_select_all_is_false(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_ids.db")

    with accounts_module.get_db() as db:
        first = _seed_account(db, email="first@example.com", status="active")
        second = _seed_account(db, email="second@example.com", status="active")
        first_id = first.id
        second_id = second.id

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [first_id],
            "status": "banned",
            "select_all": False,
            "status_filter": "active",
            "refresh_token_state_filter": "missing",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["requested_count"] == 1
    assert payload["updated_count"] == 1
    assert payload["skipped_count"] == 0
    assert payload["missing_ids"] == []
    assert payload["errors"] is None

    with accounts_module.get_db() as db:
        refreshed_first = db.query(Account).filter(Account.id == first_id).one()
        refreshed_second = db.query(Account).filter(Account.id == second_id).one()
        assert refreshed_first.status == "banned"
        assert refreshed_second.status == "active"


def test_batch_update_accounts_reports_missing_ids_when_select_all_is_false(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_missing_ids.db")

    with accounts_module.get_db() as db:
        account = _seed_account(db, email="exists@example.com", status="active")
        account_id = account.id

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [account_id, 999999],
            "status": "expired",
            "select_all": False,
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["requested_count"] == 2
    assert payload["updated_count"] == 1
    assert payload["skipped_count"] == 1
    assert payload["missing_ids"] == [999999]
    assert payload["message"] == "部分账号不存在，已跳过 1 个"

    with accounts_module.get_db() as db:
        refreshed = db.query(Account).filter(Account.id == account_id).one()
        assert refreshed.status == "expired"


def test_batch_update_accounts_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_select_all.db")

    with accounts_module.get_db() as db:
        _seed_account(db, email="has-1@example.com", refresh_token="rt-1", status="active")
        _seed_account(db, email="has-2@example.com", refresh_token="rt-2", status="active")
        _seed_account(db, email="missing@example.com", refresh_token="", status="active")

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [9999],
            "status": "expired",
            "select_all": True,
            "status_filter": "active",
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["requested_count"] == 2
    assert payload["updated_count"] == 2
    assert payload["skipped_count"] == 0
    assert payload["missing_ids"] == []
    assert payload["errors"] is None

    with accounts_module.get_db() as db:
        statuses = {
            account.email: account.status
            for account in db.query(Account).order_by(Account.email).all()
        }
        assert statuses == {
            "has-1@example.com": "expired",
            "has-2@example.com": "expired",
            "missing@example.com": "active",
        }


def test_batch_update_accounts_returns_success_for_zero_matches(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_zero.db")

    with accounts_module.get_db() as db:
        _seed_account(db, email="zero@example.com", refresh_token="", status="active")

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [],
            "status": "expired",
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["success"] is True
    assert payload["requested_count"] == 0
    assert payload["updated_count"] == 0
    assert payload["skipped_count"] == 0
    assert payload["missing_ids"] == []
    assert payload["message"] == "当前筛选结果下无可更新账号"


def test_batch_update_accounts_reports_partial_failures(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_partial.db")

    with accounts_module.get_db() as db:
        first = _seed_account(db, email="partial-1@example.com", status="active")
        second = _seed_account(db, email="partial-2@example.com", status="active")
        first_id = first.id
        second_id = second.id

    original_update_account = accounts_module.crud.update_account

    def flaky_update_account(db, account_id, **kwargs):
        if account_id == second_id:
            raise RuntimeError("boom")
        return original_update_account(db, account_id, **kwargs)

    monkeypatch.setattr(accounts_module.crud, "update_account", flaky_update_account)

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [first_id, second_id],
            "status": "failed",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["requested_count"] == 2
    assert payload["updated_count"] == 1
    assert payload["skipped_count"] == 0
    assert payload["missing_ids"] == []
    assert payload["errors"] == [f"ID {second_id}: boom"]


def test_batch_update_accounts_rejects_invalid_status(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_invalid.db")

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [1],
            "status": "invalid-status",
            "select_all": False,
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "无效的状态值"


def test_batch_update_accounts_rejects_invalid_refresh_token_filter_even_when_select_all_is_false(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_invalid_rt_filter.db")

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [1],
            "status": "active",
            "select_all": False,
            "refresh_token_state_filter": "oops",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "无效的 refresh_token_state_filter"


def test_batch_update_accounts_rejects_invalid_refresh_token_filter_when_select_all_is_true(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_invalid_rt_filter_select_all.db")

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [],
            "status": "active",
            "select_all": True,
            "refresh_token_state_filter": "oops",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "无效的 refresh_token_state_filter"


def test_batch_update_accounts_supports_select_all_with_combined_filters(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_combined_filters.db")

    with accounts_module.get_db() as db:
        _seed_account(
            db,
            email="group-match@example.com",
            account_id="group-001",
            email_service="outlook",
            refresh_token="rt-1",
            status="active",
        )
        _seed_account(
            db,
            email="group-wrong-status@example.com",
            account_id="group-002",
            email_service="outlook",
            refresh_token="rt-2",
            status="expired",
        )
        _seed_account(
            db,
            email="group-wrong-service@example.com",
            account_id="group-003",
            email_service="tempmail",
            refresh_token="rt-3",
            status="active",
        )
        _seed_account(
            db,
            email="group-missing-rt@example.com",
            account_id="group-004",
            email_service="outlook",
            refresh_token="",
            status="active",
        )

    response = client.post(
        "/api/accounts/batch-update",
        json={
            "ids": [99999],
            "status": "banned",
            "select_all": True,
            "status_filter": "active",
            "email_service_filter": "outlook",
            "search_filter": "group",
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["requested_count"] == 1
    assert payload["updated_count"] == 1
    assert payload["skipped_count"] == 0
    assert payload["missing_ids"] == []
    assert payload["errors"] is None

    with accounts_module.get_db() as db:
        statuses = {
            account.email: account.status
            for account in db.query(Account).order_by(Account.email).all()
        }
        assert statuses["group-match@example.com"] == "banned"
        assert statuses["group-wrong-status@example.com"] == "expired"
        assert statuses["group-wrong-service@example.com"] == "active"
        assert statuses["group-missing-rt@example.com"] == "active"


def test_batch_delete_accounts_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_delete_rt_filter.db")

    with accounts_module.get_db() as db:
        seeded = _seed_rt_accounts(db, prefix="delete")

    response = client.post(
        "/api/accounts/batch-delete",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    assert response.json()["deleted_count"] == 2

    with accounts_module.get_db() as db:
        remaining_emails = [account.email for account in db.query(Account).order_by(Account.email).all()]
        assert remaining_emails == [seeded["emails"]["missing"]]


def test_batch_refresh_accounts_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_refresh_rt_filter.db")

    with accounts_module.get_db() as db:
        seeded = _seed_rt_accounts(db, prefix="refresh")

    refreshed_ids = []

    def fake_do_refresh(account_id, proxy):
        refreshed_ids.append(account_id)
        return SimpleNamespace(success=True, error_message=None, expires_at=None)

    monkeypatch.setattr(accounts_module, "do_refresh", fake_do_refresh)

    response = client.post(
        "/api/accounts/batch-refresh",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    assert response.json()["success_count"] == 2
    assert refreshed_ids == seeded["has_ids"]


def test_batch_validate_accounts_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_validate_rt_filter.db")

    with accounts_module.get_db() as db:
        seeded = _seed_rt_accounts(db, prefix="validate")

    validated_ids = []

    def fake_do_validate(account_id, proxy):
        validated_ids.append(account_id)
        return True, None

    monkeypatch.setattr(accounts_module, "do_validate", fake_do_validate)

    response = client.post(
        "/api/accounts/batch-validate",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["valid_count"] == 2
    assert [item["id"] for item in payload["details"]] == seeded["has_ids"]
    assert validated_ids == seeded["has_ids"]


def test_export_accounts_json_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_export_json_rt_filter.db")

    with accounts_module.get_db() as db:
        seeded = _seed_rt_accounts(db, prefix="export")

    response = client.post(
        "/api/accounts/export/json",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    exported = json.loads(response.content.decode("utf-8"))
    assert [item["email"] for item in exported] == seeded["emails"]["has"]


def test_batch_upload_endpoints_support_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_upload_rt_filter.db")

    with accounts_module.get_db() as db:
        seeded = _seed_rt_accounts(db, prefix="upload")

    captured = {}

    def fake_batch_upload_to_cpa(ids, proxy, api_url=None, api_token=None):
        captured["cpa"] = list(ids)
        return {"success_count": len(ids), "failed_count": 0, "skipped_count": 0}

    def fake_batch_upload_to_sub2api(ids, api_url, api_key, concurrency=3, priority=50, target_type="sub2api"):
        captured["sub2api"] = list(ids)
        return {"success_count": len(ids), "failed_count": 0, "skipped_count": 0}

    def fake_batch_upload_to_team_manager(ids, api_url, api_key):
        captured["tm"] = list(ids)
        return {"success_count": len(ids), "failed_count": 0, "skipped_count": 0}

    monkeypatch.setattr(accounts_module, "batch_upload_to_cpa", fake_batch_upload_to_cpa)
    monkeypatch.setattr(accounts_module, "batch_upload_to_sub2api", fake_batch_upload_to_sub2api)
    monkeypatch.setattr(accounts_module, "batch_upload_to_team_manager", fake_batch_upload_to_team_manager)
    monkeypatch.setattr(
        accounts_module.crud,
        "get_sub2api_services",
        lambda db, enabled=True: [SimpleNamespace(api_url="https://sub2api.example", api_key="key")],
    )
    monkeypatch.setattr(
        accounts_module.crud,
        "get_tm_services",
        lambda db, enabled=True: [SimpleNamespace(api_url="https://tm.example", api_key="key", target_type="sub2api")],
    )

    for endpoint in [
        "/api/accounts/batch-upload-cpa",
        "/api/accounts/batch-upload-sub2api",
        "/api/accounts/batch-upload-tm",
    ]:
        response = client.post(
            endpoint,
            json={
                "ids": [],
                "select_all": True,
                "refresh_token_state_filter": "has",
            },
        )
        assert response.status_code == 200

    assert captured["cpa"] == seeded["has_ids"]
    assert captured["sub2api"] == seeded["has_ids"]
    assert captured["tm"] == seeded["has_ids"]


def test_upload_account_to_sub2api_marks_account_uploaded(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_upload_sub2api.db")

    with accounts_module.get_db() as db:
        account = _seed_account(
            db,
            email="single-sub2api@example.com",
            access_token="access-token",
        )
        account_id = account.id

    monkeypatch.setattr(
        accounts_module,
        "upload_to_sub2api",
        lambda accounts, api_url, api_key, concurrency=3, priority=50, target_type="sub2api": (True, "成功上传 1 个账号"),
    )
    monkeypatch.setattr(
        accounts_module.crud,
        "get_sub2api_services",
        lambda db, enabled=True: [SimpleNamespace(api_url="https://sub2api.example", api_key="key")],
    )

    response = client.post(f"/api/accounts/{account_id}/upload-sub2api", json={})

    assert response.status_code == 200
    assert response.json()["success"] is True

    with accounts_module.get_db() as db:
        saved = db.query(Account).filter(Account.id == account_id).first()
        assert saved.sub2api_uploaded is True
        assert saved.sub2api_uploaded_at is not None


def test_batch_delete_accounts_rejects_invalid_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_batch_delete_invalid_rt_filter.db")

    response = client.post(
        "/api/accounts/batch-delete",
        json={
            "ids": [1],
            "select_all": False,
            "refresh_token_state_filter": "oops",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "无效的 refresh_token_state_filter"


def test_list_accounts_includes_team_relation_fields_for_owner_member_both_none(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_team_relations_list.db")

    with accounts_module.get_db() as db:
        _seed_team_relation_accounts(db)

    response = client.get("/api/accounts", params={"page_size": 10})

    assert response.status_code == 200
    payload = response.json()
    by_email = {item["email"]: item for item in payload["accounts"]}

    assert by_email["owner@example.com"]["team_role_badges"] == ["owner"]
    assert by_email["owner@example.com"]["team_relation_summary"] == {
        "owner_count": 1,
        "member_count": 0,
        "has_owner_role": True,
        "has_member_role": False,
    }
    assert by_email["owner@example.com"]["team_relation_count"] == 1

    assert by_email["member@example.com"]["team_role_badges"] == ["member"]
    assert by_email["member@example.com"]["team_relation_summary"] == {
        "owner_count": 0,
        "member_count": 1,
        "has_owner_role": False,
        "has_member_role": True,
    }
    assert by_email["member@example.com"]["team_relation_count"] == 1

    assert by_email["both@example.com"]["team_role_badges"] == ["owner", "member"]
    assert by_email["both@example.com"]["team_relation_summary"] == {
        "owner_count": 1,
        "member_count": 1,
        "has_owner_role": True,
        "has_member_role": True,
    }
    assert by_email["both@example.com"]["team_relation_count"] == 2

    assert by_email["none@example.com"]["team_role_badges"] == []
    assert by_email["none@example.com"]["team_relation_summary"] is None
    assert by_email["none@example.com"]["team_relation_count"] == 0


def test_get_account_includes_team_relation_fields_for_owner_member_both_none(monkeypatch):
    client = _create_client(monkeypatch, "accounts_routes_team_relations_detail.db")

    with accounts_module.get_db() as db:
        seeded = _seed_team_relation_accounts(db)

    expected = {
        seeded["owner"]: (["owner"], 1),
        seeded["member"]: (["member"], 1),
        seeded["both"]: (["owner", "member"], 2),
        seeded["none"]: ([], 0),
    }

    for account_id, (badges, relation_count) in expected.items():
        response = client.get(f"/api/accounts/{account_id}")
        assert response.status_code == 200
        payload = response.json()
        assert payload["team_role_badges"] == badges
        assert payload["team_relation_count"] == relation_count
        if relation_count == 0:
            assert payload["team_relation_summary"] is None
        else:
            assert payload["team_relation_summary"] is not None
