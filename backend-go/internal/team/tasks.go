package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

var ErrActiveScopeConflict = errors.New("team: active scope conflict")

type ConflictError struct {
	ScopeKey string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("409 conflict: %s already has active write task", e.ScopeKey)
}

type AcceptedTaskResponse struct {
	Success           bool   `json:"success"`
	TaskUUID          string `json:"task_uuid"`
	TaskType          string `json:"task_type"`
	Status            string `json:"status"`
	WSChannel         string `json:"ws_channel"`
	ScopeType         string `json:"scope_type,omitempty"`
	ScopeID           string `json:"scope_id,omitempty"`
	TeamID            *int64 `json:"team_id,omitempty"`
	OwnerAccountID    *int64 `json:"owner_account_id,omitempty"`
	AcceptedCount     int    `json:"accepted_count,omitempty"`
	DeduplicatedCount int    `json:"deduplicated_count,omitempty"`
	SkippedCount      int    `json:"skipped_count,omitempty"`
}

type TaskExecutionRequest struct {
	TaskUUID       string
	TaskType       string
	TeamID         *int64
	OwnerAccountID *int64
	RequestPayload map[string]any
}

type TaskExecutionItem struct {
	TargetEmail  string
	ItemStatus   string
	Before       map[string]any
	After        map[string]any
	Message      string
	ErrorMessage string
}

type TaskExecutionResult struct {
	Status  string
	Summary map[string]any
	Logs    []string
	Items   []TaskExecutionItem
}

type TaskExecutor interface {
	Execute(ctx context.Context, task TaskExecutionRequest) (TaskExecutionResult, error)
}

type TaskRuntime interface {
	CreateJob(ctx context.Context, params jobs.CreateJobParams) (jobs.Job, error)
	GetJob(ctx context.Context, jobID string) (jobs.Job, error)
	ListJobLogs(ctx context.Context, jobID string) ([]jobs.JobLog, error)
	AppendLog(ctx context.Context, jobID string, level string, message string) error
	MarkRunning(ctx context.Context, jobID string, workerID string) (jobs.Job, error)
	MarkCompleted(ctx context.Context, jobID string, result map[string]any) (jobs.Job, error)
	MarkFailed(ctx context.Context, jobID string, message string) (jobs.Job, error)
}

type TaskRepository interface {
	Repository
	CreateTask(ctx context.Context, task TeamTaskRecord) (TeamTaskRecord, error)
	SaveTask(ctx context.Context, task TeamTaskRecord) (TeamTaskRecord, error)
	SaveTaskItem(ctx context.Context, item TeamTaskItemRecord) (TeamTaskItemRecord, error)
	FindActiveTask(ctx context.Context, scopeType string, scopeID string, taskType string) (TeamTaskRecord, error)
}

type TaskService struct {
	repository  TaskRepository
	readService *Service
	jobs        TaskRuntime
	executor    TaskExecutor
}

func NewTaskService(repository TaskRepository, readService *Service, runtime TaskRuntime, executor TaskExecutor) *TaskService {
	if readService == nil {
		readService = NewService(repository, nil)
	}
	return &TaskService{
		repository:  repository,
		readService: readService,
		jobs:        runtime,
		executor:    executor,
	}
}

func (s *TaskService) StartDiscovery(ctx context.Context, ownerAccountIDs []int64) (AcceptedTaskResponse, error) {
	normalizedIDs := normalizeUniqueInt64s(ownerAccountIDs)
	if len(normalizedIDs) == 0 {
		return AcceptedTaskResponse{}, fmt.Errorf("ids 不能为空")
	}

	scopeID := fmt.Sprint(normalizedIDs[0])
	existing, err := s.repository.FindActiveTask(ctx, "owner", scopeID, "discover_owner_teams")
	if err == nil {
		return s.acceptedResponseForTask(ctx, existing), nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return AcceptedTaskResponse{}, err
	}

	return s.createAcceptedTask(ctx, createAcceptedTaskParams{
		TaskType:       "discover_owner_teams",
		ScopeType:      "owner",
		ScopeID:        scopeID,
		OwnerAccountID: ptrInt64(normalizedIDs[0]),
		RequestPayload: map[string]any{"ids": normalizedIDs},
	})
}

func (s *TaskService) StartTeamSync(ctx context.Context, teamID int64) (AcceptedTaskResponse, error) {
	return s.createScopedTeamTask(ctx, "sync_team", teamID, map[string]any{"team_id": teamID})
}

func (s *TaskService) StartTeamSyncBatch(ctx context.Context, teamIDs []int64) (AcceptedTaskResponse, error) {
	normalizedIDs := normalizeUniqueInt64s(teamIDs)
	if len(normalizedIDs) == 0 {
		return AcceptedTaskResponse{}, fmt.Errorf("ids 不能为空")
	}
	return s.createScopedTeamTask(ctx, "sync_all_teams", normalizedIDs[0], map[string]any{"ids": normalizedIDs})
}

