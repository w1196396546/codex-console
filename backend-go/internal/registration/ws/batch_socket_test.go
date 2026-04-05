package ws

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

func TestBatchSocketSendsCurrentStatusAndLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := jobs.NewInMemoryRepository()
	queue := &batchSocketQueue{}
	jobService := jobs.NewService(repo, queue)
	batchService := registration.NewBatchService(jobService)

	startResp, err := batchService.StartBatch(ctx, registration.BatchStartRequest{
		Count:            2,
		EmailServiceType: "tempmail",
	})
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}
	if err := repo.AppendJobLog(ctx, startResp.Tasks[1].TaskUUID, "info", "history log"); err != nil {
		t.Fatalf("append history log: %v", err)
	}

	handler := NewBatchHandler(batchService, WithBatchPollInterval(5*time.Millisecond))
	router := chi.NewRouter()
	router.Get("/api/ws/batch/{batch_id}", handler.HandleBatchSocket)

	server := httptest.NewServer(router)
	defer server.Close()

	conn := dialTestWebSocket(t, server.URL+"/api/ws/batch/"+startResp.BatchID)
	defer conn.close(t)

	statusMessage := conn.readJSON(t)
	assertBatchStatusMessage(t, statusMessage, startResp.BatchID, jobs.StatusRunning, 2, 0, 0, 0, 0, 0, false, false, false, 0, 0, 1)

	historyLog := conn.readJSON(t)
	assertMessageField(t, historyLog, "type", "log")
	assertMessageField(t, historyLog, "batch_id", startResp.BatchID)
	assertMessageField(t, historyLog, "message", "history log")
	assertTimestampField(t, historyLog)
	assertNumberField(t, historyLog, "log_offset", 0)
	assertNumberField(t, historyLog, "log_next_offset", 1)

	conn.writeJSON(t, map[string]any{"type": "ping"})
	pong := conn.readJSON(t)
	assertMessageField(t, pong, "type", "pong")

	conn.writeJSON(t, map[string]any{"type": "pause"})
	paused := conn.readJSON(t)
	assertBatchStatusMessage(t, paused, startResp.BatchID, jobs.StatusPaused, 2, 0, 0, 0, 0, 0, true, false, false, 0, 1, 1)
	assertMessageField(t, paused, "message", "批量任务已暂停")

	conn.writeJSON(t, map[string]any{"type": "resume"})
	resumed := conn.readJSON(t)
	assertBatchStatusMessage(t, resumed, startResp.BatchID, jobs.StatusRunning, 2, 0, 0, 0, 0, 0, false, false, false, 0, 1, 1)
	assertMessageField(t, resumed, "message", "批量任务已恢复")

	if _, err := repo.UpdateJobStatus(ctx, startResp.Tasks[0].TaskUUID, jobs.StatusCompleted); err != nil {
		t.Fatalf("mark first task completed: %v", err)
	}
	if err := repo.AppendJobLog(ctx, startResp.Tasks[0].TaskUUID, "info", "fresh log"); err != nil {
		t.Fatalf("append fresh log: %v", err)
	}

	var sawProgressStatus bool
	var sawFreshLog bool
	deadline := time.Now().Add(2 * time.Second)
	for !(sawProgressStatus && sawFreshLog) {
		if time.Now().After(deadline) {
			t.Fatalf("expected progress status and fresh log, got status=%v log=%v", sawProgressStatus, sawFreshLog)
		}

		message := conn.readJSON(t)
		switch message["type"] {
		case "status":
			if message["completed"] == float64(1) {
				sawProgressStatus = true
				assertBatchStatusMessage(t, message, startResp.BatchID, jobs.StatusRunning, 2, 1, 1, 0, 0, 1, false, false, false, 0, 2, 2)
			}
		case "log":
			if message["message"] == "fresh log" {
				sawFreshLog = true
				assertTimestampField(t, message)
				assertNumberField(t, message, "log_offset", 1)
				assertNumberField(t, message, "log_next_offset", 2)
			}
		}
	}

	conn.writeJSON(t, map[string]any{"type": "cancel"})
	cancelling := conn.readJSON(t)
	assertBatchStatusMessage(t, cancelling, startResp.BatchID, "cancelling", 2, 2, 1, 0, 0, 2, false, true, false, 0, 2, 2)
	assertMessageField(t, cancelling, "message", "取消请求已提交，正在让整队缓缓靠边停车")

	deadline = time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("expected final cancelled status")
		}

		message := conn.readJSON(t)
		if message["type"] == "status" && message["status"] == jobs.StatusCancelled {
			assertBatchStatusMessage(t, message, startResp.BatchID, jobs.StatusCancelled, 2, 2, 1, 0, 0, 2, false, true, true, 0, 2, 2)
			break
		}
	}
}

func assertBatchStatusMessage(
	t *testing.T,
	payload map[string]any,
	batchID string,
	status string,
	total float64,
	completed float64,
	success float64,
	failed float64,
	skipped float64,
	currentIndex float64,
	paused bool,
	cancelled bool,
	finished bool,
	logBaseIndex float64,
	logOffset float64,
	logNextOffset float64,
) {
	t.Helper()

	assertMessageField(t, payload, "type", "status")
	assertMessageField(t, payload, "batch_id", batchID)
	assertMessageField(t, payload, "status", status)
	assertTimestampField(t, payload)
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
	if payload["skipped"] != skipped {
		t.Fatalf("expected skipped=%v, got %#v", skipped, payload["skipped"])
	}
	if payload["current_index"] != currentIndex {
		t.Fatalf("expected current_index=%v, got %#v", currentIndex, payload["current_index"])
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
	if payload["log_base_index"] != logBaseIndex {
		t.Fatalf("expected log_base_index=%v, got %#v", logBaseIndex, payload["log_base_index"])
	}
	if payload["log_offset"] != logOffset {
		t.Fatalf("expected log_offset=%v, got %#v", logOffset, payload["log_offset"])
	}
	if payload["log_next_offset"] != logNextOffset {
		t.Fatalf("expected log_next_offset=%v, got %#v", logNextOffset, payload["log_next_offset"])
	}
}

type batchSocketQueue struct{}

func (q *batchSocketQueue) Enqueue(_ context.Context, _ *asynq.Task) error {
	return nil
}
