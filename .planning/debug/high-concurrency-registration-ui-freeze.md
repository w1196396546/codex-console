---
status: diagnosed
trigger: "用户在前端发起 20 并发注册时，页面直接卡住、一直转圈。数据库已经从 SQLite 切到 PostgreSQL，希望彻底解决并支持高并发注册。"
created: 2026-04-06T15:42:22Z
updated: 2026-04-06T15:44:58Z
---

## Current Focus

hypothesis: 已确认主根因是应用层消息风暴与前端逐条消费；次根因是批量并发与 token 补全过程并发直接耦合，导致 20 并发时消息量、线程活跃度和外部依赖压力同步放大。
test: 已完成代码复核，下一步应由主线程据此拆分服务端观测通道、前端消费模型和注册执行并发模型的改造方案。
expecting: 优先削减 per-log/per-status 推送和解耦 token 并发后，页面卡死会先显著缓解，再继续处理首连补齐竞态与历史日志策略。
next_action: 由主线程基于本诊断文档设计修复方案；本子代理不做业务实现。

## Symptoms

expected: 前端发起 20 并发注册时，页面仍能持续响应，批量任务能正常推进并可观察进度，不会长时间一直转圈。
actual: 前端发起 20 并发注册后页面直接卡住、持续转圈，批量高并发注册不可用。
errors: 未提供明确浏览器或服务端报错；主线程已有推断指向日志推送风暴、重复回放、前端逐条消费和 token 补全过程并发耦合。
reproduction: 在前端发起批量注册，请求并发数设置为 20，观察页面卡住并一直转圈。
started: 数据库已切换为 PostgreSQL，问题仍然存在；当前关注点是确认根因是否已从数据库锁争用转移到应用层推送与前端消费链路。

## Eliminated

- hypothesis: 批量 WebSocket 建连时会把连接前已有历史日志完整回放，前端再调用 `hydrateBatchLogs()` 形成必然重复。
  evidence: `register_batch_websocket()` 会在注册连接时把 `_ws_sent_index[key][id(websocket)]` 置为当前 `_batch_logs` 长度，随后 `get_unsent_batch_logs()` 默认只会返回注册后新增、但尚未发送的日志，不会把建连前已有 batch logs 全量重放。
  timestamp: 2026-04-06T15:44:58Z

## Evidence

- timestamp: 2026-04-06T15:44:58Z
  checked: `src/web/routes/registration.py:_create_registration_log_callback`, `_make_batch_helpers`, `run_batch_parallel`, `run_batch_pipeline`
  found: 每条注册日志会同时进入 task log、batch 持久快照 `batch_tasks[batch_id]["logs"]` 和 `task_manager.add_batch_log()`；批量流程自身还会额外写“开始注册”“成功/失败”“批量完成”等批量日志，并在每个子任务完成后调用 `update_batch_status()`。
  implication: 单个注册步骤会被放大为多份内存写入和多次批量通道事件，批量规模上来后消息面天然膨胀。

- timestamp: 2026-04-06T15:44:58Z
  checked: `src/web/task_manager.py:add_batch_log`, `_broadcast_batch_log`, `update_batch_status`, `_broadcast_batch_status`
  found: 每次批量日志和状态更新都会从工作线程调用 `asyncio.run_coroutine_threadsafe(...)` 投递到主事件循环，再逐连接 `await ws.send_json(...)`；没有批量聚合、节流或快照合并。
  implication: 高并发注册下，服务端事件循环会被大量微小广播任务淹没，WebSocket 推送本身成为热点，而不是数据库。

- timestamp: 2026-04-06T15:44:58Z
  checked: `static/js/app.js:connectBatchWebSocket`, `hydrateBatchLogs`, `appendLogsToConsole`, `addLog`; `static/js/registration_log_buffer.js`
  found: 批量 WebSocket 每条消息都会同步 `JSON.parse`、`getLogType`、`addLog`；`registration_log_buffer` 只把 DOM flush 推迟到 `requestAnimationFrame/setTimeout`，不会减少消息数量，也不会合并多条 WebSocket payload。
  implication: 浏览器主线程仍需为每条日志执行解析、分类、入队和去重判断，消息量上来后 UI 卡顿是必然结果。

- timestamp: 2026-04-06T15:44:58Z
  checked: `src/web/routes/websocket.py:batch_websocket`, `src/web/task_manager.py:register_batch_websocket`, `get_unsent_batch_logs`, `src/web/routes/registration.py:get_batch_status`, `static/js/app.js:hydrateBatchLogs`
  found: 批量 WebSocket 建连会立即发送当前状态，但不会默认回放连接前已有 batch logs；真正的重复窗口来自 `onopen` 后立刻用 `log_offset=0` 调 `hydrateBatchLogs()`，此时 REST 补齐与实时推送存在竞态重叠。
  implication: 首连补齐逻辑会带来额外请求和局部重复消费，但它是放大器，不是最上游根因。

