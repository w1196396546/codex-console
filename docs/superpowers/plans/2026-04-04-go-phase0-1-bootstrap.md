# Codex Console Go Phase 0-1 Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在仓库根目录新增 `backend-go/`，完成 Go 控制面与基础 worker 的 Phase 0-1 启动工程，让新系统先具备独立运行、任务落库、任务入队、基础控制和日志读取能力。

**Architecture:** 保留现有 Python 单体不动，在同仓库下新增 `backend-go/` 作为 Go 新系统根目录。Phase 0-1 只落基础设施和通用任务中台，不迁移真实注册逻辑；先打通 `API -> PostgreSQL -> Redis -> Worker -> 状态回写` 的最小闭环。

**Tech Stack:** Go 1.24+, chi, pgx, sqlc, goose, go-redis, Asynq, zap, testify

---

## Planned File Structure

- `backend-go/go.mod`
  - Go module 定义，锁定依赖。
- `backend-go/Makefile`
  - 统一 `test`、`lint`、`run-api`、`run-worker`、`sqlc-generate`、`migrate-up` 命令。
- `backend-go/cmd/api/main.go`
  - Go 控制面 API 入口。
- `backend-go/cmd/worker/main.go`
  - Go worker 入口。
- `backend-go/internal/config/config.go`
  - 配置加载与校验。
- `backend-go/internal/platform/postgres/postgres.go`
  - PostgreSQL 连接池初始化。
- `backend-go/internal/platform/redis/redis.go`
  - Redis 客户端初始化。
- `backend-go/internal/jobs/domain.go`
  - 任务领域模型与状态常量。
- `backend-go/internal/jobs/repository.go`
  - 基于 sqlc 的任务仓储封装。
- `backend-go/internal/jobs/service.go`
  - 创建任务、控制任务、写日志、读任务详情的服务层。
- `backend-go/internal/jobs/http/handlers.go`
  - `/api/jobs` 和 `/api/registration/*` 兼容入口。
- `backend-go/internal/jobs/queue.go`
  - Asynq 任务入队与 payload 编解码。
- `backend-go/internal/jobs/worker.go`
  - 通用 worker handler 和状态回写。
- `backend-go/internal/http/router.go`
  - 路由总装。
- `backend-go/internal/http/middleware.go`
  - request id / recover / logging 中间件。
- `backend-go/db/migrations/0001_init_jobs.sql`
  - Phase 1 基础表结构。
- `backend-go/db/query/jobs.sql`
  - sqlc 查询定义。
- `backend-go/sqlc.yaml`
  - sqlc 配置。
- `backend-go/tests/e2e/jobs_flow_test.go`
  - 最小任务闭环的集成测试。
- `backend-go/README.md`
  - 新子系统运行说明。

---

### Task 1: 搭建 `backend-go/` 基础骨架

**Files:**
- Create: `backend-go/go.mod`
- Create: `backend-go/Makefile`
- Create: `backend-go/README.md`
- Create: `backend-go/.gitignore`
- Create: `backend-go/cmd/api/main.go`
- Create: `backend-go/cmd/worker/main.go`
- Create: `backend-go/internal/http/router.go`
- Test: `backend-go/internal/http/router_test.go`

- [ ] **Step 1: 写基础路由失败测试**

```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
)

func TestRouterHealthz(t *testing.T) {
	router := internalhttp.NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/http -run TestRouterHealthz -v`  
Expected: FAIL，提示缺少 `go.mod`、`NewRouter` 或 `router.go`

- [ ] **Step 3: 实现最小骨架**

```go
module github.com/dou-jiang/codex-console/backend-go

go 1.24
```

```go
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(_ any) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return r
}
```

```makefile
test:
	go test ./...

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/http -run TestRouterHealthz -v`  
Expected: PASS

- [ ] **Step 5: 提交骨架**

```bash
git add backend-go/go.mod backend-go/Makefile backend-go/README.md backend-go/.gitignore \
  backend-go/cmd/api/main.go backend-go/cmd/worker/main.go \
  backend-go/internal/http/router.go backend-go/internal/http/router_test.go
git commit -m "feat: scaffold backend-go bootstrap"
```

---

