package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	"github.com/go-chi/chi/v5"
)

type logsService interface {
	ListLogs(ctx context.Context, req logs.ListLogsRequest) (logs.ListLogsResponse, error)
	GetStats(ctx context.Context) (logs.StatsResponse, error)
	CleanupLogs(ctx context.Context, req logs.CleanupRequest) (logs.CleanupResult, error)
	ClearLogs(ctx context.Context, confirm bool) (logs.ClearResult, error)
}

type Handler struct {
	service logsService
}

func NewHandler(service logsService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/logs", h.ListLogs)
	r.Get("/api/logs/stats", h.GetStats)
	r.Post("/api/logs/cleanup", h.CleanupLogs)
	r.Delete("/api/logs", h.ClearLogs)
}

func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
	req, err := decodeListLogsRequest(r)
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.service.ListLogs(r.Context(), req)
	if err != nil {
		writeDetailError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetStats(r.Context())
	if err != nil {
		writeDetailError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CleanupLogs(w http.ResponseWriter, r *http.Request) {
	req, err := decodeCleanupRequest(r)
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.service.CleanupLogs(r.Context(), req)
	if err != nil {
		writeDetailError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"retention_days":   resp.RetentionDays,
		"max_rows":         resp.MaxRows,
		"deleted_by_age":   resp.DeletedByAge,
		"deleted_by_limit": resp.DeletedByLimit,
		"deleted_total":    resp.DeletedTotal,
		"remaining":        resp.Remaining,
	})
}

func (h *Handler) ClearLogs(w http.ResponseWriter, r *http.Request) {
	confirm := r.URL.Query().Get("confirm") == "true"
	resp, err := h.service.ClearLogs(r.Context(), confirm)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, logs.ErrClearLogsConfirmationRequired) {
			status = http.StatusBadRequest
		}
		writeDetailError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"deleted_total": resp.DeletedTotal,
		"remaining":     resp.Remaining,
	})
}

func decodeListLogsRequest(r *http.Request) (logs.ListLogsRequest, error) {
	query := r.URL.Query()
	req := logs.ListLogsRequest{
		Level:      query.Get("level"),
		LoggerName: query.Get("logger_name"),
		Keyword:    query.Get("keyword"),
	}

	if rawPage := query.Get("page"); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil {
			return logs.ListLogsRequest{}, err
		}
		if page < 1 {
			return logs.ListLogsRequest{}, errors.New("page 必须大于等于 1")
		}
		req.Page = page
	}
	if rawPageSize := query.Get("page_size"); rawPageSize != "" {
		pageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			return logs.ListLogsRequest{}, err
		}
		if pageSize < 1 || pageSize > 500 {
			return logs.ListLogsRequest{}, errors.New("page_size 必须在 1 到 500 之间")
		}
		req.PageSize = pageSize
	}
	if rawSinceMinutes := query.Get("since_minutes"); rawSinceMinutes != "" {
		sinceMinutes, err := strconv.Atoi(rawSinceMinutes)
		if err != nil {
			return logs.ListLogsRequest{}, err
		}
		if sinceMinutes < 1 || sinceMinutes > 10080 {
			return logs.ListLogsRequest{}, errors.New("since_minutes 必须在 1 到 10080 之间")
		}
		req.SinceMinutes = sinceMinutes
	}

	return req, nil
}

func decodeCleanupRequest(r *http.Request) (logs.CleanupRequest, error) {
	var req logs.CleanupRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return logs.CleanupRequest{}, err
		}
	}

	if req.RetentionDays != nil {
		retention := *req.RetentionDays
		if retention < 1 || retention > 3650 {
			return logs.CleanupRequest{}, errors.New("retention_days 必须在 1 到 3650 之间")
		}
	}
	if req.MaxRows != 0 && (req.MaxRows < 1000 || req.MaxRows > 5000000) {
		return logs.CleanupRequest{}, errors.New("max_rows 必须在 1000 到 5000000 之间")
	}

	return req, nil
}

func writeDetailError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