func (s *TaskService) StartInviteAccounts(ctx context.Context, teamID int64, accountIDs []int64) (AcceptedTaskResponse, error) {
	normalizedIDs := normalizeUniqueInt64s(accountIDs)
	if len(normalizedIDs) == 0 {
		return AcceptedTaskResponse{}, fmt.Errorf("未找到可邀请账号")
	}
	return s.createScopedTeamTask(ctx, "invite_accounts", teamID, map[string]any{"ids": normalizedIDs})
}

func (s *TaskService) StartInviteEmails(ctx context.Context, teamID int64, emails []string) (AcceptedTaskResponse, error) {
	normalizedEmails := normalizeUniqueEmails(emails)
	if len(normalizedEmails) == 0 {
		return AcceptedTaskResponse{}, fmt.Errorf("emails 不能为空")
	}
	return s.createScopedTeamTask(ctx, "invite_emails", teamID, map[string]any{"emails": normalizedEmails})
}

func (s *TaskService) ExecuteTask(ctx context.Context, taskUUID string) error {
	if s.executor == nil {
		return errors.New("team task executor not configured")
	}

	task, err := s.repository.GetTaskByUUID(ctx, taskUUID)
	if err != nil {
		return err
	}

	startedAt := task.StartedAt
	if startedAt == nil {
		now := time.Now().UTC()
		startedAt = &now
	}
	task.Status = jobs.StatusRunning
	task.StartedAt = startedAt
	task.ErrorMessage = ""
	if _, err := s.repository.SaveTask(ctx, task); err != nil {
		return err
	}
	if s.jobs != nil {
		if _, err := s.jobs.MarkRunning(ctx, taskUUID, "team-runtime"); err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
			return err
		}
	}

	result, execErr := s.executor.Execute(ctx, TaskExecutionRequest{
		TaskUUID:       task.TaskUUID,
		TaskType:       task.TaskType,
		TeamID:         task.TeamID,
		OwnerAccountID: task.OwnerAccountID,
		RequestPayload: cloneMap(task.RequestPayload),
	})

	now := time.Now().UTC()
	if execErr != nil {
		if s.jobs != nil {
			if appendErr := s.jobs.AppendLog(ctx, taskUUID, "error", execErr.Error()); appendErr != nil && !errors.Is(appendErr, jobs.ErrJobNotFound) {
				return appendErr
			}
			if _, err := s.jobs.MarkFailed(ctx, taskUUID, execErr.Error()); err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
				return err
			}
		}
		task.Status = jobs.StatusFailed
		task.ErrorMessage = execErr.Error()
		task.CompletedAt = &now
		task.ActiveScopeKey = nil
		task.Logs = appendLogs(task.Logs, []string{execErr.Error()})
		_, err = s.repository.SaveTask(ctx, task)
		return err
	}

	for _, line := range result.Logs {
		if s.jobs != nil {
			if err := s.jobs.AppendLog(ctx, taskUUID, "info", line); err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
				return err
			}
		}
	}
	for _, item := range result.Items {
		if _, err := s.repository.SaveTaskItem(ctx, TeamTaskItemRecord{
			TaskID:       task.ID,
			TargetEmail:  item.TargetEmail,
			ItemStatus:   item.ItemStatus,
			Before:       cloneMap(item.Before),
			After:        cloneMap(item.After),
			Message:      item.Message,
			ErrorMessage: item.ErrorMessage,
		}); err != nil {
			return err
		}
	}

	finalStatus := strings.TrimSpace(result.Status)
	if finalStatus == "" {
		finalStatus = jobs.StatusCompleted
	}
	if s.jobs != nil {
		switch finalStatus {
		case jobs.StatusFailed:
			message := "team task failed"
			if detail, ok := result.Summary["detail"].(string); ok && strings.TrimSpace(detail) != "" {
				message = detail
			}
			if _, err := s.jobs.MarkFailed(ctx, taskUUID, message); err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
				return err
			}
		default:
			if _, err := s.jobs.MarkCompleted(ctx, taskUUID, cloneMap(result.Summary)); err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
				return err
			}
		}
	}

	task.Status = finalStatus
	task.ResultPayload = cloneMap(result.Summary)
	task.Logs = appendLogs(task.Logs, result.Logs)
	task.CompletedAt = &now
	if isFinishedTaskStatus(finalStatus) {
		task.ActiveScopeKey = nil
	}
	_, err = s.repository.SaveTask(ctx, task)
	return err
}

func (s *TaskService) ListTasks(ctx context.Context, req ListTasksRequest) (ListTasksResponse, error) {
	return s.readService.ListTasks(ctx, req)
}

