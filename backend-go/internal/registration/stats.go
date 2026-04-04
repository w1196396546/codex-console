package registration

import (
	"context"
	"time"
)

type StatsResponse struct {
	ByStatus         map[string]int `json:"by_status"`
	TodayCount       int            `json:"today_count"`
	TodayTotal       int            `json:"today_total"`
	TodaySuccess     int            `json:"today_success"`
	TodayFailed      int            `json:"today_failed"`
	TodaySuccessRate float64        `json:"today_success_rate"`
	TodayByStatus    map[string]int `json:"today_by_status"`
}

type statsRepository interface {
	ListStatusCounts(ctx context.Context, from *time.Time, to *time.Time) (map[string]int, error)
}

type StatsService struct {
	repo statsRepository
	now  func() time.Time
}

func NewStatsService(repo statsRepository) *StatsService {
	return &StatsService{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *StatsService) GetStats(ctx context.Context) (StatsResponse, error) {
	if s == nil || s.repo == nil {
		return emptyStatsResponse(), nil
	}

	overall, err := s.repo.ListStatusCounts(ctx, nil, nil)
	if err != nil {
		return StatsResponse{}, err
	}
	overall = normalizeStatusCounts(overall)

	now := s.now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	todayByStatus, err := s.repo.ListStatusCounts(ctx, &startOfDay, &endOfDay)
	if err != nil {
		return StatsResponse{}, err
	}
	todayByStatus = normalizeStatusCounts(todayByStatus)

	todayTotal := sumStatusCounts(todayByStatus)
	todaySuccess := todayByStatus["completed"]
	todayFailed := todayByStatus["failed"]
	todaySuccessRate := 0.0
	if todayTotal > 0 {
		todaySuccessRate = float64(todaySuccess) / float64(todayTotal) * 100
		todaySuccessRate = float64(int(todaySuccessRate*10+0.5)) / 10
	}

	return StatsResponse{
		ByStatus:         overall,
		TodayCount:       todayTotal,
		TodayTotal:       todayTotal,
		TodaySuccess:     todaySuccess,
		TodayFailed:      todayFailed,
		TodaySuccessRate: todaySuccessRate,
		TodayByStatus:    todayByStatus,
	}, nil
}

func emptyStatsResponse() StatsResponse {
	return StatsResponse{
		ByStatus:      map[string]int{},
		TodayByStatus: map[string]int{},
	}
}

func normalizeStatusCounts(values map[string]int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	return values
}

func sumStatusCounts(values map[string]int) int {
	total := 0
	for _, count := range values {
		total += count
	}
	return total
}
