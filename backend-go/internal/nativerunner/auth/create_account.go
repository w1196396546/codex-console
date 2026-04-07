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
	StatusCode         int
	FinalURL           string
	FinalPath          string
	PageType           string
	ContinueURL        string
	CallbackURL        string
	AccountID          string
	WorkspaceID        string
	RefreshToken       string
	RefreshTokenSource string
	RawData            map[string]any
}

type CreateAccountUserExistsError struct {
	StatusCode int
	Code       string
	Message    string
	Result     CreateAccountResult
}

func (e *CreateAccountUserExistsError) Error() string {
	if e == nil {
		return "create account user exists"
	}

	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "create account user exists"
	}
	if code := strings.TrimSpace(e.Code); code != "" {
		return fmt.Sprintf("%s (%s)", message, code)
	}
	return message
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
		"Referer":      c.aboutYouReferer(prepared),
	}
	headers["Origin"] = c.flowOrigin(headers["Referer"])
	if c != nil && strings.TrimSpace(c.deviceID) != "" {
		headers["oai-device-id"] = strings.TrimSpace(c.deviceID)
	}
	if sentinel := c.sentinelHeaderToken(ctx, "authorize_continue", c.flowRequestURL(headers["Referer"], "/api/accounts/create_account")); sentinel != "" {
		headers["openai-sentinel-token"] = sentinel
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
		Path:    c.flowRequestURL(headers["Referer"], "/api/accounts/create_account"),
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("create account: %w", err)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return CreateAccountResult{}, fmt.Errorf("decode create account response: %w", err)
		}
	}
	result := createAccountResultFromPayload(c, response.StatusCode, payload)
	if code, message, isUserExists := detectUserExistsError(payload, string(response.Body)); isUserExists {
		return result, &CreateAccountUserExistsError{
			StatusCode: response.StatusCode,
			Code:       code,
			Message:    message,
			Result:     result,
		}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return result, fmt.Errorf("create account: unexpected status %d", response.StatusCode)
	}
	return result, nil
}

func createAccountResultFromPayload(c *Client, statusCode int, payload map[string]any) CreateAccountResult {
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
		workspaceID = firstObjectID(payload["workspaces"])
	}

	finalURL := continueURL
	finalPath := ""
	if finalURL != "" {
		finalPath, _ = urlPath(finalURL)
	}

	return CreateAccountResult{
		StatusCode:   statusCode,
		FinalURL:     finalURL,
		FinalPath:    finalPath,
		PageType:     extractPayloadPageType(payload, continueURL, finalURL),
		ContinueURL:  continueURL,
		CallbackURL:  callbackURL,
		AccountID:    accountID,
		WorkspaceID:  workspaceID,
		RefreshToken: extractString(payload["refresh_token"]),
		RawData:      payload,
	}
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

func detectUserExistsError(payload map[string]any, rawBody string) (string, string, bool) {
	errorPayload := extractObject(payload["error"])
	code := strings.TrimSpace(firstNonEmpty(
		extractString(errorPayload["code"]),
		extractString(payload["error_code"]),
		extractString(payload["code"]),
	))
	message := strings.TrimSpace(firstNonEmpty(
		extractString(errorPayload["message"]),
		extractString(payload["message"]),
		extractString(payload["detail"]),
	))

	normalizedCode := strings.ToLower(code)
	normalizedMessage := strings.ToLower(message)
	normalizedBody := strings.ToLower(strings.TrimSpace(rawBody))
	if normalizedCode == "user_exists" {
		return code, message, true
	}
	if strings.Contains(normalizedBody, "user_exists") {
		if code == "" {
			code = "user_exists"
		}
		return code, message, true
	}
	for _, marker := range []string{
		"already exists",
		"already registered",
		"email already exists",
		"user exists",
	} {
		if strings.Contains(normalizedMessage, marker) || strings.Contains(normalizedBody, marker) {
			return code, message, true
		}
	}
	return code, message, false
}

func extractErrorMessage(payload map[string]any, rawBody string) string {
	errorPayload := extractObject(payload["error"])
	message := strings.TrimSpace(firstNonEmpty(
		extractString(errorPayload["message"]),
		extractString(payload["message"]),
		extractString(payload["detail"]),
	))
	if message != "" {
		return message
	}

	rawBody = strings.TrimSpace(rawBody)
	if rawBody == "" {
		return ""
	}
	if len(rawBody) > 200 {
		return rawBody[:200]
	}
	return rawBody
}
