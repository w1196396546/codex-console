package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

type Executor interface {
	Execute(ctx context.Context, job Job) (map[string]any, error)
}

type ControlAction string

const (
	ControlActionWait ControlAction = "wait"
	ControlActionStop ControlAction = "stop"
)

type ExecutionControlError struct {
	action ControlAction
	status string
}

func NewExecutionControlError(action ControlAction, status string) error {
	return &ExecutionControlError{
		action: action,
		status: strings.TrimSpace(status),
	}
}

func (e *ExecutionControlError) Error() string {
	return fmt.Sprintf("worker execution control: action=%s status=%s", e.action, e.status)
}

func (e *ExecutionControlError) Action() ControlAction {
	return e.action
}

func (e *ExecutionControlError) Status() string {
	return e.status
}

type ExecutorFunc func(ctx context.Context, job Job) (map[string]any, error)

func (f ExecutorFunc) Execute(ctx context.Context, job Job) (map[string]any, error) {
	return f(ctx, job)
}

type Worker struct {
	service  *Service
	workerID string
	executor Executor
}

func NewWorker(service *Service) *Worker {
	return NewWorkerWithID(service, "worker-default")
}

func NewWorkerWithID(service *Service, workerID string) *Worker {
	return NewWorkerWithIDAndExecutor(service, workerID, defaultExecutor())
}

func NewWorkerWithIDAndExecutor(service *Service, workerID string, executor Executor) *Worker {
	if workerID == "" {
		workerID = "worker-default"
	}
	if executor == nil {
		executor = defaultExecutor()
	}

	return &Worker{
		service:  service,
		workerID: workerID,
		executor: executor,
	}
}

func (w *Worker) HandleTask(ctx context.Context, task *asynq.Task) error {
	if task == nil {
		return errors.New("task is required")
	}
	if task.Type() != TypeGenericJob {
		return errors.New("unsupported task type")
	}

	payload, err := UnmarshalQueuePayload(task.Payload())
	if err != nil {
		return err
	}
	if payload.JobID == "" {
		return errors.New("job_id is required")
	}

	job, err := w.service.MarkRunning(ctx, payload.JobID, w.workerID)
	if err != nil {
		return err
	}
	if err := w.service.AppendLog(ctx, payload.JobID, "info", "job started"); err != nil {
		return err
	}

	for {
		result, err := w.executor.Execute(ctx, job)
		if err == nil {
			_, err = w.service.MarkCompleted(ctx, payload.JobID, result)
			if err != nil {
				return err
			}

			if err := w.service.AppendLog(ctx, payload.JobID, "info", "job completed"); err != nil {
				return err
			}

			return nil
		}

		control, ok := executionControlFromError(err)
		if ok {
			job, ok, err = w.handleExecutionControl(ctx, payload.JobID, control)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			continue
		}

		_ = w.service.AppendLog(ctx, payload.JobID, "error", err.Error())
		if _, markErr := w.service.MarkFailed(ctx, payload.JobID, err.Error()); markErr != nil {
			return markErr
		}
		return err
	}
}

func defaultExecutor() Executor {
	return ExecutorFunc(func(_ context.Context, _ Job) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
}

func executionControlFromError(err error) (*ExecutionControlError, bool) {
	var control *ExecutionControlError
	if !errors.As(err, &control) {
		return nil, false
	}
	return control, true
}

func (w *Worker) handleExecutionControl(ctx context.Context, jobID string, control *ExecutionControlError) (Job, bool, error) {
	switch control.Action() {
	case ControlActionStop:
		return Job{}, true, nil
	case ControlActionWait:
		return w.waitUntilRunnable(ctx, jobID)
	default:
		return Job{}, false, fmt.Errorf("unsupported worker control action: %s", control.Action())
	}
}

func (w *Worker) waitUntilRunnable(ctx context.Context, jobID string) (Job, bool, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		job, err := w.service.GetJob(ctx, jobID)
		if err != nil {
			return Job{}, false, err
		}

		switch strings.TrimSpace(job.Status) {
		case StatusPaused:
			select {
			case <-ctx.Done():
				return Job{}, false, ctx.Err()
			case <-ticker.C:
			}
		case StatusCancelled, StatusCompleted, StatusFailed:
			return Job{}, true, nil
		default:
			job, err := w.service.MarkRunning(ctx, jobID, w.workerID)
			if err != nil {
				return Job{}, false, err
			}
			return job, false, nil
		}
	}
}
