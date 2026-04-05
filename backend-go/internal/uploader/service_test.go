package uploader

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestUploadServiceListAndFullResponsesPreserveCompatibilityFields(t *testing.T) {
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeUploadAdminRepository{
		listConfigs: []ManagedServiceConfig{
			{
				ServiceConfig: ServiceConfig{
					ID:         11,
					Kind:       UploadKindCPA,
					Name:       "CPA One",
					BaseURL:    "https://cpa.example.com",
					Credential: "cpa-token",
					ProxyURL:   "http://proxy.internal",
					Enabled:    true,
					Priority:   5,
				},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
		},
		getConfig: ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         22,
				Kind:       UploadKindSub2API,
				Name:       "Sub2API One",
				BaseURL:    "https://sub2api.example.com",
				Credential: "sub2api-key",
				TargetType: "",
				Enabled:    true,
				Priority:   9,
			},
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}
	svc := NewService(repo)

	cpaList, err := svc.ListCPAServices(context.Background(), boolPointer(true))
	if err != nil {
		t.Fatalf("list cpa services: %v", err)
	}
	if len(cpaList) != 1 {
		t.Fatalf("expected one cpa list item, got %d", len(cpaList))
	}
	if !cpaList[0].HasToken || cpaList[0].ProxyURL != "http://proxy.internal" {
		t.Fatalf("expected cpa list response to expose has_token/proxy_url only, got %+v", cpaList[0])
	}

	sub2apiResp, err := svc.GetSub2APIService(context.Background(), 22)
	if err != nil {
		t.Fatalf("get sub2api service: %v", err)
	}
	if !sub2apiResp.HasKey || sub2apiResp.APIURL != "https://sub2api.example.com" {
		t.Fatalf("unexpected sub2api summary response: %+v", sub2apiResp)
	}

	sub2apiFull, err := svc.GetSub2APIServiceFull(context.Background(), 22)
	if err != nil {
		t.Fatalf("get sub2api full response: %v", err)
	}
	if sub2apiFull.APIKey != "sub2api-key" {
		t.Fatalf("expected full response to include api_key, got %+v", sub2apiFull)
	}
	if sub2apiFull.TargetType != DefaultSub2APITargetType {
		t.Fatalf("expected empty target_type to normalize to %q, got %q", DefaultSub2APITargetType, sub2apiFull.TargetType)
	}

	if repo.lastListKind != UploadKindCPA || repo.lastListFilter == nil || repo.lastListFilter.Enabled == nil || !*repo.lastListFilter.Enabled {
		t.Fatalf("expected cpa list request to preserve enabled filter, got kind=%q filter=%+v", repo.lastListKind, repo.lastListFilter)
	}
	if repo.lastGetKind != UploadKindSub2API || repo.lastGetID != 22 {
		t.Fatalf("expected get calls to use sub2api id=22, got kind=%q id=%d", repo.lastGetKind, repo.lastGetID)
	}
}

