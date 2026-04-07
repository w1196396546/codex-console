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

func TestDuckMailWaitCodeDeduplicatesFingerprintsAcrossCalls(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
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
		t.Fatalf("first wait code: %v", err)
	}
	if code != "654321" {
		t.Fatalf("expected first code 654321, got %q", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = provider.WaitCode(ctx, Inbox{
		Email: "tester@duckmail.sbs",
		Token: "token-123",
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected duplicate code to be ignored until context deadline, got %v", err)
	}
}

func TestDuckMailWaitCodeResetsDedupeForNewOTPStage(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2025, 3, 3, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
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
				"text": "Use 864209 to verify your OpenAI account",
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
		"poll_interval":  1,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	code, err := provider.WaitCode(context.Background(), Inbox{
		Email:     "tester@duckmail.sbs",
		Token:     "token-123",
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
		Email:     "tester@duckmail.sbs",
		Token:     "token-123",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected repeated stage to skip duplicate, got %v", err)
	}

	code, err = provider.WaitCode(context.Background(), Inbox{
		Email:     "tester@duckmail.sbs",
		Token:     "token-123",
		OTPSentAt: sentAt.Add(10 * time.Second),
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code for new otp stage: %v", err)
	}
	if code != "864209" {
		t.Fatalf("expected same code to be accepted for a new stage, got %q", code)
	}
}

func TestDuckMailWaitCodePrefersTimestampedFreshRawMessageOverUnknownTimestampCandidate(t *testing.T) {
	t.Parallel()

	sentAt := time.Now().UTC()
	var pollCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			pollCount++
			messages := []map[string]any{
				{
					"id": "msg-old",
					"from": map[string]any{
						"name":    "OpenAI",
						"address": "noreply@openai.com",
					},
					"subject": "Your verification code",
				},
			}
			if pollCount >= 2 {
				messages = append(messages, map[string]any{
					"id":        "msg-fresh",
					"createdAt": sentAt.Add(2 * time.Second).UnixMilli(),
					"from": map[string]any{
						"name":    "OpenAI",
						"address": "noreply@openai.com",
					},
					"subject": "OpenAI sign-in",
				})
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"hydra:member": messages,
			}); err != nil {
				t.Fatalf("encode messages response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-old":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":   "msg-old",
				"text": "Use 111111 to verify your OpenAI account",
			}); err != nil {
				t.Fatalf("encode old detail response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-fresh":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id": "msg-fresh",
				"raw": strings.Join([]string{
					"From: OpenAI <noreply@openai.com>",
					"Subject: =?ISO-8859-1?Q?Votre_code_de_v=E9rification?=",
					"MIME-Version: 1.0",
					"Content-Type: text/html; charset=ISO-8859-1",
					"Content-Transfer-Encoding: quoted-printable",
					"",
					"<html><body>Votre&nbsp;v=E9rification code is <strong>222222</strong>.</body></html>",
				}, "\r\n"),
			}); err != nil {
				t.Fatalf("encode fresh detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewDuckMail(DuckMailConfig{
		BaseURL:       server.URL,
		DefaultDomain: "duckmail.sbs",
		PollInterval:  10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	code, err := provider.WaitCode(ctx, Inbox{
		Email:     "tester@duckmail.sbs",
		Token:     "token-123",
		OTPSentAt: sentAt,
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "222222" {
		t.Fatalf("expected fresh code 222222, got %q", code)
	}
	if pollCount < 2 {
		t.Fatalf("expected at least two polls before returning code, got %d", pollCount)
	}
}

func TestDuckMailPollCodeExtractsVerificationCodeFromRFC822Detail(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"hydra:member": []map[string]any{
					{
						"id": "msg-rfc822",
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
		case r.Method == http.MethodGet && r.URL.Path == "/messages/msg-rfc822":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id": "msg-rfc822",
				"rfc822": strings.Join([]string{
					"From: OpenAI <noreply@openai.com>",
					"Subject: Your verification code",
					"MIME-Version: 1.0",
					"Content-Type: text/plain; charset=UTF-8",
					"",
					"Use 246810 to verify your OpenAI account.",
				}, "\r\n"),
			}); err != nil {
				t.Fatalf("encode detail response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewDuckMail(DuckMailConfig{
		BaseURL:       server.URL,
		DefaultDomain: "duckmail.sbs",
	})

	code, found, err := provider.pollCode(context.Background(), Inbox{
		Email: "tester@duckmail.sbs",
		Token: "token-123",
	}, DefaultCodePattern)
	if err != nil {
		t.Fatalf("poll code: %v", err)
	}
	if !found {
		t.Fatal("expected verification code from rfc822 detail to be found")
	}
	if code != "246810" {
		t.Fatalf("expected code 246810, got %q", code)
	}
}
