package mail

import (
	"context"
	"errors"
	"regexp"
	"strings"
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

func TestOutlookCreateRequiresEmailAndPassword(t *testing.T) {
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
			name: "missing password",
			config: OutlookConfig{
				Email: "native@example.com",
			},
			wantErr: "outlook password is required",
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

func TestOutlookWaitCodeDelegatesToIMAPProvider(t *testing.T) {
	t.Parallel()

	provider := NewOutlook(OutlookConfig{
		Email:    "native@example.com",
		Password: "secret-password",
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
		t.Fatalf("expected delegated password, got %q", captured.Password)
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
		hosts = append(hosts, config.Host)

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
	if gotHosts != "outlook.office365.com,outlook.live.com" {
		t.Fatalf("expected office365 then live host fallback, got %q", gotHosts)
	}
}
