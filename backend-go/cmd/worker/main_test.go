package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner"
	"github.com/hibiken/asynq"
	redisv9 "github.com/redis/go-redis/v9"

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
			runFn: func(context.Context, registration.RunnerRequest, func(level string, message string) error) (registration.RunnerOutput, error) {
				t.Fatal("runner should not be called when preparation fails")
				return registration.RunnerOutput{}, nil
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

	proxySelector, ok := deps.Proxies.(*registration.PostgresProxySelector)
	if !ok || proxySelector == nil {
		t.Fatalf("expected postgres proxy selector, got %#v", deps.Proxies)
	}

	reservationStore, ok := deps.Reservations.(*registration.JobPayloadOutlookReservationStore)
	if !ok || reservationStore == nil {
		t.Fatalf("expected jobs-backed outlook reservation store, got %#v", deps.Reservations)
	}
}

func TestNewWorkerRegistrationRunnerUsesNativeRunner(t *testing.T) {
	runner, err := newWorkerRegistrationRunner(nil, nil)
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

	provider := newWorkerHistoricalPasswordProvider(&workerMainFakeAccountRepository{
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

	provider := newWorkerHistoricalPasswordProvider(&workerMainFakeAccountRepository{})
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

func TestNewWorkerTokenCompletionLeaseStoreClaimsRenewsAndReleasesLease(t *testing.T) {
	t.Parallel()

	backend := &workerMainFakeLeaseRedisBackend{
		values: make(map[string]string),
	}
	store := &workerTokenCompletionLeaseStore{backend: backend}
	key := workerTokenCompletionLeaseKey("native@example.com")

	claimed, err := store.Claim(context.Background(), "native@example.com", "lease-1", 30*time.Second)
	if err != nil {
		t.Fatalf("claim external lease: %v", err)
	}
	if !claimed {
		t.Fatal("expected lease claim to succeed")
	}
	if backend.values[key] != "lease-1" {
		t.Fatalf("expected claimed lease value, got %#v", backend.values)
	}

	active, err := store.IsActive(context.Background(), "native@example.com", "lease-1")
	if err != nil {
		t.Fatalf("check lease activity: %v", err)
	}
	if !active {
		t.Fatal("expected lease to be active for owner")
	}

	renewed, err := store.Renew(context.Background(), "native@example.com", "lease-1", 45*time.Second)
	if err != nil {
		t.Fatalf("renew external lease: %v", err)
	}
	if !renewed {
		t.Fatal("expected lease renew to succeed")
	}

	if err := store.Release(context.Background(), "native@example.com", "lease-1"); err != nil {
		t.Fatalf("release external lease: %v", err)
	}
	if _, ok := backend.values[key]; ok {
		t.Fatalf("expected released lease to be removed, got %#v", backend.values)
	}
}

func TestNewWorkerTokenCompletionLeaseStoreRejectsRenewForDifferentOwner(t *testing.T) {
	t.Parallel()

	backend := &workerMainFakeLeaseRedisBackend{
		values: map[string]string{
			workerTokenCompletionLeaseKey("native@example.com"): "lease-active",
		},
	}
	store := &workerTokenCompletionLeaseStore{backend: backend}

	renewed, err := store.Renew(context.Background(), "native@example.com", "lease-other", 45*time.Second)
	if err != nil {
		t.Fatalf("renew external lease: %v", err)
	}
	if renewed {
		t.Fatal("expected renew to fail for different owner")
	}

	active, err := store.IsActive(context.Background(), "native@example.com", "lease-other")
	if err != nil {
		t.Fatalf("check lease activity: %v", err)
	}
	if active {
		t.Fatal("expected different owner lease check to be inactive")
	}
}

func TestNewWorkerTokenCompletionCooldownProviderReadsCooldownFromExtraData(t *testing.T) {
	t.Parallel()

	provider := newWorkerTokenCompletionCooldownProvider(&workerMainFakeAccountRepository{
		account: accounts.Account{
			Email: "native@example.com",
			ExtraData: map[string]any{
				"refresh_token_cooldown_until": "2026-04-05T10:07:00Z",
			},
		},
		found: true,
	})
	if provider == nil {
		t.Fatal("expected cooldown provider")
	}

	cooldownUntil, err := provider.ResolveTokenCompletionCooldown(context.Background(), nativerunner.FlowRequest{}, "native@example.com")
	if err != nil {
		t.Fatalf("resolve token completion cooldown: %v", err)
	}
	if cooldownUntil == nil {
		t.Fatal("expected cooldown timestamp")
	}
	expected := time.Date(2026, time.April, 5, 10, 7, 0, 0, time.UTC)
	if !cooldownUntil.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, cooldownUntil)
	}
}

func TestNewWorkerTokenCompletionAttemptProviderReadsAttemptsFromExtraData(t *testing.T) {
	t.Parallel()

	provider := newWorkerTokenCompletionAttemptProvider(&workerMainFakeAccountRepository{
		account: accounts.Account{
			Email: "native@example.com",
			ExtraData: map[string]any{
				"token_completion_attempts": []map[string]any{
					{
						"email":        "native@example.com",
						"state":        "failed",
						"started_at":   "2026-04-05T10:00:00Z",
						"completed_at": "2026-04-05T10:00:05Z",
						"error": map[string]any{
							"kind":      "rate_limited",
							"message":   "rate limited",
							"retryable": true,
						},
					},
				},
			},
		},
		found: true,
	})

	attempts, err := provider.ResolveTokenCompletionAttempts(context.Background(), nativerunner.FlowRequest{}, "native@example.com")
	if err != nil {
		t.Fatalf("resolve token completion attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected one attempt, got %+v", attempts)
	}
	if attempts[0].State != nativerunner.TokenCompletionStateFailed || attempts[0].Error == nil || attempts[0].Error.Kind != nativerunner.TokenCompletionErrorKindRateLimited {
		t.Fatalf("unexpected parsed attempts: %+v", attempts)
	}
}

func TestNewWorkerTokenCompletionRuntimeStorePersistsRunningAttemptIntoAccountExtraData(t *testing.T) {
	t.Parallel()

	repo := &workerMainFakeAccountRepository{
		account: accounts.Account{
			Email:  "native@example.com",
			Status: accounts.DefaultAccountStatus,
			ExtraData: map[string]any{
				"existing_account_detected": true,
			},
		},
		found: true,
	}
	store := newWorkerTokenCompletionRuntimeStore(repo)

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	err := store.Save(context.Background(), "native@example.com", nativerunner.TokenCompletionRuntimeState{
		Attempts: []nativerunner.TokenCompletionAttempt{
			{
				Email:      "native@example.com",
				State:      nativerunner.TokenCompletionStateRunning,
				LeaseToken: "lease-running",
				StartedAt:  now,
			},
		},
	})
	if err != nil {
		t.Fatalf("save runtime state: %v", err)
	}

	if repo.savedAccount.Email != "native@example.com" {
		t.Fatalf("expected runtime store to persist account by email, got %+v", repo.savedAccount)
	}
	if repo.savedAccount.Status != accounts.DefaultAccountStatus {
		t.Fatalf("expected runtime store to preserve existing status, got %+v", repo.savedAccount)
	}
	if repo.savedAccount.ExtraData["existing_account_detected"] != true {
		t.Fatalf("expected runtime store to preserve extra data, got %#v", repo.savedAccount.ExtraData)
	}

	runtimeState, err := nativerunner.ParseTokenCompletionRuntimeState(repo.savedAccount.ExtraData, "native@example.com")
	if err != nil {
		t.Fatalf("parse persisted runtime state: %v", err)
	}
	if len(runtimeState.Attempts) != 1 || runtimeState.Attempts[0].State != nativerunner.TokenCompletionStateRunning {
		t.Fatalf("expected persisted running attempt, got %+v", runtimeState.Attempts)
	}
	if runtimeState.Attempts[0].LeaseToken != "lease-running" {
		t.Fatalf("expected persisted running lease token, got %+v", runtimeState.Attempts[0])
	}
}

func TestNewWorkerTokenCompletionRuntimeStoreExposesCompareAndSwap(t *testing.T) {
	t.Parallel()

	repo := &workerMainFakeAccountRepository{
		compareAndSwapResult: true,
	}
	store := newWorkerTokenCompletionRuntimeStore(repo)

	casStore, ok := store.(interface {
		CompareAndSwap(context.Context, string, nativerunner.TokenCompletionRuntimeState, nativerunner.TokenCompletionRuntimeState) (bool, error)
	})
	if !ok {
		t.Fatal("expected worker runtime store to expose compare-and-swap")
	}

	nextState := nativerunner.TokenCompletionRuntimeState{
		Attempts: []nativerunner.TokenCompletionAttempt{
			{
				Email:      "native@example.com",
				State:      nativerunner.TokenCompletionStateRunning,
				LeaseToken: "lease-next",
				StartedAt:  time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	swapped, err := casStore.CompareAndSwap(context.Background(), "native@example.com", nativerunner.TokenCompletionRuntimeState{}, nextState)
	if err != nil {
		t.Fatalf("compare and swap runtime state: %v", err)
	}
	if !swapped {
		t.Fatal("expected compare-and-swap to succeed")
	}
	if repo.compareAndSwapEmail != "native@example.com" {
		t.Fatalf("expected compare-and-swap email recorded, got %q", repo.compareAndSwapEmail)
	}
	runtimeState, err := nativerunner.ParseTokenCompletionRuntimeState(repo.compareAndSwapNextData, "native@example.com")
	if err != nil {
		t.Fatalf("parse compare-and-swap runtime state: %v", err)
	}
	if len(runtimeState.Attempts) != 1 || runtimeState.Attempts[0].State != nativerunner.TokenCompletionStateRunning {
		t.Fatalf("expected compare-and-swap runtime state forwarded, got %+v", runtimeState)
	}
	if runtimeState.Attempts[0].LeaseToken != "lease-next" {
		t.Fatalf("expected compare-and-swap to forward lease token, got %+v", runtimeState.Attempts[0])
	}
}

func TestNewWorkerTokenCompletionRuntimeStoreCompareAndSwapReturnsFenceConflictWithoutSave(t *testing.T) {
	t.Parallel()

	repo := &workerMainFakeAccountRepository{
		account: accounts.Account{
			Email: "native@example.com",
		},
		compareAndSwapResult: false,
	}
	store := newWorkerTokenCompletionRuntimeStore(repo)

	casStore, ok := store.(interface {
		CompareAndSwap(context.Context, string, nativerunner.TokenCompletionRuntimeState, nativerunner.TokenCompletionRuntimeState) (bool, error)
	})
	if !ok {
		t.Fatal("expected worker runtime store to expose compare-and-swap")
	}

	swapped, err := casStore.CompareAndSwap(context.Background(), "native@example.com", nativerunner.TokenCompletionRuntimeState{
		Attempts: []nativerunner.TokenCompletionAttempt{
			{
				Email:      "native@example.com",
				State:      nativerunner.TokenCompletionStateRunning,
				LeaseToken: "lease-stale",
			},
		},
	}, nativerunner.TokenCompletionRuntimeState{
		Attempts: []nativerunner.TokenCompletionAttempt{
			{
				Email:      "native@example.com",
				State:      nativerunner.TokenCompletionStateCompleted,
				LeaseToken: "lease-stale",
			},
		},
	})
	if err != nil {
		t.Fatalf("compare and swap runtime state: %v", err)
	}
	if swapped {
		t.Fatal("expected compare-and-swap fence conflict")
	}
	if repo.savedAccount.Email != "" || len(repo.savedAccount.ExtraData) != 0 || repo.savedAccount.Status != "" || repo.savedAccount.Source != "" {
		t.Fatalf("expected fence conflict not to fall back to save, got %+v", repo.savedAccount)
	}
}

type workerMainFakeRunner struct {
	runFn func(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error)
}

func (f workerMainFakeRunner) Run(ctx context.Context, req registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error) {
	if f.runFn != nil {
		return f.runFn(ctx, req, logf)
	}
	return registration.RunnerOutput{Result: map[string]any{"ok": true}}, nil
}

type workerMainFailingPreparationSettings struct {
	err error
}

func (f workerMainFailingPreparationSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	return nil, f.err
}

type workerMainFakeAccountRepository struct {
	account      accounts.Account
	found        bool
	err          error
	savedAccount accounts.Account

	compareAndSwapEmail       string
	compareAndSwapCurrentData map[string]any
	compareAndSwapNextData    map[string]any
	compareAndSwapResult      bool
	compareAndSwapErr         error
}

type workerMainFakeLeaseRedisBackend struct {
	values map[string]string
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

func (f *workerMainFakeAccountRepository) UpsertAccount(_ context.Context, account accounts.Account) (accounts.Account, error) {
	f.savedAccount = account
	if f.err != nil {
		return accounts.Account{}, f.err
	}
	if account.Email == "" && f.account.Email != "" {
		account.Email = f.account.Email
	}
	return account, nil
}

func (f *workerMainFakeAccountRepository) CompareAndSwapTokenCompletionRuntime(_ context.Context, email string, currentExtraData map[string]any, nextExtraData map[string]any, _ accounts.Account) (accounts.Account, bool, error) {
	f.compareAndSwapEmail = email
	f.compareAndSwapCurrentData = currentExtraData
	f.compareAndSwapNextData = nextExtraData
	if f.compareAndSwapErr != nil {
		return accounts.Account{}, false, f.compareAndSwapErr
	}
	return f.account, f.compareAndSwapResult, nil
}

func (f *workerMainFakeLeaseRedisBackend) SetNX(_ context.Context, key string, value string, _ time.Duration) (bool, error) {
	if _, exists := f.values[key]; exists {
		return false, nil
	}
	f.values[key] = value
	return true, nil
}

func (f *workerMainFakeLeaseRedisBackend) Get(_ context.Context, key string) (string, error) {
	value, ok := f.values[key]
	if !ok {
		return "", redisv9.Nil
	}
	return value, nil
}

func (f *workerMainFakeLeaseRedisBackend) Eval(_ context.Context, script string, keys []string, args ...any) (any, error) {
	if len(keys) == 0 {
		return int64(0), nil
	}
	key := keys[0]
	leaseToken := strings.TrimSpace(args[0].(string))
	currentValue, ok := f.values[key]
	if !ok || currentValue != leaseToken {
		return int64(0), nil
	}

	switch script {
	case workerTokenCompletionLeaseRenewScript:
		return int64(1), nil
	case workerTokenCompletionLeaseReleaseScript:
		delete(f.values, key)
		return int64(1), nil
	default:
		return int64(0), nil
	}
}
