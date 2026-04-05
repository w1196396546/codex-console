package settings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetSettings(ctx context.Context, keys []string) (map[string]SettingRecord, error) {
	if r == nil || r.pool == nil {
		return map[string]SettingRecord{}, nil
	}

	rows, err := r.pool.Query(ctx, `
SELECT key, value, COALESCE(description, ''), category, updated_at
FROM settings
WHERE key = ANY($1)
`, keys)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]SettingRecord, len(keys))
	for rows.Next() {
		var (
			record    SettingRecord
			value     pgtype.Text
			category  pgtype.Text
			updatedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&record.Key, &value, &record.Description, &category, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan settings row: %w", err)
		}
		record.Value = textValue(value)
		record.Category = textOrFallback(category, "general")
		record.UpdatedAt = timeValue(updatedAt)
		result[record.Key] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate settings rows: %w", err)
	}

	return result, nil
}

func (r *PostgresRepository) UpsertSettings(ctx context.Context, settings []SettingRecord) error {
	if r == nil || r.pool == nil || len(settings) == 0 {
		return nil
	}

	for _, record := range settings {
		if _, err := r.pool.Exec(ctx, `
INSERT INTO settings (key, value, description, category, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    updated_at = EXCLUDED.updated_at
`, record.Key, record.Value, record.Description, record.Category, record.UpdatedAt); err != nil {
			return fmt.Errorf("upsert setting %s: %w", record.Key, err)
		}
	}

	return nil
}

func (r *PostgresRepository) ListProxies(ctx context.Context, enabled *bool) ([]ProxyRecord, error) {
	if r == nil || r.pool == nil {
		return []ProxyRecord{}, nil
	}

	query := `
SELECT id, name, type, host, port, username, password, enabled, is_default, priority, last_used, created_at, updated_at, proxy_url
FROM proxies
`
	args := make([]any, 0, 1)
	if enabled != nil {
		query += " WHERE enabled = $1"
		args = append(args, *enabled)
	}
	query += " ORDER BY created_at DESC, id DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query proxies: %w", err)
	}
	defer rows.Close()

	proxies := make([]ProxyRecord, 0)
	for rows.Next() {
		record, err := scanProxyRecord(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proxies: %w", err)
	}

	return proxies, nil
}

func (r *PostgresRepository) CreateProxy(ctx context.Context, req CreateProxyRequest) (ProxyRecord, error) {
	if r == nil || r.pool == nil {
		return ProxyRecord{}, ErrRepositoryNotConfigured
	}

	row := r.pool.QueryRow(ctx, `
INSERT INTO proxies (name, type, host, port, username, password, enabled, is_default, priority)
VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8)
RETURNING id, name, type, host, port, username, password, enabled, is_default, priority, last_used, created_at, updated_at, proxy_url
`, req.Name, req.Type, req.Host, req.Port, req.Username, req.Password, req.Enabled, req.Priority)

	return scanProxyRow(row)
}

func (r *PostgresRepository) GetProxyByID(ctx context.Context, proxyID int) (ProxyRecord, bool, error) {
	if r == nil || r.pool == nil {
		return ProxyRecord{}, false, nil
	}

	row := r.pool.QueryRow(ctx, `
SELECT id, name, type, host, port, username, password, enabled, is_default, priority, last_used, created_at, updated_at, proxy_url
FROM proxies
WHERE id = $1
`, proxyID)

	record, err := scanProxyRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ProxyRecord{}, false, nil
		}
		return ProxyRecord{}, false, fmt.Errorf("query proxy %d: %w", proxyID, err)
	}
	return record, true, nil
}

