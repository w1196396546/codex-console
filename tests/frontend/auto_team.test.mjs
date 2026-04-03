import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import autoTeam from '../../static/js/auto_team.js';

const {
  createAcceptedTaskFlow,
  deriveInitialTeamState,
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
  assert.match(template, /\/static\/js\/auto_team\.js\?v=\{\{ static_version \}\}/);
});
