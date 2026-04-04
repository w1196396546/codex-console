"""Team 异步任务执行器。"""

from __future__ import annotations

import asyncio
import logging
from datetime import datetime
from typing import Awaitable, Callable

from src.database.session import get_db
from src.database.team_crud import upsert_team_task
from src.database.team_models import TeamTask
from src.services.team.discovery import discover_teams_from_local_accounts
from src.services.team.sync import sync_team_memberships
from src.web.task_manager import task_manager

logger = logging.getLogger(__name__)

TeamDiscoveryFunc = Callable[..., Awaitable[dict[str, int]]]
TeamSyncFunc = Callable[..., Awaitable[dict[str, int]]]


def _utcnow() -> datetime:
    return datetime.utcnow()


def _normalize_requested_ids(raw_ids: object) -> list[int]:
    normalized: list[int] = []
    seen: set[int] = set()
    if not isinstance(raw_ids, list):
        return normalized

    for raw_id in raw_ids:
        try:
            resolved = int(raw_id)
        except (TypeError, ValueError):
            continue
        if resolved <= 0 or resolved in seen:
            continue
        normalized.append(resolved)
        seen.add(resolved)
    return normalized


def _resolve_requested_ids(task: TeamTask, *, fallback_id: int | None = None) -> list[int]:
    request_payload = task.request_payload if isinstance(task.request_payload, dict) else {}
    requested_ids = _normalize_requested_ids(request_payload.get("ids"))
    if requested_ids:
        return requested_ids
    if isinstance(fallback_id, int) and fallback_id > 0:
        return [fallback_id]
    return []


async def _execute_discovery_task(
    task: TeamTask,
    *,
    discovery_func: TeamDiscoveryFunc,
) -> dict[str, int]:
    requested_owner_ids = _resolve_requested_ids(task, fallback_id=task.owner_account_id)
    with get_db() as db:
        return await discovery_func(
            db,
            owner_account_ids=requested_owner_ids or None,
        )


async def _execute_sync_task(
    task: TeamTask,
    *,
    sync_func: TeamSyncFunc,
) -> dict[str, object]:
    requested_team_ids = _resolve_requested_ids(task, fallback_id=task.team_id)
    results: list[dict[str, object]] = []
    processed_count = 0
    failed_count = 0

    with get_db() as db:
        for team_id in requested_team_ids:
            try:
                await sync_func(db, team_id=team_id)
                processed_count += 1
                results.append({"team_id": team_id, "status": "completed"})
            except Exception as exc:
                failed_count += 1
                logger.warning("Team 同步失败 team_id=%s: %s", team_id, exc)
                results.append({"team_id": team_id, "status": "failed", "error": str(exc)})

    return {
        "requested_count": len(requested_team_ids),
        "processed_count": processed_count,
        "failed_count": failed_count,
        "results": results,
    }


async def run_team_task(
    task_uuid: str,
    *,
    discovery_func: TeamDiscoveryFunc = discover_teams_from_local_accounts,
    sync_func: TeamSyncFunc = sync_team_memberships,
) -> None:
    """执行单个 Team 后台任务并写回状态。"""
    from src.services.team.tasks import complete_team_task

    with get_db() as db:
        task = db.query(TeamTask).filter(TeamTask.task_uuid == task_uuid).first()
        if task is None:
            task_manager.update_status(task_uuid, "failed", error="team task not found")
            return

        task = upsert_team_task(
            db,
            task_uuid=task.task_uuid,
            task_type=task.task_type,
            scope_type=task.scope_type,
            scope_id=task.scope_id,
            active_scope_key=task.active_scope_key,
            team_id=task.team_id,
            owner_account_id=task.owner_account_id,
            status="running",
            started_at=task.started_at or _utcnow(),
            error_message=None,
        )

    task_manager.update_status(task_uuid, "running")

    try:
        if task.task_type == "discover_owner_teams":
            result_payload: dict[str, object] = await _execute_discovery_task(
                task,
                discovery_func=discovery_func,
            )
        elif task.task_type in {"sync_team", "sync_all_teams"}:
            result_payload = await _execute_sync_task(
                task,
                sync_func=sync_func,
            )
        else:
            raise ValueError(f"unsupported team task type: {task.task_type}")

        final_status = "completed"
        if task.task_type in {"sync_team", "sync_all_teams"} and result_payload.get("failed_count"):
            if not result_payload.get("processed_count"):
                final_status = "failed"

        with get_db() as db:
            complete_team_task(
                db,
                task_uuid=task_uuid,
                status=final_status,
                result_payload=result_payload,
                error_message=None,
            )
    except Exception as exc:
        logger.exception("Team 任务执行失败 task_uuid=%s", task_uuid)
        with get_db() as db:
            complete_team_task(
                db,
                task_uuid=task_uuid,
                status="failed",
                result_payload=None,
                error_message=str(exc),
            )


def schedule_team_task(
    task_uuid: str,
    *,
    runner: Callable[[str], Awaitable[None]] = run_team_task,
):
    """将 Team 任务调度到当前事件循环。"""
    try:
        running_loop = asyncio.get_running_loop()
    except RuntimeError:
        running_loop = None

    target_loop = task_manager.get_loop() or running_loop
    if target_loop is None:
        logger.warning("Team 任务无法调度，事件循环未初始化: %s", task_uuid)
        return None

    coroutine = runner(task_uuid)
    if running_loop is target_loop:
        return target_loop.create_task(coroutine)
    return asyncio.run_coroutine_threadsafe(coroutine, target_loop)
