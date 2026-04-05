package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
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

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected start 200, got %d", startRec.Code)
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
	if startResp["email_service"] != "tempmail" {
		t.Fatalf("expected start email_service=tempmail, got %#v", startResp["email_service"])
	}
	if startResp["email"] != nil {
		t.Fatalf("expected start email=nil, got %#v", startResp["email"])
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
	if getTaskResp["email_service"] != "tempmail" {
		t.Fatalf("expected email_service=tempmail, got %#v", getTaskResp["email_service"])
	}
	if getTaskResp["email"] != nil {
		t.Fatalf("expected email=nil, got %#v", getTaskResp["email"])
	}

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/registration/tasks?page=1&page_size=20", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list tasks 200, got %d", listRec.Code)
	}

	var listResp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unexpected list tasks response json error: %v", err)
	}
	if listResp["total"] != float64(1) {
		t.Fatalf("expected list total=1, got %#v", listResp["total"])
	}
	listItems, ok := listResp["tasks"].([]any)
	if !ok || len(listItems) != 1 {
		t.Fatalf("expected single task in list, got %#v", listResp["tasks"])
	}
	listTask, ok := listItems[0].(map[string]any)
	if !ok {
		t.Fatalf("expected list task object, got %#v", listItems[0])
	}
	if listTask["task_uuid"] != taskUUID {
		t.Fatalf("expected listed task_uuid %q, got %#v", taskUUID, listTask["task_uuid"])
	}
	if listTask["email_service"] != "tempmail" {
		t.Fatalf("expected listed email_service=tempmail, got %#v", listTask["email_service"])
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
	if logsResp["email_service"] != "tempmail" {
		t.Fatalf("expected logs email_service=tempmail, got %#v", logsResp["email_service"])
	}
	if logsResp["email"] != nil {
		t.Fatalf("expected logs email=nil, got %#v", logsResp["email"])
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

	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, httptest.NewRequest(http.MethodDelete, "/api/registration/tasks/"+taskUUID, nil))
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete task 200, got %d", deleteRec.Code)
	}

	var deleteResp map[string]any
	if err := json.Unmarshal(deleteRec.Body.Bytes(), &deleteResp); err != nil {
		t.Fatalf("unexpected delete task response json error: %v", err)
	}
	if success, ok := deleteResp["success"].(bool); !ok || !success {
		t.Fatalf("expected delete success=true, got %#v", deleteResp["success"])
	}

	getDeletedRec := httptest.NewRecorder()
	router.ServeHTTP(getDeletedRec, httptest.NewRequest(http.MethodGet, "/api/registration/tasks/"+taskUUID, nil))
	if getDeletedRec.Code != http.StatusNotFound {
		t.Fatalf("expected deleted task 404, got %d", getDeletedRec.Code)
	}
}

func TestAvailableServicesEndpointFallbackShape(t *testing.T) {
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
		"imap_mail",
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

func TestAvailableServicesEndpointUsesInjectedData(t *testing.T) {
	router, _, _ := newRegistrationRouterWithAvailableServices(t, fakeAvailableServicesService{
		response: registration.AvailableServicesResponse{
			"tempmail": {
				Available: true,
				Count:     1,
				Services: []map[string]any{
					{"id": nil, "name": "Tempmail.lol", "type": "tempmail"},
				},
			},
			"yyds_mail": {
				Available: true,
				Count:     1,
				Services: []map[string]any{
					{"id": nil, "name": "YYDS Mail", "type": "yyds_mail", "default_domain": "mail.example.com"},
				},
			},
			"outlook": {
				Available: true,
				Count:     1,
				Services: []map[string]any{
					{"id": 101, "name": "Outlook A", "type": "outlook", "has_oauth": true},
				},
			},
			"moe_mail":  {Available: false, Count: 0, Services: []map[string]any{}},
			"temp_mail": {Available: false, Count: 0, Services: []map[string]any{}},
			"duck_mail": {Available: false, Count: 0, Services: []map[string]any{}},
			"luckmail":  {Available: false, Count: 0, Services: []map[string]any{}},
			"freemail":  {Available: false, Count: 0, Services: []map[string]any{}},
			"imap_mail": {Available: false, Count: 0, Services: []map[string]any{}},
		},
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registration/available-services", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected response json error: %v", err)
	}

	outlook, ok := resp["outlook"].(map[string]any)
	if !ok {
		t.Fatalf("expected outlook object, got %#v", resp["outlook"])
	}
	if outlook["available"] != true || outlook["count"] != float64(1) {
		t.Fatalf("expected injected outlook availability, got %#v", outlook)
	}
	services, ok := outlook["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("expected injected outlook services, got %#v", outlook["services"])
	}
	service, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("expected injected outlook service object, got %#v", services[0])
	}
	if service["id"] != float64(101) {
		t.Fatalf("expected injected outlook id=101, got %#v", service["id"])
	}
	if service["has_oauth"] != true {
		t.Fatalf("expected injected outlook has_oauth=true, got %#v", service["has_oauth"])
	}
}

