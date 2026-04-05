package team

import (
	"context"
	"errors"
	"strings"
	"time"
)

const defaultMaxMembers = 6

var activeMembershipStatuses = map[string]struct{}{
	"joined":         {},
	"already_member": {},
	"invited":        {},
}

var finishedTaskStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"cancelled": {},
}

type Service struct {
	repository Repository
	gateway    MembershipGateway
}

func NewService(repository Repository, gateway MembershipGateway) *Service {
	return &Service{
		repository: repository,
		gateway:    gateway,
	}
}

func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) (ListTeamsResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	perPage := req.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	records, total, err := s.repository.ListTeams(ctx, ListTeamsRequest{
		Page:           page,
		PerPage:        perPage,
		Status:         strings.TrimSpace(req.Status),
		OwnerAccountID: req.OwnerAccountID,
		Search:         strings.TrimSpace(req.Search),
	})
	if err != nil {
		return ListTeamsResponse{}, err
	}

	ownerIDs := make([]int64, 0, len(records))
	seenOwners := map[int64]struct{}{}
	for _, record := range records {
		if _, ok := seenOwners[record.OwnerAccountID]; ok {
			continue
		}
		seenOwners[record.OwnerAccountID] = struct{}{}
		ownerIDs = append(ownerIDs, record.OwnerAccountID)
	}
	owners, err := s.repository.ListAccountsByIDs(ctx, ownerIDs)
	if err != nil {
		return ListTeamsResponse{}, err
	}

	items := make([]TeamListItem, 0, len(records))
	for _, record := range records {
		owner := owners[record.OwnerAccountID]
		items = append(items, teamRecordToListItem(record, owner))
	}

	return ListTeamsResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

func (s *Service) GetTeamDetail(ctx context.Context, teamID int64) (TeamDetailResponse, error) {
	teamRecord, err := s.repository.GetTeam(ctx, teamID)
	if err != nil {
		return TeamDetailResponse{}, err
	}
	owner, err := s.repository.GetAccount(ctx, teamRecord.OwnerAccountID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return TeamDetailResponse{}, err
	}
	memberships, err := s.repository.ListMembershipsByTeam(ctx, teamID)
	if err != nil {
		return TeamDetailResponse{}, err
	}
	tasks, err := s.repository.ListTasks(ctx, ListTasksRequest{TeamID: &teamID})
	if err != nil {
		return TeamDetailResponse{}, err
	}

	detail := TeamDetailResponse{
		TeamListItem:  teamRecordToListItem(teamRecord, owner),
		LastSyncError: teamRecord.SyncError,
	}
	for _, membership := range memberships {
		status := normalizeText(membership.MembershipStatus)
		if _, ok := activeMembershipStatuses[status]; ok {
			detail.ActiveMemberCount++
		}
		switch status {
		case "joined":
			detail.JoinedCount++
		case "invited":
			detail.InvitedCount++
		}
		if membership.LocalAccountID != nil {
			detail.LocalMemberCount++
		} else {
			detail.ExternalMemberCount++
		}
	}
	for _, task := range tasks {
		if _, ok := finishedTaskStatuses[normalizeText(task.Status)]; !ok {
			detail.ActiveTaskCount++
		}
	}

	return detail, nil
}

func (s *Service) ListMemberships(ctx context.Context, req ListMembershipsRequest) (ListMembershipsResponse, error) {
	if _, err := s.repository.GetTeam(ctx, req.TeamID); err != nil {
		return ListMembershipsResponse{}, err
	}
	records, err := s.repository.ListMembershipsByTeam(ctx, req.TeamID)
	if err != nil {
		return ListMembershipsResponse{}, err
	}

	localIDs := make([]int64, 0, len(records))
	seenLocalIDs := map[int64]struct{}{}
	for _, record := range records {
		if record.LocalAccountID == nil {
			continue
		}
		if _, ok := seenLocalIDs[*record.LocalAccountID]; ok {
			continue
		}
		seenLocalIDs[*record.LocalAccountID] = struct{}{}
		localIDs = append(localIDs, *record.LocalAccountID)
	}
	localAccounts, err := s.repository.ListAccountsByIDs(ctx, localIDs)
	if err != nil {
		return ListMembershipsResponse{}, err
	}

	filtered := make([]TeamMembershipItem, 0, len(records))
	for _, record := range records {
		if !membershipMatchesStatus(record, req.Status) {
			continue
		}
		if !membershipMatchesBinding(record, req.Binding) {
			continue
		}
		if !membershipMatchesSearch(record, req.Search) {
			continue
		}

		item := TeamMembershipItem{
			ID:                 record.ID,
			MemberEmail:        record.MemberEmail,
			LocalAccountID:     record.LocalAccountID,
			MemberRole:         record.MemberRole,
			MembershipStatus:   record.MembershipStatus,
			UpstreamUserID:     record.UpstreamUserID,
			InvitedAt:          record.InvitedAt,
			JoinedAt:           record.JoinedAt,
			LastSeenAt:         record.LastSeenAt,
			LocalAccountStatus: "",
		}
		if record.LocalAccountID != nil {
			item.LocalAccountStatus = localAccounts[*record.LocalAccountID].Status
		}
		filtered = append(filtered, item)
	}

	return ListMembershipsResponse{
		Items: filtered,
		Total: len(filtered),
	}, nil
}

func (s *Service) ListTasks(ctx context.Context, req ListTasksRequest) (ListTasksResponse, error) {
	records, err := s.repository.ListTasks(ctx, req)
	if err != nil {
		return ListTasksResponse{}, err
	}

	items := make([]TeamTaskListItem, 0, len(records))
	for _, record := range records {
		createdAt := record.CreatedAt
		items = append(items, TeamTaskListItem{
			TaskUUID:       record.TaskUUID,
			TaskType:       record.TaskType,
			Status:         record.Status,
			TeamID:         record.TeamID,
			OwnerAccountID: record.OwnerAccountID,
			CreatedAt:      &createdAt,
			StartedAt:      record.StartedAt,
			CompletedAt:    record.CompletedAt,
		})
	}

	return ListTasksResponse{
		Items: items,
		Total: len(items),
	}, nil
}

func (s *Service) GetTaskDetail(ctx context.Context, taskUUID string) (TeamTaskDetailResponse, error) {
	record, err := s.repository.GetTaskByUUID(ctx, taskUUID)
	if err != nil {
		return TeamTaskDetailResponse{}, err
	}
	items, err := s.repository.ListTaskItems(ctx, record.ID)
	if err != nil {
		return TeamTaskDetailResponse{}, err
	}

	responseItems := make([]TeamTaskDetailItem, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, TeamTaskDetailItem{
			TargetEmail:          item.TargetEmail,
			ItemStatus:           item.ItemStatus,
			RelationStatusBefore: lookupString(item.Before, "membership_status"),
			RelationStatusAfter:  lookupString(item.After, "membership_status"),
			Message:              item.Message,
			ErrorMessage:         item.ErrorMessage,
		})
	}

	logs := splitLogs(record.Logs)
	summary := record.ResultPayload
	if summary == nil {
		summary = map[string]any{}
	}

	createdAt := record.CreatedAt
	return TeamTaskDetailResponse{
		TaskUUID:       record.TaskUUID,
		TaskType:       record.TaskType,
		Status:         record.Status,
		TeamID:         record.TeamID,
		OwnerAccountID: record.OwnerAccountID,
		CreatedAt:      &createdAt,
		StartedAt:      record.StartedAt,
		CompletedAt:    record.CompletedAt,
		Logs:           logs,
		GuardLogs:      append([]string(nil), logs...),
		Summary:        summary,
		Items:          responseItems,
	}, nil
}

