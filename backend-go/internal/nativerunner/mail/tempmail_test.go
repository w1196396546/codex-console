package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTempmailCreateReturnsInbox(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/inbox/create" {
			t.Fatalf("expected /inbox/create, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"address": "native@example.com",
			"token":   "token-123",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewTempmail(Config{
		BaseURL: server.URL,
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@example.com" {
		t.Fatalf("expected email native@example.com, got %q", inbox.Email)
	}
	if inbox.Token != "token-123" {
		t.Fatalf("expected token token-123, got %q", inbox.Token)
	}
}

func TestTempmailWaitCodePollsInboxUntilCodeArrives(t *testing.T) {
	t.Parallel()

	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/inbox":
			pollCount++
			if got := r.URL.Query().Get("token"); got != "token-123" {
				t.Fatalf("expected token query token-123, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			payload := map[string]any{
				"emails": []map[string]any{},
			}
			if pollCount >= 2 {
				payload["emails"] = []map[string]any{
					{
						"from":    "OpenAI <noreply@openai.com>",
						"subject": "Your verification code is 654321",
						"body":    "Use 654321 to continue",
						"date":    float64(2),
					},
				}
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewTempmail(Config{
		BaseURL:      server.URL,
		PollInterval: 10 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "native@example.com",
		Token: "token-123",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected code 654321, got %q", code)
	}
	if pollCount < 2 {
		t.Fatalf("expected at least 2 polls, got %d", pollCount)
	}
}

func TestTempmailWaitCodeSkipsMessagesBeforeOTPSentAt(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/inbox" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"emails": []map[string]any{
				{
					"from":    "OpenAI <noreply@openai.com>",
					"subject": "Your verification code is 111111",
					"body":    "Use 111111 to continue",
					"date":    float64(sentAt.Add(-15 * time.Second).Unix()),
				},
				{
					"from":    "OpenAI <noreply@openai.com>",
					"subject": "Your verification code is 222222",
					"body":    "Use 222222 to continue",
					"date":    float64(sentAt.Add(2 * time.Second).Unix()),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewTempmail(Config{
		BaseURL: server.URL,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		Token:     "token-123",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with otp sent at: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected newer code 222222, got %q", code)
	}
}

func TestTempmailWaitCodeResetsFallbackCodeDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/inbox" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"emails": []map[string]any{
				{
					"from":    "OpenAI <noreply@openai.com>",
					"subject": "Your verification code is 654321",
					"body":    "Use 654321 to continue",
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewTempmail(Config{
		BaseURL:      server.URL,
		PollInterval: 5 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		Token:     "token-123",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("first wait code for stage one: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected first stage code 654321, got %q", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = provider.WaitCode(ctx, Inbox{
		Email:     "native@example.com",
		Token:     "token-123",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip fallback duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "native@example.com",
		Token:     "token-123",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected same fallback code to be accepted for a new stage, got %q", code)
	}
}
