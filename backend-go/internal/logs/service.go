package logs

import (
	"context"
	"errors"
	"time"
)

var (
	ErrClearLogsConfirmationRequired = errors.New("请传入 confirm=true 以确认清空日志")
	errRepositoryNotConfigured       = errors.New("logs repository not configured")
	shanghaiLocation                 = mustLoadShanghaiLocation()
)

type Repository interface {
	ListLogs(ctx context.Context, req ListLogsRequest) ([]AppLogRecord, int, error)
	GetStats(ctx context.Context) (LogsStats, error)
	CleanupLogs(ctx context.Context, req CleanupRequest) (CleanupResult, error)
	ClearLogs(ctx context.Context) (int, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListLogs(ctx context.Context, req ListLogsRequest) (ListLogsResponse, error) {
	normalized := req.Normalized()
	if s == nil || s.repository == nil {
		return ListLogsResponse{
			Page:     normalized.Page,
			PageSize: normalized.PageSize,
			Logs:     make([]LogEntry, 0),
		}, nil
	}

	rows, total, err := s.repository.ListLogs(ctx, normalized)
	if err != nil {
		return ListLogsResponse{}, err
	}

	logs := make([]LogEntry, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, LogEntry{
			Level:     row.Level,
			Logger:    row.Logger,
			Message:   row.Message,
			Exception: row.Exception,
			CreatedAt: toShanghaiISO(row.CreatedAt),
		})
	}

	return ListLogsResponse{
		Total:    total,
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Logs:     logs,
	}, nil
}

func (s *Service) GetStats(ctx context.Context) (StatsResponse, error) {
	if s == nil || s.repository == nil {
		return StatsResponse{Levels: map[string]int{}}, nil
	}

	stats, err := s.repository.GetStats(ctx)
	if err != nil {
		return StatsResponse{}, err
	}

	return StatsResponse{
		Total:    stats.Total,
		LatestAt: toShanghaiISOPtr(stats.LatestAt),
		Levels:   cloneLevels(stats.Levels),
	}, nil
}

func (s *Service) CleanupLogs(ctx context.Context, req CleanupRequest) (CleanupResult, error) {
	normalized := req.Normalized()
	if s == nil || s.repository == nil {
		return CleanupResult{}, errRepositoryNotConfigured
	}

	return s.repository.CleanupLogs(ctx, normalized)
}

func (s *Service) ClearLogs(ctx context.Context, confirm bool) (ClearResult, error) {
	if !confirm {
		return ClearResult{}, ErrClearLogsConfirmationRequired
	}
	if s == nil || s.repository == nil {
		return ClearResult{}, errRepositoryNotConfigured
	}

	deletedTotal, err := s.repository.ClearLogs(ctx)
	if err != nil {
		return ClearResult{}, err
	}

	return ClearResult{
		DeletedTotal: deletedTotal,
		Remaining:    0,
	}, nil
}

func mustLoadShanghaiLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}

func toShanghaiISO(value time.Time) string {
	return value.In(shanghaiLocation).Format(time.RFC3339)
}

func toShanghaiISOPtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := toShanghaiISO(*value)
	return &formatted
}

func cloneLevels(levels map[string]int) map[string]int {
	if len(levels) == 0 {
		return map[string]int{}
	}
	cloned := make(map[string]int, len(levels))
	for key, value := range levels {
		cloned[key] = value
	}
	return cloned
}
