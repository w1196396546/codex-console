package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type paymentQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type PostgresRepository struct {
	db paymentQuerier
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return newPostgresRepository(pool)
}

func newPostgresRepository(db paymentQuerier) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const bindCardTaskColumns = `
		t.id,
		t.account_id,
		a.email,
		t.plan_type,
		t.workspace_name,
		t.price_interval,
		t.seat_quantity,
		t.country,
		t.currency,
		t.checkout_url,
		t.checkout_session_id,
		t.publishable_key,
		t.client_secret,
		t.checkout_source,
		t.bind_mode,
		t.status,
		t.last_error,
		t.opened_at,
		t.last_checked_at,
		t.completed_at,
		t.created_at,
		t.updated_at
`

func (r *PostgresRepository) CreateBindCardTask(ctx context.Context, params CreateBindCardTaskParams) (BindCardTask, error) {
	if r == nil || r.db == nil {
		return BindCardTask{}, ErrRepositoryNotConfigured
	}
	const query = `
WITH inserted AS (
	INSERT INTO bind_card_tasks (
		account_id,
		plan_type,
		workspace_name,
		price_interval,
		seat_quantity,
		country,
		currency,
		checkout_url,
		checkout_session_id,
		publishable_key,
		client_secret,
		checkout_source,
		bind_mode,
		status,
		last_error,
		opened_at,
		last_checked_at,
		completed_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
	)
	RETURNING *
)
SELECT ` + bindCardTaskColumns + `
FROM inserted t
LEFT JOIN accounts a ON a.id = t.account_id
`
	row := r.db.QueryRow(
		ctx,
		query,
		params.AccountID,
		params.PlanType,
		nullString(params.WorkspaceName),
		nullString(params.PriceInterval),
		nullInt(params.SeatQuantity),
		firstNonEmpty(strings.TrimSpace(params.Country), "US"),
		firstNonEmpty(strings.TrimSpace(params.Currency), "USD"),
		params.CheckoutURL,
		nullString(params.CheckoutSessionID),
		nullString(params.PublishableKey),
		nullString(params.ClientSecret),
		nullString(params.CheckoutSource),
		firstNonEmpty(normalizeBindMode(params.BindMode), "semi_auto"),
		firstNonEmpty(normalizeStatus(params.Status), StatusLinkReady),
		nullString(params.LastError),
		params.OpenedAt,
		params.LastCheckedAt,
		params.CompletedAt,
	)
	return scanBindCardTaskRow(row)
}

