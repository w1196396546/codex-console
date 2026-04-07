package registration

import (
	"context"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

func TestExecutorProvidesRunnerControlFromJobStatus(t *testing.T) {
	repo := jobs.NewInMemoryRepository()
	svc := jobs.NewService(repo, nil)

	created, err := svc.CreateJob(context.Background(), jobs.CreateJobParams{
		JobType:   JobTypeSingle,
		ScopeType: "registration_single",
		ScopeID:   "job-control",
		Payload:   []byte(`{"email_service_type":"tempmail"}`),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	running, err := svc.MarkRunning(context.Background(), created.JobID, "worker-test")
	if err != nil {
		t.Fatalf("mark running: %v", err)
	}

	started := make(chan struct{}, 1)
	allowPauseCheck := make(chan struct{})
	pausedChecked := make(chan struct{}, 1)
	allowResumeCheck := make(chan struct{})
	resumeChecked := make(chan struct{}, 1)
	allowCancelCheck := make(chan struct{})

	executor := NewExecutor(svc, admissionTestRunner(func(ctx context.Context, req RunnerRequest, _ func(level string, message string) error) (RunnerOutput, error) {
		if req.control == nil {
			t.Fatal("expected runner control callback")
		}

		assertRunnerControlState(t, ctx, req.control, runnerControlStateRunning)
		started <- struct{}{}

		<-allowPauseCheck
		assertRunnerControlState(t, ctx, req.control, runnerControlStatePaused)
		pausedChecked <- struct{}{}

		<-allowResumeCheck
		assertRunnerControlState(t, ctx, req.control, runnerControlStateRunning)
		resumeChecked <- struct{}{}

		<-allowCancelCheck
		assertRunnerControlState(t, ctx, req.control, runnerControlStateCancelled)

		return RunnerOutput{Result: map[string]any{"ok": true}}, nil
	}), WithPreparationDependencies(executorPreparationDependencies()))

	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), running)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}

	if _, err := svc.PauseJob(context.Background(), created.JobID); err != nil {
		t.Fatalf("pause job: %v", err)
	}
	close(allowPauseCheck)
	select {
	case <-pausedChecked:
	case <-time.After(time.Second):
		t.Fatal("runner did not observe paused status")
	}

	if _, err := svc.ResumeJob(context.Background(), created.JobID); err != nil {
		t.Fatalf("resume job: %v", err)
	}
	close(allowResumeCheck)
	select {
	case <-resumeChecked:
	case <-time.After(time.Second):
		t.Fatal("runner did not observe resumed status")
	}

	if _, err := svc.CancelJob(context.Background(), created.JobID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	close(allowCancelCheck)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected execute error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("executor did not finish")
	}
}

func assertRunnerControlState(t *testing.T, ctx context.Context, control runnerControlFunc, want runnerControlState) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		got, err := control(ctx)
		if err != nil {
			t.Fatalf("runner control returned error: %v", err)
		}
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner control state mismatch: got %s want %s", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
