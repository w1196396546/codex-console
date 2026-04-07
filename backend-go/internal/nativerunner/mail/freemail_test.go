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

func TestFreemailCreateUsesGenerateEndpoint(t *testing.T) {
	t.Parallel()

	var domainsCalls int
	var generateCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token header, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/domains":
			domainsCalls++
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]string{"default.example", "chosen.example"}); err != nil {
				t.Fatalf("encode domains response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/generate":
			generateCalls++
			if got := r.URL.Query().Get("domainIndex"); got != "1" {
				t.Fatalf("expected domainIndex=1, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"email": "native@chosen.example",
			}); err != nil {
				t.Fatalf("encode generate response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:    server.URL,
		AdminToken: "token-123",
		Domain:     "chosen.example",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "native@chosen.example" {
		t.Fatalf("expected generated email, got %q", inbox.Email)
	}
	if domainsCalls != 1 {
		t.Fatalf("expected one domains call, got %d", domainsCalls)
	}
	if generateCalls != 1 {
		t.Fatalf("expected one generate call, got %d", generateCalls)
	}
}

func TestFreemailCreateUsesCreateEndpointWhenLocalPartConfigured(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/domains":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]string{"default.example", "chosen.example"}); err != nil {
				t.Fatalf("encode domains response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/create":
			var payload struct {
				Local       string `json:"local"`
				DomainIndex int    `json:"domainIndex"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if payload.Local != "alice" {
				t.Fatalf("expected local part alice, got %q", payload.Local)
			}
			if payload.DomainIndex != 1 {
				t.Fatalf("expected domainIndex=1, got %d", payload.DomainIndex)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"email": "alice@chosen.example",
			}); err != nil {
				t.Fatalf("encode create response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:    server.URL,
		AdminToken: "token-123",
		Domain:     "chosen.example",
		LocalPart:  "alice",
	})

	inbox, err := provider.Create(context.Background())
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if inbox.Email != "alice@chosen.example" {
		t.Fatalf("expected created email, got %q", inbox.Email)
	}
}

func TestFreemailGetCodeFallsBackToEmailDetail(t *testing.T) {
	t.Parallel()

	var detailCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/emails":
			if got := r.URL.Query().Get("mailbox"); got != "native@chosen.example" {
				t.Fatalf("expected mailbox query, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":      "mail-1",
					"sender":  "OpenAI <noreply@openai.com>",
					"subject": "Security check",
					"preview": "No direct code here",
				},
			}); err != nil {
				t.Fatalf("encode emails response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/email/mail-1":
			detailCalls++
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"content": "Your verification code is 654321.",
			}); err != nil {
				t.Fatalf("encode email detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:    server.URL,
		AdminToken: "token-123",
	})

	code, found, err := provider.GetCode(context.Background(), Inbox{Email: "native@chosen.example"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if !found {
		t.Fatal("expected verification code to be found")
	}
	if code != "654321" {
		t.Fatalf("expected code 654321, got %q", code)
	}
	if detailCalls != 1 {
		t.Fatalf("expected one detail call, got %d", detailCalls)
	}
}

func TestFreemailGetCodeExtractsVerificationCodeFromRFC822Detail(t *testing.T) {
	t.Parallel()

	var detailCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/emails":
			if got := r.URL.Query().Get("mailbox"); got != "native@chosen.example" {
				t.Fatalf("expected mailbox query, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":      "mail-rfc822",
					"sender":  "Mailbox Robot <robot@example.com>",
					"subject": "New unread message",
					"preview": "Open the detail to inspect this email.",
				},
			}); err != nil {
				t.Fatalf("encode emails response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/email/mail-rfc822":
			detailCalls++
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"content":      "",
				"html_content": "",
				"rfc822": strings.Join([]string{
					"From: OpenAI <noreply@openai.com>",
					"Subject: =?ISO-8859-1?Q?Votre_code_de_v=E9rification?=",
					"MIME-Version: 1.0",
					"Content-Type: text/html; charset=ISO-8859-1",
					"Content-Transfer-Encoding: quoted-printable",
					"",
					"<html><body>Votre&nbsp;v=E9rification code is <strong>112233</strong>.</body></html>",
				}, "\r\n"),
			}); err != nil {
				t.Fatalf("encode email detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:    server.URL,
		AdminToken: "token-123",
	})

	code, found, err := provider.GetCode(context.Background(), Inbox{Email: "native@chosen.example"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if !found {
		t.Fatal("expected verification code from rfc822 detail to be found")
	}
	if code != "112233" {
		t.Fatalf("expected code 112233, got %q", code)
	}
	if detailCalls != 1 {
		t.Fatalf("expected one detail call, got %d", detailCalls)
	}
}

func TestFreemailWaitCodePollsUntilVerificationCodeArrives(t *testing.T) {
	t.Parallel()

	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/emails":
			pollCount++
			w.Header().Set("Content-Type", "application/json")
			payload := []map[string]any{}
			if pollCount >= 2 {
				payload = []map[string]any{
					{
						"id":                "mail-2",
						"sender":            "OpenAI <noreply@openai.com>",
						"subject":           "OpenAI sign-in",
						"verification_code": "123456",
					},
				}
			}
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode emails response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:      server.URL,
		AdminToken:   "token-123",
		PollInterval: 10 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{Email: "native@chosen.example"}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("expected code 123456, got %q", code)
	}
	if pollCount < 2 {
		t.Fatalf("expected at least two polls, got %d", pollCount)
	}
}

func TestFreemailWaitCodeSkipsMessagesBeforeOTPSentAt(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/emails":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":                "mail-older",
					"sender":            "OpenAI <noreply@openai.com>",
					"subject":           "OpenAI sign-in",
					"verification_code": "111111",
					"received_at":       sentAt.Add(-15 * time.Second).Format(time.RFC3339),
				},
				{
					"id":                "mail-fresh",
					"sender":            "OpenAI <noreply@openai.com>",
					"subject":           "OpenAI sign-in",
					"verification_code": "222222",
					"received_at":       sentAt.Add(2 * time.Second).Format(time.RFC3339),
				},
			}); err != nil {
				t.Fatalf("encode emails response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:    server.URL,
		AdminToken: "token-123",
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@chosen.example",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with otp sent at: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected newer code 222222, got %q", code)
	}
}

func TestFreemailWaitCodeResetsFingerprintDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/emails":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":                "mail-1",
					"sender":            "OpenAI <noreply@openai.com>",
					"subject":           "OpenAI sign-in",
					"verification_code": "864209",
				},
			}); err != nil {
				t.Fatalf("encode emails response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewFreemail(FreemailConfig{
		BaseURL:      server.URL,
		AdminToken:   "token-123",
		PollInterval: 5 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@chosen.example",
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
		Email:     "native@chosen.example",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "native@chosen.example",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected same code to be accepted for a new stage, got %q", code)
	}
}
