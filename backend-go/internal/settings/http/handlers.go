package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/go-chi/chi/v5"
)

type settingsService interface {
	GetAllSettings(ctx context.Context) (settings.AllSettingsResponse, error)
	GetRegistrationSettings(ctx context.Context) (settings.RegistrationSettingsResponse, error)
	UpdateRegistrationSettings(ctx context.Context, req settings.UpdateRegistrationSettingsRequest) (settings.MutationResponse, error)
	GetTempmailSettings(ctx context.Context) (settings.TempmailSettingsResponse, error)
	UpdateTempmailSettings(ctx context.Context, req settings.UpdateTempmailSettingsRequest) (settings.MutationResponse, error)
	GetEmailCodeSettings(ctx context.Context) (settings.EmailCodeSettingsResponse, error)
	UpdateEmailCodeSettings(ctx context.Context, req settings.UpdateEmailCodeSettingsRequest) (settings.MutationResponse, error)
	GetOutlookSettings(ctx context.Context) (settings.OutlookSettingsResponse, error)
	UpdateOutlookSettings(ctx context.Context, req settings.UpdateOutlookSettingsRequest) (settings.MutationResponse, error)
	UpdateWebUISettings(ctx context.Context, req settings.UpdateWebUISettingsRequest) (settings.MutationResponse, error)
	GetDynamicProxySettings(ctx context.Context) (settings.DynamicProxySettingsResponse, error)
	UpdateDynamicProxySettings(ctx context.Context, req settings.UpdateDynamicProxySettingsRequest) (settings.MutationResponse, error)
	TestDynamicProxy(ctx context.Context, req settings.UpdateDynamicProxySettingsRequest) (settings.DynamicProxyTestResponse, error)
	ListProxies(ctx context.Context, enabled *bool) (settings.ProxyListResponse, error)
	CreateProxy(ctx context.Context, req settings.CreateProxyRequest) (settings.ProxyPayload, error)
	GetProxy(ctx context.Context, proxyID int, includePassword bool) (settings.ProxyPayload, error)
	UpdateProxy(ctx context.Context, proxyID int, req settings.UpdateProxyRequest) (settings.ProxyPayload, error)
	DeleteProxy(ctx context.Context, proxyID int) (settings.MutationResponse, error)
	EnableProxy(ctx context.Context, proxyID int) (settings.MutationResponse, error)
	DisableProxy(ctx context.Context, proxyID int) (settings.MutationResponse, error)
	SetProxyDefault(ctx context.Context, proxyID int) (settings.ProxyPayload, error)
	TestProxy(ctx context.Context, proxyID int) (settings.ProxyTestResponse, error)
	TestAllProxies(ctx context.Context) (settings.ProxyTestAllResponse, error)
	GetDatabaseInfo(ctx context.Context) (settings.DatabaseInfoResponse, error)
	BackupDatabase(ctx context.Context) (settings.DatabaseBackupResponse, error)
	ImportDatabase(ctx context.Context, req settings.DatabaseImportRequest) (settings.DatabaseImportResponse, error)
	CleanupDatabase(ctx context.Context, req settings.DatabaseCleanupRequest) (settings.DatabaseCleanupResponse, error)
}

type Handler struct {
	service settingsService
}

func NewHandler(service settingsService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/settings", func(r chi.Router) {
		r.Get("/", h.GetAllSettings)
		r.Get("/registration", h.GetRegistrationSettings)
		r.Post("/registration", h.UpdateRegistrationSettings)
		r.Get("/tempmail", h.GetTempmailSettings)
		r.Post("/tempmail", h.UpdateTempmailSettings)
		r.Get("/email-code", h.GetEmailCodeSettings)
		r.Post("/email-code", h.UpdateEmailCodeSettings)
		r.Get("/outlook", h.GetOutlookSettings)
		r.Post("/outlook", h.UpdateOutlookSettings)
		r.Post("/webui", h.UpdateWebUISettings)
		r.Get("/proxy/dynamic", h.GetDynamicProxySettings)
		r.Post("/proxy/dynamic", h.UpdateDynamicProxySettings)
		r.Post("/proxy/dynamic/test", h.TestDynamicProxy)
		r.Get("/proxies", h.ListProxies)
		r.Post("/proxies", h.CreateProxy)
		r.Get("/proxies/{proxy_id}", h.GetProxy)
		r.Patch("/proxies/{proxy_id}", h.UpdateProxy)
		r.Delete("/proxies/{proxy_id}", h.DeleteProxy)
		r.Post("/proxies/{proxy_id}/set-default", h.SetProxyDefault)
		r.Post("/proxies/{proxy_id}/test", h.TestProxy)
		r.Post("/proxies/test-all", h.TestAllProxies)
		r.Post("/proxies/{proxy_id}/enable", h.EnableProxy)
		r.Post("/proxies/{proxy_id}/disable", h.DisableProxy)
		r.Get("/database", h.GetDatabaseInfo)
		r.Post("/database/backup", h.BackupDatabase)
		r.Post("/database/import", h.ImportDatabase)
		r.Post("/database/cleanup", h.CleanupDatabase)
	})
}

