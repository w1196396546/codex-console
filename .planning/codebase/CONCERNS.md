# Codebase Concerns

**Analysis Date:** 2026-04-05

## Tech Debt

**Monolithic Python flow modules:**
- Issue: 核心注册、支付、账号管理、批量任务逻辑集中在超大单文件里，HTTP 路由、业务编排、外部调用、数据回写混在一起。
- Files: `src/web/routes/payment.py`, `src/core/register.py`, `src/web/routes/accounts.py`, `src/web/routes/registration.py`
- Impact: 改一个分支容易牵动整条链路，回归面非常大；Mock 测试能覆盖局部分支，但很难证明真实链路稳定。
- Fix approach: 先按“路由层 / service 层 / provider 层 / persistence 层”拆分，再把 OpenAI 支付和注册状态机从路由文件中抽离。

**Dual backend split without a single source of truth:**
- Issue: 仓库同时维护 Python Web/API 路径和 Go API/worker 路径，Go 侧明确仍依赖 Python legacy worker bridge，关键注册/支付流程没有迁完。
- Files: `backend-go/README.md`, `backend-go/internal/registration/python_runner.go`, `backend-go/cmd/api/main.go`, `src/web/app.py`, `src/web/routes/registration.py`
- Impact: 业务语义、数据模型、错误处理和迁移策略容易双向漂移；后续修复必须同时判断两套实现是否一致。
- Fix approach: 明确“生产主路径”与“迁移路径”，给 Go 侧设清晰边界；未迁移功能不要再复制 Python 逻辑，而是通过稳定接口桥接。

**Two schema migration systems for one product:**
- Issue: Python 侧在启动时对 SQLite 做运行时 `ALTER TABLE` 修补，Go 侧使用 Goose 维护 PostgreSQL 迁移，缺少统一 schema contract。
- Files: `src/database/session.py`, `src/database/init_db.py`, `backend-go/db/migrations/0001_init_jobs.sql`, `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`
- Impact: SQLite 与 PostgreSQL 的字段、索引、约束可能逐步失配；升级路径依赖“启动时碰运气修补”，排障成本高。
- Fix approach: 把 schema 变更收敛到显式迁移文件；Python 路径至少把启动期自动修补改成可审计迁移脚本。

## Known Bugs

**`--access-password` is persisted even though README documents it as one-run only:**
- Symptoms: 启动参数或 `WEBUI_ACCESS_PASSWORD` 会被写回数据库，后续重启继续生效，不是一次性覆盖。
- Files: `README.md`, `webui.py`, `src/config/settings.py`
- Trigger: 使用 `python webui.py --access-password ...` 或设置 `WEBUI_ACCESS_PASSWORD` 启动。
- Workaround: 修改后若希望恢复数据库内原值，只能再通过设置接口或数据库直接改回；当前实现不支持真正的“本次启动覆盖”。

**`check_database_connection()` is broken with SQLAlchemy 2.x:**
- Symptoms: 连接检查路径执行 `db.execute("SELECT 1")`，在当前依赖 `sqlalchemy>=2.0.0` 下会抛 `ArgumentError`，不能作为可用健康检查。
- Files: `src/database/init_db.py`, `pyproject.toml`
- Trigger: 运行 `python -m src.database.init_db --check` 或调用 `check_database_connection()`。
- Workaround: 使用真实启动流程验证数据库连通性，或把实现改成 `text("SELECT 1")` 后再使用。

**SQLite backup endpoint can produce incomplete backups while WAL is enabled:**
- Symptoms: 备份接口只复制主 `.db` 文件，没有 checkpoint，也不复制 `-wal` / `-shm`，最近提交的数据可能不在备份里。
- Files: `src/database/session.py`, `src/web/routes/settings.py`
- Trigger: 应用运行中调用 `/api/settings/database/backup`，且 SQLite 仍在 WAL 模式写入。
- Workaround: 停机后做文件级备份，或先执行 checkpoint 再复制主文件和 sidecar 文件。

## Security Considerations

**API surface is unauthenticated while sensitive endpoints remain mounted:**
- Risk: 页面层有登录页和 `webui_auth` cookie，但 `/api` 路由没有统一鉴权依赖；任何能访问服务端口的人都能直接读写高敏感接口。
- Files: `src/web/app.py`, `src/web/routes/__init__.py`, `src/web/routes/accounts.py`, `src/web/routes/logs.py`, `src/web/routes/upload/cpa_services.py`, `src/web/routes/upload/sub2api_services.py`, `src/web/routes/upload/tm_services.py`
- Current mitigation: HTML 页面使用 `_is_authenticated()` 做重定向；没有看到 API 级别的 `Depends(...)`、中间件或路由守卫。
- Recommendations: 为 `/api` 和 `/api/ws` 增加统一认证中间件/依赖；默认拒绝未认证访问；把导出、日志清理、数据库导入这类高危接口再细分为管理员权限。

