package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	teamdomain "github.com/dou-jiang/codex-console/backend-go/internal/team"
	"github.com/go-chi/chi/v5"
)

type fakeRepository struct {
	accounts    map[int64]teamdomain.AccountRecord
	teams       map[int64]teamdomain.TeamRecord
	memberships map[int64]teamdomain.TeamMembershipRecord
	tasks       map[string]teamdomain.TeamTaskRecord
	taskItems   map[int64][]teamdomain.TeamTaskItemRecord
	nextTaskID  int64
	nextItemID  int64
}

type fakeMembershipGateway struct{}

func (g *fakeMembershipGateway) RevokeInvite(_ context.Context, _ teamdomain.MembershipGatewayRevokeInviteParams) error {
	return nil
}

func (g *fakeMembershipGateway) RemoveMember(_ context.Context, _ teamdomain.MembershipGatewayRemoveMemberParams) error {
	return nil
}

type fakeExecutor struct {
	results map[string]teamdomain.TaskExecutionResult
}

func (e *fakeExecutor) Execute(_ context.Context, task teamdomain.TaskExecutionRequest) (teamdomain.TaskExecutionResult, error) {
	result, ok := e.results[task.TaskType]
	if !ok {
		return teamdomain.TaskExecutionResult{}, errors.New("missing fake executor result")
	}
	return result, nil
}

