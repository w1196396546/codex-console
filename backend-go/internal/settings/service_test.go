package settings

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"
)

func TestSettingsServiceBuildsCompatibilityPayloadFromDbKeys(t *testing.T) {
	repo := &fakeRepository{
		settings: map[string]SettingRecord{
			"proxy.enabled":                                 {Key: "proxy.enabled", Value: "true"},
			"proxy.type":                                    {Key: "proxy.type", Value: "socks5"},
			"proxy.host":                                    {Key: "proxy.host", Value: "proxy.example.com"},
			"proxy.port":                                    {Key: "proxy.port", Value: "1080"},
			"proxy.username":                                {Key: "proxy.username", Value: "alice"},
			"proxy.password":                                {Key: "proxy.password", Value: "secret"},
			"proxy.dynamic_enabled":                         {Key: "proxy.dynamic_enabled", Value: "true"},
			"proxy.dynamic_api_url":                         {Key: "proxy.dynamic_api_url", Value: "https://proxy.example.com"},
			"proxy.dynamic_api_key_header":                  {Key: "proxy.dynamic_api_key_header", Value: "X-Token"},
			"proxy.dynamic_result_field":                    {Key: "proxy.dynamic_result_field", Value: "data.proxy"},
			"proxy.dynamic_api_key":                         {Key: "proxy.dynamic_api_key", Value: "secret-token"},
			"registration.max_retries":                      {Key: "registration.max_retries", Value: "5"},
			"registration.timeout":                          {Key: "registration.timeout", Value: "180"},
			"registration.default_password_length":          {Key: "registration.default_password_length", Value: "18"},
			"registration.sleep_min":                        {Key: "registration.sleep_min", Value: "8"},
			"registration.sleep_max":                        {Key: "registration.sleep_max", Value: "16"},
			"registration.entry_flow":                       {Key: "registration.entry_flow", Value: "outlook"},
			"registration.token_completion_max_concurrency": {Key: "registration.token_completion_max_concurrency", Value: "4"},
			"webui.host":                                    {Key: "webui.host", Value: "0.0.0.0"},
			"webui.port":                                    {Key: "webui.port", Value: "1455"},
			"webui.access_password":                         {Key: "webui.access_password", Value: "admin123"},
			"tempmail.enabled":                              {Key: "tempmail.enabled", Value: "false"},
			"tempmail.base_url":                             {Key: "tempmail.base_url", Value: "https://api.tempmail.example/v2"},
			"tempmail.timeout":                              {Key: "tempmail.timeout", Value: "45"},
			"tempmail.max_retries":                          {Key: "tempmail.max_retries", Value: "7"},
			"yyds_mail.enabled":                             {Key: "yyds_mail.enabled", Value: "true"},
			"yyds_mail.base_url":                            {Key: "yyds_mail.base_url", Value: "https://mali.example/v1"},
			"yyds_mail.api_key":                             {Key: "yyds_mail.api_key", Value: "saved-key"},
			"yyds_mail.default_domain":                      {Key: "yyds_mail.default_domain", Value: "public.example.com"},
			"yyds_mail.timeout":                             {Key: "yyds_mail.timeout", Value: "60"},
			"yyds_mail.max_retries":                         {Key: "yyds_mail.max_retries", Value: "9"},
			"email_code.timeout":                            {Key: "email_code.timeout", Value: "240"},
			"email_code.poll_interval":                      {Key: "email_code.poll_interval", Value: "5"},
		},
	}

	service := NewService(ServiceDependencies{Repository: repo})
	resp, err := service.GetAllSettings(context.Background())
	if err != nil {
		t.Fatalf("GetAllSettings error: %v", err)
	}

	if !resp.Proxy.Enabled || resp.Proxy.Type != "socks5" || resp.Proxy.Host != "proxy.example.com" || resp.Proxy.Port != 1080 {
		t.Fatalf("unexpected proxy payload: %+v", resp.Proxy)
	}
	if !resp.Proxy.HasPassword || !resp.Proxy.DynamicEnabled || !resp.Proxy.HasDynamicAPIKey {
		t.Fatalf("expected secret flags to be projected, got %+v", resp.Proxy)
	}
	if resp.Proxy.DynamicAPIKeyHeader != "X-Token" || resp.Proxy.DynamicResultField != "data.proxy" {
		t.Fatalf("unexpected dynamic proxy payload: %+v", resp.Proxy)
	}

	if resp.Registration.MaxRetries != 5 || resp.Registration.Timeout != 180 || resp.Registration.DefaultPasswordLength != 18 {
		t.Fatalf("unexpected registration payload: %+v", resp.Registration)
	}
	if resp.Registration.EntryFlow != "native" {
		t.Fatalf("expected outlook entry flow to normalize to native, got %+v", resp.Registration)
	}
	if resp.Registration.TokenCompletionMaxConcurrency != 4 {
		t.Fatalf("unexpected token completion concurrency: %+v", resp.Registration)
	}

	if resp.WebUI.Port != 1455 || !resp.WebUI.HasAccessPassword {
		t.Fatalf("unexpected webui payload: %+v", resp.WebUI)
	}

	if resp.Tempmail.APIURL != "https://api.tempmail.example/v2" || resp.Tempmail.BaseURL != "https://api.tempmail.example/v2" {
		t.Fatalf("unexpected tempmail payload: %+v", resp.Tempmail)
	}
	if resp.Tempmail.Enabled {
		t.Fatalf("expected tempmail enabled=false, got %+v", resp.Tempmail)
	}
	if !resp.YYDSMail.Enabled || !resp.YYDSMail.HasAPIKey || resp.YYDSMail.DefaultDomain != "public.example.com" {
		t.Fatalf("unexpected yyds payload: %+v", resp.YYDSMail)
	}
	if resp.EmailCode.Timeout != 240 || resp.EmailCode.PollInterval != 5 {
		t.Fatalf("unexpected email code payload: %+v", resp.EmailCode)
	}
}

