package http

import (
	"context"
	"net/http"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	jobshttp "github.com/dou-jiang/codex-console/backend-go/internal/jobs/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationhttp "github.com/dou-jiang/codex-console/backend-go/internal/registration/http"
	registrationws "github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/go-chi/chi/v5"
)

type taskSocketRouteHandler interface {
	HandleTaskSocket(w http.ResponseWriter, r *http.Request)
}

type batchSocketRouteHandler interface {
	HandleBatchSocket(w http.ResponseWriter, r *http.Request)
}

type availableServicesRouteService interface {
	ListAvailableServices(ctx context.Context) (registration.AvailableServicesResponse, error)
}

type outlookRouteService interface {
	ListOutlookAccounts(ctx context.Context) (registration.OutlookAccountsListResponse, error)
	StartOutlookBatch(ctx context.Context, req registration.OutlookBatchStartRequest) (registration.OutlookBatchStartResponse, error)
	GetOutlookBatch(ctx context.Context, batchID string, logOffset int) (registration.OutlookBatchStatusResponse, error)
}

func NewRouter(jobService *jobs.Service, dependencies ...any) *chi.Mux {
	var registrationService *registration.Service
	var batchService *registration.BatchService
	var availableServices availableServicesRouteService
	var outlookService outlookRouteService
	var taskSocketHandler taskSocketRouteHandler
	var batchSocketHandler batchSocketRouteHandler
	for _, dependency := range dependencies {
		switch value := dependency.(type) {
		case *registration.Service:
			if registrationService == nil {
				registrationService = value
			}
		case *registration.BatchService:
			if batchService == nil {
				batchService = value
			}
		case availableServicesRouteService:
			if availableServices == nil {
				availableServices = value
			}
		case outlookRouteService:
			if outlookService == nil {
				outlookService = value
			}
		case taskSocketRouteHandler:
			if taskSocketHandler == nil {
				taskSocketHandler = value
			}
		case batchSocketRouteHandler:
			if batchSocketHandler == nil {
				batchSocketHandler = value
			}
		}
	}

	return newRouter(jobService, registrationService, batchService, availableServices, outlookService, taskSocketHandler, batchSocketHandler)
}

func NewRouterWithTaskSocket(
	jobService *jobs.Service,
	registrationService *registration.Service,
	taskSocketHandler taskSocketRouteHandler,
) *chi.Mux {
	return newRouter(jobService, registrationService, nil, nil, nil, taskSocketHandler, nil)
}

func newRouter(
	jobService *jobs.Service,
	registrationService *registration.Service,
	batchService *registration.BatchService,
	availableServices availableServicesRouteService,
	outlookService outlookRouteService,
	taskSocketHandler taskSocketRouteHandler,
	batchSocketHandler batchSocketRouteHandler,
) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if jobService != nil {
		jobshttp.NewHandler(jobService).RegisterRoutes(r)
		if taskSocketHandler == nil {
			taskSocketHandler = registrationws.NewHandler(jobService)
		}
		r.Get("/api/ws/task/{task_uuid}", taskSocketHandler.HandleTaskSocket)
	}
	if registrationService != nil && jobService != nil {
		if batchService == nil {
			batchService = registration.NewBatchService(jobService)
			// A custom batch websocket handler without the shared BatchService would split
			// HTTP and websocket batch state. Fall back to the default shared handler.
			batchSocketHandler = nil
		}
		registrationhttp.NewHandler(
			registrationService,
			jobService,
			batchService,
			availableServices,
			outlookService,
		).RegisterRoutes(r)
		if batchSocketHandler == nil {
			batchSocketHandler = registrationws.NewBatchHandler(batchService)
		}
		r.Get("/api/ws/batch/{batch_id}", batchSocketHandler.HandleBatchSocket)
	}
	return r
}
