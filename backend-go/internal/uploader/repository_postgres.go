package uploader

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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

func (r *PostgresConfigRepository) ListServiceConfigs(ctx context.Context, kind UploadKind, filter ServiceConfigListFilter) ([]ManagedServiceConfig, error) {
	query, args, err := buildAdminServiceListQuery(kind, filter)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s services: %w", kind, err)
	}
	defer rows.Close()

	configs := make([]ManagedServiceConfig, 0)
	for rows.Next() {
		config, err := scanManagedServiceConfig(kind, rows)
		if err != nil {
			return nil, fmt.Errorf("scan %s service: %w", kind, err)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s services: %w", kind, err)
	}

	return configs, nil
}

func (r *PostgresConfigRepository) GetServiceConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error) {
	query, err := buildServiceSelectByIDQuery(kind)
	if err != nil {
		return ManagedServiceConfig{}, false, err
	}

	config, err := scanManagedServiceConfig(kind, r.db.QueryRow(ctx, query, int32(id)))
	if err != nil {
		if err == pgx.ErrNoRows {
			return ManagedServiceConfig{}, false, nil
		}
		return ManagedServiceConfig{}, false, fmt.Errorf("get %s service: %w", kind, err)
	}
	return config, true, nil
}

func (r *PostgresConfigRepository) CreateServiceConfig(ctx context.Context, config ManagedServiceConfig) (ManagedServiceConfig, error) {
	normalized := config.Normalized()
	query, args, err := buildServiceInsertQuery(normalized)
	if err != nil {
		return ManagedServiceConfig{}, err
	}

	created, err := scanManagedServiceConfig(normalized.Kind, r.db.QueryRow(ctx, query, args...))
	if err != nil {
		return ManagedServiceConfig{}, fmt.Errorf("create %s service: %w", normalized.Kind, err)
	}
	return created, nil
}

func (r *PostgresConfigRepository) UpdateServiceConfig(ctx context.Context, kind UploadKind, id int, patch ManagedServiceConfigPatch) (ManagedServiceConfig, bool, error) {
	query, args, err := buildServiceUpdateQuery(kind, id, patch)
	if err != nil {
		return ManagedServiceConfig{}, false, err
	}

	updated, err := scanManagedServiceConfig(kind, r.db.QueryRow(ctx, query, args...))
	if err != nil {
		if err == pgx.ErrNoRows {
			return ManagedServiceConfig{}, false, nil
		}
		return ManagedServiceConfig{}, false, fmt.Errorf("update %s service: %w", kind, err)
	}
	return updated, true, nil
}

