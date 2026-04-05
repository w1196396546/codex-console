package logs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresRepository struct {
	db postgresQuerier
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: pool}
}

func newPostgresRepository(db postgresQuerier) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListLogs(ctx context.Context, req ListLogsRequest) ([]AppLogRecord, int, error) {
	normalized := req.Normalized()

	countQuery, countArgs := buildCountLogsQuery(normalized, time.Now().UTC())
	var total int
	if err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count app logs: %w", err)
	}

	query, args := buildListLogsQuery(normalized, time.Now().UTC())
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query app logs: %w", err)
	}
	defer rows.Close()

	records := make([]AppLogRecord, 0, normalized.PageSize)
	for rows.Next() {
		record, err := scanAppLogRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate app logs: %w", err)
	}

	return records, total, nil
}

func (r *PostgresRepository) GetStats(ctx context.Context) (LogsStats, error) {
	var (
		total    int
		latestAt *time.Time
	)
	if err := r.db.QueryRow(ctx, `SELECT COUNT(id), MAX(created_at) FROM app_logs`).Scan(&total, &latestAt); err != nil {
		return LogsStats{}, fmt.Errorf("query app log stats: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT COALESCE(level, 'UNKNOWN'), COUNT(id)
		FROM app_logs
		GROUP BY COALESCE(level, 'UNKNOWN')
	`)
	if err != nil {
		return LogsStats{}, fmt.Errorf("query app log levels: %w", err)
	}
	defer rows.Close()

	levels := make(map[string]int)
	for rows.Next() {
		var (
			level string
			count int
		)
		if err := rows.Scan(&level, &count); err != nil {
			return LogsStats{}, fmt.Errorf("scan app log level row: %w", err)
		}
		levels[level] = count
	}
	if err := rows.Err(); err != nil {
		return LogsStats{}, fmt.Errorf("iterate app log level rows: %w", err)
	}

	return LogsStats{
		Total:    total,
		LatestAt: latestAt,
		Levels:   levels,
	}, nil
}

func (r *PostgresRepository) CleanupLogs(ctx context.Context, req CleanupRequest) (CleanupResult, error) {
	normalized := req.Normalized()
	retentionDays := defaultCleanupRetention
	if normalized.RetentionDays != nil {
		retentionDays = *normalized.RetentionDays
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	var deletedByAge int
	if err := r.db.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM app_logs
			WHERE created_at < $1
			RETURNING 1
		)
		SELECT COUNT(*) FROM deleted
	`, cutoff).Scan(&deletedByAge); err != nil {
		return CleanupResult{}, fmt.Errorf("cleanup app logs by age: %w", err)
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(id) FROM app_logs`).Scan(&total); err != nil {
		return CleanupResult{}, fmt.Errorf("count app logs after age cleanup: %w", err)
	}

	deletedByLimit := 0
	if total > normalized.MaxRows {
		overflow := total - normalized.MaxRows
		if err := r.db.QueryRow(ctx, `
			WITH oldest AS (
				SELECT id
				FROM app_logs
				ORDER BY created_at ASC, id ASC
				LIMIT $1
			),
			deleted AS (
				DELETE FROM app_logs
				WHERE id IN (SELECT id FROM oldest)
				RETURNING 1
			)
			SELECT COUNT(*) FROM deleted
		`, overflow).Scan(&deletedByLimit); err != nil {
			return CleanupResult{}, fmt.Errorf("cleanup app logs by limit: %w", err)
		}
	}

	var remaining int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(id) FROM app_logs`).Scan(&remaining); err != nil {
		return CleanupResult{}, fmt.Errorf("count app logs after cleanup: %w", err)
	}

	return CleanupResult{
		RetentionDays:  retentionDays,
		MaxRows:        normalized.MaxRows,
		DeletedByAge:   deletedByAge,
		DeletedByLimit: deletedByLimit,
		DeletedTotal:   deletedByAge + deletedByLimit,
		Remaining:      remaining,
	}, nil
}

func (r *PostgresRepository) ClearLogs(ctx context.Context) (int, error) {
	var deletedTotal int
	if err := r.db.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM app_logs
			RETURNING 1
		)
		SELECT COUNT(*) FROM deleted
	`).Scan(&deletedTotal); err != nil {
		return 0, fmt.Errorf("clear app logs: %w", err)
	}
	return deletedTotal, nil
}

func buildCountLogsQuery(req ListLogsRequest, now time.Time) (string, []any) {
	whereSQL, args := buildLogsWhereClause(req, now)
	return "SELECT COUNT(id) FROM app_logs" + whereSQL, args
}

func buildListLogsQuery(req ListLogsRequest, now time.Time) (string, []any) {
	normalized := req.Normalized()
	whereSQL, args := buildLogsWhereClause(normalized, now)
	args = append(args, normalized.PageSize, normalized.Offset())

	var query strings.Builder
	query.WriteString(`
SELECT
	id,
	level,
	logger,
	COALESCE(module, ''),
	COALESCE(pathname, ''),
	COALESCE(lineno, 0),
	message,
	COALESCE(exception, ''),
	created_at
FROM app_logs`)
	query.WriteString(whereSQL)
	query.WriteString(`
ORDER BY created_at DESC, id DESC
LIMIT $`)
	query.WriteString(fmt.Sprintf("%d", len(args)-1))
	query.WriteString(` OFFSET $`)
	query.WriteString(fmt.Sprintf("%d", len(args)))

	return query.String(), args
}

func buildLogsWhereClause(req ListLogsRequest, now time.Time) (string, []any) {
	normalized := req.Normalized()
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 6)

	if normalized.Level != "" {
		args = append(args, normalized.Level)
		clauses = append(clauses, fmt.Sprintf("level = $%d", len(args)))
	}
	if normalized.LoggerName != "" {
		args = append(args, "%"+normalized.LoggerName+"%")
		clauses = append(clauses, fmt.Sprintf("logger ILIKE $%d", len(args)))
	}
	if normalized.Keyword != "" {
		args = append(args, "%"+normalized.Keyword+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, fmt.Sprintf("(message ILIKE %s OR logger ILIKE %s OR module ILIKE %s)", placeholder, placeholder, placeholder))
	}
	if normalized.SinceMinutes > 0 {
		args = append(args, now.Add(-time.Duration(normalized.SinceMinutes)*time.Minute))
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return "\nWHERE " + strings.Join(clauses, "\n  AND "), args
}

func scanAppLogRecord(scanner interface{ Scan(dest ...any) error }) (AppLogRecord, error) {
	var (
		record    AppLogRecord
		module    string
		pathname  string
		lineNo    int
		exception string
	)
	if err := scanner.Scan(
		&record.ID,
		&record.Level,
		&record.Logger,
		&module,
		&pathname,
		&lineNo,
		&record.Message,
		&exception,
		&record.CreatedAt,
	); err != nil {
		return AppLogRecord{}, fmt.Errorf("scan app log row: %w", err)
	}
	record.Module = module
	record.Pathname = pathname
	record.LineNo = lineNo
	record.Exception = exception
	return record, nil
}
