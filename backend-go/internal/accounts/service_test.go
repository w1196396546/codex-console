package accounts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServiceUpsertAccountMergesMeaningfulFieldsIntoExistingAccount(t *testing.T) {
	existingRegisteredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &fakeRepository{
		foundAccount: Account{
			ID:             7,
			Email:          "user@example.com",
			Password:       "old-password",
			EmailService:   "outlook",
			RefreshToken:   "refresh-old",
			Status:         "failed",
			Source:         "login",
			RegisteredAt:   &existingRegisteredAt,
			ExtraData:      map[string]any{"persisted": "keep", "override": "old"},
			WorkspaceID:    "workspace-old",
			SessionToken:   "session-old",
			EmailServiceID: "mailbox-old",
		},
		upsertedAccount: Account{
			ID:    7,
			Email: "user@example.com",
		},
	}
	service := NewService(repo)

	saved, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:          " user@example.com ",
		EmailService:   "",
		Password:       "",
		AccessToken:    " access-new ",
		RefreshToken:   "",
		SessionToken:   " session-new ",
		WorkspaceID:    "workspace-new",
		EmailServiceID: "mailbox-new",
		ExtraData: map[string]any{
			"override": "new",
			"nested":   map[string]any{"ok": true},
		},
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}
	if saved.ID != 7 {
		t.Fatalf("expected repository return to be forwarded, got %+v", saved)
	}

	if repo.lookedUpEmail != "user@example.com" {
		t.Fatalf("expected lookup by normalized email, got %q", repo.lookedUpEmail)
	}
	if repo.savedAccount.EmailService != "outlook" {
		t.Fatalf("expected blank email_service to preserve existing value, got %q", repo.savedAccount.EmailService)
	}
	if repo.savedAccount.Password != "old-password" {
		t.Fatalf("expected blank password to preserve existing value, got %q", repo.savedAccount.Password)
	}
	if repo.savedAccount.RefreshToken != "refresh-old" {
		t.Fatalf("expected blank refresh_token to preserve existing value, got %q", repo.savedAccount.RefreshToken)
	}
	if repo.savedAccount.AccessToken != "access-new" {
		t.Fatalf("expected access_token to be updated, got %q", repo.savedAccount.AccessToken)
	}
	if repo.savedAccount.SessionToken != "session-new" {
		t.Fatalf("expected session_token to be updated, got %q", repo.savedAccount.SessionToken)
	}
	if repo.savedAccount.WorkspaceID != "workspace-new" {
		t.Fatalf("expected workspace_id to be updated, got %q", repo.savedAccount.WorkspaceID)
	}
	if repo.savedAccount.EmailServiceID != "mailbox-new" {
		t.Fatalf("expected email_service_id to be updated, got %q", repo.savedAccount.EmailServiceID)
	}
	if repo.savedAccount.Status != DefaultAccountStatus {
		t.Fatalf("expected default status to be applied, got %q", repo.savedAccount.Status)
	}
	if repo.savedAccount.Source != DefaultAccountSource {
		t.Fatalf("expected default source to be applied, got %q", repo.savedAccount.Source)
	}
	if repo.savedAccount.RegisteredAt == nil {
		t.Fatalf("expected registered_at to stay populated, got %#v", repo.savedAccount.RegisteredAt)
	}
	if repo.savedAccount.RegisteredAt.Equal(existingRegisteredAt) {
		t.Fatalf("expected failed->active merge to refresh registered_at, got %#v", repo.savedAccount.RegisteredAt)
	}
	if repo.savedAccount.ExtraData["persisted"] != "keep" || repo.savedAccount.ExtraData["override"] != "new" {
		t.Fatalf("expected merged extra_data, got %#v", repo.savedAccount.ExtraData)
	}
}

func TestServiceUpsertAccountRequiresEmailServiceForNewAccount(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email: "new@example.com",
	})
	if err == nil {
		t.Fatal("expected missing email_service to fail for new account")
	}
}

func TestServiceUpsertAccountReturnsRepositoryError(t *testing.T) {
	expectedErr := errors.New("boom")
	service := NewService(&fakeRepository{upsertErr: expectedErr})

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:        "user@example.com",
		EmailService: "outlook",
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected repository error %v, got %v", expectedErr, err)
	}
}

