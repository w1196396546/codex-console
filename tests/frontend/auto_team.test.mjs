import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

import autoTeam from '../../static/js/auto_team.js';

const {
  afterSuccessfulMembershipAction,
  buildMembershipActionRequest,
  createAcceptedTaskFlow,
  deriveInitialTeamState,
  resolveInviteAvailability,
} = autoTeam;

class FakeWebSocket {
  constructor(url) {
    this.url = url;
    this.listeners = new Map();
    this.closed = false;
  }

  addEventListener(type, listener) {
    const current = this.listeners.get(type) || [];
    current.push(listener);
    this.listeners.set(type, current);
  }

  close() {
    this.closed = true;
  }

  emit(type, event = {}) {
    const current = this.listeners.get(type) || [];
    for (const listener of current) {
      listener(event);
    }
  }
}

function createStubElement() {
    return {
    value: '',
    textContent: '',
    innerHTML: '',
    hidden: false,
    disabled: false,
    classList: {
      add() {},
      remove() {},
    },
    addEventListener() {},
    removeEventListener() {},
    querySelector() {
      return null;
    },
    closest() {
      return null;
    },
    getAttribute() {
      return null;
    },
    setAttribute() {},
    };
}

function createBrowserLikeAutoTeamContext({ search = '', fetchImpl, WebSocketImpl } = {}) {
  const selectorMap = new Map();
  const requiredSelectors = [
    '[data-role="owner-filter"]',
    '[data-role="status-filter"]',
    '[data-role="search-filter"]',
    '[data-role="team-list"]',
    '[data-role="teams-status"]',
    '[data-role="metric-total-teams"]',
    '[data-role="metric-total-seats"]',
    '[data-role="metric-active-tasks"]',
    '[data-role="team-detail-title"]',
    '[data-role="team-detail-status"]',
    '[data-role="detail-owner-email"]',
    '[data-role="detail-owner-id"]',
    '[data-role="detail-members"]',
    '[data-role="detail-sync-status"]',
    '[data-role="detail-last-sync"]',
    '[data-role="detail-seats"]',
    '[data-role="team-detail-callout"]',
    '[data-role="task-live-status"]',
    '[data-role="task-current-uuid"]',
    '[data-role="task-current-summary"]',
    '[data-role="task-list"]',
  ];

  for (const selector of requiredSelectors) {
    selectorMap.set(selector, createStubElement());
  }

  const root = createStubElement();
  root.querySelector = (selector) => selectorMap.get(selector) || null;

  const browserContext = {
    console,
    Date,
    URLSearchParams,
    fetch: fetchImpl || (async () => ({
      ok: true,
      json: async () => ({ items: [] }),
    })),
    WebSocket: WebSocketImpl,
    setTimeout,
    clearTimeout,
    location: {
      origin: 'http://localhost',
      search,
    },
  };
  browserContext.globalThis = browserContext;
  browserContext.window = browserContext;

  vm.createContext(browserContext);
  const source = readFileSync(
    new URL('../../static/js/auto_team.js', import.meta.url),
    'utf8',
  );
  vm.runInContext(source, browserContext);

  return {
    autoTeamPage: browserContext.AutoTeamPage,
    root,
  };
}

test('deriveInitialTeamState 会从 query 解析 Team 页面初始状态', () => {
  const state = deriveInitialTeamState('?team_id=42&owner_account_id=9&status=active&search=alpha');

  assert.deepEqual(state, {
    teams: [],
    taskItems: [],
    selectedTeamId: 42,
    activeTaskUuid: '',
    filters: {
      ownerAccountId: 9,
      status: 'active',
      search: 'alpha',
    },
    loading: {
      teams: false,
      detail: false,
      tasks: false,
    },
  });
});

test('initPage 在浏览器上下文不会依赖 Node global', async () => {
  const { autoTeamPage, root } = createBrowserLikeAutoTeamContext();

  await autoTeamPage.initPage(root);
});

test('initPage 会在 owner_account_id 存在时自动触发 discovery', async () => {
  const fetchCalls = [];
  const sockets = [];
  const { autoTeamPage, root } = createBrowserLikeAutoTeamContext({
    search: '?owner_account_id=126',
    fetchImpl: async (path, options = {}) => {
      fetchCalls.push([path, options.method || 'GET']);
      if (path === '/api/team/discovery/run') {
        return {
          ok: true,
          json: async () => ({
            task_uuid: 'task-owner-126',
            task_type: 'discover_owner_teams',
            status: 'pending',
            owner_account_id: 126,
            scope_type: 'owner',
            scope_id: '126',
            ws_channel: '/api/ws/task/task-owner-126',
          }),
        };
      }
      return {
        ok: true,
        json: async () => ({ items: [] }),
      };
    },
    WebSocketImpl: class extends FakeWebSocket {
      constructor(url) {
        super(url);
        sockets.push(url);
      }
    },
  });

  await autoTeamPage.initPage(root);

  assert.deepEqual(fetchCalls, [
    ['/api/team/teams?owner_account_id=126', 'GET'],
    ['/api/team/discovery/run', 'POST'],
  ]);
  assert.deepEqual(sockets, ['ws://localhost/api/ws/task/task-owner-126']);
});