func TestRegistrationStatsEndpoint(t *testing.T) {
	router, _, _ := newRegistrationRouterWithStats(t, fakeRegistrationStatsService{
		response: registration.StatsResponse{
			ByStatus: map[string]int{
				"completed": 3,
				"failed":    1,
			},
			TodayCount:       4,
			TodayTotal:       4,
			TodaySuccess:     3,
			TodayFailed:      1,
			TodaySuccessRate: 75.0,
			TodayByStatus: map[string]int{
				"completed": 3,
				"failed":    1,
			},
		},
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registration/stats", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode stats response: %v", err)
	}

	if resp["today_total"] != float64(4) || resp["today_success"] != float64(3) || resp["today_failed"] != float64(1) {
		t.Fatalf("unexpected today stats: %#v", resp)
	}
	if resp["today_success_rate"] != float64(75) {
		t.Fatalf("expected today_success_rate=75, got %#v", resp["today_success_rate"])
	}
	byStatus, ok := resp["by_status"].(map[string]any)
	if !ok || byStatus["completed"] != float64(3) || byStatus["failed"] != float64(1) {
		t.Fatalf("unexpected by_status payload: %#v", resp["by_status"])
	}
}

func TestBatchEndpoints(t *testing.T) {
	router, repo, queue := newRegistrationRouter(t)

	startReq := httptest.NewRequest(
		http.MethodPost,
		"/api/registration/batch",
		bytes.NewReader([]byte(`{"count":2,"email_service_type":"tempmail"}`)),
	)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	router.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected batch start 200, got %d", startRec.Code)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("expected 2 queued tasks, got %d", len(queue.tasks))
	}

	var startResp map[string]any
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unexpected batch start response json error: %v", err)
	}

	batchID, ok := startResp["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id string, got %#v", startResp["batch_id"])
	}
	if startResp["count"] != float64(2) {
		t.Fatalf("expected count=2, got %#v", startResp["count"])
	}

	tasks, ok := startResp["tasks"].([]any)
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected 2 batch tasks, got %#v", startResp["tasks"])
	}

	firstTask, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first batch task object, got %#v", tasks[0])
	}
	firstTaskUUID, ok := firstTask["task_uuid"].(string)
	if !ok || firstTaskUUID == "" {
		t.Fatalf("expected first task_uuid string, got %#v", firstTask["task_uuid"])
	}

	if err := repo.AppendJobLog(context.Background(), firstTaskUUID, "info", "batch compat log"); err != nil {
		t.Fatalf("append batch log: %v", err)
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(
		getRec,
		httptest.NewRequest(http.MethodGet, "/api/registration/batch/"+batchID+"?log_offset=0", nil),
	)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected batch get 200, got %d", getRec.Code)
	}

	var getResp map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unexpected batch get response json error: %v", err)
	}

	if getResp["batch_id"] != batchID {
		t.Fatalf("expected batch_id %q, got %#v", batchID, getResp["batch_id"])
	}
	if getResp["status"] != "running" {
		t.Fatalf("expected running status, got %#v", getResp["status"])
	}
	if getResp["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", getResp["total"])
	}
	if getResp["completed"] != float64(0) {
		t.Fatalf("expected completed=0, got %#v", getResp["completed"])
	}
	if getResp["success"] != float64(0) {
		t.Fatalf("expected success=0, got %#v", getResp["success"])
	}
	if getResp["failed"] != float64(0) {
		t.Fatalf("expected failed=0, got %#v", getResp["failed"])
	}
	if getResp["skipped"] != float64(0) {
		t.Fatalf("expected skipped=0, got %#v", getResp["skipped"])
	}
	if getResp["current_index"] != float64(0) {
		t.Fatalf("expected current_index=0, got %#v", getResp["current_index"])
	}
	if getResp["paused"] != false {
		t.Fatalf("expected paused=false, got %#v", getResp["paused"])
	}
	if getResp["cancelled"] != false {
		t.Fatalf("expected cancelled=false, got %#v", getResp["cancelled"])
	}
	if getResp["finished"] != false {
		t.Fatalf("expected finished=false, got %#v", getResp["finished"])
	}
	if getResp["progress"] != "0/2" {
		t.Fatalf("expected progress=0/2, got %#v", getResp["progress"])
	}
	if getResp["log_offset"] != float64(0) {
		t.Fatalf("expected log_offset=0, got %#v", getResp["log_offset"])
	}
	if getResp["log_base_index"] != float64(0) {
		t.Fatalf("expected log_base_index=0, got %#v", getResp["log_base_index"])
	}
	if getResp["log_next_offset"] != float64(1) {
		t.Fatalf("expected log_next_offset=1, got %#v", getResp["log_next_offset"])
	}
	logItems, ok := getResp["logs"].([]any)
	if !ok || len(logItems) != 1 || logItems[0] != "batch compat log" {
		t.Fatalf("expected batch logs with one string item, got %#v", getResp["logs"])
	}

	pauseResp := assertBatchControlStatus(t, router, batchID, "pause", "paused")
	if pauseResp["success"] != true {
		t.Fatalf("expected pause success=true, got %#v", pauseResp["success"])
	}

	pausedState := httptest.NewRecorder()
	router.ServeHTTP(
		pausedState,
		httptest.NewRequest(http.MethodGet, "/api/registration/batch/"+batchID+"?log_offset=1", nil),
	)
	if pausedState.Code != http.StatusOK {
		t.Fatalf("expected paused batch get 200, got %d", pausedState.Code)
	}
	var pausedResp map[string]any
	if err := json.Unmarshal(pausedState.Body.Bytes(), &pausedResp); err != nil {
		t.Fatalf("unexpected paused batch response json error: %v", err)
	}
	if pausedResp["status"] != "paused" || pausedResp["paused"] != true {
		t.Fatalf("expected paused batch state, got %#v", pausedResp)
	}

	resumeResp := assertBatchControlStatus(t, router, batchID, "resume", "running")
	if resumeResp["success"] != true {
		t.Fatalf("expected resume success=true, got %#v", resumeResp["success"])
	}

	cancelResp := assertBatchControlStatus(t, router, batchID, "cancel", "cancelling")
	if cancelResp["success"] != true {
		t.Fatalf("expected cancel success=true, got %#v", cancelResp["success"])
	}

	cancelledState := httptest.NewRecorder()
	router.ServeHTTP(
		cancelledState,
		httptest.NewRequest(http.MethodGet, "/api/registration/batch/"+batchID+"?log_offset=1", nil),
	)
	if cancelledState.Code != http.StatusOK {
		t.Fatalf("expected cancelled batch get 200, got %d", cancelledState.Code)
	}
	var cancelledResp map[string]any
	if err := json.Unmarshal(cancelledState.Body.Bytes(), &cancelledResp); err != nil {
		t.Fatalf("unexpected cancelled batch response json error: %v", err)
	}
	if cancelledResp["status"] != "cancelling" {
		t.Fatalf("expected first cancelled batch state to be cancelling, got %#v", cancelledResp["status"])
	}
	if cancelledResp["cancelled"] != true {
		t.Fatalf("expected cancelled=true, got %#v", cancelledResp["cancelled"])
	}
	if cancelledResp["finished"] != false {
		t.Fatalf("expected finished=false during cancelling, got %#v", cancelledResp["finished"])
	}

	settledState := httptest.NewRecorder()
	router.ServeHTTP(
		settledState,
		httptest.NewRequest(http.MethodGet, "/api/registration/batch/"+batchID+"?log_offset=1", nil),
	)
	if settledState.Code != http.StatusOK {
		t.Fatalf("expected settled batch get 200, got %d", settledState.Code)
	}

	var settledResp map[string]any
	if err := json.Unmarshal(settledState.Body.Bytes(), &settledResp); err != nil {
		t.Fatalf("unexpected settled batch response json error: %v", err)
	}
	if settledResp["status"] != jobs.StatusCancelled {
		t.Fatalf("expected settled cancelled status, got %#v", settledResp["status"])
	}
	if settledResp["finished"] != true {
		t.Fatalf("expected finished=true after cancellation settles, got %#v", settledResp["finished"])
	}
}

