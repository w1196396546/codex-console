---
phase: 04-payment-and-team-domains
plan: "07"
subsystem: payments
tags: [payment, transition-adapter, bootstrap, browser-open, truthfulness, go-test]
requires:
  - phase: 04-04
    provides: payment transition adapter seam and runtime helper wiring
  - phase: 04-06
    provides: cmd/api payment runtime constructor coverage
provides:
  - truthful transition session bootstrap semantics
  - truthful browser-open fallback semantics
  - runtime regression coverage for payment helper bootstrap/open behavior
affects: [payment-runtime, payment-transition, phase-5-cutover]
tech-stack:
  added: []
  patterns: [truthful fallback, no synthetic session persistence, adapter-bound compatibility tests]
key-files:
  created: [.planning/phases/04-payment-and-team-domains/04-07-SUMMARY.md]
  modified:
    - backend-go/internal/payment/transition_adapters.go
    - backend-go/internal/payment/transition_adapters_test.go
    - backend-go/cmd/api/payment_runtime_test.go
key-decisions:
  - "transition bootstrap 仅复用账号中已存在的可重用 session_token / cookie，不再合成 session-bootstrap-{id}。"
  - "transition browser opener 在 Phase 4 保持兼容回退，但统一返回未打开，避免对 /open 和 auto-open 产生假阳性。"
patterns-established:
  - "过渡适配器可以提供兼容响应，但不得把占位数据写入 accounts truth source。"
  - "未实现真实浏览器启动链路前，payment open 相关接口只能返回 manual fallback，不能伪造 opened 成功。"
requirements-completed: [PAY-01]
duration: 25min
completed: 2026-04-06
---

# Phase 4 Plan 07 Summary

**Payment transition adapter 改为只信任真实 session 数据，并把 browser-open/auto-open 恢复为 truthful fallback 语义**

## 改动概览

- 更新 `backend-go/internal/payment/transition_adapters.go`：
  - `BootstrapSessionToken(...)` 只复用账号已有的 `session_token` / session cookie，不再伪造 `session-bootstrap-{id}`。
  - `OpenIncognito(...)` 统一返回 `opened=false`，保留 Phase 4 的手动兜底语义，但不再对任意非空 URL 谎报“已打开浏览器”。
- 更新 `backend-go/internal/payment/transition_adapters_test.go`：
  - 锁定 access token / refresh token 单独存在时不得 bootstrap 出新 `session_token`。
  - 锁定无真实 session token 时 `BootstrapAccountSessionToken(...)` 不得向 accounts 持久化占位 token。
  - 锁定 `OpenBrowserIncognito(...)` 与 `CreateBindCardTask(auto_open=true)` 不再产生 false-positive opened。
- 更新 `backend-go/cmd/api/payment_runtime_test.go`：
  - 锁定 helper-built payment service 在当前 constructor path 下同样遵守无 synthetic token、无 fake-open success 的真值规则。
  - 补充 `OpenBindCardTask(...)` 路由级语义：在没有可用浏览器时应保持 Python 兼容的 `500 + last_error`，而不是伪造 `opened` 成功。

## 关键结果

- payment bootstrap 不再污染共享账号真值源，`accounts.session_token` 只会写入真实可复用 session 数据。
- `/open` 兼容路径和 auto-open 路径都不会再仅因 URL 非空就返回成功或把任务推进到 `opened`；其中 `/bind-card/tasks/{id}/open` 在没有可用浏览器时保持 Python 兼容的 `500 + detail` 语义，并写回 `last_error`。
- 既有 session diagnostic / subscription sync 的兼容覆盖仍然保留，04-07 只修 truthfulness，不扩散到 Phase 5 browser automation。

## 文件

- `backend-go/internal/payment/transition_adapters.go`
- `backend-go/internal/payment/transition_adapters_test.go`
- `backend-go/cmd/api/payment_runtime_test.go`
- `.planning/phases/04-payment-and-team-domains/04-07-SUMMARY.md`

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/payment -run 'Test(PaymentTransitionAdapter|PaymentSubscription).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPIPaymentRuntime.*' -v`

## 验证结果

- `./internal/payment`：PASS
- `./cmd/api`：PASS

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 无阻塞问题。红灯阶段直接暴露了 review finding 对应的旧行为：synthetic bootstrap token 与 fake-open success，随后以最小 adapter 改动修复。

## Next Phase Readiness

- Phase 4 payment runtime helper 已具备更可信的过渡语义，后续 Phase 5 若接入真实浏览器启动链路，需在新的实现里显式恢复 `opened=true` 的成功条件。
- 当前仍是 transition fallback，不代表已经实现真实 browser automation；但兼容接口不再对外暴露错误成功状态。
