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

func (c *Client) RegisterPasswordAndSendOTP(ctx context.Context, prepared PrepareSignupResult, email string, password string) (PrepareSignupResult, error) {
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return PrepareSignupResult{}, errors.New("signup email is required")
	}
	if strings.TrimSpace(password) == "" {
		return PrepareSignupResult{}, errors.New("signup password is required")
	}

	referer := c.signupPasswordReferer(prepared)
	origin := c.flowOrigin(referer)
	registerURL := c.flowRequestURL(referer, "/api/accounts/user/register")

	registerHeaders := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       origin,
		"Referer":      referer,
	}
	if c != nil && strings.TrimSpace(c.deviceID) != "" {
		registerHeaders["oai-device-id"] = strings.TrimSpace(c.deviceID)
	}
	if sentinel := c.sentinelHeaderToken(ctx, "username_password_create", registerURL); sentinel != "" {
		registerHeaders["openai-sentinel-token"] = sentinel
	}
	extraRegisterHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindRegisterPassword,
		Email:         trimmedEmail,
		Password:      password,
		PrepareSignup: prepared,
	})
	if err != nil {
		return PrepareSignupResult{}, err
	}
	for key, value := range extraRegisterHeaders {
		registerHeaders[key] = value
	}

	registerBody, err := json.Marshal(map[string]string{
		"username": trimmedEmail,
		"password": password,
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("encode register payload: %w", err)
	}

	registerResponse, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    registerURL,
		Headers: registerHeaders,
		Body:    bytes.NewReader(registerBody),
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("register user: %w", err)
	}
	prepared.RegisterStatusCode = registerResponse.StatusCode

	var registerPayload map[string]any
	if len(registerResponse.Body) != 0 {
		_ = json.Unmarshal(registerResponse.Body, &registerPayload)
	}
	if _, _, isUserExists := detectUserExistsError(registerPayload, string(registerResponse.Body)); isUserExists {
		prepared.PageType = "user_exists"
		prepared.ContinueURL = extractContinueURL(c, registerPayload)
		if prepared.ContinueURL != "" {
			prepared.FinalURL = prepared.ContinueURL
			if finalPath, err := urlPath(prepared.ContinueURL); err == nil {
				prepared.FinalPath = finalPath
			}
		}
		return prepared, nil
	}
	if registerResponse.StatusCode >= http.StatusBadRequest {
		if detail := extractErrorMessage(registerPayload, string(registerResponse.Body)); detail != "" {
			return PrepareSignupResult{}, fmt.Errorf("register user: unexpected status %d: %s", registerResponse.StatusCode, detail)
		}
		return PrepareSignupResult{}, fmt.Errorf("register user: unexpected status %d", registerResponse.StatusCode)
	}

	otpHeaders := Headers{
		"Accept":  "application/json, text/plain, */*",
		"Referer": referer,
	}
	extraOTPHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindSendEmailOTP,
		Email:         trimmedEmail,
		Password:      password,
		PrepareSignup: prepared,
	})
	if err != nil {
		return PrepareSignupResult{}, err
	}
	for key, value := range extraOTPHeaders {
		otpHeaders[key] = value
	}

	otpResponse, err := c.Get(ctx, c.flowRequestURL(referer, "/api/accounts/email-otp/send"), otpHeaders)
	if err == nil {
		prepared.SendOTPStatusCode = otpResponse.StatusCode
	}

	emailVerificationURL, err := url.Parse(c.flowRequestURL(referer, "/email-verification"))
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("resolve email verification url: %w", err)
	}

	prepared.FinalURL = emailVerificationURL.String()
	prepared.FinalPath = emailVerificationURL.Path
	prepared.ContinueURL = ""
	prepared.PageType = "email_otp_verification"
	return prepared, nil
}

func (c *Client) signupPasswordReferer(prepared PrepareSignupResult) string {
	pageType := strings.TrimSpace(prepared.PageType)
	if referer := strings.TrimSpace(prepared.FinalURL); referer != "" {
		if pageType == "create_account_password" || pageType == "password" {
			return referer
		}
		if canonical := c.flowRequestURL(referer, "/create-account/password"); canonical != "" {
			return canonical
		}
	}
	if c == nil {
		return ""
	}

	url, err := c.resolveURL("/create-account/password")
	if err != nil {
		return ""
	}
	return url.String()
}
