package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMoeMailCreateReturnsInbox(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/emails/generate" {
			t.Fatalf("expected /api/emails/generate, got %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["domain"] != "mail.example.com" {
			t.Fatalf("expected domain mail.example.com, got %#v", payload["domain"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":    "mailbox-1",
			"email": "native@mail.example.com",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL:       server.URL,
		APIKey:        "test-api-key",
		DefaultDomain: "mail.example.com",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@mail.example.com" {
		t.Fatalf("expected email native@mail.example.com, got %q", inbox.Email)
	}
	if inbox.Token != "mailbox-1" {
		t.Fatalf("expected token mailbox-1, got %q", inbox.Token)
	}
}

func TestMoeMailGetCodeReturnsVerificationCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		switch r.URL.Path {
		case "/api/emails/mailbox-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{
						"id":           "msg-1",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
					},
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content": "Your OpenAI verification code is 654321.",
				},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	code, found, err := provider.GetCode(context.Background(), Inbox{
		Email: "native@mail.example.com",
		Token: "mailbox-1",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if !found {
		t.Fatal("expected verification code to be found")
	}
	if code != "654321" {
		t.Fatalf("expected code 654321, got %q", code)
	}
}

func TestMoeMailGetCodeExtractsVerificationCodeFromRawMIME(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		switch r.URL.Path {
		case "/api/emails/mailbox-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{
						"id":           "msg-raw",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
					},
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-raw":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"raw": strings.Join([]string{
						"From: OpenAI <noreply@openai.com>",
						"Subject: =?ISO-8859-1?Q?Votre_code_de_v=E9rification?=",
						"MIME-Version: 1.0",
						"Content-Type: text/html; charset=ISO-8859-1",
						"Content-Transfer-Encoding: quoted-printable",
						"",
						"<html><body>Votre&nbsp;v=E9rification code is <strong>112233</strong>.</body></html>",
					}, "\r\n"),
				},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	code, found, err := provider.GetCode(context.Background(), Inbox{
		Email: "native@mail.example.com",
		Token: "mailbox-1",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if !found {
		t.Fatal("expected verification code from raw MIME to be found")
	}
	if code != "112233" {
		t.Fatalf("expected code 112233, got %q", code)
	}
}

func TestMoeMailWaitCodePollsMessagesUntilCodeArrives(t *testing.T) {
	t.Parallel()

	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		switch r.URL.Path {
		case "/api/emails/mailbox-1":
			listCalls++
			payload := map[string]any{
				"messages": []map[string]any{},
			}
			if listCalls >= 2 {
				payload["messages"] = []map[string]any{
					{
						"id":           "msg-1",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
					},
				}
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content": "Your OpenAI verification code is 246810.",
				},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL:      server.URL,
		APIKey:       "test-api-key",
		PollInterval: 10 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "native@mail.example.com",
		Token: "mailbox-1",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "246810" {
		t.Fatalf("expected code 246810, got %q", code)
	}
	if listCalls < 2 {
		t.Fatalf("expected at least 2 list calls, got %d", listCalls)
	}
}

func TestMoeMailWaitCodeSkipsMessagesBeforeOTPSentAt(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		switch r.URL.Path {
		case "/api/emails/mailbox-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{
						"id":           "msg-old",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
						"created_at":   sentAt.Add(-15 * time.Second).Format(time.RFC3339),
					},
					{
						"id":           "msg-new",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
						"created_at":   sentAt.Add(2 * time.Second).Format(time.RFC3339),
					},
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-old":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content":    "Your OpenAI verification code is 111111.",
					"created_at": sentAt.Add(-15 * time.Second).Format(time.RFC3339),
				},
			}); err != nil {
				t.Fatalf("encode old detail response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-new":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content":    "Your OpenAI verification code is 222222.",
					"created_at": sentAt.Add(2 * time.Second).Format(time.RFC3339),
				},
			}); err != nil {
				t.Fatalf("encode new detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@mail.example.com",
		Token:     "mailbox-1",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with otp sent at: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected newer code 222222, got %q", code)
	}
}

func TestMoeMailWaitCodeResetsDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("expected X-API-Key test-api-key, got %q", got)
		}

		switch r.URL.Path {
		case "/api/emails/mailbox-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{
						"id":           "msg-1",
						"from_address": "noreply@openai.com",
						"subject":      "OpenAI verification code",
					},
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case "/api/emails/mailbox-1/msg-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content": "Your OpenAI verification code is 864209.",
				},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMoeMail(MoeMailConfig{
		BaseURL:      server.URL,
		APIKey:       "test-api-key",
		PollInterval: 5 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@mail.example.com",
		Token:     "mailbox-1",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("first wait code for stage one: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected first stage code 864209, got %q", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = provider.WaitCode(ctx, Inbox{
		Email:     "native@mail.example.com",
		Token:     "mailbox-1",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "native@mail.example.com",
		Token:     "mailbox-1",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected same code to be accepted for a new stage, got %q", code)
	}
}
