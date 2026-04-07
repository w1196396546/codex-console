---
phase: 04-payment-and-team-domains
plan: "09"
subsystem: payments
tags: [payment, transition-adapter, billing-profile, checkout-link, url-safe]
requires:
  - phase: 04-07
    provides: payment transition adapter runtime seams and truthful fallback behavior
provides:
  - supported-country billing templates that stay internally consistent with advertised country/currency pairs
  - URL-safe synthetic checkout session IDs for arbitrary workspace names
  - regression coverage for non-ASCII workspace names and non-US/GB/CA billing countries
affects: [payment-runtime, payment-transition, phase-5-cutover]
tech-stack:
  added: []
  patterns: [country-template parity, deterministic workspace slug normalization, payment adapter regression tests]
key-files:
  created: [.planning/phases/04-payment-and-team-domains/04-09-SUMMARY.md]
  modified:
    - backend-go/internal/payment/transition_adapters.go
    - backend-go/internal/payment/transition_adapters_test.go
    - backend-go/cmd/api/payment_runtime_test.go
key-decisions:
  - "保持 transition adapter 对 AU/SG/HK/JP/DE/FR/IT/ES 的支持面不变，直接补齐本地 billing template，而不是静默缩窄 advertised countries。"
  - "workspace-derived synthetic checkout session id 继续保留 ASCII workspace 的旧 shape，但把危险字符映射为稳定 token、非 ASCII 映射为 `u<hex>`，确保只输出 `[A-Za-z0-9_-]`。"
patterns-established:
  - "Transition billing adapter 的 supported-country 列表必须与本地 template 一一对应；只有 unsupported country 才允许回退到 US template。"
  - "Synthetic checkout session id 只允许 URL-safe ASCII 字符，且 simple ASCII workspace 的兼容形状不可回归。"
requirements-completed: [PAY-01]
duration: 23min
completed: 2026-04-06
---

# Phase 4 Plan 09 Summary

**Payment transition adapter 现在为全部 advertised countries 返回一致的本地 billing template，并把 synthetic checkout slug 规范化为 URL-safe ASCII**

## 改动概览

- 更新 `backend-go/internal/payment/transition_adapters.go`：
  - 为 `AU/SG/HK/JP/DE/FR/IT/ES` 补齐本地 billing template，消除 `country=AU` 却返回 US 地址这类不一致。
  - 新增 workspace slug 规范化逻辑：保留 ASCII 字母数字片段，空格/连字符继续折叠为 `_`，`/ ? # %` 转成稳定 token，非 ASCII rune 转成 `u<hex>`，生成的 synthetic checkout session id 只包含 URL-safe ASCII。
- 更新 `backend-go/internal/payment/transition_adapters_test.go`：
  - 先打红锁定全部 supported countries 的 `country/currency/state/postal_code` 一致性。
  - 锁定 unsupported country 仍然稳定回退到 US template。
  - 锁定包含 `/ ? # %` 和中文的 workspace name 必须产生 deterministic、URL-safe 的 session id，同时 simple ASCII workspace 保持旧 shape。
- 更新 `backend-go/cmd/api/payment_runtime_test.go`：
  - 通过 runtime helper 再次覆盖 `AU/JP` 这类非 US/GB/CA billing country。
  - 覆盖带中文和 path/query/fragment 保留字符的 workspace name，确保最终 checkout link 与 session id 仍然兼容。

## 关键结果

- transition random billing adapter 不再出现 advertised country 与返回地址模板不匹配的问题。
- synthetic checkout link 不再因 `workspace_name` 中包含 `/ ? # %` 或非 ASCII 字符而破坏 path 语义。
- simple ASCII workspace name 仍维持既有 `acme_team_01` 兼容形状，避免无关回归。

## 文件

- `backend-go/internal/payment/transition_adapters.go`
- `backend-go/internal/payment/transition_adapters_test.go`
- `backend-go/cmd/api/payment_runtime_test.go`
- `.planning/phases/04-payment-and-team-domains/04-09-SUMMARY.md`

## 验证命令

- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./internal/payment -run 'TestPaymentTransitionAdapter.*' -v`
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && go test ./cmd/api -run 'TestAPIPaymentRuntime.*' -v`

## 验证结果

- `./internal/payment`：PASS
- `./cmd/api`：PASS

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 无阻塞问题。红灯测试直接暴露了 review 指出的两个缺口：supported-country/template 不一致，以及 workspace name 未做 URL-safe 规范化。

## Next Phase Readiness

- payment transition adapter 的 country/template 与 checkout slug 兼容约束已被测试锁定，后续若扩展国家或修改 synthetic checkout 规则，需要同步更新 adapter 与 regression tests。
