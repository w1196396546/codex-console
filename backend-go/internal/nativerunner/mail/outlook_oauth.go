package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	outlookOAuthTokenURLLive      = "https://login.live.com/oauth20_token.srf"
	outlookOAuthTokenURLConsumers = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	outlookOAuthIMAPScope         = "https://outlook.office.com/IMAP.AccessAsUser.All offline_access"
	outlookOAuthRefreshBuffer     = 2 * time.Minute
)

type outlookOAuth2TokenConfig struct {
	Email        string
	ClientID     string
	RefreshToken string
	ProxyURL     string
	TokenURL     string
	Scope        string
	HTTPClient   *http.Client
}

type outlookOAuth2TokenSource struct {
	email        string
	clientID     string
	refreshToken string
	tokenURL     string
	scope        string
	httpClient   *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func newOutlookOAuth2AccessTokenSource(config outlookOAuth2TokenConfig) IMAPAccessTokenSource {
	source := &outlookOAuth2TokenSource{
		email:        strings.TrimSpace(config.Email),
		clientID:     strings.TrimSpace(config.ClientID),
		refreshToken: strings.TrimSpace(config.RefreshToken),
		tokenURL:     strings.TrimSpace(config.TokenURL),
		scope:        strings.TrimSpace(config.Scope),
		httpClient:   config.HTTPClient,
	}
	if source.httpClient == nil {
		source.httpClient = proxyAwareHTTPClient(defaultHTTPClientTimeout, config.ProxyURL)
	}
	return source.token
}

func (s *outlookOAuth2TokenSource) token(ctx context.Context) (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}

	s.mu.Lock()
	if s.accessToken != "" && time.Until(s.expiresAt) > outlookOAuthRefreshBuffer {
		token := s.accessToken
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	form := url.Values{
		"client_id":     []string{s.clientID},
		"refresh_token": []string{s.refreshToken},
		"grant_type":    []string{"refresh_token"},
	}
	if s.scope != "" {
		form.Set("scope", s.scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build outlook oauth token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("refresh outlook oauth token for %s: %w", s.email, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf(
			"refresh outlook oauth token for %s: unexpected status %d: %s",
			s.email,
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode outlook oauth token response for %s: %w", s.email, err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("decode outlook oauth token response for %s: missing access_token", s.email)
	}
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = int64(time.Hour / time.Second)
	}

	s.mu.Lock()
	s.accessToken = strings.TrimSpace(payload.AccessToken)
	if nextRefreshToken := strings.TrimSpace(payload.RefreshToken); nextRefreshToken != "" {
		s.refreshToken = nextRefreshToken
	}
	s.expiresAt = time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
	token := s.accessToken
	s.mu.Unlock()
	return token, nil
}

func (s *outlookOAuth2TokenSource) validate() error {
	if strings.TrimSpace(s.clientID) == "" || strings.TrimSpace(s.refreshToken) == "" {
		return fmt.Errorf("outlook oauth credentials are required")
	}
	if strings.TrimSpace(s.tokenURL) == "" {
		return fmt.Errorf("outlook oauth token url is required")
	}
	if s.httpClient == nil {
		return fmt.Errorf("outlook oauth http client is required")
	}
	return nil
}

func outlookOAuthSettingsForAddress(address string) (tokenURL string, scope string) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		host = strings.TrimSpace(address)
	}
	switch strings.ToLower(host) {
	case "outlook.live.com":
		return outlookOAuthTokenURLConsumers, outlookOAuthIMAPScope
	default:
		return outlookOAuthTokenURLLive, ""
	}
}
