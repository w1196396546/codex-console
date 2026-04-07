package payment

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

func TestPaymentTransitionAdapterSetCoversLiveSeams(t *testing.T) {
	now := time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC)
	adapters := NewTransitionAdapters()
	if adapters.CheckoutLinkGenerator == nil {
		t.Fatal("expected checkout link generator adapter")
	}
	if adapters.BillingProfileGenerator == nil {
		t.Fatal("expected billing profile generator adapter")
	}
	if adapters.BrowserOpener == nil {
		t.Fatal("expected browser opener adapter")
	}
	if adapters.SessionAdapter == nil {
		t.Fatal("expected session adapter")
	}
	if adapters.SubscriptionChecker == nil {
		t.Fatal("expected subscription checker adapter")
	}
	if adapters.AutoBinder == nil {
		t.Fatal("expected auto binder adapter")
	}

	t.Run("generate_link_random_billing_browser_open_and_probe", func(t *testing.T) {
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:           41,
				Email:        "alpha@example.com",
				AccessToken:  "access-live",
				SessionToken: "session-live",
				Cookies:      "__Secure-next-auth.session-token=session-live; oai-did=device-live",
			},
		}
		service := NewService(
			&fakePaymentRepository{},
			accountRepo,
			WithCheckoutLinkGenerator(adapters.CheckoutLinkGenerator),
			WithBillingProfileGenerator(adapters.BillingProfileGenerator),
			WithBrowserOpener(adapters.BrowserOpener),
			WithSessionAdapter(adapters.SessionAdapter),
			WithNow(func() time.Time { return now }),
		)

		linkResp, err := service.GeneratePaymentLink(context.Background(), GenerateLinkRequest{
			CheckoutRequestBase: CheckoutRequestBase{
				AccountID: 41,
				PlanType:  "plus",
				Country:   "GB",
				Currency:  "GBP",
				AutoOpen:  true,
			},
		})
		if err != nil {
			t.Fatalf("expected transition checkout adapter to avoid missing seam: %v", err)
		}
		if !linkResp.Success || linkResp.Link == "" || linkResp.CheckoutSessionID == "" {
			t.Fatalf("expected compatibility checkout link response, got %+v", linkResp)
		}
		if linkResp.Source != "openai_checkout" {
			t.Fatalf("expected openai checkout source, got %+v", linkResp)
		}

		billingResp, err := service.GetRandomBillingProfile(context.Background(), "GB", "")
		if err != nil {
			t.Fatalf("expected transition random billing adapter to avoid missing seam: %v", err)
		}
		if !billingResp.Success {
			t.Fatalf("expected random billing success response, got %+v", billingResp)
		}
		if billingResp.Profile["country"] != "GB" || billingResp.Profile["postal_code"] == "" {
			t.Fatalf("expected normalized random billing profile, got %+v", billingResp.Profile)
		}

		openResp, err := service.OpenBrowserIncognito(context.Background(), OpenIncognitoRequest{
			URL:       "https://chatgpt.com/checkout/openai_llc/cs_live_transition",
			AccountID: 41,
		})
		if err != nil {
			t.Fatalf("expected transition browser opener to avoid request error: %v", err)
		}
		if openResp.Success {
			t.Fatalf("expected transition browser opener to stay truthful when no launch path exists, got %+v", openResp)
		}
		if openResp.Message == "" {
			t.Fatalf("expected compatibility browser-open message, got %+v", openResp)
		}

		diagResp, err := service.GetAccountSessionDiagnostic(context.Background(), 41, true, "")
		if err != nil {
			t.Fatalf("expected transition probe adapter to avoid missing seam: %v", err)
		}
		if !diagResp.Success || diagResp.Diagnostic.Probe == nil {
			t.Fatalf("expected diagnostic probe payload, got %+v", diagResp)
		}
		if !diagResp.Diagnostic.Probe.OK || !diagResp.Diagnostic.Probe.SessionTokenFound {
			t.Fatalf("expected probe to report session context, got %+v", diagResp.Diagnostic.Probe)
		}
	})

	t.Run("session_adapter_bootstrap_requires_reusable_session_token", func(t *testing.T) {
		result, err := adapters.SessionAdapter.BootstrapSessionToken(context.Background(), accounts.Account{
			ID:          42,
			Email:       "access-only@example.com",
			AccessToken: "access-bootstrap",
			Cookies:     "oai-did=device-bootstrap",
		}, "")
		if err != nil {
			t.Fatalf("expected transition session bootstrap adapter to stay compatible: %v", err)
		}
		if result.SessionToken != "" || result.Cookies != "" {
			t.Fatalf("expected access token alone to avoid fabricating session bootstrap data, got %+v", result)
		}

		result, err = adapters.SessionAdapter.BootstrapSessionToken(context.Background(), accounts.Account{
			ID:           43,
			Email:        "refresh-only@example.com",
			RefreshToken: "refresh-bootstrap",
		}, "")
		if err != nil {
			t.Fatalf("expected refresh-only bootstrap compatibility response: %v", err)
		}
		if result.SessionToken != "" {
			t.Fatalf("expected refresh token alone to avoid fabricating session token, got %+v", result)
		}

		result, err = adapters.SessionAdapter.BootstrapSessionToken(context.Background(), accounts.Account{
			ID:      44,
			Email:   "cookie-session@example.com",
			Cookies: "__Secure-next-auth.session-token=session-cookie; oai-did=device-cookie",
		}, "")
		if err != nil {
			t.Fatalf("expected real session cookie bootstrap compatibility response: %v", err)
		}
		if result.SessionToken != "session-cookie" {
			t.Fatalf("expected bootstrap to reuse existing session cookie, got %+v", result)
		}
		if result.Cookies == "" {
			t.Fatalf("expected bootstrap to preserve session cookie payload, got %+v", result)
		}
	})

	t.Run("session_bootstrap_without_real_session_token_does_not_persist_placeholder", func(t *testing.T) {
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:          45,
				Email:       "bootstrap@example.com",
				AccessToken: "access-bootstrap",
				Cookies:     "oai-did=device-bootstrap",
			},
		}
		service := NewService(
			&fakePaymentRepository{},
			accountRepo,
			WithSessionAdapter(adapters.SessionAdapter),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.BootstrapAccountSessionToken(context.Background(), 45, "")
		if err != nil {
			t.Fatalf("expected transition session bootstrap adapter to avoid missing seam: %v", err)
		}
		if resp.Success || resp.SessionTokenLen != 0 {
			t.Fatalf("expected bootstrap to stay unbootstrapped without a real session token, got %+v", resp)
		}
		if accountRepo.savedAccount.ID != 0 || accountRepo.savedAccount.SessionToken != "" {
			t.Fatalf("expected bootstrap to avoid persisting fabricated session data, got %+v", accountRepo.savedAccount)
		}
	})

	t.Run("create_bind_card_task_auto_open_stays_truthful", func(t *testing.T) {
		paymentRepo := &fakePaymentRepository{
			task: BindCardTask{
				ID:           46,
				AccountID:    41,
				AccountEmail: "alpha@example.com",
				PlanType:     "plus",
				Status:       StatusLinkReady,
				CheckoutURL:  "https://chatgpt.com/checkout/openai_llc/cs_transition_truthful_auto_open",
			},
		}
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:           41,
				Email:        "alpha@example.com",
				SessionToken: "session-live",
				Cookies:      "__Secure-next-auth.session-token=session-live; oai-did=device-live",
			},
		}
		service := NewService(
			paymentRepo,
			accountRepo,
			WithCheckoutLinkGenerator(adapters.CheckoutLinkGenerator),
			WithBrowserOpener(adapters.BrowserOpener),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.CreateBindCardTask(context.Background(), CreateBindCardTaskRequest{
			CheckoutRequestBase: CheckoutRequestBase{
				AccountID: 41,
				PlanType:  "plus",
				Country:   "US",
				Currency:  "USD",
				AutoOpen:  true,
			},
			BindMode: "semi_auto",
		})
		if err != nil {
			t.Fatalf("expected transition create-bind-card adapter path to stay compatible: %v", err)
		}
		if resp.AutoOpened {
			t.Fatalf("expected auto-open to remain false without a real browser launch, got %+v", resp)
		}
		if resp.Task.Status != StatusLinkReady {
			t.Fatalf("expected task to remain link_ready when no browser launch occurred, got %+v", resp.Task)
		}
		if paymentRepo.savedTask.ID != 0 {
			t.Fatalf("expected no opened-status writeback when browser launch did not happen, got %+v", paymentRepo.savedTask)
		}
	})
}

