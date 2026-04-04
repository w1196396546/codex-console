package registration

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
)

func TestAutoUploadDispatcherDispatchesEnabledKindsAndLogsResults(t *testing.T) {
	repo := &fakeAutoUploadConfigRepository{
		cpaConfigs: []uploader.ServiceConfig{
			{ID: 11, Kind: uploader.UploadKindCPA, Name: "CPA One", BaseURL: "https://cpa.example.com", Credential: "cpa-token"},
		},
		sub2apiConfigs: []uploader.ServiceConfig{
			{ID: 22, Kind: uploader.UploadKindSub2API, Name: "Sub2API One", BaseURL: "https://sub2api.example.com", Credential: "sub2api-key"},
		},
		tmConfigs: []uploader.ServiceConfig{
			{ID: 33, Kind: uploader.UploadKindTM, Name: "TM One", BaseURL: "https://tm.example.com", Credential: "tm-key"},
		},
	}
	senders := map[uploader.UploadKind]*fakeUploadSender{
		uploader.UploadKindCPA: {
			results: []uploader.UploadResult{{Kind: uploader.UploadKindCPA, ServiceID: 11, AccountEmail: "alice@example.com", Success: true, Message: "cpa ok"}},
		},
		uploader.UploadKindSub2API: {
			results: []uploader.UploadResult{{Kind: uploader.UploadKindSub2API, ServiceID: 22, AccountEmail: "alice@example.com", Success: true, Message: "sub2api ok"}},
		},
		uploader.UploadKindTM: {
			results: []uploader.UploadResult{{Kind: uploader.UploadKindTM, ServiceID: 33, AccountEmail: "alice@example.com", Success: false, Message: "tm failed"}},
		},
	}

	dispatcher := newAutoUploadDispatcher(repo, func(kind uploader.UploadKind) (uploader.Sender, error) {
		return senders[kind], nil
	})

	var logs []string
	result, err := dispatcher.Dispatch(context.Background(), AutoUploadDispatchRequest{
		JobID: "job-1",
		StartRequest: StartRequest{
			AutoUploadCPA:      true,
			CPAServiceIDs:      []int{11},
			AutoUploadSub2API:  true,
			Sub2APIServiceIDs:  []int{22},
			AutoUploadTM:       true,
			TMServiceIDs:       []int{33},
			EmailServiceType:   "outlook",
			EmailServiceConfig: map[string]any{"name": "ignored"},
		},
		Account: accounts.Account{
			Email:        "alice@example.com",
			AccessToken:  "access-1",
			RefreshToken: "refresh-1",
			SessionToken: "session-1",
			ClientID:     "client-1",
			AccountID:    "account-1",
			WorkspaceID:  "workspace-1",
			IDToken:      "id-token-1",
		},
	}, func(level string, message string) error {
		logs = append(logs, level+":"+message)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if result.AccountUpdate.Email != "alice@example.com" {
		t.Fatalf("expected account update email, got %+v", result.AccountUpdate)
	}
	if result.AccountUpdate.CPAUploaded == nil || !*result.AccountUpdate.CPAUploaded {
		t.Fatalf("expected CPA uploaded flag to be written, got %+v", result.AccountUpdate)
	}
	if result.AccountUpdate.CPAUploadedAt == nil {
		t.Fatalf("expected CPA uploaded timestamp to be written, got %+v", result.AccountUpdate)
	}
	if result.AccountUpdate.Sub2APIUploaded == nil || !*result.AccountUpdate.Sub2APIUploaded {
		t.Fatalf("expected Sub2API uploaded flag to be written, got %+v", result.AccountUpdate)
	}
	if result.AccountUpdate.Sub2APIUploadedAt == nil {
		t.Fatalf("expected Sub2API uploaded timestamp to be written, got %+v", result.AccountUpdate)
	}

	if !reflect.DeepEqual(repo.cpaIDs, []int{11}) {
		t.Fatalf("expected CPA service ids [11], got %#v", repo.cpaIDs)
	}
	if !reflect.DeepEqual(repo.sub2apiIDs, []int{22}) {
		t.Fatalf("expected Sub2API service ids [22], got %#v", repo.sub2apiIDs)
	}
	if !reflect.DeepEqual(repo.tmIDs, []int{33}) {
		t.Fatalf("expected TM service ids [33], got %#v", repo.tmIDs)
	}

	for kind, sender := range senders {
		if len(sender.requests) != 1 {
			t.Fatalf("expected one %s send request, got %#v", kind, sender.requests)
		}
		req := sender.requests[0]
		if len(req.Accounts) != 1 || req.Accounts[0].Email != "alice@example.com" {
			t.Fatalf("expected %s request account to propagate, got %+v", kind, req)
		}
	}

	joinedLogs := strings.Join(logs, "\n")
	if !strings.Contains(joinedLogs, "[CPA] 成功(CPA One): cpa ok") {
		t.Fatalf("expected CPA success log, got %v", logs)
	}
	if !strings.Contains(joinedLogs, "[Sub2API] 成功(Sub2API One): sub2api ok") {
		t.Fatalf("expected Sub2API success log, got %v", logs)
	}
	if !strings.Contains(joinedLogs, "[TM] 失败(TM One): tm failed") {
		t.Fatalf("expected TM failure log, got %v", logs)
	}
}

func TestAutoUploadDispatcherDoesNotWriteSuccessMarkersForFailedOrTMUploads(t *testing.T) {
	repo := &fakeAutoUploadConfigRepository{
		cpaConfigs: []uploader.ServiceConfig{
			{ID: 11, Kind: uploader.UploadKindCPA, Name: "CPA One", BaseURL: "https://cpa.example.com", Credential: "cpa-token"},
		},
		tmConfigs: []uploader.ServiceConfig{
			{ID: 33, Kind: uploader.UploadKindTM, Name: "TM One", BaseURL: "https://tm.example.com", Credential: "tm-key"},
		},
	}
	senders := map[uploader.UploadKind]*fakeUploadSender{
		uploader.UploadKindCPA: {
			results: []uploader.UploadResult{{Kind: uploader.UploadKindCPA, ServiceID: 11, AccountEmail: "alice@example.com", Success: false, Message: "cpa failed"}},
		},
		uploader.UploadKindTM: {
			results: []uploader.UploadResult{{Kind: uploader.UploadKindTM, ServiceID: 33, AccountEmail: "alice@example.com", Success: true, Message: "tm ok"}},
		},
	}

	dispatcher := newAutoUploadDispatcher(repo, func(kind uploader.UploadKind) (uploader.Sender, error) {
		return senders[kind], nil
	})

	result, err := dispatcher.Dispatch(context.Background(), AutoUploadDispatchRequest{
		JobID: "job-2",
		StartRequest: StartRequest{
			AutoUploadCPA: true,
			CPAServiceIDs: []int{11},
			AutoUploadTM:  true,
			TMServiceIDs:  []int{33},
		},
		Account: accounts.Account{
			Email:       "alice@example.com",
			AccessToken: "access-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}

	if result.AccountUpdate.Email != "" {
		t.Fatalf("expected no account update when CPA fails and only TM succeeds, got %+v", result.AccountUpdate)
	}
}

func TestAutoUploadDispatcherUsesFreshTimestampForSuccessfulWriteback(t *testing.T) {
	repo := &fakeAutoUploadConfigRepository{
		sub2apiConfigs: []uploader.ServiceConfig{
			{ID: 22, Kind: uploader.UploadKindSub2API, Name: "Sub2API One", BaseURL: "https://sub2api.example.com", Credential: "sub2api-key"},
		},
	}
	dispatcher := newAutoUploadDispatcher(repo, func(kind uploader.UploadKind) (uploader.Sender, error) {
		return &fakeUploadSender{
			results: []uploader.UploadResult{{Kind: kind, ServiceID: 22, AccountEmail: "alice@example.com", Success: true, Message: "ok"}},
		}, nil
	})

	before := time.Now().UTC()
	result, err := dispatcher.Dispatch(context.Background(), AutoUploadDispatchRequest{
		JobID: "job-3",
		StartRequest: StartRequest{
			AutoUploadSub2API: true,
			Sub2APIServiceIDs: []int{22},
		},
		Account: accounts.Account{
			Email:       "alice@example.com",
			AccessToken: "access-1",
		},
	}, nil)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if result.AccountUpdate.Sub2APIUploadedAt == nil {
		t.Fatalf("expected sub2api uploaded timestamp, got %+v", result.AccountUpdate)
	}
	if result.AccountUpdate.Sub2APIUploadedAt.Before(before) || result.AccountUpdate.Sub2APIUploadedAt.After(after) {
		t.Fatalf("expected timestamp between %v and %v, got %v", before, after, result.AccountUpdate.Sub2APIUploadedAt)
	}
}

type fakeAutoUploadConfigRepository struct {
	cpaIDs         []int
	sub2apiIDs     []int
	tmIDs          []int
	cpaConfigs     []uploader.ServiceConfig
	sub2apiConfigs []uploader.ServiceConfig
	tmConfigs      []uploader.ServiceConfig
}

func (f *fakeAutoUploadConfigRepository) ListCPAServiceConfigs(_ context.Context, ids []int) ([]uploader.ServiceConfig, error) {
	f.cpaIDs = append([]int(nil), ids...)
	return append([]uploader.ServiceConfig(nil), f.cpaConfigs...), nil
}

func (f *fakeAutoUploadConfigRepository) ListSub2APIServiceConfigs(_ context.Context, ids []int) ([]uploader.ServiceConfig, error) {
	f.sub2apiIDs = append([]int(nil), ids...)
	return append([]uploader.ServiceConfig(nil), f.sub2apiConfigs...), nil
}

func (f *fakeAutoUploadConfigRepository) ListTMServiceConfigs(_ context.Context, ids []int) ([]uploader.ServiceConfig, error) {
	f.tmIDs = append([]int(nil), ids...)
	return append([]uploader.ServiceConfig(nil), f.tmConfigs...), nil
}

type fakeUploadSender struct {
	requests []uploader.SendRequest
	results  []uploader.UploadResult
}

func (f *fakeUploadSender) Kind() uploader.UploadKind {
	if len(f.results) == 0 {
		return ""
	}
	return f.results[0].Kind
}

func (f *fakeUploadSender) Send(_ context.Context, req uploader.SendRequest) ([]uploader.UploadResult, error) {
	f.requests = append(f.requests, req)
	return append([]uploader.UploadResult(nil), f.results...), nil
}
