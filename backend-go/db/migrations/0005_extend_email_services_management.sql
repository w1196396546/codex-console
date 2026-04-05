-- +goose Up

ALTER TABLE email_services ADD COLUMN IF NOT EXISTS last_used TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS email_services_management_sort_idx ON email_services (service_type, priority, id, last_used DESC);
