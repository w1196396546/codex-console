package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
)

var consentStreamChunkPattern = regexp.MustCompile(`streamController\.enqueue\(("(?s:(?:\\.|[^"\\])*)")\)`)

func (c *Client) resolveWorkspaceIDForFlow(ctx context.Context, startURL string) string {
	if c == nil {
		return ""
	}

	if workspaceID := workspaceIDFromPayload(c.oauthSessionPayload()); workspaceID != "" {
		return workspaceID
	}

	return workspaceIDFromConsentHTML(c.fetchConsentHTML(ctx, startURL))
}

func (c *Client) oauthSessionPayload() map[string]any {
	if c == nil {
		return nil
	}

	value := strings.TrimSpace(c.cookieValue("oai-client-auth-session"))
	if value == "" {
		return nil
	}
	return decodeCookieJSONValue(value)
}

func decodeCookieJSONValue(value string) map[string]any {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}

	candidates := []string{raw}
	if dot := strings.Index(raw, "."); dot > 0 {
		candidates = append([]string{raw[:dot]}, candidates...)
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, encoded := range []string{
			candidate,
			candidate + strings.Repeat("=", (4-len(candidate)%4)%4),
		} {
			for _, encoding := range []*base64.Encoding{
				base64.RawURLEncoding,
				base64.URLEncoding,
				base64.RawStdEncoding,
				base64.StdEncoding,
			} {
				decoded, err := encoding.DecodeString(encoded)
				if err != nil || len(decoded) == 0 {
					continue
				}

				var payload map[string]any
				if err := json.Unmarshal(decoded, &payload); err == nil && len(payload) != 0 {
					return payload
				}
			}
		}
	}

	return nil
}

func workspaceIDFromPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	return firstNonEmpty(
		extractString(payload["workspace_id"]),
		extractString(payload["default_workspace_id"]),
		extractString(extractObject(payload["workspace"])["id"]),
		firstObjectID(payload["workspaces"]),
	)
}

func (c *Client) fetchConsentHTML(ctx context.Context, startURL string) string {
	startURL = strings.TrimSpace(startURL)
	if c == nil || startURL == "" {
		return ""
	}

	response, err := c.Get(ctx, startURL, Headers{
		"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Referer": c.callbackURL(),
	})
	if err != nil || response.StatusCode >= 400 {
		return ""
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		return ""
	}
	return string(response.Body)
}

func workspaceIDFromConsentHTML(html string) string {
	if !strings.Contains(strings.ToLower(html), "workspaces") {
		return ""
	}

	candidates := []string{html}
	for _, match := range consentStreamChunkPattern.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}

		var decoded string
		if err := json.Unmarshal([]byte(match[1]), &decoded); err == nil && strings.TrimSpace(decoded) != "" {
			candidates = append(candidates, decoded)
		}
	}
	if strings.Contains(html, `\"`) {
		candidates = append(candidates, strings.ReplaceAll(html, `\"`, `"`))
	}

	for _, candidate := range candidates {
		if workspaceID := workspaceIDFromConsentText(candidate); workspaceID != "" {
			return workspaceID
		}
	}
	return ""
}

func workspaceIDFromConsentText(text string) string {
	normalized := strings.ReplaceAll(text, `\"`, `"`)
	if workspaceID := workspaceIDFromPayload(map[string]any{
		"workspace_id":         firstMatchString(normalized, `"workspace_id"\s*:\s*"([^"]+)"`),
		"default_workspace_id": firstMatchString(normalized, `"default_workspace_id"\s*:\s*"([^"]+)"`),
	}); workspaceID != "" {
		return workspaceID
	}

	index := strings.Index(strings.ToLower(normalized), "workspaces")
	if index < 0 {
		return ""
	}

	end := index + 4096
	if end > len(normalized) {
		end = len(normalized)
	}
	chunk := normalized[index:end]
	return firstMatchString(chunk, `"id"\s*:\s*"([^"]+)"`)
}

func firstMatchString(text string, pattern string) string {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
