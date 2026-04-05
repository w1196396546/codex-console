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
	lastRefresh := registeredAt.Add(4 * time.Minute)
	expiresAt := registeredAt.Add(64 * time.Minute)
	subscriptionAt := registeredAt.Add(5 * time.Minute)
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
				&lastRefresh,
				&expiresAt,
				`{"device_id":"dev-1"}`,
				"active",
				"register",
				"team",
				&subscriptionAt,
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
		LastRefresh:       &lastRefresh,
		ExpiresAt:         &expiresAt,
		Status:            "active",
		Source:            "register",
		SubscriptionType:  "team",
		SubscriptionAt:    &subscriptionAt,
		RegisteredAt:      &registeredAt,
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if !strings.Contains(db.queryRowQuery, "INSERT INTO accounts") || !strings.Contains(db.queryRowQuery, "ON CONFLICT (email)") {
		t.Fatalf("expected ON CONFLICT upsert query, got %q", db.queryRowQuery)
	}
	if len(db.queryRowArgs) != 25 {
		t.Fatalf("expected 25 args, got %d", len(db.queryRowArgs))
	}
	if uploaded, ok := db.queryRowArgs[13].(bool); !ok || !uploaded {
		t.Fatalf("expected cpa_uploaded arg true, got %#v", db.queryRowArgs[13])
	}
	if uploaded, ok := db.queryRowArgs[15].(bool); !ok || !uploaded {
		t.Fatalf("expected sub2api_uploaded arg true, got %#v", db.queryRowArgs[15])
	}
	extraDataJSON, ok := db.queryRowArgs[19].(string)
	if !ok {
		t.Fatalf("expected extra_data arg as JSON string, got %#v", db.queryRowArgs[19])
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
	if account.LastRefresh == nil || !account.LastRefresh.Equal(lastRefresh) {
		t.Fatalf("expected last_refresh to round-trip, got %#v", account.LastRefresh)
	}
	if account.ExpiresAt == nil || !account.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at to round-trip, got %#v", account.ExpiresAt)
	}
	if account.SubscriptionType != "team" {
		t.Fatalf("expected subscription_type to round-trip, got %+v", account)
	}
	if account.SubscriptionAt == nil || !account.SubscriptionAt.Equal(subscriptionAt) {
		t.Fatalf("expected subscription_at to round-trip, got %#v", account.SubscriptionAt)
	}
	if account.RegisteredAt == nil || !account.RegisteredAt.Equal(registeredAt) {
		t.Fatalf("expected registered_at to round-trip, got %#v", account.RegisteredAt)
	}
}

