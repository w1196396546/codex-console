# 注册页与账号管理状态统一设计

- 日期：2026-04-02
- 主题：注册页 Outlook 执行状态筛选 + 账号管理 RT 筛选与状态操作
- 结论：采用 `C-lite` 方案，即统一业务状态语义，但不做全仓库大规模重构

## 1. 背景

当前系统已经具备以下基础能力：

- 注册页 Outlook 候选接口已经能区分：
  - 未注册
  - 已注册但缺少 `refresh_token`
  - 注册已完成
- 账号表已经有：
  - `refresh_token`
  - `status`
- 账号管理已经有：
  - 单账号状态更新接口
  - 批量状态更新接口雏形

但前端交互与接口契约还没有统一表达这套语义，导致：

- 注册页里“未注册可直接注册”和“已注册待补 Token”混在一起，不方便精准筛选
- 账号管理无法按 RT 存在性筛选
- 账号管理虽有状态更新能力，但前端没有暴露单个/批量改状态入口
- 批量状态更新接口还不支持“选择当前筛选结果全集”的常见管理场景

## 2. 目标

本次改动需要同时达成四个目标：

1. 注册页 Outlook 账户支持按业务执行状态筛选
2. 注册页默认勾选逻辑只覆盖“可执行项”
3. 账号管理支持按 RT 存在性筛选
4. 账号管理支持单个和批量手动改状态

## 3. 非目标

本次不做以下事项：

- 不重构所有注册链路状态流转
- 不引入新的数据库表
- 不把全项目所有状态枚举统一成一个中心模块
- 不改动支付、总览、邮箱服务页面的现有状态模型

## 4. 统一业务语义

### 4.1 注册页 Outlook 执行状态

注册页使用以下三类业务状态：

- `unregistered`
  - 含义：邮箱未注册，可直接发起注册
  - 判定：`is_registered = false`
- `registered_needs_token_refresh`
  - 含义：邮箱已注册，但账号缺少 `refresh_token`，需要补 Token
  - 判定：`is_registered = true && needs_token_refresh = true`
- `registered_complete`
  - 含义：邮箱已注册，且已有 `refresh_token`
  - 判定：`is_registration_complete = true`

为避免前后端对布尔字段组合做不同解释，本次明确引入唯一映射规则。

#### 唯一映射优先级

前端与后端都按以下优先级解释 Outlook 候选状态：

1. `registered_complete`
   - 条件：`is_registration_complete = true`
2. `registered_needs_token_refresh`
   - 条件：`is_registration_complete != true && is_registered = true`
3. `unregistered`
   - 条件：`is_registered != true`

#### 闭合性要求

- 三类状态必须互斥
- 三类状态必须覆盖全部 Outlook 候选
- 即使后端返回的布尔字段组合出现不一致，前端也必须按上述优先级映射出唯一状态

#### 异常字段组合处理

若出现以下不一致组合：

- `is_registered = true && needs_token_refresh = false && is_registration_complete = false`
- `is_registered = false && is_registration_complete = true`
- 任意字段缺失或类型异常

处理策略如下：

- 前端不直接相信 `needs_token_refresh` 的字面值
- 前端一律以上述优先级做最终状态归类
- 后端后续实现可保持现有字段，但不得返回额外第四种前端无法归类的执行状态

### 4.2 账号管理 RT 状态

账号管理页使用以下两类 RT 状态：

- `has_rt`
- `missing_rt`

映射规则：

- `has_rt`：`refresh_token` 非空
- `missing_rt`：`refresh_token` 为空

### 4.3 账号管理账号状态

账号管理继续沿用现有状态：

- `active`
- `expired`
- `banned`
- `failed`

## 5. 注册页设计

### 5.1 筛选交互

在 Outlook 批量注册区域新增业务状态筛选下拉：

- 全部
- 未注册
- 已注册待补Token
- 注册已完成

保留邮箱搜索框，两者可组合使用。

### 5.2 默认勾选策略

默认勾选仅覆盖可执行项：

- 默认勾选：`unregistered`
- 默认勾选：`registered_needs_token_refresh`
- 默认不勾选：`registered_complete`

这保证“只勾选可执行”与用户的真实目标一致。

#### 与执行接口的对齐规则

本次同步重定义 Outlook 批量注册里的 `skip_registered` 语义：

- 旧语义：跳过所有已注册邮箱
- 新语义：仅跳过 `registered_complete`

因此：

- `registered_needs_token_refresh` 被视为可执行项
- 若选中 `registered_needs_token_refresh`，后端不得因“已注册”而直接跳过
- 服务端实际过滤逻辑改为：
  - 跳过：`registered_complete`
  - 继续执行：`unregistered`、`registered_needs_token_refresh`

这条规则必须与前端默认勾选和“只选可执行”保持一致。

### 5.3 选择行为

