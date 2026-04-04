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

var DefaultCodePattern = regexp.MustCompile(`\b(\d{6})\b`)

const defaultPollInterval = time.Second

type Config struct {
	BaseURL      string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Inbox struct {
	Email string
	Token string
}

type Provider interface {
	Create(ctx context.Context) (Inbox, error)
	WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error)
}

type Tempmail struct {
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
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
		} `json:"emails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false, fmt.Errorf("decode tempmail inbox response: %w", err)
	}

	for _, mail := range payload.Emails {
		content := strings.Join([]string{mail.From, mail.Subject, mail.Body, mail.HTML}, "\n")
		lowerContent := strings.ToLower(content)
		if !strings.Contains(lowerContent, "openai") {
			continue
		}
		match := pattern.FindStringSubmatch(content)
		if len(match) >= 2 {
			return match[1], true, nil
		}
		if len(match) == 1 {
			return match[0], true, nil
		}
	}

	return "", false, nil
}
