package emailservices

import (
	"context"
	"strings"
	"time"
)

var sensitiveFields = map[string]struct{}{
	"password":       {},
	"api_key":        {},
	"refresh_token":  {},
	"access_token":   {},
	"admin_token":    {},
	"admin_password": {},
	"custom_auth":    {},
}

type Repository interface {
	ListServices(ctx context.Context, req ListServicesRequest) ([]EmailServiceRecord, error)
	GetService(ctx context.Context, serviceID int) (EmailServiceRecord, bool, error)
	CountServices(ctx context.Context) (map[string]int, int, error)
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
	ListRegisteredAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error)
}

type Tester interface{}

type Service struct {
	repository Repository
	tester     Tester
}

func NewService(repository Repository, tester Tester) *Service {
	return &Service{repository: repository, tester: tester}
}

func (s *Service) ListServices(ctx context.Context, req ListServicesRequest) (EmailServiceListResponse, error) {
	if s == nil || s.repository == nil {
		return EmailServiceListResponse{Services: make([]EmailServiceResponse, 0)}, nil
	}

	services, err := s.repository.ListServices(ctx, req)
	if err != nil {
		return EmailServiceListResponse{}, err
	}

	projected, err := s.projectServices(ctx, services)
	if err != nil {
		return EmailServiceListResponse{}, err
	}

	return EmailServiceListResponse{
		Total:    len(projected),
		Services: projected,
	}, nil
}

