# Team 管理模块设计文档

> 日期：2026-04-03  
> 仓库：`/Users/weihaiqiu/IdeaProjects/codex-console`  
> 工作区：`/Users/weihaiqiu/.config/superpowers/worktrees/codex-console/team-management`

---

## 1. 背景与目标

当前项目已经具备完整的账号注册、账号存储、Token 刷新、批量账号操作、Web 管理页面与任务监控能力，但 Team 相关能力仍停留在：

- Team 订阅识别
- Team Manager 外部上传
- `auto_team.html` 页面占位

本次目标是在当前项目内部原生实现一套完整的 Team 管理模块，覆盖：

1. 从当前账号池发现 Team 母号
2. 区分母号 / 子号 / 外部成员
3. 识别母号与子号之间的关系
4. 同步 Team 基础信息、成员列表、邀请列表、状态
5. 支持从当前账号池批量选择账号拉入指定 Team
6. 邀请后仅更新 Team 关系状态，不自动刷新 RT，不自动注册
7. 支持随时监控 Team 状态、成员状态与任务状态

---

## 2. 本次设计的硬约束

### 2.1 数据源约束

Team 数据以当前项目本地 `accounts` 表为发现入口，不依赖外部 `team-manage` 数据库作为主数据源。

```text
accounts（本地账号池）
    ↓
发现 Team 母号
    ↓
同步 Team / 成员 / 邀请
    ↓
建立本地母号-子号关系
```

### 2.2 自动化约束

邀请完成后：

- 只更新 Team 关系状态
- 不自动刷新 RT
- 不自动执行完整注册

后续账号操作由用户手动触发。

### 2.3 落地约束

- 这是一次大改动，必须在独立 worktree 中推进
- 实现阶段子 agent 统一使用 GPT-5.4
- 可并行则并行，但写代码统一落在同一个 worktree 中

---

## 3. 范围边界

### 3.1 本次纳入范围

- Team 母号发现
- Team 列表与详情
- Team 状态同步
- 成员 / 邀请状态同步
- 母号 / 子号 / 外部成员关系映射
- 从本地账号池批量邀请进入 Team
- 手动输入邮箱邀请
- 撤回邀请
- 移除成员
- Team 任务持久化与监控

### 3.2 本次明确不做

- 自动邀请后自动刷新 RT
- 自动邀请后自动注册
- 搬运 `team-manage` 的兑换码 / 质保 / webhook 补货体系
- 直接复用外部项目数据库模型作为主模型
- 将 Team 状态硬塞进现有 `accounts` 主表作为唯一事实来源

---

## 4. 外部能力来源与复用策略

本次不会整仓搬运 `team-manage`，而是按“规则复用、实现重建”的方式接入。

### 4.1 重点参考来源

- `team-manage/team_inviter/client.py`
- `team-manage/team_inviter/service.py`
- `team-manage/app/services/team.py`
- `team-manage/app/services/chatgpt.py`

### 4.2 可复用的核心规则

1. `account_user_role` 用于识别母号/管理者
2. Team account 发现依赖上游 account info 接口
3. 成员与邀请应作为两类数据源合并建模
4. `already_member` 应视为业务成功变种
5. Team 满员、失效、封禁、ghost success 需要单独建模

### 4.3 不直接搬运的内容

- 兑换码
- 质保
- webhook 补货
- 独立后台模板
- 外部项目的数据库结构

### 4.4 上游接口契约（最小集合）

当前设计依赖的上游最小接口集合如下，后续实现必须以这些契约为 TeamService 的输入边界：

#### A. 母号/Team 发现接口

- `GET /backend-api/accounts/check/v4-2023-04-27`

最小字段：

- `accounts.{account_id}.account.plan_type`
- `accounts.{account_id}.account.name`
- `accounts.{account_id}.account.account_user_role`
- `accounts.{account_id}.entitlement.subscription_plan`
- `accounts.{account_id}.entitlement.expires_at`
- `accounts.{account_id}.entitlement.has_active_subscription`

用途：

- 判断是否为 Team
- 判断是否为母号（`account-owner`）
- 发现一个本地账号名下的多个 Team account

#### B. Team 成员列表接口

- `GET /backend-api/accounts/{account_id}/users?limit={n}&offset={n}`

最小字段：

- `items[].id`
- `items[].email`
- `items[].name`
- `items[].role`
- `items[].created_time`

用途：

- 构建 `joined` 成员关系

#### C. Team 邀请列表接口

- `GET /backend-api/accounts/{account_id}/invites`

最小字段：

- `items[].email_address`
- `items[].role`
- `items[].created_time`

用途：

- 构建 `invited` 成员关系

#### D. 发送邀请接口

- `POST /backend-api/accounts/{account_id}/invites`

最小请求体：