func TestSettingsServicePersistsCurrentDbKeysAndMetadataForTempmailAndDynamicProxy(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(ServiceDependencies{Repository: repo})

	_, err := service.UpdateTempmailSettings(context.Background(), UpdateTempmailSettingsRequest{
		APIURL:            stringPtr("https://api.tempmail.lol/v2"),
		Enabled:           boolPtr(true),
		YYDSAPIURL:        stringPtr("https://maliapi.215.im/v1"),
		YYDSAPIKey:        stringPtr("AC-secret"),
		YYDSDefaultDomain: stringPtr("public.example.com"),
		YYDSEnabled:       boolPtr(true),
	})
	if err != nil {
		t.Fatalf("UpdateTempmailSettings error: %v", err)
	}

	_, err = service.UpdateDynamicProxySettings(context.Background(), UpdateDynamicProxySettingsRequest{
		Enabled:      true,
		APIURL:       "https://proxy.example.com",
		APIKey:       stringPtr("saved-key"),
		APIKeyHeader: "X-Token",
		ResultField:  "data.proxy",
	})
	if err != nil {
		t.Fatalf("UpdateDynamicProxySettings error: %v", err)
	}

	gotKeys := make([]string, 0, len(repo.savedSettings))
	gotMetadata := map[string]SettingRecord{}
	for _, item := range repo.savedSettings {
		gotKeys = append(gotKeys, item.Key)
		gotMetadata[item.Key] = item
	}

	wantKeys := []string{
		"tempmail.base_url",
		"tempmail.enabled",
		"yyds_mail.api_key",
		"yyds_mail.base_url",
		"yyds_mail.default_domain",
		"yyds_mail.enabled",
		"proxy.dynamic_api_key",
		"proxy.dynamic_api_key_header",
		"proxy.dynamic_api_url",
		"proxy.dynamic_enabled",
		"proxy.dynamic_result_field",
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("unexpected saved keys:\nwant %#v\ngot  %#v", wantKeys, gotKeys)
	}

	if gotMetadata["tempmail.base_url"].Category != "tempmail" || gotMetadata["tempmail.base_url"].Description == "" {
		t.Fatalf("expected tempmail metadata to be populated, got %+v", gotMetadata["tempmail.base_url"])
	}
	if gotMetadata["proxy.dynamic_api_key"].Category != "proxy" || gotMetadata["proxy.dynamic_api_key"].Description == "" {
		t.Fatalf("expected dynamic proxy metadata to be populated, got %+v", gotMetadata["proxy.dynamic_api_key"])
	}
}

