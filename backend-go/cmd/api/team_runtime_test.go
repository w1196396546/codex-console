package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/team"
)

func TestAPITeamRuntimeBuildsLiveGatewayAndExecutor(t *testing.T) {
	repo := newAPITeamRuntimeTestRepository()
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)

	var requests []team.TransitionRequest
	runtime := newAPITeamServices(
		repo,
		jobService,
		withAPITeamMembershipTransport(func(_ context.Context, req team.TransitionRequest) error {
			requests = append(requests, req)
			return nil
		}),
		withAPITeamExecutorHooks(team.TransitionExecutorHooks{
			Invite: func(_ context.Context, teamID int64, taskType string, payload map[string]any) (team.TaskExecutionResult, error) {
				return team.TaskExecutionResult{
					Status:  jobs.StatusCompleted,
					Summary: map[string]any{"team_id": teamID, "task_type": taskType},
					Logs:    []string{"invite completed"},
					Items: []team.TaskExecutionItem{
						{
							TargetEmail: "invitee@example.com",
							ItemStatus:  jobs.StatusCompleted,
							After:       map[string]any{"membership_status": "invited"},
							Message:     fmt.Sprintf("%s completed", taskType),
						},
					},
				}, nil
			},
		}),
	)
	if runtime.MembershipGateway == nil {
		t.Fatal("expected membership gateway to be wired")
	}
	if runtime.Executor == nil {
		t.Fatal("expected task executor to be wired")
	}
	if runtime.Service == nil || runtime.TaskService == nil {
		t.Fatalf("expected live team services, got %#v", runtime)
	}

	result, err := runtime.Service.ApplyMembershipAction(context.Background(), team.ApplyMembershipActionRequest{
		MembershipID: 201,
		Action:       "revoke",
	})
	if err != nil {
		t.Fatalf("ApplyMembershipAction returned error: %v", err)
	}
	if !result.Success || result.Message == "membership gateway not configured" {
		t.Fatalf("expected revoke to use live gateway, got %#v", result)
	}
	if len(requests) != 1 || requests[0].Path != "/backend-api/accounts/acct_101/invites" {
		t.Fatalf("expected revoke to hit upstream invite path, got %#v", requests)
	}

	removeResult, err := runtime.Service.ApplyMembershipAction(context.Background(), team.ApplyMembershipActionRequest{
		MembershipID: 202,
		Action:       "remove",
	})
	if err != nil {
		t.Fatalf("ApplyMembershipAction remove returned error: %v", err)
	}
	if !removeResult.Success || removeResult.Message == "membership gateway not configured" {
		t.Fatalf("expected remove to use live gateway, got %#v", removeResult)
	}
	if len(requests) != 2 || requests[1].Path != "/backend-api/accounts/acct_101/users/user_202" {
		t.Fatalf("expected remove to hit upstream member path, got %#v", requests)
	}
}

