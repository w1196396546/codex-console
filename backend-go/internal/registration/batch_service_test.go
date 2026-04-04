package registration_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/hibiken/asynq"
)

func TestStartBatchCreatesBatchAndTasks(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &batchFakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	resp, err := svc.StartBatch(context.Background(), registration.BatchStartRequest{
		Count:            2,
		EmailServiceType: "tempmail",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BatchID == "" {
		t.Fatalf("expected batch id, got %+v", resp)
	}
	if resp.Count != 2 {
		t.Fatalf("expected count=2, got %+v", resp)
	}
	if len(resp.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %+v", resp)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("expected 2 queued tasks, got %d", len(queue.tasks))
	}

	for i, task := range resp.Tasks {
		if task.TaskUUID == "" {
			t.Fatalf("expected task %d uuid, got %+v", i, task)
		}
		if task.Status != jobs.StatusPending {
			t.Fatalf("expected task %d pending, got %+v", i, task)
		}

		job, err := repo.GetJob(context.Background(), task.TaskUUID)
		if err != nil {
			t.Fatalf("expected stored job for task %d: %v", i, err)
		}
		if job.JobType != "registration_single" {
			t.Fatalf("expected registration_single, got %+v", job)
		}
		if job.ScopeType != "registration_batch" {
			t.Fatalf("expected registration_batch scope type, got %+v", job)
		}
		if job.ScopeID != resp.BatchID {
			t.Fatalf("expected scope id %q, got %+v", resp.BatchID, job)
		}

		var payload registration.StartRequest
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatalf("expected valid request payload for task %d: %v", i, err)
		}
		if payload.EmailServiceType != "tempmail" {
			t.Fatalf("expected tempmail payload, got %+v", payload)
		}
	}
}

func TestStartBatchRejectsNonPositiveCount(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &batchFakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	for _, count := range []int{0, -1} {
		t.Run("count="+strconv.Itoa(count), func(t *testing.T) {
			_, err := svc.StartBatch(context.Background(), registration.BatchStartRequest{
				Count:            count,
				EmailServiceType: "tempmail",
			})
			if err == nil {
				t.Fatalf("expected error for count=%d", count)
			}
		})
	}
}

func TestGetBatchReturnsStableIncrementalLogs(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &batchFakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	startResp, err := svc.StartBatch(context.Background(), registration.BatchStartRequest{
		Count:            2,
		EmailServiceType: "tempmail",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	firstTaskUUID := startResp.Tasks[0].TaskUUID
	secondTaskUUID := startResp.Tasks[1].TaskUUID

	if err := repo.AppendJobLog(context.Background(), secondTaskUUID, "info", "task-2-log-1"); err != nil {
		t.Fatalf("append second task log: %v", err)
	}

	firstRead, err := svc.GetBatch(context.Background(), startResp.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected first get error: %v", err)
	}
	if len(firstRead.Logs) != 1 || firstRead.Logs[0] != "task-2-log-1" {
		t.Fatalf("expected first incremental logs [task-2-log-1], got %#v", firstRead.Logs)
	}
	if firstRead.LogNextOffset != 1 {
		t.Fatalf("expected first log_next_offset=1, got %#v", firstRead.LogNextOffset)
	}

	if err := repo.AppendJobLog(context.Background(), firstTaskUUID, "info", "task-1-log-1"); err != nil {
		t.Fatalf("append first task log: %v", err)
	}

	secondRead, err := svc.GetBatch(context.Background(), startResp.BatchID, firstRead.LogNextOffset)
	if err != nil {
		t.Fatalf("unexpected second get error: %v", err)
	}
	if len(secondRead.Logs) != 1 || secondRead.Logs[0] != "task-1-log-1" {
		t.Fatalf("expected only new task-1-log-1, got %#v", secondRead.Logs)
	}
	if secondRead.LogNextOffset != 2 {
		t.Fatalf("expected second log_next_offset=2, got %#v", secondRead.LogNextOffset)
	}
}

type batchFakeQueue struct {
	tasks []*asynq.Task
}

func (q *batchFakeQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.tasks = append(q.tasks, task)
	return nil
}
