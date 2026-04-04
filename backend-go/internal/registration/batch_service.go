package registration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
)

var (
	ErrBatchNotFound           = errors.New("batch not found")
	ErrInvalidBatchCount       = errors.New("batch count must be greater than 0")
	ErrInvalidBatchInterval    = errors.New("batch interval is invalid")
	ErrInvalidBatchConcurrency = errors.New("batch concurrency must be between 1 and 50")
	ErrInvalidBatchMode        = errors.New("batch mode must be parallel or pipeline")
	ErrBatchAlreadyPaused      = errors.New("batch is already paused")
	ErrBatchNotPaused          = errors.New("batch is not paused")
	ErrBatchFinished           = errors.New("batch is already finished")
)

type BatchStartRequest struct {
	Count              int            `json:"count"`
	EmailServiceType   string         `json:"email_service_type"`
	Proxy              string         `json:"proxy,omitempty"`
	EmailServiceID     *int           `json:"email_service_id,omitempty"`
	EmailServiceConfig map[string]any `json:"email_service_config,omitempty"`
	IntervalMin        int            `json:"interval_min,omitempty"`
	IntervalMax        int            `json:"interval_max,omitempty"`
	Concurrency        int            `json:"concurrency,omitempty"`
	Mode               string         `json:"mode,omitempty"`
	AutoUploadCPA      bool           `json:"auto_upload_cpa,omitempty"`
	CPAServiceIDs      []int          `json:"cpa_service_ids,omitempty"`
	AutoUploadSub2API  bool           `json:"auto_upload_sub2api,omitempty"`
	Sub2APIServiceIDs  []int          `json:"sub2api_service_ids,omitempty"`
	AutoUploadTM       bool           `json:"auto_upload_tm,omitempty"`
	TMServiceIDs       []int          `json:"tm_service_ids,omitempty"`
}

type BatchTask struct {
	TaskUUID string `json:"task_uuid"`
	Status   string `json:"status"`
}

type BatchStartResponse struct {
	BatchID string      `json:"batch_id"`
	Count   int         `json:"count"`
	Tasks   []BatchTask `json:"tasks"`
}

type BatchStatusResponse struct {
	BatchID       string   `json:"batch_id"`
	Count         int      `json:"count,omitempty"`
	Status        string   `json:"status"`
	Total         int      `json:"total"`
	Completed     int      `json:"completed"`
	Success       int      `json:"success"`
	Failed        int      `json:"failed"`
	Paused        bool     `json:"paused"`
	Cancelled     bool     `json:"cancelled"`
	Finished      bool     `json:"finished"`
	Logs          []string `json:"logs"`
	LogOffset     int      `json:"log_offset"`
	LogNextOffset int      `json:"log_next_offset"`
	Progress      string   `json:"progress"`
}

