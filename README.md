# codex-console

基于 [cnlimiter/codex-manager](https://github.com/cnlimiter/codex-manager) 持续修复和维护的增强版本。

这个版本的目标很直接: 把近期 OpenAI 注册链路里那些“昨天还能跑，今天突然翻车”的坑补上，让注册、登录、拿 token、打包运行都更稳一点。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Python](https://img.shields.io/badge/Python-3.10%2B-blue.svg)](https://www.python.org/)

- GitHub Repo: [https://github.com/dou-jiang/codex-console](https://github.com/dou-jiang/codex-console)

## QQ群

- 交流群: [291638849（点击加群）](https://qm.qq.com/q/4TETC3mWco)
- Telegram 频道: [codex_console](https://t.me/codex_console)

## 致谢

首先感谢上游项目作者 [cnlimiter](https://github.com/cnlimiter) 提供的优秀基础工程。

本仓库是在原项目思路和结构之上进行兼容性修复、流程调整和体验优化，适合作为一个“当前可用的修复维护版”继续使用。

## 版本更新

### v1.0

1. 新增 Sentinel POW 求解逻辑  
   OpenAI 现在会强制校验 Sentinel POW，原先直接传空值已经不行了，这里补上了实际求解流程。

2. 注册和登录拆成两段  
   现在注册完成后通常不会直接返回可用 token，而是跳转到绑定手机或后续页面。  
   本分支改成“先注册成功，再单独走一次登录流程拿 token”，避免卡死在旧逻辑里。

3. 去掉重复发送验证码  
   登录流程里服务端本身会自动发送验证码邮件，旧逻辑再手动发一次，容易让新旧验证码打架。  
   现在改成直接等待系统自动发来的那封验证码邮件。

4. 修复重新登录流程的页面判断问题  
   针对重新登录时页面流转变化，调整了登录入口和密码提交逻辑，减少卡在错误页面的情况。

5. 优化终端和 Web UI 提示文案  
   保留可读性的前提下，把一些提示改得更友好一点，出错时至少不至于像在挨骂。

### v1.1

1. 修复注册流程中的问题，解决 Outlook 和临时邮箱收不到邮件导致注册卡住、无法完成注册的问题。

2. 修复无法检查订阅状态的问题，提升订阅识别和状态检查的可用性。

3. 新增绑卡半自动模式，支持自动随机地址；3DS 无法跳过，需按实际流程完成验证。

4. 新增已订阅账号管理功能，支持查看和管理账号额度。

5. 新增后台日志功能，并补充数据导出与导入能力，方便排查问题和迁移数据。

6. 优化部分 UI 细节与交互体验，减少页面操作时的割裂感。

7. 补充细节稳定性处理，尽量减少注册、订阅检测和账号管理过程中出现卡住或误判的情况。

### v1.1.1

1. 新增 `CloudMail` 邮箱服务实现，并完成服务注册、配置接入、邮件轮询、验证码提取和基础收件处理能力。

2. 新增上传目标 `newApi` 支持，可根据配置选择不同导入目标类型。

3. 新增 `Codex` 账号导出格式，支持后续登录、迁移和导入使用。

4. 新增 `CPA` 认证文件 `proxy_url` 支持，现可在 CPA 服务配置中保存和使用代理地址。

5. 优化 OAuth token 刷新兼容逻辑，完善异常返回与一次性令牌场景处理，降低刷新报错概率。

6. 优化批量验证流程，改为受控并发执行，减少长时间阻塞和卡死问题。

7. 修复模板渲染兼容问题，提升不同 Starlette 版本下页面渲染稳定性。

8. 修复六位数字误判为 OTP 的问题，避免邮箱域名或无关文本中的六位数字被错误识别为验证码。

9. 新增 Outlook 账户“注册状态”识别与展示功能，可直接看到“已注册/未注册”，并支持显示关联账号编号（如“已注册 #1”）。

10. 修复 Outlook 邮箱匹配大小写问题，避免 Outlook.com 因大小写差异被误判为未注册。

11. 修复 Outlook 列表列错位、乱码和占位文案问题，恢复中文显示并优化列表信息布局。

12. 优化 WebUI 端口冲突处理，默认端口占用时自动切换可用端口。

13. 增加启动时轻量字段迁移逻辑，自动补齐新增字段，提升旧数据升级兼容性。

14. 批量注册上限由 `100` 提升至 `1000`（前后端同步）。

## 核心能力

- Web UI 管理注册任务和账号数据
- 支持批量注册、日志实时查看、基础任务管理
- 支持多种邮箱服务接码
- 支持 SQLite 和远程 PostgreSQL
- 支持打包为 Windows/Linux/macOS 可执行文件
- 更适配当前 OpenAI 注册与登录链路

## 后端切换说明（Phase 5）

当前仓库的最终目标后端拓扑已经不是单进程 Python Web UI，而是：

- Go API：`backend-go/cmd/api`
- Go worker：`backend-go/cmd/worker`
- PostgreSQL：Go 持久化真值源
- Redis：队列、租约与 worker 运行时依赖

`webui.py`、当前 `docker-compose.yml` 和 Python/Jinja 页面仍可作为兼容 UI / 本地观察壳使用，但 Phase 5 的 cutover 目标是不再让它们承担生产后端关键路径。

回滚原则：

1. 在 `scripts/verify_phase5_cutover.sh` 和 live operator checks 完成之前，保留 Python 兼容启动路径。
2. 如果 Go cutover 验证失败，先停止 Go API / Go worker，再回到既有 Python Web UI 启动路径。
3. 在 rollback 路径没有被写清楚并验证前，不移除 Python 入口或残余 bridge。

## 环境要求

- Python 3.10+
- `uv`（推荐）或 `pip`

## 安装依赖

```bash
# 使用 uv（推荐）
uv sync

# 或使用 pip
pip install -r requirements.txt
```

## 环境变量配置

可选。复制 `.env.example` 为 `.env` 后按需修改:

```bash
cp .env.example .env
```

常用变量如下:

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `APP_HOST` | 监听主机 | `0.0.0.0` |
| `APP_PORT` | 监听端口 | `8000` |
| `APP_ACCESS_PASSWORD` | Web UI 访问密钥 | `admin123` |
| `APP_DATABASE_URL` | 数据库连接字符串 | `data/database.db` |

优先级:

`命令行参数 > 环境变量(.env) > 数据库设置 > 默认值`

## Go 后端切流路径（推荐）

当你准备验证或执行 Go backend cutover 时，优先使用下面这条路径，而不是直接把 Python Web UI 当成最终生产后端：

```bash
cd backend-go
cp .env.example .env
set -a
source .env
set +a

make migrate-up
make run-api
```

另开一个终端启动 worker：

```bash
cd backend-go
set -a
source .env
set +a

make run-worker
```

然后回到仓库根目录跑 Phase 5 验证入口：

```bash
bash scripts/verify_phase5_cutover.sh
```

或者直接在 `backend-go/` 下执行：

```bash
cd backend-go
make verify-phase5
```

如果你仍需要页面、noVNC 或现有模板进行本地观察，可以继续启动下面的 Python Web UI 兼容壳，但它不应再被视为最终生产后端路径。

## 启动 Python Web UI（兼容壳 / 本地使用）

```bash
# 默认启动（127.0.0.1:8000）
python webui.py

# 指定地址和端口
python webui.py --host 0.0.0.0 --port 8080

# 调试模式（热重载）
python webui.py --debug

# 设置 Web UI 访问密钥
python webui.py --access-password mypassword

# 组合参数
python webui.py --host 0.0.0.0 --port 8080 --access-password mypassword
```

说明:

- `--access-password` 的优先级高于数据库中的密钥设置
- 该参数只对本次启动生效
- 打包后的 exe 也支持这个参数

例如:

```bash
codex-console.exe --access-password mypassword
```

启动后访问:

[http://127.0.0.1:8000](http://127.0.0.1:8000)

补充说明：

- 这一节描述的是当前 Python Web UI 的兼容启动方式，不是 Phase 5 的最终 Go backend cutover 形态。
- 如果你正在做最终后端切换，请优先参考上面的“Go 后端切流路径（推荐）”以及 `scripts/verify_phase5_cutover.sh`。

## Docker Compose（默认 Go backend，兼容壳可选）

### 默认启动 Go backend cutover 拓扑

```bash
docker-compose up -d
```

默认会启动：

- `postgres`
- `redis`
- `go-api`
- `go-worker`

这样启动后，Go backend 会监听：

- API: `http://127.0.0.1:18080`

如果需要继续打开 Python Web UI 兼容壳和 noVNC 观察浏览器，使用：

```bash
docker-compose --profile compat-ui up -d
```

你可以在 `docker-compose.yml` 中修改环境变量，比如 Go API 端口、数据库账号和 Python 兼容壳访问密码。  
如果启用了 `compat-ui` profile，需要看的“全自动绑卡”可视化浏览器入口是：

- noVNC: `http://127.0.0.1:6080`

### 使用 docker run（Python 兼容壳）

```bash
docker run -d \
  -p 1455:1455 \
  -p 6080:6080 \
  -e DISPLAY=:99 \
  -e ENABLE_VNC=1 \
  -e VNC_PORT=5900 \
  -e NOVNC_PORT=6080 \
  -e WEBUI_HOST=0.0.0.0 \
  -e WEBUI_PORT=1455 \
  -e WEBUI_ACCESS_PASSWORD=your_secure_password \
  -v $(pwd)/data:/app/data \
  --name codex-console \
  ghcr.io/<yourname>/codex-console:latest
```

说明:

- `WEBUI_HOST`: 监听主机，默认 `0.0.0.0`
- `WEBUI_PORT`: 监听端口，默认 `1455`
- `WEBUI_ACCESS_PASSWORD`: Web UI 访问密码
- `DEBUG`: 设为 `1` 或 `true` 可开启调试模式
- `LOG_LEVEL`: 日志级别，例如 `info`、`debug`

注意:

`-v $(pwd)/data:/app/data` 很重要，这会把数据库和账号数据持久化到宿主机。否则容器一重启，数据也可能跟着表演消失术。

这条 `docker run` 路径当前仍然启动 Python Web UI 壳与 noVNC 观察环境，不是默认 Go backend cutover 路径。更推荐优先使用上面的 `docker-compose` 默认拓扑来启动 Go API + Go worker。

## 使用远程 PostgreSQL

```bash
export APP_DATABASE_URL="postgresql://user:password@host:5432/dbname"
python webui.py
```

也支持 `DATABASE_URL`，但优先级低于 `APP_DATABASE_URL`。

## 打包为可执行文件

```bash
# Windows
build.bat

# Linux/macOS
bash build.sh
```

Windows 打包完成后，默认会在 `dist/` 目录生成类似下面的文件:

```text
dist/codex-console-windows-X64.exe
```

如果打包失败，优先检查:

- Python 是否已加入 PATH
- 依赖是否安装完整
- 杀毒软件是否拦截了 PyInstaller 产物
- 终端里是否有更具体的报错日志

## 项目定位

这个仓库更适合作为:

- 原项目的修复增强版
- 当前注册链路的兼容维护版
- 自己二次开发的基础版本

如果你准备公开发布，建议在仓库描述里明确写上:

`Forked and fixed from cnlimiter/codex-manager`

这样既方便别人理解来源，也对上游作者更尊重。

## 仓库命名

当前仓库名:

`codex-console`

## 免责声明

本项目仅供学习、研究和技术交流使用，请遵守相关平台和服务条款，不要用于违规、滥用或非法用途。

因使用本项目产生的任何风险和后果，由使用者自行承担。
