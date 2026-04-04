package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresRepositoryUpsertAccountUsesOnConflictAndSerializesExtraData(t *testing.T) {
	registeredAt := time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC)
	createdAt := registeredAt.Add(-time.Hour)
	updatedAt := registeredAt.Add(time.Minute)
	cpaUploadedAt := registeredAt.Add(2 * time.Minute)
	sub2apiUploadedAt := registeredAt.Add(3 * time.Minute)
	db := &fakeAccountsDB{
		row: fakeAccountsRow{
			values: []any{
				9,
				"user@example.com",
				"secret",
				"client-1",
				"session-1",
				"outlook",
				"mailbox-1",
				"account-1",
				"workspace-1",
				"access-1",
				"refresh-1",
				"id-token-1",
				"cookie=value",
				"http://proxy.internal:8080",
				true,
				&cpaUploadedAt,
				true,
				&sub2apiUploadedAt,
				`{"device_id":"dev-1"}`,
				"active",
				"register",
				&registeredAt,
				&createdAt,
				&updatedAt,
			},
		},
	}
	repo := newPostgresRepository(db)

	account, err := repo.UpsertAccount(context.Background(), Account{
		Email:             "user@example.com",
		Password:          "secret",
		ClientID:          "client-1",
		SessionToken:      "session-1",
		EmailService:      "outlook",
		EmailServiceID:    "mailbox-1",
		AccountID:         "account-1",
		WorkspaceID:       "workspace-1",
		AccessToken:       "access-1",
		RefreshToken:      "refresh-1",
		IDToken:           "id-token-1",
		Cookies:           "cookie=value",
		ProxyUsed:         "http://proxy.internal:8080",
		ExtraData:         map[string]any{"device_id": "dev-1"},
		CPAUploaded:       true,
		CPAUploadedAt:     &cpaUploadedAt,
		Sub2APIUploaded:   true,
		Sub2APIUploadedAt: &sub2apiUploadedAt,
		Status:            "active",
		Source:            "register",
		RegisteredAt:      &registeredAt,
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if !strings.Contains(db.queryRowQuery, "INSERT INTO accounts") || !strings.Contains(db.queryRowQuery, "ON CONFLICT (email)") {
		t.Fatalf("expected ON CONFLICT upsert query, got %q", db.queryRowQuery)
	}
	if len(db.queryRowArgs) != 21 {
		t.Fatalf("expected 21 args, got %d", len(db.queryRowArgs))
	}
	if uploaded, ok := db.queryRowArgs[13].(bool); !ok || !uploaded {
		t.Fatalf("expected cpa_uploaded arg true, got %#v", db.queryRowArgs[13])
	}
	if uploaded, ok := db.queryRowArgs[15].(bool); !ok || !uploaded {
		t.Fatalf("expected sub2api_uploaded arg true, got %#v", db.queryRowArgs[15])
	}
	extraDataJSON, ok := db.queryRowArgs[17].(string)
	if !ok {
		t.Fatalf("expected extra_data arg as JSON string, got %#v", db.queryRowArgs[17])
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(extraDataJSON), &decoded); err != nil {
		t.Fatalf("expected valid extra_data json, got %q: %v", extraDataJSON, err)
	}
	if decoded["device_id"] != "dev-1" {
		t.Fatalf("expected device_id in serialized extra_data, got %#v", decoded)
	}
	if account.ID != 9 || account.Email != "user@example.com" || account.EmailService != "outlook" {
		t.Fatalf("unexpected mapped account: %+v", account)
	}
	if account.ExtraData["device_id"] != "dev-1" {
		t.Fatalf("expected extra_data to round-trip, got %#v", account.ExtraData)
	}
	if !account.CPAUploaded || account.CPAUploadedAt == nil || !account.CPAUploadedAt.Equal(cpaUploadedAt) {
		t.Fatalf("expected cpa upload state to round-trip, got %+v", account)
	}
	if !account.Sub2APIUploaded || account.Sub2APIUploadedAt == nil || !account.Sub2APIUploadedAt.Equal(sub2apiUploadedAt) {
		t.Fatalf("expected sub2api upload state to round-trip, got %+v", account)
	}
	if account.RegisteredAt == nil || !account.RegisteredAt.Equal(registeredAt) {
		t.Fatalf("expected registered_at to round-trip, got %#v", account.RegisteredAt)
	}
}

type fakeAccountsDB struct {
	queryRowQuery string
	queryRowArgs  []any
	row           fakeAccountsRow
}

func (f *fakeAccountsDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakeAccountsDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.queryRowQuery = query
	f.queryRowArgs = args
	return f.row
}

type fakeAccountsRow struct {
	values []any
	err    error
}

func (r fakeAccountsRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch out := dest[i].(type) {
		case *int:
			*out = r.values[i].(int)
		case *bool:
			*out = r.values[i].(bool)
		case *string:
			*out = r.values[i].(string)
		case **time.Time:
			*out = r.values[i].(*time.Time)
		default:
			return nil
		}
	}
	return nil
}
