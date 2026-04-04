package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
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

type capturingQueue struct {
	task *asynq.Task
}

func (q *capturingQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	return nil
}
