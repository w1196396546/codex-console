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