func (s *Service) GetService(ctx context.Context, serviceID int) (EmailServiceResponse, error) {
	if s == nil || s.repository == nil {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	service, found, err := s.repository.GetService(ctx, serviceID)
	if err != nil {
		return EmailServiceResponse{}, err
	}
	if !found {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	projected, err := s.projectServices(ctx, []EmailServiceRecord{service})
	if err != nil {
		return EmailServiceResponse{}, err
	}
	if len(projected) == 0 {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	return projected[0], nil
}

func (s *Service) GetServiceFull(ctx context.Context, serviceID int) (EmailServiceFullResponse, error) {
	if s == nil || s.repository == nil {
		return EmailServiceFullResponse{}, ErrServiceNotFound
	}

	service, found, err := s.repository.GetService(ctx, serviceID)
	if err != nil {
		return EmailServiceFullResponse{}, err
	}
	if !found {
		return EmailServiceFullResponse{}, ErrServiceNotFound
	}

	return EmailServiceFullResponse{
		ID:          service.ID,
		ServiceType: service.ServiceType,
		Name:        service.Name,
		Enabled:     service.Enabled,
		Priority:    service.Priority,
		Config:      cloneConfig(service.Config),
		LastUsed:    formatTime(service.LastUsed),
		CreatedAt:   formatTime(service.CreatedAt),
		UpdatedAt:   formatTime(service.UpdatedAt),
	}, nil
}

func (s *Service) GetStats(ctx context.Context) (StatsResponse, error) {
	if s == nil || s.repository == nil {
		return StatsResponse{}, nil
	}

	typeCounts, enabledCount, err := s.repository.CountServices(ctx)
	if err != nil {
		return StatsResponse{}, err
	}

	settings, err := s.repository.GetSettings(ctx, []string{
		"tempmail.enabled",
		"yyds_mail.enabled",
		"yyds_mail.api_key",
	})
	if err != nil {
		return StatsResponse{}, err
	}

	yydsEnabled := parseBoolSetting(settings["yyds_mail.enabled"]) && strings.TrimSpace(settings["yyds_mail.api_key"]) != ""
	return StatsResponse{
		OutlookCount:      typeCounts[ServiceTypeOutlook],
		CustomCount:       typeCounts[ServiceTypeMoeMail],
		YYDSMailCount:     typeCounts[ServiceTypeYYDSMail],
		TempMailCount:     typeCounts[ServiceTypeTempMail],
		DuckMailCount:     typeCounts[ServiceTypeDuckMail],
		FreemailCount:     typeCounts[ServiceTypeFreemail],
		IMAPMailCount:     typeCounts[ServiceTypeIMAPMail],
		CloudmailCount:    typeCounts[ServiceTypeCloudmail],
		LuckmailCount:     typeCounts[ServiceTypeLuckmail],
		TempmailAvailable: parseBoolSetting(settings["tempmail.enabled"]) || yydsEnabled,
		YYDSMailAvailable: yydsEnabled,
		EnabledCount:      enabledCount,
	}, nil
}

func (s *Service) GetServiceTypes() ServiceTypesResponse {
	return ServiceTypesResponse{Types: []ServiceTypeDefinition{
		{
			Value:       ServiceTypeTempmail,
			Label:       "Tempmail.lol",
			Description: "官方内置临时邮箱渠道，通过全局配置使用",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "API 地址", Default: "https://api.tempmail.lol/v2", Required: false},
				{Name: "timeout", Label: "超时时间", Default: 30, Required: false},
			},
		},
		{
			Value:       ServiceTypeYYDSMail,
			Label:       "YYDS Mail",
			Description: "官方内置临时邮箱渠道，使用 X-API-Key 创建邮箱并轮询消息",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "API 地址", Default: "https://maliapi.215.im/v1", Required: false},
				{Name: "api_key", Label: "API Key", Required: true, Secret: true},
				{Name: "default_domain", Label: "默认域名", Required: false, Placeholder: "public.example.com"},
				{Name: "timeout", Label: "超时时间", Default: 30, Required: false},
			},
		},
		{
			Value:       ServiceTypeOutlook,
			Label:       "Outlook",
			Description: "Outlook 邮箱，需要配置账户信息",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "email", Label: "邮箱地址", Required: true},
				{Name: "password", Label: "密码", Required: true},
				{Name: "client_id", Label: "OAuth Client ID", Required: false},
				{Name: "refresh_token", Label: "OAuth Refresh Token", Required: false},
			},
		},
		{
			Value:       ServiceTypeMoeMail,
			Label:       "MoeMail",
			Description: "自定义域名邮箱服务",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "API 地址", Required: true},
				{Name: "api_key", Label: "API Key", Required: true},
				{Name: "default_domain", Label: "默认域名", Required: false},
			},
		},
		{
			Value:       ServiceTypeTempMail,
			Label:       "Temp-Mail（自部署）",
			Description: "自部署 Cloudflare Worker 临时邮箱，admin 模式管理",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "Worker 地址", Required: true, Placeholder: "https://mail.example.com"},
				{Name: "admin_password", Label: "Admin 密码", Required: true, Secret: true},
				{Name: "custom_auth", Label: "Custom Auth（可选）", Required: false, Secret: true},
				{Name: "domain", Label: "邮箱域名", Required: true, Placeholder: "example.com"},
				{Name: "enable_prefix", Label: "启用前缀", Default: true, Required: false},
			},
		},
		{
			Value:       ServiceTypeDuckMail,
			Label:       "DuckMail",
			Description: "DuckMail 接口邮箱服务，支持 API Key 私有域名访问",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "API 地址", Required: true, Placeholder: "https://api.duckmail.sbs"},
				{Name: "default_domain", Label: "默认域名", Required: true, Placeholder: "duckmail.sbs"},
				{Name: "api_key", Label: "API Key", Required: false, Secret: true},
				{Name: "password_length", Label: "随机密码长度", Required: false, Default: 12},
			},
		},
		{
			Value:       ServiceTypeFreemail,
			Label:       "Freemail",
			Description: "Freemail 自部署 Cloudflare Worker 临时邮箱服务",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "API 地址", Required: true, Placeholder: "https://freemail.example.com"},
				{Name: "admin_token", Label: "Admin Token", Required: true, Secret: true},
				{Name: "domain", Label: "邮箱域名", Required: false, Placeholder: "example.com"},
			},
		},
		{
			Value:       ServiceTypeIMAPMail,
			Label:       "IMAP 邮箱",
			Description: "标准 IMAP 协议邮箱（Gmail/QQ/163等），仅用于接收验证码，强制直连",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "host", Label: "IMAP 服务器", Required: true, Placeholder: "imap.gmail.com"},
				{Name: "port", Label: "端口", Required: false, Default: 993},
				{Name: "use_ssl", Label: "使用 SSL", Required: false, Default: true},
				{Name: "email", Label: "邮箱地址", Required: true},
				{Name: "password", Label: "密码/授权码", Required: true, Secret: true},
			},
		},
		{
			Value:       ServiceTypeLuckmail,
			Label:       "LuckMail",
			Description: "LuckMail 接码服务（下单 + 轮询验证码）",
			ConfigFields: []ServiceTypeFieldDefinition{
				{Name: "base_url", Label: "平台地址", Required: false, Default: "https://mails.luckyous.com/"},
				{Name: "api_key", Label: "API Key", Required: true, Secret: true},
				{Name: "project_code", Label: "项目编码", Required: false, Default: "openai"},
				{Name: "email_type", Label: "邮箱类型", Required: false, Default: "ms_graph"},
				{Name: "preferred_domain", Label: "优先域名", Required: false, Placeholder: "outlook.com"},
				{Name: "poll_interval", Label: "轮询间隔(秒)", Required: false, Default: 3.0},
			},
		},
	}}
}

