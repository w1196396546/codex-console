package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/adminui"
	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/dou-jiang/codex-console/backend-go/internal/team"
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

func TestRouterMountsPhaseFourRoutes(t *testing.T) {
	router := NewRouter(
		nil,
		accounts.NewService(nil),
		settings.NewService(settings.ServiceDependencies{}),
		emailservices.NewService(nil, nil),
		uploader.NewService(nil),
		logs.NewService(nil),
		payment.NewService(nil, nil),
		team.NewService(nil, nil),
		team.NewTaskService(nil, nil, nil, nil),
	)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "payment generate-link path", method: http.MethodPost, path: "/api/payment/generate-link"},
		{name: "payment session bootstrap path", method: http.MethodGet, path: "/api/payment/accounts/1/session-bootstrap"},
		{name: "team list path", method: http.MethodPost, path: "/api/team/teams"},
		{name: "team sync-batch path", method: http.MethodPost, path: "/api/team/teams/sync-batch"},
		{name: "team task list path", method: http.MethodPost, path: "/api/team/tasks"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)

			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("expected %s %s to be mounted after Phase 4 wiring, got 404", tc.method, tc.path)
			}
		})
	}
}

func TestRouterMountsAdminUIRoutes(t *testing.T) {
	handler, err := adminui.NewHandler(adminui.HandlerOptions{})
	if err != nil {
		t.Fatalf("new admin ui handler: %v", err)
	}
	router := NewRouter(nil, handler)

	tests := []struct {
		name       string
		path       string
		wantAbsent bool
	}{
		{name: "login", path: "/go-admin/login"},
		{name: "home redirect", path: "/go-admin/"},
		{name: "static asset", path: "/go-admin/static/css/style.css"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("expected %s to be mounted, got 404", tc.path)
			}
		})
	}
}
