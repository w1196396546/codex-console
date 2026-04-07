package nativerunner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
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
	if !plan.BlockedReason.Retryable {
		t.Fatalf("expected email conflict to be retryable, got %+v", plan.BlockedReason)
	}
}

func TestTokenCompletionSchedulerIgnoresStaleRunningAttemptAfterTimeout(t *testing.T) {
	t.Parallel()

	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
		RunningTimeout: 5 * time.Minute,
	})
	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email: "native@example.com",
	}, []TokenCompletionAttempt{
		{
			Email:     "native@example.com",
			State:     TokenCompletionStateRunning,
			StartedAt: now.Add(-10 * time.Minute),
		},
	})

	if !plan.Allowed {
		t.Fatalf("expected stale running attempt to be ignored, got %+v", plan)
	}
	if plan.BlockedReason != nil {
		t.Fatalf("expected no blocked reason for stale running attempt, got %+v", plan.BlockedReason)
	}
}

func TestTokenCompletionSchedulerTreatsRecentHeartbeatAsActiveLease(t *testing.T) {
	t.Parallel()

	scheduler := NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
		RunningTimeout: 5 * time.Minute,
	})
	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)

	plan := scheduler.Plan(now, TokenCompletionAccount{
		Email: "native@example.com",
	}, []TokenCompletionAttempt{
		{
			Email:       "native@example.com",
			State:       TokenCompletionStateRunning,
			StartedAt:   now.Add(-10 * time.Minute),
			HeartbeatAt: now.Add(-1 * time.Minute),
		},
	})

	if plan.Allowed {
		t.Fatalf("expected recent heartbeat to keep running attempt active, got %+v", plan)
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

func TestParseTokenCompletionRuntimeStateReadsPersistedAttemptsAndCooldown(t *testing.T) {
	t.Parallel()

	runtimeState, err := ParseTokenCompletionRuntimeState(map[string]any{
		"token_completion_attempts": []any{
			map[string]any{
				"email":        "native@example.com",
				"state":        "failed",
				"started_at":   "2026-04-05T09:58:00Z",
				"completed_at": "2026-04-05T09:59:00Z",
				"error": map[string]any{
					"kind":      "provider_unavailable",
					"message":   "temporary outage",
					"retryable": true,
				},
			},
		},
		"refresh_token_cooldown_until": "2026-04-05T10:07:00Z",
	}, "native@example.com")
	if err != nil {
		t.Fatalf("parse token completion runtime state: %v", err)
	}

	if runtimeState.CooldownUntil == nil {
		t.Fatal("expected cooldown timestamp")
	}
	expectedCooldown := time.Date(2026, time.April, 5, 10, 7, 0, 0, time.UTC)
	if !runtimeState.CooldownUntil.Equal(expectedCooldown) {
		t.Fatalf("expected cooldown %v, got %+v", expectedCooldown, runtimeState.CooldownUntil)
	}
	if len(runtimeState.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %+v", runtimeState.Attempts)
	}
	if runtimeState.Attempts[0].State != TokenCompletionStateFailed {
		t.Fatalf("expected failed attempt, got %+v", runtimeState.Attempts[0])
	}
	if runtimeState.Attempts[0].Error == nil || runtimeState.Attempts[0].Error.Kind != TokenCompletionErrorKindProviderUnavailable {
		t.Fatalf("expected provider unavailable error, got %+v", runtimeState.Attempts[0].Error)
	}
}

func TestBuildTokenCompletionRuntimeStateNormalizesCompletionResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	cooldownUntil := now.Add(5 * time.Minute)
	state := BuildTokenCompletionRuntimeState(now, "native@example.com", TokenCompletionRuntimeState{
		CooldownUntil: &cooldownUntil,
	}, TokenCompletionResult{
		State:    TokenCompletionStateCompleted,
		Email:    "native@example.com",
		Strategy: TokenCompletionStrategyPasswordless,
	})

	if state.CooldownUntil != nil {
		t.Fatalf("expected completed state to clear cooldown, got %+v", state.CooldownUntil)
	}
	if len(state.Attempts) != 1 {
		t.Fatalf("expected 1 persisted attempt, got %+v", state.Attempts)
	}
	if state.Attempts[0].State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed attempt, got %+v", state.Attempts[0])
	}
	if !state.Attempts[0].StartedAt.Equal(now) || !state.Attempts[0].CompletedAt.Equal(now) {
		t.Fatalf("expected completion timestamps to normalize to now, got %+v", state.Attempts[0])
	}
}

