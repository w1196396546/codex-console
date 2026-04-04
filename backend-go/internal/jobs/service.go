package jobs

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrControlNotSupported = errors.New("job control is not supported by repository")
	ErrJobNotFound         = errors.New("job not found")
)

type JobLog struct {
	Message string `json:"message"`
}

type Queue interface{}

type statusRepository interface {
	UpdateJobStatus(ctx context.Context, jobID string, status string) (Job, error)
}

type logsRepository interface {
	ListJobLogs(ctx context.Context, jobID string) ([]JobLog, error)
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

func (s *Service) PauseJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusPaused)
}

func (s *Service) ResumeJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusPending)
}

func (s *Service) CancelJob(ctx context.Context, jobID string) (Job, error) {
	return s.updateJobStatus(ctx, jobID, StatusCancelled)
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
