-- +goose Up

ALTER TABLE settings ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE settings SET description = '' WHERE description IS NULL;
ALTER TABLE settings ALTER COLUMN description SET DEFAULT '';

UPDATE settings SET category = 'general' WHERE category IS NULL OR BTRIM(category) = '';
ALTER TABLE settings ALTER COLUMN category SET DEFAULT 'general';
ALTER TABLE settings ALTER COLUMN category SET NOT NULL;

UPDATE settings SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE settings ALTER COLUMN updated_at SET DEFAULT NOW();
ALTER TABLE settings ALTER COLUMN updated_at SET NOT NULL;

CREATE TABLE IF NOT EXISTS proxies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'http',
    host TEXT NOT NULL,
    port INT NOT NULL,
    username TEXT,
    password TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    priority INT NOT NULL DEFAULT 0,
    last_used TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    proxy_url TEXT GENERATED ALWAYS AS (
        CASE
            WHEN COALESCE(BTRIM(type), '') = '' OR BTRIM(type) = 'http' THEN
                'http://'
            ELSE
                BTRIM(type) || '://'
        END ||
        CASE
            WHEN COALESCE(BTRIM(username), '') <> '' AND COALESCE(BTRIM(password), '') <> '' THEN
                BTRIM(username) || ':' || password || '@'
            ELSE
                ''
        END ||
        host || ':' || port::text
    ) STORED
);

ALTER TABLE proxies ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'http';
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS host TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS port INT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS username TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS password TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS last_used TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'proxies'
          AND column_name = 'proxy_url'
    ) THEN
        ALTER TABLE proxies
            ADD COLUMN proxy_url TEXT GENERATED ALWAYS AS (
                CASE
                    WHEN COALESCE(BTRIM(type), '') = '' OR BTRIM(type) = 'http' THEN
                        'http://'
                    ELSE
                        BTRIM(type) || '://'
                END ||
                CASE
                    WHEN COALESCE(BTRIM(username), '') <> '' AND COALESCE(BTRIM(password), '') <> '' THEN
                        BTRIM(username) || ':' || password || '@'
                    ELSE
                        ''
                END ||
                host || ':' || port::text
            ) STORED;
    END IF;
END
$$;

UPDATE proxies SET name = '' WHERE name IS NULL;
ALTER TABLE proxies ALTER COLUMN name SET NOT NULL;

UPDATE proxies SET host = '' WHERE host IS NULL;
ALTER TABLE proxies ALTER COLUMN host SET NOT NULL;

UPDATE proxies SET port = 0 WHERE port IS NULL;
ALTER TABLE proxies ALTER COLUMN port SET NOT NULL;

UPDATE proxies SET type = 'http' WHERE type IS NULL OR BTRIM(type) = '';
ALTER TABLE proxies ALTER COLUMN type SET DEFAULT 'http';
ALTER TABLE proxies ALTER COLUMN type SET NOT NULL;

UPDATE proxies SET enabled = TRUE WHERE enabled IS NULL;
ALTER TABLE proxies ALTER COLUMN enabled SET DEFAULT TRUE;
ALTER TABLE proxies ALTER COLUMN enabled SET NOT NULL;

UPDATE proxies SET is_default = FALSE WHERE is_default IS NULL;
ALTER TABLE proxies ALTER COLUMN is_default SET DEFAULT FALSE;
ALTER TABLE proxies ALTER COLUMN is_default SET NOT NULL;

UPDATE proxies SET priority = 0 WHERE priority IS NULL;
ALTER TABLE proxies ALTER COLUMN priority SET DEFAULT 0;
ALTER TABLE proxies ALTER COLUMN priority SET NOT NULL;

UPDATE proxies SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE proxies ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE proxies ALTER COLUMN created_at SET NOT NULL;

UPDATE proxies SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE proxies ALTER COLUMN updated_at SET DEFAULT NOW();
ALTER TABLE proxies ALTER COLUMN updated_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS proxies_enabled_default_last_used_idx ON proxies (enabled, is_default, last_used, id);
