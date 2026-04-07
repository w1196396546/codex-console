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

func TestYYDSMailPollCodeExtractsVerificationCodeFromRFC822Detail(t *testing.T) {
	t.Parallel()

	var detailCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			if got := r.URL.Query().Get("address"); got != "native@public.example.com" {
				t.Fatalf("expected address native@public.example.com, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"messages": []map[string]any{
						{
							"id": "msg-rfc822",
							"from": map[string]any{
								"name":    "OpenAI",
								"address": "noreply@openai.com",
							},
							"subject": "OpenAI verification code",
							"snippet": "Open the detail to inspect this email.",
						},
					},
					"total": 1,
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-rfc822":
			detailCalls++
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": "msg-rfc822",
					"rfc822": strings.Join([]string{
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

	provider := NewYYDSMail(YYDSMailConfig{
		BaseURL: server.URL,
	})

	code, found, err := provider.pollCode(context.Background(), Inbox{
		Email: "native@public.example.com",
		Token: "temp-token-1",
	}, DefaultCodePattern, map[string]struct{}{})
	if err != nil {
		t.Fatalf("poll code: %v", err)
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

func TestYYDSMailWaitCodeSkipsMessagesBeforeOTPSentAt(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"messages": []map[string]any{
						{
							"id":          "msg-older",
							"received_at": sentAt.Add(-15 * time.Second).Format(time.RFC3339),
							"from": map[string]any{
								"name":    "OpenAI",
								"address": "noreply@openai.com",
							},
							"subject": "Your verification code",
						},
						{
							"id":          "msg-fresh",
							"received_at": sentAt.Add(2 * time.Second).Format(time.RFC3339),
							"from": map[string]any{
								"name":    "OpenAI",
								"address": "noreply@openai.com",
							},
							"subject": "Your verification code",
						},
					},
					"total": 2,
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-older":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":      "msg-older",
					"subject": "Your verification code",
					"text":    "Your OpenAI verification code is 111111",
				},
			}); err != nil {
				t.Fatalf("encode older detail response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-fresh":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":      "msg-fresh",
					"subject": "Your verification code",
					"text":    "Your OpenAI verification code is 222222",
				},
			}); err != nil {
				t.Fatalf("encode fresh detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewYYDSMail(YYDSMailConfig{
		BaseURL: server.URL,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@public.example.com",
		Token:     "temp-token-1",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code with otp sent at: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected newer code 222222, got %q", code)
	}
}

func TestYYDSMailWaitCodeResetsFingerprintDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
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
				},
			}); err != nil {
				t.Fatalf("encode list response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-1":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":      "msg-1",
					"subject": "Your verification code",
					"text":    "Your OpenAI verification code is 864209",
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
		PollInterval: 5 * time.Millisecond,
	})

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "native@public.example.com",
		Token:     "temp-token-1",
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
		Email:     "native@public.example.com",
		Token:     "temp-token-1",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "native@public.example.com",
		Token:     "temp-token-1",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected same code to be accepted for a new stage, got %q", code)
	}
}
