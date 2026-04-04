package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

	if _, err := c.Bootstrap(ctx); err != nil {
		return PrepareSignupResult{}, fmt.Errorf("bootstrap signup flow: %w", err)
	}

	csrfToken, err := c.fetchCSRFToken(ctx)
	if err != nil {
		return PrepareSignupResult{}, err
	}

	authorizeURL, err := c.signinOpenAI(ctx, trimmedEmail, csrfToken)
	if err != nil {
		return PrepareSignupResult{}, err
	}

	authorizeResponse, err := c.Get(ctx, authorizeURL, Headers{
		"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Referer": c.callbackURL(),
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("follow authorize url: %w", err)
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

	return result, nil
}

func (c *Client) fetchCSRFToken(ctx context.Context) (string, error) {
	response, err := c.Get(ctx, "/api/auth/csrf", Headers{
		"Accept": "application/json",
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
		"prompt":      []string{"login"},
		"screen_hint": []string{"login_or_signup"},
		"login_hint":  []string{email},
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