func TestPaymentTransitionAdapterBillingProfilesStayCountryConsistent(t *testing.T) {
	adapter := NewTransitionAdapters().BillingProfileGenerator
	expected := map[string]struct {
		currency string
		city     string
		state    string
		postal   string
	}{
		"US": {currency: "USD", city: "San Jose", state: "CA", postal: "95112"},
		"GB": {currency: "GBP", city: "London", state: "London", postal: "SW1A 1AA"},
		"CA": {currency: "CAD", city: "Toronto", state: "ON", postal: "M5V 2T6"},
		"AU": {currency: "AUD", city: "Sydney", state: "NSW", postal: "2000"},
		"SG": {currency: "SGD", city: "Singapore", state: "SG", postal: "018956"},
		"HK": {currency: "HKD", city: "Hong Kong", state: "HK", postal: "000000"},
		"JP": {currency: "JPY", city: "Tokyo", state: "Tokyo", postal: "100-0001"},
		"DE": {currency: "EUR", city: "Berlin", state: "BE", postal: "10115"},
		"FR": {currency: "EUR", city: "Paris", state: "IDF", postal: "75001"},
		"IT": {currency: "EUR", city: "Rome", state: "RM", postal: "00118"},
		"ES": {currency: "EUR", city: "Madrid", state: "MD", postal: "28001"},
	}

	for country, want := range expected {
		t.Run(country, func(t *testing.T) {
			profile, err := adapter.GenerateRandomBillingProfile(context.Background(), country, "")
			if err != nil {
				t.Fatalf("expected supported country %s to generate profile: %v", country, err)
			}
			if got := profile["country"]; got != country {
				t.Fatalf("expected country %s, got %v", country, got)
			}
			if got := profile["currency"]; got != want.currency {
				t.Fatalf("expected currency %s for %s, got %v", want.currency, country, got)
			}
			if got := profile["city"]; got != want.city {
				t.Fatalf("expected city %s for %s, got %v", want.city, country, got)
			}
			if got := profile["state"]; got != want.state {
				t.Fatalf("expected state %s for %s, got %v", want.state, country, got)
			}
			if got := profile["postal_code"]; got != want.postal {
				t.Fatalf("expected postal_code %s for %s, got %v", want.postal, country, got)
			}
			if got := profile["source"]; got != "local_template" {
				t.Fatalf("expected local template source for %s, got %v", country, got)
			}
		})
	}

	t.Run("unsupported_country_falls_back_to_us_template", func(t *testing.T) {
		profile, err := adapter.GenerateRandomBillingProfile(context.Background(), "BR", "")
		if err != nil {
			t.Fatalf("expected fallback profile, got error: %v", err)
		}
		if got := profile["country"]; got != "US" {
			t.Fatalf("expected fallback country US, got %v", got)
		}
		if got := profile["currency"]; got != "USD" {
			t.Fatalf("expected fallback currency USD, got %v", got)
		}
		if got := profile["state"]; got != "CA" || profile["postal_code"] != "95112" {
			t.Fatalf("expected fallback US template, got %+v", profile)
		}
	})
}

