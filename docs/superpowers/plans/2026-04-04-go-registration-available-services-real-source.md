# Go Registration Available Services Real Source Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `backend-go` 的 `/api/registration/available-services` 从硬编码响应改成真实读取 PostgreSQL 中的 `settings` 与 `email_services`，让前端邮箱服务入口、尤其是 Outlook 入口，能按真实配置显示。

**Architecture:** 新增一个 available-services compatibility service。它读取 `settings` 中与临时邮箱/自定义域名相关的关键开关，再读取 `email_services` 中已启用服务，最后按前端当前依赖的 group shape 聚合成 `tempmail / yyds_mail / outlook / moe_mail / temp_mail / duck_mail / luckmail / freemail / imap_mail`。HTTP handler 优先使用该 service；未注入时保留现有 fallback stub，避免测试和未接线场景被打断。

**Tech Stack:** Go 1.25, pgx/pgxpool, chi, existing backend-go registration/http stack

---

### Task 1: 建立 available-services 聚合 service

**Files:**
- Create: `backend-go/internal/registration/available_services.go`
- Create: `backend-go/internal/registration/available_services_postgres.go`
- Create: `backend-go/internal/registration/available_services_test.go`

- [ ] **Step 1: 写失败测试**

覆盖最小行为：
- settings 打开 `tempmail_enabled` 时，返回 `tempmail.available=true`
- settings 打开 `yyds_mail_enabled` 且有 API key 时，返回 `yyds_mail` 默认入口
- `email_services` 中的 `outlook/moe_mail/temp_mail/duck_mail/luckmail/freemail/imap_mail` 已启用记录会被聚合到对应 group
- `outlook` item 至少返回 `id/name/type/has_oauth`

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration -run 'TestBuildAvailableServices|TestListAvailableServices' -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现**

实现要点：
- 查询 `settings`：`tempmail_enabled`、`yyds_mail_enabled`、`yyds_mail_api_key`、`yyds_mail_default_domain`、`custom_domain_base_url`、`custom_domain_api_key`
- 查询 `email_services where enabled=true`
- 解析 `config` JSON 文本，按 Python 现有字段映射到前端 group shape
- 保持 key 名与前端完全一致

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration -run 'TestBuildAvailableServices|TestListAvailableServices' -v`  
Expected: PASS

---

### Task 2: 接入 HTTP handler 与 runtime

**Files:**
- Modify: `backend-go/internal/registration/http/handlers.go`
- Modify: `backend-go/internal/registration/http/handlers_test.go`
- Modify: `backend-go/internal/http/router.go`
- Modify: `backend-go/cmd/api/main.go`

- [ ] **Step 1: 写失败测试**

补 HTTP 契约：
- 注入 available-services service 后，`GET /api/registration/available-services` 返回真实 group 数据
- 未注入时仍保持现有 fallback shape

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run 'TestAvailableServicesEndpointUsesInjectedData|TestAvailableServicesEndpointFallbackShape' -v`  
Expected: FAIL

- [ ] **Step 3: 写最小实现**

实现要点：
- handler 增加可选 `availableServices` 依赖
- router 支持注入该 service
- `cmd/api/main.go` 使用 Postgres pool 创建真实 service 并注入

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -run 'TestAvailableServicesEndpointUsesInjectedData|TestAvailableServicesEndpointFallbackShape' -v`  
Expected: PASS

---

### Task 3: 跑最小回归并准备接后续 Outlook 切片

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go` (only if needed)

- [ ] **Step 1: 视需要补回归**

如果 HTTP handler 测试已足够覆盖契约，可不新增 e2e；否则补一条 `available-services` 真实 group 回归。

- [ ] **Step 2: 运行最小全量验证**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./...`  
Expected: PASS

- [ ] **Step 3: 提交**

Commit: `feat: load registration available-services from postgres`
