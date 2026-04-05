package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

func TestServiceSaveSessionTokenMergesCookieAndWritesAccountsTruth(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{
			ID:      7,
			Email:   "alpha@example.com",
			Cookies: "foo=bar",
		},
	}
	service := NewService(&fakePaymentRepository{}, accountRepo, WithNow(func() time.Time { return now }))

	resp, err := service.SaveAccountSessionToken(context.Background(), 7, SaveSessionTokenRequest{
		SessionToken: " session-123 ",
		MergeCookie:  true,
	})
	if err != nil {
		t.Fatalf("unexpected save session token error: %v", err)
	}
	if !resp.Success || resp.SessionTokenLen != len("session-123") {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if accountRepo.savedAccount.SessionToken != "session-123" {
		t.Fatalf("expected session token to be persisted, got %+v", accountRepo.savedAccount)
	}
	if accountRepo.savedAccount.LastRefresh == nil || !accountRepo.savedAccount.LastRefresh.Equal(now) {
		t.Fatalf("expected last refresh writeback, got %+v", accountRepo.savedAccount)
	}
	if accountRepo.savedAccount.Cookies == "foo=bar" || accountRepo.savedAccount.Cookies == "" {
		t.Fatalf("expected cookie merge, got %+v", accountRepo.savedAccount)
	}
}

func TestServiceBootstrapSessionTokenUsesAdapterSeam(t *testing.T) {
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{
			ID:           8,
			Email:        "bootstrap@example.com",
			SessionToken: "",
		},
	}
	adapter := &fakeSessionAdapter{
		bootstrapResult: SessionBootstrapResult{
			SessionToken: "fresh-session",
			AccessToken:  "access-1",
			Cookies:      "__Secure-next-auth.session-token=fresh-session",
		},
	}
	service := NewService(
		&fakePaymentRepository{},
		accountRepo,
		WithSessionAdapter(adapter),
		WithNow(func() time.Time { return time.Date(2026, 4, 5, 9, 5, 0, 0, time.UTC) }),
	)

	resp, err := service.BootstrapAccountSessionToken(context.Background(), 8, "")
	if err != nil {
		t.Fatalf("unexpected bootstrap error: %v", err)
	}
	if !resp.Success || resp.SessionTokenLen != len("fresh-session") {
		t.Fatalf("unexpected bootstrap response: %+v", resp)
	}
	if adapter.bootstrapAccount.ID != 8 {
		t.Fatalf("expected adapter seam to receive account, got %+v", adapter.bootstrapAccount)
	}
	if accountRepo.savedAccount.SessionToken != "fresh-session" || accountRepo.savedAccount.AccessToken != "access-1" {
		t.Fatalf("expected account truth source to be updated, got %+v", accountRepo.savedAccount)
	}
}

func TestServiceSyncBindCardSubscriptionPreservesPaidStateOnLowConfidenceFree(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 10, 0, 0, time.UTC)
	paymentRepo := &fakePaymentRepository{
		task: BindCardTask{
			ID:                31,
			AccountID:         11,
			PlanType:          "plus",
			CheckoutURL:       "https://pay.example/checkout/cs_live",
			CheckoutSessionID: "cs_live",
			Status:            StatusWaitingUserAction,
		},
	}
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{
			ID:               11,
			Email:            "paid@example.com",
			SubscriptionType: "team",
		},
	}
	service := NewService(
		paymentRepo,
		accountRepo,
		WithSubscriptionChecker(fakeSubscriptionCheckerFunc(func(context.Context, accounts.Account, string, bool) (SubscriptionCheckDetail, error) {
			return SubscriptionCheckDetail{
				Status:      "free",
				Source:      "chatgpt_web",
				Confidence:  "low",
				Note:        "still propagating",
				RefreshedToken: false,
			}, nil
		})),
		WithNow(func() time.Time { return now }),
	)

	resp, err := service.SyncBindCardTaskSubscription(context.Background(), 31, SyncBindCardTaskRequest{})
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}
	if resp.SubscriptionType != "free" || resp.Task.Status != StatusPaidPendingSync {
		t.Fatalf("expected low-confidence free to stay paid_pending_sync, got %+v", resp)
	}
	if accountRepo.savedAccount.SubscriptionType != "team" {
		t.Fatalf("expected existing paid state to be preserved, got %+v", accountRepo.savedAccount)
	}
	if paymentRepo.savedTask.LastCheckedAt == nil || !paymentRepo.savedTask.LastCheckedAt.Equal(now) {
		t.Fatalf("expected task last_checked_at writeback, got %+v", paymentRepo.savedTask)
	}
}

