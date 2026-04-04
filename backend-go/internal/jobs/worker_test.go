package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestHandleJobMarksJobCompleted(t *testing.T) {
	svc := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
		Payload:   []byte(`{"team_id":42}`),
	})
	if err != nil {
		t.Fatalf("unexpected create job error: %v", err)
	}

	payload, err := jobs.MarshalQueuePayload(job.JobID)
	if err != nil {
		t.Fatalf("unexpected marshal payload error: %v", err)
	}

	task := asynq.NewTask(jobs.TypeGenericJob, payload)
	if err := jobs.NewWorkerWithIDAndExecutor(svc, "worker-test", jobs.ExecutorFunc(func(_ context.Context, job jobs.Job) (map[string]any, error) {
		if job.Status != jobs.StatusRunning {
			t.Fatalf("expected executor to receive running job, got %s", job.Status)
		}
		return map[string]any{"ok": true}, nil
	})).HandleTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected handle task error: %v", err)
	}

	got, err := svc.GetJob(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("unexpected get job error: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestHandleJobReturnsExecutorError(t *testing.T) {
	svc := jobs.NewService(jobs.NewInMemoryRepository(), nil)
	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
		Payload:   []byte(`{"team_id":42}`),
	})
	if err != nil {
		t.Fatalf("unexpected create job error: %v", err)
	}

	payload, err := jobs.MarshalQueuePayload(job.JobID)
	if err != nil {
		t.Fatalf("unexpected marshal payload error: %v", err)
	}

	task := asynq.NewTask(jobs.TypeGenericJob, payload)
	worker := jobs.NewWorkerWithIDAndExecutor(svc, "worker-test", jobs.ExecutorFunc(func(context.Context, jobs.Job) (map[string]any, error) {
		return nil, errors.New("boom")
	}))

	if err := worker.HandleTask(context.Background(), task); err == nil {
		t.Fatal("expected worker to return executor error")
	}

	got, err := svc.GetJob(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("unexpected get job error: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("expected failed status after executor error, got %s", got.Status)
	}
}

func TestEnqueueJobPushesGenericTask(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	queue := &fakeQueue{}
	svc := jobs.NewService(repo, queue)

	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   "team_sync",
		ScopeType: "team",
		ScopeID:   "42",
		Payload:   []byte(`{"team_id":42}`),
	})
	if err != nil {
		t.Fatalf("unexpected create job error: %v", err)
	}

	if err := svc.EnqueueJob(context.Background(), job.JobID); err != nil {
		t.Fatalf("unexpected enqueue job error: %v", err)
	}
	if queue.task == nil {
		t.Fatal("expected task to be enqueued")
	}
	if queue.task.Type() != jobs.TypeGenericJob {
		t.Fatalf("expected task type %q, got %q", jobs.TypeGenericJob, queue.task.Type())
	}

	wantPayload, err := jobs.MarshalQueuePayload(job.JobID)
	if err != nil {
		t.Fatalf("unexpected marshal payload error: %v", err)
	}
	if string(queue.task.Payload()) != string(wantPayload) {
		t.Fatalf("unexpected payload: got %s want %s", string(queue.task.Payload()), string(wantPayload))
	}
}

type fakeQueue struct {
	task *asynq.Task
}

func (q *fakeQueue) Enqueue(_ context.Context, task *asynq.Task) error {
	q.task = task
	return nil
}
