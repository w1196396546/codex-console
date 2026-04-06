package adminui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminUIBaselineCopiedAssetsExist(t *testing.T) {
	paths, err := resolveAssetPaths(AssetPaths{})
	if err != nil {
		t.Fatalf("resolve asset paths: %v", err)
	}

	required := []string{
		filepath.Join(paths.TemplatesDir, "login.html"),
		filepath.Join(paths.TemplatesDir, "index.html"),
		filepath.Join(paths.TemplatesDir, "partials", "site_notice.html"),
		filepath.Join(paths.StaticDir, "css", "style.css"),
		filepath.Join(paths.StaticDir, "js", "app.js"),
		filepath.Join(paths.StaticDir, "js", "settings.js"),
	}
	for _, target := range required {
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected copied asset %s: %v", target, err)
		}
	}

	loginTemplate, err := os.ReadFile(filepath.Join(paths.TemplatesDir, "login.html"))
	if err != nil {
		t.Fatalf("read login template: %v", err)
	}
	if !strings.Contains(string(loginTemplate), `action="/go-admin/login"`) {
		t.Fatalf("expected go-admin login action in copied template, got %s", string(loginTemplate))
	}
}

func TestAdminUITemplatesUseAdminShellWithoutPromoCopy(t *testing.T) {
	paths, err := resolveAssetPaths(AssetPaths{})
	if err != nil {
		t.Fatalf("resolve asset paths: %v", err)
	}

	indexTemplate, err := os.ReadFile(filepath.Join(paths.TemplatesDir, "index.html"))
	if err != nil {
		t.Fatalf("read index template: %v", err)
	}
	indexContent := string(indexTemplate)
	for _, marker := range []string{
		`{% include "partials/admin_sidebar.html" %}`,
		`{% include "partials/admin_topbar.html" %}`,
		`class="admin-shell__workspace"`,
	} {
		if !strings.Contains(indexContent, marker) {
			t.Fatalf("expected index template to include %q", marker)
		}
	}
	if strings.Contains(indexContent, `partials/site_notice.html`) {
		t.Fatalf("did not expect legacy site notice in index template")
	}

	loginTemplate, err := os.ReadFile(filepath.Join(paths.TemplatesDir, "login.html"))
	if err != nil {
		t.Fatalf("read login template: %v", err)
	}
	loginContent := string(loginTemplate)
	for _, marker := range []string{
		`class="admin-login-shell"`,
		`class="admin-login-card card"`,
		`action="/go-admin/login"`,
	} {
		if !strings.Contains(loginContent, marker) {
			t.Fatalf("expected login template to include %q", marker)
		}
	}
	if strings.Contains(loginContent, `partials/site_notice.html`) {
		t.Fatalf("did not expect legacy site notice in login template")
	}

	noticeTemplate, err := os.ReadFile(filepath.Join(paths.TemplatesDir, "partials", "site_notice.html"))
	if err != nil {
		t.Fatalf("read site notice template: %v", err)
	}
	noticeContent := string(noticeTemplate)
	for _, forbidden := range []string{"GitHub", "Telegram", "爱发电", "赞助", "开源"} {
		if strings.Contains(noticeContent, forbidden) {
			t.Fatalf("did not expect %q in neutralized site notice", forbidden)
		}
	}
}
