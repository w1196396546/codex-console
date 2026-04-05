package registration

import (
	"context"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type JobPayloadOutlookReservationStore struct {
	pool *pgxpool.Pool
}

func NewJobPayloadOutlookReservationStore(pool *pgxpool.Pool) *JobPayloadOutlookReservationStore {
	return &JobPayloadOutlookReservationStore{pool: pool}
}

func (s *JobPayloadOutlookReservationStore) ListClaimedOutlookServiceIDs(ctx context.Context, excludeTaskUUID string) ([]int, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT CAST(payload->>'email_service_id' AS INTEGER) AS email_service_id
FROM jobs
WHERE job_type = $1
  AND ($2 = '' OR job_id::text <> $2)
  AND status = ANY($3)
  AND COALESCE(payload->>'email_service_type', '') = 'outlook'
  AND NULLIF(payload->>'email_service_id', '') IS NOT NULL
ORDER BY email_service_id ASC
`, JobTypeSingle, strings.TrimSpace(excludeTaskUUID), []string{jobs.StatusPending, jobs.StatusRunning, jobs.StatusPaused})
	if err != nil {
		return nil, fmt.Errorf("query claimed outlook services: %w", err)
	}
	defer rows.Close()

	claimed := make([]int, 0)
	for rows.Next() {
		var serviceID int
		if err := rows.Scan(&serviceID); err != nil {
			return nil, fmt.Errorf("scan claimed outlook service id: %w", err)
		}
		claimed = append(claimed, serviceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outlook service ids: %w", err)
	}
	return claimed, nil
}

func (s *JobPayloadOutlookReservationStore) ReserveOutlookService(ctx context.Context, taskUUID string, serviceID int) error {
	if s == nil || s.pool == nil {
		return nil
	}

	commandTag, err := s.pool.Exec(ctx, `
UPDATE jobs
SET payload = jsonb_set(payload, '{email_service_id}', to_jsonb($2::int), true)
WHERE job_type = $3
  AND job_id::text = $1
  AND COALESCE(payload->>'email_service_type', '') = 'outlook'
`, strings.TrimSpace(taskUUID), serviceID, JobTypeSingle)
	if err != nil {
		return fmt.Errorf("reserve outlook service in job payload: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("reserve outlook service in job payload: task not found: %s", strings.TrimSpace(taskUUID))
	}
	return nil
}
