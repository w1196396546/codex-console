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

type batchService interface {
	StartBatch(ctx context.Context, req registration.BatchStartRequest) (registration.BatchStartResponse, error)
	GetBatch(ctx context.Context, batchID string, logOffset int) (registration.BatchStatusResponse, error)
	PauseBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
	ResumeBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
	CancelBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
}

type availableServicesService interface {
	ListAvailableServices(ctx context.Context) (registration.AvailableServicesResponse, error)
}

type outlookService interface {
	ListOutlookAccounts(ctx context.Context) (registration.OutlookAccountsListResponse, error)
	StartOutlookBatch(ctx context.Context, req registration.OutlookBatchStartRequest) (registration.OutlookBatchStartResponse, error)
	GetOutlookBatch(ctx context.Context, batchID string, logOffset int) (registration.OutlookBatchStatusResponse, error)
}

type Handler struct {
	start             startService
	tasks             taskService
	batches           batchService
	availableServices availableServicesService
	outlook           outlookService
}

func NewHandler(
	start startService,
	tasks taskService,
	batches batchService,
	availableServices availableServicesService,
	outlook outlookService,
) *Handler {
	return &Handler{
		start:             start,
		tasks:             tasks,
		batches:           batches,
		availableServices: availableServices,
		outlook:           outlook,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/registration", func(r chi.Router) {
		r.Get("/available-services", h.GetAvailableServices)
		if h.outlook != nil {
			r.Get("/outlook-accounts", h.GetOutlookAccounts)
		}
		r.Post("/start", h.StartRegistration)
		if h.batches != nil {
			r.Post("/batch", h.StartBatch)
			r.Get("/batch/{batchID}", h.GetBatch)
			r.Post("/batch/{batchID}/pause", h.PauseBatch)
			r.Post("/batch/{batchID}/resume", h.ResumeBatch)
			r.Post("/batch/{batchID}/cancel", h.CancelBatch)
		}
		if h.outlook != nil && h.batches != nil {
			r.Post("/outlook-batch", h.StartOutlookBatch)
			r.Get("/outlook-batch/{batchID}", h.GetOutlookBatch)
			r.Post("/outlook-batch/{batchID}/pause", h.PauseOutlookBatch)
			r.Post("/outlook-batch/{batchID}/resume", h.ResumeOutlookBatch)
			r.Post("/outlook-batch/{batchID}/cancel", h.CancelOutlookBatch)
		}
		r.Get("/tasks/{taskUUID}", h.GetTask)
		r.Get("/tasks/{taskUUID}/logs", h.GetTaskLogs)
		r.Post("/tasks/{taskUUID}/pause", h.PauseTask)
		r.Post("/tasks/{taskUUID}/resume", h.ResumeTask)
		r.Post("/tasks/{taskUUID}/cancel", h.CancelTask)
	})
}

func (h *Handler) GetOutlookAccounts(w nethttp.ResponseWriter, r *nethttp.Request) {
	resp, err := h.outlook.ListOutlookAccounts(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetAvailableServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	if h.availableServices != nil {
		response, err := h.availableServices.ListAvailableServices(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, nethttp.StatusOK, response)
		return
	}

	writeJSON(w, nethttp.StatusOK, registration.AvailableServicesResponse{
		"tempmail": {
			Available: true,
			Count:     1,
			Services: []map[string]any{
				{"id": "default", "name": "TempMail"},
			},
		},
		"yyds_mail": {Available: false, Count: 0, Services: []map[string]any{}},
		"outlook":   {Available: false, Count: 0, Services: []map[string]any{}},
		"moe_mail":  {Available: false, Count: 0, Services: []map[string]any{}},
		"temp_mail": {Available: false, Count: 0, Services: []map[string]any{}},
		"duck_mail": {Available: false, Count: 0, Services: []map[string]any{}},
		"luckmail":  {Available: false, Count: 0, Services: []map[string]any{}},
		"freemail":  {Available: false, Count: 0, Services: []map[string]any{}},
		"imap_mail": {Available: false, Count: 0, Services: []map[string]any{}},
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

func (h *Handler) StartBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req registration.BatchStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		nethttp.Error(w, "invalid json", nethttp.StatusBadRequest)
		return
	}

	resp, err := h.batches.StartBatch(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusAccepted, resp)
}

func (h *Handler) StartOutlookBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req registration.OutlookBatchStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		nethttp.Error(w, "invalid json", nethttp.StatusBadRequest)
		return
	}

	resp, err := h.outlook.StartOutlookBatch(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusAccepted, resp)
}

func (h *Handler) GetBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	logOffset := 0
	if rawOffset := r.URL.Query().Get("log_offset"); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil || parsedOffset < 0 {
			nethttp.Error(w, "invalid log_offset", nethttp.StatusBadRequest)
			return
		}
		logOffset = parsedOffset
	}

	resp, err := h.batches.GetBatch(r.Context(), chi.URLParam(r, "batchID"), logOffset)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetOutlookBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	logOffset := 0
	if rawOffset := r.URL.Query().Get("log_offset"); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil || parsedOffset < 0 {
			nethttp.Error(w, "invalid log_offset", nethttp.StatusBadRequest)
			return
		}
		logOffset = parsedOffset
	}

	resp, err := h.outlook.GetOutlookBatch(r.Context(), chi.URLParam(r, "batchID"), logOffset)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *Handler) GetTask(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := h.tasks.GetJob(r.Context(), chi.URLParam(r, "taskUUID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	taskMetadata := registration.ResolveTaskMetadata(job)

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"task_uuid":     job.JobID,
		"status":        job.Status,
		"email":         taskMetadata.Email,
		"email_service": taskMetadata.EmailService,
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
	logMessages := make([]string, 0, len(incrementalLogs))
	for _, item := range incrementalLogs {
		logMessages = append(logMessages, item.Message)
	}
	taskMetadata := registration.ResolveTaskMetadata(job)

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"task_uuid":       job.JobID,
		"status":          job.Status,
		"email":           taskMetadata.Email,
		"email_service":   taskMetadata.EmailService,
		"logs":            logMessages,
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

func (h *Handler) PauseBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.PauseBatch)
}

func (h *Handler) ResumeBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.ResumeBatch)
}

func (h *Handler) CancelBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.CancelBatch)
}

func (h *Handler) PauseOutlookBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.PauseBatch)
}

func (h *Handler) ResumeOutlookBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.ResumeBatch)
}

func (h *Handler) CancelOutlookBatch(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.writeBatchStatusUpdate(w, r, h.batches.CancelBatch)
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

func (h *Handler) writeBatchStatusUpdate(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	update func(ctx context.Context, batchID string) (registration.BatchControlResponse, error),
) {
	resp, err := update(r.Context(), chi.URLParam(r, "batchID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, nethttp.StatusOK, resp)
}

func writeServiceError(w nethttp.ResponseWriter, err error) {
	status := nethttp.StatusInternalServerError
	if errors.Is(err, jobs.ErrJobNotFound) {
		status = nethttp.StatusNotFound
	} else if errors.Is(err, registration.ErrBatchNotFound) {
		status = nethttp.StatusNotFound
	} else if errors.Is(err, registration.ErrInvalidBatchCount) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrBatchAlreadyPaused) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrBatchNotPaused) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrBatchFinished) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrOutlookAccountSelectionRequired) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrInvalidOutlookInterval) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrInvalidOutlookConcurrency) {
		status = nethttp.StatusBadRequest
	} else if errors.Is(err, registration.ErrInvalidOutlookMode) {
		status = nethttp.StatusBadRequest
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
