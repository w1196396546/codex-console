package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewProviderReturnsTempmailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("tempmail", map[string]any{
		"base_url": "https://tempmail.example",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*Tempmail); !ok {
		t.Fatalf("expected *Tempmail provider, got %T", provider)
	}
}

func TestNewProviderReturnsSelfHostedTempMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("temp_mail", map[string]any{
		"base_url":       "https://mail.example.com",
		"admin_password": "admin-secret",
		"domain":         "mail.example.com",
		"enable_prefix":  true,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if got := typeName(provider); got != "*mail.SelfHostedTempMail" {
		t.Fatalf("expected *mail.SelfHostedTempMail provider, got %s", got)
	}
}

func TestNewProviderReturnsFreemailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("freemail", map[string]any{
		"base_url":    "https://freemail.example",
		"admin_token": "admin-token",
		"domain":      "mail.example.com",
		"local_part":  "openai",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*Freemail); !ok {
		t.Fatalf("expected *Freemail provider, got %T", provider)
	}
}

func TestNewProviderReturnsYYDSMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("yyds_mail", map[string]any{
		"base_url": "https://maliapi.215.im/v1",
		"api_key":  "AC-test-key",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*YYDSMail); !ok {
		t.Fatalf("expected *YYDSMail provider, got %T", provider)
	}
}

func TestNewProviderReturnsDuckMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("duckmail", map[string]any{
		"base_url":       "https://duckmail.example",
		"default_domain": "duckmail.sbs",
		"api_key":        "duck-key",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*DuckMail); !ok {
		t.Fatalf("expected *DuckMail provider, got %T", provider)
	}
}

func TestNewProviderReturnsLuckMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("luckmail", map[string]any{
		"base_url":         "https://mails.luckyous.com",
		"api_key":          "test-api-key",
		"project_code":     "openai",
		"email_type":       "ms_graph",
		"preferred_domain": "luck.example.com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*LuckMail); !ok {
		t.Fatalf("expected *LuckMail provider, got %T", provider)
	}
}

func TestNewProviderReturnsIMAPMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("imap_mail", map[string]any{
		"host":     "imap.example.com",
		"email":    "native@example.com",
		"password": "secret",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*IMAPMail); !ok {
		t.Fatalf("expected *IMAPMail provider, got %T", provider)
	}
}

func TestNewProviderReturnsOutlookProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("outlook", map[string]any{
		"email":    "native@example.com",
		"password": "secret-password",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := provider.(*Outlook); !ok {
		t.Fatalf("expected *Outlook provider, got %T", provider)
	}
}

func TestNewProviderReturnsOutlookProviderWithOAuthConfig(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("outlook", map[string]any{
		"email":         "native@example.com",
		"client_id":     "client-123",
		"refresh_token": "refresh-123",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	outlook, ok := provider.(*Outlook)
	if !ok {
		t.Fatalf("expected *Outlook provider, got %T", provider)
	}
	if !outlook.hasOAuth() {
		t.Fatal("expected oauth config to be preserved on outlook provider")
	}

	inbox, err := outlook.Create(context.Background())
	if err != nil {
		t.Fatalf("create oauth outlook inbox: %v", err)
	}
	if inbox.Token != "" {
		t.Fatalf("expected oauth-only inbox token to stay empty, got %q", inbox.Token)
	}
}

func TestNewProviderReturnsMoeMailProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider("moe_mail", map[string]any{
		"base_url":       "https://mail.example.com",
		"api_key":        "test-api-key",
		"default_domain": "mail.example.com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if got := typeName(provider); got != "*mail.MoeMail" {
		t.Fatalf("expected *mail.MoeMail provider, got %s", got)
	}
}

func TestNewHTTPClientRoutesRequestsThroughProxyURL(t *testing.T) {
	t.Parallel()

	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer target.Close()

	proxyRequests := make(chan *http.Request, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case proxyRequests <- r.Clone(context.Background()):
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()

	client := newHTTPClient(map[string]any{
		"proxy_url": proxy.URL,
	})
	if client == nil {
		t.Fatal("expected proxied http client")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL+"/mailbox", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected proxy response status 204, got %d", resp.StatusCode)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("expected target server to be bypassed by proxy, got %d hits", targetHits.Load())
	}

	select {
	case proxyReq := <-proxyRequests:
		if proxyReq.Method != http.MethodGet {
			t.Fatalf("expected proxied GET request, got %s", proxyReq.Method)
		}
		if proxyReq.URL.String() != target.URL+"/mailbox" {
			t.Fatalf("expected proxy to receive absolute target url, got %q", proxyReq.URL.String())
		}
	default:
		t.Fatal("expected proxy server to receive request")
	}
}

func TestNewProviderReturnsUnsupportedError(t *testing.T) {
	t.Parallel()

	_, err := NewProvider("unknown", nil)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if err.Error() != "unsupported native mail provider: unknown" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func typeName(value any) string {
	return fmt.Sprintf("%T", value)
}
