"""Team 邀请服务。"""

from __future__ import annotations

from datetime import datetime

from src.database.models import Account, AppLog
from src.database.team_crud import upsert_team_membership
from src.database.team_models import Team

from .client import TeamClient, TeamClientError
from .utils import normalize_team_email

_GUARD_LOG_MESSAGES = (
    "未触发子号自动刷新 RT",
    "未触发子号自动注册",
)
_FULL_TEAM_TOKENS = (
    "maximum number of seats reached",
    "team is full",
    "workspace is full",
    "no seats",
    "seat limit",
)
_ALREADY_MEMBER_TOKENS = (
    "already in workspace",
    "already a member",
    "already member",
)


def _utcnow() -> datetime:
    return datetime.utcnow()


def _add_app_log(db, message: str) -> None:
    db.add(
        AppLog(
            level="INFO",
            logger="team.invite",
            module="team.invite",
            pathname=__file__,
            lineno=0,
            message=message,
        )
    )


def _append_guard_logs(db) -> list[str]:
    logs = list(_GUARD_LOG_MESSAGES)
    for message in logs:
        _add_app_log(db, message)
    db.flush()
    return logs


def _load_team_and_owner(db, team_id: int) -> tuple[Team, Account]:
    team = db.query(Team).filter(Team.id == team_id).first()
    if team is None:
        raise ValueError(f"team {team_id} not found")

    owner = db.query(Account).filter(Account.id == team.owner_account_id).first()
    if owner is None:
        raise ValueError(f"owner account {team.owner_account_id} not found")

    return team, owner


def _normalize_emails(emails: list[str]) -> list[str]:
    normalized: list[str] = []
    seen: set[str] = set()
    for email in emails:
        resolved = normalize_team_email(email)
        if resolved and resolved not in seen:
            normalized.append(resolved)
            seen.add(resolved)
    return normalized


def _build_result_item(
    *,
    email: str,
    success: bool,
    message: str,
    next_status: str,
    membership_id: int | None = None,
    local_account_id: int | None = None,
) -> dict[str, object]:
    result: dict[str, object] = {
        "email": email,
        "success": success,
        "message": message,
        "next_status": next_status,
        "child_refresh_triggered": False,
        "child_registration_triggered": False,
    }
    if membership_id is not None:
        result["membership_id"] = membership_id
    if local_account_id is not None:
        result["local_account_id"] = local_account_id
    return result


async def _send_invite(
    client: TeamClient,
    *,
    upstream_account_id: str,
    owner_access_token: str,
    email: str,
) -> dict:
    return await client._call_transport(
        "POST",
        f"/backend-api/accounts/{upstream_account_id}/invites",
        access_token=owner_access_token,
        json={
            "email_addresses": [email],
            "role": "standard-user",
            "resend_emails": True,
        },
    )


def _persist_membership(
    db,
    *,
    team_id: int,
    email: str,
    membership_status: str,
    local_account_id: int | None = None,
) -> int:
    membership = upsert_team_membership(
        db,
        team_id=team_id,
        local_account_id=local_account_id,
        member_email=email,
        member_role="member",
        membership_status=membership_status,
        invited_at=_utcnow() if membership_status in {"invited", "already_member"} else None,
        source="invite",
        sync_error=None,
    )
    return membership.id


def _is_full_error(message: str) -> bool:
    normalized = message.lower()
    return any(token in normalized for token in _FULL_TEAM_TOKENS)


def _is_already_member_error(message: str) -> bool:
    normalized = message.lower()
    return any(token in normalized for token in _ALREADY_MEMBER_TOKENS)


async def _invite_emails(
    db,
    *,
    team_id: int,
    emails: list[str],
    email_to_account_id: dict[str, int] | None = None,
    client: TeamClient | None = None,
) -> dict:
    team, owner = _load_team_and_owner(db, team_id)
    team_client = client or TeamClient()
    owner_access_token = str(owner.access_token or "").strip()
    normalized_emails = _normalize_emails(emails)
    logs = _append_guard_logs(db)
    results: list[dict[str, object]] = []
    skip_remaining = False

    for email in normalized_emails:
        local_account_id = (email_to_account_id or {}).get(email)

        if skip_remaining:
            results.append(
                _build_result_item(
                    email=email,
                    success=False,
                    message="skipped because team is full",
                    next_status="skipped",
                    local_account_id=local_account_id,
                )
            )
            continue

        try:
            await _send_invite(
                team_client,
                upstream_account_id=team.upstream_account_id,
                owner_access_token=owner_access_token,
                email=email,
            )
            membership_id = _persist_membership(
                db,
                team_id=team.id,
                email=email,
                membership_status="invited",
                local_account_id=local_account_id,
            )
            results.append(
                _build_result_item(
                    email=email,
                    success=True,
                    message="invite sent",
                    next_status="invited",
                    membership_id=membership_id,
                    local_account_id=local_account_id,
                )
            )
        except TeamClientError as exc:
            error_message = str(exc)
            if _is_already_member_error(error_message):
                membership_id = _persist_membership(
                    db,
                    team_id=team.id,
                    email=email,
                    membership_status="already_member",
                    local_account_id=local_account_id,
                )
                results.append(
                    _build_result_item(
                        email=email,
                        success=True,
                        message=error_message,
                        next_status="already_member",
                        membership_id=membership_id,
                        local_account_id=local_account_id,
                    )
                )
                continue

            if _is_full_error(error_message):
                skip_remaining = True
                membership_id = _persist_membership(
                    db,
                    team_id=team.id,
                    email=email,
                    membership_status="failed",
                    local_account_id=local_account_id,
                )
                results.append(
                    _build_result_item(
                        email=email,
                        success=False,
                        message=f"team full: {error_message}",
                        next_status="failed",
                        membership_id=membership_id,
                        local_account_id=local_account_id,
                    )
                )
                continue

            membership_id = _persist_membership(
                db,
                team_id=team.id,
                email=email,
                membership_status="failed",
                local_account_id=local_account_id,
            )
            results.append(
                _build_result_item(
                    email=email,
                    success=False,
                    message=error_message,
                    next_status="failed",
                    membership_id=membership_id,
                    local_account_id=local_account_id,
                )
            )

    db.commit()
    return {
        "success": all(item["success"] for item in results),
        "team_id": team.id,
        "results": results,
        "logs": logs,
        "child_refresh_triggered": False,
        "child_registration_triggered": False,
    }


async def invite_account_ids(
    db,
    *,
    team_id: int,
    account_ids: list[int],
    client: TeamClient | None = None,
) -> dict:
    accounts = db.query(Account).filter(Account.id.in_(account_ids)).order_by(Account.id.asc()).all()
    emails: list[str] = []
    email_to_account_id: dict[str, int] = {}

    for account in accounts:
        email = normalize_team_email(account.email)
        if not email or email in email_to_account_id:
            continue
        emails.append(email)
        email_to_account_id[email] = account.id

    return await _invite_emails(
        db,
        team_id=team_id,
        emails=emails,
        email_to_account_id=email_to_account_id,
        client=client,
    )


async def invite_manual_emails(
    db,
    *,
    team_id: int,
    emails: list[str],
    client: TeamClient | None = None,
) -> dict:
    return await _invite_emails(
        db,
        team_id=team_id,
        emails=emails,
        email_to_account_id=None,
        client=client,
    )