func TestBuildTokenCompletionRuntimeStateNormalizesFailureResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	state := BuildTokenCompletionRuntimeState(now, "native@example.com", TokenCompletionRuntimeState{}, TokenCompletionResult{
		State: TokenCompletionStateFailed,
		Email: "native@example.com",
		Error: &TokenCompletionError{
			Kind:      TokenCompletionErrorKindProviderUnavailable,
			Message:   "temporary outage",
			Retryable: true,
		},
	})

	if state.CooldownUntil != nil {
		t.Fatalf("expected failed state without explicit cooldown to keep cooldown empty, got %+v", state.CooldownUntil)
	}
	if len(state.Attempts) != 1 {
		t.Fatalf("expected 1 failed attempt, got %+v", state.Attempts)
	}
	if state.Attempts[0].Error == nil || state.Attempts[0].Error.Kind != TokenCompletionErrorKindProviderUnavailable {
		t.Fatalf("expected failed attempt error copied, got %+v", state.Attempts[0])
	}
}

func TestBuildTokenCompletionRuntimeStateNormalizesBlockedResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	blockedUntil := now.Add(7 * time.Minute)
	state := BuildTokenCompletionRuntimeState(now, "native@example.com", TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateFailed,
				CompletedAt: now.Add(-1 * time.Hour),
			},
		},
	}, TokenCompletionResult{
		State:          TokenCompletionStateBlocked,
		Email:          "native@example.com",
		NextEligibleAt: &blockedUntil,
		Error: &TokenCompletionError{
			Kind:      TokenCompletionErrorKindCooldownActive,
			Message:   "token completion cooldown active",
			Retryable: true,
		},
	})

	if len(state.Attempts) != 1 {
		t.Fatalf("expected blocked state not to append attempts, got %+v", state.Attempts)
	}
	if state.CooldownUntil == nil || !state.CooldownUntil.Equal(blockedUntil) {
		t.Fatalf("expected blocked state to persist cooldown, got %+v", state.CooldownUntil)
	}
}

func TestBuildTokenCompletionRuntimeStatePersistsRunningResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	state := BuildTokenCompletionRuntimeState(now, "native@example.com", TokenCompletionRuntimeState{}, TokenCompletionResult{
		State:    TokenCompletionStateRunning,
		Email:    "native@example.com",
		Strategy: TokenCompletionStrategyPasswordless,
	})

	if len(state.Attempts) != 1 {
		t.Fatalf("expected one running attempt, got %+v", state.Attempts)
	}
	if state.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected running attempt, got %+v", state.Attempts[0])
	}
	if !state.Attempts[0].StartedAt.Equal(now) {
		t.Fatalf("expected running started_at %v, got %+v", now, state.Attempts[0].StartedAt)
	}
	if !state.Attempts[0].CompletedAt.IsZero() {
		t.Fatalf("expected running attempt completed_at to stay empty, got %+v", state.Attempts[0].CompletedAt)
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

func TestTokenCompletionCoordinatorBlocksWhenCooldownUntilIsStillActive(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	provider := &stubTokenCompletionProvider{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{}),
		Provider:  provider,
		Now: func() time.Time {
			return now
		},
	})

	cooldownUntil := now.Add(7 * time.Minute)
	result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email: "native@example.com",
		},
		CooldownUntil: &cooldownUntil,
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called while cooldown active, got %d", provider.calls)
	}
	if result.State != TokenCompletionStateBlocked {
		t.Fatalf("expected blocked state, got %+v", result)
	}
	if result.Error == nil || result.Error.Kind != TokenCompletionErrorKindCooldownActive {
		t.Fatalf("expected cooldown_active error, got %+v", result.Error)
	}
	if result.NextEligibleAt == nil || !result.NextEligibleAt.Equal(cooldownUntil) {
		t.Fatalf("expected next eligible at %s, got %+v", cooldownUntil, result.NextEligibleAt)
	}
}

