package adminui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/go-chi/chi/v5"
)

func TestHandlerServesLoginAndHome(t *testing.T) {
	staticRoot := t.TempDir()
	cssDir := filepath.Join(staticRoot, "css")
	if err := os.MkdirAll(cssDir, 0o755); err != nil {
		t.Fatalf("mkdir css: %v", err)
	}
	styleFile := filepath.Join(cssDir, "style.css")
	if err := os.WriteFile(styleFile, []byte("body{background:#fff;}"), 0o644); err != nil {
		t.Fatalf("write style: %v", err)
	}
	mtime := time.Unix(1710000000, 0)
	if err := os.Chtimes(styleFile, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	handler, err := NewHandler(HandlerOptions{
		BasePath: "/go-admin",
		AssetPaths: AssetPaths{
			BackendRoot:  filepath.Dir(filepath.Dir(staticRoot)),
			TemplatesDir: filepath.Join(t.TempDir(), "templates"),
			StaticDir:    staticRoot,
		},
		Settings: fakeSettingsReader{},
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	loginRequest := httptest.NewRequest(http.MethodGet, "/go-admin/login?next=/go-admin/", nil)
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login page 200, got %d", loginRecorder.Code)
	}
	if !strings.Contains(loginRecorder.Body.String(), `action="/go-admin/login"`) {
		t.Fatalf("expected go-admin login action, got %s", loginRecorder.Body.String())
	}
	if !strings.Contains(loginRecorder.Body.String(), "1710000000") {
		t.Fatalf("expected static version in login page, got %s", loginRecorder.Body.String())
	}

	homeRequest := httptest.NewRequest(http.MethodGet, "/go-admin/", nil)
	homeRecorder := httptest.NewRecorder()
	router.ServeHTTP(homeRecorder, homeRequest)
	if homeRecorder.Code != http.StatusFound {
		t.Fatalf("expected redirect without cookie, got %d", homeRecorder.Code)
	}
	location := homeRecorder.Result().Header.Get("Location")
	expectedNext := url.QueryEscape("/go-admin/")
	if location != "/go-admin/login?next="+expectedNext {
		t.Fatalf("expected redirect to login with next, got %q", location)
	}

	form := strings.NewReader("password=admin123&next=%2Fgo-admin%2F")
	authRequest := httptest.NewRequest(http.MethodPost, "/go-admin/login", form)
	authRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authRecorder := httptest.NewRecorder()
	router.ServeHTTP(authRecorder, authRequest)
	if authRecorder.Code != http.StatusFound {
		t.Fatalf("expected successful login redirect, got %d", authRecorder.Code)
	}
	cookies := authRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one auth cookie, got %d", len(cookies))
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/go-admin/", nil)
	authorizedRequest.AddCookie(cookies[0])
	authorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(authorizedRecorder, authorizedRequest)
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("expected authenticated home 200, got %d", authorizedRecorder.Code)
	}
	if !strings.Contains(authorizedRecorder.Body.String(), "Go Admin Frontend") {
		t.Fatalf("expected admin home title, got %s", authorizedRecorder.Body.String())
	}
	if !strings.Contains(authorizedRecorder.Body.String(), "/go-admin/static") {
		t.Fatalf("expected shared static path in home page, got %s", authorizedRecorder.Body.String())
	}
}

func TestHandlerServesStaticFilesAndRejectsBadPassword(t *testing.T) {
	staticRoot := t.TempDir()
	jsDir := filepath.Join(staticRoot, "js")
	if err := os.MkdirAll(jsDir, 0o755); err != nil {
		t.Fatalf("mkdir js: %v", err)
	}
	jsFile := filepath.Join(jsDir, "app.js")
	if err := os.WriteFile(jsFile, []byte("console.log('phase6');"), 0o644); err != nil {
		t.Fatalf("write js: %v", err)
	}

	handler, err := NewHandler(HandlerOptions{
		BasePath: "/go-admin",
		AssetPaths: AssetPaths{
			TemplatesDir: filepath.Join(t.TempDir(), "templates"),
			StaticDir:    staticRoot,
		},
		Settings: fakeSettingsReader{
			items: map[string]settings.SettingRecord{
				DefaultAccessPasswordKey: {Key: DefaultAccessPasswordKey, Value: "phase6-pass"},
			},
		},
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	staticRequest := httptest.NewRequest(http.MethodGet, "/go-admin/static/js/app.js", nil)
	staticRecorder := httptest.NewRecorder()
	router.ServeHTTP(staticRecorder, staticRequest)
	if staticRecorder.Code != http.StatusOK {
		t.Fatalf("expected static file 200, got %d", staticRecorder.Code)
	}
	if !strings.Contains(staticRecorder.Body.String(), "phase6") {
		t.Fatalf("expected static asset body, got %s", staticRecorder.Body.String())
	}

	form := strings.NewReader("password=wrong&next=%2Fgo-admin%2F")
	loginRequest := httptest.NewRequest(http.MethodPost, "/go-admin/login", form)
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized on wrong password, got %d", loginRecorder.Code)
	}
	if !strings.Contains(loginRecorder.Body.String(), "密码错误") {
		t.Fatalf("expected password error, got %s", loginRecorder.Body.String())
	}
}
