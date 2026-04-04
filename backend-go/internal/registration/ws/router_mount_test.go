package ws_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
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

	header := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		t.Fatalf("read frame header: %v", err)
	}

	payloadLength := int(header[1] & 0x7f)
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		t.Fatalf("read frame payload: %v", err)
	}

	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("unmarshal frame payload: %v", err)
	}
	return message
}
