# 04-08 Summary

## 改动概览

- 更新 `backend-go/internal/team/transition_executor.go`：
  - 默认 discovery 不再只重算本地 team 数量，改为真正请求上游 `/backend-api/accounts/check/v4-2023-04-27`，解析 owner team，并把 team 真值落到仓库。
  - 默认 sync 不再只读取本地 membership，改为真正请求上游 members / invites，按 Python 兼容语义合并 joined/invited/removed/revoked/already_member，并刷新 team 聚合字段。
  - 默认 invite 不再合成 success item，改为真正请求上游 invite 合同，按 success / already_member / team_full / failed 分支持久化 membership，并重算 team 席位。
- 更新 `backend-go/internal/team/repository_postgres.go`：
  - 为 transition executor 增加 Postgres 侧 `UpsertTeam`、`UpsertMembership`、`ListAccountsByEmails`，在不扩大公共 `Repository` 接口破坏面的前提下补齐 live persistence 能力。
- 更新 `backend-go/internal/team/transition_executor_test.go`、`backend-go/cmd/api/team_runtime_test.go`、`backend-go/tests/e2e/payment_team_flow_test.go`：
  - 新增默认 live executor 的 discovery/sync/invite side-effect 断言，确保 regression 会在回退成 placeholder 时直接失败。
  - runtime 测试验证 `newAPITeamServices(...)` 默认构造链会执行真实 transition side effects，而不是只接受任务后本地完成。
  - e2e 改为串 discovery -> sync -> invite，并验证 team / membership 持久状态变化，而不是只看 `completed/logs/items`。
- 更新 `backend-go/internal/team/tasks_test.go`：
  - `TestDiscoveryAcceptedTaskReusesOwnerScopeAndWsChannel` 改成显式阻塞 executor，避免依赖 goroutine 调度时序。

## 关键结果

- Team 默认 executor 现在具备真实 upstream-aware transition 行为，discovery / sync / invite 都会触发上游合同并落库。
- accepted task 的 websocket / task detail 兼容契约未变，但现在能同时证明持久层 side effects。
- Phase 4 的 Team e2e 不再允许“任务完成了但 team/membership 没变”的假阳性。

## 文件

- `backend-go/internal/team/transition_executor.go`
- `backend-go/internal/team/repository_postgres.go`
- `backend-go/internal/team/transition_executor_test.go`
- `backend-go/internal/team/tasks_test.go`
- `backend-go/cmd/api/team_runtime_test.go`
- `backend-go/tests/e2e/payment_team_flow_test.go`

## 关键决策

- 没有扩大公共 `Repository` 接口；transition 所需新增仓储能力通过 executor 内部的私有 capability type-assert 消费，避免波及非写集里的既有 fake repo / service 测试。
- `team_runtime.go` 本身不需要额外改 wiring：因为默认 `transition_executor` 已经从 placeholder 变成 live transition 实现，当前 constructor path 自然获得真实 side effects。
- invite summary 继续按 Python 语义从 membership 真值重算，不信任旧的裸 `current_members` 快照。

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/team -run 'Test(TeamTransitionExecutor|TeamTransitionGateway|TeamTaskLiveExecution|Accepted|Invite|Discovery|Sync).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPITeamRuntime.*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./tests/e2e -run 'TestTeamPhaseFourAcceptedTaskLiveFlow' -v`

## 验证结果

- `./internal/team`：PASS
- `./cmd/api`：PASS
- `./tests/e2e`：PASS

## 剩余风险

- 这次验证覆盖的是 transition HTTP seam + 持久化 side effects，不是生产上真实 ChatGPT Team 环境联调；真实凭证/权限/限流行为仍需要后续人工或 staging 验证。
- `backend-go/internal/team/repository.go` 和 `backend-go/cmd/api/team_runtime.go` 本次未改：现有公开接口与 constructor 形状已经够用，风险主要集中在 transition executor 与 Postgres 落库面，已被本次测试覆盖。
