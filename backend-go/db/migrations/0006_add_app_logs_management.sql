-- +goose Up

CREATE TABLE IF NOT EXISTS app_logs (
    id BIGSERIAL PRIMARY KEY,
    level TEXT NOT NULL,
    logger TEXT NOT NULL,
    module TEXT,
    pathname TEXT,
    lineno INT,
    message TEXT NOT NULL,
    exception TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS level TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS logger TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS module TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS pathname TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS lineno INT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS message TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS exception TEXT;
ALTER TABLE app_logs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE app_logs SET level = 'INFO' WHERE level IS NULL OR btrim(level) = '';
UPDATE app_logs SET logger = 'app' WHERE logger IS NULL OR btrim(logger) = '';
UPDATE app_logs SET message = '' WHERE message IS NULL;

ALTER TABLE app_logs ALTER COLUMN level SET NOT NULL;
ALTER TABLE app_logs ALTER COLUMN logger SET NOT NULL;
ALTER TABLE app_logs ALTER COLUMN message SET NOT NULL;
ALTER TABLE app_logs ALTER COLUMN created_at SET DEFAULT NOW();
ALTER TABLE app_logs ALTER COLUMN created_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS app_logs_created_sort_idx ON app_logs (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS app_logs_level_created_sort_idx ON app_logs (level, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS app_logs_logger_created_sort_idx ON app_logs (logger, created_at DESC, id DESC);
