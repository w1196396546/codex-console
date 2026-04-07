package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

type TransitionDiscoverHook func(ctx context.Context, ownerAccountIDs []int64) (TaskExecutionResult, error)
type TransitionSyncHook func(ctx context.Context, teamID int64, taskType string, payload map[string]any) (TaskExecutionResult, error)
type TransitionInviteHook func(ctx context.Context, teamID int64, taskType string, payload map[string]any) (TaskExecutionResult, error)

type TransitionExecutorHooks struct {
	Discover TransitionDiscoverHook
	Sync     TransitionSyncHook
	Invite   TransitionInviteHook
}

type transitionTaskExecutor struct {
	repository Repository
	hooks      TransitionExecutorHooks
}

type transitionMutationRepository interface {
	UpsertTeam(ctx context.Context, team TeamRecord) (TeamRecord, error)
	UpsertMembership(ctx context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error)
}

type transitionAccountEmailRepository interface {
	ListAccountsByEmails(ctx context.Context, emails []string) (map[string]AccountRecord, error)
}

type transitionRemoteTeam struct {
	UpstreamAccountID   string
	TeamName            string
	PlanType            string
	SubscriptionPlan    string
	AccountRoleSnapshot string
	ExpiresAt           *time.Time
}

type transitionRemoteMember struct {
	UpstreamUserID string
	Email          string
	Role           string
	CreatedAt      *time.Time
}

type transitionRemoteInvite struct {
	Email     string
	Role      string
	CreatedAt *time.Time
}

type transitionMembershipSnapshot struct {
	Existing       *TeamMembershipRecord
	ExistingStatus string
	Statuses       []string
	LocalAccountID *int64
	UpstreamUserID string
	MemberRole     string
	InvitedAt      *time.Time
	JoinedAt       *time.Time
	RemovedAt      *time.Time
	Source         string
}

const transitionMemberPageLimit = 100

var transitionSkippedDiscoveryStatuses = map[string]struct{}{
	"failed":  {},
	"expired": {},
	"banned":  {},
}

var transitionAbsentMembershipStatuses = map[string]string{
	"invited":        "revoked",
	"joined":         "removed",
	"already_member": "removed",
}

var transitionFullTeamTokens = []string{
	"maximum number of seats reached",
	"team is full",
	"workspace is full",
	"no seats",
	"seat limit",
}

var transitionAlreadyMemberTokens = []string{
	"already in workspace",
	"already a member",
	"already member",
}

func NewTransitionTaskExecutor(repository Repository, hooks TransitionExecutorHooks) TaskExecutor {
	return &transitionTaskExecutor{
		repository: repository,
		hooks:      hooks,
	}
}

func (e *transitionTaskExecutor) Execute(ctx context.Context, task TaskExecutionRequest) (TaskExecutionResult, error) {
	switch strings.TrimSpace(task.TaskType) {
	case "discover_owner_teams":
		ownerAccountIDs := resolveTransitionOwnerIDs(task)
		if e.hooks.Discover != nil {
			return e.hooks.Discover(ctx, ownerAccountIDs)
		}
		return e.executeDiscoveryTransition(ctx, ownerAccountIDs)
	case "sync_team", "sync_all_teams":
		teamIDs := resolveTransitionTeamIDs(task)
		teamID, ok := resolveTransitionTeamID(task)
		if !ok {
			return TaskExecutionResult{}, fmt.Errorf("team_id is required for %s", task.TaskType)
		}
		if e.hooks.Sync != nil {
			return e.hooks.Sync(ctx, teamID, task.TaskType, cloneMap(task.RequestPayload))
		}
		return e.executeSyncTransition(ctx, teamIDs, task.TaskType)
	case "invite_accounts", "invite_emails":
		teamID, ok := resolveTransitionTeamID(task)
		if !ok {
			return TaskExecutionResult{}, fmt.Errorf("team_id is required for %s", task.TaskType)
		}
		if e.hooks.Invite != nil {
			return e.hooks.Invite(ctx, teamID, task.TaskType, cloneMap(task.RequestPayload))
		}
		return e.executeInviteTransition(ctx, teamID, task)
	default:
		return TaskExecutionResult{}, fmt.Errorf("unsupported team task type: %s", task.TaskType)
	}
}

func resolveTransitionOwnerIDs(task TaskExecutionRequest) []int64 {
	resolved := normalizeTransitionIDs(task.RequestPayload["ids"])
	if len(resolved) > 0 {
		return resolved
	}
	if task.OwnerAccountID != nil && *task.OwnerAccountID > 0 {
		return []int64{*task.OwnerAccountID}
	}
	return nil
}

