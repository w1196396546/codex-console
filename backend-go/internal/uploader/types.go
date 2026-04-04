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
	ErrUploadKindInvalid         = errors.New("uploader: invalid upload kind")
	ErrUploadAccountEmailMissing = errors.New("uploader: account email is required")
	ErrUploadAccessTokenMissing  = errors.New("uploader: access token is required")
	ErrUploadAccountsEmpty       = errors.New("uploader: at least one upload account is required")
	ErrUploadServiceBaseURLEmpty = errors.New("uploader: service base url is required")
	ErrUploadCredentialMissing   = errors.New("uploader: service credential is required")
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
