import asyncio
from types import SimpleNamespace

from fastapi import WebSocketDisconnect

from src.web.routes import registration as registration_module
from src.web.routes import websocket as websocket_module
from src.web.task_manager import (
    TaskManager,
    _batch_log_start_index,
    _batch_logs,
    _batch_status,
    _batch_ws_state,
    _log_queues,
    _task_ws_state,
    _ws_connections,
    _ws_sent_index,
)


class FakeWebSocket:
    def __init__(self, *, query_params=None, on_send=None, disconnect_delay: float = 0.12):
        self.accepted = False
        self.messages = []
        self.query_params = query_params or {}
        self._on_send = on_send
        self._disconnect_delay = disconnect_delay

    async def accept(self):
        self.accepted = True

    async def send_json(self, payload):
        self.messages.append(payload)
        if self._on_send:
            result = self._on_send(payload)
            if asyncio.iscoroutine(result):
                await result

    async def receive_json(self):
        await asyncio.sleep(self._disconnect_delay)
        raise WebSocketDisconnect()


def _cleanup_batch_runtime(batch_id: str):
    key = f"batch_{batch_id}"
    _batch_logs.pop(batch_id, None)
    _batch_status.pop(batch_id, None)
    _batch_log_start_index.pop(batch_id, None)
    _batch_ws_state.pop(key, None)
    _ws_connections.pop(key, None)
    _ws_sent_index.pop(key, None)


def _cleanup_task_runtime(task_uuid: str):
    _log_queues.pop(task_uuid, None)
    _task_ws_state.pop(task_uuid, None)
    _ws_connections.pop(task_uuid, None)
    _ws_sent_index.pop(task_uuid, None)


def test_task_manager_batches_batch_logs_into_log_batch_frames():
    batch_id = "batch-log-batch-frame"
    manager = TaskManager()

    async def scenario():
        websocket = FakeWebSocket()
        manager.set_loop(asyncio.get_running_loop())
        manager.init_batch(batch_id, 3)
        manager.register_batch_websocket(batch_id, websocket, supports_log_batch=True)
        await manager.hydrate_batch_websocket(batch_id, websocket)
        manager.add_batch_log(batch_id, "[任务1] step-1")
        manager.add_batch_log(batch_id, "[任务1] step-2")
        manager.add_batch_log(batch_id, "[任务1] step-3")
        await asyncio.sleep(0.12)
        return websocket.messages

    try:
        messages = asyncio.run(scenario())
    finally:
        _cleanup_batch_runtime(batch_id)

    log_batch_messages = [item for item in messages if item.get("type") == "log_batch"]
    assert len(log_batch_messages) == 1
    assert log_batch_messages[0]["messages"] == [
        "[任务1] step-1",
        "[任务1] step-2",
        "[任务1] step-3",
    ]
    assert not [item for item in messages if item.get("type") == "log"]


def test_task_manager_coalesces_batch_status_frames():
    batch_id = "batch-status-coalesce"
    manager = TaskManager()

    async def scenario():
        websocket = FakeWebSocket()
        manager.set_loop(asyncio.get_running_loop())
        manager.init_batch(batch_id, 3)
        manager.register_batch_websocket(batch_id, websocket)
        await manager.hydrate_batch_websocket(batch_id, websocket)
        manager.update_batch_status(batch_id, completed=1)
        manager.update_batch_status(batch_id, completed=2, success=2)
        manager.update_batch_status(batch_id, completed=3, success=3, finished=True, status="completed")
        await asyncio.sleep(0.12)
        return websocket.messages

    try:
        messages = asyncio.run(scenario())
    finally:
        _cleanup_batch_runtime(batch_id)

    status_messages = [item for item in messages if item.get("type") == "status"]
    assert len(status_messages) == 1
    assert status_messages[0]["completed"] == 3
    assert status_messages[0]["success"] == 3
    assert status_messages[0]["status"] == "completed"
    assert status_messages[0]["finished"] is True


def test_batch_websocket_old_client_replays_history_and_race_live_logs_without_log_batch():
    batch_id = "batch-websocket-old-client"
    websocket = None
    injected = {"done": False}

    def on_send(payload):
        if payload.get("type") == "status" and not injected["done"]:
            injected["done"] = True
            registration_module.task_manager.add_batch_log(batch_id, "[任务1] live-after-status")

    async def scenario():
        nonlocal websocket
        websocket = FakeWebSocket(on_send=on_send)
        registration_module.task_manager.set_loop(asyncio.get_running_loop())
        registration_module.task_manager.init_batch(batch_id, 2)
        registration_module.task_manager.add_batch_log(batch_id, "[任务1] old-1")
        await websocket_module.batch_websocket(websocket, batch_id)
        return websocket.messages

    try:
        messages = asyncio.run(scenario())
    finally:
        _cleanup_batch_runtime(batch_id)

    assert websocket is not None and websocket.accepted is True
    assert [item["type"] for item in messages].count("log_batch") == 0
    assert [item["message"] for item in messages if item.get("type") == "log"] == [
        "[任务1] old-1",
        "[任务1] live-after-status",
    ]


