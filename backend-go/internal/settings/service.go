package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Repository interface {
	GetSettings(ctx context.Context, keys []string) (map[string]SettingRecord, error)
	UpsertSettings(ctx context.Context, settings []SettingRecord) error
	ListProxies(ctx context.Context, enabled *bool) ([]ProxyRecord, error)
	CreateProxy(ctx context.Context, req CreateProxyRequest) (ProxyRecord, error)
	GetProxyByID(ctx context.Context, proxyID int) (ProxyRecord, bool, error)
	UpdateProxy(ctx context.Context, proxyID int, req UpdateProxyRequest) (ProxyRecord, bool, error)
	DeleteProxy(ctx context.Context, proxyID int) (bool, error)
	SetProxyDefault(ctx context.Context, proxyID int) (ProxyRecord, bool, error)
}

type DatabaseAdmin interface {
	GetInfo(ctx context.Context) (DatabaseInfoResponse, error)
	Backup(ctx context.Context) (DatabaseBackupResponse, error)
	Import(ctx context.Context, req DatabaseImportRequest) (DatabaseImportResponse, error)
	Cleanup(ctx context.Context, req DatabaseCleanupRequest) (DatabaseCleanupResponse, error)
}

type DynamicProxyTester interface {
	TestDynamicProxy(ctx context.Context, req UpdateDynamicProxySettingsRequest) (DynamicProxyTestResponse, error)
}

type ProxyTester interface {
	TestProxy(ctx context.Context, proxy ProxyRecord) (ProxyTestResult, error)
}

type ServiceDependencies struct {
	Repository         Repository
	DatabaseAdmin      DatabaseAdmin
	DynamicProxyTester DynamicProxyTester
	ProxyTester        ProxyTester
}

type Service struct {
	repository         Repository
	databaseAdmin      DatabaseAdmin
	dynamicProxyTester DynamicProxyTester
	proxyTester        ProxyTester
}

func NewService(deps ServiceDependencies) *Service {
	return &Service{
		repository:         deps.Repository,
		databaseAdmin:      deps.DatabaseAdmin,
		dynamicProxyTester: deps.DynamicProxyTester,
		proxyTester:        deps.ProxyTester,
	}
}

func (s *Service) GetAllSettings(ctx context.Context) (AllSettingsResponse, error) {
	settings, err := s.loadSettings(ctx, allSettingsKeys())
	if err != nil {
		return AllSettingsResponse{}, err
	}

	return AllSettingsResponse{
		Proxy: ProxySettingsResponse{
			Enabled:             settingBool(settings, "proxy.enabled"),
			Type:                settingString(settings, "proxy.type"),
			Host:                settingString(settings, "proxy.host"),
			Port:                settingInt(settings, "proxy.port"),
			Username:            settingOptionalString(settings, "proxy.username"),
			HasPassword:         settingHasSecret(settings, "proxy.password"),
			DynamicEnabled:      settingBool(settings, "proxy.dynamic_enabled"),
			DynamicAPIURL:       settingString(settings, "proxy.dynamic_api_url"),
			DynamicAPIKeyHeader: settingString(settings, "proxy.dynamic_api_key_header"),
			DynamicResultField:  settingString(settings, "proxy.dynamic_result_field"),
			HasDynamicAPIKey:    settingHasSecret(settings, "proxy.dynamic_api_key"),
		},
		Registration: RegistrationSettingsResponse{
			MaxRetries:                    settingInt(settings, "registration.max_retries"),
			Timeout:                       settingInt(settings, "registration.timeout"),
			DefaultPasswordLength:         settingInt(settings, "registration.default_password_length"),
			SleepMin:                      settingInt(settings, "registration.sleep_min"),
			SleepMax:                      settingInt(settings, "registration.sleep_max"),
			EntryFlow:                     normalizeRegistrationFlow(settingString(settings, "registration.entry_flow")),
			TokenCompletionMaxConcurrency: settingInt(settings, "registration.token_completion_max_concurrency"),
		},
		WebUI: WebUISettingsResponse{
			Host:              settingString(settings, "webui.host"),
			Port:              settingInt(settings, "webui.port"),
			Debug:             settingBool(settings, "app.debug"),
			HasAccessPassword: settingHasSecret(settings, "webui.access_password"),
		},
		Tempmail: TempmailProviderResponse{
			APIURL:     settingString(settings, "tempmail.base_url"),
			BaseURL:    settingString(settings, "tempmail.base_url"),
			Timeout:    settingInt(settings, "tempmail.timeout"),
			MaxRetries: settingInt(settings, "tempmail.max_retries"),
			Enabled:    settingBool(settings, "tempmail.enabled"),
		},
		YYDSMail: YYDSMailResponse{
			APIURL:        settingString(settings, "yyds_mail.base_url"),
			BaseURL:       settingString(settings, "yyds_mail.base_url"),
			DefaultDomain: settingString(settings, "yyds_mail.default_domain"),
			Timeout:       settingInt(settings, "yyds_mail.timeout"),
			MaxRetries:    settingInt(settings, "yyds_mail.max_retries"),
			Enabled:       settingBool(settings, "yyds_mail.enabled"),
			HasAPIKey:     settingHasSecret(settings, "yyds_mail.api_key"),
		},
		EmailCode: EmailCodeSettingsResponse{
			Timeout:      settingInt(settings, "email_code.timeout"),
			PollInterval: settingInt(settings, "email_code.poll_interval"),
		},
	}, nil
}