func TestTokenCompletionCoordinatorPersistsRunningAttemptToRuntimeStore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	provider := &blockingTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
			AccountID:   "account-123",
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	store := &stubTokenCompletionRuntimeStore{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 15 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		Now: func() time.Time {
			return now
		},
	})

	firstResultCh := make(chan TokenCompletionResult, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		firstResultCh <- result
		firstErrCh <- err
	}()

	<-provider.started

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 1 || persisted.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected running attempt persisted before provider completion, got %+v", persisted.Attempts)
	}

	blocked, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email: "native@example.com",
		},
	})
	if err != nil {
		t.Fatalf("complete second token request: %v", err)
	}
	if blocked.State != TokenCompletionStateBlocked {
		t.Fatalf("expected second request blocked, got %+v", blocked)
	}
	if blocked.Error == nil || blocked.Error.Kind != TokenCompletionErrorKindEmailConflict || !blocked.Error.Retryable {
		t.Fatalf("expected retryable email conflict, got %+v", blocked.Error)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called only for first request, got %d", provider.calls)
	}

	close(provider.release)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("complete first token request: %v", err)
	}
	first := <-firstResultCh
	if first.State != TokenCompletionStateCompleted {
		t.Fatalf("expected first request completed, got %+v", first)
	}
}

func TestTokenCompletionCoordinatorClaimsRunningAttemptAtomicallyAcrossConcurrentCalls(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	release := make(chan struct{})

	provider := &countingBlockingTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
		started: make(chan struct{}, 2),
		release: release,
	}
	store := &compareAndSwapTokenCompletionRuntimeStore{
		twoLoads:        make(chan struct{}),
		allowLegacySave: make(chan struct{}),
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 15 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		Now: func() time.Time {
			return now
		},
	})

	type completionOutcome struct {
		result TokenCompletionResult
		err    error
	}
	outcomes := make(chan completionOutcome, 2)
	runComplete := func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		outcomes <- completionOutcome{result: result, err: err}
	}

	go runComplete()
	go runComplete()

	select {
	case <-store.twoLoads:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent runtime loads")
	}

	close(store.allowLegacySave)

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first provider invocation")
	}

	select {
	case outcome := <-outcomes:
		if outcome.err != nil {
			t.Fatalf("complete token: %v", outcome.err)
		}
		if outcome.result.State != TokenCompletionStateBlocked {
			t.Fatalf("expected one concurrent request to block, got %+v", outcome.result)
		}
		if outcome.result.Error == nil || outcome.result.Error.Kind != TokenCompletionErrorKindEmailConflict {
			t.Fatalf("expected retryable email conflict, got %+v", outcome.result.Error)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected second concurrent request to return while first provider call is still running")
	}

	select {
	case <-provider.started:
		t.Fatal("expected provider to be called only once for concurrent claim")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTokenCompletionCoordinatorRenewsRunningLeaseWhileProviderStillRunning(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})

	provider := &countingBlockingTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
		started: make(chan struct{}, 2),
		release: release,
	}
	store := &stubTokenCompletionRuntimeStore{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 40 * time.Millisecond,
		}),
		Provider:          provider,
		RuntimeStore:      store,
		HeartbeatInterval: 10 * time.Millisecond,
	})

	firstResultCh := make(chan TokenCompletionResult, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		firstResultCh <- result
		firstErrCh <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial provider invocation")
	}

	time.Sleep(70 * time.Millisecond)

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 1 || persisted.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected running attempt while provider still executing, got %+v", persisted.Attempts)
	}
	if persisted.Attempts[0].HeartbeatAt.IsZero() {
		t.Fatalf("expected running attempt heartbeat to be renewed, got %+v", persisted.Attempts[0])
	}

	outcomeCh := make(chan struct {
		result TokenCompletionResult
		err    error
	}, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		outcomeCh <- struct {
			result TokenCompletionResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case outcome := <-outcomeCh:
		if outcome.err != nil {
			t.Fatalf("complete second token request: %v", outcome.err)
		}
		if outcome.result.State != TokenCompletionStateBlocked {
			t.Fatalf("expected second request blocked while heartbeat is active, got %+v", outcome.result)
		}
		if outcome.result.Error == nil || outcome.result.Error.Kind != TokenCompletionErrorKindEmailConflict {
			t.Fatalf("expected retryable email conflict, got %+v", outcome.result.Error)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected second request to return while first provider call is still running")
	}

	select {
	case <-provider.started:
		t.Fatal("expected provider lease heartbeat to prevent duplicate provider invocation")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("complete first token request: %v", err)
	}
	first := <-firstResultCh
	if first.State != TokenCompletionStateCompleted {
		t.Fatalf("expected first request completed, got %+v", first)
	}
}

func TestTokenCompletionCoordinatorPersistsTerminalStateToRuntimeStore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)

	t.Run("completed", func(t *testing.T) {
		store := &stubTokenCompletionRuntimeStore{}
		coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
			Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
				RunningTimeout: 15 * time.Minute,
			}),
			Provider: &stubTokenCompletionProvider{
				result: TokenCompletionProviderResult{
					AccessToken: "access-123",
				},
			},
			RuntimeStore: store,
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
		if result.State != TokenCompletionStateCompleted {
			t.Fatalf("expected completed result, got %+v", result)
		}

		persisted, err := store.Load(context.Background(), "native@example.com")
		if err != nil {
			t.Fatalf("load runtime state: %v", err)
		}
		if len(persisted.Attempts) != 1 {
			t.Fatalf("expected one persisted attempt, got %+v", persisted.Attempts)
		}
		if persisted.Attempts[0].State != TokenCompletionStateCompleted {
			t.Fatalf("expected persisted completed attempt, got %+v", persisted.Attempts[0])
		}
		if persisted.Attempts[0].CompletedAt.IsZero() {
			t.Fatalf("expected completed attempt to persist completed_at, got %+v", persisted.Attempts[0])
		}
	})

	t.Run("failed", func(t *testing.T) {
		store := &stubTokenCompletionRuntimeStore{}
		coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
			Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
				RunningTimeout: 15 * time.Minute,
			}),
			Provider: &stubTokenCompletionProvider{
				err: &TokenCompletionError{
					Kind:      TokenCompletionErrorKindProviderUnavailable,
					Message:   "temporary outage",
					Retryable: true,
				},
			},
			RuntimeStore: store,
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
		if result.State != TokenCompletionStateFailed {
			t.Fatalf("expected failed result, got %+v", result)
		}

		persisted, err := store.Load(context.Background(), "native@example.com")
		if err != nil {
			t.Fatalf("load runtime state: %v", err)
		}
		if len(persisted.Attempts) != 1 {
			t.Fatalf("expected one persisted attempt, got %+v", persisted.Attempts)
		}
		if persisted.Attempts[0].State != TokenCompletionStateFailed {
			t.Fatalf("expected persisted failed attempt, got %+v", persisted.Attempts[0])
		}
		if persisted.Attempts[0].Error == nil || persisted.Attempts[0].Error.Kind != TokenCompletionErrorKindProviderUnavailable {
			t.Fatalf("expected persisted failed error, got %+v", persisted.Attempts[0])
		}
	})
}

