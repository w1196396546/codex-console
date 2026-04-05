# External Integrations

**Analysis Date:** 2026-04-05

## APIs & External Services

**OpenAI / ChatGPT account flow:**
- OpenAI Auth / Accounts API - 注册、登录、OAuth、OTP、workspace 选择和 token 刷新主链路。
  - SDK/Client: `curl_cffi` + 自定义客户端，核心实现位于 `src/core/register.py`、`src/core/anyauto/chatgpt_client.py`、`src/core/openai/oauth.py`、`src/core/openai/token_refresh.py`、`backend-go/internal/nativerunner/auth/*.go`。
  - Auth: `openai.client_id`、`openai.auth_url`、`openai.token_url`、`openai.redirect_uri` 存在于 `src/config/settings.py`；运行态还会使用账号级 `access_token`、`refresh_token`、`session_token`、`client_id`，模型字段见 `backend-go/db/migrations/0003_extend_registration_service_configs.sql` 与 Python 账户模型相关持久化代码。
- Sentinel API - OpenAI Sentinel POW 求解与请求签名。
  - SDK/Client: `src/core/openai/sentinel.py`、`src/core/http_client.py`、`src/core/openai/payment.py`。
  - Auth: 依赖 OpenAI token、设备指纹和浏览器请求头；无单独 env var。
- ChatGPT backend APIs - 订阅检查、账户概览、支付 checkout、session 与 Team 账户接口。
  - SDK/Client: `src/core/openai/payment.py`、`src/core/openai/overview.py`、`src/services/team/client.py`、`backend-go/internal/nativerunner/auth/*.go`。
  - Auth: 账号 `access_token` / `session_token`；Team API 通过 `Authorization: Bearer` 调用，见 `src/services/team/client.py`。

**Microsoft / Outlook mail access:**
- Microsoft Graph API - Outlook 邮件拉取与验证码轮询。
  - SDK/Client: `src/services/outlook/providers/graph_api.py`。
  - Auth: Outlook 账号级 `client_id` + `refresh_token`，token 刷新逻辑在 `src/services/outlook/token_manager.py`。
- Microsoft OAuth token endpoints - Outlook Graph/IMAP OAuth token 交换。
  - SDK/Client: `src/services/outlook/token_manager.py`、`src/services/outlook/base.py`。
  - Auth: `outlook.default_client_id` 默认配置见 `src/config/settings.py`；每个 Outlook 服务项也可带 `client_id`。
- Outlook IMAP/SMTP - Outlook 邮箱验证码收取与兼容回退。
  - SDK/Client: Python 侧 `src/services/outlook/providers/imap_old.py`、`src/services/outlook/providers/imap_new.py`、`src/services/imap_mail.py`；Go 侧 `backend-go/internal/nativerunner/mail/outlook.go`。
  - Auth: Outlook 邮箱密码或 OAuth refresh token；服务项数据通过数据库配置注入。

**Temporary email services:**
- Tempmail.lol - 默认临时邮箱服务。
  - SDK/Client: `src/services/tempmail.py`、Go native provider `backend-go/internal/nativerunner/mail/tempmail.go`。
  - Auth: 可选 `TEMPMail_API_KEY` 环境变量，或数据库/设置中的 `tempmail.*` 配置；实现见 `src/services/tempmail.py`、`src/config/settings.py`。
- YYDS Mail - 临时邮箱 REST API。
  - SDK/Client: `src/services/yyds_mail.py`、Go provider `backend-go/internal/nativerunner/mail/yydsmail.go`。
  - Auth: `yyds_mail.api_key` 与 `yyds_mail.base_url`，配置定义在 `src/config/settings.py`。
- LuckMail - 接码服务，支持本地 vendored SDK 或环境安装 SDK。
  - SDK/Client: `src/services/luckmail_mail.py`、Go provider `backend-go/internal/nativerunner/mail/luckmail.go`。
  - Auth: `api_key`、`project_code`、`email_type`、`preferred_domain`，默认值定义在 `src/config/constants.py`。
- Freemail - 自部署 Cloudflare Worker 邮箱服务。
  - SDK/Client: `src/services/freemail.py`、Go provider `backend-go/internal/nativerunner/mail/freemail.go`。
  - Auth: `admin_token` / `JWT_TOKEN`，以及 `base_url`、`domain`，由数据库邮箱服务配置提供。
- Temp-Mail / CloudMail - 自部署 Worker 风格临时邮箱。
  - SDK/Client: `src/services/temp_mail.py`、`src/services/cloudmail.py`、Go providers `backend-go/internal/nativerunner/mail/moemail.go` 等兼容实现。
  - Auth: `admin_password`、`domain`、可选 `custom_auth`；CloudMail 复用 Temp-Mail 协议。
