package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/go-chi/chi/v5"
)

type listAccountsService interface {
	ListAccounts(ctx context.Context, req accounts.ListAccountsRequest) (accounts.AccountListResponse, error)
}

type currentAccountService interface {
	GetCurrentAccount(ctx context.Context) (accounts.CurrentAccountResponse, error)
}

type accountsStatsSummaryService interface {
	GetAccountsStatsSummary(ctx context.Context) (accounts.AccountsStatsSummary, error)
}

type accountsOverviewStatsService interface {
	GetAccountsOverviewStats(ctx context.Context) (accounts.AccountsOverviewStats, error)
}

type overviewCardsService interface {
	ListOverviewCards(ctx context.Context, req accounts.AccountOverviewCardsRequest) (accounts.AccountOverviewCardsResponse, error)
}

type overviewSelectableService interface {
	ListOverviewSelectable(ctx context.Context, req accounts.AccountOverviewSelectableRequest) (accounts.AccountOverviewSelectableResponse, error)
}

type accountDetailService interface {
	GetAccount(ctx context.Context, accountID int) (accounts.Account, error)
}

type accountTokensService interface {
	GetAccountTokens(ctx context.Context, accountID int) (accounts.AccountTokensResponse, error)
}

type accountCookiesService interface {
	GetAccountCookies(ctx context.Context, accountID int) (accounts.AccountCookiesResponse, error)
}

type Handler struct {
	service any
}

func NewHandler(service any) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/accounts/current", h.GetCurrentAccount)
	r.Get("/api/accounts/stats/summary", h.GetAccountsStatsSummary)
	r.Get("/api/accounts/stats/overview", h.GetAccountsOverviewStats)
	r.Get("/api/accounts/overview/cards", h.ListOverviewCards)
	r.Get("/api/accounts/overview/cards/selectable", h.ListOverviewSelectable)
	r.Get("/api/accounts/{account_id}/tokens", h.GetAccountTokens)
	r.Get("/api/accounts/{account_id}/cookies", h.GetAccountCookies)
	r.Get("/api/accounts/{account_id}", h.GetAccount)
	r.Get("/api/accounts", h.ListAccounts)
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(listAccountsService)
	if !ok {
		http.Error(w, "accounts list service not configured", http.StatusInternalServerError)
		return
	}

	req, err := decodeListAccountsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.ListAccounts(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetCurrentAccount(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(currentAccountService)
	if !ok {
		http.Error(w, "current account service not configured", http.StatusInternalServerError)
		return
	}

	resp, err := svc.GetCurrentAccount(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountsStatsSummary(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(accountsStatsSummaryService)
	if !ok {
		http.Error(w, "accounts stats summary service not configured", http.StatusInternalServerError)
		return
	}

	resp, err := svc.GetAccountsStatsSummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountsOverviewStats(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(accountsOverviewStatsService)
	if !ok {
		http.Error(w, "accounts overview stats service not configured", http.StatusInternalServerError)
		return
	}

	resp, err := svc.GetAccountsOverviewStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListOverviewCards(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(overviewCardsService)
	if !ok {
		http.Error(w, "accounts overview cards service not configured", http.StatusInternalServerError)
		return
	}

	req, err := decodeOverviewCardsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.ListOverviewCards(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListOverviewSelectable(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(overviewSelectableService)
	if !ok {
		http.Error(w, "accounts overview selectable service not configured", http.StatusInternalServerError)
		return
	}

	req := accounts.AccountOverviewSelectableRequest{
		Search:       r.URL.Query().Get("search"),
		Status:       r.URL.Query().Get("status"),
		EmailService: r.URL.Query().Get("email_service"),
	}.Normalized()

	resp, err := svc.ListOverviewSelectable(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(accountDetailService)
	if !ok {
		http.Error(w, "account detail service not configured", http.StatusInternalServerError)
		return
	}

	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.GetAccount(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountTokens(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(accountTokensService)
	if !ok {
		http.Error(w, "account tokens service not configured", http.StatusInternalServerError)
		return
	}

	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.GetAccountTokens(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetAccountCookies(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(accountCookiesService)
	if !ok {
		http.Error(w, "account cookies service not configured", http.StatusInternalServerError)
		return
	}

	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := svc.GetAccountCookies(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeListAccountsRequest(r *http.Request) (accounts.ListAccountsRequest, error) {
	req := accounts.ListAccountsRequest{
		Status:            r.URL.Query().Get("status"),
		EmailService:      r.URL.Query().Get("email_service"),
		RefreshTokenState: r.URL.Query().Get("refresh_token_state"),
		Search:            r.URL.Query().Get("search"),
	}

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

	if err := validateRefreshTokenState(req.RefreshTokenState); err != nil {
		return accounts.ListAccountsRequest{}, err
	}

	return req.Normalized(), nil
}

func decodeOverviewCardsRequest(r *http.Request) (accounts.AccountOverviewCardsRequest, error) {
	req := accounts.AccountOverviewCardsRequest{
		Search:       r.URL.Query().Get("search"),
		Status:       r.URL.Query().Get("status"),
		EmailService: r.URL.Query().Get("email_service"),
		Proxy:        r.URL.Query().Get("proxy"),
	}

	if rawRefresh := r.URL.Query().Get("refresh"); rawRefresh != "" {
		refresh, err := strconv.ParseBool(rawRefresh)
		if err != nil {
			return accounts.AccountOverviewCardsRequest{}, err
		}
		req.Refresh = refresh
	}

	return req.Normalized(), nil
}

func decodeAccountID(r *http.Request) (int, error) {
	return strconv.Atoi(chi.URLParam(r, "account_id"))
}

func validateRefreshTokenState(raw string) error {
	switch accounts.NormalizeRefreshTokenState(raw) {
	case "", "has", "missing":
		return nil
	default:
		return errors.New("invalid refresh_token_state")
	}
}

func writeAccountError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, accounts.ErrAccountNotFound) {
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