func TestTokenCompletionCoordinatorDoesNotOverwriteNewerRunningLeaseOnFinalize(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	provider := &countingBlockingTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
		started: make(chan struct{}, 1),
		release: release,
	}
	store := &compareAndSwapTokenCompletionRuntimeStore{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 15 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		Now: func() time.Time {
			return time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
		},
	})

	resultCh := make(chan TokenCompletionResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		resultCh <- result
		errCh <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider start")
	}

	replacementStartedAt := time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC)
	replacementHeartbeatAt := replacementStartedAt.Add(15 * time.Second)
	store.setState(TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				StartedAt:   replacementStartedAt,
				HeartbeatAt: replacementHeartbeatAt,
			},
		},
	})

	close(release)

	if err := <-errCh; err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if result := <-resultCh; result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 1 {
		t.Fatalf("expected newer running lease preserved, got %+v", persisted.Attempts)
	}
	if persisted.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected newer running lease preserved, got %+v", persisted.Attempts[0])
	}
	if !persisted.Attempts[0].StartedAt.Equal(replacementStartedAt) {
		t.Fatalf("expected replacement running started_at preserved, got %+v", persisted.Attempts[0])
	}
	if !persisted.Attempts[0].HeartbeatAt.Equal(replacementHeartbeatAt) {
		t.Fatalf("expected replacement running heartbeat preserved, got %+v", persisted.Attempts[0])
	}
}

