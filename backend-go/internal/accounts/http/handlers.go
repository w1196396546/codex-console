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

type createManualAccountService interface {
	CreateManualAccount(ctx context.Context, req accounts.ManualAccountCreateRequest) (accounts.Account, error)
}

type importAccountsService interface {
	ImportAccounts(ctx context.Context, req accounts.ImportAccountsRequest) (accounts.ImportAccountsResult, error)
}

type updateAccountService interface {
	UpdateAccount(ctx context.Context, accountID int, req accounts.AccountUpdateRequest) (accounts.Account, error)
}

type deleteAccountService interface {
	DeleteAccount(ctx context.Context, accountID int) (accounts.ActionResponse, error)
}

type batchDeleteAccountsService interface {
	BatchDeleteAccounts(ctx context.Context, req accounts.AccountSelectionRequest) (accounts.BatchDeleteResponse, error)
}

type batchUpdateAccountsService interface {
	BatchUpdateAccounts(ctx context.Context, req accounts.BatchUpdateRequest) (accounts.BatchUpdateResponse, error)
}

type exportAccountsService interface {
	ExportAccounts(ctx context.Context, format string, req accounts.AccountSelectionRequest) (accounts.AccountExportResponse, error)
}

type batchRefreshTokensService interface {
	BatchRefreshTokens(ctx context.Context, req accounts.BatchTokenRefreshRequest) (accounts.BatchRefreshResponse, error)
}

type refreshAccountTokenService interface {
	RefreshAccountToken(ctx context.Context, accountID int, req accounts.TokenRefreshRequest) (accounts.TokenRefreshActionResponse, error)
}

type batchValidateTokensService interface {
	BatchValidateTokens(ctx context.Context, req accounts.BatchTokenValidateRequest) (accounts.BatchValidateResponse, error)
}

type validateAccountTokenService interface {
	ValidateAccountToken(ctx context.Context, accountID int, req accounts.TokenValidateRequest) (accounts.TokenValidateResponse, error)
}

type batchCPAUploadService interface {
	BatchUploadAccountsCPA(ctx context.Context, req accounts.BatchCPAUploadRequest) (accounts.BatchUploadResponse, error)
}

type uploadCPAService interface {
	UploadAccountCPA(ctx context.Context, accountID int, req accounts.CPAUploadRequest) (accounts.ActionResponse, error)
}

type batchSub2APIUploadService interface {
	BatchUploadAccountsSub2API(ctx context.Context, req accounts.BatchSub2APIUploadRequest) (accounts.BatchUploadResponse, error)
}

type uploadSub2APIService interface {
	UploadAccountSub2API(ctx context.Context, accountID int, req accounts.Sub2APIUploadRequest) (accounts.ActionResponse, error)
}

type batchTMUploadService interface {
	BatchUploadAccountsTM(ctx context.Context, req accounts.BatchTMUploadRequest) (accounts.BatchUploadResponse, error)
}

type uploadTMService interface {
	UploadAccountTM(ctx context.Context, accountID int, req accounts.TMUploadRequest) (accounts.ActionResponse, error)
}

type removeOverviewCardsService interface {
	RemoveOverviewCards(ctx context.Context, req accounts.OverviewCardDeleteRequest) (accounts.OverviewCardMutationResponse, error)
}

type restoreOverviewCardService interface {
	RestoreOverviewCard(ctx context.Context, accountID int) (accounts.ActionResponse, error)
}

type attachOverviewCardService interface {
	AttachOverviewCard(ctx context.Context, accountID int) (accounts.OverviewAttachResponse, error)
}

type refreshOverviewService interface {
	RefreshOverview(ctx context.Context, req accounts.OverviewRefreshRequest) (accounts.OverviewRefreshResponse, error)
}

type inboxCodeService interface {
	GetInboxCode(ctx context.Context, accountID int) (accounts.InboxCodeResponse, error)
}

type Handler struct {
	service any
}

func NewHandler(service any) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/accounts", h.CreateManualAccount)
	r.Post("/api/accounts/import", h.ImportAccounts)
	r.Get("/api/accounts/current", h.GetCurrentAccount)
	r.Get("/api/accounts/stats/summary", h.GetAccountsStatsSummary)
	r.Get("/api/accounts/stats/overview", h.GetAccountsOverviewStats)
	r.Get("/api/accounts/overview/cards", h.ListOverviewCards)
	r.Get("/api/accounts/overview/cards/selectable", h.ListOverviewSelectable)
	r.Post("/api/accounts/overview/cards/remove", h.RemoveOverviewCards)
	r.Post("/api/accounts/overview/cards/{account_id}/restore", h.RestoreOverviewCard)
	r.Post("/api/accounts/overview/cards/{account_id}/attach", h.AttachOverviewCard)
	r.Post("/api/accounts/overview/refresh", h.RefreshOverview)
	r.Post("/api/accounts/export/{format}", h.ExportAccounts)
	r.Post("/api/accounts/batch-delete", h.BatchDeleteAccounts)
	r.Post("/api/accounts/batch-update", h.BatchUpdateAccounts)
	r.Post("/api/accounts/batch-refresh", h.BatchRefreshTokens)
	r.Post("/api/accounts/{account_id}/refresh", h.RefreshAccountToken)
	r.Post("/api/accounts/batch-validate", h.BatchValidateTokens)
	r.Post("/api/accounts/{account_id}/validate", h.ValidateAccountToken)
	r.Post("/api/accounts/batch-upload-cpa", h.BatchUploadAccountsCPA)
	r.Post("/api/accounts/{account_id}/upload-cpa", h.UploadAccountCPA)
	r.Post("/api/accounts/batch-upload-sub2api", h.BatchUploadAccountsSub2API)
	r.Post("/api/accounts/{account_id}/upload-sub2api", h.UploadAccountSub2API)
	r.Post("/api/accounts/batch-upload-tm", h.BatchUploadAccountsTM)
	r.Post("/api/accounts/{account_id}/upload-tm", h.UploadAccountTM)
	r.Post("/api/accounts/{account_id}/inbox-code", h.GetInboxCode)
	r.Get("/api/accounts/{account_id}/tokens", h.GetAccountTokens)
	r.Get("/api/accounts/{account_id}/cookies", h.GetAccountCookies)
	r.Patch("/api/accounts/{account_id}", h.UpdateAccount)
	r.Delete("/api/accounts/{account_id}", h.DeleteAccount)
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

