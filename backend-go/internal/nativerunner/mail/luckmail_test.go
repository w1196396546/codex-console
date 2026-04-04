package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLuckMailCreateReturnsInbox(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/email/purchase" {
			t.Fatalf("expected /api/v1/email/purchase, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("expected bearer api key, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["project_code"] != "openai" {
			t.Fatalf("expected project_code openai, got %#v", payload["project_code"])
		}
		if payload["email_type"] != "ms_graph" {
			t.Fatalf("expected email_type ms_graph, got %#v", payload["email_type"])
		}
		if payload["domain"] != "luck.example.com" {
			t.Fatalf("expected domain luck.example.com, got %#v", payload["domain"])
		}
		if payload["quantity"] != float64(1) {
			t.Fatalf("expected quantity 1, got %#v", payload["quantity"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []map[string]any{
					{
						"id":            101,
						"email_address": "native@luck.example.com",
						"token":         "tok-123",
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewLuckMail(LuckMailConfig{
		BaseURL:         server.URL,
		APIKey:          "test-api-key",
		ProjectCode:     "openai",
		EmailType:       "ms_graph",
		PreferredDomain: "luck.example.com",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@luck.example.com" {
		t.Fatalf("expected email native@luck.example.com, got %q", inbox.Email)
	}
	if inbox.Token != "tok-123" {
		t.Fatalf("expected token tok-123, got %q", inbox.Token)
	}
}

func TestLuckMailGetCodeReturnsVerificationCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/email/query/tok-123" {
			t.Fatalf("expected /api/v1/email/query/tok-123, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("expected bearer api key, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"has_new_mail":      true,
				"verification_code": "654321",
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewLuckMail(LuckMailConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	code, found, err := provider.GetCode(context.Background(), Inbox{
		Email: "native@luck.example.com",
		Token: "tok-123",
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

func TestLuckMailWaitCodePollsUntilCodeArrives(t *testing.T) {
	t.Parallel()

	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/email/query/tok-123" {
			t.Fatalf("expected /api/v1/email/query/tok-123, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("expected bearer api key, got %q", got)
		}

		pollCount++
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"code": 0,
			"data": map[string]any{
				"has_new_mail": false,
			},
		}
		if pollCount >= 2 {
			payload["data"] = map[string]any{
				"has_new_mail":      true,
				"verification_code": "246810",
			}
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := NewLuckMail(LuckMailConfig{
		BaseURL:      server.URL,
		APIKey:       "test-api-key",
		PollInterval: 10 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email: "native@luck.example.com",
		Token: "tok-123",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "246810" {
		t.Fatalf("expected code 246810, got %q", code)
	}
	if pollCount < 2 {
		t.Fatalf("expected at least 2 polls, got %d", pollCount)
	}
}