func TestBatchEndpointsRejectInvalidTransitions(t *testing.T) {
	router, repo, _ := newRegistrationRouter(t)

	startReq := httptest.NewRequest(
		http.MethodPost,
		"/api/registration/batch",
		bytes.NewReader([]byte(`{"count":1,"email_service_type":"tempmail"}`)),
	)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	router.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected batch start 200, got %d", startRec.Code)
	}

	var startResp map[string]any
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unexpected batch start response json error: %v", err)
	}

	batchID, ok := startResp["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id string, got %#v", startResp["batch_id"])
	}
	tasks, ok := startResp["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected one batch task, got %#v", startResp["tasks"])
	}
	task, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected batch task object, got %#v", tasks[0])
	}
	taskUUID, ok := task["task_uuid"].(string)
	if !ok || taskUUID == "" {
		t.Fatalf("expected task_uuid string, got %#v", task["task_uuid"])
	}

	resumeRec := httptest.NewRecorder()
	router.ServeHTTP(resumeRec, httptest.NewRequest(http.MethodPost, "/api/registration/batch/"+batchID+"/resume", nil))
	if resumeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected resume before pause 400, got %d", resumeRec.Code)
	}

	assertBatchControlStatus(t, router, batchID, "pause", "paused")

	secondPauseRec := httptest.NewRecorder()
	router.ServeHTTP(secondPauseRec, httptest.NewRequest(http.MethodPost, "/api/registration/batch/"+batchID+"/pause", nil))
	if secondPauseRec.Code != http.StatusBadRequest {
		t.Fatalf("expected second pause 400, got %d", secondPauseRec.Code)
	}

	assertBatchControlStatus(t, router, batchID, "resume", "running")

	if _, err := repo.UpdateJobStatus(context.Background(), taskUUID, jobs.StatusCompleted); err != nil {
		t.Fatalf("mark task completed: %v", err)
	}

	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, httptest.NewRequest(http.MethodPost, "/api/registration/batch/"+batchID+"/cancel", nil))
	if cancelRec.Code != http.StatusBadRequest {
		t.Fatalf("expected cancel finished batch 400, got %d", cancelRec.Code)
	}

	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/registration/batch/"+batchID+"?log_offset=0", nil))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected batch get 200, got %d", statusRec.Code)
	}

	var statusResp map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("unexpected batch status response json error: %v", err)
	}
	if statusResp["status"] != jobs.StatusCompleted {
		t.Fatalf("expected completed status after rejected cancel, got %#v", statusResp["status"])
	}
	if statusResp["cancelled"] != false {
		t.Fatalf("expected cancelled=false after rejected cancel, got %#v", statusResp["cancelled"])
	}
}

