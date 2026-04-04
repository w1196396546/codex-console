package http_test

import (
	"bytes"
	"encoding/json"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestCreateJob(t *testing.T) {
	router := newTestRouter(t)
	body := []byte(`{"job_type":"team_sync","scope_type":"team","scope_id":"42","payload":{"team_id":42}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected response json error: %v", err)
	}
	if resp["success"] != true {
		t.Fatalf("expected success=true, got %#v", resp["success"])
	}
	if resp["job_id"] == "" {
		t.Fatalf("expected job_id, got %#v", resp["job_id"])
	}
	if resp["status"] != jobs.StatusPending {
		t.Fatalf("expected pending status, got %#v", resp["status"])
	}
}

func TestJobLifecycleEndpoints(t *testing.T) {
	router := newTestRouter(t)
	jobID := createJob(t, router)

	getReq := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get job 200, got %d", getRec.Code)
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/pause", nil)
	pauseRec := httptest.NewRecorder()
	router.ServeHTTP(pauseRec, pauseReq)

	if pauseRec.Code != http.StatusOK {
		t.Fatalf("expected pause 200, got %d", pauseRec.Code)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	router.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("expected resume 200, got %d", resumeRec.Code)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/jobs/"+jobID+"/cancel", nil)
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected cancel 200, got %d", cancelRec.Code)
	}

	logsReq := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/logs", nil)
	logsRec := httptest.NewRecorder()
	router.ServeHTTP(logsRec, logsReq)

	if logsRec.Code != http.StatusOK {
		t.Fatalf("expected logs 200, got %d", logsRec.Code)
	}
}

func TestGetJobReturnsInternalErrorForUnknownRepositoryFailure(t *testing.T) {
	router := internalhttp.NewRouter(jobs.NewService(failingRepository{}, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/job-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func createJob(t *testing.T, router http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/jobs",
		bytes.NewReader([]byte(`{"job_type":"team_sync","scope_type":"team","scope_id":"42","payload":{"team_id":42}}`)),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected create job 202, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected create response json error: %v", err)
	}

	jobID, ok := resp["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("expected created job_id string, got %#v", resp["job_id"])
	}

	return jobID
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	svc := jobs.NewService(newTestRepository(), nil)
	return internalhttp.NewRouter(svc)
}

type testRepository struct {
	mu     sync.RWMutex
	nextID int
	jobs   map[string]jobs.Job
}

func newTestRepository() *testRepository {
	return &testRepository{
		nextID: 1,
		jobs:   make(map[string]jobs.Job),
	}
}

func (r *testRepository) CreateJob(_ context.Context, params jobs.CreateJobParams) (jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobID := "job-" + strconv.Itoa(r.nextID)
	r.nextID++

	job := jobs.Job{
		JobID:     jobID,
		JobType:   params.JobType,
		ScopeType: params.ScopeType,
		ScopeID:   params.ScopeID,
		Status:    jobs.StatusPending,
		Payload:   append([]byte(nil), params.Payload...),
	}
	r.jobs[jobID] = cloneJob(job)
	return cloneJob(job), nil
}

func (r *testRepository) GetJob(_ context.Context, jobID string) (jobs.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	return cloneJob(job), nil
}

func (r *testRepository) UpdateJobStatus(_ context.Context, jobID string, status string) (jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	job.Status = status
	r.jobs[jobID] = cloneJob(job)
	return cloneJob(job), nil
}

func (r *testRepository) ListJobLogs(_ context.Context, jobID string) ([]jobs.JobLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.jobs[jobID]; !ok {
		return nil, jobs.ErrJobNotFound
	}
	return []jobs.JobLog{}, nil
}

type failingRepository struct{}

func (failingRepository) CreateJob(context.Context, jobs.CreateJobParams) (jobs.Job, error) {
	return jobs.Job{}, errors.New("boom")
}

func (failingRepository) GetJob(context.Context, string) (jobs.Job, error) {
	return jobs.Job{}, errors.New("boom")
}

func cloneJob(job jobs.Job) jobs.Job {
	job.Payload = append([]byte(nil), job.Payload...)
	return job
}
