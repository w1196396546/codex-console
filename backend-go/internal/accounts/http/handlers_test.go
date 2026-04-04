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
			Total: 2,
			Accounts: []accountspkg.Account{
				{ID: 11, Email: "alpha@example.com", Password: "secret-1", Status: "active"},
				{ID: 12, Email: "beta@example.com", Password: "", Status: "failed"},
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/accounts?page=1&page_size=10", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode accounts response: %v", err)
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
	if first["id"] != float64(11) || first["email"] != "alpha@example.com" || first["password"] != "secret-1" || first["status"] != "active" {
		t.Fatalf("unexpected first account payload: %#v", first)
	}
}

type fakeAccountsService struct {
	response accountspkg.AccountListResponse
	err      error
}

func (f fakeAccountsService) ListAccounts(context.Context, accountspkg.ListAccountsRequest) (accountspkg.AccountListResponse, error) {
	return f.response, f.err
}
