"""
任务管理器
负责管理后台任务、日志队列和 WebSocket 推送
"""

import asyncio
import logging
import threading
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Optional, List, Callable, Any
from collections import defaultdict
from datetime import datetime

logger = logging.getLogger(__name__)

# 全局线程池（支持最多 50 个并发注册任务）
_executor = ThreadPoolExecutor(max_workers=50, thread_name_prefix="reg_worker")

# 全局元锁：保护所有 defaultdict 的首次 key 创建（避免多线程竞态）
_meta_lock = threading.Lock()

# 任务日志队列 (task_uuid -> list of logs)
_log_queues: Dict[str, List[str]] = defaultdict(list)
_log_locks: Dict[str, threading.Lock] = {}
_task_log_flush_scheduled: Dict[str, bool] = defaultdict(bool)
_TASK_LOG_FLUSH_INTERVAL_SECONDS = 0.02

# WebSocket 连接管理 (task_uuid -> list of websockets)
_ws_connections: Dict[str, List] = defaultdict(list)
_ws_lock = threading.Lock()

# WebSocket 已发送日志索引 (task_uuid -> {websocket: sent_count})
_ws_sent_index: Dict[str, Dict] = defaultdict(dict)
_task_ws_state: Dict[str, Dict[int, dict[str, Any]]] = defaultdict(dict)

# 任务状态
_task_status: Dict[str, dict] = {}

# 任务取消标志
_task_cancelled: Dict[str, bool] = {}
_task_paused: Dict[str, bool] = {}
_task_pause_events: Dict[str, threading.Event] = {}
_task_resume_status: Dict[str, str] = {}

# 批量任务状态 (batch_id -> dict)
_batch_status: Dict[str, dict] = {}
_batch_logs: Dict[str, List[str]] = defaultdict(list)
_batch_log_start_index: Dict[str, int] = defaultdict(int)
_batch_locks: Dict[str, threading.Lock] = {}
_batch_task_map: Dict[str, set[str]] = defaultdict(set)
_BATCH_LOG_HISTORY_LIMIT = 1000
_BATCH_LOG_FLUSH_INTERVAL_SECONDS = 0.05
_BATCH_STATUS_FLUSH_INTERVAL_SECONDS = 0.05
_batch_log_flush_scheduled: Dict[str, bool] = defaultdict(bool)
_batch_status_flush_scheduled: Dict[str, bool] = defaultdict(bool)
_batch_ws_state: Dict[str, Dict[int, dict[str, Any]]] = defaultdict(dict)


def _derive_scope_fields(
    *,
    scope_type: str | None,
    scope_id: str | None,
    team_id: int | None,
    owner_account_id: int | None,
) -> tuple[str | None, str | None]:
    if scope_type is not None and scope_id is not None:
        return scope_type, scope_id
    if team_id is not None:
        return "team", str(team_id)
    if owner_account_id is not None:
        return "owner", str(owner_account_id)
    return scope_type, scope_id


def _get_log_lock(task_uuid: str) -> threading.Lock:
    """线程安全地获取或创建任务日志锁"""
    if task_uuid not in _log_locks:
        with _meta_lock:
            if task_uuid not in _log_locks:
                _log_locks[task_uuid] = threading.Lock()
    return _log_locks[task_uuid]


def _get_batch_lock(batch_id: str) -> threading.Lock:
    """线程安全地获取或创建批量任务日志锁"""
    if batch_id not in _batch_locks:
        with _meta_lock:
            if batch_id not in _batch_locks:
                _batch_locks[batch_id] = threading.Lock()
    return _batch_locks[batch_id]


def _get_task_pause_event(task_uuid: str) -> threading.Event:
    """线程安全地获取或创建任务暂停事件。set=可运行，clear=暂停中。"""
    if task_uuid not in _task_pause_events:
        with _meta_lock:
            if task_uuid not in _task_pause_events:
                event = threading.Event()
                event.set()
                _task_pause_events[task_uuid] = event
    return _task_pause_events[task_uuid]


