package accounts

import (
	"testing"
	"time"
)

func TestUpsertAccountRequestNormalizedAppliesDefaultsAndClonesExtraData(t *testing.T) {
	timestamp := time.Date(2026, 4, 4, 12, 30, 0, 0, time.UTC)
	originalExtra := map[string]any{
		"device_id": "dev-1",
	}

	normalized, err := (UpsertAccountRequest{
		Email:        "  user@example.com  ",
		ExtraData:    originalExtra,
		ProxyUsed:    "  http://proxy.internal:8080  ",
		ClientID:     "  client-1 ",
		EmailService: "  outlook ",
	}).Normalized(timestamp)
	if err != nil {
		t.Fatalf("unexpected normalize error: %v", err)
	}

	if normalized.Email != "user@example.com" {
		t.Fatalf("expected trimmed email, got %q", normalized.Email)
	}
	if normalized.Status != DefaultAccountStatus {
		t.Fatalf("expected default status %q, got %q", DefaultAccountStatus, normalized.Status)
	}
	if normalized.Source != DefaultAccountSource {
		t.Fatalf("expected default source %q, got %q", DefaultAccountSource, normalized.Source)
	}
	if normalized.RegisteredAt == nil || !normalized.RegisteredAt.Equal(timestamp) {
		t.Fatalf("expected registered_at=%v, got %#v", timestamp, normalized.RegisteredAt)
	}
	if normalized.EmailService != "outlook" {
		t.Fatalf("expected trimmed email_service, got %q", normalized.EmailService)
	}
	if normalized.ClientID != "client-1" {
		t.Fatalf("expected trimmed client_id, got %q", normalized.ClientID)
	}
	if normalized.ProxyUsed != "http://proxy.internal:8080" {
		t.Fatalf("expected trimmed proxy_used, got %q", normalized.ProxyUsed)
	}
	if normalized.ExtraData["device_id"] != "dev-1" {
		t.Fatalf("expected copied extra_data, got %#v", normalized.ExtraData)
	}

	originalExtra["device_id"] = "mutated"
	if normalized.ExtraData["device_id"] != "dev-1" {
		t.Fatalf("expected normalized extra_data to be detached copy, got %#v", normalized.ExtraData)
	}
}

func TestUpsertAccountRequestNormalizedRejectsEmptyEmail(t *testing.T) {
	_, err := (UpsertAccountRequest{}).Normalized(time.Now().UTC())
	if err == nil {
		t.Fatal("expected empty email to fail normalization")
	}
}