func TestServiceUpsertAccountDoesNotDowngradePartialStatusOrTemporaryExtraData(t *testing.T) {
	repo := &fakeRepository{
		foundAccount: Account{
			ID:           8,
			Email:        "stable@example.com",
			EmailService: "outlook",
			RefreshToken: "refresh-stable",
			Status:       "active",
			ExtraData: map[string]any{
				"persisted": "keep",
			},
		},
	}
	service := NewService(repo)

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:        "stable@example.com",
		EmailService: "outlook",
		SessionToken: "session-new",
		Status:       "token_pending",
		ExtraData: map[string]any{
			"token_pending":         true,
			"login_incomplete":      true,
			"account_status_reason": "missing_refresh_token",
			"refresh_token_error":   "429",
		},
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if repo.savedAccount.RefreshToken != "refresh-stable" {
		t.Fatalf("expected existing refresh_token to be preserved, got %q", repo.savedAccount.RefreshToken)
	}
	if repo.savedAccount.Status != "active" {
		t.Fatalf("expected active status to be preserved, got %q", repo.savedAccount.Status)
	}
	if repo.savedAccount.SessionToken != "session-new" {
		t.Fatalf("expected session_token to still merge, got %q", repo.savedAccount.SessionToken)
	}
	if _, exists := repo.savedAccount.ExtraData["token_pending"]; exists {
		t.Fatalf("expected token_pending to be removed from extra_data, got %#v", repo.savedAccount.ExtraData)
	}
	if _, exists := repo.savedAccount.ExtraData["login_incomplete"]; exists {
		t.Fatalf("expected login_incomplete to be removed from extra_data, got %#v", repo.savedAccount.ExtraData)
	}
	if _, exists := repo.savedAccount.ExtraData["account_status_reason"]; exists {
		t.Fatalf("expected account_status_reason to be removed from extra_data, got %#v", repo.savedAccount.ExtraData)
	}
	if repo.savedAccount.ExtraData["persisted"] != "keep" || repo.savedAccount.ExtraData["refresh_token_error"] != "429" {
		t.Fatalf("expected non-temporary extra_data to be preserved, got %#v", repo.savedAccount.ExtraData)
	}
}

func TestServiceUpsertAccountRefreshesRegisteredAtWhenFailedBecomesActive(t *testing.T) {
	existingRegisteredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	incomingRegisteredAt := existingRegisteredAt.Add(48 * time.Hour)
	repo := &fakeRepository{
		foundAccount: Account{
			ID:           9,
			Email:        "retry@example.com",
			EmailService: "outlook",
			Status:       "failed",
			RegisteredAt: &existingRegisteredAt,
		},
	}
	service := NewService(repo)

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:        "retry@example.com",
		EmailService: "outlook",
		Status:       "active",
		RegisteredAt: &incomingRegisteredAt,
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if repo.savedAccount.RegisteredAt == nil || !repo.savedAccount.RegisteredAt.Equal(incomingRegisteredAt) {
		t.Fatalf("expected registered_at to refresh on failed->active, got %#v", repo.savedAccount.RegisteredAt)
	}
}

func TestServiceUpsertAccountMergesUploadWritebackFields(t *testing.T) {
	uploadedAt := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		foundAccount: Account{
			ID:              10,
			Email:           "writeback@example.com",
			EmailService:    "outlook",
			AccessToken:     "access-1",
			Status:          "active",
			Source:          "register",
			Sub2APIUploaded: true,
		},
	}
	service := NewService(repo)

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:         "writeback@example.com",
		EmailService:  "outlook",
		AccessToken:   "access-1",
		Status:        "active",
		Source:        "register",
		CPAUploaded:   boolPtr(true),
		CPAUploadedAt: &uploadedAt,
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if !repo.savedAccount.CPAUploaded {
		t.Fatalf("expected CPA uploaded flag to be merged, got %+v", repo.savedAccount)
	}
	if repo.savedAccount.CPAUploadedAt == nil || !repo.savedAccount.CPAUploadedAt.Equal(uploadedAt) {
		t.Fatalf("expected CPA uploaded timestamp %v, got %+v", uploadedAt, repo.savedAccount)
	}
	if !repo.savedAccount.Sub2APIUploaded {
		t.Fatalf("expected existing sub2api uploaded flag to be preserved, got %+v", repo.savedAccount)
	}
}

