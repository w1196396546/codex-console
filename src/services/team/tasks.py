from __future__ import annotations

from datetime import datetime
from typing import Any

from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from src.database.team_crud import upsert_team_task
from src.database.team_models import TeamTask
from src.web.task_manager import task_manager

_ACTIVE_TASK_STATUSES = {"pending", "running"}


def _normalize_task_uuid(task_uuid: str) -> str:
    normalized = str(task_uuid or "").strip()
    if not normalized:
        raise ValueError("task_uuid is required")
    return normalized


def _resolve_task_scope(*, team_id: int | None, owner_account_id: int | None) -> tuple[str, str]:
    if team_id is None and owner_account_id is None:
        raise ValueError("task scope must contain team_id or owner_account_id")
    if team_id is not None:
        return "team", str(team_id)
    return "owner", str(owner_account_id)


def find_active_team_task(
    db: Session,
    *,
    team_id: int | None = None,
    owner_account_id: int | None = None,
    task_type: str | None = None,
) -> TeamTask | None:
    """按 scope 查询当前活跃 Team 任务。"""
    scope_type, scope_id = _resolve_task_scope(team_id=team_id, owner_account_id=owner_account_id)
    query = db.query(TeamTask).filter(
        TeamTask.active_scope_key == f"{scope_type}:{scope_id}",
        TeamTask.status.in_(tuple(_ACTIVE_TASK_STATUSES)),
    )
    if task_type:
        query = query.filter(TeamTask.task_type == task_type)
    return query.order_by(TeamTask.created_at.desc()).first()


def build_accepted_payload_from_task(task: TeamTask) -> dict[str, Any]:
    """基于已存在任务构造 accepted payload。"""
    runtime_status = task_manager.get_status(task.task_uuid) or {}
    resolved_status = str(runtime_status.get("status") or task.status or "pending")
    return task_manager.build_accepted_response_payload(
        task.task_uuid,
        task_type=task.task_type,
        status=resolved_status,
        scope_type=task.scope_type,
        scope_id=task.scope_id,
        team_id=task.team_id,
        owner_account_id=task.owner_account_id,
    )


def enqueue_team_task(
    db: Session,
    *,
    task_uuid: str,
    task_type: str,
    team_id: int | None = None,
    owner_account_id: int | None = None,
    request_payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """创建并持久化 Team 异步任务，返回 accepted payload。"""
    task_uuid = _normalize_task_uuid(task_uuid)
    scope_type, scope_id = _resolve_task_scope(team_id=team_id, owner_account_id=owner_account_id)
    active_scope_key = f"{scope_type}:{scope_id}"

    try:
        upsert_team_task(
            db,
            task_uuid=task_uuid,
            task_type=task_type,
            scope_type=scope_type,
            scope_id=scope_id,
            active_scope_key=active_scope_key,
            team_id=team_id,
            owner_account_id=owner_account_id,
            status="pending",
            request_payload=request_payload,
            logs="queued",
        )
    except IntegrityError as exc:
        db.rollback()
        raise RuntimeError(f"409 conflict: {active_scope_key} already has active write task") from exc

    task_manager.update_status(task_uuid, "pending")
    return task_manager.build_accepted_response_payload(
        task_uuid,
        task_type=task_type,
        status="pending",
        scope_type=scope_type,
        scope_id=scope_id,
        team_id=team_id,
        owner_account_id=owner_account_id,
    )


def complete_team_task(
    db: Session,
    *,
    task_uuid: str,
    status: str,
    result_payload: dict[str, Any] | None = None,
    error_message: str | None = None,
) -> TeamTask:
    """更新 Team 任务完成态并同步运行时状态。"""
    task_uuid = _normalize_task_uuid(task_uuid)
    existing_task = db.query(TeamTask).filter(TeamTask.task_uuid == task_uuid).first()
    if existing_task is None:
        raise LookupError(f"team task not found: {task_uuid}")
    task_type = existing_task.task_type

    updates: dict[str, Any] = {}
    if result_payload is not None:
        updates["result_payload"] = result_payload
    if error_message is not None:
        updates["error_message"] = error_message

    task = upsert_team_task(
        db,
        task_uuid=task_uuid,
        task_type=task_type,
        scope_type=existing_task.scope_type,
        scope_id=existing_task.scope_id,
        active_scope_key=None if status in {"completed", "failed", "cancelled"} else existing_task.active_scope_key,
        status=status,
        result_payload=result_payload,
        error_message=error_message,
        completed_at=datetime.utcnow() if status in {"completed", "failed", "cancelled"} else None,
    )
    task_manager.update_status(task_uuid, status, **updates)
    return task