- DuckMail / MoeMail / generic IMAP - 兼容更多私有邮箱服务和固定 IMAP 收件箱。
  - SDK/Client: `src/services/duck_mail.py`、`src/services/moe_mail.py`、`src/services/imap_mail.py`、Go provider 工厂 `backend-go/internal/nativerunner/mail/provider.go`。
  - Auth: 各服务分别使用 `api_key`、`base_url`、`default_domain`、`host`、`email`、`password` 等数据库配置。

**Upload targets and account distribution:**
- CPA - 账号导出到 `auth-files` 风格服务站。
  - SDK/Client: Python `src/core/upload/cpa_upload.py`，Go `backend-go/internal/uploader/sender.go`、`backend-go/internal/uploader/client.go`。
  - Auth: Bearer token；服务配置存储在 `cpa_services` 表，结构定义在 `backend-go/db/migrations/0003_extend_registration_service_configs.sql`。
- Sub2API / NewAPI - 批量账号导入目标。
  - SDK/Client: Python `src/core/upload/sub2api_upload.py`，Go `backend-go/internal/uploader/builder.go`、`backend-go/internal/uploader/sender.go`。
  - Auth: `x-api-key`；服务配置位于 `sub2api_services` 表，`target_type` 支持 `sub2api` 与 `newapi`。
- Team Manager - 账号导入 `/admin/teams/import`。
  - SDK/Client: Python `src/core/upload/team_manager_upload.py`，Go `backend-go/internal/uploader/sender.go`。
  - Auth: `X-API-Key`；服务配置位于 `tm_services` 表。

**Proxy and browser automation:**
- Dynamic Proxy API - 任务开始前从外部接口拉取代理 URL。
  - SDK/Client: `src/core/dynamic_proxy.py`。
  - Auth: `proxy.dynamic_api_key`，头名由 `proxy.dynamic_api_key_header` 定义，配置在 `src/config/settings.py`。
- Playwright / browser automation - 自动绑卡、checkout 打开、容器可视化浏览器。
  - SDK/Client: `src/core/openai/browser_bind.py`、`src/core/openai/payment.py`、`Dockerfile`、`scripts/docker/start-webui.sh`。
  - Auth: 不使用单独服务密钥，依赖账号 token、代理与本地 Chromium。
- BitBrowser helper - 可选的浏览器控制桥接脚本。
  - SDK/Client: `scripts/bitbrowser_connect.py`。
  - Auth: 由调用方提供 URL 和请求参数，仓库未固定专用密钥名。

## Data Storage

**Databases:**
- SQLite - Python 默认本地数据库。
  - Connection: 默认 `data/database.db`；可通过 `APP_DATABASE_URL` 或 `DATABASE_URL` 覆盖，规则见 `src/database/session.py` 与 `src/config/settings.py`。
  - Client: SQLAlchemy 2.x，见 `src/database/session.py`。
- PostgreSQL - Python 可选远程数据库，也是 Go 后端的主数据库。
  - Connection: Python 使用 `APP_DATABASE_URL` / `DATABASE_URL`；Go 必须提供 `DATABASE_URL`，见 `src/database/session.py` 与 `backend-go/internal/config/config.go`。
  - Client: Python 使用 `psycopg[binary]` 经 SQLAlchemy 访问；Go 使用 `pgxpool`，见 `backend-go/internal/platform/postgres/postgres.go`。

**File Storage:**
- Local filesystem only。
  - Python 默认持久化目录是 `data/` 与 `logs/`，由 `webui.py` 创建。
  - Docker 将 `./data` 和 `./logs` 挂载到容器 `/app/data` 与 `/app/logs`，见 `docker-compose.yml`。

**Caching:**
- Redis - Go 后端任务队列、worker 租约和 token completion 协调依赖 Redis。
  - Connection: `REDIS_ADDR`、`REDIS_PASSWORD`、`REDIS_DB` 等在 `backend-go/internal/config/config.go`。
  - Client: `redis/go-redis/v9` 和 `hibiken/asynq`，见 `backend-go/cmd/api/main.go`、`backend-go/cmd/worker/main.go`。
- Python WebUI - 未检测到独立外部缓存服务；状态主要落库和内存管理。

## Authentication & Identity

**Auth Provider:**
- WebUI password auth - 自定义 cookie + HMAC 认证。
  - Implementation: `src/web/app.py` 使用 `webui_secret_key` 与 `webui_access_password` 生成 `webui_auth` cookie。
- OpenAI OAuth / session auth - 账号注册、登录、续期的核心身份系统。
  - Implementation: Python 路径在 `src/core/openai/oauth.py`、`src/core/openai/token_refresh.py`、`src/core/register.py`；Go 路径在 `backend-go/internal/nativerunner/auth/*.go`。
- Outlook service auth - 每个 Outlook 服务支持密码型 IMAP 或 OAuth 型 Graph/IMAP。
  - Implementation: `src/services/outlook/account.py`、`src/services/outlook/token_manager.py`、`src/services/outlook/providers/*.py`。

## Monitoring & Observability

