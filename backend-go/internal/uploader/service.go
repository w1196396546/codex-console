package uploader

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type senderFactory func(kind UploadKind) (Sender, error)

type ServiceOption func(*Service)

type Service struct {
	repository    AdminRepository
	httpDoer      HTTPDoer
	senderFactory senderFactory
	accountStore  UploadAccountStore
	now           func() time.Time
}

func NewService(repository AdminRepository, opts ...ServiceOption) *Service {
	service := &Service{
		repository: repository,
		httpDoer:   http.DefaultClient,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.senderFactory == nil {
		service.senderFactory = func(kind UploadKind) (Sender, error) {
			return NewSender(kind, service.httpDoer)
		}
	}
	return service
}

func WithHTTPDoer(doer HTTPDoer) ServiceOption {
	return func(service *Service) {
		if doer != nil {
			service.httpDoer = doer
		}
	}
}

func WithSenderFactory(factory func(kind UploadKind) (Sender, error)) ServiceOption {
	return func(service *Service) {
		if factory != nil {
			service.senderFactory = factory
		}
	}
}

func WithUploadAccountStore(store UploadAccountStore) ServiceOption {
	return func(service *Service) {
		service.accountStore = store
	}
}

func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func (s *Service) ListCPAServices(ctx context.Context, enabled *bool) ([]CPAServiceResponse, error) {
	configs, err := s.listConfigs(ctx, UploadKindCPA, enabled)
	if err != nil {
		return nil, err
	}

	items := make([]CPAServiceResponse, 0, len(configs))
	for _, config := range configs {
		items = append(items, toCPAServiceResponse(config))
	}
	return items, nil
}

func (s *Service) GetCPAService(ctx context.Context, id int) (CPAServiceResponse, error) {
	config, err := s.getConfig(ctx, UploadKindCPA, id)
	if err != nil {
		return CPAServiceResponse{}, err
	}
	return toCPAServiceResponse(config), nil
}

func (s *Service) GetCPAServiceFull(ctx context.Context, id int) (CPAServiceFullResponse, error) {
	config, err := s.getConfig(ctx, UploadKindCPA, id)
	if err != nil {
		return CPAServiceFullResponse{}, err
	}
	return CPAServiceFullResponse{
		ID:       config.ID,
		Name:     config.Name,
		APIURL:   config.BaseURL,
		APIToken: config.Credential,
		ProxyURL: config.ProxyURL,
		Enabled:  config.Enabled,
		Priority: config.Priority,
	}, nil
}

func (s *Service) CreateCPAService(ctx context.Context, req CreateCPAServiceRequest) (CPAServiceResponse, error) {
	created, err := s.createConfig(ctx, ManagedServiceConfig{
		ServiceConfig: ServiceConfig{
			Kind:       UploadKindCPA,
			Name:       req.Name,
			BaseURL:    req.APIURL,
			Credential: req.APIToken,
			ProxyURL:   req.ProxyURL,
			Enabled:    req.Enabled,
			Priority:   req.Priority,
		},
	})
	if err != nil {
		return CPAServiceResponse{}, err
	}
	return toCPAServiceResponse(created), nil
}

func (s *Service) UpdateCPAService(ctx context.Context, id int, req UpdateCPAServiceRequest) (CPAServiceResponse, error) {
	updated, err := s.updateConfig(ctx, UploadKindCPA, id, ManagedServiceConfigPatch{
		Name:       cloneTrimmedString(req.Name),
		BaseURL:    cloneTrimmedString(req.APIURL),
		Credential: req.APIToken,
		ProxyURL:   cloneNullableString(req.ProxyURL),
		Enabled:    req.Enabled,
		Priority:   req.Priority,
	})
	if err != nil {
		return CPAServiceResponse{}, err
	}
	return toCPAServiceResponse(updated), nil
}

func (s *Service) DeleteCPAService(ctx context.Context, id int) (DeleteServiceResponse, error) {
	deleted, err := s.deleteConfig(ctx, UploadKindCPA, id)
	if err != nil {
		return DeleteServiceResponse{}, err
	}
	return DeleteServiceResponse{
		Success: true,
		Message: fmt.Sprintf("CPA 服务 %s 已删除", deleted.Name),
	}, nil
}

func (s *Service) TestCPAService(ctx context.Context, id int) (ConnectionTestResult, error) {
	config, err := s.getConfig(ctx, UploadKindCPA, id)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	return s.TestCPAConnection(ctx, CPAConnectionTestRequest{
		APIURL:   config.BaseURL,
		APIToken: config.Credential,
	})
}

func (s *Service) TestCPAConnection(ctx context.Context, req CPAConnectionTestRequest) (ConnectionTestResult, error) {
	if strings.TrimSpace(req.APIURL) == "" {
		return ConnectionTestResult{Success: false, Message: "API URL 不能为空"}, nil
	}
	if strings.TrimSpace(req.APIToken) == "" {
		return ConnectionTestResult{Success: false, Message: "API Token 不能为空"}, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeCPAAuthFilesURL(req.APIURL), nil)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.APIToken))

	statusCode, _, err := newHTTPClient(s.httpDoer).do(request)
	if err != nil {
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("连接测试失败: %v", err)}, nil
	}

	switch statusCode {
	case http.StatusOK:
		return ConnectionTestResult{Success: true, Message: "CPA 连接测试成功"}, nil
	case http.StatusUnauthorized:
		return ConnectionTestResult{Success: false, Message: "连接成功，但 API Token 无效"}, nil
	case http.StatusForbidden:
		return ConnectionTestResult{Success: false, Message: "连接成功，但服务端未启用远程管理或当前 Token 无权限"}, nil
	case http.StatusNotFound:
		return ConnectionTestResult{Success: false, Message: "未找到 CPA auth-files 接口，请检查 API URL 是否填写为根地址、/v0/management 或完整 auth-files 地址"}, nil
	case http.StatusServiceUnavailable:
		return ConnectionTestResult{Success: false, Message: "连接成功，但服务端认证管理器不可用"}, nil
	default:
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("服务器返回异常状态码: %d", statusCode)}, nil
	}
}

