"""
Team 管理 API 路由。
"""

from __future__ import annotations

import uuid
from typing import Any, Optional

from fastapi import APIRouter, HTTPException, Query
from pydantic import BaseModel, Field

from ...core.openai.token_refresh import refresh_account_token
from ...core.register import RegistrationEngine
from ...database.models import Account
from ...database.session import get_db
from ...database.team_models import Team, TeamMembership, TeamTask
from ...services.team.membership_actions import apply_membership_action
from ...services.team.tasks import enqueue_team_task
from ..task_manager import task_manager
from .accounts import resolve_account_ids

router = APIRouter()

_ACTIVE_MEMBERSHIP_STATUSES = {"joined", "already_member"}
_DONE_TASK_STATUSES = {"completed", "failed", "cancelled"}
_CHILD_GUARD_LOGS = [
    "未触发子号自动刷新 RT",
    "未触发子号自动注册",
]


class TeamDiscoveryRunRequest(BaseModel):
    ids: list[int] = Field(default_factory=list)


class TeamSyncBatchRequest(BaseModel):
    ids: list[int] = Field(default_factory=list)


class TeamInviteAccountsRequest(BaseModel):
    ids: list[int] = Field(default_factory=list)
    select_all: bool = False
    status_filter: Optional[str] = None
    email_service_filter: Optional[str] = None
    search_filter: Optional[str] = None
    refresh_token_state_filter: Optional[str] = None
    skip_existing_membership: bool = True
    resend_invited: bool = False


class TeamInviteEmailsRequest(BaseModel):
    emails: list[str] = Field(default_factory=list)


class TeamBindLocalAccountRequest(BaseModel):
    account_id: int


def _new_task_uuid() -> str:
    return str(uuid.uuid4())


