package registration_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestStartBatchPreservesCompatibilityFieldsInEachPayload(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &batchFakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	emailServiceID := 77
	resp, err := svc.StartBatch(context.Background(), registration.BatchStartRequest{
		Count:                   2,
		EmailServiceType:        "outlook",
		ChatGPTRegistrationMode: "access_token_only",
		Proxy:                   "http://proxy.internal:8080",
		EmailServiceID:          &emailServiceID,
		EmailServiceConfig:      map[string]any{"domain": "example.com"},
		IntervalMin:             5,
		IntervalMax:             30,
		Concurrency:             4,
		Mode:                    "parallel",
		AutoUploadCPA:           true,
		CPAServiceIDs:           []int{1, 2},
		AutoUploadSub2API:       true,
		Sub2APIServiceIDs:       []int{3},
		AutoUploadTM:            true,
		TMServiceIDs:            []int{4, 5},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, task := range resp.Tasks {
		job, err := repo.GetJob(context.Background(), task.TaskUUID)
		if err != nil {
			t.Fatalf("load stored job %q: %v", task.TaskUUID, err)
		}

		var payload registration.StartRequest
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatalf("decode payload for %q: %v", task.TaskUUID, err)
		}

		if payload.IntervalMin != 5 || payload.IntervalMax != 30 || payload.Concurrency != 4 || payload.Mode != "parallel" {
			t.Fatalf("expected batch scheduling fields preserved, got %+v", payload)
		}
		if payload.ChatGPTRegistrationMode != "access_token_only" {
			t.Fatalf("expected registration mode preserved, got %+v", payload)
		}
		if payload.EmailServiceID == nil || *payload.EmailServiceID != emailServiceID {
			t.Fatalf("expected email_service_id=%d, got %+v", emailServiceID, payload.EmailServiceID)
		}
		if !payload.AutoUploadCPA || !reflect.DeepEqual(payload.CPAServiceIDs, []int{1, 2}) {
			t.Fatalf("expected CPA upload fields preserved, got %+v", payload)
		}
		if !payload.AutoUploadSub2API || !reflect.DeepEqual(payload.Sub2APIServiceIDs, []int{3}) {
			t.Fatalf("expected Sub2API upload fields preserved, got %+v", payload)
		}
		if !payload.AutoUploadTM || !reflect.DeepEqual(payload.TMServiceIDs, []int{4, 5}) {
			t.Fatalf("expected TM upload fields preserved, got %+v", payload)
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

func TestStartBatchRejectsInvalidSchedulingOptions(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &batchFakeQueue{}
	svc := registration.NewBatchService(jobs.NewService(repo, queue))

	tests := []struct {
		name string
		req  registration.BatchStartRequest
		want error
	}{
		{
			name: "negative interval",
			req: registration.BatchStartRequest{
				Count:            1,
				EmailServiceType: "tempmail",
				IntervalMin:      -1,
			},
			want: registration.ErrInvalidBatchInterval,
		},
		{
			name: "interval max less than min",
			req: registration.BatchStartRequest{
				Count:            1,
				EmailServiceType: "tempmail",
				IntervalMin:      10,
				IntervalMax:      5,
			},
			want: registration.ErrInvalidBatchInterval,
		},
		{
			name: "invalid concurrency",
			req: registration.BatchStartRequest{
				Count:            1,
				EmailServiceType: "tempmail",
				Concurrency:      51,
			},
			want: registration.ErrInvalidBatchConcurrency,
		},
		{
			name: "invalid mode",
			req: registration.BatchStartRequest{
				Count:            1,
				EmailServiceType: "tempmail",
				Concurrency:      1,
				Mode:             "serial",
			},
			want: registration.ErrInvalidBatchMode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.StartBatch(context.Background(), test.req)
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
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

func TestBatchEndpointsCompatibilityProgressFields(t *testing.T) {
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

	initial, err := svc.GetBatch(context.Background(), startResp.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected initial get error: %v", err)
	}
	if initial.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %#v", initial.Skipped)
	}
	if initial.CurrentIndex != 0 {
		t.Fatalf("expected current_index=0, got %#v", initial.CurrentIndex)
	}
	if initial.LogBaseIndex != 0 {
		t.Fatalf("expected log_base_index=0, got %#v", initial.LogBaseIndex)
	}

	cancelResp, err := svc.CancelBatch(context.Background(), startResp.BatchID)
	if err != nil {
		t.Fatalf("unexpected cancel error: %v", err)
	}
	if cancelResp.Status != "cancelling" {
		t.Fatalf("expected cancel control status=cancelling, got %#v", cancelResp.Status)
	}

	cancelling, err := svc.GetBatch(context.Background(), startResp.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected cancelling get error: %v", err)
	}
	if cancelling.Status != "cancelling" || !cancelling.Cancelled || cancelling.Finished {
		t.Fatalf("expected cancelling snapshot, got %+v", cancelling)
	}

	settled, err := svc.GetBatch(context.Background(), startResp.BatchID, 0)
	if err != nil {
		t.Fatalf("unexpected settled get error: %v", err)
	}
	if settled.Status != jobs.StatusCancelled || !settled.Cancelled || !settled.Finished {
		t.Fatalf("expected settled cancelled snapshot, got %+v", settled)
	}
	if settled.CurrentIndex != 2 {
		t.Fatalf("expected current_index=2 after cancellation settles, got %#v", settled.CurrentIndex)
	}
}

type batchFakeQueue struct {
	tasks []*asynq.Task
}

func (q *batchFakeQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.tasks = append(q.tasks, task)
	return nil
}
