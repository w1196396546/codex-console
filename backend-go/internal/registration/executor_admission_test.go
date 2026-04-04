package registration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestExecutorBatchParallelRespectsConcurrencyLimitAndIgnoresInterval(t *testing.T) {
	logger := &executorAdmissionLogSink{}
	started := make(chan string, 3)
	release := make(chan struct{})

	var mu sync.Mutex
	active := 0
	maxActive := 0

	executor := NewExecutor(logger, admissionTestRunner(func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		started <- req.TaskUUID
		<-release

		mu.Lock()
		active--
		mu.Unlock()
		return map[string]any{"ok": true}, nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	results := make(chan error, 3)
	for _, jobID := range []string{"job-1", "job-2", "job-3"} {
		go func(id string) {
			_, err := executor.Execute(ctx, jobs.Job{
				JobID:     id,
				JobType:   JobTypeSingle,
				ScopeType: "registration_batch",
				ScopeID:   "batch-parallel",
				Payload:   []byte(`{"email_service_type":"tempmail","concurrency":2,"mode":"parallel","interval_min":5,"interval_max":30}`),
			})
			results <- err
		}(jobID)
	}

	waitForAdmissionStartCount(t, started, 2)

	select {
	case jobID := <-started:
		t.Fatalf("expected third batch task to wait for concurrency slot, but %s started early", jobID)
	case <-time.After(50 * time.Millisecond):
	}

	release <- struct{}{}
	waitForAdmissionStartCount(t, started, 1)
	release <- struct{}{}
	release <- struct{}{}

	for range 3 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("unexpected execute error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for executor result: %v", ctx.Err())
		}
	}

	if maxActive != 2 {
		t.Fatalf("expected max active batch executions to be 2, got %d", maxActive)
	}
}

func TestExecutorBatchPipelineEnforcesStartInterval(t *testing.T) {
	logger := &executorAdmissionLogSink{}
	clock := &executorAdmissionFakeClock{now: time.Unix(0, 0)}
	startTimes := make([]time.Time, 0, 3)

	executor := NewExecutor(logger, admissionTestRunner(func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
		startTimes = append(startTimes, clock.Now())
		return map[string]any{"task_uuid": req.TaskUUID}, nil
	}))

	controller := newProcessLocalBatchAdmissionController()
	controller.now = clock.Now
	controller.sleep = clock.Sleep
	executor.admission = controller

	for _, jobID := range []string{"job-1", "job-2", "job-3"} {
		if _, err := executor.Execute(context.Background(), jobs.Job{
			JobID:     jobID,
			JobType:   JobTypeSingle,
			ScopeType: "registration_batch",
			ScopeID:   "batch-pipeline",
			Payload:   []byte(`{"email_service_type":"tempmail","concurrency":2,"mode":"pipeline","interval_min":1,"interval_max":1}`),
		}); err != nil {
			t.Fatalf("execute %s: %v", jobID, err)
		}
	}

	if len(startTimes) != 3 {
		t.Fatalf("expected 3 start times, got %d", len(startTimes))
	}
	if got := startTimes[1].Sub(startTimes[0]); got != time.Second {
		t.Fatalf("expected second task to start after 1s, got %s", got)
	}
	if got := startTimes[2].Sub(startTimes[1]); got != time.Second {
		t.Fatalf("expected third task to start after 1s, got %s", got)
	}
}

func TestExecutorSingleJobIsNotCoordinatedByBatchAdmission(t *testing.T) {
	logger := &executorAdmissionLogSink{}
	started := make(chan string, 2)
	release := make(chan struct{})

	executor := NewExecutor(logger, admissionTestRunner(func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
		started <- req.TaskUUID
		<-release
		return map[string]any{"ok": true}, nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results := make(chan error, 2)
	for _, jobID := range []string{"single-1", "single-2"} {
		go func(id string) {
			_, err := executor.Execute(ctx, jobs.Job{
				JobID:   id,
				JobType: JobTypeSingle,
				Payload: []byte(`{"email_service_type":"tempmail","concurrency":1,"mode":"pipeline","interval_min":10,"interval_max":10}`),
			})
			results <- err
		}(jobID)
	}

	waitForAdmissionStarts(t, started, "single-1", "single-2")
	release <- struct{}{}
	release <- struct{}{}

	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("unexpected single execute error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for single executor result: %v", ctx.Err())
		}
	}

	if len(logger.entries) != 2 {
		t.Fatalf("expected two single-job compatibility logs, got %#v", logger.entries)
	}
	for _, entry := range logger.entries {
		if entry.message != "batch scheduling fields detected; single registration executor ignores interval/concurrency/mode" {
			t.Fatalf("unexpected log entry: %+v", entry)
		}
	}
}

