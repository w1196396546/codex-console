import test from 'node:test';
import assert from 'node:assert/strict';

import selector from '../../static/js/outlook_account_selector.js';

const {
    buildSelectionSummary,
    collectNormalizedEmails,
    countExecutableAccounts,
    createInitialSelectedIds,
    deselectVisibleAccounts,
    filterAccounts,
    getExecutionStateLabel,
    getVisibleSelectedIds,
    isExecutableAccount,
    mapExecutionState,
    resolveSelectedIdsByEmails,
    selectExecutableVisibleAccounts,
    selectVisibleAccounts,
    selectVisibleExecutableAccounts,
} = selector;

const accounts = [
  {
    id: 1,
    email: 'alpha@outlook.com',
    is_registered: false,
    needs_token_refresh: false,
    is_registration_complete: false,
    has_oauth: true,
  },
  {
    id: 2,
    email: 'beta@outlook.com',
    is_registered: true,
    needs_token_refresh: true,
    is_registration_complete: false,
    has_oauth: false,
  },
  {
    id: 3,
    email: 'gamma@outlook.com',
    is_registered: false,
    needs_token_refresh: false,
    is_registration_complete: false,
    has_oauth: true,
  },
  {
    id: 4,
    email: 'delta@example.com',
    is_registered: true,
    needs_token_refresh: false,
    is_registration_complete: true,
    has_oauth: false,
  },
];

const inconsistentAccounts = [
  {
    id: 11,
    email: 'complete-priority@outlook.com',
    is_registered: false,
    is_registration_complete: true,
  },
  {
    id: 12,
    email: 'registered-priority@outlook.com',
    is_registered: true,
    needs_token_refresh: false,
    is_registration_complete: false,
  },
  {
    id: 13,
    email: 'missing-fields@outlook.com',
  },
  {
    id: 14,
    email: 'invalid-types@outlook.com',
    is_registered: 'yes',
    is_registration_complete: 0,
  },
];

const reloadedAccounts = [
  {
    id: 21,
    email: 'reload-unregistered@outlook.com',
    is_registered: false,
    is_registration_complete: false,
  },
  {
    id: 22,
    email: 'reload-needs-refresh@outlook.com',
    is_registered: true,
    is_registration_complete: false,
  },
  {
    id: 23,
    email: 'reload-invalid-complete-flag@outlook.com',
    is_registered: true,
    is_registration_complete: 'false',
  },
  {
    id: 24,
    email: 'reload-complete@outlook.com',
    is_registered: false,
    is_registration_complete: true,
  },
  {
    id: 25,
    email: 'reload-missing-fields@outlook.com',
  },
];

test('createInitialSelectedIds 默认只勾选可执行账户', () => {
  const selected = createInitialSelectedIds(accounts);
  assert.deepEqual([...selected].sort((a, b) => a - b), [1, 2, 3]);
});

test('countExecutableAccounts 统计未注册与待补 Token 账户总和', () => {
  assert.equal(countExecutableAccounts(accounts), 3);
});

test('isExecutableAccount 与默认勾选语义保持一致', () => {
  assert.equal(isExecutableAccount(accounts[0]), true);
  assert.equal(isExecutableAccount(accounts[1]), true);
  assert.equal(isExecutableAccount(accounts[3]), false);
});

test('getExecutionStateLabel 返回 UI 所需的稳定文案', () => {
  assert.equal(getExecutionStateLabel('unregistered'), '未注册');
  assert.equal(getExecutionStateLabel('registered_needs_token_refresh'), '已注册，待补 Token');
  assert.equal(getExecutionStateLabel('registered_complete'), '注册已完成');
});

test('filterAccounts 支持邮箱关键字和执行状态联合筛选', () => {
  const filtered = filterAccounts(accounts, {
    keyword: 'outlook',
    executionState: 'unregistered',
  });

  assert.deepEqual(filtered.map((item) => item.id), [1, 3]);
});

test('filterAccounts 支持多行邮箱精确筛选', () => {
  const filtered = filterAccounts(accounts, {
    keyword: ' beta@outlook.com \nGAMMA@outlook.com\n',
  });

  assert.deepEqual(filtered.map((item) => item.id), [2, 3]);
});

test('collectNormalizedEmails 支持从 Outlook 导入格式中提取邮箱', () => {
  const emails = collectNormalizedEmails([
    ' yzyex92338376@hotmail.com----zrebi10493324----client-id-1----refresh-token-1 ',
    'gamma@outlook.com----password-2',
    'YZYEX92338376@HOTMAIL.COM',
  ]);

  assert.deepEqual(emails, [
    'yzyex92338376@hotmail.com',
    'gamma@outlook.com',
  ]);
});

test('filterAccounts 支持粘贴 Outlook 导入行后精确匹配对应邮箱', () => {
  const filtered = filterAccounts(accounts, {
    keyword: ' beta@outlook.com----plain-secret \nGAMMA@outlook.com----oauth-secret----client-id-1----refresh-token-1\n',
  });

  assert.deepEqual(filtered.map((item) => item.id), [2, 3]);
});