func (s *TaskService) GetTaskDetail(ctx context.Context, taskUUID string) (TeamTaskDetailResponse, error) {
	response, err := s.readService.GetTaskDetail(ctx, taskUUID)
	if err != nil {
		return TeamTaskDetailResponse{}, err
	}
	if s.jobs == nil {
		return response, nil
	}

	job, err := s.jobs.GetJob(ctx, taskUUID)
	if err == nil {
		response.Status = job.Status
	} else if !errors.Is(err, jobs.ErrJobNotFound) {
		return TeamTaskDetailResponse{}, err
	}

	runtimeLogs, err := s.jobs.ListJobLogs(ctx, taskUUID)
	if err != nil && !errors.Is(err, jobs.ErrJobNotFound) {
		return TeamTaskDetailResponse{}, err
	}
	if len(runtimeLogs) > 0 {
		merged := make([]string, 0, len(runtimeLogs)+len(response.Logs))
		seen := map[string]struct{}{}
		for _, item := range runtimeLogs {
			line := strings.TrimSpace(item.Message)
			if line == "" {
				continue
			}
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			merged = append(merged, line)
		}
		for _, line := range response.Logs {
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			merged = append(merged, line)
		}
		response.Logs = merged
		response.GuardLogs = append([]string(nil), merged...)
	}

	return response, nil
}

func (s *TaskService) createScopedTeamTask(ctx context.Context, taskType string, teamID int64, requestPayload map[string]any) (AcceptedTaskResponse, error) {
	scopeKey := fmt.Sprintf("team:%d", teamID)
	if _, err := s.repository.FindActiveTask(ctx, "team", fmt.Sprint(teamID), ""); err == nil {
		return AcceptedTaskResponse{}, &ConflictError{ScopeKey: scopeKey}
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return AcceptedTaskResponse{}, err
	}
	return s.createAcceptedTask(ctx, createAcceptedTaskParams{
		TaskType:       taskType,
		ScopeType:      "team",
		ScopeID:        fmt.Sprint(teamID),
		TeamID:         ptrInt64(teamID),
		RequestPayload: requestPayload,
	})
}

type createAcceptedTaskParams struct {
	TaskType       string
	ScopeType      string
	ScopeID        string
	TeamID         *int64
	OwnerAccountID *int64
	RequestPayload map[string]any
}

func (s *TaskService) createAcceptedTask(ctx context.Context, params createAcceptedTaskParams) (AcceptedTaskResponse, error) {
	if s.jobs == nil {
		return AcceptedTaskResponse{}, errors.New("team task runtime not configured")
	}
	payloadBytes, err := json.Marshal(params.RequestPayload)
	if err != nil {
		return AcceptedTaskResponse{}, fmt.Errorf("marshal team task payload: %w", err)
	}

	job, err := s.jobs.CreateJob(ctx, jobs.CreateJobParams{
		JobType:   "team_" + params.TaskType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Payload:   payloadBytes,
	})
	if err != nil {
		return AcceptedTaskResponse{}, err
	}

	scopeKey := fmt.Sprintf("%s:%s", params.ScopeType, params.ScopeID)
	createdAt := time.Now().UTC()
	task := TeamTaskRecord{
		TaskUUID:       job.JobID,
		TaskType:       params.TaskType,
		Status:         job.Status,
		TeamID:         params.TeamID,
		OwnerAccountID: params.OwnerAccountID,
		ScopeType:      params.ScopeType,
		ScopeID:        params.ScopeID,
		ActiveScopeKey: &scopeKey,
		RequestPayload: cloneMap(params.RequestPayload),
		ResultPayload:  nil,
		Logs:           "",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if _, err := s.repository.CreateTask(ctx, task); err != nil {
		if errors.Is(err, ErrActiveScopeConflict) {
			return AcceptedTaskResponse{}, &ConflictError{ScopeKey: scopeKey}
		}
		return AcceptedTaskResponse{}, err
	}
	return AcceptedTaskResponse{
		Success:        true,
		TaskUUID:       job.JobID,
		TaskType:       params.TaskType,
		Status:         job.Status,
		WSChannel:      buildTaskWSPath(job.JobID),
		ScopeType:      params.ScopeType,
		ScopeID:        params.ScopeID,
		TeamID:         params.TeamID,
		OwnerAccountID: params.OwnerAccountID,
	}, nil
}

func (s *TaskService) acceptedResponseForTask(ctx context.Context, task TeamTaskRecord) AcceptedTaskResponse {
	status := task.Status
	if s.jobs != nil {
		if job, err := s.jobs.GetJob(ctx, task.TaskUUID); err == nil {
			status = job.Status
		}
	}
	return AcceptedTaskResponse{
		Success:        true,
		TaskUUID:       task.TaskUUID,
		TaskType:       task.TaskType,
		Status:         status,
		WSChannel:      buildTaskWSPath(task.TaskUUID),
		ScopeType:      task.ScopeType,
		ScopeID:        task.ScopeID,
		TeamID:         task.TeamID,
		OwnerAccountID: task.OwnerAccountID,
	}
}

func buildTaskWSPath(taskUUID string) string {
	return "/api/ws/task/" + taskUUID
}

func isFinishedTaskStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case jobs.StatusCompleted, jobs.StatusFailed, jobs.StatusCancelled:
		return true
	default:
		return false
	}
}

func normalizeUniqueInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := map[int64]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeUniqueEmails(values []string) []string {
	result := make([]string, 0, len(values))
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
		result = append(result, email)
	}
	return result
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func appendLogs(existing string, lines []string) string {
	merged := splitLogs(existing)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		merged = append(merged, line)
	}
	return strings.Join(merged, "\n")
}
