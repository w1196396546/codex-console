package registration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestExecutorPersistsAccountPayloadAndStripsInternalResult(t *testing.T) {
	upserter := &fakeAccountUpserter{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{
				Result: map[string]any{
					"email":   "alice@example.com",
					"success": true,
				},
				AccountPersistence: &accounts.UpsertAccountRequest{
					Email:          "alice@example.com",
					Password:       "secret",
					ClientID:       "client-1",
					SessionToken:   "session-1",
					EmailService:   "outlook",
					EmailServiceID: "42",
					AccountID:      "account-1",
					WorkspaceID:    "workspace-1",
					AccessToken:    "access-1",
					RefreshToken:   "refresh-1",
					IDToken:        "id-1",
					ProxyUsed:      "http://proxy.internal:8080",
					Status:         "active",
					Source:         "register",
					ExtraData:      map[string]any{"device_id": "device-1"},
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-persist",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook"}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if result["email"] != "alice@example.com" || result["success"] != true {
		t.Fatalf("expected user-facing result to stay intact, got %#v", result)
	}

	if len(upserter.requests) != 1 {
		t.Fatalf("expected one upsert request, got %#v", upserter.requests)
	}

	got := upserter.requests[0]
	if got.Email != "alice@example.com" || got.EmailService != "outlook" {
		t.Fatalf("unexpected persisted identity fields: %+v", got)
	}
	if got.AccessToken != "access-1" || got.RefreshToken != "refresh-1" || got.IDToken != "id-1" {
		t.Fatalf("unexpected persisted token fields: %+v", got)
	}
	if got.SessionToken != "session-1" || got.ClientID != "client-1" {
		t.Fatalf("unexpected persisted oauth fields: %+v", got)
	}
	if got.ProxyUsed != "http://proxy.internal:8080" || got.Status != "active" || got.Source != "register" {
		t.Fatalf("unexpected persisted metadata fields: %+v", got)
	}
	if !reflect.DeepEqual(got.ExtraData, map[string]any{"device_id": "device-1"}) {
		t.Fatalf("unexpected persisted extra_data: %#v", got.ExtraData)
	}
}

func TestExecutorSkipsAccountPersistenceWhenPayloadMissing(t *testing.T) {
	upserter := &fakeAccountUpserter{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{Result: map[string]any{
				"email":   "bob@example.com",
				"success": true,
			}}, nil
		}),
		WithAccountPersistence(upserter),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-no-persist",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if result["email"] != "bob@example.com" {
		t.Fatalf("expected original result to pass through, got %#v", result)
	}
	if len(upserter.requests) != 0 {
		t.Fatalf("expected no upsert requests when payload missing, got %#v", upserter.requests)
	}
}

