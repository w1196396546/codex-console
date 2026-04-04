package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(_ any) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return r
}
