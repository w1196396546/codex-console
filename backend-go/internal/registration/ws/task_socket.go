package ws

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/go-chi/chi/v5"
)

const defaultPollInterval = 250 * time.Millisecond

const (
	taskPausedMessage     = "任务已暂停，等待继续指令"
	taskResumedMessage    = "任务已恢复执行"
	taskCancellingMessage = "取消请求已提交，正在踩刹车，别慌"
	taskCancellingStatus  = "cancelling"
)

type taskService interface {
	GetJob(ctx context.Context, jobID string) (jobs.Job, error)
	ListJobLogs(ctx context.Context, jobID string) ([]jobs.JobLog, error)
	PauseJob(ctx context.Context, jobID string) (jobs.Job, error)
	ResumeJob(ctx context.Context, jobID string) (jobs.Job, error)
	CancelJob(ctx context.Context, jobID string) (jobs.Job, error)
}

type Option func(*Handler)

type Handler struct {
	tasks        taskService
	pollInterval time.Duration
}

func NewHandler(tasks taskService, options ...Option) *Handler {
	handler := &Handler{
		tasks:        tasks,
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

func WithPollInterval(interval time.Duration) Option {
	return func(handler *Handler) {
		handler.pollInterval = interval
	}
}

func (h *Handler) HandleTaskSocket(w nethttp.ResponseWriter, r *nethttp.Request) {
	socket, err := upgradeTaskSocket(w, r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	defer socket.close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	taskUUID := chi.URLParam(r, "task_uuid")
	state := &socketState{}

	if err := h.sendSnapshot(ctx, socket, state, taskUUID); err != nil {
		return
	}

	errs := make(chan error, 2)
	go func() {
		errs <- h.readLoop(ctx, socket, state, taskUUID)
	}()
	go func() {
		errs <- h.pollLoop(ctx, socket, state, taskUUID)
	}()

	<-errs
}

func (h *Handler) sendSnapshot(
	ctx context.Context,
	socket *taskSocketConn,
	state *socketState,
	taskUUID string,
) error {
	job, err := h.tasks.GetJob(ctx, taskUUID)
	if err != nil {
		return err
	}
	logs, err := h.tasks.ListJobLogs(ctx, taskUUID)
	if err != nil {
		return err
	}
	if err := socket.writeJSON(taskStatusMessage(job, taskUUID, 0, len(logs), "")); err != nil {
		return err
	}

	for index, item := range logs {
		if err := socket.writeJSON(taskLogMessage(taskUUID, item.Message, index)); err != nil {
			return err
		}
	}

	state.set(job.Status, len(logs))
	return nil
}

func (h *Handler) readLoop(
	ctx context.Context,
	socket *taskSocketConn,
	state *socketState,
	taskUUID string,
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
			if err := h.writeStatusUpdate(ctx, socket, state, taskUUID, h.tasks.PauseJob, taskPausedMessage); err != nil {
				return err
			}
		case "resume":
			if err := h.writeStatusUpdate(ctx, socket, state, taskUUID, h.tasks.ResumeJob, taskResumedMessage); err != nil {
				return err
			}
		case "cancel":
			if err := h.writeCancellingUpdate(ctx, socket, state, taskUUID); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) writeStatusUpdate(
	ctx context.Context,
	socket *taskSocketConn,
	state *socketState,
	taskUUID string,
	update func(ctx context.Context, jobID string) (jobs.Job, error),
	message string,
) error {
	job, err := update(ctx, taskUUID)
	if err != nil {
		return err
	}
	_, logCount := state.get()
	if err := socket.writeJSON(taskStatusMessage(job, taskUUID, logCount, logCount, message)); err != nil {
		return err
	}
	state.set(job.Status, logCount)
	return nil
}

func (h *Handler) writeCancellingUpdate(
	ctx context.Context,
	socket *taskSocketConn,
	state *socketState,
	taskUUID string,
) error {
	job, err := h.tasks.CancelJob(ctx, taskUUID)
	if err != nil {
		return err
	}

	_, logCount := state.get()
	message := taskStatusMessage(job, taskUUID, logCount, logCount, taskCancellingMessage)
	message["status"] = taskCancellingStatus
	if err := socket.writeJSON(message); err != nil {
		return err
	}
	state.set(taskCancellingStatus, logCount)
	return nil
}

func (h *Handler) pollLoop(
	ctx context.Context,
	socket *taskSocketConn,
	state *socketState,
	taskUUID string,
) error {
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			job, err := h.tasks.GetJob(ctx, taskUUID)
			if err != nil {
				return err
			}
			logs, err := h.tasks.ListJobLogs(ctx, taskUUID)
			if err != nil {
				return err
			}

			lastStatus, lastLogCount := state.get()
			if job.Status != lastStatus {
				if err := socket.writeJSON(taskStatusMessage(job, taskUUID, lastLogCount, len(logs), "")); err != nil {
					return err
				}
				lastStatus = job.Status
			}

			for index := lastLogCount; index < len(logs); index++ {
				if err := socket.writeJSON(taskLogMessage(taskUUID, logs[index].Message, index)); err != nil {
					return err
				}
			}

			state.set(lastStatus, len(logs))
		}
	}
}

func taskStatusMessage(job jobs.Job, taskUUID string, logOffset int, logNextOffset int, message string) map[string]any {
	taskMetadata := registration.ResolveTaskMetadata(job)
	payload := map[string]any{
		"type":            "status",
		"task_uuid":       taskUUID,
		"status":          job.Status,
		"email":           taskMetadata.Email,
		"email_service":   taskMetadata.EmailService,
		"log_offset":      logOffset,
		"log_next_offset": logNextOffset,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if message != "" {
		payload["message"] = message
	}
	return payload
}

func taskLogMessage(taskUUID string, message string, index int) map[string]any {
	return map[string]any{
		"type":            "log",
		"task_uuid":       taskUUID,
		"message":         message,
		"log":             message,
		"log_offset":      index,
		"log_next_offset": index + 1,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
	}
}

type socketState struct {
	mu       sync.Mutex
	status   string
	logCount int
}

func (s *socketState) get() (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.logCount
}

func (s *socketState) set(status string, logCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.logCount = logCount
}

type taskSocketConn struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

func upgradeTaskSocket(w nethttp.ResponseWriter, r *nethttp.Request) (*taskSocketConn, error) {
	if r.Method != nethttp.MethodGet {
		return nil, fmt.Errorf("websocket requires GET")
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		return nil, fmt.Errorf("missing websocket connection upgrade")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, fmt.Errorf("missing websocket upgrade header")
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, fmt.Errorf("missing websocket key")
	}

	hijacker, ok := w.(nethttp.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response writer does not support hijacking")
	}

	conn, buffer, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	accept := computeWebSocketAccept(key)
	if _, err := fmt.Fprintf(
		buffer,
		"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		accept,
	); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := buffer.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &taskSocketConn{
		conn:   conn,
		reader: buffer.Reader,
		writer: buffer.Writer,
	}, nil
}

func (c *taskSocketConn) close() {
	_ = c.conn.Close()
}

func (c *taskSocketConn) writeJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	frame := make([]byte, 0, len(data)+10)
	frame = append(frame, 0x81)
	switch {
	case len(data) < 126:
		frame = append(frame, byte(len(data)))
	case len(data) <= 65535:
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(data)))
	default:
		frame = append(frame, 127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(data)))
	}
	frame = append(frame, data...)

	if _, err := c.writer.Write(frame); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *taskSocketConn) readClientJSON() (map[string]any, error) {
	payload, err := readClientFrame(c.reader)
	if err != nil {
		return nil, err
	}

	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		return nil, err
	}
	return message, nil
}

