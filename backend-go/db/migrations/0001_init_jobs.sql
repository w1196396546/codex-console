-- +goose Up

CREATE SEQUENCE IF NOT EXISTS job_logs_seq;

CREATE TABLE IF NOT EXISTS jobs (
    job_id UUID PRIMARY KEY,
    job_type TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    status TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS job_runs (
    job_run_id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL,
    attempt INT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS job_logs (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    job_run_id UUID,
    seq BIGINT NOT NULL DEFAULT nextval('job_logs_seq'),
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Support upgrading legacy jobs tables that predate the runtime metadata columns.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS result JSONB;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS error TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;

ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '';
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 1;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;

ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS job_run_id UUID;
ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS seq BIGINT NOT NULL DEFAULT nextval('job_logs_seq');
ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS job_runs_job_id_idx ON job_runs (job_id);
CREATE INDEX IF NOT EXISTS job_logs_job_id_seq_idx ON job_logs (job_id, seq);