func (s *Service) GetRegistrationSettings(ctx context.Context) (RegistrationSettingsResponse, error) {
	settings, err := s.loadSettings(ctx, registrationSettingsKeys())
	if err != nil {
		return RegistrationSettingsResponse{}, err
	}

	return RegistrationSettingsResponse{
		MaxRetries:                    settingInt(settings, "registration.max_retries"),
		Timeout:                       settingInt(settings, "registration.timeout"),
		DefaultPasswordLength:         settingInt(settings, "registration.default_password_length"),
		SleepMin:                      settingInt(settings, "registration.sleep_min"),
		SleepMax:                      settingInt(settings, "registration.sleep_max"),
		EntryFlow:                     normalizeRegistrationFlow(settingString(settings, "registration.entry_flow")),
		TokenCompletionMaxConcurrency: settingInt(settings, "registration.token_completion_max_concurrency"),
	}, nil
}

func (s *Service) UpdateRegistrationSettings(ctx context.Context, req UpdateRegistrationSettingsRequest) (MutationResponse, error) {
	flow, err := validateRegistrationFlow(req.EntryFlow)
	if err != nil {
		return MutationResponse{}, ErrInvalidRegistrationFlow
	}

	records := []SettingRecord{
		s.newSettingRecord("registration.max_retries", req.MaxRetries),
		s.newSettingRecord("registration.timeout", req.Timeout),
		s.newSettingRecord("registration.default_password_length", req.DefaultPasswordLength),
		s.newSettingRecord("registration.sleep_min", req.SleepMin),
		s.newSettingRecord("registration.sleep_max", req.SleepMax),
		s.newSettingRecord("registration.entry_flow", flow),
		s.newSettingRecord("registration.token_completion_max_concurrency", req.TokenCompletionMaxConcurrency),
	}
	if err := s.saveSettings(ctx, records); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "注册设置已更新"}, nil
}

func (s *Service) GetTempmailSettings(ctx context.Context) (TempmailSettingsResponse, error) {
	settings, err := s.loadSettings(ctx, tempmailSettingsKeys())
	if err != nil {
		return TempmailSettingsResponse{}, err
	}

	return TempmailSettingsResponse{
		Tempmail: TempmailProviderResponse{
			APIURL:     settingString(settings, "tempmail.base_url"),
			BaseURL:    settingString(settings, "tempmail.base_url"),
			Timeout:    settingInt(settings, "tempmail.timeout"),
			MaxRetries: settingInt(settings, "tempmail.max_retries"),
			Enabled:    settingBool(settings, "tempmail.enabled"),
		},
		YYDSMail: YYDSMailResponse{
			APIURL:        settingString(settings, "yyds_mail.base_url"),
			BaseURL:       settingString(settings, "yyds_mail.base_url"),
			DefaultDomain: settingString(settings, "yyds_mail.default_domain"),
			Timeout:       settingInt(settings, "yyds_mail.timeout"),
			MaxRetries:    settingInt(settings, "yyds_mail.max_retries"),
			Enabled:       settingBool(settings, "yyds_mail.enabled"),
			HasAPIKey:     settingHasSecret(settings, "yyds_mail.api_key"),
		},
	}, nil
}

