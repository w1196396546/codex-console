package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type sessionValueSource uint8

const (
	sessionValueSourceUnknown sessionValueSource = iota
	sessionValueSourcePayload
	sessionValueSourceClaims
	sessionValueSourceFallback
)

type SessionResult struct {
	StatusCode   int
	AccessToken  string
	RefreshToken string
	SessionToken string
	AccountID    string
	UserID       string
	WorkspaceID  string
	Expires      string
	AuthProvider string
	RawSession   map[string]any

	accountSource   sessionValueSource
	workspaceSource sessionValueSource
}

func (c *Client) ReadSession(ctx context.Context) (SessionResult, error) {
	response, err := c.Get(ctx, "/api/auth/session", Headers{
		"Accept":  "application/json",
		"Referer": c.callbackURL(),
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("read session: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return SessionResult{}, fmt.Errorf("read session: unexpected status %d", response.StatusCode)
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return SessionResult{}, fmt.Errorf("decode session response: %w", err)
	}

	accessToken := extractString(payload["accessToken"])
	if accessToken == "" {
		return SessionResult{}, errors.New("session access token missing")
	}

	claims := decodeJWTAuthClaims(accessToken)
	account := extractObject(payload["account"])
	user := extractObject(payload["user"])
	workspace := extractObject(payload["workspace"])

	accountID := extractString(account["id"])
	accountSource := sessionValueSourcePayload
	if accountID == "" {
		accountID = extractString(claims["chatgpt_account_id"])
		accountSource = sessionValueSourceClaims
	}
	if accountID == "" {
		accountSource = sessionValueSourceUnknown
	}

	userID := extractString(user["id"])
	if userID == "" {
		userID = extractString(claims["chatgpt_user_id"])
	}
	if userID == "" {
		userID = extractString(claims["user_id"])
	}

	workspaceID := extractString(payload["workspace_id"])
	workspaceSource := sessionValueSourcePayload
	if workspaceID == "" {
		workspaceID = extractString(payload["default_workspace_id"])
	}
	if workspaceID == "" {
		workspaceID = extractString(workspace["id"])
	}
	if workspaceID == "" {
		workspaceID = extractString(firstObjectID(payload["workspaces"]))
	}
	if workspaceID == "" {
		workspaceID = extractString(account["workspace_id"])
	}
	if workspaceID == "" {
		workspaceID = accountID
		workspaceSource = sessionValueSourceFallback
	}
	if workspaceID == "" {
		workspaceSource = sessionValueSourceUnknown
	}

	sessionToken := extractString(payload["sessionToken"])
	if sessionToken == "" {
		sessionToken = extractString(payload["session_token"])
	}
	if sessionToken == "" {
		sessionToken = c.sessionTokenCookieValue()
	}

	return SessionResult{
		StatusCode:  response.StatusCode,
		AccessToken: accessToken,
		RefreshToken: firstNonEmpty(
			extractString(payload["refreshToken"]),
			extractString(payload["refresh_token"]),
		),
		SessionToken:    sessionToken,
		AccountID:       accountID,
		UserID:          userID,
		WorkspaceID:     workspaceID,
		Expires:         extractString(payload["expires"]),
		AuthProvider:    extractString(payload["authProvider"]),
		RawSession:      payload,
		accountSource:   accountSource,
		workspaceSource: workspaceSource,
	}, nil
}

func (c *Client) cookieValue(name string) string {
	for _, cookie := range c.Cookies() {
		if cookie != nil && cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (c *Client) sessionTokenCookieValue() string {
	for _, name := range []string{
		"__Secure-next-auth.session-token",
		"_Secure-next-auth.session-token",
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

func firstObjectID(value any) string {
	items, _ := value.([]any)
	if len(items) == 0 {
		return ""
	}
	return extractString(extractObject(items[0])["id"])
}

func (c *Client) cookieChunkValue(prefix string) string {
	if c == nil {
		return ""
	}

	chunks := map[int]string{}
	indexes := make([]int, 0)
	for _, cookie := range c.Cookies() {
		if cookie == nil || !strings.HasPrefix(cookie.Name, prefix+".") {
			continue
		}

		indexText := strings.TrimPrefix(cookie.Name, prefix+".")
		index, err := strconv.Atoi(indexText)
		if err != nil {
			continue
		}

		if _, exists := chunks[index]; !exists {
			indexes = append(indexes, index)
		}
		chunks[index] = cookie.Value
	}
	if len(indexes) == 0 {
		return ""
	}

	sort.Ints(indexes)
	var builder strings.Builder
	for _, index := range indexes {
		builder.WriteString(chunks[index])
	}
	return builder.String()
}
