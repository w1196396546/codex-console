package registration

import (
	"context"
	"strings"
)

type AvailableServiceGroup struct {
	Available bool             `json:"available"`
	Count     int              `json:"count"`
	Services  []map[string]any `json:"services"`
}

type AvailableServicesResponse map[string]AvailableServiceGroup

type EmailServiceRecord struct {
	ID          int
	ServiceType string
	Name        string
	Config      map[string]any
	Priority    int
}

type availableServicesRepository interface {
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
	ListEmailServices(ctx context.Context) ([]EmailServiceRecord, error)
}

type AvailableServicesService struct {
	repo availableServicesRepository
}

func NewAvailableServicesService(repo availableServicesRepository) *AvailableServicesService {
	return &AvailableServicesService{repo: repo}
}

func (s *AvailableServicesService) ListAvailableServices(ctx context.Context) (AvailableServicesResponse, error) {
	settings, err := s.repo.GetSettings(ctx, []string{
		"tempmail.enabled",
		"yyds_mail.enabled",
		"yyds_mail.api_key",
		"yyds_mail.default_domain",
		"custom_domain.base_url",
		"custom_domain.api_key",
	})
	if err != nil {
		return nil, err
	}

	services, err := s.repo.ListEmailServices(ctx)
	if err != nil {
		return nil, err
	}

	return BuildAvailableServices(settings, services), nil
}

func BuildAvailableServices(settings map[string]string, services []EmailServiceRecord) AvailableServicesResponse {
	result := AvailableServicesResponse{
		"tempmail":  newAvailableServiceGroup(),
		"yyds_mail": newAvailableServiceGroup(),
		"outlook":   newAvailableServiceGroup(),
		"moe_mail":  newAvailableServiceGroup(),
		"temp_mail": newAvailableServiceGroup(),
		"duck_mail": newAvailableServiceGroup(),
		"freemail":  newAvailableServiceGroup(),
		"imap_mail": newAvailableServiceGroup(),
		"luckmail":  newAvailableServiceGroup(),
	}

	if parseBoolSetting(settings["tempmail.enabled"]) {
		appendAvailableService(result, "tempmail", map[string]any{
			"id":          nil,
			"name":        "Tempmail.lol",
			"type":        "tempmail",
			"description": "临时邮箱，自动创建",
		})
	}

	if parseBoolSetting(settings["yyds_mail.enabled"]) && strings.TrimSpace(settings["yyds_mail.api_key"]) != "" {
		appendAvailableService(result, "yyds_mail", map[string]any{
			"id":             nil,
			"name":           "YYDS Mail",
			"type":           "yyds_mail",
			"default_domain": emptyStringAsNil(settings["yyds_mail.default_domain"]),
			"description":    "YYDS Mail API 临时邮箱",
		})
	}

	for _, service := range services {
		switch service.ServiceType {
		case "yyds_mail":
			appendAvailableService(result, "yyds_mail", map[string]any{
				"id":             service.ID,
				"name":           service.Name,
				"type":           "yyds_mail",
				"default_domain": stringConfig(service.Config, "default_domain"),
				"priority":       service.Priority,
			})
		case "outlook":
			appendAvailableService(result, "outlook", map[string]any{
				"id":        service.ID,
				"name":      service.Name,
				"type":      "outlook",
				"has_oauth": stringConfig(service.Config, "client_id") != "" && stringConfig(service.Config, "refresh_token") != "",
				"priority":  service.Priority,
			})
		case "moe_mail":
			appendAvailableService(result, "moe_mail", map[string]any{
				"id":             service.ID,
				"name":           service.Name,
				"type":           "moe_mail",
				"default_domain": stringConfig(service.Config, "default_domain"),
				"priority":       service.Priority,
			})
		case "temp_mail":
			appendAvailableService(result, "temp_mail", map[string]any{
				"id":       service.ID,
				"name":     service.Name,
				"type":     "temp_mail",
				"domain":   stringConfig(service.Config, "domain"),
				"priority": service.Priority,
			})
		case "duck_mail":
			appendAvailableService(result, "duck_mail", map[string]any{
				"id":             service.ID,
				"name":           service.Name,
				"type":           "duck_mail",
				"default_domain": stringConfig(service.Config, "default_domain"),
				"priority":       service.Priority,
			})
		case "freemail":
			appendAvailableService(result, "freemail", map[string]any{
				"id":       service.ID,
				"name":     service.Name,
				"type":     "freemail",
				"domain":   stringConfig(service.Config, "domain"),
				"priority": service.Priority,
			})
		case "imap_mail":
			appendAvailableService(result, "imap_mail", map[string]any{
				"id":       service.ID,
				"name":     service.Name,
				"type":     "imap_mail",
				"email":    stringConfig(service.Config, "email"),
				"host":     stringConfig(service.Config, "host"),
				"priority": service.Priority,
			})
		case "luckmail":
			appendAvailableService(result, "luckmail", map[string]any{
				"id":               service.ID,
				"name":             service.Name,
				"type":             "luckmail",
				"project_code":     stringConfig(service.Config, "project_code"),
				"email_type":       stringConfig(service.Config, "email_type"),
				"preferred_domain": stringConfig(service.Config, "preferred_domain"),
				"priority":         service.Priority,
			})
		}
	}

	if result["moe_mail"].Count == 0 &&
		strings.TrimSpace(settings["custom_domain.base_url"]) != "" &&
		strings.TrimSpace(settings["custom_domain.api_key"]) != "" {
		appendAvailableService(result, "moe_mail", map[string]any{
			"id":            nil,
			"name":          "默认自定义域名服务",
			"type":          "moe_mail",
			"from_settings": true,
		})
	}

	return result
}

func newAvailableServiceGroup() AvailableServiceGroup {
	return AvailableServiceGroup{
		Available: false,
		Count:     0,
		Services:  make([]map[string]any, 0),
	}
}

func appendAvailableService(result AvailableServicesResponse, key string, item map[string]any) {
	group := result[key]
	group.Services = append(group.Services, item)
	group.Count = len(group.Services)
	group.Available = group.Count > 0
	result[key] = group
}

func parseBoolSetting(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func emptyStringAsNil(raw string) any {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
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
