package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type SelfHostedTempMailConfig struct {
	BaseURL       string
	AdminPassword string
	CustomAuth    string
	Domain        string
	EnablePrefix  bool
	HTTPClient    *http.Client
	PollInterval  time.Duration
}

type SelfHostedTempMail struct {
	baseURL       string
	adminPassword string
	customAuth    string
	domain        string
	enablePrefix  bool
	httpClient    *http.Client
	pollInterval  time.Duration
	codeTracker   *otpCodeTracker
}

func NewSelfHostedTempMail(config SelfHostedTempMailConfig) *SelfHostedTempMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &SelfHostedTempMail{
		baseURL:       strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		adminPassword: strings.TrimSpace(config.AdminPassword),
		customAuth:    strings.TrimSpace(config.CustomAuth),
		domain:        strings.TrimSpace(config.Domain),
		enablePrefix:  config.EnablePrefix,
		httpClient:    httpClient,
		pollInterval:  pollInterval,
		codeTracker:   newOTPCodeTracker(),
	}
}

func (t *SelfHostedTempMail) Create(ctx context.Context) (Inbox, error) {
	if t == nil {
		return Inbox{}, errors.New("self-hosted temp_mail provider is required")
	}
	if t.baseURL == "" {
		return Inbox{}, errors.New("temp_mail base url is required")
	}
	if t.adminPassword == "" {
		return Inbox{}, errors.New("temp_mail admin password is required")
	}
	if t.domain == "" {
		return Inbox{}, errors.New("temp_mail domain is required")
	}

	payload, err := json.Marshal(map[string]any{
		"enablePrefix": t.enablePrefix,
		"name":         randomLocalPart(),
		"domain":       t.domain,
	})
	if err != nil {
		return Inbox{}, fmt.Errorf("encode self-hosted temp_mail create payload: %w", err)
	}

	statusCode, body, err := t.doRequest(ctx, http.MethodPost, "/admin/new_address", t.adminHeaders(), bytes.NewReader(payload))
	if err != nil {
		return Inbox{}, fmt.Errorf("create self-hosted temp_mail inbox: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return Inbox{}, fmt.Errorf("create self-hosted temp_mail inbox: unexpected status %d", statusCode)
	}

	var response struct {
		Address string `json:"address"`
		JWT     string `json:"jwt"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return Inbox{}, fmt.Errorf("decode self-hosted temp_mail create response: %w", err)
	}

	address := strings.TrimSpace(response.Address)
	token := strings.TrimSpace(response.JWT)
	if token == "" {
		token = strings.TrimSpace(response.Token)
	}
	if address == "" {
		return Inbox{}, errors.New("decode self-hosted temp_mail create response: missing address")
	}

	return Inbox{
		Email: address,
		Token: token,
	}, nil
}

func (t *SelfHostedTempMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if t == nil {
		return "", errors.New("self-hosted temp_mail provider is required")
	}
	if t.baseURL == "" {
		return "", errors.New("temp_mail base url is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	for {
		t.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
		code, found, err := t.pollCode(ctx, inbox, pattern)
		if err != nil {
			return "", err
		}
		if found {
			return code, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func (t *SelfHostedTempMail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	mails, err := t.fetchMailsOnce(ctx, inbox)
	if err != nil {
		return "", false, err
	}

	minReceivedAt := time.Time{}
	if !inbox.OTPSentAt.IsZero() {
		minReceivedAt = inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
	}
	inboxKey := otpInboxStateKey(inbox)

	for _, item := range mails {
		receivedAt := firstParsedMessageTime(
			item["createdAt"],
			item["created_at"],
			item["date"],
			item["created"],
			item["timestamp"],
			item["time"],
		)
		if !receivedAt.IsZero() && !minReceivedAt.IsZero() && receivedAt.Before(minReceivedAt) {
			continue
		}

		raw := firstRawMessageContent(item["raw"], item["mime"], item["source_raw"])
		content := buildSearchText(
			stringMapValue(item, "source", "from", "from_address", "fromAddress"),
			raw.From,
			stringMapValue(item, "subject", "title"),
			raw.Subject,
			flattenMessageText(firstMapValue(item, "text", "body", "content")),
			flattenMessageHTML(firstMapValue(item, "html")),
			raw.Body,
		)
		lowerContent := strings.ToLower(content)
		if !strings.Contains(lowerContent, "openai") {
			continue
		}

		match := pattern.FindStringSubmatch(content)
		var code string
		switch {
		case len(match) >= 2:
			code = match[1]
		case len(match) == 1:
			code = match[0]
		default:
			continue
		}

		fingerprint, fallbackCode := otpCodeFingerprint(extractMailID(item), receivedAt, content, code)
		if t.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
			continue
		}
		t.codeTracker.markSeen(inboxKey, fingerprint, fallbackCode)
		return code, true, nil
	}

	return "", false, nil
}

func (t *SelfHostedTempMail) fetchMailsOnce(ctx context.Context, inbox Inbox) ([]map[string]any, error) {
	attempts := make([]selfHostedTempMailAttempt, 0, 4)
	if strings.TrimSpace(inbox.Token) != "" {
		attempts = append(attempts,
			selfHostedTempMailAttempt{
				path: "/api/mails",
				query: url.Values{
					"limit":  {"50"},
					"offset": {"0"},
				},
				headers: map[string]string{
					"Accept":        "application/json",
					"Authorization": "Bearer " + strings.TrimSpace(inbox.Token),
				},
			},
			selfHostedTempMailAttempt{
				path: "/user_api/mails",
				query: url.Values{
					"limit":  {"50"},
					"offset": {"0"},
				},
				headers: map[string]string{
					"Accept":       "application/json",
					"x-user-token": strings.TrimSpace(inbox.Token),
				},
			},
		)
	}

	attempts = append(attempts,
		selfHostedTempMailAttempt{
			path: "/admin/mails",
			query: url.Values{
				"limit":   {"80"},
				"offset":  {"0"},
				"address": {strings.TrimSpace(inbox.Email)},
			},
			headers: t.adminHeaders(),
		},
		selfHostedTempMailAttempt{
			path: "/admin/mails",
			query: url.Values{
				"limit":  {"120"},
				"offset": {"0"},
			},
			headers:     t.adminHeaders(),
			filterEmail: true,
		},
	)

	var lastErr error
	successfulAttempt := false
	for _, attempt := range attempts {
		mails, err := t.fetchMailAttempt(ctx, attempt)
		if err != nil {
			lastErr = err
			continue
		}
		successfulAttempt = true
		if attempt.filterEmail {
			mails = filterSelfHostedTempMailByEmail(mails, inbox.Email)
		}
		if len(mails) > 0 {
			return mails, nil
		}
	}

	if successfulAttempt {
		return nil, nil
	}
	return nil, lastErr
}

func (t *SelfHostedTempMail) fetchMailAttempt(ctx context.Context, attempt selfHostedTempMailAttempt) ([]map[string]any, error) {
	statusCode, body, err := t.doRequest(ctx, http.MethodGet, buildPathWithQuery(attempt.path, attempt.query), attempt.headers, nil)
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("request %s: unexpected status %d", attempt.path, statusCode)
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", attempt.path, err)
	}
	return extractSelfHostedTempMailList(payload), nil
}

func (t *SelfHostedTempMail) doRequest(ctx context.Context, method string, path string, headers map[string]string, body *bytes.Reader) (int, []byte, error) {
	targetURL, err := url.Parse(t.baseURL + path)
	if err != nil {
		return 0, nil, fmt.Errorf("build self-hosted temp_mail url: %w", err)
	}

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = body
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build self-hosted temp_mail request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("perform self-hosted temp_mail request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read self-hosted temp_mail response: %w", err)
	}
	return resp.StatusCode, data, nil
}

func (t *SelfHostedTempMail) adminHeaders() map[string]string {
	headers := map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"x-admin-auth": t.adminPassword,
	}
	if t.customAuth != "" {
		headers["x-custom-auth"] = t.customAuth
	}
	return headers
}

type selfHostedTempMailAttempt struct {
	path        string
	query       url.Values
	headers     map[string]string
	filterEmail bool
}

func extractSelfHostedTempMailList(payload any) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		return collectSelfHostedTempMailItems(typed)
	case map[string]any:
		for _, key := range []string{"results", "mails", "data", "items", "list"} {
			if items, ok := typed[key].([]any); ok {
				return collectSelfHostedTempMailItems(items)
			}
		}
	}
	return nil
}

func collectSelfHostedTempMailItems(items []any) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if ok {
			result = append(result, object)
		}
	}
	return result
}

func filterSelfHostedTempMailByEmail(items []map[string]any, email string) []map[string]any {
	target := strings.ToLower(strings.TrimSpace(email))
	if target == "" {
		return items
	}

	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		matchedRecipient := false
		for _, key := range []string{"address", "email", "to", "to_address", "toAddress", "target", "recipient"} {
			if strings.Contains(strings.ToLower(stringMapValue(item, key)), target) {
				filtered = append(filtered, item)
				matchedRecipient = true
				break
			}
		}
		if matchedRecipient {
			continue
		}

		content := buildSearchText(
			stringMapValue(item, "source", "from", "from_address", "fromAddress"),
			stringMapValue(item, "subject", "title"),
			flattenMessageText(firstMapValue(item, "text", "body", "content")),
			flattenMessageHTML(firstMapValue(item, "html")),
			firstRawMessageContent(item["raw"], item["mime"], item["source_raw"]).Body,
		)
		if strings.Contains(strings.ToLower(content), target) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func firstMapValue(item map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := item[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func stringMapValue(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok && value != nil {
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			default:
				if trimmed := strings.TrimSpace(fmt.Sprint(typed)); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func extractMailID(item map[string]any) string {
	for _, key := range []string{"id", "mail_id", "mailId", "_id", "uuid"} {
		if value := stringMapValue(item, key); value != "" {
			return value
		}
	}
	return ""
}

func buildPathWithQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

func randomLocalPart() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		return "openai" + fmt.Sprint(time.Now().UnixNano())
	}
	for i := range buffer {
		buffer[i] = alphabet[int(buffer[i])%len(alphabet)]
	}
	return string(buffer)
}
