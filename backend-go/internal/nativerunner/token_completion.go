package nativerunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
)

type TokenCompletionStrategy string

const (
	TokenCompletionStrategyPassword     TokenCompletionStrategy = "password"
	TokenCompletionStrategyPasswordless TokenCompletionStrategy = "passwordless"
)

type TokenCompletionState string

const (
	TokenCompletionStateRunning   TokenCompletionState = "running"
	TokenCompletionStateBlocked   TokenCompletionState = "blocked"
	TokenCompletionStateCompleted TokenCompletionState = "completed"
	TokenCompletionStateFailed    TokenCompletionState = "failed"
)

type TokenCompletionErrorKind string

const (
	TokenCompletionErrorKindEmailConflict           TokenCompletionErrorKind = "email_conflict"
	TokenCompletionErrorKindSpacingActive           TokenCompletionErrorKind = "spacing_active"
	TokenCompletionErrorKindBackoffActive           TokenCompletionErrorKind = "backoff_active"
	TokenCompletionErrorKindCooldownActive          TokenCompletionErrorKind = "cooldown_active"
	TokenCompletionErrorKindRateLimited             TokenCompletionErrorKind = "rate_limited"
	TokenCompletionErrorKindProviderUnavailable     TokenCompletionErrorKind = "provider_unavailable"
	TokenCompletionErrorKindInteractiveStepRequired TokenCompletionErrorKind = "interactive_step_required"
	TokenCompletionErrorKindMissingPassword         TokenCompletionErrorKind = "missing_password"
	TokenCompletionErrorKindUnsupported             TokenCompletionErrorKind = "unsupported"
	TokenCompletionErrorKindInternal                TokenCompletionErrorKind = "internal"
)

type TokenCompletionError struct {
	Kind      TokenCompletionErrorKind `json:"kind"`
	Message   string                   `json:"message,omitempty"`
	Retryable bool                     `json:"retryable,omitempty"`
}

func (e *TokenCompletionError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return string(e.Kind)
}

type TokenCompletionAccount struct {
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
}

type TokenCompletionAttempt struct {
	Email       string                `json:"email"`
	State       TokenCompletionState  `json:"state"`
	LeaseToken  string                `json:"lease_token,omitempty"`
	StartedAt   time.Time             `json:"started_at,omitempty"`
	HeartbeatAt time.Time             `json:"heartbeat_at,omitempty"`
	CompletedAt time.Time             `json:"completed_at,omitempty"`
	Error       *TokenCompletionError `json:"error,omitempty"`
}

type TokenCompletionSchedulerPolicy struct {
	MinSpacing     time.Duration
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	Cooldown       time.Duration
	RunningTimeout time.Duration
}

type TokenCompletionPlan struct {
	Allowed        bool                    `json:"allowed"`
	Strategy       TokenCompletionStrategy `json:"strategy"`
	NextEligibleAt *time.Time              `json:"next_eligible_at,omitempty"`
	BlockedReason  *TokenCompletionError   `json:"blocked_reason,omitempty"`
}

type TokenCompletionScheduler struct {
	policy TokenCompletionSchedulerPolicy
}

func NewTokenCompletionScheduler(policy TokenCompletionSchedulerPolicy) *TokenCompletionScheduler {
	return &TokenCompletionScheduler{policy: policy}
}

func (s *TokenCompletionScheduler) Plan(now time.Time, account TokenCompletionAccount, attempts []TokenCompletionAttempt) TokenCompletionPlan {
	strategy := TokenCompletionStrategyPassword
	if strings.TrimSpace(account.Password) == "" {
		strategy = TokenCompletionStrategyPasswordless
	}

	plan := TokenCompletionPlan{
		Allowed:  true,
		Strategy: strategy,
	}

	relevant := relevantTokenCompletionAttempts(account.Email, attempts)
	for _, attempt := range relevant {
		if tokenCompletionRunningAttemptActive(s.policy, now, attempt) {
			plan.Allowed = false
			plan.BlockedReason = &TokenCompletionError{
				Kind:      TokenCompletionErrorKindEmailConflict,
				Message:   "token completion already running for email",
				Retryable: true,
			}
			return plan
		}
	}

	latest := latestTokenCompletionAttempt(relevant)
	if latest == nil {
		return plan
	}

	if eligibleAt, ok := tokenCompletionSpacingEligibleAt(s.policy, *latest); ok && now.Before(eligibleAt) {
		plan.Allowed = false
		plan.NextEligibleAt = &eligibleAt
		plan.BlockedReason = &TokenCompletionError{
			Kind:      TokenCompletionErrorKindSpacingActive,
			Message:   "token completion spacing active",
			Retryable: true,
		}
		return plan
	}

	if eligibleAt, ok := tokenCompletionCooldownEligibleAt(s.policy, *latest); ok && now.Before(eligibleAt) {
		plan.Allowed = false
		plan.NextEligibleAt = &eligibleAt
		plan.BlockedReason = &TokenCompletionError{
			Kind:      TokenCompletionErrorKindCooldownActive,
			Message:   "token completion cooldown active",
			Retryable: true,
		}
		return plan
	}

	if eligibleAt, ok := tokenCompletionBackoffEligibleAt(s.policy, relevant); ok && now.Before(eligibleAt) {
		plan.Allowed = false
		plan.NextEligibleAt = &eligibleAt
		plan.BlockedReason = &TokenCompletionError{
			Kind:      TokenCompletionErrorKindBackoffActive,
			Message:   "token completion backoff active",
			Retryable: true,
		}
		return plan
	}

	return plan
}

