package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *jobs.Service
}

type CreateJobRequest struct {
	JobType   string          `json:"job_type"`
	ScopeType string          `json:"scope_type"`
	ScopeID   string          `json:"scope_id"`
	Payload   json.RawMessage `json:"payload"`
}

func NewHandler(service *jobs.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/jobs", h.CreateJob)
	r.Get("/api/jobs/{jobID}", h.GetJob)
	r.Post("/api/jobs/{jobID}/pause", h.PauseJob)
	r.Post("/api/jobs/{jobID}/resume", h.ResumeJob)
	r.Post("/api/jobs/{jobID}/cancel", h.CancelJob)
	r.Get("/api/jobs/{jobID}/logs", h.ListJobLogs)
}

func (h *Handler) CreateJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		nethttp.Error(w, "invalid json", nethttp.StatusBadRequest)
		return
	}

	job, err := h.service.CreateJob(r.Context(), jobs.CreateJobParams{
		JobType:   req.JobType,
		ScopeType: req.ScopeType,
		ScopeID:   req.ScopeID,
		Payload:   []byte(req.Payload),
	})
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}

	if err := h.service.EnqueueJob(r.Context(), job.JobID); err != nil {
		_, _ = h.service.MarkFailed(r.Context(), job.JobID, "enqueue job: "+err.Error())
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusAccepted, map[string]any{
		"success": true,
		"job_id":  job.JobID,
		"status":  job.Status,
	})
}

func (h *Handler) GetJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := h.service.GetJob(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"success": true,
		"job":     job,
	})
}

func (h *Handler) PauseJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeJobStatusUpdate(w, r, h.service.PauseJob)
}

func (h *Handler) ResumeJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeJobStatusUpdate(w, r, h.service.ResumeJob)
}

func (h *Handler) CancelJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeJobStatusUpdate(w, r, h.service.CancelJob)
}

func (h *Handler) ListJobLogs(w nethttp.ResponseWriter, r *nethttp.Request) {
	logs, err := h.service.ListJobLogs(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"success": true,
		"items":   logs,
	})
}

func (h *Handler) writeJobStatusUpdate(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	update func(ctx context.Context, jobID string) (jobs.Job, error),
) {
	job, err := update(r.Context(), chi.URLParam(r, "jobID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"success": true,
		"job_id":  job.JobID,
		"status":  job.Status,
	})
}

func writeServiceError(w nethttp.ResponseWriter, err error) {
	status := nethttp.StatusInternalServerError
	if errors.Is(err, jobs.ErrJobNotFound) {
		status = nethttp.StatusNotFound
	} else if errors.Is(err, jobs.ErrControlNotSupported) {
		status = nethttp.StatusNotImplemented
	}

	nethttp.Error(w, err.Error(), status)
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