func (h *Handler) GetAllSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetAllSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetRegistrationSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetRegistrationSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateRegistrationSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateRegistrationSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateRegistrationSettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetTempmailSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetTempmailSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateTempmailSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateTempmailSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateTempmailSettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetEmailCodeSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetEmailCodeSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateEmailCodeSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateEmailCodeSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateEmailCodeSettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetOutlookSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetOutlookSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateOutlookSettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateOutlookSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateOutlookSettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateWebUISettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateWebUISettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateWebUISettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetDynamicProxySettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetDynamicProxySettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateDynamicProxySettings(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateDynamicProxySettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateDynamicProxySettings(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) TestDynamicProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.UpdateDynamicProxySettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.TestDynamicProxy(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) ListProxies(w nethttp.ResponseWriter, r *nethttp.Request) {
	enabled, err := parseOptionalBool(r.URL.Query().Get("enabled"))
	if err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid enabled")
		return
	}
	resp, err := h.service.ListProxies(r.Context(), enabled)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) CreateProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req settings.CreateProxyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	proxy, err := h.service.CreateProxy(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, map[string]any{"success": true, "proxy": proxy})
}

func (h *Handler) GetProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetProxy(r.Context(), proxyID, true)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	var req settings.UpdateProxyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid json")
		return
	}
	proxy, err := h.service.UpdateProxy(r.Context(), proxyID, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, map[string]any{"success": true, "proxy": proxy})
}

func (h *Handler) DeleteProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DeleteProxy(r.Context(), proxyID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) SetProxyDefault(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	proxy, err := h.service.SetProxyDefault(r.Context(), proxyID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, map[string]any{"success": true, "proxy": proxy})
}

func (h *Handler) TestProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.TestProxy(r.Context(), proxyID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) TestAllProxies(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.TestAllProxies(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) EnableProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.EnableProxy(r.Context(), proxyID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) DisableProxy(w nethttp.ResponseWriter, r *nethttp.Request) {
	proxyID, ok := parseProxyID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DisableProxy(r.Context(), proxyID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetDatabaseInfo(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetDatabaseInfo(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) BackupDatabase(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.BackupDatabase(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) ImportDatabase(w nethttp.ResponseWriter, r *nethttp.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "缺少导入文件")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "读取导入文件失败")
		return
	}

	resp, err := h.service.ImportDatabase(r.Context(), settings.DatabaseImportRequest{
		Filename: header.Filename,
		Content:  content,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) CleanupDatabase(w nethttp.ResponseWriter, r *nethttp.Request) {
	days := 30
	if rawDays := r.URL.Query().Get("days"); rawDays != "" {
		parsedDays, err := strconv.Atoi(rawDays)
		if err != nil {
			writeDetailError(w, nethttp.StatusBadRequest, "invalid days")
			return
		}
		days = parsedDays
	}

	keepFailed := true
	if rawKeepFailed := r.URL.Query().Get("keep_failed"); rawKeepFailed != "" {
		parsedKeepFailed, err := strconv.ParseBool(rawKeepFailed)
		if err != nil {
			writeDetailError(w, nethttp.StatusBadRequest, "invalid keep_failed")
			return
		}
		keepFailed = parsedKeepFailed
	}

	resp, err := h.service.CleanupDatabase(r.Context(), settings.DatabaseCleanupRequest{
		Days:       days,
		KeepFailed: keepFailed,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func parseProxyID(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	proxyID, err := strconv.Atoi(chi.URLParam(r, "proxy_id"))
	if err != nil {
		writeDetailError(w, nethttp.StatusBadRequest, "invalid proxy_id")
		return 0, false
	}
	return proxyID, true
}

func parseOptionalBool(raw string) (*bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func decodeJSON(r *nethttp.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeDetailError(w nethttp.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func writeServiceError(w nethttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, settings.ErrProxyNotFound):
		writeDetailError(w, nethttp.StatusNotFound, "代理不存在")
	case errors.Is(err, settings.ErrInvalidProxyName),
		errors.Is(err, settings.ErrInvalidProxyHost),
		errors.Is(err, settings.ErrInvalidProxyType),
		errors.Is(err, settings.ErrInvalidProxyPort),
		errors.Is(err, settings.ErrInvalidRegistrationFlow),
		errors.Is(err, settings.ErrInvalidEmailCodeTimeout),
		errors.Is(err, settings.ErrInvalidEmailCodePollPeriod):
		writeDetailError(w, nethttp.StatusBadRequest, err.Error())
	default:
		writeDetailError(w, nethttp.StatusInternalServerError, err.Error())
	}
}
