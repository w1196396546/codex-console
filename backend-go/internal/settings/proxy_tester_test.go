package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProxyTesterTestDynamicProxyFetchesAndVerifiesProxy(t *testing.T) {
	t.Parallel()

	const verifyURL = "http://ip-check.invalid/json"

	proxyRequests := 0
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		if got := r.URL.String(); got != verifyURL {
			t.Fatalf("expected proxy verification url %q, got %q", verifyURL, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ip": "1.2.3.4"})
	}))
	defer proxyServer.Close()

	apiRequests := 0
	apiKey := "saved-key"
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiRequests++
		if got := r.Header.Get("X-Proxy-Key"); got != apiKey {
			t.Fatalf("expected dynamic proxy api key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"proxy": strings.TrimPrefix(proxyServer.URL, "http://"),
			},
		})
	}))
	defer apiServer.Close()

	tester := NewHTTPProxyTester(HTTPProxyTesterOptions{
		VerifyURL: verifyURL,
	})

	resp, err := tester.TestDynamicProxy(context.Background(), UpdateDynamicProxySettingsRequest{
		APIURL:       apiServer.URL,
		APIKey:       &apiKey,
		APIKeyHeader: "X-Proxy-Key",
		ResultField:  "data.proxy",
	})
	if err != nil {
		t.Fatalf("test dynamic proxy: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected dynamic proxy success, got %+v", resp)
	}
	if resp.ProxyURL != proxyServer.URL {
		t.Fatalf("expected normalized proxy url %q, got %q", proxyServer.URL, resp.ProxyURL)
	}
	if resp.IP != "1.2.3.4" {
		t.Fatalf("expected verification ip, got %+v", resp)
	}
	if apiRequests != 1 || proxyRequests != 1 {
		t.Fatalf("expected one api request and one proxy verification, got api=%d proxy=%d", apiRequests, proxyRequests)
	}
}

func TestHTTPProxyTesterTestProxyVerifiesProxyRecord(t *testing.T) {
	t.Parallel()

	const verifyURL = "http://ip-check.invalid/json"

	proxyRequests := 0
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		if got := r.URL.String(); got != verifyURL {
			t.Fatalf("expected proxy verification url %q, got %q", verifyURL, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ip": "5.6.7.8"})
	}))
	defer proxyServer.Close()

	tester := NewHTTPProxyTester(HTTPProxyTesterOptions{
		VerifyURL: verifyURL,
	})

	resp, err := tester.TestProxy(context.Background(), ProxyRecord{
		ID:       7,
		Name:     "default-proxy",
		Type:     "http",
		Host:     "127.0.0.1",
		Port:     7890,
		ProxyURL: proxyServer.URL,
	})
	if err != nil {
		t.Fatalf("test proxy: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected proxy success, got %+v", resp)
	}
	if resp.ID != 7 || resp.Name != "default-proxy" {
		t.Fatalf("expected proxy identity to round-trip, got %+v", resp)
	}
	if resp.IP != "5.6.7.8" {
		t.Fatalf("expected proxy exit ip, got %+v", resp)
	}
	if proxyRequests != 1 {
		t.Fatalf("expected one proxy verification request, got %d", proxyRequests)
	}
}
