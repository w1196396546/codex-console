package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/adminui"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
)

type adminFrontendSettingsReader struct {
	items map[string]settings.SettingRecord
}

func (r adminFrontendSettingsReader) GetSettings(_ context.Context, keys []string) (map[string]settings.SettingRecord, error) {
	result := make(map[string]settings.SettingRecord, len(keys))
	for _, key := range keys {
		if item, ok := r.items[key]; ok {
			result[key] = item
		}
	}
	return result, nil
}

func TestAdminFrontendBaselineRoutes(t *testing.T) {
	handler, err := adminui.NewHandler(adminui.HandlerOptions{
		BasePath: "/go-admin",
		Settings: adminFrontendSettingsReader{
			items: map[string]settings.SettingRecord{
				adminui.DefaultAccessPasswordKey: {Key: adminui.DefaultAccessPasswordKey, Value: "admin123"},
			},
		},
	})
	if err != nil {
		t.Fatalf("new admin ui handler: %v", err)
	}

	server := httptest.NewServer(internalhttp.NewRouter(nil, handler))
	defer server.Close()

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{name: "login page", path: "/go-admin/login", status: http.StatusOK},
		{name: "home redirect", path: "/go-admin/", status: http.StatusFound},
		{name: "static css", path: "/go-admin/static/css/style.css", status: http.StatusOK},
		{name: "healthz", path: "/healthz", status: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := server.Client()
			if tc.status == http.StatusFound {
				client = &http.Client{
					Transport: server.Client().Transport,
					CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
			}
			resp, err := client.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("get %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("expected %d for %s, got %d", tc.status, tc.path, resp.StatusCode)
			}
			if tc.status == http.StatusFound {
				location := resp.Header.Get("Location")
				if location == "" {
					t.Fatalf("expected redirect location for %s", tc.path)
				}
				expected := fmt.Sprintf("/go-admin/login?next=%%2Fgo-admin%%2F")
				if location != expected {
					t.Fatalf("expected redirect location %q, got %q", expected, location)
				}
			}
		})
	}
}
