package jobs

import (
	"context"
	"fmt"

	jobsdb "github.com/dou-jiang/codex-console/backend-go/internal/jobs/sqlc"
	"github.com/jackc/pgx/v5"
)

const jobColumns = `
job_id,
job_type,
scope_type,
scope_id,
status,
priority,
payload,
result,
error,
created_at,
started_at,
finished_at
`

func (r *SQLCRepository) UpdateJobStatus(ctx context.Context, jobID string, status string) (Job, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return Job{}, fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return Job{}, ErrControlNotSupported
	}

	row := r.db.QueryRow(ctx, `
UPDATE jobs
SET status = $2
WHERE job_id = $1
RETURNING `+jobColumns, parsedJobID, status)

	record, err := scanJob(row)
	if err != nil {
		return Job{}, fmt.Errorf("update job status: %w", err)
	}
	return mapJob(record), nil
}

func (r *SQLCRepository) MarkJobRunning(ctx context.Context, jobID string, workerID string) (Job, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return Job{}, fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return Job{}, ErrControlNotSupported
	}

	jobRunID, err := newUUID()
	if err != nil {
		return Job{}, fmt.Errorf("generate job run id: %w", err)
	}

	return r.withJobTx(ctx, func(tx jobsdb.DBTX) (Job, error) {
		if _, err := tx.Exec(ctx, `
INSERT INTO job_runs (job_run_id, job_id, worker_id, attempt, status)
VALUES ($1, $2, $3, $4, $5)
`, jobRunID, parsedJobID, workerID, 1, StatusRunning); err != nil {
			return Job{}, fmt.Errorf("insert job run: %w", err)
		}

		row := tx.QueryRow(ctx, `
UPDATE jobs
SET status = $2,
    started_at = COALESCE(started_at, NOW()),
    finished_at = NULL,
    error = NULL
WHERE job_id = $1
RETURNING `+jobColumns, parsedJobID, StatusRunning)

		record, err := scanJob(row)
		if err != nil {
			return Job{}, fmt.Errorf("mark job running: %w", err)
		}
		return mapJob(record), nil
	})
}

func (r *SQLCRepository) MarkJobCompleted(ctx context.Context, jobID string, result []byte) (Job, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return Job{}, fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return Job{}, ErrControlNotSupported
	}

	return r.withJobTx(ctx, func(tx jobsdb.DBTX) (Job, error) {
		if _, err := tx.Exec(ctx, `
UPDATE job_runs
SET status = $2,
    finished_at = NOW()
WHERE job_id = $1
  AND finished_at IS NULL
`, parsedJobID, StatusCompleted); err != nil {
			return Job{}, fmt.Errorf("update job run: %w", err)
		}

		row := tx.QueryRow(ctx, `
UPDATE jobs
SET status = $2,
    result = $3,
    error = NULL,
    finished_at = NOW()
WHERE job_id = $1
RETURNING `+jobColumns, parsedJobID, StatusCompleted, result)

		record, err := scanJob(row)
		if err != nil {
			return Job{}, fmt.Errorf("mark job completed: %w", err)
		}
		return mapJob(record), nil
	})
}

func (r *SQLCRepository) MarkJobFailed(ctx context.Context, jobID string, message string) (Job, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return Job{}, fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return Job{}, ErrControlNotSupported
	}

	return r.withJobTx(ctx, func(tx jobsdb.DBTX) (Job, error) {
		if _, err := tx.Exec(ctx, `
UPDATE job_runs
SET status = $2,
    finished_at = NOW()
WHERE job_id = $1
  AND finished_at IS NULL
`, parsedJobID, StatusFailed); err != nil {
			return Job{}, fmt.Errorf("update failed job run: %w", err)
		}

		row := tx.QueryRow(ctx, `
UPDATE jobs
SET status = $2,
    error = $3,
    finished_at = NOW()
WHERE job_id = $1
RETURNING `+jobColumns, parsedJobID, StatusFailed, message)

		record, err := scanJob(row)
		if err != nil {
			return Job{}, fmt.Errorf("mark job failed: %w", err)
		}
		return mapJob(record), nil
	})
}

func (r *SQLCRepository) AppendJobLog(ctx context.Context, jobID string, level string, message string) error {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return ErrControlNotSupported
	}

	_, err = r.db.Exec(ctx, `
INSERT INTO job_logs (job_id, level, message)
VALUES ($1, $2, $3)
`, parsedJobID, level, message)
	if err != nil {
		return fmt.Errorf("append job log: %w", err)
	}
	return nil
}

func (r *SQLCRepository) ListJobLogs(ctx context.Context, jobID string) ([]JobLog, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return nil, fmt.Errorf("parse job id: %w", err)
	}
	if r.db == nil {
		return nil, ErrControlNotSupported
	}

	rows, err := r.db.Query(ctx, `
SELECT level, message
FROM job_logs
WHERE job_id = $1
ORDER BY seq ASC
`, parsedJobID)
	if err != nil {
		return nil, fmt.Errorf("list job logs: %w", err)
	}
	defer rows.Close()

	logs := make([]JobLog, 0)
	for rows.Next() {
		var log JobLog
		var level string
		if err := rows.Scan(&level, &log.Message); err != nil {
			return nil, fmt.Errorf("scan job log: %w", err)
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job logs: %w", err)
	}
	return logs, nil
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(row jobScanner) (jobsdb.Job, error) {
	return scanJobRecord(row)
}

func scanJobRecord(row jobScanner) (jobsdb.Job, error) {
	var jobRecord jobsdb.Job
	err := row.Scan(
		&jobRecord.JobID,
		&jobRecord.JobType,
		&jobRecord.ScopeType,
		&jobRecord.ScopeID,
		&jobRecord.Status,
		&jobRecord.Priority,
		&jobRecord.Payload,
		&jobRecord.Result,
		&jobRecord.Error,
		&jobRecord.CreatedAt,
		&jobRecord.StartedAt,
		&jobRecord.FinishedAt,
	)
	return jobRecord, err
}

type txBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func (r *SQLCRepository) withJobTx(
	ctx context.Context,
	run func(jobsdb.DBTX) (Job, error),
) (Job, error) {
	beginner, ok := r.db.(txBeginner)
	if !ok {
		return run(r.db)
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("begin tx: %w", err)
	}

	job, err := run(tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return Job{}, fmt.Errorf("commit tx: %w", err)
	}

	return job, nil
}