延续现有“筛选不清空已选集”的交互，但进一步约束快捷操作含义：

- `全选当前结果`
  - 仅追加当前筛选可见项
- `只选可执行`
  - 仅追加当前筛选结果中属于可执行状态的账户
- `清空当前结果`
  - 仅移除当前筛选可见项
- 切换筛选不会清空已选集合
- 再次筛选后继续勾选只会叠加，不会覆盖先前选择

### 5.4 已选摘要

摘要区显示：

- 已选总数
- 当前显示数量
- 当前被筛选隐藏但仍保留的已选数量

## 6. 账号管理页设计

### 6.1 RT 筛选

在工具栏新增 RT 筛选下拉：

- 全部
- 有RT
- 无RT

该筛选与现有：

- 状态筛选
- 邮箱服务筛选
- 搜索

并列生效。

### 6.2 单账号改状态

在每行“更多”菜单中新增“设置状态”子操作：

- 设为 active
- 设为 expired
- 设为 banned
- 设为 failed

执行方式：

- 前端直接调用现有 `PATCH /accounts/{account_id}`
- 成功后刷新当前列表

### 6.3 批量改状态

在顶部工具栏新增“批量改状态”入口。

建议采用下拉动作菜单：

- 设为 active
- 设为 expired
- 设为 banned
- 设为 failed

交互规则：

- 未选中任何账号时禁用
- 使用“全选当前筛选结果全集”时，批量改状态应作用于全集，而不是仅当前页
- 执行前弹确认框
- 确认框展示数量以发起时前端已知数量为准
- 服务端响应中的 `updated_count` 作为最终实际结果展示
- 若最终命中数量与确认框数量不一致，前端明确提示“筛选结果在提交期间发生变化”

## 7. 后端接口设计

### 7.1 注册页 Outlook 候选接口

复用现有接口：

- `GET /registration/outlook-accounts`

不新增接口，只正式消费已有字段：

- `is_registered`
- `has_refresh_token`
- `needs_token_refresh`
- `is_registration_complete`

前端基于这些字段映射业务状态。

### 7.2 账号列表接口

扩展：

- `GET /accounts`

新增查询参数：

- `refresh_token_state`
  - 可选值：`has` / `missing`
  - 非法值：返回 `400`

新增返回字段：

- `has_refresh_token: bool`

### 7.3 批量状态更新接口

复用现有接口：

- `POST /accounts/batch-update`

将请求体从当前的：

- `ids`
- `status`

扩展为支持：

- `ids`
- `status`
- `select_all`
- `status_filter`
- `email_service_filter`
- `search_filter`
- `refresh_token_state_filter`

批量接口沿用当前仓库已有批量请求体命名风格，即：

- `status_filter`
- `email_service_filter`
- `search_filter`
- `refresh_token_state_filter`

列表接口继续使用查询参数命名：

- `status`
- `email_service`
- `search`
- `refresh_token_state`

#### 列表参数到批量参数的映射规则

- `status` -> `status_filter`
- `email_service` -> `email_service_filter`
- `search` -> `search_filter`
- `refresh_token_state` -> `refresh_token_state_filter`

前端在发起“按当前筛选结果全集批量改状态”时，必须执行这层显式映射。

#### 请求契约

- `ids`
  - 类型：`List[int]`
  - 默认：`[]`
- `status`
  - 类型：`str`
  - 必填
- `select_all`
  - 类型：`bool`
  - 默认：`false`
- `status_filter`
  - 类型：`Optional[str]`
- `email_service_filter`
  - 类型：`Optional[str]`
- `search_filter`
  - 类型：`Optional[str]`
- `refresh_token_state_filter`
  - 类型：`Optional[str]`
  - 值域：`has` / `missing`
  - 非法值：返回 `400`

#### 字段优先级

服务端处理顺序固定为：

1. 先校验请求体字段合法性
2. 若存在非法枚举值，直接返回 `400`
3. 再根据 `select_all` 判定采用 `ids` 还是筛选条件

因此：

- 即使 `select_all = false`
- 只要传入非法的 `refresh_token_state_filter`
- 仍然返回 `400`

- 当 `select_all = false` 时：
  - 仅使用 `ids`
  - 其他筛选字段忽略
- 当 `select_all = true` 时：
  - 以服务端实时查询结果为准
  - `ids` 被忽略
  - 所有筛选字段参与组合过滤

#### 空值语义

- `null`、空字符串、缺失字段，均视为“不参与该条件过滤”
- `search_filter = \"\"` 等价于未传

#### 0 命中与部分失败

- 若最终命中 0 条：
  - 返回 `success = true`
  - 返回 `updated_count = 0`
  - 返回可读消息，提示当前筛选结果下无可更新账号
- 若部分账号更新失败：
  - 返回 `updated_count`
  - 返回 `errors`
  - 前端提示“部分成功”

