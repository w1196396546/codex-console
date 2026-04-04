package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/hibiken/asynq"
)

func TestJobsFlow(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &capturingQueue{}
	service := jobs.NewService(repo, queue)
	server := httptest.NewServer(internalhttp.NewRouter(service))
	defer server.Close()

	createResp := createJobThroughAPI(t, server.URL)
	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("expected job_id, got %#v", createResp["job_id"])
	}
	if queue.task == nil {
		t.Fatal("expected API create to enqueue task")
	}

	worker := jobs.NewWorker(service)
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	job := getJobThroughAPI(t, server.URL, jobID)
	status := job["status"]
	if status == nil {
		status = job["Status"]
	}
	if status != jobs.StatusCompleted {
		t.Fatalf("expected completed status, got %#v", status)
	}

	logsResp := getLogsThroughAPI(t, server.URL, jobID)
	items, ok := logsResp["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %#v", logsResp["items"])
	}
	if len(items) < 2 {
		t.Fatalf("expected at least two log items, got %#v", items)
	}
}

func TestRegistrationCompatibilityFlow(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &capturingQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistrationThroughCompatAPI(t, server.URL)

	task := getRegistrationTaskThroughCompatAPI(t, server.URL, taskUUID)
	assertRegistrationCompatTaskFields(t, task, taskUUID, jobs.StatusPending)

	initialLogs := getRegistrationLogsThroughCompatAPI(t, server.URL, taskUUID, 0)
	assertRegistrationCompatLogFields(t, initialLogs, taskUUID, jobs.StatusPending, 0, 0)
	initialLogItems, ok := initialLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected initial logs array, got %#v", initialLogs["logs"])
	}
	if len(initialLogItems) != 0 {
		t.Fatalf("expected no initial logs, got %#v", initialLogItems)
	}
	worker := jobs.NewWorker(jobService)
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	logs := getRegistrationLogsThroughCompatAPI(t, server.URL, taskUUID, 1)
	assertRegistrationCompatLogFields(t, logs, taskUUID, jobs.StatusCompleted, 1, 2)

	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 1 {
		t.Fatalf("expected one incremental log item, got %#v", logs["logs"])
	}

	clampedLogs := getRegistrationLogsThroughCompatAPI(t, server.URL, taskUUID, 999)
	assertRegistrationCompatLogFields(t, clampedLogs, taskUUID, jobs.StatusCompleted, 2, 2)
	clampedItems, ok := clampedLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected clamped logs array, got %#v", clampedLogs["logs"])
	}
	if len(clampedItems) != 0 {
		t.Fatalf("expected no clamped logs, got %#v", clampedItems)
	}
}

func assertRegistrationCompatTaskFields(t *testing.T, payload map[string]any, taskUUID string, status string) {
	t.Helper()

	if payload["task_uuid"] != taskUUID {
		t.Fatalf("expected task uuid %q, got %#v", taskUUID, payload["task_uuid"])
	}
	if payload["status"] != status {
		t.Fatalf("expected status %q, got %#v", status, payload["status"])
	}
	if value, ok := payload["email"]; !ok {
		t.Fatalf("expected email field, got %#v", payload)
	} else if value != nil {
		t.Fatalf("expected email to be null, got %#v", value)
	}
	if value, ok := payload["email_service"]; !ok {
		t.Fatalf("expected email_service field, got %#v", payload)
	} else if value != nil {
		t.Fatalf("expected email_service to be null, got %#v", value)
	}
}

func assertRegistrationCompatLogFields(
	t *testing.T,
	payload map[string]any,
	taskUUID string,
	status string,
	offset float64,
	nextOffset float64,
) {
	t.Helper()

	assertRegistrationCompatTaskFields(t, payload, taskUUID, status)
	if _, ok := payload["logs"]; !ok {
		t.Fatalf("expected logs field, got %#v", payload)
	}
	if logs, ok := payload["logs"].([]any); ok {
		for _, item := range logs {
			if _, ok := item.(string); !ok {
				t.Fatalf("expected log item to be string, got %#v", item)
			}
		}
	}
	if payload["log_offset"] != offset {
		t.Fatalf("expected log_offset=%v, got %#v", offset, payload["log_offset"])
	}
	if payload["log_next_offset"] != nextOffset {
		t.Fatalf("expected log_next_offset=%v, got %#v", nextOffset, payload["log_next_offset"])
	}
}

func createJobThroughAPI(t *testing.T, baseURL string) map[string]any {
	t.Helper()

	body := []byte(`{"job_type":"team_sync","scope_type":"team","scope_id":"42","payload":{"team_id":42}}`)
	resp, err := http.Post(baseURL+"/api/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create job request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected create 202, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return payload
}

func getJobThroughAPI(t *testing.T, baseURL string, jobID string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/jobs/" + jobID)
	if err != nil {
		t.Fatalf("get job request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get job 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode get job response: %v", err)
	}

	job, ok := payload["job"].(map[string]any)
	if !ok {
		t.Fatalf("expected job object, got %#v", payload["job"])
	}
	return job
}

func getLogsThroughAPI(t *testing.T, baseURL string, jobID string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/jobs/" + jobID + "/logs")
	if err != nil {
		t.Fatalf("get logs request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get logs 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode get logs response: %v", err)
	}
	return payload
}

func startRegistrationThroughCompatAPI(t *testing.T, baseURL string) string {
	t.Helper()

	body := []byte(`{"email_service_type":"tempmail"}`)
	resp, err := http.Post(baseURL+"/api/registration/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("start registration request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected start 202, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode start response: %v", err)
	}

	taskUUID, ok := payload["task_uuid"].(string)
	if !ok || taskUUID == "" {
		t.Fatalf("expected task_uuid, got %#v", payload["task_uuid"])
	}
	return taskUUID
}

func getRegistrationTaskThroughCompatAPI(t *testing.T, baseURL string, taskUUID string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/tasks/" + taskUUID)
	if err != nil {
		t.Fatalf("get registration task request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get registration task 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration task response: %v", err)
	}
	return payload
}

func getRegistrationLogsThroughCompatAPI(t *testing.T, baseURL string, taskUUID string, offset int) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/tasks/" + taskUUID + "/logs?offset=" + strconv.Itoa(offset))
	if err != nil {
		t.Fatalf("get registration logs request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get registration logs 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration logs response: %v", err)
	}
	return payload
}

type capturingQueue struct {
	task *asynq.Task
}

func (q *capturingQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	return nil
}
