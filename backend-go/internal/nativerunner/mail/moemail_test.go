package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
