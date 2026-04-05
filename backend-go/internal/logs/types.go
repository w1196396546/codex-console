package logs

import (
	"strings"
	"time"
)

const (
	defaultLogsPage         = 1
	defaultLogsPageSize     = 100
	maxLogsPageSize         = 500
	maxLogsSinceMinutes     = 10080
	defaultCleanupRetention = 30
	minCleanupRetention     = 1
	maxCleanupRetention     = 3650
	defaultCleanupMaxRows   = 50000
	minCleanupMaxRows       = 1000
	maxCleanupMaxRows       = 5000000
)

type ListLogsRequest struct {
	Page         int
	PageSize     int
	Level        string
	LoggerName   string
	Keyword      string
	SinceMinutes int
}

func (r ListLogsRequest) Normalized() ListLogsRequest {
	normalized := r
	if normalized.Page < 1 {
		normalized.Page = defaultLogsPage
	}
	if normalized.PageSize < 1 {
		normalized.PageSize = defaultLogsPageSize
	}
	if normalized.PageSize > maxLogsPageSize {
		normalized.PageSize = maxLogsPageSize
	}
	if normalized.SinceMinutes < 0 {
		normalized.SinceMinutes = 0
	}
	if normalized.SinceMinutes > maxLogsSinceMinutes {
		normalized.SinceMinutes = maxLogsSinceMinutes
	}
	normalized.Level = strings.ToUpper(strings.TrimSpace(normalized.Level))
	normalized.LoggerName = strings.TrimSpace(normalized.LoggerName)
	normalized.Keyword = strings.TrimSpace(normalized.Keyword)
	return normalized
}

func (r ListLogsRequest) Offset() int {
	normalized := r.Normalized()
	return (normalized.Page - 1) * normalized.PageSize
}

type AppLogRecord struct {
	ID        int64
	Level     string
	Logger    string
	Module    string
	Pathname  string
	LineNo    int
	Message   string
	Exception string
	CreatedAt time.Time
}

type LogEntry struct {
	Level     string `json:"level"`
	Logger    string `json:"logger"`
	Message   string `json:"message"`
	Exception string `json:"exception"`
	CreatedAt string `json:"created_at"`
}

type ListLogsResponse struct {
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Logs     []LogEntry `json:"logs"`
}

type LogsStats struct {
	Total    int
	LatestAt *time.Time
	Levels   map[string]int
}

type StatsResponse struct {
	Total    int            `json:"total"`
	LatestAt *string        `json:"latest_at"`
	Levels   map[string]int `json:"levels"`
}

type CleanupRequest struct {
	RetentionDays *int `json:"retention_days,omitempty"`
	MaxRows       int  `json:"max_rows"`
}

func (r CleanupRequest) Normalized() CleanupRequest {
	normalized := r
	retention := defaultCleanupRetention
	if normalized.RetentionDays != nil {
		retention = *normalized.RetentionDays
	}
	if retention < minCleanupRetention {
		retention = minCleanupRetention
	}
	if retention > maxCleanupRetention {
		retention = maxCleanupRetention
	}
	normalized.RetentionDays = &retention
	if normalized.MaxRows < 1 {
		normalized.MaxRows = defaultCleanupMaxRows
	}
	if normalized.MaxRows < minCleanupMaxRows {
		normalized.MaxRows = minCleanupMaxRows
	}
	if normalized.MaxRows > maxCleanupMaxRows {
		normalized.MaxRows = maxCleanupMaxRows
	}
	return normalized
}

type CleanupResult struct {
	RetentionDays  int `json:"retention_days"`
	MaxRows        int `json:"max_rows"`
	DeletedByAge   int `json:"deleted_by_age"`
	DeletedByLimit int `json:"deleted_by_limit"`
	DeletedTotal   int `json:"deleted_total"`
	Remaining      int `json:"remaining"`
}

type ClearResult struct {
	DeletedTotal int `json:"deleted_total"`
	Remaining    int `json:"remaining"`
}
