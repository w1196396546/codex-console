package registration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutlookPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewOutlookPostgresRepository(pool *pgxpool.Pool) *OutlookPostgresRepository {
	return &OutlookPostgresRepository{pool: pool}
}

func (r *OutlookPostgresRepository) ListOutlookServices(ctx context.Context) ([]EmailServiceRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, service_type, name, config::text, priority
		FROM email_services
		WHERE service_type = 'outlook' AND enabled = TRUE
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query outlook email services: %w", err)
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
			return nil, fmt.Errorf("scan outlook email service: %w", err)
		}

		config := make(map[string]any)
		if configRaw != "" {
			if err := json.Unmarshal([]byte(configRaw), &config); err != nil {
				return nil, fmt.Errorf("decode outlook email service config for %s(%d): %w", serviceType, id, err)
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
		return nil, fmt.Errorf("iterate outlook email services: %w", err)
	}

	return services, nil
}

func (r *OutlookPostgresRepository) ListAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error) {
	if len(emails) == 0 {
		return []RegisteredAccountRecord{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, email, refresh_token
		FROM accounts
		WHERE email = ANY($1)
		ORDER BY id ASC
	`, emails)
	if err != nil {
		return nil, fmt.Errorf("query accounts by email: %w", err)
	}
	defer rows.Close()

	accounts := make([]RegisteredAccountRecord, 0)
	for rows.Next() {
		var (
			id           int32
			email        string
			refreshToken pgtype.Text
		)
		if err := rows.Scan(&id, &email, &refreshToken); err != nil {
			return nil, fmt.Errorf("scan account by email: %w", err)
		}

		record := RegisteredAccountRecord{
			ID:    int(id),
			Email: email,
		}
		if refreshToken.Valid {
			record.RefreshToken = refreshToken.String
		}
		accounts = append(accounts, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts by email: %w", err)
	}

	return accounts, nil
}
