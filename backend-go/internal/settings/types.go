package settings

import (
	"errors"
	"time"
)

var (
	ErrRepositoryNotConfigured    = errors.New("settings repository is not configured")
	ErrProxyNotFound              = errors.New("proxy not found")
	ErrInvalidProxyName           = errors.New("proxy name is required")
	ErrInvalidProxyHost           = errors.New("proxy host is required")
	ErrInvalidProxyType           = errors.New("proxy type must be http or socks5")
	ErrInvalidProxyPort           = errors.New("proxy port must be between 1 and 65535")
	ErrInvalidRegistrationFlow    = errors.New("entry_flow must be native or abcard")
	ErrInvalidEmailCodeTimeout    = errors.New("timeout must be between 30 and 600")
	ErrInvalidEmailCodePollPeriod = errors.New("poll_interval must be between 1 and 30")
)

type SettingRecord struct {
	Key         string
	Value       string
	Description string
	Category    string
	UpdatedAt   time.Time
}

type ProxyRecord struct {
	ID        int
	Name      string
	Type      string
	Host      string
	Port      int
	Username  *string
	Password  *string
	Enabled   bool
	IsDefault bool
	Priority  int
	LastUsed  *time.Time
	CreatedAt *time.Time
	UpdatedAt *time.Time
	ProxyURL  string
}

type ProxyPayload struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    *string    `json:"username,omitempty"`
	Password    *string    `json:"password,omitempty"`
	HasPassword bool       `json:"has_password,omitempty"`
	Enabled     bool       `json:"enabled"`
	IsDefault   bool       `json:"is_default"`
	Priority    int        `json:"priority"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type ProxyListResponse struct {
	Proxies []ProxyPayload `json:"proxies"`
	Total   int            `json:"total"`
}

type ProxySettingsResponse struct {
	Enabled             bool    `json:"enabled"`
	Type                string  `json:"type"`
	Host                string  `json:"host"`
	Port                int     `json:"port"`
	Username            *string `json:"username"`
	HasPassword         bool    `json:"has_password"`
	DynamicEnabled      bool    `json:"dynamic_enabled"`
	DynamicAPIURL       string  `json:"dynamic_api_url"`
	DynamicAPIKeyHeader string  `json:"dynamic_api_key_header"`
	DynamicResultField  string  `json:"dynamic_result_field"`
	HasDynamicAPIKey    bool    `json:"has_dynamic_api_key"`
}

type RegistrationSettingsResponse struct {
	MaxRetries                    int    `json:"max_retries"`
	Timeout                       int    `json:"timeout"`
	DefaultPasswordLength         int    `json:"default_password_length"`
	SleepMin                      int    `json:"sleep_min"`
	SleepMax                      int    `json:"sleep_max"`
	EntryFlow                     string `json:"entry_flow"`
	TokenCompletionMaxConcurrency int    `json:"token_completion_max_concurrency"`
}

type WebUISettingsResponse struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Debug             bool   `json:"debug"`
	HasAccessPassword bool   `json:"has_access_password"`
}

type TempmailProviderResponse struct {
	APIURL     string `json:"api_url"`
	BaseURL    string `json:"base_url"`
	Timeout    int    `json:"timeout"`
	MaxRetries int    `json:"max_retries"`
	Enabled    bool   `json:"enabled"`
}

type YYDSMailResponse struct {
	APIURL        string `json:"api_url"`
	BaseURL       string `json:"base_url"`
	DefaultDomain string `json:"default_domain"`
	Timeout       int    `json:"timeout"`
	MaxRetries    int    `json:"max_retries"`
	Enabled       bool   `json:"enabled"`
	HasAPIKey     bool   `json:"has_api_key"`
}

type TempmailSettingsResponse struct {
	Tempmail TempmailProviderResponse `json:"tempmail"`
	YYDSMail YYDSMailResponse         `json:"yyds_mail"`
}

type EmailCodeSettingsResponse struct {
	Timeout      int `json:"timeout"`
	PollInterval int `json:"poll_interval"`
}

type OutlookSettingsResponse struct {
	DefaultClientID        string   `json:"default_client_id"`
	ProviderPriority       []string `json:"provider_priority"`
	HealthFailureThreshold int      `json:"health_failure_threshold"`
	HealthDisableDuration  int      `json:"health_disable_duration"`
}

type AllSettingsResponse struct {
	Proxy        ProxySettingsResponse        `json:"proxy"`
	Registration RegistrationSettingsResponse `json:"registration"`
	WebUI        WebUISettingsResponse        `json:"webui"`
	Tempmail     TempmailProviderResponse     `json:"tempmail"`
	YYDSMail     YYDSMailResponse             `json:"yyds_mail"`
	EmailCode    EmailCodeSettingsResponse    `json:"email_code"`
}

type MutationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type CreateProxyRequest struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	Enabled  bool    `json:"enabled"`
	Priority int     `json:"priority"`
}

type UpdateProxyRequest struct {
	Name      *string `json:"name"`
	Type      *string `json:"type"`
	Host      *string `json:"host"`
	Port      *int    `json:"port"`
	Username  *string `json:"username"`
	Password  *string `json:"password"`
	Enabled   *bool   `json:"enabled"`
	Priority  *int    `json:"priority"`
	IsDefault *bool   `json:"is_default"`
}

type UpdateDynamicProxySettingsRequest struct {
	Enabled      bool    `json:"enabled"`
	APIURL       string  `json:"api_url"`
	APIKey       *string `json:"api_key"`
	APIKeyHeader string  `json:"api_key_header"`
	ResultField  string  `json:"result_field"`
}

type DynamicProxySettingsResponse struct {
	Enabled      bool   `json:"enabled"`
	APIURL       string `json:"api_url"`
	APIKeyHeader string `json:"api_key_header"`
	ResultField  string `json:"result_field"`
	HasAPIKey    bool   `json:"has_api_key"`
}

type UpdateRegistrationSettingsRequest struct {
	MaxRetries                    int    `json:"max_retries"`
	Timeout                       int    `json:"timeout"`
	DefaultPasswordLength         int    `json:"default_password_length"`
	SleepMin                      int    `json:"sleep_min"`
	SleepMax                      int    `json:"sleep_max"`
	EntryFlow                     string `json:"entry_flow"`
	TokenCompletionMaxConcurrency int    `json:"token_completion_max_concurrency"`
}

type UpdateWebUISettingsRequest struct {
	Host           *string `json:"host"`
	Port           *int    `json:"port"`
	Debug          *bool   `json:"debug"`
	AccessPassword *string `json:"access_password"`
}

type UpdateTempmailSettingsRequest struct {
	APIURL            *string `json:"api_url"`
	Enabled           *bool   `json:"enabled"`
	YYDSAPIURL        *string `json:"yyds_api_url"`
	YYDSAPIKey        *string `json:"yyds_api_key"`
	YYDSDefaultDomain *string `json:"yyds_default_domain"`
	YYDSEnabled       *bool   `json:"yyds_enabled"`
}

type UpdateEmailCodeSettingsRequest struct {
	Timeout      int `json:"timeout"`
	PollInterval int `json:"poll_interval"`
}

type UpdateOutlookSettingsRequest struct {
	DefaultClientID *string `json:"default_client_id"`
}