func (s *Service) UpdateTempmailSettings(ctx context.Context, req UpdateTempmailSettingsRequest) (MutationResponse, error) {
	records := make([]SettingRecord, 0, 6)
	if req.APIURL != nil {
		records = append(records, s.newSettingRecord("tempmail.base_url", *req.APIURL))
	}
	if req.Enabled != nil {
		records = append(records, s.newSettingRecord("tempmail.enabled", *req.Enabled))
	}
	if req.YYDSAPIKey != nil {
		records = append(records, s.newSettingRecord("yyds_mail.api_key", *req.YYDSAPIKey))
	}
	if req.YYDSAPIURL != nil {
		records = append(records, s.newSettingRecord("yyds_mail.base_url", *req.YYDSAPIURL))
	}
	if req.YYDSDefaultDomain != nil {
		records = append(records, s.newSettingRecord("yyds_mail.default_domain", *req.YYDSDefaultDomain))
	}
	if req.YYDSEnabled != nil {
		records = append(records, s.newSettingRecord("yyds_mail.enabled", *req.YYDSEnabled))
	}
	if err := s.saveSettings(ctx, records); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "临时邮箱设置已更新"}, nil
}

func (s *Service) GetEmailCodeSettings(ctx context.Context) (EmailCodeSettingsResponse, error) {
	settings, err := s.loadSettings(ctx, emailCodeSettingsKeys())
	if err != nil {
		return EmailCodeSettingsResponse{}, err
	}

	return EmailCodeSettingsResponse{
		Timeout:      settingInt(settings, "email_code.timeout"),
		PollInterval: settingInt(settings, "email_code.poll_interval"),
	}, nil
}

func (s *Service) UpdateEmailCodeSettings(ctx context.Context, req UpdateEmailCodeSettingsRequest) (MutationResponse, error) {
	if req.Timeout < 30 || req.Timeout > 600 {
		return MutationResponse{}, ErrInvalidEmailCodeTimeout
	}
	if req.PollInterval < 1 || req.PollInterval > 30 {
		return MutationResponse{}, ErrInvalidEmailCodePollPeriod
	}

	if err := s.saveSettings(ctx, []SettingRecord{
		s.newSettingRecord("email_code.timeout", req.Timeout),
		s.newSettingRecord("email_code.poll_interval", req.PollInterval),
	}); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "验证码等待设置已更新"}, nil
}

func (s *Service) GetOutlookSettings(ctx context.Context) (OutlookSettingsResponse, error) {
	settings, err := s.loadSettings(ctx, outlookSettingsKeys())
	if err != nil {
		return OutlookSettingsResponse{}, err
	}

	return OutlookSettingsResponse{
		DefaultClientID:        settingString(settings, "outlook.default_client_id"),
		ProviderPriority:       settingStringList(settings, "outlook.provider_priority"),
		HealthFailureThreshold: settingInt(settings, "outlook.health_failure_threshold"),
		HealthDisableDuration:  settingInt(settings, "outlook.health_disable_duration"),
	}, nil
}

func (s *Service) UpdateOutlookSettings(ctx context.Context, req UpdateOutlookSettingsRequest) (MutationResponse, error) {
	records := make([]SettingRecord, 0, 1)
	if req.DefaultClientID != nil {
		records = append(records, s.newSettingRecord("outlook.default_client_id", *req.DefaultClientID))
	}
	if err := s.saveSettings(ctx, records); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "Outlook 设置已更新"}, nil
}

func (s *Service) UpdateWebUISettings(ctx context.Context, req UpdateWebUISettingsRequest) (MutationResponse, error) {
	records := make([]SettingRecord, 0, 4)
	if req.Host != nil {
		records = append(records, s.newSettingRecord("webui.host", *req.Host))
	}
	if req.Port != nil {
		records = append(records, s.newSettingRecord("webui.port", *req.Port))
	}
	if req.Debug != nil {
		records = append(records, s.newSettingRecord("app.debug", *req.Debug))
	}
	if req.AccessPassword != nil && strings.TrimSpace(*req.AccessPassword) != "" {
		records = append(records, s.newSettingRecord("webui.access_password", *req.AccessPassword))
	}
	if err := s.saveSettings(ctx, records); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "Web UI 设置已更新"}, nil
}

func (s *Service) GetDynamicProxySettings(ctx context.Context) (DynamicProxySettingsResponse, error) {
	settings, err := s.loadSettings(ctx, dynamicProxySettingsKeys())
	if err != nil {
		return DynamicProxySettingsResponse{}, err
	}

	return DynamicProxySettingsResponse{
		Enabled:      settingBool(settings, "proxy.dynamic_enabled"),
		APIURL:       settingString(settings, "proxy.dynamic_api_url"),
		APIKeyHeader: settingString(settings, "proxy.dynamic_api_key_header"),
		ResultField:  settingString(settings, "proxy.dynamic_result_field"),
		HasAPIKey:    settingHasSecret(settings, "proxy.dynamic_api_key"),
	}, nil
}

