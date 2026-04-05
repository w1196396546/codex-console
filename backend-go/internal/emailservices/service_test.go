package emailservices

import (
	"context"
	"testing"
	"time"
)

func TestEmailServicesServiceListAndFullPreservePythonContracts(t *testing.T) {
	t.Parallel()

	lastUsed := time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 4, 5, 8, 45, 0, 0, time.UTC)

	repo := &fakeRepository{
		services: []EmailServiceRecord{
			{
				ID:          7,
				ServiceType: ServiceTypeOutlook,
				Name:        "owner@example.com",
				Enabled:     true,
				Priority:    2,
				Config: map[string]any{
					"email":         "Owner@Example.com",
					"password":      "secret-password",
					"client_id":     "client-id",
					"refresh_token": "refresh-token",
				},
				LastUsed:  &lastUsed,
				CreatedAt: &createdAt,
				UpdatedAt: &updatedAt,
			},
		},
		accounts: []RegisteredAccountRecord{
			{ID: 99, Email: "owner@example.com"},
		},
	}

	svc := NewService(repo, nil)

	listResp, err := svc.ListServices(context.Background(), ListServicesRequest{ServiceType: "outlook"})
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Services) != 1 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	service := listResp.Services[0]
	if service.Config["password"] != nil {
		t.Fatalf("expected filtered config to hide password, got %+v", service.Config)
	}
	if service.Config["has_password"] != true {
		t.Fatalf("expected filtered config to expose has_password, got %+v", service.Config)
	}
	if service.Config["has_oauth"] != true {
		t.Fatalf("expected filtered config to expose has_oauth, got %+v", service.Config)
	}
	if service.RegistrationStatus != "registered" {
		t.Fatalf("expected registration_status=registered, got %+v", service)
	}
	if service.RegisteredAccountID == nil || *service.RegisteredAccountID != 99 {
		t.Fatalf("expected registered_account_id=99, got %+v", service.RegisteredAccountID)
	}
	if service.LastUsed != lastUsed.Format(time.RFC3339) {
		t.Fatalf("expected last_used=%q, got %q", lastUsed.Format(time.RFC3339), service.LastUsed)
	}

	fullResp, err := svc.GetServiceFull(context.Background(), 7)
	if err != nil {
		t.Fatalf("get full service: %v", err)
	}
	if fullResp.Config["password"] != "secret-password" || fullResp.Config["refresh_token"] != "refresh-token" {
		t.Fatalf("expected full config to preserve secrets, got %+v", fullResp.Config)
	}
}

func TestEmailServicesStatsPreserveFieldsAndSettingsDependency(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		stats: map[string]int{
			"outlook":   2,
			"moe_mail":  1,
			"yyds_mail": 1,
			"temp_mail": 1,
			"duck_mail": 1,
			"freemail":  1,
			"imap_mail": 1,
			"luckmail":  1,
		},
		enabledCount: 6,
		settings: map[string]string{
			"tempmail.enabled":  "true",
			"yyds_mail.enabled": "true",
			"yyds_mail.api_key": "secret-key",
		},
	}

	svc := NewService(repo, nil)
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if stats.OutlookCount != 2 || stats.CustomCount != 1 || stats.YYDSMailCount != 1 || stats.EnabledCount != 6 {
		t.Fatalf("unexpected stats core fields: %+v", stats)
	}
	if !stats.TempmailAvailable || !stats.YYDSMailAvailable {
		t.Fatalf("expected tempmail/yyds availability from settings dependency, got %+v", stats)
	}
	if stats.TempMailCount != 1 || stats.DuckMailCount != 1 || stats.FreemailCount != 1 || stats.IMAPMailCount != 1 || stats.LuckmailCount != 1 {
		t.Fatalf("expected per-type counters to be preserved, got %+v", stats)
	}
}

func TestEmailServicesTypeCatalogPreservesKnownTypes(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{}, nil)

	resp := svc.GetServiceTypes()
	if len(resp.Types) == 0 {
		t.Fatal("expected non-empty service type catalog")
	}

	expectedValues := []string{"tempmail", "yyds_mail", "outlook", "moe_mail", "temp_mail", "duck_mail", "freemail", "imap_mail", "luckmail"}
	for _, expected := range expectedValues {
		if !typeCatalogContains(resp.Types, expected) {
			t.Fatalf("expected type catalog to contain %q, got %+v", expected, resp.Types)
		}
	}
}

