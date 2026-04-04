package uploader

import "testing"

func TestUploadKindAndServiceConfigDefaults(t *testing.T) {
	cfg := ServiceConfig{
		Kind:       UploadKindSub2API,
		BaseURL:    " https://sub2api.example.com/ ",
		Credential: " secret-key ",
	}

	normalized := cfg.Normalized()
	if !normalized.Kind.Valid() {
		t.Fatalf("expected upload kind %q to be valid", normalized.Kind)
	}
	if normalized.BaseURL != "https://sub2api.example.com/" {
		t.Fatalf("expected trimmed base url, got %q", normalized.BaseURL)
	}
	if normalized.Credential != "secret-key" {
		t.Fatalf("expected trimmed credential, got %q", normalized.Credential)
	}
	if normalized.TargetType != DefaultSub2APITargetType {
		t.Fatalf("expected default target type %q, got %q", DefaultSub2APITargetType, normalized.TargetType)
	}
}

func TestUploadAccountNormalizedTrimsCommonFields(t *testing.T) {
	account := UploadAccount{
		Email:        " user@example.com ",
		AccessToken:  " access ",
		RefreshToken: " refresh ",
		SessionToken: " session ",
		ClientID:     " client ",
		AccountID:    " account ",
		WorkspaceID:  " workspace ",
	}

	normalized := account.Normalized()
	if normalized.Email != "user@example.com" {
		t.Fatalf("expected trimmed email, got %q", normalized.Email)
	}
	if normalized.AccessToken != "access" || normalized.RefreshToken != "refresh" || normalized.SessionToken != "session" {
		t.Fatalf("expected token fields to be trimmed, got %+v", normalized)
	}
	if normalized.ClientID != "client" || normalized.AccountID != "account" || normalized.WorkspaceID != "workspace" {
		t.Fatalf("expected account identifiers to be trimmed, got %+v", normalized)
	}
}

func TestUploadKindValidRejectsUnknownValue(t *testing.T) {
	if UploadKind("unknown").Valid() {
		t.Fatal("expected unknown upload kind to be invalid")
	}
}
