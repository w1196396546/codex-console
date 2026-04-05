# Technology Stack

**Analysis Date:** 2026-04-05

## Languages

**Primary:**
- Python 3.10+ - 主应用与 WebUI 运行时，入口在 `webui.py`，主体代码在 `src/`，版本要求定义在 `pyproject.toml`，容器镜像使用 `Dockerfile` 中的 Python 3.11。
- Go 1.25.0 - 独立控制 API、任务 worker、PostgreSQL/Redis 队列处理与原生注册 runner，代码位于 `backend-go/`，版本定义在 `backend-go/go.mod`。

**Secondary:**
- JavaScript / Node.js - 仅检测到工具链依赖而非前端构建系统，依赖定义在 `package.json` / `package-lock.json`，当前仓库内未检测到对应的项目级 `npm` scripts 或业务源码引用。
- SQL - Python 侧通过 SQLAlchemy 管理 SQLite / PostgreSQL，Go 侧通过 `goose` 迁移和 `sqlc` 查询生成访问 PostgreSQL，相关文件在 `src/database/session.py`、`backend-go/db/migrations/*.sql`、`backend-go/db/query/jobs.sql`、`backend-go/sqlc.yaml`。
- Shell / PowerShell - 构建与容器启动脚本使用 Bash 和 PowerShell，见 `build.sh`、`scripts/docker/start-webui.sh`、`scripts/make_windows_test_bundle.ps1`。

## Runtime

**Environment:**
- Python 运行时使用本地解释器或打包二进制，默认入口是 `webui.py`，打包配置在 `codex_register.spec`。
- Docker 运行时基于 `python:3.11-slim`，并附带 Xvfb、Fluxbox、x11vnc、websockify、noVNC 与 Playwright Chromium，定义在 `Dockerfile`。
- Go 运行时分为 API 进程 `backend-go/cmd/api/main.go` 和 worker 进程 `backend-go/cmd/worker/main.go`。
- Node.js 版本未在仓库中固定；锁定依赖中 `@puppeteer/browsers` 要求 `node >=18`，见 `package-lock.json`。

**Package Manager:**
- `uv` - Python 推荐包管理器，仓库存在锁文件 `uv.lock`，安装方式记录在 `README.md`。
- `pip` - Python 兼容安装路径，依赖清单在 `requirements.txt`。
- `npm` - Node 工具依赖管理器，锁文件 `package-lock.json` 存在。
- `go` modules - Go 依赖管理，锁定在 `backend-go/go.mod` / `backend-go/go.sum`。
- Lockfile: `uv.lock`、`package-lock.json`、`backend-go/go.sum` present。

## Frameworks

**Core:**
- FastAPI `>=0.100.0` - WebUI 与 HTTP API 框架，应用装配在 `src/web/app.py`，依赖来源 `pyproject.toml` / `requirements.txt`。
- Uvicorn `>=0.23.0` - ASGI 服务启动器，启动参数与 WebSocket 适配定义在 `webui.py`。
- Jinja2 `>=3.1.0` - 服务端模板渲染，模板目录与全局变量注入见 `src/web/app.py`。
- SQLAlchemy `>=2.0.0` - Python 侧 ORM 与 SQLite/PostgreSQL 访问层，见 `src/database/session.py`。
- Pydantic `>=2.0.0` 与 `pydantic-settings >=2.0.0` - 设置模型与配置转换，见 `src/config/settings.py`。
- `curl-cffi >=0.14.0` - Python 侧统一 HTTP 客户端、OpenAI/Auth/支付/上传/邮箱服务请求基座，见 `src/core/http_client.py` 与 `src/core/openai/*.py`。
- `websockets >=16.0` - Python WebSocket 服务端支持，启用位置在 `webui.py`。
- `go-chi/chi v5.2.3` - Go API 路由器，见 `backend-go/internal/http/router.go`。
- `hibiken/asynq v0.26.0` - Go 后端任务队列与 worker 运行时，见 `backend-go/cmd/api/main.go`、`backend-go/cmd/worker/main.go`。
- `jackc/pgx/v5 v5.9.1` - Go 侧 PostgreSQL 连接池，见 `backend-go/internal/platform/postgres/postgres.go`。
- `redis/go-redis/v9 v9.18.0` - Go 侧 Redis 客户端，见 `backend-go/internal/platform/redis/redis.go`。

**Testing:**
- `pytest >=7.0.0` - Python 测试框架，配置在 `pyproject.toml` 的 `[tool.pytest.ini_options]`。
- `httpx >=0.24.0` - Python 测试 HTTP 客户端依赖，列在 `pyproject.toml` 和 `requirements.txt`。
- Go 原生 `go test` - Go 单测、迁移测试、e2e 测试通过 `backend-go/Makefile` 组织。

**Build/Dev:**
- Hatchling - Python 包构建后端，定义在 `pyproject.toml`。
- PyInstaller `>=6.19.0` - 桌面二进制打包链路，依赖在 `pyproject.toml`，spec 文件为 `codex_register.spec`，CI 入口在 `.github/workflows/build.yml`。
- Playwright `>=1.40.0` - Python 支付/绑卡自动化与容器浏览器依赖，定义在 `pyproject.toml` 可选依赖、`requirements.txt`、`Dockerfile`。
- `sqlc` - Go 查询代码生成工具，配置在 `backend-go/sqlc.yaml`，命令在 `backend-go/Makefile`。
- `goose v3.27.0` - Go PostgreSQL 迁移工具，依赖在 `backend-go/go.mod`，命令在 `backend-go/Makefile`。
- Docker Buildx / GitHub Actions - 镜像构建与发布链路，见 `.github/workflows/docker-image.yml` 与 `.github/workflows/docker-publish.yml`。

