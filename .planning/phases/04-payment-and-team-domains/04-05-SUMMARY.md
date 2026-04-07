# 04-05 Summary

## 改动概览

- 在 `backend-go/internal/team/transition_gateway.go` 增加 concrete/transition `MembershipGateway`，用统一 `TransitionRequest` + `TransitionTransport` 封装 revoke/remove 上游调用；默认走 `https://chatgpt.com/backend-api/...`，测试可注入 fake transport。
- 在 `backend-go/internal/team/transition_executor.go` 增加 concrete/transition `TaskExecutor`，支持 discovery/sync/invite 的 hook 化执行；默认没有 live hook 时不再让任务停在 `pending`，而是通过共享 jobs/task runtime 进入失败终态并释放 active scope。
- 补齐 `transition executor` 的默认过渡语义：即使 `main.go` 没有显式注入自定义 hook，discovery/sync/invite 也会产出兼容的 transition 结果，而不是直接失败。
  - discovery：汇总当前 owner 视角下已持久化 team 数量。
  - sync：更新 team 的 `last_sync_at` / `sync_status` / `current_members` / `seats_available`，并把当前 membership 快照投影成 task items。
  - invite：基于邮箱或本地账号构造 transition invite items/logs，保持 accepted task 能在 Go 侧完成并沉淀详情。
- 在 `backend-go/internal/team/tasks.go` 增加 accepted-task 自动 launch。`StartDiscovery` / `StartTeamSync` / `StartInvite*` 创建 accepted task 后，会在 Go 侧异步调用 `ExecuteTask(...)`，不再要求测试或上层手动补一层 `ExecuteTask(...)`。
- 在 `backend-go/cmd/api/team_runtime.go` 增加 fully wired helper：统一组装 `team.Service`、`team.TaskService`、transition membership gateway、transition executor，供 API bootstrap 复用。

## 测试覆盖

- `transition_gateway_test.go`
  - 锁定 revoke invite 的 DELETE path / payload / token 语义。
  - 锁定 remove member 的 path 与上游错误透传。
- `transition_executor_test.go`
  - 锁定 discovery owner id 规范化后进入 executor hook。
  - 锁定 invite task 错误透传。
  - 锁定 default transition discovery/sync/invite 在无自定义 hook 时也能返回 completed 结果，而不是退化成 failed。
- `tasks_test.go`
  - 新增 accepted discovery/sync/invite 自动执行测试，验证任务无需手工 `ExecuteTask(...)` 即可从 `pending` 推进到终态。
  - 更新既有 invite task 测试，改为等待自动执行链，避免与新 launch 机制重复执行。
- `team_runtime_test.go`
  - 验证 helper 产出的 runtime 注入了非 nil gateway/executor。
  - 验证 helper 路径下 revoke 不再命中 `membership gateway not configured`。
  - 补充 remove 的 helper 级覆盖。
  - 验证 helper 路径下 accepted invite task 会自动推进到 `completed`。
  - 新增默认 executor 场景，验证 helper-built runtime 在无自定义 hook 时也能自动完成 sync accepted task。

## 验证命令

### 通过

```bash
cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/team -run 'Test(TeamTransitionGateway|TeamTaskLiveExecution|Accepted|Invite|Discovery|Sync).*' -v
```

结果：通过。

```bash
cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPITeamRuntime.*' -v
```

结果：通过。

## 剩余风险

- `backend-go/cmd/api/main.go` 仍未接入本次新增 `newAPITeamServices(...)` helper。由于本次写集明确禁止修改 `main.go`，所以 helper 已提供并由独立测试证明可用，但真实 API bootstrap 是否切换到该 helper 仍需上层集成步骤完成。
- transition executor 当前提供的是保守的 default transition behavior，不等同于完整上游 live Team API 执行：
  - discovery 复用当前已持久化 team 视角做汇总；
  - sync 会刷新本地 team 聚合与 task items，但不主动拉取上游；
  - invite 会输出兼容 items/logs，但不会在当前 fake/runtime 测试仓储里创建新的 membership 持久记录。
- 因此，本次结果保证的是“live constructor path 不再因为 nil executor/gateway 或 accepted task 无执行链而失效”，完整上游语义仍需要后续 cutover 阶段的人验或更深的 transition 接入。
