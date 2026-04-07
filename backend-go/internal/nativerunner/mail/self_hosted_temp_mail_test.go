package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSelfHostedTempMailCreateAndWaitCode(t *testing.T) {
	t.Parallel()

	mailRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/new_address":
			if got := r.Header.Get("x-admin-auth"); got != "admin-secret" {
				t.Fatalf("expected x-admin-auth header, got %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			if payload["domain"] != "mail.example.com" {
				t.Fatalf("expected domain mail.example.com, got %#v", payload["domain"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"address": "openai@mail.example.com",
				"jwt":     "user-jwt",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/mails":
			mailRequests++
			if got := r.Header.Get("Authorization"); got != "Bearer user-jwt" {
				t.Fatalf("expected bearer jwt, got %q", got)
			}
			if mailRequests == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"mails": []map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mails": []map[string]any{
					{
						"id":      "mail-1",
						"from":    "OpenAI <noreply@openai.com>",
						"subject": "Your verification code",
						"text":    "Your OpenAI verification code is 123456",
						"date":    time.Now().UTC().Format(time.RFC3339),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewSelfHostedTempMail(SelfHostedTempMailConfig{
		BaseURL:       server.URL,
		AdminPassword: "admin-secret",
		Domain:        "mail.example.com",
		EnablePrefix:  true,
		PollInterval:  5 * time.Millisecond,
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "openai@mail.example.com" || inbox.Token != "user-jwt" {
		t.Fatalf("unexpected inbox: %+v", inbox)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	code, err := provider.WaitCode(ctx, inbox, nil)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("expected code 123456, got %q", code)
	}
}
