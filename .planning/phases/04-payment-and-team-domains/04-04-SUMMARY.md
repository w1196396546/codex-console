# 04-04 Summary

## 改动概述

- 新增 `backend-go/internal/payment/transition_adapters.go`，集中提供 Go-owned payment transition adapters：
  - checkout-link
  - random-billing
  - browser-open
  - session bootstrap / probe
  - subscription-check
  - auto-bind(local / third-party)
- 新增 `backend-go/cmd/api/payment_runtime.go`，提供 `buildAPIPaymentService(...)` / `newAPIPaymentService(...)`，统一把上述 6 个 seams 注入 `payment.NewService(...)`。
- 新增 payment / cmd/api 两组 TDD 测试，覆盖：
  - helper-built service 不再因为 nil seam 返回 `not configured`
  - subscription 高置信度 `free` 才允许清 paid
  - 低置信度 `free` 继续保留 paid state
  - local / third-party auto-bind 的 transition compatibility 状态

## 文件

- `backend-go/internal/payment/transition_adapters.go`
- `backend-go/internal/payment/transition_adapters_test.go`
- `backend-go/cmd/api/payment_runtime.go`
- `backend-go/cmd/api/payment_runtime_test.go`

## 实现说明

- transition adapters 保持保守策略：
  - checkout-link 返回兼容的官方 checkout URL 形态与 source/session_id 字段，避免 live constructor 因 seam 缺失直接报错。
  - random-billing 使用本地模板生成兼容字段（`name` / `line1` / `city` / `state` / `postal_code` / `country` / `currency`）。
  - browser-open adapter 明确存在，但保持 `false, nil`，继续复用现有“未找到可用浏览器，请手动复制链接”兼容语义。
  - session probe / bootstrap 优先复用已有 access/session/cookie 上下文；无法补全时返回兼容失败结果，不再暴露 `ErrSessionAdapterMissing`。
  - subscription-check adapter 默认只给出保守 `free + low confidence`；若账号 truth 已知为 paid，返回高置信度 paid；若 `ExtraData` 已带 transition hint，则按 hint 输出。
  - auto-bind adapters 统一返回兼容的 pending / need_user_action 结果，避免 nil seam 直接中断任务流。

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/payment -run 'Test(PaymentTransitionAdapter|PaymentSubscription|PaymentAutoBind).*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPIPaymentRuntime.*' -v`

## 验证结果

- `./internal/payment`：PASS
- `./cmd/api`：PASS

## 剩余风险

- `backend-go/cmd/api/main.go` 仍未接入 `newAPIPaymentService(...)`。本次按写集约束只交付 runtime helper，不做实际 bootstrap 切换；该接线工作需要在后续 plan 中完成。
- 当前 transition adapters 是“防 hollow bootstrap”的最小闭环，不等同于完整上游 live 支付/浏览器/订阅实检：
  - browser-open 仍走手动兜底语义
  - checkout-link / subscription / auto-bind 目前是 bounded transition behavior，而不是直接调用上游支付/浏览器自动化链路
- 因此，这次结果保证的是“helper 构建的 payment service 不再因 nil seam 失败”，而不是“main.go 已完成生产切换”。
