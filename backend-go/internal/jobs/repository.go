package jobs

import (
	"context"
	"crypto/rand"
	"fmt"

	jobsdb "github.com/dou-jiang/codex-console/backend-go/internal/jobs/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository interface {
	CreateJob(ctx context.Context, params CreateJobParams) (Job, error)
	GetJob(ctx context.Context, jobID string) (Job, error)
}

type SQLCRepository struct {
	queries *jobsdb.Queries
	db      jobsdb.DBTX
}

func NewRepository(db jobsdb.DBTX) *SQLCRepository {
	return &SQLCRepository{
		queries: jobsdb.New(db),
		db:      db,
	}
}

func NewRepositoryWithQueries(queries *jobsdb.Queries) *SQLCRepository {
	return &SQLCRepository{
		queries: queries,
	}
}

func (r *SQLCRepository) CreateJob(ctx context.Context, params CreateJobParams) (Job, error) {
	jobID, err := newUUID()
	if err != nil {
		return Job{}, fmt.Errorf("generate job id: %w", err)
	}

	record, err := r.queries.CreateJob(ctx, jobsdb.CreateJobParams{
		JobID:     jobID,
		JobType:   params.JobType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Status:    StatusPending,
		Priority:  0,
		Payload:   append([]byte(nil), params.Payload...),
	})
	if err != nil {
		return Job{}, fmt.Errorf("create job: %w", err)
	}

	return mapJob(record), nil
}

func (r *SQLCRepository) GetJob(ctx context.Context, jobID string) (Job, error) {
	parsedJobID, err := parseUUID(jobID)
	if err != nil {
		return Job{}, fmt.Errorf("parse job id: %w", err)
	}

	record, err := r.queries.GetJob(ctx, parsedJobID)
	if err != nil {
		return Job{}, fmt.Errorf("get job: %w", err)
	}

	return mapJob(record), nil
}

func mapJob(record jobsdb.Job) Job {
	return Job{
		JobID:     formatUUID(record.JobID),
		JobType:   record.JobType,
		ScopeType: record.ScopeType,
		ScopeID:   record.ScopeID,
		Status:    record.Status,
		Payload:   append([]byte(nil), record.Payload...),
	}
}

func newUUID() (pgtype.UUID, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return pgtype.UUID{}, err
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return pgtype.UUID{
		Bytes: bytes,
		Valid: true,
	}, nil
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	b := id.Bytes[:]
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
}

func parseUUID(raw string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