test('filterAccounts 在多行邮箱筛选时继续叠加执行状态过滤', () => {
  const filtered = filterAccounts(accounts, {
    keyword: 'beta@outlook.com\ngamma@outlook.com',
    executionState: 'unregistered',
  });

  assert.deepEqual(filtered.map((item) => item.id), [3]);
});

test('切换筛选不会意外重置或改写已选集合', () => {
  const selected = createInitialSelectedIds(accounts);
  const before = [...selected].sort((a, b) => a - b);

  const completeVisible = filterAccounts(accounts, { executionState: 'registered_complete' });
  const refreshVisible = filterAccounts(accounts, { executionState: 'registered_needs_token_refresh' });

  assert.deepEqual([...selected].sort((a, b) => a - b), before);
  assert.deepEqual([...getVisibleSelectedIds(selected, completeVisible)], []);
  assert.deepEqual([...getVisibleSelectedIds(selected, refreshVisible)], [2]);
});

test('selectVisibleAccounts 会在已有选择上叠加，而不是覆盖', () => {
  const selected = new Set([1]);
  const visible = accounts.filter((item) => item.id === 2 || item.id === 3);

  const nextSelected = selectVisibleAccounts(selected, visible);

  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 2, 3]);
});

test('selectVisibleExecutableAccounts 只追加当前可见的可执行账户', () => {
  const selected = new Set([2]);
  const visible = accounts.filter((item) => item.id !== 4);

  const nextSelected = selectVisibleExecutableAccounts(selected, visible);

  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 2, 3]);
});

test('selectExecutableVisibleAccounts 会排除 registered_complete', () => {
  const nextSelected = selectExecutableVisibleAccounts(new Set(), accounts);

  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 2, 3]);
});

test('deselectVisibleAccounts 只移除当前可见结果，不影响隐藏的已选项', () => {
  const selected = new Set([1, 2, 3, 4]);
  const visible = accounts.filter((item) => item.id === 2 || item.id === 3);

  const nextSelected = deselectVisibleAccounts(selected, visible);

  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 4]);
});

test('buildSelectionSummary 会提示当前筛选外仍保留的已选数量', () => {
  const summary = buildSelectionSummary({
    totalCount: accounts.length,
    filteredCount: 2,
    selectedIds: new Set([1, 2, 3]),
    visibleSelectedIds: new Set([2]),
  });

  assert.equal(summary, '已选 3 / 4 个账户，当前显示 2 个，其中 2 个已选项已被筛选隐藏');
});

test('resolveSelectedIdsByEmails 按邮箱精确匹配可用的 Outlook 账户 ID', () => {
  const selected = resolveSelectedIdsByEmails(accounts, [
    'GAMMA@outlook.com',
    'missing@outlook.com',
    'beta@outlook.com',
    'beta@outlook.com',
  ]);

  assert.deepEqual([...selected].sort((a, b) => a - b), [2, 3]);
});

test('mapExecutionState 将缺少 RT 的已注册账户归类为待补 Token', () => {
  assert.equal(mapExecutionState(accounts[1]), 'registered_needs_token_refresh');
});

test('mapExecutionState 当前不引入 account.status 额外语义', () => {
  assert.equal(mapExecutionState({
    status: 'expired',
    is_registered: true,
    is_registration_complete: false,
  }), 'registered_needs_token_refresh');
});

test('filterAccounts 支持 executionState=registered_needs_token_refresh', () => {
  const filtered = filterAccounts(accounts, {
    executionState: 'registered_needs_token_refresh',
  });

  assert.deepEqual(filtered.map((item) => item.id), [2]);
});

test('filterAccounts 支持 executionState=registered_complete', () => {
  const filtered = filterAccounts(accounts, {
    executionState: 'registered_complete',
  });

  assert.deepEqual(filtered.map((item) => item.id), [4]);
});

test('mapExecutionState 按优先级处理 is_registered=true 但未标记 needs_token_refresh 的异常组合', () => {
  assert.equal(mapExecutionState(inconsistentAccounts[1]), 'registered_needs_token_refresh');
});

test('mapExecutionState 按优先级处理 is_registered=false 但 complete=true 的异常组合', () => {
  assert.equal(mapExecutionState(inconsistentAccounts[0]), 'registered_complete');
});

test('mapExecutionState 将字段缺失归类为 unregistered', () => {
  assert.equal(mapExecutionState(inconsistentAccounts[2]), 'unregistered');
});

test('mapExecutionState 容忍异常字段类型且不产生第四种状态', () => {
  assert.equal(mapExecutionState(inconsistentAccounts[3]), 'registered_needs_token_refresh');
});

test('数据重新拉取后默认勾选与状态映射仍保持一致', () => {
  const selected = createInitialSelectedIds(reloadedAccounts);
  const mappedStates = reloadedAccounts.map((account) => mapExecutionState(account));

  assert.deepEqual([...selected].sort((a, b) => a - b), [21, 22, 23, 25]);
  assert.deepEqual(mappedStates, [
    'unregistered',
    'registered_needs_token_refresh',
    'registered_needs_token_refresh',
    'registered_complete',
    'unregistered',
  ]);
  assert.deepEqual([...new Set(mappedStates)].sort(), [
    'registered_complete',
    'registered_needs_token_refresh',
    'unregistered',
  ]);
});
