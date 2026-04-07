package mail

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func NewProvider(serviceType string, config map[string]any) (Provider, error) {
	pollIntervalSeconds, _ := intConfig(config, "poll_interval")
	pollInterval := time.Duration(pollIntervalSeconds) * time.Second

	switch strings.ToLower(strings.TrimSpace(serviceType)) {
	case "tempmail":
		return NewTempmail(Config{
			BaseURL:      stringConfig(config, "base_url"),
			HTTPClient:   newHTTPClient(config),
			PollInterval: pollInterval,
		}), nil
	case "temp_mail":
		return NewSelfHostedTempMail(SelfHostedTempMailConfig{
			BaseURL:       stringConfig(config, "base_url"),
			AdminPassword: stringConfig(config, "admin_password"),
			CustomAuth:    stringConfig(config, "custom_auth"),
			Domain:        stringConfig(config, "domain"),
			EnablePrefix:  boolConfig(config, "enable_prefix"),
			HTTPClient:    newHTTPClient(config),
			PollInterval:  pollInterval,
		}), nil
	case "freemail":
		return NewFreemail(FreemailConfig{
			BaseURL:      stringConfig(config, "base_url"),
			AdminToken:   stringConfig(config, "admin_token"),
			Domain:       stringConfig(config, "domain"),
			LocalPart:    stringConfig(config, "local_part"),
			HTTPClient:   newHTTPClient(config),
			PollInterval: pollInterval,
		}), nil
	case "yyds_mail", "yydsmail":
		defaultDomain := stringConfig(config, "default_domain")
		if defaultDomain == "" {
			defaultDomain = stringConfig(config, "domain")
		}
		return NewYYDSMail(YYDSMailConfig{
			BaseURL:       stringConfig(config, "base_url"),
			APIKey:        stringConfig(config, "api_key"),
			DefaultDomain: defaultDomain,
			HTTPClient:    newHTTPClient(config),
			PollInterval:  pollInterval,
		}), nil
	case "duckmail", "duck_mail":
		defaultDomain := stringConfig(config, "default_domain")
		if defaultDomain == "" {
			defaultDomain = stringConfig(config, "domain")
		}
		return NewDuckMail(DuckMailConfig{
			BaseURL:       stringConfig(config, "base_url"),
			DefaultDomain: defaultDomain,
			APIKey:        stringConfig(config, "api_key"),
			HTTPClient:    newHTTPClient(config),
			PollInterval:  pollInterval,
		}), nil
	case "luckmail", "luck_mail":
		preferredDomain := stringConfig(config, "preferred_domain")
		if preferredDomain == "" {
			preferredDomain = stringConfig(config, "domain")
		}
		return NewLuckMail(LuckMailConfig{
			BaseURL:         stringConfig(config, "base_url"),
			APIKey:          stringConfig(config, "api_key"),
			ProjectCode:     stringConfig(config, "project_code"),
			EmailType:       stringConfig(config, "email_type"),
			PreferredDomain: preferredDomain,
			HTTPClient:      newHTTPClient(config),
			PollInterval:    pollInterval,
		}), nil
	case "moe_mail":
		defaultDomain := stringConfig(config, "default_domain")
		if defaultDomain == "" {
			defaultDomain = stringConfig(config, "domain")
		}
		return NewMoeMail(MoeMailConfig{
			BaseURL:       stringConfig(config, "base_url"),
			APIKey:        stringConfig(config, "api_key"),
			DefaultDomain: defaultDomain,
			HTTPClient:    newHTTPClient(config),
			PollInterval:  pollInterval,
		}), nil
	case "imap_mail", "imap":
		timeoutSeconds, _ := intConfig(config, "timeout")
		return NewIMAPMail(IMAPConfig{
			Host:         stringConfig(config, "host"),
			Port:         intValue(config, "port"),
			Email:        stringConfig(config, "email"),
			Username:     stringConfig(config, "username"),
			Password:     stringConfig(config, "password"),
			ProxyURL:     stringConfig(config, "proxy_url"),
			UseSSL:       !boolConfig(config, "disable_ssl"),
			DialTimeout:  time.Duration(timeoutSeconds) * time.Second,
			PollInterval: time.Duration(pollIntervalSeconds) * time.Second,
		}), nil
	case "outlook":
		return NewOutlook(OutlookConfig{
			Email:        stringConfig(config, "email"),
			Password:     stringConfig(config, "password"),
			ClientID:     stringConfig(config, "client_id"),
			RefreshToken: stringConfig(config, "refresh_token"),
			ProxyURL:     stringConfig(config, "proxy_url"),
		}), nil
	default:
		return nil, fmt.Errorf("unsupported native mail provider: %s", strings.TrimSpace(serviceType))
	}
}

func newHTTPClient(config map[string]any) *http.Client {
	timeoutSeconds, ok := intConfig(config, "timeout")
	proxyURL := stringConfig(config, "proxy_url")
	if (!ok || timeoutSeconds <= 0) && proxyURL == "" {
		return nil
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	if !ok || timeoutSeconds <= 0 {
		timeout = defaultHTTPClientTimeout
	}
	return proxyAwareHTTPClient(timeout, proxyURL)
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

func intConfig(config map[string]any, key string) (int, bool) {
	if config == nil {
		return 0, false
	}

	raw, ok := config[key]
	if !ok || raw == nil {
		return 0, false
	}

	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func intValue(config map[string]any, key string) int {
	value, _ := intConfig(config, key)
	return value
}

func boolConfig(config map[string]any, key string) bool {
	if config == nil {
		return false
	}

	raw, ok := config[key]
	if !ok || raw == nil {
		return false
	}

	value, ok := raw.(bool)
	return ok && value
}
