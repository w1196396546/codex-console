package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	"github.com/go-chi/chi/v5"
)

type uploaderService interface {
	ListCPAServices(ctx context.Context, enabled *bool) ([]uploader.CPAServiceResponse, error)
	CreateCPAService(ctx context.Context, req uploader.CreateCPAServiceRequest) (uploader.CPAServiceResponse, error)
	GetCPAService(ctx context.Context, id int) (uploader.CPAServiceResponse, error)
	GetCPAServiceFull(ctx context.Context, id int) (uploader.CPAServiceFullResponse, error)
	UpdateCPAService(ctx context.Context, id int, req uploader.UpdateCPAServiceRequest) (uploader.CPAServiceResponse, error)
	DeleteCPAService(ctx context.Context, id int) (uploader.DeleteServiceResponse, error)
	TestCPAService(ctx context.Context, id int) (uploader.ConnectionTestResult, error)
	TestCPAConnection(ctx context.Context, req uploader.CPAConnectionTestRequest) (uploader.ConnectionTestResult, error)

	ListSub2APIServices(ctx context.Context, enabled *bool) ([]uploader.Sub2APIServiceResponse, error)
	CreateSub2APIService(ctx context.Context, req uploader.CreateSub2APIServiceRequest) (uploader.Sub2APIServiceResponse, error)
	GetSub2APIService(ctx context.Context, id int) (uploader.Sub2APIServiceResponse, error)
	GetSub2APIServiceFull(ctx context.Context, id int) (uploader.Sub2APIServiceFullResponse, error)
	UpdateSub2APIService(ctx context.Context, id int, req uploader.UpdateSub2APIServiceRequest) (uploader.Sub2APIServiceResponse, error)
	DeleteSub2APIService(ctx context.Context, id int) (uploader.DeleteServiceResponse, error)
	TestSub2APIService(ctx context.Context, id int) (uploader.ConnectionTestResult, error)
	TestSub2APIConnection(ctx context.Context, req uploader.Sub2APIConnectionTestRequest) (uploader.ConnectionTestResult, error)
	UploadSub2API(ctx context.Context, req uploader.Sub2APIUploadRequest) (uploader.Sub2APIUploadResult, error)

	ListTMServices(ctx context.Context, enabled *bool) ([]uploader.TMServiceResponse, error)
	CreateTMService(ctx context.Context, req uploader.CreateTMServiceRequest) (uploader.TMServiceResponse, error)
	GetTMService(ctx context.Context, id int) (uploader.TMServiceResponse, error)
	UpdateTMService(ctx context.Context, id int, req uploader.UpdateTMServiceRequest) (uploader.TMServiceResponse, error)
	DeleteTMService(ctx context.Context, id int) (uploader.DeleteServiceResponse, error)
	TestTMService(ctx context.Context, id int) (uploader.ConnectionTestResult, error)
	TestTMConnection(ctx context.Context, req uploader.TMConnectionTestRequest) (uploader.ConnectionTestResult, error)
}

type Handler struct {
	service uploaderService
}

func NewHandler(service uploaderService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/cpa-services", func(r chi.Router) {
		r.Get("/", h.ListCPAServices)
		r.Post("/", h.CreateCPAService)
		r.Post("/test-connection", h.TestCPAConnection)
		r.Get("/{serviceID}", h.GetCPAService)
		r.Get("/{serviceID}/full", h.GetCPAServiceFull)
		r.Patch("/{serviceID}", h.UpdateCPAService)
		r.Delete("/{serviceID}", h.DeleteCPAService)
		r.Post("/{serviceID}/test", h.TestCPAService)
	})

	r.Route("/api/sub2api-services", func(r chi.Router) {
		r.Get("/", h.ListSub2APIServices)
		r.Post("/", h.CreateSub2APIService)
		r.Post("/test-connection", h.TestSub2APIConnection)
		r.Post("/upload", h.UploadSub2API)
		r.Get("/{serviceID}", h.GetSub2APIService)
		r.Get("/{serviceID}/full", h.GetSub2APIServiceFull)
		r.Patch("/{serviceID}", h.UpdateSub2APIService)
		r.Delete("/{serviceID}", h.DeleteSub2APIService)
		r.Post("/{serviceID}/test", h.TestSub2APIService)
	})

	r.Route("/api/tm-services", func(r chi.Router) {
		r.Get("/", h.ListTMServices)
		r.Post("/", h.CreateTMService)
		r.Post("/test-connection", h.TestTMConnection)
		r.Get("/{serviceID}", h.GetTMService)
		r.Patch("/{serviceID}", h.UpdateTMService)
		r.Delete("/{serviceID}", h.DeleteTMService)
		r.Post("/{serviceID}/test", h.TestTMService)
	})
}

