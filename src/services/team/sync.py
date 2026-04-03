"""Team 成员/邀请同步服务。"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

from sqlalchemy.orm import Session

from src.database.models import Account
from src.database.team_models import Team, TeamMembership

from .client import TeamClient
from .utils import (
    count_current_members,
    normalize_team_email,
    resolve_team_member_status,
    seats_available as calculate_seats_available,
)

# 预留给后续 invite 流程复用的“ghost success”窗口配置。
GHOST_SUCCESS_WINDOW_SECONDS = 12
GHOST_SUCCESS_POLL_INTERVAL_SECONDS = 0.5
GHOST_SUCCESS_URL_TOKENS = ("subscribed=true", "success", "/settings/subscription")
GHOST_SUCCESS_TEXT_MARKERS = ("you're all set", "you’re all set", "subscription active")
_ABSENT_MEMBERSHIP_STATUS = {
    "invited": "revoked",
    "joined": "removed",
    "already_member": "removed",
}

class TeamSyncError(Exception):
    """Team 同步基础异常。"""


class TeamSyncNotFoundError(TeamSyncError):
    """指定 Team 不存在。"""


def _parse_upstream_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    normalized = str(value).strip().replace("Z", "+00:00")
    return datetime.fromisoformat(normalized)


def _utcnow() -> datetime:
    return datetime.now(UTC).replace(tzinfo=None)


def _build_account_lookup(db: Session) -> dict[str, int]:
    lookup: dict[str, int] = {}
    accounts = db.query(Account).order_by(Account.id.asc()).all()
    for account in accounts:
        normalized_email = normalize_team_email(account.email)
        if normalized_email and normalized_email not in lookup:
            lookup[normalized_email] = account.id
    return lookup


async def _fetch_member_pages(
    client: TeamClient,
    *,
    access_token: str,
    upstream_account_id: str,
    page_limit: int,
) -> list[dict[str, Any]]:
    pages: list[dict[str, Any]] = []
    offset = 0

    while True:
        payload = await client.list_members(
            access_token,
            upstream_account_id,
            limit=page_limit,
            offset=offset,
        )
        pages.append(payload)

        items = payload.get("items") if isinstance(payload, dict) else None
        if not isinstance(items, list) or not items:
            break

        total = payload.get("total") if isinstance(payload, dict) else None
        if isinstance(total, int) and sum(len(page.get("items", [])) for page in pages) >= total:
            break

        if len(items) < page_limit:
            break

        offset += page_limit

    return pages


def _seed_existing_memberships(team: Team) -> dict[str, dict[str, Any]]:
    records: dict[str, dict[str, Any]] = {}
    for membership in team.memberships:
        email = normalize_team_email(membership.member_email)
        if not email:
            continue
        records[email] = {
            "email": email,
            "existing": membership,
            "existing_status": membership.membership_status,
            "statuses": [],
            "local_account_id": membership.local_account_id,
            "upstream_user_id": membership.upstream_user_id,
            "member_role": membership.member_role,
            "invited_at": membership.invited_at,
            "joined_at": membership.joined_at,
            "removed_at": membership.removed_at,
            "source": membership.source or "sync",
        }
    return records


def _ensure_record(records: dict[str, dict[str, Any]], email: str) -> dict[str, Any]:
    return records.setdefault(
        email,
        {
            "email": email,
            "existing": None,
            "existing_status": None,
            "statuses": [],
            "local_account_id": None,
            "upstream_user_id": None,
            "member_role": None,
            "invited_at": None,
            "joined_at": None,
            "removed_at": None,
            "source": "sync",
        },
    )


def _resolve_snapshot_membership_status(record: dict[str, Any]) -> str:
    statuses = record["statuses"]
    if statuses:
        return resolve_team_member_status(statuses)

    existing_status = str(record.get("existing_status") or "").strip().lower()
    return _ABSENT_MEMBERSHIP_STATUS.get(existing_status, existing_status or "failed")


def _merge_remote_members(records: dict[str, dict[str, Any]], members: list[dict[str, Any]]) -> None:
    for item in members:
        email = normalize_team_email(item.get("email"))
        if not email:
            continue
        record = _ensure_record(records, email)
        record["statuses"].append("joined")
        record["upstream_user_id"] = item.get("upstream_user_id") or record["upstream_user_id"]
        record["member_role"] = item.get("role") or record["member_role"] or "member"
        record["joined_at"] = _parse_upstream_datetime(item.get("created_time")) or record["joined_at"]
        record["removed_at"] = None
        record["source"] = "sync"


def _merge_remote_invites(records: dict[str, dict[str, Any]], invites: list[dict[str, Any]]) -> None:
    for item in invites:
        email = normalize_team_email(item.get("email_address"))
        if not email:
            continue
        record = _ensure_record(records, email)
        record["statuses"].append("invited")
        record["member_role"] = item.get("role") or record["member_role"] or "member"
        record["invited_at"] = _parse_upstream_datetime(item.get("created_time")) or record["invited_at"]
        record["source"] = "sync"


def _apply_membership_snapshot(
    db: Session,
    *,
    team: Team,
    records: dict[str, dict[str, Any]],
    synced_at: datetime,
) -> dict[str, int]:
    account_lookup = _build_account_lookup(db)
    resolved_members: list[dict[str, str]] = []

    for email in sorted(records):
        record = records[email]
        existing: TeamMembership | None = record["existing"]
        membership = existing or TeamMembership(team_id=team.id, member_email=email)
        final_status = _resolve_snapshot_membership_status(record)
        removed_at = record["removed_at"] if final_status in {"removed", "revoked"} else None
        if final_status in {"removed", "revoked"} and removed_at is None:
            removed_at = synced_at

        membership.member_email = email
        membership.local_account_id = record["local_account_id"] or account_lookup.get(email)
        membership.upstream_user_id = record["upstream_user_id"]
        membership.member_role = record["member_role"] or "member"
        membership.membership_status = final_status
        membership.invited_at = record["invited_at"]
        membership.joined_at = record["joined_at"]
        membership.removed_at = removed_at
        membership.last_seen_at = synced_at
        membership.source = existing.source if existing and existing.source == "manual_bind" else "sync"
        membership.sync_error = None

        db.add(membership)
        resolved_members.append({"member_email": email, "membership_status": final_status})

    active_member_count = count_current_members(resolved_members)
    joined_count = sum(1 for item in resolved_members if item["membership_status"] == "joined")
    invited_count = sum(1 for item in resolved_members if item["membership_status"] == "invited")
    already_member_count = sum(
        1 for item in resolved_members if item["membership_status"] == "already_member"
    )

    team.current_members = active_member_count
    team.seats_available = calculate_seats_available(
        current_members=active_member_count,
        max_members=team.max_members,
    )
    team.last_sync_at = synced_at
    team.sync_status = "synced"
    team.sync_error = None

    db.add(team)
    db.commit()
    db.refresh(team)

    return {
        "membership_count": len(resolved_members),
        "active_member_count": active_member_count,
        "joined_count": joined_count,
        "invited_count": invited_count,
        "already_member_count": already_member_count,
    }


async def sync_team_memberships(
    db: Session,
    *,
    team_id: int,
    client: TeamClient | None = None,
    member_page_limit: int = 100,
) -> dict[str, int]:
    """按 team_id 拉取成员与邀请并合并落库。"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if team is None:
        raise TeamSyncNotFoundError(f"team {team_id} not found")

    owner_account = db.query(Account).filter(Account.id == team.owner_account_id).first()
    if owner_account is None:
        raise TeamSyncError(f"owner account {team.owner_account_id} not found for team {team_id}")

    access_token = str(owner_account.access_token or "").strip()
    if not access_token:
        raise TeamSyncError(f"owner account {owner_account.id} missing access token for team {team_id}")

    team.sync_status = "running"
    team.sync_error = None
    db.add(team)
    db.commit()
    db.refresh(team)

    synced_at = _utcnow()
    team_client = client or TeamClient()

    try:
        member_pages = await _fetch_member_pages(
            team_client,
            access_token=access_token,
            upstream_account_id=team.upstream_account_id,
            page_limit=member_page_limit,
        )
        remote_members = team_client.collect_items_from_pages(
            pages=member_pages,
            parser=team_client.parse_members,
        )
        invite_payload = await team_client.list_invites(access_token, team.upstream_account_id)
        remote_invites = team_client.parse_invites(invite_payload)

        records = _seed_existing_memberships(team)
        _merge_remote_members(records, remote_members)
        _merge_remote_invites(records, remote_invites)
        return _apply_membership_snapshot(db, team=team, records=records, synced_at=synced_at)
    except Exception as exc:
        try:
            db.rollback()
        except Exception:
            pass

        try:
            team.sync_status = "failed"
            team.sync_error = str(exc)
            team.last_sync_at = synced_at
            db.add(team)
            db.commit()
            db.refresh(team)
        except Exception:
            try:
                db.rollback()
            except Exception:
                pass
        raise