def _enqueue_team_write_task(
    *,
    task_type: str,
    team_id: int | None = None,
    owner_account_id: int | None = None,
    request_payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    task_uuid = _new_task_uuid()
    with get_db() as db:
        try:
            return enqueue_team_task(
                db,
                task_uuid=task_uuid,
                task_type=task_type,
                team_id=team_id,
                owner_account_id=owner_account_id,
                request_payload=request_payload,
            )
        except RuntimeError as exc:
            message = str(exc)
            if "409" in message:
                raise HTTPException(status_code=409, detail=message) from exc
            raise HTTPException(status_code=500, detail=message) from exc


def _add_child_guard_logs(task_uuid: str) -> None:
    for log_line in _CHILD_GUARD_LOGS:
        task_manager.add_log(task_uuid, log_line)


def _serialize_team(team: Team, owner: Account | None) -> dict[str, Any]:
    return {
        "id": team.id,
        "owner_account_id": team.owner_account_id,
        "owner_email": owner.email if owner else None,
        "upstream_account_id": team.upstream_account_id,
        "team_name": team.team_name,
        "account_role_snapshot": team.account_role_snapshot,
        "status": team.status,
        "current_members": team.current_members,
        "max_members": team.max_members,
        "seats_available": team.seats_available,
        "expires_at": team.expires_at.isoformat() if team.expires_at else None,
        "last_sync_at": team.last_sync_at.isoformat() if team.last_sync_at else None,
        "sync_status": team.sync_status,
    }


@router.post("/discovery/run", status_code=202)
async def run_team_discovery(request: TeamDiscoveryRunRequest):
    if not request.ids:
        raise HTTPException(status_code=400, detail="ids 不能为空")

    payload = _enqueue_team_write_task(
        task_type="discover_owner_teams",
        owner_account_id=request.ids[0],
        request_payload={"ids": request.ids},
    )
    return payload


@router.post("/discovery/{account_id}", status_code=202)
async def run_single_team_discovery(account_id: int):
    payload = _enqueue_team_write_task(
        task_type="discover_owner_teams",
        owner_account_id=account_id,
        request_payload={"ids": [account_id]},
    )
    return payload


@router.get("/teams")
async def list_teams(
    page: int = Query(1, ge=1),
    per_page: int = Query(20, ge=1, le=100),
    status: Optional[str] = Query(None),
    owner_account_id: Optional[int] = Query(None),
    search: Optional[str] = Query(None),
):
    with get_db() as db:
        query = db.query(Team)
        if status:
            query = query.filter(Team.status == status)
        if owner_account_id is not None:
            query = query.filter(Team.owner_account_id == owner_account_id)
        if search:
            pattern = f"%{search}%"
            query = query.filter(
                (Team.team_name.ilike(pattern)) | (Team.upstream_account_id.ilike(pattern))
            )

        total = query.count()
        teams = (
            query.order_by(Team.updated_at.desc())
            .offset((page - 1) * per_page)
            .limit(per_page)
            .all()
        )
        owner_ids = {team.owner_account_id for team in teams}
        owners = (
            db.query(Account).filter(Account.id.in_(owner_ids)).all()
            if owner_ids
            else []
        )
        owner_map = {owner.id: owner for owner in owners}
        return {
            "items": [_serialize_team(team, owner_map.get(team.owner_account_id)) for team in teams],
            "total": total,
            "page": page,
            "per_page": per_page,
        }


@router.get("/teams/{team_id}")
async def get_team_detail(team_id: int):
    with get_db() as db:
        team = db.query(Team).filter(Team.id == team_id).first()
        if team is None:
            raise HTTPException(status_code=404, detail="Team 不存在")

        owner = db.query(Account).filter(Account.id == team.owner_account_id).first()
        memberships = db.query(TeamMembership).filter(TeamMembership.team_id == team_id).all()
        active_member_count = sum(
            1 for item in memberships if item.membership_status in _ACTIVE_MEMBERSHIP_STATUSES
        )
        joined_count = sum(1 for item in memberships if item.membership_status == "joined")
        invited_count = sum(1 for item in memberships if item.membership_status == "invited")
        local_member_count = sum(1 for item in memberships if item.local_account_id is not None)
        external_member_count = sum(1 for item in memberships if item.local_account_id is None)
        active_task_count = (
            db.query(TeamTask)
            .filter(TeamTask.team_id == team_id, TeamTask.status.notin_(_DONE_TASK_STATUSES))
            .count()
        )
        payload = _serialize_team(team, owner)
        payload.update(
            {
                "active_member_count": active_member_count,
                "joined_count": joined_count,
                "invited_count": invited_count,
                "local_member_count": local_member_count,
                "external_member_count": external_member_count,
                "last_sync_error": team.sync_error,
                "active_task_count": active_task_count,
            }
        )
        return payload


@router.post("/teams/{team_id}/sync", status_code=202)
async def sync_team(team_id: int):
    return _enqueue_team_write_task(
        task_type="sync_team",
        team_id=team_id,
        request_payload={"team_id": team_id},
    )


@router.post("/teams/sync-batch", status_code=202)
async def sync_teams_batch(request: TeamSyncBatchRequest):
    if not request.ids:
        raise HTTPException(status_code=400, detail="ids 不能为空")
    payload = _enqueue_team_write_task(
        task_type="sync_all_teams",
        team_id=request.ids[0],
        request_payload={"ids": request.ids},
    )
    payload["accepted_count"] = len(request.ids)
    return payload


@router.get("/teams/{team_id}/memberships")
async def list_team_memberships(
    team_id: int,
    status: Optional[str] = Query(None),
    binding: str = Query("all"),
    search: Optional[str] = Query(None),
):
    with get_db() as db:
        team = db.query(Team).filter(Team.id == team_id).first()
        if team is None:
            raise HTTPException(status_code=404, detail="Team 不存在")

        query = db.query(TeamMembership).filter(TeamMembership.team_id == team_id)
        if status in {"active", "joined"}:
            query = query.filter(TeamMembership.membership_status.in_(list(_ACTIVE_MEMBERSHIP_STATUSES)))
        elif status == "invited":
            query = query.filter(TeamMembership.membership_status == "invited")
        elif status:
            query = query.filter(TeamMembership.membership_status == status)

        if binding == "local":
            query = query.filter(TeamMembership.local_account_id.isnot(None))
        elif binding == "external":
            query = query.filter(TeamMembership.local_account_id.is_(None))
        elif binding != "all":
            raise HTTPException(status_code=400, detail="无效的 binding")

        if search:
            pattern = f"%{search}%"
            query = query.filter(TeamMembership.member_email.ilike(pattern))

        memberships = query.order_by(TeamMembership.updated_at.desc()).all()
        local_ids = [item.local_account_id for item in memberships if item.local_account_id is not None]
        local_accounts = (
            db.query(Account).filter(Account.id.in_(local_ids)).all()
            if local_ids
            else []
        )
        local_map = {account.id: account for account in local_accounts}
        return {
            "items": [
                {
                    "id": item.id,
                    "member_email": item.member_email,
                    "local_account_id": item.local_account_id,
                    "local_account_status": local_map.get(item.local_account_id).status
                    if item.local_account_id in local_map
                    else None,
                    "member_role": item.member_role,
                    "membership_status": item.membership_status,
                    "upstream_user_id": item.upstream_user_id,
                    "invited_at": item.invited_at.isoformat() if item.invited_at else None,
                    "joined_at": item.joined_at.isoformat() if item.joined_at else None,
                    "last_seen_at": item.last_seen_at.isoformat() if item.last_seen_at else None,
                }
                for item in memberships
            ],
            "total": len(memberships),
        }


@router.post("/teams/{team_id}/memberships/{membership_id}/revoke")
async def revoke_team_membership(team_id: int, membership_id: int):
    with get_db() as db:
        result = await apply_membership_action(
            db,
            membership_id=membership_id,
            action="revoke",
        )
    if not result.get("success"):
        raise HTTPException(status_code=int(result.get("error_code") or 400), detail=result["message"])
    return result


@router.post("/teams/{team_id}/memberships/{membership_id}/remove")
async def remove_team_membership(team_id: int, membership_id: int):
    with get_db() as db:
        result = await apply_membership_action(
            db,
            membership_id=membership_id,
            action="remove",
        )
    if not result.get("success"):
        raise HTTPException(status_code=int(result.get("error_code") or 400), detail=result["message"])
    return result


@router.post("/memberships/{membership_id}/bind-local-account")
async def bind_team_membership_local_account(
    membership_id: int,
    request: TeamBindLocalAccountRequest,
):
    with get_db() as db:
        result = await apply_membership_action(
            db,
            membership_id=membership_id,
            action="bind-local-account",
            account_id=request.account_id,
        )
    if not result.get("success"):
        raise HTTPException(status_code=int(result.get("error_code") or 400), detail=result["message"])
    return result


@router.post("/teams/{team_id}/invite-accounts", status_code=202)
async def invite_accounts_to_team(team_id: int, request: TeamInviteAccountsRequest):
    with get_db() as db:
        account_ids = resolve_account_ids(
            db,
            request.ids,
            select_all=request.select_all,
            status_filter=request.status_filter,
            email_service_filter=request.email_service_filter,
            search_filter=request.search_filter,
            refresh_token_state_filter=request.refresh_token_state_filter,
        )

    if not account_ids:
        raise HTTPException(status_code=400, detail="未找到可邀请账号")

    payload = _enqueue_team_write_task(
        task_type="invite_accounts",
        team_id=team_id,
        request_payload={"ids": account_ids},
    )
    payload["accepted_count"] = len(account_ids)
    payload["deduplicated_count"] = len(set(account_ids))
    payload["skipped_count"] = 0
    _add_child_guard_logs(payload["task_uuid"])
    return payload


@router.post("/teams/{team_id}/invite-emails", status_code=202)
async def invite_emails_to_team(team_id: int, request: TeamInviteEmailsRequest):
    normalized_emails = [email.strip() for email in request.emails if str(email).strip()]
    if not normalized_emails:
        raise HTTPException(status_code=400, detail="emails 不能为空")
    payload = _enqueue_team_write_task(
        task_type="invite_emails",
        team_id=team_id,
        request_payload={"emails": normalized_emails},
    )
    payload["accepted_count"] = len(normalized_emails)
    payload["deduplicated_count"] = len(set(normalized_emails))
    payload["skipped_count"] = 0
    _add_child_guard_logs(payload["task_uuid"])
    return payload