func TestServiceUpsertAccountMergesTokenTimingAndSubscriptionFields(t *testing.T) {
	existingLastRefresh := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	existingExpiresAt := existingLastRefresh.Add(30 * time.Minute)
	incomingLastRefresh := existingLastRefresh.Add(2 * time.Hour)
	incomingExpiresAt := incomingLastRefresh.Add(90 * time.Minute)
	existingSubscriptionAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	incomingSubscriptionAt := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		foundAccount: Account{
			ID:               11,
			Email:            "timing@example.com",
			EmailService:     "outlook",
			LastRefresh:      &existingLastRefresh,
			ExpiresAt:        &existingExpiresAt,
			SubscriptionType: "plus",
			SubscriptionAt:   &existingSubscriptionAt,
			Status:           "active",
		},
	}
	service := NewService(repo)

	_, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{
		Email:            "timing@example.com",
		EmailService:     "outlook",
		LastRefresh:      &incomingLastRefresh,
		ExpiresAt:        &incomingExpiresAt,
		SubscriptionType: "team",
		SubscriptionAt:   &incomingSubscriptionAt,
		Status:           "active",
	})
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if repo.savedAccount.LastRefresh == nil || !repo.savedAccount.LastRefresh.Equal(incomingLastRefresh) {
		t.Fatalf("expected last_refresh to be updated, got %#v", repo.savedAccount.LastRefresh)
	}
	if repo.savedAccount.ExpiresAt == nil || !repo.savedAccount.ExpiresAt.Equal(incomingExpiresAt) {
		t.Fatalf("expected expires_at to be updated, got %#v", repo.savedAccount.ExpiresAt)
	}
	if repo.savedAccount.SubscriptionType != "team" {
		t.Fatalf("expected subscription_type to be updated, got %q", repo.savedAccount.SubscriptionType)
	}
	if repo.savedAccount.SubscriptionAt == nil || !repo.savedAccount.SubscriptionAt.Equal(incomingSubscriptionAt) {
		t.Fatalf("expected subscription_at to be updated, got %#v", repo.savedAccount.SubscriptionAt)
	}
}

func TestServiceListAccountsNormalizesCompatibilityFiltersAndEnvelope(t *testing.T) {
	repo := &fakeRepository{
		listedAccounts: []Account{
			{
				ID:               41,
				Email:            "alpha@example.com",
				EmailService:     "outlook",
				Status:           "active",
				SubscriptionType: "team",
			},
		},
		listedTotal: 1,
	}
	service := NewService(repo)

	resp, err := service.ListAccounts(context.Background(), ListAccountsRequest{
		Page:              0,
		PageSize:          999,
		Status:            " invalid ",
		EmailService:      " outlook ",
		RefreshTokenState: " has ",
		Search:            " alpha ",
	})
	if err != nil {
		t.Fatalf("unexpected list accounts error: %v", err)
	}

	if repo.listReq.Page != DefaultPage {
		t.Fatalf("expected normalized page=%d, got %+v", DefaultPage, repo.listReq)
	}
	if repo.listReq.PageSize != MaxPageSize {
		t.Fatalf("expected normalized page_size=%d, got %+v", MaxPageSize, repo.listReq)
	}
	if repo.listReq.Status != "invalid" || repo.listReq.EmailService != "outlook" || repo.listReq.RefreshTokenState != "has" || repo.listReq.Search != "alpha" {
		t.Fatalf("expected normalized compatibility filters, got %+v", repo.listReq)
	}
	if resp.Total != 1 || resp.Page != DefaultPage || resp.PageSize != MaxPageSize {
		t.Fatalf("unexpected list response envelope: %+v", resp)
	}
	if len(resp.Accounts) != 1 || resp.Accounts[0].EmailService != "outlook" || resp.Accounts[0].SubscriptionType != "team" {
		t.Fatalf("unexpected list response accounts: %+v", resp.Accounts)
	}
}

