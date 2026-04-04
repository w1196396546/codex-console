package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/hibiken/asynq"
)

func TestRegistrationCompatibleEndpoints(t *testing.T) {
	router, repo, queue := newRegistrationRouter(t)

	startReq := httptest.NewRequest(
		http.MethodPost,
		"/api/registration/start",
		bytes.NewReader([]byte(`{"email_service_type":"tempmail"}`)),
	)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	router.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("expected start 202, got %d", startRec.Code)
	}
	if queue.task == nil {
		t.Fatal("expected start to enqueue a job")
	}

	var startResp map[string]any
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unexpected start response json error: %v", err)
	}

	taskUUID, ok := startResp["task_uuid"].(string)
	if !ok || taskUUID == "" {
		t.Fatalf("expected task_uuid string, got %#v", startResp["task_uuid"])
	}
	if startResp["status"] != jobs.StatusPending {
		t.Fatalf("expected pending status, got %#v", startResp["status"])
	}

	if err := repo.AppendJobLog(context.Background(), taskUUID, "info", "compat log"); err != nil {
		t.Fatalf("append log: %v", err)
	}

	getTaskRec := httptest.NewRecorder()
	router.ServeHTTP(getTaskRec, httptest.NewRequest(http.MethodGet, "/api/registration/tasks/"+taskUUID, nil))

	if getTaskRec.Code != http.StatusOK {
		t.Fatalf("expected get task 200, got %d", getTaskRec.Code)
	}

	var getTaskResp map[string]any
	if err := json.Unmarshal(getTaskRec.Body.Bytes(), &getTaskResp); err != nil {
		t.Fatalf("unexpected task response json error: %v", err)
	}
	if getTaskResp["task_uuid"] != taskUUID {
		t.Fatalf("expected task_uuid %q, got %#v", taskUUID, getTaskResp["task_uuid"])
	}
	if getTaskResp["status"] != jobs.StatusPending {
		t.Fatalf("expected pending status, got %#v", getTaskResp["status"])
	}

	logsRec := httptest.NewRecorder()
	router.ServeHTTP(logsRec, httptest.NewRequest(http.MethodGet, "/api/registration/tasks/"+taskUUID+"/logs", nil))

	if logsRec.Code != http.StatusOK {
		t.Fatalf("expected logs 200, got %d", logsRec.Code)
	}

	var logsResp map[string]any
	if err := json.Unmarshal(logsRec.Body.Bytes(), &logsResp); err != nil {
		t.Fatalf("unexpected logs response json error: %v", err)
	}
	if logsResp["status"] != jobs.StatusPending {
		t.Fatalf("expected pending log status, got %#v", logsResp["status"])
	}
	if logsResp["log_next_offset"] != float64(1) {
		t.Fatalf("expected log_next_offset=1, got %#v", logsResp["log_next_offset"])
	}

	logItems, ok := logsResp["logs"].([]any)
	if !ok || len(logItems) != 1 {
		t.Fatalf("expected one log item, got %#v", logsResp["logs"])
	}

	assertTaskControlStatus(t, router, taskUUID, "pause", jobs.StatusPaused)
	assertTaskControlStatus(t, router, taskUUID, "resume", jobs.StatusPending)
	assertTaskControlStatus(t, router, taskUUID, "cancel", jobs.StatusCancelled)
}

