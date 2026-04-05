package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	"github.com/go-chi/chi/v5"
)

type emailServicesService interface {
	ListServices(ctx context.Context, req emailservices.ListServicesRequest) (emailservices.EmailServiceListResponse, error)
	GetService(ctx context.Context, serviceID int) (emailservices.EmailServiceResponse, error)
	GetServiceFull(ctx context.Context, serviceID int) (emailservices.EmailServiceFullResponse, error)
	GetStats(ctx context.Context) (emailservices.StatsResponse, error)
	GetServiceTypes() emailservices.ServiceTypesResponse
	CreateService(ctx context.Context, req emailservices.CreateServiceRequest) (emailservices.EmailServiceResponse, error)
	UpdateService(ctx context.Context, serviceID int, req emailservices.UpdateServiceRequest) (emailservices.EmailServiceResponse, error)
	DeleteService(ctx context.Context, serviceID int) (emailservices.ActionResponse, error)
	TestService(ctx context.Context, serviceID int) (emailservices.ServiceTestResult, error)
	EnableService(ctx context.Context, serviceID int) (emailservices.ActionResponse, error)
	DisableService(ctx context.Context, serviceID int) (emailservices.ActionResponse, error)
	ReorderServices(ctx context.Context, serviceIDs []int) (emailservices.ActionResponse, error)
	BatchImportOutlook(ctx context.Context, req emailservices.OutlookBatchImportRequest) (emailservices.OutlookBatchImportResponse, error)
	BatchDeleteOutlook(ctx context.Context, serviceIDs []int) (emailservices.BatchDeleteResponse, error)
	TestTempmail(ctx context.Context, req emailservices.TempmailTestRequest) (emailservices.ServiceTestResult, error)
}

type Handler struct {
	service emailServicesService
}

func NewHandler(service emailServicesService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/email-services", func(r chi.Router) {
		r.Get("/stats", h.GetStats)
		r.Get("/types", h.GetServiceTypes)
		r.Get("/", h.ListServices)
		r.Post("/", h.CreateService)
		r.Post("/reorder", h.ReorderServices)
		r.Post("/outlook/batch-import", h.BatchImportOutlook)
		r.Delete("/outlook/batch", h.BatchDeleteOutlook)
		r.Post("/test-tempmail", h.TestTempmail)
		r.Get("/{serviceID}", h.GetService)
		r.Get("/{serviceID}/full", h.GetServiceFull)
		r.Patch("/{serviceID}", h.UpdateService)
		r.Delete("/{serviceID}", h.DeleteService)
		r.Post("/{serviceID}/test", h.TestService)
		r.Post("/{serviceID}/enable", h.EnableService)
		r.Post("/{serviceID}/disable", h.DisableService)
	})
}

func (h *Handler) GetStats(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.GetStats(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetServiceTypes(w nethttp.ResponseWriter, _ *nethttp.Request) {
	writeJSON(w, nethttp.StatusOK, h.service.GetServiceTypes())
}

func (h *Handler) ListServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.service.ListServices(r.Context(), emailservices.ListServicesRequest{
		ServiceType: strings.TrimSpace(r.URL.Query().Get("service_type")),
		EnabledOnly: parseBoolQuery(r.URL.Query().Get("enabled_only")),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetService(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetServiceFull(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetServiceFull(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) CreateService(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req emailservices.CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.CreateService(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) UpdateService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	var req emailservices.UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.UpdateService(r.Context(), serviceID, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) DeleteService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DeleteService(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) TestService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.TestService(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) EnableService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.EnableService(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) DisableService(w nethttp.ResponseWriter, r *nethttp.Request) {
	serviceID, ok := parseServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DisableService(r.Context(), serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) ReorderServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	var serviceIDs []int
	if err := json.NewDecoder(r.Body).Decode(&serviceIDs); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.ReorderServices(r.Context(), serviceIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) BatchImportOutlook(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req emailservices.OutlookBatchImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.BatchImportOutlook(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) BatchDeleteOutlook(w nethttp.ResponseWriter, r *nethttp.Request) {
	var serviceIDs []int
	if err := json.NewDecoder(r.Body).Decode(&serviceIDs); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.BatchDeleteOutlook(r.Context(), serviceIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) TestTempmail(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req emailservices.TempmailTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "请求体格式错误")
		return
	}
	resp, err := h.service.TestTempmail(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func parseServiceID(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	serviceID, err := strconv.Atoi(chi.URLParam(r, "serviceID"))
	if err != nil || serviceID <= 0 {
		writeError(w, nethttp.StatusBadRequest, "无效的服务 ID")
		return 0, false
	}
	return serviceID, true
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeServiceError(w nethttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, emailservices.ErrServiceNotFound):
		writeError(w, nethttp.StatusNotFound, "服务不存在")
	case errors.Is(err, emailservices.ErrInvalidServiceType),
		errors.Is(err, emailservices.ErrDuplicateServiceName),
		errors.Is(err, emailservices.ErrInvalidReorderInput),
		errors.Is(err, emailservices.ErrEmptyOutlookImport),
		errors.Is(err, emailservices.ErrTempmailProviderError):
		writeError(w, nethttp.StatusBadRequest, err.Error())
	default:
		writeError(w, nethttp.StatusInternalServerError, err.Error())
	}
}

func writeError(w nethttp.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