- `email_addresses`
- `role=standard-user`
- `resend_emails=true`

结果分类：

- 成功创建邀请 -> `invited`
- 文本命中 `already in workspace/team/member` -> `already_member`
- 文本命中 `maximum number of seats/full/no seats` -> Team 标记 `full`
- 明确 `token_invalidated/account_deactivated` -> Team 标记 `banned`
- 返回 2xx 但后续同步看不到目标邮箱 -> `ghost success`

#### E. 撤回邀请接口

- `DELETE /backend-api/accounts/{account_id}/invites`

最小请求体：

- `email_address`

#### F. 移除成员接口

- `DELETE /backend-api/accounts/{account_id}/users/{user_id}`

#### G. 母号 Token 前提

Team 管理过程中，允许为**母号**复用当前项目已有的 ST/RT 刷新链路，仅用于保证 Team API 可调用。

但该能力**绝不能**扩散到被邀请子号：

- 不允许为子号自动刷新 RT
- 不允许为子号自动执行注册
- 不允许借 Team 邀请结果顺带补齐账号资料

### 4.5 上游字段映射与判定算法

| 本地字段 | 来源 | 原始字段/算法 | 缺失时回退策略 |
| --- | --- | --- | --- |
| `teams.upstream_account_id` | account check | `accounts.{account_id}` 的 key | 无回退，缺失则该 Team 不落库 |
| `teams.team_name` | account check | `account.name` | 缺失时使用 `Team-{account_id[:8]}` 占位 |
| `teams.plan_type` | account check | `account.plan_type` | 缺失时记 `unknown` |
| `teams.account_role_snapshot` | account check | `account.account_user_role` | 缺失时记 `unknown` |
| `teams.subscription_plan` | account check | `entitlement.subscription_plan` | 缺失时记 `unknown` |
| `teams.expires_at` | account check | `entitlement.expires_at` | 缺失时保留现值或记空 |
| `teams.current_members` | users + invites | 基于**规范化邮箱去重后的活跃关系集合**计数 | 若接口失败，保留上次成功值并标记 `sync_error` |
| `teams.max_members` | 上游显式席位字段（若后续发现）或本地学习值 | 第一阶段默认 `6`；当收到满员错误且 `current_members < max_members` 时，下调为 `current_members` | 若无显式来源，持续使用“默认值 + 学习修正” |
| `teams.seats_available` | 本地算法 | `max(max_members - current_members, 0)` | 不单独落上游缺失 |
| `team_memberships.member_email` | users/invites | `email` 或 `email_address` 经标准化 | 缺失则跳过该条并记录日志 |
| `team_memberships.upstream_user_id` | users | `items[].id` | invites 阶段允许为空 |

#### A. `users` 分页停止规则

分页读取 `GET /users?limit={n}&offset={n}` 时，满足任一条件即停止：

1. 返回 `items` 为空
2. 已收集数量 `>= total`
3. 当前页返回条数 `< limit`

#### B. 上游错误结构最小假设

统一从以下位置抽取错误：

1. `response.json().detail`
2. `response.json().error`
3. `response.json().error.code`
4. `response.text`

#### C. `ghost success` 确认窗口

第一阶段采用外部项目已验证过的保守策略：

- 邀请接口返回 2xx 后
- 进行 **3 次**同步确认
- 每次间隔 **5 秒**
- 总窗口 **15 秒**

若在该窗口内：

- 成员列表未出现目标邮箱
- 邀请列表也未出现目标邮箱

则记为 `ghost success`

#### D. 活跃关系优先级与计数口径

同一 Team 内同一规范化邮箱若出现多种关系状态，优先级如下：

```text
joined > already_member > invited > revoked/removed/failed
```

计数规则：

- `current_members` 只统计去重后的**活跃唯一邮箱**
- `joined` 与 `already_member` 都视为占用席位
- `invited` 在第一阶段也视为占用席位
- `revoked/removed/failed` 不计入 `current_members`
- 任一邮箱绝不允许因为同时存在 `joined + invited` 而双计数

---

## 5. 核心概念定义

### 5.1 母号

本地 `accounts` 中的账号，如果发现到：

- `plan_type = team`
- 且 `account_user_role = account-owner`

则认定为 Team 母号。

### 5.2 子号

存在于某个 Team 的成员关系中，并且其邮箱能映射到本地 `accounts` 记录的账号。

### 5.3 外部成员

存在于某个 Team 的成员或邀请列表中，但当前项目本地 `accounts` 中还没有该邮箱账号记录。

### 5.4 Team 关系

通过 `Team -> TeamMembership -> local_account_id/member_email` 三段结构表达：

- 某个本地账号是否为母号
- 某个邮箱是否属于某个 Team
- 该邮箱是否映射到本地账号
- 当前是 invited / joined / removed 等何种状态