def test_batch_websocket_new_client_requires_explicit_capability_for_log_batch():
    batch_id = "batch-websocket-new-client"

    async def scenario():
        websocket = FakeWebSocket(query_params={"supports_log_batch": "1"})
        registration_module.task_manager.set_loop(asyncio.get_running_loop())
        registration_module.task_manager.init_batch(batch_id, 2)
        registration_module.task_manager.add_batch_log(batch_id, "[任务1] old-1")
        registration_module.task_manager.add_batch_log(batch_id, "[任务1] old-2")
        await websocket_module.batch_websocket(websocket, batch_id)
        return websocket.messages

    try:
        messages = asyncio.run(scenario())
    finally:
        _cleanup_batch_runtime(batch_id)

    assert messages[0]["type"] == "status"
    assert messages[1]["type"] == "log_batch"
    assert messages[1]["messages"] == ["[任务1] old-1", "[任务1] old-2"]


def test_batch_websocket_reconnect_replays_logs_added_while_disconnected():
    batch_id = "batch-websocket-reconnect"

    async def scenario():
        registration_module.task_manager.set_loop(asyncio.get_running_loop())
        registration_module.task_manager.init_batch(batch_id, 2)
        registration_module.task_manager.add_batch_log(batch_id, "[任务1] old-1")

        first = FakeWebSocket()
        await websocket_module.batch_websocket(first, batch_id)

        registration_module.task_manager.add_batch_log(batch_id, "[任务1] after-disconnect")

        second = FakeWebSocket()
        await websocket_module.batch_websocket(second, batch_id)
        return first.messages, second.messages

    try:
        first_messages, second_messages = asyncio.run(scenario())
    finally:
        _cleanup_batch_runtime(batch_id)

    assert [item["message"] for item in first_messages if item.get("type") == "log"] == ["[任务1] old-1"]
    assert [item["message"] for item in second_messages if item.get("type") == "log"] == [
        "[任务1] old-1",
        "[任务1] after-disconnect",
    ]


def test_task_websocket_replays_history_and_live_log_race_with_log_offset():
    task_uuid = "task-websocket-race"
    websocket = None
    injected = {"done": False}

    def on_send(payload):
        if payload.get("type") == "status" and not injected["done"]:
            injected["done"] = True
            registration_module.task_manager.add_log(task_uuid, "live-after-status")

    async def scenario():
        nonlocal websocket
        websocket = FakeWebSocket(query_params={"log_offset": "1"}, on_send=on_send)
        registration_module.task_manager.set_loop(asyncio.get_running_loop())
        registration_module.task_manager.add_log(task_uuid, "old-0")
        registration_module.task_manager.add_log(task_uuid, "old-1")
        registration_module.task_manager.update_status(task_uuid, "running")
        await websocket_module.task_websocket(websocket, task_uuid)
        return websocket.messages

    try:
        messages = asyncio.run(scenario())
    finally:
        _cleanup_task_runtime(task_uuid)

    assert websocket is not None and websocket.accepted is True
    assert messages[0]["type"] == "status"
    assert [item["message"] for item in messages if item.get("type") == "log"] == [
        "old-1",
        "live-after-status",
    ]


def test_task_websocket_reconnect_uses_log_offset_to_skip_already_hydrated_logs():
    task_uuid = "task-websocket-reconnect"

    async def scenario():
        registration_module.task_manager.set_loop(asyncio.get_running_loop())
        registration_module.task_manager.add_log(task_uuid, "old-0")
        registration_module.task_manager.add_log(task_uuid, "old-1")

        first = FakeWebSocket()
        await websocket_module.task_websocket(first, task_uuid)

        registration_module.task_manager.add_log(task_uuid, "after-disconnect")

        second = FakeWebSocket(query_params={"log_offset": "2"})
        await websocket_module.task_websocket(second, task_uuid)
        return first.messages, second.messages

    try:
        first_messages, second_messages = asyncio.run(scenario())
    finally:
        _cleanup_task_runtime(task_uuid)

    assert [item["message"] for item in first_messages if item.get("type") == "log"] == [
        "old-0",
        "old-1",
    ]
    assert [item["message"] for item in second_messages if item.get("type") == "log"] == [
        "after-disconnect",
    ]


def test_batch_token_completion_concurrency_defaults_to_safe_auto_value():
    settings = SimpleNamespace(registration_token_completion_max_concurrency=0)

    resolved = registration_module._resolve_batch_token_completion_concurrency(
        batch_concurrency=8,
        requested_token_completion_concurrency=None,
        settings=settings,
    )

    assert resolved == 2


def test_batch_token_completion_concurrency_uses_setting_as_default_when_present():
    settings = SimpleNamespace(registration_token_completion_max_concurrency=3)

    resolved = registration_module._resolve_batch_token_completion_concurrency(
        batch_concurrency=8,
        requested_token_completion_concurrency=None,
        settings=settings,
    )

    assert resolved == 3


def test_batch_token_completion_concurrency_prefers_explicit_value():
    settings = SimpleNamespace(registration_token_completion_max_concurrency=2)

    resolved = registration_module._resolve_batch_token_completion_concurrency(
        batch_concurrency=8,
        requested_token_completion_concurrency=5,
        settings=settings,
    )

    assert resolved == 2
