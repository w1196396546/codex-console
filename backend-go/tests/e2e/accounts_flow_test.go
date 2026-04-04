package e2e_test

import (
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

type e2eAccountsService struct {
	response accountspkg.AccountListResponse
	err      error
}

func (s e2eAccountsService) ListAccounts(context.Context, accountspkg.ListAccountsRequest) (accountspkg.AccountListResponse, error) {
	return s.response, s.err
}
