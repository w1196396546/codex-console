package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/go-chi/chi/v5"
)

type startService interface {
	StartRegistration(ctx context.Context, req registration.StartRequest) (registration.TaskResponse, error)
}

type taskService interface {
	GetJob(ctx context.Context, jobID string) (jobs.Job, error)
	ListJobLogs(ctx context.Context, jobID string) ([]jobs.JobLog, error)
	PauseJob(ctx context.Context, jobID string) (jobs.Job, error)
	ResumeJob(ctx context.Context, jobID string) (jobs.Job, error)
	CancelJob(ctx context.Context, jobID string) (jobs.Job, error)
}

type Handler struct {
	start startService
	tasks taskService
}

func NewHandler(start startService, tasks taskService) *Handler {
	return &Handler{
		start: start,
		tasks: tasks,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/registration", func(r chi.Router) {
		r.Get("/available-services", h.GetAvailableServices)
		r.Post("/start", h.StartRegistration)
		r.Get("/tasks/{taskUUID}", h.GetTask)
		r.Get("/tasks/{taskUUID}/logs", h.GetTaskLogs)
		r.Post("/tasks/{taskUUID}/pause", h.PauseTask)
		r.Post("/tasks/{taskUUID}/resume", h.ResumeTask)
		r.Post("/tasks/{taskUUID}/cancel", h.CancelTask)
	})
}

func (h *Handler) GetAvailableServices(w nethttp.ResponseWriter, _ *nethttp.Request) {
	writeJSON(w, nethttp.StatusOK, map[string]any{
		"tempmail": map[string]any{
			"available": true,
			"count":     1,
			"services": []map[string]any{
				{"id": "default", "name": "TempMail"},
			},
		},
		"yyds_mail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"outlook": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"moe_mail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"temp_mail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"duck_mail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"luckmail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
		"freemail": map[string]any{
			"available": false,
			"count":     0,
			"services":  []any{},
		},
	})
}

func (h *Handler) StartRegistration(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req registration.StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		nethttp.Error(w, "invalid json", nethttp.StatusBadRequest)
		return
	}

	resp, err := h.start.StartRegistration(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusAccepted, resp)
}

func (h *Handler) GetTask(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := h.tasks.GetJob(r.Context(), chi.URLParam(r, "taskUUID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"task_uuid": job.JobID,
		"status":    job.Status,
	})
}

func (h *Handler) GetTaskLogs(w nethttp.ResponseWriter, r *nethttp.Request) {
	taskUUID := chi.URLParam(r, "taskUUID")
	job, err := h.tasks.GetJob(r.Context(), taskUUID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	logs, err := h.tasks.ListJobLogs(r.Context(), taskUUID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	offset := 0
	if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
		parsedOffset, parseErr := strconv.Atoi(rawOffset)
		if parseErr != nil || parsedOffset < 0 {
			nethttp.Error(w, "invalid offset", nethttp.StatusBadRequest)
			return
		}
		offset = parsedOffset
	}
	if offset > len(logs) {
		offset = len(logs)
	}
	incrementalLogs := logs[offset:]

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"status":          job.Status,
		"logs":            incrementalLogs,
		"log_offset":      offset,
		"log_next_offset": len(logs),
	})
}

func (h *Handler) PauseTask(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeTaskStatusUpdate(w, r, h.tasks.PauseJob)
}

func (h *Handler) ResumeTask(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeTaskStatusUpdate(w, r, h.tasks.ResumeJob)
}

func (h *Handler) CancelTask(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeTaskStatusUpdate(w, r, h.tasks.CancelJob)
}

func (h *Handler) writeTaskStatusUpdate(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	update func(ctx context.Context, jobID string) (jobs.Job, error),
) {
	job, err := update(r.Context(), chi.URLParam(r, "taskUUID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"task_uuid": job.JobID,
		"status":    job.Status,
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
