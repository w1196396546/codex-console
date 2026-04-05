package payment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryBindCardTaskCreateAndGetRoundTripsCompatibilityFields(t *testing.T) {
	now := time.Date(2026, 4, 5, 8, 9, 10, 0, time.UTC)
	openedAt := now.Add(5 * time.Minute)
	lastCheckedAt := now.Add(10 * time.Minute)
	updatedAt := now.Add(11 * time.Minute)

	db := &fakePaymentDB{
		queryRows: []fakePaymentRow{
			{values: bindCardTaskRowValues(BindCardTask{
				ID:                17,
				AccountID:         9,
				AccountEmail:      "alpha@example.com",
				PlanType:          "team",
				WorkspaceName:     "Ops",
				PriceInterval:     "month",
				SeatQuantity:      5,
				Country:           "US",
				Currency:          "USD",
				CheckoutURL:       "https://pay.example/checkout/cs_test_123",
				CheckoutSessionID: "cs_test_123",
				PublishableKey:    "pk_live_123",
				ClientSecret:      "seti_secret_123",
				CheckoutSource:    "openai_checkout",
				BindMode:          "third_party",
				Status:            StatusWaitingUserAction,
				LastError:         "need_3ds",
				OpenedAt:          &openedAt,
				LastCheckedAt:     &lastCheckedAt,
				CreatedAt:         now,
				UpdatedAt:         updatedAt,
			})},
			{values: bindCardTaskRowValues(BindCardTask{
				ID:                17,
				AccountID:         9,
				AccountEmail:      "alpha@example.com",
				PlanType:          "team",
				WorkspaceName:     "Ops",
				PriceInterval:     "month",
				SeatQuantity:      5,
				Country:           "US",
				Currency:          "USD",
				CheckoutURL:       "https://pay.example/checkout/cs_test_123",
				CheckoutSessionID: "cs_test_123",
				PublishableKey:    "pk_live_123",
				ClientSecret:      "seti_secret_123",
				CheckoutSource:    "openai_checkout",
				BindMode:          "third_party",
				Status:            StatusWaitingUserAction,
				LastError:         "need_3ds",
				OpenedAt:          &openedAt,
				LastCheckedAt:     &lastCheckedAt,
				CreatedAt:         now,
				UpdatedAt:         updatedAt,
			})},
		},
	}

	repo := newPostgresRepository(db)

	created, err := repo.CreateBindCardTask(context.Background(), CreateBindCardTaskParams{
		AccountID:         9,
		PlanType:          "team",
		WorkspaceName:     "Ops",
		PriceInterval:     "month",
		SeatQuantity:      5,
		Country:           "US",
		Currency:          "USD",
		CheckoutURL:       "https://pay.example/checkout/cs_test_123",
		CheckoutSessionID: "cs_test_123",
		PublishableKey:    "pk_live_123",
		ClientSecret:      "seti_secret_123",
		CheckoutSource:    "openai_checkout",
		BindMode:          "third_party",
		Status:            StatusWaitingUserAction,
		LastError:         "need_3ds",
		OpenedAt:          &openedAt,
		LastCheckedAt:     &lastCheckedAt,
	})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}

	if created.ID != 17 || created.AccountEmail != "alpha@example.com" {
		t.Fatalf("unexpected created task: %+v", created)
	}
	if !strings.Contains(db.queryLog[0], "INSERT INTO bind_card_tasks") {
		t.Fatalf("expected create query to insert bind_card_tasks, got %q", db.queryLog[0])
	}
	if len(db.argsLog[0]) != 18 {
		t.Fatalf("expected 18 insert args, got %#v", db.argsLog[0])
	}

	got, err := repo.GetBindCardTask(context.Background(), 17)
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if got.CheckoutSessionID != "cs_test_123" || got.PublishableKey != "pk_live_123" || got.ClientSecret != "seti_secret_123" {
		t.Fatalf("expected checkout/session secrets to round-trip, got %+v", got)
	}
	if got.BindMode != "third_party" || got.Status != StatusWaitingUserAction || got.LastError != "need_3ds" {
		t.Fatalf("expected bind/state fields to round-trip, got %+v", got)
	}
}

