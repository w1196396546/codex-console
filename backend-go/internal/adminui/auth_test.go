package adminui

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
)

type fakeSettingsReader struct {
	items map[string]settings.SettingRecord
	err   error
}

func (f fakeSettingsReader) GetSettings(_ context.Context, keys []string) (map[string]settings.SettingRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[string]settings.SettingRecord, len(keys))
	for _, key := range keys {
		if item, ok := f.items[key]; ok {
			result[key] = item
		}
	}
	return result, nil
}

func TestAuthHelperAccessPasswordFallsBackToDefault(t *testing.T) {
	helper := NewAuthHelper(AuthOptions{
		BasePath:              DefaultBasePath,
		DefaultAccessPassword: "fallback-value",
	})

	password, err := helper.AccessPassword(context.Background())
	if err != nil {
		t.Fatalf("access password: %v", err)
	}
	if password != "fallback-value" {
		t.Fatalf("expected fallback password, got %q", password)
	}
}

func TestAuthHelperReadsSettingRecord(t *testing.T) {
	helper := NewAuthHelper(AuthOptions{
		BasePath: DefaultBasePath,
		Settings: fakeSettingsReader{
			items: map[string]settings.SettingRecord{
				DefaultAccessPasswordKey: {Key: DefaultAccessPasswordKey, Value: "phase6-secret"},
			},
		},
	})

	password, err := helper.AccessPassword(context.Background())
	if err != nil {
		t.Fatalf("access password: %v", err)
	}
	if password != "phase6-secret" {
		t.Fatalf("expected settings password, got %q", password)
	}
}

func TestAuthHelperCookieLifecycle(t *testing.T) {
	helper := NewAuthHelper(AuthOptions{
		BasePath:     DefaultBasePath,
		CookieSecret: "cookie-secret",
		Settings: fakeSettingsReader{
			items: map[string]settings.SettingRecord{
				DefaultAccessPasswordKey: {Key: DefaultAccessPasswordKey, Value: "admin-pass"},
			},
		},
	})

	recorder := httptest.NewRecorder()
	helper.SetAuthCookie(recorder, "admin-pass")
	response := recorder.Result()
	if len(response.Cookies()) != 1 {
		t.Fatalf("expected one cookie, got %d", len(response.Cookies()))
	}

	request := httptest.NewRequest("GET", helper.LoginRedirect(nil), nil)
	request.AddCookie(response.Cookies()[0])
	authenticated, err := helper.IsAuthenticated(request)
	if err != nil {
		t.Fatalf("is authenticated: %v", err)
	}
	if !authenticated {
		t.Fatal("expected cookie to authenticate request")
	}

	clearRecorder := httptest.NewRecorder()
	helper.ClearAuthCookie(clearRecorder)
	clearCookies := clearRecorder.Result().Cookies()
	if len(clearCookies) != 1 || clearCookies[0].MaxAge != -1 {
		t.Fatalf("expected clearing cookie, got %#v", clearCookies)
	}
}

func TestAuthHelperSanitizeNextPath(t *testing.T) {
	helper := NewAuthHelper(AuthOptions{BasePath: DefaultBasePath})

	if got := helper.SanitizeNextPath("/go-admin/accounts"); got != "/go-admin/accounts" {
		t.Fatalf("expected go-admin path to pass through, got %q", got)
	}
	if got := helper.SanitizeNextPath("/"); got != "/go-admin/" {
		t.Fatalf("expected root path to collapse to home, got %q", got)
	}
	if got := helper.SanitizeNextPath("https://evil.example"); got != "/go-admin/" {
		t.Fatalf("expected external url to collapse to home, got %q", got)
	}
}