func TestServiceOverviewRefreshFetchesRemoteQuotaAndPersistsOverview(t *testing.T) {
	repo := &fakeRepository{
		listedAccounts: []Account{
			{
				ID:               201,
				Email:            "team@example.com",
				EmailService:     "outlook",
				SubscriptionType: "team",
				AccessToken:      "access-201",
				AccountID:        "account-201",
			},
		},
	}
	service := NewService(repo)
	service.httpDoer = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer access-201" {
			t.Fatalf("expected authorization header to reuse account token, got %q", got)
		}
		switch req.URL.Path {
		case "/backend-api/me":
			return jsonHTTPResponse(http.StatusOK, `{"plan_type":"team"}`), nil
		case "/backend-api/wham/usage":
			return jsonHTTPResponse(http.StatusOK, `{
				"rate_limit": {
					"primary_window": {
						"remaining_percent": 75,
						"total": 100,
						"remaining": 75,
						"resets_in_seconds": 3600
					},
					"secondary_window": {
						"remaining_percent": 50,
						"total": 200,
						"remaining": 100,
						"resets_in_seconds": 604800
					}
				},
				"code_review_rate_limit": {
					"primary_window": {
						"remaining_percent": 80,
						"total": 20,
						"remaining": 16,
						"resets_in_seconds": 7200
					}
				}
			}`), nil
		case "/backend-api/codex/usage":
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected refresh request path: %s", req.URL.Path)
			return nil, nil
		}
	})

	resp, err := service.RefreshOverview(context.Background(), OverviewRefreshRequest{
		IDs:   []int{201},
		Force: true,
	})
	if err != nil {
		t.Fatalf("unexpected refresh overview error: %v", err)
	}

	if resp.SuccessCount != 1 || resp.FailedCount != 0 {
		t.Fatalf("expected 1 success and 0 failures, got %+v", resp)
	}
	if len(resp.Details) != 1 || !resp.Details[0].Success || resp.Details[0].PlanType != "Team" {
		t.Fatalf("unexpected detail payload: %+v", resp.Details)
	}

	overview, ok := repo.savedAccount.ExtraData[OverviewExtraDataKey].(map[string]any)
	if !ok {
		t.Fatalf("expected overview to be written back, got %#v", repo.savedAccount.ExtraData)
	}
	if extractStringMapValue(overview, "plan_type") != "Team" {
		t.Fatalf("expected persisted plan_type=Team, got %#v", overview)
	}
	if extractStringMapValue(extractQuotaSnapshot(overview, "hourly_quota"), "status") != "ok" {
		t.Fatalf("expected hourly quota to be refreshed, got %#v", overview["hourly_quota"])
	}
	if extractStringMapValue(extractQuotaSnapshot(overview, "weekly_quota"), "status") != "ok" {
		t.Fatalf("expected weekly quota to be refreshed, got %#v", overview["weekly_quota"])
	}
	if extractStringMapValue(extractQuotaSnapshot(overview, "code_review_quota"), "status") != "ok" {
		t.Fatalf("expected code review quota to be refreshed, got %#v", overview["code_review_quota"])
	}
}