func TestRepositoryBindCardTaskListAppliesFiltersAndPagination(t *testing.T) {
	now := time.Date(2026, 4, 5, 8, 9, 10, 0, time.UTC)
	db := &fakePaymentDB{
		queryRows: []fakePaymentRow{
			{values: []any{2}},
		},
		queryResult: &fakePaymentRows{
			rows: [][]any{
				bindCardTaskRowValues(BindCardTask{
					ID:             18,
					AccountID:      9,
					AccountEmail:   "alpha@example.com",
					PlanType:       "plus",
					Country:        "US",
					Currency:       "USD",
					CheckoutURL:    "https://pay.example/checkout/cs_list_1",
					CheckoutSource: "openai_checkout",
					BindMode:       "semi_auto",
					Status:         StatusOpened,
					CreatedAt:      now,
					UpdatedAt:      now,
				}),
			},
		},
	}

	repo := newPostgresRepository(db)
	resp, err := repo.ListBindCardTasks(context.Background(), ListBindCardTasksRequest{
		Page:     2,
		PageSize: 20,
		Status:   StatusOpened,
		Search:   "alpha",
	})
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if resp.Total != 2 || len(resp.Tasks) != 1 || resp.Tasks[0].ID != 18 {
		t.Fatalf("unexpected list response: %+v", resp)
	}
	if !strings.Contains(db.queryLog[0], "COUNT(*)") || !strings.Contains(db.queryLog[1], "ORDER BY t.created_at DESC") {
		t.Fatalf("expected count and list queries, got %#v", db.queryLog)
	}
	if !strings.Contains(db.queryLog[1], "a.email ILIKE") || !strings.Contains(db.queryLog[1], "t.status =") {
		t.Fatalf("expected search and status filters in list query, got %q", db.queryLog[1])
	}
	if len(db.argsLog[1]) < 4 {
		t.Fatalf("expected filter and pagination args, got %#v", db.argsLog[1])
	}
}

func TestRepositoryBindCardTaskUpdateAndDeletePersistLifecycleFields(t *testing.T) {
	now := time.Date(2026, 4, 5, 8, 9, 10, 0, time.UTC)
	completedAt := now.Add(20 * time.Minute)
	db := &fakePaymentDB{
		queryRows: []fakePaymentRow{
			{values: bindCardTaskRowValues(BindCardTask{
				ID:                19,
				AccountID:         11,
				AccountEmail:      "beta@example.com",
				PlanType:          "plus",
				Country:           "US",
				Currency:          "USD",
				CheckoutURL:       "https://pay.example/checkout/cs_done",
				CheckoutSessionID: "cs_done",
				CheckoutSource:    "openai_checkout",
				BindMode:          "local_auto",
				Status:            StatusCompleted,
				CompletedAt:       &completedAt,
				CreatedAt:         now,
				UpdatedAt:         completedAt,
			})},
		},
		execTag: pgconn.NewCommandTag("DELETE 1"),
	}

	repo := newPostgresRepository(db)
	updated, err := repo.UpdateBindCardTask(context.Background(), BindCardTask{
		ID:                19,
		AccountID:         11,
		PlanType:          "plus",
		Country:           "US",
		Currency:          "USD",
		CheckoutURL:       "https://pay.example/checkout/cs_done",
		CheckoutSessionID: "cs_done",
		CheckoutSource:    "openai_checkout",
		BindMode:          "local_auto",
		Status:            StatusCompleted,
		CompletedAt:       &completedAt,
	})
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if updated.Status != StatusCompleted || updated.CompletedAt == nil || !updated.CompletedAt.Equal(completedAt) {
		t.Fatalf("expected lifecycle fields to persist, got %+v", updated)
	}
	if !strings.Contains(db.queryLog[0], "UPDATE bind_card_tasks") {
		t.Fatalf("expected update query, got %q", db.queryLog[0])
	}

	if err := repo.DeleteBindCardTask(context.Background(), 19); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if !strings.Contains(db.execQuery, "DELETE FROM bind_card_tasks") {
		t.Fatalf("expected delete query, got %q", db.execQuery)
	}
}