---

## 6. 数据模型设计

### 6.1 `teams`

用途：记录每个 Team 实体本身。

建议字段：

- `id`
- `owner_account_id`：关联本地 `accounts.id`
- `upstream_team_id`：可空，保留给未来若上游返回独立 Team/Workspace 主键时使用
- `upstream_account_id`：上游 Team account id
- `team_name`
- `plan_type`
- `subscription_plan`
- `account_role_snapshot`
- `status`
- `current_members`
- `max_members`
- `seats_available`
- `expires_at`
- `last_sync_at`
- `sync_status`
- `sync_error`
- `created_at`
- `updated_at`

约束建议：

- 当前阶段使用 `unique(owner_account_id, upstream_account_id)` 作为稳定唯一键
- 若未来拿到独立 `upstream_team_id`，则新增 `unique(upstream_team_id)`

设计说明：

- 基于当前已验证的上游接口，Team 管理动作（同步、邀请、撤回、移除）都围绕 `account_id` 运行，因此第一阶段把 `upstream_account_id` 视为**当前可操作的上游 Team 管理主键**
- 一个本地母号允许发现多个 Team account，因此 `owner_account_id` 与 `upstream_account_id` 是一对多关系
- `upstream_team_id` 作为前瞻保留字段，不在第一阶段依赖，但模型预留以免后续迁移破坏主键语义

### 6.2 `team_memberships`

用途：记录 Team 与成员/邀请之间的关系，是母号/子号关系的核心。

建议字段：

- `id`
- `team_id`
- `local_account_id`（可空，关联 `accounts.id`）
- `member_email`
- `upstream_user_id`（可空）
- `member_role`
- `membership_status`
- `invited_at`
- `joined_at`
- `removed_at`
- `last_seen_at`
- `source`
- `sync_error`
- `created_at`
- `updated_at`

约束建议：

- `unique(team_id, member_email)`

设计要点：

- `local_account_id` 允许为空，用于承接“Team 中已有，但本地还没有账号”的外部成员
- `member_email` 必须保留，不能只依赖本地账号 ID

### 6.3 关系与规范化规则

#### A. 邮箱标准化

所有进入 Team 关系域的邮箱都必须先做：

1. `trim`
2. 转小写
3. 去掉批量导入文本中的非邮箱包裹内容

规范化后的邮箱作为：

- `team_memberships.member_email`
- 本地账号映射键
- 批量邀请去重键

#### B. 一个本地账号是否允许出现在多个 Team

允许。

原因：

- 历史关系需要保留
- 外部数据不能假设永远单 Team
- 当前阶段不对“一个账号只能属于一个 Team”做数据库级硬限制

但 UI 和任务层要给出规则：

- 若同一邮箱存在多个 `joined/invited/already_member` 活跃关系，账号页显示“多 Team 关联”警告
- 后续任何需要“唯一所属 Team”的功能，必须显式选择，不得隐式猜测

#### C. 自动映射与手动绑定优先级

优先级：

```text
手动绑定 > 自动邮箱映射
```

规则：

- `source=manual_bind` 的关系，不允许被自动同步覆盖成本地其他账号
- 自动映射仅在 `local_account_id` 为空时触发
- 若手动绑定后邮箱与本地账号邮箱不一致，视为异常配置，在详情页告警但不自动改写

#### D. 外部成员补链

外部成员补链触发时机：

1. 每次 Team 同步后
2. 本地账号创建/导入成功后
3. 手动执行“重绑本地账号”动作时

补链结果：

- 找到同邮箱本地账号 -> 回填 `local_account_id`
- 找不到 -> 保持外部成员状态

#### E. 解绑 / 重绑规则

- 解绑只能将 `local_account_id` 清空，不删除 Team 关系本身
- 重绑只允许指向同邮箱或用户明确确认的目标账号
- 重绑必须写审计日志

#### F. 角色冲突优先级

同一邮箱/本地账号可能同时具有多重身份，展示与业务优先级定义如下：

```text
母号身份 > 当前 Team 成员身份 > 当前 Team 外部成员身份
```

细则：

- `teams.owner_account_id` 是母号身份的唯一表达，不额外为同一 Team 的 owner 建 `team_memberships`
- 若上游成员列表返回了 owner 本人的邮箱，默认跳过，不落子号关系
- 同一账号允许出现“Team A 的母号 + Team B 的子号”组合
- 在 Team 详情页按**当前 Team 视角**展示角色
- 在账号总览页按**聚合视角**展示：
  - 有任何母号关系 -> 显示 `母号`
  - 同时有成员关系 -> 补充 `多角色` 或 `子号` 数量提示
- `already_member` 在展示与计数上归入“活跃成员态”，不归入“邀请中”

