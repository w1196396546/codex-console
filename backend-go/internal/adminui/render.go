package adminui

import (
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	includePattern = regexp.MustCompile(`\{%\s*include\s+"([^"]+)"\s*%\}`)
	errorIfPattern = regexp.MustCompile(`(?s)\{%\s*if error\s*%\}(.*?)\{%\s*endif\s*%\}`)
)

const (
	fallbackLoginTemplate = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Go Admin Frontend</title><link rel="stylesheet" href="{{ static_base }}/css/style.css?v={{ static_version }}"></head><body><main><h1>Go Admin Frontend</h1>{% if error %}<div>{{ error }}</div>{% endif %}<form method="post" action="{{ login_path }}"><input type="hidden" name="next" value="{{ next }}"><input type="password" name="password"><button type="submit">登录</button></form></main></body></html>`
	fallbackIndexTemplate = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Go Admin Frontend</title><link rel="stylesheet" href="{{ static_base }}/css/style.css?v={{ static_version }}"></head><body><main><h1>Go Admin Frontend</h1><p>Phase 6 baseline is running.</p><a href="{{ logout_path }}">退出</a></main></body></html>`
)

type PageData struct {
	StaticVersion string
	StaticBase    string
	LoginPath     string
	LogoutPath    string
	Error         string
	Next          string
}

type Renderer struct {
	paths         AssetPaths
	staticVersion string
}

func newRenderer(paths AssetPaths) *Renderer {
	return &Renderer{
		paths:         paths,
		staticVersion: buildStaticAssetVersion(paths.StaticDir),
	}
}

func (r *Renderer) StaticDir() string {
	return r.paths.StaticDir
}

func (r *Renderer) StaticVersion() string {
	return r.staticVersion
}

func buildStaticAssetVersion(staticDir string) string {
	latestMtime := int64(0)
	_ = filepath.WalkDir(staticDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if modified := info.ModTime().Unix(); modified > latestMtime {
			latestMtime = modified
		}
		return nil
	})
	if latestMtime == 0 {
		return "1"
	}
	return fmt.Sprintf("%d", latestMtime)
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data PageData) error {
	body, err := r.renderTemplate(name, data)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = w.Write(body)
	return err
}

func (r *Renderer) RenderWithStatus(w http.ResponseWriter, status int, name string, data PageData) error {
	body, err := r.renderTemplate(name, data)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}

func (r *Renderer) renderTemplate(name string, data PageData) ([]byte, error) {
	content, err := r.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	replacer := strings.NewReplacer(
		"{{ static_version }}", data.StaticVersion,
		"{{ static_base }}", data.StaticBase,
		"{{ login_path }}", data.LoginPath,
		"{{ logout_path }}", data.LogoutPath,
		"{{ next }}", html.EscapeString(data.Next),
		"{{ error }}", html.EscapeString(data.Error),
	)
	content = replacer.Replace(content)
	content = applyErrorBlock(content, data.Error)
	return []byte(content), nil
}

func (r *Renderer) loadTemplate(name string) (string, error) {
	path := filepath.Join(r.paths.TemplatesDir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		switch name {
		case "login.html":
			return fallbackLoginTemplate, nil
		case "index.html":
			return fallbackIndexTemplate, nil
		default:
			return "", fmt.Errorf("read template %s: %w", name, err)
		}
	}
	content := string(raw)
	return includePattern.ReplaceAllStringFunc(content, func(match string) string {
		submatches := includePattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return ""
		}
		partial, partialErr := os.ReadFile(filepath.Join(r.paths.TemplatesDir, filepath.Clean(submatches[1])))
		if partialErr != nil {
			return ""
		}
		return string(partial)
	}), nil
}

func applyErrorBlock(content string, errorMessage string) string {
	return errorIfPattern.ReplaceAllStringFunc(content, func(match string) string {
		submatches := errorIfPattern.FindStringSubmatch(match)
		if len(submatches) < 2 || stringsTrimSpace(errorMessage) == "" {
			return ""
		}
		return submatches[1]
	})
}
