package accounts

import (
	"errors"
	"strings"
	"time"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100

	DefaultAccountStatus = "active"
	DefaultAccountSource = "register"

	CurrentAccountSettingKey = "codex.current_account_id"
	OverviewExtraDataKey     = "codex_overview"
	OverviewCardRemovedKey   = "codex_overview_card_removed"
	OverviewCacheTTLSeconds  = 300
)

var (
	ErrAccountEmailRequired        = errors.New("accounts: email is required")
	ErrAccountEmailServiceRequired = errors.New("accounts: email_service is required for new account")
	ErrAccountNotFound             = errors.New("accounts: account not found")
	ErrRepositoryNotConfigured     = errors.New("accounts: repository not configured")
)

type Account struct {
	ID                int            `json:"id"`
	Email             string         `json:"email"`
	Password          string         `json:"password"`
	ClientID          string         `json:"client_id,omitempty"`
	SessionToken      string         `json:"session_token,omitempty"`
	EmailService      string         `json:"email_service,omitempty"`
	EmailServiceID    string         `json:"email_service_id,omitempty"`
	AccountID         string         `json:"account_id,omitempty"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	AccessToken       string         `json:"access_token,omitempty"`
	RefreshToken      string         `json:"refresh_token,omitempty"`
	IDToken           string         `json:"id_token,omitempty"`
	Cookies           string         `json:"cookies,omitempty"`
	ProxyUsed         string         `json:"proxy_used,omitempty"`
	LastRefresh       *time.Time     `json:"last_refresh,omitempty"`
	ExpiresAt         *time.Time     `json:"expires_at,omitempty"`
	ExtraData         map[string]any `json:"-"`
	CPAUploaded       bool           `json:"cpa_uploaded,omitempty"`
	CPAUploadedAt     *time.Time     `json:"cpa_uploaded_at,omitempty"`
	Sub2APIUploaded   bool           `json:"sub2api_uploaded,omitempty"`
	Sub2APIUploadedAt *time.Time     `json:"sub2api_uploaded_at,omitempty"`
	Status            string         `json:"status"`
	Source            string         `json:"source,omitempty"`
	SubscriptionType  string         `json:"subscription_type,omitempty"`
	SubscriptionAt    *time.Time     `json:"subscription_at,omitempty"`
	RegisteredAt      *time.Time     `json:"registered_at,omitempty"`
	CreatedAt         *time.Time     `json:"created_at,omitempty"`
	UpdatedAt         *time.Time     `json:"updated_at,omitempty"`

	HasRefreshToken    bool           `json:"has_refresh_token,omitempty"`
	TeamRoleBadges     []string       `json:"team_role_badges,omitempty"`
	TeamRelationSummary map[string]any `json:"team_relation_summary"`
	TeamRelationCount  int            `json:"team_relation_count"`
	DeviceID           string         `json:"device_id,omitempty"`
}

type ListAccountsRequest struct {
	Page              int    `json:"page"`
	PageSize          int    `json:"page_size"`
	Status            string `json:"status,omitempty"`
	EmailService      string `json:"email_service,omitempty"`
	RefreshTokenState string `json:"refresh_token_state,omitempty"`
	Search            string `json:"search,omitempty"`
}

type AccountOverviewCardsRequest struct {
	Search       string `json:"search,omitempty"`
	Status       string `json:"status,omitempty"`
	EmailService string `json:"email_service,omitempty"`
	Refresh      bool   `json:"refresh,omitempty"`
	Proxy        string `json:"proxy,omitempty"`
}

type AccountOverviewSelectableRequest struct {
	Search       string `json:"search,omitempty"`
	Status       string `json:"status,omitempty"`
	EmailService string `json:"email_service,omitempty"`
}

type UpsertAccountRequest struct {
	Email             string         `json:"email"`
	Password          string         `json:"password,omitempty"`
	ClientID          string         `json:"client_id,omitempty"`
	SessionToken      string         `json:"session_token,omitempty"`
	EmailService      string         `json:"email_service,omitempty"`
	EmailServiceID    string         `json:"email_service_id,omitempty"`
	AccountID         string         `json:"account_id,omitempty"`
	WorkspaceID       string         `json:"workspace_id,omitempty"`
	AccessToken       string         `json:"access_token,omitempty"`
	RefreshToken      string         `json:"refresh_token,omitempty"`
	IDToken           string         `json:"id_token,omitempty"`
	Cookies           string         `json:"cookies,omitempty"`
	ProxyUsed         string         `json:"proxy_used,omitempty"`
	LastRefresh       *time.Time     `json:"last_refresh,omitempty"`
	ExpiresAt         *time.Time     `json:"expires_at,omitempty"`
	ExtraData         map[string]any `json:"extra_data,omitempty"`
	CPAUploaded       *bool          `json:"cpa_uploaded,omitempty"`
	CPAUploadedAt     *time.Time     `json:"cpa_uploaded_at,omitempty"`
	Sub2APIUploaded   *bool          `json:"sub2api_uploaded,omitempty"`
	Sub2APIUploadedAt *time.Time     `json:"sub2api_uploaded_at,omitempty"`
	Status            string         `json:"status,omitempty"`
	Source            string         `json:"source,omitempty"`
	SubscriptionType  string         `json:"subscription_type,omitempty"`
	SubscriptionAt    *time.Time     `json:"subscription_at,omitempty"`
	RegisteredAt      *time.Time     `json:"registered_at,omitempty"`
}

type AccountListResponse struct {
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int       `json:"total"`
	Accounts []Account `json:"accounts"`
}

type CurrentAccountSummary struct {
	ID           int    `json:"id"`
	Email        string `json:"email"`
	Status       string `json:"status"`
	EmailService string `json:"email_service"`
	PlanType     string `json:"plan_type"`
}

type CurrentAccountResponse struct {
	CurrentAccountID *int                  `json:"current_account_id"`
	Account          *CurrentAccountSummary `json:"account"`
}

type AccountsStatsSummary struct {
	Total          int            `json:"total"`
	ByStatus       map[string]int `json:"by_status"`
	ByEmailService map[string]int `json:"by_email_service"`
}

type AccountTokenStats struct {
	WithAccessToken    int `json:"with_access_token"`
	WithRefreshToken   int `json:"with_refresh_token"`
	WithoutAccessToken int `json:"without_access_token"`
}

type AccountOverviewRecentItem struct {
	ID               int    `json:"id"`
	Email            string `json:"email"`
	Status           string `json:"status"`
	EmailService     string `json:"email_service"`
	Source           string `json:"source"`
	SubscriptionType string `json:"subscription_type"`
	CreatedAt        string `json:"created_at,omitempty"`
	LastRefresh      string `json:"last_refresh,omitempty"`
}

type AccountsOverviewStats struct {
	Total            int                         `json:"total"`
	ActiveCount      int                         `json:"active_count"`
	TokenStats       AccountTokenStats           `json:"token_stats"`
	CPAUploadedCount int                         `json:"cpa_uploaded_count"`
	ByStatus         map[string]int              `json:"by_status"`
	ByEmailService   map[string]int              `json:"by_email_service"`
	BySource         map[string]int              `json:"by_source"`
	BySubscription   map[string]int              `json:"by_subscription"`
	RecentAccounts   []AccountOverviewRecentItem `json:"recent_accounts"`
}

type AccountOverviewCard struct {
	ID               int            `json:"id"`
	Email            string         `json:"email"`
	Status           string         `json:"status"`
	EmailService     string         `json:"email_service"`
	CreatedAt        string         `json:"created_at,omitempty"`
	LastRefresh      string         `json:"last_refresh,omitempty"`
	Current          bool           `json:"current"`
	HasAccessToken   bool           `json:"has_access_token"`
	PlanType         string         `json:"plan_type"`
	PlanSource       string         `json:"plan_source"`
	HasPlusOrTeam    bool           `json:"has_plus_or_team"`
	HourlyQuota      map[string]any `json:"hourly_quota"`
	WeeklyQuota      map[string]any `json:"weekly_quota"`
	CodeReviewQuota  map[string]any `json:"code_review_quota"`
	OverviewFetchedAt string        `json:"overview_fetched_at,omitempty"`
	OverviewStale    bool           `json:"overview_stale"`
	OverviewError    any            `json:"overview_error"`
}

type AccountOverviewCardsResponse struct {
	Total            int                   `json:"total"`
	CurrentAccountID *int                  `json:"current_account_id"`
	CacheTTLSeconds  int                   `json:"cache_ttl_seconds"`
	NetworkMode      string                `json:"network_mode"`
	Proxy            string                `json:"proxy"`
	Accounts         []AccountOverviewCard `json:"accounts"`
	RefreshedAt      string                `json:"refreshed_at"`
}

type AccountOverviewSelectableItem struct {
	ID               int    `json:"id"`
	Email            string `json:"email"`
	Password         string `json:"password,omitempty"`
	Status           string `json:"status"`
	EmailService     string `json:"email_service"`
	SubscriptionType string `json:"subscription_type"`
	ClientID         string `json:"client_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	HasAccessToken   bool   `json:"has_access_token"`
	CreatedAt        string `json:"created_at,omitempty"`
}

