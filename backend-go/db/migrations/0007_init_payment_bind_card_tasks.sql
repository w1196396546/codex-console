-- +goose Up
CREATE TABLE IF NOT EXISTS bind_card_tasks (
    id SERIAL PRIMARY KEY,
    account_id INT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    plan_type TEXT NOT NULL,
    workspace_name TEXT,
    price_interval TEXT,
    seat_quantity INT,
    country TEXT NOT NULL DEFAULT 'US',
    currency TEXT NOT NULL DEFAULT 'USD',
    checkout_url TEXT NOT NULL,
    checkout_session_id TEXT,
    publishable_key TEXT,
    client_secret TEXT,
    checkout_source TEXT,
    bind_mode TEXT NOT NULL DEFAULT 'semi_auto',
    status TEXT NOT NULL DEFAULT 'link_ready',
    last_error TEXT,
    opened_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS bind_card_tasks_account_id_idx ON bind_card_tasks (account_id);
CREATE INDEX IF NOT EXISTS bind_card_tasks_status_created_at_idx ON bind_card_tasks (status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS bind_card_tasks_account_status_idx ON bind_card_tasks (account_id, status, id DESC);

-- +goose Down
DROP TABLE IF EXISTS bind_card_tasks;