func (s *Service) UpdateDynamicProxySettings(ctx context.Context, req UpdateDynamicProxySettingsRequest) (MutationResponse, error) {
	records := make([]SettingRecord, 0, 5)
	if req.APIKey != nil {
		records = append(records, s.newSettingRecord("proxy.dynamic_api_key", *req.APIKey))
	}
	records = append(records,
		s.newSettingRecord("proxy.dynamic_api_key_header", defaultIfBlank(req.APIKeyHeader, settingDefinitions["proxy.dynamic_api_key_header"].Default)),
		s.newSettingRecord("proxy.dynamic_api_url", req.APIURL),
		s.newSettingRecord("proxy.dynamic_enabled", req.Enabled),
		s.newSettingRecord("proxy.dynamic_result_field", req.ResultField),
	)
	if err := s.saveSettings(ctx, records); err != nil {
		return MutationResponse{}, err
	}

	return MutationResponse{Success: true, Message: "动态代理设置已更新"}, nil
}

func (s *Service) ListProxies(ctx context.Context, enabled *bool) (ProxyListResponse, error) {
	if s == nil || s.repository == nil {
		return ProxyListResponse{}, ErrRepositoryNotConfigured
	}

	proxies, err := s.repository.ListProxies(ctx, enabled)
	if err != nil {
		return ProxyListResponse{}, fmt.Errorf("list proxies: %w", err)
	}

	items := make([]ProxyPayload, 0, len(proxies))
	for _, proxy := range proxies {
		items = append(items, toProxyPayload(proxy, false))
	}

	return ProxyListResponse{
		Proxies: items,
		Total:   len(items),
	}, nil
}

func (s *Service) CreateProxy(ctx context.Context, req CreateProxyRequest) (ProxyPayload, error) {
	if s == nil || s.repository == nil {
		return ProxyPayload{}, ErrRepositoryNotConfigured
	}

	normalized, err := normalizeCreateProxyRequest(req)
	if err != nil {
		return ProxyPayload{}, err
	}

	record, err := s.repository.CreateProxy(ctx, normalized)
	if err != nil {
		return ProxyPayload{}, fmt.Errorf("create proxy: %w", err)
	}

	return toProxyPayload(record, false), nil
}

func (s *Service) GetProxy(ctx context.Context, proxyID int, includePassword bool) (ProxyPayload, error) {
	if s == nil || s.repository == nil {
		return ProxyPayload{}, ErrRepositoryNotConfigured
	}

	record, found, err := s.repository.GetProxyByID(ctx, proxyID)
	if err != nil {
		return ProxyPayload{}, fmt.Errorf("get proxy: %w", err)
	}
	if !found {
		return ProxyPayload{}, ErrProxyNotFound
	}

	return toProxyPayload(record, includePassword), nil
}

func (s *Service) UpdateProxy(ctx context.Context, proxyID int, req UpdateProxyRequest) (ProxyPayload, error) {
	if s == nil || s.repository == nil {
		return ProxyPayload{}, ErrRepositoryNotConfigured
	}

	normalized, err := normalizeUpdateProxyRequest(req)
	if err != nil {
		return ProxyPayload{}, err
	}

	record, found, err := s.repository.UpdateProxy(ctx, proxyID, normalized)
	if err != nil {
		return ProxyPayload{}, fmt.Errorf("update proxy: %w", err)
	}
	if !found {
		return ProxyPayload{}, ErrProxyNotFound
	}

	return toProxyPayload(record, false), nil
}

func (s *Service) DeleteProxy(ctx context.Context, proxyID int) (MutationResponse, error) {
	if s == nil || s.repository == nil {
		return MutationResponse{}, ErrRepositoryNotConfigured
	}

	deleted, err := s.repository.DeleteProxy(ctx, proxyID)
	if err != nil {
		return MutationResponse{}, fmt.Errorf("delete proxy: %w", err)
	}
	if !deleted {
		return MutationResponse{}, ErrProxyNotFound
	}

	return MutationResponse{Success: true, Message: "代理已删除"}, nil
}

func (s *Service) EnableProxy(ctx context.Context, proxyID int) (MutationResponse, error) {
	enabled := true
	if _, err := s.UpdateProxy(ctx, proxyID, UpdateProxyRequest{Enabled: &enabled}); err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Success: true, Message: "代理已启用"}, nil
}

