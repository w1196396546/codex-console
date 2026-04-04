package mail

import (
	"fmt"
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
