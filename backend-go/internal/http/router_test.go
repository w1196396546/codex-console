package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
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

func TestRouterMountsManagementSlices(t *testing.T) {
	router := NewRouter(
		nil,
		accounts.NewService(nil),
		settings.NewService(settings.ServiceDependencies{}),
		emailservices.NewService(nil, nil),
		uploader.NewService(nil),
		logs.NewService(nil),
	)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "accounts", method: http.MethodGet, path: "/api/accounts"},
		{name: "settings", method: http.MethodGet, path: "/api/settings"},
		{name: "email services", method: http.MethodGet, path: "/api/email-services"},
		{name: "cpa services", method: http.MethodGet, path: "/api/cpa-services"},
		{name: "sub2api services", method: http.MethodGet, path: "/api/sub2api-services"},
		{name: "tm services", method: http.MethodGet, path: "/api/tm-services"},
		{name: "logs", method: http.MethodGet, path: "/api/logs"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)

			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("expected %s %s to be mounted, got 404", tc.method, tc.path)
			}
		})
	}
}

func TestRouterLeavesPhaseFourRoutesUnmounted(t *testing.T) {
	router := NewRouter(
		nil,
		accounts.NewService(nil),
		settings.NewService(settings.ServiceDependencies{}),
		emailservices.NewService(nil, nil),
		uploader.NewService(nil),
		logs.NewService(nil),
	)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "payment session bootstrap", method: http.MethodPost, path: "/api/payment/accounts/1/session-bootstrap"},
		{name: "team list", method: http.MethodGet, path: "/api/team/teams"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected %s %s to stay unmounted with 404, got %d", tc.method, tc.path, rec.Code)
			}
		})
	}
}
