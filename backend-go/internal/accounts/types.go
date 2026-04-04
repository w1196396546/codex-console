package accounts

import (
	"errors"
	"strings"
	"time"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 100

	DefaultAccountStatus = "active"
	DefaultAccountSource = "register"
)

var (
	ErrAccountEmailRequired        = errors.New("accounts: email is required")
	ErrAccountEmailServiceRequired = errors.New("accounts: email_service is required for new account")
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
	ExtraData         map[string]any `json:"extra_data,omitempty"`
	CPAUploaded       bool           `json:"cpa_uploaded,omitempty"`
	CPAUploadedAt     *time.Time     `json:"cpa_uploaded_at,omitempty"`
	Sub2APIUploaded   bool           `json:"sub2api_uploaded,omitempty"`
	Sub2APIUploadedAt *time.Time     `json:"sub2api_uploaded_at,omitempty"`
	Status            string         `json:"status"`
	Source            string         `json:"source,omitempty"`
	RegisteredAt      *time.Time     `json:"registered_at,omitempty"`
	CreatedAt         *time.Time     `json:"created_at,omitempty"`
	UpdatedAt         *time.Time     `json:"updated_at,omitempty"`
}

type ListAccountsRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
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
	ExtraData         map[string]any `json:"extra_data,omitempty"`
	CPAUploaded       *bool          `json:"cpa_uploaded,omitempty"`
	CPAUploadedAt     *time.Time     `json:"cpa_uploaded_at,omitempty"`
	Sub2APIUploaded   *bool          `json:"sub2api_uploaded,omitempty"`
	Sub2APIUploadedAt *time.Time     `json:"sub2api_uploaded_at,omitempty"`
	Status            string         `json:"status,omitempty"`
	Source            string         `json:"source,omitempty"`
	RegisteredAt      *time.Time     `json:"registered_at,omitempty"`
}

type AccountListResponse struct {
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int       `json:"total"`
	Accounts []Account `json:"accounts"`
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
	normalized.CPAUploadedAt = cloneTimePtr(normalized.CPAUploadedAt)
	normalized.Sub2APIUploadedAt = cloneTimePtr(normalized.Sub2APIUploadedAt)

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
		ExtraData:         cloneExtraData(r.ExtraData),
		CPAUploadedAt:     cloneTimePtr(r.CPAUploadedAt),
		Sub2APIUploadedAt: cloneTimePtr(r.Sub2APIUploadedAt),
		Status:            r.Status,
		Source:            r.Source,
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
