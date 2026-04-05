package emailservices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool         *pgxpool.Pool
	settingsRepo *registration.AvailableServicesPostgresRepository
	outlookRepo  *registration.OutlookPostgresRepository
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	if pool == nil {
		return &PostgresRepository{}
	}

	return &PostgresRepository{
		pool:         pool,
		settingsRepo: registration.NewAvailableServicesPostgresRepository(pool),
		outlookRepo:  registration.NewOutlookPostgresRepository(pool),
	}
}

func (r *PostgresRepository) ListServices(ctx context.Context, req ListServicesRequest) ([]EmailServiceRecord, error) {
	if r == nil || r.pool == nil {
		return []EmailServiceRecord{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
		FROM email_services
		WHERE ($1 = '' OR service_type = $1)
		  AND ($2 = FALSE OR enabled = TRUE)
		ORDER BY priority ASC, id ASC
	`, req.ServiceType, req.EnabledOnly)
	if err != nil {
		return nil, fmt.Errorf("query email services: %w", err)
	}
	defer rows.Close()

	services := make([]EmailServiceRecord, 0)
	for rows.Next() {
		record, err := scanEmailService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email services: %w", err)
	}

	return services, nil
}

func (r *PostgresRepository) GetService(ctx context.Context, serviceID int) (EmailServiceRecord, bool, error) {
	if r == nil || r.pool == nil {
		return EmailServiceRecord{}, false, nil
	}

	row := r.pool.QueryRow(ctx, `
		SELECT id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
		FROM email_services
		WHERE id = $1
	`, serviceID)
	record, err := scanEmailService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmailServiceRecord{}, false, nil
		}
		return EmailServiceRecord{}, false, err
	}
	return record, true, nil
}

func (r *PostgresRepository) FindServiceByName(ctx context.Context, name string) (EmailServiceRecord, bool, error) {
	if r == nil || r.pool == nil {
		return EmailServiceRecord{}, false, nil
	}

	row := r.pool.QueryRow(ctx, `
		SELECT id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
		FROM email_services
		WHERE name = $1
		ORDER BY id ASC
		LIMIT 1
	`, name)
	record, err := scanEmailService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmailServiceRecord{}, false, nil
		}
		return EmailServiceRecord{}, false, err
	}
	return record, true, nil
}

func (r *PostgresRepository) CreateService(ctx context.Context, service EmailServiceRecord) (EmailServiceRecord, error) {
	if r == nil || r.pool == nil {
		return EmailServiceRecord{}, nil
	}

	configRaw, err := json.Marshal(normalizeConfigForStorage(service.Config))
	if err != nil {
		return EmailServiceRecord{}, fmt.Errorf("marshal email service config: %w", err)
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO email_services (service_type, name, config, enabled, priority)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
	`, service.ServiceType, service.Name, string(configRaw), service.Enabled, service.Priority)
	return scanEmailService(row)
}

func (r *PostgresRepository) SaveService(ctx context.Context, service EmailServiceRecord) (EmailServiceRecord, error) {
	if r == nil || r.pool == nil {
		return service, nil
	}

	configRaw, err := json.Marshal(normalizeConfigForStorage(service.Config))
	if err != nil {
		return EmailServiceRecord{}, fmt.Errorf("marshal email service config: %w", err)
	}

	row := r.pool.QueryRow(ctx, `
		UPDATE email_services
		SET name = $2,
			config = $3::jsonb,
			enabled = $4,
			priority = $5,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
	`, service.ID, service.Name, string(configRaw), service.Enabled, service.Priority)
	return scanEmailService(row)
}

func (r *PostgresRepository) DeleteService(ctx context.Context, serviceID int) (EmailServiceRecord, bool, error) {
	if r == nil || r.pool == nil {
		return EmailServiceRecord{}, false, nil
	}

	row := r.pool.QueryRow(ctx, `
		DELETE FROM email_services
		WHERE id = $1
		RETURNING id, service_type, name, config::text, enabled, priority, last_used, created_at, updated_at
	`, serviceID)
	record, err := scanEmailService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmailServiceRecord{}, false, nil
		}
		return EmailServiceRecord{}, false, err
	}
	return record, true, nil
}

func (r *PostgresRepository) UpdateServicePriority(ctx context.Context, serviceID int, priority int) error {
	if r == nil || r.pool == nil {
		return nil
	}
	_, err := r.pool.Exec(ctx, `UPDATE email_services SET priority = $2, updated_at = NOW() WHERE id = $1`, serviceID, priority)
	if err != nil {
		return fmt.Errorf("update email service priority: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CountServices(ctx context.Context) (map[string]int, int, error) {
	if r == nil || r.pool == nil {
		return map[string]int{}, 0, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT service_type, COUNT(*)
		FROM email_services
		GROUP BY service_type
	`)
	if err != nil {
		return nil, 0, fmt.Errorf("count email services by type: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var (
			serviceType string
			count       int
		)
		if err := rows.Scan(&serviceType, &count); err != nil {
			return nil, 0, fmt.Errorf("scan email service count: %w", err)
		}
		counts[serviceType] = count
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate email service counts: %w", err)
	}

	var enabledCount int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_services WHERE enabled = TRUE`).Scan(&enabledCount); err != nil {
		return nil, 0, fmt.Errorf("count enabled email services: %w", err)
	}

	return counts, enabledCount, nil
}

func (r *PostgresRepository) GetSettings(ctx context.Context, keys []string) (map[string]string, error) {
	if r == nil || r.settingsRepo == nil {
		return map[string]string{}, nil
	}
	return r.settingsRepo.GetSettings(ctx, keys)
}

func (r *PostgresRepository) ListRegisteredAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error) {
	if r == nil || r.outlookRepo == nil {
		return []RegisteredAccountRecord{}, nil
	}

	rows, err := r.outlookRepo.ListAccountsByEmails(ctx, emails)
	if err != nil {
		return nil, err
	}

	records := make([]RegisteredAccountRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, RegisteredAccountRecord{
			ID:    row.ID,
			Email: row.Email,
		})
	}
	return records, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEmailService(source scanner) (EmailServiceRecord, error) {
	var (
		record    EmailServiceRecord
		configRaw string
		lastUsed  pgtype.Timestamptz
		createdAt pgtype.Timestamptz
		updatedAt pgtype.Timestamptz
	)
	if err := source.Scan(
		&record.ID,
		&record.ServiceType,
		&record.Name,
		&configRaw,
		&record.Enabled,
		&record.Priority,
		&lastUsed,
		&createdAt,
		&updatedAt,
	); err != nil {
		return EmailServiceRecord{}, err
	}

	record.Config = make(map[string]any)
	if configRaw != "" {
		if err := json.Unmarshal([]byte(configRaw), &record.Config); err != nil {
			return EmailServiceRecord{}, fmt.Errorf("decode email service config for %s(%d): %w", record.ServiceType, record.ID, err)
		}
	}
	record.LastUsed = timestampPtr(lastUsed)
	record.CreatedAt = timestampPtr(createdAt)
	record.UpdatedAt = timestampPtr(updatedAt)
	return record, nil
}

func normalizeConfigForStorage(config map[string]any) map[string]any {
	if len(config) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(config))
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}