func TestPostgresRepositoryCompareAndSwapTokenCompletionRuntimeUsesRuntimeConditions(t *testing.T) {
	registeredAt := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	createdAt := registeredAt.Add(-time.Hour)
	updatedAt := registeredAt.Add(time.Minute)
	db := &fakeAccountsDB{
		row: fakeAccountsRow{
			values: []any{
				9,
				"user@example.com",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				false,
				(*time.Time)(nil),
				false,
				(*time.Time)(nil),
				(*time.Time)(nil),
				(*time.Time)(nil),
				`{"token_completion_attempts":[{"email":"user@example.com","state":"running"}],"refresh_token_cooldown_until":""}`,
				"token_pending",
				"login",
				"",
				(*time.Time)(nil),
				&registeredAt,
				&createdAt,
				&updatedAt,
			},
		},
	}
	repo := newPostgresRepository(db)

	account, swapped, err := repo.CompareAndSwapTokenCompletionRuntime(
		context.Background(),
		"user@example.com",
		map[string]any{
			"token_completion_attempts": []map[string]any{
				{
					"email":       "user@example.com",
					"state":       "blocked",
					"lease_token": "lease-old",
				},
			},
			"refresh_token_cooldown_until": "2026-04-05T09:55:00Z",
		},
		map[string]any{
			"token_completion_attempts": []map[string]any{
				{
					"email":       "user@example.com",
					"state":       "running",
					"lease_token": "lease-new",
				},
			},
			"refresh_token_cooldown_until": "",
		},
		Account{
			Status: "token_pending",
			Source: "login",
		},
	)
	if err != nil {
		t.Fatalf("unexpected compare-and-swap error: %v", err)
	}
	if !swapped {
		t.Fatal("expected compare-and-swap to succeed")
	}
	if !strings.Contains(db.queryRowQuery, "extra_data = (COALESCE(extra_data, '{}'::jsonb) - 'token_completion_attempts' - 'refresh_token_cooldown_until') || $2::jsonb") {
		t.Fatalf("expected compare-and-swap update query, got %q", db.queryRowQuery)
	}
	if !strings.Contains(db.queryRowQuery, "COALESCE(extra_data->'token_completion_attempts', 'null'::jsonb) = $3::jsonb") {
		t.Fatalf("expected compare-and-swap attempts condition, got %q", db.queryRowQuery)
	}
	if !strings.Contains(db.queryRowQuery, "COALESCE(extra_data->>'refresh_token_cooldown_until', '') = $4") {
		t.Fatalf("expected compare-and-swap cooldown condition, got %q", db.queryRowQuery)
	}
	if len(db.queryRowArgs) != 4 {
		t.Fatalf("expected four compare-and-swap args, got %d", len(db.queryRowArgs))
	}
	if nextRuntimeJSON, ok := db.queryRowArgs[1].(string); !ok || !strings.Contains(nextRuntimeJSON, `"state":"running"`) || !strings.Contains(nextRuntimeJSON, `"lease_token":"lease-new"`) {
		t.Fatalf("expected next runtime json arg, got %#v", db.queryRowArgs[1])
	}
	if currentAttemptsJSON, ok := db.queryRowArgs[2].(string); !ok || !strings.Contains(currentAttemptsJSON, `"state":"blocked"`) || !strings.Contains(currentAttemptsJSON, `"lease_token":"lease-old"`) {
		t.Fatalf("expected current attempts json arg, got %#v", db.queryRowArgs[2])
	}
	if cooldown, ok := db.queryRowArgs[3].(string); !ok || cooldown != "2026-04-05T09:55:00Z" {
		t.Fatalf("expected cooldown compare arg, got %#v", db.queryRowArgs[3])
	}
	if account.Status != "token_pending" || account.Source != "login" {
		t.Fatalf("expected compare-and-swap account to round-trip, got %+v", account)
	}
}

func TestPostgresRepositoryCompareAndSwapTokenCompletionRuntimeReturnsFalseOnFenceConflict(t *testing.T) {
	db := &fakeAccountsDB{
		row: fakeAccountsRow{err: pgx.ErrNoRows},
	}
	repo := newPostgresRepository(db)

	account, swapped, err := repo.CompareAndSwapTokenCompletionRuntime(
		context.Background(),
		"user@example.com",
		map[string]any{
			"token_completion_attempts": []map[string]any{
				{
					"email":       "user@example.com",
					"state":       "running",
					"lease_token": "lease-stale",
				},
			},
		},
		map[string]any{
			"token_completion_attempts": []map[string]any{
				{
					"email":       "user@example.com",
					"state":       "completed",
					"lease_token": "lease-stale",
				},
			},
		},
		Account{
			Status: "token_pending",
			Source: "login",
		},
	)
	if err != nil {
		t.Fatalf("unexpected compare-and-swap error: %v", err)
	}
	if swapped {
		t.Fatalf("expected compare-and-swap conflict, got account %+v", account)
	}
	if account.Email != "" || len(account.ExtraData) != 0 || account.Status != "" || account.Source != "" {
		t.Fatalf("expected empty account on fence conflict, got %+v", account)
	}
	if !strings.Contains(db.queryRowQuery, "COALESCE(extra_data->'token_completion_attempts', 'null'::jsonb) = $3::jsonb") {
		t.Fatalf("expected compare-and-swap fence condition, got %q", db.queryRowQuery)
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
