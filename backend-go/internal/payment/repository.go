package payment

import (
	"context"
	"fmt"
	"strings"
)

func normalizeListRequest(req ListBindCardTasksRequest) ListBindCardTasksRequest {
	normalized := req
	if normalized.Page < 1 {
		normalized.Page = 1
	}
	if normalized.PageSize < 1 {
		normalized.PageSize = 20
	}
	if normalized.PageSize > 100 {
		normalized.PageSize = 100
	}
	normalized.Status = strings.TrimSpace(normalized.Status)
	normalized.Search = strings.TrimSpace(normalized.Search)
	return normalized
}

func normalizeStatus(status string) string {
	value := strings.TrimSpace(strings.ToLower(status))
	switch value {
	case StatusLinkReady, StatusOpened, StatusWaitingUserAction, StatusVerifying, StatusPaidPendingSync, StatusCompleted, StatusFailed:
		return value
	default:
		return value
	}
}

func normalizeBindMode(bindMode string) string {
	value := strings.TrimSpace(strings.ToLower(bindMode))
	if value == "" {
		return "semi_auto"
	}
	return value
}

func ensureSupportedSubscriptionType(value string) error {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "free", "plus", "team":
		return nil
	default:
		return fmt.Errorf("subscription_type 必须为 [free plus team]")
	}
}

func buildSelectionRequest(req BatchCheckSubscriptionRequest) accountsSelection {
	return accountsSelection{
		ids:                     append([]int(nil), req.IDs...),
		selectAll:               req.SelectAll,
		statusFilter:            strings.TrimSpace(req.StatusFilter),
		emailServiceFilter:      strings.TrimSpace(req.EmailServiceFilter),
		searchFilter:            strings.TrimSpace(req.SearchFilter),
		refreshTokenStateFilter: strings.TrimSpace(req.RefreshTokenStateFilter),
	}
}

type accountsSelection struct {
	ids                     []int
	selectAll               bool
	statusFilter            string
	emailServiceFilter      string
	searchFilter            string
	refreshTokenStateFilter string
}

func (s accountsSelection) ToRequest() map[string]any {
	return map[string]any{
		"ids":                        append([]int(nil), s.ids...),
		"select_all":                 s.selectAll,
		"status_filter":              s.statusFilter,
		"email_service_filter":       s.emailServiceFilter,
		"search_filter":              s.searchFilter,
		"refresh_token_state_filter": s.refreshTokenStateFilter,
	}
}

type bindCardTaskRepository interface {
	Repository
}

type accountReader interface {
	AccountsRepository
}

type serviceContext interface {
	context.Context
}
