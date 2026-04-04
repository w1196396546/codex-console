package jobs

import (
	"context"
	"errors"

	"github.com/hibiken/asynq"
)

type Executor interface {
	Execute(ctx context.Context, job Job) (map[string]any, error)
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

	result, err := w.executor.Execute(ctx, job)
	if err != nil {
		_ = w.service.AppendLog(ctx, payload.JobID, "error", err.Error())
		if _, markErr := w.service.MarkFailed(ctx, payload.JobID, err.Error()); markErr != nil {
			return markErr
		}
		return err
	}

	_, err = w.service.MarkCompleted(ctx, payload.JobID, result)
	if err != nil {
		return err
	}

	if err := w.service.AppendLog(ctx, payload.JobID, "info", "job completed"); err != nil {
		return err
	}

	return err
}

func defaultExecutor() Executor {
	return ExecutorFunc(func(_ context.Context, _ Job) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
}
