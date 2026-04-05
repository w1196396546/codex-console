package uploader

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	repository AdminRepository
}

func NewService(repository AdminRepository) *Service {
	return &Service{repository: repository}
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
