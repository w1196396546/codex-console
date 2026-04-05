package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	logspkg "github.com/dou-jiang/codex-console/backend-go/internal/logs"
	logshttp "github.com/dou-jiang/codex-console/backend-go/internal/logs/http"
	"github.com/go-chi/chi/v5"
)

func TestLogsHandlerCompatibilityRoutes(t *testing.T) {
	latestAt := "2026-04-05T14:07:08+08:00"
	service := &fakeLogsService{
		listResponse: logspkg.ListLogsResponse{
			Total:    1,
			Page:     1,
			PageSize: 100,
			Logs: []logspkg.LogEntry{
				{
					Level:     "ERROR",
					Logger:    "auth.service",
					Message:   "token expired",
					Exception: "traceback",
					CreatedAt: "2026-04-05T09:02:03+08:00",
				},
			},
		},
		statsResponse: logspkg.StatsResponse{
			Total:    9,
			LatestAt: &latestAt,
			Levels: map[string]int{
				"INFO":  4,
				"ERROR": 5,
			},
		},
	}
	router := newTestRouter(service)

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/logs?page=2&page_size=50&level=ERROR&logger_name=auth&keyword=token&since_minutes=60", nil)
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d", listRec.Code)
	}
	if service.listReq.Page != 2 || service.listReq.PageSize != 50 || service.listReq.SinceMinutes != 60 {
		t.Fatalf("unexpected list request: %+v", service.listReq)
	}
	if service.listReq.Level != "ERROR" || service.listReq.LoggerName != "auth" || service.listReq.Keyword != "token" {
		t.Fatalf("unexpected list filters: %+v", service.listReq)
	}

	var listPayload map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if listPayload["total"] != float64(1) || listPayload["page_size"] != float64(100) {
		t.Fatalf("unexpected list payload: %#v", listPayload)
	}

	statsRec := httptest.NewRecorder()
	statsReq := httptest.NewRequest(http.MethodGet, "/api/logs/stats", nil)
	router.ServeHTTP(statsRec, statsReq)

	if statsRec.Code != http.StatusOK {
		t.Fatalf("expected stats 200, got %d", statsRec.Code)
	}

	var statsPayload map[string]any
	if err := json.Unmarshal(statsRec.Body.Bytes(), &statsPayload); err != nil {
		t.Fatalf("decode stats payload: %v", err)
	}
	if statsPayload["total"] != float64(9) || statsPayload["latest_at"] != latestAt {
		t.Fatalf("unexpected stats payload: %#v", statsPayload)
	}
}

func TestCleanupHandlerPreservesRequestShapeAndResponse(t *testing.T) {
	service := &fakeLogsService{
		cleanupResponse: logspkg.CleanupResult{
			RetentionDays:  15,
			MaxRows:        90000,
			DeletedByAge:   3,
			DeletedByLimit: 2,
			DeletedTotal:   5,
			Remaining:      7,
		},
	}
	router := newTestRouter(service)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/logs/cleanup", bytes.NewReader([]byte(`{"retention_days":15,"max_rows":90000}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected cleanup 200, got %d", rec.Code)
	}
	if service.cleanupReq.RetentionDays == nil || *service.cleanupReq.RetentionDays != 15 || service.cleanupReq.MaxRows != 90000 {
		t.Fatalf("unexpected cleanup request: %+v", service.cleanupReq)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode cleanup payload: %v", err)
	}
	if payload["success"] != true || payload["deleted_total"] != float64(5) || payload["remaining"] != float64(7) {
		t.Fatalf("unexpected cleanup payload: %#v", payload)
	}
}

func TestCleanupHandlerRejectsOutOfRangeValues(t *testing.T) {
	router := newTestRouter(&fakeLogsService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/logs/cleanup", bytes.NewReader([]byte(`{"retention_days":0,"max_rows":99}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected cleanup 400, got %d", rec.Code)
	}
	assertDetailMessage(t, rec.Body.Bytes(), "retention_days 必须在 1 到 3650 之间")
}

func TestClearHandlerRequiresConfirm(t *testing.T) {
	router := newTestRouter(&fakeLogsService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/logs", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected clear 400, got %d", rec.Code)
	}
	assertDetailMessage(t, rec.Body.Bytes(), "请传入 confirm=true 以确认清空日志")
}

func TestClearHandlerReturnsCompatibilityPayload(t *testing.T) {
	router := newTestRouter(&fakeLogsService{
		clearResponse: logspkg.ClearResult{
			DeletedTotal: 8,
			Remaining:    0,
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/logs?confirm=true", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode clear payload: %v", err)
	}
	if payload["success"] != true || payload["deleted_total"] != float64(8) || payload["remaining"] != float64(0) {
		t.Fatalf("unexpected clear payload: %#v", payload)
	}
}

func newTestRouter(service *fakeLogsService) http.Handler {
	router := chi.NewRouter()
	logshttp.NewHandler(service).RegisterRoutes(router)
	return router
}

func assertDetailMessage(t *testing.T, body []byte, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["detail"] != want {
		t.Fatalf("expected detail %q, got %#v", want, payload["detail"])
	}
}

type fakeLogsService struct {
	listReq         logspkg.ListLogsRequest
	listResponse    logspkg.ListLogsResponse
	listErr         error
	statsResponse   logspkg.StatsResponse
	statsErr        error
	cleanupReq      logspkg.CleanupRequest
	cleanupResponse logspkg.CleanupResult
	cleanupErr      error
	clearResponse   logspkg.ClearResult
	clearErr        error
}

func (f *fakeLogsService) ListLogs(_ context.Context, req logspkg.ListLogsRequest) (logspkg.ListLogsResponse, error) {
	f.listReq = req
	return f.listResponse, f.listErr
}

func (f *fakeLogsService) GetStats(context.Context) (logspkg.StatsResponse, error) {
	return f.statsResponse, f.statsErr
}

func (f *fakeLogsService) CleanupLogs(_ context.Context, req logspkg.CleanupRequest) (logspkg.CleanupResult, error) {
	f.cleanupReq = req
	return f.cleanupResponse, f.cleanupErr
}

func (f *fakeLogsService) ClearLogs(_ context.Context, confirm bool) (logspkg.ClearResult, error) {
	if !confirm {
		return logspkg.ClearResult{}, logspkg.ErrClearLogsConfirmationRequired
	}
	return f.clearResponse, f.clearErr
}
