package emailservices

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
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
	FindServiceByName(ctx context.Context, name string) (EmailServiceRecord, bool, error)
	CreateService(ctx context.Context, service EmailServiceRecord) (EmailServiceRecord, error)
	SaveService(ctx context.Context, service EmailServiceRecord) (EmailServiceRecord, error)
	DeleteService(ctx context.Context, serviceID int) (EmailServiceRecord, bool, error)
	UpdateServicePriority(ctx context.Context, serviceID int, priority int) error
	CountServices(ctx context.Context) (map[string]int, int, error)
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
	ListRegisteredAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error)
}

type Tester interface {
	Test(ctx context.Context, serviceType string, config map[string]any) (ServiceTestResult, error)
}

type Service struct {
	repository Repository
	tester     Tester
}

func NewService(repository Repository, tester Tester) *Service {
	if tester == nil {
		tester = nativeTester{}
	}
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

func (s *Service) CreateService(ctx context.Context, req CreateServiceRequest) (EmailServiceResponse, error) {
	if !isValidServiceType(req.ServiceType) {
		return EmailServiceResponse{}, fmt.Errorf("%w: 无效的服务类型: %s", ErrInvalidServiceType, strings.TrimSpace(req.ServiceType))
	}

	if s == nil || s.repository == nil {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	if existing, found, err := s.repository.FindServiceByName(ctx, req.Name); err != nil {
		return EmailServiceResponse{}, err
	} else if found && existing.ID != 0 {
		return EmailServiceResponse{}, fmt.Errorf("%w: 服务名称已存在", ErrDuplicateServiceName)
	}

	record, err := s.repository.CreateService(ctx, EmailServiceRecord{
		ServiceType: req.ServiceType,
		Name:        req.Name,
		Config:      cloneConfig(req.Config),
		Enabled:     req.Enabled,
		Priority:    req.Priority,
	})
	if err != nil {
		return EmailServiceResponse{}, err
	}

	projected, err := s.projectServices(ctx, []EmailServiceRecord{record})
	if err != nil {
		return EmailServiceResponse{}, err
	}
	return projected[0], nil
}

func (s *Service) UpdateService(ctx context.Context, serviceID int, req UpdateServiceRequest) (EmailServiceResponse, error) {
	if s == nil || s.repository == nil {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	record, found, err := s.repository.GetService(ctx, serviceID)
	if err != nil {
		return EmailServiceResponse{}, err
	}
	if !found {
		return EmailServiceResponse{}, ErrServiceNotFound
	}

	if req.Name != nil {
		record.Name = *req.Name
	}
	if req.Config != nil {
		record.Config = mergeConfig(record.Config, req.Config)
	}
	if req.Enabled != nil {
		record.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		record.Priority = *req.Priority
	}

	saved, err := s.repository.SaveService(ctx, record)
	if err != nil {
		return EmailServiceResponse{}, err
	}

	projected, err := s.projectServices(ctx, []EmailServiceRecord{saved})
	if err != nil {
		return EmailServiceResponse{}, err
	}
	return projected[0], nil
}

func (s *Service) DeleteService(ctx context.Context, serviceID int) (ActionResponse, error) {
	if s == nil || s.repository == nil {
		return ActionResponse{}, ErrServiceNotFound
	}

	record, found, err := s.repository.DeleteService(ctx, serviceID)
	if err != nil {
		return ActionResponse{}, err
	}
	if !found {
		return ActionResponse{}, ErrServiceNotFound
	}

	return ActionResponse{Success: true, Message: fmt.Sprintf("服务 %s 已删除", record.Name)}, nil
}

func (s *Service) TestService(ctx context.Context, serviceID int) (ServiceTestResult, error) {
	record, found, err := s.repository.GetService(ctx, serviceID)
	if err != nil {
		return ServiceTestResult{}, err
	}
	if !found {
		return ServiceTestResult{}, ErrServiceNotFound
	}

	result, err := s.tester.Test(ctx, record.ServiceType, cloneConfig(record.Config))
	if err != nil {
		return ServiceTestResult{Success: false, Message: "测试失败: " + err.Error()}, nil
	}
	if result.Success && strings.TrimSpace(result.Message) == "" {
		result.Message = "服务连接正常"
	}
	if !result.Success && strings.TrimSpace(result.Message) == "" {
		result.Message = "服务连接失败"
	}
	return result, nil
}

func (s *Service) EnableService(ctx context.Context, serviceID int) (ActionResponse, error) {
	return s.setEnabled(ctx, serviceID, true)
}

func (s *Service) DisableService(ctx context.Context, serviceID int) (ActionResponse, error) {
	return s.setEnabled(ctx, serviceID, false)
}

func (s *Service) ReorderServices(ctx context.Context, serviceIDs []int) (ActionResponse, error) {
	if len(serviceIDs) == 0 {
		return ActionResponse{}, fmt.Errorf("%w: 优先级列表不能为空", ErrInvalidReorderInput)
	}

	seen := make(map[int]struct{}, len(serviceIDs))
	for _, serviceID := range serviceIDs {
		if serviceID <= 0 {
			return ActionResponse{}, fmt.Errorf("%w: 服务 ID 必须大于 0", ErrInvalidReorderInput)
		}
		if _, ok := seen[serviceID]; ok {
			return ActionResponse{}, fmt.Errorf("%w: 排序列表存在重复服务 ID", ErrInvalidReorderInput)
		}
		seen[serviceID] = struct{}{}
	}

	for index, serviceID := range serviceIDs {
		if err := s.repository.UpdateServicePriority(ctx, serviceID, index); err != nil {
			return ActionResponse{}, err
		}
	}
	return ActionResponse{Success: true, Message: "优先级已更新"}, nil
}

func (s *Service) BatchImportOutlook(ctx context.Context, req OutlookBatchImportRequest) (OutlookBatchImportResponse, error) {
	trimmed := strings.TrimSpace(req.Data)
	if trimmed == "" {
		return OutlookBatchImportResponse{}, fmt.Errorf("%w: 导入数据不能为空", ErrEmptyOutlookImport)
	}

	lines := strings.Split(trimmed, "\n")
	response := OutlookBatchImportResponse{
		Total:    len(lines),
		Accounts: make([]map[string]any, 0, len(lines)),
		Errors:   make([]string, 0),
	}
	seenEmails := make(map[string]struct{}, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "----")
		if len(parts) < 2 {
			response.Failed++
			response.Errors = append(response.Errors, fmt.Sprintf("行 %d: 格式错误，至少需要邮箱和密码", i+1))
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])
		if !strings.Contains(email, "@") {
			response.Failed++
			response.Errors = append(response.Errors, fmt.Sprintf("行 %d: 无效的邮箱地址: %s", i+1, email))
			continue
		}
		key := strings.ToLower(email)
		if _, ok := seenEmails[key]; ok {
			response.Failed++
			response.Errors = append(response.Errors, fmt.Sprintf("行 %d: 邮箱已存在: %s", i+1, email))
			continue
		}
		seenEmails[key] = struct{}{}

		existing, found, err := s.repository.FindServiceByName(ctx, email)
		if err != nil {
			return OutlookBatchImportResponse{}, err
		}
		if found && existing.ServiceType == ServiceTypeOutlook {
			response.Failed++
			response.Errors = append(response.Errors, fmt.Sprintf("行 %d: 邮箱已存在: %s", i+1, email))
			continue
		}

		config := map[string]any{
			"email":    email,
			"password": password,
		}
		if len(parts) >= 4 {
			clientID := strings.TrimSpace(parts[2])
			refreshToken := strings.TrimSpace(parts[3])
			if clientID != "" && refreshToken != "" {
				config["client_id"] = clientID
				config["refresh_token"] = refreshToken
			}
		}

		record, err := s.repository.CreateService(ctx, EmailServiceRecord{
			ServiceType: ServiceTypeOutlook,
			Name:        email,
			Config:      config,
			Enabled:     req.Enabled,
			Priority:    req.Priority,
		})
		if err != nil {
			response.Failed++
			response.Errors = append(response.Errors, fmt.Sprintf("行 %d: 创建失败: %s", i+1, err.Error()))
			continue
		}

		response.Accounts = append(response.Accounts, map[string]any{
			"id":        record.ID,
			"email":     email,
			"has_oauth": config["client_id"] != nil,
			"name":      email,
		})
		response.Success++
	}

	return response, nil
}

func (s *Service) BatchDeleteOutlook(ctx context.Context, serviceIDs []int) (BatchDeleteResponse, error) {
	deleted := 0
	for _, serviceID := range serviceIDs {
		record, found, err := s.repository.GetService(ctx, serviceID)
		if err != nil {
			return BatchDeleteResponse{}, err
		}
		if !found || record.ServiceType != ServiceTypeOutlook {
			continue
		}
		if _, found, err := s.repository.DeleteService(ctx, serviceID); err != nil {
			return BatchDeleteResponse{}, err
		} else if found {
			deleted++
		}
	}

	return BatchDeleteResponse{
		Success: true,
		Deleted: deleted,
		Message: fmt.Sprintf("已删除 %d 个服务", deleted),
	}, nil
}

func (s *Service) TestTempmail(ctx context.Context, req TempmailTestRequest) (ServiceTestResult, error) {
	if s == nil || s.repository == nil {
		return ServiceTestResult{}, fmt.Errorf("%w: settings dependency is not configured", ErrTempmailProviderError)
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = ServiceTypeTempmail
	}

	var (
		config         map[string]any
		successMessage string
		failMessage    string
		err            error
	)
	switch provider {
	case ServiceTypeYYDSMail:
		config, err = s.resolveYYDSMailConfig(ctx, req)
		successMessage = "YYDS Mail 连接正常"
		failMessage = "YYDS Mail 连接失败"
	default:
		provider = ServiceTypeTempmail
		config, err = s.resolveTempmailConfig(ctx, req)
		successMessage = "临时邮箱连接正常"
		failMessage = "临时邮箱连接失败"
	}
	if err != nil {
		return ServiceTestResult{Success: false, Message: "测试失败: " + err.Error()}, nil
	}

	result, err := s.tester.Test(ctx, provider, config)
	if err != nil {
		return ServiceTestResult{Success: false, Message: "测试失败: " + err.Error()}, nil
	}
	if result.Success {
		result.Message = successMessage
	} else {
		result.Message = failMessage
	}
	return result, nil
}

func (s *Service) setEnabled(ctx context.Context, serviceID int, enabled bool) (ActionResponse, error) {
	record, found, err := s.repository.GetService(ctx, serviceID)
	if err != nil {
		return ActionResponse{}, err
	}
	if !found {
		return ActionResponse{}, ErrServiceNotFound
	}

	record.Enabled = enabled
	record, err = s.repository.SaveService(ctx, record)
	if err != nil {
		return ActionResponse{}, err
	}

	action := "禁用"
	if enabled {
		action = "启用"
	}
	return ActionResponse{Success: true, Message: fmt.Sprintf("服务 %s 已%s", record.Name, action)}, nil
}

func (s *Service) resolveTempmailConfig(ctx context.Context, req TempmailTestRequest) (map[string]any, error) {
	settings, err := s.repository.GetSettings(ctx, []string{
		"tempmail.base_url",
		"tempmail.timeout",
		"tempmail.max_retries",
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"base_url":    firstNonEmpty(req.APIURL, settings["tempmail.base_url"]),
		"timeout":     parseIntSetting(settings["tempmail.timeout"], 30),
		"max_retries": parseIntSetting(settings["tempmail.max_retries"], 3),
	}, nil
}

func (s *Service) resolveYYDSMailConfig(ctx context.Context, req TempmailTestRequest) (map[string]any, error) {
	settings, err := s.repository.GetSettings(ctx, []string{
		"yyds_mail.base_url",
		"yyds_mail.api_key",
		"yyds_mail.default_domain",
		"yyds_mail.timeout",
		"yyds_mail.max_retries",
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"base_url":       firstNonEmpty(req.APIURL, settings["yyds_mail.base_url"]),
		"api_key":        firstNonEmpty(req.APIKey, settings["yyds_mail.api_key"]),
		"default_domain": settings["yyds_mail.default_domain"],
		"timeout":        parseIntSetting(settings["yyds_mail.timeout"], 30),
		"max_retries":    parseIntSetting(settings["yyds_mail.max_retries"], 3),
	}, nil
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

func mergeConfig(current map[string]any, updates map[string]any) map[string]any {
	merged := cloneConfig(current)
	for key, value := range updates {
		merged[key] = value
	}
	for key, value := range merged {
		if !isTruthyConfigValue(value) {
			delete(merged, key)
		}
	}
	return merged
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

func parseIntSetting(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
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

func isValidServiceType(value string) bool {
	switch strings.TrimSpace(value) {
	case ServiceTypeTempmail, ServiceTypeYYDSMail, ServiceTypeOutlook, ServiceTypeMoeMail, ServiceTypeTempMail, ServiceTypeDuckMail, ServiceTypeFreemail, ServiceTypeIMAPMail, ServiceTypeCloudmail, ServiceTypeLuckmail:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type nativeTester struct{}

func (nativeTester) Test(ctx context.Context, serviceType string, config map[string]any) (ServiceTestResult, error) {
	provider, err := mail.NewProvider(serviceType, config)
	if err != nil {
		return ServiceTestResult{}, err
	}

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	inbox, err := provider.Create(testCtx)
	if err != nil {
		return ServiceTestResult{}, err
	}

	details := map[string]any{}
	if inbox.Email != "" {
		details["email"] = inbox.Email
	}
	return ServiceTestResult{Success: true, Message: "服务连接正常", Details: details}, nil
}
