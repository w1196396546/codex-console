package mail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewOutlookUsesOffice365IMAPEndpointAndReturnsCredentialsInbox(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:    "  native@example.com  ",
		Password: "  secret-password  ",
	})

	if provider.imapAddress != "outlook.office365.com:993" {
		t.Fatalf("expected office365 imap address, got %q", provider.imapAddress)
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox: %v", err)
	}
	if inbox.Email != "native@example.com" {
		t.Fatalf("expected normalized email, got %q", inbox.Email)
	}
	if inbox.Token != "secret-password" {
		t.Fatalf("expected password carried in token slot, got %q", inbox.Token)
	}
}

func TestOutlookCreateAllowsOAuthWithoutPassword(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:        "  native@example.com  ",
		ClientID:     "client-123",
		RefreshToken: "refresh-123",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox with oauth: %v", err)
	}
	if inbox.Email != "native@example.com" {
		t.Fatalf("expected normalized email, got %q", inbox.Email)
	}
	if inbox.Token != "" {
		t.Fatalf("expected oauth inbox token to stay empty, got %q", inbox.Token)
	}
}

func TestOutlookCreateRequiresEmailAndAuthMaterial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  OutlookConfig
		wantErr string
	}{
		{
			name: "missing email",
			config: OutlookConfig{
				Password: "secret-password",
			},
			wantErr: "outlook email is required",
		},
		{
			name: "missing auth material",
			config: OutlookConfig{
				Email: "native@example.com",
			},
			wantErr: "outlook password or oauth credentials are required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := NewOutlook(tt.config)
			_, err := provider.Create(context.Background())
			if err == nil {
				t.Fatal("expected create error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOutlookWaitCodePrefersOAuthIMAPProviderWhenOAuthConfigPresent(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:        "native@example.com",
		Password:     "secret-password",
		ClientID:     "client-123",
		RefreshToken: "refresh-123",
		ProxyURL:     "http://proxy.internal:8080",
	})
	provider.imapAddress = "imap.override.example:1143"

	var captured IMAPConfig
	provider.newIMAP = func(config IMAPConfig) *IMAPMail {
		captured = config
		return NewIMAPMail(IMAPConfig{
			Email:        config.Email,
			Password:     config.Password,
			PollInterval: 10,
			Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
				return []IMAPMessage{
					{
						From:    "OpenAI <noreply@openai.com>",
						Subject: "OpenAI sign-in",
						Body:    "Your verification code is 864209.",
					},
				}, nil
			},
		})
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), inbox, regexp.MustCompile(`\d{6}`))
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected code 864209, got %q", code)
	}
	if captured.Host != "imap.override.example" {
		t.Fatalf("expected imap host override, got %q", captured.Host)
	}
	if captured.Port != 1143 {
		t.Fatalf("expected imap port 1143, got %d", captured.Port)
	}
	if !captured.UseSSL {
		t.Fatal("expected outlook IMAP delegation to keep SSL enabled")
	}
	if captured.Email != "native@example.com" {
		t.Fatalf("expected delegated email, got %q", captured.Email)
	}
	if captured.Password != "secret-password" {
		t.Fatalf("expected password to remain available for fallback, got %q", captured.Password)
	}
	if captured.ProxyURL != "http://proxy.internal:8080" {
		t.Fatalf("expected delegated proxy url, got %q", captured.ProxyURL)
	}
	if captured.OAuth2AccessTokenSource == nil {
		t.Fatal("expected oauth config to provide xoauth2 token source")
	}
}

func TestOutlookWaitCodeKeepsPasswordIMAPWhenOAuthConfigMissing(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:    "native@example.com",
		Password: "secret-password",
	})

	var captured IMAPConfig
	provider.newIMAP = func(config IMAPConfig) *IMAPMail {
		captured = config
		return NewIMAPMail(IMAPConfig{
			Email:        config.Email,
			Password:     config.Password,
			PollInterval: 10,
			Fetcher: func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
				return []IMAPMessage{
					{
						From:    "OpenAI <noreply@openai.com>",
						Subject: "OpenAI sign-in",
						Body:    "Your verification code is 864209.",
					},
				}, nil
			},
		})
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), inbox, regexp.MustCompile(`\d{6}`))
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected code 864209, got %q", code)
	}
	if captured.OAuth2AccessTokenSource != nil {
		t.Fatal("expected password-only config to avoid oauth token source")
	}
}

