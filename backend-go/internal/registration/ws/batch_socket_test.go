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
	assertBatchStatusMessage(t, statusMessage, startResp.BatchID, jobs.StatusRunning, 2, 0, 0, 0, false, false, false)

	historyLog := conn.readJSON(t)
	assertMessageField(t, historyLog, "type", "log")
	assertMessageField(t, historyLog, "batch_id", startResp.BatchID)
	assertMessageField(t, historyLog, "message", "history log")

	conn.writeJSON(t, map[string]any{"type": "ping"})
	pong := conn.readJSON(t)
	assertMessageField(t, pong, "type", "pong")

	conn.writeJSON(t, map[string]any{"type": "pause"})
	paused := conn.readJSON(t)
	assertBatchStatusMessage(t, paused, startResp.BatchID, jobs.StatusPaused, 2, 0, 0, 0, true, false, false)

	conn.writeJSON(t, map[string]any{"type": "resume"})
	resumed := conn.readJSON(t)
	assertBatchStatusMessage(t, resumed, startResp.BatchID, jobs.StatusRunning, 2, 0, 0, 0, false, false, false)

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
				assertBatchStatusMessage(t, message, startResp.BatchID, jobs.StatusRunning, 2, 1, 1, 0, false, false, false)
			}
		case "log":
			if message["message"] == "fresh log" {
				sawFreshLog = true
			}
		}
	}

	conn.writeJSON(t, map[string]any{"type": "cancel"})
	cancelling := conn.readJSON(t)
	assertBatchStatusMessage(t, cancelling, startResp.BatchID, "cancelling", 2, 2, 1, 0, false, true, false)

	deadline = time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("expected final cancelled status")
		}

		message := conn.readJSON(t)
		if message["type"] == "status" && message["status"] == jobs.StatusCancelled {
			assertBatchStatusMessage(t, message, startResp.BatchID, jobs.StatusCancelled, 2, 2, 1, 0, false, true, true)
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
	paused bool,
	cancelled bool,
	finished bool,
) {
	t.Helper()

	assertMessageField(t, payload, "type", "status")
	assertMessageField(t, payload, "batch_id", batchID)
	assertMessageField(t, payload, "status", status)
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
}

type batchSocketQueue struct{}

func (q *batchSocketQueue) Enqueue(_ context.Context, _ *asynq.Task) error {
	return nil
}