func TestEmailServicesWriteOperationsPreservePythonContracts(t *testing.T) {
	t.Parallel()

	existing := EmailServiceRecord{
		ID:          1,
		ServiceType: ServiceTypeOutlook,
		Name:        "owner@example.com",
		Enabled:     true,
		Priority:    5,
		Config: map[string]any{
			"email":    "owner@example.com",
			"password": "secret-password",
		},
	}

	repo := &fakeRepository{
		services: []EmailServiceRecord{existing},
	}
	tester := &fakeTester{
		results: map[string]ServiceTestResult{
			ServiceTypeOutlook: {Success: true, Message: "服务连接正常"},
		},
	}

	svc := NewService(repo, tester)

	created, err := svc.CreateService(context.Background(), CreateServiceRequest{
		ServiceType: ServiceTypeDuckMail,
		Name:        "duck-primary",
		Config: map[string]any{
			"base_url":       "https://duckmail.example",
			"default_domain": "duckmail.sbs",
			"api_key":        "duck-key",
		},
		Enabled:  true,
		Priority: 3,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if created.ServiceType != ServiceTypeDuckMail || created.Config["has_api_key"] != true {
		t.Fatalf("unexpected created service response: %+v", created)
	}

	updated, err := svc.UpdateService(context.Background(), 1, UpdateServiceRequest{
		Name:     stringPtr("owner+updated@example.com"),
		Enabled:  boolPtr(false),
		Priority: intPtr(1),
		Config: map[string]any{
			"email":    "owner+updated@example.com",
			"password": "",
		},
	})
	if err != nil {
		t.Fatalf("update service: %v", err)
	}
	if updated.Name != "owner+updated@example.com" || updated.Enabled {
		t.Fatalf("unexpected updated service response: %+v", updated)
	}
	full, err := svc.GetServiceFull(context.Background(), 1)
	if err != nil {
		t.Fatalf("get full service after update: %v", err)
	}
	if _, ok := full.Config["password"]; ok {
		t.Fatalf("expected empty password to be removed from merged config, got %+v", full.Config)
	}

	testResult, err := svc.TestService(context.Background(), 1)
	if err != nil {
		t.Fatalf("test service: %v", err)
	}
	if !testResult.Success || len(tester.calls) == 0 || tester.calls[0] != ServiceTypeOutlook {
		t.Fatalf("expected native tester call for outlook service, got result=%+v calls=%+v", testResult, tester.calls)
	}

	enableResp, err := svc.EnableService(context.Background(), 1)
	if err != nil {
		t.Fatalf("enable service: %v", err)
	}
	if !enableResp.Success || enableResp.Message != "服务 owner+updated@example.com 已启用" {
		t.Fatalf("unexpected enable response: %+v", enableResp)
	}

	reorderResp, err := svc.ReorderServices(context.Background(), []int{created.ID, 1})
	if err != nil {
		t.Fatalf("reorder services: %v", err)
	}
	if !reorderResp.Success || repo.services[0].Priority != 1 || repo.services[1].Priority != 0 {
		t.Fatalf("unexpected reorder response/state: resp=%+v services=%+v", reorderResp, repo.services)
	}

	deleteResp, err := svc.DeleteService(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("delete service: %v", err)
	}
	if !deleteResp.Success || deleteResp.Message != "服务 duck-primary 已删除" {
		t.Fatalf("unexpected delete response: %+v", deleteResp)
	}
}

func TestOutlookBatchImportAndTempmailDependencyStayInGo(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		services: []EmailServiceRecord{
			{
				ID:          9,
				ServiceType: ServiceTypeOutlook,
				Name:        "exists@example.com",
				Config: map[string]any{
					"email":    "exists@example.com",
					"password": "exists-password",
				},
			},
		},
		settings: map[string]string{
			"tempmail.enabled":     "true",
			"tempmail.base_url":    "https://api.tempmail.example/v2",
			"tempmail.timeout":     "45",
			"tempmail.max_retries": "7",
			"yyds_mail.enabled":    "true",
			"yyds_mail.base_url":   "https://maliapi.example/v1",
			"yyds_mail.api_key":    "saved-key",
		},
	}
	tester := &fakeTester{
		results: map[string]ServiceTestResult{
			ServiceTypeTempmail: {Success: true, Message: "临时邮箱连接正常"},
			ServiceTypeYYDSMail: {Success: true, Message: "YYDS Mail 连接正常"},
		},
	}

	svc := NewService(repo, tester)

	importResp, err := svc.BatchImportOutlook(context.Background(), OutlookBatchImportRequest{
		Data:     "new@example.com----secret\nexists@example.com----secret\nnew@example.com----duplicate\nbad-line",
		Enabled:  true,
		Priority: 4,
	})
	if err != nil {
		t.Fatalf("batch import outlook: %v", err)
	}
	if importResp.Total != 4 || importResp.Success != 1 || importResp.Failed != 3 {
		t.Fatalf("unexpected import response: %+v", importResp)
	}
	if len(importResp.Accounts) != 1 || importResp.Accounts[0]["has_oauth"] != false {
		t.Fatalf("expected exactly one created outlook account, got %+v", importResp.Accounts)
	}

	deleteResp, err := svc.BatchDeleteOutlook(context.Background(), []int{9, 10})
	if err != nil {
		t.Fatalf("batch delete outlook: %v", err)
	}
	if !deleteResp.Success || deleteResp.Deleted != 2 {
		t.Fatalf("unexpected batch delete response: %+v", deleteResp)
	}

	tempmailResp, err := svc.TestTempmail(context.Background(), TempmailTestRequest{
		Provider: ServiceTypeTempmail,
	})
	if err != nil {
		t.Fatalf("test tempmail: %v", err)
	}
	if !tempmailResp.Success || tester.calls[len(tester.calls)-1] != ServiceTypeTempmail {
		t.Fatalf("expected tempmail test to stay inside Go dependency chain, got %+v calls=%+v", tempmailResp, tester.calls)
	}

	yydsResp, err := svc.TestTempmail(context.Background(), TempmailTestRequest{
		Provider: ServiceTypeYYDSMail,
		APIURL:   "https://override.maliapi.example/v1",
		APIKey:   "override-key",
	})
	if err != nil {
		t.Fatalf("test yyds mail: %v", err)
	}
	if !yydsResp.Success || tester.calls[len(tester.calls)-1] != ServiceTypeYYDSMail {
		t.Fatalf("expected yyds tempmail test to use Go tester, got %+v calls=%+v", yydsResp, tester.calls)
	}
}

type fakeRepository struct {
	services     []EmailServiceRecord
	stats        map[string]int
	enabledCount int
	accounts     []RegisteredAccountRecord
	settings     map[string]string
	nextID       int
}

func (f *fakeRepository) ListServices(context.Context, ListServicesRequest) ([]EmailServiceRecord, error) {
	return append([]EmailServiceRecord(nil), f.services...), nil
}

func (f *fakeRepository) GetService(context.Context, int) (EmailServiceRecord, bool, error) {
	if len(f.services) == 0 {
		return EmailServiceRecord{}, false, nil
	}
	return f.services[0], true, nil
}

func (f *fakeRepository) CountServices(context.Context) (map[string]int, int, error) {
	return f.stats, f.enabledCount, nil
}

func (f *fakeRepository) GetSettings(context.Context, []string) (map[string]string, error) {
	return f.settings, nil
}

func (f *fakeRepository) ListRegisteredAccountsByEmails(context.Context, []string) ([]RegisteredAccountRecord, error) {
	return append([]RegisteredAccountRecord(nil), f.accounts...), nil
}

func (f *fakeRepository) FindServiceByName(_ context.Context, name string) (EmailServiceRecord, bool, error) {
	for _, service := range f.services {
		if service.Name == name {
			return service, true, nil
		}
	}
	return EmailServiceRecord{}, false, nil
}

func (f *fakeRepository) CreateService(_ context.Context, service EmailServiceRecord) (EmailServiceRecord, error) {
	if f.nextID == 0 {
		f.nextID = len(f.services) + 1
	}
	service.ID = f.nextID
	f.nextID++
	f.services = append(f.services, service)
	return service, nil
}

func (f *fakeRepository) SaveService(_ context.Context, service EmailServiceRecord) (EmailServiceRecord, error) {
	for idx := range f.services {
		if f.services[idx].ID == service.ID {
			f.services[idx] = service
			return service, nil
		}
	}
	f.services = append(f.services, service)
	return service, nil
}

func (f *fakeRepository) DeleteService(_ context.Context, serviceID int) (EmailServiceRecord, bool, error) {
	for idx := range f.services {
		if f.services[idx].ID == serviceID {
			service := f.services[idx]
			f.services = append(f.services[:idx], f.services[idx+1:]...)
			return service, true, nil
		}
	}
	return EmailServiceRecord{}, false, nil
}

func (f *fakeRepository) UpdateServicePriority(_ context.Context, serviceID int, priority int) error {
	for idx := range f.services {
		if f.services[idx].ID == serviceID {
			f.services[idx].Priority = priority
			return nil
		}
	}
	return nil
}

func typeCatalogContains(items []ServiceTypeDefinition, want string) bool {
	for _, item := range items {
		if item.Value == want {
			return true
		}
	}
	return false
}

type fakeTester struct {
	results map[string]ServiceTestResult
	calls   []string
}

func (f *fakeTester) Test(_ context.Context, serviceType string, config map[string]any) (ServiceTestResult, error) {
	f.calls = append(f.calls, serviceType)
	if result, ok := f.results[serviceType]; ok {
		return result, nil
	}
	return ServiceTestResult{Success: false, Message: "服务连接失败", Details: config}, nil
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}
