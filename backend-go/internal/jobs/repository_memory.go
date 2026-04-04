package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type InMemoryRepository struct {
	mu     sync.RWMutex
	nextID int
	jobs   map[string]Job
	logs   map[string][]JobLog
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		jobs:   make(map[string]Job),
		logs:   make(map[string][]JobLog),
	}
}

func (r *InMemoryRepository) CreateJob(_ context.Context, params CreateJobParams) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobID := fmt.Sprintf("job-%d", r.nextID)
	r.nextID++

	payload := append([]byte(nil), params.Payload...)
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if !json.Valid(payload) {
		return Job{}, fmt.Errorf("payload must be valid json")
	}

	job := Job{
		JobID:     jobID,
		JobType:   params.JobType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Status:    StatusPending,
		Payload:   payload,
	}
	r.jobs[jobID] = cloneStoredJob(job)
	return cloneStoredJob(job), nil
}

func (r *InMemoryRepository) GetJob(_ context.Context, jobID string) (Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	return cloneStoredJob(job), nil
}

func (r *InMemoryRepository) UpdateJobStatus(_ context.Context, jobID string, status string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	job.Status = status
	r.jobs[jobID] = cloneStoredJob(job)
	return cloneStoredJob(job), nil
}

func (r *InMemoryRepository) MarkJobRunning(ctx context.Context, jobID string, _ string) (Job, error) {
	return r.UpdateJobStatus(ctx, jobID, StatusRunning)
}

func (r *InMemoryRepository) MarkJobCompleted(_ context.Context, jobID string, result []byte) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	job.Status = StatusCompleted
	job.Result = append([]byte(nil), result...)
	r.jobs[jobID] = cloneStoredJob(job)
	return cloneStoredJob(job), nil
}

func (r *InMemoryRepository) MarkJobFailed(_ context.Context, jobID string, _ string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	job.Status = StatusFailed
	r.jobs[jobID] = cloneStoredJob(job)
	return cloneStoredJob(job), nil
}

func (r *InMemoryRepository) AppendJobLog(_ context.Context, jobID string, _ string, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[jobID]; !ok {
		return ErrJobNotFound
	}

	r.logs[jobID] = append(r.logs[jobID], JobLog{Message: message})
	return nil
}

func (r *InMemoryRepository) ListJobLogs(_ context.Context, jobID string) ([]JobLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.jobs[jobID]; !ok {
		return nil, ErrJobNotFound
	}

	logs := make([]JobLog, 0, len(r.logs[jobID]))
	for _, item := range r.logs[jobID] {
		logs = append(logs, item)
	}
	return logs, nil
}

func cloneStoredJob(job Job) Job {
	job.Payload = append([]byte(nil), job.Payload...)
	job.Result = append([]byte(nil), job.Result...)
	return job
}