type fakePaymentDB struct {
	queryRows   []fakePaymentRow
	queryResult *fakePaymentRows
	execTag     pgconn.CommandTag
	execErr     error

	queryLog  []string
	argsLog   [][]any
	execQuery string
	execArgs  []any
}

func (f *fakePaymentDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.queryLog = append(f.queryLog, sql)
	f.argsLog = append(f.argsLog, args)
	if len(f.queryRows) == 0 {
		return fakePaymentRow{err: pgx.ErrNoRows}
	}
	row := f.queryRows[0]
	f.queryRows = f.queryRows[1:]
	return row
}

func (f *fakePaymentDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queryLog = append(f.queryLog, sql)
	f.argsLog = append(f.argsLog, args)
	if f.queryResult == nil {
		return &fakePaymentRows{}, nil
	}
	return f.queryResult, nil
}

func (f *fakePaymentDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execQuery = sql
	f.execArgs = args
	return f.execTag, f.execErr
}

type fakePaymentRow struct {
	values []any
	err    error
}

func (r fakePaymentRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for idx := range dest {
		switch target := dest[idx].(type) {
		case *int:
			*target = r.values[idx].(int)
		case **int:
			value := r.values[idx]
			if value == nil {
				*target = nil
				continue
			}
			v := value.(int)
			*target = &v
		case *string:
			value := r.values[idx]
			if value == nil {
				*target = ""
				continue
			}
			*target = value.(string)
		case **string:
			value := r.values[idx]
			if value == nil {
				*target = nil
				continue
			}
			v := value.(string)
			*target = &v
		case **time.Time:
			value := r.values[idx]
			if value == nil {
				*target = nil
				continue
			}
			switch typed := value.(type) {
			case time.Time:
				v := typed
				*target = &v
			case *time.Time:
				if typed == nil {
					*target = nil
					continue
				}
				v := *typed
				*target = &v
			default:
				panic("unsupported time pointer value")
			}
		case *time.Time:
			*target = r.values[idx].(time.Time)
		default:
			panic("unsupported scan target")
		}
	}
	return nil
}

type fakePaymentRows struct {
	rows [][]any
	idx  int
}

func (r *fakePaymentRows) Close()                                       {}
func (r *fakePaymentRows) Err() error                                   { return nil }
func (r *fakePaymentRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 1") }
func (r *fakePaymentRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakePaymentRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}
func (r *fakePaymentRows) Scan(dest ...any) error {
	row := fakePaymentRow{values: r.rows[r.idx-1]}
	return row.Scan(dest...)
}
func (r *fakePaymentRows) Values() ([]any, error) { return nil, nil }
func (r *fakePaymentRows) RawValues() [][]byte    { return nil }
func (r *fakePaymentRows) Conn() *pgx.Conn        { return nil }

func bindCardTaskRowValues(task BindCardTask) []any {
	return []any{
		task.ID,
		task.AccountID,
		nullableString(task.AccountEmail),
		task.PlanType,
		nullableString(task.WorkspaceName),
		nullableString(task.PriceInterval),
		nullableInt(task.SeatQuantity),
		task.Country,
		task.Currency,
		task.CheckoutURL,
		nullableString(task.CheckoutSessionID),
		nullableString(task.PublishableKey),
		nullableString(task.ClientSecret),
		nullableString(task.CheckoutSource),
		nullableString(task.BindMode),
		task.Status,
		nullableString(task.LastError),
		task.OpenedAt,
		task.LastCheckedAt,
		task.CompletedAt,
		task.CreatedAt,
		task.UpdatedAt,
	}
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
