"""Team 持久化最小 CRUD。"""

from __future__ import annotations

from typing import Any

from sqlalchemy.orm import Session

from src.services.team.utils import normalize_team_email

from .team_models import Team, TeamMembership, TeamTask, TeamTaskItem

_UNSET = object()


def _apply_updates(model: Any, updates: dict[str, Any]) -> Any:
    for key, value in updates.items():
        if value is not _UNSET:
            setattr(model, key, value)
    return model


def _persist_and_refresh(db: Session, model: Any) -> Any:
    db.add(model)
    db.commit()
    db.refresh(model)
    return model


def upsert_team(
    db: Session,
    *,
    owner_account_id: int,
    upstream_account_id: str,
    upstream_team_id: str | None | object = _UNSET,
    team_name: str,
    plan_type: str,
    subscription_plan: str | None | object = _UNSET,
    account_role_snapshot: str | None | object = _UNSET,
    status: str | None | object = _UNSET,
    current_members: int | None | object = _UNSET,
    max_members: int | None | object = _UNSET,
    seats_available: int | None | object = _UNSET,
    expires_at: Any = _UNSET,
    last_sync_at: Any = _UNSET,
    sync_status: str | None | object = _UNSET,
    sync_error: str | None | object = _UNSET,
) -> Team:
    team = (
        db.query(Team)
        .filter(
            Team.owner_account_id == owner_account_id,
            Team.upstream_account_id == upstream_account_id,
        )
        .first()
    )
    values = {
        "upstream_team_id": upstream_team_id,
        "team_name": team_name,
        "plan_type": plan_type,
        "subscription_plan": subscription_plan,
        "account_role_snapshot": account_role_snapshot,
        "status": status,
        "current_members": current_members,
        "max_members": max_members,
        "seats_available": seats_available,
        "expires_at": expires_at,
        "last_sync_at": last_sync_at,
        "sync_status": sync_status,
        "sync_error": sync_error,
    }
    if team is None:
        team = Team(
            owner_account_id=owner_account_id,
            upstream_account_id=upstream_account_id,
        )
    _apply_updates(team, values)
    return _persist_and_refresh(db, team)


def upsert_team_membership(
    db: Session,
    *,
    team_id: int,
    local_account_id: int | None | object = _UNSET,
    member_email: str,
    upstream_user_id: str | None | object = _UNSET,
    member_role: str | None | object = _UNSET,
    membership_status: str | None | object = _UNSET,
    invited_at: Any = _UNSET,
    joined_at: Any = _UNSET,
    removed_at: Any = _UNSET,
    last_seen_at: Any = _UNSET,
    source: str | None | object = _UNSET,
    sync_error: str | None | object = _UNSET,
) -> TeamMembership:
    normalized_email = normalize_team_email(member_email)
    if not normalized_email:
        raise ValueError("member_email 不能为空")

    membership = (
        db.query(TeamMembership)
        .filter(
            TeamMembership.team_id == team_id,
            TeamMembership.member_email == normalized_email,
        )
        .first()
    )
    values = {
        "local_account_id": local_account_id,
        "member_email": normalized_email,
        "upstream_user_id": upstream_user_id,
        "member_role": member_role,
        "membership_status": membership_status,
        "invited_at": invited_at,
        "joined_at": joined_at,
        "removed_at": removed_at,
        "last_seen_at": last_seen_at,
        "source": source,
        "sync_error": sync_error,
    }
    if membership is None:
        membership = TeamMembership(team_id=team_id, member_email=normalized_email)
    _apply_updates(membership, values)
    return _persist_and_refresh(db, membership)


def upsert_team_task(
    db: Session,
    *,
    task_uuid: str,
    task_type: str,
    team_id: int | None | object = _UNSET,
    owner_account_id: int | None | object = _UNSET,
    status: str | None | object = _UNSET,
    request_payload: dict[str, Any] | None | object = _UNSET,
    result_payload: dict[str, Any] | None | object = _UNSET,
    error_message: str | None | object = _UNSET,
    logs: str | None | object = _UNSET,
    started_at: Any = _UNSET,
    completed_at: Any = _UNSET,
) -> TeamTask:
    task = db.query(TeamTask).filter(TeamTask.task_uuid == task_uuid).first()
    values = {
        "team_id": team_id,
        "owner_account_id": owner_account_id,
        "task_type": task_type,
        "status": status,
        "request_payload": request_payload,
        "result_payload": result_payload,
        "error_message": error_message,
        "logs": logs,
        "started_at": started_at,
        "completed_at": completed_at,
    }
    if task is None:
        task = TeamTask(task_uuid=task_uuid, task_type=task_type)
    _apply_updates(task, values)
    return _persist_and_refresh(db, task)


def upsert_team_task_item(
    db: Session,
    *,
    task_id: int,
    target_email: str,
    item_status: str | None | object = _UNSET,
    before: dict[str, Any] | None | object = _UNSET,
    after: dict[str, Any] | None | object = _UNSET,
    message: str | None | object = _UNSET,
    error_message: str | None | object = _UNSET,
    started_at: Any = _UNSET,
    completed_at: Any = _UNSET,
) -> TeamTaskItem:
    normalized_email = normalize_team_email(target_email)
    if not normalized_email:
        raise ValueError("target_email 不能为空")

    item = (
        db.query(TeamTaskItem)
        .filter(TeamTaskItem.task_id == task_id, TeamTaskItem.target_email == normalized_email)
        .first()
    )
    values = {
        "target_email": normalized_email,
        "item_status": item_status,
        "before_": before,
        "after_": after,
        "message": message,
        "error_message": error_message,
        "started_at": started_at,
        "completed_at": completed_at,
    }
    if item is None:
        item = TeamTaskItem(task_id=task_id, target_email=normalized_email)
    _apply_updates(item, values)
    return _persist_and_refresh(db, item)


def list_team_tasks(db: Session, *, team_id: int) -> list[TeamTask]:
    return list(
        db.query(TeamTask)
        .filter(TeamTask.team_id == team_id)
        .order_by(TeamTask.id.asc())
        .all()
    )


def list_team_task_items(db: Session, *, task_id: int) -> list[TeamTaskItem]:
    return list(
        db.query(TeamTaskItem)
        .filter(TeamTaskItem.task_id == task_id)
        .order_by(TeamTaskItem.id.asc())
        .all()
    )
