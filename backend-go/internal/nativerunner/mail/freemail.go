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

type FreemailConfig struct {
	BaseURL      string
	AdminToken   string
	Domain       string
	LocalPart    string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Freemail struct {
	baseURL      string
	adminToken   string
	domain       string
	localPart    string
	httpClient   *http.Client
	pollInterval time.Duration
	domains      []string
}

func NewFreemail(config FreemailConfig) *Freemail {
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Freemail{
		baseURL:      strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		adminToken:   strings.TrimSpace(config.AdminToken),
		domain:       strings.TrimSpace(config.Domain),
		localPart:    strings.TrimSpace(config.LocalPart),
		httpClient:   httpClient,
		pollInterval: pollInterval,
	}
}

func (f *Freemail) Create(ctx context.Context) (Inbox, error) {
	if f == nil {
		return Inbox{}, errors.New("freemail provider is required")
	}
	if f.baseURL == "" {
		return Inbox{}, errors.New("freemail base url is required")
	}
	if f.adminToken == "" {
		return Inbox{}, errors.New("freemail admin token is required")
	}

	domainIndex, err := f.ensureDomains(ctx)
	if err != nil {
		return Inbox{}, err
	}

	var payload struct {
		Email string `json:"email"`
	}
	if f.localPart != "" {
		body, err := json.Marshal(map[string]any{
			"local":       f.localPart,
			"domainIndex": domainIndex,
		})
		if err != nil {
			return Inbox{}, fmt.Errorf("marshal freemail create request: %w", err)
		}
		if err := f.doJSON(ctx, http.MethodPost, "/api/create", bytes.NewReader(body), nil, &payload); err != nil {
			return Inbox{}, fmt.Errorf("create freemail inbox: %w", err)
		}
	} else {
		params := url.Values{}
		params.Set("domainIndex", fmt.Sprintf("%d", domainIndex))
		if err := f.doJSON(ctx, http.MethodGet, "/api/generate", nil, params, &payload); err != nil {
			return Inbox{}, fmt.Errorf("generate freemail inbox: %w", err)
		}
	}

	payload.Email = strings.TrimSpace(payload.Email)
	if payload.Email == "" {
		return Inbox{}, errors.New("decode freemail create response: missing email")
	}

	return Inbox{Email: payload.Email}, nil
}

func (f *Freemail) GetCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, bool, error) {
	return f.pollCode(ctx, inbox, pattern, nil)
}

func (f *Freemail) WaitCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp) (string, error) {
	if f == nil {
		return "", errors.New("freemail provider is required")
	}
	if f.baseURL == "" {
		return "", errors.New("freemail base url is required")
	}
	if f.adminToken == "" {
		return "", errors.New("freemail admin token is required")
	}
	if strings.TrimSpace(inbox.Email) == "" {
		return "", errors.New("freemail inbox email is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	seenMailIDs := make(map[string]struct{})
	ticker := time.NewTicker(f.pollInterval)
	defer ticker.Stop()

	for {
		code, found, err := f.pollCode(ctx, inbox, pattern, seenMailIDs)
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

func (f *Freemail) ensureDomains(ctx context.Context) (int, error) {
	if len(f.domains) == 0 {
		var domains []string
		if err := f.doJSON(ctx, http.MethodGet, "/api/domains", nil, nil, &domains); err != nil {
			return 0, fmt.Errorf("list freemail domains: %w", err)
		}
		f.domains = domains
	}

	for i, domain := range f.domains {
		if strings.EqualFold(strings.TrimSpace(domain), f.domain) {
			return i, nil
		}
	}
	return 0, nil
}

func (f *Freemail) pollCode(ctx context.Context, inbox Inbox, pattern *regexp.Regexp, seenMailIDs map[string]struct{}) (string, bool, error) {
	if f == nil {
		return "", false, errors.New("freemail provider is required")
	}
	if strings.TrimSpace(inbox.Email) == "" {
		return "", false, errors.New("freemail inbox email is required")
	}
	if pattern == nil {
		pattern = DefaultCodePattern
	}

	params := url.Values{}
	params.Set("mailbox", strings.TrimSpace(inbox.Email))
	params.Set("limit", "20")

	var emails []struct {
		ID               string `json:"id"`
		Sender           string `json:"sender"`
		Subject          string `json:"subject"`
		Preview          string `json:"preview"`
		VerificationCode string `json:"verification_code"`
	}
	if err := f.doJSON(ctx, http.MethodGet, "/api/emails", nil, params, &emails); err != nil {
		return "", false, fmt.Errorf("list freemail emails: %w", err)
	}

	for _, email := range emails {
		mailID := strings.TrimSpace(email.ID)
		if seenMailIDs != nil {
			if _, ok := seenMailIDs[mailID]; ok {
				continue
			}
			if mailID != "" {
				seenMailIDs[mailID] = struct{}{}
			}
		}

		content := strings.Join([]string{email.Sender, email.Subject, email.Preview}, "\n")
		if !strings.Contains(strings.ToLower(content), "openai") {
			continue
		}

		if code := strings.TrimSpace(email.VerificationCode); code != "" {
			return code, true, nil
		}

		if code, found := matchCode(pattern, content); found {
			return code, true, nil
		}

		if mailID == "" {
			continue
		}

		var detail struct {
			Content     string `json:"content"`
			HTMLContent string `json:"html_content"`
		}
		if err := f.doJSON(ctx, http.MethodGet, "/api/email/"+url.PathEscape(mailID), nil, nil, &detail); err != nil {
			continue
		}

		if code, found := matchCode(pattern, detail.Content+"\n"+detail.HTMLContent); found {
			return code, true, nil
		}
	}

	return "", false, nil
}

func (f *Freemail) doJSON(ctx context.Context, method string, path string, body *bytes.Reader, params url.Values, target any) error {
	reqURL := f.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		bodyReader = body
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("build freemail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.adminToken)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request freemail %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request freemail %s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode freemail %s response: %w", path, err)
	}
	return nil
}

func matchCode(pattern *regexp.Regexp, content string) (string, bool) {
	match := pattern.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1], true
	}
	if len(match) == 1 {
		return match[0], true
	}
	return "", false
}