func TestPaymentTransitionAdapterCheckoutSessionIDUsesURLSafeWorkspaceSlug(t *testing.T) {
	urlSafeSessionID := regexp.MustCompile(`^cs_[A-Za-z0-9_-]+$`)

	t.Run("simple_ascii_workspace_keeps_compatibility_shape", func(t *testing.T) {
		got := buildTransitionCheckoutSessionID(7, "team", "Acme Team-01")
		want := "cs_transition_team_7_acme_team_01"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("unsafe_and_non_ascii_workspace_names_are_normalized", func(t *testing.T) {
		workspaceName := "研发/Team?50%#1"
		got := buildTransitionCheckoutSessionID(7, "team", workspaceName)
		if got == "cs_transition_team_7_研发/team?50%#1" {
			t.Fatalf("expected workspace name to be normalized, got raw session id %q", got)
		}
		if !urlSafeSessionID.MatchString(got) {
			t.Fatalf("expected URL-safe session id, got %q", got)
		}
		if gotAgain := buildTransitionCheckoutSessionID(7, "team", workspaceName); gotAgain != got {
			t.Fatalf("expected deterministic session id, got %q then %q", got, gotAgain)
		}
	})
}

func TestPaymentSubscriptionTransitionAdapterPreservesConfidenceGate(t *testing.T) {
	now := time.Date(2026, 4, 6, 9, 15, 0, 0, time.UTC)
	adapters := NewTransitionAdapters()

	t.Run("low_confidence_free_preserves_paid_state", func(t *testing.T) {
		paymentRepo := &fakePaymentRepository{
			task: BindCardTask{
				ID:          51,
				AccountID:   61,
				PlanType:    "plus",
				Status:      StatusOpened,
				CheckoutURL: "https://chatgpt.com/checkout/openai_llc/cs_low_confidence",
			},
		}
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:               61,
				Email:            "paid-low@example.com",
				SubscriptionType: "team",
				ExtraData: map[string]any{
					"payment_subscription_status":     "free",
					"payment_subscription_confidence": "low",
					"payment_subscription_source":     "transition.test.low",
				},
			},
		}
		service := NewService(
			paymentRepo,
			accountRepo,
			WithSubscriptionChecker(adapters.SubscriptionChecker),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.SyncBindCardTaskSubscription(context.Background(), 51, SyncBindCardTaskRequest{})
		if err != nil {
			t.Fatalf("expected low-confidence transition subscription check to avoid missing seam: %v", err)
		}
		if resp.Task.Status != StatusPaidPendingSync {
			t.Fatalf("expected low-confidence free to stay paid_pending_sync, got %+v", resp)
		}
		if accountRepo.savedAccount.SubscriptionType != "team" {
			t.Fatalf("expected paid state to be preserved on low confidence free, got %+v", accountRepo.savedAccount)
		}
		if resp.Detail["confidence"] != "low" {
			t.Fatalf("expected low confidence detail, got %+v", resp.Detail)
		}
	})

	t.Run("high_confidence_free_clears_paid_state", func(t *testing.T) {
		paymentRepo := &fakePaymentRepository{
			task: BindCardTask{
				ID:        52,
				AccountID: 62,
				PlanType:  "plus",
				Status:    StatusPaidPendingSync,
			},
		}
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:               62,
				Email:            "paid-high@example.com",
				SubscriptionType: "plus",
				SubscriptionAt:   timePtr(now.Add(-2 * time.Hour)),
				ExtraData: map[string]any{
					"payment_subscription_status":     "free",
					"payment_subscription_confidence": "high",
					"payment_subscription_source":     "transition.test.high",
				},
			},
		}
		service := NewService(
			paymentRepo,
			accountRepo,
			WithSubscriptionChecker(adapters.SubscriptionChecker),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.SyncBindCardTaskSubscription(context.Background(), 52, SyncBindCardTaskRequest{})
		if err != nil {
			t.Fatalf("expected high-confidence transition subscription check to avoid missing seam: %v", err)
		}
		if resp.Task.Status != StatusWaitingUserAction {
			t.Fatalf("expected high-confidence free to move back to waiting_user_action, got %+v", resp)
		}
		if accountRepo.savedAccount.SubscriptionType != "" || accountRepo.savedAccount.SubscriptionAt != nil {
			t.Fatalf("expected paid state to clear on high confidence free, got %+v", accountRepo.savedAccount)
		}
		if resp.Detail["confidence"] != "high" {
			t.Fatalf("expected high confidence detail, got %+v", resp.Detail)
		}
	})
}