func TestBatchStartRejectsInvalidSchedulingOptions(t *testing.T) {
	router, _, _ := newRegistrationRouter(t)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/registration/batch",
		bytes.NewReader([]byte(`{"count":1,"email_service_type":"tempmail","interval_min":10,"interval_max":5,"concurrency":99,"mode":"serial"}`)),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid batch scheduling, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestOutlookAccountsEndpoint(t *testing.T) {
	router, _, _ := newRegistrationRouterWithOutlook(t, fakeOutlookRepository{
		services: []registration.EmailServiceRecord{
			{
				ID:          101,
				ServiceType: "outlook",
				Name:        "Outlook Alpha",
				Config: map[string]any{
					"email":         "alpha@example.com",
					"client_id":     "client-alpha",
					"refresh_token": "oauth-refresh-alpha",
				},
			},
			{
				ID:          102,
				ServiceType: "outlook",
				Name:        "Outlook Beta",
				Config: map[string]any{
					"email": "beta@example.com",
				},
			},
		},
		accounts: []registration.RegisteredAccountRecord{
			{
				ID:           9001,
				Email:        "alpha@example.com",
				RefreshToken: "account-refresh-alpha",
			},
		},
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registration/outlook-accounts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected response json error: %v", err)
	}

	if resp["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", resp["total"])
	}
	if resp["registered_count"] != float64(1) {
		t.Fatalf("expected registered_count=1, got %#v", resp["registered_count"])
	}
	if resp["unregistered_count"] != float64(1) {
		t.Fatalf("expected unregistered_count=1, got %#v", resp["unregistered_count"])
	}

	accounts, ok := resp["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("expected accounts length=2, got %#v", resp["accounts"])
	}

	first, ok := accounts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first account object, got %#v", accounts[0])
	}
	if first["id"] != float64(101) || first["email"] != "alpha@example.com" || first["name"] != "Outlook Alpha" {
		t.Fatalf("unexpected first account payload: %#v", first)
	}
	if first["has_oauth"] != true || first["is_registered"] != true || first["has_refresh_token"] != true {
		t.Fatalf("unexpected first account flags: %#v", first)
	}
	if first["needs_token_refresh"] != false || first["is_registration_complete"] != true {
		t.Fatalf("unexpected first account registration state: %#v", first)
	}
	if first["registered_account_id"] != float64(9001) {
		t.Fatalf("expected first registered_account_id=9001, got %#v", first["registered_account_id"])
	}
}

