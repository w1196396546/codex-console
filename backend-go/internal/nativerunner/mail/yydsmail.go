package mail

import (
	"bytes"
	"context"
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

type YYDSMailConfig struct {
	BaseURL       string
	APIKey        string
	DefaultDomain string
	HTTPClient    *http.Client
	PollInterval  time.Duration
}

type YYDSMail struct {
	baseURL       string
	apiKey        string
	defaultDomain string
	httpClient    *http.Client
	pollInterval  time.Duration
}

type yydsMailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type yydsMailMessageSummary struct {
	ID      string          `json:"id"`
	From    yydsMailAddress `json:"from"`
	Subject string          `json:"subject"`
	Snippet string          `json:"snippet"`
	Preview string          `json:"preview"`
}

type yydsMailMessageDetail struct {
	From    yydsMailAddress `json:"from"`
	Subject string          `json:"subject"`
	Text    string          `json:"text"`
	HTML    any             `json:"html"`
}

func NewYYDSMail(config YYDSMailConfig) *YYDSMail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &YYDSMail{
		baseURL:       strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:        strings.TrimSpace(config.APIKey),
		defaultDomain: strings.TrimSpace(strings.TrimPrefix(config.DefaultDomain, "@")),
		httpClient:    httpClient,
		pollInterval:  pollInterval,
	}
}

func (y *YYDSMail) Create(ctx context.Context) (Inbox, error) {
	if y == nil {
		return Inbox{}, errors.New("yydsmail provider is required")
	}
	if y.baseURL == "" {
		return Inbox{}, errors.New("yydsmail base url is required")
	}
	if y.apiKey == "" {
		return Inbox{}, errors.New("yydsmail api key is required")
	}

	payload := map[string]string{
		"address": randomAlphaNumeric(10),
	}
	if y.defaultDomain != "" {
		payload["domain"] = y.defaultDomain
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Inbox{}, fmt.Errorf("marshal yydsmail create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, y.baseURL+"/accounts", bytes.NewReader(body))
	if err != nil {
		return Inbox{}, fmt.Errorf("build yydsmail create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", y.apiKey)

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return Inbox{}, fmt.Errorf("create yydsmail inbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Inbox{}, fmt.Errorf("create yydsmail inbox: unexpected status %d", resp.StatusCode)
	}

	var account struct {
		ID      string `json:"id"`
		Address string `json:"address"`
		Token   string `json:"token"`
	}
	if err := decodeYYDSMailPayload(resp.Body, &account); err != nil {
		return Inbox{}, fmt.Errorf("decode yydsmail create response: %w", err)
	}

	if strings.TrimSpace(account.ID) == "" || strings.TrimSpace(account.Address) == "" || strings.TrimSpace(account.Token) == "" {
		return Inbox{}, errors.New("decode yydsmail create response: missing id, address, or token")
	}

	return Inbox{
		Email: strings.TrimSpace(account.Address),
		Token: strings.TrimSpace(account.Token),
	}, nil
}

func (y *YYDSMail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if y == nil {
		return "", errors.New("yydsmail provider is required")
	}
	if y.baseURL == "" {
		return "", errors.New("yydsmail base url is required")
	}
	if strings.TrimSpace(inbox.Email) == "" {
		return "", errors.New("yydsmail inbox email is required")
	}
	if strings.TrimSpace(inbox.Token) == "" {
		return "", errors.New("yydsmail inbox token is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	seenMessageIDs := make(map[string]struct{})
	ticker := time.NewTicker(y.pollInterval)
	defer ticker.Stop()

	for {
		code, found, err := y.pollCode(ctx, inbox, pattern, seenMessageIDs)
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

func (y *YYDSMail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp, seenMessageIDs map[string]struct{}) (string, bool, error) {
	messagesURL, err := url.Parse(y.baseURL + "/messages")
	if err != nil {
		return "", false, fmt.Errorf("build yydsmail messages url: %w", err)
	}

	query := messagesURL.Query()
	query.Set("address", strings.TrimSpace(inbox.Email))
	query.Set("limit", "50")
	messagesURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, messagesURL.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("build yydsmail messages request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(inbox.Token))

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("poll yydsmail messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, nil
	}

	var listPayload struct {
		Messages []yydsMailMessageSummary `json:"messages"`
	}
	if err := decodeYYDSMailPayload(resp.Body, &listPayload); err != nil {
		return "", false, fmt.Errorf("decode yydsmail messages response: %w", err)
	}

	for _, message := range listPayload.Messages {
		messageID := strings.TrimSpace(message.ID)
		if messageID == "" {
			continue
		}
		if _, seen := seenMessageIDs[messageID]; seen {
			continue
		}

		detail, err := y.getMessageDetail(ctx, inbox.Token, messageID)
		if err != nil {
			return "", false, err
		}
		seenMessageIDs[messageID] = struct{}{}

		content := strings.Join([]string{
			buildYYDSMailSenderText(message.From),
			message.Subject,
			message.Snippet,
			message.Preview,
			buildYYDSMailSenderText(detail.From),
			detail.Subject,
			detail.Text,
			flattenYYDSMailHTML(detail.HTML),
		}, "\n")
		if !looksLikeOpenAIVerification(content) {
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

func (y *YYDSMail) getMessageDetail(ctx context.Context, token string, messageID string) (yydsMailMessageDetail, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.baseURL+"/messages/"+url.PathEscape(messageID), nil)
	if err != nil {
		return yydsMailMessageDetail{}, fmt.Errorf("build yydsmail message detail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return yydsMailMessageDetail{}, fmt.Errorf("get yydsmail message detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return yydsMailMessageDetail{}, fmt.Errorf("get yydsmail message detail: unexpected status %d", resp.StatusCode)
	}

	var detail yydsMailMessageDetail
	if err := decodeYYDSMailPayload(resp.Body, &detail); err != nil {
		return yydsMailMessageDetail{}, fmt.Errorf("decode yydsmail message detail response: %w", err)
	}

	return detail, nil
}

func decodeYYDSMailPayload(body io.Reader, target any) error {
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	var envelope struct {
		Success *bool           `json:"success"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if envelope.Success != nil && !*envelope.Success {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = strings.TrimSpace(envelope.Message)
		}
		if message == "" {
			message = "request failed"
		}
		return errors.New(message)
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return json.Unmarshal(envelope.Data, target)
	}
	return json.Unmarshal(raw, target)
}

func buildYYDSMailSenderText(sender yydsMailAddress) string {
	return strings.TrimSpace(strings.TrimSpace(sender.Name) + " " + strings.TrimSpace(sender.Address))
}

func looksLikeOpenAIVerification(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	if text == "" || !strings.Contains(text, "openai") {
		return false
	}

	keywords := []string{
		"verification code",
		"verify",
		"one-time code",
		"one time code",
		"security code",
		"your openai code",
		"验证码",
		"code is",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}

	return false
}

func flattenYYDSMailHTML(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			part := strings.TrimSpace(fmt.Sprint(item))
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(typed)
	}
}
