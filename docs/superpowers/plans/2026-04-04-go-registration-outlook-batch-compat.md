# Go Registration Outlook Batch Compatibility Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `backend-go` 增加 Outlook 账户列表与 Outlook 批量注册 HTTP 兼容层，让前端切到 Outlook 模式后可以完成 `loadOutlookAccounts -> start outlook batch -> polling/ws monitor -> control` 主流程。

**Architecture:** 新增一个 Outlook compatibility service：读 PostgreSQL 里的 `email_services` 和 `accounts`，输出前端所需的 Outlook 账户列表；启动批量时复用现有 `BatchService` 的批量任务与 websocket 能力，只是把每个 `service_id` 映射成一条 `email_service_type=outlook`、`email_service_id=<id>` 的注册 job。HTTP handler 额外挂 `/api/registration/outlook-accounts` 与 `/api/registration/outlook-batch*`，而 `/api/ws/batch/{batch_id}` 继续直接复用已有 batch websocket。

**Tech Stack:** Go 1.25, pgx/pgxpool, chi, existing backend-go registration/http/ws stack

---

### Task 1: 建立 Outlook compatibility service 与仓储

**Files:**
- Create: `backend-go/internal/registration/outlook_service.go`
- Create: `backend-go/internal/registration/outlook_repository_postgres.go`
- Create: `backend-go/internal/registration/outlook_service_test.go`

- [ ] **Step 1: 写失败测试**

覆盖最小行为：
- `ListOutlookAccounts` 返回：
  - `total`
  - `registered_count`
  - `unregistered_count`
  - `accounts`
- 每个 account 至少包含：
  - `id`
  - `email`
  - `name`
  - `has_oauth`
  - `is_registered`
  - `has_refresh_token`
  - `needs_token_refresh`
  - `is_registration_complete`
  - `registered_account_id`
- `StartOutlookBatch` 返回：
  - `batch_id`
  - `total`
  - `skipped`
  - `to_register`
  - `service_ids`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration -run 'TestListOutlookAccounts|TestStartOutlookBatch' -v`  
Expected: FAIL，提示 service/repository 不存在

- [ ] **Step 3: 写最小实现**

实现要点：
- 仓储查询 `email_services` 中 `service_type='outlook' and enabled=true`
- 用 `accounts.email = email_services.config->>'email'` 做关联，推导注册状态
- `StartOutlookBatch` 不单独造一套批量状态机，直接复用现有 `BatchService`
- 每个 `service_id` 映射成一条 `StartRequest{EmailServiceType:\"outlook\", EmailServiceID:&id}`
- 先保持 `skipped=0`，与当前 Python 主路径一致

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration -run 'TestListOutlookAccounts|TestStartOutlookBatch' -v`  
Expected: PASS

---

### Task 2: 接出 Outlook HTTP 兼容接口

**Files:**
- Modify: `backend-go/internal/registration/http/handlers.go`
- Modify: `backend-go/internal/registration/http/handlers_test.go`
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`

- [ ] **Step 1: 写失败测试**

补最小契约：
- `GET /api/registration/outlook-accounts`
- `POST /api/registration/outlook-batch`
- `GET /api/registration/outlook-batch/{batch_id}`
- `POST /pause|resume|cancel`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run 'TestOutlookAccountsEndpoint|TestOutlookBatchEndpoints' -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现**

实现要点：
- 只有注入了 Outlook service 时才挂这些路由
- `GET /outlook-batch/{batch_id}` 复用 generic batch `GetBatch` 结果，再补 `skipped`
- `pause/resume/cancel` 直接代理到现有 `BatchService`
- 启动响应字段最少对齐前端：
  - `batch_id`
  - `total`
  - `skipped`
  - `to_register`
  - `service_ids`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -run 'TestOutlookAccountsEndpoint|TestOutlookBatchEndpoints' -v`  
Expected: PASS

---

### Task 3: 补 Outlook 主链回归

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

补一条最小前端主链：
- `GET /api/registration/outlook-accounts`
- `POST /api/registration/outlook-batch`
- `GET /api/registration/outlook-batch/{batch_id}?log_offset=0`
- `pause/resume/cancel`
- 复用 `/api/ws/batch/{batch_id}` 校验 websocket 可消费 Outlook batch

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./tests/e2e -run TestRegistrationOutlookBatchCompatibility -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现并回归**

兼容重点：
- `outlook-accounts` 返回的账户状态要能被前端默认勾选逻辑消费
- `outlook-batch` 复用 batch websocket，不另起一个 ws route
- 完成提示里要带 `skipped`

- [ ] **Step 4: 运行最小全量验证**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./tests/e2e -v && go test ./...`  
Expected: PASS
