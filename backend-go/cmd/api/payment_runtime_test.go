package main

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
)

func TestAPIPaymentRuntimeHelperBuildsServiceWithoutNotConfiguredSeams(t *testing.T) {
	t.Run("generate_link_random_billing_browser_and_session", func(t *testing.T) {
		service, _, accountsRepo := newTestAPIPaymentService(accounts.Account{
			ID:           101,
			Email:        "alpha@example.com",
			AccessToken:  "access-runtime",
			SessionToken: "session-runtime",
			Cookies:      "__Secure-next-auth.session-token=session-runtime; oai-did=device-runtime",
		}, payment.BindCardTask{
			ID:          201,
			AccountID:   101,
			PlanType:    "plus",
			Status:      payment.StatusOpened,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_runtime",
		})

		linkResp, err := service.GeneratePaymentLink(context.Background(), payment.GenerateLinkRequest{
			CheckoutRequestBase: payment.CheckoutRequestBase{
				AccountID: 101,
				PlanType:  "plus",
				Country:   "US",
				AutoOpen:  true,
			},
		})
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing checkout seam: %v", err)
		}
		if !linkResp.Success || linkResp.Link == "" {
			t.Fatalf("expected compatibility generate-link response, got %+v", linkResp)
		}

		billingResp, err := service.GetRandomBillingProfile(context.Background(), "US", "")
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing billing seam: %v", err)
		}
		if !billingResp.Success || billingResp.Profile["postal_code"] == "" {
			t.Fatalf("expected compatibility random billing response, got %+v", billingResp)
		}

		openResp, err := service.OpenBrowserIncognito(context.Background(), payment.OpenIncognitoRequest{
			URL:       "https://chatgpt.com/checkout/openai_llc/cs_runtime",
			AccountID: 101,
		})
		if err != nil {
			t.Fatalf("expected helper-built service to avoid browser open request error: %v", err)
		}
		if openResp.Success {
			t.Fatalf("expected helper-built browser open path to stay truthful when no launch occurs, got %+v", openResp)
		}
		if openResp.Message == "" {
			t.Fatalf("expected compatibility browser-open response, got %+v", openResp)
		}

		diagResp, err := service.GetAccountSessionDiagnostic(context.Background(), 101, true, "")
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing session probe seam: %v", err)
		}
		if !diagResp.Success || diagResp.Diagnostic.Probe == nil {
			t.Fatalf("expected diagnostic probe response, got %+v", diagResp)
		}

		accountsRepo.accountByID = accounts.Account{
			ID:          102,
			Email:       "bootstrap@example.com",
			AccessToken: "access-bootstrap",
			Cookies:     "oai-did=device-bootstrap",
		}
		bootstrapResp, err := service.BootstrapAccountSessionToken(context.Background(), 102, "")
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing bootstrap seam: %v", err)
		}
		if bootstrapResp.Success || bootstrapResp.SessionTokenLen != 0 {
			t.Fatalf("expected helper-built bootstrap to avoid synthetic session success, got %+v", bootstrapResp)
		}
		if accountsRepo.savedAccount.ID != 0 || accountsRepo.savedAccount.SessionToken != "" {
			t.Fatalf("expected helper-built bootstrap to avoid persisting placeholder session token, got %+v", accountsRepo.savedAccount)
		}
	})

	t.Run("subscription_sync_and_auto_bind", func(t *testing.T) {
		service, paymentRepo, _ := newTestAPIPaymentService(accounts.Account{
			ID:           103,
			Email:        "bind@example.com",
			AccessToken:  "access-bind",
			SessionToken: "session-bind",
			Cookies:      "__Secure-next-auth.session-token=session-bind; oai-did=device-bind",
		}, payment.BindCardTask{
			ID:          202,
			AccountID:   103,
			PlanType:    "plus",
			Status:      payment.StatusOpened,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_bind",
		})

		syncResp, err := service.SyncBindCardTaskSubscription(context.Background(), 202, payment.SyncBindCardTaskRequest{})
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing subscription seam: %v", err)
		}
		if syncResp.SubscriptionType == "" {
			t.Fatalf("expected compatibility subscription payload, got %+v", syncResp)
		}

		paymentRepo.task = payment.BindCardTask{
			ID:          203,
			AccountID:   103,
			PlanType:    "plus",
			Status:      payment.StatusOpened,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_bind_third",
		}
		thirdResp, err := service.AutoBindBindCardTaskThirdParty(context.Background(), 203, payment.ThirdPartyAutoBindRequest{
			Card:    payment.ThirdPartyCardRequest{Number: "4242424242424242", ExpMonth: "01", ExpYear: "30", CVC: "123"},
			Profile: payment.ThirdPartyProfileRequest{Name: "Ada", Country: "GB"},
		})
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing third-party auto-bind seam: %v", err)
		}
		if !thirdResp.Pending || !thirdResp.NeedUserAction {
			t.Fatalf("expected compatibility third-party auto-bind response, got %+v", thirdResp)
		}

		paymentRepo.task = payment.BindCardTask{
			ID:          204,
			AccountID:   103,
			PlanType:    "plus",
			Status:      payment.StatusOpened,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_bind_local",
		}
		localResp, err := service.AutoBindBindCardTaskLocal(context.Background(), 204, payment.LocalAutoBindRequest{
			Card:    payment.ThirdPartyCardRequest{Number: "4242424242424242", ExpMonth: "01", ExpYear: "30", CVC: "123"},
			Profile: payment.ThirdPartyProfileRequest{Name: "Ada", Country: "GB"},
		})
		if err != nil {
			t.Fatalf("expected helper-built service to avoid missing local auto-bind seam: %v", err)
		}
		if !localResp.Pending || !localResp.NeedUserAction {
			t.Fatalf("expected compatibility local auto-bind response, got %+v", localResp)
		}

		paymentRepo.task = payment.BindCardTask{
			ID:          205,
			AccountID:   103,
			PlanType:    "plus",
			Status:      payment.StatusLinkReady,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_bind_open",
		}
		_, err = service.OpenBindCardTask(context.Background(), 205)
		if err == nil {
			t.Fatal("expected helper-built open task path to surface truthful no-browser fallback")
		}
		if payment.StatusCode(err) != 500 || payment.Detail(err) != "未找到可用的浏览器，请手动复制链接" {
			t.Fatalf("expected truthful no-browser fallback error, got status=%d detail=%q", payment.StatusCode(err), payment.Detail(err))
		}
		if paymentRepo.savedTask.LastError != "未找到可用的浏览器" || paymentRepo.savedTask.Status != payment.StatusLinkReady {
			t.Fatalf("expected open failure to persist last_error without faking opened state, got %+v", paymentRepo.savedTask)
		}
	})
}