func (s *Service) ListSub2APIServices(ctx context.Context, enabled *bool) ([]Sub2APIServiceResponse, error) {
	configs, err := s.listConfigs(ctx, UploadKindSub2API, enabled)
	if err != nil {
		return nil, err
	}

	items := make([]Sub2APIServiceResponse, 0, len(configs))
	for _, config := range configs {
		items = append(items, toSub2APIServiceResponse(config))
	}
	return items, nil
}

func (s *Service) GetSub2APIService(ctx context.Context, id int) (Sub2APIServiceResponse, error) {
	config, err := s.getConfig(ctx, UploadKindSub2API, id)
	if err != nil {
		return Sub2APIServiceResponse{}, err
	}
	return toSub2APIServiceResponse(config), nil
}

func (s *Service) GetSub2APIServiceFull(ctx context.Context, id int) (Sub2APIServiceFullResponse, error) {
	config, err := s.getConfig(ctx, UploadKindSub2API, id)
	if err != nil {
		return Sub2APIServiceFullResponse{}, err
	}
	targetType, err := normalizeSub2APITargetType(config.TargetType)
	if err != nil {
		return Sub2APIServiceFullResponse{}, err
	}
	return Sub2APIServiceFullResponse{
		ID:         config.ID,
		Name:       config.Name,
		APIURL:     config.BaseURL,
		APIKey:     config.Credential,
		TargetType: targetType,
		Enabled:    config.Enabled,
		Priority:   config.Priority,
	}, nil
}