func (s *Service) DisableProxy(ctx context.Context, proxyID int) (MutationResponse, error) {
	enabled := false
	if _, err := s.UpdateProxy(ctx, proxyID, UpdateProxyRequest{Enabled: &enabled}); err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Success: true, Message: "代理已禁用"}, nil
}

func (s *Service) SetProxyDefault(ctx context.Context, proxyID int) (ProxyPayload, error) {
	if s == nil || s.repository == nil {
		return ProxyPayload{}, ErrRepositoryNotConfigured
	}

	record, found, err := s.repository.SetProxyDefault(ctx, proxyID)
	if err != nil {
		return ProxyPayload{}, fmt.Errorf("set proxy default: %w", err)
	}
	if !found {
		return ProxyPayload{}, ErrProxyNotFound
	}

	return toProxyPayload(record, false), nil
}

func (s *Service) TestDynamicProxy(ctx context.Context, req UpdateDynamicProxySettingsRequest) (DynamicProxyTestResponse, error) {
	if s == nil || s.dynamicProxyTester == nil {
		return DynamicProxyTestResponse{}, ErrDynamicProxyTesterMissing
	}

	resolved := req
	resolved.APIKeyHeader = defaultIfBlank(resolved.APIKeyHeader, settingDefinitions["proxy.dynamic_api_key_header"].Default)
	if resolved.APIKey == nil && s.repository != nil {
		settings, err := s.repository.GetSettings(ctx, []string{"proxy.dynamic_api_key"})
		if err != nil {
			return DynamicProxyTestResponse{}, fmt.Errorf("load saved dynamic proxy api key: %w", err)
		}
		if value := settingString(settings, "proxy.dynamic_api_key"); strings.TrimSpace(value) != "" {
			resolved.APIKey = &value
		}
	}

	return s.dynamicProxyTester.TestDynamicProxy(ctx, resolved)
}

func (s *Service) TestProxy(ctx context.Context, proxyID int) (ProxyTestResponse, error) {
	if s == nil || s.proxyTester == nil {
		return ProxyTestResponse{}, ErrProxyTesterMissing
	}

	record, found, err := s.repository.GetProxyByID(ctx, proxyID)
	if err != nil {
		return ProxyTestResponse{}, fmt.Errorf("load proxy for test: %w", err)
	}
	if !found {
		return ProxyTestResponse{}, ErrProxyNotFound
	}

	result, err := s.proxyTester.TestProxy(ctx, record)
	if err != nil {
		return ProxyTestResponse{}, err
	}

	return ProxyTestResponse{
		Success:      result.Success,
		IP:           result.IP,
		ResponseTime: result.ResponseTime,
		Message:      result.Message,
	}, nil
}

func (s *Service) TestAllProxies(ctx context.Context) (ProxyTestAllResponse, error) {
	if s == nil || s.proxyTester == nil {
		return ProxyTestAllResponse{}, ErrProxyTesterMissing
	}

	enabled := true
	proxies, err := s.repository.ListProxies(ctx, &enabled)
	if err != nil {
		return ProxyTestAllResponse{}, fmt.Errorf("list enabled proxies: %w", err)
	}

	results := make([]ProxyTestResult, 0, len(proxies))
	successCount := 0
	for _, proxy := range proxies {
		result, testErr := s.proxyTester.TestProxy(ctx, proxy)
		if testErr != nil {
			result = ProxyTestResult{
				ID:      proxy.ID,
				Name:    proxy.Name,
				Success: false,
				Message: testErr.Error(),
			}
		}
		if result.ID == 0 {
			result.ID = proxy.ID
		}
		if result.Name == "" {
			result.Name = proxy.Name
		}
		if result.Success {
			successCount++
		}
		results = append(results, result)
	}

	return ProxyTestAllResponse{
		Total:   len(proxies),
		Success: successCount,
		Failed:  len(proxies) - successCount,
		Results: results,
	}, nil
}

func (s *Service) GetDatabaseInfo(ctx context.Context) (DatabaseInfoResponse, error) {
	if s == nil || s.databaseAdmin == nil {
		return DatabaseInfoResponse{}, ErrDatabaseAdminNotConfigured
	}
	return s.databaseAdmin.GetInfo(ctx)
}

func (s *Service) BackupDatabase(ctx context.Context) (DatabaseBackupResponse, error) {
	if s == nil || s.databaseAdmin == nil {
		return DatabaseBackupResponse{}, ErrDatabaseAdminNotConfigured
	}
	return s.databaseAdmin.Backup(ctx)
}