### 6.4 `team_tasks`

用途：持久化 Team 相关长任务。

建议字段：

- `id`
- `task_uuid`
- `task_type`
- `team_id`（可空）
- `owner_account_id`（可空）
- `status`
- `request_payload`
- `result_payload`
- `error_message`
- `logs`
- `created_at`
- `started_at`
- `completed_at`

建议任务类型：

- `discover_owner_teams`
- `sync_team`
- `sync_all_teams`
- `invite_members`
- `revoke_invite`
- `remove_member`

### 6.5 `team_task_items`

用途：保存批量任务中的逐条结果。

建议字段：

- `id`
- `task_id`
- `local_account_id`（可空）
- `target_email`
- `target_team_id`
- `upstream_user_id`（可空）
- `item_status`
- `relation_status_before`
- `relation_status_after`
- `message`
- `error_message`
- `created_at`
- `updated_at`

---

## 7. 模块边界设计

### 7.1 `TeamDiscoveryService`

职责：

- 从本地 `accounts` 中识别 Team 账号
- 识别母号与非母号 Team 账号
- 发现母号名下多个 Team account
- 将 Team 基础实体写入 `teams`

不负责：

- 邀请成员
- 成员关系同步

### 7.2 `TeamSyncService`

职责：

- 同步 Team 基础信息
- 同步成员列表与邀请列表
- 计算 Team 状态
- 将成员邮箱映射到本地 `accounts`
- 更新 `team_memberships`

这是 Team 关系的唯一权威刷新入口。

### 7.3 `TeamInviteService`

职责：

- 从本地账号池提取邮箱并批量邀请
- 支持手动邮箱邀请
- 保存逐条邀请结果
- 更新 `team_memberships`

重要约束：

- 邀请后只更新 Team 状态
- 不自动刷新 RT
- 不自动注册

### 7.4 `TeamRelationService`

职责：

- 根据邮箱将 Team 成员映射到本地账号
- 修复关系漂移
- 支持手动绑定外部成员到本地账号

### 7.5 `TeamTaskService`

职责：

- Team 任务创建、日志、状态流转
- 与现有任务监控体系对接
- 保存逐条 item 结果

---

## 8. 状态机设计

### 8.1 Team 状态

`teams.status`

- `active`
- `full`
- `expired`
- `error`
- `banned`

规则：

- `full`：当前人数达到上限，或上游明确返回满员
- `expired`：母号 Token 失效且刷新失败
- `banned`：明确封禁/失效类错误
- `error`：临时异常、网络失败、ghost success

### 8.2 成员关系状态

`team_memberships.membership_status`

- `unknown`
- `invited`
- `already_member`
- `joined`
- `revoked`
- `removed`
- `failed`

### 8.3 任务状态

`team_tasks.status`

- `pending`
- `running`
- `completed`
- `failed`
- `cancelled`

`team_task_items.item_status`

- `pending`
- `success`
- `failed`
- `skipped`

### 8.4 禁止自动后续流程边界

这是本次实现的硬性保护规则，必须同时体现在 service、API、测试与验收中。

#### 邀请任务完成后允许做的事

- 写入 `team_memberships`
- 写入 `team_tasks` / `team_task_items`
- 更新 `teams` 状态与同步信息
- 触发 Team 级同步确认任务
- 写日志、写审计记录

#### 邀请任务完成后禁止做的事

- 调用子号 RT 刷新流程
- 调用子号注册流程
- 调用账号补全 / 密码补齐 / Token 补齐流程
- 修改本地 `accounts` 的注册完成态字段（除关系映射附加信息外）

#### 允许的唯一自动刷新

仅允许对**母号**在 Team API 调用前做 ST/RT 刷新，以保障 Team 同步、邀请、撤回、移除等管理动作可执行。

#### 可验证行为

Team 邀请任务完成后的日志与任务结果中必须明确出现：

```text
本次任务未触发子号自动注册
本次任务未触发子号自动刷新 RT
```

#### 结构化保护字段

`team_tasks.result_payload` 中必须包含以下结构化布尔字段：

- `child_refresh_triggered=false`
- `child_registration_triggered=false`

#### 禁止调用边界

Team 邀请任务及其后置同步流程中，禁止调用以下现有能力：

- `src/core/openai/token_refresh.py::refresh_account_token`（针对子号）
- `src/web/routes/accounts.py` 中的账号刷新入口（针对子号）
- `src/core/register.py::RegistrationEngine.run`
- `src/web/routes/registration.py` 中任何注册任务启动入口

#### 邀请后 `accounts` 表不变规则

对于被邀请的本地子号账号，在未发生用户手动操作前，以下字段必须保持不变：

