package http

import (
	"context"
	"net/http"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	accountshttp "github.com/dou-jiang/codex-console/backend-go/internal/accounts/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	emailserviceshttp "github.com/dou-jiang/codex-console/backend-go/internal/emailservices/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	jobshttp "github.com/dou-jiang/codex-console/backend-go/internal/jobs/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	logshttp "github.com/dou-jiang/codex-console/backend-go/internal/logs/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationhttp "github.com/dou-jiang/codex-console/backend-go/internal/registration/http"
	registrationws "github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	settingshttp "github.com/dou-jiang/codex-console/backend-go/internal/settings/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	uploaderhttp "github.com/dou-jiang/codex-console/backend-go/internal/uploader/http"
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

type registrationStatsRouteService interface {
	GetStats(ctx context.Context) (registration.StatsResponse, error)
}

type outlookRouteService interface {
	ListOutlookAccounts(ctx context.Context) (registration.OutlookAccountsListResponse, error)
	StartOutlookBatch(ctx context.Context, req registration.OutlookBatchStartRequest) (registration.OutlookBatchStartResponse, error)
	GetOutlookBatch(ctx context.Context, batchID string, logOffset int) (registration.OutlookBatchStatusResponse, error)
}

type accountsRouteService interface {
	ListAccounts(ctx context.Context, req accounts.ListAccountsRequest) (accounts.AccountListResponse, error)
}

func NewRouter(jobService *jobs.Service, dependencies ...any) *chi.Mux {
	var registrationService *registration.Service
	var batchService *registration.BatchService
	var availableServices availableServicesRouteService
	var registrationStatsService registrationStatsRouteService
	var outlookService outlookRouteService
	var accountsService accountsRouteService
	var settingsService *settings.Service
	var emailServicesService *emailservices.Service
	var uploaderService *uploader.Service
	var logsService *logs.Service
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
		case registrationStatsRouteService:
			if registrationStatsService == nil {
				registrationStatsService = value
			}
		case outlookRouteService:
			if outlookService == nil {
				outlookService = value
			}
		case accountsRouteService:
			if accountsService == nil {
				accountsService = value
			}
		case *settings.Service:
			if settingsService == nil {
				settingsService = value
			}
		case *emailservices.Service:
			if emailServicesService == nil {
				emailServicesService = value
			}
		case *uploader.Service:
			if uploaderService == nil {
				uploaderService = value
			}
		case *logs.Service:
			if logsService == nil {
				logsService = value
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

	return newRouter(jobService, registrationService, batchService, availableServices, registrationStatsService, outlookService, accountsService, settingsService, emailServicesService, uploaderService, logsService, taskSocketHandler, batchSocketHandler)
}

func NewRouterWithTaskSocket(
	jobService *jobs.Service,
	registrationService *registration.Service,
	taskSocketHandler taskSocketRouteHandler,
) *chi.Mux {
	return newRouter(jobService, registrationService, nil, nil, nil, nil, nil, nil, nil, nil, nil, taskSocketHandler, nil)
}

func newRouter(
	jobService *jobs.Service,
	registrationService *registration.Service,
	batchService *registration.BatchService,
	availableServices availableServicesRouteService,
	registrationStatsService registrationStatsRouteService,
	outlookService outlookRouteService,
	accountsService accountsRouteService,
	settingsService *settings.Service,
	emailServicesService *emailservices.Service,
	uploaderService *uploader.Service,
	logsService *logs.Service,
	taskSocketHandler taskSocketRouteHandler,
	batchSocketHandler batchSocketRouteHandler,
) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if accountsService != nil {
		accountshttp.NewHandler(accountsService).RegisterRoutes(r)
	}
	if settingsService != nil {
		settingshttp.NewHandler(settingsService).RegisterRoutes(r)
	}
	if emailServicesService != nil {
		emailserviceshttp.NewHandler(emailServicesService).RegisterRoutes(r)
	}
	if uploaderService != nil {
		uploaderhttp.NewHandler(uploaderService).RegisterRoutes(r)
	}
	if logsService != nil {
		logshttp.NewHandler(logsService).RegisterRoutes(r)
	}
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
			registrationStatsService,
			outlookService,
		).RegisterRoutes(r)
		if batchSocketHandler == nil {
			batchSocketHandler = registrationws.NewBatchHandler(batchService)
		}
		r.Get("/api/ws/batch/{batch_id}", batchSocketHandler.HandleBatchSocket)
	}
	return r
}