func (s *Service) ImportDatabase(ctx context.Context, req DatabaseImportRequest) (DatabaseImportResponse, error) {
	if s == nil || s.databaseAdmin == nil {
		return DatabaseImportResponse{}, ErrDatabaseAdminNotConfigured
	}
	return s.databaseAdmin.Import(ctx, req)
}

func (s *Service) CleanupDatabase(ctx context.Context, req DatabaseCleanupRequest) (DatabaseCleanupResponse, error) {
	if s == nil || s.databaseAdmin == nil {
		return DatabaseCleanupResponse{}, ErrDatabaseAdminNotConfigured
	}
	return s.databaseAdmin.Cleanup(ctx, req)
}

func (s *Service) loadSettings(ctx context.Context, keys []string) (map[string]SettingRecord, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return s.repository.GetSettings(ctx, keys)
}

func (s *Service) saveSettings(ctx context.Context, records []SettingRecord) error {
	if len(records) == 0 {
		return nil
	}
	if s == nil || s.repository == nil {
		return ErrRepositoryNotConfigured
	}
	return s.repository.UpsertSettings(ctx, records)
}

func (s *Service) newSettingRecord(key string, value any) SettingRecord {
	defn := settingDefinitions[key]
	return SettingRecord{
		Key:         key,
		Value:       formatSettingValue(value),
		Description: defn.Description,
		Category:    defn.Category,
		UpdatedAt:   time.Now().UTC(),
	}
}

func toProxyPayload(record ProxyRecord, includePassword bool) ProxyPayload {
	payload := ProxyPayload{
		ID:          record.ID,
		Name:        record.Name,
		Type:        record.Type,
		Host:        record.Host,
		Port:        record.Port,
		Username:    normalizeOptionalString(record.Username),
		Enabled:     record.Enabled,
		IsDefault:   record.IsDefault,
		Priority:    record.Priority,
		LastUsed:    cloneTime(record.LastUsed),
		CreatedAt:   cloneTime(record.CreatedAt),
		UpdatedAt:   cloneTime(record.UpdatedAt),
		HasPassword: strings.TrimSpace(derefString(record.Password)) != "",
	}
	if includePassword {
		payload.Password = normalizeOptionalString(record.Password)
	}
	return payload
}

func normalizeCreateProxyRequest(req CreateProxyRequest) (CreateProxyRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.Host = strings.TrimSpace(req.Host)
	req.Username = normalizeOptionalString(req.Username)
	req.Password = normalizeOptionalString(req.Password)

	if req.Name == "" {
		return CreateProxyRequest{}, ErrInvalidProxyName
	}
	if req.Host == "" {
		return CreateProxyRequest{}, ErrInvalidProxyHost
	}
	if req.Type == "" {
		req.Type = "http"
	}
	if req.Type != "http" && req.Type != "socks5" {
		return CreateProxyRequest{}, ErrInvalidProxyType
	}
	if req.Port < 1 || req.Port > 65535 {
		return CreateProxyRequest{}, ErrInvalidProxyPort
	}

	return req, nil
}

func normalizeUpdateProxyRequest(req UpdateProxyRequest) (UpdateProxyRequest, error) {
	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if value == "" {
			return UpdateProxyRequest{}, ErrInvalidProxyName
		}
		req.Name = &value
	}
	if req.Type != nil {
		value := strings.ToLower(strings.TrimSpace(*req.Type))
		if value == "" {
			value = "http"
		}
		if value != "http" && value != "socks5" {
			return UpdateProxyRequest{}, ErrInvalidProxyType
		}
		req.Type = &value
	}
	if req.Host != nil {
		value := strings.TrimSpace(*req.Host)
		if value == "" {
			return UpdateProxyRequest{}, ErrInvalidProxyHost
		}
		req.Host = &value
	}
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		return UpdateProxyRequest{}, ErrInvalidProxyPort
	}
	req.Username = normalizeOptionalString(req.Username)
	req.Password = normalizeOptionalString(req.Password)

	return req, nil
}

