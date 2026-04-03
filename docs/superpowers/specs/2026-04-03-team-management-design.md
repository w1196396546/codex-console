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

- `unique(owner_account_id, upstream_account_id)`

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

### 6.3 `team_tasks`

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

### 6.4 `team_task_items`

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

### 9.3 成员关系管理

- `GET /api/team/teams/{team_id}/memberships`
- `POST /api/team/teams/{team_id}/memberships/{membership_id}/revoke`
- `POST /api/team/teams/{team_id}/memberships/{membership_id}/remove`
- `POST /api/team/memberships/{membership_id}/bind-local-account`

### 9.4 批量邀请

- `POST /api/team/teams/{team_id}/invite-accounts`
- `POST /api/team/teams/{team_id}/invite-emails`

### 9.5 任务监控

- `GET /api/team/tasks`
- `GET /api/team/tasks/{task_uuid}`
- 复用现有 websocket 任务监控机制，或补充 `/api/ws/team-task/{task_uuid}`

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
- 外部成员数 / 本地子号数

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