func TestUploadServiceCreateUpdateAndDeletePreserveCurrentCredentialRules(t *testing.T) {
	repo := &fakeUploadAdminRepository{
		getConfig: ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         33,
				Kind:       UploadKindTM,
				Name:       "TM One",
				BaseURL:    "https://tm.example.com",
				Credential: "tm-key-old",
				Enabled:    true,
				Priority:   1,
			},
		},
		createConfig: ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         44,
				Kind:       UploadKindSub2API,
				Name:       "Sub2API Two",
				BaseURL:    "https://sub2api-2.example.com",
				Credential: "sub2api-key-2",
				TargetType: DefaultSub2APITargetType,
				Enabled:    true,
				Priority:   0,
			},
		},
		updateConfig: ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:         33,
				Kind:       UploadKindTM,
				Name:       "TM One Updated",
				BaseURL:    "https://tm-new.example.com",
				Credential: "tm-key-old",
				Enabled:    false,
				Priority:   7,
			},
		},
		deleteConfig: ManagedServiceConfig{
			ServiceConfig: ServiceConfig{
				ID:   33,
				Kind: UploadKindTM,
				Name: "TM One Updated",
			},
		},
	}
	svc := NewService(repo)

	created, err := svc.CreateSub2APIService(context.Background(), CreateSub2APIServiceRequest{
		Name:       "Sub2API Two",
		APIURL:     "https://sub2api-2.example.com",
		APIKey:     "sub2api-key-2",
		TargetType: "",
		Enabled:    true,
		Priority:   0,
	})
	if err != nil {
		t.Fatalf("create sub2api service: %v", err)
	}
	if !created.HasKey {
		t.Fatalf("expected created sub2api response to expose has_key=true, got %+v", created)
	}
	if repo.lastCreated.ServiceConfig.TargetType != DefaultSub2APITargetType {
		t.Fatalf("expected create to normalize empty target_type to %q, got %+v", DefaultSub2APITargetType, repo.lastCreated)
	}

	updated, err := svc.UpdateTMService(context.Background(), 33, UpdateTMServiceRequest{
		Name:     testStringPointer("TM One Updated"),
		APIURL:   testStringPointer("https://tm-new.example.com"),
		APIKey:   testStringPointer(""),
		Enabled:  boolPointer(false),
		Priority: intPointer(7),
	})
	if err != nil {
		t.Fatalf("update tm service: %v", err)
	}
	if updated.Name != "TM One Updated" || updated.HasKey != true || updated.Enabled != false || updated.Priority != 7 {
		t.Fatalf("unexpected tm update response: %+v", updated)
	}
	if repo.lastUpdated.Credential == nil || *repo.lastUpdated.Credential != "tm-key-old" {
		t.Fatalf("expected blank api_key patch to preserve old credential, got %+v", repo.lastUpdated)
	}

	deleted, err := svc.DeleteTMService(context.Background(), 33)
	if err != nil {
		t.Fatalf("delete tm service: %v", err)
	}
	if !deleted.Success || deleted.Message != "Team Manager 服务 TM One Updated 已删除" {
		t.Fatalf("unexpected tm delete response: %+v", deleted)
	}
}

func TestUploadServiceTestConnectionUsesTargetSpecificProbeRequests(t *testing.T) {
	doer := &fakeServiceHTTPDoer{
		responses: []*http.Response{
			serviceTextResponse(http.StatusOK, ""),
			serviceTextResponse(http.StatusMethodNotAllowed, ""),
			serviceTextResponse(http.StatusNoContent, ""),
		},
	}
	svc := NewService(nil, WithHTTPDoer(doer))

	cpaResult, err := svc.TestCPAConnection(context.Background(), CPAConnectionTestRequest{
		APIURL:   "https://cpa.example.com",
		APIToken: "cpa-token",
	})
	if err != nil {
		t.Fatalf("test cpa connection: %v", err)
	}
	if !cpaResult.Success || cpaResult.Message != "CPA 连接测试成功" {
		t.Fatalf("unexpected cpa connection result: %+v", cpaResult)
	}

	sub2apiResult, err := svc.TestSub2APIConnection(context.Background(), Sub2APIConnectionTestRequest{
		APIURL: "https://sub2api.example.com",
		APIKey: "sub2api-key",
	})
	if err != nil {
		t.Fatalf("test sub2api connection: %v", err)
	}
	if !sub2apiResult.Success || sub2apiResult.Message != "Sub2API 连接测试成功" {
		t.Fatalf("unexpected sub2api connection result: %+v", sub2apiResult)
	}

	tmResult, err := svc.TestTMConnection(context.Background(), TMConnectionTestRequest{
		APIURL: "https://tm.example.com",
		APIKey: "tm-key",
	})
	if err != nil {
		t.Fatalf("test tm connection: %v", err)
	}
	if !tmResult.Success || tmResult.Message != "Team Manager 连接测试成功" {
		t.Fatalf("unexpected tm connection result: %+v", tmResult)
	}

	if len(doer.requests) != 3 {
		t.Fatalf("expected 3 probe requests, got %d", len(doer.requests))
	}
	if doer.requests[0].Method != http.MethodGet || doer.requests[0].URL.String() != "https://cpa.example.com/v0/management/auth-files" {
		t.Fatalf("unexpected cpa probe request: %s %s", doer.requests[0].Method, doer.requests[0].URL.String())
	}
	if got := doer.requests[0].Header.Get("Authorization"); got != "Bearer cpa-token" {
		t.Fatalf("expected cpa bearer token, got %q", got)
	}
	if doer.requests[1].Method != http.MethodGet || doer.requests[1].URL.String() != "https://sub2api.example.com/api/v1/admin/accounts/data" {
		t.Fatalf("unexpected sub2api probe request: %s %s", doer.requests[1].Method, doer.requests[1].URL.String())
	}
	if got := doer.requests[1].Header.Get("x-api-key"); got != "sub2api-key" {
		t.Fatalf("expected sub2api api key, got %q", got)
	}
	if doer.requests[2].Method != http.MethodOptions || doer.requests[2].URL.String() != "https://tm.example.com/admin/teams/import" {
		t.Fatalf("unexpected tm probe request: %s %s", doer.requests[2].Method, doer.requests[2].URL.String())
	}
	if got := doer.requests[2].Header.Get("X-API-Key"); got != "tm-key" {
		t.Fatalf("expected tm api key, got %q", got)
	}
}

