package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type WorkspaceSelectionResult struct {
	StatusCode     int
	ContinueURL    string
	CallbackURL    string
	PageType       string
	OrganizationID string
	ProjectID      string
	RawData        map[string]any
}

type OrganizationSelectionResult struct {
	StatusCode  int
	ContinueURL string
	CallbackURL string
	PageType    string
	RawData     map[string]any
}

type ContinueCreateAccountResult struct {
	StatusCode     int
	ContinueURL    string
	CallbackURL    string
	FinalURL       string
	FinalPath      string
	PageType       string
	AccountID      string
	UserID         string
	WorkspaceID    string
	OrganizationID string
	ProjectID      string
	RefreshToken   string
	AccessToken    string
	SessionToken   string
	AuthProvider   string
	RawData        map[string]any
}

func (c *Client) ContinueCreateAccount(ctx context.Context, created CreateAccountResult) (ContinueCreateAccountResult, error) {
	if c == nil {
		return ContinueCreateAccountResult{}, errors.New("auth client is required")
	}

	result := ContinueCreateAccountResult{
		ContinueURL:  strings.TrimSpace(created.ContinueURL),
		CallbackURL:  strings.TrimSpace(created.CallbackURL),
		PageType:     strings.TrimSpace(created.PageType),
		AccountID:    strings.TrimSpace(created.AccountID),
		WorkspaceID:  strings.TrimSpace(created.WorkspaceID),
		RefreshToken: strings.TrimSpace(created.RefreshToken),
		RawData:      created.RawData,
	}
	if result.PageType == "" {
		result.PageType = extractPayloadPageType(created.RawData, result.CallbackURL, result.ContinueURL, created.FinalURL)
	}

	if result.CallbackURL == "" && shouldSelectWorkspace(result.PageType, result.ContinueURL) && result.WorkspaceID == "" {
		result.WorkspaceID = c.resolveWorkspaceIDForFlow(ctx, firstNonEmpty(result.ContinueURL, result.FinalURL))
	}

	if result.CallbackURL == "" && shouldSelectWorkspace(result.PageType, result.ContinueURL) && result.WorkspaceID != "" {
		selection, err := c.SelectWorkspace(ctx, result.WorkspaceID, result.ContinueURL)
		if err != nil {
			return ContinueCreateAccountResult{}, err
		}
		result.StatusCode = selection.StatusCode
		if selection.ContinueURL != "" {
			result.ContinueURL = selection.ContinueURL
		}
		if selection.CallbackURL != "" {
			result.CallbackURL = selection.CallbackURL
		}
		if selection.PageType != "" {
			result.PageType = selection.PageType
		}
		result.OrganizationID = selection.OrganizationID
		result.ProjectID = selection.ProjectID
	}

	startURL := firstNonEmpty(result.CallbackURL, result.ContinueURL)
	if startURL == "" {
		return ContinueCreateAccountResult{}, errors.New("continue create account requires callback or continue url")
	}

	followed, err := c.followFlow(ctx, startURL)
	if err != nil {
		return ContinueCreateAccountResult{}, err
	}
	return c.finalizeContinueCreateAccount(ctx, result, followed)
}

func (c *Client) continueInteractiveCreateAccount(ctx context.Context, result ContinueCreateAccountResult, continuation interactiveContinuationRequest) (ContinueCreateAccountResult, error) {
	if strings.EqualFold(strings.TrimSpace(continuation.Method), http.MethodPost) {
		followed, err := c.submitInteractiveContinuation(ctx, continuation, result.FinalURL)
		if err != nil {
			return ContinueCreateAccountResult{}, err
		}

		next := ContinueCreateAccountResult{
			StatusCode:     result.StatusCode,
			ContinueURL:    continuation.URL,
			CallbackURL:    callbackURLFromValue(continuation.URL),
			PageType:       inferPageTypeFromURL(continuation.URL),
			AccountID:      result.AccountID,
			UserID:         result.UserID,
			WorkspaceID:    result.WorkspaceID,
			OrganizationID: result.OrganizationID,
			ProjectID:      result.ProjectID,
			RefreshToken:   result.RefreshToken,
			AccessToken:    result.AccessToken,
			SessionToken:   result.SessionToken,
			AuthProvider:   result.AuthProvider,
			RawData:        result.RawData,
		}
		return c.finalizeContinueCreateAccount(ctx, next, followed)
	}

	return c.ContinueCreateAccount(ctx, CreateAccountResult{
		ContinueURL:  continuation.URL,
		CallbackURL:  callbackURLFromValue(continuation.URL),
		PageType:     inferPageTypeFromURL(continuation.URL),
		AccountID:    result.AccountID,
		WorkspaceID:  result.WorkspaceID,
		RefreshToken: result.RefreshToken,
		RawData:      result.RawData,
	})
}

