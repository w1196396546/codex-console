package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	settingspkg "github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/go-chi/chi/v5"
)

func TestSettingsHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakeSettingsService{
		allSettings: settingspkg.AllSettingsResponse{
			Proxy: settingspkg.ProxySettingsResponse{
				Enabled:             true,
				Type:                "http",
				Host:                "127.0.0.1",
				Port:                7890,
				HasPassword:         true,
				DynamicEnabled:      true,
				DynamicAPIURL:       "https://proxy.example.com",
				DynamicAPIKeyHeader: "X-Token",
				DynamicResultField:  "data.proxy",
				HasDynamicAPIKey:    true,
			},
			Registration: settingspkg.RegistrationSettingsResponse{
				MaxRetries:                    5,
				Timeout:                       180,
				DefaultPasswordLength:         18,
				SleepMin:                      8,
				SleepMax:                      16,
				EntryFlow:                     "native",
				TokenCompletionMaxConcurrency: 4,
			},
			WebUI: settingspkg.WebUISettingsResponse{
				Host:              "0.0.0.0",
				Port:              1455,
				HasAccessPassword: true,
			},
			Tempmail: settingspkg.TempmailProviderResponse{
				APIURL:     "https://api.tempmail.example/v2",
				BaseURL:    "https://api.tempmail.example/v2",
				Timeout:    45,
				MaxRetries: 7,
				Enabled:    true,
			},
			YYDSMail: settingspkg.YYDSMailResponse{
				APIURL:        "https://mali.example/v1",
				BaseURL:       "https://mali.example/v1",
				DefaultDomain: "public.example.com",
				Enabled:       true,
				HasAPIKey:     true,
			},
			EmailCode: settingspkg.EmailCodeSettingsResponse{Timeout: 240, PollInterval: 5},
		},
		tempmail: settingspkg.TempmailSettingsResponse{
			Tempmail: settingspkg.TempmailProviderResponse{APIURL: "https://api.tempmail.example/v2", BaseURL: "https://api.tempmail.example/v2", Enabled: true},
			YYDSMail: settingspkg.YYDSMailResponse{APIURL: "https://mali.example/v1", BaseURL: "https://mali.example/v1", DefaultDomain: "public.example.com", Enabled: true, HasAPIKey: true},
		},
		dynamicProxy: settingspkg.DynamicProxySettingsResponse{
			Enabled:      true,
			APIURL:       "https://proxy.example.com",
			APIKeyHeader: "X-Token",
			ResultField:  "data.proxy",
			HasAPIKey:    true,
		},
		dynamicProxyTest: settingspkg.DynamicProxyTestResponse{
			Success:      true,
			ProxyURL:     "http://1.2.3.4:8080",
			IP:           "1.2.3.4",
			ResponseTime: 120,
			Message:      "动态代理可用，出口 IP: 1.2.3.4，响应时间: 120ms",
		},
		updatedTempmail: settingspkg.MutationResponse{Success: true, Message: "临时邮箱设置已更新"},
		updatedDynamic:  settingspkg.MutationResponse{Success: true, Message: "动态代理设置已更新"},
		registration: settingspkg.RegistrationSettingsResponse{
			MaxRetries:                    5,
			Timeout:                       180,
			DefaultPasswordLength:         18,
			SleepMin:                      8,
			SleepMax:                      16,
			EntryFlow:                     "native",
			TokenCompletionMaxConcurrency: 4,
		},
		updatedRegistration: settingspkg.MutationResponse{Success: true, Message: "注册设置已更新"},
		emailCode:           settingspkg.EmailCodeSettingsResponse{Timeout: 240, PollInterval: 5},
		updatedEmailCode:    settingspkg.MutationResponse{Success: true, Message: "验证码等待设置已更新"},
		outlook:             settingspkg.OutlookSettingsResponse{DefaultClientID: "client-1"},
		updatedOutlook:      settingspkg.MutationResponse{Success: true, Message: "Outlook 设置已更新"},
		updatedWebUI:        settingspkg.MutationResponse{Success: true, Message: "Web UI 设置已更新"},
	}

	router := chi.NewRouter()
	NewHandler(service).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET /api/settings 200, got %d", rec.Code)
	}

	var allPayload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &allPayload); err != nil {
		t.Fatalf("decode settings payload: %v", err)
	}
	if allPayload["tempmail"] == nil || allPayload["email_code"] == nil {
		t.Fatalf("expected aggregate settings payload to contain tempmail and email_code, got %#v", allPayload)
	}

	assertJSONPost(t, router, "/api/settings/tempmail", `{"api_url":"https://api.tempmail.example/v2","enabled":true,"yyds_api_url":"https://mali.example/v1","yyds_default_domain":"public.example.com","yyds_enabled":true}`, http.StatusOK)
	if service.lastTempmail.APIURL == nil || *service.lastTempmail.APIURL != "https://api.tempmail.example/v2" {
		t.Fatalf("expected tempmail request to be decoded, got %+v", service.lastTempmail)
	}

	assertJSONPost(t, router, "/api/settings/proxy/dynamic", `{"enabled":true,"api_url":"https://proxy.example.com","api_key":"saved-key","api_key_header":"X-Token","result_field":"data.proxy"}`, http.StatusOK)
	if service.lastDynamic.APIURL != "https://proxy.example.com" || service.lastDynamic.APIKey == nil || *service.lastDynamic.APIKey != "saved-key" {
		t.Fatalf("expected dynamic proxy request to be decoded, got %+v", service.lastDynamic)
	}

	assertJSONPost(t, router, "/api/settings/proxy/dynamic/test", `{"api_url":"https://proxy.example.com","api_key":"saved-key","api_key_header":"X-Token","result_field":"data.proxy"}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/registration", `{"max_retries":5,"timeout":180,"default_password_length":18,"sleep_min":8,"sleep_max":16,"entry_flow":"native","token_completion_max_concurrency":4}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/email-code", `{"timeout":240,"poll_interval":5}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/outlook", `{"default_client_id":"client-1"}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/webui", `{"access_password":"admin123"}`, http.StatusOK)
}

func TestProxyHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakeSettingsService{
		proxyList: settingspkg.ProxyListResponse{
			Total: 2,
			Proxies: []settingspkg.ProxyPayload{
				{ID: 1, Name: "proxy-a", Type: "http", Host: "127.0.0.1", Port: 7890, Enabled: true, IsDefault: true, HasPassword: false},
				{ID: 2, Name: "proxy-b", Type: "socks5", Host: "10.0.0.2", Port: 1080, Enabled: true, IsDefault: false, HasPassword: true},
			},
		},
		proxyDetail: settingspkg.ProxyPayload{
			ID:       2,
			Name:     "proxy-b",
			Type:     "socks5",
			Host:     "10.0.0.2",
			Port:     1080,
			Password: stringPtr("secret"),
			Enabled:  true,
		},
		createdProxy: settingspkg.ProxyPayload{ID: 3, Name: "proxy-c", Type: "http", Host: "10.0.0.3", Port: 3128, Enabled: true},
		updatedProxy: settingspkg.ProxyPayload{ID: 2, Name: "proxy-b", Type: "socks5", Host: "10.0.0.2", Port: 1080, Enabled: true, IsDefault: true},
		setDefault:   settingspkg.ProxyPayload{ID: 2, Name: "proxy-b", Type: "socks5", Host: "10.0.0.2", Port: 1080, Enabled: true, IsDefault: true},
		proxyTest:    settingspkg.ProxyTestResponse{Success: true, IP: "1.2.3.4", ResponseTime: 120, Message: "代理连接成功，出口 IP: 1.2.3.4"},
		testAll: settingspkg.ProxyTestAllResponse{
			Total:   2,
			Success: 1,
			Failed:  1,
			Results: []settingspkg.ProxyTestResult{
				{ID: 1, Name: "proxy-a", Success: true, IP: "1.2.3.4", ResponseTime: 120},
				{ID: 2, Name: "proxy-b", Success: false, Message: "dial tcp timeout"},
			},
		},
		deletedProxy:  settingspkg.MutationResponse{Success: true, Message: "代理已删除"},
		enabledProxy:  settingspkg.MutationResponse{Success: true, Message: "代理已启用"},
		disabledProxy: settingspkg.MutationResponse{Success: true, Message: "代理已禁用"},
	}

	router := chi.NewRouter()
	NewHandler(service).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/proxies", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET /api/settings/proxies 200, got %d", rec.Code)
	}

	var listPayload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode proxy list: %v", err)
	}
	if listPayload["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", listPayload["total"])
	}

	assertJSONPost(t, router, "/api/settings/proxies", `{"name":"proxy-c","type":"http","host":"10.0.0.3","port":3128,"enabled":true}`, http.StatusOK)

	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, httptest.NewRequest(http.MethodGet, "/api/settings/proxies/2", nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected GET /api/settings/proxies/2 200, got %d", detailRec.Code)
	}
	var detail settingspkg.ProxyPayload
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode proxy detail: %v", err)
	}
	if detail.Password == nil || *detail.Password != "secret" {
		t.Fatalf("expected include_password detail response, got %+v", detail)
	}

	assertJSON(t, router, http.MethodPatch, "/api/settings/proxies/2", `{"name":"proxy-b","enabled":true}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/proxies/2/set-default", `{}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/proxies/2/test", `{}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/proxies/test-all", `{}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/proxies/2/enable", `{}`, http.StatusOK)
	assertJSONPost(t, router, "/api/settings/proxies/2/disable", `{}`, http.StatusOK)

	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, httptest.NewRequest(http.MethodDelete, "/api/settings/proxies/2", nil))
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected DELETE /api/settings/proxies/2 200, got %d", deleteRec.Code)
	}
}

