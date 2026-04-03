import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import accountsPage from '../../static/js/accounts.js';

const { buildTeamManagementEntryUrl } = accountsPage;

test('buildTeamManagementEntryUrl 为账号页生成 Team 管理入口链接', () => {
  assert.equal(buildTeamManagementEntryUrl(42), '/auto-team?owner_account_id=42');
  assert.equal(buildTeamManagementEntryUrl(), '/auto-team');
});

test('accounts 模板提供进入 Team 管理入口', () => {
  const template = readFileSync(
    new URL('../../templates/accounts.html', import.meta.url),
    'utf8',
  );

  assert.match(template, /id="team-management-entry"/);
  assert.match(template, />进入 Team 管理</);
  assert.match(template, /href="\/auto-team"/);
});
