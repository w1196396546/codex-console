import asyncio
import inspect

import pytest

from src.services.team.client import (
    TeamAuthenticationError,
    TeamClient,
    TeamNotFoundError,
    TeamPermissionError,
    TeamRateLimitError,
    TeamResponseFormatError,
)


class _FakeResponse:
    def __init__(self, *, status_code=200, payload=None, text=""):
        self.status_code = status_code
        self._payload = {} if payload is None else payload
        self.text = text

    def json(self):
        return self._payload


def test_parse_team_accounts_keeps_only_team_accounts_from_accounts_mapping():
    client = TeamClient()

    parsed = client.parse_team_accounts(
        {
            "accounts": {
                "acc_team": {
                    "account": {
                        "plan_type": "team",
                        "name": "Alpha",
                        "account_user_role": "account-owner",
                    },
                    "entitlement": {
                        "subscription_plan": "team",
                        "has_active_subscription": True,
                        "expires_at": "2026-12-31T00:00:00Z",
                    },
                },
                "acc_personal": {
                    "account": {
                        "plan_type": "personal",
                        "name": "Beta",
                    },
                    "entitlement": {
                        "subscription_plan": "personal",
                        "has_active_subscription": True,
                    },
                },
            }
        }
    )

    assert parsed == [
        {
            "upstream_account_id": "acc_team",
            "team_name": "Alpha",
            "plan_type": "team",
            "account_role_snapshot": "account-owner",
            "subscription_plan": "team",
            "expires_at": "2026-12-31T00:00:00Z",
        }
    ]


def test_parse_team_accounts_falls_back_to_placeholder_and_unknown_fields():
    client = TeamClient()

    parsed = client.parse_team_accounts(
        {
            "accounts": {
                "acc_team_1234567890": {
                    "account": {"plan_type": "team"},
                    "entitlement": {},
                }
            }
        }
    )

    assert parsed == [
        {
            "upstream_account_id": "acc_team_1234567890",
            "team_name": "Team-acc_team",
            "plan_type": "team",
            "account_role_snapshot": "unknown",
            "subscription_plan": "unknown",
            "expires_at": "",
        }
    ]


def test_parse_members_accepts_items_with_limit_offset_total_shape():
    client = TeamClient()

    members = client.parse_members(
        {
            "items": [
                {
                    "id": "user_1",
                    "email": "owner@example.com",
                    "name": "Owner",
                    "role": "account-owner",
                    "created_time": "2026-04-03T10:00:00Z",
                },
                {
                    "id": "user_2",
                    "email": "member@example.com",
                    "name": "Member",
                    "role": "standard-user",
                    "created_time": "2026-04-03T11:00:00Z",
                },
            ],
            "limit": 100,
            "offset": 0,
            "total": 2,
        }
    )

    assert members == [
        {
            "upstream_user_id": "user_1",
            "email": "owner@example.com",
            "name": "Owner",
            "role": "account-owner",
            "created_time": "2026-04-03T10:00:00Z",
        },
        {
            "upstream_user_id": "user_2",
            "email": "member@example.com",
            "name": "Member",
            "role": "standard-user",
            "created_time": "2026-04-03T11:00:00Z",
        },
    ]


def test_parse_invites_accepts_items_and_email_address_shape():
    client = TeamClient()

    invites = client.parse_invites(
        {
            "items": [
                {
                    "email_address": " invitee@example.com ",
                    "role": "standard-user",
                    "created_time": "2026-04-03T12:00:00Z",
                }
            ]
        }
    )

    assert invites == [
        {
            "email_address": "invitee@example.com",
            "role": "standard-user",
            "created_time": "2026-04-03T12:00:00Z",
        }
    ]


@pytest.mark.parametrize(
    ("status_code", "payload", "text", "expected_exception", "expected_message"),
    [
        (401, {"detail": "token invalid"}, "", TeamAuthenticationError, "token invalid"),
        (403, {"error": "forbidden"}, "", TeamPermissionError, "forbidden"),
        (429, {"error": {"code": "rate_limit_exceeded"}}, "", TeamRateLimitError, "rate_limit_exceeded"),
        (404, {}, "missing account", TeamNotFoundError, "missing account"),
    ],
)
def test_raise_for_error_extracts_message_from_spec_layers(
    status_code, payload, text, expected_exception, expected_message
):
    client = TeamClient()

    with pytest.raises(expected_exception, match=expected_message):
        client.raise_for_error(status_code=status_code, payload=payload, text=text)


