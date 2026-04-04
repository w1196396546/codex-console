package registration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/hibiken/asynq"
)

func TestNativeRunnerAdapterAllowsExecutorToReturnUserResultAndPersistAccount(t *testing.T) {
	logs := &executorAdmissionLogSink{}
	upserter := &fakeAccountUpserter{}
	executor := NewExecutor(
		logs,
		NewNativeRunner(nativeRunnerStub{
			runFn: func(_ context.Context, req RunnerRequest, logf func(level string, message string) error) (NativeRunnerResult, error) {
				if !req.GoPersistenceEnabled {
					t.Fatalf("expected go persistence flag for native runner, got %+v", req)
				}
				if err := logf("info", "native runner started"); err != nil {
					return NativeRunnerResult{}, err
				}
				return NativeRunnerResult{
					Result: map[string]any{
						"email":   "native@example.com",
						"success": true,
					},
					AccountPersistence: &accounts.UpsertAccountRequest{
						Email:        "native@example.com",
						EmailService: "tempmail",
						AccessToken:  "native-access",
						Status:       "active",
					},
				}, nil
			},
		}),
		WithAccountPersistence(upserter),
	)

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-native-executor",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if got := result["email"]; got != "native@example.com" {
		t.Fatalf("expected native result email, got %#v", result)
	}
	if got := result["success"]; got != true {
		t.Fatalf("expected native success flag, got %#v", result)
	}
	if _, exists := result[runnerAccountPersistenceResultKey]; exists {
		t.Fatalf("expected native adapter to keep persistence payload internal, got %#v", result)
	}
	if len(upserter.requests) != 1 {
		t.Fatalf("expected one persisted account, got %#v", upserter.requests)
	}
	if upserter.requests[0].AccessToken != "native-access" {
		t.Fatalf("expected persisted native access token, got %+v", upserter.requests[0])
	}
	if len(logs.entries) != 1 || logs.entries[0].message != "native runner started" {
		t.Fatalf("expected native runner log forwarding, got %#v", logs.entries)
	}
}

func TestNativeRunnerAdapterWorksThroughWorkerAndStoresCompletedResult(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	service := jobs.NewService(repo, nil)
	created, err := service.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   JobTypeSingle,
		ScopeType: "registration_task",
		ScopeID:   "task-native-worker",
		Payload:   []byte(`{"email_service_type":"tempmail"}`),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	upserter := &fakeAccountUpserter{}
	worker := jobs.NewWorkerWithIDAndExecutor(
		service,
		"worker-native",
		NewExecutor(
			service,
			NewNativeRunner(nativeRunnerStub{
				runFn: func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (NativeRunnerResult, error) {
					return NativeRunnerResult{
						Result: map[string]any{
							"email":   "worker-native@example.com",
							"success": true,
						},
						AccountPersistence: &accounts.UpsertAccountRequest{
							Email:        "worker-native@example.com",
							EmailService: req.StartRequest.EmailServiceType,
							AccessToken:  "worker-native-access",
						},
					}, nil
				},
			}),
			WithAccountPersistence(upserter),
		),
	)

	taskPayload, err := jobs.MarshalQueuePayload(created.JobID)
	if err != nil {
		t.Fatalf("marshal queue payload: %v", err)
	}
	task := asynq.NewTask(jobs.TypeGenericJob, taskPayload)

	if err := worker.HandleTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	stored, err := service.GetJob(context.Background(), created.JobID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if stored.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed job, got %+v", stored)
	}

	var storedResult map[string]any
	if err := json.Unmarshal(stored.Result, &storedResult); err != nil {
		t.Fatalf("decode stored result: %v", err)
	}
	if storedResult["email"] != "worker-native@example.com" || storedResult["success"] != true {
		t.Fatalf("expected completed worker result from native runner, got %#v", storedResult)
	}
	if _, exists := storedResult[runnerAccountPersistenceResultKey]; exists {
		t.Fatalf("expected completed result to omit internal persistence payload, got %#v", storedResult)
	}
	if len(upserter.requests) != 1 {
		t.Fatalf("expected worker path to persist native account once, got %#v", upserter.requests)
	}
}

type nativeRunnerStub struct {
	runFn func(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (NativeRunnerResult, error)
}

func (s nativeRunnerStub) RunNative(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (NativeRunnerResult, error) {
	if s.runFn != nil {
		return s.runFn(ctx, req, logf)
	}
	return NativeRunnerResult{}, nil
}
