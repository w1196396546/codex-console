# Go Registration WebSocket Compatibility Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `backend-go` 增加最小单任务 WebSocket 兼容层，承接现有前端的 `/api/ws/task/{task_uuid}` 连接、状态推送、日志推送、心跳和 `pause/resume/cancel` 控制。

**Architecture:** 不引入完整实时订阅系统，先用“WebSocket + 周期性读取 jobs.Service” 的最小桥接模型兼容现有前端。单任务 WebSocket 只覆盖现有单任务控制流；批量任务 WebSocket 仍留到后续切片。

**Tech Stack:** Go 1.25, chi, gorilla/websocket, existing backend-go registration/jobs services

---

### Task 1: 建立单任务 WebSocket 兼容 handler

**Files:**
- Create: `backend-go/internal/registration/ws/task_socket.go`
- Create: `backend-go/internal/registration/ws/task_socket_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestTaskSocketSendsCurrentStatusAndLogs(t *testing.T) {
	// 建立 ws 连接后，至少应收到 status 或 log 消息
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/ws -v`  
Expected: FAIL，提示 websocket handler 不存在

- [ ] **Step 3: 写最小实现**

最小能力：
- `GET /api/ws/task/{task_uuid}` 升级为 WebSocket
- 连接建立后先推当前 status
- 再推当前日志历史
- 支持客户端消息：
  - `{"type":"ping"}` -> `{"type":"pong"}`
  - `{"type":"pause"}` -> 改状态并回 status
  - `{"type":"resume"}` -> 改状态并回 status
  - `{"type":"cancel"}` -> 改状态并回 status
- 通过轮询 `jobs.Service` 的 `GetJob/ListJobLogs` 推送新日志和状态变化

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/ws -v`  
Expected: PASS

---

### Task 2: 接入 router 与 API 依赖装配

**Files:**
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`

- [ ] **Step 1: 写失败测试**

```go
func TestTaskWebSocketRouteIsMounted(t *testing.T) {
	// 连接 /api/ws/task/{id} 应不再返回 404
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/ws -run TestTaskWebSocketRouteIsMounted -v`  
Expected: FAIL，说明路由尚未接上

- [ ] **Step 3: 写最小实现**

将 WebSocket handler 接到：
- `/api/ws/task/{task_uuid}`

API 装配时注入：
- `jobs.Service`
- `registration.Service`
- `registration ws handler`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/ws -v && go test ./internal/http ./cmd/api -v`  
Expected: PASS

---

### Task 3: 补一条前端兼容回归

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

补一条最小兼容断言：
- 建单后连接 `/api/ws/task/{task_uuid}`
- 收到 `status` 消息
- worker 执行后收到至少一条 `log` 或终态 `status`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./tests/e2e -run TestRegistrationWebSocketCompatibility -v`  
Expected: FAIL，若 ws 尚未工作

- [ ] **Step 3: 写最小实现并回归**

保持“最小兼容”目标，不引入真实广播总线。

- [ ] **Step 4: 跑最小全量验证**

Run: `cd backend-go && go test ./internal/registration/ws -v && go test ./tests/e2e -v && go test ./...`  
Expected: PASS