func TestDatabaseHandlerCompatibilityRoutes(t *testing.T) {
	service := &fakeSettingsService{
		databaseInfo: settingspkg.DatabaseInfoResponse{
			DatabaseURL:        "postgres://codex:test@localhost:5432/codex",
			DatabaseSizeBytes:  2048,
			DatabaseSizeMB:     2.0,
			AccountsCount:      12,
			EmailServicesCount: 4,
			TasksCount:         7,
		},
		backup: settingspkg.DatabaseBackupResponse{
			Success:    true,
			Message:    "数据库备份成功",
			BackupPath: "/tmp/database-backup.json",
		},
		importResult: settingspkg.DatabaseImportResponse{
			Success:    true,
			Message:    "数据库导入成功",
			BackupPath: "/tmp/database-before-import.json",
		},
		cleanup: settingspkg.DatabaseCleanupResponse{
			Success:      true,
			Message:      "已清理 3 条过期任务记录",
			DeletedCount: 3,
		},
	}

	router := chi.NewRouter()
	NewHandler(service).RegisterRoutes(router)

	infoRec := httptest.NewRecorder()
	router.ServeHTTP(infoRec, httptest.NewRequest(http.MethodGet, "/api/settings/database", nil))
	if infoRec.Code != http.StatusOK {
		t.Fatalf("expected GET /api/settings/database 200, got %d", infoRec.Code)
	}

	assertJSONPost(t, router, "/api/settings/database/backup", `{}`, http.StatusOK)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "backup.json")
	if err != nil {
		t.Fatalf("create import form file: %v", err)
	}
	if _, err := part.Write([]byte(`{"format":"codex-console-postgres-backup.v1"}`)); err != nil {
		t.Fatalf("write import payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	importReq := httptest.NewRequest(http.MethodPost, "/api/settings/database/import", &body)
	importReq.Header.Set("Content-Type", writer.FormDataContentType())
	importRec := httptest.NewRecorder()
	router.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected POST /api/settings/database/import 200, got %d", importRec.Code)
	}
	if service.lastImport.Filename != "backup.json" {
		t.Fatalf("expected import request filename to be forwarded, got %+v", service.lastImport)
	}

	cleanupRec := httptest.NewRecorder()
	router.ServeHTTP(cleanupRec, httptest.NewRequest(http.MethodPost, "/api/settings/database/cleanup?days=30", nil))
	if cleanupRec.Code != http.StatusOK {
		t.Fatalf("expected POST /api/settings/database/cleanup 200, got %d", cleanupRec.Code)
	}
	if service.lastCleanup.Days != 30 || !service.lastCleanup.KeepFailed {
		t.Fatalf("expected cleanup request to preserve default keep_failed=true semantics, got %+v", service.lastCleanup)
	}
}