type AccountOverviewSelectableResponse struct {
	Total    int                            `json:"total"`
	Accounts []AccountOverviewSelectableItem `json:"accounts"`
}

type AccountTokensResponse struct {
	ID                 int    `json:"id"`
	Email              string `json:"email"`
	AccessToken        string `json:"access_token"`
	RefreshToken       string `json:"refresh_token"`
	IDToken            string `json:"id_token"`
	SessionToken       string `json:"session_token"`
	SessionTokenSource string `json:"session_token_source"`
	DeviceID           string `json:"device_id"`
	HasTokens          bool   `json:"has_tokens"`
}

type AccountCookiesResponse struct {
	AccountID int    `json:"account_id"`
	Cookies   string `json:"cookies"`
}

func (r ListAccountsRequest) Normalized() ListAccountsRequest {
	normalized := r
	if normalized.Page <= 0 {
		normalized.Page = DefaultPage
	}
	if normalized.PageSize <= 0 {
		normalized.PageSize = DefaultPageSize
	}
	if normalized.PageSize > MaxPageSize {
		normalized.PageSize = MaxPageSize
	}
	normalized.Status = normalizeFilterText(normalized.Status)
	normalized.EmailService = normalizeFilterText(normalized.EmailService)
	normalized.RefreshTokenState = normalizeFilterText(normalized.RefreshTokenState)
	normalized.Search = strings.TrimSpace(normalized.Search)
	return normalized
}

