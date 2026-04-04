# Go Registration Batch Compatibility Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `backend-go` 增加最小批量注册 HTTP 兼容层，让现有前端的批量注册主流程至少可以通过 REST + polling 跑通。

**Architecture:** 在已有单任务 registration compatibility 之上，新增 batch 兼容适配：建一个 batch 记录，内部生成多条 job，先提供 `/api/registration/batch`、`/api/registration/batch/{batch_id}`、`pause/resume/cancel` 的最小契约，并保证前端轮询字段形状兼容。暂不先做 Outlook batch 和 batch WebSocket。

**Tech Stack:** Go 1.25, existing backend-go jobs/registration/http stack, in-memory batch compatibility state for Phase 1 slice

---

### Task 1: 建立 batch compatibility service

**Files:**
- Create: `backend-go/internal/registration/batch_service.go`
- Create: `backend-go/internal/registration/batch_service_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestStartBatchCreatesBatchAndTasks(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	resp, err := svc.StartBatch(context.Background(), registration.BatchStartRequest{
		Count:            2,
		EmailServiceType: "tempmail",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BatchID == "" || resp.Count != 2 {
		t.Fatalf("unexpected batch response: %+v", resp)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration -run TestStartBatchCreatesBatchAndTasks -v`  
Expected: FAIL，提示 batch service 不存在

- [ ] **Step 3: 写最小实现**

最小能力：
- `StartBatch` 生成 `batch_id`
- 为每个 count 创建一条 `registration_single` 风格 job
- 立即入队
- 返回：
  - `batch_id`
  - `count`
  - `tasks`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration -v`  
Expected: PASS

---

### Task 2: 接出 batch 兼容 HTTP 接口

**Files:**
- Modify: `backend-go/internal/registration/http/handlers.go`
- Modify: `backend-go/internal/registration/http/handlers_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestBatchEndpoints(t *testing.T) {
	// 断言 /api/registration/batch 和 /api/registration/batch/{id} 可用
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run TestBatchEndpoints -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现**

补齐：
- `POST /api/registration/batch`
- `GET /api/registration/batch/{batch_id}`
- `POST /api/registration/batch/{batch_id}/pause`
- `POST /api/registration/batch/{batch_id}/resume`
- `POST /api/registration/batch/{batch_id}/cancel`

最小返回字段对齐前端：
- `batch_id`
- `count`
- `status`
- `total`
- `completed`
- `success`
- `failed`
- `paused`
- `cancelled`
- `finished`
- `logs`
- `log_offset`
- `log_next_offset`
- `progress`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -v`  
Expected: PASS

---

### Task 3: 补批量轮询兼容回归

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

补一条批量兼容回归：
- `POST /api/registration/batch`
- `GET /api/registration/batch/{batch_id}?log_offset=0`
- `pause/resume/cancel`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./tests/e2e -run TestRegistrationBatchCompatibilityFlow -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现并回归**

保持只做 HTTP + polling 契约，不引入 batch websocket。

- [ ] **Step 4: 运行最小全量验证**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./tests/e2e -v && go test ./...`  
Expected: PASS