func (s *Service) CreateSub2APIService(ctx context.Context, req CreateSub2APIServiceRequest) (Sub2APIServiceResponse, error) {
	targetType, err := normalizeSub2APITargetType(req.TargetType)
	if err != nil {
		return Sub2APIServiceResponse{}, err
	}

	created, err := s.createConfig(ctx, ManagedServiceConfig{
		ServiceConfig: ServiceConfig{
			Kind:       UploadKindSub2API,
			Name:       req.Name,
			BaseURL:    req.APIURL,
			Credential: req.APIKey,
			TargetType: targetType,
			Enabled:    req.Enabled,
			Priority:   req.Priority,
		},
	})
	if err != nil {
		return Sub2APIServiceResponse{}, err
	}
	return toSub2APIServiceResponse(created), nil
}

func (s *Service) UpdateSub2APIService(ctx context.Context, id int, req UpdateSub2APIServiceRequest) (Sub2APIServiceResponse, error) {
	var targetType *string
	if req.TargetType != nil {
		normalizedTargetType, err := normalizeSub2APITargetType(*req.TargetType)
		if err != nil {
			return Sub2APIServiceResponse{}, err
		}
		targetType = stringPointer(normalizedTargetType)
	}

	updated, err := s.updateConfig(ctx, UploadKindSub2API, id, ManagedServiceConfigPatch{
		Name:       cloneTrimmedString(req.Name),
		BaseURL:    cloneTrimmedString(req.APIURL),
		Credential: req.APIKey,
		TargetType: targetType,
		Enabled:    req.Enabled,
		Priority:   req.Priority,
	})
	if err != nil {
		return Sub2APIServiceResponse{}, err
	}
	return toSub2APIServiceResponse(updated), nil
}

func (s *Service) DeleteSub2APIService(ctx context.Context, id int) (DeleteServiceResponse, error) {
	deleted, err := s.deleteConfig(ctx, UploadKindSub2API, id)
	if err != nil {
		return DeleteServiceResponse{}, err
	}
	return DeleteServiceResponse{
		Success: true,
		Message: fmt.Sprintf("Sub2API 服务 %s 已删除", deleted.Name),
	}, nil
}

func (s *Service) TestSub2APIService(ctx context.Context, id int) (ConnectionTestResult, error) {
	config, err := s.getConfig(ctx, UploadKindSub2API, id)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	return s.TestSub2APIConnection(ctx, Sub2APIConnectionTestRequest{
		APIURL: config.BaseURL,
		APIKey: config.Credential,
	})
}

func (s *Service) TestSub2APIConnection(ctx context.Context, req Sub2APIConnectionTestRequest) (ConnectionTestResult, error) {
	if strings.TrimSpace(req.APIURL) == "" {
		return ConnectionTestResult{Success: false, Message: "API URL 不能为空"}, nil
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return ConnectionTestResult{Success: false, Message: "API Key 不能为空"}, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURLPath(req.APIURL, "/api/v1/admin/accounts/data"), nil)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	request.Header.Set("x-api-key", strings.TrimSpace(req.APIKey))

	statusCode, _, err := newHTTPClient(s.httpDoer).do(request)
	if err != nil {
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("连接测试失败: %v", err)}, nil
	}

	switch statusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent, http.StatusMethodNotAllowed:
		return ConnectionTestResult{Success: true, Message: "Sub2API 连接测试成功"}, nil
	case http.StatusUnauthorized:
		return ConnectionTestResult{Success: false, Message: "连接成功，但 API Key 无效"}, nil
	case http.StatusForbidden:
		return ConnectionTestResult{Success: false, Message: "连接成功，但权限不足"}, nil
	default:
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("服务器返回异常状态码: %d", statusCode)}, nil
	}
}

