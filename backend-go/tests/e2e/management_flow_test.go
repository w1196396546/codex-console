package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	emailservicespkg "github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	logspkg "github.com/dou-jiang/codex-console/backend-go/internal/logs"
	settingspkg "github.com/dou-jiang/codex-console/backend-go/internal/settings"
	uploaderpkg "github.com/dou-jiang/codex-console/backend-go/internal/uploader"
)

func TestManagementSettingsCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(newManagementRouter())
	defer server.Close()

	settingsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/settings", nil).(map[string]any)
	proxyPayload := settingsPayload["proxy"].(map[string]any)
	if proxyPayload["dynamic_enabled"] != true || proxyPayload["dynamic_api_url"] != "https://proxy.example.com" {
		t.Fatalf("unexpected settings proxy payload: %#v", proxyPayload)
	}
	yydsPayload := settingsPayload["yyds_mail"].(map[string]any)
	if yydsPayload["has_api_key"] != true {
		t.Fatalf("expected yyds_mail.has_api_key=true, got %#v", yydsPayload)
	}

	proxiesPayload := mustRequestJSON(t, server, http.MethodGet, "/api/settings/proxies", nil).(map[string]any)
	proxies, ok := proxiesPayload["proxies"].([]any)
	if !ok || len(proxies) != 1 {
		t.Fatalf("expected one proxy row, got %#v", proxiesPayload["proxies"])
	}
	proxy, ok := proxies[0].(map[string]any)
	if !ok || proxy["is_default"] != true || proxy["name"] != "默认代理" {
		t.Fatalf("unexpected proxy row: %#v", proxies[0])
	}

	dynamicTest := mustRequestJSON(t, server, http.MethodPost, "/api/settings/proxy/dynamic/test", map[string]any{
		"api_url": "https://proxy.example.com",
	}).(map[string]any)
	if dynamicTest["success"] != true || dynamicTest["proxy_url"] != "http://proxy.example.com:7890" {
		t.Fatalf("unexpected dynamic proxy test payload: %#v", dynamicTest)
	}

	databasePayload := mustRequestJSON(t, server, http.MethodGet, "/api/settings/database", nil).(map[string]any)
	if databasePayload["accounts_count"] != float64(12) || databasePayload["email_services_count"] != float64(4) {
		t.Fatalf("unexpected database payload: %#v", databasePayload)
	}

	cleanupPayload := mustRequestJSON(t, server, http.MethodPost, "/api/settings/database/cleanup?days=30", nil).(map[string]any)
	if cleanupPayload["deleted_count"] != float64(3) {
		t.Fatalf("unexpected cleanup payload: %#v", cleanupPayload)
	}
}

func TestManagementEmailServicesCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(newManagementRouter())
	defer server.Close()

	statsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/email-services/stats", nil).(map[string]any)
	if statsPayload["outlook_count"] != float64(1) || statsPayload["enabled_count"] != float64(2) {
		t.Fatalf("unexpected email stats payload: %#v", statsPayload)
	}

	outlookPayload := mustRequestJSON(t, server, http.MethodGet, "/api/email-services?service_type=outlook", nil).(map[string]any)
	services, ok := outlookPayload["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("expected one outlook service, got %#v", outlookPayload["services"])
	}
	service, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("expected outlook service object, got %#v", services[0])
	}
	if service["registration_status"] != "registered" || service["registered_account_id"] != float64(501) {
		t.Fatalf("unexpected outlook registration payload: %#v", service)
	}
	config, ok := service["config"].(map[string]any)
	if !ok || config["has_oauth"] != true {
		t.Fatalf("unexpected outlook config payload: %#v", service["config"])
	}

	fullPayload := mustRequestJSON(t, server, http.MethodGet, "/api/email-services/11/full", nil).(map[string]any)
	fullConfig, ok := fullPayload["config"].(map[string]any)
	if !ok || fullConfig["refresh_token"] != "refresh-11" {
		t.Fatalf("unexpected full email-service payload: %#v", fullPayload)
	}

	typesPayload := mustRequestJSON(t, server, http.MethodGet, "/api/email-services/types", nil).(map[string]any)
	types, ok := typesPayload["types"].([]any)
	if !ok || len(types) == 0 {
		t.Fatalf("expected service types array, got %#v", typesPayload["types"])
	}
}

func TestManagementUploaderCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(newManagementRouter())
	defer server.Close()

	cpaPayload := mustRequestJSON(t, server, http.MethodGet, "/api/cpa-services", nil)
	cpaServices, ok := cpaPayload.([]any)
	if !ok || len(cpaServices) != 1 {
		t.Fatalf("expected plain-array CPA services, got %#v", cpaPayload)
	}
	cpaService, ok := cpaServices[0].(map[string]any)
	if !ok || cpaService["api_url"] != "https://cpa.example.com" || cpaService["has_token"] != true {
		t.Fatalf("unexpected CPA service payload: %#v", cpaServices[0])
	}

	sub2Payload := mustRequestJSON(t, server, http.MethodGet, "/api/sub2api-services", nil)
	sub2Services, ok := sub2Payload.([]any)
	if !ok || len(sub2Services) != 1 {
		t.Fatalf("expected plain-array Sub2API services, got %#v", sub2Payload)
	}
	sub2Service, ok := sub2Services[0].(map[string]any)
	if !ok || sub2Service["has_key"] != true {
		t.Fatalf("unexpected Sub2API service payload: %#v", sub2Services[0])
	}
	if _, exists := sub2Service["api_key"]; exists {
		t.Fatalf("expected list endpoint to hide api_key, got %#v", sub2Service)
	}

	sub2Full := mustRequestJSON(t, server, http.MethodGet, "/api/sub2api-services/2/full", nil).(map[string]any)
	if sub2Full["api_key"] != "sub2-key" || sub2Full["target_type"] != "sub2api" {
		t.Fatalf("unexpected Sub2API full payload: %#v", sub2Full)
	}

	tmPayload := mustRequestJSON(t, server, http.MethodGet, "/api/tm-services", nil)
	tmServices, ok := tmPayload.([]any)
	if !ok || len(tmServices) != 1 {
		t.Fatalf("expected plain-array TM services, got %#v", tmPayload)
	}
	tmService, ok := tmServices[0].(map[string]any)
	if !ok || tmService["api_url"] != "https://tm.example.com" || tmService["has_key"] != true {
		t.Fatalf("unexpected TM service payload: %#v", tmServices[0])
	}
}

func TestManagementLogsCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(newManagementRouter())
	defer server.Close()

	logsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/logs?page=1&page_size=100&level=INFO", nil).(map[string]any)
	if logsPayload["total"] != float64(1) {
		t.Fatalf("unexpected logs total: %#v", logsPayload)
	}
	logRows, ok := logsPayload["logs"].([]any)
	if !ok || len(logRows) != 1 {
		t.Fatalf("expected one log row, got %#v", logsPayload["logs"])
	}
	logRow, ok := logRows[0].(map[string]any)
	if !ok || logRow["logger"] != "worker" || logRow["message"] != "registration finished" {
		t.Fatalf("unexpected log row payload: %#v", logRows[0])
	}

	statsPayload := mustRequestJSON(t, server, http.MethodGet, "/api/logs/stats", nil).(map[string]any)
	levels, ok := statsPayload["levels"].(map[string]any)
	if !ok || levels["INFO"] != float64(1) || levels["ERROR"] != float64(1) {
		t.Fatalf("unexpected log stats payload: %#v", statsPayload)
	}

	cleanupPayload := mustRequestJSON(t, server, http.MethodPost, "/api/logs/cleanup", map[string]any{
		"retention_days": 30,
		"max_rows":       50000,
	}).(map[string]any)
	if cleanupPayload["deleted_total"] != float64(2) || cleanupPayload["remaining"] != float64(7) {
		t.Fatalf("unexpected logs cleanup payload: %#v", cleanupPayload)
	}

	clearPayload := mustRequestJSON(t, server, http.MethodDelete, "/api/logs?confirm=true", nil).(map[string]any)
	if clearPayload["deleted_total"] != float64(2) || clearPayload["remaining"] != float64(0) {
		t.Fatalf("unexpected logs clear payload: %#v", clearPayload)
	}
}