type fakeSettingsService struct {
	allSettings         settingspkg.AllSettingsResponse
	registration        settingspkg.RegistrationSettingsResponse
	tempmail            settingspkg.TempmailSettingsResponse
	emailCode           settingspkg.EmailCodeSettingsResponse
	outlook             settingspkg.OutlookSettingsResponse
	dynamicProxy        settingspkg.DynamicProxySettingsResponse
	dynamicProxyTest    settingspkg.DynamicProxyTestResponse
	proxyList           settingspkg.ProxyListResponse
	proxyDetail         settingspkg.ProxyPayload
	createdProxy        settingspkg.ProxyPayload
	updatedProxy        settingspkg.ProxyPayload
	setDefault          settingspkg.ProxyPayload
	proxyTest           settingspkg.ProxyTestResponse
	testAll             settingspkg.ProxyTestAllResponse
	databaseInfo        settingspkg.DatabaseInfoResponse
	backup              settingspkg.DatabaseBackupResponse
	importResult        settingspkg.DatabaseImportResponse
	cleanup             settingspkg.DatabaseCleanupResponse
	updatedRegistration settingspkg.MutationResponse
	updatedTempmail     settingspkg.MutationResponse
	updatedEmailCode    settingspkg.MutationResponse
	updatedOutlook      settingspkg.MutationResponse
	updatedWebUI        settingspkg.MutationResponse
	updatedDynamic      settingspkg.MutationResponse
	deletedProxy        settingspkg.MutationResponse
	enabledProxy        settingspkg.MutationResponse
	disabledProxy       settingspkg.MutationResponse
	lastRegistration    settingspkg.UpdateRegistrationSettingsRequest
	lastTempmail        settingspkg.UpdateTempmailSettingsRequest
	lastEmailCode       settingspkg.UpdateEmailCodeSettingsRequest
	lastOutlook         settingspkg.UpdateOutlookSettingsRequest
	lastWebUI           settingspkg.UpdateWebUISettingsRequest
	lastDynamic         settingspkg.UpdateDynamicProxySettingsRequest
	lastProxyCreate     settingspkg.CreateProxyRequest
	lastProxyUpdate     settingspkg.UpdateProxyRequest
	lastImport          settingspkg.DatabaseImportRequest
	lastCleanup         settingspkg.DatabaseCleanupRequest
}