func (r *PostgresRepository) GetBindCardTask(ctx context.Context, taskID int) (BindCardTask, error) {
	if r == nil || r.db == nil {
		return BindCardTask{}, ErrRepositoryNotConfigured
	}
	const query = `
SELECT ` + bindCardTaskColumns + `
FROM bind_card_tasks t
LEFT JOIN accounts a ON a.id = t.account_id
WHERE t.id = $1
`
	task, err := scanBindCardTaskRow(r.db.QueryRow(ctx, query, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return BindCardTask{}, ErrBindCardTaskNotFound
	}
	return task, err
}

func (r *PostgresRepository) ListBindCardTasks(ctx context.Context, req ListBindCardTasksRequest) (ListBindCardTasksResponse, error) {
	if r == nil || r.db == nil {
		return ListBindCardTasksResponse{}, ErrRepositoryNotConfigured
	}
	normalized := normalizeListRequest(req)
	filters, args := buildBindCardTaskFilters(normalized)
	countQuery := `SELECT COUNT(*) FROM bind_card_tasks t LEFT JOIN accounts a ON a.id = t.account_id` + filters

	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListBindCardTasksResponse{}, fmt.Errorf("count bind_card_tasks: %w", err)
	}

	listQuery := `
SELECT ` + bindCardTaskColumns + `
FROM bind_card_tasks t
LEFT JOIN accounts a ON a.id = t.account_id` + filters + `
ORDER BY t.created_at DESC, t.id DESC
LIMIT $` + fmt.Sprintf("%d", len(args)+1) + ` OFFSET $` + fmt.Sprintf("%d", len(args)+2)
	listArgs := append(args, normalized.PageSize, (normalized.Page-1)*normalized.PageSize)

	rows, err := r.db.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return ListBindCardTasksResponse{}, fmt.Errorf("list bind_card_tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]BindCardTask, 0)
	for rows.Next() {
		task, scanErr := scanBindCardTaskRows(rows)
		if scanErr != nil {
			return ListBindCardTasksResponse{}, scanErr
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return ListBindCardTasksResponse{}, fmt.Errorf("iterate bind_card_tasks: %w", err)
	}

	return ListBindCardTasksResponse{
		Total: total,
		Tasks: tasks,
	}, nil
}

func (r *PostgresRepository) UpdateBindCardTask(ctx context.Context, task BindCardTask) (BindCardTask, error) {
	if r == nil || r.db == nil {
		return BindCardTask{}, ErrRepositoryNotConfigured
	}
	const query = `
WITH updated AS (
	UPDATE bind_card_tasks
	SET
		workspace_name = $2,
		price_interval = $3,
		seat_quantity = $4,
		country = $5,
		currency = $6,
		checkout_url = $7,
		checkout_session_id = $8,
		publishable_key = $9,
		client_secret = $10,
		checkout_source = $11,
		bind_mode = $12,
		status = $13,
		last_error = $14,
		opened_at = $15,
		last_checked_at = $16,
		completed_at = $17,
		updated_at = NOW()
	WHERE id = $1
	RETURNING *
)
SELECT ` + bindCardTaskColumns + `
FROM updated t
LEFT JOIN accounts a ON a.id = t.account_id
`
	row := r.db.QueryRow(
		ctx,
		query,
		task.ID,
		nullString(task.WorkspaceName),
		nullString(task.PriceInterval),
		nullInt(task.SeatQuantity),
		firstNonEmpty(strings.TrimSpace(task.Country), "US"),
		firstNonEmpty(strings.TrimSpace(task.Currency), "USD"),
		task.CheckoutURL,
		nullString(task.CheckoutSessionID),
		nullString(task.PublishableKey),
		nullString(task.ClientSecret),
		nullString(task.CheckoutSource),
		firstNonEmpty(normalizeBindMode(task.BindMode), "semi_auto"),
		firstNonEmpty(normalizeStatus(task.Status), StatusLinkReady),
		nullString(task.LastError),
		task.OpenedAt,
		task.LastCheckedAt,
		task.CompletedAt,
	)
	updated, err := scanBindCardTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return BindCardTask{}, ErrBindCardTaskNotFound
	}
	return updated, err
}

func (r *PostgresRepository) DeleteBindCardTask(ctx context.Context, taskID int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	const query = `DELETE FROM bind_card_tasks WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("delete bind_card_task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBindCardTaskNotFound
	}
	return nil
}

func buildBindCardTaskFilters(req ListBindCardTasksRequest) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if req.Status != "" {
		args = append(args, req.Status)
		clauses = append(clauses, fmt.Sprintf("t.status = $%d", len(args)))
	}
	if req.Search != "" {
		args = append(args, "%"+req.Search+"%")
		clauses = append(clauses, fmt.Sprintf("(a.email ILIKE $%d OR COALESCE(a.account_id, '') ILIKE $%d)", len(args), len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanBindCardTaskRow(row pgx.Row) (BindCardTask, error) {
	return scanBindCardTask(func(dest ...any) error {
		return row.Scan(dest...)
	})
}

func scanBindCardTaskRows(rows pgx.Rows) (BindCardTask, error) {
	return scanBindCardTask(func(dest ...any) error {
		return rows.Scan(dest...)
	})
}

func scanBindCardTask(scan func(dest ...any) error) (BindCardTask, error) {
	var (
		task              BindCardTask
		accountEmail      *string
		workspace         *string
		priceInterval     *string
		seatQuantity      *int
		checkoutSessionID *string
		publishableKey    *string
		clientSecret      *string
		checkoutSource    *string
		bindMode          *string
		lastError         *string
		openedAt          *time.Time
		lastCheckedAt     *time.Time
		completedAt       *time.Time
	)
	if err := scan(
		&task.ID,
		&task.AccountID,
		&accountEmail,
		&task.PlanType,
		&workspace,
		&priceInterval,
		&seatQuantity,
		&task.Country,
		&task.Currency,
		&task.CheckoutURL,
		&checkoutSessionID,
		&publishableKey,
		&clientSecret,
		&checkoutSource,
		&bindMode,
		&task.Status,
		&lastError,
		&openedAt,
		&lastCheckedAt,
		&completedAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return BindCardTask{}, err
	}

	task.AccountEmail = derefString(accountEmail)
	task.WorkspaceName = derefString(workspace)
	task.PriceInterval = derefString(priceInterval)
	if seatQuantity != nil {
		task.SeatQuantity = *seatQuantity
	}
	task.CheckoutSessionID = derefString(checkoutSessionID)
	task.PublishableKey = derefString(publishableKey)
	task.ClientSecret = derefString(clientSecret)
	task.CheckoutSource = derefString(checkoutSource)
	task.BindMode = firstNonEmpty(derefString(bindMode), "semi_auto")
	task.LastError = derefString(lastError)
	task.OpenedAt = openedAt
	task.LastCheckedAt = lastCheckedAt
	task.CompletedAt = completedAt
	return task, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
