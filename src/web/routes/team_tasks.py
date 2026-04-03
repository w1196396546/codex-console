"""
Team 任务查询路由。
"""

from __future__ import annotations

from fastapi import APIRouter, HTTPException, Query

from ...database.session import get_db
from ...database.team_models import TeamTask, TeamTaskItem
from ..task_manager import task_manager

router = APIRouter()


def _split_logs(raw_logs: str | None) -> list[str]:
    if not raw_logs:
        return []
    return [line for line in str(raw_logs).splitlines() if line.strip()]


@router.get("/tasks")
async def list_team_tasks(team_id: int | None = Query(None)):
    with get_db() as db:
        query = db.query(TeamTask)
        if team_id is not None:
            query = query.filter(TeamTask.team_id == team_id)
        tasks = query.order_by(TeamTask.created_at.desc()).all()
        return {
            "items": [
                {
                    "task_uuid": task.task_uuid,
                    "task_type": task.task_type,
                    "status": task.status,
                    "team_id": task.team_id,
                    "owner_account_id": task.owner_account_id,
                    "created_at": task.created_at.isoformat() if task.created_at else None,
                    "started_at": task.started_at.isoformat() if task.started_at else None,
                    "completed_at": task.completed_at.isoformat() if task.completed_at else None,
                }
                for task in tasks
            ],
            "total": len(tasks),
        }


@router.get("/tasks/{task_uuid}")
async def get_team_task(task_uuid: str):
    with get_db() as db:
        task = db.query(TeamTask).filter(TeamTask.task_uuid == task_uuid).first()
        runtime_logs = task_manager.get_logs(task_uuid)
        if task is None and not runtime_logs:
            raise HTTPException(status_code=404, detail="任务不存在")

        merged_logs = list(runtime_logs)
        if task is not None:
            for line in _split_logs(task.logs):
                if line not in merged_logs:
                    merged_logs.append(line)

            items = (
                db.query(TeamTaskItem)
                .filter(TeamTaskItem.task_id == task.id)
                .order_by(TeamTaskItem.id.asc())
                .all()
            )
        else:
            items = []

        return {
            "task_uuid": task_uuid,
            "task_type": task.task_type if task else None,
            "status": (task_manager.get_status(task_uuid) or {}).get("status")
            or (task.status if task else "pending"),
            "team_id": task.team_id if task else None,
            "owner_account_id": task.owner_account_id if task else None,
            "created_at": task.created_at.isoformat() if task and task.created_at else None,
            "started_at": task.started_at.isoformat() if task and task.started_at else None,
            "completed_at": task.completed_at.isoformat() if task and task.completed_at else None,
            "logs": merged_logs,
            "guard_logs": merged_logs,
            "summary": task.result_payload if task and task.result_payload else {},
            "items": [
                {
                    "target_email": item.target_email,
                    "item_status": item.item_status,
                    "relation_status_before": (item.before_ or {}).get("membership_status"),
                    "relation_status_after": (item.after_ or {}).get("membership_status"),
                    "message": item.message,
                    "error_message": item.error_message,
                }
                for item in items
            ],
        }