func TestAPITeamRuntimeAcceptedTasksAdvanceWithoutManualExecute(t *testing.T) {
	repo := newAPITeamRuntimeTestRepository()
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	runtime := newAPITeamServices(
		repo,
		jobService,
		withAPITeamExecutorHooks(team.TransitionExecutorHooks{
			Invite: func(_ context.Context, teamID int64, taskType string, payload map[string]any) (team.TaskExecutionResult, error) {
				if teamID != 101 || taskType != "invite_emails" {
					return team.TaskExecutionResult{}, errors.New("unexpected invite scope")
				}
				if _, ok := payload["emails"]; !ok {
					return team.TaskExecutionResult{}, errors.New("missing invite emails")
				}
				return team.TaskExecutionResult{
					Status:  jobs.StatusCompleted,
					Summary: map[string]any{"success": true},
					Logs:    []string{"invite completed"},
					Items: []team.TaskExecutionItem{
						{
							TargetEmail: "invitee@example.com",
							ItemStatus:  jobs.StatusCompleted,
							After:       map[string]any{"membership_status": "invited"},
							Message:     "invite completed",
						},
					},
				}, nil
			},
		}),
	)

	accepted, err := runtime.TaskService.StartInviteEmails(context.Background(), 101, []string{"invitee@example.com"})
	if err != nil {
		t.Fatalf("StartInviteEmails returned error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := runtime.TaskService.GetTaskDetail(context.Background(), accepted.TaskUUID)
		if err == nil && detail.Status != jobs.StatusPending {
			if detail.Status != jobs.StatusCompleted || len(detail.Items) != 1 {
				t.Fatalf("expected helper-built runtime to complete invite execution, got %#v", detail)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	detail, err := runtime.TaskService.GetTaskDetail(context.Background(), accepted.TaskUUID)
	if err != nil {
		t.Fatalf("GetTaskDetail returned error: %v", err)
	}
	t.Fatalf("expected helper-built runtime to auto execute accepted task, final detail=%#v", detail)
}

func TestAPITeamRuntimeDefaultExecutorUsesLiveTransitionHooks(t *testing.T) {
	installAPITeamHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected authorization header: %q", auth)
		}
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/check/v4-2023-04-27":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{
				"accounts": map[string]any{
					"acct_101": map[string]any{
						"account": map[string]any{
							"plan_type":         "team",
							"name":              "Alpha Team",
							"account_user_role": "account-owner",
						},
						"entitlement": map[string]any{
							"subscription_plan": "chatgpt-team",
						},
					},
				},
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_101/users":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user_21",
						"email":        "invitee@example.com",
						"role":         "member",
						"created_time": "2026-04-03T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case req.Method == http.MethodGet && req.URL.Path == "/backend-api/accounts/acct_101/invites":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{},
			})
		case req.Method == http.MethodPost && req.URL.Path == "/backend-api/accounts/acct_101/invites":
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode invite body: %v", err)
			}
			if payload["email_addresses"].([]any)[0] != "manual@example.com" {
				t.Fatalf("unexpected invite payload: %#v", payload)
			}
			return apiTeamJSONResponse(http.StatusOK, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected transition request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})

	repo := &teamRuntimeTestRepository{
		accounts: map[int64]team.AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			21: {ID: 21, Email: "invitee@example.com", Status: "active"},
		},
		teams:       map[int64]team.TeamRecord{},
		memberships: map[int64]team.TeamMembershipRecord{},
		tasks:       map[string]team.TeamTaskRecord{},
		taskItems:   map[int64][]team.TeamTaskItemRecord{},
	}
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	runtime := newAPITeamServices(repo, jobService)

	discoveryAccepted, err := runtime.TaskService.StartDiscovery(context.Background(), []int64{7})
	if err != nil {
		t.Fatalf("StartDiscovery returned error: %v", err)
	}
	waitForAPITeamRuntimeTask(t, runtime, discoveryAccepted.TaskUUID)
	if len(repo.teams) != 1 {
		t.Fatalf("expected discovery to persist one team, got %#v", repo.teams)
	}
	discoveredTeam := findAPITeamRecord(repo.teams, "acct_101")
	if discoveredTeam.UpstreamAccountID != "acct_101" || discoveredTeam.SyncStatus != "synced" {
		t.Fatalf("expected discovery to persist upstream team metadata, got %#v", discoveredTeam)
	}

	syncAccepted, err := runtime.TaskService.StartTeamSync(context.Background(), discoveredTeam.ID)
	if err != nil {
		t.Fatalf("StartTeamSync returned error: %v", err)
	}
	waitForAPITeamRuntimeTask(t, runtime, syncAccepted.TaskUUID)
	joined := findAPITeamMembership(repo.memberships, "invitee@example.com")
	if joined.MembershipStatus != "joined" || joined.UpstreamUserID != "user_21" || joined.LocalAccountID == nil || *joined.LocalAccountID != 21 {
		t.Fatalf("expected sync to persist joined membership, got %#v", joined)
	}
	if discovered := repo.teams[discoveredTeam.ID]; discovered.CurrentMembers != 1 || discovered.SeatsAvailable == nil || *discovered.SeatsAvailable != 5 {
		t.Fatalf("expected sync to refresh team counts, got %#v", discovered)
	}

	inviteAccepted, err := runtime.TaskService.StartInviteEmails(context.Background(), discoveredTeam.ID, []string{"manual@example.com"})
	if err != nil {
		t.Fatalf("StartInviteEmails returned error: %v", err)
	}
	waitForAPITeamRuntimeTask(t, runtime, inviteAccepted.TaskUUID)
	invited := findAPITeamMembership(repo.memberships, "manual@example.com")
	if invited.MembershipStatus != "invited" || invited.Source != "invite" {
		t.Fatalf("expected invite to persist invited membership, got %#v", invited)
	}
	if discovered := repo.teams[discoveredTeam.ID]; discovered.CurrentMembers != 2 || discovered.SeatsAvailable == nil || *discovered.SeatsAvailable != 4 {
		t.Fatalf("expected invite to refresh team counts, got %#v", discovered)
	}
}

