package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type CreateAccountResult struct {
	StatusCode   int
	FinalURL     string
	FinalPath    string
	PageType     string
	ContinueURL  string
	CallbackURL  string
	AccountID    string
	WorkspaceID  string
	RefreshToken string
	RawData      map[string]any
}

func (c *Client) CreateAccount(ctx context.Context, prepared PrepareSignupResult, firstName string, lastName string, birthdate string) (CreateAccountResult, error) {
	name := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(firstName),
		strings.TrimSpace(lastName),
	}, " "))
	if name == "" {
		return CreateAccountResult{}, errors.New("account name is required")
	}

	trimmedBirthdate := strings.TrimSpace(birthdate)
	if trimmedBirthdate == "" {
		return CreateAccountResult{}, errors.New("birthdate is required")
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer":      c.aboutYouReferer(prepared),
	}
	extraHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindCreateAccount,
		PrepareSignup: prepared,
	})
	if err != nil {
		return CreateAccountResult{}, err
	}
	for key, value := range extraHeaders {
		headers[key] = value
	}

	body, err := json.Marshal(map[string]string{
		"name":      name,
		"birthdate": trimmedBirthdate,
	})
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("encode create account payload: %w", err)
	}

	response, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    "/api/accounts/create_account",
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("create account: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return CreateAccountResult{}, fmt.Errorf("create account: unexpected status %d", response.StatusCode)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return CreateAccountResult{}, fmt.Errorf("decode create account response: %w", err)
		}
	}

	continueURL := extractContinueURL(c, payload)
	callbackURL := c.normalizeFlowURL(extractString(payload["callback_url"]))
	if callbackURL == "" && strings.Contains(continueURL, "/api/auth/callback/openai") && strings.Contains(continueURL, "code=") {
		callbackURL = continueURL
	}
	if continueURL == "" && callbackURL != "" {
		continueURL = callbackURL
	}

	account := extractObject(payload["account"])
	workspace := extractObject(payload["workspace"])

	accountID := extractString(payload["account_id"])
	if accountID == "" {
		accountID = extractString(payload["chatgpt_account_id"])
	}
	if accountID == "" {
		accountID = extractString(account["id"])
	}

	workspaceID := extractString(payload["workspace_id"])
	if workspaceID == "" {
		workspaceID = extractString(payload["default_workspace_id"])
	}
	if workspaceID == "" {
		workspaceID = extractString(workspace["id"])
	}
	if workspaceID == "" {
		if workspaces, ok := payload["workspaces"].([]any); ok && len(workspaces) != 0 {
			workspaceID = extractString(extractObject(workspaces[0])["id"])
		}
	}

	finalURL := continueURL
	finalPath := ""
	if finalURL != "" {
		finalPath, _ = urlPath(finalURL)
	}

	return CreateAccountResult{
		StatusCode:   response.StatusCode,
		FinalURL:     finalURL,
		FinalPath:    finalPath,
		PageType:     extractPayloadPageType(payload, continueURL, finalURL),
		ContinueURL:  continueURL,
		CallbackURL:  callbackURL,
		AccountID:    accountID,
		WorkspaceID:  workspaceID,
		RefreshToken: extractString(payload["refresh_token"]),
		RawData:      payload,
	}, nil
}

func (c *Client) aboutYouReferer(prepared PrepareSignupResult) string {
	if referer := strings.TrimSpace(prepared.FinalURL); referer != "" {
		return referer
	}
	if c == nil {
		return ""
	}

	resolved, err := c.resolveURL("/about-you")
	if err != nil {
		return ""
	}
	return resolved.String()
}
