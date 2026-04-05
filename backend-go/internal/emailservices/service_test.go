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
			"tempmail.enabled": "true",
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

type fakeRepository struct {
	services      []EmailServiceRecord
	stats         map[string]int
	enabledCount  int
	accounts      []RegisteredAccountRecord
	settings      map[string]string
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

func typeCatalogContains(items []ServiceTypeDefinition, want string) bool {
	for _, item := range items {
		if item.Value == want {
			return true
		}
	}
	return false
}