func (f *fakeSettingsService) GetAllSettings(context.Context) (settingspkg.AllSettingsResponse, error) {
	return f.allSettings, nil
}

func (f *fakeSettingsService) GetRegistrationSettings(context.Context) (settingspkg.RegistrationSettingsResponse, error) {
	return f.registration, nil
}

func (f *fakeSettingsService) UpdateRegistrationSettings(_ context.Context, req settingspkg.UpdateRegistrationSettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastRegistration = req
	return f.updatedRegistration, nil
}

func (f *fakeSettingsService) GetTempmailSettings(context.Context) (settingspkg.TempmailSettingsResponse, error) {
	return f.tempmail, nil
}

func (f *fakeSettingsService) UpdateTempmailSettings(_ context.Context, req settingspkg.UpdateTempmailSettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastTempmail = req
	return f.updatedTempmail, nil
}

func (f *fakeSettingsService) GetEmailCodeSettings(context.Context) (settingspkg.EmailCodeSettingsResponse, error) {
	return f.emailCode, nil
}

func (f *fakeSettingsService) UpdateEmailCodeSettings(_ context.Context, req settingspkg.UpdateEmailCodeSettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastEmailCode = req
	return f.updatedEmailCode, nil
}

func (f *fakeSettingsService) GetOutlookSettings(context.Context) (settingspkg.OutlookSettingsResponse, error) {
	return f.outlook, nil
}