func TestServiceOverviewRefreshFailsWhenQuotaRemainsUnknown(t *testing.T) {
	repo := &fakeRepository{
		listedAccounts: []Account{
			{
				ID:               202,
				Email:            "unknown@example.com",
				EmailService:     "outlook",
				SubscriptionType: "team",
				AccessToken:      "access-202",
			},
		},
	}
	service := NewService(repo)
	service.httpDoer = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/backend-api/me":
			return jsonHTTPResponse(http.StatusOK, `{"plan_type":"team"}`), nil
		case "/backend-api/wham/usage", "/backend-api/codex/usage":
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected refresh request path: %s", req.URL.Path)
			return nil, nil
		}
	})

	resp, err := service.RefreshOverview(context.Background(), OverviewRefreshRequest{
		IDs:   []int{202},
		Force: true,
	})
	if err != nil {
		t.Fatalf("unexpected refresh overview error: %v", err)
	}

	if resp.SuccessCount != 0 || resp.FailedCount != 1 {
		t.Fatalf("expected 0 success and 1 failure, got %+v", resp)
	}
	if len(resp.Details) != 1 {
		t.Fatalf("expected one detail, got %+v", resp.Details)
	}
	if resp.Details[0].Success {
		t.Fatalf("expected unknown quota refresh to fail, got %+v", resp.Details[0])
	}
	if resp.Details[0].Error != "未获取到配额数据" {
		t.Fatalf("expected compatibility error detail, got %+v", resp.Details[0])
	}

	overview, ok := repo.savedAccount.ExtraData[OverviewExtraDataKey].(map[string]any)
	if !ok {
		t.Fatalf("expected overview fallback to still be written back, got %#v", repo.savedAccount.ExtraData)
	}
	if extractStringMapValue(extractQuotaSnapshot(overview, "hourly_quota"), "status") != "unknown" {
		t.Fatalf("expected hourly quota to remain unknown, got %#v", overview["hourly_quota"])
	}
	if extractStringMapValue(extractQuotaSnapshot(overview, "weekly_quota"), "status") != "unknown" {
		t.Fatalf("expected weekly quota to remain unknown, got %#v", overview["weekly_quota"])
	}
}

type fakeRepository struct {
	foundAccount     Account
	found            bool
	findErr          error
	accountByID      Account
	accountByIDErr   error
	currentAccountID *int
	listReq          ListAccountsRequest
	listedAccounts   []Account
	listedTotal      int
	upsertedAccount  Account
	upsertErr        error
	lookedUpEmail    string
	savedAccount     Account
}

func (f *fakeRepository) ListAccounts(_ context.Context, req ListAccountsRequest) ([]Account, int, error) {
	f.listReq = req
	return append([]Account(nil), f.listedAccounts...), f.listedTotal, nil
}

func (f *fakeRepository) GetAccountByEmail(_ context.Context, email string) (Account, bool, error) {
	f.lookedUpEmail = email
	if f.findErr != nil {
		return Account{}, false, f.findErr
	}
	if f.found || f.foundAccount.Email != "" {
		return f.foundAccount, true, nil
	}
	return Account{}, false, nil
}

func (f *fakeRepository) GetAccountByID(context.Context, int) (Account, error) {
	if f.accountByIDErr != nil {
		return Account{}, f.accountByIDErr
	}
	if f.accountByID.ID != 0 || f.accountByID.Email != "" {
		return f.accountByID, nil
	}
	return Account{}, ErrAccountNotFound
}

func (f *fakeRepository) GetCurrentAccountID(context.Context) (*int, error) {
	return f.currentAccountID, nil
}

func (f *fakeRepository) GetAccountsStatsSummary(context.Context) (AccountsStatsSummary, error) {
	return AccountsStatsSummary{ByStatus: map[string]int{}, ByEmailService: map[string]int{}}, nil
}

func (f *fakeRepository) GetAccountsOverviewStats(context.Context) (AccountsOverviewStats, error) {
	return AccountsOverviewStats{
		ByStatus:       map[string]int{},
		ByEmailService: map[string]int{},
		BySource:       map[string]int{},
		BySubscription: map[string]int{},
	}, nil
}

func (f *fakeRepository) ListAccountsForOverview(context.Context, AccountOverviewCardsRequest) ([]Account, error) {
	return nil, nil
}

func (f *fakeRepository) ListAccountsForSelectable(context.Context, AccountOverviewSelectableRequest) ([]Account, error) {
	return nil, nil
}

func (f *fakeRepository) ListAccountsBySelection(context.Context, AccountSelectionRequest) ([]Account, error) {
	return append([]Account(nil), f.listedAccounts...), nil
}

func (f *fakeRepository) UpsertAccount(_ context.Context, account Account) (Account, error) {
	f.savedAccount = account
	if f.upsertErr != nil {
		return Account{}, f.upsertErr
	}
	if f.upsertedAccount.Email != "" || f.upsertedAccount.ID != 0 {
		return f.upsertedAccount, nil
	}
	return account, nil
}

func (f *fakeRepository) DeleteAccount(context.Context, int) error {
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
