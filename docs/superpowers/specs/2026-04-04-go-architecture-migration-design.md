# Codex Console Go 架构迁移设计文档

> 日期：2026-04-04  
> 仓库：`/Users/weihaiqiu/IdeaProjects/codex-console`

---

## 1. 背景与目标

当前项目已经从“单页面注册工具”演进为一个包含以下能力的后台系统：

- 批量注册
- Token 刷新与验证
- Team discovery / sync / invite
- 邮箱服务管理
- 代理管理
- 日志实时推送
- 半自动绑卡与支付链路

但系统的并发基础设施仍停留在单体 FastAPI + 进程内任务管理阶段，主要表现为：

- 注册和批量任务依赖进程内 `ThreadPoolExecutor`
- 任务状态、暂停/恢复、批量状态和实时日志仍大量驻留在进程内内存
- 数据库层长期兼容 SQLite，导致整体设计倾向于单机模式
- 前后端实时日志和任务控制强耦合于当前 Web 进程
- 一旦后端进程重启，运行时态恢复能力有限

本次目标不是“把 Python 逐文件翻译成 Go”，而是将系统升级为：

1. **控制面与执行面分离**
2. **PostgreSQL 成为唯一权威数据源**
3. **Redis 成为队列、分布式控制和短期流式日志通道**
4. **Go 成为新的控制面与主执行面语言**
5. **迁移过程允许 Python 注册核心短期作为兼容 worker 存在**

本次设计服务的最终目标是：**显著提升并发能力、稳定性、可恢复性和横向扩容能力，并让系统从单进程任务模型平滑迁移到 Go 驱动的分布式任务架构。**

---

## 2. 现状诊断

### 2.1 当前架构特征

当前项目核心运行模型大致如下：

```text
Browser / WebUI
    ↓
FastAPI 单体
    ├── API 路由
    ├── WebSocket 推送
    ├── TaskManager（进程内）
    ├── ThreadPoolExecutor
    └── SQLAlchemy + SQLite/PostgreSQL
            ↓
        外部依赖（邮箱/代理/OpenAI/支付/Team）
```

### 2.2 当前瓶颈

从代码结构和运行方式看，真正限制高并发的不是 Python 语法层，而是以下架构问题：

#### A. 进程内任务控制

- 任务状态、日志队列、批量状态、暂停恢复标志大量保存在内存字典中
- `TaskManager` 使用全局线程池和进程内锁管理注册任务
- 当前模型天然不适合多实例横向扩容

#### B. 运行时状态不具备天然恢复能力

- 服务进程重启后，内存中的任务控制态、日志偏移、WebSocket 订阅态会丢失
- 数据库只记录结果快照，不是完整的任务运行事实来源

#### C. 执行面与控制面耦合

- Web API、任务调度、任务执行和日志推送都在同一进程中
- 一个热点路径阻塞或广播风暴，会拖累整站响应

#### D. 数据库设计长期围绕 SQLite 兼容

- 当前数据库初始化和迁移逻辑明显优先照顾 SQLite
- 即使切到了 PostgreSQL，整体设计思路仍未彻底转成“中心化数据库 + 多 worker”

#### E. 安全模型偏弱

- 账号密码、部分外部服务 token 当前仍有明文或弱保护存储
- 当系统进入更强并发和多实例部署阶段，这类问题风险会被放大

### 2.3 迁移动因总结

本次迁移并非为了“追求更酷的语言”，而是为了同时解决：

- 更高并发
- 更强任务恢复能力
- 更稳的暂停 / 恢复 / 取消
- 更稳的日志流
- 更清晰的模块边界
- 更容易做横向扩容

---

## 3. 设计原则

### 3.1 架构优先，语言跟随终态

本次迁移优先重构系统边界，而不是逐文件机械重写。  
但新架构从第一阶段起就直接以 Go 为终态收敛，不再额外做一轮“纯 Python 新架构”。

### 3.2 PostgreSQL 是唯一权威业务存储

任务元数据、账号数据、Team 数据、配置、审计日志都应以 PostgreSQL 为最终事实来源。  
Redis 只承担短期状态和传输角色，不承担权威业务持久化角色。

### 3.3 Redis 负责队列、控制信号和短期流

Redis 用于：

- 任务入队
- worker 竞争
- 暂停 / 恢复 / 取消控制信号
- 实时日志流
- 分布式锁

### 3.4 控制面与执行面解耦

