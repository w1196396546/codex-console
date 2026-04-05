package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultOAuthClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultOAuthRedirectURI = "http://localhost:1455/auth/callback"
)

type PasswordLoginStepError struct {
	PageType  string
	FinalPath string
	Message   string
}

func (e *PasswordLoginStepError) Error() string {
	if e == nil {
		return "password login requires interactive step"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("password login requires interactive step (page_type=%s final_path=%s)", strings.TrimSpace(e.PageType), strings.TrimSpace(e.FinalPath))
}

type oauthFlowState struct {
	StatusCode  int
	CurrentURL  string
	ContinueURL string
	CallbackURL string
	FinalURL    string
	FinalPath   string
	PageType    string
	AccountID   string
	WorkspaceID string
	RawData     map[string]any
}

func (c *Client) CompletePasswordLogin(ctx context.Context, email string, password string) (ContinueCreateAccountResult, error) {
	if c == nil {
		return ContinueCreateAccountResult{}, errors.New("auth client is required")
	}

	prepared, err := c.PrepareSignup(ctx, email)
	if err != nil {
		return ContinueCreateAccountResult{}, fmt.Errorf("prepare password login: %w", err)
	}

	state, err := c.authorizeContinue(ctx, email, prepared)
	if err != nil {
		return ContinueCreateAccountResult{}, err
	}
	if strings.TrimSpace(state.PageType) == "login_password" {
		state, err = c.passwordVerify(ctx, password, state, prepared)
		if err != nil {
			return ContinueCreateAccountResult{}, err
		}
	}

	switch strings.TrimSpace(state.PageType) {
	case "callback", "continue", "workspace_selection":
		result, err := c.ContinueCreateAccount(ctx, state.createAccountResult())
		if err != nil {
			return ContinueCreateAccountResult{}, err
		}
		if strings.TrimSpace(result.RefreshToken) == "" {
			if refreshToken, err := c.exchangePasswordLoginToken(ctx, prepared, firstNonEmpty(result.CallbackURL, state.CallbackURL, state.ContinueURL, state.CurrentURL)); err == nil && strings.TrimSpace(refreshToken) != "" {
				result.RefreshToken = strings.TrimSpace(refreshToken)
			}
		}
		return result, nil
	case "login_password", "email_otp_verification", "about_you", "add_phone":
		return ContinueCreateAccountResult{}, &PasswordLoginStepError{
			PageType:  state.PageType,
			FinalPath: state.FinalPath,
			Message: fmt.Sprintf(
				"password login requires interactive step (page_type=%s final_path=%s)",
				strings.TrimSpace(state.PageType),
				strings.TrimSpace(state.FinalPath),
			),
		}
	default:
		if strings.TrimSpace(state.CallbackURL) != "" || strings.TrimSpace(state.ContinueURL) != "" {
			result, err := c.ContinueCreateAccount(ctx, state.createAccountResult())
			if err != nil {
				return ContinueCreateAccountResult{}, err
			}
			if strings.TrimSpace(result.RefreshToken) == "" {
				if refreshToken, err := c.exchangePasswordLoginToken(ctx, prepared, firstNonEmpty(result.CallbackURL, state.CallbackURL, state.ContinueURL, state.CurrentURL)); err == nil && strings.TrimSpace(refreshToken) != "" {
					result.RefreshToken = strings.TrimSpace(refreshToken)
				}
			}
			return result, nil
		}
		return ContinueCreateAccountResult{}, &PasswordLoginStepError{
			PageType:  state.PageType,
			FinalPath: state.FinalPath,
			Message: fmt.Sprintf(
				"password login produced unsupported state (page_type=%s final_path=%s)",
				strings.TrimSpace(state.PageType),
				strings.TrimSpace(state.FinalPath),
			),
		}
	}
}

func (c *Client) exchangePasswordLoginToken(ctx context.Context, prepared PrepareSignupResult, callbackURL string) (string, error) {
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return "", errors.New("callback url is required")
	}

	code := extractURLQueryValue(callbackURL, "code")
	if code == "" {
		return "", errors.New("authorization code is required")
	}

	clientID := firstNonEmpty(
		extractURLQueryValue(prepared.AuthorizeURL, "client_id"),
		defaultOAuthClientID,
	)
	redirectURI := firstNonEmpty(
		extractURLQueryValue(prepared.AuthorizeURL, "redirect_uri"),
		defaultOAuthRedirectURI,
	)

	codeVerifier := c.passwordLoginCodeVerifier()
	if codeVerifier == "" {
		return "", errors.New("pkce code verifier is required")
	}

	tokenURL := c.passwordLoginOAuthTokenURL(prepared.AuthorizeURL)
	if tokenURL == "" {
		return "", errors.New("oauth token url is required")
	}

	form := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{redirectURI},
		"client_id":     []string{clientID},
		"code_verifier": []string{codeVerifier},
	}
	tokenOrigin := urlOrigin(tokenURL)
	response, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   tokenURL,
		Headers: Headers{
			"Accept":       "application/json",
			"Content-Type": "application/x-www-form-urlencoded",
			"Origin":       tokenOrigin,
			"Referer":      firstNonEmpty(callbackURL, tokenOrigin+"/sign-in-with-chatgpt/codex/consent"),
		},
		Body: strings.NewReader(form.Encode()),
	})
	if err != nil {
		return "", fmt.Errorf("exchange oauth token: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("exchange oauth token: unexpected status %d", response.StatusCode)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return "", fmt.Errorf("decode oauth token response: %w", err)
		}
	}

	refreshToken := firstNonEmpty(
		extractString(payload["refresh_token"]),
		extractString(payload["refreshToken"]),
	)
	if refreshToken == "" {
		return "", errors.New("oauth token response missing refresh token")
	}
	return refreshToken, nil
}