func TestTokenCompletionCoordinatorDoesNotFinalizeExpiredLeaseWithoutReplacement(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC)
	staleStartedAt := now.Add(-20 * time.Minute)
	staleHeartbeatAt := now.Add(-6 * time.Minute)
	store := &compareAndSwapTokenCompletionRuntimeStore{}
	store.setState(TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				LeaseToken:  "lease-stale",
				StartedAt:   staleStartedAt,
				HeartbeatAt: staleHeartbeatAt,
			},
		},
	})
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 5 * time.Minute,
		}),
		Provider:     &stubTokenCompletionProvider{},
		RuntimeStore: store,
		Now: func() time.Time {
			return now
		},
	})

	err := coordinator.persistRuntimeState(context.Background(), now, "native@example.com", TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				LeaseToken:  "lease-stale",
				StartedAt:   staleStartedAt,
				HeartbeatAt: staleHeartbeatAt,
			},
		},
	}, TokenCompletionResult{
		State:      TokenCompletionStateCompleted,
		Email:      "native@example.com",
		LeaseToken: "lease-stale",
		Strategy:   TokenCompletionStrategyPasswordless,
		Provider: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
	})
	if err != nil {
		t.Fatalf("persist runtime state: %v", err)
	}

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 1 {
		t.Fatalf("expected stale running attempt preserved, got %+v", persisted.Attempts)
	}
	if persisted.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected expired lease finalize to be fenced, got %+v", persisted.Attempts[0])
	}
	if persisted.Attempts[0].LeaseToken != "lease-stale" {
		t.Fatalf("expected stale lease token preserved, got %+v", persisted.Attempts[0])
	}
}

func TestTokenCompletionCoordinatorAllowsClaimWhenExternalLeaseExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC)
	store := &compareAndSwapTokenCompletionRuntimeStore{}
	store.setState(TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				LeaseToken:  "lease-stale",
				StartedAt:   now.Add(-2 * time.Minute),
				HeartbeatAt: now.Add(-30 * time.Second),
			},
		},
	})
	leaseStore := newStubTokenCompletionLeaseStore()
	leaseStore.active["native@example.com|lease-stale"] = false
	provider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 5 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		LeaseStore:   leaseStore,
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
	if result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected expired external lease to allow a new claim, got %+v", result)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider invoked once after external lease expiry, got %d", provider.calls)
	}
	if len(leaseStore.claims) == 0 {
		t.Fatalf("expected at least one external lease claim, got %+v", leaseStore.claims)
	}
}

func TestTokenCompletionCoordinatorReturnsEmailConflictWhenExternalLeaseClaimWinsBeforeRuntimeVisible(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC)
	store := &delayedCompareAndSwapTokenCompletionRuntimeStore{
		firstCompareStarted: make(chan struct{}),
		allowFirstCompare:   make(chan struct{}),
	}
	leaseStore := newExclusiveStubTokenCompletionLeaseStore()
	provider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 5 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		LeaseStore:   leaseStore,
		Now: func() time.Time {
			return now
		},
	})

	firstResultCh := make(chan TokenCompletionResult, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		firstResultCh <- result
		firstErrCh <- err
	}()

	select {
	case <-store.firstCompareStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first runtime claim compare-and-swap")
	}

	blocked, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email: "native@example.com",
		},
	})
	if err != nil {
		t.Fatalf("complete second token request: %v", err)
	}
	if blocked.State != TokenCompletionStateBlocked {
		t.Fatalf("expected second request blocked by external lease owner, got %+v", blocked)
	}
	if blocked.Error == nil || blocked.Error.Kind != TokenCompletionErrorKindEmailConflict || !blocked.Error.Retryable {
		t.Fatalf("expected retryable email conflict, got %+v", blocked.Error)
	}
	if blocked.Error.Message != "token completion already running for email" {
		t.Fatalf("expected direct email conflict message, got %+v", blocked.Error)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called before first runtime claim persists, got %d", provider.calls)
	}

	close(store.allowFirstCompare)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("complete first token request: %v", err)
	}
	first := <-firstResultCh
	if first.State != TokenCompletionStateCompleted {
		t.Fatalf("expected first request completed, got %+v", first)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once after first runtime claim persisted, got %d", provider.calls)
	}
}

func TestTokenCompletionCoordinatorReleasesExternalLeaseAfterCompletion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC)
	leaseStore := newStubTokenCompletionLeaseStore()
	provider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
	}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 5 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: &stubTokenCompletionRuntimeStore{},
		LeaseStore:   leaseStore,
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
	if result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if len(leaseStore.releases) != 1 {
		t.Fatalf("expected external lease release after completion, got %+v", leaseStore.releases)
	}
}