func (r *PostgresRepository) UpdateProxy(ctx context.Context, proxyID int, req UpdateProxyRequest) (ProxyRecord, bool, error) {
	current, found, err := r.GetProxyByID(ctx, proxyID)
	if err != nil || !found {
		return ProxyRecord{}, found, err
	}

	name := current.Name
	if req.Name != nil {
		name = *req.Name
	}
	proxyType := current.Type
	if req.Type != nil {
		proxyType = *req.Type
	}
	host := current.Host
	if req.Host != nil {
		host = *req.Host
	}
	port := current.Port
	if req.Port != nil {
		port = *req.Port
	}
	username := current.Username
	if req.Username != nil {
		username = req.Username
	}
	password := current.Password
	if req.Password != nil {
		password = req.Password
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := current.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}
	isDefault := current.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	row := r.pool.QueryRow(ctx, `
UPDATE proxies
SET name = $2,
    type = $3,
    host = $4,
    port = $5,
    username = $6,
    password = $7,
    enabled = $8,
    is_default = $9,
    priority = $10,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, type, host, port, username, password, enabled, is_default, priority, last_used, created_at, updated_at, proxy_url
`, proxyID, name, proxyType, host, port, username, password, enabled, isDefault, priority)

	record, err := scanProxyRow(row)
	if err != nil {
		return ProxyRecord{}, false, fmt.Errorf("update proxy %d: %w", proxyID, err)
	}
	return record, true, nil
}

func (r *PostgresRepository) DeleteProxy(ctx context.Context, proxyID int) (bool, error) {
	if r == nil || r.pool == nil {
		return false, nil
	}

	result, err := r.pool.Exec(ctx, `DELETE FROM proxies WHERE id = $1`, proxyID)
	if err != nil {
		return false, fmt.Errorf("delete proxy %d: %w", proxyID, err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *PostgresRepository) SetProxyDefault(ctx context.Context, proxyID int) (ProxyRecord, bool, error) {
	if r == nil || r.pool == nil {
		return ProxyRecord{}, false, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ProxyRecord{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `UPDATE proxies SET is_default = FALSE WHERE is_default = TRUE`); err != nil {
		return ProxyRecord{}, false, fmt.Errorf("clear proxy defaults: %w", err)
	}

	row := tx.QueryRow(ctx, `
UPDATE proxies
SET is_default = TRUE, updated_at = NOW()
WHERE id = $1
RETURNING id, name, type, host, port, username, password, enabled, is_default, priority, last_used, created_at, updated_at, proxy_url
`, proxyID)

	record, err := scanProxyRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ProxyRecord{}, false, nil
		}
		return ProxyRecord{}, false, fmt.Errorf("set proxy default %d: %w", proxyID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ProxyRecord{}, false, fmt.Errorf("commit proxy default tx: %w", err)
	}

	return record, true, nil
}

type proxyScanner interface {
	Scan(dest ...any) error
}

func scanProxyRecord(rows pgx.Rows) (ProxyRecord, error) {
	record, err := scanProxyRow(rows)
	if err != nil {
		return ProxyRecord{}, fmt.Errorf("scan proxy row: %w", err)
	}
	return record, nil
}

func scanProxyRow(row proxyScanner) (ProxyRecord, error) {
	var (
		record    ProxyRecord
		username  pgtype.Text
		password  pgtype.Text
		lastUsed  pgtype.Timestamptz
		createdAt pgtype.Timestamptz
		updatedAt pgtype.Timestamptz
		proxyURL  pgtype.Text
	)
	if err := row.Scan(
		&record.ID,
		&record.Name,
		&record.Type,
		&record.Host,
		&record.Port,
		&username,
		&password,
		&record.Enabled,
		&record.IsDefault,
		&record.Priority,
		&lastUsed,
		&createdAt,
		&updatedAt,
		&proxyURL,
	); err != nil {
		return ProxyRecord{}, err
	}
	record.Username = textPointer(username)
	record.Password = textPointer(password)
	record.LastUsed = timestamptzPointer(lastUsed)
	record.CreatedAt = timestamptzPointer(createdAt)
	record.UpdatedAt = timestamptzPointer(updatedAt)
	record.ProxyURL = strings.TrimSpace(textValue(proxyURL))
	return record, nil
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := strings.TrimSpace(value.String)
	if text == "" {
		return nil
	}
	return &text
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func textOrFallback(value pgtype.Text, fallback string) string {
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return fallback
	}
	return text
}

func timestamptzPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	ts := value.Time.UTC()
	return &ts
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