func (c *Client) passwordLoginCodeVerifier() string {
	if c == nil {
		return ""
	}

	for _, name := range []string{
		"__Secure-next-auth.pkce.code_verifier",
		"_Secure-next-auth.pkce.code_verifier",
		"next-auth.pkce.code_verifier",
		"__Secure-authjs.pkce.code_verifier",
		"_Secure-authjs.pkce.code_verifier",
		"authjs.pkce.code_verifier",
	} {
		if value := strings.TrimSpace(c.cookieValue(name)); value != "" {
			return value
		}
		if value := strings.TrimSpace(c.cookieChunkValue(name)); value != "" {
			return value
		}
	}

	return ""
}

func (c *Client) passwordLoginOAuthTokenURL(authorizeURL string) string {
	origin := firstNonEmpty(urlOrigin(authorizeURL), c.origin())
	if origin == "" {
		return ""
	}
	return origin + "/oauth/token"
}

func (c *Client) authorizeContinue(ctx context.Context, email string, prepared PrepareSignupResult) (oauthFlowState, error) {
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return oauthFlowState{}, errors.New("login email is required")
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer":      firstNonEmpty(strings.TrimSpace(prepared.FinalURL), strings.TrimSpace(prepared.AuthorizeURL), c.callbackURL()),
	}
	extraHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindAuthorizeContinue,
		Email:         trimmedEmail,
		PrepareSignup: prepared,
	})
	if err != nil {
		return oauthFlowState{}, err
	}
	for key, value := range extraHeaders {
		headers[key] = value
	}

	body, err := json.Marshal(map[string]any{
		"username": map[string]string{
			"kind":  "email",
			"value": trimmedEmail,
		},
		"screen_hint": "login",
	})
	if err != nil {
		return oauthFlowState{}, fmt.Errorf("encode authorize continue payload: %w", err)
	}

	response, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    "/api/accounts/authorize/continue",
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return oauthFlowState{}, fmt.Errorf("authorize continue: %w", err)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return oauthFlowState{}, fmt.Errorf("decode authorize continue response: %w", err)
		}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return oauthFlowState{}, fmt.Errorf("authorize continue: unexpected status %d", response.StatusCode)
	}
	return oauthFlowStateFromPayload(c, response.StatusCode, payload, response.FinalURL), nil
}

