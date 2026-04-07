package team

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

type fakeTaskRepository struct {
	fakeRepository
	nextTaskID     int64
	nextTaskItemID int64
	createTaskErr  error
}

func (r *fakeTaskRepository) CreateTask(_ context.Context, task TeamTaskRecord) (TeamTaskRecord, error) {
	if r.createTaskErr != nil {
		return TeamTaskRecord{}, r.createTaskErr
	}
	scopeKey := ""
	if task.ActiveScopeKey != nil {
		scopeKey = *task.ActiveScopeKey
	}
	for _, existing := range r.tasks {
		if existing.ActiveScopeKey == nil || scopeKey == "" {
			continue
		}
		if *existing.ActiveScopeKey == scopeKey && !isFinishedTaskStatus(existing.Status) {
			return TeamTaskRecord{}, ErrActiveScopeConflict
		}
	}
	r.nextTaskID++
	task.ID = r.nextTaskID
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *fakeTaskRepository) SaveTask(_ context.Context, task TeamTaskRecord) (TeamTaskRecord, error) {
	if _, ok := r.tasks[task.TaskUUID]; !ok {
		return TeamTaskRecord{}, ErrNotFound
	}
	r.tasks[task.TaskUUID] = task
	return task, nil
}

func (r *fakeTaskRepository) SaveTaskItem(_ context.Context, item TeamTaskItemRecord) (TeamTaskItemRecord, error) {
	r.nextTaskItemID++
	item.ID = r.nextTaskItemID
	r.taskItems[item.TaskID] = append(r.taskItems[item.TaskID], item)
	return item, nil
}

func (r *fakeTaskRepository) FindActiveTask(_ context.Context, scopeType string, scopeID string, taskType string) (TeamTaskRecord, error) {
	for _, task := range r.tasks {
		if task.ScopeType != scopeType || task.ScopeID != scopeID {
			continue
		}
		if taskType != "" && task.TaskType != taskType {
			continue
		}
		if !isFinishedTaskStatus(task.Status) {
			return task, nil
		}
	}
	return TeamTaskRecord{}, ErrNotFound
}

type fakeTaskExecutor struct {
	results         map[string]TaskExecutionResult
	errs            map[string]error
	executed        []string
	blockByTaskType map[string]chan struct{}
}

func (e *fakeTaskExecutor) Execute(_ context.Context, task TaskExecutionRequest) (TaskExecutionResult, error) {
	e.executed = append(e.executed, task.TaskType)
	if gate := e.blockByTaskType[task.TaskType]; gate != nil {
		<-gate
	}
	if err := e.errs[task.TaskType]; err != nil {
		return TaskExecutionResult{}, err
	}
	result, ok := e.results[task.TaskType]
	if !ok {
		return TaskExecutionResult{}, errors.New("missing fake result")
	}
	return result, nil
}

func TestDiscoveryAcceptedTaskReusesOwnerScopeAndWsChannel(t *testing.T) {
	discoveryGate := make(chan struct{})
	repo := &fakeTaskRepository{
		fakeRepository: fakeRepository{
			accounts:  map[int64]AccountRecord{},
			teams:     map[int64]TeamRecord{},
			tasks:     map[string]TeamTaskRecord{},
			taskItems: map[int64][]TeamTaskItemRecord{},
		},
	}
	jobRuntime := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	readService := NewService(repo, &fakeMembershipGateway{})
	executor := &fakeTaskExecutor{
		results: map[string]TaskExecutionResult{
			"discover_owner_teams": {
				Status:  jobs.StatusCompleted,
				Summary: map[string]any{"accounts_scanned": 1, "teams_persisted": 0},
				Logs:    []string{"discovery completed"},
			},
		},
		blockByTaskType: map[string]chan struct{}{
			"discover_owner_teams": discoveryGate,
		},
	}
	taskService := NewTaskService(repo, readService, jobRuntime, executor)

	firstAccepted, err := taskService.StartDiscovery(context.Background(), []int64{7, 8})
	if err != nil {
		t.Fatalf("StartDiscovery returned error: %v", err)
	}
	if !firstAccepted.Success {
		t.Fatalf("expected success accepted payload, got %#v", firstAccepted)
	}
	if firstAccepted.TaskUUID == "" || firstAccepted.WSChannel != "/api/ws/task/"+firstAccepted.TaskUUID {
		t.Fatalf("expected ws channel to point at task websocket, got %#v", firstAccepted)
	}
	if firstAccepted.ScopeType != "owner" || firstAccepted.ScopeID != "7" {
		t.Fatalf("expected owner scope payload, got %#v", firstAccepted)
	}
	if firstAccepted.OwnerAccountID == nil || *firstAccepted.OwnerAccountID != 7 {
		t.Fatalf("expected owner_account_id=7, got %#v", firstAccepted.OwnerAccountID)
	}

	secondAccepted, err := taskService.StartDiscovery(context.Background(), []int64{7, 9})
	if err != nil {
		t.Fatalf("second StartDiscovery returned error: %v", err)
	}
	if secondAccepted.TaskUUID != firstAccepted.TaskUUID {
		t.Fatalf("expected discovery to reuse active task_uuid by first owner scope, got first=%s second=%s", firstAccepted.TaskUUID, secondAccepted.TaskUUID)
	}

	close(discoveryGate)
	waitForTeamTaskStatus(t, repo, firstAccepted.TaskUUID, jobs.StatusCompleted)
}

