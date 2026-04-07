package registration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestExecutorRunsRegistrationSingleJobAndStreamsLogs(t *testing.T) {
	emailServiceID := 42
	runner := &fakeRunner{
		runFn: func(_ context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error) {
			if req.TaskUUID != "job-1" {
				t.Fatalf("expected task uuid job-1, got %q", req.TaskUUID)
			}
			if req.Plan.Stage != registration.ExecuteStageRegistration {
				t.Fatalf("expected execute stage, got %+v", req.Plan)
			}
			if req.StartRequest.EmailServiceType != "outlook" {
				t.Fatalf("expected outlook payload, got %+v", req.StartRequest)
			}
			if req.StartRequest.EmailServiceID == nil || *req.StartRequest.EmailServiceID != emailServiceID {
				t.Fatalf("expected email_service_id=%d, got %+v", emailServiceID, req.StartRequest.EmailServiceID)
			}
			if req.StartRequest.Proxy != "http://proxy.internal:8080" {
				t.Fatalf("expected proxy to round-trip, got %+v", req.StartRequest)
			}
			if req.StartRequest.ChatGPTRegistrationMode != "access_token_only" {
				t.Fatalf("expected registration mode to round-trip, got %+v", req.StartRequest)
			}
			if !req.StartRequest.AutoUploadCPA || len(req.StartRequest.CPAServiceIDs) != 2 {
				t.Fatalf("expected CPA upload fields preserved, got %+v", req.StartRequest)
			}
			if !req.Plan.EmailService.Prepared {
				t.Fatalf("expected prepared email service plan, got %+v", req.Plan.EmailService)
			}
			if req.Plan.EmailService.Type != "outlook" {
				t.Fatalf("expected prepared outlook type, got %+v", req.Plan.EmailService)
			}
			if req.Plan.EmailService.ServiceID == nil || *req.Plan.EmailService.ServiceID != emailServiceID {
				t.Fatalf("expected prepared email_service_id=%d, got %+v", emailServiceID, req.Plan.EmailService.ServiceID)
			}
			if req.Plan.Proxy.Selected != "http://proxy.internal:8080" || req.Plan.Proxy.Source != "request" {
				t.Fatalf("expected proxy plan to preserve request proxy, got %+v", req.Plan.Proxy)
			}
			if req.Plan.Outlook == nil {
				t.Fatalf("expected outlook preparation details, got %+v", req.Plan)
			}
			if req.Plan.Outlook.ReservationStatus != "reservation_not_configured" {
				t.Fatalf("expected reservation skeleton status, got %+v", req.Plan.Outlook)
			}
			if err := logf("info", "native flow started"); err != nil {
				return registration.RunnerOutput{}, err
			}
			if err := logf("warning", "following pending oauth continuation"); err != nil {
				return registration.RunnerOutput{}, err
			}
			return registration.RunnerOutput{Result: map[string]any{
				"email":   "alice@example.com",
				"success": true,
			}}, nil
		},
	}
	logger := &fakeJobLogger{}
	executor := registration.NewExecutor(logger, runner, registration.WithPreparationDependencies(registration.PreparationDependencies{
		EmailServices: executorFakeEmailServiceCatalog{
			services: []registration.EmailServiceRecord{
				{
					ID:          emailServiceID,
					ServiceType: "outlook",
					Name:        "Outlook Prepared",
					Config: map[string]any{
						"email":         "prepared@example.com",
						"client_id":     "client-1",
						"refresh_token": "refresh-1",
					},
				},
			},
		},
		Outlook: executorFakeOutlookReader{},
	}))

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-1",
		JobType: "registration_single",
		Payload: []byte(`{"email_service_type":"outlook","chatgpt_registration_mode":"access_token_only","email_service_id":42,"proxy":"http://proxy.internal:8080","auto_upload_cpa":true,"cpa_service_ids":[11,22]}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if result["email"] != "alice@example.com" {
		t.Fatalf("expected result email, got %#v", result)
	}
	if len(logger.entries) != 2 {
		t.Fatalf("expected two streamed log entries, got %#v", logger.entries)
	}
	if logger.entries[0].level != "info" || logger.entries[0].message != "native flow started" {
		t.Fatalf("unexpected first log entry: %+v", logger.entries[0])
	}
	if logger.entries[1].level != "warning" || logger.entries[1].message != "following pending oauth continuation" {
		t.Fatalf("unexpected second log entry: %+v", logger.entries[1])
	}
}

func TestExecutorReturnsPayloadDecodeError(t *testing.T) {
	executor := registration.NewExecutor(&fakeJobLogger{}, &fakeRunner{})

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-1",
		JobType: "registration_single",
		Payload: []byte(`{`),
	})
	if err == nil {
		t.Fatal("expected payload decode error")
	}
}

func TestExecutorReturnsRunnerError(t *testing.T) {
	logger := &fakeJobLogger{}
	executor := registration.NewExecutor(logger, &fakeRunner{
		runFn: func(_ context.Context, _ registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error) {
			if err := logf("info", "native flow started"); err != nil {
				return registration.RunnerOutput{}, err
			}
			return registration.RunnerOutput{}, errors.New("native flow failed")
		},
	}, registration.WithPreparationDependencies(executorPreparationDeps()))

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-1",
		JobType: "registration_single",
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err == nil {
		t.Fatal("expected runner error")
	}
	if len(logger.entries) != 1 {
		t.Fatalf("expected one streamed log entry before failure, got %#v", logger.entries)
	}
	if logger.entries[0].message != "native flow started" {
		t.Fatalf("unexpected log entry: %+v", logger.entries[0])
	}
}

func TestExecutorWrapsPreparationError(t *testing.T) {
	executor := registration.NewExecutor(
		&fakeJobLogger{},
		&fakeRunner{
			runFn: func(_ context.Context, _ registration.RunnerRequest, _ func(level string, message string) error) (registration.RunnerOutput, error) {
				t.Fatal("runner should not be called when preparation fails")
				return registration.RunnerOutput{}, nil
			},
		},
		registration.WithPreparationDependencies(registration.PreparationDependencies{
			Settings: failingPreparationSettings{err: errors.New("settings unavailable")},
		}),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-prepare-fail",
		JobType: "registration_single",
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err == nil {
		t.Fatal("expected preparation error")
	}
	if !strings.Contains(err.Error(), "prepare registration flow") {
		t.Fatalf("expected preparation wrapper, got %v", err)
	}
	if !strings.Contains(err.Error(), "settings unavailable") {
		t.Fatalf("expected underlying settings error, got %v", err)
	}
}

func TestExecutorRejectsUnsupportedEmailServiceBeforeRunnerStarts(t *testing.T) {
	executor := registration.NewExecutor(
		&fakeJobLogger{},
		&fakeRunner{
			runFn: func(_ context.Context, _ registration.RunnerRequest, _ func(level string, message string) error) (registration.RunnerOutput, error) {
				t.Fatal("runner should not be called when email service type is unsupported")
				return registration.RunnerOutput{}, nil
			},
		},
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-unsupported-service",
		JobType: "registration_single",
		Payload: []byte(`{"email_service_type":"custom_mail","email_service_config":{"base_url":"https://custom.example/api"}}`),
	})
	if err == nil {
		t.Fatal("expected unsupported email service error")
	}
	if !strings.Contains(err.Error(), "prepare registration flow") {
		t.Fatalf("expected preparation wrapper, got %v", err)
	}
	if !strings.Contains(err.Error(), "does not support this email service") {
		t.Fatalf("expected unsupported email service reason, got %v", err)
	}
}

func TestExecutorRejectsTempmailWithoutConfiguredBaseURLBeforeRunnerStarts(t *testing.T) {
	executor := registration.NewExecutor(
		&fakeJobLogger{},
		&fakeRunner{
			runFn: func(_ context.Context, _ registration.RunnerRequest, _ func(level string, message string) error) (registration.RunnerOutput, error) {
				t.Fatal("runner should not be called when tempmail base_url is missing")
				return registration.RunnerOutput{}, nil
			},
		},
		registration.WithPreparationDependencies(registration.PreparationDependencies{
			Settings: executorPreparationSettings{
				settings: map[string]string{
					"tempmail.enabled": "true",
				},
			},
		}),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-tempmail-missing-base-url",
		JobType: "registration_single",
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err == nil {
		t.Fatal("expected missing tempmail base_url error")
	}
	if !strings.Contains(err.Error(), "prepare registration flow") {
		t.Fatalf("expected preparation wrapper, got %v", err)
	}
	if !strings.Contains(err.Error(), "tempmail") || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("expected missing tempmail base_url reason, got %v", err)
	}
}

type fakeRunner struct {
	runFn func(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error)
}

func (f *fakeRunner) Run(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error) {
	if f.runFn != nil {
		return f.runFn(ctx, req, logf)
	}
	return registration.RunnerOutput{Result: map[string]any{"email": "default@example.com"}}, nil
}

type fakeJobLogger struct {
	entries []jobLogEntry
}

func (f *fakeJobLogger) AppendLog(_ context.Context, _ string, level string, message string) error {
	f.entries = append(f.entries, jobLogEntry{level: level, message: message})
	return nil
}

type jobLogEntry struct {
	level   string
	message string
}

type executorFakeEmailServiceCatalog struct {
	services []registration.EmailServiceRecord
}

func (f executorFakeEmailServiceCatalog) ListEmailServices(context.Context) ([]registration.EmailServiceRecord, error) {
	return append([]registration.EmailServiceRecord(nil), f.services...), nil
}

type executorFakeOutlookReader struct{}

func (executorFakeOutlookReader) ListOutlookServices(context.Context) ([]registration.EmailServiceRecord, error) {
	return nil, nil
}

func (executorFakeOutlookReader) ListAccountsByEmails(context.Context, []string) ([]registration.RegisteredAccountRecord, error) {
	return nil, nil
}

type failingPreparationSettings struct {
	err error
}

func (f failingPreparationSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	return nil, f.err
}

type executorPreparationSettings struct {
	settings map[string]string
}

func (s executorPreparationSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	return s.settings, nil
}

func executorPreparationDeps() registration.PreparationDependencies {
	return registration.PreparationDependencies{
		Settings: executorPreparationSettings{
			settings: map[string]string{
				"tempmail.enabled":     "true",
				"tempmail.base_url":    "https://api.tempmail.example/v2",
				"tempmail.timeout":     "45",
				"tempmail.max_retries": "7",
			},
		},
	}
}
