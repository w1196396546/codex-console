package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestYYDSMailCreateReturnsInbox(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/accounts" {
			t.Fatalf("expected /accounts, got %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "AC-test-key" {
			t.Fatalf("expected X-API-Key AC-test-key, got %q", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["domain"] != "public.example.com" {
			t.Fatalf("expected domain public.example.com, got %#v", payload["domain"])
		}
		if payload["address"] == "" {
			t.Fatal("expected create address to be non-empty")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":      "inbox-1",
				"address": "native@public.example.com",
				"token":   "temp-token-1",
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewYYDSMail(YYDSMailConfig{
		BaseURL:       server.URL,
		APIKey:        "AC-test-key",
		DefaultDomain: "public.example.com",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@public.example.com" {
		t.Fatalf("expected email native@public.example.com, got %q", inbox.Email)
	}
	if inbox.Token != "temp-token-1" {
		t.Fatalf("expected token temp-token-1, got %q", inbox.Token)
	}
}

func TestYYDSMailWaitCodePollsMessagesUntilCodeArrives(t *testing.T) {
	t.Parallel()

	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			listCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer temp-token-1" {
				t.Fatalf("expected bearer token, got %q", got)
			}
			if got := r.URL.Query().Get("address"); got != "native@public.example.com" {
				t.Fatalf("expected address native@public.example.com, got %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "50" {
				t.Fatalf("expected limit 50, got %q", got)
			}

			payload := map[string]any{
				"success": true,
				"data": map[string]any{
					"messages": []map[string]any{},
					"total":    0,
				},
			}
			if listCalls >= 2 {
				payload["data"] = map[string]any{
					"messages": []map[string]any{
						{
							"id": "msg-1",
							"from": map[string]any{
								"name":    "OpenAI",
								"address": "noreply@openai.com",
							},
							"subject": "Your verification code",
						},
					},
					"total": 1,
				}
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-1":
			if got := r.Header.Get("Authorization"); got != "Bearer temp-token-1" {
				t.Fatalf("expected bearer token, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": "msg-1",
					"from": map[string]any{
						"name":    "OpenAI",
						"address": "noreply@openai.com",
					},
					"subject": "Your verification code",
					"text":    "Your OpenAI verification code is 654321",
				},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewYYDSMail(YYDSMailConfig{
		BaseURL:      server.URL,
		APIKey:       "AC-test-key",
		PollInterval: 10 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "native@public.example.com",
		Token: "temp-token-1",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected code 654321, got %q", code)
	}
	if listCalls < 2 {
		t.Fatalf("expected at least 2 list calls, got %d", listCalls)
	}
}
