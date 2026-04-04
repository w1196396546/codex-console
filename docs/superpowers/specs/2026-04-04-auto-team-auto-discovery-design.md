# Auto Team Auto Discovery Design

**日期：** 2026-04-04

**目标：**
当用户从账号管理页进入 `/auto-team?owner_account_id=<id>` 时，自动对该母号发起 Team discovery；如果该母号已有活跃 discovery 任务，则复用已有任务而不是返回 `409 conflict`。

## 背景

当前账号管理页已经会把 `owner_account_id` 带到 `auto-team` 页面，但 `auto-team` 页面仍要求用户手动点击“发现母号”。与此同时，同一母号若存在活跃 discovery 任务，后端会直接返回 `409 conflict`，前端表现为控制台报错和无明显恢复路径。

## 设计

### 1. 页面自动 discovery

- `auto_team.js` 初始化页面时读取 `owner_account_id`
- 若存在合法母号 ID，则在首次渲染后自动发起一次 discovery
- 自动 discovery 仅在本次页面生命周期中触发一次，避免重复触发

### 2. discovery 幂等复用

- `POST /api/team/discovery/run` 对同一 `owner_account_id` 采用“复用活跃任务”策略
- 若数据库中已存在同 scope 的活跃 discovery 任务：
  - 直接返回该任务的 accepted payload
  - 若运行时内存中没有该任务状态，则重新接管并调度该任务
- 目标是把“重复请求”从错误改成可继续监听的正常行为

### 3. 前端错误体验

- discovery/sync 点击动作增加统一异常处理
- 对接口异常展示可读提示，避免未捕获 Promise 抛到控制台成为主交互反馈

## 验证

- 后端：
  - 同 scope discovery 已存在时，接口返回 202 + 既有 task payload
  - 既有 discovery 任务缺失运行时状态时，可重新调度执行
- 前端：
  - 带 `owner_account_id` 进入页面时自动触发 discovery
  - 自动 discovery 返回既有 task payload 时，仍能正常进入 WebSocket 监听流
