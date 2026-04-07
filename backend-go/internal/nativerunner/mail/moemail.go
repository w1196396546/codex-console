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

type MoeMailConfig struct {
	BaseURL       string
	APIKey        string
	DefaultDomain string
	HTTPClient    *http.Client
	PollInterval  time.Duration
}

type MoeMail struct {
	baseURL       string
	apiKey        string
	defaultDomain string
	httpClient    *http.Client
	pollInterval  time.Duration
	codeTracker   *otpCodeTracker
}

func NewMoeMail(config MoeMailConfig) *MoeMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &MoeMail{
		baseURL:       strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:        strings.TrimSpace(config.APIKey),
		defaultDomain: strings.TrimSpace(strings.TrimPrefix(config.DefaultDomain, "@")),
		httpClient:    httpClient,
		pollInterval:  pollInterval,
		codeTracker:   newOTPCodeTracker(),
	}
}

func (m *MoeMail) Create(ctx context.Context) (Inbox, error) {
	if m == nil {
		return Inbox{}, errors.New("moemail provider is required")
	}
	if m.baseURL == "" {
		return Inbox{}, errors.New("moemail base url is required")
	}
	if m.apiKey == "" {
		return Inbox{}, errors.New("moemail api key is required")
	}

	payload := map[string]any{
		"name": randomAlphaNumeric(10),
	}
	if m.defaultDomain != "" {
		payload["domain"] = m.defaultDomain
	}

	var response struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := m.doJSON(ctx, http.MethodPost, "/api/emails/generate", payload, &response); err != nil {
		return Inbox{}, fmt.Errorf("create moemail inbox: %w", err)
	}

	response.ID = strings.TrimSpace(response.ID)
	response.Email = strings.TrimSpace(response.Email)
	if response.ID == "" || response.Email == "" {
		return Inbox{}, errors.New("decode moemail create response: missing id or email")
	}

	return Inbox{
		Email: response.Email,
		Token: response.ID,
	}, nil
}

func (m *MoeMail) GetCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	m.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
	return m.pollCode(ctx, inbox, pattern, nil)
}

func (m *MoeMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if m == nil {
		return "", errors.New("moemail provider is required")
	}
	if m.baseURL == "" {
		return "", errors.New("moemail base url is required")
	}
	if m.apiKey == "" {
		return "", errors.New("moemail api key is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", errors.New("moemail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	seenMessageIDs := make(map[string]struct{})
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		m.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
		code, found, err := m.pollCode(ctx, inbox, pattern, seenMessageIDs)
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

func (m *MoeMail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp, seenMessageIDs map[string]struct{}) (string, bool, error) {
	if m == nil {
		return "", false, errors.New("moemail provider is required")
	}
	if m.baseURL == "" {
		return "", false, errors.New("moemail base url is required")
	}
	if m.apiKey == "" {
		return "", false, errors.New("moemail api key is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", false, errors.New("moemail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	var payload struct {
		Messages []struct {
			ID          string `json:"id"`
			FromAddress string `json:"from_address"`
			Subject     string `json:"subject"`
			CreatedAt   any    `json:"created_at"`
			ReceivedAt  any    `json:"received_at"`
			Date        any    `json:"date"`
		} `json:"messages"`
	}
	if err := m.doJSON(ctx, http.MethodGet, "/api/emails/"+url.PathEscape(strings.TrimSpace(inbox.Token)), nil, &payload); err != nil {
		return "", false, fmt.Errorf("list moemail messages: %w", err)
	}

	inboxKey := otpInboxStateKey(inbox)
	for _, message := range payload.Messages {
		listReceivedAt := firstMoeMailMessageTime(message.CreatedAt, message.ReceivedAt, message.Date)
		if !inbox.OTPSentAt.IsZero() {
			minReceivedAt := inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
			if !listReceivedAt.IsZero() && listReceivedAt.Before(minReceivedAt) {
				continue
			}
		}

		messageID := strings.TrimSpace(message.ID)
		if messageID == "" {
			continue
		}
		if seenMessageIDs != nil {
			if _, seen := seenMessageIDs[messageID]; seen {
				continue
			}
		}

		detail, err := m.getMessageDetail(ctx, strings.TrimSpace(inbox.Token), messageID)
		if err != nil {
			return "", false, err
		}
		if seenMessageIDs != nil {
			seenMessageIDs[messageID] = struct{}{}
		}

		detailReceivedAt := firstMoeMailMessageTime(detail.CreatedAt, detail.ReceivedAt, detail.Date)
		if detailReceivedAt.IsZero() {
			detailReceivedAt = listReceivedAt
		}
		if !inbox.OTPSentAt.IsZero() {
			minReceivedAt := inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
			if !detailReceivedAt.IsZero() && detailReceivedAt.Before(minReceivedAt) {
				continue
			}
		}

		raw := firstRawMessageContent(detail.Raw, detail.RFC822)
		content := buildSearchText(
			message.FromAddress,
			message.Subject,
			detail.FromAddress,
			detail.Subject,
			flattenMessageText(detail.Content),
			flattenMessageHTML(detail.HTML),
			raw.From,
			raw.Subject,
			raw.Body,
		)
		if !looksLikeOpenAIVerification(content) {
			continue
		}

		if code, found := matchCode(pattern, content); found {
			fingerprint, fallbackCode := otpCodeFingerprint(messageID, detailReceivedAt, content, code)
			if m.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
				continue
			}
			m.codeTracker.markSeen(inboxKey, fingerprint, fallbackCode)
			return code, true, nil
		}
	}

	return "", false, nil
}

func (m *MoeMail) getMessageDetail(ctx context.Context, emailID, messageID string) (struct {
	FromAddress string `json:"from_address"`
	Subject     string `json:"subject"`
	Content     string `json:"content"`
	HTML        any    `json:"html"`
	Raw         any    `json:"raw"`
	RFC822      any    `json:"rfc822"`
	CreatedAt   any    `json:"created_at"`
	ReceivedAt  any    `json:"received_at"`
	Date        any    `json:"date"`
}, error) {
	var payload struct {
		Message struct {
			FromAddress string `json:"from_address"`
			Subject     string `json:"subject"`
			Content     string `json:"content"`
			HTML        any    `json:"html"`
			Raw         any    `json:"raw"`
			RFC822      any    `json:"rfc822"`
			CreatedAt   any    `json:"created_at"`
			ReceivedAt  any    `json:"received_at"`
			Date        any    `json:"date"`
		} `json:"message"`
	}
	if err := m.doJSON(ctx, http.MethodGet, "/api/emails/"+url.PathEscape(emailID)+"/"+url.PathEscape(messageID), nil, &payload); err != nil {
		return payload.Message, fmt.Errorf("get moemail message detail: %w", err)
	}
	return payload.Message, nil
}

func (m *MoeMail) doJSON(ctx context.Context, method, path string, payload map[string]any, target any) error {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode moemail request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build moemail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", m.apiKey)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request moemail %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request moemail %s %s: unexpected status %d", method, path, resp.StatusCode)
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode moemail %s response: %w", path, err)
	}
	return nil
}

func firstMoeMailMessageTime(values ...any) time.Time {
	for _, value := range values {
		if parsed := parseMoeMailTimeValue(value); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Time{}
}

func parseMoeMailTimeValue(value any) time.Time {
	return parseMessageTimeValue(value)
}