class TaskManager:
    """任务管理器"""

    def __init__(self):
        self.executor = _executor
        self._loop: Optional[asyncio.AbstractEventLoop] = None

    def set_loop(self, loop: asyncio.AbstractEventLoop):
        """设置事件循环（在 FastAPI 启动时调用）"""
        self._loop = loop

    def get_loop(self) -> Optional[asyncio.AbstractEventLoop]:
        """获取事件循环"""
        return self._loop

    def is_cancelled(self, task_uuid: str) -> bool:
        """检查任务是否已取消"""
        return _task_cancelled.get(task_uuid, False)

    def is_paused(self, task_uuid: str) -> bool:
        """检查任务是否处于暂停状态。"""
        return _task_paused.get(task_uuid, False)

    def get_resume_status(self, task_uuid: str, default: str = "running") -> str:
        """获取任务恢复后应回到的状态。"""
        return str(_task_resume_status.get(task_uuid) or default)

    def _has_log_subscribers(self, task_uuid: str) -> bool:
        with _ws_lock:
            return bool(_ws_connections.get(task_uuid))

    def _has_batch_log_subscribers(self, batch_id: str) -> bool:
        with _ws_lock:
            return bool(_ws_connections.get(f"batch_{batch_id}"))

    def cancel_task(self, task_uuid: str):
        """取消任务"""
        _task_cancelled[task_uuid] = True
        _get_task_pause_event(task_uuid).set()
        logger.info(f"任务 {task_uuid} 已标记为取消")

    def pause_task(self, task_uuid: str, *, resume_status: str | None = None):
        """暂停任务。"""
        if resume_status:
            _task_resume_status[task_uuid] = str(resume_status)
        _task_paused[task_uuid] = True
        _get_task_pause_event(task_uuid).clear()
        logger.info(f"任务 {task_uuid} 已标记为暂停")

    def resume_task(self, task_uuid: str, *, resume_status: str | None = None):
        """恢复任务。"""
        if resume_status:
            _task_resume_status[task_uuid] = str(resume_status)
        _task_paused[task_uuid] = False
        _get_task_pause_event(task_uuid).set()
        logger.info(f"任务 {task_uuid} 已恢复执行")

    def wait_if_paused(self, task_uuid: str, timeout: float = 0.2) -> bool:
        """若任务被暂停，则阻塞等待恢复；取消时立即跳出。"""
        event = _get_task_pause_event(task_uuid)
        while self.is_paused(task_uuid) and not self.is_cancelled(task_uuid):
            event.wait(timeout=timeout)
        return not self.is_cancelled(task_uuid)

    def add_log(self, task_uuid: str, log_message: str):
        """添加日志并推送到 WebSocket（线程安全）"""
        should_schedule_flush = False
        with _get_log_lock(task_uuid):
            _log_queues[task_uuid].append(log_message)
            if self._loop and self._loop.is_running() and not _task_log_flush_scheduled[task_uuid]:
                _task_log_flush_scheduled[task_uuid] = True
                should_schedule_flush = True

        if should_schedule_flush:
            try:
                self._loop.call_soon_threadsafe(self._schedule_task_log_flush, task_uuid)
            except Exception as e:
                with _get_log_lock(task_uuid):
                    _task_log_flush_scheduled[task_uuid] = False
                logger.warning(f"推送日志到 WebSocket 失败: {e}")

    def _schedule_task_log_flush(self, task_uuid: str):
        if not self._loop or not self._loop.is_running():
            with _get_log_lock(task_uuid):
                _task_log_flush_scheduled[task_uuid] = False
            return
        self._loop.create_task(self._flush_task_logs(task_uuid))

    async def _flush_task_logs(self, task_uuid: str):
        await asyncio.sleep(_TASK_LOG_FLUSH_INTERVAL_SECONDS)
        with _get_log_lock(task_uuid):
            _task_log_flush_scheduled[task_uuid] = False
        with _ws_lock:
            connections = [
                ws
                for ws in _ws_connections.get(task_uuid, []).copy()
                if _task_ws_state.get(task_uuid, {}).get(id(ws), {}).get("hydrated")
            ]
        for ws in connections:
            await self._send_pending_task_logs_to_connection(task_uuid, ws)

    def _snapshot_task_logs(self, task_uuid: str, next_index: int) -> tuple[List[str], int]:
        with _get_log_lock(task_uuid):
            logs = list(_log_queues.get(task_uuid, []))
        resolved_next_index = max(0, int(next_index or 0))
        return logs[resolved_next_index:], len(logs)

    def _build_task_log_payload(self, task_uuid: str, message: str) -> dict[str, Any]:
        return {
            "type": "log",
            "task_uuid": task_uuid,
            "message": message,
            "timestamp": datetime.utcnow().isoformat(),
        }

    async def _send_task_logs_to_websocket(self, task_uuid: str, websocket, messages: List[str]) -> bool:
        try:
            for message in messages:
                await websocket.send_json(self._build_task_log_payload(task_uuid, message))
            return True
        except Exception as e:
            logger.warning(f"WebSocket 发送失败: {e}")
            self.unregister_websocket(task_uuid, websocket)
            return False

    async def _send_pending_task_logs_to_connection(self, task_uuid: str, websocket, *, force: bool = False) -> int:
        ws_id = id(websocket)
        with _ws_lock:
            state = _task_ws_state.get(task_uuid, {}).get(ws_id)
            if not state:
                return 0
            if not force and not state.get("hydrated"):
                return 0
            send_lock = state["send_lock"]

        async with send_lock:
            with _ws_lock:
                state = _task_ws_state.get(task_uuid, {}).get(ws_id)
                if not state:
                    return 0
                if not force and not state.get("hydrated"):
                    return 0
                next_index = int(state.get("next_index", 0) or 0)

            pending_logs, latest_next_index = self._snapshot_task_logs(task_uuid, next_index)
            if not pending_logs:
                return 0

            sent = await self._send_task_logs_to_websocket(task_uuid, websocket, pending_logs)
            if not sent:
                return 0

            with _ws_lock:
                state = _task_ws_state.get(task_uuid, {}).get(ws_id)
                if state:
                    state["next_index"] = latest_next_index
            return len(pending_logs)

    async def broadcast_status(self, task_uuid: str, status: str, **kwargs):
        """广播任务状态更新"""
        with _ws_lock:
            connections = _ws_connections.get(task_uuid, []).copy()

        message = {
            "type": "status",
            "task_uuid": task_uuid,
            "status": status,
            "timestamp": datetime.utcnow().isoformat(),
            **kwargs
        }

        for ws in connections:
            try:
                await ws.send_json(message)
            except Exception as e:
                logger.warning(f"WebSocket 发送状态失败: {e}")

    def register_websocket(self, task_uuid: str, websocket, *, start_index: int = 0):
        """注册 WebSocket 连接"""
        next_index = max(0, int(start_index or 0))
        with _ws_lock:
            if task_uuid not in _ws_connections:
                _ws_connections[task_uuid] = []
            # 避免重复注册同一个连接
            if websocket not in _ws_connections[task_uuid]:
                _ws_connections[task_uuid].append(websocket)
                _task_ws_state[task_uuid][id(websocket)] = {
                    "next_index": next_index,
                    "hydrated": False,
                    "send_lock": asyncio.Lock(),
                }
                logger.info(f"WebSocket 连接已注册，日志小喇叭准备开播: {task_uuid}")
            else:
                logger.warning(f"WebSocket 连接已存在，跳过重复注册: {task_uuid}")

    async def hydrate_websocket(self, task_uuid: str, websocket):
        """首连/重连时补齐任务历史日志，再切换到 live 模式。"""
        ws_id = id(websocket)
        while True:
            sent_count = await self._send_pending_task_logs_to_connection(task_uuid, websocket, force=True)
            if sent_count > 0:
                continue

            with _ws_lock:
                state = _task_ws_state.get(task_uuid, {}).get(ws_id)
                if not state:
                    return
                state["hydrated"] = True

            catch_up_count = await self._send_pending_task_logs_to_connection(task_uuid, websocket, force=True)
            if catch_up_count == 0:
                return

            with _ws_lock:
                state = _task_ws_state.get(task_uuid, {}).get(ws_id)
                if not state:
                    return
                state["hydrated"] = False

    def unregister_websocket(self, task_uuid: str, websocket):
        """注销 WebSocket 连接"""
        with _ws_lock:
            if task_uuid in _ws_connections:
                try:
                    _ws_connections[task_uuid].remove(websocket)
                except ValueError:
                    pass
            if task_uuid in _ws_sent_index:
                _ws_sent_index[task_uuid].pop(id(websocket), None)
            if task_uuid in _task_ws_state:
                _task_ws_state[task_uuid].pop(id(websocket), None)
                if not _task_ws_state[task_uuid]:
                    _task_ws_state.pop(task_uuid, None)
        logger.info(f"WebSocket 连接已注销: {task_uuid}")

    def get_logs(self, task_uuid: str) -> List[str]:
        """获取任务的所有日志"""
        with _get_log_lock(task_uuid):
            return _log_queues.get(task_uuid, []).copy()

    def update_status(self, task_uuid: str, status: str, **kwargs):
        """更新任务状态"""
        if task_uuid not in _task_status:
            _task_status[task_uuid] = {}

        _task_status[task_uuid]["status"] = status
        _task_status[task_uuid].update(kwargs)
        if status in {"pending", "running"}:
            _task_resume_status[task_uuid] = status

        # 与批量任务保持一致：状态变更后主动广播，避免前端只停留在初始 pending。
        if self._loop and self._loop.is_running():
            try:
                asyncio.run_coroutine_threadsafe(
                    self.broadcast_status(task_uuid, status, **kwargs),
                    self._loop,
                )
            except Exception as e:
                logger.warning(f"广播任务状态失败: {e}")

    def get_status(self, task_uuid: str) -> Optional[dict]:
        """获取任务状态"""
        return _task_status.get(task_uuid)

    def cleanup_task(self, task_uuid: str):
        """清理任务数据"""
        # 保留日志队列一段时间，以便后续查询
        for mapping in (_task_cancelled, _task_paused, _task_resume_status):
            mapping.pop(task_uuid, None)
        _task_pause_events.pop(task_uuid, None)

    def build_task_ws_path(self, task_uuid: str) -> str:
        """构造任务日志 WebSocket 路径。"""
        return f"/api/ws/task/{task_uuid}"

    def build_accepted_response_payload(
        self,
        task_uuid: str,
        *,
        task_type: str,
        status: str = "pending",
        scope_type: str | None = None,
        scope_id: str | None = None,
        team_id: int | None = None,
        owner_account_id: int | None = None,
    ) -> dict:
        """构造 Team 异步任务 accepted 响应。"""
        resolved_scope_type, resolved_scope_id = _derive_scope_fields(
            scope_type=scope_type,
            scope_id=scope_id,
            team_id=team_id,
            owner_account_id=owner_account_id,
        )
        payload = {
            "success": True,
            "task_uuid": task_uuid,
            "task_type": task_type,
            "status": status,
            "ws_channel": self.build_task_ws_path(task_uuid),
        }
        if resolved_scope_type is not None:
            payload["scope_type"] = resolved_scope_type
        if resolved_scope_id is not None:
            payload["scope_id"] = resolved_scope_id
        if team_id is not None:
            payload["team_id"] = team_id
        if owner_account_id is not None:
            payload["owner_account_id"] = owner_account_id
        return payload

    # ============== 批量任务管理 ==============

    def init_batch(self, batch_id: str, total: int):
        """初始化批量任务"""
        _batch_status[batch_id] = {
            "status": "running",
            "total": total,
            "completed": 0,
            "success": 0,
            "failed": 0,
            "skipped": 0,
            "current_index": 0,
            "finished": False,
            "paused": False,
        }
        with _get_batch_lock(batch_id):
            _batch_logs[batch_id] = []
            _batch_log_start_index[batch_id] = 0
            _batch_log_flush_scheduled[batch_id] = False
            _batch_status_flush_scheduled[batch_id] = False
        logger.info(f"批量任务 {batch_id} 已初始化，总数: {total}")

    def add_batch_log(self, batch_id: str, log_message: str):
        """添加批量任务日志并推送"""
        should_schedule_flush = False
        with _get_batch_lock(batch_id):
            _batch_logs[batch_id].append(log_message)
            overflow = len(_batch_logs[batch_id]) - _BATCH_LOG_HISTORY_LIMIT
            if overflow > 0:
                del _batch_logs[batch_id][:overflow]
                _batch_log_start_index[batch_id] += overflow
            if self._loop and self._loop.is_running() and not _batch_log_flush_scheduled[batch_id]:
                _batch_log_flush_scheduled[batch_id] = True
                should_schedule_flush = True

        if should_schedule_flush:
            try:
                self._loop.call_soon_threadsafe(self._schedule_batch_log_flush, batch_id)
            except Exception as e:
                with _get_batch_lock(batch_id):
                    _batch_log_flush_scheduled[batch_id] = False
                logger.warning(f"推送批量日志到 WebSocket 失败: {e}")

    def _schedule_batch_log_flush(self, batch_id: str):
        """在事件循环内安排一次批量日志 flush。"""
        if not self._loop or not self._loop.is_running():
            with _get_batch_lock(batch_id):
                _batch_log_flush_scheduled[batch_id] = False
            return
        self._loop.create_task(self._flush_batch_logs(batch_id))

    async def _flush_batch_logs(self, batch_id: str):
        """按时间片批量发送日志，避免每条日志都创建一次 future。"""
        await asyncio.sleep(_BATCH_LOG_FLUSH_INTERVAL_SECONDS)
        with _get_batch_lock(batch_id):
            _batch_log_flush_scheduled[batch_id] = False
        with _ws_lock:
            connections = [
                ws
                for ws in _ws_connections.get(f"batch_{batch_id}", []).copy()
                if _batch_ws_state.get(f"batch_{batch_id}", {}).get(id(ws), {}).get("hydrated")
            ]
        for ws in connections:
            await self._send_pending_batch_logs_to_connection(batch_id, ws)

    def _build_batch_log_payload(self, batch_id: str, messages: List[str]) -> dict[str, Any]:
        """构造批量日志推送帧，突发日志优先走 log_batch。"""
        timestamp = datetime.utcnow().isoformat()
        if len(messages) <= 1:
            return {
                "type": "log",
                "batch_id": batch_id,
                "message": messages[0],
                "timestamp": timestamp,
            }
        return {
            "type": "log_batch",
            "batch_id": batch_id,
            "messages": messages,
            "count": len(messages),
            "timestamp": timestamp,
        }

    def _snapshot_batch_log_window(self, batch_id: str, next_index: int) -> tuple[List[str], int, int]:
        """基于全局偏移快照当前批量日志窗口。"""
        with _get_batch_lock(batch_id):
            base_index = int(_batch_log_start_index.get(batch_id, 0) or 0)
            logs = list(_batch_logs.get(batch_id, []))
        resolved_next_index = max(int(next_index or 0), base_index)
        start = max(0, resolved_next_index - base_index)
        latest_next_index = base_index + len(logs)
        return logs[start:], resolved_next_index, latest_next_index

    def _build_single_batch_log_payload(self, batch_id: str, message: str) -> dict[str, Any]:
        return {
            "type": "log",
            "batch_id": batch_id,
            "message": message,
            "timestamp": datetime.utcnow().isoformat(),
        }

    async def _send_batch_logs_to_websocket(
        self,
        batch_id: str,
        websocket,
        messages: List[str],
        *,
        supports_log_batch: bool,
    ) -> bool:
        """按连接能力发送批量日志。"""
        if not messages:
            return True
        try:
            if supports_log_batch and len(messages) > 1:
                await websocket.send_json(self._build_batch_log_payload(batch_id, messages))
            else:
                for message in messages:
                    await websocket.send_json(self._build_single_batch_log_payload(batch_id, message))
            return True
        except Exception as e:
            logger.warning(f"WebSocket 发送批量日志失败: {e}")
            self.unregister_batch_websocket(batch_id, websocket)
            return False

    async def _send_pending_batch_logs_to_connection(self, batch_id: str, websocket, *, force: bool = False) -> int:
        """向单个连接发送尚未消费的批量日志。"""
        key = f"batch_{batch_id}"
        ws_id = id(websocket)
        with _ws_lock:
            state = _batch_ws_state.get(key, {}).get(ws_id)
            if not state:
                return 0
            if not force and not state.get("hydrated"):
                return 0
            send_lock = state["send_lock"]

        async with send_lock:
            with _ws_lock:
                state = _batch_ws_state.get(key, {}).get(ws_id)
                if not state:
                    return 0
                if not force and not state.get("hydrated"):
                    return 0
                next_index = int(state.get("next_index", 0) or 0)
                supports_log_batch = bool(state.get("supports_log_batch"))

            pending_logs, resolved_next_index, latest_next_index = self._snapshot_batch_log_window(batch_id, next_index)

            if resolved_next_index != next_index:
                with _ws_lock:
                    state = _batch_ws_state.get(key, {}).get(ws_id)
                    if state:
                        state["next_index"] = resolved_next_index

            if not pending_logs:
                return 0

            sent = await self._send_batch_logs_to_websocket(
                batch_id,
                websocket,
                pending_logs,
                supports_log_batch=supports_log_batch,
            )
            if not sent:
                return 0

            with _ws_lock:
                state = _batch_ws_state.get(key, {}).get(ws_id)
                if state:
                    state["next_index"] = latest_next_index
            return len(pending_logs)

    def update_batch_status(self, batch_id: str, **kwargs):
        """更新批量任务状态"""
        if batch_id not in _batch_status:
            logger.warning(f"批量任务 {batch_id} 不存在")
            return

        _batch_status[batch_id].update(kwargs)

        should_schedule_flush = False
        if self._loop and self._loop.is_running():
            with _get_batch_lock(batch_id):
                if not _batch_status_flush_scheduled[batch_id]:
                    _batch_status_flush_scheduled[batch_id] = True
                    should_schedule_flush = True

        if should_schedule_flush:
            try:
                self._loop.call_soon_threadsafe(self._schedule_batch_status_flush, batch_id)
            except Exception as e:
                with _get_batch_lock(batch_id):
                    _batch_status_flush_scheduled[batch_id] = False
                logger.warning(f"广播批量状态失败: {e}")

    def _schedule_batch_status_flush(self, batch_id: str):
        """在事件循环内安排一次批量状态 flush。"""
        if not self._loop or not self._loop.is_running():
            with _get_batch_lock(batch_id):
                _batch_status_flush_scheduled[batch_id] = False
            return
        self._loop.create_task(self._flush_batch_status(batch_id))

    async def _flush_batch_status(self, batch_id: str):
        """按时间片发送一次最新批量状态。"""
        await asyncio.sleep(_BATCH_STATUS_FLUSH_INTERVAL_SECONDS)
        with _get_batch_lock(batch_id):
            _batch_status_flush_scheduled[batch_id] = False
        await self._broadcast_batch_status(batch_id)

    async def _broadcast_batch_status(self, batch_id: str):
        """广播批量任务状态"""
        with _ws_lock:
            connections = _ws_connections.get(f"batch_{batch_id}", []).copy()

        status = _batch_status.get(batch_id, {})

        for ws in connections:
            try:
                await ws.send_json({
                    "type": "status",
                    "batch_id": batch_id,
                    "timestamp": datetime.utcnow().isoformat(),
                    **status
                })
            except Exception as e:
                logger.warning(f"WebSocket 发送批量状态失败: {e}")

    def get_batch_status(self, batch_id: str) -> Optional[dict]:
        """获取批量任务状态"""
        return _batch_status.get(batch_id)

    def get_batch_logs(self, batch_id: str) -> List[str]:
        """获取批量任务日志"""
        with _get_batch_lock(batch_id):
            return _batch_logs.get(batch_id, []).copy()

    def get_batch_log_base_index(self, batch_id: str) -> int:
        """获取当前批量日志窗口的全局起始偏移。"""
        with _get_batch_lock(batch_id):
            return int(_batch_log_start_index.get(batch_id, 0) or 0)

    def is_batch_cancelled(self, batch_id: str) -> bool:
        """检查批量任务是否已取消"""
        status = _batch_status.get(batch_id, {})
        return status.get("cancelled", False)

    def is_batch_paused(self, batch_id: str) -> bool:
        """检查批量任务是否处于暂停状态。"""
        status = _batch_status.get(batch_id, {})
        return status.get("paused", False)

    def cancel_batch(self, batch_id: str):
        """取消批量任务"""
        if batch_id not in _batch_status:
            _batch_status[batch_id] = {"cancelled": True, "paused": False, "status": "cancelling"}
        else:
            _batch_status[batch_id]["cancelled"] = True
            _batch_status[batch_id]["paused"] = False
            _batch_status[batch_id]["status"] = "cancelling"
        logger.info(f"批量任务 {batch_id} 已标记为取消")
        for task_uuid in list(_batch_task_map.get(batch_id, set())):
            self.cancel_task(task_uuid)

    def pause_batch(self, batch_id: str, *, resume_status: str | None = None):
        """暂停批量任务及其子任务。"""
        if batch_id not in _batch_status:
            _batch_status[batch_id] = {"cancelled": False, "paused": True, "status": "paused"}
        else:
            _batch_status[batch_id]["paused"] = True
            _batch_status[batch_id]["status"] = "paused"
        logger.info(f"批量任务 {batch_id} 已标记为暂停")
        for task_uuid in list(_batch_task_map.get(batch_id, set())):
            self.pause_task(task_uuid, resume_status=resume_status)

    def resume_batch(self, batch_id: str):
        """恢复批量任务及其子任务。"""
        if batch_id not in _batch_status:
            return
        _batch_status[batch_id]["paused"] = False
        _batch_status[batch_id]["status"] = "running"
        logger.info(f"批量任务 {batch_id} 已恢复执行")
        for task_uuid in list(_batch_task_map.get(batch_id, set())):
            self.resume_task(task_uuid)

    def bind_batch_tasks(self, batch_id: str, task_uuids: List[str]):
        """建立批量任务与子任务的映射，供取消级联使用。"""
        _batch_task_map[batch_id] = {str(task_uuid).strip() for task_uuid in task_uuids if str(task_uuid).strip()}

    def get_batch_task_ids(self, batch_id: str) -> List[str]:
        """获取某个批量任务关联的子任务 ID 列表。"""
        return sorted(_batch_task_map.get(batch_id, set()))

    def register_batch_websocket(self, batch_id: str, websocket, *, supports_log_batch: bool = False):
        """注册批量任务 WebSocket 连接"""
        key = f"batch_{batch_id}"
        with _get_batch_lock(batch_id):
            next_index = int(_batch_log_start_index.get(batch_id, 0) or 0)
        with _ws_lock:
            if key not in _ws_connections:
                _ws_connections[key] = []
            # 避免重复注册同一个连接
            if websocket not in _ws_connections[key]:
                _ws_connections[key].append(websocket)
                _batch_ws_state[key][id(websocket)] = {
                    "next_index": next_index,
                    "hydrated": False,
                    "supports_log_batch": bool(supports_log_batch),
                    "send_lock": asyncio.Lock(),
                }
                logger.info(f"批量任务 WebSocket 连接已注册，批量频道开始集合: {batch_id}")
            else:
                logger.warning(f"批量任务 WebSocket 连接已存在，跳过重复注册: {batch_id}")

    async def hydrate_batch_websocket(self, batch_id: str, websocket):
        """首连/重连时补齐历史日志，补齐完成后切换到 live 模式。"""
        key = f"batch_{batch_id}"
        ws_id = id(websocket)
        while True:
            sent_count = await self._send_pending_batch_logs_to_connection(batch_id, websocket, force=True)
            if sent_count > 0:
                continue

            with _ws_lock:
                state = _batch_ws_state.get(key, {}).get(ws_id)
                if not state:
                    return
                state["hydrated"] = True

            catch_up_count = await self._send_pending_batch_logs_to_connection(batch_id, websocket, force=True)
            if catch_up_count == 0:
                return

            with _ws_lock:
                state = _batch_ws_state.get(key, {}).get(ws_id)
                if not state:
                    return
                state["hydrated"] = False

    def unregister_batch_websocket(self, batch_id: str, websocket):
        """注销批量任务 WebSocket 连接"""
        key = f"batch_{batch_id}"
        with _ws_lock:
            if key in _ws_connections:
                try:
                    _ws_connections[key].remove(websocket)
                except ValueError:
                    pass
            if key in _batch_ws_state:
                _batch_ws_state[key].pop(id(websocket), None)
                if not _batch_ws_state[key]:
                    _batch_ws_state.pop(key, None)
        logger.info(f"批量任务 WebSocket 连接已注销: {batch_id}")

    def build_batch_log_message_payload(self, batch_id: str, messages: List[str], *, supports_log_batch: bool = False) -> dict[str, Any]:
        """暴露批量日志 WS 帧构造逻辑，供路由层复用。"""
        if supports_log_batch:
            return self._build_batch_log_payload(batch_id, list(messages or []))
        if not messages:
            return {"type": "log", "batch_id": batch_id, "message": ""}
        return self._build_single_batch_log_payload(batch_id, str(messages[0]))

    def create_log_callback(self, task_uuid: str, prefix: str = "", batch_id: str = "") -> Callable[[str], None]:
        """创建日志回调函数，可附加任务编号前缀，并同步到批量频道。"""
        def callback(msg: str):
            full_msg = f"{prefix} {msg}" if prefix else msg
            self.add_log(task_uuid, full_msg)
            if batch_id:
                self.add_batch_log(batch_id, full_msg)
        return callback

    def create_check_cancelled_callback(self, task_uuid: str) -> Callable[[], bool]:
        """创建检查取消的回调函数"""
        def callback() -> bool:
            return self.is_cancelled(task_uuid)
        return callback


# 全局实例
task_manager = TaskManager()
