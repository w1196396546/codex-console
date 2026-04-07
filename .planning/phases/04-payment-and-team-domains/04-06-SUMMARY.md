# 04-06 Summary

## 改动概览

- 更新 `backend-go/cmd/api/main.go`：
  - payment 改为通过 `newAPIPaymentService(...)` 构建，真实 API bootstrap 不再直接挂空心 `payment.NewService(...)`。
  - team 改为通过 `newAPITeamServices(...)` 构建，真实 API bootstrap 不再直接把 `MembershipGateway` / `TaskExecutor` 置空。
- 刷新 `backend-go/internal/http/router_test.go`：
  - 扩大 Phase 4 路由覆盖到 payment generate-link、payment session-bootstrap、team list、team sync-batch、team task list 等关键入口，持续验证 `/api/*` 挂载不回退成 404。
- 刷新 `backend-go/tests/e2e/payment_team_flow_test.go`：
  - payment 改为使用 `payment.NewTransitionAdapters()` 形成的真实 transition seams，而不是手工 fake generators/checkers。
  - team 改为使用 `team.NewTransitionTaskExecutor(...)` 的真实 transition executor 路径，不再从测试体手工调用 `ExecuteTask(...)`。
  - websocket 断言改为兼容 accepted task 自动启动后的 `pending/running/completed` 实际竞态，避免把旧的手工触发时序当成协议契约。

## 关键结果

- `cmd/api` 当前真实构造链已经消费 04-04 / 04-05 新增的 payment / team runtime helpers。
- payment live constructor path 不再遗漏 checkout/session/subscription/auto-bind seams。
- team live constructor path 不再遗漏 membership gateway / task executor，accepted task 会在真实 runtime 下自动推进。
- Phase 4 router/e2e 自动化不再依赖“测试里直接 fake seam 注入 + 手工 ExecuteTask”这种假 bootstrap 路径。

## 文件

- `backend-go/cmd/api/main.go`
- `backend-go/internal/http/router_test.go`
- `backend-go/tests/e2e/payment_team_flow_test.go`

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPI(Payment|Team)Runtime.*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/http -run 'Test(Router|PaymentTeam|PhaseFour).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./tests/e2e -run 'Test(Payment|Team|PhaseFour).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/team -run 'Test(TeamTransitionGateway|TeamTransitionExecutor|TeamTaskLiveExecution|Accepted|Invite|Discovery|Sync).*' -v`

## 验证结果

- `./cmd/api`：PASS
- `./internal/http`：PASS
- `./tests/e2e`：PASS
- `./internal/team`：PASS

## 剩余风险

- payment / team 当前接入的是“防空心 bootstrap”的 transition runtime，不等同于最终完整 cutover：
  - payment browser-open / subscription / auto-bind 仍是保守过渡实现；
  - team discovery/sync/invite 默认路径现在会完成并产出兼容 summary/items，但完整上游 Team API 语义仍需要更深的 transition 接入或人工验证。
- 不过就 Phase 4 当前 blocker 而言，live wiring 已经闭环：真实 API process 不再因为 nil seam / accepted task 无执行链而直接失效。