func (f *fakeSettingsService) UpdateOutlookSettings(_ context.Context, req settingspkg.UpdateOutlookSettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastOutlook = req
	return f.updatedOutlook, nil
}

func (f *fakeSettingsService) UpdateWebUISettings(_ context.Context, req settingspkg.UpdateWebUISettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastWebUI = req
	return f.updatedWebUI, nil
}

func (f *fakeSettingsService) GetDynamicProxySettings(context.Context) (settingspkg.DynamicProxySettingsResponse, error) {
	return f.dynamicProxy, nil
}

func (f *fakeSettingsService) UpdateDynamicProxySettings(_ context.Context, req settingspkg.UpdateDynamicProxySettingsRequest) (settingspkg.MutationResponse, error) {
	f.lastDynamic = req
	return f.updatedDynamic, nil
}

func (f *fakeSettingsService) TestDynamicProxy(context.Context, settingspkg.UpdateDynamicProxySettingsRequest) (settingspkg.DynamicProxyTestResponse, error) {
	return f.dynamicProxyTest, nil
}

func (f *fakeSettingsService) ListProxies(context.Context, *bool) (settingspkg.ProxyListResponse, error) {
	return f.proxyList, nil
}

func (f *fakeSettingsService) CreateProxy(_ context.Context, req settingspkg.CreateProxyRequest) (settingspkg.ProxyPayload, error) {
	f.lastProxyCreate = req
	return f.createdProxy, nil
}

func (f *fakeSettingsService) GetProxy(context.Context, int, bool) (settingspkg.ProxyPayload, error) {
	return f.proxyDetail, nil
}

func (f *fakeSettingsService) UpdateProxy(_ context.Context, _ int, req settingspkg.UpdateProxyRequest) (settingspkg.ProxyPayload, error) {
	f.lastProxyUpdate = req
	return f.updatedProxy, nil
}

func (f *fakeSettingsService) DeleteProxy(context.Context, int) (settingspkg.MutationResponse, error) {
	return f.deletedProxy, nil
}

func (f *fakeSettingsService) EnableProxy(context.Context, int) (settingspkg.MutationResponse, error) {
	return f.enabledProxy, nil
}

func (f *fakeSettingsService) DisableProxy(context.Context, int) (settingspkg.MutationResponse, error) {
	return f.disabledProxy, nil
}

func (f *fakeSettingsService) SetProxyDefault(context.Context, int) (settingspkg.ProxyPayload, error) {
	return f.setDefault, nil
}

func (f *fakeSettingsService) TestProxy(context.Context, int) (settingspkg.ProxyTestResponse, error) {
	return f.proxyTest, nil
}

func (f *fakeSettingsService) TestAllProxies(context.Context) (settingspkg.ProxyTestAllResponse, error) {
	return f.testAll, nil
}

func (f *fakeSettingsService) GetDatabaseInfo(context.Context) (settingspkg.DatabaseInfoResponse, error) {
	return f.databaseInfo, nil
}

func (f *fakeSettingsService) BackupDatabase(context.Context) (settingspkg.DatabaseBackupResponse, error) {
	return f.backup, nil
}

func (f *fakeSettingsService) ImportDatabase(_ context.Context, req settingspkg.DatabaseImportRequest) (settingspkg.DatabaseImportResponse, error) {
	f.lastImport = req
	return f.importResult, nil
}

func (f *fakeSettingsService) CleanupDatabase(_ context.Context, req settingspkg.DatabaseCleanupRequest) (settingspkg.DatabaseCleanupResponse, error) {
	f.lastCleanup = req
	return f.cleanup, nil
}

func assertJSONPost(t *testing.T, router http.Handler, path string, body string, wantStatus int) {
	t.Helper()
	assertJSON(t, router, http.MethodPost, path, body, wantStatus)
}

func assertJSON(t *testing.T, router http.Handler, method string, path string, body string, wantStatus int) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("expected %s %s status=%d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}
}

func stringPtr(value string) *string {
	return &value
}