func TestAvailableServicesEndpointMatchesFrontendShape(t *testing.T) {
	router, _, _ := newRegistrationRouter(t)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registration/available-services", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected response json error: %v", err)
	}

	tempmail, ok := resp["tempmail"].(map[string]any)
	if !ok {
		t.Fatalf("expected tempmail object, got %#v", resp["tempmail"])
	}
	if tempmail["available"] != true {
		t.Fatalf("expected tempmail available=true, got %#v", tempmail["available"])
	}
	if tempmail["count"] != float64(1) {
		t.Fatalf("expected tempmail count=1, got %#v", tempmail["count"])
	}
	tempmailServices, ok := tempmail["services"].([]any)
	if !ok || len(tempmailServices) != 1 {
		t.Fatalf("expected tempmail services length=1, got %#v", tempmail["services"])
	}

	for _, serviceType := range []string{
		"yyds_mail",
		"outlook",
		"moe_mail",
		"temp_mail",
		"duck_mail",
		"luckmail",
		"freemail",
	} {
		group, ok := resp[serviceType].(map[string]any)
		if !ok {
			t.Fatalf("expected %s object, got %#v", serviceType, resp[serviceType])
		}
		if group["available"] != false {
			t.Fatalf("expected %s available=false, got %#v", serviceType, group["available"])
		}
		if group["count"] != float64(0) {
			t.Fatalf("expected %s count=0, got %#v", serviceType, group["count"])
		}
		services, ok := group["services"].([]any)
		if !ok || len(services) != 0 {
			t.Fatalf("expected %s services empty, got %#v", serviceType, group["services"])
		}
	}
}

func assertTaskControlStatus(t *testing.T, router http.Handler, taskUUID string, action string, wantStatus string) {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/registration/tasks/"+taskUUID+"/"+action, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s 200, got %d", action, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected %s response json error: %v", action, err)
	}
	if resp["task_uuid"] != taskUUID {
		t.Fatalf("expected %s task_uuid %q, got %#v", action, taskUUID, resp["task_uuid"])
	}
	if resp["status"] != wantStatus {
		t.Fatalf("expected %s status %q, got %#v", action, wantStatus, resp["status"])
	}
}

func newRegistrationRouter(t *testing.T) (http.Handler, *registrationTestRepository, *fakeQueue) {
	t.Helper()

	repo := newRegistrationTestRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	return internalhttp.NewRouter(jobService, registrationService), repo, queue
}

type registrationTestRepository struct {
	mu     sync.RWMutex
	nextID int
	jobs   map[string]jobs.Job
	logs   map[string][]jobs.JobLog
}

func newRegistrationTestRepository() *registrationTestRepository {
	return &registrationTestRepository{
		nextID: 1,
		jobs:   make(map[string]jobs.Job),
		logs:   make(map[string][]jobs.JobLog),
	}
}

func (r *registrationTestRepository) CreateJob(_ context.Context, params jobs.CreateJobParams) (jobs.Job, error) {
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
	r.jobs[jobID] = cloneRegistrationJob(job)
	return cloneRegistrationJob(job), nil
}

func (r *registrationTestRepository) GetJob(_ context.Context, jobID string) (jobs.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	return cloneRegistrationJob(job), nil
}

func (r *registrationTestRepository) UpdateJobStatus(_ context.Context, jobID string, status string) (jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[jobID]
	if !ok {
		return jobs.Job{}, jobs.ErrJobNotFound
	}
	job.Status = status
	r.jobs[jobID] = cloneRegistrationJob(job)
	return cloneRegistrationJob(job), nil
}

func (r *registrationTestRepository) AppendJobLog(_ context.Context, jobID string, _ string, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[jobID]; !ok {
		return jobs.ErrJobNotFound
	}

	r.logs[jobID] = append(r.logs[jobID], jobs.JobLog{Message: message})
	return nil
}

func (r *registrationTestRepository) ListJobLogs(_ context.Context, jobID string) ([]jobs.JobLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.jobs[jobID]; !ok {
		return nil, jobs.ErrJobNotFound
	}

	logs := make([]jobs.JobLog, len(r.logs[jobID]))
	copy(logs, r.logs[jobID])
	return logs, nil
}

func cloneRegistrationJob(job jobs.Job) jobs.Job {
	job.Payload = append([]byte(nil), job.Payload...)
	return job
}

type fakeQueue struct {
	task *asynq.Task
}

func (q *fakeQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	return nil
}
