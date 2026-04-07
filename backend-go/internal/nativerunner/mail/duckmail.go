package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultDuckMailPasswordLength = 12

type DuckMailConfig struct {
	BaseURL        string
	DefaultDomain  string
	APIKey         string
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PasswordLength int
}

type DuckMail struct {
	baseURL        string
	defaultDomain  string
	apiKey         string
	httpClient     *http.Client
	pollInterval   time.Duration
	passwordLength int
	codeTracker    *otpCodeTracker
}

func NewDuckMail(config DuckMailConfig) *DuckMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	passwordLength := config.PasswordLength
	if passwordLength <= 0 {
		passwordLength = defaultDuckMailPasswordLength
	}

	return &DuckMail{
		baseURL:        strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		defaultDomain:  strings.TrimSpace(strings.TrimPrefix(config.DefaultDomain, "@")),
		apiKey:         strings.TrimSpace(config.APIKey),
		httpClient:     httpClient,
		pollInterval:   pollInterval,
		passwordLength: passwordLength,
		codeTracker:    newOTPCodeTracker(),
	}
}

func (d *DuckMail) Create(ctx context.Context) (Inbox, error) {
	if d == nil {
		return Inbox{}, errors.New("duckmail provider is required")
	}
	if d.baseURL == "" {
		return Inbox{}, errors.New("duckmail base url is required")
	}
	if d.defaultDomain == "" {
		return Inbox{}, errors.New("duckmail default domain is required")
	}

	address := fmt.Sprintf("%s@%s", randomAlphaNumeric(8), d.defaultDomain)
	password := randomAlphaNumeric(d.passwordLength)

	accountPayload, err := d.doJSON(
		ctx,
		http.MethodPost,
		d.baseURL+"/accounts",
		map[string]any{
			"address":  address,
			"password": password,
		},
		d.apiKey,
	)
	if err != nil {
		return Inbox{}, fmt.Errorf("create duckmail account: %w", err)
	}

	var accountResponse struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal(accountPayload, &accountResponse); err != nil {
		return Inbox{}, fmt.Errorf("decode duckmail account response: %w", err)
	}

	resolvedAddress := strings.TrimSpace(accountResponse.Address)
	if resolvedAddress == "" {
		resolvedAddress = address
	}

	tokenPayload, err := d.doJSON(
		ctx,
		http.MethodPost,
		d.baseURL+"/token",
		map[string]any{
			"address":  resolvedAddress,
			"password": password,
		},
		"",
	)
	if err != nil {
		return Inbox{}, fmt.Errorf("get duckmail token: %w", err)
	}

	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(tokenPayload, &tokenResponse); err != nil {
		return Inbox{}, fmt.Errorf("decode duckmail token response: %w", err)
	}

	if strings.TrimSpace(tokenResponse.Token) == "" {
		return Inbox{}, errors.New("decode duckmail token response: missing token")
	}

	return Inbox{
		Email: resolvedAddress,
		Token: strings.TrimSpace(tokenResponse.Token),
	}, nil
}