type TokenCompletionCommand struct {
	Account       TokenCompletionAccount   `json:"account"`
	Attempts      []TokenCompletionAttempt `json:"attempts,omitempty"`
	CooldownUntil *time.Time               `json:"cooldown_until,omitempty"`
	ContinueURL   string                   `json:"continue_url,omitempty"`
	CallbackURL   string                   `json:"callback_url,omitempty"`
	PageType      string                   `json:"page_type,omitempty"`
	AccountID     string                   `json:"account_id,omitempty"`
	WorkspaceID   string                   `json:"workspace_id,omitempty"`
	Inbox         mail.Inbox               `json:"inbox,omitempty"`
	MailProvider  mail.Provider            `json:"-"`
	AuthClient    *auth.Client             `json:"-"`
}

type TokenCompletionRuntimeState struct {
	Attempts      []TokenCompletionAttempt `json:"attempts,omitempty"`
	CooldownUntil *time.Time               `json:"cooldown_until,omitempty"`
}

type TokenCompletionRequest struct {
	Email        string                  `json:"email"`
	Password     string                  `json:"password,omitempty"`
	Strategy     TokenCompletionStrategy `json:"strategy"`
	ContinueURL  string                  `json:"continue_url,omitempty"`
	CallbackURL  string                  `json:"callback_url,omitempty"`
	PageType     string                  `json:"page_type,omitempty"`
	AccountID    string                  `json:"account_id,omitempty"`
	WorkspaceID  string                  `json:"workspace_id,omitempty"`
	Inbox        mail.Inbox              `json:"inbox,omitempty"`
	MailProvider mail.Provider           `json:"-"`
	AuthClient   *auth.Client            `json:"-"`
}