- 控制面负责 API、任务编排、权限、状态查询、日志订阅
- 执行面负责真正调用外部服务执行耗时操作

### 3.5 先迁并发热点，后迁复杂状态机

优先迁移并发收益高、规则清晰的模块；  
最复杂的注册状态机采用过渡期兼容方案，最后再完成 Go 原生化。

### 3.6 双栈过渡，避免一次性重写

迁移期间允许：

- Go control plane
- Go worker
- Python legacy worker

并行存在，直到核心链路都切稳为止。

---

## 4. 目标架构

### 4.1 目标拓扑

```text
Browser / WebUI
    ↓
Go Control API
    ├── Auth / Session
    ├── Task API
    ├── Accounts API
    ├── Team API
    ├── Settings API
    └── WebSocket / SSE Gateway
            ↓
        PostgreSQL
            ↓
        Redis
    ┌───────────────┬────────────────┬────────────────┐
    ↓               ↓                ↓
Go Worker       Go Worker        Python Legacy Worker
    ↓               ↓                ↓
邮箱 / 代理 / OpenAI / Team / 支付等外部系统
```

### 4.2 模块职责

#### A. Go Control API

职责：

- 创建任务
- 查询任务 / 批量任务 / 日志
- 暂停、恢复、取消任务
- 聚合 PostgreSQL 与 Redis 中的状态
- 向前端提供 WebSocket 或 SSE 流
- 承担未来主 API 服务角色

#### B. PostgreSQL

职责：

- 账号主数据
- 任务主数据
- Team 相关数据
- 系统设置
- 审计与业务日志索引
- 幂等记录

#### C. Redis

职责：

- 异步任务队列
- 短期任务日志流
- 运行时控制信号
- worker 心跳
- 分布式锁

#### D. Worker 集群

职责：

- 消费任务
- 与外部依赖交互
- 落库任务状态
- 产生日志流
- 定期上报心跳

#### E. Python Legacy Worker

职责：

- 在过渡期继续承接最复杂、最难一次性重写的注册核心链路
- 按新的 Redis + PostgreSQL 协议接入，不再走旧 TaskManager

---

## 5. 技术选型

### 5.1 Go 技术栈建议

- Web/API：`chi` + `net/http`
- PostgreSQL 驱动：`pgx`
- SQL 生成：`sqlc`
- Redis 客户端：`go-redis`
- 任务队列：`Asynq`
- 配置：`koanf` 或 `envconfig`
- 日志：`zap`
- OpenAPI：`oapi-codegen` 或 `swaggo`
- 迁移工具：`goose` 或 `atlas`

### 5.2 为什么不在第一阶段引入更多中间件

第一阶段不推荐同时引入：

- NATS
- Kafka
- Temporal
- Kubernetes 原生复杂编排

原因：

- 你当前已有 PostgreSQL + Redis，可满足第一阶段目标
- 迁移本身已经足够复杂，继续增加基础设施只会扩大不确定性
- 当前首要问题是任务模型和运行时状态，而不是事件系统不够花哨

---

## 6. 领域边界拆分

### 6.1 第一批迁移模块

这些模块优先迁移，因为规则相对清晰、并发收益高、和现有 TaskManager 解耦价值最大：

1. 批量 token 刷新
2. 批量 token 验证
3. Team discovery
4. Team sync
5. Team invite / revoke / remove
6. 邮箱健康检查
7. 代理测试与健康检查

### 6.2 第二批迁移模块

1. 注册任务控制面
2. 批量注册调度器
3. 日志流服务
4. Outlook 资源分配与抢占逻辑

### 6.3 第三批迁移模块

1. 注册核心状态机
2. AnyAuto / OAuth / OpenAI 注册链路
3. 支付 / 半自动绑卡链路

### 6.4 可暂时不迁或后迁模块

- Web 模板层
- 管理后台静态资源
- 低频设置页逻辑

这些可以在控制面 API 切稳后，再决定是否继续保留现有前端，或再做前后端分离。

---

## 7. 数据架构设计

### 7.1 PostgreSQL 作为唯一权威数据源

迁移后，以下数据必须以 PostgreSQL 为准：

- `accounts`
- `email_services`
- `proxies`
- `registration_tasks`
- `team_tasks`
- `teams`
- `team_memberships`
- `settings`
- `bind_card_tasks`

### 7.2 数据表设计调整方向

#### A. 任务表标准化