func (h *Handler) CreateManualAccount(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(createManualAccountService)
	if !ok {
		http.Error(w, "create account service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.ManualAccountCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.CreateManualAccount(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ImportAccounts(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(importAccountsService)
	if !ok {
		http.Error(w, "import accounts service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.ImportAccountsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.ImportAccounts(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(updateAccountService)
	if !ok {
		http.Error(w, "update account service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.AccountUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.UpdateAccount(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(deleteAccountService)
	if !ok {
		http.Error(w, "delete account service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.DeleteAccount(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchDeleteAccounts(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchDeleteAccountsService)
	if !ok {
		http.Error(w, "batch delete accounts service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.AccountSelectionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchDeleteAccounts(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchUpdateAccounts(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchUpdateAccountsService)
	if !ok {
		http.Error(w, "batch update accounts service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchUpdateAccounts(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ExportAccounts(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(exportAccountsService)
	if !ok {
		http.Error(w, "export accounts service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.AccountSelectionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.ExportAccounts(r.Context(), chi.URLParam(r, "format"), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	w.Header().Set("Content-Type", resp.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+resp.Filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Content)
}

func (h *Handler) BatchRefreshTokens(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchRefreshTokensService)
	if !ok {
		http.Error(w, "batch refresh tokens service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchTokenRefreshRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchRefreshTokens(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RefreshAccountToken(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(refreshAccountTokenService)
	if !ok {
		http.Error(w, "refresh account token service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.TokenRefreshRequest
	_ = decodeJSONBodyAllowEmpty(r, &req)
	resp, err := svc.RefreshAccountToken(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchValidateTokens(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchValidateTokensService)
	if !ok {
		http.Error(w, "batch validate tokens service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchTokenValidateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchValidateTokens(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ValidateAccountToken(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(validateAccountTokenService)
	if !ok {
		http.Error(w, "validate account token service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.TokenValidateRequest
	_ = decodeJSONBodyAllowEmpty(r, &req)
	resp, err := svc.ValidateAccountToken(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchUploadAccountsCPA(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchCPAUploadService)
	if !ok {
		http.Error(w, "batch cpa upload service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchCPAUploadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchUploadAccountsCPA(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UploadAccountCPA(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(uploadCPAService)
	if !ok {
		http.Error(w, "upload cpa service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.CPAUploadRequest
	_ = decodeJSONBodyAllowEmpty(r, &req)
	resp, err := svc.UploadAccountCPA(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchUploadAccountsSub2API(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchSub2APIUploadService)
	if !ok {
		http.Error(w, "batch sub2api upload service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchSub2APIUploadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchUploadAccountsSub2API(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UploadAccountSub2API(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(uploadSub2APIService)
	if !ok {
		http.Error(w, "upload sub2api service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.Sub2APIUploadRequest
	_ = decodeJSONBodyAllowEmpty(r, &req)
	resp, err := svc.UploadAccountSub2API(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) BatchUploadAccountsTM(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(batchTMUploadService)
	if !ok {
		http.Error(w, "batch tm upload service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.BatchTMUploadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.BatchUploadAccountsTM(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UploadAccountTM(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(uploadTMService)
	if !ok {
		http.Error(w, "upload tm service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req accounts.TMUploadRequest
	_ = decodeJSONBodyAllowEmpty(r, &req)
	resp, err := svc.UploadAccountTM(r.Context(), accountID, req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RemoveOverviewCards(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(removeOverviewCardsService)
	if !ok {
		http.Error(w, "remove overview cards service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.OverviewCardDeleteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.RemoveOverviewCards(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RestoreOverviewCard(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(restoreOverviewCardService)
	if !ok {
		http.Error(w, "restore overview card service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.RestoreOverviewCard(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) AttachOverviewCard(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(attachOverviewCardService)
	if !ok {
		http.Error(w, "attach overview card service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.AttachOverviewCard(r.Context(), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RefreshOverview(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(refreshOverviewService)
	if !ok {
		http.Error(w, "refresh overview service not configured", http.StatusInternalServerError)
		return
	}
	var req accounts.OverviewRefreshRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.RefreshOverview(r.Context(), req)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetInboxCode(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service.(inboxCodeService)
	if !ok {
		http.Error(w, "inbox code service not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := decodeAccountID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := svc.GetInboxCode(r.Context(), accountID)
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

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	return json.NewDecoder(r.Body).Decode(target)
}

func decodeJSONBodyAllowEmpty(r *http.Request, target any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(target)
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
	if errors.Is(err, accounts.ErrAccountAlreadyExists) {
		status = http.StatusConflict
	}
	if errors.Is(err, accounts.ErrUnsupportedExportFormat) {
		status = http.StatusBadRequest
	}
	http.Error(w, err.Error(), status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
