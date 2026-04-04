package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterHealthz(t *testing.T) {
	router := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouterUsesChiMux(t *testing.T) {
	router := NewRouter(nil)

	if router == nil {
		t.Fatal("expected non-nil chi router")
	}
}