func (r *PostgresConfigRepository) DeleteServiceConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error) {
	query, err := buildServiceDeleteQuery(kind)
	if err != nil {
		return ManagedServiceConfig{}, false, err
	}

	deleted, err := scanManagedServiceConfig(kind, r.db.QueryRow(ctx, query, int32(id)))
	if err != nil {
		if err == pgx.ErrNoRows {
			return ManagedServiceConfig{}, false, nil
		}
		return ManagedServiceConfig{}, false, fmt.Errorf("delete %s service: %w", kind, err)
	}
	return deleted, true, nil
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

func buildAdminServiceListQuery(kind UploadKind, filter ServiceConfigListFilter) (string, []any, error) {
	selectClause, err := serviceSelectClause(kind)
	if err != nil {
		return "", nil, err
	}

	query := selectClause
	if filter.Enabled != nil {
		return query + "\n\t\tWHERE enabled = $1\n\t\tORDER BY priority ASC, id ASC", []any{*filter.Enabled}, nil
	}
	return query + "\n\t\tORDER BY priority ASC, id ASC", nil, nil
}

func buildServiceSelectByIDQuery(kind UploadKind) (string, error) {
	selectClause, err := serviceSelectClause(kind)
	if err != nil {
		return "", err
	}
	return selectClause + "\n\t\tWHERE id = $1", nil
}

func buildServiceInsertQuery(config ManagedServiceConfig) (string, []any, error) {
	switch config.Kind {
	case UploadKindCPA:
		return `
		INSERT INTO cpa_services (
			name,
			api_url,
			api_token,
			proxy_url,
			enabled,
			priority
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, api_url, api_token, COALESCE(proxy_url, ''), enabled, priority, created_at, updated_at
	`, []any{config.Name, config.BaseURL, config.Credential, config.ProxyURL, config.Enabled, config.Priority}, nil
	case UploadKindSub2API:
		return `
		INSERT INTO sub2api_services (
			name,
			api_url,
			api_key,
			target_type,
			enabled,
			priority
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority, created_at, updated_at
	`, []any{config.Name, config.BaseURL, config.Credential, config.TargetType, config.Enabled, config.Priority}, nil
	case UploadKindTM:
		return `
		INSERT INTO tm_services (
			name,
			api_url,
			api_key,
			enabled,
			priority
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, api_url, api_key, enabled, priority, created_at, updated_at
	`, []any{config.Name, config.BaseURL, config.Credential, config.Enabled, config.Priority}, nil
	default:
		return "", nil, ErrUploadKindInvalid
	}
}

func buildServiceUpdateQuery(kind UploadKind, id int, patch ManagedServiceConfigPatch) (string, []any, error) {
	switch kind {
	case UploadKindCPA:
		return `
		UPDATE cpa_services
		SET
			name = COALESCE($2, name),
			api_url = COALESCE($3, api_url),
			api_token = COALESCE($4, api_token),
			proxy_url = COALESCE($5, proxy_url),
			enabled = COALESCE($6, enabled),
			priority = COALESCE($7, priority),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, api_url, api_token, COALESCE(proxy_url, ''), enabled, priority, created_at, updated_at
	`, []any{int32(id), patch.Name, patch.BaseURL, patch.Credential, patch.ProxyURL, patch.Enabled, patch.Priority}, nil
	case UploadKindSub2API:
		return `
		UPDATE sub2api_services
		SET
			name = COALESCE($2, name),
			api_url = COALESCE($3, api_url),
			api_key = COALESCE($4, api_key),
			target_type = COALESCE($5, target_type),
			enabled = COALESCE($6, enabled),
			priority = COALESCE($7, priority),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority, created_at, updated_at
	`, []any{int32(id), patch.Name, patch.BaseURL, patch.Credential, patch.TargetType, patch.Enabled, patch.Priority}, nil
	case UploadKindTM:
		return `
		UPDATE tm_services
		SET
			name = COALESCE($2, name),
			api_url = COALESCE($3, api_url),
			api_key = COALESCE($4, api_key),
			enabled = COALESCE($5, enabled),
			priority = COALESCE($6, priority),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, api_url, api_key, enabled, priority, created_at, updated_at
	`, []any{int32(id), patch.Name, patch.BaseURL, patch.Credential, patch.Enabled, patch.Priority}, nil
	default:
		return "", nil, ErrUploadKindInvalid
	}
}

func buildServiceDeleteQuery(kind UploadKind) (string, error) {
	switch kind {
	case UploadKindCPA:
		return `
		DELETE FROM cpa_services
		WHERE id = $1
		RETURNING id, name, api_url, api_token, COALESCE(proxy_url, ''), enabled, priority, created_at, updated_at
	`, nil
	case UploadKindSub2API:
		return `
		DELETE FROM sub2api_services
		WHERE id = $1
		RETURNING id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority, created_at, updated_at
	`, nil
	case UploadKindTM:
		return `
		DELETE FROM tm_services
		WHERE id = $1
		RETURNING id, name, api_url, api_key, enabled, priority, created_at, updated_at
	`, nil
	default:
		return "", ErrUploadKindInvalid
	}
}

func serviceSelectClause(kind UploadKind) (string, error) {
	switch kind {
	case UploadKindCPA:
		return `
		SELECT id, name, api_url, api_token, COALESCE(proxy_url, ''), enabled, priority, created_at, updated_at
		FROM cpa_services
	`, nil
	case UploadKindSub2API:
		return `
		SELECT id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority, created_at, updated_at
		FROM sub2api_services
	`, nil
	case UploadKindTM:
		return `
		SELECT id, name, api_url, api_key, enabled, priority, created_at, updated_at
		FROM tm_services
	`, nil
	default:
		return "", ErrUploadKindInvalid
	}
}

type serviceScanner interface {
	Scan(dest ...any) error
}

func scanManagedServiceConfig(kind UploadKind, scanner serviceScanner) (ManagedServiceConfig, error) {
	var (
		id         int32
		name       string
		baseURL    string
		credential string
		enabled    bool
		priority   int32
		createdAt  *time.Time
		updatedAt  *time.Time
	)

	switch kind {
	case UploadKindCPA:
		var proxyURL string
		if err := scanner.Scan(&id, &name, &baseURL, &credential, &proxyURL, &enabled, &priority, &createdAt, &updatedAt); err != nil {
			return ManagedServiceConfig{}, err
		}
		return ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         int(id),
				Kind:       kind,
				Name:       name,
				BaseURL:    baseURL,
				Credential: credential,
				ProxyURL:   proxyURL,
				Enabled:    enabled,
				Priority:   int(priority),
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}.Normalized(), nil
	case UploadKindSub2API:
		var targetType string
		if err := scanner.Scan(&id, &name, &baseURL, &credential, &targetType, &enabled, &priority, &createdAt, &updatedAt); err != nil {
			return ManagedServiceConfig{}, err
		}
		return ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         int(id),
				Kind:       kind,
				Name:       name,
				BaseURL:    baseURL,
				Credential: credential,
				TargetType: targetType,
				Enabled:    enabled,
				Priority:   int(priority),
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}.Normalized(), nil
	case UploadKindTM:
		if err := scanner.Scan(&id, &name, &baseURL, &credential, &enabled, &priority, &createdAt, &updatedAt); err != nil {
			return ManagedServiceConfig{}, err
		}
		return ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         int(id),
				Kind:       kind,
				Name:       name,
				BaseURL:    baseURL,
				Credential: credential,
				Enabled:    enabled,
				Priority:   int(priority),
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}.Normalized(), nil
	default:
		return ManagedServiceConfig{}, ErrUploadKindInvalid
	}
}
