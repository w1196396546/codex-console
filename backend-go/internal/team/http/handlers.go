package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	teamdomain "github.com/dou-jiang/codex-console/backend-go/internal/team"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *teamdomain.Service
	tasks   *teamdomain.TaskService
}

func NewHandler(service *teamdomain.Service, tasks *teamdomain.TaskService) *Handler {
	return &Handler{
		service: service,
		tasks:   tasks,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/team", func(r chi.Router) {
		r.Post("/discovery/run", h.runDiscovery)
		r.Post("/discovery/{account_id}", h.runSingleDiscovery)
		r.Get("/teams", h.listTeams)
		r.Get("/teams/{team_id}", h.getTeamDetail)
		r.Post("/teams/{team_id}/sync", h.syncTeam)
		r.Post("/teams/sync-batch", h.syncTeamsBatch)
		r.Get("/teams/{team_id}/memberships", h.listMemberships)
		r.Post("/teams/{team_id}/memberships/{membership_id}/revoke", h.revokeMembership)
		r.Post("/teams/{team_id}/memberships/{membership_id}/remove", h.removeMembership)
		r.Post("/memberships/{membership_id}/bind-local-account", h.bindMembershipLocalAccount)
		r.Post("/teams/{team_id}/invite-accounts", h.inviteAccounts)
		r.Post("/teams/{team_id}/invite-emails", h.inviteEmails)
		r.Get("/tasks", h.listTasks)
		r.Get("/tasks/{task_uuid}", h.getTaskDetail)
	})
}

type discoveryRunRequest struct {
	IDs []int64 `json:"ids"`
}

type syncBatchRequest struct {
	IDs []int64 `json:"ids"`
}

type inviteAccountsRequest struct {
	IDs                     []int64 `json:"ids"`
	SelectAll               bool    `json:"select_all"`
	StatusFilter            string  `json:"status_filter"`
	EmailServiceFilter      string  `json:"email_service_filter"`
	SearchFilter            string  `json:"search_filter"`
	RefreshTokenStateFilter string  `json:"refresh_token_state_filter"`
	SkipExistingMembership  bool    `json:"skip_existing_membership"`
	ResendInvited           bool    `json:"resend_invited"`
}

type inviteEmailsRequest struct {
	Emails []string `json:"emails"`
}

type bindLocalAccountRequest struct {
	AccountID int64 `json:"account_id"`
}

