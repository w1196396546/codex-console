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
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusPending, nil, "tempmail")

	listing := listRegistrationTasks(t, server.URL)
	if listing["total"] != float64(1) {
		t.Fatalf("expected task listing total=1, got %#v", listing["total"])
	}
	listItems, ok := listing["tasks"].([]any)
	if !ok || len(listItems) != 1 {
		t.Fatalf("expected one listed task, got %#v", listing["tasks"])
	}
	listTask, ok := listItems[0].(map[string]any)
	if !ok {
		t.Fatalf("expected listed task object, got %#v", listItems[0])
	}
	assertRegistrationTaskFrontendFields(t, listTask, taskUUID, jobs.StatusPending, nil, "tempmail")

	initialLogs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, initialLogs, taskUUID, jobs.StatusPending, nil, "tempmail", 0, 0)
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
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, nil, "tempmail", 0, 2)

	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 2 {
		t.Fatalf("expected two log items, got %#v", logs["logs"])
	}

	deleteRegistrationTask(t, server.URL, taskUUID)

	resp, err := http.Get(server.URL + "/api/registration/tasks/" + taskUUID)
	if err != nil {
		t.Fatalf("get deleted task request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected deleted task 404, got %d", resp.StatusCode)
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
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, nil, "tempmail", 1, 2)
	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 1 {
		t.Fatalf("expected one incremental log item, got %#v", logs["logs"])
	}

	clampedLogs := getRegistrationLogs(t, server.URL, taskUUID, 999)
	assertRegistrationLogFrontendFields(t, clampedLogs, taskUUID, jobs.StatusCompleted, nil, "tempmail", 2, 2)
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
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusPending, nil, "tempmail")

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusPending, nil, "tempmail", 0, 0)
}

func TestRegistrationLogsExposeCompletedEmailFromJobResult(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistration(t, server.URL)

	worker := jobs.NewWorkerWithIDAndExecutor(jobService, "worker-email", jobs.ExecutorFunc(func(_ context.Context, _ jobs.Job) (map[string]any, error) {
		return map[string]any{
			"email": "alice@example.com",
		}, nil
	}))
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	task := getRegistrationTask(t, server.URL, taskUUID)
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusCompleted, ptr("alice@example.com"), "tempmail")

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, ptr("alice@example.com"), "tempmail", 0, 2)
}

func TestRegistrationWorkerUsesRegistrationExecutorRunner(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistrationWithPayload(t, server.URL, []byte(`{
		"email_service_type":"tempmail",
		"email_service_config":{
			"base_url":"https://api.tempmail.example/v2"
		}
	}`))

	worker := jobs.NewWorkerWithIDAndExecutor(
		jobService,
		"worker-runner",
		registration.NewExecutor(jobService, &fakeRegistrationRunner{
			result: map[string]any{
				"email":   "runner@example.com",
				"success": true,
			},
			logs: []runnerLog{
				{level: "info", message: "registration started"},
			},
		}),
	)
	if err := worker.HandleTask(context.Background(), queue.task); err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}

	task := getRegistrationTask(t, server.URL, taskUUID)
	assertRegistrationTaskFrontendFields(t, task, taskUUID, jobs.StatusCompleted, ptr("runner@example.com"), "tempmail")

	logs := getRegistrationLogs(t, server.URL, taskUUID, 0)
	assertRegistrationLogFrontendFields(t, logs, taskUUID, jobs.StatusCompleted, ptr("runner@example.com"), "tempmail", 0, 3)
	logItems, ok := logs["logs"].([]any)
	if !ok {
		t.Fatalf("expected logs array, got %#v", logs["logs"])
	}
	if len(logItems) != 3 {
		t.Fatalf("expected three log items, got %#v", logItems)
	}
	if logItems[1] != "registration started" {
		t.Fatalf("expected runner log in middle, got %#v", logItems)
	}
}

func assertRegistrationTaskFrontendFields(t *testing.T, payload map[string]any, taskUUID string, status string, email *string, emailService string) {
	t.Helper()

	if payload["task_uuid"] != taskUUID {
		t.Fatalf("expected task uuid %q, got %#v", taskUUID, payload["task_uuid"])
	}
	if payload["status"] != status {
		t.Fatalf("expected status %q, got %#v", status, payload["status"])
	}
	if value, ok := payload["email"]; !ok {
		t.Fatalf("expected email field, got %#v", payload)
	} else if email == nil {
		if value != nil {
			t.Fatalf("expected email to be null, got %#v", value)
		}
	} else if value != *email {
		t.Fatalf("expected email %q, got %#v", *email, value)
	}
	if value, ok := payload["email_service"]; !ok {
		t.Fatalf("expected email_service field, got %#v", payload)
	} else if value != emailService {
		t.Fatalf("expected email_service %q, got %#v", emailService, value)
	}
}

func assertRegistrationLogFrontendFields(
	t *testing.T,
	payload map[string]any,
	taskUUID string,
	status string,
	email *string,
	emailService string,
	offset float64,
	nextOffset float64,
) {
	t.Helper()

	assertRegistrationTaskFrontendFields(t, payload, taskUUID, status, email, emailService)
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

func ptr(value string) *string {
	return &value
}

type fakeRegistrationRunner struct {
	result map[string]any
	logs   []runnerLog
	err    error
}

func (f *fakeRegistrationRunner) Run(_ context.Context, _ registration.RunnerRequest, logf func(level string, message string) error) (registration.RunnerOutput, error) {
	for _, entry := range f.logs {
		if err := logf(entry.level, entry.message); err != nil {
			return registration.RunnerOutput{}, err
		}
	}
	if f.err != nil {
		return registration.RunnerOutput{}, f.err
	}
	return registration.RunnerOutput{Result: f.result}, nil
}

type runnerLog struct {
	level   string
	message string
}

func startRegistration(t *testing.T, baseURL string) string {
	t.Helper()

	return startRegistrationWithPayload(t, baseURL, []byte(`{"email_service_type":"tempmail"}`))
}

func startRegistrationWithPayload(t *testing.T, baseURL string, body []byte) string {
	t.Helper()

	resp, err := http.Post(baseURL+"/api/registration/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("start registration request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected start 200, got %d", resp.StatusCode)
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

func listRegistrationTasks(t *testing.T, baseURL string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/tasks?page=1&page_size=20")
	if err != nil {
		t.Fatalf("list tasks request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected list tasks 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list tasks response: %v", err)
	}
	return payload
}

func deleteRegistrationTask(t *testing.T, baseURL string, taskUUID string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/registration/tasks/"+taskUUID, nil)
	if err != nil {
		t.Fatalf("create delete task request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete task request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete task 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode delete task response: %v", err)
	}
	if success, ok := payload["success"].(bool); !ok || !success {
		t.Fatalf("expected delete success=true, got %#v", payload["success"])
	}
}