- `password`
- `access_token`
- `refresh_token`
- `id_token`
- `session_token`
- `client_id`
- `cookies`
- `status`
- `source`
- `registered_at`
- `expires_at`

换言之：邀请任务对 `accounts` 表不做持久化修改。

---

## 9. API 草案

建议新增：

- `src/web/routes/team.py`
- `src/web/routes/team_tasks.py`

### 9.1 Team 发现

- `POST /api/team/discovery/run`
- `POST /api/team/discovery/{account_id}`

### 9.2 Team 列表与同步

- `GET /api/team/teams`
- `GET /api/team/teams/{team_id}`
- `POST /api/team/teams/{team_id}/sync`
- `POST /api/team/teams/sync-batch`

#### `GET /api/team/teams` 最小契约

筛选字段：

- `page`
- `per_page`
- `status`
- `owner_account_id`
- `owner_role`
- `search`
- `has_local_members`
- `has_external_members`
- `sort_by`

默认排序：

- `updated_at_desc`

最小返回字段：

- `id`
- `owner_account_id`
- `owner_email`
- `upstream_account_id`
- `team_name`
- `account_role_snapshot`
- `status`
- `current_members`
- `max_members`
- `seats_available`
- `expires_at`
- `last_sync_at`
- `sync_status`

#### `GET /api/team/teams/{team_id}` 最小契约

除 Team 基础字段外，至少返回：

- `active_member_count`
- `joined_count`
- `invited_count`
- `local_member_count`
- `external_member_count`
- `last_sync_error`
- `active_task_count`

### 9.3 成员关系管理

- `GET /api/team/teams/{team_id}/memberships`
- `POST /api/team/teams/{team_id}/memberships/{membership_id}/revoke`
- `POST /api/team/teams/{team_id}/memberships/{membership_id}/remove`
- `POST /api/team/memberships/{membership_id}/bind-local-account`

#### `GET /api/team/teams/{team_id}/memberships` 最小契约

筛选字段：

- `status`
- `binding=local|external|all`
- `search`
- `sort_by`

默认排序：

- `status_priority + updated_at_desc`

最小返回字段：

- `id`
- `member_email`
- `local_account_id`
- `local_account_status`
- `member_role`
- `membership_status`
- `upstream_user_id`
- `invited_at`
- `joined_at`
- `last_seen_at`

合并规则：

- `joined` 与 `invited` 以规范化邮箱为主键合并
- 若同一邮箱同时存在于成员列表与邀请列表，以 `joined` 为最终展示状态

状态过滤补充：

- `status=active` -> 返回 `joined + already_member`
- `status=joined` 在第一阶段等价于 `status=active`
- `status=invited` -> 仅返回 `invited`

动作闭环：

- 列表行必须携带 `id`
- 前端对 `revoke/remove/bind-local-account` 的操作均以该 `id` 作为 `membership_id`
- 动作成功后刷新当前 Team 详情与当前列表
- `already_member` 支持 `remove` 与 `bind-local-account`
- `already_member` 不支持 `revoke`，非法调用返回 `400`

### 9.4 批量邀请

- `POST /api/team/teams/{team_id}/invite-accounts`
- `POST /api/team/teams/{team_id}/invite-emails`

#### `POST /api/team/teams/{team_id}/invite-accounts` 最小契约

请求输入：

- `ids`
- `select_all`
- `status_filter`
- `email_service_filter`
- `search_filter`
- `refresh_token_state_filter`
- `skip_existing_membership`
- `resend_invited`

逐条处理规则：

- 已 `joined/already_member` -> `skipped`
- 已 `invited` 且 `resend_invited=false` -> `skipped`
- 已 `invited` 且 `resend_invited=true` -> 允许重发
- 同批次重复邮箱 -> 后项 `skipped`
- Team 满员 -> 当前失败，后续项默认 `skipped`

任务接受返回：

- `task_uuid`
- `accepted_count`
- `deduplicated_count`
- `skipped_count`

#### API 错误码约定

- `400`：输入参数非法 / Team 不可操作
- `404`：Team 或 membership 不存在
- `409`：同一 Team 上已有互斥写任务执行中
- `500`：任务创建或服务执行异常

#### 异步入口 accepted 响应统一契约

以下异步写入口统一返回：

- `POST /api/team/discovery/run`
- `POST /api/team/discovery/{account_id}`
- `POST /api/team/teams/{team_id}/sync`
- `POST /api/team/teams/sync-batch`
- `POST /api/team/teams/{team_id}/invite-accounts`
- `POST /api/team/teams/{team_id}/invite-emails`

最小返回体：

- `success=true`
- `task_uuid`
- `task_type`
- `status=pending`
- `team_id`（如适用）
- `owner_account_id`（如适用）
- `ws_channel=/api/ws/task/{task_uuid}`

订阅链路：