func TestProxyServiceMaintainsLegacyCrudAndDefaultSemantics(t *testing.T) {
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		proxies: []ProxyRecord{
			{ID: 1, Name: "proxy-a", Type: "http", Host: "127.0.0.1", Port: 8080, Enabled: true, IsDefault: true, LastUsed: &now},
			{ID: 2, Name: "proxy-b", Type: "socks5", Host: "10.0.0.2", Port: 1080, Username: stringPtr("alice"), Password: stringPtr("secret"), Enabled: true},
		},
	}
	service := NewService(ServiceDependencies{Repository: repo})

	created, err := service.CreateProxy(context.Background(), CreateProxyRequest{
		Name:     "proxy-c",
		Type:     "http",
		Host:     "10.0.0.3",
		Port:     3128,
		Username: stringPtr("bob"),
		Password: stringPtr("pwd"),
		Enabled:  true,
		Priority: 0,
	})
	if err != nil {
		t.Fatalf("CreateProxy error: %v", err)
	}
	if created.Name != "proxy-c" || created.Type != "http" || created.Port != 3128 || !created.HasPassword {
		t.Fatalf("unexpected created proxy payload: %+v", created)
	}

	detail, err := service.GetProxy(context.Background(), 2, true)
	if err != nil {
		t.Fatalf("GetProxy error: %v", err)
	}
	if detail.Password == nil || *detail.Password != "secret" {
		t.Fatalf("expected include_password response, got %+v", detail)
	}

	if _, err := service.SetProxyDefault(context.Background(), 2); err != nil {
		t.Fatalf("SetProxyDefault error: %v", err)
	}
	if _, err := service.DisableProxy(context.Background(), 1); err != nil {
		t.Fatalf("DisableProxy error: %v", err)
	}
	if _, err := service.EnableProxy(context.Background(), 1); err != nil {
		t.Fatalf("EnableProxy error: %v", err)
	}

	list, err := service.ListProxies(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListProxies error: %v", err)
	}
	if list.Total != 3 {
		t.Fatalf("expected total=3, got %+v", list)
	}
	if list.Proxies[1].Type != "socks5" || list.Proxies[1].Username == nil || *list.Proxies[1].Username != "alice" {
		t.Fatalf("expected proxy credentials to remain intact, got %+v", list.Proxies[1])
	}
	if !list.Proxies[1].IsDefault {
		t.Fatalf("expected proxy-b to become default, got %+v", list.Proxies[1])
	}
	if !list.Proxies[1].HasPassword {
		t.Fatalf("expected has_password projection, got %+v", list.Proxies[1])
	}
}

func TestDatabaseServiceDelegatesInfoBackupImportAndCleanupToAdmin(t *testing.T) {
	admin := &fakeDatabaseAdmin{
		info: DatabaseInfoResponse{
			DatabaseURL:        "postgres://codex:test@localhost:5432/codex",
			DatabaseSizeBytes:  2048,
			DatabaseSizeMB:     2.0,
			AccountsCount:      12,
			EmailServicesCount: 4,
			TasksCount:         7,
		},
		backup: DatabaseBackupResponse{
			Success:    true,
			Message:    "数据库备份成功",
			BackupPath: "/tmp/database-backup.json",
		},
		importResult: DatabaseImportResponse{
			Success:    true,
			Message:    "数据库导入成功",
			BackupPath: "/tmp/database-before-import.json",
		},
		cleanup: DatabaseCleanupResponse{
			Success:      true,
			Message:      "已清理 3 条过期任务记录",
			DeletedCount: 3,
		},
	}

	service := NewService(ServiceDependencies{DatabaseAdmin: admin})

	info, err := service.GetDatabaseInfo(context.Background())
	if err != nil {
		t.Fatalf("GetDatabaseInfo error: %v", err)
	}
	if info.DatabaseSizeMB != 2.0 || info.AccountsCount != 12 || info.TasksCount != 7 {
		t.Fatalf("unexpected database info payload: %+v", info)
	}

	backup, err := service.BackupDatabase(context.Background())
	if err != nil {
		t.Fatalf("BackupDatabase error: %v", err)
	}
	if !backup.Success || backup.BackupPath != "/tmp/database-backup.json" {
		t.Fatalf("unexpected backup payload: %+v", backup)
	}

	importResult, err := service.ImportDatabase(context.Background(), DatabaseImportRequest{
		Filename: "backup.json",
		Content:  []byte(`{"format":"codex-console-postgres-backup.v1"}`),
	})
	if err != nil {
		t.Fatalf("ImportDatabase error: %v", err)
	}
	if !importResult.Success || admin.lastImport.Filename != "backup.json" || !bytes.Equal(admin.lastImport.Content, []byte(`{"format":"codex-console-postgres-backup.v1"}`)) {
		t.Fatalf("unexpected import payload: %+v admin=%+v", importResult, admin.lastImport)
	}

	cleanup, err := service.CleanupDatabase(context.Background(), DatabaseCleanupRequest{Days: 30, KeepFailed: true})
	if err != nil {
		t.Fatalf("CleanupDatabase error: %v", err)
	}
	if cleanup.DeletedCount != 3 || admin.lastCleanup.Days != 30 || !admin.lastCleanup.KeepFailed {
		t.Fatalf("unexpected cleanup payload: %+v admin=%+v", cleanup, admin.lastCleanup)
	}
}