#### 计数口径

- 确认框数量：以前端发起时本地已知数量为准
- 实际更新数量：以后端实时解析出的 `updated_count` 为准
- 若二者不一致，前端需提示筛选结果已变化

## 8. 代码改动边界

### 8.1 注册页

- `templates/index.html`
  - 筛选下拉文案与说明
- `static/js/app.js`
  - 消费新增业务状态筛选
  - 调整默认勾选
  - 调整“只选可执行”逻辑
- `static/js/outlook_account_selector.js`
  - 扩展状态过滤与选择器纯函数
- `tests/frontend/outlook_account_selector.test.mjs`
  - 增加新状态与默认勾选测试

### 8.2 账号管理

- `templates/accounts.html`
  - 新增 RT 筛选
  - 新增批量改状态入口
- `static/js/accounts.js`
  - 传递 RT 筛选参数
  - 新增单个改状态和批量改状态动作
- `src/web/routes/accounts.py`
  - 账号列表接口支持 RT 筛选与返回 `has_refresh_token`
  - 批量改状态接口支持 `select_all + filters`
- `tests/`
  - 补账号管理接口测试

## 9. 实施顺序

采用两波并行，降低写冲突。

### Wave 1

- 注册页状态筛选与选择器测试
- 账号管理后端接口扩展与 pytest 测试

### Wave 2

- 账号管理前端 RT 筛选
- 单个改状态与批量改状态 UI
- 联调与回归验证

## 10. 测试方案

### 10.1 前端逻辑测试

使用 `node:test` 覆盖：

- Outlook 三类业务状态筛选
- 默认仅勾选可执行项
- 筛选后重复选择只叠加不覆盖
- “只选可执行”不选入已完成项
- “清空当前结果”不误删隐藏已选项
- 异常布尔组合按唯一优先级被稳定映射
- 切换筛选后默认勾选集合保持稳定
- 数据重新拉取后默认勾选与状态映射保持一致

### 10.2 后端接口测试

使用 `pytest` 覆盖：

- `GET /accounts?refresh_token_state=has`
- `GET /accounts?refresh_token_state=missing`
- `GET /accounts?refresh_token_state=非法值` 返回 `400`
- `GET /accounts` 在 `refresh_token_state + status + email_service + search` 组合下过滤正确
- `PATCH /accounts/{id}` 改状态
- `POST /accounts/batch-update` 指定 ID 批量改状态
- `POST /accounts/batch-update` 在 `select_all=false + ids + filters` 下仅按 `ids` 更新
- `POST /accounts/batch-update` 在 `select_all=false + ids + refresh_token_state_filter=非法值` 下返回 `400`
- `POST /accounts/batch-update` 在 `select_all + filters` 下批量改状态
- `POST /accounts/batch-update` 在 `select_all=true + ids + filters` 下忽略 `ids`
- `POST /accounts/batch-update` 在 `select_all=true + refresh_token_state_filter=非法值` 下返回 `400`
- `POST /accounts/batch-update` 在 `select_all + refresh_token_state` 下批量改状态
- `POST /accounts/batch-update` 在 `select_all + 多筛选组合` 下批量改状态
- `POST /accounts/batch-update` 在 0 命中时返回可预期结果
- `POST /accounts/batch-update` 在部分失败时返回 `updated_count + errors`

### 10.3 轻量验证

至少执行：

- `node --test` 前端逻辑测试
- `node --check` 相关 JS 文件
- `pytest` 相关接口测试
- `git diff --check`

## 11. 风险与防护

### 风险 1

注册页“可执行项”定义与后端跳过逻辑不一致。

防护：

- 前端按唯一优先级映射状态，不直接信任零散布尔组合
- `skip_registered` 明确收敛为“仅跳过 registered_complete”
- 默认勾选逻辑与后端执行过滤保持同源语义

### 风险 2

批量改状态只覆盖当前页，没有覆盖“当前筛选全集”。

防护：

- 统一复用 `select_all + filters` 解析模式
- 在契约中明确 `select_all=true` 时由服务端实时查询决定全集
- 前端展示“确认数量”和“最终更新数量”两种计数

### 风险 3

RT 筛选与现有状态筛选组合后出现意外空结果或漏筛。

防护：

- 后端统一组合过滤
- 前端只传参，不在列表页自行做二次推断

### 风险 4

列表接口与批量接口筛选参数命名不一致，导致联调混乱。

防护：

- 文档明确保留“列表查询参数”和“批量请求体字段”两套命名
- 前端在批量请求发起前执行固定映射
- 测试覆盖参数映射与非法值校验

## 12. 推荐实施决策

本次按 `C-lite` 方案落地：

- 统一业务状态语义
- 不做全局大重构
- 复用现有接口和模型
- 通过少量后端补强支撑完整前端体验

这是当前收益、风险、改动面最均衡的实现方式。
