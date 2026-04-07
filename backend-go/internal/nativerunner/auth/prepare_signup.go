package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type PrepareSignupResult struct {
	CSRFToken          string
	AuthorizeURL       string
	FinalURL           string
	FinalPath          string
	ContinueURL        string
	PageType           string
	RegisterStatusCode int
	SendOTPStatusCode  int
}

func (c *Client) PrepareSignup(ctx context.Context, email string) (PrepareSignupResult, error) {
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return PrepareSignupResult{}, errors.New("signup email is required")
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := c.resetSession(); err != nil {
				return PrepareSignupResult{}, err
			}
		}

		result, blocked, err := c.prepareSignupOnce(ctx, trimmedEmail)
		if err == nil && !blocked {
			return result, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf(
				"prepare signup blocked by auth interstitial: final_url=%s final_path=%s",
				strings.TrimSpace(result.FinalURL),
				strings.TrimSpace(result.FinalPath),
			)
		}
	}

	if lastErr != nil {
		return PrepareSignupResult{}, lastErr
	}
	return PrepareSignupResult{}, errors.New("prepare signup failed")
}

func (c *Client) prepareSignupOnce(ctx context.Context, email string) (PrepareSignupResult, bool, error) {
	if _, err := c.BootstrapWith(ctx, BootstrapOptions{
		Path: "/",
		Headers: Headers{
			"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		},
	}); err != nil {
		return PrepareSignupResult{}, false, fmt.Errorf("bootstrap signup flow: %w", err)
	}

	csrfToken, err := c.fetchCSRFToken(ctx)
	if err != nil {
		return PrepareSignupResult{}, false, err
	}

	authorizeURL, err := c.signinOpenAI(ctx, email, csrfToken)
	if err != nil {
		return PrepareSignupResult{}, false, err
	}

	authorizeResponse, err := c.Get(ctx, authorizeURL, Headers{
		"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Referer": c.callbackURL(),
	})
	if err != nil {
		return PrepareSignupResult{}, false, fmt.Errorf("follow authorize url: %w", err)
	}

	result := PrepareSignupResult{
		CSRFToken:    csrfToken,
		AuthorizeURL: authorizeURL,
		FinalURL:     authorizeResponse.FinalURL,
		FinalPath:    authorizeResponse.FinalPath,
		PageType:     inferPageType(authorizeResponse),
	}
	if result.PageType == "continue" {
		result.ContinueURL = authorizeResponse.FinalURL
	}

	if prepareSignupBlockedByInterstitial(result, string(authorizeResponse.Body)) {
		return result, true, nil
	}

	normalized, err := c.normalizePreparedSignupResult(ctx, result, string(authorizeResponse.Body))
	if err != nil {
		return PrepareSignupResult{}, false, err
	}
	return normalized, false, nil
}

func prepareSignupBlockedByInterstitial(result PrepareSignupResult, body string) bool {
	for _, candidate := range []string{result.FinalURL, result.FinalPath} {
		normalized := strings.ToLower(strings.TrimSpace(candidate))
		if normalized == "" {
			continue
		}
		if strings.Contains(normalized, "/api/accounts/authorize") || strings.HasSuffix(normalized, "/error") {
			return true
		}
	}
	body = strings.ToLower(strings.TrimSpace(body))
	if body == "" {
		return false
	}
	return strings.Contains(body, "just a moment") ||
		strings.Contains(body, "checking your browser before accessing") ||
		strings.Contains(body, "__cf_chl_tk") ||
		strings.Contains(body, "cloudflare")
}

func (c *Client) normalizePreparedSignupResult(ctx context.Context, prepared PrepareSignupResult, currentBody string) (PrepareSignupResult, error) {
	pageType := strings.TrimSpace(prepared.PageType)
	switch pageType {
	case "create_account_password", "password", "email_otp_verification", "login_password", "about_you", "add_phone":
		return prepared, nil
	}
	if pageType != "continue" {
		return prepared, nil
	}

	followURL := c.extractContinueNavigationTarget(currentBody, prepared.FinalURL)
	if strings.TrimSpace(followURL) == "" {
		followURL = c.flowRequestURL(prepared.FinalURL, "/create-account/password")
	}
	if strings.TrimSpace(followURL) == "" {
		return prepared, nil
	}

	response, err := c.Get(ctx, followURL, Headers{
		"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Referer": firstNonEmpty(strings.TrimSpace(prepared.FinalURL), c.callbackURL()),
	})
	if err != nil {
		return prepared, nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return prepared, nil
	}

	prepared.FinalURL = response.FinalURL
	prepared.FinalPath = response.FinalPath
	prepared.PageType = inferPageType(response)
	if strings.TrimSpace(prepared.PageType) == "continue" {
		prepared.ContinueURL = response.FinalURL
	} else {
		prepared.ContinueURL = ""
	}
	return prepared, nil
}

func (c *Client) fetchCSRFToken(ctx context.Context) (string, error) {
	response, err := c.Get(ctx, "/api/auth/csrf", Headers{
		"Accept":  "application/json",
		"Referer": c.callbackURL(),
		"Origin":  c.origin(),
	})
	if err != nil {
		return "", fmt.Errorf("request csrf token: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("request csrf token: unexpected status %d", response.StatusCode)
	}

	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return "", fmt.Errorf("decode csrf token: %w", err)
	}

	token := strings.TrimSpace(payload.CSRFToken)
	if token == "" {
		return "", errors.New("csrf token missing")
	}
	return token, nil
}

func (c *Client) signinOpenAI(ctx context.Context, email string, csrfToken string) (string, error) {
	query := url.Values{
		"prompt":                  []string{"login"},
		"screen_hint":             []string{"login_or_signup"},
		"login_hint":              []string{email},
		"auth_session_logging_id": []string{uuid.NewString()},
	}
	if c != nil && strings.TrimSpace(c.deviceID) != "" {
		query.Set("ext-oai-did", strings.TrimSpace(c.deviceID))
	}
	form := url.Values{
		"callbackUrl": []string{c.callbackURL()},
		"csrfToken":   []string{csrfToken},
		"json":        []string{"true"},
	}

	response, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   "/api/auth/signin/openai?" + query.Encode(),
		Headers: Headers{
			"Accept":       "application/json",
			"Content-Type": "application/x-www-form-urlencoded",
			"Origin":       c.origin(),
			"Referer":      c.callbackURL(),
		},
		Body: strings.NewReader(form.Encode()),
	})
	if err != nil {
		return "", fmt.Errorf("signin openai: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("signin openai: unexpected status %d", response.StatusCode)
	}

	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return "", fmt.Errorf("decode authorize url: %w", err)
	}

	authorizeURL := strings.TrimSpace(payload.URL)
	if authorizeURL == "" {
		return "", errors.New("authorize url missing")
	}

	resolvedAuthorizeURL, err := c.resolveURL(authorizeURL)
	if err != nil {
		return "", fmt.Errorf("resolve authorize url: %w", err)
	}
	return resolvedAuthorizeURL.String(), nil
}

