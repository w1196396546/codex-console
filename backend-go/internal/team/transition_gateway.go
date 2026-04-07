package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const transitionTeamAPIBaseURL = "https://chatgpt.com"

var transitionHTTPRequestTimeout = 30 * time.Second

type TransitionRequest struct {
	Method      string
	Path        string
	AccessToken string
	JSON        map[string]any
}

type TransitionTransport func(ctx context.Context, req TransitionRequest) error

type transitionMembershipGateway struct {
	transport TransitionTransport
}

func NewTransitionMembershipGateway(transport TransitionTransport) MembershipGateway {
	return &transitionMembershipGateway{
		transport: resolveTransitionTransport(transport),
	}
}

func (g *transitionMembershipGateway) RevokeInvite(ctx context.Context, params MembershipGatewayRevokeInviteParams) error {
	return g.transport(ctx, TransitionRequest{
		Method:      http.MethodDelete,
		Path:        fmt.Sprintf("/backend-api/accounts/%s/invites", strings.TrimSpace(params.TeamUpstreamAccountID)),
		AccessToken: strings.TrimSpace(params.OwnerAccessToken),
		JSON: map[string]any{
			"email_address": strings.TrimSpace(params.MemberEmail),
		},
	})
}

func (g *transitionMembershipGateway) RemoveMember(ctx context.Context, params MembershipGatewayRemoveMemberParams) error {
	return g.transport(ctx, TransitionRequest{
		Method:      http.MethodDelete,
		Path:        fmt.Sprintf("/backend-api/accounts/%s/users/%s", strings.TrimSpace(params.TeamUpstreamAccountID), strings.TrimSpace(params.UpstreamUserID)),
		AccessToken: strings.TrimSpace(params.OwnerAccessToken),
	})
}

func resolveTransitionTransport(transport TransitionTransport) TransitionTransport {
	if transport != nil {
		return transport
	}
	return defaultTransitionTransport
}

func defaultTransitionTransport(ctx context.Context, req TransitionRequest) error {
	requestCtx, cancel := withTransitionHTTPRequestTimeout(ctx)
	defer cancel()

	var body io.Reader
	if req.JSON != nil {
		payload, err := json.Marshal(req.JSON)
		if err != nil {
			return fmt.Errorf("marshal transition request: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(requestCtx, strings.TrimSpace(req.Method), transitionTeamAPIBaseURL+strings.TrimSpace(req.Path), body)
	if err != nil {
		return fmt.Errorf("build transition request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", transitionTeamAPIBaseURL)
	request.Header.Set("Referer", transitionTeamAPIBaseURL+"/")
	if token := strings.TrimSpace(req.AccessToken); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if req.JSON != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("team upstream request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusBadRequest {
		return nil
	}

	responseBody, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return fmt.Errorf("team upstream returned %d", response.StatusCode)
	}
	return fmt.Errorf("%s", extractTransitionErrorMessage(response.StatusCode, responseBody))
}

func withTransitionHTTPRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if transitionHTTPRequestTimeout <= 0 {
		return ctx, noopCancel
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, noopCancel
	}
	return context.WithTimeout(ctx, transitionHTTPRequestTimeout)
}

func noopCancel() {}

func extractTransitionErrorMessage(statusCode int, body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Sprintf("team upstream returned %d", statusCode)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if detail := strings.TrimSpace(stringValue(payload["detail"])); detail != "" {
			return detail
		}
		if errorMessage := nestedStringValue(payload, "error", "message"); strings.TrimSpace(errorMessage) != "" {
			return strings.TrimSpace(errorMessage)
		}
	}

	return trimmed
}

func nestedStringValue(payload map[string]any, keys ...string) string {
	current := any(payload)
	for _, key := range keys {
		node, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = node[key]
	}
	return stringValue(current)
}

func stringValue(value any) string {
	switch resolved := value.(type) {
	case string:
		return resolved
	case fmt.Stringer:
		return resolved.String()
	default:
		return ""
	}
}
