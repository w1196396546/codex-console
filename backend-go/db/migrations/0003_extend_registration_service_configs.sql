-- +goose Up

CREATE TABLE IF NOT EXISTS cpa_services (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    api_url TEXT NOT NULL,
    api_token TEXT NOT NULL,
    proxy_url TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sub2api_services (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    api_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT 'sub2api',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tm_services (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    api_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Support upgrading legacy accounts tables that may predate the full Go-native schema.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS password TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS client_id TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS email_service TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS email_service_id TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS account_id TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS workspace_id TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS access_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS refresh_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS id_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cookies TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_used TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cpa_uploaded BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cpa_uploaded_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS sub2api_uploaded BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS sub2api_uploaded_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_refresh TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS extra_data JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'register';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS subscription_type TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS subscription_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Keep legacy custom_domain rows visible to Go native flows that only read moe_mail.
UPDATE email_services SET service_type = 'moe_mail' WHERE service_type = 'custom_domain';
UPDATE accounts SET email_service = 'moe_mail' WHERE email_service = 'custom_domain';

CREATE INDEX IF NOT EXISTS accounts_activity_sort_idx ON accounts ((COALESCE(registered_at, created_at, updated_at)) DESC, id DESC);

ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS api_url TEXT;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS api_token TEXT;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS proxy_url TEXT;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS cpa_services_enabled_priority_idx ON cpa_services (enabled, priority, id);

ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS api_url TEXT;
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS api_key TEXT;
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS target_type TEXT NOT NULL DEFAULT 'sub2api';
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS sub2api_services_enabled_priority_idx ON sub2api_services (enabled, priority, id);

ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS api_url TEXT;
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS api_key TEXT;
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS tm_services_enabled_priority_idx ON tm_services (enabled, priority, id);