func TestOutlookBatchEndpoints(t *testing.T) {
	router, repo, queue := newRegistrationRouterWithOutlook(t, fakeOutlookRepository{})

	startReq := httptest.NewRequest(
		http.MethodPost,
		"/api/registration/outlook-batch",
		bytes.NewReader([]byte(`{"service_ids":[101,202],"proxy":"http://proxy.internal:8080","interval_min":5,"interval_max":30,"concurrency":3,"mode":"pipeline"}`)),
	)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	router.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected outlook batch start 200, got %d", startRec.Code)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("expected 2 queued tasks, got %d", len(queue.tasks))
	}

	var startResp map[string]any
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unexpected start response json error: %v", err)
	}

	batchID, ok := startResp["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id string, got %#v", startResp["batch_id"])
	}
	if startResp["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", startResp["total"])
	}
	if startResp["skipped"] != float64(0) {
		t.Fatalf("expected skipped=0, got %#v", startResp["skipped"])
	}
	if startResp["to_register"] != float64(2) {
		t.Fatalf("expected to_register=2, got %#v", startResp["to_register"])
	}
	serviceIDs, ok := startResp["service_ids"].([]any)
	if !ok || len(serviceIDs) != 2 || serviceIDs[0] != float64(101) || serviceIDs[1] != float64(202) {
		t.Fatalf("expected service_ids [101,202], got %#v", startResp["service_ids"])
	}

	jobIDs := registrationBatchJobIDsByScope(repo, batchID)
	if len(jobIDs) != 2 {
		t.Fatalf("expected 2 jobs for batch %q, got %#v", batchID, jobIDs)
	}
	if err := repo.AppendJobLog(context.Background(), jobIDs[0], "info", "outlook compat log"); err != nil {
		t.Fatalf("append batch log: %v", err)
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(
		getRec,
		httptest.NewRequest(http.MethodGet, "/api/registration/outlook-batch/"+batchID+"?log_offset=0", nil),
	)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected outlook batch get 200, got %d", getRec.Code)
	}

	var getResp map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unexpected get response json error: %v", err)
	}
	if getResp["batch_id"] != batchID {
		t.Fatalf("expected batch_id %q, got %#v", batchID, getResp["batch_id"])
	}
	if getResp["status"] != "running" {
		t.Fatalf("expected status=running, got %#v", getResp["status"])
	}
	if getResp["total"] != float64(2) || getResp["completed"] != float64(0) {
		t.Fatalf("unexpected totals payload: %#v", getResp)
	}
	if getResp["skipped"] != float64(0) {
		t.Fatalf("expected skipped=0, got %#v", getResp["skipped"])
	}
	if getResp["current_index"] != float64(0) {
		t.Fatalf("expected current_index=0, got %#v", getResp["current_index"])
	}
	if getResp["log_base_index"] != float64(0) {
		t.Fatalf("expected log_base_index=0, got %#v", getResp["log_base_index"])
	}
	logs, ok := getResp["logs"].([]any)
	if !ok || len(logs) != 1 || logs[0] != "outlook compat log" {
		t.Fatalf("expected one outlook batch log, got %#v", getResp["logs"])
	}

	pauseResp := assertOutlookBatchControlStatus(t, router, batchID, "pause", "paused")
	if pauseResp["success"] != true {
		t.Fatalf("expected pause success=true, got %#v", pauseResp["success"])
	}

	pausedRec := httptest.NewRecorder()
	router.ServeHTTP(
		pausedRec,
		httptest.NewRequest(http.MethodGet, "/api/registration/outlook-batch/"+batchID+"?log_offset=1", nil),
	)
	if pausedRec.Code != http.StatusOK {
		t.Fatalf("expected paused outlook batch get 200, got %d", pausedRec.Code)
	}

	var pausedResp map[string]any
	if err := json.Unmarshal(pausedRec.Body.Bytes(), &pausedResp); err != nil {
		t.Fatalf("unexpected paused response json error: %v", err)
	}
	if pausedResp["status"] != "paused" || pausedResp["paused"] != true {
		t.Fatalf("expected paused outlook batch state, got %#v", pausedResp)
	}

	resumeResp := assertOutlookBatchControlStatus(t, router, batchID, "resume", "running")
	if resumeResp["success"] != true {
		t.Fatalf("expected resume success=true, got %#v", resumeResp["success"])
	}

	cancelResp := assertOutlookBatchControlStatus(t, router, batchID, "cancel", "cancelling")
	if cancelResp["success"] != true {
		t.Fatalf("expected cancel success=true, got %#v", cancelResp["success"])
	}

	cancelledRec := httptest.NewRecorder()
	router.ServeHTTP(
		cancelledRec,
		httptest.NewRequest(http.MethodGet, "/api/registration/outlook-batch/"+batchID+"?log_offset=1", nil),
	)
	if cancelledRec.Code != http.StatusOK {
		t.Fatalf("expected cancelled outlook batch get 200, got %d", cancelledRec.Code)
	}

	var cancelledResp map[string]any
	if err := json.Unmarshal(cancelledRec.Body.Bytes(), &cancelledResp); err != nil {
		t.Fatalf("unexpected cancelled response json error: %v", err)
	}
	if cancelledResp["status"] != "cancelling" || cancelledResp["cancelled"] != true || cancelledResp["finished"] != false {
		t.Fatalf("expected cancelling outlook batch state, got %#v", cancelledResp)
	}

	settledRec := httptest.NewRecorder()
	router.ServeHTTP(
		settledRec,
		httptest.NewRequest(http.MethodGet, "/api/registration/outlook-batch/"+batchID+"?log_offset=1", nil),
	)
	if settledRec.Code != http.StatusOK {
		t.Fatalf("expected settled outlook batch get 200, got %d", settledRec.Code)
	}

	var settledResp map[string]any
	if err := json.Unmarshal(settledRec.Body.Bytes(), &settledResp); err != nil {
		t.Fatalf("unexpected settled outlook response json error: %v", err)
	}
	if settledResp["status"] != jobs.StatusCancelled || settledResp["cancelled"] != true || settledResp["finished"] != true {
		t.Fatalf("expected settled cancelled finished outlook batch state, got %#v", settledResp)
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

func assertBatchControlStatus(t *testing.T, router http.Handler, batchID string, action string, wantStatus string) map[string]any {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/registration/batch/"+batchID+"/"+action, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s 200, got %d", action, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected %s response json error: %v", action, err)
	}
	if resp["status"] != wantStatus {
		t.Fatalf("expected %s status %q, got %#v", action, wantStatus, resp["status"])
	}
	return resp
}