**Error Tracking:**
- None - 未检测到 Sentry、Honeycomb、Datadog SaaS 等外部错误跟踪服务。

**Logs:**
- Python - 文件日志、数据库日志和任务日志并存；入口在 `webui.py`、`src/core/db_logs.py`、`src/web/task_manager.py`。
- Go - 标准库 `log` + HTTP/WebSocket 实时任务状态流，入口在 `backend-go/cmd/api/main.go`、`backend-go/cmd/worker/main.go`、`backend-go/internal/registration/ws/*`。

## CI/CD & Deployment

**Hosting:**
- GitHub Releases - 多平台二进制打包与发布，见 `.github/workflows/build.yml`。
- GitHub Container Registry (`ghcr.io`) - Docker 镜像构建与推送，见 `.github/workflows/docker-image.yml` 与 `.github/workflows/docker-publish.yml`。
- Docker Compose / bare process - 本地或服务器运行模式，见 `docker-compose.yml` 与 `README.md`。

**CI Pipeline:**
- GitHub Actions - Python 打包、Docker 镜像构建和发布均在仓库内定义。
  - `.github/workflows/build.yml`: Python 3.11 + PyInstaller 打包。
  - `.github/workflows/docker-image.yml`: 使用 `GHCR_TOKEN` 推送 GHCR。
  - `.github/workflows/docker-publish.yml`: 使用 `GITHUB_TOKEN` 推送 GHCR。

## Environment Configuration

**Required env vars:**
- Python WebUI 常用：
  - `APP_DATABASE_URL` / `DATABASE_URL` - 切换 Python 数据库连接，见 `src/config/settings.py`、`src/database/session.py`。
  - `APP_HOST` / `APP_PORT` / `APP_ACCESS_PASSWORD` - 覆盖 WebUI 监听与访问密码，见 `src/config/settings.py`。
  - `WEBUI_HOST` / `WEBUI_PORT` / `WEBUI_ACCESS_PASSWORD` / `DEBUG` / `LOG_LEVEL` - CLI 启动覆盖项，见 `webui.py`。
  - `APP_DATA_DIR` / `APP_LOGS_DIR` - 运行目录注入，见 `webui.py`。
  - `DISPLAY` / `ENABLE_VNC` / `VNC_PORT` / `NOVNC_PORT` - Docker 图形环境变量，见 `Dockerfile` 与 `scripts/docker/start-webui.sh`。
  - `TEMPMail_API_KEY` - Tempmail.lol 可选 API Key，见 `src/services/tempmail.py`。
- Go backend 必需：
  - `DATABASE_URL`、`REDIS_ADDR` - Go API / worker 启动硬要求，见 `backend-go/internal/config/config.go`。
- Go backend 可选：
  - `APP_ENV`、`HTTP_ADDR`、`POSTGRES_MIN_CONNS`、`POSTGRES_MAX_CONNS`、`WORKER_CONCURRENCY`、`REDIS_PASSWORD`、`REDIS_DB`、`REDIS_DIAL_TIMEOUT`、`REDIS_READ_TIMEOUT`、`REDIS_WRITE_TIMEOUT`，见 `backend-go/internal/config/config.go`。
- CI secrets：
  - `GHCR_TOKEN` - `docker-image.yml` 使用。
  - `GITHUB_TOKEN` - `docker-publish.yml` 使用。

**Secrets location:**
- Python 全局设置和部分外部服务凭据默认存储在数据库 settings 表，由 `src/config/settings.py` 管理。
- 邮箱服务、CPA、Sub2API、TM 等目标凭据存储在数据库服务配置表中，结构可见 `backend-go/db/migrations/0003_extend_registration_service_configs.sql`。
- 根目录 `.env.example` 与 `backend-go/.env.example` 存在，但按规则未读取内容；应将其视为环境配置模板文件。

## Webhooks & Callbacks

**Incoming:**
- OpenAI OAuth callback - 默认回调地址是 `http://localhost:1455/auth/callback`，见 `src/config/constants.py`、`src/config/settings.py`、`src/core/openai/oauth.py`、`backend-go/internal/nativerunner/auth/login_password.go`。
- Realtime task callbacks - 本地 WebSocket 推送端点 `/api/ws/task/{task_uuid}` 与 `/api/ws/batch/{batch_id}`，见 `src/web/routes/websocket.py`、`backend-go/internal/http/router.go`。
- Third-party webhook endpoints: None detected。

**Outgoing:**
- 注册、登录、支付、概览、Team、邮箱服务、动态代理与上传目标全部通过应用主动发起 HTTP(S) 请求；关键实现散布于 `src/core/openai/*.py`、`src/services/*.py`、`src/core/upload/*.py`、`backend-go/internal/uploader/*.go`。
- 未检测到面向第三方系统的 webhook 推送实现；当前对外主要是轮询、上传和同步接口调用。

---

*Integration audit: 2026-04-05*
