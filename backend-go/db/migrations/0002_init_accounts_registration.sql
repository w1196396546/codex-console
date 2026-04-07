-- +goose Up

CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    password TEXT,
    client_id TEXT,
    session_token TEXT,
    email_service TEXT,
    email_service_id TEXT,
    account_id TEXT,
    workspace_id TEXT,
    access_token TEXT,
    refresh_token TEXT,
    id_token TEXT,
    cookies TEXT,
    proxy_used TEXT,
    cpa_uploaded BOOLEAN NOT NULL DEFAULT FALSE,
    cpa_uploaded_at TIMESTAMPTZ,
    sub2api_uploaded BOOLEAN NOT NULL DEFAULT FALSE,
    sub2api_uploaded_at TIMESTAMPTZ,
    last_refresh TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    extra_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active',
    source TEXT NOT NULL DEFAULT 'register',
    subscription_type TEXT,
    subscription_at TIMESTAMPTZ,
    registered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_email_uidx ON accounts (email);

CREATE TABLE IF NOT EXISTS email_services (
    id SERIAL PRIMARY KEY,
    service_type TEXT NOT NULL,
    name TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Backfill legacy email service tables before creating indexes that depend on these columns.
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS service_type TEXT NOT NULL DEFAULT '';
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE email_services ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Repair legacy snapshots where columns exist but still allow NULLs or lack defaults.
UPDATE email_services SET service_type = '' WHERE service_type IS NULL;
ALTER TABLE email_services ALTER COLUMN service_type SET DEFAULT '';
ALTER TABLE email_services ALTER COLUMN service_type SET NOT NULL;

UPDATE email_services SET name = '' WHERE name IS NULL;
ALTER TABLE email_services ALTER COLUMN name SET DEFAULT '';
ALTER TABLE email_services ALTER COLUMN name SET NOT NULL;

UPDATE email_services SET config = '{}'::jsonb WHERE config IS NULL;
ALTER TABLE email_services ALTER COLUMN config SET DEFAULT '{}'::jsonb;
ALTER TABLE email_services ALTER COLUMN config SET NOT NULL;

UPDATE email_services SET enabled = TRUE WHERE enabled IS NULL;
ALTER TABLE email_services ALTER COLUMN enabled SET DEFAULT TRUE;
ALTER TABLE email_services ALTER COLUMN enabled SET NOT NULL;

UPDATE email_services SET priority = 0 WHERE priority IS NULL;
ALTER TABLE email_services ALTER COLUMN priority SET DEFAULT 0;
ALTER TABLE email_services ALTER COLUMN priority SET NOT NULL;

UPDATE email_services SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE email_services ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE email_services ALTER COLUMN created_at SET NOT NULL;

UPDATE email_services SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE email_services ALTER COLUMN updated_at SET DEFAULT NOW();
ALTER TABLE email_services ALTER COLUMN updated_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS email_services_enabled_priority_idx ON email_services (enabled, priority, id);
CREATE INDEX IF NOT EXISTS email_services_type_enabled_priority_idx ON email_services (service_type, enabled, priority, id);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT
);