- timestamp: 2026-04-06T15:44:58Z
  checked: `src/web/routes/registration.py:_run_sync_registration_task`, `run_batch_parallel`, `run_batch_pipeline`; `src/config/settings.py:registration_token_completion_max_concurrency`; `src/core/anyauto/register_flow.py`
  found: 批量 `concurrency` 被直接传给单任务 `token_completion_concurrency`；配置 `registration_token_completion_max_concurrency=0` 在当前代码里表示“不设上限”，`AnyAutoRegistrationEngine` 会把请求值作为全局 refresh-token completion slot 上限。
  implication: 20 批量并发不仅是 20 个注册任务，还会把 token/OAuth 补全过程允许并发同步抬到 20，进一步增加外部请求压力、日志量和线程活跃度。

- timestamp: 2026-04-06T15:44:58Z
  checked: `src/web/task_manager.py`, `src/web/routes/registration.py:run_registration_task`, `src/core/register.py`, `src/core/anyauto/register_flow.py`
  found: 注册任务运行在线程池中，线程池上限为 50；`RegistrationEngine` 与 `AnyAutoRegistrationEngine` 内部存在大量 `_log(...)` 调用，`rg` 统计分别约为 250 和 44 处。
  implication: 只要高并发路径命中较多日志分支，消息产量足以把“逐条推送 + 逐条消费”链路压垮，PostgreSQL 替换 SQLite 也无法消除这个热点。

## Resolution

root_cause: 主根因不是数据库，而是批量注册把高频日志和状态更新逐条推到 WebSocket，前端又逐条同步解析和渲染，形成服务端事件循环与浏览器主线程的双重消息风暴；次根因是 batch concurrency 被错误地复用为 token/OAuth 补全过程并发，导致 20 并发时内部敏感子流程也被同步放大。
fix: 未实施业务修复。本次建议的改造边界是 1) 服务端批量观测通道从 per-log/per-status 推送改为聚合、节流、快照化；2) 前端批量监控从逐消息同步消费改为批次消费与状态增量渲染；3) 将 batch 并发与 token/OAuth 补全过程并发彻底解耦，并给 token 路径单独硬上限；4) 最后再收敛 `hydrateBatchLogs()` 首连竞态和重复补齐窗口。
verification: 通过代码复核确认根因链路与主线程提供的 1/2/3/5 号推断一致；4 号推断经复核后修正为“存在首连竞态和局部重复窗口，但批量 WebSocket 不会默认全量回放已有历史日志”。
files_changed:
- .planning/debug/high-concurrency-registration-ui-freeze.md

## Root Cause Ranking

1. 服务端批量日志/状态逐条广播是首要根因。
   20 并发下，工作线程持续通过 `run_coroutine_threadsafe` 向主事件循环投递微任务，再逐条 `send_json`，服务端先被消息密度拖慢。

2. 前端批量监控逐条消费是首要共犯。
   WebSocket 每条消息都同步 `JSON.parse + getLogType + addLog`，`registration_log_buffer` 只缓解 DOM flush，不缓解消息数量，页面卡住与“一直转圈”直接吻合。

3. 批量并发与 token/OAuth 补全过程并发耦合是高并发放大器。
   `concurrency=20` 会把 token completion 全局 slot 也推到 20，导致内部并发、外部依赖压力和日志量一并上升。

4. 首连日志补齐竞态是次级放大器。
   `hydrateBatchLogs()` 与实时推送在首连窗口可能重叠，增加额外请求和局部重复消费，但不是最上游根因。

5. SQLite 已切换 PostgreSQL 只能移除数据库锁争用，不会解决当前应用层消息风暴。
   所以“数据库已切 PostgreSQL 但还是卡”与本次诊断完全一致，不构成反证。

## Suggested Boundaries

- 服务端观测层
  目标是削减事件数量，而不是继续优化单条 `send_json`。优先改 batch log/status 推送模型，做时间窗聚合、批次消息、快照轮询或增量摘要，避免每条步骤日志都进入 WebSocket。

- 前端监控层
  目标是削减主线程同步工作量。优先把批量日志从“每条消息即刻解析并入控制台”改为批次入队、懒渲染、按需展开，并把进度条/统计卡与详细日志分层。

- 注册执行并发层
  目标是解耦批量调度并发和 token/OAuth 补全过程并发。批量层控制任务数，token 补齐层使用单独配置和硬上限，默认值不能再跟随批量并发。

- 首连补齐与恢复层
  目标是消除 `hydrateBatchLogs()` 与实时推送的重叠窗口，并统一 offset/ack 语义。这层应在前面三层收敛后再做，否则容易只修表象。

## Regression Risks

- 如果只降低前端渲染频率、不减少服务端事件数，页面可能稍好一些，但服务端主循环仍会在高并发下堆积广播任务。

- 如果只限制批量 `concurrency`，而不解耦 token/OAuth 子流程并发，高并发能力会被硬性压低，且瓶颈仍未真正定位到观测链路。

- 如果直接删除详细日志而没有保留批次摘要/快照，运维可观测性会明显退化，故需要区分“实时摘要”和“完整历史”两条路径。

- 如果修 `hydrateBatchLogs()` 但不动推送模型，只能减少局部重复，无法解释和解决 20 并发下整页卡死。
