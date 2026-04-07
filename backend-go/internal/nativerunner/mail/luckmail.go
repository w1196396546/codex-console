package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultLuckMailBaseURL     = "https://mails.luckyous.com"
	defaultLuckMailProjectCode = "openai"
	defaultLuckMailEmailType   = "ms_graph"
)

type LuckMailConfig struct {
	BaseURL         string
	APIKey          string
	ProjectCode     string
	EmailType       string
	PreferredDomain string
	HTTPClient      *http.Client
	PollInterval    time.Duration
}

type LuckMail struct {
	baseURL         string
	apiKey          string
	projectCode     string
	emailType       string
	preferredDomain string
	httpClient      *http.Client
	pollInterval    time.Duration
	codeTracker     *otpCodeTracker
}

func NewLuckMail(config LuckMailConfig) *LuckMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultLuckMailBaseURL
	}

	projectCode := strings.TrimSpace(config.ProjectCode)
	if projectCode == "" {
		projectCode = defaultLuckMailProjectCode
	}

	emailType := strings.TrimSpace(config.EmailType)
	if emailType == "" {
		emailType = defaultLuckMailEmailType
	}

	return &LuckMail{
		baseURL:         baseURL,
		apiKey:          strings.TrimSpace(config.APIKey),
		projectCode:     projectCode,
		emailType:       emailType,
		preferredDomain: strings.TrimSpace(strings.TrimPrefix(config.PreferredDomain, "@")),
		httpClient:      httpClient,
		pollInterval:    pollInterval,
		codeTracker:     newOTPCodeTracker(),
	}
}

func (l *LuckMail) Create(ctx context.Context) (Inbox, error) {
	if l == nil {
		return Inbox{}, errors.New("luckmail provider is required")
	}
	if l.baseURL == "" {
		return Inbox{}, errors.New("luckmail base url is required")
	}
	if l.apiKey == "" {
		return Inbox{}, errors.New("luckmail api key is required")
	}

	payload := map[string]any{
		"project_code": l.projectCode,
		"quantity":     1,
		"email_type":   l.emailType,
	}
	if l.preferredDomain != "" {
		payload["domain"] = l.preferredDomain
	}

	response, err := l.doJSON(ctx, http.MethodPost, "/api/v1/email/purchase", payload)
	if err != nil {
		return Inbox{}, fmt.Errorf("create luckmail inbox: %w", err)
	}

	item := firstLuckMailItem(response)
	email := strings.TrimSpace(luckMailStringField(item, "email_address", "address", "email"))
	token := strings.TrimSpace(luckMailStringField(item, "token"))
	if email == "" || token == "" {
		return Inbox{}, errors.New("decode luckmail create response: missing email or token")
	}

	return Inbox{
		Email: email,
		Token: token,
	}, nil
}

func (l *LuckMail) GetCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	l.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
	return l.pollCode(ctx, inbox, pattern)
}

func (l *LuckMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if l == nil {
		return "", errors.New("luckmail provider is required")
	}
	if l.baseURL == "" {
		return "", errors.New("luckmail base url is required")
	}
	if l.apiKey == "" {
		return "", errors.New("luckmail api key is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", errors.New("luckmail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()

	for {
		l.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
		code, found, err := l.pollCode(ctx, inbox, pattern)
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

func (l *LuckMail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	if l == nil {
		return "", false, errors.New("luckmail provider is required")
	}
	if l.baseURL == "" {
		return "", false, errors.New("luckmail base url is required")
	}
	if l.apiKey == "" {
		return "", false, errors.New("luckmail api key is required")
	}
	token := strings.TrimSpace(inbox.Token)
	if token == "" {
		return "", false, errors.New("luckmail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	response, err := l.doJSON(ctx, http.MethodGet, "/api/v1/email/query/"+url.PathEscape(token), nil)
	if err != nil {
		return "", false, fmt.Errorf("query luckmail code: %w", err)
	}

	receivedAt := parseLuckMailMessageTime(response)
	if !inbox.OTPSentAt.IsZero() {
		minReceivedAt := inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
		if !receivedAt.IsZero() && receivedAt.Before(minReceivedAt) {
			return "", false, nil
		}
	}

	inboxKey := otpInboxStateKey(inbox)
	messageID := luckMailStringField(response, "id", "mail_id", "message_id")
	content := buildSearchText(
		luckMailStringField(response, "subject"),
		flattenMessageText(luckMailStringField(response, "content")),
		flattenMessageText(luckMailStringField(response, "body")),
		flattenMessageText(luckMailStringField(response, "mail_content")),
		flattenMessageText(luckMailStringField(response, "text")),
		flattenMessageHTML(response["html"]),
	)

	code := strings.TrimSpace(luckMailStringField(response, "verification_code"))
	if code == "" {
		if looksLikeOpenAIVerification(content) {
			if matched, found := matchCode(pattern, content); found {
				fingerprint, fallbackCode := otpCodeFingerprint(messageID, receivedAt, content, matched)
				if l.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
					return "", false, nil
				}
				l.codeTracker.markSeen(inboxKey, fingerprint, fallbackCode)
				return matched, true, nil
			}
		}
		return "", false, nil
	}

	if matched, found := matchCode(pattern, code); found {
		fingerprint, fallbackCode := otpCodeFingerprint(messageID, receivedAt, content, matched)
		if l.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
			return "", false, nil
		}
		l.codeTracker.markSeen(inboxKey, fingerprint, fallbackCode)
		return matched, true, nil
	}

	return "", false, nil
}

func (l *LuckMail) doJSON(ctx context.Context, method, path string, payload map[string]any) (map[string]any, error) {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode luckmail request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, l.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build luckmail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request luckmail %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request luckmail %s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	var payloadResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payloadResponse); err != nil {
		return nil, fmt.Errorf("decode luckmail %s response: %w", path, err)
	}

	if code, ok := payloadResponse["code"]; ok && stringValue(code) != "" && stringValue(code) != "0" {
		return nil, fmt.Errorf("luckmail api error: %s", strings.TrimSpace(luckMailStringField(payloadResponse, "message", "msg")))
	}

	if data, ok := payloadResponse["data"].(map[string]any); ok {
		return data, nil
	}

	return payloadResponse, nil
}

func firstLuckMailItem(payload map[string]any) map[string]any {
	for _, key := range []string{"items", "list", "purchases"} {
		if item := firstLuckMailMap(payload[key]); item != nil {
			return item
		}
	}
	return payload
}

func firstLuckMailMap(value any) map[string]any {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		return nil
	}
	return item
}

func luckMailStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return strings.TrimSpace(stringValue(value))
		}
	}
	return ""
}

func parseLuckMailMessageTime(payload map[string]any) time.Time {
	for _, key := range []string{
		"received_at",
		"receivedAt",
		"created_at",
		"createdAt",
		"date",
		"updated_at",
		"updatedAt",
		"timestamp",
	} {
		if value, ok := payload[key]; ok {
			if parsed := parseLuckMailTimeValue(value); !parsed.IsZero() {
				return parsed
			}
		}
	}
	return time.Time{}
}

func parseLuckMailTimeValue(value any) time.Time {
	return parseMessageTimeValue(value)
}

func unixTimeFromNumeric(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value >= 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}
