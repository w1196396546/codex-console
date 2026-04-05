package team

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	accounts    map[int64]AccountRecord
	teams       map[int64]TeamRecord
	memberships map[int64]TeamMembershipRecord
	tasks       map[string]TeamTaskRecord
	taskItems   map[int64][]TeamTaskItemRecord
}

type fakeMembershipGateway struct {
	revoked []string
	removed []string
}

func (r *fakeRepository) ListTeams(_ context.Context, req ListTeamsRequest) ([]TeamRecord, int, error) {
	records := make([]TeamRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if strings.TrimSpace(req.Status) != "" && record.Status != req.Status {
			continue
		}
		if req.OwnerAccountID > 0 && record.OwnerAccountID != req.OwnerAccountID {
			continue
		}
		if search := strings.ToLower(strings.TrimSpace(req.Search)); search != "" {
			if !strings.Contains(strings.ToLower(record.TeamName), search) && !strings.Contains(strings.ToLower(record.UpstreamAccountID), search) {
				continue
			}
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, len(records), nil
}

func (r *fakeRepository) GetTeam(_ context.Context, teamID int64) (TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok {
		return TeamRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) GetAccount(_ context.Context, accountID int64) (AccountRecord, error) {
	record, ok := r.accounts[accountID]
	if !ok {
		return AccountRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) ListAccountsByIDs(_ context.Context, accountIDs []int64) (map[int64]AccountRecord, error) {
	result := make(map[int64]AccountRecord, len(accountIDs))
	for _, accountID := range accountIDs {
		if record, ok := r.accounts[accountID]; ok {
			result[accountID] = record
		}
	}
	return result, nil
}

func (r *fakeRepository) ListMembershipsByTeam(_ context.Context, teamID int64) ([]TeamMembershipRecord, error) {
	records := make([]TeamMembershipRecord, 0)
	for _, record := range r.memberships {
		if record.TeamID == teamID {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func (r *fakeRepository) GetMembership(_ context.Context, membershipID int64) (TeamMembershipRecord, error) {
	record, ok := r.memberships[membershipID]
	if !ok {
		return TeamMembershipRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) SaveMembership(_ context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error) {
	if _, ok := r.memberships[membership.ID]; !ok {
		return TeamMembershipRecord{}, ErrNotFound
	}
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *fakeRepository) SaveTeam(_ context.Context, team TeamRecord) (TeamRecord, error) {
	if _, ok := r.teams[team.ID]; !ok {
		return TeamRecord{}, ErrNotFound
	}
	r.teams[team.ID] = team
	return team, nil
}

func (r *fakeRepository) ListTasks(_ context.Context, req ListTasksRequest) ([]TeamTaskRecord, error) {
	records := make([]TeamTaskRecord, 0)
	for _, record := range r.tasks {
		if req.TeamID != nil {
			if record.TeamID == nil || *record.TeamID != *req.TeamID {
				continue
			}
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

func (r *fakeRepository) GetTaskByUUID(_ context.Context, taskUUID string) (TeamTaskRecord, error) {
	record, ok := r.tasks[taskUUID]
	if !ok {
		return TeamTaskRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) ListTaskItems(_ context.Context, taskID int64) ([]TeamTaskItemRecord, error) {
	return append([]TeamTaskItemRecord(nil), r.taskItems[taskID]...), nil
}

func (g *fakeMembershipGateway) RevokeInvite(_ context.Context, params MembershipGatewayRevokeInviteParams) error {
	g.revoked = append(g.revoked, params.MemberEmail)
	return nil
}

func (g *fakeMembershipGateway) RemoveMember(_ context.Context, params MembershipGatewayRemoveMemberParams) error {
	g.removed = append(g.removed, params.UpstreamUserID)
	return nil
}

func TestServiceReadModelsPreserveCompatibilityShape(t *testing.T) {
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			11: {ID: 11, Email: "member@example.com", Status: "paused"},
		},
		teams: map[int64]TeamRecord{
			101: {
				ID:                  101,
				OwnerAccountID:      7,
				UpstreamTeamID:      "up-team-1",
				UpstreamAccountID:   "acct_123",
				TeamName:            "Alpha Team",
				PlanType:            "team",
				SubscriptionPlan:    "business",
				AccountRoleSnapshot: "owner",
				Status:              "active",
				CurrentMembers:      2,
				MaxMembers:          intPtr(5),
				SeatsAvailable:      intPtr(3),
				LastSyncAt:          &now,
				SyncStatus:          "success",
				SyncError:           "",
				UpdatedAt:           now,
			},
		},
		memberships: map[int64]TeamMembershipRecord{
			201: {
				ID:               201,
				TeamID:           101,
				LocalAccountID:   int64Ptr(11),
				MemberEmail:      "member@example.com",
				MemberRole:       "member",
				MembershipStatus: "joined",
				JoinedAt:         &now,
			},
			202: {
				ID:               202,
				TeamID:           101,
				MemberEmail:      "invitee@example.com",
				MemberRole:       "member",
				MembershipStatus: "invited",
				InvitedAt:        &now,
			},
			203: {
				ID:               203,
				TeamID:           101,
				MemberEmail:      "removed@example.com",
				MemberRole:       "member",
				MembershipStatus: "removed",
				RemovedAt:        &now,
			},
		},
		tasks: map[string]TeamTaskRecord{
			"task-1": {
				ID:             301,
				TaskUUID:       "task-1",
				TaskType:       "sync_team",
				Status:         "completed",
				TeamID:         int64Ptr(101),
				OwnerAccountID: int64Ptr(7),
				ScopeType:      "team",
				ScopeID:        "101",
				Logs:           "queued\nsync ok",
				ResultPayload:  map[string]any{"processed_count": 1},
				CreatedAt:      now.Add(-2 * time.Hour),
				StartedAt:      timePtr(now.Add(-90 * time.Minute)),
				CompletedAt:    timePtr(now.Add(-80 * time.Minute)),
			},
		},
		taskItems: map[int64][]TeamTaskItemRecord{
			301: {
				{
					TaskID:       301,
					TargetEmail:  "member@example.com",
					ItemStatus:   "completed",
					Before:       map[string]any{"membership_status": "invited"},
					After:        map[string]any{"membership_status": "joined"},
					Message:      "invited",
					ErrorMessage: "",
				},
			},
		},
	}

	svc := NewService(repo, &fakeMembershipGateway{})

	listResp, err := svc.ListTeams(context.Background(), ListTeamsRequest{
		Page:           1,
		PerPage:        20,
		Status:         "active",
		OwnerAccountID: 7,
		Search:         "Alpha",
	})
	if err != nil {
		t.Fatalf("ListTeams returned error: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Items) != 1 {
		t.Fatalf("expected one team item, got total=%d len=%d", listResp.Total, len(listResp.Items))
	}
	if listResp.Items[0].OwnerEmail != "owner@example.com" {
		t.Fatalf("expected owner_email to be populated, got %#v", listResp.Items[0].OwnerEmail)
	}
	if listResp.Items[0].SyncStatus != "success" {
		t.Fatalf("expected sync_status success, got %#v", listResp.Items[0].SyncStatus)
	}

	detailResp, err := svc.GetTeamDetail(context.Background(), 101)
	if err != nil {
		t.Fatalf("GetTeamDetail returned error: %v", err)
	}
	if detailResp.ActiveMemberCount != 2 {
		t.Fatalf("expected active_member_count=2, got %d", detailResp.ActiveMemberCount)
	}
	if detailResp.JoinedCount != 1 || detailResp.InvitedCount != 1 {
		t.Fatalf("expected joined=1 invited=1, got joined=%d invited=%d", detailResp.JoinedCount, detailResp.InvitedCount)
	}
	if detailResp.LocalMemberCount != 1 || detailResp.ExternalMemberCount != 2 {
		t.Fatalf("expected local=1 external=2, got local=%d external=%d", detailResp.LocalMemberCount, detailResp.ExternalMemberCount)
	}

	membershipsResp, err := svc.ListMemberships(context.Background(), ListMembershipsRequest{
		TeamID:  101,
		Status:  "active",
		Binding: "all",
		Search:  "@example.com",
	})
	if err != nil {
		t.Fatalf("ListMemberships returned error: %v", err)
	}
	if membershipsResp.Total != 2 || len(membershipsResp.Items) != 2 {
		t.Fatalf("expected two active memberships, got total=%d len=%d", membershipsResp.Total, len(membershipsResp.Items))
	}
	if membershipsResp.Items[0].LocalAccountStatus == "" {
		t.Fatalf("expected local account status to be populated for local membership")
	}

	tasksResp, err := svc.ListTasks(context.Background(), ListTasksRequest{TeamID: int64Ptr(101)})
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if tasksResp.Total != 1 || len(tasksResp.Items) != 1 {
		t.Fatalf("expected one task, got total=%d len=%d", tasksResp.Total, len(tasksResp.Items))
	}
	if tasksResp.Items[0].TaskUUID != "task-1" {
		t.Fatalf("expected task_uuid=task-1, got %#v", tasksResp.Items[0].TaskUUID)
	}

	taskDetail, err := svc.GetTaskDetail(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetTaskDetail returned error: %v", err)
	}
	if len(taskDetail.Logs) != 2 {
		t.Fatalf("expected two merged logs, got %#v", taskDetail.Logs)
	}
	if len(taskDetail.Items) != 1 {
		t.Fatalf("expected one task item, got %#v", taskDetail.Items)
	}
	if taskDetail.Summary["processed_count"] != 1 {
		t.Fatalf("expected processed_count summary, got %#v", taskDetail.Summary)
	}
}

func TestMembershipActionsPreserveCompatibilitySemantics(t *testing.T) {
	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		accounts: map[int64]AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			11: {ID: 11, Email: "member@example.com", Status: "active"},
		},
		teams: map[int64]TeamRecord{
			101: {
				ID:                101,
				OwnerAccountID:    7,
				UpstreamAccountID: "acct_123",
				TeamName:          "Alpha Team",
				Status:            "active",
				CurrentMembers:    2,
				MaxMembers:        intPtr(5),
				SeatsAvailable:    intPtr(3),
				SyncStatus:        "success",
			},
		},
		memberships: map[int64]TeamMembershipRecord{
			201: {
				ID:               201,
				TeamID:           101,
				MemberEmail:      "invitee@example.com",
				MembershipStatus: "invited",
				InvitedAt:        &now,
				Source:           "invite",
			},
			202: {
				ID:               202,
				TeamID:           101,
				MemberEmail:      "member@example.com",
				UpstreamUserID:   "user_202",
				MembershipStatus: "joined",
				JoinedAt:         &now,
				Source:           "sync",
			},
			203: {
				ID:               203,
				TeamID:           101,
				MemberEmail:      "member@example.com",
				MembershipStatus: "already_member",
				Source:           "sync",
			},
		},
	}
	gateway := &fakeMembershipGateway{}
	svc := NewService(repo, gateway)

	revokeResult, err := svc.ApplyMembershipAction(context.Background(), ApplyMembershipActionRequest{
		MembershipID: 201,
		Action:       "revoke",
	})
	if err != nil {
		t.Fatalf("ApplyMembershipAction revoke returned error: %v", err)
	}
	if !revokeResult.Success || revokeResult.NextStatus != "revoked" {
		t.Fatalf("expected revoke success -> revoked, got %#v", revokeResult)
	}
	if repo.memberships[201].RemovedAt == nil {
		t.Fatal("expected revoke to set removed_at")
	}
	if len(gateway.revoked) != 1 || gateway.revoked[0] != "invitee@example.com" {
		t.Fatalf("expected revoke gateway call, got %#v", gateway.revoked)
	}

	removeResult, err := svc.ApplyMembershipAction(context.Background(), ApplyMembershipActionRequest{
		MembershipID: 202,
		Action:       "remove",
	})
	if err != nil {
		t.Fatalf("ApplyMembershipAction remove returned error: %v", err)
	}
	if !removeResult.Success || removeResult.NextStatus != "removed" {
		t.Fatalf("expected remove success -> removed, got %#v", removeResult)
	}
	if len(gateway.removed) != 1 || gateway.removed[0] != "user_202" {
		t.Fatalf("expected remove gateway call, got %#v", gateway.removed)
	}

	bindResult, err := svc.ApplyMembershipAction(context.Background(), ApplyMembershipActionRequest{
		MembershipID: 203,
		Action:       "bind-local-account",
		AccountID:    int64Ptr(11),
	})
	if err != nil {
		t.Fatalf("ApplyMembershipAction bind returned error: %v", err)
	}
	if !bindResult.Success {
		t.Fatalf("expected bind to succeed, got %#v", bindResult)
	}
	if repo.memberships[203].LocalAccountID == nil || *repo.memberships[203].LocalAccountID != 11 {
		t.Fatalf("expected membership to bind local account 11, got %#v", repo.memberships[203].LocalAccountID)
	}
	if repo.memberships[203].Source != "manual_bind" {
		t.Fatalf("expected manual_bind source, got %#v", repo.memberships[203].Source)
	}
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