func (s *Service) UploadSub2API(ctx context.Context, req Sub2APIUploadRequest) (Sub2APIUploadResult, error) {
	if s == nil || s.accountStore == nil {
		return Sub2APIUploadResult{}, ErrUploadAccountStoreNotConfigured
	}

	normalized := req.Normalized()
	service, err := s.resolveSub2APIUploadService(ctx, normalized.ServiceID)
	if err != nil {
		return Sub2APIUploadResult{}, err
	}

	accounts, err := s.accountStore.ListUploadAccounts(ctx, normalized.AccountIDs)
	if err != nil {
		return Sub2APIUploadResult{}, err
	}

	accountByID := make(map[int]UploadAccount, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account.Normalized()
	}

	result := Sub2APIUploadResult{
		Details: make([]Sub2APIUploadDetail, 0, len(normalized.AccountIDs)),
	}
	validAccounts := make([]UploadAccount, 0, len(normalized.AccountIDs))

	for _, id := range normalized.AccountIDs {
		account, found := accountByID[id]
		if !found {
			result.FailedCount++
			result.Details = append(result.Details, Sub2APIUploadDetail{
				ID:      id,
				Email:   nil,
				Success: false,
				Error:   "账号不存在",
			})
			continue
		}
		if strings.TrimSpace(account.AccessToken) == "" {
			result.SkippedCount++
			result.Details = append(result.Details, Sub2APIUploadDetail{
				ID:      id,
				Email:   stringPointer(account.Email),
				Success: false,
				Error:   "缺少 access_token",
			})
			continue
		}
		validAccounts = append(validAccounts, account)
	}

	if len(validAccounts) == 0 {
		return result, nil
	}

	sender, err := s.senderFactory(UploadKindSub2API)
	if err != nil {
		return Sub2APIUploadResult{}, err
	}

	uploadResults, sendErr := sender.Send(ctx, SendRequest{
		Service:  service.ServiceConfig.Normalized(),
		Accounts: validAccounts,
		Sub2API:  Sub2APIBatchOptions{Concurrency: normalized.Concurrency, Priority: normalized.Priority},
	})
	if sendErr != nil {
		for _, account := range validAccounts {
			result.FailedCount++
			result.Details = append(result.Details, Sub2APIUploadDetail{
				ID:      account.ID,
				Email:   stringPointer(account.Email),
				Success: false,
				Error:   sendErr.Error(),
			})
		}
		return result, nil
	}

	successIDs := make([]int, 0, len(uploadResults))
	accountByEmail := make(map[string]UploadAccount, len(validAccounts))
	for _, account := range validAccounts {
		accountByEmail[strings.ToLower(strings.TrimSpace(account.Email))] = account
	}

	for _, uploadResult := range uploadResults {
		account, found := accountByEmail[strings.ToLower(strings.TrimSpace(uploadResult.AccountEmail))]
		if !found {
			continue
		}

		detail := Sub2APIUploadDetail{
			ID:      account.ID,
			Email:   stringPointer(account.Email),
			Success: uploadResult.Success,
		}
		if uploadResult.Success {
			result.SuccessCount++
			detail.Message = uploadResult.Message
			successIDs = append(successIDs, account.ID)
		} else {
			result.FailedCount++
			detail.Error = uploadResult.Message
		}
		result.Details = append(result.Details, detail)
	}

	if len(successIDs) > 0 {
		if err := s.accountStore.MarkSub2APIUploaded(ctx, successIDs, s.now()); err != nil {
			return Sub2APIUploadResult{}, err
		}
	}

	return result, nil
}

func (s *Service) ListTMServices(ctx context.Context, enabled *bool) ([]TMServiceResponse, error) {
	configs, err := s.listConfigs(ctx, UploadKindTM, enabled)
	if err != nil {
		return nil, err
	}

	items := make([]TMServiceResponse, 0, len(configs))
	for _, config := range configs {
		items = append(items, toTMServiceResponse(config))
	}
	return items, nil
}

func (s *Service) GetTMService(ctx context.Context, id int) (TMServiceResponse, error) {
	config, err := s.getConfig(ctx, UploadKindTM, id)
	if err != nil {
		return TMServiceResponse{}, err
	}
	return toTMServiceResponse(config), nil
}