- 第一阶段统一复用现有 `/api/ws/task/{task_uuid}`
- 不新增 Team 专用 websocket 通道

### 9.5 任务监控

- `GET /api/team/tasks`
- `GET /api/team/tasks/{task_uuid}`
- 第一阶段统一复用现有 websocket：`/api/ws/task/{task_uuid}`

---

## 10. 页面草案

### 10.1 Team 总览页

建议继续使用现有入口页面：

- `/auto-team`

页面职责：

- Team 总览
- 母号发现入口
- 批量同步入口
- 任务中心入口

### 10.2 Team 详情页 / 抽屉

展示：

- Team 基础信息
- 母号信息
- joined 列表
- invited 列表
- external 列表
- 外部成员数 / 本地子号数

Tab 建议：

- `概览`
- `活跃成员（joined + already_member）`
- `邀请中`
- `外部成员`
- `任务记录`
- `同步日志`

操作：

- 同步
- 批量拉人
- 手动输入邮箱邀请
- 撤回邀请
- 移除成员

### 10.3 批量拉人弹窗

复用当前账号池筛选逻辑：

- 状态筛选
- 搜索
- 全选
- 批量勾选

右侧显示目标 Team 基础信息与剩余席位。

提交前必须明确提示：

- 已在队成员会被跳过
- 已邀请成员默认跳过，除非开启重发
- 本次操作不会自动触发注册或 RT 刷新

### 10.4 Team 任务中心

展示：

- 任务类型
- Team
- 发起时间
- 当前状态
- 逐条 item 结果

### 10.5 账号页 Team 增强信息

在现有账号页追加：

- Team 角色 badge（母号 / 子号）
- 所属 Team
- Team 关系状态
- 跳转 Team 详情快捷入口

若一个账号同时关联多个 Team：

- 账号页不展示单一 Team 名称
- 统一展示 `多 Team`
- 点击后展开全部 Team 关系

### 10.6 空态与冲突态

必须明确展示以下状态：

- 当前没有发现任何母号
- Team 列表为空
- Team 详情页没有任何成员
- 当前 Team 满员，禁止继续批量邀请
- 当前 Team 正在同步/写入，禁止再发起互斥操作
- 当前成员存在手动绑定冲突

### 10.7 页面数据契约与刷新链路

#### A. Team 总览页

- 数据来源：`GET /api/team/teams`
- 点击“同步”：
  1. 调 `POST /api/team/teams/{team_id}/sync`
  2. 使用 accepted 返回中的 `task_uuid` 订阅 `/api/ws/task/{task_uuid}`
  3. 收到完成事件后刷新 `GET /api/team/teams`

#### B. Team 详情页

- 基础信息来源：`GET /api/team/teams/{team_id}`
- 活跃成员来源：`GET /api/team/teams/{team_id}/memberships?status=active`
- 邀请列表来源：`GET /api/team/teams/{team_id}/memberships?status=invited`
- 外部成员来源：`GET /api/team/teams/{team_id}/memberships?binding=external`
- 动作完成后固定刷新：
  - `GET /api/team/teams/{team_id}`
  - 当前 `memberships` 列表接口

#### C. Team 任务中心

- 列表来源：`GET /api/team/tasks`
- 详情来源：`GET /api/team/tasks/{task_uuid}`

`GET /api/team/tasks/{task_uuid}` 最小返回体：

- `task_uuid`
- `task_type`
- `status`
- `team_id`
- `owner_account_id`
- `created_at`
- `started_at`
- `completed_at`
- `logs`
- `summary`
- `items`

其中 `items` 至少包含：

- `target_email`
- `item_status`
- `relation_status_before`
- `relation_status_after`
- `message`
- `error_message`

#### D. 撤回邀请 / 移除成员 / 手动绑定

动作接口：

- `POST /api/team/teams/{team_id}/memberships/{membership_id}/revoke`
- `POST /api/team/teams/{team_id}/memberships/{membership_id}/remove`
- `POST /api/team/memberships/{membership_id}/bind-local-account`

统一最小返回体：

- `success`
- `message`
- `team_id`
- `membership_id`
- `next_status`
- `refresh_required=true`

前端刷新链路：

1. 动作成功后刷新当前 Team 详情
2. 同步刷新当前 membership 列表
3. 若涉及账号映射变化，再刷新账号页 Team badge 数据

非法动作约定：

- 对 `invited` 执行 `remove` -> `400`
- 对 `joined/already_member` 执行 `revoke` -> `400`

#### E. 账号页 Team badge 数据来源

账号页不直接拼装上游数据，统一读取后端聚合字段：

- `team_role_badges`
- `team_relation_summary`
- `team_relation_count`

这些字段由后端基于 `teams + team_memberships` 聚合后返回，避免前端重复推导业务规则

