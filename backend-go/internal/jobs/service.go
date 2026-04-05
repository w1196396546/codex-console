package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hibiken/asynq"
)

var (
	ErrControlNotSupported = errors.New("job control is not supported by repository")
	ErrJobNotFound         = errors.New("job not found")
	ErrQueueNotConfigured  = errors.New("job queue is not configured")
)

type JobLog struct {
	Message string `json:"message"`
}

type Queue interface {
	Enqueue(ctx context.Context, task *asynq.Task) error
}

type statusRepository interface {
	UpdateJobStatus(ctx context.Context, jobID string, status string) (Job, error)
}

type runningRepository interface {
	MarkJobRunning(ctx context.Context, jobID string, workerID string) (Job, error)
}

type completionRepository interface {
	MarkJobCompleted(ctx context.Context, jobID string, result []byte) (Job, error)
}

type failureRepository interface {
	MarkJobFailed(ctx context.Context, jobID string, message string) (Job, error)
}

type logsRepository interface {
	ListJobLogs(ctx context.Context, jobID string) ([]JobLog, error)
}

type listJobsRepository interface {
	ListJobs(ctx context.Context, params ListJobsParams) (ListJobsResult, error)
}

type deleteJobRepository interface {
	DeleteJob(ctx context.Context, jobID string) error
}

type appendLogsRepository interface {
	AppendJobLog(ctx context.Context, jobID string, level string, message string) error
}

type ListJobsParams struct {
	JobType    string
	ScopeTypes []string
	Status     string
	Limit      int
	Offset     int
}

type ListJobsResult struct {
	Total int
	Jobs  []Job
}

type Service struct {
	repository Repository
	queue      Queue
}

func NewService(repository Repository, queue Queue) *Service {
	return &Service{
		repository: repository,
		queue:      queue,
	}
}

func (s *Service) CreateJob(ctx context.Context, params CreateJobParams) (Job, error) {
	if err := validateCreateJobParams(params); err != nil {
		return Job{}, err
	}

	return s.repository.CreateJob(ctx, CreateJobParams{
		JobType:   params.JobType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Payload:   append([]byte(nil), params.Payload...),
	})
}

func (s *Service) GetJob(ctx context.Context, jobID string) (Job, error) {
	return s.repository.GetJob(ctx, jobID)
}

func (s *Service) ListJobs(ctx context.Context, params ListJobsParams) (ListJobsResult, error) {
	if listRepo, ok := s.repository.(listJobsRepository); ok {
		return listRepo.ListJobs(ctx, params)
	}

	return ListJobsResult{}, ErrControlNotSupported
}

func (s *Service) EnqueueJob(ctx context.Context, jobID string) error {
	if s.queue == nil {
		return ErrQueueNotConfigured
	}

	payload, err := MarshalQueuePayload(jobID)
	if err != nil {
		return err
	}

	return s.queue.Enqueue(ctx, asynq.NewTask(TypeGenericJob, payload))
}

func (s *Service) PauseJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusPaused)
}

func (s *Service) ResumeJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusPending)
}

func (s *Service) CancelJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusCancelled)
}

func (s *Service) DeleteJob(ctx context.Context, jobID string) error {
	if deleteRepo, ok := s.repository.(deleteJobRepository); ok {
		return deleteRepo.DeleteJob(ctx, jobID)
	}

	return ErrControlNotSupported
}

func (s *Service) ListJobLogs(ctx context.Context, jobID string) ([]JobLog, error) {
	if logsRepo, ok := s.repository.(logsRepository); ok {
		return logsRepo.ListJobLogs(ctx, jobID)
	}

	if _, err := s.repository.GetJob(ctx, jobID); err != nil {
		return nil, err
	}

	return []JobLog{}, nil
}

func (s *Service) MarkRunning(ctx context.Context, jobID string, workerID string) (Job, error) {
	if runningRepo, ok := s.repository.(runningRepository); ok {
		return runningRepo.MarkJobRunning(ctx, jobID, workerID)
	}

	return s.updateJobStatus(ctx, jobID, StatusRunning)
}

func (s *Service) MarkCompleted(ctx context.Context, jobID string, result map[string]any) (Job, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return Job{}, err
	}

	if completionRepo, ok := s.repository.(completionRepository); ok {
		return completionRepo.MarkJobCompleted(ctx, jobID, payload)
	}

	return s.updateJobStatus(ctx, jobID, StatusCompleted)
}

func (s *Service) AppendLog(ctx context.Context, jobID string, level string, message string) error {
	if appendRepo, ok := s.repository.(appendLogsRepository); ok {
		return appendRepo.AppendJobLog(ctx, jobID, level, message)
	}

	_, err := s.repository.GetJob(ctx, jobID)
	return err
}

func (s *Service) MarkFailed(ctx context.Context, jobID string, message string) (Job, error) {
	if failureRepo, ok := s.repository.(failureRepository); ok {
		return failureRepo.MarkJobFailed(ctx, jobID, message)
	}

	return s.updateJobStatus(ctx, jobID, StatusFailed)
}

func (s *Service) updateJobStatus(ctx context.Context, jobID string, status string) (Job, error) {
	statusRepo, ok := s.repository.(statusRepository)
	if !ok {
		return Job{}, ErrControlNotSupported
	}

	return statusRepo.UpdateJobStatus(ctx, jobID, status)
}

func validateCreateJobParams(params CreateJobParams) error {
	if params.JobType == "" {
		return errors.New("job_type is required")
	}
	if params.ScopeType == "" {
		return errors.New("scope_type is required")
	}
	if params.ScopeID == "" {
		return errors.New("scope_id is required")
	}
	if len(params.Payload) == 0 {
		params.Payload = []byte("{}")
	}
	if !json.Valid(params.Payload) {
		return errors.New("payload must be valid json")
	}

	return nil
}