## Key Dependencies

**Critical:**
- `curl-cffi` - 所有高仿真浏览器 HTTP 请求、OpenAI/Auth/ChatGPT/邮箱服务/上传服务都基于它实现，关键文件包括 `src/core/http_client.py`、`src/core/openai/oauth.py`、`src/core/openai/payment.py`、`src/services/tempmail.py`。
- `playwright` - 绑卡与浏览器自动化能力依赖它，相关逻辑在 `src/core/openai/browser_bind.py`、`src/core/openai/payment.py`，Docker 镜像会预装 Chromium。
- `sqlalchemy` + `aiosqlite` + `psycopg[binary]` - Python 侧本地 SQLite 与远程 PostgreSQL 双栈数据库支持，见 `src/database/session.py` 与 `src/config/settings.py`。
- `go-chi/chi`, `hibiken/asynq`, `pgx/v5`, `redis/go-redis/v9` - Go API、队列、数据库与缓存核心依赖，分别对应 `backend-go/internal/http/router.go`、`backend-go/cmd/worker/main.go`、`backend-go/internal/platform/postgres/postgres.go`、`backend-go/internal/platform/redis/redis.go`。

**Infrastructure:**
- PostgreSQL - Python 可选远程数据库、Go 强制主数据库，连接规则见 `src/database/session.py` 与 `backend-go/internal/config/config.go`。
- SQLite - Python 默认本地数据库，默认文件在 `data/database.db`，见 `src/database/session.py`。
- Redis - Go worker 队列、租约、任务状态依赖项，见 `backend-go/internal/config/config.go` 与 `backend-go/cmd/worker/main.go`。
- Xvfb / Fluxbox / x11vnc / websockify / noVNC - Docker 中提供有界面浏览器运行环境，见 `Dockerfile` 与 `scripts/docker/start-webui.sh`。
- `axios ^1.14.0` 与 `puppeteer-core ^24.40.0` - 仅在 Node 清单中声明，当前仓库未检测到业务级直接引用；将其视为保留型工具依赖，依据为 `package.json` 与 `package-lock.json`。

## Configuration

**Environment:**
- Python 主应用以数据库设置为主，环境变量只做覆盖层；实现见 `src/config/settings.py`、`webui.py`。
- 使用 `APP_DATABASE_URL` 或 `DATABASE_URL` 将 Python 默认 SQLite 切换为 PostgreSQL；数据库 URL 规范化逻辑在 `src/config/settings.py` 与 `src/database/session.py`。
- 使用 `APP_HOST`、`APP_PORT`、`APP_ACCESS_PASSWORD` 覆盖 Python WebUI 监听与认证；CLI 入口额外支持 `WEBUI_HOST`、`WEBUI_PORT`、`WEBUI_ACCESS_PASSWORD`、`DEBUG`、`LOG_LEVEL`，见 `webui.py`。
- Go 后端完全通过环境变量驱动，至少要求 `DATABASE_URL` 与 `REDIS_ADDR`；完整列表定义在 `backend-go/internal/config/config.go`。
- Docker 运行时额外依赖 `DISPLAY`、`ENABLE_VNC`、`VNC_PORT`、`NOVNC_PORT` 等图形环境变量，见 `Dockerfile`、`docker-compose.yml`、`scripts/docker/start-webui.sh`。

**Build:**
- Python 构建文件：`pyproject.toml`、`requirements.txt`、`uv.lock`、`codex_register.spec`。
- Docker 构建文件：`Dockerfile`、`docker-compose.yml`、`scripts/docker/start-webui.sh`。
- Go 构建与代码生成文件：`backend-go/go.mod`、`backend-go/Makefile`、`backend-go/sqlc.yaml`。
- CI/CD 文件：`.github/workflows/build.yml`、`.github/workflows/docker-image.yml`、`.github/workflows/docker-publish.yml`。

## Platform Requirements

**Development:**
- Python 3.10+，推荐使用 `uv sync`，也兼容 `pip install -r requirements.txt`；依据 `README.md` 与 `pyproject.toml`。
- 若需要支付/绑卡自动化，本地环境还要具备 Playwright Chromium；安装方式写在 `Dockerfile` 与 `scripts/make_windows_test_bundle.ps1`。
- Go 后端开发需要 Go 1.25.0、PostgreSQL、Redis，以及可选的 `sqlc` / `goose` 命令；依据 `backend-go/go.mod` 与 `backend-go/Makefile`。
- Docker 开发模式需要宿主机开放 `1455`、`6080`，并挂载 `./data` 与 `./logs`；见 `docker-compose.yml`。

**Production:**
- Python 应用可以直接运行 `python webui.py`、以 PyInstaller 二进制发布，或通过 GHCR Docker 镜像运行；发布链路见 `README.md`、`codex_register.spec`、`.github/workflows/build.yml`、`.github/workflows/docker-image.yml`。
- Go 后端部署目标是独立 API + worker 进程，依赖外部 PostgreSQL 与 Redis；入口在 `backend-go/cmd/api/main.go` 与 `backend-go/cmd/worker/main.go`。
- 当前仓库是混合栈而非单一迁移完成态。`backend-go/README.md` 明确标注部分注册与支付链路尚未完全迁移，worker 仍保留 Python bridge，见 `backend-go/internal/registration/python_runner.go` 与 `backend-go/internal/registration/python_runner_script.go`。

---

*Stack analysis: 2026-04-05*
