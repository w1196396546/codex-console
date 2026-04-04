package registration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

type jobsService interface {
	CreateJob(ctx context.Context, params jobs.CreateJobParams) (jobs.Job, error)
	EnqueueJob(ctx context.Context, jobID string) error
}

type Service struct {
	jobs jobsService
}

func NewService(jobsService jobsService) *Service {
	return &Service{jobs: jobsService}
}

func (s *Service) StartRegistration(ctx context.Context, req StartRequest) (TaskResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return TaskResponse{}, fmt.Errorf("marshal registration request: %w", err)
	}

	job, err := s.jobs.CreateJob(ctx, jobs.CreateJobParams{
		JobType:   "registration_single",
		ScopeType: "registration",
		ScopeID:   "single",
		Payload:   payload,
	})
	if err != nil {
		return TaskResponse{}, fmt.Errorf("create registration job: %w", err)
	}

	if err := s.jobs.EnqueueJob(ctx, job.JobID); err != nil {
		return TaskResponse{}, fmt.Errorf("enqueue registration job: %w", err)
	}

	return TaskResponse{
		TaskUUID: job.JobID,
		Status:   job.Status,
	}, nil
}
