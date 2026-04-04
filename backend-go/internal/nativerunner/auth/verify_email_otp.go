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

func (c *Client) VerifyEmailOTP(ctx context.Context, prepared PrepareSignupResult, code string) (PrepareSignupResult, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return PrepareSignupResult{}, errors.New("email otp code is required")
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer":      c.emailVerificationReferer(prepared),
	}
	extraHeaders, err := c.flowRequestHeaders(ctx, RequestHeadersInput{
		Kind:          FlowRequestKindVerifyEmailOTP,
		Email:         "",
		PrepareSignup: prepared,
	})
	if err != nil {
		return PrepareSignupResult{}, err
	}
	for key, value := range extraHeaders {
		headers[key] = value
	}

	body, err := json.Marshal(map[string]string{"code": trimmedCode})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("encode email otp payload: %w", err)
	}

	response, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    "/api/accounts/email-otp/validate",
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("verify email otp: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return PrepareSignupResult{}, fmt.Errorf("verify email otp: unexpected status %d", response.StatusCode)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return PrepareSignupResult{}, fmt.Errorf("decode email otp response: %w", err)
		}
	}

	continueURL := extractContinueURL(c, payload)
	finalURL := continueURL
	if finalURL == "" {
		finalURL = prepared.FinalURL
	}

	finalPath := prepared.FinalPath
	if finalURL != "" {
		if parsed, err := urlPath(finalURL); err == nil {
			finalPath = parsed
		}
	}

	prepared.ContinueURL = continueURL
	prepared.FinalURL = finalURL
	prepared.FinalPath = finalPath
	prepared.PageType = extractPayloadPageType(payload, continueURL, finalURL)
	if prepared.PageType == "" {
		prepared.PageType = inferPageType(response)
	}

	return prepared, nil
}

func (c *Client) emailVerificationReferer(prepared PrepareSignupResult) string {
	if referer := strings.TrimSpace(prepared.FinalURL); referer != "" {
		return referer
	}
	if c == nil {
		return ""
	}

	resolved, err := c.resolveURL("/email-verification")
	if err != nil {
		return ""
	}
	return resolved.String()
}
