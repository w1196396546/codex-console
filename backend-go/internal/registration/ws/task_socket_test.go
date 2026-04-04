package ws

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/go-chi/chi/v5"
)

func TestTaskSocketSendsCurrentStatusAndLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := jobs.NewInMemoryRepository()
	service := jobs.NewService(repo, nil)

	job, err := service.CreateJob(ctx, jobs.CreateJobParams{
		JobType:   "registration_single",
		ScopeType: "registration",
		ScopeID:   "scope-1",
		Payload:   []byte(`{"email_service_type":"tempmail"}`),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := repo.AppendJobLog(ctx, job.JobID, "info", "first log"); err != nil {
		t.Fatalf("append first log: %v", err)
	}
	if err := repo.AppendJobLog(ctx, job.JobID, "info", "second log"); err != nil {
		t.Fatalf("append second log: %v", err)
	}

	handler := NewHandler(service, WithPollInterval(5*time.Millisecond))
	router := chi.NewRouter()
	router.Get("/api/ws/task/{task_uuid}", handler.HandleTaskSocket)

	server := httptest.NewServer(router)
	defer server.Close()

	conn := dialTestWebSocket(t, server.URL+"/api/ws/task/"+job.JobID)
	defer conn.close(t)

	statusMessage := conn.readJSON(t)
	assertMessageField(t, statusMessage, "type", "status")
	assertMessageField(t, statusMessage, "task_uuid", job.JobID)
	assertMessageField(t, statusMessage, "status", jobs.StatusPending)
	assertMessageField(t, statusMessage, "email_service", "tempmail")

	firstLog := conn.readJSON(t)
	assertMessageField(t, firstLog, "type", "log")
	assertMessageField(t, firstLog, "task_uuid", job.JobID)
	assertMessageField(t, firstLog, "message", "first log")

	secondLog := conn.readJSON(t)
	assertMessageField(t, secondLog, "type", "log")
	assertMessageField(t, secondLog, "task_uuid", job.JobID)
	assertMessageField(t, secondLog, "message", "second log")

	conn.writeJSON(t, map[string]any{"type": "ping"})
	pong := conn.readJSON(t)
	assertMessageField(t, pong, "type", "pong")

	conn.writeJSON(t, map[string]any{"type": "pause"})
	paused := conn.readJSON(t)
	assertMessageField(t, paused, "type", "status")
	assertMessageField(t, paused, "status", jobs.StatusPaused)
	assertMessageField(t, paused, "email_service", "tempmail")

	conn.writeJSON(t, map[string]any{"type": "resume"})
	resumed := conn.readJSON(t)
	assertMessageField(t, resumed, "type", "status")
	assertMessageField(t, resumed, "status", jobs.StatusPending)
	assertMessageField(t, resumed, "email_service", "tempmail")

	if _, err := repo.UpdateJobStatus(ctx, job.JobID, jobs.StatusRunning); err != nil {
		t.Fatalf("set running status: %v", err)
	}
	if err := repo.AppendJobLog(ctx, job.JobID, "info", "third log"); err != nil {
		t.Fatalf("append third log: %v", err)
	}

	updates := conn.readMessagesUntil(t, 2, func(message map[string]any) bool {
		messageType, _ := message["type"].(string)
		if messageType == "status" {
			return message["status"] == jobs.StatusRunning
		}
		if messageType == "log" {
			return message["message"] == "third log"
		}
		return false
	})
	assertContainsMessage(t, updates, "status", "type", "status")
	assertContainsMessage(t, updates, jobs.StatusRunning, "status", "status")
	assertContainsMessage(t, updates, "tempmail", "email_service", "email_service")
	assertContainsMessage(t, updates, "third log", "message", "message")

	conn.writeJSON(t, map[string]any{"type": "cancel"})
	cancelled := conn.readJSON(t)
	assertMessageField(t, cancelled, "type", "status")
	assertMessageField(t, cancelled, "status", jobs.StatusCancelled)
	assertMessageField(t, cancelled, "email_service", "tempmail")
}

func assertMessageField(t *testing.T, payload map[string]any, key string, want string) {
	t.Helper()

	got, ok := payload[key].(string)
	if !ok {
		t.Fatalf("expected %s to be string, got %#v", key, payload[key])
	}
	if got != want {
		t.Fatalf("expected %s=%q, got %#v", key, want, payload[key])
	}
}

func assertContainsMessage(t *testing.T, messages []map[string]any, want string, key string, matchKey string) {
	t.Helper()

	for _, message := range messages {
		got, _ := message[key].(string)
		if got != want {
			continue
		}
		if _, ok := message[matchKey]; ok {
			return
		}
	}

	t.Fatalf("expected messages to contain %s=%q, got %#v", key, want, messages)
}

type testWebSocketConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dialTestWebSocket(t *testing.T, rawURL string) *testWebSocketConn {
	t.Helper()

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse ws url: %v", err)
	}

	address := parsedURL.Host
	if !strings.Contains(address, ":") {
		address += ":80"
	}

	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("dial ws server: %v", err)
	}

	reader := bufio.NewReader(conn)
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate websocket key: %v", err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	requestPath := parsedURL.RequestURI()
	if requestPath == "" {
		requestPath = "/"
	}

	request := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		requestPath,
		parsedURL.Host,
		key,
	)
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write websocket handshake: %v", err)
	}

	response, err := nethttp.ReadResponse(reader, &nethttp.Request{Method: nethttp.MethodGet})
	if err != nil {
		t.Fatalf("read websocket handshake response: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != nethttp.StatusSwitchingProtocols {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected 101 switching protocols, got %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	return &testWebSocketConn{
		conn:   conn,
		reader: reader,
	}
}

