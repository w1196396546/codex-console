package registration

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestOrchestratorPreparesTempmailFromSettings(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"tempmail.enabled":     "true",
				"tempmail.base_url":    "https://api.tempmail.example/v2",
				"tempmail.timeout":     "45",
				"tempmail.max_retries": "7",
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-tempmail", StartRequest{})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if prepared.Request.EmailServiceType != "tempmail" {
		t.Fatalf("expected normalized tempmail request, got %+v", prepared.Request)
	}
	if prepared.Plan.Stage != ExecuteStageRegistration {
		t.Fatalf("expected execute stage, got %+v", prepared.Plan)
	}
	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "settings.tempmail" {
		t.Fatalf("expected native tempmail preparation, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":    "https://api.tempmail.example/v2",
		"timeout":     45,
		"max_retries": 7,
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected tempmail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
	if prepared.Plan.Proxy.Source != "unassigned" {
		t.Fatalf("expected unassigned proxy plan, got %+v", prepared.Plan.Proxy)
	}
}

func TestOrchestratorPreparesTempMailAliasFromCatalogByType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          21,
					ServiceType: "temp_mail",
					Name:        "Temp Mail Alias",
					Priority:    1,
					Config: map[string]any{
						"base_url":       "https://alias.tempmail.example/api",
						"default_domain": "alias.example.com",
						"timeout":        33,
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-temp-mail-alias", StartRequest{
		EmailServiceType: "temp_mail",
		Proxy:            "http://proxy.internal:8181",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.temp_mail" {
		t.Fatalf("expected native temp_mail preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.Type != "tempmail" {
		t.Fatalf("expected temp_mail alias to normalize to tempmail, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 21 {
		t.Fatalf("expected selected temp_mail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":  "https://alias.tempmail.example/api",
		"domain":    "alias.example.com",
		"timeout":   33,
		"proxy_url": "http://proxy.internal:8181",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected temp_mail alias config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorResolvesConfiguredEmailServiceByID(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          42,
					ServiceType: "duck_mail",
					Name:        "Duck",
					Config: map[string]any{
						"api_url": "https://duck.example/api",
						"domain":  "duck.example.com",
					},
				},
			},
		},
	})

	serviceID := 42
	prepared, err := orchestrator.Prepare(context.Background(), "task-service-id", StartRequest{
		EmailServiceType: "outlook",
		EmailServiceID:   &serviceID,
		Proxy:            "http://proxy.internal:8080",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_id" {
		t.Fatalf("expected native service-id preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.Type != "duck_mail" {
		t.Fatalf("expected resolved duck_mail type, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":       "https://duck.example/api",
		"default_domain": "duck.example.com",
		"proxy_url":      "http://proxy.internal:8080",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected resolved config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesYYDSMailFromSettings(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"yyds_mail.enabled":        "true",
				"yyds_mail.base_url":       "https://maliapi.example/v1",
				"yyds_mail.api_key":        "secret-key",
				"yyds_mail.default_domain": "mail.example.com",
				"yyds_mail.timeout":        "40",
				"yyds_mail.max_retries":    "5",
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-yyds", StartRequest{
		EmailServiceType: "yyds_mail",
		Proxy:            "http://proxy.internal:8080",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "settings.yyds_mail" {
		t.Fatalf("expected native yyds_mail preparation, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":       "https://maliapi.example/v1",
		"api_key":        "secret-key",
		"default_domain": "mail.example.com",
		"timeout":        40,
		"max_retries":    5,
		"proxy_url":      "http://proxy.internal:8080",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected yyds_mail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesDuckMailFromCatalogByType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          11,
					ServiceType: "duck_mail",
					Name:        "Duck Primary",
					Priority:    1,
					Config: map[string]any{
						"api_url": "https://duck.example/api",
						"domain":  "duck.example.com",
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-duck", StartRequest{
		EmailServiceType: "duck_mail",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.duck_mail" {
		t.Fatalf("expected native duck_mail preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 11 {
		t.Fatalf("expected selected duck_mail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":       "https://duck.example/api",
		"default_domain": "duck.example.com",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected duck_mail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesFreemailFromCatalogByType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          12,
					ServiceType: "freemail",
					Name:        "Freemail Primary",
					Priority:    1,
					Config: map[string]any{
						"base_url":    "https://freemail.example",
						"admin_token": "admin-secret",
						"domain":      "mail.freemail.example",
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-freemail", StartRequest{
		EmailServiceType: "freemail",
		Proxy:            "http://proxy.internal:9090",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.freemail" {
		t.Fatalf("expected native freemail preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 12 {
		t.Fatalf("expected selected freemail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":    "https://freemail.example",
		"admin_token": "admin-secret",
		"domain":      "mail.freemail.example",
		"proxy_url":   "http://proxy.internal:9090",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected freemail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesLuckMailFromCatalogByType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          31,
					ServiceType: "luckmail",
					Name:        "Luck Primary",
					Priority:    1,
					Config: map[string]any{
						"api_url":      "https://luckmail.example/api",
						"api_key":      "luck-key",
						"project_code": "project-openai",
						"email_type":   "ms_graph",
						"domain":       "luck.example.com",
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-luckmail", StartRequest{
		EmailServiceType: "luckmail",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.luckmail" {
		t.Fatalf("expected native luckmail preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 31 {
		t.Fatalf("expected selected luckmail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":         "https://luckmail.example/api",
		"api_key":          "luck-key",
		"project_code":     "project-openai",
		"email_type":       "ms_graph",
		"preferred_domain": "luck.example.com",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected luckmail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesIMAPMailFromCatalogByType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          41,
					ServiceType: "imap_mail",
					Name:        "IMAP Primary",
					Priority:    1,
					Config: map[string]any{
						"host":     "imap.example.com",
						"port":     993,
						"email":    "native@example.com",
						"password": "imap-secret",
						"use_ssl":  true,
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-imap-mail", StartRequest{
		EmailServiceType: "imap_mail",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.imap_mail" {
		t.Fatalf("expected native imap_mail preparation, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 41 {
		t.Fatalf("expected selected imap_mail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"host":     "imap.example.com",
		"port":     993,
		"email":    "native@example.com",
		"password": "imap-secret",
		"use_ssl":  true,
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected imap_mail config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesMoeMailFromCatalogBeforeSettings(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"custom_domain.base_url": "https://settings.moe.example/api",
				"custom_domain.api_key":  "settings-key",
			},
		},
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          13,
					ServiceType: "moe_mail",
					Name:        "Moe Primary",
					Priority:    1,
					Config: map[string]any{
						"api_url": "https://catalog.moe.example/api",
						"domain":  "mail.moe.example",
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-moe-catalog", StartRequest{
		EmailServiceType: "moe_mail",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "email_service_type.moe_mail" {
		t.Fatalf("expected moe_mail to prefer configured email service, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 13 {
		t.Fatalf("expected selected moe_mail service id, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":       "https://catalog.moe.example/api",
		"default_domain": "mail.moe.example",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected moe_mail catalog config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorPreparesMoeMailFromSettingsWhenCatalogMissing(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"custom_domain.base_url": "https://settings.moe.example/api",
				"custom_domain.api_key":  "settings-key",
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-moe-settings", StartRequest{
		EmailServiceType: "moe_mail",
		Proxy:            "http://proxy.internal:7070",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "settings.custom_domain" {
		t.Fatalf("expected native moe_mail settings preparation, got %+v", prepared.Plan.EmailService)
	}
	wantConfig := map[string]any{
		"base_url":  "https://settings.moe.example/api",
		"api_key":   "settings-key",
		"proxy_url": "http://proxy.internal:7070",
	}
	if !reflect.DeepEqual(prepared.Plan.EmailService.Config, wantConfig) {
		t.Fatalf("unexpected moe_mail settings config: got %#v want %#v", prepared.Plan.EmailService.Config, wantConfig)
	}
}

func TestOrchestratorSelectsAndReservesOutlookAccount(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Outlook: fakeOutlookPreparationReader{
			services: []EmailServiceRecord{
				{
					ID:          101,
					ServiceType: "outlook",
					Name:        "Already Registered",
					Config: map[string]any{
						"email":         "alpha@example.com",
						"client_id":     "client-alpha",
						"refresh_token": "refresh-alpha",
					},
				},
				{
					ID:          202,
					ServiceType: "outlook",
					Name:        "Fresh Outlook",
					Config: map[string]any{
						"email":         "beta@example.com",
						"client_id":     "client-beta",
						"refresh_token": "refresh-beta",
					},
				},
			},
			accounts: []RegisteredAccountRecord{
				{ID: 1, Email: "alpha@example.com", RefreshToken: "registered-refresh"},
			},
		},
		Reservations: &fakeOutlookReservationStore{},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-outlook", StartRequest{
		EmailServiceType: "outlook",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.ServiceID == nil || *prepared.Plan.EmailService.ServiceID != 202 {
		t.Fatalf("expected selected fresh outlook account, got %+v", prepared.Plan.EmailService)
	}
	if prepared.Plan.Outlook == nil {
		t.Fatalf("expected outlook metadata, got %+v", prepared.Plan)
	}
	if prepared.Plan.Outlook.Email != "beta@example.com" || prepared.Plan.Outlook.RegistrationState != "unregistered" {
		t.Fatalf("unexpected outlook metadata: %+v", prepared.Plan.Outlook)
	}
	if prepared.Plan.Outlook.ReservationStatus != "reserved" {
		t.Fatalf("expected reserved outlook account, got %+v", prepared.Plan.Outlook)
	}
}

func TestOrchestratorReturnsWrappedSettingsError(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{err: errors.New("boom")},
	})

	_, err := orchestrator.Prepare(context.Background(), "task-error", StartRequest{EmailServiceType: "tempmail"})
	if err == nil {
		t.Fatal("expected prepare error")
	}
	if !errors.Is(err, errPreparationSettingsLookup) {
		t.Fatalf("expected settings sentinel, got %v", err)
	}
}

func TestOrchestratorRejectsUnsupportedInlineEmailServiceType(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{})

	_, err := orchestrator.Prepare(context.Background(), "task-unsupported", StartRequest{
		EmailServiceType: "custom_mail",
		EmailServiceConfig: map[string]any{
			"base_url": "https://custom.example/api",
		},
	})
	if err == nil {
		t.Fatal("expected unsupported email service error")
	}
	if !errors.Is(err, errNativeUnsupportedEmailService) {
		t.Fatalf("expected unsupported email service sentinel, got %v", err)
	}
}

func TestOrchestratorRequiresSettingsProviderForTempmail(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{})

	_, err := orchestrator.Prepare(context.Background(), "task-tempmail-missing-settings", StartRequest{
		EmailServiceType: "tempmail",
	})
	if err == nil {
		t.Fatal("expected missing settings provider error")
	}
	if !errors.Is(err, errNativePreparationDependencyMissing) {
		t.Fatalf("expected missing dependency sentinel, got %v", err)
	}
}

func TestOrchestratorRequiresEmailServiceCatalogForBoundServiceID(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{})

	serviceID := 42
	_, err := orchestrator.Prepare(context.Background(), "task-bound-service", StartRequest{
		EmailServiceType: "outlook",
		EmailServiceID:   &serviceID,
	})
	if err == nil {
		t.Fatal("expected email service catalog dependency error")
	}
	if !errors.Is(err, errNativePreparationDependencyMissing) {
		t.Fatalf("expected missing dependency sentinel, got %v", err)
	}
}

func TestOrchestratorRejectsYYDSMailWithoutAPIKey(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"yyds_mail.enabled":        "true",
				"yyds_mail.base_url":       "https://maliapi.example/v1",
				"yyds_mail.default_domain": "mail.example.com",
			},
		},
	})

	_, err := orchestrator.Prepare(context.Background(), "task-yyds-missing-key", StartRequest{
		EmailServiceType: "yyds_mail",
	})
	if err == nil {
		t.Fatal("expected missing yyds api key error")
	}
	if !errors.Is(err, errNativeEmailServiceConfiguration) {
		t.Fatalf("expected missing configuration sentinel, got %v", err)
	}
}

func TestOrchestratorRejectsTempmailWithoutBaseURL(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"tempmail.enabled": "true",
			},
		},
	})

	_, err := orchestrator.Prepare(context.Background(), "task-tempmail-missing-base-url", StartRequest{
		EmailServiceType: "tempmail",
	})
	if err == nil {
		t.Fatal("expected missing tempmail base_url error")
	}
	if !errors.Is(err, errNativeEmailServiceConfiguration) {
		t.Fatalf("expected missing configuration sentinel, got %v", err)
	}
}

func TestOrchestratorMoeMailFallsBackToSettingsWhenCatalogHasNoMatchingService(t *testing.T) {
	orchestrator := newOrchestrator(PreparationDependencies{
		Settings: fakePreparationSettings{
			settings: map[string]string{
				"custom_domain.base_url": "https://settings.moe.example/api",
				"custom_domain.api_key":  "settings-key",
			},
		},
		EmailServices: fakeEmailServiceCatalog{
			services: []EmailServiceRecord{
				{
					ID:          99,
					ServiceType: "duck_mail",
					Name:        "Other Service",
					Config: map[string]any{
						"api_url": "https://duck.example/api",
					},
				},
			},
		},
	})

	prepared, err := orchestrator.Prepare(context.Background(), "task-moe-settings-fallback", StartRequest{
		EmailServiceType: "moe_mail",
	})
	if err != nil {
		t.Fatalf("unexpected prepare error: %v", err)
	}
	if !prepared.Plan.EmailService.Prepared || prepared.Plan.EmailService.Source != "settings.custom_domain" {
		t.Fatalf("expected moe_mail settings preparation, got %+v", prepared.Plan.EmailService)
	}
}

type fakePreparationSettings struct {
	settings map[string]string
	err      error
}

func (f fakePreparationSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.settings, nil
}

type fakeEmailServiceCatalog struct {
	services []EmailServiceRecord
	err      error
}

func (f fakeEmailServiceCatalog) ListEmailServices(context.Context) ([]EmailServiceRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]EmailServiceRecord(nil), f.services...), nil
}

type fakeOutlookPreparationReader struct {
	services []EmailServiceRecord
	accounts []RegisteredAccountRecord
	err      error
}

func (f fakeOutlookPreparationReader) ListOutlookServices(context.Context) ([]EmailServiceRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]EmailServiceRecord(nil), f.services...), nil
}

func (f fakeOutlookPreparationReader) ListAccountsByEmails(context.Context, []string) ([]RegisteredAccountRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]RegisteredAccountRecord(nil), f.accounts...), nil
}

type fakeOutlookReservationStore struct {
	claimed      []int
	reservedTask string
	reservedID   int
	err          error
}

func (f *fakeOutlookReservationStore) ListClaimedOutlookServiceIDs(context.Context, string) ([]int, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]int(nil), f.claimed...), nil
}

func (f *fakeOutlookReservationStore) ReserveOutlookService(_ context.Context, taskUUID string, serviceID int) error {
	if f.err != nil {
		return f.err
	}
	f.reservedTask = taskUUID
	f.reservedID = serviceID
	return nil
}
