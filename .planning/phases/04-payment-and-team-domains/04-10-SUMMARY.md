# 04-10 Summary

## 改动概览

- 更新 `backend-go/internal/team/transition_executor.go`：
  - `resolveTransitionTeamIDs(...)` 改为优先读取 `request_payload.ids`，不再因为 accepted task 为兼容而保留的首个 `TeamID` 就把 `sync_all_teams` 塌缩成单 team 执行。
- 更新 `backend-go/internal/team/tasks.go`：
  - 在 `StartTeamSyncBatch(...)` 增加兼容语义注释，明确首个 `team_id` 仅用于 accepted payload / scope，上层执行必须以 `payload.ids` 为准。
- 更新 `backend-go/internal/team/transition_executor_test.go`、`backend-go/cmd/api/team_runtime_test.go`、`backend-go/tests/e2e/payment_team_flow_test.go`：
  - 新增默认 executor、API runtime、e2e 三层 multi-id `sync-batch` 回归断言，要求每个请求的 team 都产生持久 side effects。
- 更新 `backend-go/internal/team/tasks_test.go`：
  - 将 discovery 复用测试改成显式 mixed-owner 输入，锁定“按第一个 owner 复用 active scope”的 Python oracle 兼容语义，避免以后把 accidental behavior 当成设计。

## 关键结果

- `sync_all_teams` 现在会对 `payload.ids` 中的每个 team 执行同步，不再只处理第一个 id。
- accepted task 的外部兼容契约未变：
  - `team_id` 仍然保留首个 team，便于现有 websocket / accepted payload / scope 行为保持稳定。
  - 真正的 batch 执行语义改由 `payload.ids` 决定。
- discovery 的 mixed-owner first-owner scope 复用现在有明确测试表达，后续若要改语义会直接打破回归测试，而不是继续依赖隐式行为。

## 文件

- `backend-go/internal/team/tasks.go`
- `backend-go/internal/team/tasks_test.go`
- `backend-go/internal/team/transition_executor.go`
- `backend-go/internal/team/transition_executor_test.go`
- `backend-go/cmd/api/team_runtime_test.go`
- `backend-go/tests/e2e/payment_team_flow_test.go`

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/team -run 'Test(TeamTransitionExecutor|Accepted|Sync).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPITeamRuntime.*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./tests/e2e -run 'TestTeamPhaseFourAcceptedTaskLiveFlow' -v`

## 验证结果

- `./internal/team`：PASS
- `./cmd/api`：PASS
- `./tests/e2e`：PASS

## 剩余风险

- 这次修复只闭合了 multi-id `sync-batch` 的执行塌缩问题，没有扩大 team task 的 scope / task listing 语义；因此批量任务仍以首个 `team_id` 作为 accepted 上下文。
- 覆盖面已经到默认 executor、runtime 与 e2e，但仍是基于测试桩的 ChatGPT Team HTTP 合同，不等于真实生产凭证环境联调。
