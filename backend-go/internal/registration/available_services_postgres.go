package registration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AvailableServicesPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewAvailableServicesPostgresRepository(pool *pgxpool.Pool) *AvailableServicesPostgresRepository {
	return &AvailableServicesPostgresRepository{pool: pool}
}

func (r *AvailableServicesPostgresRepository) GetSettings(ctx context.Context, keys []string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT key, value
		FROM settings
		WHERE key = ANY($1)
	`, keys)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string, len(keys))
	for rows.Next() {
		var key string
		var value pgtype.Text
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan settings: %w", err)
		}
		if value.Valid {
			settings[key] = value.String
			continue
		}
		settings[key] = ""
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate settings: %w", err)
	}

	return settings, nil
}

func (r *AvailableServicesPostgresRepository) ListEmailServices(ctx context.Context) ([]EmailServiceRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, service_type, name, config, priority
		FROM email_services
		WHERE enabled = TRUE
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query email services: %w", err)
	}
	defer rows.Close()

	services := make([]EmailServiceRecord, 0)
	for rows.Next() {
		var (
			id          int32
			serviceType string
			name        string
			configRaw   string
			priority    int32
		)
		if err := rows.Scan(&id, &serviceType, &name, &configRaw, &priority); err != nil {
			return nil, fmt.Errorf("scan email service: %w", err)
		}

		config := make(map[string]any)
		if configRaw != "" {
			if err := json.Unmarshal([]byte(configRaw), &config); err != nil {
				return nil, fmt.Errorf("decode email service config for %s(%d): %w", serviceType, id, err)
			}
		}

		services = append(services, EmailServiceRecord{
			ID:          int(id),
			ServiceType: serviceType,
			Name:        name,
			Config:      config,
			Priority:    int(priority),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email services: %w", err)
	}

	return services, nil
}