func TestWorkerSkipsRunnerWhenWaitingBatchJobIsCancelled(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	svc := jobs.NewService(repo, nil)

	executor := NewExecutor(svc, admissionTestRunner(func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
		switch req.TaskUUID {
		case "job-1":
			return waitForAdmissionRunnerRelease(t)
		case "job-2":
			t.Fatalf("cancelled waiting job must not call runner")
		default:
			t.Fatalf("unexpected task uuid %s", req.TaskUUID)
		}
		return nil, nil
	}))

	job1 := createAdmissionBatchJob(t, svc, "batch-cancel-wait")
	job2 := createAdmissionBatchJob(t, svc, "batch-cancel-wait")

	task1 := newAdmissionQueueTask(t, job1.JobID)
	task2 := newAdmissionQueueTask(t, job2.JobID)

	worker1Done := make(chan error, 1)
	worker2Done := make(chan error, 1)

	releaseFirst := make(chan struct{})
	admissionRunnerHold = releaseFirst
	defer func() { admissionRunnerHold = nil }()

	go func() {
		worker1Done <- jobs.NewWorkerWithIDAndExecutor(svc, "worker-1", executor).HandleTask(context.Background(), task1)
	}()
	waitForJobStatus(t, svc, job1.JobID, jobs.StatusRunning)

	go func() {
		worker2Done <- jobs.NewWorkerWithIDAndExecutor(svc, "worker-2", executor).HandleTask(context.Background(), task2)
	}()
	waitForJobStatus(t, svc, job2.JobID, jobs.StatusRunning)

	if _, err := svc.CancelJob(context.Background(), job2.JobID); err != nil {
		t.Fatalf("cancel waiting job: %v", err)
	}

	close(releaseFirst)

	if err := <-worker1Done; err != nil {
		t.Fatalf("first worker returned error: %v", err)
	}
	if err := <-worker2Done; err != nil {
		t.Fatalf("second worker returned error: %v", err)
	}

	got, err := svc.GetJob(context.Background(), job2.JobID)
	if err != nil {
		t.Fatalf("get cancelled job: %v", err)
	}
	if got.Status != jobs.StatusCancelled {
		t.Fatalf("expected cancelled waiting job to stay cancelled, got %s", got.Status)
	}
}

func TestWorkerWaitsForPausedBatchJobToResumeBeforeRunnerStart(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	svc := jobs.NewService(repo, nil)

	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{}, 1)

	executor := NewExecutor(svc, admissionTestRunner(func(_ context.Context, req RunnerRequest, _ func(level string, message string) error) (map[string]any, error) {
		switch req.TaskUUID {
		case "job-1":
			<-firstRelease
			return map[string]any{"ok": true}, nil
		case "job-2":
			secondStarted <- struct{}{}
			return map[string]any{"ok": true}, nil
		default:
			t.Fatalf("unexpected task uuid %s", req.TaskUUID)
		}
		return nil, nil
	}))

	job1 := createAdmissionBatchJob(t, svc, "batch-pause-wait")
	job2 := createAdmissionBatchJob(t, svc, "batch-pause-wait")

	task1 := newAdmissionQueueTask(t, job1.JobID)
	task2 := newAdmissionQueueTask(t, job2.JobID)

	worker1Done := make(chan error, 1)
	worker2Done := make(chan error, 1)

	go func() {
		worker1Done <- jobs.NewWorkerWithIDAndExecutor(svc, "worker-1", executor).HandleTask(context.Background(), task1)
	}()
	waitForJobStatus(t, svc, job1.JobID, jobs.StatusRunning)

	go func() {
		worker2Done <- jobs.NewWorkerWithIDAndExecutor(svc, "worker-2", executor).HandleTask(context.Background(), task2)
	}()
	waitForJobStatus(t, svc, job2.JobID, jobs.StatusRunning)

	if _, err := svc.PauseJob(context.Background(), job2.JobID); err != nil {
		t.Fatalf("pause waiting job: %v", err)
	}

	close(firstRelease)

	select {
	case <-secondStarted:
		t.Fatal("paused waiting job started runner before resume")
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := svc.ResumeJob(context.Background(), job2.JobID); err != nil {
		t.Fatalf("resume waiting job: %v", err)
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("resumed waiting job did not start runner")
	}

	if err := <-worker1Done; err != nil {
		t.Fatalf("first worker returned error: %v", err)
	}
	if err := <-worker2Done; err != nil {
		t.Fatalf("second worker returned error: %v", err)
	}
}