func TestExecutorTriggersAutoUploadAfterPersistedAccountSaved(t *testing.T) {
	upserter := &fakeAccountUpserter{
		result: accounts.Account{
			ID:           1001,
			Email:        "alice@example.com",
			AccessToken:  "persisted-access",
			RefreshToken: "persisted-refresh",
			SessionToken: "persisted-session",
			ClientID:     "persisted-client",
			AccountID:    "persisted-account",
			WorkspaceID:  "persisted-workspace",
			IDToken:      "persisted-id-token",
		},
	}
	dispatcher := &fakeAutoUploadDispatcher{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{
				Result: map[string]any{
					"email":   "alice@example.com",
					"success": true,
				},
				AccountPersistence: &accounts.UpsertAccountRequest{
					Email:          "alice@example.com",
					EmailService:   "outlook",
					AccessToken:    "raw-access",
					RefreshToken:   "raw-refresh",
					SessionToken:   "raw-session",
					ClientID:       "raw-client",
					AccountID:      "raw-account",
					WorkspaceID:    "raw-workspace",
					IDToken:        "raw-id-token",
					EmailServiceID: "42",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-auto-upload",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook","auto_upload_cpa":true,"cpa_service_ids":[11],"auto_upload_sub2api":true,"sub2api_service_ids":[22],"auto_upload_tm":true,"tm_service_ids":[33]}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if len(dispatcher.requests) != 1 {
		t.Fatalf("expected one auto upload dispatch, got %#v", dispatcher.requests)
	}

	got := dispatcher.requests[0]
	if got.JobID != "job-auto-upload" {
		t.Fatalf("expected job id to propagate, got %+v", got)
	}
	if !got.StartRequest.AutoUploadCPA || !got.StartRequest.AutoUploadSub2API || !got.StartRequest.AutoUploadTM {
		t.Fatalf("expected auto upload flags to propagate, got %+v", got.StartRequest)
	}
	if !reflect.DeepEqual(got.StartRequest.CPAServiceIDs, []int{11}) ||
		!reflect.DeepEqual(got.StartRequest.Sub2APIServiceIDs, []int{22}) ||
		!reflect.DeepEqual(got.StartRequest.TMServiceIDs, []int{33}) {
		t.Fatalf("expected service ids to propagate, got %+v", got.StartRequest)
	}
	if got.Account.AccessToken != "persisted-access" || got.Account.RefreshToken != "persisted-refresh" {
		t.Fatalf("expected persisted account to be used for auto upload, got %+v", got.Account)
	}
	if got.Account.AccountID != "persisted-account" || got.Account.WorkspaceID != "persisted-workspace" {
		t.Fatalf("expected persisted account identity to be used, got %+v", got.Account)
	}
}

func TestExecutorPersistsAutoUploadWritebackWhenDispatcherReturnsSuccessState(t *testing.T) {
	uploadedAt := time.Date(2026, 4, 4, 8, 30, 0, 0, time.UTC)
	upserter := &fakeAccountUpserter{
		result: accounts.Account{
			ID:           1002,
			Email:        "alice@example.com",
			EmailService: "outlook",
			AccessToken:  "persisted-access",
			Status:       "active",
			Source:       "register",
		},
	}
	dispatcher := &fakeAutoUploadDispatcher{
		result: AutoUploadDispatchResult{
			AccountUpdate: accounts.UpsertAccountRequest{
				Email:             "alice@example.com",
				EmailService:      "outlook",
				AccessToken:       "persisted-access",
				Status:            "active",
				Source:            "register",
				CPAUploaded:       boolPtr(true),
				CPAUploadedAt:     &uploadedAt,
				Sub2APIUploaded:   boolPtr(true),
				Sub2APIUploadedAt: &uploadedAt,
			},
		},
	}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{
				Result: map[string]any{
					"email":   "alice@example.com",
					"success": true,
				},
				AccountPersistence: &accounts.UpsertAccountRequest{
					Email:        "alice@example.com",
					EmailService: "outlook",
					AccessToken:  "persisted-access",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-auto-upload-writeback",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook","auto_upload_cpa":true,"cpa_service_ids":[11],"auto_upload_sub2api":true,"sub2api_service_ids":[22]}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if len(upserter.requests) != 2 {
		t.Fatalf("expected initial account persistence plus upload writeback, got %#v", upserter.requests)
	}

	writeback := upserter.requests[1]
	if writeback.CPAUploaded == nil || !*writeback.CPAUploaded {
		t.Fatalf("expected CPA writeback flag, got %+v", writeback)
	}
	if writeback.CPAUploadedAt == nil || !writeback.CPAUploadedAt.Equal(uploadedAt) {
		t.Fatalf("expected CPA writeback timestamp %v, got %+v", uploadedAt, writeback)
	}
	if writeback.Sub2APIUploaded == nil || !*writeback.Sub2APIUploaded {
		t.Fatalf("expected Sub2API writeback flag, got %+v", writeback)
	}
	if writeback.Sub2APIUploadedAt == nil || !writeback.Sub2APIUploadedAt.Equal(uploadedAt) {
		t.Fatalf("expected Sub2API writeback timestamp %v, got %+v", uploadedAt, writeback)
	}
}

func TestExecutorSkipsAutoUploadWhenPersistencePayloadMissing(t *testing.T) {
	upserter := &fakeAccountUpserter{}
	dispatcher := &fakeAutoUploadDispatcher{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{Result: map[string]any{
				"email":   "bob@example.com",
				"success": true,
			}}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-skip-auto-upload",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook","auto_upload_cpa":true,"cpa_service_ids":[11]}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if len(dispatcher.requests) != 0 {
		t.Fatalf("expected no auto upload dispatch without persistence payload, got %#v", dispatcher.requests)
	}
}

func TestExecutorDoesNotTriggerAutoUploadWhenAccountPersistenceFails(t *testing.T) {
	upserter := &fakeAccountUpserter{err: errFakeUpsertFailed}
	dispatcher := &fakeAutoUploadDispatcher{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{
				Result: map[string]any{
					"email":   "carol@example.com",
					"success": true,
				},
				AccountPersistence: &accounts.UpsertAccountRequest{
					Email:        "carol@example.com",
					EmailService: "outlook",
					AccessToken:  "access-3",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-upsert-fail",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook","auto_upload_tm":true,"tm_service_ids":[33]}`),
	})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if len(dispatcher.requests) != 0 {
		t.Fatalf("expected no auto upload dispatch when persistence fails, got %#v", dispatcher.requests)
	}
}

func TestExecutorPersistsAccountPayloadEvenWhenRunnerReturnsTypedError(t *testing.T) {
	upserter := &fakeAccountUpserter{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{}, &RunnerError{
				Err: errors.New("token completion blocked"),
				Output: RunnerOutput{
					AccountPersistence: &accounts.UpsertAccountRequest{
						Email:        "cooldown@example.com",
						EmailService: "tempmail",
						Status:       "token_pending",
						Source:       "login",
						RefreshToken: "",
						AccessToken:  "",
						AccountID:    "account-cooldown",
						WorkspaceID:  "workspace-cooldown",
						ExtraData: map[string]any{
							"refresh_token_cooldown_until": "2026-04-05T10:07:00Z",
							"token_completion_attempts": []map[string]any{
								{"state": "failed"},
							},
						},
					},
				},
			}
		}),
		WithAccountPersistence(upserter),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	_, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-persist-on-error",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"tempmail"}`),
	})
	if err == nil {
		t.Fatal("expected runner error")
	}
	if len(upserter.requests) != 1 {
		t.Fatalf("expected one persistence write even on runner error, got %#v", upserter.requests)
	}
	if upserter.requests[0].Email != "cooldown@example.com" || upserter.requests[0].Status != "token_pending" {
		t.Fatalf("unexpected persisted request on runner error: %+v", upserter.requests[0])
	}
}

func TestExecutorCompletesJobWhenErrorCarriesRetryableTokenPendingPersistence(t *testing.T) {
	upserter := &fakeAccountUpserter{}
	executor := NewExecutor(
		&executorAdmissionLogSink{},
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
			return RunnerOutput{}, &fakeAccountPersistenceCarrierError{
				err: errors.New("token completion blocked"),
				req: &accounts.UpsertAccountRequest{
					Email:        "pending@example.com",
					EmailService: "outlook",
					Status:       "token_pending",
					Source:       "login",
					AccountID:    "account-pending",
					WorkspaceID:  "workspace-pending",
					ExtraData: map[string]any{
						"account_status_reason": "email_conflict",
					},
				},
			}
		}),
		WithAccountPersistence(upserter),
		WithPreparationDependencies(executorPreparationDependencies()),
	)

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-soft-complete-on-token-pending",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook"}`),
	})
	if err != nil {
		t.Fatalf("expected token_pending carrier to complete without error, got %v", err)
	}
	if len(upserter.requests) != 1 {
		t.Fatalf("expected one persistence write for token_pending carrier, got %#v", upserter.requests)
	}
	if upserter.requests[0].Email != "pending@example.com" || upserter.requests[0].Status != "token_pending" {
		t.Fatalf("unexpected persisted request for token_pending carrier: %+v", upserter.requests[0])
	}
	if result["email"] != "pending@example.com" {
		t.Fatalf("expected soft-completed result to expose email, got %#v", result)
	}
	if result["status"] != "token_pending" {
		t.Fatalf("expected soft-completed result to expose token_pending status, got %#v", result)
	}
	if result["reason"] != "email_conflict" {
		t.Fatalf("expected soft-completed result to expose reason, got %#v", result)
	}
}

type fakeAccountUpserter struct {
	requests []accounts.UpsertAccountRequest
	result   accounts.Account
	err      error
}

func (f *fakeAccountUpserter) UpsertAccount(_ context.Context, req accounts.UpsertAccountRequest) (accounts.Account, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return accounts.Account{}, f.err
	}
	if f.result.Email != "" {
		return f.result, nil
	}
	return accounts.Account{Email: req.Email}, nil
}

type fakeAutoUploadDispatcher struct {
	requests []AutoUploadDispatchRequest
	result   AutoUploadDispatchResult
}

func (f *fakeAutoUploadDispatcher) Dispatch(_ context.Context, req AutoUploadDispatchRequest, _ func(level string, message string) error) (AutoUploadDispatchResult, error) {
	f.requests = append(f.requests, req)
	return f.result, nil
}

var errFakeUpsertFailed = accounts.ErrRepositoryNotConfigured

func boolPtr(value bool) *bool {
	return &value
}

type fakeAccountPersistenceCarrierError struct {
	err error
	req *accounts.UpsertAccountRequest
}

func (e *fakeAccountPersistenceCarrierError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *fakeAccountPersistenceCarrierError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *fakeAccountPersistenceCarrierError) AccountPersistenceRequest() *accounts.UpsertAccountRequest {
	if e == nil {
		return nil
	}
	return e.req
}