账号页聚合展示规则：

- 若同一本地账号同时是 Team A 母号、Team B 子号：
  - badge 主标签显示 `母号`
  - 辅助标签显示 `子号(1+)`
  - 详情面板列出全部 Team 关系

---

## 11. 错误处理与幂等

### 11.1 错误分类

- 账号失效 / 封禁
- Token 过期且刷新失败
- Team 满员
- 网络/接口临时异常
- ghost success
- 数据同步不一致

### 11.2 处理规则

- 明确失效：`banned`
- Token 无法刷新：`expired`
- 满员：`full`
- 临时错误：`error`
- 邀请成功但未确认加入：先记 `invited`，后续同步确认为 `joined`

### 11.3 幂等规则

- 同步按 `(team_id, member_email)` 做 upsert
- 邀请前先查本地关系，避免重复邀请
- 同一 Team 上的同步/邀请/移除建议串行

### 11.4 串行约束与任务互斥

同一 Team 上以下写操作必须互斥：

- `sync_team`
- `invite_members`
- `revoke_invite`
- `remove_member`

规则：

- 同一时刻只允许一个写任务持有 Team 锁
- 新任务进入时若已有互斥任务运行，返回 `409`
- 只读查询不受影响

### 11.5 `already_member` 与 `ghost success` 规则

#### `already_member`

- 视为业务成功变种
- `team_task_items.item_status=success`
- `team_memberships.membership_status=already_member`
- 若后续同步确认该邮箱确实在成员列表中，则升级为 `joined`
- 升级以**下一次成功同步**为准，不要求在邀请任务内即时改写

#### `ghost success`

定义：

- 邀请接口返回 2xx
- 但在约定的同步确认窗口内，成员列表与邀请列表都未看到目标邮箱

落库策略：

- 当前 item 标记 `failed`
- Team 标记 `error`
- `team_memberships` 仅保留失败记录，不提升为 `invited/joined`

---

## 12. 测试策略

### 12.1 单元测试

覆盖：

- 母号识别逻辑
- Team 状态判定
- 成员关系状态流转
- 本地账号映射逻辑
- 邀请后不自动注册/刷新保护逻辑

### 12.2 集成测试

覆盖：

- Team 发现
- Team 同步
- 成员/邀请落库
- 批量邀请
- 撤回邀请 / 移除成员
- Team 任务落库

### 12.3 验收测试

核心验收点：

1. 能从本地账号发现母号
2. 能识别母号 / 子号 / 外部成员
3. 能同步 Team、成员、邀请状态
4. 能从本地账号池批量拉人
5. 邀请后不会自动刷新 RT 或自动注册
6. 可随时查看 Team 状态与任务状态

至少落成以下可执行场景：

1. **母号发现**
   - 本地账号返回 `plan_type=team` 且 `account_user_role=account-owner`
   - 成功落库 `teams`

2. **成员/邀请合并**
   - 同一邮箱同时出现在成员列表与邀请列表
   - 最终展示状态为 `joined`

3. **外部成员映射**
   - Team 中已有邮箱，本地暂无账号 -> `local_account_id` 为空
   - 本地后续出现同邮箱账号 -> 自动补链成功

4. **批量邀请去重**
   - 同批次重复邮箱只处理一次
   - 已在队成员不重复邀请

5. **`already_member` 记账**
   - 邀请接口返回已在队
   - 当前 item 记成功，关系状态为 `already_member`

6. **`ghost success` 落库**
   - 邀请接口返回成功，但确认窗口内未见目标邮箱
   - 当前 item 记失败，Team 记 `error`

7. **撤回邀请 / 移除成员**
   - `invited -> revoked`
   - `joined -> removed`

8. **同 Team 互斥约束**
   - Team 同步任务运行中发起批量邀请
   - 新请求收到 `409`，且不出现脏写

9. **禁止自动后续动作**
   - 邀请任务完成后，日志和任务结果明确表明未触发子号注册、未触发子号 RT 刷新

10. **一个母号发现多个 Team account**
   - 输入：单个本地母号账号返回多个 Team account
   - 预期数据库变化：`teams` 中新增多条 `(owner_account_id, upstream_account_id)` 记录
   - 预期 API 返回：总览页返回多条 Team
   - 预期页面提示：同一母号下展示多个 Team

11. **Team 状态判定**
   - 输入：分别制造 `full/expired/banned/error`
   - 预期数据库变化：`teams.status` 精确落对应状态
   - 预期 API 返回：详情页与列表页状态一致
   - 预期页面提示：可见明确状态 badge 与错误说明

12. **409 互斥提示**
   - 输入：在同一 Team 写任务运行中再提交另一个互斥写任务
   - 预期 API 返回：`409`
   - 预期页面提示：明确提示“当前 Team 正在处理其他写操作，请稍后再试”

