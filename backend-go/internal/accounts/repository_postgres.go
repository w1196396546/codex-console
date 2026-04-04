package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error) {
	normalized := req.Normalized()

	total, err := r.countAccounts(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, email, password, status, registered_at, created_at, updated_at
		FROM accounts
		ORDER BY COALESCE(registered_at, created_at, updated_at) DESC, id DESC
		LIMIT $1 OFFSET $2
	`, normalized.PageSize, normalized.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, normalized.PageSize)
	for rows.Next() {
		var (
			account      Account
			registeredAt *time.Time
			createdAt    *time.Time
			updatedAt    *time.Time
		)
		if err := rows.Scan(
			&account.ID,
			&account.Email,
			&account.Password,
			&account.Status,
			&registeredAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan account: %w", err)
		}

		account.RegisteredAt = registeredAt
		account.CreatedAt = createdAt
		account.UpdatedAt = updatedAt
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, total, nil
}

func (r *PostgresRepository) countAccounts(ctx context.Context) (int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&total); err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}

	return total, nil
}
