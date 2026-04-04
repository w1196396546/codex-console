package registration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewStatsPostgresRepository(pool *pgxpool.Pool) *StatsPostgresRepository {
	return &StatsPostgresRepository{pool: pool}
}

func (r *StatsPostgresRepository) ListStatusCounts(ctx context.Context, from *time.Time, to *time.Time) (map[string]int, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT status, COUNT(*)
		FROM jobs
		WHERE job_type = 'registration_single'
	`)

	args := make([]any, 0, 2)
	if from != nil {
		query.WriteString(` AND created_at >= $1`)
		args = append(args, *from)
	}
	if to != nil {
		query.WriteString(fmt.Sprintf(" AND created_at < $%d", len(args)+1))
		args = append(args, *to)
	}
	query.WriteString(` GROUP BY status`)

	rows, err := r.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query registration stats: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var (
			status string
			count  int64
		)
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan registration stats: %w", err)
		}
		counts[status] = int(count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registration stats: %w", err)
	}

	return counts, nil
}