func (c *Client) submitInteractiveContinuation(ctx context.Context, continuation interactiveContinuationRequest, referer string) (followedFlow, error) {
	headers := Headers{
		"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Origin":  c.origin(),
		"Referer": strings.TrimSpace(referer),
	}
	for key, value := range cloneHeaders(continuation.Headers) {
		headers[key] = value
	}

	body := append([]byte(nil), continuation.Body...)
	if len(body) == 0 {
		form := continuation.Form
		if form == nil {
			form = url.Values{}
		}
		body = []byte(form.Encode())
		if strings.TrimSpace(headers["Content-Type"]) == "" {
			headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
	}

	return c.followFlowRequest(ctx, flowRequest{
		Method:  firstNonEmpty(strings.ToUpper(strings.TrimSpace(continuation.Method)), http.MethodPost),
		URL:     continuation.URL,
		Headers: headers,
		Body:    body,
	})
}

func (c *Client) SelectWorkspace(ctx context.Context, workspaceID string, referer string) (WorkspaceSelectionResult, error) {
	trimmedWorkspaceID := strings.TrimSpace(workspaceID)
	if trimmedWorkspaceID == "" {
		return WorkspaceSelectionResult{}, errors.New("workspace id is required")
	}

	response, payload, err := c.postJSONFlow(ctx, "/api/accounts/workspace/select", referer, map[string]string{
		"workspace_id": trimmedWorkspaceID,
	})
	if err != nil {
		return WorkspaceSelectionResult{}, fmt.Errorf("select workspace: %w", err)
	}

	continueURL := extractContinueURL(c, payload)
	if continueURL == "" {
		continueURL = resolveLocation(response.Header.Get("Location"), response.FinalURL)
	}

	result := WorkspaceSelectionResult{
		StatusCode:  response.StatusCode,
		ContinueURL: continueURL,
		CallbackURL: callbackURLFromValue(continueURL),
		PageType:    extractPayloadPageType(payload, continueURL, response.FinalURL),
		RawData:     payload,
	}

	orgID, projectID := extractFirstOrganization(payload)
	if orgID != "" {
		result.OrganizationID = orgID
		result.ProjectID = projectID

		orgSelection, err := c.SelectOrganization(ctx, orgID, projectID, firstNonEmpty(continueURL, referer))
		if err != nil {
			return WorkspaceSelectionResult{}, err
		}

		if orgSelection.StatusCode != 0 {
			result.StatusCode = orgSelection.StatusCode
		}
		if orgSelection.ContinueURL != "" {
			result.ContinueURL = orgSelection.ContinueURL
		}
		if orgSelection.CallbackURL != "" {
			result.CallbackURL = orgSelection.CallbackURL
		}
		if orgSelection.PageType != "" {
			result.PageType = orgSelection.PageType
		}
	}

	return result, nil
}

func (c *Client) SelectOrganization(ctx context.Context, organizationID string, projectID string, referer string) (OrganizationSelectionResult, error) {
	trimmedOrganizationID := strings.TrimSpace(organizationID)
	if trimmedOrganizationID == "" {
		return OrganizationSelectionResult{}, errors.New("organization id is required")
	}

	body := map[string]string{
		"org_id": trimmedOrganizationID,
	}
	if trimmedProjectID := strings.TrimSpace(projectID); trimmedProjectID != "" {
		body["project_id"] = trimmedProjectID
	}

	response, payload, err := c.postJSONFlow(ctx, "/api/accounts/organization/select", referer, body)
	if err != nil {
		return OrganizationSelectionResult{}, fmt.Errorf("select organization: %w", err)
	}

	continueURL := extractContinueURL(c, payload)
	if continueURL == "" {
		continueURL = resolveLocation(response.Header.Get("Location"), response.FinalURL)
	}

	return OrganizationSelectionResult{
		StatusCode:  response.StatusCode,
		ContinueURL: continueURL,
		CallbackURL: callbackURLFromValue(continueURL),
		PageType:    extractPayloadPageType(payload, continueURL, response.FinalURL),
		RawData:     payload,
	}, nil
}

type followedFlow struct {
	StatusCode  int
	CallbackURL string
	FinalURL    string
	FinalPath   string
	PageType    string
	Body        string
}

type flowRequest struct {
	Method  string
	URL     string
	Headers Headers
	Body    []byte
}

func (c *Client) followFlow(ctx context.Context, startURL string) (followedFlow, error) {
	return c.followFlowRequest(ctx, flowRequest{
		Method: http.MethodGet,
		URL:    startURL,
		Headers: Headers{
			"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		},
	})
}

func (c *Client) followFlowRequest(ctx context.Context, start flowRequest) (followedFlow, error) {
	currentURL := strings.TrimSpace(start.URL)
	if currentURL == "" {
		return followedFlow{}, errors.New("flow start url is required")
	}

	httpClient := c.redirectTrackingClient()
	currentMethod := firstNonEmpty(strings.ToUpper(strings.TrimSpace(start.Method)), http.MethodGet)
	currentHeaders := cloneHeaders(start.Headers)
	currentBody := append([]byte(nil), start.Body...)

	referer := strings.TrimSpace(currentHeaders["Referer"])
	if referer == "" {
		referer = c.callbackURL()
	}
	callbackURL := callbackURLFromValue(currentURL)
	var lastResponse Response

	for redirectCount := 0; redirectCount < 12; redirectCount++ {
		requestURL, err := c.resolveURL(currentURL)
		if err != nil {
			return followedFlow{}, fmt.Errorf("resolve flow url: %w", err)
		}

		var requestBody io.Reader
		if len(currentBody) != 0 {
			requestBody = bytes.NewReader(currentBody)
		}

		httpRequest, err := http.NewRequestWithContext(ctx, currentMethod, requestURL.String(), requestBody)
		if err != nil {
			return followedFlow{}, fmt.Errorf("build flow request: %w", err)
		}

		applyHeaders(httpRequest.Header, c.defaultHeaders)
		requestHeaders := cloneHeaders(currentHeaders)
		if requestHeaders == nil {
			requestHeaders = Headers{}
		}
		if strings.TrimSpace(requestHeaders["Accept"]) == "" {
			requestHeaders["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
		}
		requestHeaders["Referer"] = referer
		applyHeaders(httpRequest.Header, requestHeaders)
		if c.userAgent != "" && httpRequest.Header.Get("User-Agent") == "" {
			httpRequest.Header.Set("User-Agent", c.userAgent)
		}

		httpResponse, err := httpClient.Do(httpRequest)
		if err != nil {
			return followedFlow{}, fmt.Errorf("follow flow: %w", err)
		}

		body, err := io.ReadAll(httpResponse.Body)
		httpResponse.Body.Close()
		if err != nil {
			return followedFlow{}, fmt.Errorf("read flow response: %w", err)
		}

		lastResponse = Response{
			StatusCode: httpResponse.StatusCode,
			Header:     httpResponse.Header.Clone(),
			Body:       body,
			FinalURL:   requestURL.String(),
			FinalPath:  requestURL.Path,
		}
		if lastResponse.FinalPath == "" {
			lastResponse.FinalPath = "/"
		}
		if callbackURL == "" {
			callbackURL = callbackURLFromValue(lastResponse.FinalURL)
		}

		location := strings.TrimSpace(httpResponse.Header.Get("Location"))
		if httpResponse.StatusCode < http.StatusMultipleChoices || httpResponse.StatusCode >= http.StatusBadRequest || location == "" {
			break
		}

		nextURL := resolveLocation(location, requestURL.String())
		if nextURL == "" {
			break
		}
		if callbackURL == "" {
			callbackURL = callbackURLFromValue(nextURL)
		}
		referer = requestURL.String()
		currentURL = nextURL
		if httpResponse.StatusCode == http.StatusTemporaryRedirect || httpResponse.StatusCode == http.StatusPermanentRedirect {
			currentHeaders = requestHeaders
			continue
		}

		currentMethod = http.MethodGet
		currentBody = nil
		currentHeaders = Headers{
			"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		}
	}

	return followedFlow{
		StatusCode:  lastResponse.StatusCode,
		CallbackURL: callbackURL,
		FinalURL:    lastResponse.FinalURL,
		FinalPath:   lastResponse.FinalPath,
		PageType:    extractPayloadPageType(nil, lastResponse.FinalURL, lastResponse.FinalPath),
		Body:        string(lastResponse.Body),
	}, nil
}

func (c *Client) finalizeContinueCreateAccount(ctx context.Context, result ContinueCreateAccountResult, followed followedFlow) (ContinueCreateAccountResult, error) {
	if result.StatusCode == 0 {
		result.StatusCode = followed.StatusCode
	}
	if followed.CallbackURL != "" {
		result.CallbackURL = followed.CallbackURL
	}
	if followed.FinalURL != "" {
		result.FinalURL = followed.FinalURL
	}
	if followed.FinalPath != "" {
		result.FinalPath = followed.FinalPath
	}
	if followed.PageType != "" {
		result.PageType = followed.PageType
	} else {
		result.PageType = extractPayloadPageType(nil, result.FinalURL, result.FinalPath)
	}
	if isInteractiveAuthPageType(result.PageType) {
		if continuation := c.extractInteractiveContinuation(result.PageType, followed.Body, result.FinalURL); continuation.URL != "" {
			continued, err := c.continueInteractiveCreateAccount(ctx, result, continuation)
			if err != nil {
				return ContinueCreateAccountResult{}, err
			}
			if strings.TrimSpace(continued.AccountID) == "" {
				continued.AccountID = result.AccountID
			}
			if strings.TrimSpace(continued.WorkspaceID) == "" {
				continued.WorkspaceID = result.WorkspaceID
			}
			if strings.TrimSpace(continued.RefreshToken) == "" {
				continued.RefreshToken = result.RefreshToken
			}
			return continued, nil
		}
		return result, nil
	}

	session, err := c.ReadSession(ctx)
	if err != nil {
		return ContinueCreateAccountResult{}, err
	}

	mergeContinueCreateAccountSession(&result, session)
	if result.PageType == "" {
		result.PageType = inferPageTypeFromURL(result.FinalURL)
	}

	return result, nil
}

func (c *Client) redirectTrackingClient() *http.Client {
	cloned := *c.httpClient
	cloned.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cloned
}

func (c *Client) postJSONFlow(ctx context.Context, path string, referer string, body any) (Response, map[string]any, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return Response{}, nil, fmt.Errorf("encode flow payload: %w", err)
	}

	headers := Headers{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"Origin":       c.origin(),
		"Referer":      strings.TrimSpace(referer),
	}
	if headers["Referer"] == "" {
		headers["Referer"] = c.callbackURL()
	}

	response, err := c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    path,
		Headers: headers,
		Body:    bytes.NewReader(bodyJSON),
	})
	if err != nil {
		return Response{}, nil, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return Response{}, nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	var payload map[string]any
	if len(response.Body) != 0 {
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			return Response{}, nil, fmt.Errorf("decode flow response: %w", err)
		}
	}

	return response, payload, nil
}

func shouldSelectWorkspace(pageType string, continueURL string) bool {
	if isWorkspaceSelectionPageType(pageType) {
		return true
	}
	return isWorkspaceSelectionPageType(inferPageTypeFromURL(continueURL))
}

func isWorkspaceSelectionPageType(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "workspace_selection", "consent":
		return true
	default:
		return false
	}
}

func callbackURLFromValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/api/auth/callback/openai") && strings.Contains(raw, "code=") {
		return raw
	}
	return ""
}

func resolveLocation(location string, base string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	baseURL, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return location
	}
	locationURL, err := url.Parse(location)
	if err != nil {
		return location
	}
	return baseURL.ResolveReference(locationURL).String()
}

func extractFirstOrganization(payload map[string]any) (string, string) {
	if len(payload) == 0 {
		return "", ""
	}

	candidates := []any{payload["orgs"]}
	if data := extractObject(payload["data"]); len(data) != 0 {
		candidates = append(candidates, data["orgs"])
	}

	for _, candidate := range candidates {
		orgs, _ := candidate.([]any)
		if len(orgs) == 0 {
			continue
		}

		organization := extractObject(orgs[0])
		organizationID := extractString(organization["id"])
		if organizationID == "" {
			continue
		}

		projectID := ""
		if projects, ok := organization["projects"].([]any); ok && len(projects) != 0 {
			projectID = extractString(extractObject(projects[0])["id"])
		}
		return organizationID, projectID
	}

	return "", ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeContinueCreateAccountSession(result *ContinueCreateAccountResult, session SessionResult) {
	if result == nil {
		return
	}

	if accessToken := strings.TrimSpace(session.AccessToken); accessToken != "" {
		result.AccessToken = accessToken
	}
	if sessionToken := strings.TrimSpace(session.SessionToken); sessionToken != "" {
		result.SessionToken = sessionToken
	}
	if authProvider := strings.TrimSpace(session.AuthProvider); authProvider != "" {
		result.AuthProvider = authProvider
	}
	if userID := strings.TrimSpace(session.UserID); userID != "" {
		result.UserID = userID
	}
	if refreshToken := strings.TrimSpace(session.RefreshToken); refreshToken != "" && strings.TrimSpace(result.RefreshToken) == "" {
		result.RefreshToken = refreshToken
	}

	if accountID := strings.TrimSpace(session.AccountID); accountID != "" {
		if strings.TrimSpace(result.AccountID) == "" || session.accountSource == sessionValueSourcePayload {
			result.AccountID = accountID
		}
	}
	if workspaceID := strings.TrimSpace(session.WorkspaceID); workspaceID != "" {
		if strings.TrimSpace(result.WorkspaceID) == "" || session.workspaceSource == sessionValueSourcePayload {
			result.WorkspaceID = workspaceID
		}
	}
}
