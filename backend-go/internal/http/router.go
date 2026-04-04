package http

import (
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

func NewRouter(jobService *jobs.Service, dependencies ...any) *chi.Mux {
	var registrationService *registration.Service
	var taskSocketHandler taskSocketRouteHandler
	for _, dependency := range dependencies {
		switch value := dependency.(type) {
		case *registration.Service:
			if registrationService == nil {
				registrationService = value
			}
		case taskSocketRouteHandler:
			if taskSocketHandler == nil {
				taskSocketHandler = value
			}
		}
	}

	return newRouter(jobService, registrationService, taskSocketHandler)
}

func NewRouterWithTaskSocket(
	jobService *jobs.Service,
	registrationService *registration.Service,
	taskSocketHandler taskSocketRouteHandler,
) *chi.Mux {
	return newRouter(jobService, registrationService, taskSocketHandler)
}

func newRouter(
	jobService *jobs.Service,
	registrationService *registration.Service,
	taskSocketHandler taskSocketRouteHandler,
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
		registrationhttp.NewHandler(
			registrationService,
			jobService,
			registration.NewBatchService(jobService),
		).RegisterRoutes(r)
	}
	return r
}