type BatchControlResponse struct {
	Success bool   `json:"success"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

type batchJobsService interface {
	CreateJob(ctx context.Context, params jobs.CreateJobParams) (jobs.Job, error)
	EnqueueJob(ctx context.Context, jobID string) error
	GetJob(ctx context.Context, jobID string) (jobs.Job, error)
	ListJobLogs(ctx context.Context, jobID string) ([]jobs.JobLog, error)
	PauseJob(ctx context.Context, jobID string) (jobs.Job, error)
	ResumeJob(ctx context.Context, jobID string) (jobs.Job, error)
	CancelJob(ctx context.Context, jobID string) (jobs.Job, error)
}

type batchRecord struct {
	count           int
	taskUUIDs       []string
	logs            []string
	logOffsets      map[string]int
	cancelRequested bool
}

type BatchService struct {
	jobs    batchJobsService
	mu      sync.RWMutex
	batches map[string]batchRecord
}

func NewBatchService(jobsService batchJobsService) *BatchService {
	return &BatchService{
		jobs:    jobsService,
		batches: make(map[string]batchRecord),
	}
}

func (s *BatchService) StartBatch(ctx context.Context, req BatchStartRequest) (BatchStartResponse, error) {
	if req.Count <= 0 {
		return BatchStartResponse{}, ErrInvalidBatchCount
	}
	options, err := normalizeBatchExecutionOptions(req.IntervalMin, req.IntervalMax, req.Concurrency, req.Mode)
	if err != nil {
		return BatchStartResponse{}, err
	}

	requests := make([]StartRequest, 0, req.Count)
	for range req.Count {
		requests = append(requests, StartRequest{
			EmailServiceType:   req.EmailServiceType,
			Proxy:              req.Proxy,
			EmailServiceID:     req.EmailServiceID,
			EmailServiceConfig: req.EmailServiceConfig,
			IntervalMin:        options.IntervalMin,
			IntervalMax:        options.IntervalMax,
			Concurrency:        options.Concurrency,
			Mode:               options.Mode,
			AutoUploadCPA:      req.AutoUploadCPA,
			CPAServiceIDs:      append([]int(nil), req.CPAServiceIDs...),
			AutoUploadSub2API:  req.AutoUploadSub2API,
			Sub2APIServiceIDs:  append([]int(nil), req.Sub2APIServiceIDs...),
			AutoUploadTM:       req.AutoUploadTM,
			TMServiceIDs:       append([]int(nil), req.TMServiceIDs...),
		})
	}

	return s.startBatchRequests(ctx, requests)
}

func (s *BatchService) startBatchRequests(ctx context.Context, requests []StartRequest) (BatchStartResponse, error) {
	if len(requests) == 0 {
		return BatchStartResponse{}, ErrInvalidBatchCount
	}

	batchID, err := newBatchID()
	if err != nil {
		return BatchStartResponse{}, fmt.Errorf("generate batch id: %w", err)
	}

	s.mu.Lock()
	s.batches[batchID] = batchRecord{
		count:      len(requests),
		taskUUIDs:  make([]string, 0, len(requests)),
		logs:       make([]string, 0),
		logOffsets: make(map[string]int, len(requests)),
	}
	s.mu.Unlock()

	tasks := make([]BatchTask, 0, len(requests))
	for _, request := range requests {
		payload, err := json.Marshal(request)
		if err != nil {
			return BatchStartResponse{}, fmt.Errorf("marshal registration request: %w", err)
		}

		job, createErr := s.jobs.CreateJob(ctx, jobs.CreateJobParams{
			JobType:   "registration_single",
			ScopeType: "registration_batch",
			ScopeID:   batchID,
			Payload:   payload,
		})
		if createErr != nil {
			return BatchStartResponse{}, fmt.Errorf("create batch registration job for batch %s: %w", batchID, createErr)
		}

		if enqueueErr := s.jobs.EnqueueJob(ctx, job.JobID); enqueueErr != nil {
			return BatchStartResponse{}, fmt.Errorf("enqueue batch registration job for batch %s: %w", batchID, enqueueErr)
		}

		tasks = append(tasks, BatchTask{
			TaskUUID: job.JobID,
			Status:   job.Status,
		})
		s.appendBatchTask(batchID, job.JobID)
	}

	return BatchStartResponse{
		BatchID: batchID,
		Count:   len(requests),
		Tasks:   tasks,
	}, nil
}

type batchExecutionOptions struct {
	IntervalMin int
	IntervalMax int
	Concurrency int
	Mode        string
}

func normalizeBatchExecutionOptions(intervalMin int, intervalMax int, concurrency int, mode string) (batchExecutionOptions, error) {
	if intervalMin < 0 || intervalMax < intervalMin {
		return batchExecutionOptions{}, ErrInvalidBatchInterval
	}
	if concurrency == 0 {
		concurrency = 1
	}
	if concurrency < 1 || concurrency > 50 {
		return batchExecutionOptions{}, ErrInvalidBatchConcurrency
	}
	if mode == "" {
		mode = "pipeline"
	}
	if mode != "parallel" && mode != "pipeline" {
		return batchExecutionOptions{}, ErrInvalidBatchMode
	}

	return batchExecutionOptions{
		IntervalMin: intervalMin,
		IntervalMax: intervalMax,
		Concurrency: concurrency,
		Mode:        mode,
	}, nil
}

func (s *BatchService) GetBatch(ctx context.Context, batchID string, logOffset int) (BatchStatusResponse, error) {
	record, stats, err := s.snapshotBatch(ctx, batchID)
	if err != nil {
		return BatchStatusResponse{}, err
	}

	offset := logOffset
	if offset > len(stats.logs) {
		offset = len(stats.logs)
	}

	incrementalLogs := make([]string, len(stats.logs[offset:]))
	copy(incrementalLogs, stats.logs[offset:])
	return BatchStatusResponse{
		BatchID:       batchID,
		Count:         record.count,
		Status:        stats.status,
		Total:         record.count,
		Completed:     stats.completed,
		Success:       stats.success,
		Failed:        stats.failed,
		Paused:        stats.paused,
		Cancelled:     stats.cancelled,
		Finished:      stats.finished,
		Logs:          incrementalLogs,
		LogOffset:     offset,
		LogNextOffset: len(stats.logs),
		Progress:      fmt.Sprintf("%d/%d", stats.completed, record.count),
	}, nil
}

func (s *BatchService) PauseBatch(ctx context.Context, batchID string) (BatchControlResponse, error) {
	record, stats, err := s.snapshotBatch(ctx, batchID)
	if err != nil {
		return BatchControlResponse{}, err
	}
	if stats.finished {
		return BatchControlResponse{}, ErrBatchFinished
	}
	if stats.paused {
		return BatchControlResponse{}, ErrBatchAlreadyPaused
	}

	if err := s.updateBatchJobs(ctx, record.taskUUIDs, shouldPauseBatchJob, s.jobs.PauseJob); err != nil {
		return BatchControlResponse{}, err
	}

	return BatchControlResponse{
		Success: true,
		Status:  jobs.StatusPaused,
		Message: "batch paused",
	}, nil
}

func (s *BatchService) ResumeBatch(ctx context.Context, batchID string) (BatchControlResponse, error) {
	record, stats, err := s.snapshotBatch(ctx, batchID)
	if err != nil {
		return BatchControlResponse{}, err
	}
	if stats.finished {
		return BatchControlResponse{}, ErrBatchFinished
	}
	if !stats.paused {
		return BatchControlResponse{}, ErrBatchNotPaused
	}

	if err := s.updateBatchJobs(ctx, record.taskUUIDs, shouldResumeBatchJob, s.jobs.ResumeJob); err != nil {
		return BatchControlResponse{}, err
	}

	return BatchControlResponse{
		Success: true,
		Status:  jobs.StatusRunning,
		Message: "batch resumed",
	}, nil
}

func (s *BatchService) CancelBatch(ctx context.Context, batchID string) (BatchControlResponse, error) {
	record, stats, err := s.snapshotBatch(ctx, batchID)
	if err != nil {
		return BatchControlResponse{}, err
	}
	if stats.finished {
		return BatchControlResponse{}, ErrBatchFinished
	}

	if err := s.updateBatchJobs(ctx, record.taskUUIDs, shouldCancelBatchJob, s.jobs.CancelJob); err != nil {
		return BatchControlResponse{}, err
	}

	s.mu.Lock()
	stored := s.batches[batchID]
	stored.cancelRequested = true
	s.batches[batchID] = stored
	s.mu.Unlock()

	return BatchControlResponse{
		Success: true,
		Status:  jobs.StatusCancelled,
		Message: "batch cancellation requested",
	}, nil
}

type batchStats struct {
	status    string
	completed int
	success   int
	failed    int
	paused    bool
	cancelled bool
	finished  bool
	logs      []string
}

func (s *BatchService) snapshotBatch(ctx context.Context, batchID string) (batchRecord, batchStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.batches[batchID]
	if !ok {
		return batchRecord{}, batchStats{}, ErrBatchNotFound
	}
	if record.logOffsets == nil {
		record.logOffsets = make(map[string]int, len(record.taskUUIDs))
	}
	if record.logs == nil {
		record.logs = make([]string, 0)
	}

	stats := batchStats{
		status: "running",
		logs:   make([]string, 0, len(record.logs)),
	}

	terminalCount := 0
	pausedCount := 0
	cancelledCount := 0
	for _, taskUUID := range record.taskUUIDs {
		job, err := s.jobs.GetJob(ctx, taskUUID)
		if err != nil {
			return batchRecord{}, batchStats{}, err
		}

		switch job.Status {
		case jobs.StatusCompleted:
			stats.success++
			stats.completed++
			terminalCount++
		case jobs.StatusFailed:
			stats.failed++
			stats.completed++
			terminalCount++
		case jobs.StatusCancelled:
			stats.completed++
			terminalCount++
			cancelledCount++
		case jobs.StatusPaused:
			pausedCount++
		}

		jobLogs, err := s.jobs.ListJobLogs(ctx, taskUUID)
		if err != nil {
			return batchRecord{}, batchStats{}, err
		}
		start := record.logOffsets[taskUUID]
		if start > len(jobLogs) {
			start = len(jobLogs)
		}
		for _, item := range jobLogs[start:] {
			record.logs = append(record.logs, item.Message)
		}
		record.logOffsets[taskUUID] = len(jobLogs)
	}

	unfinishedCount := len(record.taskUUIDs) - terminalCount
	stats.finished = terminalCount == len(record.taskUUIDs)
	stats.paused = unfinishedCount > 0 && pausedCount == unfinishedCount && !record.cancelRequested
	stats.cancelled = cancelledCount > 0 || record.cancelRequested

	switch {
	case stats.finished && stats.cancelled:
		stats.status = jobs.StatusCancelled
	case stats.finished && stats.success == 0 && stats.failed > 0:
		stats.status = jobs.StatusFailed
	case stats.finished:
		stats.status = jobs.StatusCompleted
	case stats.paused:
		stats.status = jobs.StatusPaused
	case record.cancelRequested:
		stats.status = "cancelling"
	default:
		stats.status = jobs.StatusRunning
	}

	stats.logs = append(stats.logs, record.logs...)
	s.batches[batchID] = record

	return cloneBatchRecord(record), stats, nil
}

func (s *BatchService) updateBatchJobs(
	ctx context.Context,
	taskUUIDs []string,
	shouldUpdate func(status string) bool,
	update func(context.Context, string) (jobs.Job, error),
) error {
	for _, taskUUID := range taskUUIDs {
		job, err := s.jobs.GetJob(ctx, taskUUID)
		if err != nil {
			return err
		}
		if !shouldUpdate(job.Status) {
			continue
		}
		if _, err := update(ctx, taskUUID); err != nil {
			return err
		}
	}

	return nil
}

func (s *BatchService) appendBatchTask(batchID string, taskUUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.batches[batchID]
	if !ok {
		return
	}

	record.taskUUIDs = append(record.taskUUIDs, taskUUID)
	if record.logOffsets == nil {
		record.logOffsets = make(map[string]int)
	}
	record.logOffsets[taskUUID] = 0
	s.batches[batchID] = record
}

func cloneBatchRecord(record batchRecord) batchRecord {
	cloned := batchRecord{
		count:           record.count,
		taskUUIDs:       append([]string(nil), record.taskUUIDs...),
		logs:            append([]string(nil), record.logs...),
		cancelRequested: record.cancelRequested,
	}
	if record.logOffsets != nil {
		cloned.logOffsets = make(map[string]int, len(record.logOffsets))
		for taskUUID, offset := range record.logOffsets {
			cloned.logOffsets[taskUUID] = offset
		}
	}
	return cloned
}

func shouldPauseBatchJob(status string) bool {
	return status == jobs.StatusPending || status == jobs.StatusRunning
}

func shouldResumeBatchJob(status string) bool {
	return status == jobs.StatusPaused
}

func shouldCancelBatchJob(status string) bool {
	return status == jobs.StatusPending || status == jobs.StatusRunning || status == jobs.StatusPaused
}

func newBatchID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes[:]), nil
}