建议新增或重构以下表：

- `jobs`
  - 统一任务主表
  - 字段：`job_id`、`job_type`、`scope_type`、`scope_id`、`status`、`priority`、`payload`、`result`、`error`、`created_at`、`started_at`、`finished_at`

- `job_runs`
  - 同一任务的多次执行记录
  - 字段：`job_run_id`、`job_id`、`worker_id`、`attempt`、`status`、`started_at`、`finished_at`

- `job_logs`
  - 日志持久化落点
  - 字段：`id`、`job_id`、`job_run_id`、`seq`、`level`、`message`、`created_at`

原有 `registration_tasks` / `team_tasks` 可在过渡期保留，并逐步兼容到统一任务模型。

#### B. 结构化配置

- 将 `settings` 继续保留为配置存储
- 但对复杂 provider 配置建议从文本 JSON 逐步升级为 `JSONB`
- 敏感字段必须在写入前加密

#### C. 审计与幂等

建议新增：

- `idempotency_keys`
- `worker_heartbeats`
- `job_control_events`

用于：

- 防重复提交
- worker 存活追踪
- 暂停 / 恢复 / 取消的可审计控制记录

### 7.3 安全调整

必须修复以下问题：

- 账号密码不再明文存储
- 外部服务 API token 使用加密字段
- PostgreSQL / Redis 使用独立应用账号
- 生产环境密钥改为外部注入，不进入仓库

---

## 8. Redis 设计

### 8.1 Redis 职责边界

Redis 只用于以下场景：

- 队列
- 控制信号
- 短期日志流
- 心跳
- 锁

不用于长期保存最终业务事实。

### 8.2 Key 设计建议

```text
queue:jobs:default
queue:jobs:registration
queue:jobs:team

job:control:{job_id}
job:state:{job_id}
job:logs:{job_id}

worker:heartbeat:{worker_id}
lock:email_service:{service_id}
lock:proxy:{proxy_id}
lock:account:{account_id}
```

### 8.3 控制信号模型

控制面写入：

- `pause`
- `resume`
- `cancel`

worker 执行过程在关键边界点轮询或订阅控制信号，并写回 PostgreSQL 事件记录。

### 8.4 日志流模型

日志采用 append-only 模式：

1. worker 先写 Redis Stream/List
2. 控制面实时消费并推送到前端
3. 后台异步批量刷入 PostgreSQL `job_logs`

这样可以兼顾：

- 实时性
- 查询一致性
- 控制存储成本

---

## 9. API 兼容策略

### 9.1 总体原则

前端第一阶段尽量不重写，以“接口兼容 + 渐进替换”为主。

### 9.2 兼容策略

#### A. 路由兼容

Go Control API 首批需要兼容以下接口族：

- `/api/registration/*`
- `/api/accounts/*`
- `/api/team/*`
- `/api/settings/*`
- `/api/email/*`

#### B. 实时推送兼容

当前前端已使用 WebSocket 监听任务和批量任务日志。  
新控制面可优先继续提供 WebSocket，后续再评估是否补 SSE。

#### C. 响应格式兼容

保留当前前端依赖的关键字段：

- `task_uuid`
- `status`
- `logs`
- `success`
- `message`
- `error`

这样可以减少前端同步改造成本。

---

## 10. 迁移阶段

### 10.1 Phase 0：准备期

目标：

- 停止继续围绕 SQLite 增加新能力
- 固定 PostgreSQL 作为主部署目标
- 梳理现有接口、表结构、任务类型和状态机

交付：

- 任务类型清单
- 接口兼容清单
- 数据模型映射表

### 10.2 Phase 1：基础设施切换

目标：

- 引入 Go Control API
- 引入 PostgreSQL 统一 schema
- 引入 Redis 队列与控制通道
- 前端仍接当前模板页

交付：

- Go API 服务骨架
- `jobs` / `job_runs` / `job_logs` 表
- Redis 队列和控制协议

### 10.3 Phase 2：低风险高收益 worker 迁移

目标：

- 批量刷新 / 验证迁到 Go
- Team discovery / sync / invite 迁到 Go
- 健康检查类任务迁到 Go

交付：

- 第一批稳定运行的 Go worker
- 并发与吞吐压测结果

### 10.4 Phase 3：注册控制面迁移

目标：

- 前端注册任务创建、批量调度、暂停恢复取消走 Go
- 注册实际执行仍允许由 Python legacy worker 承接

