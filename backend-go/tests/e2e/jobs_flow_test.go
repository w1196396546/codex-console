package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/gorilla/websocket"
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

	availableServices := getRegistrationAvailableServicesThroughCompatAPI(t, server.URL)
	assertRegistrationCompatAvailableServices(t, availableServices)

	taskUUID := startRegistrationThroughCompatAPI(t, server.URL)
	if queue.task == nil {
		t.Fatal("expected registration start to enqueue a job")
	}

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
	if len(logItems) == 0 {
		t.Fatalf("expected at least one incremental log item, got %#v", logs["logs"])
	}
	for _, item := range logItems {
		if _, ok := item.(string); !ok {
			t.Fatalf("expected log item to be string, got %#v", item)
		}
	}
	logNextOffset, ok := logs["log_next_offset"].(float64)
	if !ok {
		t.Fatalf("expected numeric log_next_offset, got %#v", logs["log_next_offset"])
	}
	if logNextOffset < logs["log_offset"].(float64) {
		t.Fatalf("expected log_next_offset >= log_offset, got %#v", logs)
	}

	clampedLogs := getRegistrationLogsThroughCompatAPI(t, server.URL, taskUUID, 999)
	clampedOffset, ok := clampedLogs["log_offset"].(float64)
	if !ok {
		t.Fatalf("expected numeric clamped log_offset, got %#v", clampedLogs["log_offset"])
	}
	assertRegistrationCompatLogFields(t, clampedLogs, taskUUID, jobs.StatusCompleted, clampedOffset, clampedOffset)
	clampedItems, ok := clampedLogs["logs"].([]any)
	if !ok {
		t.Fatalf("expected clamped logs array, got %#v", clampedLogs["logs"])
	}
	if len(clampedItems) != 0 {
		t.Fatalf("expected no clamped logs, got %#v", clampedItems)
	}
	if clampedOffset < logNextOffset {
		t.Fatalf("expected clamped log_offset >= previous log_next_offset, got %#v", clampedLogs)
	}
}

