package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	accountspkg "github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
)

func TestAccountsEndpointCompatibility(t *testing.T) {
	router := internalhttp.NewRouter(nil, fakeAccountsService{
		response: accountspkg.AccountListResponse{
			Page:     1,
			PageSize: 10,
			Total: 2,
			Accounts: []accountspkg.Account{
				{ID: 11, Email: "alpha@example.com", Password: "secret-1", EmailService: "outlook", SubscriptionType: "team", Status: "active"},
				{ID: 12, Email: "beta@example.com", Password: "", EmailService: "tempmail", SubscriptionType: "free", Status: "failed"},
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/accounts?page=1&page_size=10&status=invalid&email_service=outlook&refresh_token_state=has&search=alpha", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}

	if payload["page"] != float64(1) || payload["page_size"] != float64(10) {
		t.Fatalf("expected page envelope, got %#v", payload)
	}
	if payload["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", payload["total"])
	}
	items, ok := payload["accounts"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected accounts length=2, got %#v", payload["accounts"])
	}

	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first account object, got %#v", items[0])
	}
	if first["id"] != float64(11) || first["email"] != "alpha@example.com" || first["password"] != "secret-1" || first["status"] != "active" || first["email_service"] != "outlook" || first["subscription_type"] != "team" {
		t.Fatalf("unexpected first account payload: %#v", first)
	}
	if got := fakeAccountsServiceRequestLog.lastListRequest; got.Status != "invalid" || got.EmailService != "outlook" || got.RefreshTokenState != "has" || got.Search != "alpha" {
		t.Fatalf("expected compatibility filters to reach service, got %+v", got)
	}
}

func TestAccountsReadCompatibilityEndpoints(t *testing.T) {
	service := fakeAccountsService{
		response: accountspkg.AccountListResponse{
			Page:     2,
			PageSize: 5,
			Total:    6,
			Accounts: []accountspkg.Account{{ID: 99, Email: "viewer@example.com", EmailService: "outlook", Status: "active"}},
		},
		currentResponse: accountspkg.CurrentAccountResponse{
			CurrentAccountID: 99,
			Account: &accountspkg.CurrentAccountSummary{
				ID:           99,
				Email:        "viewer@example.com",
				Status:       "active",
				EmailService: "outlook",
				PlanType:     "Team",
			},
		},
		statsSummaryResponse: accountspkg.AccountsStatsSummary{
			Total: 12,
			ByStatus: map[string]int{
				"active": 10,
			},
			ByEmailService: map[string]int{
				"outlook": 8,
			},
		},
		statsOverviewResponse: accountspkg.AccountsOverviewStats{
			Total:       12,
			ActiveCount: 10,
			TokenStats: accountspkg.AccountTokenStats{
				WithAccessToken:   9,
				WithRefreshToken:  8,
				WithoutAccessToken: 3,
			},
			BySubscription: map[string]int{"team": 4},
			RecentAccounts: []accountspkg.AccountOverviewRecentItem{
				{ID: 99, Email: "viewer@example.com", EmailService: "outlook", Status: "active", Source: "register", SubscriptionType: "team"},
			},
		},
		overviewCardsResponse: accountspkg.AccountOverviewCardsResponse{
			Total:            1,
			CurrentAccountID: 99,
			Accounts: []accountspkg.AccountOverviewCard{
				{
					ID:               99,
					Email:            "viewer@example.com",
					Status:           "active",
					EmailService:     "outlook",
					Current:          true,
					PlanType:         "Team",
					PlanSource:       "db.subscription_type",
					HasPlusOrTeam:    true,
					HourlyQuota:      accountspkg.UnknownQuotaSnapshot(),
					WeeklyQuota:      accountspkg.UnknownQuotaSnapshot(),
					CodeReviewQuota:  accountspkg.UnknownQuotaSnapshot(),
					OverviewFetchedAt: "2026-04-05T13:10:00Z",
				},
			},
		},
		overviewSelectableResponse: accountspkg.AccountOverviewSelectableResponse{
			Total: 1,
			Accounts: []accountspkg.AccountOverviewSelectableItem{
				{ID: 99, Email: "viewer@example.com", EmailService: "outlook", SubscriptionType: "team", HasAccessToken: true},
			},
		},
		detailResponse: accountspkg.Account{
			ID:                  99,
			Email:               "viewer@example.com",
			EmailService:        "outlook",
			SubscriptionType:    "team",
			Status:              "active",
			TeamRoleBadges:      []string{"owner"},
			TeamRelationCount:   1,
			TeamRelationSummary: map[string]any{"has_owner_role": true},
		},
		tokensResponse: accountspkg.AccountTokensResponse{
			ID:                 99,
			Email:              "viewer@example.com",
			AccessToken:        "access-99",
			RefreshToken:       "refresh-99",
			SessionToken:       "session-99",
			SessionTokenSource: "db",
			DeviceID:           "device-99",
			HasTokens:          true,
		},
		cookiesResponse: accountspkg.AccountCookiesResponse{
			AccountID: 99,
			Cookies:   "cookie=value",
		},
	}
	router := internalhttp.NewRouter(nil, service)

	assertStatus := func(path string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d (%s)", path, rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload for %s: %v", path, err)
		}
		return payload
	}

	current := assertStatus("/api/accounts/current")
	if current["current_account_id"] != float64(99) {
		t.Fatalf("unexpected current payload: %#v", current)
	}

	summary := assertStatus("/api/accounts/stats/summary")
	if summary["total"] != float64(12) {
		t.Fatalf("unexpected stats summary payload: %#v", summary)
	}

	overview := assertStatus("/api/accounts/stats/overview")
	if overview["total"] != float64(12) || overview["active_count"] != float64(10) {
		t.Fatalf("unexpected stats overview payload: %#v", overview)
	}
	if _, ok := overview["token_stats"].(map[string]any); !ok {
		t.Fatalf("expected token_stats object, got %#v", overview["token_stats"])
	}

	cards := assertStatus("/api/accounts/overview/cards")
	if cards["current_account_id"] != float64(99) || cards["total"] != float64(1) {
		t.Fatalf("unexpected overview cards payload: %#v", cards)
	}

	selectable := assertStatus("/api/accounts/overview/cards/selectable")
	if selectable["total"] != float64(1) {
		t.Fatalf("unexpected selectable cards payload: %#v", selectable)
	}

	detail := assertStatus("/api/accounts/99")
	if detail["subscription_type"] != "team" || detail["email_service"] != "outlook" {
		t.Fatalf("unexpected detail payload: %#v", detail)
	}
	if _, ok := detail["team_relation_summary"].(map[string]any); !ok {
		t.Fatalf("expected team_relation_summary object, got %#v", detail["team_relation_summary"])
	}

	tokens := assertStatus("/api/accounts/99/tokens")
	if tokens["session_token_source"] != "db" || tokens["device_id"] != "device-99" {
		t.Fatalf("unexpected tokens payload: %#v", tokens)
	}

	cookies := assertStatus("/api/accounts/99/cookies")
	if cookies["cookies"] != "cookie=value" {
		t.Fatalf("unexpected cookies payload: %#v", cookies)
	}
}