func (r *fakeRepository) ListTeams(_ context.Context, req teamdomain.ListTeamsRequest) ([]teamdomain.TeamRecord, int, error) {
	records := make([]teamdomain.TeamRecord, 0, len(r.teams))
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

func (r *fakeRepository) GetTeam(_ context.Context, teamID int64) (teamdomain.TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok {
		return teamdomain.TeamRecord{}, teamdomain.ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) GetAccount(_ context.Context, accountID int64) (teamdomain.AccountRecord, error) {
	record, ok := r.accounts[accountID]
	if !ok {
		return teamdomain.AccountRecord{}, teamdomain.ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) ListAccountsByIDs(_ context.Context, accountIDs []int64) (map[int64]teamdomain.AccountRecord, error) {
	result := make(map[int64]teamdomain.AccountRecord, len(accountIDs))
	for _, accountID := range accountIDs {
		if record, ok := r.accounts[accountID]; ok {
			result[accountID] = record
		}
	}
	return result, nil
}

func (r *fakeRepository) ListMembershipsByTeam(_ context.Context, teamID int64) ([]teamdomain.TeamMembershipRecord, error) {
	records := make([]teamdomain.TeamMembershipRecord, 0)
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

func (r *fakeRepository) GetMembership(_ context.Context, membershipID int64) (teamdomain.TeamMembershipRecord, error) {
	record, ok := r.memberships[membershipID]
	if !ok {
		return teamdomain.TeamMembershipRecord{}, teamdomain.ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) SaveMembership(_ context.Context, membership teamdomain.TeamMembershipRecord) (teamdomain.TeamMembershipRecord, error) {
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *fakeRepository) SaveTeam(_ context.Context, team teamdomain.TeamRecord) (teamdomain.TeamRecord, error) {
	r.teams[team.ID] = team
	return team, nil
}

func (r *fakeRepository) ListTasks(_ context.Context, req teamdomain.ListTasksRequest) ([]teamdomain.TeamTaskRecord, error) {
	records := make([]teamdomain.TeamTaskRecord, 0)
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

func (r *fakeRepository) GetTaskByUUID(_ context.Context, taskUUID string) (teamdomain.TeamTaskRecord, error) {
	record, ok := r.tasks[taskUUID]
	if !ok {
		return teamdomain.TeamTaskRecord{}, teamdomain.ErrNotFound
	}
	return record, nil
}

func (r *fakeRepository) ListTaskItems(_ context.Context, taskID int64) ([]teamdomain.TeamTaskItemRecord, error) {
	return append([]teamdomain.TeamTaskItemRecord(nil), r.taskItems[taskID]...), nil
}

func (r *fakeRepository) CreateTask(_ context.Context, task teamdomain.TeamTaskRecord) (teamdomain.TeamTaskRecord, error) {
	scopeKey := ""
	if task.ActiveScopeKey != nil {
		scopeKey = *task.ActiveScopeKey
	}
	for _, existing := range r.tasks {
		if existing.ActiveScopeKey == nil || scopeKey == "" {
			continue
		}
		if *existing.ActiveScopeKey == scopeKey && existing.Status != jobs.StatusCompleted && existing.Status != jobs.StatusFailed && existing.Status != jobs.StatusCancelled {
			return teamdomain.TeamTaskRecord{}, teamdomain.ErrActiveScopeConflict
		}
	}
	r.nextTaskID++
	task.ID = r.nextTaskID
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *fakeRepository) SaveTask(_ context.Context, task teamdomain.TeamTaskRecord) (teamdomain.TeamTaskRecord, error) {
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *fakeRepository) SaveTaskItem(_ context.Context, item teamdomain.TeamTaskItemRecord) (teamdomain.TeamTaskItemRecord, error) {
	r.nextItemID++
	item.ID = r.nextItemID
	r.taskItems[item.TaskID] = append(r.taskItems[item.TaskID], item)
	return item, nil
}

func (r *fakeRepository) FindActiveTask(_ context.Context, scopeType string, scopeID string, taskType string) (teamdomain.TeamTaskRecord, error) {
	for _, task := range r.tasks {
		if task.ScopeType != scopeType || task.ScopeID != scopeID {
			continue
		}
		if taskType != "" && task.TaskType != taskType {
			continue
		}
		if task.Status != jobs.StatusCompleted && task.Status != jobs.StatusFailed && task.Status != jobs.StatusCancelled {
			return task, nil
		}
	}
	return teamdomain.TeamTaskRecord{}, teamdomain.ErrNotFound
}

func newTestHandler(t *testing.T) (*chi.Mux, *fakeRepository, *teamdomain.TaskService) {
	t.Helper()

	now := time.Date(2026, time.April, 5, 15, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		accounts: map[int64]teamdomain.AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			11: {ID: 11, Email: "member@example.com", Status: "active"},
		},
		teams: map[int64]teamdomain.TeamRecord{
			101: {
				ID:                101,
				OwnerAccountID:    7,
				UpstreamAccountID: "acct_101",
				TeamName:          "Alpha Team",
				Status:            "active",
				CurrentMembers:    1,
				MaxMembers:        intPtr(5),
				SeatsAvailable:    intPtr(4),
				SyncStatus:        "success",
				UpdatedAt:         now,
			},
		},
		memberships: map[int64]teamdomain.TeamMembershipRecord{
			201: {
				ID:               201,
				TeamID:           101,
				MemberEmail:      "invitee@example.com",
				MembershipStatus: "invited",
				Source:           "invite",
			},
			202: {
				ID:               202,
				TeamID:           101,
				MemberEmail:      "member@example.com",
				UpstreamUserID:   "user_202",
				MembershipStatus: "joined",
				LocalAccountID:   int64Ptr(11),
				JoinedAt:         &now,
			},
		},
		tasks:     map[string]teamdomain.TeamTaskRecord{},
		taskItems: map[int64][]teamdomain.TeamTaskItemRecord{},
	}

	jobRuntime := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	readService := teamdomain.NewService(repo, &fakeMembershipGateway{})
	taskService := teamdomain.NewTaskService(repo, readService, jobRuntime, &fakeExecutor{
		results: map[string]teamdomain.TaskExecutionResult{
			"invite_emails": {
				Status: "completed",
				Summary: map[string]any{
					"success": true,
				},
				Logs: []string{
					"未触发子号自动刷新 RT",
					"未触发子号自动注册",
					"invite sent",
				},
				Items: []teamdomain.TaskExecutionItem{
					{
						TargetEmail:  "invitee@example.com",
						ItemStatus:   "completed",
						Before:       map[string]any{"membership_status": "pending"},
						After:        map[string]any{"membership_status": "invited"},
						Message:      "invite sent",
						ErrorMessage: "",
					},
				},
			},
		},
	})

	handler := NewHandler(readService, taskService)
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	return router, repo, taskService
}

func TestTeamHandlerAcceptedCompatibilityEndpoints(t *testing.T) {
	router, _, _ := newTestHandler(t)

	discoveryReq := httptest.NewRequest(http.MethodPost, "/api/team/discovery/run", bytes.NewReader([]byte(`{"ids":[7]}`)))
	discoveryReq.Header.Set("Content-Type", "application/json")
	discoveryRec := httptest.NewRecorder()
	router.ServeHTTP(discoveryRec, discoveryReq)
	if discoveryRec.Code != http.StatusAccepted {
		t.Fatalf("expected discovery 202, got %d", discoveryRec.Code)
	}
	var discoveryResp map[string]any
	if err := json.Unmarshal(discoveryRec.Body.Bytes(), &discoveryResp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	taskUUID, _ := discoveryResp["task_uuid"].(string)
	if discoveryResp["ws_channel"] != "/api/ws/task/"+taskUUID {
		t.Fatalf("expected ws_channel for accepted task, got %#v", discoveryResp["ws_channel"])
	}

	syncBatchReq := httptest.NewRequest(http.MethodPost, "/api/team/teams/sync-batch", bytes.NewReader([]byte(`{"ids":[101,102]}`)))
	syncBatchReq.Header.Set("Content-Type", "application/json")
	syncBatchRec := httptest.NewRecorder()
	router.ServeHTTP(syncBatchRec, syncBatchReq)
	if syncBatchRec.Code != http.StatusAccepted {
		t.Fatalf("expected sync-batch 202, got %d", syncBatchRec.Code)
	}
	var syncBatchResp map[string]any
	if err := json.Unmarshal(syncBatchRec.Body.Bytes(), &syncBatchResp); err != nil {
		t.Fatalf("decode sync-batch response: %v", err)
	}
	if syncBatchResp["accepted_count"] != float64(2) {
		t.Fatalf("expected accepted_count=2, got %#v", syncBatchResp["accepted_count"])
	}
}

func TestMembershipHandlerCompatibilityEndpoints(t *testing.T) {
	router, _, _ := newTestHandler(t)

	revokeRec := httptest.NewRecorder()
	router.ServeHTTP(revokeRec, httptest.NewRequest(http.MethodPost, "/api/team/teams/101/memberships/201/revoke", nil))
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected revoke 200, got %d", revokeRec.Code)
	}
	var revokeResp map[string]any
	if err := json.Unmarshal(revokeRec.Body.Bytes(), &revokeResp); err != nil {
		t.Fatalf("decode revoke response: %v", err)
	}
	if revokeResp["next_status"] != "revoked" {
		t.Fatalf("expected revoke next_status=revoked, got %#v", revokeResp["next_status"])
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/team/memberships/202/bind-local-account", bytes.NewReader([]byte(`{"account_id":11}`)))
	bindReq.Header.Set("Content-Type", "application/json")
	bindRec := httptest.NewRecorder()
	router.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected bind 200, got %d", bindRec.Code)
	}
}

func TestTasksHandlerCompatibilityPayloads(t *testing.T) {
	router, repo, taskService := newTestHandler(t)

	syncTask, err := taskService.StartTeamSync(context.Background(), 101)
	if err != nil {
		t.Fatalf("start sync task: %v", err)
	}
	record, err := repo.GetTaskByUUID(context.Background(), syncTask.TaskUUID)
	if err != nil {
		t.Fatalf("get sync task: %v", err)
	}
	record.Status = jobs.StatusCompleted
	record.ActiveScopeKey = nil
	if _, err := repo.SaveTask(context.Background(), record); err != nil {
		t.Fatalf("save completed sync task: %v", err)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/team/teams/101/invite-emails", bytes.NewReader([]byte(`{"emails":["invitee@example.com"]}`)))
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteRec := httptest.NewRecorder()
	router.ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusAccepted {
		t.Fatalf("expected invite-emails 202, got %d", inviteRec.Code)
	}
	var inviteResp map[string]any
	if err := json.Unmarshal(inviteRec.Body.Bytes(), &inviteResp); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	inviteTaskUUID, _ := inviteResp["task_uuid"].(string)
	if err := taskService.ExecuteTask(context.Background(), inviteTaskUUID); err != nil {
		t.Fatalf("execute invite task: %v", err)
	}

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/team/tasks?team_id=101", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected tasks list 200, got %d", listRec.Code)
	}

	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, httptest.NewRequest(http.MethodGet, "/api/team/tasks/"+inviteTaskUUID, nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected task detail 200, got %d", detailRec.Code)
	}
	var detailResp map[string]any
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode task detail response: %v", err)
	}
	if detailResp["status"] != jobs.StatusCompleted {
		t.Fatalf("expected completed task detail status, got %#v", detailResp["status"])
	}
	if logs, ok := detailResp["logs"].([]any); !ok || len(logs) != 3 {
		t.Fatalf("expected three task logs, got %#v", detailResp["logs"])
	}
	if items, ok := detailResp["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("expected one task item, got %#v", detailResp["items"])
	}
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
