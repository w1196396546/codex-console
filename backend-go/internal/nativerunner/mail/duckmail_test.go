package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDuckMailCreateReturnsInbox(t *testing.T) {
	t.Parallel()

	var createdPassword string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/accounts":
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected Accept application/json, got %q", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("expected Content-Type application/json, got %q", got)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}

			address, _ := payload["address"].(string)
			if !strings.HasSuffix(address, "@duckmail.sbs") {
				t.Fatalf("expected duckmail domain, got %q", address)
			}

			createdPassword, _ = payload["password"].(string)
			if strings.TrimSpace(createdPassword) == "" {
				t.Fatal("expected generated password")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":      "account-1",
				"address": "tester@duckmail.sbs",
			}); err != nil {
				t.Fatalf("encode account response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode token payload: %v", err)
			}

			if got, _ := payload["address"].(string); got != "tester@duckmail.sbs" {
				t.Fatalf("expected token request address tester@duckmail.sbs, got %q", got)
			}
			if got, _ := payload["password"].(string); got != createdPassword {
				t.Fatalf("expected token request password %q, got %q", createdPassword, got)
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":    "account-1",
				"token": "token-123",
			}); err != nil {
				t.Fatalf("encode token response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewProvider("duckmail", map[string]any{
		"base_url":       server.URL,
		"default_domain": "duckmail.sbs",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "tester@duckmail.sbs" {
		t.Fatalf("expected email tester@duckmail.sbs, got %q", inbox.Email)
	}
	if inbox.Token != "token-123" {
		t.Fatalf("expected token token-123, got %q", inbox.Token)
	}
}

func TestDuckMailWaitCodeReadsMessageDetailAndExtractsCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("expected page=1, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"hydra:member": []map[string]any{
					{
						"id": "msg-1",
						"from": map[string]any{
							"name":    "OpenAI",
							"address": "noreply@openai.com",
						},
						"subject": "Your verification code",
					},
				},
			}); err != nil {
				t.Fatalf("encode messages response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":   "msg-1",
				"text": "Use 654321 to verify your OpenAI account",
				"html": []string{},
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewProvider("duckmail", map[string]any{
		"base_url":       server.URL,
		"default_domain": "duckmail.sbs",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "tester@duckmail.sbs",
		Token: "token-123",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected code 654321, got %q", code)
	}
}
