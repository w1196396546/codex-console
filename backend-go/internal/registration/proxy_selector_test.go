package registration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type proxySelectorTestSettings struct {
	settings map[string]string
}

func (s proxySelectorTestSettings) GetSettings(context.Context, []string) (map[string]string, error) {
	return s.settings, nil
}

func TestPostgresProxySelectorFallsBackWhenDynamicProxyIsUnreachable(t *testing.T) {
	t.Parallel()

	proxyAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("127.0.0.1:1"))
	}))
	defer proxyAPI.Close()

	selector := NewPostgresProxySelector(nil, proxySelectorTestSettings{
		settings: map[string]string{
			"proxy.enabled":                "false",
			"proxy.dynamic_enabled":        "true",
			"proxy.dynamic_api_url":        proxyAPI.URL,
			"proxy.dynamic_api_key":        "",
			"proxy.dynamic_api_key_header": "X-API-Key",
			"proxy.dynamic_result_field":   "",
		},
	})

	selection, err := selector.SelectProxy(context.Background(), StartRequest{})
	if err != nil {
		t.Fatalf("select proxy: %v", err)
	}
	if selection.Selected != "" {
		t.Fatalf("expected no selected proxy after failed validation, got %+v", selection)
	}
	if selection.Source != "unassigned" {
		t.Fatalf("expected unassigned source after failed validation, got %+v", selection)
	}
	if !strings.Contains(selection.Note, "dynamic proxy unavailable") {
		t.Fatalf("expected validation failure note, got %+v", selection)
	}
}
