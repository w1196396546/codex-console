package ws

import (
	"context"
	nethttp "net/http"
	"sync"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/go-chi/chi/v5"
)

type batchTaskService interface {
	GetBatch(ctx context.Context, batchID string, logOffset int) (registration.BatchStatusResponse, error)
	PauseBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
	ResumeBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
	CancelBatch(ctx context.Context, batchID string) (registration.BatchControlResponse, error)
}

type BatchOption func(*BatchHandler)

type BatchHandler struct {
	batches      batchTaskService
	pollInterval time.Duration
}

func NewBatchHandler(batches batchTaskService, options ...BatchOption) *BatchHandler {
	handler := &BatchHandler{
		batches:      batches,
		pollInterval: defaultPollInterval,
	}
	for _, option := range options {
		option(handler)
	}
	if handler.pollInterval <= 0 {
		handler.pollInterval = defaultPollInterval
	}
	return handler
}

func WithBatchPollInterval(interval time.Duration) BatchOption {
	return func(handler *BatchHandler) {
		handler.pollInterval = interval
	}
}

func (h *BatchHandler) HandleBatchSocket(w nethttp.ResponseWriter, r *nethttp.Request) {
	socket, err := upgradeTaskSocket(w, r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	defer socket.close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	batchID := chi.URLParam(r, "batch_id")
	state := &batchSocketState{}

	if err := h.sendSnapshot(ctx, socket, state, batchID); err != nil {
		return
	}

	errs := make(chan error, 2)
	go func() {
		errs <- h.readLoop(ctx, socket, state, batchID)
	}()
	go func() {
		errs <- h.pollLoop(ctx, socket, state, batchID)
	}()

	<-errs
}

func (h *BatchHandler) sendSnapshot(
	ctx context.Context,
	socket *taskSocketConn,
	state *batchSocketState,
	batchID string,
) error {
	resp, err := h.batches.GetBatch(ctx, batchID, 0)
	if err != nil {
		return err
	}

	if err := socket.writeJSON(batchStatusMessage(resp)); err != nil {
		return err
	}
	for _, log := range resp.Logs {
		if err := socket.writeJSON(batchLogMessage(batchID, log)); err != nil {
			return err
		}
	}

	state.set(batchSnapshotFromResponse(resp), resp.LogNextOffset)
	return nil
}

func (h *BatchHandler) readLoop(
	ctx context.Context,
	socket *taskSocketConn,
	state *batchSocketState,
	batchID string,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := socket.readClientJSON()
		if err != nil {
			return err
		}

		messageType, _ := payload["type"].(string)
		switch messageType {
		case "ping":
			if err := socket.writeJSON(map[string]string{"type": "pong"}); err != nil {
				return err
			}
		case "pause":
			if err := h.writeBatchStatusUpdate(ctx, socket, state, batchID, h.batches.PauseBatch); err != nil {
				return err
			}
		case "resume":
			if err := h.writeBatchStatusUpdate(ctx, socket, state, batchID, h.batches.ResumeBatch); err != nil {
				return err
			}
		case "cancel":
			state.syncMu.Lock()
			_, logOffset := state.get()
			if _, err := h.batches.CancelBatch(ctx, batchID); err != nil {
				state.syncMu.Unlock()
				return err
			}

			resp, err := h.batches.GetBatch(ctx, batchID, logOffset)
			if err != nil {
				state.syncMu.Unlock()
				return err
			}
			for _, log := range resp.Logs {
				if err := socket.writeJSON(batchLogMessage(batchID, log)); err != nil {
					state.syncMu.Unlock()
					return err
				}
			}

			resp.Status = "cancelling"
			resp.Cancelled = true
			resp.Finished = false
			if err := socket.writeJSON(batchStatusMessage(resp)); err != nil {
				state.syncMu.Unlock()
				return err
			}
			state.set(batchSnapshotFromResponse(resp), resp.LogNextOffset)
			state.syncMu.Unlock()
		}
	}
}

func (h *BatchHandler) writeBatchStatusUpdate(
	ctx context.Context,
	socket *taskSocketConn,
	state *batchSocketState,
	batchID string,
	update func(context.Context, string) (registration.BatchControlResponse, error),
) error {
	state.syncMu.Lock()
	defer state.syncMu.Unlock()

	if _, err := update(ctx, batchID); err != nil {
		return err
	}

	_, logOffset := state.get()
	resp, err := h.batches.GetBatch(ctx, batchID, logOffset)
	if err != nil {
		return err
	}
	for _, log := range resp.Logs {
		if err := socket.writeJSON(batchLogMessage(batchID, log)); err != nil {
			return err
		}
	}
	if err := socket.writeJSON(batchStatusMessage(resp)); err != nil {
		return err
	}

	state.set(batchSnapshotFromResponse(resp), resp.LogNextOffset)
	return nil
}

func (h *BatchHandler) pollLoop(
	ctx context.Context,
	socket *taskSocketConn,
	state *batchSocketState,
	batchID string,
) error {
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			state.syncMu.Lock()
			lastSnapshot, lastLogOffset := state.get()
			resp, err := h.batches.GetBatch(ctx, batchID, lastLogOffset)
			if err != nil {
				state.syncMu.Unlock()
				return err
			}

			for _, log := range resp.Logs {
				if err := socket.writeJSON(batchLogMessage(batchID, log)); err != nil {
					state.syncMu.Unlock()
					return err
				}
			}

			nextSnapshot := batchSnapshotFromResponse(resp)
			if nextSnapshot != lastSnapshot {
				if err := socket.writeJSON(batchStatusMessage(resp)); err != nil {
					state.syncMu.Unlock()
					return err
				}
			}

			state.set(nextSnapshot, resp.LogNextOffset)
			state.syncMu.Unlock()
		}
	}
}

type batchSnapshot struct {
	Status    string
	Total     int
	Completed int
	Success   int
	Failed    int
	Paused    bool
	Cancelled bool
	Finished  bool
}

type batchSocketState struct {
	mu        sync.Mutex
	syncMu    sync.Mutex
	snapshot  batchSnapshot
	logOffset int
}

func (s *batchSocketState) get() (batchSnapshot, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot, s.logOffset
}

func (s *batchSocketState) set(snapshot batchSnapshot, logOffset int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshot
	s.logOffset = logOffset
}

func batchSnapshotFromResponse(resp registration.BatchStatusResponse) batchSnapshot {
	return batchSnapshot{
		Status:    resp.Status,
		Total:     resp.Total,
		Completed: resp.Completed,
		Success:   resp.Success,
		Failed:    resp.Failed,
		Paused:    resp.Paused,
		Cancelled: resp.Cancelled,
		Finished:  resp.Finished,
	}
}

func batchStatusMessage(resp registration.BatchStatusResponse) map[string]any {
	return map[string]any{
		"type":      "status",
		"batch_id":  resp.BatchID,
		"status":    resp.Status,
		"total":     resp.Total,
		"completed": resp.Completed,
		"success":   resp.Success,
		"failed":    resp.Failed,
		"paused":    resp.Paused,
		"cancelled": resp.Cancelled,
		"finished":  resp.Finished,
	}
}

func batchLogMessage(batchID string, message string) map[string]any {
	return map[string]any{
		"type":     "log",
		"batch_id": batchID,
		"message":  message,
	}
}