func resolveTransitionTeamID(task TaskExecutionRequest) (int64, bool) {
	if task.TeamID != nil && *task.TeamID > 0 {
		return *task.TeamID, true
	}
	resolved := resolveTransitionTeamIDs(task)
	if len(resolved) > 0 {
		return resolved[0], true
	}
	if value, ok := toInt64(task.RequestPayload["team_id"]); ok && value > 0 {
		return value, true
	}
	return 0, false
}

func resolveTransitionTeamIDs(task TaskExecutionRequest) []int64 {
	resolved := normalizeTransitionIDs(task.RequestPayload["ids"])
	if len(resolved) > 0 {
		return resolved
	}
	if task.TeamID != nil && *task.TeamID > 0 {
		return []int64{*task.TeamID}
	}
	if value, ok := toInt64(task.RequestPayload["team_id"]); ok && value > 0 {
		return []int64{value}
	}
	return nil
}

func normalizeTransitionIDs(raw any) []int64 {
	var values []int64
	switch typed := raw.(type) {
	case []int64:
		values = append(values, typed...)
	case []int:
		values = make([]int64, 0, len(typed))
		for _, value := range typed {
			values = append(values, int64(value))
		}
	case []any:
		values = make([]int64, 0, len(typed))
		for _, value := range typed {
			if resolved, ok := toInt64(value); ok {
				values = append(values, resolved)
			}
		}
	case []float64:
		values = make([]int64, 0, len(typed))
		for _, value := range typed {
			values = append(values, int64(value))
		}
	default:
		return nil
	}
	return normalizeUniqueInt64s(values)
}

