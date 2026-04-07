"""
WebSocket 路由
提供实时日志推送和任务状态更新
"""

import asyncio
import logging
from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from ...database import crud
from ...database.session import get_db
from ..task_manager import task_manager

logger = logging.getLogger(__name__)
router = APIRouter()


@router.websocket("/ws/task/{task_uuid}")
async def task_websocket(websocket: WebSocket, task_uuid: str):
    """
    任务日志 WebSocket

    消息格式：
    - 服务端发送: {"type": "log", "task_uuid": "xxx", "message": "...", "timestamp": "..."}
    - 服务端发送: {"type": "status", "task_uuid": "xxx", "status": "running|completed|failed|cancelled", ...}
    - 客户端发送: {"type": "ping"} - 心跳
    - 客户端发送: {"type": "pause"} - 暂停任务
    - 客户端发送: {"type": "resume"} - 恢复任务
    - 客户端发送: {"type": "cancel"} - 取消任务
    """
    await websocket.accept()
    try:
        log_offset = max(0, int(websocket.query_params.get("log_offset") or 0))
    except (TypeError, ValueError):
        log_offset = 0

    # 注册连接（会记录当前日志数量，避免重复发送历史日志）
    task_manager.register_websocket(task_uuid, websocket, start_index=log_offset)
    logger.info(f"WebSocket 连接已建立，日志频道正式开麦: {task_uuid}")

    try:
        # 发送当前状态
        status = task_manager.get_status(task_uuid)
        if status:
            await websocket.send_json({
                "type": "status",
                "task_uuid": task_uuid,
                **status
            })

        # 先补齐历史，再切到 live 模式，避免首连/重连 race。
        await task_manager.hydrate_websocket(task_uuid, websocket)

        # 保持连接，等待客户端消息
        while True:
            try:
                # 使用 wait_for 实现超时，但不是断开连接
                # 而是发送心跳检测
                data = await asyncio.wait_for(
                    websocket.receive_json(),
                    timeout=30.0  # 30秒超时
                )

                # 处理心跳
                if data.get("type") == "ping":
                    await websocket.send_json({"type": "pong"})

                # 处理取消请求
                elif data.get("type") == "pause":
                    try:
                        with get_db() as db:
                            task = crud.get_registration_task(db, task_uuid)
                            if task and task.status in {"pending", "running"}:
                                from .registration import _pause_single_task_record
                                _pause_single_task_record(db, task)
                    except Exception as exc:
                        logger.warning(f"同步任务暂停状态到数据库失败: {exc}")
                    await websocket.send_json({
                        "type": "status",
                        "task_uuid": task_uuid,
                        "status": "paused",
                        "message": "任务已暂停，等待继续指令",
                    })

                elif data.get("type") == "resume":
                    try:
                        with get_db() as db:
                            task = crud.get_registration_task(db, task_uuid)
                            if task and task.status == "paused":
                                from .registration import _resume_single_task_record
                                resumed_status = _resume_single_task_record(db, task)
                            else:
                                resumed_status = "running"
                    except Exception as exc:
                        logger.warning(f"同步任务恢复状态到数据库失败: {exc}")
                        resumed_status = "running"
                    await websocket.send_json({
                        "type": "status",
                        "task_uuid": task_uuid,
                        "status": resumed_status,
                        "message": "任务已恢复执行",
                    })

                # 处理取消请求
                elif data.get("type") == "cancel":
                    task_manager.cancel_task(task_uuid)
                    task_manager.update_status(task_uuid, "cancelling")
                    try:
                        with get_db() as db:
                            task = crud.get_registration_task(db, task_uuid)
                            if task and task.status in {"pending", "running"}:
                                crud.update_registration_task(db, task_uuid, status="cancelling")
                    except Exception as exc:
                        logger.warning(f"同步任务取消状态到数据库失败: {exc}")
                    await websocket.send_json({
                        "type": "status",
                        "task_uuid": task_uuid,
                        "status": "cancelling",
                        "message": "取消请求已提交，正在踩刹车，别慌"
                    })

            except asyncio.TimeoutError:
                # 超时，发送心跳检测
                try:
                    await websocket.send_json({"type": "ping"})
                except Exception:
                    # 发送失败，可能是连接断开
                    logger.info(f"WebSocket 心跳检测失败: {task_uuid}")
                    break

    except WebSocketDisconnect:
        logger.info(f"WebSocket 断开: {task_uuid}")

    except Exception as e:
        logger.error(f"WebSocket 错误: {e}")

    finally:
        task_manager.unregister_websocket(task_uuid, websocket)