func (s *Service) CreateTMService(ctx context.Context, req CreateTMServiceRequest) (TMServiceResponse, error) {
	created, err := s.createConfig(ctx, ManagedServiceConfig{
		ServiceConfig: ServiceConfig{
			Kind:       UploadKindTM,
			Name:       req.Name,
			BaseURL:    req.APIURL,
			Credential: req.APIKey,
			Enabled:    req.Enabled,
			Priority:   req.Priority,
		},
	})
	if err != nil {
		return TMServiceResponse{}, err
	}
	return toTMServiceResponse(created), nil
}

func (s *Service) UpdateTMService(ctx context.Context, id int, req UpdateTMServiceRequest) (TMServiceResponse, error) {
	updated, err := s.updateConfig(ctx, UploadKindTM, id, ManagedServiceConfigPatch{
		Name:       cloneTrimmedString(req.Name),
		BaseURL:    cloneTrimmedString(req.APIURL),
		Credential: req.APIKey,
		Enabled:    req.Enabled,
		Priority:   req.Priority,
	})
	if err != nil {
		return TMServiceResponse{}, err
	}
	return toTMServiceResponse(updated), nil
}

func (s *Service) DeleteTMService(ctx context.Context, id int) (DeleteServiceResponse, error) {
	deleted, err := s.deleteConfig(ctx, UploadKindTM, id)
	if err != nil {
		return DeleteServiceResponse{}, err
	}
	return DeleteServiceResponse{
		Success: true,
		Message: fmt.Sprintf("Team Manager 服务 %s 已删除", deleted.Name),
	}, nil
}

func (s *Service) TestTMService(ctx context.Context, id int) (ConnectionTestResult, error) {
	config, err := s.getConfig(ctx, UploadKindTM, id)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	return s.TestTMConnection(ctx, TMConnectionTestRequest{
		APIURL: config.BaseURL,
		APIKey: config.Credential,
	})
}

func (s *Service) TestTMConnection(ctx context.Context, req TMConnectionTestRequest) (ConnectionTestResult, error) {
	if strings.TrimSpace(req.APIURL) == "" {
		return ConnectionTestResult{Success: false, Message: "API URL 不能为空"}, nil
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return ConnectionTestResult{Success: false, Message: "API Key 不能为空"}, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodOptions, joinURLPath(req.APIURL, "/admin/teams/import"), nil)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	request.Header.Set("X-API-Key", strings.TrimSpace(req.APIKey))

	statusCode, _, err := newHTTPClient(s.httpDoer).do(request)
	if err != nil {
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("连接测试失败: %v", err)}, nil
	}

	switch statusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusForbidden, http.StatusMethodNotAllowed:
		return ConnectionTestResult{Success: true, Message: "Team Manager 连接测试成功"}, nil
	case http.StatusUnauthorized:
		return ConnectionTestResult{Success: false, Message: "连接成功，但 API Key 无效"}, nil
	default:
		return ConnectionTestResult{Success: false, Message: fmt.Sprintf("服务器返回异常状态码: %d", statusCode)}, nil
	}
}

func (s *Service) listConfigs(ctx context.Context, kind UploadKind, enabled *bool) ([]ManagedServiceConfig, error) {
	if s == nil || s.repository == nil {
		return nil, ErrConfigRepositoryNotConfigured
	}
	return s.repository.ListServiceConfigs(ctx, kind, ServiceConfigListFilter{Enabled: cloneBool(enabled)})
}

func (s *Service) getConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, error) {
	if s == nil || s.repository == nil {
		return ManagedServiceConfig{}, ErrConfigRepositoryNotConfigured
	}
	config, found, err := s.repository.GetServiceConfig(ctx, kind, id)
	if err != nil {
		return ManagedServiceConfig{}, err
	}
	if !found {
		return ManagedServiceConfig{}, ErrServiceConfigNotFound
	}
	return config.Normalized(), nil
}

func (s *Service) createConfig(ctx context.Context, config ManagedServiceConfig) (ManagedServiceConfig, error) {
	if s == nil || s.repository == nil {
		return ManagedServiceConfig{}, ErrConfigRepositoryNotConfigured
	}
	return s.repository.CreateServiceConfig(ctx, config.Normalized())
}