func (r AccountOverviewCardsRequest) Normalized() AccountOverviewCardsRequest {
	normalized := r
	normalized.Search = strings.TrimSpace(normalized.Search)
	normalized.Status = normalizeFilterText(normalized.Status)
	normalized.EmailService = normalizeFilterText(normalized.EmailService)
	normalized.Proxy = strings.TrimSpace(normalized.Proxy)
	return normalized
}

func (r AccountOverviewSelectableRequest) Normalized() AccountOverviewSelectableRequest {
	normalized := r
	normalized.Search = strings.TrimSpace(normalized.Search)
	normalized.Status = normalizeFilterText(normalized.Status)
	normalized.EmailService = normalizeFilterText(normalized.EmailService)
	return normalized
}

func (r ListAccountsRequest) Offset() int {
	normalized := r.Normalized()
	return (normalized.Page - 1) * normalized.PageSize
}

func (r UpsertAccountRequest) Normalized(now time.Time) (UpsertAccountRequest, error) {
	normalized := r
	normalized.Email = strings.TrimSpace(normalized.Email)
	if normalized.Email == "" {
		return UpsertAccountRequest{}, ErrAccountEmailRequired
	}

	normalized.Password = strings.TrimSpace(normalized.Password)
	normalized.ClientID = strings.TrimSpace(normalized.ClientID)
	normalized.SessionToken = strings.TrimSpace(normalized.SessionToken)
	normalized.EmailService = strings.TrimSpace(normalized.EmailService)
	normalized.EmailServiceID = strings.TrimSpace(normalized.EmailServiceID)
	normalized.AccountID = strings.TrimSpace(normalized.AccountID)
	normalized.WorkspaceID = strings.TrimSpace(normalized.WorkspaceID)
	normalized.AccessToken = strings.TrimSpace(normalized.AccessToken)
	normalized.RefreshToken = strings.TrimSpace(normalized.RefreshToken)
	normalized.IDToken = strings.TrimSpace(normalized.IDToken)
	normalized.Cookies = strings.TrimSpace(normalized.Cookies)
	normalized.ProxyUsed = strings.TrimSpace(normalized.ProxyUsed)
	normalized.Status = strings.TrimSpace(normalized.Status)
	normalized.Source = strings.TrimSpace(normalized.Source)
	normalized.SubscriptionType = strings.TrimSpace(normalized.SubscriptionType)
	normalized.LastRefresh = cloneTimePtr(normalized.LastRefresh)
	normalized.ExpiresAt = cloneTimePtr(normalized.ExpiresAt)
	normalized.CPAUploadedAt = cloneTimePtr(normalized.CPAUploadedAt)
	normalized.Sub2APIUploadedAt = cloneTimePtr(normalized.Sub2APIUploadedAt)
	normalized.SubscriptionAt = cloneTimePtr(normalized.SubscriptionAt)

	if normalized.Status == "" {
		normalized.Status = DefaultAccountStatus
	}
	if normalized.Source == "" {
		normalized.Source = DefaultAccountSource
	}
	if normalized.RegisteredAt == nil {
		timestamp := now.UTC()
		normalized.RegisteredAt = &timestamp
	}
	normalized.ExtraData = cloneExtraData(normalized.ExtraData)

	return normalized, nil
}

