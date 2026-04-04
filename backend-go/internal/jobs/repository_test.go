package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreateJobPersistsPendingJobViaSQLCRepository(t *testing.T) {
	db := &fakeDBTX{}
	repo := jobs.NewRepository(db)

	job, err := repo.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
		Payload:   []byte(`{"team_id":42}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.query, "INSERT INTO jobs") {
		t.Fatalf("expected insert query, got %q", db.query)
	}
	if len(db.args) != 7 {
		t.Fatalf("expected 7 args, got %d", len(db.args))
	}
	if got := db.args[1]; got != "team_sync" {
		t.Fatalf("expected job_type team_sync, got %#v", got)
	}
	if got := db.args[4]; got != jobs.StatusPending {
		t.Fatalf("expected pending status arg, got %#v", got)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("expected pending, got %s", job.Status)
	}
	if job.JobType != "team_sync" || job.ScopeType != "team" || job.ScopeID != "42" {
		t.Fatalf("unexpected job mapping: %+v", job)
	}
	if string(job.Payload) != `{"team_id":42}` {
		t.Fatalf("unexpected payload: %s", string(job.Payload))
	}
	if job.JobID == "" {
		t.Fatal("expected generated job id")
	}
}

func TestCreateJobReturnsWrappedErrorWhenInsertFails(t *testing.T) {
	db := &fakeDBTX{
		forcedRow: fakeRow{err: errors.New("insert failed")},
	}
	repo := jobs.NewRepository(db)

	_, err := repo.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
	})
	if err == nil {
		t.Fatal("expected create job to fail")
	}
	if !strings.Contains(err.Error(), "create job") {
		t.Fatalf("expected wrapped create job error, got %v", err)
	}
}

func TestGetJobReturnsMappedJob(t *testing.T) {
	jobID := mustUUID(t, "11111111-2222-4333-8444-555555555555")
	db := &fakeDBTX{
		forcedRow: fakeRow{
			jobID:     jobID,
			jobType:   "team_sync",
			scopeType: "team",
			scopeID:   "42",
			status:    jobs.StatusPending,
			payload:   []byte(`{"team_id":42}`),
		},
	}
	repo := jobs.NewRepository(db)

	job, err := repo.GetJob(context.Background(), "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(db.query, "SELECT") {
		t.Fatalf("expected select query, got %q", db.query)
	}
	if job.JobID != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("unexpected job id: %s", job.JobID)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("expected pending, got %s", job.Status)
	}
}

type fakeDBTX struct {
	query     string
	args      []any
	forcedRow fakeRow
}

func (f *fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakeDBTX) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	f.query = query
	f.args = args

	if f.forcedRow.err != nil || f.forcedRow.jobID.Valid {
		return f.forcedRow
	}

	jobID, ok := args[0].(pgtype.UUID)
	if !ok {
		return fakeRow{err: pgx.ErrNoRows}
	}

	return fakeRow{
		jobID:     jobID,
		jobType:   args[1].(string),
		scopeType: args[2].(string),
		scopeID:   args[3].(string),
		status:    args[4].(string),
		payload:   append([]byte(nil), args[6].([]byte)...),
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var value pgtype.UUID
	if err := value.Scan(raw); err != nil {
		t.Fatalf("unexpected uuid parse error: %v", err)
	}
	return value
}

type fakeRow struct {
	err       error
	jobID     pgtype.UUID
	jobType   string
	scopeType string
	scopeID   string
	status    string
	payload   []byte
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	*(dest[0].(*pgtype.UUID)) = r.jobID
	*(dest[1].(*string)) = r.jobType
	*(dest[2].(*string)) = r.scopeType
	*(dest[3].(*string)) = r.scopeID
	*(dest[4].(*string)) = r.status
	*(dest[5].(*int32)) = 0
	*(dest[6].(*[]byte)) = append([]byte(nil), r.payload...)
	*(dest[7].(*[]byte)) = nil
	*(dest[8].(*pgtype.Text)) = pgtype.Text{}
	*(dest[9].(*pgtype.Timestamptz)) = pgtype.Timestamptz{}
	*(dest[10].(*pgtype.Timestamptz)) = pgtype.Timestamptz{}
	*(dest[11].(*pgtype.Timestamptz)) = pgtype.Timestamptz{}
	return nil
}