def test_collect_items_from_pages_stops_when_items_are_empty():
    client = TeamClient()

    collected = client.collect_items_from_pages(
        pages=[
            {
                "items": [{"id": "user_1", "email": "one@example.com", "name": "One", "role": "standard-user", "created_time": "t1"}],
                "limit": 2,
                "offset": 0,
                "total": 5,
            },
            {
                "items": [],
                "limit": 2,
                "offset": 2,
                "total": 5,
            },
            {
                "items": [{"id": "user_3", "email": "three@example.com", "name": "Three", "role": "standard-user", "created_time": "t3"}],
                "limit": 2,
                "offset": 4,
                "total": 5,
            },
        ],
        parser=client.parse_members,
    )

    assert [item["upstream_user_id"] for item in collected] == ["user_1"]


def test_collect_items_from_pages_stops_when_collected_count_reaches_total():
    client = TeamClient()

    collected = client.collect_items_from_pages(
        pages=[
            {
                "items": [{"id": "user_1", "email": "one@example.com", "name": "One", "role": "standard-user", "created_time": "t1"}],
                "limit": 1,
                "offset": 0,
                "total": 2,
            },
            {
                "items": [{"id": "user_2", "email": "two@example.com", "name": "Two", "role": "standard-user", "created_time": "t2"}],
                "limit": 1,
                "offset": 1,
                "total": 2,
            },
            {
                "items": [{"id": "user_3", "email": "three@example.com", "name": "Three", "role": "standard-user", "created_time": "t3"}],
                "limit": 1,
                "offset": 2,
                "total": 2,
            },
        ],
        parser=client.parse_members,
    )

    assert [item["upstream_user_id"] for item in collected] == ["user_1", "user_2"]


def test_collect_items_from_pages_stops_when_page_size_is_smaller_than_limit():
    client = TeamClient()

    collected = client.collect_items_from_pages(
        pages=[
            {
                "items": [{"id": "user_1", "email": "one@example.com", "name": "One", "role": "standard-user", "created_time": "t1"}],
                "limit": 2,
                "offset": 0,
                "total": 5,
            },
            {
                "items": [{"id": "user_2", "email": "two@example.com", "name": "Two", "role": "standard-user", "created_time": "t2"}],
                "limit": 2,
                "offset": 2,
                "total": 5,
            },
        ],
        parser=client.parse_members,
    )

    assert [item["upstream_user_id"] for item in collected] == ["user_1"]


def test_parse_members_raises_response_format_error_when_items_is_not_a_list():
    client = TeamClient()

    with pytest.raises(TeamResponseFormatError):
        client.parse_members({"items": {"id": "user_1"}, "limit": 100, "offset": 0, "total": 1})


def test_client_exposes_async_http_method_signatures_even_without_transport():
    client = TeamClient()

    assert inspect.iscoroutinefunction(client.get_team_accounts)
    assert inspect.iscoroutinefunction(client.list_members)
    assert inspect.iscoroutinefunction(client.list_invites)


def test_client_uses_default_curl_cffi_transport_when_transport_is_not_injected(monkeypatch):
    request_log = {}

    def fake_request(method, url, **kwargs):
        request_log["method"] = method
        request_log["url"] = url
        request_log["kwargs"] = kwargs
        return _FakeResponse(payload={"items": [], "limit": 20, "offset": 5, "total": 0})

    monkeypatch.setattr("src.services.team.client.cffi_requests", type("FakeRequests", (), {"request": staticmethod(fake_request)}), raising=False)

    client = TeamClient()
    payload = asyncio.run(client.list_members("token-123", "acct_1", limit=20, offset=5))

    assert payload == {"items": [], "limit": 20, "offset": 5, "total": 0}
    assert request_log["method"] == "GET"
    assert request_log["url"] == "https://chatgpt.com/backend-api/accounts/acct_1/users"
    assert request_log["kwargs"]["params"] == {"limit": 20, "offset": 5}
    assert request_log["kwargs"]["headers"]["Authorization"] == "Bearer token-123"
