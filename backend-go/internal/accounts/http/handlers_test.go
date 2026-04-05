package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
			CurrentAccountID: intPtr(99),
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
			CurrentAccountID: intPtr(99),
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

func TestAccountsWriteCompatibilityEndpoints(t *testing.T) {
	service := fakeAccountsService{
		createResponse: accountspkg.Account{
			ID:               199,
			Email:            "new@example.com",
			Password:         "secret-199",
			EmailService:     "manual",
			SubscriptionType: "team",
			Status:           "active",
		},
		importResponse: accountspkg.ImportAccountsResult{
			Success: true,
			Total:   2,
			Created: 1,
			Updated: 1,
		},
		updateResponse: accountspkg.Account{
			ID:               99,
			Email:            "viewer@example.com",
			EmailService:     "outlook",
			SubscriptionType: "team",
			Status:           "expired",
		},
		deleteResponse: accountspkg.ActionResponse{Success: true, Message: "账号 viewer@example.com 已删除"},
		batchDeleteResponse: accountspkg.BatchDeleteResponse{
			Success:      true,
			DeletedCount: 2,
		},
		batchUpdateResponse: accountspkg.BatchUpdateResponse{
			Success:        true,
			RequestedCount: 2,
			UpdatedCount:   2,
		},
		batchRefreshResponse: accountspkg.BatchRefreshResponse{
			SuccessCount: 2,
			FailedCount:  0,
		},
		refreshResponse: accountspkg.TokenRefreshActionResponse{
			Success: true,
			Message: "Token 刷新成功",
		},
		batchValidateResponse: accountspkg.BatchValidateResponse{
			ValidCount:   1,
			InvalidCount: 1,
		},
		validateResponse: accountspkg.TokenValidateResponse{
			ID:    99,
			Valid: true,
		},
		batchCPAResponse: accountspkg.BatchUploadResponse{
			SuccessCount: 1,
			FailedCount:  0,
		},
		uploadCPAResponse: accountspkg.ActionResponse{Success: true, Message: "上传成功"},
		batchSub2APIResponse: accountspkg.BatchUploadResponse{
			SuccessCount: 1,
			FailedCount:  0,
		},
		uploadSub2APIResponse: accountspkg.ActionResponse{Success: true, Message: "上传成功"},
		batchTMResponse: accountspkg.BatchUploadResponse{
			SuccessCount: 1,
			FailedCount:  0,
		},
		uploadTMResponse: accountspkg.ActionResponse{Success: true, Message: "上传成功"},
		exportResponse: accountspkg.AccountExportResponse{
			ContentType: "application/json",
			Filename:    "accounts_20260405.json",
			Content:     []byte(`[{"email":"viewer@example.com"}]`),
		},
		removeCardsResponse: accountspkg.OverviewCardMutationResponse{
			Success:      true,
			RemovedCount: 1,
			Total:        1,
		},
		restoreCardResponse: accountspkg.ActionResponse{Success: true, Message: "restored"},
		attachCardResponse: accountspkg.OverviewAttachResponse{
			Success:        true,
			ID:             99,
			Email:          "viewer@example.com",
			AlreadyInCards: false,
		},
		refreshOverviewResponse: accountspkg.OverviewRefreshResponse{
			SuccessCount: 1,
			FailedCount:  0,
			Details: []accountspkg.OverviewRefreshDetail{
				{ID: 99, Email: "viewer@example.com", Success: true, PlanType: "Team"},
			},
		},
		inboxCodeResponse: accountspkg.InboxCodeResponse{
			Success: true,
			Code:    "123456",
			Email:   "viewer@example.com",
		},
	}
	router := internalhttp.NewRouter(nil, service)

	doJSON := func(method string, path string, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(rec, req)
		return rec
	}

	createRec := doJSON(http.MethodPost, "/api/accounts", `{"email":"new@example.com","password":"secret-199","email_service":"manual","subscription_type":"team","status":"active","source":"manual"}`)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d (%s)", createRec.Code, createRec.Body.String())
	}
	if fakeAccountsServiceRequestLog.createRequest.EmailService != "manual" || fakeAccountsServiceRequestLog.createRequest.SubscriptionType != "team" {
		t.Fatalf("unexpected create request decode: %+v", fakeAccountsServiceRequestLog.createRequest)
	}

	importRec := doJSON(http.MethodPost, "/api/accounts/import", `{"accounts":[{"email":"alpha@example.com"},{"email":"beta@example.com"}],"overwrite":true}`)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected import 200, got %d (%s)", importRec.Code, importRec.Body.String())
	}
	if fakeAccountsServiceRequestLog.importRequest.Overwrite != true || len(fakeAccountsServiceRequestLog.importRequest.Accounts) != 2 {
		t.Fatalf("unexpected import request decode: %+v", fakeAccountsServiceRequestLog.importRequest)
	}

	updateRec := doJSON(http.MethodPatch, "/api/accounts/99", `{"status":"expired","cookies":"cookie=value","session_token":"session-new"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d (%s)", updateRec.Code, updateRec.Body.String())
	}

	deleteRec := doJSON(http.MethodDelete, "/api/accounts/99", "")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d (%s)", deleteRec.Code, deleteRec.Body.String())
	}

	batchUpdateRec := doJSON(http.MethodPost, "/api/accounts/batch-update", `{"ids":[],"select_all":true,"status":"failed","status_filter":"active","email_service_filter":"outlook","refresh_token_state_filter":"has","search_filter":"viewer"}`)
	if batchUpdateRec.Code != http.StatusOK {
		t.Fatalf("expected batch update 200, got %d (%s)", batchUpdateRec.Code, batchUpdateRec.Body.String())
	}
	if !fakeAccountsServiceRequestLog.batchUpdateRequest.SelectAll || fakeAccountsServiceRequestLog.batchUpdateRequest.RefreshTokenStateFilter != "has" {
		t.Fatalf("unexpected batch update decode: %+v", fakeAccountsServiceRequestLog.batchUpdateRequest)
	}

	if rec := doJSON(http.MethodPost, "/api/accounts/batch-delete", `{"ids":[99,100],"select_all":false}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch delete 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/batch-refresh", `{"ids":[99],"select_all":false}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch refresh 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/99/refresh", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("expected refresh 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/batch-validate", `{"ids":[99],"select_all":false}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch validate 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/99/validate", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("expected validate 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	if rec := doJSON(http.MethodPost, "/api/accounts/batch-upload-cpa", `{"ids":[99],"select_all":false,"cpa_service_id":11}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch upload cpa 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if fakeAccountsServiceRequestLog.batchCPAUploadRequest.CPAServiceID == nil || *fakeAccountsServiceRequestLog.batchCPAUploadRequest.CPAServiceID != 11 {
		t.Fatalf("unexpected batch cpa upload decode: %+v", fakeAccountsServiceRequestLog.batchCPAUploadRequest)
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/99/upload-cpa", `{"cpa_service_id":11}`); rec.Code != http.StatusOK {
		t.Fatalf("expected single upload cpa 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/batch-upload-sub2api", `{"ids":[99],"select_all":false,"service_id":22}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch upload sub2api 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/99/upload-sub2api", `{"service_id":22}`); rec.Code != http.StatusOK {
		t.Fatalf("expected single upload sub2api 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/batch-upload-tm", `{"ids":[99],"select_all":false,"service_id":33}`); rec.Code != http.StatusOK {
		t.Fatalf("expected batch upload tm 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/99/upload-tm", `{"service_id":33}`); rec.Code != http.StatusOK {
		t.Fatalf("expected single upload tm 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	exportRec := doJSON(http.MethodPost, "/api/accounts/export/json", `{"ids":[99],"select_all":false}`)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d (%s)", exportRec.Code, exportRec.Body.String())
	}
	if contentDisposition := exportRec.Header().Get("Content-Disposition"); !strings.Contains(contentDisposition, "accounts_20260405.json") {
		t.Fatalf("expected export filename header, got %q", contentDisposition)
	}

	if rec := doJSON(http.MethodPost, "/api/accounts/overview/cards/remove", `{"ids":[99],"select_all":false}`); rec.Code != http.StatusOK {
		t.Fatalf("expected overview remove 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/overview/cards/99/restore", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("expected overview restore 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doJSON(http.MethodPost, "/api/accounts/overview/cards/99/attach", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("expected overview attach 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	refreshOverviewRec := doJSON(http.MethodPost, "/api/accounts/overview/refresh", `{"ids":[99],"force":true,"select_all":false}`)
	if refreshOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected overview refresh 200, got %d (%s)", refreshOverviewRec.Code, refreshOverviewRec.Body.String())
	}
	if len(fakeAccountsServiceRequestLog.refreshOverviewRequest.IDs) != 1 || !fakeAccountsServiceRequestLog.refreshOverviewRequest.Force {
		t.Fatalf("unexpected overview refresh decode: %+v", fakeAccountsServiceRequestLog.refreshOverviewRequest)
	}

	inboxRec := doJSON(http.MethodPost, "/api/accounts/99/inbox-code", `{}`)
	if inboxRec.Code != http.StatusOK {
		t.Fatalf("expected inbox-code 200, got %d (%s)", inboxRec.Code, inboxRec.Body.String())
	}
	var inboxPayload map[string]any
	if err := json.Unmarshal(inboxRec.Body.Bytes(), &inboxPayload); err != nil {
		t.Fatalf("decode inbox payload: %v", err)
	}
	if inboxPayload["code"] != "123456" {
		t.Fatalf("unexpected inbox payload: %#v", inboxPayload)
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
	createResponse             accountspkg.Account
	importResponse             accountspkg.ImportAccountsResult
	updateResponse             accountspkg.Account
	deleteResponse             accountspkg.ActionResponse
	batchDeleteResponse        accountspkg.BatchDeleteResponse
	batchUpdateResponse        accountspkg.BatchUpdateResponse
	batchRefreshResponse       accountspkg.BatchRefreshResponse
	refreshResponse            accountspkg.TokenRefreshActionResponse
	batchValidateResponse      accountspkg.BatchValidateResponse
	validateResponse           accountspkg.TokenValidateResponse
	batchCPAResponse           accountspkg.BatchUploadResponse
	uploadCPAResponse          accountspkg.ActionResponse
	batchSub2APIResponse       accountspkg.BatchUploadResponse
	uploadSub2APIResponse      accountspkg.ActionResponse
	batchTMResponse            accountspkg.BatchUploadResponse
	uploadTMResponse           accountspkg.ActionResponse
	exportResponse             accountspkg.AccountExportResponse
	removeCardsResponse        accountspkg.OverviewCardMutationResponse
	restoreCardResponse        accountspkg.ActionResponse
	attachCardResponse         accountspkg.OverviewAttachResponse
	refreshOverviewResponse    accountspkg.OverviewRefreshResponse
	inboxCodeResponse          accountspkg.InboxCodeResponse
	err                        error
}

var fakeAccountsServiceRequestLog struct {
	lastListRequest         accountspkg.ListAccountsRequest
	createRequest           accountspkg.ManualAccountCreateRequest
	importRequest           accountspkg.ImportAccountsRequest
	updateRequest           accountspkg.AccountUpdateRequest
	batchUpdateRequest      accountspkg.BatchUpdateRequest
	batchCPAUploadRequest   accountspkg.BatchCPAUploadRequest
	refreshOverviewRequest  accountspkg.OverviewRefreshRequest
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

func (f fakeAccountsService) CreateManualAccount(_ context.Context, req accountspkg.ManualAccountCreateRequest) (accountspkg.Account, error) {
	fakeAccountsServiceRequestLog.createRequest = req
	return f.createResponse, f.err
}

func (f fakeAccountsService) ImportAccounts(_ context.Context, req accountspkg.ImportAccountsRequest) (accountspkg.ImportAccountsResult, error) {
	fakeAccountsServiceRequestLog.importRequest = req
	return f.importResponse, f.err
}

func (f fakeAccountsService) UpdateAccount(_ context.Context, _ int, req accountspkg.AccountUpdateRequest) (accountspkg.Account, error) {
	fakeAccountsServiceRequestLog.updateRequest = req
	return f.updateResponse, f.err
}

func (f fakeAccountsService) DeleteAccount(context.Context, int) (accountspkg.ActionResponse, error) {
	return f.deleteResponse, f.err
}

func (f fakeAccountsService) BatchDeleteAccounts(context.Context, accountspkg.AccountSelectionRequest) (accountspkg.BatchDeleteResponse, error) {
	return f.batchDeleteResponse, f.err
}

func (f fakeAccountsService) BatchUpdateAccounts(_ context.Context, req accountspkg.BatchUpdateRequest) (accountspkg.BatchUpdateResponse, error) {
	fakeAccountsServiceRequestLog.batchUpdateRequest = req
	return f.batchUpdateResponse, f.err
}

func (f fakeAccountsService) ExportAccounts(context.Context, string, accountspkg.AccountSelectionRequest) (accountspkg.AccountExportResponse, error) {
	return f.exportResponse, f.err
}

func (f fakeAccountsService) BatchRefreshTokens(context.Context, accountspkg.BatchTokenRefreshRequest) (accountspkg.BatchRefreshResponse, error) {
	return f.batchRefreshResponse, f.err
}

func (f fakeAccountsService) RefreshAccountToken(context.Context, int, accountspkg.TokenRefreshRequest) (accountspkg.TokenRefreshActionResponse, error) {
	return f.refreshResponse, f.err
}

func (f fakeAccountsService) BatchValidateTokens(context.Context, accountspkg.BatchTokenValidateRequest) (accountspkg.BatchValidateResponse, error) {
	return f.batchValidateResponse, f.err
}

func (f fakeAccountsService) ValidateAccountToken(context.Context, int, accountspkg.TokenValidateRequest) (accountspkg.TokenValidateResponse, error) {
	return f.validateResponse, f.err
}

func (f fakeAccountsService) BatchUploadAccountsCPA(_ context.Context, req accountspkg.BatchCPAUploadRequest) (accountspkg.BatchUploadResponse, error) {
	fakeAccountsServiceRequestLog.batchCPAUploadRequest = req
	return f.batchCPAResponse, f.err
}

func (f fakeAccountsService) UploadAccountCPA(context.Context, int, accountspkg.CPAUploadRequest) (accountspkg.ActionResponse, error) {
	return f.uploadCPAResponse, f.err
}

func (f fakeAccountsService) BatchUploadAccountsSub2API(context.Context, accountspkg.BatchSub2APIUploadRequest) (accountspkg.BatchUploadResponse, error) {
	return f.batchSub2APIResponse, f.err
}

func (f fakeAccountsService) UploadAccountSub2API(context.Context, int, accountspkg.Sub2APIUploadRequest) (accountspkg.ActionResponse, error) {
	return f.uploadSub2APIResponse, f.err
}

func (f fakeAccountsService) BatchUploadAccountsTM(context.Context, accountspkg.BatchTMUploadRequest) (accountspkg.BatchUploadResponse, error) {
	return f.batchTMResponse, f.err
}

func (f fakeAccountsService) UploadAccountTM(context.Context, int, accountspkg.TMUploadRequest) (accountspkg.ActionResponse, error) {
	return f.uploadTMResponse, f.err
}

func (f fakeAccountsService) RemoveOverviewCards(context.Context, accountspkg.OverviewCardDeleteRequest) (accountspkg.OverviewCardMutationResponse, error) {
	return f.removeCardsResponse, f.err
}

func (f fakeAccountsService) RestoreOverviewCard(context.Context, int) (accountspkg.ActionResponse, error) {
	return f.restoreCardResponse, f.err
}

func (f fakeAccountsService) AttachOverviewCard(context.Context, int) (accountspkg.OverviewAttachResponse, error) {
	return f.attachCardResponse, f.err
}

func (f fakeAccountsService) RefreshOverview(_ context.Context, req accountspkg.OverviewRefreshRequest) (accountspkg.OverviewRefreshResponse, error) {
	fakeAccountsServiceRequestLog.refreshOverviewRequest = req
	return f.refreshOverviewResponse, f.err
}

func (f fakeAccountsService) GetInboxCode(context.Context, int) (accountspkg.InboxCodeResponse, error) {
	return f.inboxCodeResponse, f.err
}

func intPtr(value int) *int {
	return &value
}
