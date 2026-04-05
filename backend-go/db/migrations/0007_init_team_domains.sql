-- +goose Up
CREATE TABLE IF NOT EXISTS teams (
    id SERIAL PRIMARY KEY,
    owner_account_id INT NOT NULL REFERENCES accounts(id),
    upstream_team_id TEXT,
    upstream_account_id TEXT NOT NULL,
    team_name TEXT NOT NULL,
    plan_type TEXT NOT NULL,
    subscription_plan TEXT,
    account_role_snapshot TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    current_members INT NOT NULL DEFAULT 0,
    max_members INT,
    seats_available INT,
    expires_at TIMESTAMPTZ,
    last_sync_at TIMESTAMPTZ,
    sync_status TEXT NOT NULL DEFAULT 'pending',
    sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS teams_owner_upstream_account_uidx ON teams (owner_account_id, upstream_account_id);
CREATE INDEX IF NOT EXISTS teams_owner_account_id_idx ON teams (owner_account_id);
CREATE INDEX IF NOT EXISTS teams_upstream_team_id_idx ON teams (upstream_team_id);
CREATE INDEX IF NOT EXISTS teams_upstream_account_id_idx ON teams (upstream_account_id);
CREATE INDEX IF NOT EXISTS teams_updated_at_idx ON teams (updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS team_memberships (
    id SERIAL PRIMARY KEY,
    team_id INT NOT NULL REFERENCES teams(id),
    local_account_id INT REFERENCES accounts(id),
    member_email TEXT NOT NULL,
    upstream_user_id TEXT,
    member_role TEXT NOT NULL DEFAULT 'member',
    membership_status TEXT NOT NULL DEFAULT 'pending',
    invited_at TIMESTAMPTZ,
    joined_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    source TEXT NOT NULL DEFAULT 'sync',
    sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS team_memberships_team_email_uidx ON team_memberships (team_id, member_email);
CREATE INDEX IF NOT EXISTS team_memberships_team_id_idx ON team_memberships (team_id);
CREATE INDEX IF NOT EXISTS team_memberships_local_account_id_idx ON team_memberships (local_account_id);
CREATE INDEX IF NOT EXISTS team_memberships_upstream_user_id_idx ON team_memberships (upstream_user_id);
CREATE INDEX IF NOT EXISTS team_memberships_status_idx ON team_memberships (membership_status, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS team_tasks (
    id SERIAL PRIMARY KEY,
    team_id INT REFERENCES teams(id),
    owner_account_id INT REFERENCES accounts(id),
    task_uuid TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    active_scope_key TEXT UNIQUE,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_payload JSONB,
    error_message TEXT,
    logs TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS team_tasks_task_uuid_uidx ON team_tasks (task_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS team_tasks_active_scope_key_uidx ON team_tasks (active_scope_key) WHERE active_scope_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS team_tasks_team_id_idx ON team_tasks (team_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS team_tasks_owner_account_id_idx ON team_tasks (owner_account_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS team_tasks_status_idx ON team_tasks (status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS team_task_items (
    id SERIAL PRIMARY KEY,
    task_id INT NOT NULL REFERENCES team_tasks(id),
    target_email TEXT NOT NULL,
    item_status TEXT NOT NULL DEFAULT 'pending',
    before JSONB,
    after JSONB,
    message TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS team_task_items_task_target_uidx ON team_task_items (task_id, target_email);
CREATE INDEX IF NOT EXISTS team_task_items_task_id_idx ON team_task_items (task_id, id ASC);

-- +goose Down
DROP TABLE IF EXISTS team_task_items;
DROP TABLE IF EXISTS team_tasks;
DROP TABLE IF EXISTS team_memberships;
DROP TABLE IF EXISTS teams;
