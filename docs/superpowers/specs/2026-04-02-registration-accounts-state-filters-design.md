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

当 `select_all = true` 时，服务端通过统一筛选条件解析实际账号 ID 集合。

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

### 10.2 后端接口测试

使用 `pytest` 覆盖：

- `GET /accounts?refresh_token_state=has`
- `GET /accounts?refresh_token_state=missing`
- `PATCH /accounts/{id}` 改状态
- `POST /accounts/batch-update` 指定 ID 批量改状态
- `POST /accounts/batch-update` 在 `select_all + filters` 下批量改状态

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

- 前端只消费后端字段，不再自定义猜测
- 默认勾选逻辑与 `skip_registered` 判定保持同源语义

### 风险 2

批量改状态只覆盖当前页，没有覆盖“当前筛选全集”。

防护：

- 统一复用 `select_all + filters` 解析模式

### 风险 3

RT 筛选与现有状态筛选组合后出现意外空结果或漏筛。

防护：

- 后端统一组合过滤
- 前端只传参，不在列表页自行做二次推断

## 12. 推荐实施决策

本次按 `C-lite` 方案落地：

- 统一业务状态语义
- 不做全局大重构
- 复用现有接口和模型
- 通过少量后端补强支撑完整前端体验

这是当前收益、风险、改动面最均衡的实现方式。
