package uploader

import (
	"context"
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
		Name:     stringPointer("TM One Updated"),
		APIURL:   stringPointer("https://tm-new.example.com"),
		APIKey:   stringPointer(""),
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

type fakeUploadAdminRepository struct {
	lastListKind   UploadKind
	lastListFilter *ServiceConfigListFilter
	lastGetKind    UploadKind
	lastGetID      int
	lastCreated    ManagedServiceConfig
	lastUpdated    ManagedServiceConfigPatch
	lastDeletedKind UploadKind
	lastDeletedID   int

	listConfigs   []ManagedServiceConfig
	getConfig     ManagedServiceConfig
	createConfig  ManagedServiceConfig
	updateConfig  ManagedServiceConfig
	deleteConfig  ManagedServiceConfig
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

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}