func assertOutlookBatchControlStatus(t *testing.T, router http.Handler, batchID string, action string, wantStatus string) map[string]any {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/registration/outlook-batch/"+batchID+"/"+action, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected outlook %s 200, got %d", action, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected outlook %s response json error: %v", action, err)
	}
	if resp["status"] != wantStatus {
		t.Fatalf("expected outlook %s status %q, got %#v", action, wantStatus, resp["status"])
	}
	return resp
}

func newRegistrationRouter(t *testing.T) (http.Handler, *registrationTestRepository, *fakeQueue) {
	t.Helper()

	repo := newRegistrationTestRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	return internalhttp.NewRouter(jobService, registrationService), repo, queue
}

func newRegistrationRouterWithAvailableServices(
	t *testing.T,
	availableServices fakeAvailableServicesService,
) (http.Handler, *registrationTestRepository, *fakeQueue) {
	t.Helper()

	repo := newRegistrationTestRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	return internalhttp.NewRouter(jobService, registrationService, availableServices), repo, queue
}

func newRegistrationRouterWithOutlook(
	t *testing.T,
	outlookRepo fakeOutlookRepository,
) (http.Handler, *registrationTestRepository, *fakeQueue) {
	t.Helper()

	repo := newRegistrationTestRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	batchService := registration.NewBatchService(jobService)
	outlookService := registration.NewOutlookService(outlookRepo, batchService)

	return internalhttp.NewRouter(jobService, registrationService, batchService, outlookService), repo, queue
}