func (s *Service) ApplyMembershipAction(ctx context.Context, req ApplyMembershipActionRequest) (MembershipActionResult, error) {
	membership, err := s.repository.GetMembership(ctx, req.MembershipID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return MembershipActionResult{
				Success:      false,
				Message:      "membership not found",
				MembershipID: req.MembershipID,
				ErrorCode:    404,
			}, nil
		}
		return MembershipActionResult{}, err
	}

	action := normalizeText(req.Action)
	currentStatus := normalizeText(membership.MembershipStatus)

	switch action {
	case "bind-local-account":
		return s.applyBindLocalAccount(ctx, membership, req.AccountID)
	case "revoke":
		return s.applyRevoke(ctx, membership, currentStatus)
	case "remove":
		return s.applyRemove(ctx, membership, currentStatus)
	default:
		return MembershipActionResult{
			Success:      false,
			Message:      "unsupported membership action: " + strings.TrimSpace(req.Action),
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}
}

func (s *Service) applyBindLocalAccount(ctx context.Context, membership TeamMembershipRecord, accountID *int64) (MembershipActionResult, error) {
	if accountID == nil {
		return MembershipActionResult{
			Success:      false,
			Message:      "account_id is required for bind-local-account",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}
	account, err := s.repository.GetAccount(ctx, *accountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return MembershipActionResult{
				Success:      false,
				Message:      "account not found",
				TeamID:       ptrInt64(membership.TeamID),
				MembershipID: membership.ID,
				NextStatus:   membership.MembershipStatus,
				ErrorCode:    404,
			}, nil
		}
		return MembershipActionResult{}, err
	}

	if normalizeEmail(membership.MemberEmail) != normalizeEmail(account.Email) {
		return MembershipActionResult{
			Success:      false,
			Message:      "cross-email binding requires explicit confirmation",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}
	if membership.LocalAccountID != nil && *membership.LocalAccountID != account.ID {
		return MembershipActionResult{
			Success:      false,
			Message:      "membership already bound to another local account",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    409,
		}, nil
	}

	membership.LocalAccountID = ptrInt64(account.ID)
	membership.Source = "manual_bind"
	if _, err := s.repository.SaveMembership(ctx, membership); err != nil {
		return MembershipActionResult{}, err
	}
	if err := s.refreshTeamAggregate(ctx, membership.TeamID); err != nil {
		return MembershipActionResult{}, err
	}

	return MembershipActionResult{
		Success:         true,
		Message:         "local account bound",
		TeamID:          ptrInt64(membership.TeamID),
		MembershipID:    membership.ID,
		NextStatus:      membership.MembershipStatus,
		RefreshRequired: true,
		AccountID:       ptrInt64(account.ID),
	}, nil
}

func (s *Service) applyRevoke(ctx context.Context, membership TeamMembershipRecord, currentStatus string) (MembershipActionResult, error) {
	if currentStatus != "invited" {
		return MembershipActionResult{
			Success:      false,
			Message:      "revoke is only allowed for invited memberships",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}

	teamRecord, ownerRecord, failureResult, err := s.loadMembershipOwnerContext(ctx, membership)
	if err != nil || failureResult != nil {
		if failureResult != nil {
			return *failureResult, nil
		}
		return MembershipActionResult{}, err
	}

	if s.gateway == nil {
		return MembershipActionResult{
			Success:      false,
			Message:      "membership gateway not configured",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    502,
		}, nil
	}
	if err := s.gateway.RevokeInvite(ctx, MembershipGatewayRevokeInviteParams{
		TeamUpstreamAccountID: teamRecord.UpstreamAccountID,
		OwnerAccessToken:      ownerRecord.AccessToken,
		MemberEmail:           membership.MemberEmail,
	}); err != nil {
		membership.SyncError = err.Error()
		if _, saveErr := s.repository.SaveMembership(ctx, membership); saveErr != nil {
			return MembershipActionResult{}, saveErr
		}
		return MembershipActionResult{
			Success:      false,
			Message:      err.Error(),
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    502,
		}, nil
	}

	now := time.Now().UTC()
	membership.MembershipStatus = "revoked"
	membership.RemovedAt = &now
	membership.SyncError = ""
	if _, err := s.repository.SaveMembership(ctx, membership); err != nil {
		return MembershipActionResult{}, err
	}
	if err := s.refreshTeamAggregate(ctx, membership.TeamID); err != nil {
		return MembershipActionResult{}, err
	}

	return MembershipActionResult{
		Success:         true,
		Message:         "membership invite revoked",
		TeamID:          ptrInt64(membership.TeamID),
		MembershipID:    membership.ID,
		NextStatus:      "revoked",
		RefreshRequired: true,
	}, nil
}

func (s *Service) applyRemove(ctx context.Context, membership TeamMembershipRecord, currentStatus string) (MembershipActionResult, error) {
	if currentStatus != "joined" && currentStatus != "already_member" {
		return MembershipActionResult{
			Success:      false,
			Message:      "remove is only allowed for joined/already_member memberships",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}
	if normalizeText(membership.UpstreamUserID) == "" {
		return MembershipActionResult{
			Success:      false,
			Message:      "upstream_user_id is required for remove",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}, nil
	}

	teamRecord, ownerRecord, failureResult, err := s.loadMembershipOwnerContext(ctx, membership)
	if err != nil || failureResult != nil {
		if failureResult != nil {
			return *failureResult, nil
		}
		return MembershipActionResult{}, err
	}

	if s.gateway == nil {
		return MembershipActionResult{
			Success:      false,
			Message:      "membership gateway not configured",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    502,
		}, nil
	}
	if err := s.gateway.RemoveMember(ctx, MembershipGatewayRemoveMemberParams{
		TeamUpstreamAccountID: teamRecord.UpstreamAccountID,
		OwnerAccessToken:      ownerRecord.AccessToken,
		UpstreamUserID:        membership.UpstreamUserID,
	}); err != nil {
		membership.SyncError = err.Error()
		if _, saveErr := s.repository.SaveMembership(ctx, membership); saveErr != nil {
			return MembershipActionResult{}, saveErr
		}
		return MembershipActionResult{
			Success:      false,
			Message:      err.Error(),
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    502,
		}, nil
	}

	now := time.Now().UTC()
	membership.MembershipStatus = "removed"
	membership.RemovedAt = &now
	membership.SyncError = ""
	if _, err := s.repository.SaveMembership(ctx, membership); err != nil {
		return MembershipActionResult{}, err
	}
	if err := s.refreshTeamAggregate(ctx, membership.TeamID); err != nil {
		return MembershipActionResult{}, err
	}

	return MembershipActionResult{
		Success:         true,
		Message:         "membership removed",
		TeamID:          ptrInt64(membership.TeamID),
		MembershipID:    membership.ID,
		NextStatus:      "removed",
		RefreshRequired: true,
	}, nil
}

func (s *Service) loadMembershipOwnerContext(ctx context.Context, membership TeamMembershipRecord) (TeamRecord, AccountRecord, *MembershipActionResult, error) {
	teamRecord, err := s.repository.GetTeam(ctx, membership.TeamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			result := MembershipActionResult{
				Success:      false,
				Message:      "team not found",
				TeamID:       ptrInt64(membership.TeamID),
				MembershipID: membership.ID,
				NextStatus:   membership.MembershipStatus,
				ErrorCode:    404,
			}
			return TeamRecord{}, AccountRecord{}, &result, nil
		}
		return TeamRecord{}, AccountRecord{}, nil, err
	}
	ownerRecord, err := s.repository.GetAccount(ctx, teamRecord.OwnerAccountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			result := MembershipActionResult{
				Success:      false,
				Message:      "owner account not found",
				TeamID:       ptrInt64(membership.TeamID),
				MembershipID: membership.ID,
				NextStatus:   membership.MembershipStatus,
				ErrorCode:    404,
			}
			return TeamRecord{}, AccountRecord{}, &result, nil
		}
		return TeamRecord{}, AccountRecord{}, nil, err
	}
	if normalizeText(ownerRecord.AccessToken) == "" {
		result := MembershipActionResult{
			Success:      false,
			Message:      "owner access token missing",
			TeamID:       ptrInt64(membership.TeamID),
			MembershipID: membership.ID,
			NextStatus:   membership.MembershipStatus,
			ErrorCode:    400,
		}
		return TeamRecord{}, AccountRecord{}, &result, nil
	}
	return teamRecord, ownerRecord, nil, nil
}

func (s *Service) refreshTeamAggregate(ctx context.Context, teamID int64) error {
	teamRecord, err := s.repository.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	memberships, err := s.repository.ListMembershipsByTeam(ctx, teamID)
	if err != nil {
		return err
	}

	activeCount := 0
	for _, membership := range memberships {
		if _, ok := activeMembershipStatuses[normalizeText(membership.MembershipStatus)]; ok {
			activeCount++
		}
	}
	teamRecord.CurrentMembers = activeCount
	maxMembers := defaultMaxMembers
	if teamRecord.MaxMembers != nil {
		maxMembers = *teamRecord.MaxMembers
	}
	seatsAvailable := maxMembers - activeCount
	if seatsAvailable < 0 {
		seatsAvailable = 0
	}
	teamRecord.SeatsAvailable = ptrInt(seatsAvailable)
	_, err = s.repository.SaveTeam(ctx, teamRecord)
	return err
}

func teamRecordToListItem(teamRecord TeamRecord, owner AccountRecord) TeamListItem {
	return TeamListItem{
		ID:                  teamRecord.ID,
		OwnerAccountID:      teamRecord.OwnerAccountID,
		OwnerEmail:          owner.Email,
		UpstreamAccountID:   teamRecord.UpstreamAccountID,
		TeamName:            teamRecord.TeamName,
		AccountRoleSnapshot: teamRecord.AccountRoleSnapshot,
		Status:              teamRecord.Status,
		CurrentMembers:      teamRecord.CurrentMembers,
		MaxMembers:          teamRecord.MaxMembers,
		SeatsAvailable:      teamRecord.SeatsAvailable,
		ExpiresAt:           teamRecord.ExpiresAt,
		LastSyncAt:          teamRecord.LastSyncAt,
		SyncStatus:          teamRecord.SyncStatus,
	}
}

func membershipMatchesStatus(record TeamMembershipRecord, status string) bool {
	switch normalizeText(status) {
	case "", "all":
		return true
	case "active", "joined":
		_, ok := activeMembershipStatuses[normalizeText(record.MembershipStatus)]
		return ok
	case "invited":
		return normalizeText(record.MembershipStatus) == "invited"
	default:
		return normalizeText(record.MembershipStatus) == normalizeText(status)
	}
}

func membershipMatchesBinding(record TeamMembershipRecord, binding string) bool {
	switch normalizeText(binding) {
	case "", "all":
		return true
	case "local":
		return record.LocalAccountID != nil
	case "external":
		return record.LocalAccountID == nil
	default:
		return false
	}
}

func membershipMatchesSearch(record TeamMembershipRecord, search string) bool {
	needle := normalizeText(search)
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(record.MemberEmail), needle)
}

func splitLogs(raw string) []string {
	lines := strings.Split(raw, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func lookupString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func normalizeText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEmail(value string) string {
	return normalizeText(value)
}

func ptrInt(value int) *int {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}