func TestAPITeamRuntimeDefaultSyncBatchPersistsEveryRequestedTeam(t *testing.T) {
	installAPITeamHTTPStub(t, func(req *http.Request) (*http.Response, error) {
		if auth := req.Header.Get("Authorization"); auth != "Bearer owner-token" {
			t.Fatalf("unexpected authorization header: %q", auth)
		}
		switch req.URL.Path {
		case "/backend-api/accounts/acct_101/users":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user_21",
						"email":        "invitee@example.com",
						"role":         "member",
						"created_time": "2026-04-03T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case "/backend-api/accounts/acct_101/invites":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{"items": []any{}})
		case "/backend-api/accounts/acct_202/users":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{
				"items": []any{
					map[string]any{
						"id":           "user_22",
						"email":        "second@example.com",
						"role":         "member",
						"created_time": "2026-04-04T00:00:00Z",
					},
				},
				"total": 1,
				"limit": 100,
			})
		case "/backend-api/accounts/acct_202/invites":
			return apiTeamJSONResponse(http.StatusOK, map[string]any{"items": []any{}})
		default:
			t.Fatalf("unexpected transition request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})

	repo := newAPITeamRuntimeTestRepository()
	repo.accounts[22] = team.AccountRecord{ID: 22, Email: "second@example.com", Status: "active"}
	repo.teams[202] = team.TeamRecord{ID: 202, OwnerAccountID: 7, UpstreamAccountID: "acct_202", TeamName: "Beta Team", Status: "active", MaxMembers: intPtr(5), SeatsAvailable: intPtr(4)}
	repo.memberships = map[int64]team.TeamMembershipRecord{}
	jobService := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	runtime := newAPITeamServices(repo, jobService)

	accepted, err := runtime.TaskService.StartTeamSyncBatch(context.Background(), []int64{101, 202})
	if err != nil {
		t.Fatalf("StartTeamSyncBatch returned error: %v", err)
	}
	if accepted.TeamID == nil || *accepted.TeamID != 101 {
		t.Fatalf("expected accepted payload to keep first team context, got %#v", accepted.TeamID)
	}

	waitForAPITeamRuntimeTask(t, runtime, accepted.TaskUUID)

	alpha := findAPITeamMembership(repo.memberships, "invitee@example.com")
	if alpha.TeamID != 101 || alpha.LocalAccountID == nil || *alpha.LocalAccountID != 21 || alpha.MembershipStatus != "joined" {
		t.Fatalf("expected first team membership to persist, got %#v", alpha)
	}
	beta := findAPITeamMembership(repo.memberships, "second@example.com")
	if beta.TeamID != 202 || beta.LocalAccountID == nil || *beta.LocalAccountID != 22 || beta.MembershipStatus != "joined" {
		t.Fatalf("expected second team membership to persist, got %#v", beta)
	}
	if repo.teams[101].SyncStatus != "synced" || repo.teams[101].LastSyncAt == nil {
		t.Fatalf("expected first team sync markers, got %#v", repo.teams[101])
	}
	if repo.teams[202].SyncStatus != "synced" || repo.teams[202].LastSyncAt == nil {
		t.Fatalf("expected second team sync markers, got %#v", repo.teams[202])
	}
}

func newAPITeamRuntimeTestRepository() *teamRuntimeTestRepository {
	return &teamRuntimeTestRepository{
		accounts: map[int64]team.AccountRecord{
			7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			21: {ID: 21, Email: "invitee@example.com", Status: "active"},
		},
		teams: map[int64]team.TeamRecord{
			101: {ID: 101, OwnerAccountID: 7, UpstreamAccountID: "acct_101", TeamName: "Alpha Team", Status: "active", MaxMembers: intPtr(5), SeatsAvailable: intPtr(4)},
		},
		memberships: map[int64]team.TeamMembershipRecord{
			201: {
				ID:               201,
				TeamID:           101,
				MemberEmail:      "invitee@example.com",
				MemberRole:       "member",
				MembershipStatus: "invited",
			},
			202: {
				ID:               202,
				TeamID:           101,
				MemberEmail:      "joined@example.com",
				MemberRole:       "member",
				MembershipStatus: "joined",
				UpstreamUserID:   "user_202",
			},
		},
		tasks:     map[string]team.TeamTaskRecord{},
		taskItems: map[int64][]team.TeamTaskItemRecord{},
	}
}

type teamRuntimeTestRepository struct {
	accounts       map[int64]team.AccountRecord
	teams          map[int64]team.TeamRecord
	memberships    map[int64]team.TeamMembershipRecord
	tasks          map[string]team.TeamTaskRecord
	taskItems      map[int64][]team.TeamTaskItemRecord
	nextTaskID     int64
	nextTaskItemID int64
}

func (r *teamRuntimeTestRepository) ListTeams(_ context.Context, req team.ListTeamsRequest) ([]team.TeamRecord, int, error) {
	records := make([]team.TeamRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if strings.TrimSpace(req.Status) != "" && record.Status != req.Status {
			continue
		}
		if req.OwnerAccountID > 0 && record.OwnerAccountID != req.OwnerAccountID {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, len(records), nil
}

func (r *teamRuntimeTestRepository) GetTeam(_ context.Context, teamID int64) (team.TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok {
		return team.TeamRecord{}, team.ErrNotFound
	}
	return record, nil
}

func (r *teamRuntimeTestRepository) GetAccount(_ context.Context, accountID int64) (team.AccountRecord, error) {
	record, ok := r.accounts[accountID]
	if !ok {
		return team.AccountRecord{}, team.ErrNotFound
	}
	return record, nil
}

func (r *teamRuntimeTestRepository) ListAccountsByIDs(_ context.Context, accountIDs []int64) (map[int64]team.AccountRecord, error) {
	result := make(map[int64]team.AccountRecord, len(accountIDs))
	for _, accountID := range accountIDs {
		if record, ok := r.accounts[accountID]; ok {
			result[accountID] = record
		}
	}
	return result, nil
}

func (r *teamRuntimeTestRepository) ListMembershipsByTeam(_ context.Context, teamID int64) ([]team.TeamMembershipRecord, error) {
	records := make([]team.TeamMembershipRecord, 0)
	for _, record := range r.memberships {
		if record.TeamID == teamID {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

func (r *teamRuntimeTestRepository) GetMembership(_ context.Context, membershipID int64) (team.TeamMembershipRecord, error) {
	record, ok := r.memberships[membershipID]
	if !ok {
		return team.TeamMembershipRecord{}, team.ErrNotFound
	}
	return record, nil
}

func (r *teamRuntimeTestRepository) SaveMembership(_ context.Context, membership team.TeamMembershipRecord) (team.TeamMembershipRecord, error) {
	if _, ok := r.memberships[membership.ID]; !ok {
		return team.TeamMembershipRecord{}, team.ErrNotFound
	}
	r.memberships[membership.ID] = membership
	return membership, nil
}

func (r *teamRuntimeTestRepository) SaveTeam(_ context.Context, record team.TeamRecord) (team.TeamRecord, error) {
	if _, ok := r.teams[record.ID]; !ok {
		return team.TeamRecord{}, team.ErrNotFound
	}
	r.teams[record.ID] = record
	return record, nil
}

func (r *teamRuntimeTestRepository) UpsertTeam(_ context.Context, record team.TeamRecord) (team.TeamRecord, error) {
	for id, existing := range r.teams {
		if existing.OwnerAccountID == record.OwnerAccountID && strings.EqualFold(existing.UpstreamAccountID, record.UpstreamAccountID) {
			record.ID = id
			if record.CreatedAt.IsZero() {
				record.CreatedAt = existing.CreatedAt
			}
			if record.UpdatedAt.IsZero() {
				record.UpdatedAt = time.Now().UTC()
			}
			r.teams[id] = record
			return record, nil
		}
	}
	r.nextTaskID++
	record.ID = r.nextTaskID
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	r.teams[record.ID] = record
	return record, nil
}

func (r *teamRuntimeTestRepository) ListTasks(_ context.Context, req team.ListTasksRequest) ([]team.TeamTaskRecord, error) {
	records := make([]team.TeamTaskRecord, 0, len(r.tasks))
	for _, record := range r.tasks {
		if req.TeamID != nil && (record.TeamID == nil || *record.TeamID != *req.TeamID) {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	return records, nil
}

func (r *teamRuntimeTestRepository) GetTaskByUUID(_ context.Context, taskUUID string) (team.TeamTaskRecord, error) {
	record, ok := r.tasks[taskUUID]
	if !ok {
		return team.TeamTaskRecord{}, team.ErrNotFound
	}
	return record, nil
}

func (r *teamRuntimeTestRepository) ListTaskItems(_ context.Context, taskID int64) ([]team.TeamTaskItemRecord, error) {
	return append([]team.TeamTaskItemRecord(nil), r.taskItems[taskID]...), nil
}

func (r *teamRuntimeTestRepository) CreateTask(_ context.Context, task team.TeamTaskRecord) (team.TeamTaskRecord, error) {
	scopeKey := ""
	if task.ActiveScopeKey != nil {
		scopeKey = *task.ActiveScopeKey
	}
	for _, existing := range r.tasks {
		if existing.ActiveScopeKey == nil || scopeKey == "" {
			continue
		}
		if *existing.ActiveScopeKey == scopeKey && !isFinishedJobStatus(existing.Status) {
			return team.TeamTaskRecord{}, team.ErrActiveScopeConflict
		}
	}
	r.nextTaskID++
	task.ID = r.nextTaskID
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *teamRuntimeTestRepository) SaveTask(_ context.Context, task team.TeamTaskRecord) (team.TeamTaskRecord, error) {
	if _, ok := r.tasks[task.TaskUUID]; !ok {
		return team.TeamTaskRecord{}, team.ErrNotFound
	}
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *teamRuntimeTestRepository) SaveTaskItem(_ context.Context, item team.TeamTaskItemRecord) (team.TeamTaskItemRecord, error) {
	r.nextTaskItemID++
	item.ID = r.nextTaskItemID
	r.taskItems[item.TaskID] = append(r.taskItems[item.TaskID], item)
	return item, nil
}

func (r *teamRuntimeTestRepository) UpsertMembership(_ context.Context, record team.TeamMembershipRecord) (team.TeamMembershipRecord, error) {
	for id, existing := range r.memberships {
		if existing.TeamID == record.TeamID && strings.EqualFold(existing.MemberEmail, record.MemberEmail) {
			record.ID = id
			if record.CreatedAt.IsZero() {
				record.CreatedAt = existing.CreatedAt
			}
			if record.UpdatedAt.IsZero() {
				record.UpdatedAt = time.Now().UTC()
			}
			r.memberships[id] = record
			return record, nil
		}
	}
	r.nextTaskItemID++
	record.ID = r.nextTaskItemID
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	r.memberships[record.ID] = record
	return record, nil
}

func (r *teamRuntimeTestRepository) ListAccountsByEmails(_ context.Context, emails []string) (map[string]team.AccountRecord, error) {
	result := make(map[string]team.AccountRecord, len(emails))
	for _, email := range emails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		for _, account := range r.accounts {
			if strings.ToLower(strings.TrimSpace(account.Email)) == normalized {
				result[normalized] = account
				break
			}
		}
	}
	return result, nil
}

func (r *teamRuntimeTestRepository) FindActiveTask(_ context.Context, scopeType string, scopeID string, taskType string) (team.TeamTaskRecord, error) {
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
	return team.TeamTaskRecord{}, team.ErrNotFound
}

func intPtr(value int) *int {
	return &value
}

func isFinishedJobStatus(status string) bool {
	switch status {
	case jobs.StatusCompleted, jobs.StatusFailed, jobs.StatusCancelled:
		return true
	default:
		return false
	}
}

func installAPITeamHTTPStub(t *testing.T, fn func(req *http.Request) (*http.Response, error)) {
	t.Helper()

	previous := http.DefaultTransport
	http.DefaultTransport = apiTeamRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "chatgpt.com" {
			return previous.RoundTrip(req)
		}
		return fn(req)
	})
	t.Cleanup(func() {
		http.DefaultTransport = previous
	})
}

type apiTeamRoundTripFunc func(req *http.Request) (*http.Response, error)

func (fn apiTeamRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func apiTeamJSONResponse(status int, payload map[string]any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func waitForAPITeamRuntimeTask(t *testing.T, runtime apiTeamRuntime, taskUUID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := runtime.TaskService.GetTaskDetail(context.Background(), taskUUID)
		if err == nil && detail.Status != jobs.StatusPending && detail.Status != jobs.StatusRunning {
			if detail.Status != jobs.StatusCompleted {
				t.Fatalf("expected task %s to complete, got %#v", taskUUID, detail)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	detail, err := runtime.TaskService.GetTaskDetail(context.Background(), taskUUID)
	if err != nil {
		t.Fatalf("GetTaskDetail returned error: %v", err)
	}
	t.Fatalf("expected task %s to finish, got %#v", taskUUID, detail)
}

func findAPITeamMembership(records map[int64]team.TeamMembershipRecord, email string) team.TeamMembershipRecord {
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, record := range records {
		if strings.ToLower(strings.TrimSpace(record.MemberEmail)) == normalized {
			return record
		}
	}
	return team.TeamMembershipRecord{}
}

func findAPITeamRecord(records map[int64]team.TeamRecord, upstreamAccountID string) team.TeamRecord {
	normalized := strings.ToLower(strings.TrimSpace(upstreamAccountID))
	for _, record := range records {
		if strings.ToLower(strings.TrimSpace(record.UpstreamAccountID)) == normalized {
			return record
		}
	}
	return team.TeamRecord{}
}