func TestAcceptedTaskAdmissionFailureDoesNotLeavePendingOrphanJob(t *testing.T) {
	repo := &fakeTaskRepository{
		fakeRepository: fakeRepository{
			accounts:  map[int64]AccountRecord{},
			teams:     map[int64]TeamRecord{},
			tasks:     map[string]TeamTaskRecord{},
			taskItems: map[int64][]TeamTaskItemRecord{},
		},
		createTaskErr: errors.New("persist team task failed"),
	}
	jobRuntime := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	taskService := NewTaskService(repo, NewService(repo, &fakeMembershipGateway{}), jobRuntime, &fakeTaskExecutor{})

	_, err := taskService.StartTeamSync(context.Background(), 101)
	if err == nil || err.Error() != "persist team task failed" {
		t.Fatalf("expected task persistence failure, got %v", err)
	}

	job, jobErr := jobRuntime.GetJob(context.Background(), "job-1")
	if errors.Is(jobErr, jobs.ErrJobNotFound) {
		return
	}
	if jobErr != nil {
		t.Fatalf("GetJob returned unexpected error: %v", jobErr)
	}
	if job.Status == jobs.StatusPending {
		t.Fatalf("expected admission cleanup to avoid orphan pending job, got %#v", job)
	}
}

func TestInviteExecutionPersistsResultsAndConflictsOnActiveScope(t *testing.T) {
	now := time.Date(2026, time.April, 5, 14, 0, 0, 0, time.UTC)
	repo := &fakeTaskRepository{
		fakeRepository: fakeRepository{
			accounts: map[int64]AccountRecord{
				7: {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
			},
			teams: map[int64]TeamRecord{
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
			memberships: map[int64]TeamMembershipRecord{},
			tasks:       map[string]TeamTaskRecord{},
			taskItems:   map[int64][]TeamTaskItemRecord{},
		},
	}
	jobRepo := jobs.NewInMemoryRepository()
	jobRuntime := jobs.NewService(jobRepo, nil)
	readService := NewService(repo, &fakeMembershipGateway{})
	executor := &fakeTaskExecutor{
		results: map[string]TaskExecutionResult{
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
				Items: []TaskExecutionItem{
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
		errs: map[string]error{},
	}
	taskService := NewTaskService(repo, readService, jobRuntime, executor)

	activeSync, err := taskService.StartTeamSync(context.Background(), 101)
	if err != nil {
		t.Fatalf("StartTeamSync returned error: %v", err)
	}
	if activeSync.TaskUUID == "" {
		t.Fatalf("expected sync accepted payload, got %#v", activeSync)
	}

	_, err = taskService.StartInviteEmails(context.Background(), 101, []string{"invitee@example.com"})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected active scope conflict, got %v", err)
	}

	syncTask, err := repo.GetTaskByUUID(context.Background(), activeSync.TaskUUID)
	if err != nil {
		t.Fatalf("load sync task: %v", err)
	}
	syncTask.Status = jobs.StatusCompleted
	syncTask.ActiveScopeKey = nil
	syncTask.CompletedAt = timePtr(now.Add(5 * time.Minute))
	if _, err := repo.SaveTask(context.Background(), syncTask); err != nil {
		t.Fatalf("complete sync task in fake repo: %v", err)
	}

	inviteAccepted, err := taskService.StartInviteEmails(context.Background(), 101, []string{"invitee@example.com"})
	if err != nil {
		t.Fatalf("StartInviteEmails returned error: %v", err)
	}
	if inviteAccepted.TaskType != "invite_emails" || inviteAccepted.WSChannel != "/api/ws/task/"+inviteAccepted.TaskUUID {
		t.Fatalf("expected invite accepted payload to reuse task websocket path, got %#v", inviteAccepted)
	}
	waitForTeamTaskStatus(t, repo, inviteAccepted.TaskUUID, jobs.StatusCompleted)

	detail, err := taskService.GetTaskDetail(context.Background(), inviteAccepted.TaskUUID)
	if err != nil {
		t.Fatalf("GetTaskDetail returned error: %v", err)
	}
	if detail.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed status from shared jobs runtime, got %#v", detail.Status)
	}
	if len(detail.Logs) != 3 || len(detail.Items) != 1 {
		t.Fatalf("expected invite logs/items to persist, got logs=%#v items=%#v", detail.Logs, detail.Items)
	}
	if detail.Summary["success"] != true {
		t.Fatalf("expected success summary, got %#v", detail.Summary)
	}
}

func TestTeamTaskLiveExecutionLaunchesAcceptedTasksWithoutManualExecute(t *testing.T) {
	repo := &fakeTaskRepository{
		fakeRepository: fakeRepository{
			accounts: map[int64]AccountRecord{
				7:  {ID: 7, Email: "owner@example.com", Status: "active", AccessToken: "owner-token"},
				8:  {ID: 8, Email: "owner-two@example.com", Status: "active", AccessToken: "owner-token-2"},
				21: {ID: 21, Email: "invitee@example.com", Status: "active"},
			},
			teams: map[int64]TeamRecord{
				101: {ID: 101, OwnerAccountID: 7, UpstreamAccountID: "acct_101", TeamName: "Alpha Team", Status: "active", MaxMembers: intPtr(5), SeatsAvailable: intPtr(4)},
				202: {ID: 202, OwnerAccountID: 8, UpstreamAccountID: "acct_202", TeamName: "Beta Team", Status: "active", MaxMembers: intPtr(5), SeatsAvailable: intPtr(4)},
			},
			memberships: map[int64]TeamMembershipRecord{},
			tasks:       map[string]TeamTaskRecord{},
			taskItems:   map[int64][]TeamTaskItemRecord{},
		},
	}
	jobRuntime := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	readService := NewService(repo, &fakeMembershipGateway{})
	executor := &fakeTaskExecutor{
		results: map[string]TaskExecutionResult{
			"discover_owner_teams": {
				Status:  jobs.StatusCompleted,
				Summary: map[string]any{"accounts_scanned": 1, "teams_persisted": 1},
				Logs:    []string{"discovery completed"},
			},
			"sync_team": {
				Status:  jobs.StatusCompleted,
				Summary: map[string]any{"processed_count": 1, "failed_count": 0},
				Logs:    []string{"sync completed"},
			},
			"invite_emails": {
				Status:  jobs.StatusCompleted,
				Summary: map[string]any{"success": true},
				Logs:    []string{"invite completed"},
				Items: []TaskExecutionItem{
					{
						TargetEmail: "invitee@example.com",
						ItemStatus:  jobs.StatusCompleted,
						After:       map[string]any{"membership_status": "invited"},
						Message:     "invite completed",
					},
				},
			},
		},
		errs: map[string]error{},
	}
	taskService := NewTaskService(repo, readService, jobRuntime, executor)

	testCases := []struct {
		name  string
		start func(context.Context) (AcceptedTaskResponse, error)
	}{
		{
			name: "discovery",
			start: func(ctx context.Context) (AcceptedTaskResponse, error) {
				return taskService.StartDiscovery(ctx, []int64{7})
			},
		},
		{
			name: "sync",
			start: func(ctx context.Context) (AcceptedTaskResponse, error) {
				return taskService.StartTeamSync(ctx, 101)
			},
		},
		{
			name: "invite",
			start: func(ctx context.Context) (AcceptedTaskResponse, error) {
				return taskService.StartInviteEmails(ctx, 202, []string{"invitee@example.com"})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			accepted, err := tc.start(context.Background())
			if err != nil {
				t.Fatalf("start accepted task: %v", err)
			}
			waitForTeamTaskStatus(t, repo, accepted.TaskUUID, jobs.StatusCompleted)

			detail, err := taskService.GetTaskDetail(context.Background(), accepted.TaskUUID)
			if err != nil {
				t.Fatalf("GetTaskDetail returned error: %v", err)
			}
			if detail.Status != jobs.StatusCompleted {
				t.Fatalf("expected completed status without manual ExecuteTask, got %#v", detail.Status)
			}
			if detail.StartedAt == nil || detail.CompletedAt == nil {
				t.Fatalf("expected started/completed timestamps to be populated, got %#v", detail)
			}
		})
	}

	if len(executor.executed) != 3 {
		t.Fatalf("expected accepted tasks to auto launch all three task types, got %#v", executor.executed)
	}
}

func waitForTeamTaskStatus(t *testing.T, repo *fakeTaskRepository, taskUUID string, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := repo.GetTaskByUUID(context.Background(), taskUUID)
		if err == nil && task.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	task, err := repo.GetTaskByUUID(context.Background(), taskUUID)
	if err != nil {
		t.Fatalf("load task %s: %v", taskUUID, err)
	}
	t.Fatalf("timed out waiting for task %s to reach %s, final status=%s", taskUUID, want, task.Status)
}