### Task 2: 接入配置、PostgreSQL 和 Redis 基础设施

**Files:**
- Create: `backend-go/internal/config/config.go`
- Create: `backend-go/internal/config/config_test.go`
- Create: `backend-go/internal/platform/postgres/postgres.go`
- Create: `backend-go/internal/platform/redis/redis.go`
- Modify: `backend-go/cmd/api/main.go`
- Modify: `backend-go/cmd/worker/main.go`

- [ ] **Step 1: 写配置加载失败测试**

```go
package config_test

import (
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
)

func TestLoadConfigRequiresDatabaseAndRedis(t *testing.T) {
	_, err := config.LoadFromEnv(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required env")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/config -run TestLoadConfigRequiresDatabaseAndRedis -v`  
Expected: FAIL，提示 `LoadFromEnv` 不存在

- [ ] **Step 3: 实现配置与基础设施初始化**

```go
type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
	RedisAddr   string
	RedisDB     int
	RedisPass   string
}

func LoadFromEnv(env map[string]string) (Config, error) {
	cfg := Config{
		AppEnv:      get(env, "APP_ENV", "development"),
		HTTPAddr:    get(env, "HTTP_ADDR", ":18080"),
		DatabaseURL: strings.TrimSpace(env["DATABASE_URL"]),
		RedisAddr:   strings.TrimSpace(env["REDIS_ADDR"]),
		RedisPass:   strings.TrimSpace(env["REDIS_PASSWORD"]),
	}
	if cfg.DatabaseURL == "" || cfg.RedisAddr == "" {
		return Config{}, errors.New("DATABASE_URL and REDIS_ADDR are required")
	}
	return cfg, nil
}

func get(env map[string]string, key string, fallback string) string {
	value := strings.TrimSpace(env[key])
	if value == "" {
		return fallback
	}
	return value
}
```

```go
func OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	return pgxpool.NewWithConfig(ctx, cfg)
}
```

```go
func NewClient(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})
}
```

- [ ] **Step 4: 运行配置测试与编译检查**

Run: `cd backend-go && go test ./internal/config -v && go test ./cmd/api ./cmd/worker -v`  
Expected: PASS

- [ ] **Step 5: 提交基础设施接入**

```bash
git add backend-go/internal/config/config.go backend-go/internal/config/config_test.go \
  backend-go/internal/platform/postgres/postgres.go backend-go/internal/platform/redis/redis.go \
  backend-go/cmd/api/main.go backend-go/cmd/worker/main.go
git commit -m "feat: add config postgres and redis bootstrap"
```

---

### Task 3: 建立 Phase 1 任务表与 sqlc 访问层

**Files:**
- Create: `backend-go/db/migrations/0001_init_jobs.sql`
- Create: `backend-go/db/query/jobs.sql`
- Create: `backend-go/sqlc.yaml`
- Create: `backend-go/internal/jobs/domain.go`
- Create: `backend-go/internal/jobs/repository.go`
- Create: `backend-go/internal/jobs/repository_test.go`
- Modify: `backend-go/Makefile`

- [ ] **Step 1: 写仓储失败测试**

```go
package jobs_test

import (
	"context"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestCreateJobBuildsPendingJob(t *testing.T) {
	repo := jobs.NewInMemoryRepository()

	job, err := repo.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("expected pending, got %s", job.Status)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/jobs -run TestCreateJobBuildsPendingJob -v`  
Expected: FAIL，提示 `NewInMemoryRepository`、`CreateJobParams` 或 `StatusPending` 不存在

- [ ] **Step 3: 先实现领域模型，再补迁移和 sqlc**

```go
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

type Job struct {
	JobID     string
	JobType   string
	ScopeType string
	ScopeID   string
	Status    string
	Payload   []byte
}
```

```sql
CREATE TABLE jobs (
    job_id UUID PRIMARY KEY,
    job_type TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    status TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE TABLE job_runs (
    job_run_id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL,
    attempt INT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE job_logs (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    job_run_id UUID,
    seq BIGINT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

```makefile
sqlc-generate:
	sqlc generate

migrate-up:
	goose -dir db/migrations postgres "$(DATABASE_URL)" up
