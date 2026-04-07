# ChatGPT Registration Python Go Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 ChatGPT 注册链路的两类问题：Python 侧避免落库失败仍标记 completed，并为 Python/Go 都引入显式 `refresh_token` / `access_token_only` 注册模式透传与执行。

**Architecture:** Python 侧在路由层和 any-auto 注册引擎补模式字段透传，并把“落库失败”从成功路径剥离出来；Go 侧先把同一字段接入 HTTP、service、batch 与 executor 请求链路，确保跨栈请求语义一致。

**Tech Stack:** Python、FastAPI、原生 JS、Go、chi、现有 pytest/go test

---

### Task 1: Python 路由与任务状态修复

**Files:**
- Modify: `src/web/routes/registration.py`
- Modify: `tests/test_registration_task_binding.py`

- [ ] 写失败测试，覆盖 `save_to_database()` 返回 `False` 时任务不能进入 `completed`
- [ ] 运行对应 pytest，确认先红
- [ ] 最小修改 Python 任务收尾逻辑，把落库失败改成失败态并保留错误信息
- [ ] 重新运行 pytest，确认转绿

### Task 2: Python 显式注册模式支持

**Files:**
- Modify: `src/web/routes/registration.py`
- Modify: `src/core/anyauto/register_flow.py`
- Modify: `src/core/anyauto/chatgpt_client.py`
- Modify: `static/js/app.js`
- Modify: `templates/index.html`
- Modify: `tests/test_anyauto_register_flow.py`
- Modify: `tests/test_registration_task_binding.py`
- Modify: `tests/test_registration_routes.py`

- [ ] 写失败测试，覆盖 `chatgpt_registration_mode=access_token_only` 时跳过 OAuth 补 RT，以及路由把模式下发到引擎
- [ ] 运行对应 pytest，确认先红
- [ ] 最小修改请求模型、前端请求体和 any-auto 主链，让 `refresh_token`/`access_token_only` 成为显式模式
- [ ] 重新运行 pytest，确认转绿

### Task 3: Go 请求链路模式对齐

**Files:**
- Modify: `backend-go/internal/registration/types.go`
- Modify: `backend-go/internal/registration/batch_service.go`
- Modify: `backend-go/internal/registration/service_test.go`
- Modify: `backend-go/internal/registration/batch_service_test.go`
- Modify: `backend-go/internal/registration/executor_test.go`

- [ ] 写失败测试，覆盖 `chatgpt_registration_mode` 在单任务、批量和 executor payload 中被保留
- [ ] 运行对应 `go test`，确认先红
- [ ] 最小修改 Go 请求结构与 payload 透传链路
- [ ] 重新运行 `go test`，确认转绿
