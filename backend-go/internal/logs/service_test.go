package logs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLogsServiceListPreservesCompatibilityFiltersAndShanghaiTime(t *testing.T) {
	createdAt := time.Date(2026, 4, 5, 1, 2, 3, 0, time.UTC)
	repo := &fakeRepository{
		listRows: []AppLogRecord{
			{
				ID:        42,
				Level:     "ERROR",
				Logger:    "auth.service",
				Module:    "login",
				Message:   "token expired",
				Exception: "traceback",
				CreatedAt: createdAt,
			},
		},
		listTotal: 1,
	}

	service := NewService(repo)
	resp, err := service.ListLogs(context.Background(), ListLogsRequest{
		Page:         2,
		PageSize:     999,
		Level:        " error ",
		LoggerName:   " auth.service ",
		Keyword:      " token ",
		SinceMinutes: 20000,
	})
	if err != nil {
		t.Fatalf("unexpected list logs error: %v", err)
	}

	if repo.listReq.Page != 2 {
		t.Fatalf("expected page=2, got %+v", repo.listReq)
	}
	if repo.listReq.PageSize != 500 {
		t.Fatalf("expected page_size to clamp at 500, got %+v", repo.listReq)
	}
	if repo.listReq.Level != "ERROR" || repo.listReq.LoggerName != "auth.service" || repo.listReq.Keyword != "token" {
		t.Fatalf("expected normalized filters, got %+v", repo.listReq)
	}
	if repo.listReq.SinceMinutes != 10080 {
		t.Fatalf("expected since_minutes to clamp at 10080, got %+v", repo.listReq)
	}

	if resp.Total != 1 || resp.Page != 2 || resp.PageSize != 500 {
		t.Fatalf("unexpected response pagination: %+v", resp)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("expected one log row, got %+v", resp.Logs)
	}
	if resp.Logs[0].CreatedAt != "2026-04-05T09:02:03+08:00" {
		t.Fatalf("expected Shanghai timestamp, got %+v", resp.Logs[0])
	}
	if resp.Logs[0].Logger != "auth.service" || resp.Logs[0].Exception != "traceback" {
		t.Fatalf("expected compatibility fields, got %+v", resp.Logs[0])
	}
}

func TestLogsRepositoryBuildsCompatibilityFilterQuery(t *testing.T) {
	now := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)
	query, args := buildListLogsQuery(ListLogsRequest{
		Page:         3,
		PageSize:     120,
		Level:        "ERROR",
		LoggerName:   "worker",
		Keyword:      "boom",
		SinceMinutes: 30,
	}, now)

	requiredSnippets := []string{
		"FROM app_logs",
		"level = $1",
		"logger ILIKE $2",
		"(message ILIKE $3 OR logger ILIKE $3 OR module ILIKE $3)",
		"created_at >= $4",
		"ORDER BY created_at DESC, id DESC",
		"LIMIT $5 OFFSET $6",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(query, snippet) {
			t.Fatalf("expected query to contain %q, got %q", snippet, query)
		}
	}

	if len(args) != 6 {
		t.Fatalf("expected six query args, got %#v", args)
	}
	if args[0] != "ERROR" || args[1] != "%worker%" || args[2] != "%boom%" {
		t.Fatalf("unexpected filter args: %#v", args)
	}
	sinceAt, ok := args[3].(time.Time)
	if !ok || !sinceAt.Equal(now.Add(-30*time.Minute)) {
		t.Fatalf("expected since_at=%v, got %#v", now.Add(-30*time.Minute), args[3])
	}
	if args[4] != 120 || args[5] != 240 {
		t.Fatalf("expected limit/offset args, got %#v", args)
	}
}

func TestStatsServiceReturnsCompatibilityShape(t *testing.T) {
	latestAt := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	service := NewService(&fakeRepository{
		stats: LogsStats{
			Total:    9,
			LatestAt: &latestAt,
			Levels: map[string]int{
				"INFO":     4,
				"WARNING":  2,
				"ERROR":    2,
				"CRITICAL": 1,
			},
		},
	})

	resp, err := service.GetStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected stats error: %v", err)
	}

	if resp.Total != 9 {
		t.Fatalf("expected total=9, got %+v", resp)
	}
	if resp.LatestAt == nil || *resp.LatestAt != "2026-04-05T14:07:08+08:00" {
		t.Fatalf("expected Shanghai latest_at, got %+v", resp)
	}
	if resp.Levels["CRITICAL"] != 1 || resp.Levels["INFO"] != 4 {
		t.Fatalf("unexpected level counts: %+v", resp.Levels)
	}
}

func TestStatsServiceReturnsRepositoryError(t *testing.T) {
	expectedErr := errors.New("boom")
	service := NewService(&fakeRepository{statsErr: expectedErr})

	_, err := service.GetStats(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected stats error %v, got %v", expectedErr, err)
	}
}

func TestCleanupLogsServiceAppliesCompatibilityDefaults(t *testing.T) {
	repo := &fakeRepository{
		cleanupResult: CleanupResult{
			RetentionDays: 30,
			MaxRows:       50000,
			DeletedTotal:  4,
			Remaining:     9,
		},
	}
	service := NewService(repo)

	resp, err := service.CleanupLogs(context.Background(), CleanupRequest{})
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}

	if repo.cleanupReq.RetentionDays == nil || *repo.cleanupReq.RetentionDays != 30 {
		t.Fatalf("expected default retention_days=30, got %+v", repo.cleanupReq)
	}
	if repo.cleanupReq.MaxRows != 50000 {
		t.Fatalf("expected default max_rows=50000, got %+v", repo.cleanupReq)
	}
	if resp.DeletedTotal != 4 || resp.Remaining != 9 {
		t.Fatalf("unexpected cleanup response: %+v", resp)
	}
}

func TestClearLogsServiceRequiresConfirm(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, err := service.ClearLogs(context.Background(), false)
	if !errors.Is(err, ErrClearLogsConfirmationRequired) {
		t.Fatalf("expected confirmation error %v, got %v", ErrClearLogsConfirmationRequired, err)
	}
}

func TestClearLogsServiceReturnsCompatibilityPayload(t *testing.T) {
	service := NewService(&fakeRepository{clearDeletedTotal: 12})

	resp, err := service.ClearLogs(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected clear error: %v", err)
	}

	if resp.DeletedTotal != 12 || resp.Remaining != 0 {
		t.Fatalf("unexpected clear response: %+v", resp)
	}
}

type fakeRepository struct {
	listReq   ListLogsRequest
	listRows  []AppLogRecord
	listTotal int
	listErr   error

	stats    LogsStats
	statsErr error

	cleanupReq    CleanupRequest
	cleanupResult CleanupResult
	cleanupErr    error

	clearDeletedTotal int
	clearErr          error
}

func (f *fakeRepository) ListLogs(_ context.Context, req ListLogsRequest) ([]AppLogRecord, int, error) {
	f.listReq = req
	return f.listRows, f.listTotal, f.listErr
}

func (f *fakeRepository) GetStats(context.Context) (LogsStats, error) {
	return f.stats, f.statsErr
}

func (f *fakeRepository) CleanupLogs(_ context.Context, req CleanupRequest) (CleanupResult, error) {
	f.cleanupReq = req
	return f.cleanupResult, f.cleanupErr
}

func (f *fakeRepository) ClearLogs(context.Context) (int, error) {
	return f.clearDeletedTotal, f.clearErr
}
