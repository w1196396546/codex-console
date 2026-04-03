from contextlib import contextmanager
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from src.core.openai import payment as payment_core
from src.database.models import Account, Base
from src.database.session import DatabaseSessionManager
from src.web.app import create_app
from src.web.routes import payment as payment_module


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

    monkeypatch.setattr(payment_module, "get_db", fake_get_db)
    return TestClient(create_app())


def _seed_account(session, **overrides):
    defaults = {
        "email": "payment@example.com",
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


def test_batch_check_subscription_supports_select_all_with_refresh_token_filter(monkeypatch):
    client = _create_client(monkeypatch, "payment_routes_batch_check_subscription_rt_filter.db")

    with payment_module.get_db() as db:
        first = _seed_account(
            db,
            email="sub-has-1@example.com",
            refresh_token="rt-1",
        )
        second = _seed_account(
            db,
            email="sub-has-2@example.com",
            refresh_token="rt-2",
        )
        first_id = first.id
        second_id = second.id
        _seed_account(
            db,
            email="sub-missing@example.com",
            refresh_token="",
        )

    checked_ids = []

    def fake_check_subscription_detail_with_retry(db, account, proxy, allow_token_refresh):
        checked_ids.append(account.id)
        return {"status": "free", "confidence": "low"}, False

    monkeypatch.setattr(
        payment_module,
        "_check_subscription_detail_with_retry",
        fake_check_subscription_detail_with_retry,
    )

    response = client.post(
        "/api/payment/accounts/batch-check-subscription",
        json={
            "ids": [],
            "select_all": True,
            "refresh_token_state_filter": "has",
        },
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["success_count"] == 2
    assert checked_ids == [first_id, second_id]


def test_generate_checkout_link_for_account_supports_business_trial(monkeypatch):
    account = Account(
        id=1,
        email="biztrial@example.com",
        password="pass-1",
        email_service="outlook",
        status="active",
    )
    request = payment_module.GenerateLinkRequest(
        account_id=account.id,
        plan_type="business_trial",
        country="US",
    )

    captured = {}

    def fake_generate_business_trial_checkout_bundle(account, proxy, country, workspace_name):
        captured.update(
            {
                "account_id": account.id,
                "workspace_name": workspace_name,
                "proxy": proxy,
                "country": country,
            }
        )
        return {
            "checkout_url": "https://chatgpt.com/checkout/openai_llc/cs_live_test_business_trial",
            "checkout_session_id": "cs_live_test_business_trial",
            "publishable_key": "pk_live_test",
            "client_secret": "cs_test_secret",
        }

    monkeypatch.setattr(
        payment_module,
        "generate_business_trial_checkout_bundle",
        fake_generate_business_trial_checkout_bundle,
    )

    link, source, fallback_reason, checkout_session_id, publishable_key, client_secret = (
        payment_module._generate_checkout_link_for_account(
            account=account,
            request=request,
            proxy=None,
        )
    )

    assert captured == {
        "account_id": account.id,
        "workspace_name": "MyTeam",
        "proxy": None,
        "country": "US",
    }
    assert link == "https://chatgpt.com/checkout/openai_llc/cs_live_test_business_trial"
    assert source == "openai_checkout_business_trial"
    assert fallback_reason is None
    assert checkout_session_id == "cs_live_test_business_trial"
    assert publishable_key == "pk_live_test"
    assert client_secret == "cs_test_secret"


def test_generate_business_trial_checkout_bundle_uses_fixed_team_trial_defaults(monkeypatch):
    account = Account(
        id=2,
        email="bundle@example.com",
        password="pass-1",
        email_service="outlook",
        status="active",
    )
    captured = {}

    def fake_request_checkout_bundle(account, payload, proxy):
        captured.update(
            {
                "account_id": account.id,
                "payload": payload,
                "proxy": proxy,
            }
        )
        return {"checkout_url": "https://example.com/checkout"}

    monkeypatch.setattr(payment_core, "_request_checkout_bundle", fake_request_checkout_bundle)

    result = payment_core.generate_business_trial_checkout_bundle(
        account=account,
        proxy="http://127.0.0.1:8080",
        country="SG",
        workspace_name="BizTrial",
    )

    assert captured == {
        "account_id": account.id,
        "payload": {
            "plan_name": "chatgptteamplan",
            "team_plan_data": {
                "workspace_name": "BizTrial",
                "price_interval": "month",
                "seat_quantity": 5,
            },
            "billing_details": {"country": "SG", "currency": "SGD"},
            "cancel_url": "https://chatgpt.com/?promo_campaign=team-1-month-free&utm_campaign=WEB-team-1-month-free&utm_internal_medium=referral#team-pricing",
            "promo_campaign": {
                "promo_campaign_id": "team-1-month-free",
                "is_coupon_from_query_param": True,
            },
            "entry_point": "team_workspace_purchase_modal",
            "checkout_ui_mode": "custom",
        },
        "proxy": "http://127.0.0.1:8080",
    }
    assert result == {"checkout_url": "https://example.com/checkout"}


def test_generate_checkout_link_for_account_business_trial_does_not_fallback_to_aimizy(monkeypatch):
    account = Account(
        id=3,
        email="biztrial-no-fallback@example.com",
        password="pass-1",
        email_service="outlook",
        status="active",
    )
    request = payment_module.GenerateLinkRequest(
        account_id=account.id,
        plan_type="business_trial",
        country="US",
    )

    def fake_generate_business_trial_checkout_bundle(account, proxy, country, workspace_name):
        raise RuntimeError("mock official failure")

    def fake_generate_aimizy_payment_link(*args, **kwargs):
        raise AssertionError("business_trial 不应回退到 aimizy")

    monkeypatch.setattr(
        payment_module,
        "generate_business_trial_checkout_bundle",
        fake_generate_business_trial_checkout_bundle,
    )
    monkeypatch.setattr(payment_module, "generate_aimizy_payment_link", fake_generate_aimizy_payment_link)

    with pytest.raises(payment_module.HTTPException) as exc_info:
        payment_module._generate_checkout_link_for_account(
            account=account,
            request=request,
            proxy=None,
        )

    assert exc_info.value.status_code == 502
    assert "Business 试用 checkout 生成失败" in str(exc_info.value.detail)
