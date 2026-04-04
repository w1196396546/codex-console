package ws_test

import (
	"bufio"
	"bytes"
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

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/hibiken/asynq"
)

func TestTaskWebSocketRouteIsMounted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := jobs.NewInMemoryRepository()
	jobService := jobs.NewService(repo, nil)
	registrationService := registration.NewService(jobService)

	job, err := jobService.CreateJob(ctx, jobs.CreateJobParams{
		JobType:   "registration",
		ScopeType: "registration",
		ScopeID:   "scope-route",
		Payload:   []byte(`{"email":"route@example.com"}`),
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	conn := dialRouteTestWebSocket(t, server.URL+"/api/ws/task/"+job.JobID)
	defer conn.close(t)

	message := conn.readJSON(t)
	assertRouteMessageField(t, message, "type", "status")
	assertRouteMessageField(t, message, "task_uuid", job.JobID)
}

func TestBatchWebSocketRouteIsMounted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := jobs.NewInMemoryRepository()
	queue := &routeTestQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)
	batchService := registration.NewBatchService(jobService)

	startResp, err := batchService.StartBatch(ctx, registration.BatchStartRequest{
		Count:            1,
		EmailServiceType: "tempmail",
	})
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}

	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService, batchService, ws.NewBatchHandler(batchService)))
	defer server.Close()

	conn := dialRouteTestWebSocket(t, server.URL+"/api/ws/batch/"+startResp.BatchID)
	defer conn.close(t)

	message := conn.readJSON(t)
	assertRouteMessageField(t, message, "type", "status")
	assertRouteMessageField(t, message, "batch_id", startResp.BatchID)
	assertRouteMessageField(t, message, "status", jobs.StatusRunning)
}

func TestBatchWebSocketRouteSharesBatchService(t *testing.T) {
	t.Parallel()

	repo := jobs.NewInMemoryRepository()
	queue := &routeTestQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService))
	defer server.Close()

	resp, err := nethttp.Post(
		server.URL+"/api/registration/batch",
		"application/json",
		bytes.NewReader([]byte(`{"count":1,"email_service_type":"tempmail"}`)),
	)
	if err != nil {
		t.Fatalf("start batch through http: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected batch start 202, got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch start response: %v", err)
	}

	batchID, ok := payload["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id string, got %#v", payload["batch_id"])
	}

	conn := dialRouteTestWebSocket(t, server.URL+"/api/ws/batch/"+batchID)
	defer conn.close(t)

	message := conn.readJSON(t)
	assertRouteMessageField(t, message, "type", "status")
	assertRouteMessageField(t, message, "batch_id", batchID)
	assertRouteMessageField(t, message, "status", jobs.StatusRunning)
}

func TestBatchWebSocketRouteFallsBackToSharedHandlerWhenBatchServiceMissing(t *testing.T) {
	t.Parallel()

	repo := jobs.NewInMemoryRepository()
	queue := &routeTestQueue{}
	jobService := jobs.NewService(repo, queue)
	registrationService := registration.NewService(jobService)

	server := httptest.NewServer(internalhttp.NewRouter(jobService, registrationService, failingBatchSocketHandler{}))
	defer server.Close()

	resp, err := nethttp.Post(
		server.URL+"/api/registration/batch",
		"application/json",
		bytes.NewReader([]byte(`{"count":1,"email_service_type":"tempmail"}`)),
	)
	if err != nil {
		t.Fatalf("start batch through http: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected batch start 202, got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode batch start response: %v", err)
	}

	batchID, ok := payload["batch_id"].(string)
	if !ok || batchID == "" {
		t.Fatalf("expected batch_id string, got %#v", payload["batch_id"])
	}

	conn := dialRouteTestWebSocket(t, server.URL+"/api/ws/batch/"+batchID)
	defer conn.close(t)

	message := conn.readJSON(t)
	assertRouteMessageField(t, message, "type", "status")
	assertRouteMessageField(t, message, "batch_id", batchID)
	assertRouteMessageField(t, message, "status", jobs.StatusRunning)
}

func assertRouteMessageField(t *testing.T, payload map[string]any, key string, want string) {
	t.Helper()

	got, ok := payload[key].(string)
	if !ok {
		t.Fatalf("expected %s to be string, got %#v", key, payload[key])
	}
	if got != want {
		t.Fatalf("expected %s=%q, got %#v", key, want, payload[key])
	}
}

type routeTestWebSocketConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

type routeTestQueue struct{}

func (q *routeTestQueue) Enqueue(_ context.Context, _ *asynq.Task) error {
	return nil
}

type failingBatchSocketHandler struct{}

func (failingBatchSocketHandler) HandleBatchSocket(w nethttp.ResponseWriter, _ *nethttp.Request) {
	w.WriteHeader(nethttp.StatusTeapot)
}

func dialRouteTestWebSocket(t *testing.T, rawURL string) *routeTestWebSocketConn {
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

	return &routeTestWebSocketConn{
		conn:   conn,
		reader: reader,
	}
}

func (c *routeTestWebSocketConn) close(t *testing.T) {
	t.Helper()
	_ = c.conn.Close()
}

func (c *routeTestWebSocketConn) readJSON(t *testing.T) map[string]any {
	t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	payload, err := readRouteServerFrame(c.reader)
	if err != nil {
		t.Fatalf("read frame payload: %v", err)
	}

	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("unmarshal frame payload: %v", err)
	}
	return message
}

func readRouteServerFrame(reader *bufio.Reader) ([]byte, error) {
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
