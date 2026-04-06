package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/adminui"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
)

func TestAPIHandlerMountsAdminUI(t *testing.T) {
	handler, err := adminui.NewHandler(adminui.HandlerOptions{
		BasePath: "/go-admin",
		Settings: apiAdminUISettingsReader{
			repo: fakeAdminUISettingsRepository{
				items: map[string]settings.SettingRecord{
					adminui.DefaultAccessPasswordKey: {Key: adminui.DefaultAccessPasswordKey, Value: "admin123"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new admin ui handler: %v", err)
	}

	server := httptest.NewServer(newAPIHandler(nil, handler))
	defer server.Close()

	resp, err := http.Get(server.URL + "/go-admin/login")
	if err != nil {
		t.Fatalf("get go-admin login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 response, got %d", resp.StatusCode)
	}
}

func TestAPISub2APIUploadServiceInjectsUploadAccountStore(t *testing.T) {
	serviceID := 55
	uploadedAt := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	repo := &fakeAPIUploadAdminRepository{
		config: uploader.ManagedServiceConfig{
			ServiceConfig: uploader.ServiceConfig{
				ID:         serviceID,
				Kind:       uploader.UploadKindSub2API,
				Name:       "Sub2API Main",
				BaseURL:    "https://sub2api.example.com",
				Credential: "sub2api-key",
				TargetType: uploader.DefaultSub2APITargetType,
				Enabled:    true,
				Priority:   6,
			},
		},
	}
	store := &fakeAPIUploadAccountStore{
		accounts: []uploader.UploadAccount{
			{ID: 1, Email: "alpha@example.com", AccessToken: "access-1"},
		},
	}
	sender := &fakeAPIUploadSender{
		results: []uploader.UploadResult{
			{
				Kind:         uploader.UploadKindSub2API,
				ServiceID:    serviceID,
				AccountEmail: "alpha@example.com",
				Success:      true,
				Message:      "上传成功",
			},
		},
	}

	service := newAPIUploaderService(
		repo,
		store,
		uploader.WithClock(func() time.Time { return uploadedAt }),
		uploader.WithSenderFactory(func(kind uploader.UploadKind) (uploader.Sender, error) {
			if kind != uploader.UploadKindSub2API {
				t.Fatalf("unexpected sender kind %q", kind)
			}
			return sender, nil
		}),
	)

	result, err := service.UploadSub2API(context.Background(), uploader.Sub2APIUploadRequest{
		AccountIDs: []int{1},
		ServiceID:  &serviceID,
	})
	if err != nil {
		t.Fatalf("upload sub2api: %v", err)
	}
	if result.SuccessCount != 1 || result.FailedCount != 0 || result.SkippedCount != 0 {
		t.Fatalf("unexpected upload result: %+v", result)
	}
	if len(result.Details) != 1 || !result.Details[0].Success {
		t.Fatalf("expected one successful upload detail, got %+v", result.Details)
	}
	if len(store.markedIDs) != 1 || store.markedIDs[0] != 1 {
		t.Fatalf("expected successful account writeback, got %+v", store.markedIDs)
	}
	if store.markedAt == nil || !store.markedAt.Equal(uploadedAt) {
		t.Fatalf("expected uploaded_at writeback %v, got %#v", uploadedAt, store.markedAt)
	}
}

func TestAPISub2APIUploadWiring(t *testing.T) {
	serviceID := 77
	uploadedAt := time.Date(2026, 4, 5, 16, 0, 0, 0, time.UTC)
	repo := &fakeAPIUploadAdminRepository{
		config: uploader.ManagedServiceConfig{
			ServiceConfig: uploader.ServiceConfig{
				ID:         serviceID,
				Kind:       uploader.UploadKindSub2API,
				Name:       "Sub2API Main",
				BaseURL:    "https://sub2api.example.com",
				Credential: "sub2api-key",
				TargetType: uploader.DefaultSub2APITargetType,
				Enabled:    true,
				Priority:   6,
			},
		},
	}
	store := &fakeAPIUploadAccountStore{
		accounts: []uploader.UploadAccount{
			{ID: 1, Email: "alpha@example.com", AccessToken: "access-1"},
			{ID: 2, Email: "skip@example.com"},
		},
	}
	sender := &fakeAPIUploadSender{
		results: []uploader.UploadResult{
			{
				Kind:         uploader.UploadKindSub2API,
				ServiceID:    serviceID,
				AccountEmail: "alpha@example.com",
				Success:      true,
				Message:      "上传成功",
			},
		},
	}
	server := httptest.NewServer(newAPIHandler(nil, newAPIUploaderService(
		repo,
		store,
		uploader.WithClock(func() time.Time { return uploadedAt }),
		uploader.WithSenderFactory(func(kind uploader.UploadKind) (uploader.Sender, error) {
			if kind != uploader.UploadKindSub2API {
				t.Fatalf("unexpected sender kind %q", kind)
			}
			return sender, nil
		}),
	)))
	defer server.Close()

	payload, err := json.Marshal(map[string]any{
		"account_ids": []int{1, 2, 999},
		"service_id":  serviceID,
		"concurrency": 5,
		"priority":    80,
	})
	if err != nil {
		t.Fatalf("marshal upload payload: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/sub2api-services/upload", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post upload route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 response, got %d", resp.StatusCode)
	}

	var result uploader.Sub2APIUploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if result.SuccessCount != 1 || result.FailedCount != 1 || result.SkippedCount != 1 {
		t.Fatalf("unexpected upload counts: %+v", result)
	}
	if len(result.Details) != 3 {
		t.Fatalf("expected 3 upload details, got %+v", result.Details)
	}
	if result.Details[0].Error == uploader.ErrUploadAccountStoreNotConfigured.Error() {
		t.Fatalf("unexpected store-not-configured failure leaked into route response: %+v", result.Details[0])
	}
	if len(store.markedIDs) != 1 || store.markedIDs[0] != 1 {
		t.Fatalf("expected successful account writeback, got %+v", store.markedIDs)
	}
}

type fakeAPIUploadAdminRepository struct {
	config uploader.ManagedServiceConfig
}

func (f *fakeAPIUploadAdminRepository) ListServiceConfigs(_ context.Context, kind uploader.UploadKind, filter uploader.ServiceConfigListFilter) ([]uploader.ManagedServiceConfig, error) {
	if kind != uploader.UploadKindSub2API || filter.Enabled == nil || !*filter.Enabled {
		return nil, nil
	}
	return []uploader.ManagedServiceConfig{f.config}, nil
}

func (f *fakeAPIUploadAdminRepository) GetServiceConfig(_ context.Context, kind uploader.UploadKind, id int) (uploader.ManagedServiceConfig, bool, error) {
	if kind != uploader.UploadKindSub2API || id != f.config.ID {
		return uploader.ManagedServiceConfig{}, false, nil
	}
	return f.config, true, nil
}

func (f *fakeAPIUploadAdminRepository) CreateServiceConfig(_ context.Context, config uploader.ManagedServiceConfig) (uploader.ManagedServiceConfig, error) {
	return config, nil
}

func (f *fakeAPIUploadAdminRepository) UpdateServiceConfig(_ context.Context, _ uploader.UploadKind, _ int, _ uploader.ManagedServiceConfigPatch) (uploader.ManagedServiceConfig, bool, error) {
	return uploader.ManagedServiceConfig{}, false, nil
}

func (f *fakeAPIUploadAdminRepository) DeleteServiceConfig(_ context.Context, _ uploader.UploadKind, _ int) (uploader.ManagedServiceConfig, bool, error) {
	return uploader.ManagedServiceConfig{}, false, nil
}

type fakeAPIUploadAccountStore struct {
	accounts  []uploader.UploadAccount
	markedIDs []int
	markedAt  *time.Time
}

func (f *fakeAPIUploadAccountStore) ListUploadAccounts(_ context.Context, ids []int) ([]uploader.UploadAccount, error) {
	selected := make([]uploader.UploadAccount, 0, len(ids))
	for _, id := range ids {
		for _, account := range f.accounts {
			if account.ID == id {
				selected = append(selected, account)
			}
		}
	}
	return selected, nil
}

func (f *fakeAPIUploadAccountStore) MarkSub2APIUploaded(_ context.Context, ids []int, uploadedAt time.Time) error {
	f.markedIDs = append([]int(nil), ids...)
	cloned := uploadedAt
	f.markedAt = &cloned
	return nil
}

type fakeAPIUploadSender struct {
	results []uploader.UploadResult
}

type fakeAdminUISettingsRepository struct {
	items map[string]settings.SettingRecord
}

func (f fakeAdminUISettingsRepository) GetSettings(_ context.Context, keys []string) (map[string]settings.SettingRecord, error) {
	result := make(map[string]settings.SettingRecord, len(keys))
	for _, key := range keys {
		if item, ok := f.items[key]; ok {
			result[key] = item
		}
	}
	return result, nil
}

func (f *fakeAPIUploadSender) Kind() uploader.UploadKind {
	return uploader.UploadKindSub2API
}

func (f *fakeAPIUploadSender) Send(_ context.Context, req uploader.SendRequest) ([]uploader.UploadResult, error) {
	return append([]uploader.UploadResult(nil), f.results...), nil
}
