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
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusPending)

	initialLogs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, initialLogs, taskUUID, jobs.StatusPending, 0, 0)
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

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, 0, 2)

	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 2 {
		t.Fatalf("expected two log items, got %#v", logs["logs"])
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
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, 1, 2)
	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 1 {
		t.Fatalf("expected one incremental log item, got %#v", logs["logs"])
	}

	clampedLogs := getRegistrationLogs(t, server.URL, taskUUID, 999)
	assertRegistrationLogFrontendFields(t, clampedLogs, taskUUID, jobs.StatusCompleted, 2, 2)
	clampedItems, ok := clampedLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected clamped logs array, got %#v", clampedLogs["logs"])
	}
	if len(clampedItems) != 0 {
		t.Fatalf("expected no logs after clamped offset, got %#v", clampedItems)
	}
}

func TestRegistrationTaskResponseIncludesFrontendFields(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistration(t, server.URL)

	task := getRegistrationTask(t, server.URL, taskUUID)
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusPending)

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusPending, 0, 0)
}

func assertRegistrationTaskFrontendFields(t *testing.T, payload map[string]any, taskUUID string, status string) {
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

func assertRegistrationLogFrontendFields(
	t *testing.T,
	payload map[string]any,
	taskUUID string,
	status string,
	offset float64,
	nextOffset float64,
) {
	t.Helper()

	assertRegistrationTaskFrontendFields(t, payload, taskUUID, status)
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
