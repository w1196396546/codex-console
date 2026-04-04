package http_test

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
)

func TestRegistrationStartAndTaskReadback(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistration(t, server.URL)

	task := getRegistrationTask(t, server.URL, taskUUID)
	if task["task_uuid"] != taskUUID {
		t.Fatalf("expected task uuid %q, got %#v", taskUUID, task["task_uuid"])
	}
	if task["status"] != jobs.StatusPending {
		t.Fatalf("expected pending status, got %#v", task["status"])
	}

	initialLogs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	if initialLogs["status"] != jobs.StatusPending {
		t.Fatalf("expected initial pending status, got %#v", initialLogs["status"])
	}
	initialLogItems, ok := initialLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected initial logs array, got %#v", initialLogs["logs"])
	}
	if len(initialLogItems) != 0 {
		t.Fatalf("expected no initial logs, got %#v", initialLogItems)
	}
	if initialLogs["log_offset"] != float64(0) {
		t.Fatalf("expected initial log_offset=0, got %#v", initialLogs["log_offset"])
	}
	if initialLogs["log_next_offset"] != float64(0) {
		t.Fatalf("expected initial log_next_offset=0, got %#v", initialLogs["log_next_offset"])
	}

	worker := jobs.NewWorker(jobService)
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	if logs["status"] != jobs.StatusCompleted {
		t.Fatalf("expected completed status, got %#v", logs["status"])
	}

	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 2 {
		t.Fatalf("expected two log items, got %#v", logs["logs"])
	}
	if logs["log_offset"] != float64(0) {
		t.Fatalf("expected log_offset=0, got %#v", logs["log_offset"])
	}
	if logs["log_next_offset"] != float64(2) {
		t.Fatalf("expected log_next_offset=2, got %#v", logs["log_next_offset"])
	}
}

func TestRegistrationLogsRespectsOffset(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistration(t, server.URL)

	worker := jobs.NewWorker(jobService)
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	logs := getRegistrationLogs(t, server.URL, taskUUID, 1)
	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 1 {
		t.Fatalf("expected one incremental log item, got %#v", logs["logs"])
	}
	if logs["log_offset"] != float64(1) {
		t.Fatalf("expected log_offset=1, got %#v", logs["log_offset"])
	}
	if logs["log_next_offset"] != float64(2) {
		t.Fatalf("expected log_next_offset=2, got %#v", logs["log_next_offset"])
	}

	clampedLogs := getRegistrationLogs(t, server.URL, taskUUID, 999)
	clampedItems, ok := clampedLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected clamped logs array, got %#v", clampedLogs["logs"])
	}
	if len(clampedItems) != 0 {
		t.Fatalf("expected no logs after clamped offset, got %#v", clampedItems)
	}
	if clampedLogs["log_offset"] != float64(2) {
		t.Fatalf("expected clamped log_offset=2, got %#v", clampedLogs["log_offset"])
	}
	if clampedLogs["log_next_offset"] != float64(2) {
		t.Fatalf("expected clamped log_next_offset=2, got %#v", clampedLogs["log_next_offset"])
	}
}

func startRegistration(t *testing.T, baseURL string) string {
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

func getRegistrationTask(t *testing.T, baseURL string, taskUUID string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/tasks/" + taskUUID)
	if err != nil {
		t.Fatalf("get task request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get task 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	return payload
}

func getRegistrationLogs(t *testing.T, baseURL string, taskUUID string, offset int) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/tasks/" + taskUUID + "/logs?offset=" + strconv.Itoa(offset))
	if err != nil {
		t.Fatalf("get logs request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get logs 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	return payload
}