func TestServiceSyncBindCardSubscriptionClearsOnHighConfidenceFree(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 20, 0, 0, time.UTC)
	paymentRepo := &fakePaymentRepository{
		task: BindCardTask{
			ID:        32,
			AccountID: 12,
			PlanType:  "plus",
			Status:    StatusOpened,
		},
	}
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{
			ID:               12,
			Email:            "free@example.com",
			SubscriptionType: "plus",
			SubscriptionAt:   timePtr(now.Add(-24 * time.Hour)),
		},
	}
	service := NewService(
		paymentRepo,
		accountRepo,
		WithSubscriptionChecker(fakeSubscriptionCheckerFunc(func(context.Context, accounts.Account, string, bool) (SubscriptionCheckDetail, error) {
			return SubscriptionCheckDetail{
				Status:      "free",
				Source:      "chatgpt_web",
				Confidence:  "high",
				Note:        "",
				RefreshedToken: true,
			}, nil
		})),
		WithNow(func() time.Time { return now }),
	)

	resp, err := service.SyncBindCardTaskSubscription(context.Background(), 32, SyncBindCardTaskRequest{})
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}
	if resp.Task.Status != StatusWaitingUserAction {
		t.Fatalf("expected high-confidence free to move back to waiting_user_action, got %+v", resp)
	}
	if accountRepo.savedAccount.SubscriptionType != "" || accountRepo.savedAccount.SubscriptionAt != nil {
		t.Fatalf("expected paid subscription to clear on high confidence free, got %+v", accountRepo.savedAccount)
	}
}

func TestServiceMarkUserActionTransitionsToCompletedOnPaidSubscription(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 30, 0, 0, time.UTC)
	paymentRepo := &fakePaymentRepository{
		task: BindCardTask{
			ID:        33,
			AccountID: 13,
			PlanType:  "team",
			Status:    StatusOpened,
		},
	}
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{
			ID:    13,
			Email: "verified@example.com",
		},
	}
	service := NewService(
		paymentRepo,
		accountRepo,
		WithSubscriptionChecker(fakeSubscriptionCheckerFunc(func(context.Context, accounts.Account, string, bool) (SubscriptionCheckDetail, error) {
			return SubscriptionCheckDetail{
				Status:     "team",
				Source:     "chatgpt_web",
				Confidence: "high",
			}, nil
		})),
		WithNow(func() time.Time { return now }),
	)

	resp, err := service.MarkBindCardTaskUserAction(context.Background(), 33, MarkUserActionRequest{
		TimeoutSeconds:  180,
		IntervalSeconds: 10,
	})
	if err != nil {
		t.Fatalf("unexpected mark-user-action error: %v", err)
	}
	if !resp.Verified || resp.Task.Status != StatusCompleted || resp.SubscriptionType != "team" {
		t.Fatalf("expected completed verification response, got %+v", resp)
	}
	if paymentRepo.savedTask.CompletedAt == nil || !paymentRepo.savedTask.CompletedAt.Equal(now) {
		t.Fatalf("expected task completed_at writeback, got %+v", paymentRepo.savedTask)
	}
}

func TestBindCardServiceAutoBindThirdPartyUsesAdapterSeamAndStatusFlow(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 40, 0, 0, time.UTC)
	paymentRepo := &fakePaymentRepository{
		task: BindCardTask{
			ID:        34,
			AccountID: 14,
			PlanType:  "plus",
			Status:    StatusOpened,
		},
	}
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{ID: 14, Email: "third@example.com"},
	}
	autoBinder := &fakeAutoBinder{
		thirdPartyResult: AutoBindResult{
			PaidConfirmed: true,
			NeedUserAction: true,
			ThirdParty: map[string]any{"assessment": map[string]any{"state": "pending"}},
		},
	}
	service := NewService(
		paymentRepo,
		accountRepo,
		WithAutoBinder(autoBinder),
		WithNow(func() time.Time { return now }),
	)

	resp, err := service.AutoBindBindCardTaskThirdParty(context.Background(), 34, ThirdPartyAutoBindRequest{})
	if err != nil {
		t.Fatalf("unexpected auto-bind-third-party error: %v", err)
	}
	if !resp.PaidConfirmed || resp.Task.Status != StatusPaidPendingSync {
		t.Fatalf("expected paid-pending-sync response, got %+v", resp)
	}
	if autoBinder.thirdPartyTask.ID != 34 || autoBinder.thirdPartyAccount.ID != 14 {
		t.Fatalf("expected adapter seam to receive task/account, got %+v %+v", autoBinder.thirdPartyTask, autoBinder.thirdPartyAccount)
	}
}

