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

func (c *Client) RegisterPasswordAndSendOTP(ctx context.Context, prepared PrepareSignupResult, email string, password string) (PrepareSignupResult, error) {
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return PrepareSignupResult{}, errors.New("signup email is required")
	}
	if strings.TrimSpace(password) == "" {
		return PrepareSignupResult{}, errors.New("signup password is required")
	}

	referer := c.signupPasswordReferer(prepared)

	registerHeaders := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer":      referer,
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
		Path:    "/api/accounts/user/register",
		Headers: registerHeaders,
		Body:    bytes.NewReader(registerBody),
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("register user: %w", err)
	}
	if registerResponse.StatusCode >= http.StatusBadRequest {
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

	otpResponse, err := c.Get(ctx, "/api/accounts/email-otp/send", otpHeaders)
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("send email otp: %w", err)
	}
	if otpResponse.StatusCode >= http.StatusBadRequest {
		return PrepareSignupResult{}, fmt.Errorf("send email otp: unexpected status %d", otpResponse.StatusCode)
	}

	emailVerificationURL, err := c.resolveURL("/email-verification")
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("resolve email verification url: %w", err)
	}

	prepared.FinalURL = emailVerificationURL.String()
	prepared.FinalPath = emailVerificationURL.Path
	prepared.ContinueURL = ""
	prepared.PageType = "email_otp_verification"
	prepared.RegisterStatusCode = registerResponse.StatusCode
	prepared.SendOTPStatusCode = otpResponse.StatusCode
	return prepared, nil
}

func (c *Client) signupPasswordReferer(prepared PrepareSignupResult) string {
	if referer := strings.TrimSpace(prepared.FinalURL); referer != "" {
		return referer
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
