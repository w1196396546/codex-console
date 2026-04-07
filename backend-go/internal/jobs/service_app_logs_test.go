package jobs_test

import (
	"context"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestAppendLogMirrorsToAppLogSink(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	sink := &recordingAppLogSink{}
	svc := jobs.NewService(repo, nil, jobs.WithAppLogSink(sink))

	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "registration_single",
		ScopeType: "registration",
		ScopeID:   "task-1",
		Payload:   []byte(`{"email_service_type":"temp_mail"}`),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := svc.AppendLog(context.Background(), job.JobID, "error", "prepare registration flow failed"); err != nil {
		t.Fatalf("append log: %v", err)
	}

	if len(sink.entries) != 1 {
		t.Fatalf("expected one mirrored app log entry, got %+v", sink.entries)
	}
	if sink.entries[0].Level != "ERROR" {
		t.Fatalf("expected normalized level ERROR, got %+v", sink.entries[0])
	}
	if sink.entries[0].Logger != "jobs.runtime" {
		t.Fatalf("expected logger jobs.runtime, got %+v", sink.entries[0])
	}
	if sink.entries[0].Message == "" {
		t.Fatalf("expected mirrored message, got %+v", sink.entries[0])
	}
}

type recordingAppLogSink struct {
	entries []appLogEntry
}

type appLogEntry struct {
	Level   string
	Logger  string
	Message string
}

func (s *recordingAppLogSink) AppendAppLog(_ context.Context, level string, logger string, message string) error {
	s.entries = append(s.entries, appLogEntry{
		Level:   level,
		Logger:  logger,
		Message: message,
	})
	return nil
}
