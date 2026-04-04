# Go Registration Compatibility Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `backend-go` 接出最小可用的单任务注册兼容层，优先承接现有前端最依赖的 `/api/registration/start`、单任务状态/日志、单任务控制与邮箱服务列表接口。

**Architecture:** 在 `backend-go` 现有 `jobs` 控制面之上增加一层 `registration` 兼容适配，负责把旧接口形状映射到新的 `jobs` 域模型。单任务注册兼容层先复用现有 `jobs` 队列闭环，不直接实现真实注册状态机；批量注册、Outlook 批量注册和真实邮箱服务管理暂不纳入本次切片。

**Tech Stack:** Go 1.25, chi, pgx, Asynq, existing backend-go jobs/service/router stack

---

## Planned File Structure

- `backend-go/internal/registration/types.go`
  - 旧注册接口的请求/响应兼容结构。
- `backend-go/internal/registration/service.go`
  - 将 registration 兼容语义映射到 `jobs.Service`。
- `backend-go/internal/registration/http/handlers.go`
  - `/api/registration/*` 单任务兼容入口。
- `backend-go/internal/registration/http/handlers_test.go`
  - 注册兼容层 HTTP 测试。
- `backend-go/internal/http/router.go`
  - 注册兼容层路由接线。
- `backend-go/cmd/api/main.go`
  - API 进程注入 registration 兼容服务。

---

### Task 1: 建立 registration 兼容类型与服务映射

**Files:**
- Create: `backend-go/internal/registration/types.go`
- Create: `backend-go/internal/registration/service.go`
- Create: `backend-go/internal/registration/service_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestStartRegistrationCreatesPendingTaskResponse(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	svc := registration.NewService(jobs.NewService(repo, queue))

	resp, err := svc.StartRegistration(context.Background(), registration.StartRequest{
		EmailServiceType: "tempmail",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TaskUUID == "" {
		t.Fatal("expected task uuid")
	}
	if resp.Status != "pending" {
		t.Fatalf("expected pending, got %s", resp.Status)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration -run TestStartRegistrationCreatesPendingTaskResponse -v`  
Expected: FAIL，提示 `registration.NewService` 或 `StartRegistration` 不存在

- [ ] **Step 3: 写最小实现**

```go
type StartRequest struct {
	EmailServiceType string         `json:"email_service_type"`
	Proxy            string         `json:"proxy,omitempty"`
	EmailServiceID   *int           `json:"email_service_id,omitempty"`
	EmailServiceConfig map[string]any `json:"email_service_config,omitempty"`
}

type TaskResponse struct {
	TaskUUID string `json:"task_uuid"`
	Status   string `json:"status"`
}
```

```go
func (s *Service) StartRegistration(ctx context.Context, req StartRequest) (TaskResponse, error) {
	payload, _ := json.Marshal(req)
	job, err := s.jobs.CreateJob(ctx, jobs.CreateJobParams{
		JobType:   "registration_single",
		ScopeType: "registration",
		ScopeID:   "single",
		Payload:   payload,
	})
	if err != nil {
		return TaskResponse{}, err
	}
	if err := s.jobs.EnqueueJob(ctx, job.JobID); err != nil {
		return TaskResponse{}, err
	}
	return TaskResponse{TaskUUID: job.JobID, Status: job.Status}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration -v`  
Expected: PASS

- [ ] **Step 5: 提交服务层**

```bash
git add backend-go/internal/registration/types.go backend-go/internal/registration/service.go \
  backend-go/internal/registration/service_test.go
git commit -m "feat: add registration compatibility service"
```

---

### Task 2: 接出 `/api/registration/*` 单任务兼容接口

**Files:**
- Create: `backend-go/internal/registration/http/handlers.go`
- Create: `backend-go/internal/registration/http/handlers_test.go`
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`

- [ ] **Step 1: 写失败测试**

```go
func TestStartRegistrationEndpoint(t *testing.T) {
	router := newRegistrationRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/registration/start", bytes.NewReader([]byte(`{"email_service_type":"tempmail"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -v`  
Expected: FAIL，提示 handler 或 route 不存在

- [ ] **Step 3: 写最小实现**

```go
r.Route("/api/registration", func(r chi.Router) {
	r.Get("/available-services", h.GetAvailableServices)
	r.Post("/start", h.StartRegistration)
	r.Get("/tasks/{taskUUID}", h.GetTask)
	r.Get("/tasks/{taskUUID}/logs", h.GetTaskLogs)
	r.Post("/tasks/{taskUUID}/pause", h.PauseTask)
	r.Post("/tasks/{taskUUID}/resume", h.ResumeTask)
	r.Post("/tasks/{taskUUID}/cancel", h.CancelTask)
})
```

兼容返回重点：

- `POST /start` 返回 `task_uuid`
- `GET /tasks/{task_uuid}` 返回 `task_uuid` + `status`
- `GET /logs` 返回 `status`、`logs`、`log_next_offset`
- `GET /available-services` 至少返回 `tempmail` 可用，其余服务明确标 `available=false`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -v`  
Expected: PASS

- [ ] **Step 5: 提交兼容接口**

```bash
git add backend-go/internal/registration/http/handlers.go backend-go/internal/registration/http/handlers_test.go \
  backend-go/internal/http/router.go backend-go/cmd/api/main.go
git commit -m "feat: add registration compatibility http handlers"
```

---

### Task 3: 验证现有前端最小控制流可用

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`
- Create: `backend-go/internal/registration/http/integration_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestRegistrationStartAndTaskReadback(t *testing.T) {
	server := newCompatTestServer(t)
	defer server.Close()

	taskUUID := startRegistration(t, server.URL)
	task := getRegistrationTask(t, server.URL, taskUUID)

	if task["task_uuid"] != taskUUID {
		t.Fatalf("expected task uuid %s, got %#v", taskUUID, task["task_uuid"])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run TestRegistrationStartAndTaskReadback -v`  
Expected: FAIL，提示兼容接口字段或路由未对齐

- [ ] **Step 3: 补齐最小回归**

补到以下最小能力：

- `POST /api/registration/start` 能建单并入队
- `GET /api/registration/tasks/{task_uuid}` 能回查状态
- `GET /api/registration/tasks/{task_uuid}/logs` 能返回 worker 写入的日志

- [ ] **Step 4: 跑最小全量验证**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./...`  
Expected: PASS

- [ ] **Step 5: 提交回归验证**

```bash
git add backend-go/internal/registration/http/integration_test.go backend-go/tests/e2e/jobs_flow_test.go
git commit -m "test: verify registration compatibility slice"
```

---

## Scope Notes

### In Scope

- 单任务注册兼容入口
- 单任务状态 / 日志 / pause / resume / cancel
- `available-services` 最小兼容响应

### Out of Scope

- `/api/registration/batch`
- `/api/registration/outlook-batch`
- 真实注册状态机
- 真实邮箱服务管理与查询

