# Auto Team Auto Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从账号管理页进入 Team 管理时自动发现对应母号，并将重复 discovery 请求改为可复用既有任务而不是返回 409。

**Architecture:** 后端在 discovery 入队前检查同 scope 活跃任务，必要时复用并重新调度；前端在 `auto-team` 页面初始化时基于 URL 参数自动触发 discovery，并统一处理异常提示。

**Tech Stack:** FastAPI, SQLAlchemy, 原生 JavaScript, pytest, node:test

---

### Task 1: 后端 discovery 任务复用

**Files:**
- Modify: `src/services/team/tasks.py`
- Modify: `src/web/routes/team.py`
- Test: `tests/test_team_task_service.py`
- Test: `tests/test_team_routes.py`

- [ ] 写 discovery 复用/恢复调度的失败测试
- [ ] 运行相关 pytest，确认当前行为失败
- [ ] 实现同 scope discovery 活跃任务复用逻辑
- [ ] 对缺失运行时状态的既有任务补调度
- [ ] 重新运行相关 pytest，确认通过

### Task 2: 前端自动 discovery

**Files:**
- Modify: `static/js/auto_team.js`
- Test: `tests/frontend/auto_team.test.mjs`

- [ ] 写页面初始化自动 discovery 的前端测试
- [ ] 运行 node:test，确认当前行为失败
- [ ] 实现基于 `owner_account_id` 的自动 discovery 与本地 guard
- [ ] 为点击动作补统一错误处理
- [ ] 重新运行 node:test，确认通过

### Task 3: 回归验证

**Files:**
- Test: `tests/test_team_sync_service.py`
- Test: `tests/test_team_task_service.py`
- Test: `tests/test_team_discovery_service.py`
- Test: `tests/test_team_routes.py`
- Test: `tests/test_team_task_runner.py`
- Test: `tests/frontend/auto_team.test.mjs`

- [ ] 运行 Team 相关后端测试
- [ ] 运行 `auto_team` 前端测试
- [ ] 检查 git diff，确认仅包含本次目标改动