func TestUploadServiceUploadSub2APIUsesSenderAndWritesBackCompatibleDetails(t *testing.T) {
	accountStore := &fakeUploadAccountStore{
		accounts: []UploadAccount{
			{ID: 1, Email: "alpha@example.com", AccessToken: "access-1"},
			{ID: 2, Email: "skip@example.com"},
		},
	}
	repo := &fakeUploadAdminRepository{
		listConfigs: []ManagedServiceConfig{
			{
				ServiceConfig: ServiceConfig{
					ID:         55,
					Kind:       UploadKindSub2API,
					Name:       "Sub2API Enabled",
					BaseURL:    "https://sub2api.example.com",
					Credential: "sub2api-key",
					TargetType: "newapi",
					Enabled:    true,
					Priority:   4,
				},
			},
		},
	}
	sender := &fakeServiceSender{
		results: []UploadResult{
			{Kind: UploadKindSub2API, ServiceID: 55, AccountEmail: "alpha@example.com", Success: true, Message: "成功上传 1 个账号"},
		},
	}
	svc := NewService(
		repo,
		WithUploadAccountStore(accountStore),
		WithSenderFactory(func(kind UploadKind) (Sender, error) {
			if kind != UploadKindSub2API {
				t.Fatalf("expected sub2api sender, got %q", kind)
			}
			return sender, nil
		}),
		WithClock(func() time.Time {
			return time.Date(2026, 4, 5, 13, 0, 0, 0, time.UTC)
		}),
	)

	result, err := svc.UploadSub2API(context.Background(), Sub2APIUploadRequest{
		AccountIDs:  []int{1, 2, 999},
		Concurrency: 7,
		Priority:    88,
	})
	if err != nil {
		t.Fatalf("upload sub2api: %v", err)
	}
	if result.SuccessCount != 1 || result.SkippedCount != 1 || result.FailedCount != 1 {
		t.Fatalf("unexpected upload counters: %+v", result)
	}
	if len(result.Details) != 3 {
		t.Fatalf("expected 3 upload details, got %+v", result.Details)
	}
	if result.Details[0].ID != 2 || result.Details[0].Error != "缺少 access_token" {
		t.Fatalf("unexpected skipped detail: %+v", result.Details[0])
	}
	if result.Details[1].ID != 999 || result.Details[1].Email != nil || result.Details[1].Error != "账号不存在" {
		t.Fatalf("unexpected missing-account detail: %+v", result.Details[1])
	}
	if result.Details[2].ID != 1 || !result.Details[2].Success || result.Details[2].Message != "成功上传 1 个账号" {
		t.Fatalf("unexpected success detail: %+v", result.Details[2])
	}
	if len(sender.requests) != 1 {
		t.Fatalf("expected one sender request, got %#v", sender.requests)
	}
	if sender.requests[0].Service.TargetType != "newapi" || sender.requests[0].Sub2API.Concurrency != 7 || sender.requests[0].Sub2API.Priority != 88 {
		t.Fatalf("expected sender to reuse service/option settings, got %+v", sender.requests[0])
	}
	if len(accountStore.markedIDs) != 1 || accountStore.markedIDs[0] != 1 {
		t.Fatalf("expected only successful account to be marked uploaded, got %#v", accountStore.markedIDs)
	}
	if accountStore.markedAt == nil || !accountStore.markedAt.Equal(time.Date(2026, 4, 5, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected mark uploaded time: %#v", accountStore.markedAt)
	}
}

type fakeUploadAdminRepository struct {
	lastListKind    UploadKind
	lastListFilter  *ServiceConfigListFilter
	lastGetKind     UploadKind
	lastGetID       int
	lastCreated     ManagedServiceConfig
	lastUpdated     ManagedServiceConfigPatch
	lastDeletedKind UploadKind
	lastDeletedID   int

	listConfigs  []ManagedServiceConfig
	getConfig    ManagedServiceConfig
	createConfig ManagedServiceConfig
	updateConfig ManagedServiceConfig
	deleteConfig ManagedServiceConfig
}

func (f *fakeUploadAdminRepository) ListServiceConfigs(_ context.Context, kind UploadKind, filter ServiceConfigListFilter) ([]ManagedServiceConfig, error) {
	f.lastListKind = kind
	f.lastListFilter = cloneServiceConfigListFilter(filter)
	return append([]ManagedServiceConfig(nil), f.listConfigs...), nil
}

func (f *fakeUploadAdminRepository) GetServiceConfig(_ context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error) {
	f.lastGetKind = kind
	f.lastGetID = id
	return f.getConfig, true, nil
}

func (f *fakeUploadAdminRepository) CreateServiceConfig(_ context.Context, config ManagedServiceConfig) (ManagedServiceConfig, error) {
	f.lastCreated = config
	return f.createConfig, nil
}

func (f *fakeUploadAdminRepository) UpdateServiceConfig(_ context.Context, kind UploadKind, id int, patch ManagedServiceConfigPatch) (ManagedServiceConfig, bool, error) {
	f.lastUpdated = patch
	f.lastGetKind = kind
	f.lastGetID = id
	return f.updateConfig, true, nil
}

func (f *fakeUploadAdminRepository) DeleteServiceConfig(_ context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error) {
	f.lastDeletedKind = kind
	f.lastDeletedID = id
	return f.deleteConfig, true, nil
}

func cloneServiceConfigListFilter(filter ServiceConfigListFilter) *ServiceConfigListFilter {
	cloned := filter
	if filter.Enabled != nil {
		value := *filter.Enabled
		cloned.Enabled = &value
	}
	return &cloned
}

func testStringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

type fakeServiceHTTPDoer struct {
	requests  []*http.Request
	responses []*http.Response
	err       error
}

func (f *fakeServiceHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

type fakeUploadAccountStore struct {
	accounts  []UploadAccount
	markedIDs []int
	markedAt  *time.Time
}

func (f *fakeUploadAccountStore) ListUploadAccounts(_ context.Context, ids []int) ([]UploadAccount, error) {
	selected := make([]UploadAccount, 0, len(ids))
	for _, id := range ids {
		for _, account := range f.accounts {
			if account.ID == id {
				selected = append(selected, account)
			}
		}
	}
	return selected, nil
}

func (f *fakeUploadAccountStore) MarkSub2APIUploaded(_ context.Context, ids []int, uploadedAt time.Time) error {
	f.markedIDs = append([]int(nil), ids...)
	cloned := uploadedAt
	f.markedAt = &cloned
	return nil
}

type fakeServiceSender struct {
	requests []SendRequest
	results  []UploadResult
}

func (f *fakeServiceSender) Kind() UploadKind {
	if len(f.results) == 0 {
		return ""
	}
	return f.results[0].Kind
}

func (f *fakeServiceSender) Send(_ context.Context, req SendRequest) ([]UploadResult, error) {
	f.requests = append(f.requests, req)
	return append([]UploadResult(nil), f.results...), nil
}

func serviceTextResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