func (c *Client) callbackURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.ResolveReference(&url.URL{Path: "/"}).String()
}

func (c *Client) loginURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.ResolveReference(&url.URL{Path: "/auth/login"}).String()
}

func (c *Client) origin() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return c.baseURL.Scheme + "://" + c.baseURL.Host
}

func inferPageType(response Response) string {
	if pageType := inferPageTypeFromURL(response.FinalURL); pageType != "" {
		return pageType
	}
	if pageType := inferPageTypeFromURL(response.FinalPath); pageType != "" {
		return pageType
	}

	if strings.Contains(response.FinalPath, "/create-account/password") {
		return "create_account_password"
	}
	if strings.Contains(response.FinalPath, "/email-verification") || strings.Contains(response.FinalPath, "/email-otp") {
		return "email_otp_verification"
	}
	if strings.Contains(response.FinalPath, "/u/continue") {
		return "continue"
	}

	body := strings.ToLower(string(response.Body))
	switch {
	case strings.Contains(body, `data-page="continue"`):
		return "continue"
	case strings.Contains(body, `data-page="captcha"`):
		return "captcha"
	case strings.Contains(body, `data-page="about-you"`):
		return "about_you"
	case strings.Contains(body, "consent"):
		return "consent"
	default:
		return ""
	}
}
