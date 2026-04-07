---
phase: 05-cutover-and-decommission
plan: "01"
subsystem: cutover
tags: [cutover, rollback, docs, verification, deployment]
requires: []
provides:
  - documented Go-owned backend cutover topology and rollback contract
  - environment-gated Phase 5 verification harness with backend-go Makefile entrypoint
  - initial Phase 5 verification artifact with honest local-pass / live-gated status
affects: [operator docs, deployment guidance, phase verification, final cutover readiness]
tech-stack:
  added:
    - shell verification entrypoint
  patterns:
    - environment-gated live validation
    - docs/scripts/make-target alignment for cutover operations
key-files:
  created:
    - scripts/verify_phase5_cutover.sh
    - .planning/phases/05-cutover-and-decommission/05-VERIFICATION.md
  modified:
    - README.md
    - backend-go/README.md
    - backend-go/docs/phase1-runbook.md
    - backend-go/Makefile
    - scripts/docker/start-webui.sh
    - docker-compose.yml
key-decisions:
  - "先把 Go API + Go worker + PostgreSQL + Redis 明确成目标后端拓扑，再谈去 Python 化，避免把 rollback 路径提前删掉。"
  - "Phase 5 验证入口必须把 local PASS 与 live-gated SKIP 明确分开，不能靠静态测试假装完成 cutover。"
patterns-established:
  - "公开文档、运行脚本、Makefile 入口需要共同指向同一条 cutover 叙事，避免 operator 因文档漂移继续走 Python-first 路径。"
  - "最终 cutover gate 以 repo-root shell 脚本承载，并允许 backend-go 通过 Makefile 转发调用。"
requirements-completed: [CUT-02]
duration: 30min
completed: 2026-04-06
---

# Phase 05 Plan 01: Cutover Topology and Verification Gate Summary

**Phase 5 的 cutover 基线已经立起来了：Go-owned backend topology、rollback 口径、Phase 5 verification harness 和初始 verification artifact 都已经落地。**

## Performance

- **Duration:** 30 min
- **Started:** 2026-04-06T03:58:00Z
- **Completed:** 2026-04-06T04:27:02Z
- **Tasks:** 3
- **Files modified:** 7
- **Files created:** 2

## Accomplishments

- 在公开文档中统一了 Phase 5 的目标后端拓扑：Go API + Go worker + PostgreSQL + Redis，并明确 Python Web UI 当前只是兼容壳 / 本地观察壳，不应再被当作最终生产后端路径。
- 新增 `scripts/verify_phase5_cutover.sh`，把 migration、runtime wiring、registration、management、payment/team 的本地 cutover 预检串成一个入口，同时把 live health / real-postgres migration honest 地标记为 gated。
- 在 `backend-go/Makefile` 增加 `verify-phase5`，让 `backend-go/` 目录下也有一致的执行入口。
- 初始化 `05-VERIFICATION.md`，把当前证据状态写成“local_checks passed / live_checks gated”，避免后续 final sign-off 时回忆式补文档。

## Files Created/Modified

- `scripts/verify_phase5_cutover.sh` - 新增 Phase 5 cutover 验证入口，覆盖本地 suite 并对 live checks 做显式 gating。
- `.planning/phases/05-cutover-and-decommission/05-VERIFICATION.md` - 新增 Phase 5 验证报告骨架，并记录本轮 local-pass / live-gated 结果。
- `backend-go/Makefile` - 新增 `verify-phase5` 入口，转发到 repo-root cutover 脚本。
- `README.md` - 增加 Phase 5 cutover 说明、Go 后端切流路径和 Python 兼容壳定位。
- `backend-go/README.md` - 更新当前 scope / not-yet-migrated 描述，并加入 cutover topology / verify-phase5 说明。
- `backend-go/docs/phase1-runbook.md` - 标注当前里程碑语境，明确该文档是 bootstrap baseline，最终 cutover 还要走 Phase 5 gate。
- `scripts/docker/start-webui.sh` - 在运行时提示当前仍是 Python compatibility shell，而非最终后端 ownership。
- `docker-compose.yml` - 标注当前 compose 仍是 Python compatibility shell 路径。

## Decisions Made

- 先修“cutover truth surface”再动生产路径去 Python 化，避免在 rollback 未立住前就删除兼容入口。
- Phase 5 verification 只对 live checks 做 gating，不对 local checks 降级；没有 `BACKEND_GO_BASE_URL` / `DATABASE_URL` 时只允许 honest skip。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 并行子代理的完成信号依旧不稳定，但其生成的脚本与 Makefile 改动已经被主线程接管、复核并补强。

## Validation

- `bash /Users/weihaiqiu/IdeaProjects/codex-console/scripts/verify_phase5_cutover.sh`  
  Result: PASS (local checks) / SKIP-GATED (live checks)
- `cd /Users/weihaiqiu/IdeaProjects/codex-console/backend-go && make verify-phase5`  
  Result: PASS (local checks) / SKIP-GATED (live checks)
- `rg -n "Phase 5|verify_phase5_cutover|兼容壳|rollback|Go-owned" /Users/weihaiqiu/IdeaProjects/codex-console/README.md /Users/weihaiqiu/IdeaProjects/codex-console/backend-go/README.md /Users/weihaiqiu/IdeaProjects/codex-console/backend-go/docs/phase1-runbook.md /Users/weihaiqiu/IdeaProjects/codex-console/scripts/docker/start-webui.sh /Users/weihaiqiu/IdeaProjects/codex-console/docker-compose.yml /Users/weihaiqiu/IdeaProjects/codex-console/.planning/phases/05-cutover-and-decommission/05-VERIFICATION.md`  
  Result: PASS

## Next Phase Readiness

- `05-02` 现在可以聚焦在真正的 production-path 去 Python 化：重写默认 startup surface、审计残余 Python bridges，并把任何保留的 Python 壳明确降为 non-critical。
- Final sign-off 仍然缺真实环境的 gated live checks；这是 `05-02` 收尾和 Phase 5 最终完成前必须补上的证据。

---
*Phase: 05-cutover-and-decommission*
*Completed: 2026-04-06*
