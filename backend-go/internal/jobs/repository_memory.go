package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

func (r *InMemoryRepository) ListJobs(_ context.Context, params ListJobsParams) (ListJobsResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	scopeTypes := make(map[string]struct{}, len(params.ScopeTypes))
	for _, scopeType := range params.ScopeTypes {
		if scopeType == "" {
			continue
		}
		scopeTypes[scopeType] = struct{}{}
	}

	items := make([]Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		if params.JobType != "" && job.JobType != params.JobType {
			continue
		}
		if params.Status != "" && job.Status != params.Status {
			continue
		}
		if len(scopeTypes) > 0 {
			if _, ok := scopeTypes[job.ScopeType]; !ok {
				continue
			}
		}
		items = append(items, cloneStoredJob(job))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].JobID > items[j].JobID
	})

	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	page := make([]Job, 0, end-offset)
	for _, job := range items[offset:end] {
		page = append(page, cloneStoredJob(job))
	}

	return ListJobsResult{
		Total: total,
		Jobs:  page,
	}, nil
}

func (r *InMemoryRepository) DeleteJob(_ context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[jobID]; !ok {
		return ErrJobNotFound
	}
	delete(r.jobs, jobID)
	delete(r.logs, jobID)
	return nil
}

func cloneStoredJob(job Job) Job {
	job.Payload = append([]byte(nil), job.Payload...)
	job.Result = append([]byte(nil), job.Result...)
	return job
}
