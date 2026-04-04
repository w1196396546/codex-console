package registration

import (
	"context"
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "alice@example.com",
				"success": true,
				runnerAccountPersistenceResultKey: map[string]any{
					"email":            "alice@example.com",
					"password":         "secret",
					"client_id":        "client-1",
					"session_token":    "session-1",
					"email_service":    "outlook",
					"email_service_id": "42",
					"account_id":       "account-1",
					"workspace_id":     "workspace-1",
					"access_token":     "access-1",
					"refresh_token":    "refresh-1",
					"id_token":         "id-1",
					"proxy_used":       "http://proxy.internal:8080",
					"status":           "active",
					"source":           "register",
					"extra_data": map[string]any{
						"device_id": "device-1",
					},
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
	)

	result, err := executor.Execute(context.Background(), jobs.Job{
		JobID:   "job-persist",
		JobType: JobTypeSingle,
		Payload: []byte(`{"email_service_type":"outlook"}`),
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if _, exists := result[runnerAccountPersistenceResultKey]; exists {
		t.Fatalf("expected internal persistence payload to be stripped from result, got %#v", result)
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "bob@example.com",
				"success": true,
			}, nil
		}),
		WithAccountPersistence(upserter),
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "alice@example.com",
				"success": true,
				runnerAccountPersistenceResultKey: map[string]any{
					"email":            "alice@example.com",
					"email_service":    "outlook",
					"access_token":     "raw-access",
					"refresh_token":    "raw-refresh",
					"session_token":    "raw-session",
					"client_id":        "raw-client",
					"account_id":       "raw-account",
					"workspace_id":     "raw-workspace",
					"id_token":         "raw-id-token",
					"email_service_id": "42",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "alice@example.com",
				"success": true,
				runnerAccountPersistenceResultKey: map[string]any{
					"email":         "alice@example.com",
					"email_service": "outlook",
					"access_token":  "persisted-access",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "bob@example.com",
				"success": true,
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
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
		admissionTestRunner(func(_ context.Context, _ RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
			return map[string]any{
				"email":   "carol@example.com",
				"success": true,
				runnerAccountPersistenceResultKey: map[string]any{
					"email":         "carol@example.com",
					"email_service": "outlook",
					"access_token":  "access-3",
				},
			}, nil
		}),
		WithAccountPersistence(upserter),
		WithAutoUploadDispatcher(dispatcher),
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