13. **邀请后本地账号字段未变化**
   - 输入：对已存在本地账号发起邀请
   - 预期数据库变化：仅 `team_memberships/team_tasks/team_task_items/teams` 更新
   - 预期数据库不变：`accounts.password/access_token/refresh_token/id_token/session_token/client_id/cookies/status/source/registered_at/expires_at`

14. **跨 Team 多角色展示**
   - 输入：同一本地账号同时是 Team A 母号、Team B 子号
   - 预期数据库变化：`teams.owner_account_id` 与 `team_memberships.local_account_id` 同时成立
   - 预期 API 返回：账号聚合字段返回母号与子号双重信息
   - 预期页面提示：账号页主 badge 为 `母号`，并显示 `子号(1+)`

15. **手动绑定优先于自动补链**
   - 输入：某条 `membership` 已手动绑定 `local_account_id`
   - 操作：再次执行 Team 同步并触发自动补链
   - 预期数据库变化：`local_account_id` 保持不变
   - 预期页面提示：若邮箱不一致，出现绑定冲突告警

16. **去重计数矩阵**
   - 输入：同一 Team 内构造以下关系组合：
     - 邮箱 A：`joined + invited`
     - 邮箱 B：`already_member`
     - 邮箱 C：`invited`
     - 邮箱 D：`revoked`
     - 邮箱 E：`removed`
     - 邮箱 F：`failed`
   - 预期数据库变化：活跃唯一邮箱为 A/B/C，共 3 个
   - 预期 API 返回：
     - `current_members = 3`
     - `joined_count/active_count = 2`（A/B）
     - `invited_count = 1`（C）
   - 预期页面提示：活跃成员 Tab 显示 A/B，邀请中 Tab 显示 C

17. **异步入口闭环**
   - 输入：依次调用 discovery、sync、invite-accounts、invite-emails
   - 预期 API 返回：统一包含 `success/task_uuid/task_type/status/ws_channel`
   - 预期 websocket：`/api/ws/task/{task_uuid}` 可订阅到状态流转
   - 预期页面刷新：
     - discovery 完成后刷新 Team 总览
     - sync 完成后刷新 Team 详情
     - invite 完成后刷新 Team 详情 + 任务中心

18. **memberships 动作闭环**
   - 输入：从 `memberships` 列表获取 `id`
   - 操作：分别执行 `revoke/remove/bind-local-account`
   - 预期 API 返回：合法动作成功，非法动作返回 `400`，互斥冲突返回 `409`
   - 预期页面提示：动作后当前列表与详情同步刷新

19. **邮箱规范化归并**
   - 输入：同一邮箱分别以大小写差异、首尾空格、批量文本包裹形式出现
   - 预期数据库变化：最终只产生一条规范化 `member_email`
   - 预期 API 返回：计数与去重结果稳定，不出现重复成员

### 12.4 基线说明

当前主干基线测试并不完全干净：在 worktree 初始状态下执行

```bash
uv run --with pytest pytest
```

会因缺少 `httpx` 导致现有两处测试在收集阶段报错。该问题属于仓库既有问题，不属于本次 Team 设计范围，但实现阶段需要在测试计划中注明并避免误判。

---

## 13. 分阶段落地建议

### Phase 0：底座

- 新增 Team 数据表与迁移
- 新增 Team 状态常量
- 新增基础 CRUD 与任务骨架

### Phase 1：可用版闭环

- Team 母号发现
- Team 状态同步
- 成员/邀请同步
- 母号/子号关系映射
- Team 总览页
- Team 详情页
- 从本地账号池批量邀请
- Team 任务中心
- 邀请后仅更新状态，不自动注册/刷新

### Phase 2：管理动作补齐

- 撤回邀请
- 移除成员
- 手动绑定外部成员到本地账号
- 高级筛选与批量修复
- 账号页 Team badge 增强

### Phase 3：稳定性增强

- 更细粒度重试与退避
- 并发控制优化
- 日志可观测性增强
- 敏感字段展示脱敏

---

## 14. 推荐方案结论

本次采用：

**方案 A：在当前项目中原生重建完整 Team 管理模块。**

原因：

1. 当前项目已有成熟账号体系、任务体系、页面体系
2. 直接搬运 `team-manage` 整个后台会引入兑换码、质保、Webhook 等无关耦合
3. 当前项目已有 Team 识别与 Team Manager 上传基础能力，适合继续原生扩展
4. 用户明确要求完整 Team 管理，但不需要邀请后自动注册/刷新

---

## 15. 下一步

在用户确认本设计文档后，进入实现计划阶段：

1. 梳理实际文件改动清单
2. 拆分可并行实施任务
3. 生成正式 implementation plan
4. 再进入编码实施