func TestTokenCompletionCoordinatorCleansUpInactiveExternalLeaseBeforePersistingCooldownBlock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC)
	cooldownUntil := now.Add(3 * time.Minute)
	store := &compareAndSwapTokenCompletionRuntimeStore{}
	store.setState(TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				LeaseToken:  "lease-stale",
				StartedAt:   now.Add(-2 * time.Minute),
				HeartbeatAt: now.Add(-30 * time.Second),
			},
		},
		CooldownUntil: &cooldownUntil,
	})
	leaseStore := newStubTokenCompletionLeaseStore()
	leaseStore.active["native@example.com|lease-stale"] = false
	provider := &stubTokenCompletionProvider{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 5 * time.Minute,
		}),
		Provider:     provider,
		RuntimeStore: store,
		LeaseStore:   leaseStore,
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
	if result.State != TokenCompletionStateBlocked {
		t.Fatalf("expected cooldown block result, got %+v", result)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not called while cooldown active, got %d", provider.calls)
	}

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 0 {
		t.Fatalf("expected inactive external lease cleanup to remove stale running attempt, got %+v", persisted.Attempts)
	}
	if persisted.CooldownUntil == nil || !persisted.CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("expected cooldown to remain persisted, got %+v", persisted.CooldownUntil)
	}
}

func TestTokenCompletionCoordinatorDoesNotRenewHeartbeatAfterLeaseChanges(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	provider := &countingBlockingTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken: "access-123",
		},
		started: make(chan struct{}, 1),
		release: release,
	}
	store := &compareAndSwapTokenCompletionRuntimeStore{}
	coordinator := NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
		Scheduler: NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{
			RunningTimeout: 60 * time.Millisecond,
		}),
		Provider:          provider,
		RuntimeStore:      store,
		HeartbeatInterval: 10 * time.Millisecond,
	})

	resultCh := make(chan TokenCompletionResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := coordinator.Complete(context.Background(), TokenCompletionCommand{
			Account: TokenCompletionAccount{
				Email: "native@example.com",
			},
		})
		resultCh <- result
		errCh <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider start")
	}

	replacementStartedAt := time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC)
	replacementHeartbeatAt := replacementStartedAt.Add(15 * time.Second)
	store.setState(TokenCompletionRuntimeState{
		Attempts: []TokenCompletionAttempt{
			{
				Email:       "native@example.com",
				State:       TokenCompletionStateRunning,
				StartedAt:   replacementStartedAt,
				HeartbeatAt: replacementHeartbeatAt,
			},
		},
	})

	time.Sleep(50 * time.Millisecond)

	persisted, err := store.Load(context.Background(), "native@example.com")
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(persisted.Attempts) != 1 {
		t.Fatalf("expected newer running lease preserved, got %+v", persisted.Attempts)
	}
	if persisted.Attempts[0].State != TokenCompletionStateRunning {
		t.Fatalf("expected running lease preserved, got %+v", persisted.Attempts[0])
	}
	if !persisted.Attempts[0].HeartbeatAt.Equal(replacementHeartbeatAt) {
		t.Fatalf("expected stale heartbeat not to renew replacement lease, got %+v", persisted.Attempts[0])
	}

	close(release)

	if err := <-errCh; err != nil {
		t.Fatalf("complete token: %v", err)
	}
	<-resultCh
}

