package e2e_test

import (
	"context"
	"bytes"
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

func TestRecentAccountsManagementCompatibilityEndpoints(t *testing.T) {
	server := httptest.NewServer(internalhttp.NewRouter(nil, e2eAccountsService{
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

type e2eAccountsService struct {
	response       accountspkg.AccountListResponse
	createResponse accountspkg.Account
	exportResponse accountspkg.AccountExportResponse
	err            error
}

func (s e2eAccountsService) ListAccounts(context.Context, accountspkg.ListAccountsRequest) (accountspkg.AccountListResponse, error) {
	return s.response, s.err
}

func (s e2eAccountsService) CreateManualAccount(context.Context, accountspkg.ManualAccountCreateRequest) (accountspkg.Account, error) {
	return s.createResponse, s.err
}

func (s e2eAccountsService) ExportAccounts(context.Context, string, accountspkg.AccountSelectionRequest) (accountspkg.AccountExportResponse, error) {
	return s.exportResponse, s.err
}
