package registration

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

type batchAdmissionController interface {
	Acquire(ctx context.Context, job jobs.Job, req StartRequest) (func(), error)
}

type batchAdmissionJobReader interface {
	GetJob(ctx context.Context, jobID string) (jobs.Job, error)
}

type processLocalBatchAdmissionController struct {
	mu      sync.Mutex
	batches map[string]*batchAdmissionState

	jobs batchAdmissionJobReader

	now          func() time.Time
	sleep        func(ctx context.Context, delay time.Duration) error
	afterFunc    func(delay time.Duration, fn func())
	nextInterval func(intervalMin int, intervalMax int) time.Duration
}

type batchAdmissionState struct {
	running          int
	waiters          int
	nextStart        time.Time
	changed          chan struct{}
	cleanupScheduled bool
}

func newProcessLocalBatchAdmissionController(readers ...batchAdmissionJobReader) *processLocalBatchAdmissionController {
	var reader batchAdmissionJobReader
	if len(readers) > 0 {
		reader = readers[0]
	}

	return &processLocalBatchAdmissionController{
		batches: make(map[string]*batchAdmissionState),
		jobs:    reader,
		now:     time.Now,
		sleep:   sleepWithContext,
		afterFunc: func(delay time.Duration, fn func()) {
			time.AfterFunc(delay, fn)
		},
		nextInterval: func(intervalMin int, intervalMax int) time.Duration {
			if intervalMax <= intervalMin {
				return time.Duration(intervalMin) * time.Second
			}
			return time.Duration(rand.Intn(intervalMax-intervalMin+1)+intervalMin) * time.Second
		},
	}
}

func (c *processLocalBatchAdmissionController) Acquire(ctx context.Context, job jobs.Job, req StartRequest) (func(), error) {
	if c == nil {
		return func() {}, nil
	}

	if job.ScopeType != "registration_batch" || strings.TrimSpace(job.ScopeID) == "" {
		return func() {}, nil
	}

	options, err := normalizeBatchExecutionOptions(req.IntervalMin, req.IntervalMax, req.Concurrency, req.Mode)
	if err != nil {
		return nil, err
	}

	batchID := strings.TrimSpace(job.ScopeID)
	for {
		status, err := c.currentJobStatus(ctx, job)
		if err != nil {
			return nil, err
		}
		switch status {
		case jobs.StatusPaused:
			return nil, jobs.NewExecutionControlError(jobs.ControlActionWait, status)
		case jobs.StatusCancelled:
			return nil, jobs.NewExecutionControlError(jobs.ControlActionStop, status)
		}

		c.mu.Lock()
		state := c.batchStateLocked(batchID)
		now := c.now()
		if state.running >= options.Concurrency {
			changed := state.changed
			state.waiters++
			c.mu.Unlock()
			if err := waitForBatchAdmissionChange(ctx, changed); err != nil {
				c.finishWait(batchID)
				return nil, err
			}
			c.finishWait(batchID)
			continue
		}

		// Python 侧 parallel 模式只做并发闸门，不消费 interval 配置。
		if options.Mode == "pipeline" && !state.nextStart.IsZero() && now.Before(state.nextStart) {
			delay := state.nextStart.Sub(now)
			state.waiters++
			c.mu.Unlock()
			if err := c.sleep(ctx, delay); err != nil {
				c.finishWait(batchID)
				return nil, err
			}
			c.finishWait(batchID)
			continue
		}

		state.running++
		if options.Mode == "pipeline" {
			state.nextStart = now.Add(c.nextInterval(options.IntervalMin, options.IntervalMax))
		}
		c.mu.Unlock()

		return func() {
			c.release(batchID)
		}, nil
	}
}

func (c *processLocalBatchAdmissionController) release(batchID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.batches[batchID]
	if !ok {
		return
	}
	if state.running > 0 {
		state.running--
	}
	close(state.changed)
	state.changed = make(chan struct{})
	c.cleanupIdleBatchStateLocked(batchID, state)
}

func (c *processLocalBatchAdmissionController) finishWait(batchID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.batches[batchID]
	if !ok {
		return
	}
	if state.waiters > 0 {
		state.waiters--
	}
	c.cleanupIdleBatchStateLocked(batchID, state)
}

func (c *processLocalBatchAdmissionController) batchStateLocked(batchID string) *batchAdmissionState {
	state, ok := c.batches[batchID]
	if ok {
		return state
	}

	state = &batchAdmissionState{
		changed: make(chan struct{}),
	}
	c.batches[batchID] = state
	return state
}

func (c *processLocalBatchAdmissionController) cleanupIdleBatchStateLocked(batchID string, state *batchAdmissionState) {
	if state.running > 0 || state.waiters > 0 {
		return
	}
	if !state.nextStart.IsZero() && c.now().Before(state.nextStart) {
		if state.cleanupScheduled {
			return
		}
		state.cleanupScheduled = true
		c.scheduleIdleBatchCleanup(batchID, state.nextStart.Sub(c.now()))
		return
	}
	delete(c.batches, batchID)
}

func (c *processLocalBatchAdmissionController) scheduleIdleBatchCleanup(batchID string, delay time.Duration) {
	if delay <= 0 {
		delay = time.Millisecond
	}

	c.afterFunc(delay, func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		state, ok := c.batches[batchID]
		if !ok {
			return
		}

		state.cleanupScheduled = false
		c.cleanupIdleBatchStateLocked(batchID, state)
	})
}

func (c *processLocalBatchAdmissionController) currentJobStatus(ctx context.Context, job jobs.Job) (string, error) {
	if c == nil || c.jobs == nil {
		return strings.TrimSpace(job.Status), nil
	}

	current, err := c.jobs.GetJob(ctx, job.JobID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(current.Status), nil
}

func waitForBatchAdmissionChange(ctx context.Context, changed <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
