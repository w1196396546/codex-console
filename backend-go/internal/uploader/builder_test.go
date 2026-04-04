package uploader

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildCPAAuthFile(t *testing.T) {
	expiresAt := time.Date(2026, 4, 4, 2, 3, 4, 0, time.UTC)
	lastRefresh := time.Date(2026, 4, 3, 16, 5, 6, 0, time.UTC)
	file, err := BuildCPAAuthFile(UploadAccount{
		Email:        "user@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		IDToken:      "id-1",
		AccountID:    "account-1",
		ExpiresAt:    &expiresAt,
		LastRefresh:  &lastRefresh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if file.Filename != "user@example.com.json" {
		t.Fatalf("expected filename to use email, got %q", file.Filename)
	}

	var payload map[string]any
	if err := json.Unmarshal(file.Content, &payload); err != nil {
		t.Fatalf("expected valid cpa json, got %v", err)
	}
	if payload["type"] != "codex" || payload["email"] != "user@example.com" {
		t.Fatalf("unexpected cpa payload: %#v", payload)
	}
	if payload["access_token"] != "access-1" || payload["refresh_token"] != "refresh-1" {
		t.Fatalf("expected token fields in cpa payload, got %#v", payload)
	}
	if payload["expired"] != "2026-04-04T10:03:04+08:00" {
		t.Fatalf("unexpected expired timestamp: %#v", payload["expired"])
	}
}

func TestBuildSub2APIBatchPayloadUsesDefaultsAndSkipsAccountsWithoutAccessToken(t *testing.T) {
	expiresAt := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	payload, err := BuildSub2APIBatchPayload(ServiceConfig{
		Kind:       UploadKindSub2API,
		TargetType: "newapi",
	}, []UploadAccount{
		{
			Email:        "user@example.com",
			AccessToken:  "access-1",
			RefreshToken: "refresh-1",
			ClientID:     "client-1",
			AccountID:    "account-1",
			WorkspaceID:  "workspace-1",
			ExpiresAt:    &expiresAt,
		},
		{
			Email: "skip@example.com",
		},
	}, Sub2APIBatchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.Data.Type != "newapi-data" {
		t.Fatalf("expected newapi payload type, got %q", payload.Data.Type)
	}
	if payload.SkipDefaultGroupBind != true {
		t.Fatalf("expected skip_default_group_bind=true, got %#v", payload.SkipDefaultGroupBind)
	}
	if len(payload.Data.Accounts) != 1 {
		t.Fatalf("expected one account, got %d", len(payload.Data.Accounts))
	}
	account := payload.Data.Accounts[0]
	if account.Name != "user@example.com" || account.Concurrency != DefaultSub2APIConcurrency || account.Priority != DefaultSub2APIPriority {
		t.Fatalf("unexpected sub2api account payload: %+v", account)
	}
	if account.Credentials.AccessToken != "access-1" || account.Credentials.OrganizationID != "workspace-1" {
		t.Fatalf("unexpected sub2api credentials: %+v", account.Credentials)
	}
}

func TestBuildTMSingleAndBatchPayload(t *testing.T) {
	account := UploadAccount{
		Email:        "user@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		SessionToken: "session-1",
		ClientID:     "client-1",
		AccountID:    "account-1",
	}

	single, err := BuildTMSinglePayload(account)
	if err != nil {
		t.Fatalf("unexpected single payload error: %v", err)
	}
	if single.ImportType != "single" || single.Email != "user@example.com" || single.AccessToken != "access-1" {
		t.Fatalf("unexpected tm single payload: %+v", single)
	}

	batch, err := BuildTMBatchPayload([]UploadAccount{
		account,
		{
			Email: "other@example.com",
		},
	})
	if err != nil {
		t.Fatalf("unexpected batch payload error: %v", err)
	}
	if batch.ImportType != "batch" {
		t.Fatalf("expected batch import type, got %q", batch.ImportType)
	}
	if batch.Content != "user@example.com,access-1,refresh-1,session-1,client-1" {
		t.Fatalf("unexpected tm batch content: %q", batch.Content)
	}
}
