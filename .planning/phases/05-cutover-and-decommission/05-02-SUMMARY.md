---
phase: 05-cutover-and-decommission
plan: "02"
subsystem: decommission
tags: [decommission, startup-surface, python-bridge, compatibility-shell, docker]
requires:
  - phase: 05-cutover-and-decommission/05-01
    provides: cutover topology docs and Phase 5 verification harness
provides:
  - Go-first default startup surface in docker-compose
  - explicit isolation language for retained Python shell and legacy bridges
  - residual Python ownership inventory wired into Phase 5 verification
affects: [deployment defaults, python shell semantics, bridge isolation, final phase verification]
tech-stack:
  added: []
  patterns:
    - Go-first default deployment with optional compatibility-shell profile
    - explicit legacy-fallback annotation for retained Python bridges
key-files:
  created: []
  modified:
    - docker-compose.yml
    - README.md
    - backend-go/README.md
    - webui.py
    - src/web/app.py
    - backend-go/internal/registration/python_runner.go
    - src/web/routes/payment.py
    - .planning/phases/05-cutover-and-decommission/05-VERIFICATION.md
key-decisions:
  - "默认 startup surface 改为 Go API + Go worker；Python Web UI 只保留在 `compat-ui` profile 下作为兼容壳。"
  - "不强拆 Python bridge 代码，而是先把它们显式降级为 transition-only / legacy fallback，并从默认生产路径切开。"
patterns-established:
  - "Phase 5 的 repo-side 完成标准是：默认部署面 Go-first，残余 Python 能力有明确 disposition，而不是代码必须全部删除。"
  - "最终 phase sign-off 仍需 live-gated 证据，但那不阻止先完成 repo 内的 decommission/isolation 工作。"
requirements-completed: [CUT-02]
duration: 25min
completed: 2026-04-06
---

# Phase 05 Plan 02: Production-Path Decommission Summary

**默认启动面已经从 Python-first 改成 Go-first；残余 Python 代码也被明确降成 compatibility shell / legacy fallback，而不是继续模糊地占着生产关键路径。**

## Performance

- **Duration:** 25 min
- **Started:** 2026-04-06T04:08:00Z
- **Completed:** 2026-04-06T04:32:54Z
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- 将 `docker-compose.yml` 的默认启动面改为 `postgres + redis + go-api + go-worker`，并把 Python Web UI 放入 `compat-ui` profile，避免默认路径继续是 Python-first。
- 在 `webui.py` 和 `src/web/app.py` 增加运行时/文档级提示，明确 Python app 现在是 compatibility shell / presentation shell，不再应被视为生产后端真值源。
- 在 `backend-go/internal/registration/python_runner.go` 和 `src/web/routes/payment.py` 的关键 bridge helper 处补上 legacy/transition-only 边界说明，给残余 Python ownership 一个可审计的最终 disposition。
- 更新 `05-VERIFICATION.md` 的 residual Python inventory，让 final sign-off 可以明确说明：哪些 Python 代码仍保留、为什么保留、为什么不再是 backend critical path。

## Files Modified

- `docker-compose.yml` - 默认启动面改为 Go-first，并把 Python Web UI 降为 `compat-ui` profile。
- `README.md` - 更新 compose 默认拓扑与 Python 兼容壳说明。
- `backend-go/README.md` - 补充 compose 默认面已切到 Go topology。
- `webui.py` - 在 Python Web UI 启动日志中明确 compatibility-shell / non-critical 定位。
- `src/web/app.py` - 在模块文档和 startup 日志中声明 Python app 的 compatibility-shell 角色。
- `backend-go/internal/registration/python_runner.go` - 明确标注 transition-only compatibility bridge 语义。
- `src/web/routes/payment.py` - 为 `abcard_bridge` 会话补全 helper 标注 legacy fallback 角色。
- `.planning/phases/05-cutover-and-decommission/05-VERIFICATION.md` - 记录残余 Python ownership inventory 与当前验证结论。

## Decisions Made

- 不把 05-02 扩成前端重写；只切 startup surface 和 backend ownership。
- Python bridge 先做显式隔离与默认路径切断，不在本回合强行删除所有残余代码。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 当前工作区没有真实 cutover 环境，所以 Phase 5 最终 sign-off 仍然受 `BACKEND_GO_BASE_URL` / `MIGRATION_TEST_DATABASE_URL` gating 约束。

## Validation

- `python -m py_compile /Users/weihaiqiu/IdeaProjects/codex-console/webui.py /Users/weihaiqiu/IdeaProjects/codex-console/src/web/app.py /Users/weihaiqiu/IdeaProjects/codex-console/src/web/routes/payment.py`  
  Result: PASS
- `bash /Users/weihaiqiu/IdeaProjects/codex-console/scripts/verify_phase5_cutover.sh`  
  Result: PASS (local checks) / SKIP-GATED (live checks)
- `python - <<'PY' ... yaml.safe_load(open('docker-compose.yml')) ... PY`  
  Result: PASS (`YAML_OK`)
- `rg -n "python_runner|bridge|abcard_bridge|fallback|legacy" /Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/python_runner.go /Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/python_runner_script.go /Users/weihaiqiu/IdeaProjects/codex-console/src/web/routes/payment.py /Users/weihaiqiu/IdeaProjects/codex-console/.planning/phases/05-cutover-and-decommission/05-VERIFICATION.md`  
  Result: PASS
- `rg -n "critical path|presentation shell|compatibility-shell|compatibility shell|non-critical|rollback" /Users/weihaiqiu/IdeaProjects/codex-console/webui.py /Users/weihaiqiu/IdeaProjects/codex-console/src/web/app.py /Users/weihaiqiu/IdeaProjects/codex-console/.planning/phases/05-cutover-and-decommission/05-VERIFICATION.md`  
  Result: PASS

## Next Phase Readiness

- Repo-side Phase 5 plans are now both implemented.
- 整个 Phase 5 还不能宣称完成，因为最终 live-gated cutover/rollback 证据仍未在真实环境跑出来。
- 下一步不再是继续写代码，而是补真实环境验证并把 `05-VERIFICATION.md` 从 `in_progress` 推到最终状态。

---
*Phase: 05-cutover-and-decommission*
*Completed: 2026-04-06*
