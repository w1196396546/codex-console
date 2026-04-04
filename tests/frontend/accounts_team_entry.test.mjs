import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import accountsPage from '../../static/js/accounts.js';

const {
  buildTeamManagementEntryUrl,
  resolveTeamManagementEntryHref,
} = accountsPage;

test('buildTeamManagementEntryUrl 为账号页生成 Team 管理入口链接', () => {
  assert.equal(buildTeamManagementEntryUrl(42), '/auto-team?owner_account_id=42');
  assert.equal(buildTeamManagementEntryUrl(), '/auto-team');
});

test('resolveTeamManagementEntryHref 仅对 Team 母号生成带 owner 参数的入口', () => {
  assert.equal(
    resolveTeamManagementEntryHref({
      id: 126,
      team_relation_summary: { has_owner_role: true, has_member_role: false },
    }),
    '/auto-team?owner_account_id=126',
  );

  assert.equal(
    resolveTeamManagementEntryHref({
      id: 127,
      subscription_type: 'team',
      team_relation_summary: null,
    }),
    '/auto-team?owner_account_id=127',
  );

  assert.equal(
    resolveTeamManagementEntryHref({
      id: 128,
      team_relation_summary: { has_owner_role: false, has_member_role: true },
    }),
    '/auto-team',
  );

  assert.equal(
    resolveTeamManagementEntryHref({
      id: 129,
      subscription_type: 'free',
      team_relation_summary: null,
    }),
    '/auto-team',
  );
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
