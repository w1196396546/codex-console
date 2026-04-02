import test from 'node:test';
import assert from 'node:assert/strict';

import actions from '../../static/js/accounts_state_actions.js';

const {
  buildAccountsQuery,
  buildBatchOperationPayload,
  buildBatchStatePayload,
  createLatestRequestOrchestrator,
  buildSingleStateRequest,
  deriveFilterChangeState,
  getBatchStateControlState,
  summarizeBatchStateResult,
} = actions;

test('RT 筛选会映射到 accounts 列表查询参数', () => {
  const query = buildAccountsQuery({
    page: 3,
    pageSize: 50,
    filters: {
      status: 'active',
      email_service: 'outlook',
      refresh_token_state: 'missing',
      search: 'demo',
    },
  });

  assert.equal(
    query,
    'page=3&page_size=50&status=active&email_service=outlook&refresh_token_state=missing&search=demo',
  );
});

test('筛选变更会重置分页并清空当前选择与跨页全选状态', () => {
  const nextState = deriveFilterChangeState({
    previousFilters: {
      status: 'active',
      email_service: '',
      refresh_token_state: 'has',
      search: '',
    },
    nextFilters: {
      status: 'active',
      email_service: '',
      refresh_token_state: 'missing',
      search: '',
    },
    currentPage: 4,
    selectedIds: new Set([1, 2]),
    selectAllPages: true,
  });

  assert.equal(nextState.currentPage, 1);
  assert.equal(nextState.selectAllPages, false);
  assert.deepEqual([...nextState.selectedIds], []);
  assert.equal(nextState.changed, true);
});

test('单账号改状态会组装 PATCH 请求', () => {
  const request = buildSingleStateRequest({ accountId: 42, status: 'banned' });

  assert.deepEqual(request, {
    path: '/accounts/42',
    method: 'PATCH',
    body: { status: 'banned' },
  });
});

test('批量改状态会把列表筛选映射为批量字段', () => {
  const payload = buildBatchStatePayload({
    status: 'expired',
    selectedIds: new Set([2, 5]),
    selectAllPages: true,
    filters: {
      status: 'active',
      email_service: 'outlook',
      refresh_token_state: 'has',
      search: 'vip',
    },
  });

  assert.deepEqual(payload, {
    ids: [],
    status: 'expired',
    select_all: true,
    status_filter: 'active',
    email_service_filter: 'outlook',
    refresh_token_state_filter: 'has',
    search_filter: 'vip',
  });
});

test('通用批量 payload helper 会统一映射 select_all 和筛选字段', () => {
  const payload = buildBatchOperationPayload({
    selectedIds: new Set([9, 3]),
    selectAllPages: true,
    filters: {
      status: 'active',
      email_service: 'outlook',
      refresh_token_state: 'missing',
      search: 'group',
    },
    extraFields: {
      reason: 'review',
    },
  });

  assert.deepEqual(payload, {
    ids: [],
    select_all: true,
    status_filter: 'active',
    email_service_filter: 'outlook',
    refresh_token_state_filter: 'missing',
    search_filter: 'group',
    reason: 'review',
  });
});

test('未选中账号时批量改状态按钮禁用，选中后显示数量', () => {
  const disabledState = getBatchStateControlState({
    selectedCount: 0,
    selectAllPages: false,
    totalAccounts: 12,
  });
  assert.deepEqual(disabledState, {
    disabled: true,
    count: 0,
    label: '批量改状态',
  });

  const enabledState = getBatchStateControlState({
    selectedCount: 2,
    selectAllPages: true,
    totalAccounts: 12,
  });
  assert.deepEqual(enabledState, {
    disabled: false,
    count: 12,
    label: '批量改状态 (12)',
  });
});

test('批量改状态结果提示支持成功、部分成功与筛选结果变化', () => {
  assert.deepEqual(
    summarizeBatchStateResult({
      requestedCount: 3,
      updatedCount: 3,
      errors: [],
    }),
    {
      level: 'success',
      message: '已成功更新 3 个账号状态',
    },
  );

  assert.deepEqual(
    summarizeBatchStateResult({
      requestedCount: 3,
      updatedCount: 2,
      errors: ['ID 9: boom'],
    }),
    {
      level: 'warning',
      message: '部分成功：已更新 2 个账号，1 个失败。筛选结果在提交期间发生变化。',
    },
  );

  assert.deepEqual(
    summarizeBatchStateResult({
      requestedCount: 3,
      updatedCount: 2,
      skippedCount: 1,
      missingIds: [99],
      errors: [],
    }),
    {
      level: 'warning',
      message: '部分成功：已更新 2 个账号，跳过 1 个不存在账号。',
    },
  );

  assert.deepEqual(
    summarizeBatchStateResult({
      requestedCount: 3,
      updatedCount: 2,
      errors: [],
    }),
    {
      level: 'warning',
      message: '部分成功：已更新 2 个账号，0 个失败。筛选结果在提交期间发生变化。',
    },
  );
});

test('连续筛选变更时只应用最后一次请求结果', async () => {
  let resolveFirst;
  let resolveSecond;
  const fetchCalls = [];
  const appliedResults = [];

  const orchestrator = createLatestRequestOrchestrator({
    fetcher: (filters) => {
      fetchCalls.push(filters.search);
      if (filters.search === 'first') {
        return new Promise((resolve) => {
          resolveFirst = () => resolve({ rows: ['first'] });
        });
      }
      return new Promise((resolve) => {
        resolveSecond = () => resolve({ rows: ['second'] });
      });
    },
    applyResult: (result, filters) => {
      appliedResults.push({ result, filters });
    },
  });

  const firstPromise = orchestrator.request({ search: 'first' });
  const secondPromise = orchestrator.request({ search: 'second' });

  resolveFirst();
  await Promise.resolve();
  assert.deepEqual(fetchCalls, ['first', 'second']);
  assert.deepEqual(appliedResults, []);

  resolveSecond();
  await firstPromise;
  await secondPromise;

  assert.deepEqual(appliedResults, [
    {
      result: { rows: ['second'] },
      filters: { search: 'second' },
    },
  ]);
});
