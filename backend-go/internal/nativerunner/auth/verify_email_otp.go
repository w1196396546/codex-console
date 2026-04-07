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

type RetryableEmailOTPError struct {
	statusCode int
	errorCode  string
	message    string
	prepared   PrepareSignupResult
}

func (e *RetryableEmailOTPError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.message) != "" {
		return e.message
	}
	return fmt.Sprintf("verify email otp: unexpected status %d", e.statusCode)
}

func (e *RetryableEmailOTPError) Retryable() bool {
	return true
}

func (e *RetryableEmailOTPError) RequireNewCode() bool {
	return true
}

func (e *RetryableEmailOTPError) HTTPStatusCode() int {
	if e == nil {
		return 0
	}
	return e.statusCode
}

func (e *RetryableEmailOTPError) ErrorCode() string {
	if e == nil {
		return ""
	}
	return e.errorCode
}

func (e *RetryableEmailOTPError) PreparedResult() PrepareSignupResult {
	if e == nil {
		return PrepareSignupResult{}
	}
	return e.prepared
}

func (c *Client) VerifyEmailOTP(ctx context.Context, prepared PrepareSignupResult, code string) (PrepareSignupResult, error) {
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return PrepareSignupResult{}, errors.New("email otp code is required")
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Referer":      c.emailVerificationReferer(prepared),
	}
	headers["Origin"] = c.flowOrigin(headers["Referer"])
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
		Path:    c.flowRequestURL(headers["Referer"], "/api/accounts/email-otp/validate"),
		Headers: headers,
		Body:    bytes.NewReader(body),
	})
	if err != nil {
		return PrepareSignupResult{}, fmt.Errorf("verify email otp: %w", err)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return PrepareSignupResult{}, fmt.Errorf("decode email otp response: %w", err)
		}
	}
	if response.StatusCode >= http.StatusBadRequest {
		if otpErr := classifyRetryableEmailOTPError(response.StatusCode, payload, prepared); otpErr != nil {
			return PrepareSignupResult{}, otpErr
		}
		return PrepareSignupResult{}, fmt.Errorf("verify email otp: unexpected status %d", response.StatusCode)
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

func classifyRetryableEmailOTPError(statusCode int, payload map[string]any, prepared PrepareSignupResult) error {
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
	combined := strings.ToLower(strings.TrimSpace(code + " " + message))
	if statusCode == http.StatusBadRequest &&
		(strings.Contains(combined, "invalid_code") ||
			strings.Contains(combined, "incorrect") ||
			strings.Contains(combined, "expired") ||
			strings.Contains(combined, "stale")) {
		return &RetryableEmailOTPError{
			statusCode: statusCode,
			errorCode:  code,
			message:    firstNonEmpty(message, fmt.Sprintf("verify email otp: unexpected status %d", statusCode)),
			prepared:   prepared,
		}
	}
	return nil
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