**Secrets are stored in plaintext across accounts, services, proxy settings, and app settings:**
- Risk: 账户密码、`access_token`、`refresh_token`、`session_token`、完整 cookies、CPA/Sub2API/Team Manager key、代理密码、动态代理 key 和 Web UI 访问密码都直接进数据库文本字段。
- Files: `src/database/models.py`, `src/config/settings.py`, `src/database/crud.py`, `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`
- Current mitigation: `SecretStr` 只是在内存里包装和掩码显示；`_value_to_string()` 仍然把真实值写入 `settings.value`。
- Recommendations: 对落库 secret 做加密存储或外置 secret store；收缩 `/full` 配置接口；默认不要导出密码、cookie、refresh token。

**Sensitive data is exposed through first-class read/export endpoints:**
- Risk: 未鉴权情况下可直接读取单账号 token/cookie、批量导出 JSON/CSV/Codex/Sub2API/CPA 格式、读取完整第三方服务 key。
- Files: `src/web/routes/accounts.py`, `src/web/routes/upload/cpa_services.py`, `src/web/routes/upload/sub2api_services.py`
- Current mitigation: 列表接口会返回 `has_token` / `has_key` 之类的掩码布尔值，但同一模块仍保留 `/{id}/full` 与导出接口返回真实 secret。
- Recommendations: 删除或强权限保护明文接口；导出前要求二次确认并记录审计日志；默认导出改成脱敏格式。

**Weak default auth configuration and under-hardened cookie setup:**
- Risk: 默认 `webui_access_password` 为 `admin123`，`webui_secret_key` 也是固定默认值；登录 cookie 未设置 `secure=True`，CORS 还是 `allow_origins=["*"]` + `allow_credentials=True`。
- Files: `README.md`, `src/config/settings.py`, `src/web/app.py`
- Current mitigation: 页面层至少使用 `httponly` + `samesite="lax"`；没有看到启动时强制更改默认密码/密钥的机制。
- Recommendations: 启动时检测并拒绝默认密码与默认 secret；按部署环境设置 `secure=True`；把 CORS 限制到明确来源。

**Runtime logs can contain secrets and are queryable through the API:**
- Risk: 注册流程会把生成的密码写入日志，数据库日志处理器会把应用日志原文持久化；日志接口可分页读取、统计、清空。
- Files: `src/core/register.py`, `src/core/db_logs.py`, `src/web/routes/logs.py`
- Current mitigation: 邮箱服务配置在 `src/web/routes/email.py` 有单独字段掩码，但日志系统没有全局 secret redaction。
- Recommendations: 对密码、token、cookie、Authorization 头、API key 做统一脱敏；把日志接口放到认证和审计之下。

## Performance Bottlenecks

**SQLite write contention is a known operating mode, not an edge case:**
- Problem: Python 路径明显围绕 “database is locked” 做补丁式重试，说明注册、日志、任务状态更新已经持续撞到 SQLite 单写瓶颈。
- Files: `src/database/crud.py`, `src/database/session.py`, `src/web/routes/registration.py`, `tests/test_sqlite_lock_mitigation.py`, `tests/test_registration_routes.py`
- Cause: 默认本地存储仍以 SQLite 为主，任务线程、日志写入、状态回写同时竞争同一个文件数据库；当前仅靠 WAL、busy timeout 和 4 次重试缓解。
- Improvement path: 把高频日志和任务状态迁到外部队列/数据库；默认生产部署转 PostgreSQL；减少同步 commit 次数。

**Task execution is bounded by one process and one in-memory runtime map:**
- Problem: `ThreadPoolExecutor(max_workers=50)`、任务状态、日志缓存、批量状态、WebSocket 连接全是进程内全局对象，进程越忙越吃内存，重启即丢运行态。
- Files: `src/web/task_manager.py`, `src/web/routes/registration.py`
- Cause: 运行态没有外部协调层，`batch_tasks` 和 `TaskManager` 只在当前 Python 进程里存在；完成后也没有系统性清理批量快照。
- Improvement path: 把任务控制迁到持久化 job system（Redis/PostgreSQL/Go worker）；对日志和批量状态做 TTL 与后台清理。

## Fragile Areas

**Registration and payment automation state machines:**
- Files: `src/core/register.py`, `src/web/routes/payment.py`, `src/core/openai/browser_bind.py`, `src/core/anyauto/oauth_client.py`
- Why fragile: 这几处既依赖 OpenAI 页面/接口行为，又包含大量 fallback、重试、代理切换、cookie/session 拼装；同时存在大量 `except Exception: pass` 与静默降级。
- Safe modification: 只做小范围改动，先固定一条链路再扩展；每次修改都补同层级测试和一条真实 smoke check。
- Test coverage: `tests/test_registration_engine.py`, `tests/test_anyauto_register_flow.py`, `tests/test_payment_routes.py` 主要是 mock 驱动，缺少真实外部链路回归。

**Batch/task runtime state and websocket fan-out:**
- Files: `src/web/task_manager.py`, `src/web/routes/registration.py`
- Why fragile: 任务状态既写数据库，又写全局 dict，又写 WebSocket 缓冲；重启、线程异常、批量取消/恢复都可能让三份状态不一致。
- Safe modification: 先定义“数据库为准”还是“内存为准”；不要再增加新的旁路状态容器。
- Test coverage: 没有看到“进程重启后恢复中任务 / 批量任务”的验证。

