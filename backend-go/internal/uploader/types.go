package uploader

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	DefaultSub2APITargetType     = "sub2api"
	DefaultSub2APIConcurrency    = 3
	DefaultSub2APIPriority       = 50
	DefaultSub2APIRateMultiplier = 1
)

var (
	ErrUploadKindInvalid               = errors.New("uploader: invalid upload kind")
	ErrUploadAccountEmailMissing       = errors.New("uploader: account email is required")
	ErrUploadAccessTokenMissing        = errors.New("uploader: access token is required")
	ErrUploadAccountsEmpty             = errors.New("uploader: at least one upload account is required")
	ErrUploadServiceBaseURLEmpty       = errors.New("uploader: service base url is required")
	ErrUploadCredentialMissing         = errors.New("uploader: service credential is required")
	ErrConfigRepositoryNotConfigured   = errors.New("uploader: config repository is not configured")
	ErrUploadAccountStoreNotConfigured = errors.New("uploader: upload account store is not configured")
	ErrServiceConfigNotFound           = errors.New("uploader: service config not found")
	ErrUploadServiceUnavailable        = errors.New("uploader: upload service is unavailable")
	ErrSub2APITargetTypeInvalid        = errors.New("uploader: invalid sub2api target type")
)

type UploadKind string

const (
	UploadKindCPA     UploadKind = "cpa"
	UploadKindSub2API UploadKind = "sub2api"
	UploadKindTM      UploadKind = "tm"
)

func (k UploadKind) Valid() bool {
	switch k {
	case UploadKindCPA, UploadKindSub2API, UploadKindTM:
		return true
	default:
		return false
	}
}

type ServiceConfig struct {
	ID         int        `json:"id"`
	Kind       UploadKind `json:"kind"`
	Name       string     `json:"name"`
	BaseURL    string     `json:"base_url"`
	Credential string     `json:"credential"`
	TargetType string     `json:"target_type,omitempty"`
	ProxyURL   string     `json:"proxy_url,omitempty"`
	Enabled    bool       `json:"enabled"`
	Priority   int        `json:"priority"`
}

func (c ServiceConfig) Normalized() ServiceConfig {
	normalized := c
	normalized.Name = strings.TrimSpace(normalized.Name)
	normalized.BaseURL = strings.TrimSpace(normalized.BaseURL)
	normalized.Credential = strings.TrimSpace(normalized.Credential)
	normalized.TargetType = strings.TrimSpace(normalized.TargetType)
	normalized.ProxyURL = strings.TrimSpace(normalized.ProxyURL)

	if normalized.Kind == UploadKindSub2API && normalized.TargetType == "" {
		normalized.TargetType = DefaultSub2APITargetType
	}

	return normalized
}

type ManagedServiceConfig struct {
	ServiceConfig
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func (c ManagedServiceConfig) Normalized() ManagedServiceConfig {
	normalized := c
	normalized.ServiceConfig = normalized.ServiceConfig.Normalized()
	normalized.CreatedAt = cloneTime(normalized.CreatedAt)
	normalized.UpdatedAt = cloneTime(normalized.UpdatedAt)
	return normalized
}

type ServiceConfigListFilter struct {
	Enabled *bool
}

type ManagedServiceConfigPatch struct {
	Name       *string
	BaseURL    *string
	Credential *string
	TargetType *string
	ProxyURL   *string
	Enabled    *bool
	Priority   *int
}

type AdminRepository interface {
	ListServiceConfigs(ctx context.Context, kind UploadKind, filter ServiceConfigListFilter) ([]ManagedServiceConfig, error)
	GetServiceConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error)
	CreateServiceConfig(ctx context.Context, config ManagedServiceConfig) (ManagedServiceConfig, error)
	UpdateServiceConfig(ctx context.Context, kind UploadKind, id int, patch ManagedServiceConfigPatch) (ManagedServiceConfig, bool, error)
	DeleteServiceConfig(ctx context.Context, kind UploadKind, id int) (ManagedServiceConfig, bool, error)
}

type CPAServiceResponse struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	APIURL    string     `json:"api_url"`
	ProxyURL  string     `json:"proxy_url,omitempty"`
	HasToken  bool       `json:"has_token"`
	Enabled   bool       `json:"enabled"`
	Priority  int        `json:"priority"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type CPAServiceFullResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

type Sub2APIServiceResponse struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	APIURL    string     `json:"api_url"`
	HasKey    bool       `json:"has_key"`
	Enabled   bool       `json:"enabled"`
	Priority  int        `json:"priority"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type Sub2APIServiceFullResponse struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	APIURL     string `json:"api_url"`
	APIKey     string `json:"api_key"`
	TargetType string `json:"target_type"`
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"`
}