func TestPaymentAutoBindTransitionAdaptersReturnCompatibilityStates(t *testing.T) {
	now := time.Date(2026, 4, 6, 9, 30, 0, 0, time.UTC)
	adapters := NewTransitionAdapters()

	t.Run("third_party_auto_bind", func(t *testing.T) {
		paymentRepo := &fakePaymentRepository{
			task: BindCardTask{
				ID:           71,
				AccountID:    81,
				AccountEmail: "bind@example.com",
				PlanType:     "plus",
				Status:       StatusOpened,
				CheckoutURL:  "https://chatgpt.com/checkout/openai_llc/cs_transition_bind",
			},
		}
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:           81,
				Email:        "bind@example.com",
				SessionToken: "session-bind",
				Cookies:      "__Secure-next-auth.session-token=session-bind; oai-did=device-bind",
			},
		}
		service := NewService(
			paymentRepo,
			accountRepo,
			WithAutoBinder(adapters.AutoBinder),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.AutoBindBindCardTaskThirdParty(context.Background(), 71, ThirdPartyAutoBindRequest{
			Card:    ThirdPartyCardRequest{Number: "4242424242424242", ExpMonth: "01", ExpYear: "30", CVC: "123"},
			Profile: ThirdPartyProfileRequest{Name: "Ada", Country: "GB"},
		})
		if err != nil {
			t.Fatalf("expected transition third-party auto bind to avoid missing seam: %v", err)
		}
		if !resp.Pending || !resp.NeedUserAction || resp.ThirdParty == nil {
			t.Fatalf("expected compatibility pending third-party result, got %+v", resp)
		}
		if paymentRepo.savedTask.Status != StatusWaitingUserAction {
			t.Fatalf("expected task to move to waiting_user_action, got %+v", paymentRepo.savedTask)
		}
	})

	t.Run("local_auto_bind", func(t *testing.T) {
		paymentRepo := &fakePaymentRepository{
			task: BindCardTask{
				ID:           72,
				AccountID:    82,
				AccountEmail: "local@example.com",
				PlanType:     "plus",
				Status:       StatusOpened,
				CheckoutURL:  "https://chatgpt.com/checkout/openai_llc/cs_transition_local",
			},
		}
		accountRepo := &fakeAccountRepository{
			accountByID: accounts.Account{
				ID:           82,
				Email:        "local@example.com",
				SessionToken: "session-local",
				Cookies:      "__Secure-next-auth.session-token=session-local; oai-did=device-local",
			},
		}
		service := NewService(
			paymentRepo,
			accountRepo,
			WithAutoBinder(adapters.AutoBinder),
			WithNow(func() time.Time { return now }),
		)

		resp, err := service.AutoBindBindCardTaskLocal(context.Background(), 72, LocalAutoBindRequest{
			Card:    ThirdPartyCardRequest{Number: "4242424242424242", ExpMonth: "01", ExpYear: "30", CVC: "123"},
			Profile: ThirdPartyProfileRequest{Name: "Ada", Country: "GB"},
		})
		if err != nil {
			t.Fatalf("expected transition local auto bind to avoid missing seam: %v", err)
		}
		if !resp.Pending || !resp.NeedUserAction || resp.LocalAuto == nil {
			t.Fatalf("expected compatibility pending local result, got %+v", resp)
		}
		if paymentRepo.savedTask.Status != StatusWaitingUserAction {
			t.Fatalf("expected task to move to waiting_user_action, got %+v", paymentRepo.savedTask)
		}
	})
}
