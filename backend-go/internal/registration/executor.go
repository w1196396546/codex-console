package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

const JobTypeSingle = "registration_single"
const runnerAccountPersistenceResultKey = "__registration_account_persistence"

type runnerControlState string

const (
	runnerControlStateRunning   runnerControlState = "running"
	runnerControlStatePaused    runnerControlState = "paused"
	runnerControlStateCancelled runnerControlState = "cancelled"
)

type runnerControlFunc func(ctx context.Context) (runnerControlState, error)

type RunnerRequest struct {
	TaskUUID             string
	StartRequest         StartRequest
	Plan                 ExecutionPlan
	GoPersistenceEnabled bool
	control              runnerControlFunc
}

type Runner interface {
	Run(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (map[string]any, error)
}

type logAppender interface {
	AppendLog(ctx context.Context, jobID string, level string, message string) error
}

type accountUpserter interface {
	UpsertAccount(ctx context.Context, req accounts.UpsertAccountRequest) (accounts.Account, error)
}

type Executor struct {
	logs               logAppender
	runner             Runner
	admission          batchAdmissionController
	preparer           requestPreparer
	accountPersistence accountUpserter
	autoUpload         AutoUploadDispatcher
}

type ExecutorOption func(*Executor)

func WithPreparationDependencies(deps PreparationDependencies) ExecutorOption {
	return func(executor *Executor) {
		if executor == nil {
			return
		}
		executor.preparer = newOrchestrator(deps)
	}
}

func WithAccountPersistence(upserter accountUpserter) ExecutorOption {
	return func(executor *Executor) {
		if executor == nil {
			return
		}
		executor.accountPersistence = upserter
	}
}

func WithAutoUploadDispatcher(dispatcher AutoUploadDispatcher) ExecutorOption {
	return func(executor *Executor) {
		if executor == nil {
			return
		}
		executor.autoUpload = dispatcher
	}
}

func NewExecutor(logs logAppender, runner Runner, options ...ExecutorOption) *Executor {
	var jobReader batchAdmissionJobReader
	if reader, ok := logs.(batchAdmissionJobReader); ok {
		jobReader = reader
	}

	executor := &Executor{
		logs:      logs,
		runner:    runner,
		admission: newProcessLocalBatchAdmissionController(jobReader),
		preparer:  newOrchestrator(PreparationDependencies{}),
	}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}
	return executor
}

func (e *Executor) Execute(ctx context.Context, job jobs.Job) (map[string]any, error) {
	if e == nil {
		return nil, errors.New("registration executor is required")
	}
	if strings.TrimSpace(job.JobID) == "" {
		return nil, errors.New("job_id is required")
	}
	if job.JobType != JobTypeSingle {
		return nil, fmt.Errorf("unsupported registration job type: %s", job.JobType)
	}
	if e.runner == nil {
		return nil, errors.New("registration runner is required")
	}
	if e.preparer == nil {
		return nil, errors.New("registration preparer is required")
	}

	var req StartRequest
	if err := json.Unmarshal(job.Payload, &req); err != nil {
		return nil, fmt.Errorf("decode registration payload: %w", err)
	}
	if strings.TrimSpace(req.EmailServiceType) == "" {
		req.EmailServiceType = "tempmail"
	}

	logf := func(level string, message string) error {
		if e.logs == nil || strings.TrimSpace(message) == "" {
			return nil
		}
		normalizedLevel := strings.TrimSpace(level)
		if normalizedLevel == "" {
			normalizedLevel = "info"
		}
		return e.logs.AppendLog(ctx, job.JobID, normalizedLevel, strings.TrimSpace(message))
	}

	if req.IntervalMin > 0 || req.IntervalMax > 0 || req.Concurrency > 0 || strings.TrimSpace(req.Mode) != "" {
		if job.ScopeType != "registration_batch" || strings.TrimSpace(job.ScopeID) == "" {
			if err := logf("info", "batch scheduling fields detected; single registration executor ignores interval/concurrency/mode"); err != nil {
				return nil, err
			}
		}
	}

	releaseAdmission, err := e.admission.Acquire(ctx, job, req)
	if err != nil {
		return nil, fmt.Errorf("admit batch registration job: %w", err)
	}
	defer releaseAdmission()

	prepared, err := e.preparer.Prepare(ctx, job.JobID, req)
	if err != nil {
		return nil, fmt.Errorf("prepare registration flow: %w", err)
	}

	result, err := e.runner.Run(ctx, RunnerRequest{
		TaskUUID:             job.JobID,
		StartRequest:         prepared.Request,
		Plan:                 prepared.Plan,
		GoPersistenceEnabled: e.accountPersistence != nil,
		control:              e.runnerControl(job.JobID),
	}, logf)
	if err != nil {
		return nil, fmt.Errorf("run registration flow: %w", err)
	}
	if result == nil {
		return nil, errors.New("registration runner returned empty result")
	}
	persistenceReq, hasPersistence, err := extractAccountPersistenceRequest(result)
	if err != nil {
		return nil, fmt.Errorf("decode runner account persistence payload: %w", err)
	}
	if hasPersistence && e.accountPersistence != nil {
		savedAccount, err := e.accountPersistence.UpsertAccount(ctx, persistenceReq)
		if err != nil {
			return nil, fmt.Errorf("persist account via go account service: %w", err)
		}
		if e.autoUpload != nil {
			dispatchResult, dispatchErr := e.autoUpload.Dispatch(ctx, AutoUploadDispatchRequest{
				JobID:        job.JobID,
				StartRequest: prepared.Request,
				Account:      savedAccount,
			}, logf)
			if dispatchErr != nil {
				_ = logf("warning", fmt.Sprintf("auto upload dispatch failed: %v", dispatchErr))
			} else if e.accountPersistence != nil && strings.TrimSpace(dispatchResult.AccountUpdate.Email) != "" {
				if _, err := e.accountPersistence.UpsertAccount(ctx, dispatchResult.AccountUpdate); err != nil {
					_ = logf("warning", fmt.Sprintf("persist auto upload status failed: %v", err))
				}
			}
		}
	}

	return result, nil
}

func (e *Executor) runnerControl(jobID string) runnerControlFunc {
	if e == nil {
		return nil
	}

	reader, ok := e.logs.(batchAdmissionJobReader)
	if !ok {
		return nil
	}

	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}

	return func(ctx context.Context) (runnerControlState, error) {
		job, err := reader.GetJob(ctx, jobID)
		if err != nil {
			return "", err
		}
		switch strings.TrimSpace(job.Status) {
		case jobs.StatusPaused:
			return runnerControlStatePaused, nil
		case jobs.StatusCancelled:
			return runnerControlStateCancelled, nil
		default:
			return runnerControlStateRunning, nil
		}
	}
}

func extractAccountPersistenceRequest(result map[string]any) (accounts.UpsertAccountRequest, bool, error) {
	if result == nil {
		return accounts.UpsertAccountRequest{}, false, nil
	}

	raw, ok := result[runnerAccountPersistenceResultKey]
	if !ok {
		return accounts.UpsertAccountRequest{}, false, nil
	}
	delete(result, runnerAccountPersistenceResultKey)

	if raw == nil {
		return accounts.UpsertAccountRequest{}, false, nil
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return accounts.UpsertAccountRequest{}, false, err
	}

	var req accounts.UpsertAccountRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return accounts.UpsertAccountRequest{}, false, err
	}

	return req, true, nil
}