type TMServiceResponse struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	APIURL    string     `json:"api_url"`
	HasKey    bool       `json:"has_key"`
	Enabled   bool       `json:"enabled"`
	Priority  int        `json:"priority"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type DeleteServiceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ConnectionTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type CPAConnectionTestRequest struct {
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
}

type Sub2APIConnectionTestRequest struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
}

type TMConnectionTestRequest struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
}

type UploadAccountStore interface {
	ListUploadAccounts(ctx context.Context, ids []int) ([]UploadAccount, error)
	MarkSub2APIUploaded(ctx context.Context, ids []int, uploadedAt time.Time) error
}

type Sub2APIUploadRequest struct {
	AccountIDs  []int `json:"account_ids"`
	ServiceID   *int  `json:"service_id,omitempty"`
	Concurrency int   `json:"concurrency"`
	Priority    int   `json:"priority"`
}

func (r Sub2APIUploadRequest) Normalized() Sub2APIUploadRequest {
	normalized := r
	normalized.AccountIDs = append([]int(nil), normalized.AccountIDs...)
	if normalized.Concurrency <= 0 {
		normalized.Concurrency = DefaultSub2APIConcurrency
	}
	if normalized.Priority <= 0 {
		normalized.Priority = DefaultSub2APIPriority
	}
	if normalized.ServiceID != nil {
		value := *normalized.ServiceID
		normalized.ServiceID = &value
	}
	return normalized
}

type Sub2APIUploadDetail struct {
	ID      int     `json:"id"`
	Email   *string `json:"email"`
	Success bool    `json:"success"`
	Message string  `json:"message,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type Sub2APIUploadResult struct {
	SuccessCount int                   `json:"success_count"`
	FailedCount  int                   `json:"failed_count"`
	SkippedCount int                   `json:"skipped_count"`
	Details      []Sub2APIUploadDetail `json:"details"`
}

type CreateCPAServiceRequest struct {
	Name     string `json:"name"`
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

type UpdateCPAServiceRequest struct {
	Name     *string `json:"name,omitempty"`
	APIURL   *string `json:"api_url,omitempty"`
	APIToken *string `json:"api_token,omitempty"`
	ProxyURL *string `json:"proxy_url,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
	Priority *int    `json:"priority,omitempty"`
}

type CreateSub2APIServiceRequest struct {
	Name       string `json:"name"`
	APIURL     string `json:"api_url"`
	APIKey     string `json:"api_key"`
	TargetType string `json:"target_type,omitempty"`
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"`
}

type UpdateSub2APIServiceRequest struct {
	Name       *string `json:"name,omitempty"`
	APIURL     *string `json:"api_url,omitempty"`
	APIKey     *string `json:"api_key,omitempty"`
	TargetType *string `json:"target_type,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
	Priority   *int    `json:"priority,omitempty"`
}

type CreateTMServiceRequest struct {
	Name     string `json:"name"`
	APIURL   string `json:"api_url"`
	APIKey   string `json:"api_key"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

type UpdateTMServiceRequest struct {
	Name     *string `json:"name,omitempty"`
	APIURL   *string `json:"api_url,omitempty"`
	APIKey   *string `json:"api_key,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
	Priority *int    `json:"priority,omitempty"`
}

func normalizeSub2APITargetType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return DefaultSub2APITargetType, nil
	}
	switch normalized {
	case DefaultSub2APITargetType, "newapi":
		return normalized, nil
	default:
		return "", ErrSub2APITargetTypeInvalid
	}
}

type UploadAccount struct {
	ID           int        `json:"id"`
	Email        string     `json:"email"`
	AccessToken  string     `json:"access_token,omitempty"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	SessionToken string     `json:"session_token,omitempty"`
	ClientID     string     `json:"client_id,omitempty"`
	AccountID    string     `json:"account_id,omitempty"`
	WorkspaceID  string     `json:"workspace_id,omitempty"`
	IDToken      string     `json:"id_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	LastRefresh  *time.Time `json:"last_refresh,omitempty"`
}

func (a UploadAccount) Normalized() UploadAccount {
	normalized := a
	normalized.Email = strings.TrimSpace(normalized.Email)
	normalized.AccessToken = strings.TrimSpace(normalized.AccessToken)
	normalized.RefreshToken = strings.TrimSpace(normalized.RefreshToken)
	normalized.SessionToken = strings.TrimSpace(normalized.SessionToken)
	normalized.ClientID = strings.TrimSpace(normalized.ClientID)
	normalized.AccountID = strings.TrimSpace(normalized.AccountID)
	normalized.WorkspaceID = strings.TrimSpace(normalized.WorkspaceID)
	normalized.IDToken = strings.TrimSpace(normalized.IDToken)
	normalized.ExpiresAt = cloneTime(normalized.ExpiresAt)
	normalized.LastRefresh = cloneTime(normalized.LastRefresh)
	return normalized
}

type UploadResult struct {
	Kind         UploadKind `json:"kind"`
	ServiceID    int        `json:"service_id"`
	AccountEmail string     `json:"account_email,omitempty"`
	Success      bool       `json:"success"`
	Message      string     `json:"message,omitempty"`
}

type ConfigRepository interface {
	ListCPAServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error)
	ListSub2APIServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error)
	ListTMServiceConfigs(ctx context.Context, ids []int) ([]ServiceConfig, error)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