func TestRegistrationWebSocketCompatibility(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &capturingQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	taskUUID := startRegistrationThroughCompatAPI(t, server.URL)
	if queue.task == nil {
		t.Fatal("expected registration start to enqueue a job")
	}

	conn := dialTestWebSocket(t, server.URL+"/api/ws/task/"+taskUUID)
	defer conn.Close()

	initialStatus := conn.readJSON(t)
	assertWebSocketMessageField(t, initialStatus, "type", "status")
	assertWebSocketMessageField(t, initialStatus, "task_uuid", taskUUID)
	assertWebSocketMessageField(t, initialStatus, "status", jobs.StatusPending)

	worker := jobs.NewWorker(jobService)
	done := make(chan error, 1)
	go func() {
		done <- worker.HandleTask(context.Background(), queue.task)
	}()

	var sawCompletedStatus bool
	var sawLog bool
	deadline := time.Now().Add(2 * time.Second)
	for !(sawCompletedStatus && sawLog) {
		if time.Now().After(deadline) {
			t.Fatalf("expected websocket to deliver both completed status and log, got completed=%v log=%v", sawCompletedStatus, sawLog)
		}

		message := conn.readJSON(t)
		messageType, _ := message["type"].(string)
		switch messageType {
		case "status":
			assertWebSocketMessageField(t, message, "task_uuid", taskUUID)
			if message["status"] == jobs.StatusCompleted {
				sawCompletedStatus = true
			}
		case "log":
			assertWebSocketMessageField(t, message, "task_uuid", taskUUID)
			if _, ok := message["log"].(string); !ok {
				t.Fatalf("expected websocket log message to include string log, got %#v", message["log"])
			}
			sawLog = true
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}
}

func TestRegistrationBatchCompatibilityFlow(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &capturingQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	startResp := startRegistrationBatchThroughCompatAPI(t, server.URL, 2)
	batchID, ok := startResp["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id, got %#v", startResp["batch_id"])
	}
	if startResp["count"] != float64(2) {
		t.Fatalf("expected count=2, got %#v", startResp["count"])
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("expected 2 queued tasks, got %d", len(queue.tasks))
	}

	initial := getRegistrationBatchThroughCompatAPI(t, server.URL, batchID, 0)
	assertRegistrationBatchCompatFields(t, initial, batchID, "running", 2, 0, 0, 0, false, false, false, 0, 0)
	initialLogs, ok := initial["logs"].([]any)
	if !ok {
		t.Fatalf("expected initial batch logs array, got %#v", initial["logs"])
	}
	if len(initialLogs) != 0 {
		t.Fatalf("expected no initial batch logs, got %#v", initialLogs)
	}

	pauseResp := controlRegistrationBatchThroughCompatAPI(t, server.URL, batchID, "pause")
	if pauseResp["status"] != jobs.StatusPaused {
		t.Fatalf("expected pause status paused, got %#v", pauseResp["status"])
	}

	paused := getRegistrationBatchThroughCompatAPI(t, server.URL, batchID, 0)
	assertRegistrationBatchCompatFields(t, paused, batchID, jobs.StatusPaused, 2, 0, 0, 0, true, false, false, 0, 0)

	resumeResp := controlRegistrationBatchThroughCompatAPI(t, server.URL, batchID, "resume")
	if resumeResp["status"] != jobs.StatusRunning {
		t.Fatalf("expected resume status running, got %#v", resumeResp["status"])
	}

	worker := jobs.NewWorker(jobService)
	for _, task := range queue.tasks {
		if err := worker.HandleTask(context.Background(), task); err != nil {
			t.Fatalf("unexpected batch worker error: %v", err)
		}
	}

	completed := getRegistrationBatchThroughCompatAPI(t, server.URL, batchID, 0)
	logItems, ok := completed["logs"].([]any)
	if !ok {
		t.Fatalf("expected completed batch logs array, got %#v", completed["logs"])
	}
	if len(logItems) == 0 {
		t.Fatalf("expected completed batch logs, got %#v", logItems)
	}
	for _, item := range logItems {
		if _, ok := item.(string); !ok {
			t.Fatalf("expected batch log item string, got %#v", item)
		}
	}
	nextOffset, ok := completed["log_next_offset"].(float64)
	if !ok {
		t.Fatalf("expected numeric log_next_offset, got %#v", completed["log_next_offset"])
	}
	assertRegistrationBatchCompatFields(t, completed, batchID, jobs.StatusCompleted, 2, 2, 2, 0, false, false, true, 0, nextOffset)

	clamped := getRegistrationBatchThroughCompatAPI(t, server.URL, batchID, 999)
	assertRegistrationBatchCompatFields(t, clamped, batchID, jobs.StatusCompleted, 2, 2, 2, 0, false, false, true, nextOffset, nextOffset)
	clampedLogs, ok := clamped["logs"].([]any)
	if !ok {
		t.Fatalf("expected clamped batch logs array, got %#v", clamped["logs"])
	}
	if len(clampedLogs) != 0 {
		t.Fatalf("expected no clamped batch logs, got %#v", clampedLogs)
	}

	cancelStart := startRegistrationBatchThroughCompatAPI(t, server.URL, 1)
	cancelBatchID, ok := cancelStart["batch_id"].(string)
	if !ok || cancelBatchID == "" {
		t.Fatalf("expected cancel batch_id, got %#v", cancelStart["batch_id"])
	}
	cancelResp := controlRegistrationBatchThroughCompatAPI(t, server.URL, cancelBatchID, "cancel")
	if success, ok := cancelResp["success"].(bool); !ok || !success {
		t.Fatalf("expected cancel success=true, got %#v", cancelResp["success"])
	}

	cancelled := getRegistrationBatchThroughCompatAPI(t, server.URL, cancelBatchID, 0)
	assertRegistrationBatchCompatFields(t, cancelled, cancelBatchID, jobs.StatusCancelled, 1, 1, 0, 0, false, true, true, 0, 0)
}

