import test from 'node:test';
import assert from 'node:assert/strict';

import selector from '../../static/js/outlook_account_selector.js';

const {
  buildSelectionSummary,
  createInitialSelectedIds,
  deselectVisibleAccounts,
  filterAccounts,
  selectVisibleAccounts,
  selectVisibleUnregisteredAccounts,
} = selector;

const accounts = [
  { id: 1, email: 'alpha@outlook.com', is_registered: false, has_oauth: true },
  { id: 2, email: 'beta@outlook.com', is_registered: true, has_oauth: false },
  { id: 3, email: 'gamma@outlook.com', is_registered: false, has_oauth: true },
  { id: 4, email: 'delta@example.com', is_registered: true, has_oauth: false },
];

test('createInitialSelectedIds 默认勾选未注册账户', () => {
  const selected = createInitialSelectedIds(accounts);
  assert.deepEqual([...selected].sort((a, b) => a - b), [1, 3]);
});

test('filterAccounts 支持邮箱关键字和注册状态联合筛选', () => {
  const filtered = filterAccounts(accounts, {
    keyword: 'outlook',
    status: 'unregistered',
  });

  assert.deepEqual(filtered.map((item) => item.id), [1, 3]);
});

test('selectVisibleAccounts 会在已有选择上叠加，而不是覆盖', () => {
  const selected = new Set([1]);
  const visible = accounts.filter((item) => item.id === 2 || item.id === 3);

  const nextSelected = selectVisibleAccounts(selected, visible);

  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 2, 3]);
});

test('selectVisibleUnregisteredAccounts 只追加当前可见的未注册账户', () => {
  const selected = new Set([2]);
  const visible = accounts.filter((item) => item.id !== 4);

  const nextSelected = selectVisibleUnregisteredAccounts(selected, visible);

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
