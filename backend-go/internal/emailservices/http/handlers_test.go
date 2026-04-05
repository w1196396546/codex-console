package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	emailservices "github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	emailhttp "github.com/dou-jiang/codex-console/backend-go/internal/emailservices/http"
	"github.com/go-chi/chi/v5"
)

func TestEmailServicesHandlersExposeCompatibilityRoutes(t *testing.T) {
	t.Parallel()

	service := &fakeEmailServicesService{
		listResponse: emailservices.EmailServiceListResponse{
			Total: 1,
			Services: []emailservices.EmailServiceResponse{
				{ID: 11, Name: "duck-primary", ServiceType: emailservices.ServiceTypeDuckMail, Enabled: true, Priority: 1},
			},
		},
		fullResponse: emailservices.EmailServiceFullResponse{
			ID: 11, Name: "duck-primary", ServiceType: emailservices.ServiceTypeDuckMail, Enabled: true, Priority: 1, Config: map[string]any{"api_key": "secret"},
		},
		statsResponse: emailservices.StatsResponse{OutlookCount: 2, EnabledCount: 3},
		typeResponse: emailservices.ServiceTypesResponse{
			Types: []emailservices.ServiceTypeDefinition{{Value: emailservices.ServiceTypeOutlook, Label: "Outlook"}},
		},
		actionResponse: emailservices.ActionResponse{Success: true, Message: "ok"},
		testResponse:   emailservices.ServiceTestResult{Success: true, Message: "服务连接正常"},
		outlookImportResponse: emailservices.OutlookBatchImportResponse{
			Total: 2, Success: 1, Failed: 1, Accounts: []map[string]any{{"id": 9, "email": "new@example.com"}}, Errors: []string{"行 2: 邮箱已存在: old@example.com"},
		},
		outlookDeleteResponse: emailservices.BatchDeleteResponse{Success: true, Deleted: 2, Message: "已删除 2 个服务"},
		tempmailResponse:      emailservices.ServiceTestResult{Success: true, Message: "临时邮箱连接正常"},
	}

	router := chi.NewRouter()
	emailhttp.NewHandler(service).RegisterRoutes(router)

	t.Run("read routes", func(t *testing.T) {
		assertJSONStatus(t, router, http.MethodGet, "/api/email-services", nil, http.StatusOK, func(payload map[string]any) {
			if payload["total"] != float64(1) {
				t.Fatalf("unexpected list payload: %#v", payload)
			}
		})
		assertJSONStatus(t, router, http.MethodGet, "/api/email-services/stats", nil, http.StatusOK, func(payload map[string]any) {
			if payload["enabled_count"] != float64(3) {
				t.Fatalf("unexpected stats payload: %#v", payload)
			}
		})
		assertJSONStatus(t, router, http.MethodGet, "/api/email-services/types", nil, http.StatusOK, func(payload map[string]any) {
			types, ok := payload["types"].([]any)
			if !ok || len(types) != 1 {
				t.Fatalf("unexpected type payload: %#v", payload)
			}
		})
		assertJSONStatus(t, router, http.MethodGet, "/api/email-services/11/full", nil, http.StatusOK, func(payload map[string]any) {
			if payload["id"] != float64(11) {
				t.Fatalf("unexpected full payload: %#v", payload)
			}
		})
	})

	t.Run("write routes", func(t *testing.T) {
		assertJSONStatus(t, router, http.MethodPost, "/api/email-services", map[string]any{
			"service_type": "duck_mail",
			"name":         "duck-primary",
			"config":       map[string]any{"base_url": "https://duckmail.example"},
			"enabled":      true,
			"priority":     1,
		}, http.StatusOK, func(payload map[string]any) {
			if payload["name"] != "duck-primary" {
				t.Fatalf("unexpected create payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodPatch, "/api/email-services/11", map[string]any{
			"name":     "duck-updated",
			"enabled":  false,
			"priority": 2,
			"config":   map[string]any{"api_key": "updated"},
		}, http.StatusOK, func(payload map[string]any) {
			if payload["name"] != "duck-primary" {
				t.Fatalf("unexpected update payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodDelete, "/api/email-services/11", nil, http.StatusOK, func(payload map[string]any) {
			if payload["success"] != true {
				t.Fatalf("unexpected delete payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/11/test", nil, http.StatusOK, func(payload map[string]any) {
			if payload["success"] != true {
				t.Fatalf("unexpected test payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/11/enable", nil, http.StatusOK, func(payload map[string]any) {
			if payload["message"] != "ok" {
				t.Fatalf("unexpected enable payload: %#v", payload)
			}
		})
		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/11/disable", nil, http.StatusOK, func(payload map[string]any) {
			if payload["message"] != "ok" {
				t.Fatalf("unexpected disable payload: %#v", payload)
			}
		})
		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/reorder", []int{11, 12, 13}, http.StatusOK, func(payload map[string]any) {
			if payload["success"] != true {
				t.Fatalf("unexpected reorder payload: %#v", payload)
			}
		})
	})

	t.Run("outlook and tempmail helpers", func(t *testing.T) {
		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/outlook/batch-import", map[string]any{
			"data":     "new@example.com----secret",
			"enabled":  true,
			"priority": 0,
		}, http.StatusOK, func(payload map[string]any) {
			if payload["success"] != float64(1) {
				t.Fatalf("unexpected batch import payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodDelete, "/api/email-services/outlook/batch", []int{9, 10}, http.StatusOK, func(payload map[string]any) {
			if payload["deleted"] != float64(2) {
				t.Fatalf("unexpected batch delete payload: %#v", payload)
			}
		})

		assertJSONStatus(t, router, http.MethodPost, "/api/email-services/test-tempmail", map[string]any{
			"provider": "tempmail",
		}, http.StatusOK, func(payload map[string]any) {
			if payload["message"] != "临时邮箱连接正常" {
				t.Fatalf("unexpected tempmail payload: %#v", payload)
			}
		})
	})
}

func TestEmailServicesHandlersReturnChineseJSONErrors(t *testing.T) {
	t.Parallel()

	service := &fakeEmailServicesService{
		createErr: emailservices.ErrInvalidServiceType,
		updateErr: emailservices.ErrServiceNotFound,
	}

	router := chi.NewRouter()
	emailhttp.NewHandler(service).RegisterRoutes(router)

	assertJSONStatus(t, router, http.MethodPost, "/api/email-services", map[string]any{
		"service_type": "bad-type",
		"name":         "broken",
		"config":       map[string]any{},
	}, http.StatusBadRequest, func(payload map[string]any) {
		if payload["detail"] == "" {
			t.Fatalf("expected json detail error, got %#v", payload)
		}
	})

	assertJSONStatus(t, router, http.MethodPatch, "/api/email-services/404", map[string]any{
		"name": "missing",
	}, http.StatusNotFound, func(payload map[string]any) {
		if payload["detail"] != "服务不存在" {
			t.Fatalf("unexpected not found payload: %#v", payload)
		}
	})
}

type fakeEmailServicesService struct {
	listResponse          emailservices.EmailServiceListResponse
	fullResponse          emailservices.EmailServiceFullResponse
	statsResponse         emailservices.StatsResponse
	typeResponse          emailservices.ServiceTypesResponse
	actionResponse        emailservices.ActionResponse
	testResponse          emailservices.ServiceTestResult
	outlookImportResponse emailservices.OutlookBatchImportResponse
	outlookDeleteResponse emailservices.BatchDeleteResponse
	tempmailResponse      emailservices.ServiceTestResult
	createErr             error
	updateErr             error
}

func (f *fakeEmailServicesService) ListServices(context.Context, emailservices.ListServicesRequest) (emailservices.EmailServiceListResponse, error) {
	return f.listResponse, nil
}

func (f *fakeEmailServicesService) GetService(context.Context, int) (emailservices.EmailServiceResponse, error) {
	if len(f.listResponse.Services) == 0 {
		return emailservices.EmailServiceResponse{}, nil
	}
	return f.listResponse.Services[0], nil
}

func (f *fakeEmailServicesService) GetServiceFull(context.Context, int) (emailservices.EmailServiceFullResponse, error) {
	return f.fullResponse, nil
}

func (f *fakeEmailServicesService) GetStats(context.Context) (emailservices.StatsResponse, error) {
	return f.statsResponse, nil
}

func (f *fakeEmailServicesService) GetServiceTypes() emailservices.ServiceTypesResponse {
	return f.typeResponse
}

func (f *fakeEmailServicesService) CreateService(context.Context, emailservices.CreateServiceRequest) (emailservices.EmailServiceResponse, error) {
	if f.createErr != nil {
		return emailservices.EmailServiceResponse{}, f.createErr
	}
	if len(f.listResponse.Services) == 0 {
		return emailservices.EmailServiceResponse{}, nil
	}
	return f.listResponse.Services[0], nil
}

func (f *fakeEmailServicesService) UpdateService(context.Context, int, emailservices.UpdateServiceRequest) (emailservices.EmailServiceResponse, error) {
	if f.updateErr != nil {
		return emailservices.EmailServiceResponse{}, f.updateErr
	}
	if len(f.listResponse.Services) == 0 {
		return emailservices.EmailServiceResponse{}, nil
	}
	return f.listResponse.Services[0], nil
}

func (f *fakeEmailServicesService) DeleteService(context.Context, int) (emailservices.ActionResponse, error) {
	return f.actionResponse, nil
}

func (f *fakeEmailServicesService) TestService(context.Context, int) (emailservices.ServiceTestResult, error) {
	return f.testResponse, nil
}

func (f *fakeEmailServicesService) EnableService(context.Context, int) (emailservices.ActionResponse, error) {
	return f.actionResponse, nil
}

func (f *fakeEmailServicesService) DisableService(context.Context, int) (emailservices.ActionResponse, error) {
	return f.actionResponse, nil
}

func (f *fakeEmailServicesService) ReorderServices(context.Context, []int) (emailservices.ActionResponse, error) {
	return f.actionResponse, nil
}

func (f *fakeEmailServicesService) BatchImportOutlook(context.Context, emailservices.OutlookBatchImportRequest) (emailservices.OutlookBatchImportResponse, error) {
	return f.outlookImportResponse, nil
}

func (f *fakeEmailServicesService) BatchDeleteOutlook(context.Context, []int) (emailservices.BatchDeleteResponse, error) {
	return f.outlookDeleteResponse, nil
}

func (f *fakeEmailServicesService) TestTempmail(context.Context, emailservices.TempmailTestRequest) (emailservices.ServiceTestResult, error) {
	return f.tempmailResponse, nil
}

func assertJSONStatus(t *testing.T, router http.Handler, method string, path string, body any, wantStatus int, check func(map[string]any)) {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("%s %s expected status %d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}

	payload := make(map[string]any)
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("%s %s decode json: %v body=%s", method, path, err, rec.Body.String())
	}
	check(payload)
}