func (d *DuckMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if d == nil {
		return "", errors.New("duckmail provider is required")
	}
	if d.baseURL == "" {
		return "", errors.New("duckmail base url is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", errors.New("duckmail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		d.codeTracker.prepare(otpInboxStateKey(inbox), inbox.OTPSentAt)
		code, found, err := d.pollCode(ctx, inbox, pattern)
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

func (d *DuckMail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	token := strings.TrimSpace(inbox.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/messages?page=1", nil)
	if err != nil {
		return "", false, fmt.Errorf("build duckmail messages request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("list duckmail messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, nil
	}

	var payload struct {
		Members []struct {
			ID        string         `json:"id"`
			From      map[string]any `json:"from"`
			Subject   string         `json:"subject"`
			CreatedAt any            `json:"createdAt"`
			Received  any            `json:"receivedAt"`
			Date      any            `json:"date"`
			UpdatedAt any            `json:"updatedAt"`
			Timestamp any            `json:"timestamp"`
		} `json:"hydra:member"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false, fmt.Errorf("decode duckmail messages response: %w", err)
	}

	inboxKey := otpInboxStateKey(inbox)
	minReceivedAt := time.Time{}
	if !inbox.OTPSentAt.IsZero() {
		minReceivedAt = inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
	}

	type codeCandidate struct {
		code         string
		fingerprint  string
		fallbackCode string
		receivedAt   time.Time
	}

	var timestampedCandidates []codeCandidate
	var unknownTimeCandidates []codeCandidate

	for _, message := range payload.Members {
		if strings.TrimSpace(message.ID) == "" {
			continue
		}

		detail, err := d.getMessageDetail(ctx, token, message.ID)
		if err != nil {
			return "", false, err
		}

		receivedAt := firstParsedMessageTime(
			message.CreatedAt,
			message.Received,
			message.Date,
			message.UpdatedAt,
			message.Timestamp,
			detail["createdAt"],
			detail["created_at"],
			detail["receivedAt"],
			detail["received_at"],
			detail["date"],
			detail["updatedAt"],
			detail["updated_at"],
			detail["timestamp"],
		)
		if !receivedAt.IsZero() && !minReceivedAt.IsZero() && receivedAt.Before(minReceivedAt) {
			continue
		}

		content := buildDuckMailSearchText(message.From, message.Subject, detail)
		if !strings.Contains(strings.ToLower(content), "openai") {
			continue
		}

		code, found := matchCode(pattern, content)
		if !found {
			continue
		}

		fingerprint, fallbackCode := otpCodeFingerprint(message.ID, receivedAt, content, code)
		if d.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
			continue
		}

		candidate := codeCandidate{
			code:         code,
			fingerprint:  fingerprint,
			fallbackCode: fallbackCode,
			receivedAt:   receivedAt,
		}
		if receivedAt.IsZero() && !inbox.OTPSentAt.IsZero() {
			unknownTimeCandidates = append(unknownTimeCandidates, candidate)
			continue
		}
		timestampedCandidates = append(timestampedCandidates, candidate)
	}

	selectCandidate := func(candidates []codeCandidate) (codeCandidate, bool) {
		if len(candidates) == 0 {
			return codeCandidate{}, false
		}
		best := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.receivedAt.After(best.receivedAt) {
				best = candidate
			}
		}
		return best, true
	}

	if candidate, ok := selectCandidate(timestampedCandidates); ok {
		d.codeTracker.markSeen(inboxKey, candidate.fingerprint, candidate.fallbackCode)
		return candidate.code, true, nil
	}

	if len(unknownTimeCandidates) == 0 {
		return "", false, nil
	}

	if shouldWaitForTimestampedMessage(inbox.OTPSentAt, time.Now().UTC()) {
		return "", false, nil
	}

	candidate := unknownTimeCandidates[0]
	d.codeTracker.markSeen(inboxKey, candidate.fingerprint, candidate.fallbackCode)
	return candidate.code, true, nil
}

func (d *DuckMail) getMessageDetail(ctx context.Context, token, messageID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/messages/"+messageID, nil)
	if err != nil {
		return nil, fmt.Errorf("build duckmail message detail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get duckmail message detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get duckmail message detail: unexpected status %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode duckmail message detail response: %w", err)
	}
	return payload, nil
}

func (d *DuckMail) doJSON(ctx context.Context, method, rawURL string, payload map[string]any, bearerToken string) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var responsePayload any
	if err := json.NewDecoder(resp.Body).Decode(&responsePayload); err != nil {
		return nil, err
	}

	return json.Marshal(responsePayload)
}

func buildDuckMailSearchText(summaryFrom map[string]any, subject string, detail map[string]any) string {
	raw := firstRawMessageContent(detail["raw"], detail["rfc822"])
	return buildSearchText(
		joinDuckMailSender(summaryFrom),
		joinDuckMailSender(detail["from"]),
		raw.From,
		strings.TrimSpace(subject),
		strings.TrimSpace(stringValue(detail["subject"])),
		raw.Subject,
		flattenMessageText(detail["text"]),
		flattenMessageText(detail["snippet"]),
		flattenMessageHTML(detail["html"]),
		raw.Body,
	)
}

func joinDuckMailSender(sender any) string {
	senderMap, ok := sender.(map[string]any)
	if !ok || len(senderMap) == 0 {
		return ""
	}

	parts := []string{
		strings.TrimSpace(stringValue(senderMap["name"])),
		strings.TrimSpace(stringValue(senderMap["address"])),
	}

	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}

	return strings.Join(filtered, " ")
}

func stripDuckMailHTML(value any) string {
	switch htmlValue := value.(type) {
	case []any:
		parts := make([]string, 0, len(htmlValue))
		for _, part := range htmlValue {
			text := strings.TrimSpace(stringValue(part))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return html.UnescapeString(strings.Join(parts, "\n"))
	case []string:
		return html.UnescapeString(strings.Join(htmlValue, "\n"))
	default:
		return html.UnescapeString(stringValue(value))
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func randomAlphaNumeric(length int) string {
	if length <= 0 {
		length = 8
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return strings.Repeat("a", length)
	}

	for index, value := range randomBytes {
		buffer[index] = alphabet[int(value)%len(alphabet)]
	}

	return string(buffer)
}