test('accepted task flow 会在 discover 与 sync-batch 成功后刷新 teams 列表', async () => {
  const refreshCalls = [];
  const sockets = [];
  const flow = createAcceptedTaskFlow({
    refreshTeams: async (path) => {
      refreshCalls.push(path);
      return { items: [] };
    },
    createWebSocket: (path) => {
      const socket = new FakeWebSocket(path);
      sockets.push(socket);
      return socket;
    },
  });

  await flow.start({
    task_uuid: 'task-discovery',
    task_type: 'discover_owner_teams',
    ws_channel: '/api/ws/task/task-discovery',
  });
  await flow.start({
    task_uuid: 'task-sync-batch',
    task_type: 'sync_all_teams',
    ws_channel: '/api/ws/task/task-sync-batch',
  });

  assert.equal(sockets.length, 2);
  assert.deepEqual(refreshCalls, []);

  sockets[0].emit('message', { data: JSON.stringify({ status: 'running' }) });
  sockets[1].emit('message', { data: JSON.stringify({ status: 'completed' }) });
  await Promise.resolve();

  sockets[0].emit('message', { data: JSON.stringify({ status: 'completed' }) });
  await Promise.resolve();

  assert.deepEqual(refreshCalls, [
    '/api/team/teams',
    '/api/team/teams',
  ]);
  assert.equal(sockets[0].closed, true);
  assert.equal(sockets[1].closed, true);
});

test('membership actions require membership id and refresh detail after success', async () => {
  const action = buildMembershipActionRequest({ id: 7, action: 'remove' });
  assert.deepEqual(action, { membershipId: 7, action: 'remove' });

  assert.throws(
    () => buildMembershipActionRequest({ id: 0, action: 'remove' }),
    /membership/i,
  );

  const calls = [];
  const refreshResult = await afterSuccessfulMembershipAction(12, 'invited', {
    refreshTeamDetail: async (path) => {
      calls.push(['detail', path]);
    },
    refreshMemberships: async (path) => {
      calls.push(['memberships', path]);
    },
    refreshTasks: async (path) => {
      calls.push(['tasks', path]);
    },
  });

  assert.deepEqual(refreshResult, {
    detailPath: '/api/team/teams/12',
    membershipsPath: '/api/team/teams/12/memberships?status=invited',
    tasksPath: '/api/team/tasks?team_id=12',
  });
  assert.deepEqual(calls, [
    ['detail', '/api/team/teams/12'],
    ['memberships', '/api/team/teams/12/memberships?status=invited'],
    ['tasks', '/api/team/tasks?team_id=12'],
  ]);
});

test('batch invite modal renders full-team conflict state', () => {
  assert.deepEqual(
    resolveInviteAvailability({ status: 'active', syncStatus: 'success' }),
    {
      disabled: false,
      tone: 'ready',
      reason: '',
    },
  );

  assert.deepEqual(
    resolveInviteAvailability({ status: 'full', syncStatus: 'success' }),
    {
      disabled: true,
      tone: 'warning',
      reason: '当前 Team 已满，无法继续批量邀请。',
    },
  );

  assert.deepEqual(
    resolveInviteAvailability({ status: 'active', syncStatus: 'failed' }),
    {
      disabled: true,
      tone: 'danger',
      reason: '同步状态异常，请先完成一次成功同步再继续邀请。',
    },
  );
});

test('auto_team 模板包含 overview/detail/task center 和主操作按钮', () => {
  const template = readFileSync(
    new URL('../../templates/auto_team.html', import.meta.url),
    'utf8',
  );

  assert.match(template, /data-team-shell/);
  assert.match(template, /data-panel="overview"/);
  assert.match(template, /data-panel="detail"/);
  assert.match(template, /data-panel="task-center"/);
  assert.match(template, />发现母号</);
  assert.match(template, />批量同步</);
  assert.match(template, /data-role="membership-list"/);
  assert.match(template, /data-role="invite-modal"/);
  assert.match(template, />批量邀请</);
  assert.match(template, /\/static\/js\/auto_team\.js\?v=\{\{ static_version \}\}/);
});
