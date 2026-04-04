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

func TestHandleJobStopsWithoutFailureOnControlStop(t *testing.T) {
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
	worker := jobs.NewWorkerWithIDAndExecutor(svc, "worker-test", jobs.ExecutorFunc(func(ctx context.Context, job jobs.Job) (map[string]any, error) {
		if _, err := svc.CancelJob(ctx, job.JobID); err != nil {
			t.Fatalf("cancel job: %v", err)
		}
		return nil, jobs.NewExecutionControlError(jobs.ControlActionStop, jobs.StatusCancelled)
	}))

	if err := worker.HandleTask(context.Background(), task); err != nil {
		t.Fatalf("expected control stop to return nil, got %v", err)
	}

	got, err := svc.GetJob(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("unexpected get job error: %v", err)
	}
	if got.Status != jobs.StatusCancelled {
		t.Fatalf("expected cancelled status after control stop, got %s", got.Status)
	}
}

func TestHandleJobWaitsForControlResumeBeforeRetry(t *testing.T) {
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
	resumeReady := make(chan struct{})
	resumed := make(chan struct{})
	calls := 0
	worker := jobs.NewWorkerWithIDAndExecutor(svc, "worker-test", jobs.ExecutorFunc(func(ctx context.Context, job jobs.Job) (map[string]any, error) {
		calls++
		switch calls {
		case 1:
			if _, err := svc.PauseJob(ctx, job.JobID); err != nil {
				t.Fatalf("pause job: %v", err)
			}
			close(resumeReady)
			return nil, jobs.NewExecutionControlError(jobs.ControlActionWait, jobs.StatusPaused)
		case 2:
			select {
			case <-resumed:
			default:
				t.Fatal("executor retried before job was resumed")
			}
			if job.Status != jobs.StatusRunning {
				t.Fatalf("expected resumed retry to see running job, got %s", job.Status)
			}
			return map[string]any{"ok": true}, nil
		default:
			t.Fatalf("unexpected executor call #%d", calls)
		}
		return nil, nil
	}))

	done := make(chan error, 1)
	go func() {
		done <- worker.HandleTask(context.Background(), task)
	}()

	<-resumeReady

	select {
	case err := <-done:
		t.Fatalf("worker returned before resume: %v", err)
	default:
	}

	if _, err := svc.ResumeJob(context.Background(), job.JobID); err != nil {
		t.Fatalf("resume job: %v", err)
	}
	close(resumed)

	if err := <-done; err != nil {
		t.Fatalf("expected resumed worker to succeed, got %v", err)
	}

	got, err := svc.GetJob(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("unexpected get job error: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed status after resume, got %s", got.Status)
	}
	if calls != 2 {
		t.Fatalf("expected executor to be called twice, got %d", calls)
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
