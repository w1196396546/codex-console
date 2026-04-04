package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

func inferPageTypeFromURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	path := raw
	if err == nil {
		path = parsed.Path
	}

	path = strings.ToLower(path)
	switch {
	case strings.Contains(path, "/create-account/password"):
		return "create_account_password"
	case strings.Contains(path, "/email-verification"), strings.Contains(path, "/email-otp"):
		return "email_otp_verification"
	case strings.Contains(path, "/about-you"):
		return "about_you"
	case strings.Contains(path, "/add-phone"):
		return "add_phone"
	case strings.Contains(path, "/u/continue"):
		return "continue"
	case strings.Contains(path, "/api/auth/callback/openai"):
		return "callback"
	case strings.Contains(path, "/workspace"), strings.Contains(path, "/organization"), strings.Contains(path, "/consent"):
		return "workspace_selection"
	default:
		return ""
	}
}

func extractString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func extractObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func (c *Client) normalizeFlowURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	resolved, err := c.resolveURL(raw)
	if err != nil {
		return raw
	}
	return resolved.String()
}

func urlPath(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.Path, nil
}

func extractContinueURL(c *Client, payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	continueURL := c.normalizeFlowURL(extractString(payload["continue_url"]))
	if continueURL != "" {
		return continueURL
	}

	page := extractObject(payload["page"])
	for _, key := range []string{"url", "href", "external_url"} {
		if pageURL := c.normalizeFlowURL(extractString(page[key])); pageURL != "" {
			return pageURL
		}
	}

	return ""
}

func extractPayloadPageType(payload map[string]any, urls ...string) string {
	if len(payload) != 0 {
		for _, key := range []string{"page_type", "pageType", "type"} {
			if pageType := extractString(payload[key]); pageType != "" {
				return pageType
			}
		}

		page := extractObject(payload["page"])
		for _, key := range []string{"page_type", "pageType", "type", "name"} {
			if pageType := extractString(page[key]); pageType != "" {
				return pageType
			}
		}
	}

	for _, raw := range urls {
		if pageType := inferPageTypeFromURL(raw); pageType != "" {
			return pageType
		}
	}

	return ""
}

func decodeJWTAuthClaims(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil
	}

	claims, _ := payload["https://api.openai.com/auth"].(map[string]any)
	return claims
}
