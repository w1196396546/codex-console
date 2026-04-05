package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
	"github.com/go-chi/chi/v5"
)

type service interface {
	GetRandomBillingProfile(ctx context.Context, country string, proxy string) (payment.RandomBillingResponse, error)
	GetAccountSessionDiagnostic(ctx context.Context, accountID int, probe bool, proxy string) (payment.SessionDiagnosticResponse, error)
	BootstrapAccountSessionToken(ctx context.Context, accountID int, proxy string) (payment.SessionBootstrapResponse, error)
	SaveAccountSessionToken(ctx context.Context, accountID int, req payment.SaveSessionTokenRequest) (payment.SaveSessionTokenResponse, error)
	GeneratePaymentLink(ctx context.Context, req payment.GenerateLinkRequest) (payment.GenerateLinkResponse, error)
	OpenBrowserIncognito(ctx context.Context, req payment.OpenIncognitoRequest) (payment.OpenIncognitoResponse, error)
	CreateBindCardTask(ctx context.Context, req payment.CreateBindCardTaskRequest) (payment.CreateBindCardTaskResponse, error)
	ListBindCardTasks(ctx context.Context, req payment.ListBindCardTasksRequest) (payment.ListBindCardTasksResponse, error)
	OpenBindCardTask(ctx context.Context, taskID int) (payment.BindCardTaskActionResponse, error)
	AutoBindBindCardTaskThirdParty(ctx context.Context, taskID int, req payment.ThirdPartyAutoBindRequest) (payment.AutoBindResult, error)
	AutoBindBindCardTaskLocal(ctx context.Context, taskID int, req payment.LocalAutoBindRequest) (payment.AutoBindResult, error)
	MarkBindCardTaskUserAction(ctx context.Context, taskID int, req payment.MarkUserActionRequest) (payment.SyncBindCardTaskResponse, error)
	SyncBindCardTaskSubscription(ctx context.Context, taskID int, req payment.SyncBindCardTaskRequest) (payment.SyncBindCardTaskResponse, error)
	DeleteBindCardTask(ctx context.Context, taskID int) (payment.DeleteBindCardTaskResponse, error)
	MarkSubscription(ctx context.Context, accountID int, req payment.MarkSubscriptionRequest) (payment.MarkSubscriptionResponse, error)
	BatchCheckSubscription(ctx context.Context, req payment.BatchCheckSubscriptionRequest) (payment.BatchCheckSubscriptionResponse, error)
}

type Handler struct {
	service service
}

func NewHandler(service service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/payment/random-billing", h.GetRandomBillingProfile)
	r.Post("/api/payment/generate-link", h.GeneratePaymentLink)
	r.Post("/api/payment/open-incognito", h.OpenBrowserIncognito)

	r.Get("/api/payment/accounts/{account_id}/session-diagnostic", h.GetAccountSessionDiagnostic)
	r.Post("/api/payment/accounts/{account_id}/session-bootstrap", h.BootstrapAccountSessionToken)
	r.Post("/api/payment/accounts/{account_id}/session-token", h.SaveAccountSessionToken)
	r.Post("/api/payment/accounts/{account_id}/mark-subscription", h.MarkSubscription)
	r.Post("/api/payment/accounts/batch-check-subscription", h.BatchCheckSubscription)

	r.Post("/api/payment/bind-card/tasks", h.CreateBindCardTask)
	r.Get("/api/payment/bind-card/tasks", h.ListBindCardTasks)
	r.Delete("/api/payment/bind-card/tasks/{task_id}", h.DeleteBindCardTask)
	r.Post("/api/payment/bind-card/tasks/{task_id}/open", h.OpenBindCardTask)
	r.Post("/api/payment/bind-card/tasks/{task_id}/auto-bind-local", h.AutoBindBindCardTaskLocal)
	r.Post("/api/payment/bind-card/tasks/{task_id}/auto-bind-third-party", h.AutoBindBindCardTaskThirdParty)
	r.Post("/api/payment/bind-card/tasks/{task_id}/mark-user-action", h.MarkBindCardTaskUserAction)
	r.Post("/api/payment/bind-card/tasks/{task_id}/sync-subscription", h.SyncBindCardTaskSubscription)
}

func (h *Handler) GetRandomBillingProfile(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetRandomBillingProfile(r.Context(), r.URL.Query().Get("country"), r.URL.Query().Get("proxy"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GeneratePaymentLink(w http.ResponseWriter, r *http.Request) {
	var req payment.GenerateLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.GeneratePaymentLink(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) OpenBrowserIncognito(w http.ResponseWriter, r *http.Request) {
	var req payment.OpenIncognitoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.URL == "" {
		writeDetailError(w, http.StatusBadRequest, "URL 不能为空")
		return
	}
	resp, err := h.service.OpenBrowserIncognito(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountSessionDiagnostic(w http.ResponseWriter, r *http.Request) {
	accountID, err := parseIntParam(r, "account_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	probe := true
	if value := r.URL.Query().Get("probe"); value != "" {
		probe = value != "0" && value != "false"
	}
	resp, err := h.service.GetAccountSessionDiagnostic(r.Context(), accountID, probe, r.URL.Query().Get("proxy"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BootstrapAccountSessionToken(w http.ResponseWriter, r *http.Request) {
	accountID, err := parseIntParam(r, "account_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.BootstrapAccountSessionToken(r.Context(), accountID, r.URL.Query().Get("proxy"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SaveAccountSessionToken(w http.ResponseWriter, r *http.Request) {
	accountID, err := parseIntParam(r, "account_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.SaveSessionTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.SaveAccountSessionToken(r.Context(), accountID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateBindCardTask(w http.ResponseWriter, r *http.Request) {
	var req payment.CreateBindCardTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.CreateBindCardTask(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListBindCardTasks(w http.ResponseWriter, r *http.Request) {
	req := payment.ListBindCardTasksRequest{
		Status: r.URL.Query().Get("status"),
		Search: r.URL.Query().Get("search"),
	}
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil {
			writeDetailError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Page = page
	}
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		pageSize, err := strconv.Atoi(raw)
		if err != nil {
			writeDetailError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.PageSize = pageSize
	}
	resp, err := h.service.ListBindCardTasks(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) OpenBindCardTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.OpenBindCardTask(r.Context(), taskID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) AutoBindBindCardTaskThirdParty(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.ThirdPartyAutoBindRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.AutoBindBindCardTaskThirdParty(r.Context(), taskID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) AutoBindBindCardTaskLocal(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.LocalAutoBindRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.AutoBindBindCardTaskLocal(r.Context(), taskID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) MarkBindCardTaskUserAction(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.MarkUserActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.MarkBindCardTaskUserAction(r.Context(), taskID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SyncBindCardTaskSubscription(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.SyncBindCardTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.SyncBindCardTaskSubscription(r.Context(), taskID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteBindCardTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseIntParam(r, "task_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.DeleteBindCardTask(r.Context(), taskID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) MarkSubscription(w http.ResponseWriter, r *http.Request) {
	accountID, err := parseIntParam(r, "account_id")
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req payment.MarkSubscriptionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.MarkSubscription(r.Context(), accountID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchCheckSubscription(w http.ResponseWriter, r *http.Request) {
	var req payment.BatchCheckSubscriptionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.BatchCheckSubscription(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func parseIntParam(r *http.Request, name string) (int, error) {
	value := chi.URLParam(r, name)
	return strconv.Atoi(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeDetailError(w, payment.StatusCode(err), payment.Detail(err))
}

func writeDetailError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
