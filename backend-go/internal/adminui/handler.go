package adminui

import (
	"fmt"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
)

type HandlerOptions struct {
	BasePath              string
	AssetPaths            AssetPaths
	Settings              SettingsReader
	DefaultAccessPassword string
	CookieName            string
	CookieSecret          string
}

type Handler struct {
	auth     *AuthHelper
	renderer *Renderer
}

func NewHandler(opts HandlerOptions) (*Handler, error) {
	paths, err := resolveAssetPaths(opts.AssetPaths)
	if err != nil {
		return nil, err
	}
	auth := NewAuthHelper(AuthOptions{
		BasePath:              opts.BasePath,
		CookieName:            opts.CookieName,
		CookieSecret:          opts.CookieSecret,
		DefaultAccessPassword: opts.DefaultAccessPassword,
		Settings:              opts.Settings,
	})
	return &Handler{
		auth:     auth,
		renderer: newRenderer(paths),
	}, nil
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	staticPrefix := path.Clean(h.auth.StaticPath()) + "/"
	r.Handle(staticPrefix+"*", http.StripPrefix(staticPrefix, http.FileServer(http.Dir(h.renderer.StaticDir()))))
	r.Get(h.auth.LoginPath(), h.handleLoginPage)
	r.Post(h.auth.LoginPath(), h.handleLoginSubmit)
	r.Get(h.auth.LogoutPath(), h.handleLogout)
	r.Get(h.auth.BasePath(), h.handleProtected("index.html"))
	r.Get(h.auth.HomePath(), h.handleProtected("index.html"))

	pages := map[string]string{
		h.auth.BasePath() + "/accounts":          "accounts.html",
		h.auth.BasePath() + "/accounts-overview": "accounts_overview.html",
		h.auth.BasePath() + "/email-services":    "email_services.html",
		h.auth.BasePath() + "/payment":           "payment.html",
		h.auth.BasePath() + "/card-pool":         "card_pool.html",
		h.auth.BasePath() + "/auto-team":         "auto_team.html",
		h.auth.BasePath() + "/logs":              "logs.html",
		h.auth.BasePath() + "/settings":          "settings.html",
	}
	for route, templateName := range pages {
		r.Get(route, h.handleProtected(templateName))
	}
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	next := h.auth.SanitizeNextPath(r.URL.Query().Get("next"))
	if err := h.renderer.Render(w, "login.html", h.pageData(next, "")); err != nil {
		http.Error(w, fmt.Sprintf("render login: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	password, err := h.auth.AccessPassword(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("load access password: %v", err), http.StatusInternalServerError)
		return
	}
	next := h.auth.SanitizeNextPath(r.FormValue("next"))
	if password != stringsTrimSpace(r.FormValue("password")) {
		if err := h.renderer.RenderWithStatus(w, http.StatusUnauthorized, "login.html", h.pageData(next, "密码错误")); err != nil {
			http.Error(w, fmt.Sprintf("render login error: %v", err), http.StatusInternalServerError)
		}
		return
	}
	h.auth.SetAuthCookie(w, password)
	http.Redirect(w, r, next, http.StatusFound)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearAuthCookie(w)
	http.Redirect(w, r, h.auth.LoginPath(), http.StatusFound)
}

func (h *Handler) handleProtected(templateName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticated, err := h.auth.IsAuthenticated(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("check auth: %v", err), http.StatusInternalServerError)
			return
		}
		if !authenticated {
			http.Redirect(w, r, h.auth.LoginRedirect(r), http.StatusFound)
			return
		}
		if err := h.renderer.Render(w, templateName, h.pageData("", "")); err != nil {
			http.Error(w, fmt.Sprintf("render %s: %v", templateName, err), http.StatusInternalServerError)
		}
	}
}

func (h *Handler) pageData(next string, errorMessage string) PageData {
	return PageData{
		StaticVersion: h.renderer.StaticVersion(),
		StaticBase:    h.auth.StaticPath(),
		LoginPath:     h.auth.LoginPath(),
		LogoutPath:    h.auth.LogoutPath(),
		Next:          next,
		Error:         errorMessage,
	}
}
