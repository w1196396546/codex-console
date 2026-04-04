package e2e_test

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealthzEndpoint(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("BACKEND_GO_BASE_URL"))
	if baseURL == "" {
		t.Skip("set BACKEND_GO_BASE_URL to enable the minimal healthz e2e check")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read healthz body: %v", err)
	}
	if strings.TrimSpace(string(body)) != "ok" {
		t.Fatalf("expected healthz body ok, got %q", string(body))
	}
}