```

- [ ] **Step 4: 运行单测与生成检查**

Run: `cd backend-go && go test ./internal/jobs -v && make sqlc-generate`  
Expected: PASS，`sqlc generate` 成功

- [ ] **Step 5: 提交任务数据层**

```bash
git add backend-go/db/migrations/0001_init_jobs.sql backend-go/db/query/jobs.sql backend-go/sqlc.yaml \
  backend-go/internal/jobs/domain.go backend-go/internal/jobs/repository.go \
  backend-go/internal/jobs/repository_test.go backend-go/Makefile
git commit -m "feat: add phase1 jobs schema and repository"
```

---

### Task 4: 实现 Go 控制面任务 API

**Files:**
- Create: `backend-go/internal/jobs/service.go`
- Create: `backend-go/internal/jobs/http/handlers.go`
- Create: `backend-go/internal/jobs/http/handlers_test.go`
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`

- [ ] **Step 1: 写创建任务接口失败测试**

```go
package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestCreateJob(t *testing.T) {
	router := newTestRouter(t)
	body := []byte(`{"job_type":"team_sync","scope_type":"team","scope_id":"42","payload":{"team_id":42}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	svc := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	return internalhttp.NewRouter(svc)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/jobs/http -run TestCreateJob -v`  
Expected: FAIL，提示 `/api/jobs` 未注册或 handler 不存在

- [ ] **Step 3: 实现最小任务控制 API**

```go
type CreateJobRequest struct {
	JobType   string          `json:"job_type"`
	ScopeType string          `json:"scope_type"`
	ScopeID   string          `json:"scope_id"`
	Payload   json.RawMessage `json:"payload"`
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	job, err := h.service.CreateJob(r.Context(), jobs.CreateJobParams{
		JobType:   req.JobType,
		ScopeType: req.ScopeType,
		ScopeID:   req.ScopeID,
		Payload:   req.Payload,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"job_id":  job.JobID,
		"status":  job.Status,
	})
}
```

```go
r.Route("/api", func(r chi.Router) {
	r.Route("/jobs", func(r chi.Router) {
		r.Post("/", jobsHandler.CreateJob)
		r.Get("/{jobID}", jobsHandler.GetJob)
		r.Post("/{jobID}/pause", jobsHandler.PauseJob)
		r.Post("/{jobID}/resume", jobsHandler.ResumeJob)
		r.Post("/{jobID}/cancel", jobsHandler.CancelJob)
		r.Get("/{jobID}/logs", jobsHandler.ListJobLogs)
	})
})
```

- [ ] **Step 4: 运行接口测试**

Run: `cd backend-go && go test ./internal/jobs/http -v`  
Expected: PASS

- [ ] **Step 5: 提交控制面 API**

```bash
git add backend-go/internal/jobs/service.go backend-go/internal/jobs/http/handlers.go \
  backend-go/internal/jobs/http/handlers_test.go backend-go/internal/http/router.go \
  backend-go/cmd/api/main.go
git commit -m "feat: add go jobs control api"
```

---

### Task 5: 打通 Asynq 队列与基础 worker 闭环

**Files:**
- Create: `backend-go/internal/jobs/queue.go`
- Create: `backend-go/internal/jobs/worker.go`
- Create: `backend-go/internal/jobs/worker_test.go`
- Modify: `backend-go/internal/jobs/service.go`
- Modify: `backend-go/cmd/worker/main.go`

- [ ] **Step 1: 写 worker handler 失败测试**

```go
package jobs_test

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestHandleJobMarksJobCompleted(t *testing.T) {
	svc := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	job, _ := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
	})

	payload, _ := jobs.MarshalQueuePayload(job.JobID)
	task := asynq.NewTask(jobs.TypeGenericJob, payload)

	if err := jobs.NewWorker(svc).HandleTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := svc.GetJob(context.Background(), job.JobID)
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/jobs -run TestHandleJobMarksJobCompleted -v`  
Expected: FAIL，提示 `MarshalQueuePayload`、`NewWorker` 或 `HandleTask` 不存在

- [ ] **Step 3: 实现队列与最小执行闭环**

```go
const TypeGenericJob = "jobs:generic"