func TestManagementPhaseBoundaryExcludesPaymentAndTeamRoutes(t *testing.T) {
	server := httptest.NewServer(newManagementRouter())
	defer server.Close()

	assertNotFound := func(method string, path string) {
		t.Helper()

		req, err := http.NewRequest(method, server.URL+path, nil)
		if err != nil {
			t.Fatalf("build %s %s request: %v", method, path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected %s %s to stay 404, got %d", method, path, resp.StatusCode)
		}
	}

	assertNotFound(http.MethodPost, "/api/payment/accounts/7/session-bootstrap")
	assertNotFound(http.MethodGet, "/api/team/teams")
}

func newManagementRouter() http.Handler {
	now := time.Date(2026, 4, 5, 13, 30, 0, 0, time.UTC)

	settingsRepository := &fakeSettingsRepository{
		settings: map[string]settingspkg.SettingRecord{
			"proxy.dynamic_enabled":        {Key: "proxy.dynamic_enabled", Value: "true"},
			"proxy.dynamic_api_url":        {Key: "proxy.dynamic_api_url", Value: "https://proxy.example.com"},
			"proxy.dynamic_api_key":        {Key: "proxy.dynamic_api_key", Value: "secret-proxy-key"},
			"proxy.dynamic_api_key_header": {Key: "proxy.dynamic_api_key_header", Value: "X-Proxy-Key"},
			"proxy.dynamic_result_field":   {Key: "proxy.dynamic_result_field", Value: "data.proxy"},
			"registration.max_retries":     {Key: "registration.max_retries", Value: "5"},
			"registration.timeout":         {Key: "registration.timeout", Value: "180"},
			"registration.default_password_length": {
				Key:   "registration.default_password_length",
				Value: "16",
			},
			"registration.entry_flow":                       {Key: "registration.entry_flow", Value: "native"},
			"registration.token_completion_max_concurrency": {Key: "registration.token_completion_max_concurrency", Value: "4"},
			"registration.sleep_min":                        {Key: "registration.sleep_min", Value: "8"},
			"registration.sleep_max":                        {Key: "registration.sleep_max", Value: "16"},
			"webui.host":                                    {Key: "webui.host", Value: "0.0.0.0"},
			"webui.port":                                    {Key: "webui.port", Value: "8000"},
			"webui.access_password":                         {Key: "webui.access_password", Value: "admin123"},
			"app.debug":                                     {Key: "app.debug", Value: "false"},
			"tempmail.enabled":                              {Key: "tempmail.enabled", Value: "true"},
			"tempmail.base_url":                             {Key: "tempmail.base_url", Value: "https://api.tempmail.example/v2"},
			"tempmail.timeout":                              {Key: "tempmail.timeout", Value: "30"},
			"tempmail.max_retries":                          {Key: "tempmail.max_retries", Value: "3"},
			"yyds_mail.enabled":                             {Key: "yyds_mail.enabled", Value: "true"},
			"yyds_mail.base_url":                            {Key: "yyds_mail.base_url", Value: "https://mali.example/v1"},
			"yyds_mail.api_key":                             {Key: "yyds_mail.api_key", Value: "yyds-key"},
			"yyds_mail.default_domain":                      {Key: "yyds_mail.default_domain", Value: "public.example.com"},
			"yyds_mail.timeout":                             {Key: "yyds_mail.timeout", Value: "30"},
			"yyds_mail.max_retries":                         {Key: "yyds_mail.max_retries", Value: "3"},
			"email_code.timeout":                            {Key: "email_code.timeout", Value: "240"},
			"email_code.poll_interval":                      {Key: "email_code.poll_interval", Value: "5"},
			"outlook.provider_priority":                     {Key: "outlook.provider_priority", Value: `["imap_old","graph_api"]`},
			"outlook.health_failure_threshold":              {Key: "outlook.health_failure_threshold", Value: "5"},
			"outlook.health_disable_duration":               {Key: "outlook.health_disable_duration", Value: "60"},
			"outlook.default_client_id":                     {Key: "outlook.default_client_id", Value: "client-123"},
		},
		proxies: []settingspkg.ProxyRecord{
			{
				ID:        7,
				Name:      "默认代理",
				Type:      "http",
				Host:      "127.0.0.1",
				Port:      7890,
				Enabled:   true,
				IsDefault: true,
				Priority:  1,
				LastUsed:  timePtr(now.Add(-5 * time.Minute)),
				CreatedAt: timePtr(now.Add(-24 * time.Hour)),
				UpdatedAt: timePtr(now),
			},
		},
	}

	settingsService := settingspkg.NewService(settingspkg.ServiceDependencies{
		Repository: settingsRepository,
		DatabaseAdmin: fakeSettingsDatabaseAdmin{
			info: settingspkg.DatabaseInfoResponse{
				DatabaseURL:        "postgres://codex-console",
				DatabaseSizeBytes:  12 * 1024 * 1024,
				DatabaseSizeMB:     12,
				AccountsCount:      12,
				EmailServicesCount: 4,
				TasksCount:         6,
			},
			cleanup: settingspkg.DatabaseCleanupResponse{
				Success:      true,
				Message:      "数据库清理完成",
				DeletedCount: 3,
			},
		},
		DynamicProxyTester: fakeDynamicProxyTester{
			response: settingspkg.DynamicProxyTestResponse{
				Success:      true,
				ProxyURL:     "http://proxy.example.com:7890",
				IP:           "1.2.3.4",
				ResponseTime: 120,
				Message:      "动态代理可用",
			},
		},
		ProxyTester: fakeProxyTester{
			response: settingspkg.ProxyTestResult{
				Success:      true,
				IP:           "1.2.3.4",
				ResponseTime: 120,
				Message:      "代理可用",
			},
		},
	})

	emailService := emailservicespkg.NewService(&fakeEmailRepository{
		services: []emailservicespkg.EmailServiceRecord{
			{
				ID:          11,
				ServiceType: emailservicespkg.ServiceTypeOutlook,
				Name:        "outlook-primary",
				Enabled:     true,
				Priority:    1,
				Config: map[string]any{
					"email":         "outlook@example.com",
					"client_id":     "client-123",
					"refresh_token": "refresh-11",
				},
				LastUsed:  timePtr(now.Add(-2 * time.Hour)),
				CreatedAt: timePtr(now.Add(-48 * time.Hour)),
				UpdatedAt: timePtr(now),
			},
			{
				ID:          12,
				ServiceType: emailservicespkg.ServiceTypeMoeMail,
				Name:        "custom-mail",
				Enabled:     true,
				Priority:    2,
				Config: map[string]any{
					"base_url":       "https://mail.example.com",
					"default_domain": "mail.example.com",
					"api_key":        "mail-key",
				},
				LastUsed:  timePtr(now.Add(-4 * time.Hour)),
				CreatedAt: timePtr(now.Add(-72 * time.Hour)),
				UpdatedAt: timePtr(now.Add(-30 * time.Minute)),
			},
		},
		settings: map[string]string{
			"tempmail.enabled":  "true",
			"yyds_mail.enabled": "true",
			"yyds_mail.api_key": "yyds-key",
		},
		registered: []emailservicespkg.RegisteredAccountRecord{
			{ID: 501, Email: "outlook@example.com"},
		},
	}, nil)

	uploaderService := uploaderpkg.NewService(&fakeUploaderRepository{
		configs: []uploaderpkg.ManagedServiceConfig{
			{
				ServiceConfig: uploaderpkg.ServiceConfig{
					ID:         1,
					Kind:       uploaderpkg.UploadKindCPA,
					Name:       "CPA Main",
					BaseURL:    "https://cpa.example.com",
					Credential: "cpa-token",
					Enabled:    true,
					Priority:   5,
				},
			},
			{
				ServiceConfig: uploaderpkg.ServiceConfig{
					ID:         2,
					Kind:       uploaderpkg.UploadKindSub2API,
					Name:       "Sub2API Main",
					BaseURL:    "https://sub2api.example.com",
					Credential: "sub2-key",
					TargetType: "sub2api",
					Enabled:    true,
					Priority:   6,
				},
			},
			{
				ServiceConfig: uploaderpkg.ServiceConfig{
					ID:         3,
					Kind:       uploaderpkg.UploadKindTM,
					Name:       "TM Main",
					BaseURL:    "https://tm.example.com",
					Credential: "tm-key",
					Enabled:    true,
					Priority:   7,
				},
			},
		},
	})

	logsService := logspkg.NewService(&fakeLogsRepository{
		logs: []logspkg.AppLogRecord{
			{
				ID:        1,
				Level:     "INFO",
				Logger:    "worker",
				Message:   "registration finished",
				Exception: "",
				CreatedAt: now,
			},
			{
				ID:        2,
				Level:     "ERROR",
				Logger:    "worker",
				Message:   "upload failed",
				Exception: "boom",
				CreatedAt: now.Add(-1 * time.Hour),
			},
		},
		stats: logspkg.LogsStats{
			Total:    2,
			LatestAt: timePtr(now),
			Levels:   map[string]int{"INFO": 1, "ERROR": 1},
		},
		cleanup: logspkg.CleanupResult{
			RetentionDays:  30,
			MaxRows:        50000,
			DeletedByAge:   1,
			DeletedByLimit: 1,
			DeletedTotal:   2,
			Remaining:      7,
		},
		clearDeletedTotal: 2,
	})

	return internalhttp.NewRouter(nil, settingsService, emailService, uploaderService, logsService)
}

func mustRequestJSON(t *testing.T, server *httptest.Server, method string, path string, body any) any {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body for %s %s: %v", method, path, err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, server.URL+path, reqBody)
	if err != nil {
		t.Fatalf("build %s %s request: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected %s %s 200, got %d", method, path, resp.StatusCode)
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	return payload
}

type fakeSettingsRepository struct {
	settings map[string]settingspkg.SettingRecord
	proxies  []settingspkg.ProxyRecord
}

func (f *fakeSettingsRepository) GetSettings(_ context.Context, keys []string) (map[string]settingspkg.SettingRecord, error) {
	result := make(map[string]settingspkg.SettingRecord, len(keys))
	for _, key := range keys {
		if record, ok := f.settings[key]; ok {
			result[key] = record
		}
	}
	return result, nil
}

func (f *fakeSettingsRepository) UpsertSettings(_ context.Context, records []settingspkg.SettingRecord) error {
	if f.settings == nil {
		f.settings = map[string]settingspkg.SettingRecord{}
	}
	for _, record := range records {
		f.settings[record.Key] = record
	}
	return nil
}

func (f *fakeSettingsRepository) ListProxies(_ context.Context, enabled *bool) ([]settingspkg.ProxyRecord, error) {
	if enabled == nil {
		return append([]settingspkg.ProxyRecord(nil), f.proxies...), nil
	}
	filtered := make([]settingspkg.ProxyRecord, 0, len(f.proxies))
	for _, proxy := range f.proxies {
		if proxy.Enabled == *enabled {
			filtered = append(filtered, proxy)
		}
	}
	return filtered, nil
}

func (f *fakeSettingsRepository) CreateProxy(_ context.Context, req settingspkg.CreateProxyRequest) (settingspkg.ProxyRecord, error) {
	record := settingspkg.ProxyRecord{ID: len(f.proxies) + 1, Name: req.Name, Type: req.Type, Host: req.Host, Port: req.Port, Enabled: req.Enabled}
	f.proxies = append(f.proxies, record)
	return record, nil
}

func (f *fakeSettingsRepository) GetProxyByID(_ context.Context, proxyID int) (settingspkg.ProxyRecord, bool, error) {
	for _, proxy := range f.proxies {
		if proxy.ID == proxyID {
			return proxy, true, nil
		}
	}
	return settingspkg.ProxyRecord{}, false, nil
}

func (f *fakeSettingsRepository) UpdateProxy(_ context.Context, proxyID int, _ settingspkg.UpdateProxyRequest) (settingspkg.ProxyRecord, bool, error) {
	return f.GetProxyByID(context.Background(), proxyID)
}

func (f *fakeSettingsRepository) DeleteProxy(_ context.Context, proxyID int) (bool, error) {
	for idx, proxy := range f.proxies {
		if proxy.ID == proxyID {
			f.proxies = append(f.proxies[:idx], f.proxies[idx+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeSettingsRepository) SetProxyDefault(_ context.Context, proxyID int) (settingspkg.ProxyRecord, bool, error) {
	for idx, proxy := range f.proxies {
		if proxy.ID == proxyID {
			proxy.IsDefault = true
			f.proxies[idx] = proxy
			return proxy, true, nil
		}
	}
	return settingspkg.ProxyRecord{}, false, nil
}

type fakeSettingsDatabaseAdmin struct {
	info    settingspkg.DatabaseInfoResponse
	cleanup settingspkg.DatabaseCleanupResponse
}

func (f fakeSettingsDatabaseAdmin) GetInfo(context.Context) (settingspkg.DatabaseInfoResponse, error) {
	return f.info, nil
}

func (f fakeSettingsDatabaseAdmin) Backup(context.Context) (settingspkg.DatabaseBackupResponse, error) {
	return settingspkg.DatabaseBackupResponse{Success: true, Message: "数据库备份成功", BackupPath: "backups/test.json"}, nil
}

func (f fakeSettingsDatabaseAdmin) Import(context.Context, settingspkg.DatabaseImportRequest) (settingspkg.DatabaseImportResponse, error) {
	return settingspkg.DatabaseImportResponse{Success: true, Message: "数据库导入成功", BackupPath: "backups/pre-import.json"}, nil
}

func (f fakeSettingsDatabaseAdmin) Cleanup(context.Context, settingspkg.DatabaseCleanupRequest) (settingspkg.DatabaseCleanupResponse, error) {
	return f.cleanup, nil
}

type fakeDynamicProxyTester struct {
	response settingspkg.DynamicProxyTestResponse
}

func (f fakeDynamicProxyTester) TestDynamicProxy(context.Context, settingspkg.UpdateDynamicProxySettingsRequest) (settingspkg.DynamicProxyTestResponse, error) {
	return f.response, nil
}

type fakeProxyTester struct {
	response settingspkg.ProxyTestResult
}

func (f fakeProxyTester) TestProxy(_ context.Context, proxy settingspkg.ProxyRecord) (settingspkg.ProxyTestResult, error) {
	result := f.response
	result.ID = proxy.ID
	result.Name = proxy.Name
	return result, nil
}

type fakeEmailRepository struct {
	services    []emailservicespkg.EmailServiceRecord
	settings    map[string]string
	registered  []emailservicespkg.RegisteredAccountRecord
	serviceByID map[int]emailservicespkg.EmailServiceRecord
}

func (f *fakeEmailRepository) ListServices(_ context.Context, req emailservicespkg.ListServicesRequest) ([]emailservicespkg.EmailServiceRecord, error) {
	rows := make([]emailservicespkg.EmailServiceRecord, 0, len(f.services))
	for _, service := range f.services {
		if req.ServiceType != "" && service.ServiceType != req.ServiceType {
			continue
		}
		if req.EnabledOnly && !service.Enabled {
			continue
		}
		rows = append(rows, service)
	}
	return rows, nil
}

func (f *fakeEmailRepository) GetService(_ context.Context, serviceID int) (emailservicespkg.EmailServiceRecord, bool, error) {
	for _, service := range f.services {
		if service.ID == serviceID {
			return service, true, nil
		}
	}
	return emailservicespkg.EmailServiceRecord{}, false, nil
}

func (f *fakeEmailRepository) FindServiceByName(_ context.Context, name string) (emailservicespkg.EmailServiceRecord, bool, error) {
	for _, service := range f.services {
		if service.Name == name {
			return service, true, nil
		}
	}
	return emailservicespkg.EmailServiceRecord{}, false, nil
}

func (f *fakeEmailRepository) CreateService(_ context.Context, service emailservicespkg.EmailServiceRecord) (emailservicespkg.EmailServiceRecord, error) {
	f.services = append(f.services, service)
	return service, nil
}

func (f *fakeEmailRepository) SaveService(_ context.Context, service emailservicespkg.EmailServiceRecord) (emailservicespkg.EmailServiceRecord, error) {
	for idx, current := range f.services {
		if current.ID == service.ID {
			f.services[idx] = service
			return service, nil
		}
	}
	return service, nil
}

func (f *fakeEmailRepository) DeleteService(_ context.Context, serviceID int) (emailservicespkg.EmailServiceRecord, bool, error) {
	for idx, service := range f.services {
		if service.ID == serviceID {
			f.services = append(f.services[:idx], f.services[idx+1:]...)
			return service, true, nil
		}
	}
	return emailservicespkg.EmailServiceRecord{}, false, nil
}

func (f *fakeEmailRepository) UpdateServicePriority(_ context.Context, serviceID int, priority int) error {
	for idx, service := range f.services {
		if service.ID == serviceID {
			service.Priority = priority
			f.services[idx] = service
			break
		}
	}
	return nil
}

func (f *fakeEmailRepository) CountServices(_ context.Context) (map[string]int, int, error) {
	counts := map[string]int{}
	enabledCount := 0
	for _, service := range f.services {
		counts[service.ServiceType]++
		if service.Enabled {
			enabledCount++
		}
	}
	return counts, enabledCount, nil
}

func (f *fakeEmailRepository) GetSettings(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = f.settings[key]
	}
	return result, nil
}

func (f *fakeEmailRepository) ListRegisteredAccountsByEmails(_ context.Context, emails []string) ([]emailservicespkg.RegisteredAccountRecord, error) {
	lookup := map[string]struct{}{}
	for _, email := range emails {
		lookup[email] = struct{}{}
	}
	rows := make([]emailservicespkg.RegisteredAccountRecord, 0, len(f.registered))
	for _, record := range f.registered {
		if _, ok := lookup[record.Email]; ok {
			rows = append(rows, record)
		}
	}
	return rows, nil
}

type fakeUploaderRepository struct {
	configs []uploaderpkg.ManagedServiceConfig
}

func (f *fakeUploaderRepository) ListServiceConfigs(_ context.Context, kind uploaderpkg.UploadKind, filter uploaderpkg.ServiceConfigListFilter) ([]uploaderpkg.ManagedServiceConfig, error) {
	rows := make([]uploaderpkg.ManagedServiceConfig, 0, len(f.configs))
	for _, config := range f.configs {
		if config.Kind != kind {
			continue
		}
		if filter.Enabled != nil && config.Enabled != *filter.Enabled {
			continue
		}
		rows = append(rows, config)
	}
	return rows, nil
}

func (f *fakeUploaderRepository) GetServiceConfig(_ context.Context, kind uploaderpkg.UploadKind, id int) (uploaderpkg.ManagedServiceConfig, bool, error) {
	for _, config := range f.configs {
		if config.Kind == kind && config.ID == id {
			return config, true, nil
		}
	}
	return uploaderpkg.ManagedServiceConfig{}, false, nil
}

func (f *fakeUploaderRepository) CreateServiceConfig(_ context.Context, config uploaderpkg.ManagedServiceConfig) (uploaderpkg.ManagedServiceConfig, error) {
	f.configs = append(f.configs, config)
	return config, nil
}

func (f *fakeUploaderRepository) UpdateServiceConfig(_ context.Context, kind uploaderpkg.UploadKind, id int, _ uploaderpkg.ManagedServiceConfigPatch) (uploaderpkg.ManagedServiceConfig, bool, error) {
	return f.GetServiceConfig(context.Background(), kind, id)
}

func (f *fakeUploaderRepository) DeleteServiceConfig(_ context.Context, kind uploaderpkg.UploadKind, id int) (uploaderpkg.ManagedServiceConfig, bool, error) {
	for idx, config := range f.configs {
		if config.Kind == kind && config.ID == id {
			f.configs = append(f.configs[:idx], f.configs[idx+1:]...)
			return config, true, nil
		}
	}
	return uploaderpkg.ManagedServiceConfig{}, false, nil
}

type fakeLogsRepository struct {
	logs              []logspkg.AppLogRecord
	stats             logspkg.LogsStats
	cleanup           logspkg.CleanupResult
	clearDeletedTotal int
}

func (f *fakeLogsRepository) ListLogs(_ context.Context, req logspkg.ListLogsRequest) ([]logspkg.AppLogRecord, int, error) {
	normalized := req.Normalized()
	rows := make([]logspkg.AppLogRecord, 0, len(f.logs))
	for _, record := range f.logs {
		if normalized.Level != "" && record.Level != normalized.Level {
			continue
		}
		rows = append(rows, record)
	}
	return rows, len(rows), nil
}

func (f *fakeLogsRepository) GetStats(context.Context) (logspkg.LogsStats, error) {
	return f.stats, nil
}

func (f *fakeLogsRepository) CleanupLogs(context.Context, logspkg.CleanupRequest) (logspkg.CleanupResult, error) {
	return f.cleanup, nil
}

func (f *fakeLogsRepository) ClearLogs(context.Context) (int, error) {
	return f.clearDeletedTotal, nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}
