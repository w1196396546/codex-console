package nativerunner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTokenCompletionSchedulerChoosesPasswordlessWhenPasswordMissing(t *testing.T) {
	t.Parallel()

	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{})
	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email: "native@example.com",
	}, nil)

	if !plan.Allowed {
		t.Fatalf("expected plan allowed, got %+v", plan)
	}
	if plan.Strategy != TokenCompletionStrategyPasswordless {
		t.Fatalf("expected passwordless strategy, got %q", plan.Strategy)
	}
	if plan.BlockedReason != nil {
		t.Fatalf("expected no blocked reason, got %+v", plan.BlockedReason)
	}
}

func TestTokenCompletionSchedulerRejectsConcurrentAttemptForSameEmail(t *testing.T) {
	t.Parallel()

	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{})
	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email:    "native@example.com",
		Password: "Password123!",
	}, []TokenCompletionAttempt{
		{
			Email:     "native@example.com",
			State:     TokenCompletionStateRunning,
			StartedAt: now.Add(-1 * time.Minute),
		},
	})

	if plan.Allowed {
		t.Fatalf("expected plan blocked, got %+v", plan)
	}
	if plan.BlockedReason == nil || plan.BlockedReason.Kind != TokenCompletionErrorKindEmailConflict {
		t.Fatalf("expected email conflict, got %+v", plan.BlockedReason)
	}
}

func TestTokenCompletionSchedulerAppliesSpacingBetweenAttempts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
		MinSpacing: 10 * time.Minute,
	})

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email:    "native@example.com",
		Password: "Password123!",
	}, []TokenCompletionAttempt{
		{
			Email:     "native@example.com",
			State:     TokenCompletionStateCompleted,
			StartedAt: now.Add(-5 * time.Minute),
		},
	})

	if plan.Allowed {
		t.Fatalf("expected plan blocked by spacing, got %+v", plan)
	}
	if plan.BlockedReason == nil || plan.BlockedReason.Kind != TokenCompletionErrorKindSpacingActive {
		t.Fatalf("expected spacing error, got %+v", plan.BlockedReason)
	}
	if plan.NextEligibleAt == nil || !plan.NextEligibleAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("expected next eligible at %s, got %+v", now.Add(5*time.Minute), plan.NextEligibleAt)
	}
}

func TestTokenCompletionSchedulerAppliesExponentialBackoffForRetryableFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
		BaseBackoff: 1 * time.Minute,
		MaxBackoff:  10 * time.Minute,
	})

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email:    "native@example.com",
		Password: "Password123!",
	}, []TokenCompletionAttempt{
		{
			Email:       "native@example.com",
			State:       TokenCompletionStateFailed,
			CompletedAt: now.Add(-10 * time.Minute),
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Retryable: true,
			},
		},
		{
			Email:       "native@example.com",
			State:       TokenCompletionStateFailed,
			CompletedAt: now.Add(-1 * time.Minute),
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Retryable: true,
			},
		},
	})

	if plan.Allowed {
		t.Fatalf("expected plan blocked by backoff, got %+v", plan)
	}
	if plan.BlockedReason == nil || plan.BlockedReason.Kind != TokenCompletionErrorKindBackoffActive {
		t.Fatalf("expected backoff error, got %+v", plan.BlockedReason)
	}
	if plan.NextEligibleAt == nil || !plan.NextEligibleAt.Equal(now.Add(1*time.Minute)) {
		t.Fatalf("expected next eligible at %s, got %+v", now.Add(1*time.Minute), plan.NextEligibleAt)
	}
}

func TestTokenCompletionSchedulerAppliesCooldownForRateLimitedFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
		Cooldown: 15 * time.Minute,
	})

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email:    "native@example.com",
		Password: "Password123!",
	}, []TokenCompletionAttempt{
		{
			Email:       "native@example.com",
			State:       TokenCompletionStateFailed,
			CompletedAt: now.Add(-5 * time.Minute),
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindRateLimited,
				Retryable: true,
			},
		},
	})

	if plan.Allowed {
		t.Fatalf("expected plan blocked by cooldown, got %+v", plan)
	}
	if plan.BlockedReason == nil || plan.BlockedReason.Kind != TokenCompletionErrorKindCooldownActive {
		t.Fatalf("expected cooldown error, got %+v", plan.BlockedReason)
	}
	if plan.NextEligibleAt == nil || !plan.NextEligibleAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("expected next eligible at %s, got %+v", now.Add(10*time.Minute), plan.NextEligibleAt)
	}
}

func TestTokenCompletionCoordinatorCallsProviderWithScheduledStrategy(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	provider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
			AccountID:   "account-123",
		},
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{}),
		Provider:  provider,
		Now: func() time.Time {
			return now
		},
	})

	result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email: "native@example.com",
		},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected provider called once, got %d", provider.calls)
	}
	if provider.lastRequest.Strategy != TokenCompletionStrategyPasswordless {
		t.Fatalf("expected provider request strategy passwordless, got %q", provider.lastRequest.Strategy)
	}
	if result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if result.Provider.AccessToken != "access-123" {
		t.Fatalf("expected provider access token, got %+v", result.Provider)
	}
}

func TestTokenCompletionCoordinatorClassifiesProviderError(t *testing.T) {
	t.Parallel()

	provider := &stubTokenCompletionProvider{
		err: &TokenCompletionError{
			Kind:      TokenCompletionErrorKindRateLimited,
			Message:   "rate limited",
			Retryable: true,
		},
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{}),
		Provider:  provider,
		Now: func() time.Time {
			return time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
		},
	})

	result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email:    "native@example.com",
			Password: "Password123!",
		},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if result.State != TokenCompletionStateFailed {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if result.Error == nil || result.Error.Kind != TokenCompletionErrorKindRateLimited {
		t.Fatalf("expected rate-limited error, got %+v", result.Error)
	}
}

func TestTokenCompletionCoordinatorRejectsMissingProvider(t *testing.T) {
	t.Parallel()

	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{}),
	})

	_, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{Email: "native@example.com"},
	})
	if err == nil {
		t.Fatal("expected missing provider error")
	}
	if err.Error() != "token completion provider is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type stubTokenCompletionProvider struct {
	result      TokenCompletionProviderResult
	err         error
	calls       int
	lastRequest TokenCompletionRequest
}

func (s *stubTokenCompletionProvider) CompleteToken(_ context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	s.calls++
	s.lastRequest = request
	if s.err != nil {
		return TokenCompletionProviderResult{}, s.err
	}
	return s.result, nil
}

func TestTokenCompletionErrorWrapsUnknownProviderFailuresAsInternal(t *testing.T) {
	t.Parallel()

	got := classifyTokenCompletionError(errors.New("boom"))
	if got.Kind != TokenCompletionErrorKindInternal {
		t.Fatalf("expected internal error, got %+v", got)
	}
	if got.Retryable {
		t.Fatalf("expected internal error to be non-retryable, got %+v", got)
	}
}
