package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCPASenderSendUsesMultipartAuthFilesEndpointAndBearerHeader(t *testing.T) {
	client := &fakeSenderHTTPClient{
		do: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://cpa.example.com/v0/management/auth-files" {
				t.Fatalf("unexpected cpa url: %s", req.URL.String())
			}
			if got := req.Header.Get("Authorization"); got != "Bearer cpa-secret" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			if got := req.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data; boundary=") {
				t.Fatalf("expected multipart content type, got %q", got)
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if !bytes.Contains(body, []byte(`name="file"`)) {
				t.Fatalf("expected multipart file field, got %q", string(body))
			}
			if !bytes.Contains(body, []byte(`user@example.com.json`)) {
				t.Fatalf("expected multipart filename, got %q", string(body))
			}

			return jsonResponse(http.StatusCreated, map[string]any{"message": "ok"}), nil
		},
	}

	sender := NewCPASender(client)
	results, err := sender.Send(context.Background(), SendRequest{
		Service: ServiceConfig{
			ID:         7,
			Kind:       UploadKindCPA,
			BaseURL:    "https://cpa.example.com",
			Credential: "cpa-secret",
		},
		Accounts: []UploadAccount{
			{
				Email:        "user@example.com",
				AccessToken:  "access-1",
				RefreshToken: "refresh-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success || results[0].Kind != UploadKindCPA || results[0].ServiceID != 7 {
		t.Fatalf("unexpected cpa result: %+v", results[0])
	}
}

func TestSub2APISenderSendParsesFailureResponseAndSetsCoreHeaders(t *testing.T) {
	exportedAt := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
	client := &fakeSenderHTTPClient{
		do: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://sub2api.example.com/api/v1/admin/accounts/data" {
				t.Fatalf("unexpected sub2api url: %s", req.URL.String())
			}
			if got := req.Header.Get("x-api-key"); got != "sub2api-key" {
				t.Fatalf("unexpected x-api-key header: %q", got)
			}
			if got := req.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type: %q", got)
			}
			if got := req.Header.Get("Idempotency-Key"); got != "import-2026-04-04T00:00:00Z" {
				t.Fatalf("unexpected idempotency key: %q", got)
			}

			var payload Sub2APIBatchPayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload.Data.Type != "sub2api-data" || len(payload.Data.Accounts) != 1 {
				t.Fatalf("unexpected sub2api payload: %+v", payload)
			}

			return jsonResponse(http.StatusBadRequest, map[string]any{"message": "bad key"}), nil
		},
	}

	sender := NewSub2APISender(client)
	results, err := sender.Send(context.Background(), SendRequest{
		Service: ServiceConfig{
			ID:         9,
			Kind:       UploadKindSub2API,
			BaseURL:    "https://sub2api.example.com",
			Credential: "sub2api-key",
		},
		Accounts: []UploadAccount{
			{
				Email:       "user@example.com",
				AccessToken: "access-1",
			},
		},
		Sub2API: Sub2APIBatchOptions{
			ExportedAt: exportedAt,
		},
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success || results[0].Message != "bad key" || results[0].Kind != UploadKindSub2API {
		t.Fatalf("unexpected sub2api result: %+v", results[0])
	}
}

func TestTMSenderSendUsesImportEndpointAndReturnsSuccess(t *testing.T) {
	client := &fakeSenderHTTPClient{
		do: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://tm.example.com/admin/teams/import" {
				t.Fatalf("unexpected tm url: %s", req.URL.String())
			}
			if got := req.Header.Get("X-API-Key"); got != "tm-key" {
				t.Fatalf("unexpected X-API-Key header: %q", got)
			}
			if got := req.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type: %q", got)
			}

			var payload TMSinglePayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload.ImportType != "single" || payload.Email != "user@example.com" {
				t.Fatalf("unexpected tm payload: %+v", payload)
			}

			return textResponse(http.StatusOK, `{"message":"ok"}`), nil
		},
	}

	sender := NewTMSender(client)
	results, err := sender.Send(context.Background(), SendRequest{
		Service: ServiceConfig{
			ID:         13,
			Kind:       UploadKindTM,
			BaseURL:    "https://tm.example.com",
			Credential: "tm-key",
		},
		Accounts: []UploadAccount{
			{
				Email:       "user@example.com",
				AccessToken: "access-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success || results[0].Kind != UploadKindTM || results[0].ServiceID != 13 {
		t.Fatalf("unexpected tm result: %+v", results[0])
	}
}

type fakeSenderHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (f *fakeSenderHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.do(req)
}

func jsonResponse(statusCode int, payload map[string]any) *http.Response {
	body, _ := json.Marshal(payload)
	return textResponse(statusCode, string(body))
}

func textResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
