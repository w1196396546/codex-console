package uploader

import (
	"encoding/json"
	"strings"
	"time"
)

var defaultSub2APIModelMapping = map[string]string{
	"gpt-5.1":            "gpt-5.1",
	"gpt-5.1-codex":      "gpt-5.1-codex",
	"gpt-5.1-codex-max":  "gpt-5.1-codex-max",
	"gpt-5.1-codex-mini": "gpt-5.1-codex-mini",
	"gpt-5.2":            "gpt-5.2",
	"gpt-5.2-codex":      "gpt-5.2-codex",
	"gpt-5.3":            "gpt-5.3",
	"gpt-5.3-codex":      "gpt-5.3-codex",
	"gpt-5.4":            "gpt-5.4",
}

var cpaTimeZone = time.FixedZone("UTC+8", 8*60*60)

type CPAAuthFile struct {
	Filename    string
	ContentType string
	Content     []byte
}

type Sub2APIBatchOptions struct {
	Concurrency int
	Priority    int
	ExportedAt  time.Time
}

type Sub2APIBatchPayload struct {
	Data                 Sub2APIDataPayload `json:"data"`
	SkipDefaultGroupBind bool               `json:"skip_default_group_bind"`
}

type Sub2APIDataPayload struct {
	Type       string                  `json:"type"`
	Version    int                     `json:"version"`
	ExportedAt string                  `json:"exported_at"`
	Proxies    []any                   `json:"proxies"`
	Accounts   []Sub2APIAccountPayload `json:"accounts"`
}

type Sub2APIAccountPayload struct {
	Name               string             `json:"name"`
	Platform           string             `json:"platform"`
	Type               string             `json:"type"`
	Credentials        Sub2APICredentials `json:"credentials"`
	Extra              map[string]any     `json:"extra"`
	Concurrency        int                `json:"concurrency"`
	Priority           int                `json:"priority"`
	RateMultiplier     int                `json:"rate_multiplier"`
	AutoPauseOnExpired bool               `json:"auto_pause_on_expired"`
}

type Sub2APICredentials struct {
	AccessToken      string            `json:"access_token"`
	ChatGPTAccountID string            `json:"chatgpt_account_id"`
	ChatGPTUserID    string            `json:"chatgpt_user_id"`
	ClientID         string            `json:"client_id"`
	ExpiresAt        int64             `json:"expires_at"`
	ExpiresIn        int               `json:"expires_in"`
	ModelMapping     map[string]string `json:"model_mapping"`
	OrganizationID   string            `json:"organization_id"`
	RefreshToken     string            `json:"refresh_token"`
}

type TMSinglePayload struct {
	ImportType   string `json:"import_type"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	SessionToken string `json:"session_token"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	AccountID    string `json:"account_id"`
}

type TMBatchPayload struct {
	ImportType string `json:"import_type"`
	Content    string `json:"content"`
}

func BuildCPAAuthFile(account UploadAccount) (CPAAuthFile, error) {
	normalized := account.Normalized()
	if normalized.Email == "" {
		return CPAAuthFile{}, ErrUploadAccountEmailMissing
	}

	content, err := json.MarshalIndent(map[string]any{
		"type":          "codex",
		"email":         normalized.Email,
		"expired":       formatCPATimestamp(normalized.ExpiresAt),
		"id_token":      normalized.IDToken,
		"account_id":    normalized.AccountID,
		"access_token":  normalized.AccessToken,
		"last_refresh":  formatCPATimestamp(normalized.LastRefresh),
		"refresh_token": normalized.RefreshToken,
	}, "", "  ")
	if err != nil {
		return CPAAuthFile{}, err
	}

	return CPAAuthFile{
		Filename:    normalized.Email + ".json",
		ContentType: "application/json",
		Content:     content,
	}, nil
}