func (c *testWebSocketConn) close(t *testing.T) {
	t.Helper()
	_ = c.conn.Close()
}

func (c *testWebSocketConn) writeJSON(t *testing.T, payload map[string]any) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal ws message: %v", err)
	}

	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		t.Fatalf("generate ws mask: %v", err)
	}

	frame := make([]byte, 0, len(data)+14)
	frame = append(frame, 0x81)
	switch {
	case len(data) < 126:
		frame = append(frame, byte(len(data))|0x80)
	case len(data) <= 65535:
		frame = append(frame, 126|0x80)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(data)))
	default:
		frame = append(frame, 127|0x80)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(data)))
	}
	frame = append(frame, mask...)
	for index, value := range data {
		frame = append(frame, value^mask[index%4])
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if _, err := c.conn.Write(frame); err != nil {
		t.Fatalf("write ws frame: %v", err)
	}
}

func (c *testWebSocketConn) readJSON(t *testing.T) map[string]any {
	t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	payload, err := readServerFrame(c.reader)
	if err != nil {
		t.Fatalf("read ws frame: %v", err)
	}

	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("unmarshal ws message: %v", err)
	}
	return message
}

func (c *testWebSocketConn) readMessagesUntil(
	t *testing.T,
	want int,
	match func(message map[string]any) bool,
) []map[string]any {
	t.Helper()

	messages := make([]map[string]any, 0, want)
	deadline := time.Now().Add(2 * time.Second)
	for len(messages) < want && time.Now().Before(deadline) {
		message := c.readJSON(t)
		if match(message) {
			messages = append(messages, message)
		}
	}

	if len(messages) < want {
		t.Fatalf("expected %d matching websocket messages, got %d", want, len(messages))
	}
	return messages
}

func readServerFrame(reader *bufio.Reader) ([]byte, error) {
	header, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if header&0x0f == 0x8 {
		return nil, io.EOF
	}
	if header&0x0f != 0x1 {
		return nil, fmt.Errorf("unexpected opcode %d", header&0x0f)
	}

	lengthByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if lengthByte&0x80 != 0 {
		return nil, fmt.Errorf("server frames must not be masked")
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

	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
