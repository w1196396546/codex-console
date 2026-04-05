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
	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
	paymenthttp "github.com/dou-jiang/codex-console/backend-go/internal/payment/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationhttp "github.com/dou-jiang/codex-console/backend-go/internal/registration/http"
	registrationws "github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	settingshttp "github.com/dou-jiang/codex-console/backend-go/internal/settings/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/team"
	teamhttp "github.com/dou-jiang/codex-console/backend-go/internal/team/http"
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

type paymentRouteService interface {
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
	var paymentService paymentRouteService
	var teamService *team.Service
	var teamTaskService *team.TaskService
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
		case paymentRouteService:
			if paymentService == nil {
				paymentService = value
			}
		case *team.Service:
			if teamService == nil {
				teamService = value
			}
		case *team.TaskService:
			if teamTaskService == nil {
				teamTaskService = value
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

	return newRouter(jobService, registrationService, batchService, availableServices, registrationStatsService, outlookService, accountsService, settingsService, emailServicesService, uploaderService, logsService, paymentService, teamService, teamTaskService, taskSocketHandler, batchSocketHandler)
}

func NewRouterWithTaskSocket(
	jobService *jobs.Service,
	registrationService *registration.Service,
	taskSocketHandler taskSocketRouteHandler,
) *chi.Mux {
	return newRouter(jobService, registrationService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, taskSocketHandler, nil)
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
	paymentService paymentRouteService,
	teamService *team.Service,
	teamTaskService *team.TaskService,
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
	if paymentService != nil {
		paymenthttp.NewHandler(paymentService).RegisterRoutes(r)
	}
	if teamService != nil && teamTaskService != nil {
		teamhttp.NewHandler(teamService, teamTaskService).RegisterRoutes(r)
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