func assertRegistrationCompatAvailableServices(t *testing.T, payload map[string]any) {
	t.Helper()

	serviceKeys := []string{
		"tempmail",
		"yyds_mail",
		"outlook",
		"moe_mail",
		"temp_mail",
		"duck_mail",
		"luckmail",
		"freemail",
	}
	for _, key := range serviceKeys {
		service, ok := payload[key].(map[string]any)
		if !ok {
			t.Fatalf("expected %s object, got %#v", key, payload[key])
		}
		if _, ok := service["available"].(bool); !ok {
			t.Fatalf("expected %s available bool, got %#v", key, service["available"])
		}
		if _, ok := service["services"].([]any); !ok {
			t.Fatalf("expected %s services array, got %#v", key, service["services"])
		}
	}

	tempmail, ok := payload["tempmail"].(map[string]any)
	if !ok {
		t.Fatalf("expected tempmail object, got %#v", payload["tempmail"])
	}
	if tempmail["available"] != true {
		t.Fatalf("expected tempmail available=true, got %#v", tempmail["available"])
	}
	if tempmail["count"] != float64(1) {
		t.Fatalf("expected tempmail count=1, got %#v", tempmail["count"])
	}
	if _, ok := tempmail["services"].([]any); !ok {
		t.Fatalf("expected tempmail services array, got %#v", tempmail["services"])
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

func assertRegistrationBatchCompatFields(
	t *testing.T,
	payload map[string]any,
	batchID string,
	status string,
	total float64,
	completed float64,
	success float64,
	failed float64,
	paused bool,
	cancelled bool,
	finished bool,
	offset float64,
	nextOffset float64,
) {
	t.Helper()

	if payload["batch_id"] != batchID {
		t.Fatalf("expected batch_id %q, got %#v", batchID, payload["batch_id"])
	}
	if payload["status"] != status {
		t.Fatalf("expected batch status %q, got %#v", status, payload["status"])
	}
	if payload["total"] != total {
		t.Fatalf("expected total=%v, got %#v", total, payload["total"])
	}
	if payload["completed"] != completed {
		t.Fatalf("expected completed=%v, got %#v", completed, payload["completed"])
	}
	if payload["success"] != success {
		t.Fatalf("expected success=%v, got %#v", success, payload["success"])
	}
	if payload["failed"] != failed {
		t.Fatalf("expected failed=%v, got %#v", failed, payload["failed"])
	}
	if payload["paused"] != paused {
		t.Fatalf("expected paused=%v, got %#v", paused, payload["paused"])
	}
	if payload["cancelled"] != cancelled {
		t.Fatalf("expected cancelled=%v, got %#v", cancelled, payload["cancelled"])
	}
	if payload["finished"] != finished {
		t.Fatalf("expected finished=%v, got %#v", finished, payload["finished"])
	}
	if payload["progress"] != strconv.Itoa(int(completed))+"/"+strconv.Itoa(int(total)) {
		t.Fatalf("expected progress=%d/%d, got %#v", int(completed), int(total), payload["progress"])
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

func startRegistrationBatchThroughCompatAPI(t *testing.T, baseURL string, count int) map[string]any {
	t.Helper()

	body := []byte(`{"count":` + strconv.Itoa(count) + `,"email_service_type":"tempmail"}`)
	resp, err := http.Post(baseURL+"/api/registration/batch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("start batch request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected batch start 202, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch start response: %v", err)
	}
	return payload
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

func getRegistrationAvailableServicesThroughCompatAPI(t *testing.T, baseURL string) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/available-services")
	if err != nil {
		t.Fatalf("get registration available services request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get registration available services 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration available services response: %v", err)
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

func getRegistrationBatchThroughCompatAPI(t *testing.T, baseURL string, batchID string, logOffset int) map[string]any {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/registration/batch/" + batchID + "?log_offset=" + strconv.Itoa(logOffset))
	if err != nil {
		t.Fatalf("get registration batch request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected get registration batch 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registration batch response: %v", err)
	}
	return payload
}

func controlRegistrationBatchThroughCompatAPI(t *testing.T, baseURL string, batchID string, action string) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/registration/batch/"+batchID+"/"+action, nil)
	if err != nil {
		t.Fatalf("new batch %s request failed: %v", action, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("batch %s request failed: %v", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected batch %s 200, got %d", action, resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch %s response: %v", action, err)
	}
	return payload
}

type capturingQueue struct {
	task  *asynq.Task
	tasks []*asynq.Task
}

func (q *capturingQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	q.tasks = append(q.tasks, task)
	return nil
}

func assertWebSocketMessageField(t *testing.T, payload map[string]any, key string, want string) {
	t.Helper()

	got, ok := payload[key].(string)
	if !ok {
		t.Fatalf("expected websocket %s to be string, got %#v", key, payload[key])
	}
	if got != want {
		t.Fatalf("expected websocket %s=%q, got %#v", key, want, payload[key])
	}
}

type testWebSocketConn struct {
	conn *websocket.Conn
}

func (c *testWebSocketConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func dialTestWebSocket(t *testing.T, rawURL string) *testWebSocketConn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(rawURL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		statusCode := 0
		if response != nil {
			statusCode = response.StatusCode
		}
		t.Fatalf("dial websocket: status=%d err=%v", statusCode, err)
	}
	return &testWebSocketConn{conn: conn}
}

func (c *testWebSocketConn) readJSON(t *testing.T) map[string]any {
	t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var message map[string]any
	if err := c.conn.ReadJSON(&message); err != nil {
		t.Fatalf("read websocket json: %v", err)
	}
	return message
}