func toInt64(value any) (int64, bool) {
	switch resolved := value.(type) {
	case int64:
		return resolved, true
	case int:
		return int64(resolved), true
	case float64:
		return int64(resolved), true
	case json.Number:
		parsed, err := resolved.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func transitionUnsupportedResult(taskType string) TaskExecutionResult {
	message := "team transition executor has no live hook for " + strings.TrimSpace(taskType)
	return TaskExecutionResult{
		Status: jobs.StatusFailed,
		Summary: map[string]any{
			"detail": message,
		},
		Logs: []string{message},
	}
}

func (e *transitionTaskExecutor) executeDiscoveryTransition(ctx context.Context, ownerAccountIDs []int64) (TaskExecutionResult, error) {
	normalizedIDs := normalizeUniqueInt64s(ownerAccountIDs)
	summary := map[string]any{
		"accounts_scanned": len(normalizedIDs),
		"teams_found":      0,
		"teams_persisted":  0,
		"transition":       true,
	}
	if e.repository == nil || len(normalizedIDs) == 0 {
		return TaskExecutionResult{
			Status:  jobs.StatusCompleted,
			Summary: summary,
			Logs:    []string{"transition discovery completed with no owner accounts"},
		}, nil
	}

	mutationRepo, ok := e.repository.(transitionMutationRepository)
	if !ok {
		return TaskExecutionResult{}, fmt.Errorf("team transition repository does not support discovery persistence")
	}

	accountsByID, err := e.repository.ListAccountsByIDs(ctx, normalizedIDs)
	if err != nil {
		return TaskExecutionResult{}, err
	}

	scannedCount := 0
	totalTeams := 0
	persistedCount := 0
	for _, ownerAccountID := range normalizedIDs {
		account, ok := accountsByID[ownerAccountID]
		if !ok {
			continue
		}
		if normalizeText(account.AccessToken) == "" {
			continue
		}
		if _, skipped := transitionSkippedDiscoveryStatuses[normalizeText(account.Status)]; skipped {
			continue
		}

		scannedCount++
		payload, err := executeTransitionJSONRequest(ctx, http.MethodGet, "/backend-api/accounts/check/v4-2023-04-27", account.AccessToken, nil, nil)
		if err != nil {
			return TaskExecutionResult{}, err
		}
		discoveredTeams, err := parseTransitionTeamAccounts(payload)
		if err != nil {
			return TaskExecutionResult{}, err
		}

		totalTeams += len(discoveredTeams)
		for _, discoveredTeam := range discoveredTeams {
			if normalizeText(discoveredTeam.AccountRoleSnapshot) != "account-owner" {
				continue
			}
			now := time.Now().UTC()
			if _, err := mutationRepo.UpsertTeam(ctx, TeamRecord{
				OwnerAccountID:      ownerAccountID,
				UpstreamAccountID:   discoveredTeam.UpstreamAccountID,
				TeamName:            discoveredTeam.TeamName,
				PlanType:            discoveredTeam.PlanType,
				SubscriptionPlan:    discoveredTeam.SubscriptionPlan,
				AccountRoleSnapshot: discoveredTeam.AccountRoleSnapshot,
				Status:              defaultIfEmpty(account.Status, "active"),
				ExpiresAt:           discoveredTeam.ExpiresAt,
				LastSyncAt:          &now,
				SyncStatus:          "synced",
				SyncError:           "",
			}); err != nil {
				return TaskExecutionResult{}, err
			}
			persistedCount++
		}
	}

	summary["accounts_scanned"] = scannedCount
	summary["teams_found"] = totalTeams
	summary["teams_persisted"] = persistedCount
	return TaskExecutionResult{
		Status:  jobs.StatusCompleted,
		Summary: summary,
		Logs: []string{
			fmt.Sprintf("transition discovery scanned %d owner accounts", scannedCount),
			fmt.Sprintf("transition discovery found %d teams", totalTeams),
		},
	}, nil
}

func (e *transitionTaskExecutor) executeSyncTransition(ctx context.Context, teamIDs []int64, taskType string) (TaskExecutionResult, error) {
	normalizedIDs := normalizeUniqueInt64s(teamIDs)
	if len(normalizedIDs) == 0 {
		return TaskExecutionResult{}, fmt.Errorf("team_id is required for %s", taskType)
	}

	requestedCount := len(normalizedIDs)
	processedCount := 0
	results := make([]map[string]any, 0, requestedCount)
	logs := make([]string, 0, requestedCount)
	items := make([]TaskExecutionItem, 0)
	mutationRepo, ok := e.repository.(transitionMutationRepository)
	if !ok {
		return TaskExecutionResult{}, fmt.Errorf("team transition repository does not support sync persistence")
	}
	accountEmailRepo, _ := e.repository.(transitionAccountEmailRepository)

	for _, teamID := range normalizedIDs {
		teamRecord, err := e.repository.GetTeam(ctx, teamID)
		if err != nil {
			return TaskExecutionResult{}, err
		}
		ownerAccount, err := e.repository.GetAccount(ctx, teamRecord.OwnerAccountID)
		if err != nil {
			return TaskExecutionResult{}, err
		}
		if normalizeText(ownerAccount.AccessToken) == "" {
			return TaskExecutionResult{}, fmt.Errorf("owner account %d missing access token for team %d", ownerAccount.ID, teamID)
		}

		memberPayloads, err := fetchTransitionMemberPages(ctx, teamRecord.UpstreamAccountID, ownerAccount.AccessToken)
		if err != nil {
			markTransitionSyncFailure(ctx, e.repository, teamRecord, err)
			return TaskExecutionResult{}, err
		}
		remoteMembers, err := collectTransitionMembers(memberPayloads)
		if err != nil {
			markTransitionSyncFailure(ctx, e.repository, teamRecord, err)
			return TaskExecutionResult{}, err
		}
		invitePayload, err := executeTransitionJSONRequest(ctx, http.MethodGet, fmt.Sprintf("/backend-api/accounts/%s/invites", strings.TrimSpace(teamRecord.UpstreamAccountID)), ownerAccount.AccessToken, nil, nil)
		if err != nil {
			markTransitionSyncFailure(ctx, e.repository, teamRecord, err)
			return TaskExecutionResult{}, err
		}
		remoteInvites, err := parseTransitionInvites(invitePayload)
		if err != nil {
			markTransitionSyncFailure(ctx, e.repository, teamRecord, err)
			return TaskExecutionResult{}, err
		}

		existingMemberships, err := e.repository.ListMembershipsByTeam(ctx, teamID)
		if err != nil {
			return TaskExecutionResult{}, err
		}
		snapshot := seedTransitionMembershipSnapshot(existingMemberships)
		mergeTransitionRemoteMembers(snapshot, remoteMembers)
		mergeTransitionRemoteInvites(snapshot, remoteInvites)

		emails := sortedTransitionSnapshotEmails(snapshot)
		accountsByEmail := map[string]AccountRecord{}
		if accountEmailRepo != nil && len(emails) > 0 {
			accountsByEmail, err = accountEmailRepo.ListAccountsByEmails(ctx, emails)
			if err != nil {
				return TaskExecutionResult{}, err
			}
		}

		syncedAt := time.Now().UTC()
		activeMemberCount := 0
		invitedCount := 0
		joinedCount := 0
		alreadyMemberCount := 0
		for _, email := range emails {
			entry := snapshot[email]
			beforeStatus := ""
			membership := TeamMembershipRecord{
				TeamID:      teamID,
				MemberEmail: email,
				Source:      "sync",
			}
			if entry.Existing != nil {
				beforeStatus = entry.Existing.MembershipStatus
				membership = *entry.Existing
			}
			finalStatus := resolveTransitionMembershipStatus(entry)
			if account, ok := accountsByEmail[email]; ok && account.ID > 0 {
				membership.LocalAccountID = ptrInt64(account.ID)
			} else {
				membership.LocalAccountID = entry.LocalAccountID
			}
			membership.MemberEmail = email
			membership.UpstreamUserID = entry.UpstreamUserID
			membership.MemberRole = defaultIfEmpty(entry.MemberRole, "member")
			membership.MembershipStatus = finalStatus
			membership.InvitedAt = entry.InvitedAt
			membership.JoinedAt = entry.JoinedAt
			if finalStatus == "removed" || finalStatus == "revoked" {
				if entry.RemovedAt != nil {
					membership.RemovedAt = entry.RemovedAt
				} else {
					membership.RemovedAt = &syncedAt
				}
			} else {
				membership.RemovedAt = nil
			}
			membership.LastSeenAt = &syncedAt
			if entry.Existing != nil && entry.Existing.Source == "manual_bind" {
				membership.Source = "manual_bind"
			} else {
				membership.Source = defaultIfEmpty(entry.Source, "sync")
			}
			membership.SyncError = ""
			if _, err := mutationRepo.UpsertMembership(ctx, membership); err != nil {
				return TaskExecutionResult{}, err
			}

			if _, ok := activeMembershipStatuses[finalStatus]; ok {
				activeMemberCount++
			}
			switch finalStatus {
			case "invited":
				invitedCount++
			case "joined":
				joinedCount++
			case "already_member":
				alreadyMemberCount++
			}
			items = append(items, TaskExecutionItem{
				TargetEmail: email,
				ItemStatus:  jobs.StatusCompleted,
				Before:      map[string]any{"membership_status": beforeStatus},
				After:       map[string]any{"membership_status": finalStatus},
				Message:     "membership state confirmed",
			})
		}

		teamRecord.CurrentMembers = activeMemberCount
		teamRecord.LastSyncAt = &syncedAt
		teamRecord.SyncStatus = "synced"
		teamRecord.SyncError = ""
		teamRecord.SeatsAvailable = ptrInt(calculateTransitionSeatsAvailable(activeMemberCount, teamRecord.MaxMembers))
		if _, err := e.repository.SaveTeam(ctx, teamRecord); err != nil {
			return TaskExecutionResult{}, err
		}

		processedCount++
		results = append(results, map[string]any{
			"team_id":              teamID,
			"status":               jobs.StatusCompleted,
			"membership_count":     len(emails),
			"active_member_count":  activeMemberCount,
			"invited_count":        invitedCount,
			"joined_count":         joinedCount,
			"already_member_count": alreadyMemberCount,
		})
		logs = append(logs, fmt.Sprintf("transition %s completed for team %d", taskType, teamID))
	}

	return TaskExecutionResult{
		Status: jobs.StatusCompleted,
		Summary: map[string]any{
			"requested_count": requestedCount,
			"processed_count": processedCount,
			"failed_count":    0,
			"results":         results,
			"transition":      true,
		},
		Logs:  logs,
		Items: items,
	}, nil
}

func (e *transitionTaskExecutor) executeInviteTransition(ctx context.Context, teamID int64, task TaskExecutionRequest) (TaskExecutionResult, error) {
	if e.repository == nil {
		return TaskExecutionResult{}, fmt.Errorf("team transition repository not configured")
	}
	mutationRepo, ok := e.repository.(transitionMutationRepository)
	if !ok {
		return TaskExecutionResult{}, fmt.Errorf("team transition repository does not support invite persistence")
	}

	teamRecord, err := e.repository.GetTeam(ctx, teamID)
	if err != nil {
		return TaskExecutionResult{}, err
	}
	ownerAccount, err := e.repository.GetAccount(ctx, teamRecord.OwnerAccountID)
	if err != nil {
		return TaskExecutionResult{}, err
	}
	if normalizeText(ownerAccount.AccessToken) == "" {
		return TaskExecutionResult{}, fmt.Errorf("owner account %d missing access token for team %d", ownerAccount.ID, teamID)
	}

	existingMemberships, err := e.repository.ListMembershipsByTeam(ctx, teamID)
	if err != nil {
		return TaskExecutionResult{}, err
	}
	existingByEmail := make(map[string]TeamMembershipRecord, len(existingMemberships))
	for _, membership := range existingMemberships {
		existingByEmail[normalizeEmail(membership.MemberEmail)] = membership
	}

	emails, err := e.resolveInviteEmails(ctx, task)
	if err != nil {
		return TaskExecutionResult{}, err
	}
	if len(emails) == 0 {
		return TaskExecutionResult{}, fmt.Errorf("未找到可邀请邮箱")
	}

	accountByEmail := map[string]AccountRecord{}
	if accountEmailRepo, ok := e.repository.(transitionAccountEmailRepository); ok {
		accountByEmail, err = accountEmailRepo.ListAccountsByEmails(ctx, emails)
		if err != nil {
			return TaskExecutionResult{}, err
		}
	}

	items := make([]TaskExecutionItem, 0, len(emails))
	results := make([]map[string]any, 0, len(emails))
	logs := []string{
		"未触发子号自动刷新 RT",
		"未触发子号自动注册",
	}
	skipRemaining := false
	invitedAt := time.Now().UTC()
	for _, email := range emails {
		if skipRemaining {
			results = append(results, map[string]any{
				"email":                        email,
				"success":                      false,
				"message":                      "skipped because team is full",
				"next_status":                  "skipped",
				"child_refresh_triggered":      false,
				"child_registration_triggered": false,
			})
			logs = append(logs, fmt.Sprintf("skipped invite for %s because team is full", email))
			continue
		}

		beforeStatus := ""
		if existing, ok := existingByEmail[email]; ok {
			beforeStatus = existing.MembershipStatus
		}
		nextStatus := "invited"
		message := "invite sent"
		success := true
		itemStatus := jobs.StatusCompleted
		err := executeTransitionNoContentRequest(ctx, http.MethodPost, fmt.Sprintf("/backend-api/accounts/%s/invites", strings.TrimSpace(teamRecord.UpstreamAccountID)), ownerAccount.AccessToken, map[string]string{
			"email_addresses": email,
		}, map[string]any{
			"email_addresses": []string{email},
			"role":            "standard-user",
			"resend_emails":   true,
		})
		if err != nil {
			errorMessage := strings.TrimSpace(err.Error())
			switch {
			case isTransitionAlreadyMemberError(errorMessage):
				nextStatus = "already_member"
				message = errorMessage
			case isTransitionFullTeamError(errorMessage):
				nextStatus = "failed"
				message = "team full: " + errorMessage
				success = false
				itemStatus = jobs.StatusFailed
				skipRemaining = true
			default:
				nextStatus = "failed"
				message = errorMessage
				success = false
				itemStatus = jobs.StatusFailed
			}
		}

		membership := TeamMembershipRecord{
			TeamID:           teamID,
			MemberEmail:      email,
			MemberRole:       "member",
			MembershipStatus: nextStatus,
			Source:           "invite",
			SyncError:        "",
		}
		if existing, ok := existingByEmail[email]; ok {
			membership = existing
			membership.MemberEmail = email
			membership.MemberRole = "member"
			membership.MembershipStatus = nextStatus
			membership.Source = "invite"
			membership.SyncError = ""
		}
		if account, ok := accountByEmail[email]; ok && account.ID > 0 {
			membership.LocalAccountID = ptrInt64(account.ID)
		}
		if nextStatus == "invited" || nextStatus == "already_member" || nextStatus == "failed" {
			membership.InvitedAt = &invitedAt
		}
		if _, err := mutationRepo.UpsertMembership(ctx, membership); err != nil {
			return TaskExecutionResult{}, err
		}

		items = append(items, TaskExecutionItem{
			TargetEmail: email,
			ItemStatus:  itemStatus,
			Before:      map[string]any{"membership_status": beforeStatus},
			After:       map[string]any{"membership_status": nextStatus},
			Message:     message,
		})
		result := map[string]any{
			"email":                        email,
			"success":                      success,
			"message":                      message,
			"next_status":                  nextStatus,
			"child_refresh_triggered":      false,
			"child_registration_triggered": false,
			"transition":                   true,
		}
		if membership.LocalAccountID != nil {
			result["local_account_id"] = *membership.LocalAccountID
		}
		results = append(results, result)
		logs = append(logs, message)
	}

	updatedMemberships, err := e.repository.ListMembershipsByTeam(ctx, teamID)
	if err != nil {
		return TaskExecutionResult{}, err
	}
	teamRecord.CurrentMembers = countTransitionActiveMembers(updatedMemberships)
	teamRecord.SeatsAvailable = ptrInt(calculateTransitionSeatsAvailable(teamRecord.CurrentMembers, teamRecord.MaxMembers))
	if _, err := e.repository.SaveTeam(ctx, teamRecord); err != nil {
		return TaskExecutionResult{}, err
	}

	return TaskExecutionResult{
		Status: jobs.StatusCompleted,
		Summary: map[string]any{
			"success":                      allTransitionInviteResultsSuccessful(results),
			"team_id":                      teamID,
			"results":                      results,
			"child_refresh_triggered":      false,
			"child_registration_triggered": false,
			"transition":                   true,
		},
		Logs:  logs,
		Items: items,
	}, nil
}

func (e *transitionTaskExecutor) resolveInviteEmails(ctx context.Context, task TaskExecutionRequest) ([]string, error) {
	if emails := normalizeTransitionEmails(task.RequestPayload["emails"]); len(emails) > 0 {
		return emails, nil
	}

	accountIDs := normalizeTransitionIDs(task.RequestPayload["ids"])
	if len(accountIDs) == 0 || e.repository == nil {
		return nil, nil
	}

	accountsByID, err := e.repository.ListAccountsByIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	emails := make([]string, 0, len(accountIDs))
	seen := map[string]struct{}{}
	for _, accountID := range accountIDs {
		record, ok := accountsByID[accountID]
		if !ok {
			continue
		}
		email := normalizeEmail(record.Email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	return emails, nil
}

func normalizeTransitionEmails(raw any) []string {
	values := make([]string, 0)
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, value := range typed {
			if resolved, ok := value.(string); ok {
				values = append(values, resolved)
			}
		}
	default:
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		email := normalizeEmail(value)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		normalized = append(normalized, email)
	}
	return normalized
}

func executeTransitionJSONRequest(ctx context.Context, method string, path string, accessToken string, query map[string]string, payload map[string]any) (map[string]any, error) {
	requestCtx, cancel := withTransitionHTTPRequestTimeout(ctx)
	defer cancel()

	var body io.Reader
	if payload != nil {
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal team transition request: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	requestURL := transitionTeamAPIBaseURL + strings.TrimSpace(path)
	if len(query) > 0 {
		params := url.Values{}
		for key, value := range query {
			if strings.TrimSpace(value) == "" {
				continue
			}
			params.Set(key, value)
		}
		if encoded := params.Encode(); encoded != "" {
			requestURL += "?" + encoded
		}
	}

	request, err := http.NewRequestWithContext(requestCtx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("build team transition request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", transitionTeamAPIBaseURL)
	request.Header.Set("Referer", transitionTeamAPIBaseURL+"/")
	if token := strings.TrimSpace(accessToken); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("team upstream request failed: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read team transition response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s", extractTransitionErrorMessage(response.StatusCode, bodyBytes))
	}
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		return nil, fmt.Errorf("decode team transition response: %w", err)
	}
	return decoded, nil
}

func executeTransitionNoContentRequest(ctx context.Context, method string, path string, accessToken string, query map[string]string, payload map[string]any) error {
	_, err := executeTransitionJSONRequest(ctx, method, path, accessToken, query, payload)
	return err
}

func parseTransitionTeamAccounts(payload map[string]any) ([]transitionRemoteTeam, error) {
	accounts, ok := payload["accounts"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("team accounts 响应缺少 accounts 映射")
	}

	teams := make([]transitionRemoteTeam, 0, len(accounts))
	for rawUpstreamAccountID, rawValue := range accounts {
		record, ok := rawValue.(map[string]any)
		if !ok {
			continue
		}
		accountPayload, _ := record["account"].(map[string]any)
		entitlementPayload, _ := record["entitlement"].(map[string]any)
		planType := transitionStringValue(accountPayload["plan_type"])
		subscriptionPlan := transitionStringValue(entitlementPayload["subscription_plan"])
		if normalizeText(planType) != "team" && normalizeText(subscriptionPlan) != "team" && !strings.Contains(normalizeText(subscriptionPlan), "team") {
			continue
		}
		upstreamAccountID := strings.TrimSpace(rawUpstreamAccountID)
		if upstreamAccountID == "" {
			continue
		}
		teams = append(teams, transitionRemoteTeam{
			UpstreamAccountID:   upstreamAccountID,
			TeamName:            defaultIfEmpty(transitionStringValue(accountPayload["name"]), "Team-"+upstreamAccountID),
			PlanType:            defaultIfEmpty(planType, "unknown"),
			SubscriptionPlan:    defaultIfEmpty(subscriptionPlan, "unknown"),
			AccountRoleSnapshot: defaultIfEmpty(transitionStringValue(accountPayload["account_user_role"]), "unknown"),
			ExpiresAt:           parseTransitionTime(transitionStringValue(entitlementPayload["expires_at"])),
		})
	}

	return teams, nil
}

func fetchTransitionMemberPages(ctx context.Context, upstreamAccountID string, accessToken string) ([]map[string]any, error) {
	pages := make([]map[string]any, 0)
	offset := 0
	for {
		payload, err := executeTransitionJSONRequest(ctx, http.MethodGet, fmt.Sprintf("/backend-api/accounts/%s/users", strings.TrimSpace(upstreamAccountID)), accessToken, map[string]string{
			"limit":  fmt.Sprintf("%d", transitionMemberPageLimit),
			"offset": fmt.Sprintf("%d", offset),
		}, nil)
		if err != nil {
			return nil, err
		}
		pages = append(pages, payload)

		items, _ := payload["items"].([]any)
		if len(items) == 0 {
			break
		}
		if total, ok := transitionIntValue(payload["total"]); ok && collectedTransitionItems(pages) >= total {
			break
		}
		if len(items) < transitionMemberPageLimit {
			break
		}
		offset += transitionMemberPageLimit
	}
	return pages, nil
}

func collectTransitionMembers(pages []map[string]any) ([]transitionRemoteMember, error) {
	collected := make([]transitionRemoteMember, 0)
	for _, payload := range pages {
		members, err := parseTransitionMembers(payload)
		if err != nil {
			return nil, err
		}
		collected = append(collected, members...)
	}
	return collected, nil
}

func parseTransitionMembers(payload map[string]any) ([]transitionRemoteMember, error) {
	items, ok := payload["items"].([]any)
	if !ok {
		return nil, fmt.Errorf("team members 响应中的 items 不是列表")
	}

	result := make([]transitionRemoteMember, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("team members.items 列表项不是对象")
		}
		upstreamUserID := strings.TrimSpace(transitionStringValue(item["id"]))
		if upstreamUserID == "" {
			return nil, fmt.Errorf("team members.items[].id 缺失")
		}
		result = append(result, transitionRemoteMember{
			UpstreamUserID: upstreamUserID,
			Email:          normalizeEmail(transitionStringValue(item["email"])),
			Role:           defaultIfEmpty(transitionStringValue(item["role"]), "member"),
			CreatedAt:      parseTransitionTime(transitionStringValue(item["created_time"])),
		})
	}
	return result, nil
}

func parseTransitionInvites(payload map[string]any) ([]transitionRemoteInvite, error) {
	items, ok := payload["items"].([]any)
	if !ok {
		return nil, fmt.Errorf("team invites 响应中的 items 不是列表")
	}

	result := make([]transitionRemoteInvite, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("team invites.items 列表项不是对象")
		}
		email := normalizeEmail(transitionStringValue(item["email_address"]))
		if email == "" {
			return nil, fmt.Errorf("team invites.items[].email_address 缺失")
		}
		result = append(result, transitionRemoteInvite{
			Email:     email,
			Role:      defaultIfEmpty(transitionStringValue(item["role"]), "member"),
			CreatedAt: parseTransitionTime(transitionStringValue(item["created_time"])),
		})
	}
	return result, nil
}

func seedTransitionMembershipSnapshot(memberships []TeamMembershipRecord) map[string]*transitionMembershipSnapshot {
	result := make(map[string]*transitionMembershipSnapshot, len(memberships))
	for _, membership := range memberships {
		email := normalizeEmail(membership.MemberEmail)
		if email == "" {
			continue
		}
		copied := membership
		result[email] = &transitionMembershipSnapshot{
			Existing:       &copied,
			ExistingStatus: membership.MembershipStatus,
			LocalAccountID: membership.LocalAccountID,
			UpstreamUserID: membership.UpstreamUserID,
			MemberRole:     membership.MemberRole,
			InvitedAt:      membership.InvitedAt,
			JoinedAt:       membership.JoinedAt,
			RemovedAt:      membership.RemovedAt,
			Source:         defaultIfEmpty(membership.Source, "sync"),
		}
	}
	return result
}

func ensureTransitionSnapshotEntry(snapshot map[string]*transitionMembershipSnapshot, email string) *transitionMembershipSnapshot {
	if entry, ok := snapshot[email]; ok {
		return entry
	}
	entry := &transitionMembershipSnapshot{Source: "sync"}
	snapshot[email] = entry
	return entry
}

func mergeTransitionRemoteMembers(snapshot map[string]*transitionMembershipSnapshot, members []transitionRemoteMember) {
	for _, member := range members {
		if member.Email == "" {
			continue
		}
		entry := ensureTransitionSnapshotEntry(snapshot, member.Email)
		entry.Statuses = append(entry.Statuses, "joined")
		entry.UpstreamUserID = member.UpstreamUserID
		entry.MemberRole = defaultIfEmpty(member.Role, entry.MemberRole)
		if member.CreatedAt != nil {
			entry.JoinedAt = member.CreatedAt
		}
		entry.RemovedAt = nil
		entry.Source = "sync"
	}
}

func mergeTransitionRemoteInvites(snapshot map[string]*transitionMembershipSnapshot, invites []transitionRemoteInvite) {
	for _, invite := range invites {
		if invite.Email == "" {
			continue
		}
		entry := ensureTransitionSnapshotEntry(snapshot, invite.Email)
		entry.Statuses = append(entry.Statuses, "invited")
		entry.MemberRole = defaultIfEmpty(invite.Role, entry.MemberRole)
		if invite.CreatedAt != nil {
			entry.InvitedAt = invite.CreatedAt
		}
		entry.Source = "sync"
	}
}

func sortedTransitionSnapshotEmails(snapshot map[string]*transitionMembershipSnapshot) []string {
	emails := make([]string, 0, len(snapshot))
	for email := range snapshot {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	return emails
}

func resolveTransitionMembershipStatus(entry *transitionMembershipSnapshot) string {
	bestStatus := "failed"
	bestRank := -1
	for _, status := range entry.Statuses {
		rank := transitionMembershipStatusRank(normalizeText(status))
		if rank > bestRank {
			bestRank = rank
			bestStatus = normalizeText(status)
		}
	}
	if bestRank >= 0 {
		return bestStatus
	}
	if resolved, ok := transitionAbsentMembershipStatuses[normalizeText(entry.ExistingStatus)]; ok {
		return resolved
	}
	if normalizeText(entry.ExistingStatus) != "" {
		return normalizeText(entry.ExistingStatus)
	}
	return "failed"
}

func transitionMembershipStatusRank(status string) int {
	switch status {
	case "failed":
		return 0
	case "removed":
		return 1
	case "revoked":
		return 2
	case "invited":
		return 3
	case "already_member":
		return 4
	case "joined":
		return 5
	default:
		return -1
	}
}

func calculateTransitionSeatsAvailable(currentMembers int, maxMembers *int) int {
	resolvedMaxMembers := defaultMaxMembers
	if maxMembers != nil && *maxMembers > 0 {
		resolvedMaxMembers = *maxMembers
	}
	if resolvedMaxMembers-currentMembers < 0 {
		return 0
	}
	return resolvedMaxMembers - currentMembers
}

func countTransitionActiveMembers(memberships []TeamMembershipRecord) int {
	active := map[string]struct{}{}
	for _, membership := range memberships {
		if _, ok := activeMembershipStatuses[normalizeText(membership.MembershipStatus)]; !ok {
			continue
		}
		email := normalizeEmail(membership.MemberEmail)
		if email == "" {
			continue
		}
		active[email] = struct{}{}
	}
	return len(active)
}

func allTransitionInviteResultsSuccessful(results []map[string]any) bool {
	for _, result := range results {
		success, _ := result["success"].(bool)
		if !success {
			return false
		}
	}
	return true
}

func isTransitionFullTeamError(message string) bool {
	normalized := normalizeText(message)
	for _, token := range transitionFullTeamTokens {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func isTransitionAlreadyMemberError(message string) bool {
	normalized := normalizeText(message)
	for _, token := range transitionAlreadyMemberTokens {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func markTransitionSyncFailure(ctx context.Context, repository Repository, teamRecord TeamRecord, cause error) {
	teamRecord.SyncStatus = "failed"
	teamRecord.SyncError = cause.Error()
	now := time.Now().UTC()
	teamRecord.LastSyncAt = &now
	_, _ = repository.SaveTeam(ctx, teamRecord)
}

func parseTransitionTime(value string) *time.Time {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.ReplaceAll(normalized, "Z", "+00:00"))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func transitionStringValue(value any) string {
	switch resolved := value.(type) {
	case string:
		return resolved
	case json.Number:
		return resolved.String()
	default:
		return ""
	}
}

func transitionIntValue(value any) (int, bool) {
	switch resolved := value.(type) {
	case int:
		return resolved, true
	case int64:
		return int(resolved), true
	case float64:
		return int(resolved), true
	case json.Number:
		parsed, err := resolved.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func collectedTransitionItems(pages []map[string]any) int {
	total := 0
	for _, payload := range pages {
		items, _ := payload["items"].([]any)
		total += len(items)
	}
	return total
}