func TestAPIPaymentRuntimeSubscriptionFreeConfidenceGate(t *testing.T) {
	t.Run("low_confidence_free_keeps_paid", func(t *testing.T) {
		service, paymentRepo, accountsRepo := newTestAPIPaymentService(accounts.Account{
			ID:               111,
			Email:            "paid-low@example.com",
			SubscriptionType: "team",
			ExtraData: map[string]any{
				"payment_subscription_status":     "free",
				"payment_subscription_confidence": "low",
				"payment_subscription_source":     "runtime.low",
			},
		}, payment.BindCardTask{
			ID:          211,
			AccountID:   111,
			PlanType:    "plus",
			Status:      payment.StatusOpened,
			CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_runtime_low",
		})

		resp, err := service.SyncBindCardTaskSubscription(context.Background(), 211, payment.SyncBindCardTaskRequest{})
		if err != nil {
			t.Fatalf("expected low-confidence sync to succeed: %v", err)
		}
		if resp.Task.Status != payment.StatusPaidPendingSync {
			t.Fatalf("expected low-confidence free to keep paid_pending_sync, got %+v", resp)
		}
		if accountsRepo.savedAccount.SubscriptionType != "team" {
			t.Fatalf("expected paid subscription to be preserved, got %+v", accountsRepo.savedAccount)
		}
		if paymentRepo.savedTask.Status != payment.StatusPaidPendingSync {
			t.Fatalf("expected task writeback to remain paid_pending_sync, got %+v", paymentRepo.savedTask)
		}
	})

	t.Run("high_confidence_free_clears_paid", func(t *testing.T) {
		now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
		service, paymentRepo, accountsRepo := newTestAPIPaymentService(accounts.Account{
			ID:               112,
			Email:            "paid-high@example.com",
			SubscriptionType: "plus",
			SubscriptionAt:   &now,
			ExtraData: map[string]any{
				"payment_subscription_status":     "free",
				"payment_subscription_confidence": "high",
				"payment_subscription_source":     "runtime.high",
			},
		}, payment.BindCardTask{
			ID:        212,
			AccountID: 112,
			PlanType:  "plus",
			Status:    payment.StatusPaidPendingSync,
		})

		resp, err := service.SyncBindCardTaskSubscription(context.Background(), 212, payment.SyncBindCardTaskRequest{})
		if err != nil {
			t.Fatalf("expected high-confidence sync to succeed: %v", err)
		}
		if resp.Task.Status != payment.StatusWaitingUserAction {
			t.Fatalf("expected high-confidence free to move back to waiting_user_action, got %+v", resp)
		}
		if accountsRepo.savedAccount.SubscriptionType != "" || accountsRepo.savedAccount.SubscriptionAt != nil {
			t.Fatalf("expected paid subscription to clear, got %+v", accountsRepo.savedAccount)
		}
		if paymentRepo.savedTask.Status != payment.StatusWaitingUserAction {
			t.Fatalf("expected task writeback to reset to waiting_user_action, got %+v", paymentRepo.savedTask)
		}
	})
}