type fakeRepository struct {
	settings      map[string]SettingRecord
	savedSettings []SettingRecord
	proxies       []ProxyRecord
}

func (f *fakeRepository) GetSettings(_ context.Context, keys []string) (map[string]SettingRecord, error) {
	result := make(map[string]SettingRecord, len(keys))
	for _, key := range keys {
		if record, ok := f.settings[key]; ok {
			result[key] = record
		}
	}
	return result, nil
}

func (f *fakeRepository) UpsertSettings(_ context.Context, settings []SettingRecord) error {
	f.savedSettings = append(f.savedSettings, settings...)
	return nil
}

func (f *fakeRepository) ListProxies(_ context.Context, _ *bool) ([]ProxyRecord, error) {
	result := make([]ProxyRecord, len(f.proxies))
	copy(result, f.proxies)
	return result, nil
}

func (f *fakeRepository) CreateProxy(_ context.Context, req CreateProxyRequest) (ProxyRecord, error) {
	record := ProxyRecord{
		ID:        len(f.proxies) + 1,
		Name:      req.Name,
		Type:      req.Type,
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		Password:  req.Password,
		Enabled:   req.Enabled,
		IsDefault: false,
		Priority:  req.Priority,
	}
	f.proxies = append(f.proxies, record)
	return record, nil
}

func (f *fakeRepository) GetProxyByID(_ context.Context, proxyID int) (ProxyRecord, bool, error) {
	for _, proxy := range f.proxies {
		if proxy.ID == proxyID {
			return proxy, true, nil
		}
	}
	return ProxyRecord{}, false, nil
}

func (f *fakeRepository) UpdateProxy(_ context.Context, proxyID int, req UpdateProxyRequest) (ProxyRecord, bool, error) {
	for idx, proxy := range f.proxies {
		if proxy.ID != proxyID {
			continue
		}
		if req.Name != nil {
			proxy.Name = *req.Name
		}
		if req.Type != nil {
			proxy.Type = *req.Type
		}
		if req.Host != nil {
			proxy.Host = *req.Host
		}
		if req.Port != nil {
			proxy.Port = *req.Port
		}
		if req.Username != nil {
			proxy.Username = req.Username
		}
		if req.Password != nil {
			proxy.Password = req.Password
		}
		if req.Enabled != nil {
			proxy.Enabled = *req.Enabled
		}
		if req.Priority != nil {
			proxy.Priority = *req.Priority
		}
		if req.IsDefault != nil {
			proxy.IsDefault = *req.IsDefault
		}
		f.proxies[idx] = proxy
		return proxy, true, nil
	}
	return ProxyRecord{}, false, nil
}

func (f *fakeRepository) DeleteProxy(_ context.Context, proxyID int) (bool, error) {
	for idx, proxy := range f.proxies {
		if proxy.ID != proxyID {
			continue
		}
		f.proxies = append(f.proxies[:idx], f.proxies[idx+1:]...)
		return true, nil
	}
	return false, nil
}

func (f *fakeRepository) SetProxyDefault(_ context.Context, proxyID int) (ProxyRecord, bool, error) {
	var found *ProxyRecord
	for idx := range f.proxies {
		f.proxies[idx].IsDefault = f.proxies[idx].ID == proxyID
		if f.proxies[idx].ID == proxyID {
			proxy := f.proxies[idx]
			found = &proxy
		}
	}
	if found == nil {
		return ProxyRecord{}, false, nil
	}
	return *found, true, nil
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

type fakeDatabaseAdmin struct {
	info         DatabaseInfoResponse
	backup       DatabaseBackupResponse
	importResult DatabaseImportResponse
	cleanup      DatabaseCleanupResponse
	lastImport   DatabaseImportRequest
	lastCleanup  DatabaseCleanupRequest
}

func (f *fakeDatabaseAdmin) GetInfo(context.Context) (DatabaseInfoResponse, error) {
	return f.info, nil
}

func (f *fakeDatabaseAdmin) Backup(context.Context) (DatabaseBackupResponse, error) {
	return f.backup, nil
}

func (f *fakeDatabaseAdmin) Import(_ context.Context, req DatabaseImportRequest) (DatabaseImportResponse, error) {
	f.lastImport = req
	return f.importResult, nil
}

func (f *fakeDatabaseAdmin) Cleanup(_ context.Context, req DatabaseCleanupRequest) (DatabaseCleanupResponse, error) {
	f.lastCleanup = req
	return f.cleanup, nil
}