type TokenCompletionProviderResult struct {
	AccessToken        string `json:"access_token,omitempty"`
	RefreshToken       string `json:"refresh_token,omitempty"`
	SessionToken       string `json:"session_token,omitempty"`
	AccountID          string `json:"account_id,omitempty"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	Source             string `json:"source,omitempty"`
	AuthProvider       string `json:"auth_provider,omitempty"`
	AccessTokenSource  string `json:"access_token_source,omitempty"`
	SessionTokenSource string `json:"session_token_source,omitempty"`
	RefreshTokenSource string `json:"refresh_token_source,omitempty"`
}

type TokenCompletionProvider interface {
	CompleteToken(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error)
}

type TokenCompletionProviderFunc func(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error)

func (f TokenCompletionProviderFunc) CompleteToken(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	return f(ctx, request)
}

type TokenCompletionResult struct {
	State          TokenCompletionState          `json:"state"`
	Email          string                        `json:"email"`
	Strategy       TokenCompletionStrategy       `json:"strategy"`
	LeaseToken     string                        `json:"lease_token,omitempty"`
	NextEligibleAt *time.Time                    `json:"next_eligible_at,omitempty"`
	Error          *TokenCompletionError         `json:"error,omitempty"`
	Provider       TokenCompletionProviderResult `json:"provider,omitempty"`
}

var tokenCompletionLeaseCounter uint64

var errTokenCompletionRuntimeSlotBusy = errors.New("token completion runtime slot busy")

type TokenCompletionCoordinatorOptions struct {
	Scheduler         *TokenCompletionScheduler
	Provider          TokenCompletionProvider
	RuntimeStore      TokenCompletionRuntimeStore
	LeaseStore        TokenCompletionLeaseStore
	Now               func() time.Time
	HeartbeatInterval time.Duration
}

type TokenCompletionCoordinator struct {
	scheduler         *TokenCompletionScheduler
	provider          TokenCompletionProvider
	runtimeStore      TokenCompletionRuntimeStore
	leaseStore        TokenCompletionLeaseStore
	now               func() time.Time
	heartbeatInterval time.Duration
}

type TokenCompletionLeaseStore interface {
	Claim(ctx context.Context, email string, leaseToken string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, email string, leaseToken string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, email string, leaseToken string) error
	IsActive(ctx context.Context, email string, leaseToken string) (bool, error)
}

type TokenCompletionRuntimeStore interface {
	Load(ctx context.Context, email string) (TokenCompletionRuntimeState, error)
	Save(ctx context.Context, email string, state TokenCompletionRuntimeState) error
}

type TokenCompletionRuntimeCompareAndSwapStore interface {
	TokenCompletionRuntimeStore
	CompareAndSwap(ctx context.Context, email string, current TokenCompletionRuntimeState, next TokenCompletionRuntimeState) (bool, error)
}

func NewTokenCompletionCoordinator(options TokenCompletionCoordinatorOptions) *TokenCompletionCoordinator {
	scheduler := options.Scheduler
	if scheduler == nil {
		scheduler = NewTokenCompletionScheduler(TokenCompletionSchedulerPolicy{})
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &TokenCompletionCoordinator{
		scheduler:         scheduler,
		provider:          options.Provider,
		runtimeStore:      options.RuntimeStore,
		leaseStore:        options.LeaseStore,
		now:               now,
		heartbeatInterval: resolveTokenCompletionHeartbeatInterval(options.HeartbeatInterval, scheduler.policy.RunningTimeout),
	}
}

func (c *TokenCompletionCoordinator) Complete(ctx context.Context, command TokenCompletionCommand) (TokenCompletionResult, error) {
	if c == nil {
		return TokenCompletionResult{}, errors.New("token completion coordinator is required")
	}
	if c.provider == nil {
		return TokenCompletionResult{}, errors.New("token completion provider is required")
	}

	now := c.now()
	email := strings.TrimSpace(command.Account.Email)
	for attempt := 0; attempt < 5; attempt++ {
		runtimeState, err := c.loadRuntimeState(ctx, command)
		if err != nil {
			return TokenCompletionResult{}, err
		}
		effectiveRuntimeState, err := c.reconcileExternalLeaseState(ctx, email, runtimeState)
		if err != nil {
			return TokenCompletionResult{}, err
		}
		if !tokenCompletionRuntimeStatesEqual(runtimeState, effectiveRuntimeState) {
			persisted, retry, err := c.persistReconciledRuntimeState(ctx, email, runtimeState, effectiveRuntimeState)
			if err != nil {
				return TokenCompletionResult{}, err
			}
			if retry {
				continue
			}
			runtimeState = persisted
			effectiveRuntimeState = persisted
		}

		plan := c.scheduler.Plan(now, command.Account, effectiveRuntimeState.Attempts)
		result := TokenCompletionResult{
			Email:          email,
			Strategy:       plan.Strategy,
			NextEligibleAt: plan.NextEligibleAt,
		}
		if runtimeState.CooldownUntil != nil {
			cooldownUntil := runtimeState.CooldownUntil.UTC()
			if now.Before(cooldownUntil) {
				result.State = TokenCompletionStateBlocked
				result.NextEligibleAt = &cooldownUntil
				result.Error = &TokenCompletionError{
					Kind:      TokenCompletionErrorKindCooldownActive,
					Message:   "token completion cooldown active",
					Retryable: true,
				}
				if err := c.persistRuntimeState(ctx, now, result.Email, runtimeState, result); err != nil {
					return TokenCompletionResult{}, err
				}
				return result, nil
			}
		}
		if !plan.Allowed {
			result.State = TokenCompletionStateBlocked
			result.Error = plan.BlockedReason
			if err := c.persistRuntimeState(ctx, now, result.Email, runtimeState, result); err != nil {
				return TokenCompletionResult{}, err
			}
			return result, nil
		}

		leaseToken, claimed, err := c.claimRunningAttempt(ctx, now, result.Email, runtimeState, result.Strategy)
		if err != nil {
			if errors.Is(err, errTokenCompletionRuntimeSlotBusy) {
				result.State = TokenCompletionStateBlocked
				result.Error = &TokenCompletionError{
					Kind:      TokenCompletionErrorKindEmailConflict,
					Message:   "token completion already running for email",
					Retryable: true,
				}
				return result, nil
			}
			return TokenCompletionResult{}, err
		}
		if !claimed {
			continue
		}

		result.LeaseToken = leaseToken
		stopHeartbeat := c.startRunningAttemptHeartbeat(ctx, email, result.Strategy, leaseToken)
		providerResult, err := c.provider.CompleteToken(ctx, TokenCompletionRequest{
			Email:        email,
			Password:     command.Account.Password,
			Strategy:     plan.Strategy,
			ContinueURL:  strings.TrimSpace(command.ContinueURL),
			CallbackURL:  strings.TrimSpace(command.CallbackURL),
			PageType:     strings.TrimSpace(command.PageType),
			AccountID:    strings.TrimSpace(command.AccountID),
			WorkspaceID:  strings.TrimSpace(command.WorkspaceID),
			Inbox:        command.Inbox,
			MailProvider: command.MailProvider,
			AuthClient:   command.AuthClient,
		})
		stopHeartbeat()
		if err != nil {
			result.State = TokenCompletionStateFailed
			result.Error = classifyTokenCompletionError(err)
			if err := c.persistRuntimeState(ctx, now, result.Email, runtimeState, result); err != nil {
				return TokenCompletionResult{}, err
			}
			_ = c.releaseRunningAttemptLease(ctx, email, leaseToken)
			return result, nil
		}

		result.State = TokenCompletionStateCompleted
		result.Provider = providerResult
		if err := c.persistRuntimeState(ctx, now, result.Email, runtimeState, result); err != nil {
			return TokenCompletionResult{}, err
		}
		_ = c.releaseRunningAttemptLease(ctx, email, leaseToken)
		return result, nil
	}

	return TokenCompletionResult{
		State:    TokenCompletionStateBlocked,
		Email:    email,
		Strategy: TokenCompletionStrategyPasswordless,
		Error: &TokenCompletionError{
			Kind:      TokenCompletionErrorKindEmailConflict,
			Message:   "token completion state changed while claiming runtime slot",
			Retryable: true,
		},
	}, nil
}

func (c *TokenCompletionCoordinator) persistReconciledRuntimeState(
	ctx context.Context,
	email string,
	current TokenCompletionRuntimeState,
	next TokenCompletionRuntimeState,
) (TokenCompletionRuntimeState, bool, error) {
	if c == nil || c.runtimeStore == nil {
		return next, false, nil
	}
	if casStore := tokenCompletionRuntimeCompareAndSwapStore(c.runtimeStore); casStore != nil {
		saved, err := casStore.CompareAndSwap(ctx, email, current, next)
		if err != nil {
			return TokenCompletionRuntimeState{}, false, fmt.Errorf("persist token completion runtime cleanup: %w", err)
		}
		if !saved {
			return TokenCompletionRuntimeState{}, true, nil
		}
		return next, false, nil
	}
	if err := c.runtimeStore.Save(ctx, email, next); err != nil {
		return TokenCompletionRuntimeState{}, false, fmt.Errorf("persist token completion runtime cleanup: %w", err)
	}
	return next, false, nil
}

func (c *TokenCompletionCoordinator) persistRuntimeState(
	ctx context.Context,
	now time.Time,
	email string,
	runtimeState TokenCompletionRuntimeState,
	result TokenCompletionResult,
) error {
	if c == nil || c.runtimeStore == nil {
		return nil
	}

	if casStore := tokenCompletionRuntimeCompareAndSwapStore(c.runtimeStore); casStore != nil {
		for attempt := 0; attempt < 5; attempt++ {
			storedState, err := casStore.Load(ctx, email)
			if err != nil {
				return fmt.Errorf("load token completion runtime state: %w", err)
			}
			if !c.canPersistLeaseResult(ctx, now, email, storedState, result) {
				return nil
			}
			mergedState := mergeTokenCompletionRuntimeState(email, runtimeState, storedState)
			nextState := BuildTokenCompletionRuntimeState(now, email, mergedState, result)
			saved, err := casStore.CompareAndSwap(ctx, email, storedState, nextState)
			if err != nil {
				return fmt.Errorf("persist token completion runtime state: %w", err)
			}
			if saved {
				return nil
			}
		}
		return fmt.Errorf("persist token completion runtime state: compare-and-swap conflict")
	}

	if err := c.runtimeStore.Save(ctx, email, BuildTokenCompletionRuntimeState(now, email, runtimeState, result)); err != nil {
		return fmt.Errorf("persist token completion runtime state: %w", err)
	}
	return nil
}

func (c *TokenCompletionCoordinator) loadRuntimeState(ctx context.Context, command TokenCompletionCommand) (TokenCompletionRuntimeState, error) {
	runtimeState := tokenCompletionCommandRuntimeState(command)
	if c.runtimeStore == nil {
		return runtimeState, nil
	}

	storedState, err := c.runtimeStore.Load(ctx, command.Account.Email)
	if err != nil {
		return TokenCompletionRuntimeState{}, fmt.Errorf("load token completion runtime state: %w", err)
	}
	return mergeTokenCompletionRuntimeState(command.Account.Email, runtimeState, storedState), nil
}

func (c *TokenCompletionCoordinator) claimRunningAttempt(
	ctx context.Context,
	now time.Time,
	email string,
	runtimeState TokenCompletionRuntimeState,
	strategy TokenCompletionStrategy,
) (string, bool, error) {
	if c == nil || c.runtimeStore == nil {
		return "", true, nil
	}

	leaseToken := newTokenCompletionLeaseToken(now)
	leaseClaimedExternally := false
	if c.leaseStore != nil {
		claimed, err := c.leaseStore.Claim(ctx, email, leaseToken, c.externalLeaseTTL())
		if err != nil {
			return "", false, fmt.Errorf("claim token completion external lease: %w", err)
		}
		if !claimed {
			return "", false, errTokenCompletionRuntimeSlotBusy
		}
		leaseClaimedExternally = true
	}
	runningState := BuildTokenCompletionRuntimeState(now, email, runtimeState, TokenCompletionResult{
		State:      TokenCompletionStateRunning,
		Email:      email,
		Strategy:   strategy,
		LeaseToken: leaseToken,
	})
	if casStore := tokenCompletionRuntimeCompareAndSwapStore(c.runtimeStore); casStore != nil {
		claimed, err := casStore.CompareAndSwap(ctx, email, runtimeState, runningState)
		if err != nil {
			if leaseClaimedExternally {
				_ = c.releaseRunningAttemptLease(ctx, email, leaseToken)
			}
			return "", false, fmt.Errorf("persist token completion runtime state: %w", err)
		}
		if !claimed && leaseClaimedExternally {
			_ = c.releaseRunningAttemptLease(ctx, email, leaseToken)
		}
		return leaseToken, claimed, nil
	}

	if err := c.runtimeStore.Save(ctx, email, runningState); err != nil {
		if leaseClaimedExternally {
			_ = c.releaseRunningAttemptLease(ctx, email, leaseToken)
		}
		return "", false, fmt.Errorf("persist token completion runtime state: %w", err)
	}
	return leaseToken, true, nil
}

func (c *TokenCompletionCoordinator) startRunningAttemptHeartbeat(ctx context.Context, email string, strategy TokenCompletionStrategy, leaseToken string) func() {
	if c == nil || c.heartbeatInterval <= 0 || (c.runtimeStore == nil && c.leaseStore == nil) {
		return func() {}
	}

	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := time.NewTicker(c.heartbeatInterval)
	go func() {
		defer close(done)
		defer ticker.Stop()

		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if c.leaseStore != nil {
					renewed, err := c.leaseStore.Renew(heartbeatCtx, email, leaseToken, c.externalLeaseTTL())
					if err != nil || !renewed {
						return
					}
				}
				if err := c.renewRunningAttempt(heartbeatCtx, c.now(), email, strategy, leaseToken); err != nil {
					return
				}
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func (c *TokenCompletionCoordinator) renewRunningAttempt(
	ctx context.Context,
	now time.Time,
	email string,
	strategy TokenCompletionStrategy,
	leaseToken string,
) error {
	if c == nil || c.runtimeStore == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}

	if casStore := tokenCompletionRuntimeCompareAndSwapStore(c.runtimeStore); casStore != nil {
		for attempt := 0; attempt < 5; attempt++ {
			currentState, err := casStore.Load(ctx, email)
			if err != nil {
				return fmt.Errorf("load token completion runtime state: %w", err)
			}
			if !tokenCompletionHasRunningAttemptLease(email, currentState.Attempts, leaseToken) {
				return nil
			}
			nextState := BuildTokenCompletionRuntimeState(now, email, currentState, TokenCompletionResult{
				State:      TokenCompletionStateRunning,
				Email:      email,
				Strategy:   strategy,
				LeaseToken: leaseToken,
			})
			if err := ctx.Err(); err != nil {
				return nil
			}
			renewed, err := casStore.CompareAndSwap(ctx, email, currentState, nextState)
			if err != nil {
				return fmt.Errorf("persist token completion runtime state: %w", err)
			}
			if renewed {
				return nil
			}
		}
		return fmt.Errorf("persist token completion runtime state: compare-and-swap conflict")
	}

	currentState, err := c.runtimeStore.Load(ctx, email)
	if err != nil {
		return fmt.Errorf("load token completion runtime state: %w", err)
	}
	if !tokenCompletionHasRunningAttemptLease(email, currentState.Attempts, leaseToken) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	if err := c.runtimeStore.Save(ctx, email, BuildTokenCompletionRuntimeState(now, email, currentState, TokenCompletionResult{
		State:      TokenCompletionStateRunning,
		Email:      email,
		Strategy:   strategy,
		LeaseToken: leaseToken,
	})); err != nil {
		return fmt.Errorf("persist token completion runtime state: %w", err)
	}
	return nil
}

func tokenCompletionRuntimeCompareAndSwapStore(store TokenCompletionRuntimeStore) TokenCompletionRuntimeCompareAndSwapStore {
	if store == nil {
		return nil
	}
	casStore, ok := store.(TokenCompletionRuntimeCompareAndSwapStore)
	if !ok {
		return nil
	}
	return casStore
}

func tokenCompletionRuntimeStatesEqual(left TokenCompletionRuntimeState, right TokenCompletionRuntimeState) bool {
	leftJSON, err := json.Marshal(TokenCompletionRuntimeExtraData(left))
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(TokenCompletionRuntimeExtraData(right))
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func (c *TokenCompletionCoordinator) canPersistLeaseResult(
	ctx context.Context,
	now time.Time,
	email string,
	storedState TokenCompletionRuntimeState,
	result TokenCompletionResult,
) bool {
	if !tokenCompletionCanPersistLeaseResult(c.scheduler.policy, now, email, storedState, result) {
		return false
	}
	if c == nil || c.leaseStore == nil || strings.TrimSpace(result.LeaseToken) == "" {
		return true
	}
	switch result.State {
	case TokenCompletionStateRunning, TokenCompletionStateCompleted, TokenCompletionStateFailed:
		active, err := c.leaseStore.IsActive(ctx, email, result.LeaseToken)
		return err == nil && active
	default:
		return true
	}
}

func (c *TokenCompletionCoordinator) reconcileExternalLeaseState(
	ctx context.Context,
	email string,
	runtimeState TokenCompletionRuntimeState,
) (TokenCompletionRuntimeState, error) {
	if c == nil || c.leaseStore == nil || len(runtimeState.Attempts) == 0 {
		return runtimeState, nil
	}

	filtered := runtimeState
	filtered.Attempts = make([]TokenCompletionAttempt, 0, len(runtimeState.Attempts))
	for _, attempt := range runtimeState.Attempts {
		if attempt.State != TokenCompletionStateRunning || strings.TrimSpace(attempt.LeaseToken) == "" {
			filtered.Attempts = append(filtered.Attempts, attempt)
			continue
		}
		active, err := c.leaseStore.IsActive(ctx, email, attempt.LeaseToken)
		if err != nil {
			return TokenCompletionRuntimeState{}, fmt.Errorf("check token completion external lease: %w", err)
		}
		if active {
			filtered.Attempts = append(filtered.Attempts, attempt)
		}
	}
	return filtered, nil
}

func (c *TokenCompletionCoordinator) releaseRunningAttemptLease(ctx context.Context, email string, leaseToken string) error {
	if c == nil || c.leaseStore == nil || strings.TrimSpace(leaseToken) == "" {
		return nil
	}
	if err := c.leaseStore.Release(ctx, email, leaseToken); err != nil {
		return fmt.Errorf("release token completion external lease: %w", err)
	}
	return nil
}

func (c *TokenCompletionCoordinator) externalLeaseTTL() time.Duration {
	if c == nil {
		return 0
	}
	return resolveTokenCompletionExternalLeaseTTL(c.heartbeatInterval, c.scheduler.policy.RunningTimeout)
}

func classifyTokenCompletionError(err error) *TokenCompletionError {
	if err == nil {
		return nil
	}

	var typed *TokenCompletionError
	if errors.As(err, &typed) && typed != nil {
		cloned := *typed
		return &cloned
	}

	return &TokenCompletionError{
		Kind:    TokenCompletionErrorKindInternal,
		Message: err.Error(),
	}
}

func ParseTokenCompletionRuntimeState(extraData map[string]any, email string) (TokenCompletionRuntimeState, error) {
	var state TokenCompletionRuntimeState
	if len(extraData) == 0 {
		return state, nil
	}

	if rawAttempts, ok := extraData["token_completion_attempts"]; ok && rawAttempts != nil {
		payload, err := json.Marshal(rawAttempts)
		if err != nil {
			return TokenCompletionRuntimeState{}, fmt.Errorf("marshal token_completion_attempts: %w", err)
		}
		if err := json.Unmarshal(payload, &state.Attempts); err != nil {
			return TokenCompletionRuntimeState{}, fmt.Errorf("unmarshal token_completion_attempts: %w", err)
		}
	}
	state.Attempts = relevantTokenCompletionAttempts(email, state.Attempts)
	sortTokenCompletionAttempts(state.Attempts)

	cooldownUntil, err := parseTokenCompletionCooldownValue(extraData["refresh_token_cooldown_until"])
	if err != nil {
		return TokenCompletionRuntimeState{}, err
	}
	state.CooldownUntil = cooldownUntil
	return state, nil
}

func BuildTokenCompletionRuntimeState(now time.Time, email string, existing TokenCompletionRuntimeState, result TokenCompletionResult) TokenCompletionRuntimeState {
	state := TokenCompletionRuntimeState{
		Attempts: relevantTokenCompletionAttempts(email, append([]TokenCompletionAttempt(nil), existing.Attempts...)),
	}
	if existing.CooldownUntil != nil {
		cooldownUntil := existing.CooldownUntil.UTC()
		state.CooldownUntil = &cooldownUntil
	}

	switch result.State {
	case TokenCompletionStateRunning:
		normalizedNow := now.UTC()
		startedAt := normalizedNow
		leaseToken := strings.TrimSpace(result.LeaseToken)
		if runningAttempt := latestRunningTokenCompletionAttempt(state.Attempts); runningAttempt != nil && !runningAttempt.StartedAt.IsZero() {
			startedAt = runningAttempt.StartedAt.UTC()
			if leaseToken == "" {
				leaseToken = strings.TrimSpace(runningAttempt.LeaseToken)
			}
		}
		state.Attempts = removeTokenCompletionRunningAttempts(state.Attempts)
		state.Attempts = append(state.Attempts, TokenCompletionAttempt{
			Email:       firstNonEmptyTrimmed(result.Email, email),
			State:       TokenCompletionStateRunning,
			LeaseToken:  leaseToken,
			StartedAt:   startedAt,
			HeartbeatAt: normalizedNow,
		})
		state.CooldownUntil = nil
	case TokenCompletionStateCompleted, TokenCompletionStateFailed:
		normalizedNow := now.UTC()
		state.Attempts = removeTokenCompletionRunningAttempts(state.Attempts)
		state.Attempts = append(state.Attempts, TokenCompletionAttempt{
			Email:       firstNonEmptyTrimmed(result.Email, email),
			State:       result.State,
			LeaseToken:  strings.TrimSpace(result.LeaseToken),
			StartedAt:   normalizedNow,
			CompletedAt: normalizedNow,
			Error:       classifyTokenCompletionError(result.Error),
		})
		if result.NextEligibleAt != nil {
			cooldownUntil := result.NextEligibleAt.UTC()
			state.CooldownUntil = &cooldownUntil
		} else {
			state.CooldownUntil = nil
		}
	case TokenCompletionStateBlocked:
		if result.Error != nil && result.Error.Kind == TokenCompletionErrorKindCooldownActive && result.NextEligibleAt != nil {
			cooldownUntil := result.NextEligibleAt.UTC()
			state.CooldownUntil = &cooldownUntil
		}
	default:
		state.CooldownUntil = nil
	}

	sortTokenCompletionAttempts(state.Attempts)
	if len(state.Attempts) > 10 {
		state.Attempts = append([]TokenCompletionAttempt(nil), state.Attempts[len(state.Attempts)-10:]...)
	}
	return state
}

func serializeTokenCompletionAttempts(attempts []TokenCompletionAttempt) []map[string]any {
	if len(attempts) == 0 {
		return nil
	}

	serialized := make([]map[string]any, 0, len(attempts))
	for _, attempt := range attempts {
		item := map[string]any{
			"email": strings.TrimSpace(attempt.Email),
			"state": string(attempt.State),
		}
		if strings.TrimSpace(attempt.LeaseToken) != "" {
			item["lease_token"] = strings.TrimSpace(attempt.LeaseToken)
		}
		if !attempt.StartedAt.IsZero() {
			item["started_at"] = attempt.StartedAt.UTC().Format(time.RFC3339)
		}
		if !attempt.HeartbeatAt.IsZero() {
			item["heartbeat_at"] = attempt.HeartbeatAt.UTC().Format(time.RFC3339)
		}
		if !attempt.CompletedAt.IsZero() {
			item["completed_at"] = attempt.CompletedAt.UTC().Format(time.RFC3339)
		}
		if attempt.Error != nil {
			item["error"] = map[string]any{
				"kind":      string(attempt.Error.Kind),
				"message":   attempt.Error.Message,
				"retryable": attempt.Error.Retryable,
			}
		}
		serialized = append(serialized, item)
	}
	return serialized
}

func formatTokenCompletionCooldown(cooldownUntil *time.Time) string {
	if cooldownUntil == nil {
		return ""
	}
	return cooldownUntil.UTC().Format(time.RFC3339)
}

func TokenCompletionRuntimeExtraData(state TokenCompletionRuntimeState) map[string]any {
	return map[string]any{
		"token_completion_attempts":    serializeTokenCompletionAttempts(state.Attempts),
		"refresh_token_cooldown_until": formatTokenCompletionCooldown(state.CooldownUntil),
	}
}

func parseTokenCompletionCooldownValue(value any) (*time.Time, error) {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" || text == "<nil>" {
		return nil, nil
	}
	if ts, err := time.Parse(time.RFC3339, text); err == nil {
		parsed := ts.UTC()
		return &parsed, nil
	}
	if ts, err := time.Parse("2006-01-02T15:04:05", text); err == nil {
		parsed := ts.UTC()
		return &parsed, nil
	}
	if seconds, err := strconv.ParseFloat(text, 64); err == nil {
		parsed := time.Unix(int64(seconds), 0).UTC()
		return &parsed, nil
	}
	return nil, fmt.Errorf("parse refresh_token_cooldown_until: %q", text)
}

func sortTokenCompletionAttempts(attempts []TokenCompletionAttempt) {
	sort.Slice(attempts, func(left int, right int) bool {
		return tokenCompletionAttemptTimestamp(attempts[left]).Before(tokenCompletionAttemptTimestamp(attempts[right]))
	})
}

func relevantTokenCompletionAttempts(email string, attempts []TokenCompletionAttempt) []TokenCompletionAttempt {
	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" || len(attempts) == 0 {
		return nil
	}

	relevant := make([]TokenCompletionAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if strings.EqualFold(strings.TrimSpace(attempt.Email), normalizedEmail) {
			relevant = append(relevant, attempt)
		}
	}
	return relevant
}

func removeTokenCompletionRunningAttempts(attempts []TokenCompletionAttempt) []TokenCompletionAttempt {
	if len(attempts) == 0 {
		return nil
	}

	filtered := make([]TokenCompletionAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.State == TokenCompletionStateRunning {
			continue
		}
		filtered = append(filtered, attempt)
	}
	return filtered
}

func latestTokenCompletionAttempt(attempts []TokenCompletionAttempt) *TokenCompletionAttempt {
	if len(attempts) == 0 {
		return nil
	}

	latestIndex := 0
	latestAt := tokenCompletionAttemptTimestamp(attempts[0])
	for index := 1; index < len(attempts); index++ {
		candidateAt := tokenCompletionAttemptTimestamp(attempts[index])
		if candidateAt.After(latestAt) {
			latestIndex = index
			latestAt = candidateAt
		}
	}

	return &attempts[latestIndex]
}

func latestRunningTokenCompletionAttempt(attempts []TokenCompletionAttempt) *TokenCompletionAttempt {
	var latest *TokenCompletionAttempt
	var latestAt time.Time
	for index := range attempts {
		attempt := &attempts[index]
		if attempt.State != TokenCompletionStateRunning {
			continue
		}
		candidateAt := tokenCompletionRunningLeaseTimestamp(*attempt)
		if latest == nil || candidateAt.After(latestAt) {
			latest = attempt
			latestAt = candidateAt
		}
	}
	return latest
}

func tokenCompletionAttemptTimestamp(attempt TokenCompletionAttempt) time.Time {
	if !attempt.CompletedAt.IsZero() && attempt.CompletedAt.After(attempt.StartedAt) {
		return attempt.CompletedAt
	}
	if !attempt.StartedAt.IsZero() {
		return attempt.StartedAt
	}
	return attempt.CompletedAt
}

func tokenCompletionRunningLeaseTimestamp(attempt TokenCompletionAttempt) time.Time {
	if !attempt.HeartbeatAt.IsZero() {
		return attempt.HeartbeatAt
	}
	return tokenCompletionAttemptTimestamp(attempt)
}

func tokenCompletionSpacingEligibleAt(policy TokenCompletionSchedulerPolicy, attempt TokenCompletionAttempt) (time.Time, bool) {
	if policy.MinSpacing <= 0 {
		return time.Time{}, false
	}
	base := attempt.StartedAt
	if base.IsZero() {
		base = tokenCompletionAttemptTimestamp(attempt)
	}
	if base.IsZero() {
		return time.Time{}, false
	}
	return base.Add(policy.MinSpacing), true
}

func tokenCompletionCommandRuntimeState(command TokenCompletionCommand) TokenCompletionRuntimeState {
	state := TokenCompletionRuntimeState{
		Attempts: append([]TokenCompletionAttempt(nil), command.Attempts...),
	}
	if command.CooldownUntil != nil {
		cooldownUntil := command.CooldownUntil.UTC()
		state.CooldownUntil = &cooldownUntil
	}
	return state
}

func mergeTokenCompletionRuntimeState(email string, current TokenCompletionRuntimeState, stored TokenCompletionRuntimeState) TokenCompletionRuntimeState {
	merged := TokenCompletionRuntimeState{
		Attempts: dedupeTokenCompletionAttempts(append(
			relevantTokenCompletionAttempts(email, stored.Attempts),
			relevantTokenCompletionAttempts(email, current.Attempts)...,
		)),
	}
	sortTokenCompletionAttempts(merged.Attempts)
	merged.CooldownUntil = chooseLaterTokenCompletionCooldown(current.CooldownUntil, stored.CooldownUntil)
	return merged
}

func dedupeTokenCompletionAttempts(attempts []TokenCompletionAttempt) []TokenCompletionAttempt {
	if len(attempts) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(attempts))
	deduped := make([]TokenCompletionAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(attempt.Email)),
			string(attempt.State),
			strings.TrimSpace(attempt.LeaseToken),
			attempt.StartedAt.UTC().Format(time.RFC3339Nano),
			attempt.HeartbeatAt.UTC().Format(time.RFC3339Nano),
			attempt.CompletedAt.UTC().Format(time.RFC3339Nano),
			tokenCompletionErrorKey(attempt.Error),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, attempt)
	}
	return deduped
}

func tokenCompletionErrorKey(err *TokenCompletionError) string {
	if err == nil {
		return ""
	}
	return strings.Join([]string{
		string(err.Kind),
		err.Message,
		strconv.FormatBool(err.Retryable),
	}, "|")
}

func chooseLaterTokenCompletionCooldown(current *time.Time, stored *time.Time) *time.Time {
	switch {
	case current == nil && stored == nil:
		return nil
	case current == nil:
		cooldownUntil := stored.UTC()
		return &cooldownUntil
	case stored == nil:
		cooldownUntil := current.UTC()
		return &cooldownUntil
	case stored.After(*current):
		cooldownUntil := stored.UTC()
		return &cooldownUntil
	default:
		cooldownUntil := current.UTC()
		return &cooldownUntil
	}
}

func tokenCompletionCooldownEligibleAt(policy TokenCompletionSchedulerPolicy, attempt TokenCompletionAttempt) (time.Time, bool) {
	if policy.Cooldown <= 0 || attempt.State != TokenCompletionStateFailed || attempt.Error == nil {
		return time.Time{}, false
	}
	if attempt.Error.Kind != TokenCompletionErrorKindRateLimited {
		return time.Time{}, false
	}
	base := tokenCompletionAttemptTimestamp(attempt)
	if base.IsZero() {
		return time.Time{}, false
	}
	return base.Add(policy.Cooldown), true
}

func tokenCompletionBackoffEligibleAt(policy TokenCompletionSchedulerPolicy, attempts []TokenCompletionAttempt) (time.Time, bool) {
	if policy.BaseBackoff <= 0 || len(attempts) == 0 {
		return time.Time{}, false
	}

	latest := latestTokenCompletionAttempt(attempts)
	if latest == nil || latest.State != TokenCompletionStateFailed || latest.Error == nil || !latest.Error.Retryable {
		return time.Time{}, false
	}
	if latest.Error.Kind == TokenCompletionErrorKindRateLimited {
		return time.Time{}, false
	}

	consecutiveFailures := 0
	for index := len(attempts) - 1; index >= 0; index-- {
		attempt := attempts[index]
		if attempt.State != TokenCompletionStateFailed || attempt.Error == nil || !attempt.Error.Retryable {
			break
		}
		consecutiveFailures++
	}
	if consecutiveFailures == 0 {
		return time.Time{}, false
	}

	delay := policy.BaseBackoff
	for retry := 1; retry < consecutiveFailures; retry++ {
		delay *= 2
		if policy.MaxBackoff > 0 && delay > policy.MaxBackoff {
			delay = policy.MaxBackoff
			break
		}
	}

	base := tokenCompletionAttemptTimestamp(*latest)
	if base.IsZero() {
		return time.Time{}, false
	}
	return base.Add(delay), true
}

func tokenCompletionRunningAttemptActive(policy TokenCompletionSchedulerPolicy, now time.Time, attempt TokenCompletionAttempt) bool {
	if attempt.State != TokenCompletionStateRunning {
		return false
	}
	if policy.RunningTimeout <= 0 {
		return true
	}

	leaseAt := tokenCompletionRunningLeaseTimestamp(attempt)
	if leaseAt.IsZero() {
		return true
	}

	return now.Before(leaseAt.Add(policy.RunningTimeout))
}

func tokenCompletionHasRunningAttempt(email string, attempts []TokenCompletionAttempt) bool {
	return latestRunningTokenCompletionAttempt(relevantTokenCompletionAttempts(email, attempts)) != nil
}

func tokenCompletionHasRunningAttemptLease(email string, attempts []TokenCompletionAttempt, leaseToken string) bool {
	normalizedLeaseToken := strings.TrimSpace(leaseToken)
	if normalizedLeaseToken == "" {
		return tokenCompletionHasRunningAttempt(email, attempts)
	}

	for _, attempt := range relevantTokenCompletionAttempts(email, attempts) {
		if attempt.State != TokenCompletionStateRunning {
			continue
		}
		if strings.TrimSpace(attempt.LeaseToken) == normalizedLeaseToken {
			return true
		}
	}
	return false
}

func tokenCompletionCanPersistLeaseResult(policy TokenCompletionSchedulerPolicy, now time.Time, email string, storedState TokenCompletionRuntimeState, result TokenCompletionResult) bool {
	if strings.TrimSpace(result.LeaseToken) == "" {
		return true
	}
	switch result.State {
	case TokenCompletionStateRunning, TokenCompletionStateCompleted, TokenCompletionStateFailed:
		return tokenCompletionHasActiveRunningAttemptLease(policy, now, email, storedState.Attempts, result.LeaseToken)
	default:
		return true
	}
}

func tokenCompletionHasActiveRunningAttemptLease(
	policy TokenCompletionSchedulerPolicy,
	now time.Time,
	email string,
	attempts []TokenCompletionAttempt,
	leaseToken string,
) bool {
	normalizedLeaseToken := strings.TrimSpace(leaseToken)
	if normalizedLeaseToken == "" {
		return tokenCompletionHasRunningAttempt(email, attempts)
	}

	for _, attempt := range relevantTokenCompletionAttempts(email, attempts) {
		if attempt.State != TokenCompletionStateRunning {
			continue
		}
		if strings.TrimSpace(attempt.LeaseToken) != normalizedLeaseToken {
			continue
		}
		return tokenCompletionRunningAttemptActive(policy, now, attempt)
	}
	return false
}

func newTokenCompletionLeaseToken(now time.Time) string {
	return fmt.Sprintf("lease-%d-%d", now.UTC().UnixNano(), atomic.AddUint64(&tokenCompletionLeaseCounter, 1))
}

func resolveTokenCompletionHeartbeatInterval(configured time.Duration, runningTimeout time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	if runningTimeout <= 0 {
		return 0
	}

	interval := runningTimeout / 3
	if interval <= 0 {
		interval = runningTimeout
	}
	if interval > time.Minute {
		return time.Minute
	}
	return interval
}

func resolveTokenCompletionExternalLeaseTTL(heartbeatInterval time.Duration, runningTimeout time.Duration) time.Duration {
	if heartbeatInterval > 0 {
		ttl := heartbeatInterval * 2
		if ttl <= 0 {
			ttl = heartbeatInterval
		}
		if runningTimeout > 0 && ttl > runningTimeout {
			return runningTimeout
		}
		return ttl
	}
	if runningTimeout > 0 {
		return runningTimeout
	}
	return 0
}

func DefaultTokenCompletionSchedulerPolicy() TokenCompletionSchedulerPolicy {
	return TokenCompletionSchedulerPolicy{
		MinSpacing:     defaultTokenCompletionMinSpacing,
		BaseBackoff:    defaultTokenCompletionBaseBackoff,
		MaxBackoff:     defaultTokenCompletionMaxBackoff,
		Cooldown:       defaultTokenCompletionCooldown,
		RunningTimeout: defaultTokenCompletionCooldown,
	}
}