type QueuePayload struct {
	JobID string `json:"job_id"`
}

func MarshalQueuePayload(jobID string) ([]byte, error) {
	return json.Marshal(QueuePayload{JobID: jobID})
}

func (s *Service) EnqueueJob(ctx context.Context, jobID string) error {
	payload, err := MarshalQueuePayload(jobID)
	if err != nil {
		return err
	}
	_, err = s.client.EnqueueContext(ctx, asynq.NewTask(TypeGenericJob, payload))
	return err
}
```

```go
func (w *Worker) HandleTask(ctx context.Context, task *asynq.Task) error {
	var payload QueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if err := w.service.MarkRunning(ctx, payload.JobID, w.workerID); err != nil {
		return err
	}
	if err := w.service.AppendLog(ctx, payload.JobID, "info", "job started"); err != nil {
		return err
	}
	return w.service.MarkCompleted(ctx, payload.JobID, map[string]any{"ok": true})
}
```

- [ ] **Step 4: 运行单测与本地闭环**

Run: `cd backend-go && go test ./internal/jobs -v`  
Expected: PASS

Run: `cd backend-go && go test ./... -v`  
Expected: PASS

- [ ] **Step 5: 提交 worker 闭环**

```bash
git add backend-go/internal/jobs/queue.go backend-go/internal/jobs/worker.go \
  backend-go/internal/jobs/worker_test.go backend-go/internal/jobs/service.go \
  backend-go/cmd/worker/main.go
git commit -m "feat: add go worker queue execution loop"
```

---

### Task 6: 补最小 E2E、运行说明和迁移护栏

**Files:**
- Create: `backend-go/tests/e2e/jobs_flow_test.go`
- Modify: `backend-go/README.md`
- Modify: `backend-go/Makefile`
- Create: `backend-go/.env.example`
- Create: `backend-go/docs/phase1-runbook.md`

- [ ] **Step 1: 写最小闭环 E2E 测试**

```go
package e2e_test

import "testing"

func TestJobsFlow(t *testing.T) {
	t.Skip("enable when local postgres and redis containers are available")
}
```

- [ ] **Step 2: 运行测试确认已纳入测试树**

Run: `cd backend-go && go test ./tests/e2e -v`  
Expected: PASS with SKIP

- [ ] **Step 3: 补运行文档与护栏**

```markdown
# backend-go

## Current Scope

- Go control API
- Phase 1 jobs schema
- Generic worker loop

## Not Yet Migrated

- Real registration state machine
- Python legacy worker bridge
- Payment and bind-card flows
```

```markdown
# Phase 1 Runbook

1. Start PostgreSQL and Redis
2. Export `DATABASE_URL` and `REDIS_ADDR`
3. Run `make migrate-up`
4. Start API with `make run-api`
5. Start worker with `make run-worker`
```

- [ ] **Step 4: 运行全量测试**

Run: `cd backend-go && go test ./... -v`  
Expected: PASS

- [ ] **Step 5: 提交 Phase 0-1 收尾**

```bash
git add backend-go/tests/e2e/jobs_flow_test.go backend-go/README.md backend-go/Makefile \
  backend-go/.env.example backend-go/docs/phase1-runbook.md
git commit -m "docs: add backend-go phase1 runbook and safeguards"
```

---

## Self-Review Notes

- 规格覆盖：
  - `backend-go/` 独立目录约束：Task 1
  - Go control API：Task 4
  - PostgreSQL 权威任务模型：Task 3
  - Redis 队列与 worker：Task 5
  - Phase 0-1 运行闭环：Task 6
- 占位检查：
  - 真实注册链路、Python bridge 明确列为 out of scope，没有假装在本计划内完成
- 类型一致性：
  - 统一使用 `JobID` / `job_id`、`StatusPending` / `StatusCompleted`、`TypeGenericJob`

---

## Scope Notes

### In Scope

- 新建 `backend-go/`
- Go API / PostgreSQL / Redis / Asynq 最小闭环
- 通用 jobs 模型
- Phase 1 运行与测试脚手架

### Out of Scope

- 真实注册状态机迁移
- Python legacy worker 接入实现
- 前端模板替换
- 支付和绑卡迁移
