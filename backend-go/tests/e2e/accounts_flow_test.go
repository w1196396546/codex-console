package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	accountspkg "github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
)

func TestRecentAccountsCompatibilityEndpoint(t *testing.T) {
	server := httptest.NewServer(internalhttp.NewRouter(nil, e2eAccountsService{
		response: accountspkg.AccountListResponse{
			Total: 1,
			Accounts: []accountspkg.Account{
				{ID: 101, Email: "recent@example.com", Password: "pwd-101", Status: "active"},
			},
		},
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/accounts?page=1&page_size=10")
	if err != nil {
		t.Fatalf("get accounts request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get accounts 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}

	if payload["total"] != float64(1) {
		t.Fatalf("expected total=1, got %#v", payload["total"])
	}
	items, ok := payload["accounts"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected accounts length=1, got %#v", payload["accounts"])
	}

	account, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected account object, got %#v", items[0])
	}
	if account["id"] != float64(101) || account["email"] != "recent@example.com" || account["password"] != "pwd-101" || account["status"] != "active" {
		t.Fatalf("unexpected account payload: %#v", account)
	}
}

func TestManagementAccountsCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(internalhttp.NewRouter(nil, e2eAccountsService{
		currentResponse: accountspkg.CurrentAccountResponse{
			CurrentAccountID: intPtr(202),
			Account: &accountspkg.CurrentAccountSummary{
				ID:           202,
				Email:        "new@example.com",
				Status:       "active",
				EmailService: "manual",
				PlanType:     "Team",
			},
		},
		statsSummaryResponse: accountspkg.AccountsStatsSummary{
			Total:          1,
			ByStatus:       map[string]int{"active": 1},
			ByEmailService: map[string]int{"manual": 1},
		},
		statsOverviewResponse: accountspkg.AccountsOverviewStats{
			Total:          1,
			ActiveCount:    1,
			BySubscription: map[string]int{"team": 1},
			RecentAccounts: []accountspkg.AccountOverviewRecentItem{
				{ID: 202, Email: "new@example.com", EmailService: "manual", Status: "active", Source: "import", SubscriptionType: "team"},
			},
		},
		overviewCardsResponse: accountspkg.AccountOverviewCardsResponse{
			Total:            1,
			CurrentAccountID: intPtr(202),
			Accounts: []accountspkg.AccountOverviewCard{
				{
					ID:              202,
					Email:           "new@example.com",
					Status:          "active",
					EmailService:    "manual",
					Current:         true,
					PlanType:        "Team",
					PlanSource:      "db.subscription_type",
					HasPlusOrTeam:   true,
					HourlyQuota:     accountspkg.UnknownQuotaSnapshot(),
					WeeklyQuota:     accountspkg.UnknownQuotaSnapshot(),
					CodeReviewQuota: accountspkg.UnknownQuotaSnapshot(),
				},
			},
		},
		detailResponse: accountspkg.Account{
			ID:               202,
			Email:            "new@example.com",
			EmailService:     "manual",
			SubscriptionType: "team",
			Status:           "active",
		},
		tokensResponse: accountspkg.AccountTokensResponse{
			ID:                 202,
			Email:              "new@example.com",
			AccessToken:        "access-202",
			RefreshToken:       "refresh-202",
			SessionToken:       "session-202",
			SessionTokenSource: "db",
			HasTokens:          true,
		},
		createResponse: accountspkg.Account{
			ID:               202,
			Email:            "new@example.com",
			EmailService:     "manual",
			SubscriptionType: "team",
			Status:           "active",
		},
		exportResponse: accountspkg.AccountExportResponse{
			ContentType: "application/json",
			Filename:    "accounts_export.json",
			Content:     []byte(`[{"email":"new@example.com"}]`),
		},
	}))
	defer server.Close()

	getJSON := func(path string) map[string]any {
		t.Helper()

		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s failed: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected %s 200, got %d", path, resp.StatusCode)
		}

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s response: %v", path, err)
		}
		return payload
	}

	current := getJSON("/api/accounts/current")
	if current["current_account_id"] != float64(202) {
		t.Fatalf("unexpected current account payload: %#v", current)
	}

	summary := getJSON("/api/accounts/stats/summary")
	if summary["total"] != float64(1) {
		t.Fatalf("unexpected stats summary payload: %#v", summary)
	}

	overview := getJSON("/api/accounts/stats/overview")
	if overview["total"] != float64(1) {
		t.Fatalf("unexpected overview stats payload: %#v", overview)
	}
	if recent, ok := overview["recent_accounts"].([]any); !ok || len(recent) != 1 {
		t.Fatalf("expected recent_accounts length=1, got %#v", overview["recent_accounts"])
	}

	cards := getJSON("/api/accounts/overview/cards")
	if cards["current_account_id"] != float64(202) || cards["total"] != float64(1) {
		t.Fatalf("unexpected overview cards payload: %#v", cards)
	}

	detail := getJSON("/api/accounts/202")
	if detail["email_service"] != "manual" || detail["subscription_type"] != "team" {
		t.Fatalf("unexpected account detail payload: %#v", detail)
	}

	tokens := getJSON("/api/accounts/202/tokens")
	if tokens["session_token_source"] != "db" || tokens["access_token"] != "access-202" {
		t.Fatalf("unexpected tokens payload: %#v", tokens)
	}

	createResp, err := http.Post(server.URL+"/api/accounts", "application/json", bytes.NewBufferString(`{"email":"new@example.com","password":"secret","email_service":"manual","status":"active","subscription_type":"team"}`))
	if err != nil {
		t.Fatalf("post create account failed: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected create account 200, got %d", createResp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/accounts/export/json", bytes.NewBufferString(`{"ids":[202],"select_all":false}`))
	if err != nil {
		t.Fatalf("build export request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	exportResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post export request failed: %v", err)
	}
	defer exportResp.Body.Close()
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("expected export 200, got %d", exportResp.StatusCode)
	}
	if exportResp.Header.Get("Content-Disposition") == "" {
		t.Fatalf("expected export attachment header, got %#v", exportResp.Header)
	}
}

func TestManagementAccountsOverviewRefreshCompatibility(t *testing.T) {
	server := httptest.NewServer(internalhttp.NewRouter(nil, e2eAccountsService{
		refreshOverviewResponse: accountspkg.OverviewRefreshResponse{
			SuccessCount: 1,
			FailedCount:  1,
			Details: []accountspkg.OverviewRefreshDetail{
				{
					ID:       202,
					Email:    "plus@example.com",
					Success:  true,
					PlanType: "Plus",
				},
				{
					ID:      303,
					Email:   "unknown@example.com",
					Success: false,
					Error:   "未获取到配额数据",
				},
			},
		},
	}))
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/accounts/overview/refresh",
		"application/json",
		bytes.NewBufferString(`{"ids":[202,303],"force":true,"select_all":false}`),
	)
	if err != nil {
		t.Fatalf("post overview refresh failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected overview refresh 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode overview refresh response: %v", err)
	}

	if payload["success_count"] != float64(1) || payload["failed_count"] != float64(1) {
		t.Fatalf("unexpected overview refresh counters: %#v", payload)
	}
	details, ok := payload["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("expected 2 details, got %#v", payload["details"])
	}

	successDetail, ok := details[0].(map[string]any)
	if !ok || successDetail["success"] != true || successDetail["plan_type"] != "Plus" {
		t.Fatalf("unexpected success detail payload: %#v", details[0])
	}
	failedDetail, ok := details[1].(map[string]any)
	if !ok || failedDetail["success"] != false || failedDetail["error"] != "未获取到配额数据" {
		t.Fatalf("unexpected failed detail payload: %#v", details[1])
	}
}

type e2eAccountsService struct {
	response              accountspkg.AccountListResponse
	currentResponse       accountspkg.CurrentAccountResponse
	statsSummaryResponse  accountspkg.AccountsStatsSummary
	statsOverviewResponse accountspkg.AccountsOverviewStats
	overviewCardsResponse accountspkg.AccountOverviewCardsResponse
	detailResponse        accountspkg.Account
	tokensResponse        accountspkg.AccountTokensResponse
	createResponse        accountspkg.Account
	exportResponse        accountspkg.AccountExportResponse
	err                   error
}

func (s e2eAccountsService) ListAccounts(context.Context, accountspkg.ListAccountsRequest) (accountspkg.AccountListResponse, error) {
	return s.response, s.err
}

func (s e2eAccountsService) GetCurrentAccount(context.Context) (accountspkg.CurrentAccountResponse, error) {
	return s.currentResponse, s.err
}

func (s e2eAccountsService) GetAccountsStatsSummary(context.Context) (accountspkg.AccountsStatsSummary, error) {
	return s.statsSummaryResponse, s.err
}

func (s e2eAccountsService) GetAccountsOverviewStats(context.Context) (accountspkg.AccountsOverviewStats, error) {
	return s.statsOverviewResponse, s.err
}

func (s e2eAccountsService) ListOverviewCards(context.Context, accountspkg.AccountOverviewCardsRequest) (accountspkg.AccountOverviewCardsResponse, error) {
	return s.overviewCardsResponse, s.err
}

func (s e2eAccountsService) GetAccount(context.Context, int) (accountspkg.Account, error) {
	return s.detailResponse, s.err
}

func (s e2eAccountsService) GetAccountTokens(context.Context, int) (accountspkg.AccountTokensResponse, error) {
	return s.tokensResponse, s.err
}

func (s e2eAccountsService) CreateManualAccount(context.Context, accountspkg.ManualAccountCreateRequest) (accountspkg.Account, error) {
	return s.createResponse, s.err
}

func (s e2eAccountsService) ExportAccounts(context.Context, string, accountspkg.AccountSelectionRequest) (accountspkg.AccountExportResponse, error) {
	return s.exportResponse, s.err
}

func intPtr(value int) *int {
	return &value
}
