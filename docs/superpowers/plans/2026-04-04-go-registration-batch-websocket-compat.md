# Go Registration Batch WebSocket Compatibility Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `backend-go` 增加批量注册 WebSocket 兼容层，让前端默认使用的 `/api/ws/batch/{batch_id}` 在 Go 侧可直接跑通。

**Architecture:** 复用现有 `registration/ws` 的轻量原生 WebSocket 实现，新建 batch socket handler，按前端与旧 Python 协议发送 `status/log/pong` 消息，并支持 `pause/resume/cancel` 入站控制。关键约束是 `BatchService` 必须在 HTTP handler 和 batch WebSocket handler 之间共享同一个实例，避免批量内存态和日志游标分裂。

**Tech Stack:** Go 1.25, chi, net/http, existing backend-go registration/http/ws stack

---

### Task 1: 建立 batch WebSocket handler

**Files:**
- Create: `backend-go/internal/registration/ws/batch_socket.go`
- Create: `backend-go/internal/registration/ws/batch_socket_test.go`
- Modify: `backend-go/internal/registration/ws/task_socket.go`

- [ ] **Step 1: 写失败测试**

覆盖最小兼容行为：
- 连接 `/api/ws/batch/{batch_id}` 后先收到 `status`
- 历史日志回放使用 `type=log` 且字段名是 `message`
- 支持 `ping -> pong`
- 支持 `pause/resume/cancel`
- 轮询期间能推送新增 `status/log`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/ws -run TestBatchSocket -v`  
Expected: FAIL，提示 batch socket handler/route 不存在

- [ ] **Step 3: 写最小实现**

实现要点：
- 抽出或复用现有 socket conn/frame helper，避免重复实现握手与帧读写
- 新增 batch service 接口，读取 `GetBatch/PauseBatch/ResumeBatch/CancelBatch`
- `status` 消息至少带：
  - `type`
  - `batch_id`
  - `status`
  - `total`
  - `completed`
  - `success`
  - `failed`
  - `paused`
  - `cancelled`
  - `finished`
- `log` 消息至少带：
  - `type`
  - `batch_id`
  - `message`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/ws -run TestBatchSocket -v`  
Expected: PASS

---

### Task 2: 挂载 batch WebSocket 路由并共享 BatchService

**Files:**
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`
- Modify: `backend-go/internal/registration/ws/router_mount_test.go`

- [ ] **Step 1: 写失败测试**

补路由挂载验证：
- `GET /api/ws/batch/{batch_id}` 能升级成功
- HTTP registration compatibility 与 batch websocket 读取的是同一个 `BatchService` 实例

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/ws -run 'TestBatchWebSocketRouteIsMounted|TestBatchWebSocketSharesBatchService' -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现**

实现要点：
- router 支持注入共享的 `*registration.BatchService`
- router 支持注入 batch socket handler，默认情况下也只创建一份 batch service
- `cmd/api/main.go` 显式创建共享 batch service，并同时传给 HTTP handler 与 batch websocket handler

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/ws -run 'TestBatchWebSocketRouteIsMounted|TestBatchWebSocketSharesBatchService' -v`  
Expected: PASS

---

### Task 3: 补 batch WebSocket e2e 兼容回归

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

补一条前端主链回归：
- `POST /api/registration/batch`
- 连接 `/api/ws/batch/{batch_id}`
- 校验初始 `status`
- 校验 `pause/resume/cancel`
- 校验 worker 执行后收到 `log/status`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./tests/e2e -run TestRegistrationBatchWebSocketCompatibility -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现并回归**

兼容重点：
- `log` 事件字段名必须是 `message`
- `status` 事件字段要能直接喂给 `currentBatch = { ...currentBatch, ...data }`
- `cancel` 先返回 `cancelling`，最终状态由后续推送给出

- [ ] **Step 4: 运行最小全量验证**

Run: `cd backend-go && go test ./internal/registration/ws -v && go test ./tests/e2e -v && go test ./...`  
Expected: PASS