func (r UpsertAccountRequest) ToAccount() Account {
	account := Account{
		Email:             r.Email,
		Password:          r.Password,
		ClientID:          r.ClientID,
		SessionToken:      r.SessionToken,
		EmailService:      r.EmailService,
		EmailServiceID:    r.EmailServiceID,
		AccountID:         r.AccountID,
		WorkspaceID:       r.WorkspaceID,
		AccessToken:       r.AccessToken,
		RefreshToken:      r.RefreshToken,
		IDToken:           r.IDToken,
		Cookies:           r.Cookies,
		ProxyUsed:         r.ProxyUsed,
		LastRefresh:       cloneTimePtr(r.LastRefresh),
		ExpiresAt:         cloneTimePtr(r.ExpiresAt),
		ExtraData:         cloneExtraData(r.ExtraData),
		CPAUploadedAt:     cloneTimePtr(r.CPAUploadedAt),
		Sub2APIUploadedAt: cloneTimePtr(r.Sub2APIUploadedAt),
		Status:            r.Status,
		Source:            r.Source,
		SubscriptionType:  r.SubscriptionType,
		SubscriptionAt:    cloneTimePtr(r.SubscriptionAt),
		RegisteredAt:      cloneTimePtr(r.RegisteredAt),
	}
	if r.CPAUploaded != nil {
		account.CPAUploaded = *r.CPAUploaded
	}
	if r.Sub2APIUploaded != nil {
		account.Sub2APIUploaded = *r.Sub2APIUploaded
	}
	return account
}

func UnknownQuotaSnapshot() map[string]any {
	return map[string]any{
		"used":          nil,
		"total":         nil,
		"remaining":     nil,
		"percentage":    nil,
		"reset_at":      nil,
		"reset_in_text": "-",
		"status":        "unknown",
	}
}

func NormalizeRefreshTokenState(value string) string {
	return normalizeFilterText(value)
}

func normalizeFilterText(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func cloneExtraData(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}