func BuildSub2APIBatchPayload(service ServiceConfig, accounts []UploadAccount, options Sub2APIBatchOptions) (Sub2APIBatchPayload, error) {
	normalizedService := service.Normalized()
	if normalizedService.Kind != UploadKindSub2API {
		return Sub2APIBatchPayload{}, ErrUploadKindInvalid
	}

	normalizedAccounts := make([]UploadAccount, 0, len(accounts))
	for _, account := range accounts {
		normalized := account.Normalized()
		if normalized.AccessToken == "" {
			continue
		}
		normalizedAccounts = append(normalizedAccounts, normalized)
	}
	if len(normalizedAccounts) == 0 {
		return Sub2APIBatchPayload{}, ErrUploadAccessTokenMissing
	}

	concurrency := options.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultSub2APIConcurrency
	}
	priority := options.Priority
	if priority <= 0 {
		priority = DefaultSub2APIPriority
	}
	exportedAt := options.ExportedAt.UTC()
	if exportedAt.IsZero() {
		exportedAt = time.Now().UTC()
	}

	items := make([]Sub2APIAccountPayload, 0, len(normalizedAccounts))
	for _, account := range normalizedAccounts {
		expiresAt := int64(0)
		if account.ExpiresAt != nil {
			expiresAt = account.ExpiresAt.Unix()
		}
		items = append(items, Sub2APIAccountPayload{
			Name:     account.Email,
			Platform: "openai",
			Type:     "oauth",
			Credentials: Sub2APICredentials{
				AccessToken:      account.AccessToken,
				ChatGPTAccountID: account.AccountID,
				ChatGPTUserID:    "",
				ClientID:         account.ClientID,
				ExpiresAt:        expiresAt,
				ExpiresIn:        863999,
				ModelMapping:     cloneStringMap(defaultSub2APIModelMapping),
				OrganizationID:   account.WorkspaceID,
				RefreshToken:     account.RefreshToken,
			},
			Extra:              map[string]any{},
			Concurrency:        concurrency,
			Priority:           priority,
			RateMultiplier:     DefaultSub2APIRateMultiplier,
			AutoPauseOnExpired: true,
		})
	}

	payloadType := "sub2api-data"
	if strings.EqualFold(normalizedService.TargetType, "newapi") {
		payloadType = "newapi-data"
	}

	return Sub2APIBatchPayload{
		Data: Sub2APIDataPayload{
			Type:       payloadType,
			Version:    1,
			ExportedAt: exportedAt.Format(time.RFC3339),
			Proxies:    []any{},
			Accounts:   items,
		},
		SkipDefaultGroupBind: true,
	}, nil
}

func BuildTMSinglePayload(account UploadAccount) (TMSinglePayload, error) {
	normalized := account.Normalized()
	if normalized.Email == "" {
		return TMSinglePayload{}, ErrUploadAccountEmailMissing
	}
	if normalized.AccessToken == "" {
		return TMSinglePayload{}, ErrUploadAccessTokenMissing
	}

	return TMSinglePayload{
		ImportType:   "single",
		Email:        normalized.Email,
		AccessToken:  normalized.AccessToken,
		SessionToken: normalized.SessionToken,
		RefreshToken: normalized.RefreshToken,
		ClientID:     normalized.ClientID,
		AccountID:    normalized.AccountID,
	}, nil
}

func BuildTMBatchPayload(accounts []UploadAccount) (TMBatchPayload, error) {
	if len(accounts) == 0 {
		return TMBatchPayload{}, ErrUploadAccountsEmpty
	}

	lines := make([]string, 0, len(accounts))
	for _, account := range accounts {
		normalized := account.Normalized()
		if normalized.AccessToken == "" {
			continue
		}
		lines = append(lines, strings.Join([]string{
			normalized.Email,
			normalized.AccessToken,
			normalized.RefreshToken,
			normalized.SessionToken,
			normalized.ClientID,
		}, ","))
	}
	if len(lines) == 0 {
		return TMBatchPayload{}, ErrUploadAccessTokenMissing
	}

	return TMBatchPayload{
		ImportType: "batch",
		Content:    strings.Join(lines, "\n"),
	}, nil
}

func formatCPATimestamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.In(cpaTimeZone).Format("2006-01-02T15:04:05-07:00")
}

func cloneStringMap(input map[string]string) map[string]string {
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