func TestProcessLocalBatchAdmissionControllerCleansUpIdleBatchState(t *testing.T) {
	controller := newProcessLocalBatchAdmissionController()

	release, err := controller.Acquire(context.Background(), jobs.Job{
		JobID:     "job-1",
		ScopeType: "registration_batch",
		ScopeID:   "batch-cleanup",
	}, StartRequest{
		EmailServiceType: "tempmail",
		Concurrency:      1,
		Mode:             "parallel",
	})
	if err != nil {
		t.Fatalf("acquire admission: %v", err)
	}

	controller.mu.Lock()
	if got := len(controller.batches); got != 1 {
		controller.mu.Unlock()
		t.Fatalf("expected 1 batch state after acquire, got %d", got)
	}
	controller.mu.Unlock()

	release()

	controller.mu.Lock()
	defer controller.mu.Unlock()
	if got := len(controller.batches); got != 0 {
		t.Fatalf("expected idle batch state to be cleaned up, got %d entries", got)
	}
}

func TestProcessLocalBatchAdmissionControllerCleansUpIdlePipelineBatchStateAfterNextStart(t *testing.T) {
	controller := newProcessLocalBatchAdmissionController()
	controller.nextInterval = func(intervalMin int, intervalMax int) time.Duration {
		return 20 * time.Millisecond
	}

	release, err := controller.Acquire(context.Background(), jobs.Job{
		JobID:     "job-1",
		ScopeType: "registration_batch",
		ScopeID:   "batch-pipeline-cleanup",
	}, StartRequest{
		EmailServiceType: "tempmail",
		Concurrency:      1,
		Mode:             "pipeline",
		IntervalMin:      1,
		IntervalMax:      1,
	})
	if err != nil {
		t.Fatalf("acquire admission: %v", err)
	}

	release()

	deadline := time.After(200 * time.Millisecond)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		controller.mu.Lock()
		got := len(controller.batches)
		controller.mu.Unlock()
		if got == 0 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected idle pipeline batch state to be cleaned up after next start, got %d entries", got)
		case <-ticker.C:
		}
	}
}

type admissionTestRunner func(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (map[string]any, error)

func (f admissionTestRunner) Run(ctx context.Context, req RunnerRequest, logf func(level string, message string) error) (map[string]any, error) {
	return f(ctx, req, logf)
}

type executorAdmissionLogSink struct {
	mu      sync.Mutex
	entries []executorAdmissionLogEntry
}

func (s *executorAdmissionLogSink) AppendLog(_ context.Context, _ string, level string, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, executorAdmissionLogEntry{level: level, message: message})
	return nil
}

type executorAdmissionLogEntry struct {
	level   string
	message string
}

type executorAdmissionFakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *executorAdmissionFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *executorAdmissionFakeClock) Sleep(_ context.Context, delay time.Duration) error {
	c.mu.Lock()
	c.now = c.now.Add(delay)
	c.mu.Unlock()
	return nil
}

func waitForAdmissionStarts(t *testing.T, started <-chan string, want ...string) {
	t.Helper()

	remaining := make(map[string]struct{}, len(want))
	for _, item := range want {
		remaining[item] = struct{}{}
	}

	for len(remaining) > 0 {
		select {
		case got := <-started:
			if _, ok := remaining[got]; !ok {
				t.Fatalf("expected one of %v to start, got %s", want, got)
			}
			delete(remaining, got)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %v to start", want)
		}
	}
}

func waitForAdmissionStartCount(t *testing.T, started <-chan string, count int) {
	t.Helper()

	for range count {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %d admission starts", count)
		}
	}
}

var admissionRunnerHold chan struct{}

func waitForAdmissionRunnerRelease(t *testing.T) (map[string]any, error) {
	t.Helper()

	if admissionRunnerHold != nil {
		<-admissionRunnerHold
	}
	return map[string]any{"ok": true}, nil
}

func createAdmissionBatchJob(t *testing.T, svc *jobs.Service, batchID string) jobs.Job {
	t.Helper()

	job, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   JobTypeSingle,
		ScopeType: "registration_batch",
		ScopeID:   batchID,
		Payload:   []byte(`{"email_service_type":"tempmail","concurrency":1,"mode":"parallel"}`),
	})
	if err != nil {
		t.Fatalf("create batch job: %v", err)
	}
	return job
}

func newAdmissionQueueTask(t *testing.T, jobID string) *asynq.Task {
	t.Helper()

	payload, err := jobs.MarshalQueuePayload(jobID)
	if err != nil {
		t.Fatalf("marshal queue payload: %v", err)
	}
	return asynq.NewTask(jobs.TypeGenericJob, payload)
}

func waitForJobStatus(t *testing.T, svc *jobs.Service, jobID string, want string) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		job, err := svc.GetJob(context.Background(), jobID)
		if err != nil {
			t.Fatalf("get job %s: %v", jobID, err)
		}
		if job.Status == want {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s to reach status %s, got %s", jobID, want, job.Status)
		case <-ticker.C:
		}
	}
}