func TestNewDefaultPrepareSignupFlowUsesDefaultTokenCompletionSchedulerPolicy(t *testing.T) {
	t.Parallel()

	flow := NewDefaultPrepareSignupFlow(DefaultPrepareSignupFlowOptions{})
	coordinator, ok := flow.tokenCompletionCoordinator.(*TokenCompletionCoordinator)
	if !ok || coordinator == nil {
		t.Fatalf("expected default token completion coordinator, got %T", flow.tokenCompletionCoordinator)
	}

	policy := coordinator.scheduler.policy
	expected := DefaultTokenCompletionSchedulerPolicy()
	if policy.MinSpacing != expected.MinSpacing {
		t.Fatalf("expected min spacing %s, got %s", expected.MinSpacing, policy.MinSpacing)
	}
	if policy.BaseBackoff != expected.BaseBackoff {
		t.Fatalf("expected base backoff %s, got %s", expected.BaseBackoff, policy.BaseBackoff)
	}
	if policy.MaxBackoff != expected.MaxBackoff {
		t.Fatalf("expected max backoff %s, got %s", expected.MaxBackoff, policy.MaxBackoff)
	}
	if policy.Cooldown != expected.Cooldown {
		t.Fatalf("expected cooldown %s, got %s", expected.Cooldown, policy.Cooldown)
	}
	if policy.RunningTimeout != expected.RunningTimeout {
		t.Fatalf("expected running timeout %s, got %s", expected.RunningTimeout, policy.RunningTimeout)
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

type blockingTokenCompletionProvider struct {
	result      TokenCompletionProviderResult
	calls       int
	lastRequest TokenCompletionRequest
	started     chan struct{}
	release     chan struct{}
}

func (s *blockingTokenCompletionProvider) CompleteToken(_ context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	s.calls++
	s.lastRequest = request
	if s.started != nil {
		close(s.started)
		s.started = nil
	}
	if s.release != nil {
		<-s.release
	}
	return s.result, nil
}

type stubTokenCompletionRuntimeStore struct {
	mu    sync.Mutex
	state TokenCompletionRuntimeState
}

func (s *stubTokenCompletionRuntimeStore) Load(_ context.Context, _ string) (TokenCompletionRuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return TokenCompletionRuntimeState{
		Attempts: append([]TokenCompletionAttempt(nil), s.state.Attempts...),
	}, nil
}

func (s *stubTokenCompletionRuntimeStore) Save(_ context.Context, _ string, state TokenCompletionRuntimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = TokenCompletionRuntimeState{
		Attempts: append([]TokenCompletionAttempt(nil), state.Attempts...),
	}
	if state.CooldownUntil != nil {
		cooldownUntil := state.CooldownUntil.UTC()
		s.state.CooldownUntil = &cooldownUntil
	} else {
		s.state.CooldownUntil = nil
	}
	return nil
}

type compareAndSwapTokenCompletionRuntimeStore struct {
	mu              sync.Mutex
	state           TokenCompletionRuntimeState
	loads           int
	twoLoads        chan struct{}
	allowLegacySave chan struct{}
}

func (s *compareAndSwapTokenCompletionRuntimeStore) Load(_ context.Context, _ string) (TokenCompletionRuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loads == 2 && s.twoLoads != nil {
		close(s.twoLoads)
		s.twoLoads = nil
	}
	return cloneTokenCompletionRuntimeState(s.state), nil
}

func (s *compareAndSwapTokenCompletionRuntimeStore) Save(_ context.Context, _ string, state TokenCompletionRuntimeState) error {
	if s.allowLegacySave != nil {
		<-s.allowLegacySave
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = cloneTokenCompletionRuntimeState(state)
	return nil
}

func (s *compareAndSwapTokenCompletionRuntimeStore) CompareAndSwap(_ context.Context, _ string, current TokenCompletionRuntimeState, next TokenCompletionRuntimeState) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !reflect.DeepEqual(s.state, current) {
		return false, nil
	}
	s.state = cloneTokenCompletionRuntimeState(next)
	return true, nil
}

func (s *compareAndSwapTokenCompletionRuntimeStore) setState(state TokenCompletionRuntimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = cloneTokenCompletionRuntimeState(state)
}

type delayedCompareAndSwapTokenCompletionRuntimeStore struct {
	mu                  sync.Mutex
	state               TokenCompletionRuntimeState
	compareCalls        int
	firstCompareStarted chan struct{}
	allowFirstCompare   chan struct{}
}

func (s *delayedCompareAndSwapTokenCompletionRuntimeStore) Load(_ context.Context, _ string) (TokenCompletionRuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTokenCompletionRuntimeState(s.state), nil
}

func (s *delayedCompareAndSwapTokenCompletionRuntimeStore) Save(_ context.Context, _ string, state TokenCompletionRuntimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = cloneTokenCompletionRuntimeState(state)
	return nil
}

func (s *delayedCompareAndSwapTokenCompletionRuntimeStore) CompareAndSwap(_ context.Context, _ string, current TokenCompletionRuntimeState, next TokenCompletionRuntimeState) (bool, error) {
	s.mu.Lock()
	if !reflect.DeepEqual(s.state, current) {
		s.mu.Unlock()
		return false, nil
	}

	s.compareCalls++
	shouldDelay := s.compareCalls == 1 && s.allowFirstCompare != nil
	started := s.firstCompareStarted
	wait := s.allowFirstCompare
	if shouldDelay {
		s.firstCompareStarted = nil
	}
	s.mu.Unlock()

	if shouldDelay {
		if started != nil {
			close(started)
		}
		<-wait
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !reflect.DeepEqual(s.state, current) {
		return false, nil
	}
	s.state = cloneTokenCompletionRuntimeState(next)
	return true, nil
}

type countingBlockingTokenCompletionProvider struct {
	mu          sync.Mutex
	result      TokenCompletionProviderResult
	calls       int
	lastRequest TokenCompletionRequest
	started     chan struct{}
	release     <-chan struct{}
}

func (s *countingBlockingTokenCompletionProvider) CompleteToken(_ context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	s.mu.Lock()
	s.calls++
	s.lastRequest = request
	started := s.started
	release := s.release
	s.mu.Unlock()

	if started != nil {
		started <- struct{}{}
	}
	if release != nil {
		<-release
	}
	return s.result, nil
}

func cloneTokenCompletionRuntimeState(state TokenCompletionRuntimeState) TokenCompletionRuntimeState {
	cloned := TokenCompletionRuntimeState{
		Attempts: append([]TokenCompletionAttempt(nil), state.Attempts...),
	}
	if state.CooldownUntil != nil {
		cooldownUntil := state.CooldownUntil.UTC()
		cloned.CooldownUntil = &cooldownUntil
	}
	return cloned
}

type stubTokenCompletionLeaseStore struct {
	mu       sync.Mutex
	active   map[string]bool
	claims   []string
	renews   []string
	releases []string
}

func newStubTokenCompletionLeaseStore() *stubTokenCompletionLeaseStore {
	return &stubTokenCompletionLeaseStore{
		active: make(map[string]bool),
	}
}

func (s *stubTokenCompletionLeaseStore) Claim(_ context.Context, email string, leaseToken string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := tokenCompletionLeaseStoreKey(email, leaseToken)
	s.claims = append(s.claims, key)
	s.active[key] = true
	return true, nil
}

func (s *stubTokenCompletionLeaseStore) Renew(_ context.Context, email string, leaseToken string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := tokenCompletionLeaseStoreKey(email, leaseToken)
	s.renews = append(s.renews, key)
	return s.active[key], nil
}

func (s *stubTokenCompletionLeaseStore) Release(_ context.Context, email string, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := tokenCompletionLeaseStoreKey(email, leaseToken)
	s.releases = append(s.releases, key)
	delete(s.active, key)
	return nil
}

func (s *stubTokenCompletionLeaseStore) IsActive(_ context.Context, email string, leaseToken string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.active[tokenCompletionLeaseStoreKey(email, leaseToken)], nil
}

func tokenCompletionLeaseStoreKey(email string, leaseToken string) string {
	return strings.TrimSpace(email) + "|" + strings.TrimSpace(leaseToken)
}

type exclusiveStubTokenCompletionLeaseStore struct {
	mu     sync.Mutex
	owners map[string]string
}

func newExclusiveStubTokenCompletionLeaseStore() *exclusiveStubTokenCompletionLeaseStore {
	return &exclusiveStubTokenCompletionLeaseStore{
		owners: make(map[string]string),
	}
}

func (s *exclusiveStubTokenCompletionLeaseStore) Claim(_ context.Context, email string, leaseToken string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedEmail := strings.TrimSpace(email)
	currentOwner := s.owners[normalizedEmail]
	if currentOwner != "" && currentOwner != strings.TrimSpace(leaseToken) {
		return false, nil
	}
	s.owners[normalizedEmail] = strings.TrimSpace(leaseToken)
	return true, nil
}

func (s *exclusiveStubTokenCompletionLeaseStore) Renew(_ context.Context, email string, leaseToken string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.owners[strings.TrimSpace(email)] == strings.TrimSpace(leaseToken), nil
}

func (s *exclusiveStubTokenCompletionLeaseStore) Release(_ context.Context, email string, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedEmail := strings.TrimSpace(email)
	if s.owners[normalizedEmail] == strings.TrimSpace(leaseToken) {
		delete(s.owners, normalizedEmail)
	}
	return nil
}

func (s *exclusiveStubTokenCompletionLeaseStore) IsActive(_ context.Context, email string, leaseToken string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.owners[strings.TrimSpace(email)] == strings.TrimSpace(leaseToken), nil
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