type fakeAccountsService struct {
	response                   accountspkg.AccountListResponse
	currentResponse            accountspkg.CurrentAccountResponse
	statsSummaryResponse       accountspkg.AccountsStatsSummary
	statsOverviewResponse      accountspkg.AccountsOverviewStats
	overviewCardsResponse      accountspkg.AccountOverviewCardsResponse
	overviewSelectableResponse accountspkg.AccountOverviewSelectableResponse
	detailResponse             accountspkg.Account
	tokensResponse             accountspkg.AccountTokensResponse
	cookiesResponse            accountspkg.AccountCookiesResponse
	err                        error
}

var fakeAccountsServiceRequestLog struct {
	lastListRequest accountspkg.ListAccountsRequest
}

func (f fakeAccountsService) ListAccounts(_ context.Context, req accountspkg.ListAccountsRequest) (accountspkg.AccountListResponse, error) {
	fakeAccountsServiceRequestLog.lastListRequest = req
	return f.response, f.err
}

func (f fakeAccountsService) GetCurrentAccount(context.Context) (accountspkg.CurrentAccountResponse, error) {
	return f.currentResponse, f.err
}

func (f fakeAccountsService) GetAccountsStatsSummary(context.Context) (accountspkg.AccountsStatsSummary, error) {
	return f.statsSummaryResponse, f.err
}

func (f fakeAccountsService) GetAccountsOverviewStats(context.Context) (accountspkg.AccountsOverviewStats, error) {
	return f.statsOverviewResponse, f.err
}

func (f fakeAccountsService) ListOverviewCards(context.Context, accountspkg.AccountOverviewCardsRequest) (accountspkg.AccountOverviewCardsResponse, error) {
	return f.overviewCardsResponse, f.err
}

func (f fakeAccountsService) ListOverviewSelectable(context.Context, accountspkg.AccountOverviewSelectableRequest) (accountspkg.AccountOverviewSelectableResponse, error) {
	return f.overviewSelectableResponse, f.err
}

func (f fakeAccountsService) GetAccount(context.Context, int) (accountspkg.Account, error) {
	return f.detailResponse, f.err
}

func (f fakeAccountsService) GetAccountTokens(context.Context, int) (accountspkg.AccountTokensResponse, error) {
	return f.tokensResponse, f.err
}

func (f fakeAccountsService) GetAccountCookies(context.Context, int) (accountspkg.AccountCookiesResponse, error) {
	return f.cookiesResponse, f.err
}