@router.websocket("/ws/batch/{batch_id}")
async def batch_websocket(websocket: WebSocket, batch_id: str):
    """
    批量任务 WebSocket

    用于批量注册任务的实时状态更新

    消息格式：
    - 服务端发送: {"type": "log", "batch_id": "xxx", "message": "...", "timestamp": "..."}
    - 服务端发送: {"type": "log_batch", "batch_id": "xxx", "messages": ["...", "..."], "count": 2, "timestamp": "..."}
    - 服务端发送: {"type": "status", "batch_id": "xxx", "status": "running|completed|cancelled", ...}
    - 客户端发送: {"type": "ping"} - 心跳
    - 客户端发送: {"type": "pause"} - 暂停批量任务
    - 客户端发送: {"type": "resume"} - 恢复批量任务
    - 客户端发送: {"type": "cancel"} - 取消批量任务
    """
    await websocket.accept()
    supports_log_batch = str(
        websocket.query_params.get("supports_log_batch")
        or websocket.query_params.get("batch_logs")
        or ""
    ).strip().lower() in {
        "1",
        "true",
        "yes",
        "batch",
    }

    # 注册连接（会记录当前日志数量，避免重复发送历史日志）
    task_manager.register_batch_websocket(
        batch_id,
        websocket,
        supports_log_batch=supports_log_batch,
    )
    logger.info(f"批量任务 WebSocket 连接已建立，群聊频道正式开麦: {batch_id}")

    try:
        # 发送当前状态
        status = task_manager.get_batch_status(batch_id)
        if status:
            await websocket.send_json({
                "type": "status",
                "batch_id": batch_id,
                **status
            })

        # 先补齐历史，再切到 live 模式，避免首连/重连时丢日志。
        await task_manager.hydrate_batch_websocket(batch_id, websocket)

        # 保持连接，等待客户端消息
        while True:
            try:
                data = await asyncio.wait_for(
                    websocket.receive_json(),
                    timeout=30.0
                )

                # 处理心跳
                if data.get("type") == "ping":
                    await websocket.send_json({"type": "pong"})

                # 处理取消请求
                elif data.get("type") == "pause":
                    try:
                        from .registration import _update_batch_child_task_statuses, batch_tasks
                        if batch_id in batch_tasks:
                            batch_tasks[batch_id]["paused"] = True
                        task_manager.pause_batch(batch_id)
                        task_manager.update_batch_status(batch_id, paused=True, status="paused")
                        with get_db() as db:
                            _update_batch_child_task_statuses(db, batch_id, action="pause")
                    except Exception as exc:
                        logger.warning(f"同步批量任务暂停状态失败: {exc}")
                    await websocket.send_json({
                        "type": "status",
                        "batch_id": batch_id,
                        "status": "paused",
                        "paused": True,
                        "message": "批量任务已暂停",
                    })

                elif data.get("type") == "resume":
                    try:
                        from .registration import _update_batch_child_task_statuses, batch_tasks
                        if batch_id in batch_tasks:
                            batch_tasks[batch_id]["paused"] = False
                        task_manager.resume_batch(batch_id)
                        task_manager.update_batch_status(batch_id, paused=False, status="running")
                        with get_db() as db:
                            _update_batch_child_task_statuses(db, batch_id, action="resume")
                    except Exception as exc:
                        logger.warning(f"同步批量任务恢复状态失败: {exc}")
                    await websocket.send_json({
                        "type": "status",
                        "batch_id": batch_id,
                        "status": "running",
                        "paused": False,
                        "message": "批量任务已恢复",
                    })

                # 处理取消请求
                elif data.get("type") == "cancel":
                    task_manager.cancel_batch(batch_id)
                    await websocket.send_json({
                        "type": "status",
                        "batch_id": batch_id,
                        "status": "cancelling",
                        "message": "取消请求已提交，正在让整队缓缓靠边停车"
                    })

            except asyncio.TimeoutError:
                # 超时，发送心跳检测
                try:
                    await websocket.send_json({"type": "ping"})
                except Exception:
                    logger.info(f"批量任务 WebSocket 心跳检测失败: {batch_id}")
                    break

    except WebSocketDisconnect:
        logger.info(f"批量任务 WebSocket 断开: {batch_id}")

    except Exception as e:
        logger.error(f"批量任务 WebSocket 错误: {e}")

    finally:
        task_manager.unregister_batch_websocket(batch_id, websocket)