func TestAPIPaymentRuntimeTransitionAdaptersPreserveBillingAndCheckoutCompatibility(t *testing.T) {
	t.Run("non_us_supported_billing_countries_keep_local_templates", func(t *testing.T) {
		service, _, _ := newTestAPIPaymentService(accounts.Account{
			ID:    121,
			Email: "billing@example.com",
		}, payment.BindCardTask{})

		cases := map[string]struct {
			currency string
			state    string
			postal   string
		}{
			"AU": {currency: "AUD", state: "NSW", postal: "2000"},
			"JP": {currency: "JPY", state: "Tokyo", postal: "100-0001"},
		}

		for country, want := range cases {
			t.Run(country, func(t *testing.T) {
				resp, err := service.GetRandomBillingProfile(context.Background(), country, "")
				if err != nil {
					t.Fatalf("expected billing profile for %s: %v", country, err)
				}
				if !resp.Success {
					t.Fatalf("expected success for %s, got %+v", country, resp)
				}
				if got := resp.Profile["country"]; got != country {
					t.Fatalf("expected country %s, got %v", country, got)
				}
				if got := resp.Profile["currency"]; got != want.currency {
					t.Fatalf("expected currency %s, got %v", want.currency, got)
				}
				if got := resp.Profile["state"]; got != want.state || resp.Profile["postal_code"] != want.postal {
					t.Fatalf("expected local template for %s, got %+v", country, resp.Profile)
				}
			})
		}
	})

	t.Run("workspace_name_is_normalized_into_url_safe_checkout_link", func(t *testing.T) {
		service, _, _ := newTestAPIPaymentService(accounts.Account{
			ID:    122,
			Email: "checkout@example.com",
		}, payment.BindCardTask{})

		resp, err := service.GeneratePaymentLink(context.Background(), payment.GenerateLinkRequest{
			CheckoutRequestBase: payment.CheckoutRequestBase{
				AccountID:     122,
				PlanType:      "team",
				Country:       "US",
				Currency:      "USD",
				WorkspaceName: "研发/Team?50%#1",
				SeatQuantity:  5,
				PriceInterval: "month",
			},
		})
		if err != nil {
			t.Fatalf("expected normalized checkout link: %v", err)
		}
		urlSafeSessionID := regexp.MustCompile(`^cs_[A-Za-z0-9_-]+$`)
		if !urlSafeSessionID.MatchString(resp.CheckoutSessionID) {
			t.Fatalf("expected URL-safe session id, got %q", resp.CheckoutSessionID)
		}
		wantLink := "https://chatgpt.com/checkout/openai_llc/" + resp.CheckoutSessionID
		if resp.Link != wantLink {
			t.Fatalf("expected checkout link %q, got %q", wantLink, resp.Link)
		}

		asciiResp, err := service.GeneratePaymentLink(context.Background(), payment.GenerateLinkRequest{
			CheckoutRequestBase: payment.CheckoutRequestBase{
				AccountID:     122,
				PlanType:      "team",
				Country:       "US",
				Currency:      "USD",
				WorkspaceName: "Acme Team-01",
			},
		})
		if err != nil {
			t.Fatalf("expected compatibility checkout link for ascii workspace: %v", err)
		}
		if asciiResp.CheckoutSessionID != "cs_transition_team_122_acme_team_01" {
			t.Fatalf("expected ascii workspace compatibility shape, got %q", asciiResp.CheckoutSessionID)
		}
	})
}

func newTestAPIPaymentService(account accounts.Account, task payment.BindCardTask) (*payment.Service, *apiPaymentRepository, *apiPaymentAccountsRepository) {
	paymentRepo := &apiPaymentRepository{task: task}
	accountsRepo := &apiPaymentAccountsRepository{accountByID: account}
	return buildAPIPaymentService(paymentRepo, accountsRepo), paymentRepo, accountsRepo
}

type apiPaymentRepository struct {
	task      payment.BindCardTask
	savedTask payment.BindCardTask
}

func (f *apiPaymentRepository) CreateBindCardTask(context.Context, payment.CreateBindCardTaskParams) (payment.BindCardTask, error) {
	return f.task, nil
}

func (f *apiPaymentRepository) GetBindCardTask(context.Context, int) (payment.BindCardTask, error) {
	return f.task, nil
}

func (f *apiPaymentRepository) ListBindCardTasks(context.Context, payment.ListBindCardTasksRequest) (payment.ListBindCardTasksResponse, error) {
	return payment.ListBindCardTasksResponse{Tasks: []payment.BindCardTask{f.task}, Total: 1}, nil
}

func (f *apiPaymentRepository) UpdateBindCardTask(_ context.Context, task payment.BindCardTask) (payment.BindCardTask, error) {
	f.task = task
	f.savedTask = task
	return task, nil
}

func (f *apiPaymentRepository) DeleteBindCardTask(context.Context, int) error {
	return nil
}

type apiPaymentAccountsRepository struct {
	accountByID  accounts.Account
	savedAccount accounts.Account
}

func (f *apiPaymentAccountsRepository) GetAccountByID(context.Context, int) (accounts.Account, error) {
	return f.accountByID, nil
}

func (f *apiPaymentAccountsRepository) ListAccountsBySelection(context.Context, accounts.AccountSelectionRequest) ([]accounts.Account, error) {
	return []accounts.Account{f.accountByID}, nil
}

func (f *apiPaymentAccountsRepository) UpsertAccount(_ context.Context, account accounts.Account) (accounts.Account, error) {
	f.accountByID = account
	f.savedAccount = account
	return account, nil
}