func (s *Service) projectServices(ctx context.Context, services []EmailServiceRecord) ([]EmailServiceResponse, error) {
	accountLookup, err := s.lookupRegisteredAccounts(ctx, services)
	if err != nil {
		return nil, err
	}

	response := make([]EmailServiceResponse, 0, len(services))
	for _, service := range services {
		item := EmailServiceResponse{
			ID:          service.ID,
			ServiceType: service.ServiceType,
			Name:        service.Name,
			Enabled:     service.Enabled,
			Priority:    service.Priority,
			Config:      filterSensitiveConfig(service.Config),
			LastUsed:    formatTime(service.LastUsed),
			CreatedAt:   formatTime(service.CreatedAt),
			UpdatedAt:   formatTime(service.UpdatedAt),
		}

		if strings.TrimSpace(service.ServiceType) == ServiceTypeOutlook {
			email := resolveOutlookServiceEmail(service)
			if account, ok := accountLookup[email]; ok {
				item.RegistrationStatus = "registered"
				accountID := account.ID
				item.RegisteredAccountID = &accountID
			} else if email != "" {
				item.RegistrationStatus = "unregistered"
			}
		}

		response = append(response, item)
	}
	return response, nil
}

func (s *Service) lookupRegisteredAccounts(ctx context.Context, services []EmailServiceRecord) (map[string]RegisteredAccountRecord, error) {
	lookup := make(map[string]RegisteredAccountRecord)
	if s == nil || s.repository == nil {
		return lookup, nil
	}

	emails := make([]string, 0, len(services)*2)
	seen := make(map[string]struct{}, len(services)*2)
	for _, service := range services {
		if strings.TrimSpace(service.ServiceType) != ServiceTypeOutlook {
			continue
		}
		email := resolveOutlookServiceEmail(service)
		if email == "" {
			continue
		}
		for _, candidate := range []string{email, strings.ToLower(email)} {
			if candidate == "" {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			emails = append(emails, candidate)
		}
	}
	if len(emails) == 0 {
		return lookup, nil
	}

	rows, err := s.repository.ListRegisteredAccountsByEmails(ctx, emails)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := strings.ToLower(strings.TrimSpace(row.Email))
		if key == "" {
			continue
		}
		lookup[key] = row
	}
	return lookup, nil
}

func resolveOutlookServiceEmail(service EmailServiceRecord) string {
	if service.Config != nil {
		if raw, ok := service.Config["email"].(string); ok {
			email := strings.ToLower(strings.TrimSpace(raw))
			if email != "" {
				return email
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(service.Name))
}

func filterSensitiveConfig(config map[string]any) map[string]any {
	if len(config) == 0 {
		return map[string]any{}
	}

	filtered := make(map[string]any, len(config))
	for key, value := range config {
		if _, ok := sensitiveFields[key]; ok {
			filtered["has_"+key] = isTruthyConfigValue(value)
			continue
		}
		filtered[key] = value
	}

	if stringConfig(config, "client_id") != "" && stringConfig(config, "refresh_token") != "" {
		filtered["has_oauth"] = true
	}

	return filtered
}

func cloneConfig(config map[string]any) map[string]any {
	if len(config) == 0 {
		return map[string]any{}
	}

	cloned := make(map[string]any, len(config))
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func parseBoolSetting(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func stringConfig(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func isTruthyConfigValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return strings.TrimSpace(typed) != ""
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}