func (h *Handler) ListCPAServices(w http.ResponseWriter, r *http.Request) {
	enabled, err := decodeEnabledFilter(r)
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, "enabled 参数无效")
		return
	}
	resp, err := h.service.ListCPAServices(r.Context(), enabled)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateCPAService(w http.ResponseWriter, r *http.Request) {
	var req uploader.CreateCPAServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.CreateCPAService(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetCPAService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetCPAService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetCPAServiceFull(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetCPAServiceFull(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateCPAService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	var req uploader.UpdateCPAServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateCPAService(r.Context(), serviceID, req)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteCPAService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DeleteCPAService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestCPAService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.TestCPAService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestCPAConnection(w http.ResponseWriter, r *http.Request) {
	var req uploader.CPAConnectionTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.APIURL == "" || req.APIToken == "" {
		writeDetailError(w, http.StatusBadRequest, "api_url 和 api_token 不能为空")
		return
	}
	resp, err := h.service.TestCPAConnection(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindCPA, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListSub2APIServices(w http.ResponseWriter, r *http.Request) {
	enabled, err := decodeEnabledFilter(r)
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, "enabled 参数无效")
		return
	}
	resp, err := h.service.ListSub2APIServices(r.Context(), enabled)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateSub2APIService(w http.ResponseWriter, r *http.Request) {
	var req uploader.CreateSub2APIServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.CreateSub2APIService(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetSub2APIService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetSub2APIService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetSub2APIServiceFull(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetSub2APIServiceFull(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateSub2APIService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	var req uploader.UpdateSub2APIServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateSub2APIService(r.Context(), serviceID, req)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteSub2APIService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DeleteSub2APIService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestSub2APIService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.TestSub2APIService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestSub2APIConnection(w http.ResponseWriter, r *http.Request) {
	var req uploader.Sub2APIConnectionTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.APIURL == "" || req.APIKey == "" {
		writeDetailError(w, http.StatusBadRequest, "api_url 和 api_key 不能为空")
		return
	}
	resp, err := h.service.TestSub2APIConnection(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UploadSub2API(w http.ResponseWriter, r *http.Request) {
	var req uploader.Sub2APIUploadRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.AccountIDs) == 0 {
		writeDetailError(w, http.StatusBadRequest, "账号 ID 列表不能为空")
		return
	}
	resp, err := h.service.UploadSub2API(r.Context(), req)
	if err != nil {
		if errors.Is(err, uploader.ErrUploadServiceUnavailable) {
			writeDetailError(w, http.StatusBadRequest, "未找到可用的 Sub2API 服务")
			return
		}
		writeKindError(w, uploader.UploadKindSub2API, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListTMServices(w http.ResponseWriter, r *http.Request) {
	enabled, err := decodeEnabledFilter(r)
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, "enabled 参数无效")
		return
	}
	resp, err := h.service.ListTMServices(r.Context(), enabled)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateTMService(w http.ResponseWriter, r *http.Request) {
	var req uploader.CreateTMServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.CreateTMService(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetTMService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.GetTMService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateTMService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	var req uploader.UpdateTMServiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	resp, err := h.service.UpdateTMService(r.Context(), serviceID, req)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteTMService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.DeleteTMService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestTMService(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := decodeServiceID(w, r)
	if !ok {
		return
	}
	resp, err := h.service.TestTMService(r.Context(), serviceID)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) TestTMConnection(w http.ResponseWriter, r *http.Request) {
	var req uploader.TMConnectionTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.APIURL == "" || req.APIKey == "" {
		writeDetailError(w, http.StatusBadRequest, "api_url 和 api_key 不能为空")
		return
	}
	resp, err := h.service.TestTMConnection(r.Context(), req)
	if err != nil {
		writeKindError(w, uploader.UploadKindTM, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeServiceID(w http.ResponseWriter, r *http.Request) (int, bool) {
	serviceID, err := strconv.Atoi(chi.URLParam(r, "serviceID"))
	if err != nil {
		writeDetailError(w, http.StatusBadRequest, "service_id 参数无效")
		return 0, false
	}
	return serviceID, true
}

func decodeEnabledFilter(r *http.Request) (*bool, error) {
	rawEnabled := r.URL.Query().Get("enabled")
	if rawEnabled == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(rawEnabled)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func decodeJSON(r *http.Request, dest any) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

func writeKindError(w http.ResponseWriter, kind uploader.UploadKind, err error) {
	status := http.StatusInternalServerError
	detail := err.Error()

	if errors.Is(err, uploader.ErrServiceConfigNotFound) {
		status = http.StatusNotFound
		switch kind {
		case uploader.UploadKindCPA:
			detail = "CPA 服务不存在"
		case uploader.UploadKindSub2API:
			detail = "Sub2API 服务不存在"
		case uploader.UploadKindTM:
			detail = "Team Manager 服务不存在"
		}
	}
	writeDetailError(w, status, detail)
}

func writeDetailError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
