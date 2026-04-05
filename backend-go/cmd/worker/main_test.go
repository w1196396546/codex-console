package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner"
	"github.com/hibiken/asynq"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestNewWorkerExecutorRejectsUnsupportedJobType(t *testing.T) {
	executor := newWorkerExecutor(jobs.ExecutorFunc(func(context.Context, jobs.Job) (map[string]any, error) {
		t.Fatal("registration executor should not be called for unsupported job type")
		return nil, nil
	}))

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-1",
		JobType: "team_sync",
	})
	if err == nil {
		t.Fatal("expected unsupported job type error")
	}
	if !strings.Contains(err.Error(), "unsupported worker job type") {
		t.Fatalf("expected unsupported job type error, got %v", err)
	}
}

func TestWorkerHandleTaskFailsUnsupportedJobType(t *testing.T) {
	svc := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
		Payload:   []byte(`{"team_id":42}`),
	})
	if err != nil {
		t.Fatalf("unexpected create job error: %v", err)
	}

	payload, err := jobs.MarshalQueuePayload(job.JobID)
	if err != nil {
		t.Fatalf("unexpected marshal payload error: %v", err)
	}

	task := asynq.NewTask(jobs.TypeGenericJob, payload)
	worker := jobs.NewWorkerWithIDAndExecutor(svc, "worker-test", newWorkerExecutor(jobs.ExecutorFunc(func(context.Context, jobs.Job) (map[string]any, error) {
		t.Fatal("registration executor should not be called for unsupported job type")
		return nil, nil
	})))

	err = worker.HandleTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected worker to return executor error")
	}
	if !strings.Contains(err.Error(), "unsupported worker job type") {
		t.Fatalf("expected unsupported job type error, got %v", err)
	}

	got, err := svc.GetJob(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("unexpected get job error: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("expected failed status after unsupported job type, got %s", got.Status)
	}
}

func TestNewWorkerExecutorDelegatesRegistrationJob(t *testing.T) {
	executor := newWorkerExecutor(jobs.ExecutorFunc(func(_ context.Context, job jobs.Job) (map[string]any, error) {
		if job.JobType != registration.JobTypeSingle {
			t.Fatalf("expected registration job type, got %q", job.JobType)
		}
		return map[string]any{"ok": true}, nil
	}))

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-2",
		JobType: registration.JobTypeSingle,
	})
	if err != nil {
		t.Fatalf("expected registration job to pass through, got %v", err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected delegated result, got %#v", result)
	}
}

func TestNewRegistrationExecutorUsesPreparationDependencies(t *testing.T) {
	service := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	executor := newRegistrationExecutor(
		service,
		workerMainFakeRunner{
			runFn: func(context.Context, registration.RunnerRequest, func(level string, message string) error) (map[string]any, error) {
				t.Fatal("runner should not be called when preparation fails")
				return nil, nil
			},
		},
		nil,
		nil,
		registration.PreparationDependencies{
			Settings: workerMainFailingPreparationSettings{err: errors.New("settings unavailable")},
		},
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-prepare-fail",
		JobType: registration.JobTypeSingle,
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

func TestNewRegistrationPreparationDependenciesUsesPostgresRepositories(t *testing.T) {
	deps := newRegistrationPreparationDependencies(nil)

	settingsRepo, ok := deps.Settings.(*registration.AvailableServicesPostgresRepository)
	if !ok || settingsRepo == nil {
		t.Fatalf("expected postgres settings repository, got %#v", deps.Settings)
	}

	emailRepo, ok := deps.EmailServices.(*registration.AvailableServicesPostgresRepository)
	if !ok || emailRepo == nil {
		t.Fatalf("expected postgres email services repository, got %#v", deps.EmailServices)
	}
	if emailRepo != settingsRepo {
		t.Fatalf("expected settings and email services to share one repository instance")
	}

	outlookRepo, ok := deps.Outlook.(*registration.OutlookPostgresRepository)
	if !ok || outlookRepo == nil {
		t.Fatalf("expected postgres outlook repository, got %#v", deps.Outlook)
	}
}

func TestNewWorkerRegistrationRunnerUsesNativeRunner(t *testing.T) {
	runner, err := newWorkerRegistrationRunner(nil)
	if err != nil {
		t.Fatalf("unexpected create runner error: %v", err)
	}

	_, err = runner.Run(context.Background(), registration.RunnerRequest{
		TaskUUID: "job-native-runner",
		StartRequest: registration.StartRequest{
			EmailServiceType: "unknown",
		},
		Plan: registration.ExecutionPlan{
			EmailService: registration.PreparedEmailService{
				Prepared: true,
				Type:     "unknown",
			},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected native runner provider error")
	}
	if !strings.Contains(err.Error(), "create native mail provider") {
		t.Fatalf("expected native runner error path, got %v", err)
	}
}

func TestNewWorkerHistoricalPasswordProviderReadsRepositoryPassword(t *testing.T) {
	t.Parallel()

	provider := newWorkerHistoricalPasswordProvider(workerMainFakeAccountRepository{
		account: accounts.Account{
			Email:    "native@example.com",
			Password: "known-pass",
		},
		found: true,
	})
	if provider == nil {
		t.Fatal("expected historical password provider")
	}

	password, err := provider.ResolveHistoricalPassword(context.Background(), nativerunner.FlowRequest{}, "native@example.com")
	if err != nil {
		t.Fatalf("resolve historical password: %v", err)
	}
	if password != "known-pass" {
		t.Fatalf("expected known-pass, got %q", password)
	}
}

func TestNewWorkerHistoricalPasswordProviderReturnsEmptyWhenAccountMissing(t *testing.T) {
	t.Parallel()

	provider := newWorkerHistoricalPasswordProvider(workerMainFakeAccountRepository{})
	if provider == nil {
		t.Fatal("expected historical password provider")
	}

	password, err := provider.ResolveHistoricalPassword(context.Background(), nativerunner.FlowRequest{}, "missing@example.com")
	if err != nil {
		t.Fatalf("resolve historical password: %v", err)
	}
	if password != "" {
		t.Fatalf("expected empty password for missing account, got %q", password)
	}
}

type workerMainFakeRunner struct {
	runFn func(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (map[string]any, error)
}

func (f workerMainFakeRunner) Run(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (map[string]any, error) {
	if f.runFn != nil {
		return f.runFn(ctx, req, logf)
	}
	return map[string]any{"ok": true}, nil
}

type workerMainFailingPreparationSettings struct {
	err error
}

func (f workerMainFailingPreparationSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	return nil, f.err
}

type workerMainFakeAccountRepository struct {
	account accounts.Account
	found   bool
	err     error
}

func (f workerMainFakeAccountRepository) ListAccounts(context.Context, accounts.ListAccountsRequest) ([]accounts.Account, int, error) {
	return nil, 0, nil
}

func (f workerMainFakeAccountRepository) GetAccountByEmail(context.Context, string) (accounts.Account, bool, error) {
	if f.err != nil {
		return accounts.Account{}, false, f.err
	}
	return f.account, f.found, nil
}

func (f workerMainFakeAccountRepository) UpsertAccount(context.Context, accounts.Account) (accounts.Account, error) {
	return accounts.Account{}, nil
}