func readClientFrame(reader *bufio.Reader) ([]byte, error) {
	header, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	opcode := header & 0x0f
	if opcode == 0x8 {
		return nil, io.EOF
	}
	if opcode != 0x1 {
		return nil, fmt.Errorf("unexpected websocket opcode %d", opcode)
	}

	lengthByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if lengthByte&0x80 == 0 {
		return nil, fmt.Errorf("client websocket frames must be masked")
	}

	payloadLength := int(lengthByte & 0x7f)
	switch payloadLength {
	case 126:
		lengthBytes := make([]byte, 2)
		if _, err := io.ReadFull(reader, lengthBytes); err != nil {
			return nil, err
		}
		payloadLength = int(binary.BigEndian.Uint16(lengthBytes))
	case 127:
		lengthBytes := make([]byte, 8)
		if _, err := io.ReadFull(reader, lengthBytes); err != nil {
			return nil, err
		}
		payloadLength = int(binary.BigEndian.Uint64(lengthBytes))
	}

	mask := make([]byte, 4)
	if _, err := io.ReadFull(reader, mask); err != nil {
		return nil, err
	}

	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	for index := range payload {
		payload[index] ^= mask[index%4]
	}
	return payload, nil
}

func headerContainsToken(header nethttp.Header, key string, want string) bool {
	for _, value := range header.Values(key) {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}

func computeWebSocketAccept(key string) string {
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(hash[:])
}
