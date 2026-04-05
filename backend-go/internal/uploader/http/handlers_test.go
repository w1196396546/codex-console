package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	uploaderhttp "github.com/dou-jiang/codex-console/backend-go/internal/uploader/http"
	"github.com/go-chi/chi/v5"
)

func TestUploadHandlersPreserveCompatibilityRoutesAndPayloads(t *testing.T) {
	service := &fakeUploaderHTTPService{
		cpaList: []uploader.CPAServiceResponse{
			{ID: 1, Name: "CPA One", APIURL: "https://cpa.example.com", HasToken: true, Enabled: true, Priority: 3},
		},
		sub2apiFull: uploader.Sub2APIServiceFullResponse{
			ID:         2,
			Name:       "Sub2API One",
			APIURL:     "https://sub2api.example.com",
			APIKey:     "sub2api-key",
			TargetType: "newapi",
			Enabled:    true,
			Priority:   6,
		},
		testResult: uploader.ConnectionTestResult{Success: true, Message: "连接成功"},
		uploadResult: uploader.Sub2APIUploadResult{
			SuccessCount: 1,
			FailedCount:  0,
			SkippedCount: 0,
			Details: []uploader.Sub2APIUploadDetail{
				{ID: 99, Success: true, Message: "成功上传 1 个账号"},
			},
		},
	}
	router := chi.NewRouter()
	uploaderhttp.NewHandler(service).RegisterRoutes(router)

	t.Run("list cpa services returns plain array", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/cpa-services?enabled=true", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var payload []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode cpa list response: %v", err)
		}
		if len(payload) != 1 || payload[0]["has_token"] != true {
			t.Fatalf("unexpected cpa list payload: %#v", payload)
		}
		if _, exists := payload[0]["api_token"]; exists {
			t.Fatalf("did not expect api_token in cpa list payload: %#v", payload[0])
		}
	})

	t.Run("sub2api full exposes key and target_type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sub2api-services/2/full", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode sub2api full response: %v", err)
		}
		if payload["api_key"] != "sub2api-key" || payload["target_type"] != "newapi" {
			t.Fatalf("unexpected sub2api full payload: %#v", payload)
		}
	})

	t.Run("tm test-connection delegates to service action", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tm-services/test-connection", strings.NewReader(`{"api_url":"https://tm.example.com","api_key":"tm-key"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode tm test response: %v", err)
		}
		if payload["success"] != true || payload["message"] != "连接成功" {
			t.Fatalf("unexpected tm test payload: %#v", payload)
		}
	})

	t.Run("sub2api upload returns python-compatible counters", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sub2api-services/upload", strings.NewReader(`{"account_ids":[99],"concurrency":5,"priority":80}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode sub2api upload response: %v", err)
		}
		if payload["success_count"] != float64(1) || payload["failed_count"] != float64(0) || payload["skipped_count"] != float64(0) {
			t.Fatalf("unexpected sub2api upload payload: %#v", payload)
		}
	})

	if service.lastEnabledFilter == nil || !*service.lastEnabledFilter {
		t.Fatalf("expected enabled=true query to propagate, got %#v", service.lastEnabledFilter)
	}
	if service.lastUploadRequest.Concurrency != 5 || service.lastUploadRequest.Priority != 80 || len(service.lastUploadRequest.AccountIDs) != 1 || service.lastUploadRequest.AccountIDs[0] != 99 {
		t.Fatalf("unexpected upload request: %+v", service.lastUploadRequest)
	}
}

func TestUploadHandlersReturnJSONDetailErrorsForValidationFailures(t *testing.T) {
	router := chi.NewRouter()
	uploaderhttp.NewHandler(&fakeUploaderHTTPService{}).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cpa-services/test-connection", strings.NewReader(`{"api_url":"https://cpa.example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode validation error response: %v", err)
	}
	if payload["detail"] != "api_url 和 api_token 不能为空" {
		t.Fatalf("unexpected validation error payload: %#v", payload)
	}
}

type fakeUploaderHTTPService struct {
	cpaList      []uploader.CPAServiceResponse
	sub2apiFull  uploader.Sub2APIServiceFullResponse
	testResult   uploader.ConnectionTestResult
	uploadResult uploader.Sub2APIUploadResult

	lastEnabledFilter *bool
	lastUploadRequest uploader.Sub2APIUploadRequest
}

func (f *fakeUploaderHTTPService) ListCPAServices(_ context.Context, enabled *bool) ([]uploader.CPAServiceResponse, error) {
	f.lastEnabledFilter = enabled
	return f.cpaList, nil
}

func (f *fakeUploaderHTTPService) CreateCPAService(context.Context, uploader.CreateCPAServiceRequest) (uploader.CPAServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) GetCPAService(context.Context, int) (uploader.CPAServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) GetCPAServiceFull(context.Context, int) (uploader.CPAServiceFullResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) UpdateCPAService(context.Context, int, uploader.UpdateCPAServiceRequest) (uploader.CPAServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) DeleteCPAService(context.Context, int) (uploader.DeleteServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) TestCPAService(context.Context, int) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}

func (f *fakeUploaderHTTPService) TestCPAConnection(context.Context, uploader.CPAConnectionTestRequest) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}

func (f *fakeUploaderHTTPService) ListSub2APIServices(context.Context, *bool) ([]uploader.Sub2APIServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) CreateSub2APIService(context.Context, uploader.CreateSub2APIServiceRequest) (uploader.Sub2APIServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) GetSub2APIService(context.Context, int) (uploader.Sub2APIServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) GetSub2APIServiceFull(context.Context, int) (uploader.Sub2APIServiceFullResponse, error) {
	return f.sub2apiFull, nil
}

func (f *fakeUploaderHTTPService) UpdateSub2APIService(context.Context, int, uploader.UpdateSub2APIServiceRequest) (uploader.Sub2APIServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) DeleteSub2APIService(context.Context, int) (uploader.DeleteServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) TestSub2APIService(context.Context, int) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}

func (f *fakeUploaderHTTPService) TestSub2APIConnection(context.Context, uploader.Sub2APIConnectionTestRequest) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}

func (f *fakeUploaderHTTPService) UploadSub2API(_ context.Context, req uploader.Sub2APIUploadRequest) (uploader.Sub2APIUploadResult, error) {
	f.lastUploadRequest = req
	return f.uploadResult, nil
}

func (f *fakeUploaderHTTPService) ListTMServices(context.Context, *bool) ([]uploader.TMServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) CreateTMService(context.Context, uploader.CreateTMServiceRequest) (uploader.TMServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) GetTMService(context.Context, int) (uploader.TMServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) UpdateTMService(context.Context, int, uploader.UpdateTMServiceRequest) (uploader.TMServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) DeleteTMService(context.Context, int) (uploader.DeleteServiceResponse, error) {
	panic("not used")
}

func (f *fakeUploaderHTTPService) TestTMService(context.Context, int) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}

func (f *fakeUploaderHTTPService) TestTMConnection(context.Context, uploader.TMConnectionTestRequest) (uploader.ConnectionTestResult, error) {
	return f.testResult, nil
}
