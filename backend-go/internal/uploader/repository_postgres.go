package uploader

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type PostgresConfigRepository struct {
	db postgresQuerier
}

func NewPostgresConfigRepository(pool *pgxpool.Pool) *PostgresConfigRepository {
	return newPostgresConfigRepository(pool)
}

func newPostgresConfigRepository(db postgresQuerier) *PostgresConfigRepository {
	return &PostgresConfigRepository{db: db}
}

func (r *PostgresConfigRepository) ListCPAServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error) {
	query, args := buildServiceListQuery(`
		SELECT id, name, api_url, api_token, COALESCE(proxy_url, ''), enabled, priority
		FROM cpa_services
	`, ids)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cpa services: %w", err)
	}
	defer rows.Close()

	configs := make([]ServiceConfig, 0)
	for rows.Next() {
		var (
			id         int32
			name       string
			baseURL    string
			credential string
			proxyURL   string
			enabled    bool
			priority   int32
		)
		if err := rows.Scan(&id, &name, &baseURL, &credential, &proxyURL, &enabled, &priority); err != nil {
			return nil, fmt.Errorf("scan cpa service: %w", err)
		}
		configs = append(configs, ServiceConfig{
			ID:         int(id),
			Kind:       UploadKindCPA,
			Name:       name,
			BaseURL:    baseURL,
			Credential: credential,
			ProxyURL:   proxyURL,
			Enabled:    enabled,
			Priority:   int(priority),
		}.Normalized())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cpa services: %w", err)
	}

	return configs, nil
}

func (r *PostgresConfigRepository) ListSub2APIServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error) {
	query, args := buildServiceListQuery(`
		SELECT id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority
		FROM sub2api_services
	`, ids)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sub2api services: %w", err)
	}
	defer rows.Close()

	configs := make([]ServiceConfig, 0)
	for rows.Next() {
		var (
			id         int32
			name       string
			baseURL    string
			credential string
			targetType string
			enabled    bool
			priority   int32
		)
		if err := rows.Scan(&id, &name, &baseURL, &credential, &targetType, &enabled, &priority); err != nil {
			return nil, fmt.Errorf("scan sub2api service: %w", err)
		}
		configs = append(configs, ServiceConfig{
			ID:         int(id),
			Kind:       UploadKindSub2API,
			Name:       name,
			BaseURL:    baseURL,
			Credential: credential,
			TargetType: targetType,
			Enabled:    enabled,
			Priority:   int(priority),
		}.Normalized())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sub2api services: %w", err)
	}

	return configs, nil
}

func (r *PostgresConfigRepository) ListTMServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error) {
	query, args := buildServiceListQuery(`
		SELECT id, name, api_url, api_key, enabled, priority
		FROM tm_services
	`, ids)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tm services: %w", err)
	}
	defer rows.Close()

	configs := make([]ServiceConfig, 0)
	for rows.Next() {
		var (
			id         int32
			name       string
			baseURL    string
			credential string
			enabled    bool
			priority   int32
		)
		if err := rows.Scan(&id, &name, &baseURL, &credential, &enabled, &priority); err != nil {
			return nil, fmt.Errorf("scan tm service: %w", err)
		}
		configs = append(configs, ServiceConfig{
			ID:         int(id),
			Kind:       UploadKindTM,
			Name:       name,
			BaseURL:    baseURL,
			Credential: credential,
			Enabled:    enabled,
			Priority:   int(priority),
		}.Normalized())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tm services: %w", err)
	}

	return configs, nil
}

func buildServiceListQuery(baseQuery string, ids []int) (string, []any) {
	query := baseQuery
	if len(ids) > 0 {
		return query + "\n\t\tWHERE id = ANY($1)\n\t\tORDER BY priority ASC, id ASC", []any{toInt32Slice(ids)}
	}
	return query + "\n\t\tWHERE enabled = TRUE\n\t\tORDER BY priority ASC, id ASC", nil
}

func toInt32Slice(ids []int) []int32 {
	values := make([]int32, 0, len(ids))
	for _, id := range ids {
		values = append(values, int32(id))
	}
	return values
}