交付：

- 统一任务调度中台
- Python 通过 Redis 协议消费任务

### 10.5 Phase 4：注册核心迁移

目标：

- 将注册状态机逐步改写为 Go 原生执行链路
- 下线旧 TaskManager 和旧线程池模型

交付：

- Go registration worker
- Python legacy worker 下线计划

### 10.6 Phase 5：收尾与统一

目标：

- 清理旧 FastAPI 运行时任务逻辑
- 清理 SQLite 兼容分支
- 固化运行、监控、告警、回滚流程

---

## 11. 切流与回滚策略

### 11.1 切流原则

采用灰度切流，而不是整体替换：

1. 新任务类型先在 Go worker 跑
2. 关键注册任务先走“Go control + Python worker”
3. 核心注册链路稳定后再切换为 Go worker

### 11.2 双写 / 双读策略

在过渡期：

- 状态查询可以优先读 PostgreSQL
- 某些旧表保留镜像写入，避免前端一次性失配

### 11.3 回滚策略

若某批任务在 Go worker 上出现系统性失败：

- 停止新任务路由到该队列
- 控制面切回 Python 兼容路径
- Redis 中未消费任务重新路由或延后重试

关键原则是：**控制面和执行面都必须支持按任务类型回退，而不是按整站回退。**

---

## 12. 可观测性设计

### 12.1 基础指标

至少采集以下指标：

- 每类任务吞吐
- 成功率 / 失败率
- 平均执行时长
- 队列积压深度
- worker 心跳状态
- PostgreSQL 连接池利用率
- Redis 延迟和命中情况

### 12.2 结构化日志

Go 侧日志统一使用结构化输出，至少包含：

- `job_id`
- `job_type`
- `worker_id`
- `scope_type`
- `scope_id`
- `attempt`

### 12.3 告警

至少建立：

- 队列积压告警
- worker 掉线告警
- 注册失败率异常告警
- PostgreSQL 连接耗尽告警
- Redis 不可用告警

---

## 13. 风险清单

### 13.1 最大风险

#### A. 直接全量重写注册核心

风险最高。  
注册流程涉及邮箱、验证码、代理、OpenAI 页面/接口状态机、OAuth、各种异常分支，直接一次性 Go 重写容易造成长时间不可用。

#### B. 把旧模型机械翻译成 Go

如果只是把当前内存任务模型、全局状态字典和线程池思路照搬到 Go，最终只会得到一个“Go 写的旧系统”，并发收益远低于预期。

#### C. 迁移期间接口不兼容

如果任务响应字段、日志格式、状态机名称随意变化，前端会在迁移期产生额外故障。

#### D. 凭据和密钥安全

当前已存在敏感凭据使用习惯偏弱的问题。迁移时如果继续延续，会把风险从单机放大到分布式环境。

### 13.2 风险应对

- 复杂链路采用双栈过渡
- 新旧任务类型分批切流
- 前端优先接口兼容
- 先做基础设施，后做最复杂业务状态机

---

## 14. 工期预估

在不做前端大改、以单仓迁移为前提下，保守估计：

- Phase 0-1：1 到 2 周
- Phase 2：1 到 2 周
- Phase 3：1 周
- Phase 4：2 到 4 周
- Phase 5：1 周

总计保守预估：**6 到 10 周**

如果团队人力较少，或注册链路隐藏复杂度继续上升，则应按 **8 到 12 周** 预估更稳。

---

## 15. 最终结论

### 15.1 是否应该换架构

应该，而且优先级高。

### 15.2 是否应该换语言

可以换，而且从长期维护、并发能力、部署统一性来看，**Go 是合理目标语言**。

### 15.3 该怎么换

最优路线不是：

- 先纯 Python 架构重构，再整体重写 Go

也不是：

- 直接一次性把整仓全部改成 Go

而是：

**先切架构边界，但新边界从第一阶段就直接以 Go 落地；最复杂的注册核心允许短期继续由 Python legacy worker 承接，直到 Go worker 完成接管。**

这条路线兼顾：

- 并发收益
- 风险控制
- 迁移速度
- 长期维护统一性

---

## 16. 下一步

如果用户确认本设计方向，下一步应进入实施规划，输出：

1. Go 项目目录结构
2. PostgreSQL schema 迁移清单
3. Redis key / queue / control 协议设计
4. API 兼容清单
5. Phase 0-1 的执行计划