func TestBindCardServiceAutoBindLocalUsesAdapterSeamAndFailsTaskOnAdapterError(t *testing.T) {
	now := time.Date(2026, 4, 5, 9, 50, 0, 0, time.UTC)
	paymentRepo := &fakePaymentRepository{
		task: BindCardTask{
			ID:        35,
			AccountID: 15,
			PlanType:  "plus",
			Status:    StatusOpened,
		},
	}
	accountRepo := &fakeAccountRepository{
		accountByID: accounts.Account{ID: 15, Email: "local@example.com"},
	}
	autoBinder := &fakeAutoBinder{
		localErr: errors.New("playwright unavailable"),
	}
	service := NewService(
		paymentRepo,
		accountRepo,
		WithAutoBinder(autoBinder),
		WithNow(func() time.Time { return now }),
	)

	_, err := service.AutoBindBindCardTaskLocal(context.Background(), 35, LocalAutoBindRequest{})
	if err == nil {
		t.Fatal("expected adapter failure to surface")
	}
	if paymentRepo.savedTask.Status != StatusFailed || paymentRepo.savedTask.LastError == "" {
		t.Fatalf("expected task failure writeback, got %+v", paymentRepo.savedTask)
	}
}

type fakePaymentRepository struct {
	task      BindCardTask
	listResp  ListBindCardTasksResponse
	savedTask BindCardTask
	deletedID int
}

func (f *fakePaymentRepository) CreateBindCardTask(context.Context, CreateBindCardTaskParams) (BindCardTask, error) {
	return f.task, nil
}

func (f *fakePaymentRepository) GetBindCardTask(context.Context, int) (BindCardTask, error) {
	if f.task.ID == 0 {
		return BindCardTask{}, ErrBindCardTaskNotFound
	}
	return f.task, nil
}

func (f *fakePaymentRepository) ListBindCardTasks(context.Context, ListBindCardTasksRequest) (ListBindCardTasksResponse, error) {
	return f.listResp, nil
}

func (f *fakePaymentRepository) UpdateBindCardTask(_ context.Context, task BindCardTask) (BindCardTask, error) {
	f.savedTask = task
	f.task = task
	return task, nil
}

func (f *fakePaymentRepository) DeleteBindCardTask(_ context.Context, taskID int) error {
	f.deletedID = taskID
	return nil
}

type fakeAccountRepository struct {
	accountByID  accounts.Account
	savedAccount accounts.Account
	selection    []accounts.Account
}

func (f *fakeAccountRepository) GetAccountByID(context.Context, int) (accounts.Account, error) {
	if f.accountByID.ID == 0 {
		return accounts.Account{}, accounts.ErrAccountNotFound
	}
	return f.accountByID, nil
}

func (f *fakeAccountRepository) ListAccountsBySelection(context.Context, accounts.AccountSelectionRequest) ([]accounts.Account, error) {
	return append([]accounts.Account(nil), f.selection...), nil
}

func (f *fakeAccountRepository) UpsertAccount(_ context.Context, account accounts.Account) (accounts.Account, error) {
	f.savedAccount = account
	f.accountByID = account
	return account, nil
}

type fakeSessionAdapter struct {
	bootstrapAccount accounts.Account
	bootstrapResult  SessionBootstrapResult
	bootstrapErr     error
}

func (f *fakeSessionAdapter) BootstrapSessionToken(_ context.Context, account accounts.Account, _ string) (SessionBootstrapResult, error) {
	f.bootstrapAccount = account
	return f.bootstrapResult, f.bootstrapErr
}

func (f *fakeSessionAdapter) ProbeSession(_ context.Context, _ accounts.Account, _ string) (*SessionProbeResult, error) {
	return nil, nil
}

type fakeAutoBinder struct {
	thirdPartyTask    BindCardTask
	thirdPartyAccount accounts.Account
	thirdPartyReq     ThirdPartyAutoBindRequest
	thirdPartyResult  AutoBindResult
	thirdPartyErr     error

	localTask    BindCardTask
	localAccount accounts.Account
	localReq     LocalAutoBindRequest
	localResult  AutoBindResult
	localErr     error
}

func (f *fakeAutoBinder) AutoBindThirdParty(_ context.Context, task BindCardTask, account accounts.Account, req ThirdPartyAutoBindRequest) (AutoBindResult, error) {
	f.thirdPartyTask = task
	f.thirdPartyAccount = account
	f.thirdPartyReq = req
	return f.thirdPartyResult, f.thirdPartyErr
}

func (f *fakeAutoBinder) AutoBindLocal(_ context.Context, task BindCardTask, account accounts.Account, req LocalAutoBindRequest) (AutoBindResult, error) {
	f.localTask = task
	f.localAccount = account
	f.localReq = req
	return f.localResult, f.localErr
}

type fakeSubscriptionCheckerFunc func(context.Context, accounts.Account, string, bool) (SubscriptionCheckDetail, error)

func (f fakeSubscriptionCheckerFunc) CheckSubscription(ctx context.Context, account accounts.Account, proxy string, allowRefresh bool) (SubscriptionCheckDetail, error) {
	return f(ctx, account, proxy, allowRefresh)
}

func timePtr(value time.Time) *time.Time {
	return &value
}
