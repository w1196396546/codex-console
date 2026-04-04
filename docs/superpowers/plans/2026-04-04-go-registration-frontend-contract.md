# Go Registration Frontend Contract Hardening Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `backend-go` 的 registration 兼容响应进一步对齐到现有 `static/js/app.js` 的真实期望，先确保注册页的邮箱服务加载和单任务状态/日志读取在字段形状上真正兼容。

**Architecture:** 在已有 registration compatibility slice 的基础上补 DTO 和契约测试，不引入真实注册状态机，只修正响应形状与前端契约。优先覆盖 `available-services`、单任务详情、单任务日志三个前端直接消费的接口。

**Tech Stack:** Go 1.25, chi, existing backend-go registration handlers/tests, existing frontend contract from static/js/app.js

---

### Task 1: 对齐 `available-services` 响应结构

**Files:**
- Modify: `backend-go/internal/registration/http/handlers.go`
- Modify: `backend-go/internal/registration/http/handlers_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestAvailableServicesEndpointMatchesFrontendShape(t *testing.T) {
	router, _, _ := newRegistrationRouter(t)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registration/available-services", nil))

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if _, ok := resp["tempmail"].(map[string]any); !ok {
		t.Fatal("expected tempmail object")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run TestAvailableServicesEndpointMatchesFrontendShape -v`  
Expected: FAIL，说明当前仍是数组形状

- [ ] **Step 3: 写最小实现**

对齐到前端当前使用的最小结构：

```json
{
  "tempmail": { "available": true, "count": 1, "services": [...] },
  "yyds_mail": { "available": false, "count": 0, "services": [] },
  "outlook": { "available": false, "count": 0, "services": [] },
  "moe_mail": { "available": false, "count": 0, "services": [] },
  "temp_mail": { "available": false, "count": 0, "services": [] },
  "duck_mail": { "available": false, "count": 0, "services": [] },
  "luckmail": { "available": false, "count": 0, "services": [] },
  "freemail": { "available": false, "count": 0, "services": [] }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -v`  
Expected: PASS

---

### Task 2: 对齐单任务详情与日志字段

**Files:**
- Modify: `backend-go/internal/registration/http/handlers.go`
- Modify: `backend-go/internal/registration/http/integration_test.go`
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestRegistrationTaskResponseIncludesFrontendFields(t *testing.T) {
	// 断言 task/detail 与 logs 响应至少包含 task_uuid/status/email/email_service/log_offset/log_next_offset
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/registration/http -run TestRegistrationTaskResponseIncludesFrontendFields -v`  
Expected: FAIL，说明字段尚未完全对齐

- [ ] **Step 3: 写最小实现**

最小字段契约：

- `GET /api/registration/tasks/{task_uuid}` 返回
  - `task_uuid`
  - `status`
  - `email`
  - `email_service`
- `GET /api/registration/tasks/{task_uuid}/logs` 返回
  - `task_uuid`
  - `status`
  - `email`
  - `email_service`
  - `logs`
  - `log_offset`
  - `log_next_offset`

其中 `email` / `email_service` 暂可返回 `null`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./...`  
Expected: PASS

---

### Task 3: 用真实前端期望补一条兼容回归

**Files:**
- Modify: `backend-go/tests/e2e/jobs_flow_test.go`

- [ ] **Step 1: 写失败测试**

补一条更贴近前端消费方式的断言：

- `available-services` 返回对象结构
- `tempmail.available === true`
- `logs` 首轮为空时字段齐全

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./tests/e2e -run TestRegistrationCompatibilityFlow -v`  
Expected: FAIL，如果结构未对齐

- [ ] **Step 3: 写最小实现并回归**

补齐 handler 返回字段，保持不引入真实注册逻辑。

- [ ] **Step 4: 跑最小全量验证**

Run: `cd backend-go && go test ./internal/registration/http -v && go test ./tests/e2e -v && go test ./...`  
Expected: PASS

