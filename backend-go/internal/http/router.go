package http

import (
	"net/http"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	jobshttp "github.com/dou-jiang/codex-console/backend-go/internal/jobs/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationhttp "github.com/dou-jiang/codex-console/backend-go/internal/registration/http"
	"github.com/go-chi/chi/v5"
)

func NewRouter(jobService *jobs.Service, registrationServices ...*registration.Service) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if jobService != nil {
		jobshttp.NewHandler(jobService).RegisterRoutes(r)
	}
	if len(registrationServices) > 0 && registrationServices[0] != nil && jobService != nil {
		registrationhttp.NewHandler(registrationServices[0], jobService).RegisterRoutes(r)
	}
	return r
}