**Direct export/import and service-config endpoints:**
- Files: `src/web/routes/accounts.py`, `src/web/routes/settings.py`, `src/web/routes/upload/cpa_services.py`, `src/web/routes/upload/sub2api_services.py`, `src/web/routes/upload/tm_services.py`
- Why fragile: 这些接口直接拼装明文凭证、导出不同第三方格式、导入覆盖数据库文件；一旦字段变化或权限边界变化，最容易出现数据兼容和泄露问题。
- Safe modification: 变更字段前先收敛 DTO；对导出格式做快照测试；数据库导入导出必须补一致性验证。
- Test coverage: `tests/test_settings_routes.py` 只覆盖少量注册配置字段，没覆盖数据库备份/导入与 secret readback。

## Scaling Limits

**Python registration runtime:**
- Current capacity: 单进程线程池上限 50 个 worker，批量日志历史上限 1000 条，运行态全部保存在 `src/web/task_manager.py` 和 `src/web/routes/registration.py` 的进程内对象中。
- Limit: 进程重启后运行态丢失；多实例部署无法共享暂停/取消/日志索引；线程数继续上调会把 SQLite 锁冲突和外部站点限流一起放大。
- Scaling path: 引入持久化任务队列和集中状态存储；把长耗时注册链路从 Web 进程分离。

**SQLite-first storage path:**
- Current capacity: `src/database/session.py` 把 SQLite 调到 WAL + 30s busy timeout，并在 `src/database/crud.py` 对锁冲突重试 4 次。
- Limit: 高并发写入场景仍然只有单写者；日志、任务、账号、设置共用一个库文件，吞吐和恢复都受限。
- Scaling path: 生产默认切 PostgreSQL；把高频日志和任务事件转为 append-only event store 或消息队列。

## Dependencies at Risk

**Browser-emulation stack (`curl-cffi`, optional `playwright`, `puppeteer-core`):**
- Risk: 账号注册、支付、Token 刷新、Team 接口和浏览器绑卡高度依赖浏览器模拟栈与上游反爬行为；上游接口一变，核心路径会一起失效。
- Impact: 注册链路、支付链路、Cloudflare/checkout 绕过、浏览器补会话都会先出问题。
- Migration plan: 继续保留这些依赖时，至少把它们封装在明确适配层并加 smoke checks；更长期要减少对页面级自动化的强耦合。
- Files: `pyproject.toml`, `package.json`, `src/core/http_client.py`, `src/core/openai/payment.py`, `src/core/openai/browser_bind.py`, `src/core/register.py`

## Missing Critical Features

**Central API authorization and audit trail:**
- Problem: 当前没有统一 API 认证/鉴权层，也没有针对敏感导出、日志清理、数据库导入的审计日志。
- Blocks: 不能安全地把当前服务暴露到共享网络、反向代理或多用户环境。

**Crash-safe task orchestration for the Python path:**
- Problem: 任务暂停/恢复/取消/日志订阅依赖进程内状态，没有可恢复的 job lease、checkpoint 或重放机制。
- Blocks: 无法安全做进程重启、滚动升级、水平扩容，也不适合长时间批量任务。

## Test Coverage Gaps

**Auth and secret-exposure regression tests are missing:**
- What's not tested: `/api` 未登录访问是否被拒绝、`/{service_id}/full` 是否需要更高权限、导出和日志接口是否做二次确认。
- Files: `src/web/app.py`, `src/web/routes/accounts.py`, `src/web/routes/logs.py`, `src/web/routes/upload/cpa_services.py`, `src/web/routes/upload/sub2api_services.py`, `src/web/routes/upload/tm_services.py`
- Risk: 当前敏感接口默认开放的状态很容易长期存在，后续改动也没有自动化防线。
- Priority: High

**Backup/import and schema-upgrade paths lack end-to-end verification:**
- What's not tested: SQLite WAL 场景下的备份完整性、数据库导入覆盖后的可读性、运行时迁移与旧库升级兼容性。
- Files: `src/web/routes/settings.py`, `src/database/session.py`, `src/database/init_db.py`
- Risk: 备份恢复和版本升级问题容易等到线上数据异常时才暴露。
- Priority: High

**Real external flow regression coverage is still thin in the highest-risk modules:**
- What's not tested: `src/core/register.py`, `src/web/routes/payment.py`, `src/core/openai/browser_bind.py` 对真实 OpenAI 页面/接口变化的兼容性。
- Files: `src/core/register.py`, `src/web/routes/payment.py`, `src/core/openai/browser_bind.py`, `tests/test_anyauto_register_flow.py`, `tests/test_payment_routes.py`, `tests/test_registration_engine.py`
- Risk: mock 测试通过并不代表真实链路可用，尤其是在上游页面和反爬策略频繁变化时。
- Priority: High

---

*Concerns audit: 2026-04-05*