func TestOutlookWaitCodeWithOAuthSkipsPasswordFallbackAttempts(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:        "native@example.com",
		Password:     "secret-password",
		ClientID:     "client-123",
		RefreshToken: "refresh-123",
	})

	var hosts []string
	provider.newIMAP = func(config IMAPConfig) *IMAPMail {
		authMode := "password"
		if config.OAuth2AccessTokenSource != nil {
			authMode = "oauth"
		}
		hosts = append(hosts, config.Host+":"+authMode)

		fetcher := func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			if config.Host == "outlook.live.com" && config.OAuth2AccessTokenSource != nil {
				return []IMAPMessage{
					{
						From:    "OpenAI <noreply@openai.com>",
						Subject: "OpenAI sign-in",
						Body:    "Your verification code is 864209.",
					},
				}, nil
			}
			return nil, errors.New("oauth host unavailable")
		}

		return NewIMAPMail(IMAPConfig{
			Email:                   config.Email,
			Password:                config.Password,
			OAuth2AccessTokenSource: config.OAuth2AccessTokenSource,
			PollInterval:            time.Millisecond,
			Fetcher:                 fetcher,
		})
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), inbox, regexp.MustCompile(`\d{6}`))
	if err != nil {
		t.Fatalf("wait code with oauth-only attempts: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected code 864209, got %q", code)
	}

	gotHosts := strings.Join(hosts, ",")
	if gotHosts != "outlook.office365.com:oauth,outlook.live.com:oauth" {
		t.Fatalf("expected oauth-only host attempts, got %q", gotHosts)
	}
}

func TestOutlookWaitCodeFallsBackToLiveIMAPHostWhenPrimaryFails(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:    "native@example.com",
		Password: "secret-password",
	})

	var hosts []string
	provider.newIMAP = func(config IMAPConfig) *IMAPMail {
		authMode := "password"
		if config.OAuth2AccessTokenSource != nil {
			authMode = "oauth"
		}
		hosts = append(hosts, config.Host+":"+authMode)

		fetcher := func(ctx context.Context, inbox Inbox) ([]IMAPMessage, error) {
			if config.Host == "outlook.office365.com" {
				return nil, errors.New("primary imap host unavailable")
			}

			return []IMAPMessage{
				{
					From:    "OpenAI <noreply@openai.com>",
					Subject: "OpenAI sign-in",
					Body:    "Your verification code is 864209.",
				},
			}, nil
		}

		return NewIMAPMail(IMAPConfig{
			Email:        config.Email,
			Password:     config.Password,
			PollInterval: time.Millisecond,
			Fetcher:      fetcher,
		})
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create outlook inbox: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), inbox, regexp.MustCompile(`\d{6}`))
	if err != nil {
		t.Fatalf("wait code with fallback host: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected fallback code 864209, got %q", code)
	}

	gotHosts := strings.Join(hosts, ",")
	if gotHosts != "outlook.office365.com:password,outlook.live.com:password" {
		t.Fatalf("expected office365 then live host fallback, got %q", gotHosts)
	}
}

func TestNewOutlookOAuth2AccessTokenSourceUsesConfiguredEndpointAndCachesToken(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		if r.Method != http.MethodPost {
			t.Fatalf("expected token POST request, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if got := r.Form.Get("client_id"); got != "client-123" {
			t.Fatalf("expected client_id client-123, got %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-123" {
			t.Fatalf("expected refresh_token refresh-123, got %q", got)
		}
		if got := r.Form.Get("scope"); got != outlookOAuthIMAPScope {
			t.Fatalf("expected scope %q, got %q", outlookOAuthIMAPScope, got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-123","expires_in":3600}`))
	}))
	defer server.Close()

	source := newOutlookOAuth2AccessTokenSource(outlookOAuth2TokenConfig{
		Email:        "native@example.com",
		ClientID:     "client-123",
		RefreshToken: "refresh-123",
		TokenURL:     server.URL,
		Scope:        outlookOAuthIMAPScope,
		HTTPClient:   server.Client(),
	})

	first, err := source(context.Background())
	if err != nil {
		t.Fatalf("first token refresh: %v", err)
	}
	second, err := source(context.Background())
	if err != nil {
		t.Fatalf("second token refresh: %v", err)
	}

	if first != "access-123" || second != "access-123" {
		t.Fatalf("expected cached access token access-123, got %q and %q", first, second)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one token request due to cache, got %d", requests.Load())
	}
}