func newRegistrationRouterWithStats(
	t *testing.T,
	statsService fakeRegistrationStatsService,
) (http.Handler, *registrationTestRepository, *fakeQueue) {
	t.Helper()

	repo := newRegistrationTestRepository()
	queue := &fakeQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	return internalhttp.NewRouter(jobService, registrationService, statsService), repo, queue
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

func (r *registrationTestRepository) ListJobs(_ context.Context, params jobs.ListJobsParams) (jobs.ListJobsResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	scopeTypes := make(map[string]struct{}, len(params.ScopeTypes))
	for _, scopeType := range params.ScopeTypes {
		if scopeType == "" {
			continue
		}
		scopeTypes[scopeType] = struct{}{}
	}

	items := make([]jobs.Job, 0, len(r.jobs))
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
		items = append(items, cloneRegistrationJob(job))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].JobID > items[j].JobID
	})

	total := len(items)
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	end := offset + limit
	if end > total {
		end = total
	}

	page := make([]jobs.Job, 0, end-offset)
	for _, job := range items[offset:end] {
		page = append(page, cloneRegistrationJob(job))
	}

	return jobs.ListJobsResult{
		Total: total,
		Jobs:  page,
	}, nil
}

func (r *registrationTestRepository) DeleteJob(_ context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[jobID]; !ok {
		return jobs.ErrJobNotFound
	}
	delete(r.jobs, jobID)
	delete(r.logs, jobID)
	return nil
}

func registrationBatchJobIDsByScope(repo *registrationTestRepository, batchID string) []string {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	jobIDs := make([]string, 0)
	for _, job := range repo.jobs {
		if job.ScopeType == "registration_batch" && job.ScopeID == batchID {
			jobIDs = append(jobIDs, job.JobID)
		}
	}
	return jobIDs
}

func cloneRegistrationJob(job jobs.Job) jobs.Job {
	job.Payload = append([]byte(nil), job.Payload...)
	return job
}

type fakeQueue struct {
	task  *asynq.Task
	tasks []*asynq.Task
}

func (q *fakeQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	q.tasks = append(q.tasks, task)
	return nil
}

type fakeAvailableServicesService struct {
	response registration.AvailableServicesResponse
	err      error
}

func (f fakeAvailableServicesService) ListAvailableServices(_ context.Context) (registration.AvailableServicesResponse, error) {
	return f.response, f.err
}

type fakeOutlookRepository struct {
	services []registration.EmailServiceRecord
	accounts []registration.RegisteredAccountRecord
}

func (f fakeOutlookRepository) ListOutlookServices(_ context.Context) ([]registration.EmailServiceRecord, error) {
	return append([]registration.EmailServiceRecord(nil), f.services...), nil
}

func (f fakeOutlookRepository) ListAccountsByEmails(_ context.Context, _ []string) ([]registration.RegisteredAccountRecord, error) {
	return append([]registration.RegisteredAccountRecord(nil), f.accounts...), nil
}

type fakeRegistrationStatsService struct {
	response registration.StatsResponse
	err      error
}

func (f fakeRegistrationStatsService) GetStats(_ context.Context) (registration.StatsResponse, error) {
	return f.response, f.err
}