func (s *Service) updateConfig(ctx context.Context, kind UploadKind, id int, patch ManagedServiceConfigPatch) (ManagedServiceConfig, error) {
	if s == nil || s.repository == nil {
		return ManagedServiceConfig{}, ErrConfigRepositoryNotConfigured
	}

	current, err := s.getConfig(ctx, kind, id)
	if err != nil {
		return ManagedServiceConfig{}, err
	}

	normalizedPatch := patch
	if normalizedPatch.Credential != nil && strings.TrimSpace(*normalizedPatch.Credential) == "" {
		normalizedPatch.Credential = stringPointer(current.Credential)
	}
	if normalizedPatch.Credential != nil {
		normalizedPatch.Credential = stringPointer(strings.TrimSpace(*normalizedPatch.Credential))
	}

	updated, found, err := s.repository.UpdateServiceConfig(ctx, kind, id, normalizedPatch)
	if err != nil {
		return ManagedServiceConfig{}, err
	}
	if !found {
		return ManagedServiceConfig{}, ErrServiceConfigNotFound
	}
	return updated.Normalized(), nil
}

func (s *Service) deleteConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, error) {
	if s == nil || s.repository == nil {
		return ManagedServiceConfig{}, ErrConfigRepositoryNotConfigured
	}
	deleted, found, err := s.repository.DeleteServiceConfig(ctx, kind, id)
	if err != nil {
		return ManagedServiceConfig{}, err
	}
	if !found {
		return ManagedServiceConfig{}, ErrServiceConfigNotFound
	}
	return deleted.Normalized(), nil
}

func (s *Service) resolveSub2APIUploadService(ctx context.Context, serviceID *int) (ManagedServiceConfig, error) {
	if serviceID != nil {
		config, found, err := s.repository.GetServiceConfig(ctx, UploadKindSub2API, *serviceID)
		if err != nil {
			return ManagedServiceConfig{}, err
		}
		if !found {
			return ManagedServiceConfig{}, ErrUploadServiceUnavailable
		}
		return config.Normalized(), nil
	}

	configs, err := s.listConfigs(ctx, UploadKindSub2API, boolPointer(true))
	if err != nil {
		return ManagedServiceConfig{}, err
	}
	if len(configs) == 0 {
		return ManagedServiceConfig{}, ErrUploadServiceUnavailable
	}
	return configs[0], nil
}

func toCPAServiceResponse(config ManagedServiceConfig) CPAServiceResponse {
	return CPAServiceResponse{
		ID:        config.ID,
		Name:      config.Name,
		APIURL:    config.BaseURL,
		ProxyURL:  config.ProxyURL,
		HasToken:  strings.TrimSpace(config.Credential) != "",
		Enabled:   config.Enabled,
		Priority:  config.Priority,
		CreatedAt: cloneTime(config.CreatedAt),
		UpdatedAt: cloneTime(config.UpdatedAt),
	}
}

func toSub2APIServiceResponse(config ManagedServiceConfig) Sub2APIServiceResponse {
	return Sub2APIServiceResponse{
		ID:        config.ID,
		Name:      config.Name,
		APIURL:    config.BaseURL,
		HasKey:    strings.TrimSpace(config.Credential) != "",
		Enabled:   config.Enabled,
		Priority:  config.Priority,
		CreatedAt: cloneTime(config.CreatedAt),
		UpdatedAt: cloneTime(config.UpdatedAt),
	}
}

func toTMServiceResponse(config ManagedServiceConfig) TMServiceResponse {
	return TMServiceResponse{
		ID:        config.ID,
		Name:      config.Name,
		APIURL:    config.BaseURL,
		HasKey:    strings.TrimSpace(config.Credential) != "",
		Enabled:   config.Enabled,
		Priority:  config.Priority,
		CreatedAt: cloneTime(config.CreatedAt),
		UpdatedAt: cloneTime(config.UpdatedAt),
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringPointer(value string) *string {
	return &value
}

func cloneTrimmedString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func cloneNullableString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
