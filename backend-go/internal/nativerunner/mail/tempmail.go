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
	"strconv"
	"strings"
	"time"
)

var DefaultCodePattern = regexp.MustCompile(`\b(\d{6})\b`)

const defaultPollInterval = time.Second

type Config struct {
	BaseURL      string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Inbox struct {
	Email     string
	Token     string
	OTPSentAt time.Time
}

type Provider interface {
	Create(ctx context.Context) (Inbox, error)
	WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error)
}

type Tempmail struct {
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
	codeTracker  *otpCodeTracker
}

func NewTempmail(config Config) *Tempmail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Tempmail{
		baseURL:      strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		httpClient:   httpClient,
		pollInterval: pollInterval,
		codeTracker:  newOTPCodeTracker(),
	}
}

func (t *Tempmail) Create(ctx context.Context) (Inbox, error) {
	if t == nil {
		return Inbox{}, errors.New("tempmail provider is required")
	}
	if t.baseURL == "" {
		return Inbox{}, errors.New("tempmail base url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/inbox/create", bytes.NewBufferString(`{}`))
	if err != nil {
		return Inbox{}, fmt.Errorf("build tempmail create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return Inbox{}, fmt.Errorf("create tempmail inbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Inbox{}, fmt.Errorf("create tempmail inbox: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		Address string `json:"address"`
		Token   string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Inbox{}, fmt.Errorf("decode tempmail create response: %w", err)
	}
	payload.Address = strings.TrimSpace(payload.Address)
	payload.Token = strings.TrimSpace(payload.Token)
	if payload.Address == "" || payload.Token == "" {
		return Inbox{}, errors.New("decode tempmail create response: missing address or token")
	}

	return Inbox{
		Email: payload.Address,
		Token: payload.Token,
	}, nil
}

func (t *Tempmail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if t == nil {
		return "", errors.New("tempmail provider is required")
	}
	if t.baseURL == "" {
		return "", errors.New("tempmail base url is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", errors.New("tempmail inbox token is required")
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

func (t *Tempmail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	inboxURL, err := url.Parse(t.baseURL + "/inbox")
	if err != nil {
		return "", false, fmt.Errorf("build tempmail inbox url: %w", err)
	}
	query := inboxURL.Query()
	query.Set("token", strings.TrimSpace(inbox.Token))
	inboxURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inboxURL.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("build tempmail inbox request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("poll tempmail inbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, nil
	}

	var payload struct {
		Emails []struct {
			From    string `json:"from"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			HTML    string `json:"html"`
			Raw     any    `json:"raw"`
			Date    any    `json:"date"`
		} `json:"emails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false, fmt.Errorf("decode tempmail inbox response: %w", err)
	}

	minReceivedAt := time.Time{}
	if !inbox.OTPSentAt.IsZero() {
		minReceivedAt = inbox.OTPSentAt.Add(-defaultIMAPOTPTimeSkew)
	}
	inboxKey := otpInboxStateKey(inbox)
	for _, mail := range payload.Emails {
		receivedAt := parseTempmailMessageTime(mail.Date)
		if !receivedAt.IsZero() && !minReceivedAt.IsZero() && receivedAt.Before(minReceivedAt) {
			continue
		}

		raw := extractRawMessageContent(mail.Raw)
		content := buildSearchText(
			mail.From,
			raw.From,
			mail.Subject,
			raw.Subject,
			flattenMessageText(mail.Body),
			flattenMessageHTML(mail.HTML),
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

		fingerprint, fallbackCode := otpCodeFingerprint("", receivedAt, content, code)
		if t.codeTracker.hasSeen(inboxKey, fingerprint, fallbackCode) {
			continue
		}
		t.codeTracker.markSeen(inboxKey, fingerprint, fallbackCode)
		return code, true, nil
	}

	return "", false, nil
}

func parseTempmailMessageTime(value any) time.Time {
	return parseMessageTimeValue(value)
}

func parseUnixTimestamp(value string) (int64, error) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return seconds, nil
	}

	floatSeconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return int64(floatSeconds), nil
}