func defaultIfBlank(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func settingString(settings map[string]SettingRecord, key string) string {
	record, ok := settings[key]
	if ok {
		return record.Value
	}
	return settingDefinitions[key].Default
}

func settingOptionalString(settings map[string]SettingRecord, key string) *string {
	value := strings.TrimSpace(settingString(settings, key))
	if value == "" {
		return nil
	}
	return &value
}

func settingBool(settings map[string]SettingRecord, key string) bool {
	value := strings.ToLower(strings.TrimSpace(settingString(settings, key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func settingInt(settings map[string]SettingRecord, key string) int {
	value := strings.TrimSpace(settingString(settings, key))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		fallback, fallbackErr := strconv.Atoi(settingDefinitions[key].Default)
		if fallbackErr != nil {
			return 0
		}
		return fallback
	}
	return parsed
}

func settingHasSecret(settings map[string]SettingRecord, key string) bool {
	return strings.TrimSpace(settingString(settings, key)) != ""
}

func settingStringList(settings map[string]SettingRecord, key string) []string {
	raw := strings.TrimSpace(settingString(settings, key))
	if raw == "" {
		return parseDefaultStringList(settingDefinitions[key].Default)
	}

	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && len(parsed) > 0 {
		return parsed
	}
	return parseDefaultStringList(settingDefinitions[key].Default)
}

func parseDefaultStringList(raw string) []string {
	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return []string{}
}

func formatSettingValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(typed)
	case []string:
		payload, _ := json.Marshal(typed)
		return string(payload)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeRegistrationFlow(flow string) string {
	switch strings.ToLower(strings.TrimSpace(flow)) {
	case "abcard":
		return "abcard"
	default:
		return "native"
	}
}

func validateRegistrationFlow(flow string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(flow)) {
	case "", "native", "outlook":
		return "native", nil
	case "abcard":
		return "abcard", nil
	default:
		return "", ErrInvalidRegistrationFlow
	}
}

type settingDefinition struct {
	Default     string
	Category    string
	Description string
}

var settingDefinitions = map[string]settingDefinition{
	"app.debug":                                     {Default: "false", Category: "general", Description: "调试模式"},
	"database.url":                                  {Default: "data/database.db", Category: "database", Description: "数据库路径或连接字符串"},
	"webui.host":                                    {Default: "0.0.0.0", Category: "webui", Description: "Web UI 监听地址"},
	"webui.port":                                    {Default: "8000", Category: "webui", Description: "Web UI 监听端口"},
	"webui.access_password":                         {Default: "admin123", Category: "webui", Description: "Web UI 访问密码"},
	"log.retention_days":                            {Default: "30", Category: "log", Description: "日志保留天数"},
	"proxy.enabled":                                 {Default: "false", Category: "proxy", Description: "是否启用代理"},
	"proxy.type":                                    {Default: "http", Category: "proxy", Description: "代理类型 (http/socks5)"},
	"proxy.host":                                    {Default: "127.0.0.1", Category: "proxy", Description: "代理服务器地址"},
	"proxy.port":                                    {Default: "7890", Category: "proxy", Description: "代理服务器端口"},
	"proxy.username":                                {Default: "", Category: "proxy", Description: "代理用户名"},
	"proxy.password":                                {Default: "", Category: "proxy", Description: "代理密码"},
	"proxy.dynamic_enabled":                         {Default: "false", Category: "proxy", Description: "是否启用动态代理"},
	"proxy.dynamic_api_url":                         {Default: "", Category: "proxy", Description: "动态代理 API 地址，返回代理 URL 字符串"},
	"proxy.dynamic_api_key":                         {Default: "", Category: "proxy", Description: "动态代理 API 密钥（可选）"},
	"proxy.dynamic_api_key_header":                  {Default: "X-API-Key", Category: "proxy", Description: "动态代理 API 密钥请求头名称"},
	"proxy.dynamic_result_field":                    {Default: "", Category: "proxy", Description: "从 JSON 响应中提取代理 URL 的字段路径（留空则使用响应原文）"},
	"registration.max_retries":                      {Default: "3", Category: "registration", Description: "注册最大重试次数"},
	"registration.timeout":                          {Default: "120", Category: "registration", Description: "注册超时时间（秒）"},
	"registration.default_password_length":          {Default: "12", Category: "registration", Description: "默认密码长度"},
	"registration.sleep_min":                        {Default: "5", Category: "registration", Description: "注册间隔最小值（秒）"},
	"registration.sleep_max":                        {Default: "30", Category: "registration", Description: "注册间隔最大值（秒）"},
	"registration.entry_flow":                       {Default: "native", Category: "registration", Description: "注册入口链路（native=原本链路, abcard=ABCard入口链路；Outlook 邮箱会自动走 Outlook 链路）"},
	"registration.token_completion_max_concurrency": {Default: "0", Category: "registration", Description: "refresh_token/OAuth 收尾最大并发数（0 表示跟随批量并发）"},
	"tempmail.enabled":                              {Default: "true", Category: "tempmail", Description: "是否启用 Tempmail 渠道"},
	"tempmail.base_url":                             {Default: "https://api.tempmail.lol/v2", Category: "tempmail", Description: "Tempmail API 地址"},
	"tempmail.timeout":                              {Default: "30", Category: "tempmail", Description: "Tempmail 超时时间（秒）"},
	"tempmail.max_retries":                          {Default: "3", Category: "tempmail", Description: "Tempmail 最大重试次数"},
	"yyds_mail.enabled":                             {Default: "false", Category: "tempmail", Description: "是否启用 YYDS Mail 渠道"},
	"yyds_mail.base_url":                            {Default: "https://maliapi.215.im/v1", Category: "tempmail", Description: "YYDS Mail API 地址"},
	"yyds_mail.api_key":                             {Default: "", Category: "tempmail", Description: "YYDS Mail API Key"},
	"yyds_mail.default_domain":                      {Default: "", Category: "tempmail", Description: "YYDS Mail 默认域名"},
	"yyds_mail.timeout":                             {Default: "30", Category: "tempmail", Description: "YYDS Mail 超时时间（秒）"},
	"yyds_mail.max_retries":                         {Default: "3", Category: "tempmail", Description: "YYDS Mail 最大重试次数"},
	"email_code.timeout":                            {Default: "120", Category: "email", Description: "验证码等待超时时间（秒）"},
	"email_code.poll_interval":                      {Default: "3", Category: "email", Description: "验证码轮询间隔（秒）"},
	"outlook.provider_priority":                     {Default: "[\"imap_old\",\"imap_new\",\"graph_api\"]", Category: "email", Description: "Outlook 提供者优先级"},
	"outlook.health_failure_threshold":              {Default: "5", Category: "email", Description: "Outlook 提供者连续失败次数阈值"},
	"outlook.health_disable_duration":               {Default: "60", Category: "email", Description: "Outlook 提供者禁用时长（秒）"},
	"outlook.default_client_id":                     {Default: "24d9a0ed-8787-4584-883c-2fd79308940a", Category: "email", Description: "Outlook OAuth 默认 Client ID"},
}

func allSettingsKeys() []string {
	return []string{
		"proxy.enabled",
		"proxy.type",
		"proxy.host",
		"proxy.port",
		"proxy.username",
		"proxy.password",
		"proxy.dynamic_enabled",
		"proxy.dynamic_api_url",
		"proxy.dynamic_api_key",
		"proxy.dynamic_api_key_header",
		"proxy.dynamic_result_field",
		"registration.max_retries",
		"registration.timeout",
		"registration.default_password_length",
		"registration.sleep_min",
		"registration.sleep_max",
		"registration.entry_flow",
		"registration.token_completion_max_concurrency",
		"webui.host",
		"webui.port",
		"webui.access_password",
		"app.debug",
		"tempmail.enabled",
		"tempmail.base_url",
		"tempmail.timeout",
		"tempmail.max_retries",
		"yyds_mail.enabled",
		"yyds_mail.base_url",
		"yyds_mail.api_key",
		"yyds_mail.default_domain",
		"yyds_mail.timeout",
		"yyds_mail.max_retries",
		"email_code.timeout",
		"email_code.poll_interval",
	}
}

func registrationSettingsKeys() []string {
	return []string{
		"registration.max_retries",
		"registration.timeout",
		"registration.default_password_length",
		"registration.sleep_min",
		"registration.sleep_max",
		"registration.entry_flow",
		"registration.token_completion_max_concurrency",
	}
}

func tempmailSettingsKeys() []string {
	return []string{
		"tempmail.enabled",
		"tempmail.base_url",
		"tempmail.timeout",
		"tempmail.max_retries",
		"yyds_mail.enabled",
		"yyds_mail.base_url",
		"yyds_mail.api_key",
		"yyds_mail.default_domain",
		"yyds_mail.timeout",
		"yyds_mail.max_retries",
	}
}

func emailCodeSettingsKeys() []string {
	return []string{
		"email_code.timeout",
		"email_code.poll_interval",
	}
}

func outlookSettingsKeys() []string {
	return []string{
		"outlook.provider_priority",
		"outlook.health_failure_threshold",
		"outlook.health_disable_duration",
		"outlook.default_client_id",
	}
}

func dynamicProxySettingsKeys() []string {
	return []string{
		"proxy.dynamic_enabled",
		"proxy.dynamic_api_url",
		"proxy.dynamic_api_key",
		"proxy.dynamic_api_key_header",
		"proxy.dynamic_result_field",
	}
}
