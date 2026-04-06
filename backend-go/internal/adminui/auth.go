package adminui

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
)

const (
	DefaultBasePath          = "/go-admin"
	DefaultAccessPasswordKey = "webui.access_password"
	defaultAccessPassword    = "admin123"
	defaultCookieName        = "go_admin_auth"
	defaultCookieSecret      = "codex-console-go-admin-ui"
)

type SettingsReader interface {
	GetSettings(ctx context.Context, keys []string) (map[string]settings.SettingRecord, error)
}

type AuthOptions struct {
	BasePath              string
	CookieName            string
	CookieSecret          string
	DefaultAccessPassword string
	Settings              SettingsReader
}

type AuthHelper struct {
	basePath              string
	cookieName            string
	cookieSecret          string
	defaultAccessPassword string
	settings              SettingsReader
}

func NewAuthHelper(opts AuthOptions) *AuthHelper {
	basePath := strings.TrimRight(stringsTrimSpace(opts.BasePath), "/")
	if basePath == "" {
		basePath = DefaultBasePath
	}
	cookieName := stringsTrimSpace(opts.CookieName)
	if cookieName == "" {
		cookieName = defaultCookieName
	}
	cookieSecret := stringsTrimSpace(opts.CookieSecret)
	if cookieSecret == "" {
		cookieSecret = defaultCookieSecret
	}
	defaultPassword := stringsTrimSpace(opts.DefaultAccessPassword)
	if defaultPassword == "" {
		defaultPassword = defaultAccessPassword
	}
	return &AuthHelper{
		basePath:              basePath,
		cookieName:            cookieName,
		cookieSecret:          cookieSecret,
		defaultAccessPassword: defaultPassword,
		settings:              opts.Settings,
	}
}

func (h *AuthHelper) AccessPassword(ctx context.Context) (string, error) {
	if h.settings == nil {
		return h.defaultAccessPassword, nil
	}
	items, err := h.settings.GetSettings(ctx, []string{DefaultAccessPasswordKey})
	if err != nil {
		return "", err
	}
	if item, ok := items[DefaultAccessPasswordKey]; ok && stringsTrimSpace(item.Value) != "" {
		return stringsTrimSpace(item.Value), nil
	}
	return h.defaultAccessPassword, nil
}

func (h *AuthHelper) CookieName() string {
	return h.cookieName
}

func (h *AuthHelper) BasePath() string {
	return h.basePath
}

func (h *AuthHelper) LoginPath() string {
	return h.basePath + "/login"
}

func (h *AuthHelper) LogoutPath() string {
	return h.basePath + "/logout"
}

func (h *AuthHelper) HomePath() string {
	return h.basePath + "/"
}

func (h *AuthHelper) StaticPath() string {
	return h.basePath + "/static"
}

func (h *AuthHelper) LoginRedirect(r *http.Request) string {
	if r == nil {
		return h.LoginPath()
	}
	return h.LoginPath() + "?next=" + url.QueryEscape(h.SanitizeNextPath(r.URL.RequestURI()))
}

func (h *AuthHelper) SanitizeNextPath(next string) string {
	next = stringsTrimSpace(next)
	if next == "" || next == "/" {
		return h.HomePath()
	}
	if !strings.HasPrefix(next, h.basePath) {
		return h.HomePath()
	}
	return next
}

func (h *AuthHelper) SetAuthCookie(w http.ResponseWriter, password string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    h.sign(password),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHelper) ClearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHelper) IsAuthenticated(r *http.Request) (bool, error) {
	password, err := h.AccessPassword(r.Context())
	if err != nil {
		return false, err
	}
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		return false, nil
	}
	expected := h.sign(password)
	return hmac.Equal([]byte(cookie.Value), []byte(expected)), nil
}

func (h *AuthHelper) sign(password string) string {
	mac := hmac.New(sha256.New, []byte(h.cookieSecret))
	mac.Write([]byte(stringsTrimSpace(password)))
	return hex.EncodeToString(mac.Sum(nil))
}