func (c *Client) passwordVerify(ctx context.Context, password string, state oauthFlowState, prepared PrepareSignupResult) (oauthFlowState, error) {
	trimmedPassword := strings.TrimSpace(password)
	if trimmedPassword == "" {
		return oauthFlowState{}, errors.New("login password is required")
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer": firstNonEmpty(
			strings.TrimSpace(state.FinalURL),
			strings.TrimSpace(state.ContinueURL),
			strings.TrimSpace(state.CallbackURL),
			strings.TrimSpace(state.CurrentURL),
			strings.TrimSpace(prepared.FinalURL),
			c.callbackURL(),
		),
	}
	extraHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindPasswordVerify,
		Password:      trimmedPassword,
		PrepareSignup: prepared,
	})
	if err != nil {
		return oauthFlowState{}, err
	}
	for key, value := range extraHeaders {
		headers[key] = value
	}

	body, err := json.Marshal(map[string]string{
		"password": trimmedPassword,
	})
	if err != nil {
		return oauthFlowState{}, fmt.Errorf("encode password verify payload: %w", err)
	}

	response, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    "/api/accounts/password/verify",
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return oauthFlowState{}, fmt.Errorf("password verify: %w", err)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return oauthFlowState{}, fmt.Errorf("decode password verify response: %w", err)
		}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return oauthFlowState{}, fmt.Errorf("password verify: unexpected status %d", response.StatusCode)
	}
	return oauthFlowStateFromPayload(c, response.StatusCode, payload, response.FinalURL), nil
}

func oauthFlowStateFromPayload(c *Client, statusCode int, payload map[string]any, currentURL string) oauthFlowState {
	data := extractObject(payload["data"])
	continueURL := extractContinueURL(c, payload)
	if continueURL == "" {
		continueURL = extractContinueURL(c, data)
	}
	callbackURL := extractCallbackURL(c, payload)
	if callbackURL == "" {
		callbackURL = extractCallbackURL(c, data)
	}
	if callbackURL == "" {
		callbackURL = callbackURLFromValue(firstNonEmpty(continueURL, currentURL))
	}
	finalURL := firstNonEmpty(continueURL, callbackURL, strings.TrimSpace(currentURL))
	finalPath := ""
	if finalURL != "" {
		finalPath, _ = urlPath(finalURL)
	}

	accountID := firstNonEmpty(
		extractString(payload["account_id"]),
		extractString(payload["chatgpt_account_id"]),
		extractString(data["account_id"]),
		extractString(extractObject(payload["account"])["id"]),
	)
	workspaceID := firstNonEmpty(
		extractString(payload["workspace_id"]),
		extractString(payload["default_workspace_id"]),
		extractString(data["workspace_id"]),
		extractString(extractObject(payload["workspace"])["id"]),
		firstObjectID(payload["workspaces"]),
		firstObjectID(data["workspaces"]),
	)

	pageType := extractPayloadPageType(payload, continueURL, currentURL, callbackURL)
	if pageType == "" {
		pageType = extractPayloadPageType(data, continueURL, currentURL, callbackURL)
	}

	return oauthFlowState{
		StatusCode:  statusCode,
		CurrentURL:  strings.TrimSpace(currentURL),
		ContinueURL: continueURL,
		CallbackURL: callbackURL,
		FinalURL:    finalURL,
		FinalPath:   finalPath,
		PageType:    pageType,
		AccountID:   accountID,
		WorkspaceID: workspaceID,
		RawData:     payload,
	}
}

func (s oauthFlowState) createAccountResult() CreateAccountResult {
	finalURL := firstNonEmpty(strings.TrimSpace(s.FinalURL), strings.TrimSpace(s.CurrentURL), strings.TrimSpace(s.ContinueURL), strings.TrimSpace(s.CallbackURL))
	finalPath := strings.TrimSpace(s.FinalPath)
	if finalPath == "" && finalURL != "" {
		finalPath, _ = urlPath(finalURL)
	}
	return CreateAccountResult{
		StatusCode:  s.StatusCode,
		FinalURL:    finalURL,
		FinalPath:   finalPath,
		PageType:    strings.TrimSpace(s.PageType),
		ContinueURL: strings.TrimSpace(s.ContinueURL),
		CallbackURL: strings.TrimSpace(s.CallbackURL),
		AccountID:   strings.TrimSpace(s.AccountID),
		WorkspaceID: strings.TrimSpace(s.WorkspaceID),
		RawData:     s.RawData,
	}
}
