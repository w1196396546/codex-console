package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/go-chi/chi/v5"
)

type accountsService interface {
	ListAccounts(ctx context.Context, req accounts.ListAccountsRequest) (accounts.AccountListResponse, error)
}

type Handler struct {
	service accountsService
}

func NewHandler(service accountsService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/accounts", h.ListAccounts)
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	req, err := decodeListAccountsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.service.ListAccounts(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func decodeListAccountsRequest(r *http.Request) (accounts.ListAccountsRequest, error) {
	req := accounts.ListAccountsRequest{}

	if rawPage := r.URL.Query().Get("page"); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil {
			return accounts.ListAccountsRequest{}, err
		}
		req.Page = page
	}
	if rawPageSize := r.URL.Query().Get("page_size"); rawPageSize != "" {
		pageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			return accounts.ListAccountsRequest{}, err
		}
		req.PageSize = pageSize
	}

	return req, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