func (h *Handler) runDiscovery(w http.ResponseWriter, r *http.Request) {
	var req discoveryRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		writeDetailError(w, http.StatusBadRequest, "ids 不能为空")
		return
	}
	response, err := h.tasks.StartDiscovery(r.Context(), req.IDs)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) runSingleDiscovery(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	response, err := h.tasks.StartDiscovery(r.Context(), []int64{accountID})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request) {
	page := readPositiveIntQuery(r, "page", 1)
	perPage := readPositiveIntQuery(r, "per_page", 20)
	ownerAccountID := readPositiveInt64Query(r, "owner_account_id")

	response, err := h.service.ListTeams(r.Context(), teamdomain.ListTeamsRequest{
		Page:           page,
		PerPage:        perPage,
		Status:         r.URL.Query().Get("status"),
		OwnerAccountID: ownerAccountID,
		Search:         r.URL.Query().Get("search"),
	})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) getTeamDetail(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid team_id")
		return
	}
	response, err := h.service.GetTeamDetail(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, teamdomain.ErrNotFound) {
			writeDetailError(w, http.StatusNotFound, "Team 不存在")
			return
		}
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) syncTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid team_id")
		return
	}
	response, err := h.tasks.StartTeamSync(r.Context(), teamID)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) syncTeamsBatch(w http.ResponseWriter, r *http.Request) {
	var req syncBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		writeDetailError(w, http.StatusBadRequest, "ids 不能为空")
		return
	}
	response, err := h.tasks.StartTeamSyncBatch(r.Context(), req.IDs)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	response.AcceptedCount = len(req.IDs)
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) listMemberships(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid team_id")
		return
	}
	binding := r.URL.Query().Get("binding")
	if binding == "" {
		binding = "all"
	}
	if binding != "all" && binding != "local" && binding != "external" {
		writeDetailError(w, http.StatusBadRequest, "无效的 binding")
		return
	}
	response, err := h.service.ListMemberships(r.Context(), teamdomain.ListMembershipsRequest{
		TeamID:  teamID,
		Status:  r.URL.Query().Get("status"),
		Binding: binding,
		Search:  r.URL.Query().Get("search"),
	})
	if err != nil {
		if errors.Is(err, teamdomain.ErrNotFound) {
			writeDetailError(w, http.StatusNotFound, "Team 不存在")
			return
		}
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) revokeMembership(w http.ResponseWriter, r *http.Request) {
	membershipID, err := strconv.ParseInt(chi.URLParam(r, "membership_id"), 10, 64)
	if err != nil || membershipID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid membership_id")
		return
	}
	response, err := h.service.ApplyMembershipAction(r.Context(), teamdomain.ApplyMembershipActionRequest{
		MembershipID: membershipID,
		Action:       "revoke",
	})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	if !response.Success {
		writeDetailError(w, statusOrDefault(response.ErrorCode, http.StatusBadRequest), response.Message)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) removeMembership(w http.ResponseWriter, r *http.Request) {
	membershipID, err := strconv.ParseInt(chi.URLParam(r, "membership_id"), 10, 64)
	if err != nil || membershipID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid membership_id")
		return
	}
	response, err := h.service.ApplyMembershipAction(r.Context(), teamdomain.ApplyMembershipActionRequest{
		MembershipID: membershipID,
		Action:       "remove",
	})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	if !response.Success {
		writeDetailError(w, statusOrDefault(response.ErrorCode, http.StatusBadRequest), response.Message)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) bindMembershipLocalAccount(w http.ResponseWriter, r *http.Request) {
	membershipID, err := strconv.ParseInt(chi.URLParam(r, "membership_id"), 10, 64)
	if err != nil || membershipID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid membership_id")
		return
	}
	var req bindLocalAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, err := h.service.ApplyMembershipAction(r.Context(), teamdomain.ApplyMembershipActionRequest{
		MembershipID: membershipID,
		Action:       "bind-local-account",
		AccountID:    ptrInt64(req.AccountID),
	})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	if !response.Success {
		writeDetailError(w, statusOrDefault(response.ErrorCode, http.StatusBadRequest), response.Message)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) inviteAccounts(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid team_id")
		return
	}
	var req inviteAccountsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, err := h.tasks.StartInviteAccounts(r.Context(), teamID, req.IDs)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	response.AcceptedCount = len(req.IDs)
	response.DeduplicatedCount = countUniqueInts(req.IDs)
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) inviteEmails(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeDetailError(w, http.StatusBadRequest, "invalid team_id")
		return
	}
	var req inviteEmailsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDetailError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, err := h.tasks.StartInviteEmails(r.Context(), teamID, req.Emails)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	response.AcceptedCount = len(req.Emails)
	response.DeduplicatedCount = countUniqueEmails(req.Emails)
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	teamID := readPositiveInt64PointerQuery(r, "team_id")
	response, err := h.tasks.ListTasks(r.Context(), teamdomain.ListTasksRequest{TeamID: teamID})
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) getTaskDetail(w http.ResponseWriter, r *http.Request) {
	taskUUID := chi.URLParam(r, "task_uuid")
	if strings.TrimSpace(taskUUID) == "" {
		writeDetailError(w, http.StatusBadRequest, "invalid task_uuid")
		return
	}
	response, err := h.tasks.GetTaskDetail(r.Context(), taskUUID)
	if err != nil {
		if errors.Is(err, teamdomain.ErrNotFound) {
			writeDetailError(w, http.StatusNotFound, "任务不存在")
			return
		}
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func readPositiveIntQuery(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func readPositiveInt64Query(r *http.Request, key string) int64 {
	value := r.URL.Query().Get(key)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func readPositiveInt64PointerQuery(r *http.Request, key string) *int64 {
	value := readPositiveInt64Query(r, key)
	if value <= 0 {
		return nil
	}
	return ptrInt64(value)
}

func writeTaskError(w http.ResponseWriter, err error) {
	var conflictErr *teamdomain.ConflictError
	if errors.As(err, &conflictErr) {
		writeDetailError(w, http.StatusConflict, conflictErr.Error())
		return
	}
	writeDetailError(w, http.StatusBadRequest, err.Error())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeDetailError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func statusOrDefault(status int, fallback int) int {
	if status <= 0 {
		return fallback
	}
	return status
}

func ptrInt64(value int64) *int64 {
	return &value
}

func countUniqueInts(values []int64) int {
	seen := map[int64]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		seen[value] = struct{}{}
	}
	return len(seen)
}

func countUniqueEmails(values []string) int {
	seen := map[string]struct{}{}
	for _, value := range values {
		email := strings.ToLower(strings.TrimSpace(value))
		if email == "" {
			continue
		}
		seen[email] = struct{}{}
	}
	return len(seen)
}
