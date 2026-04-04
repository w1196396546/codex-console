package nativerunner

import (
	"context"
	"errors"
	"strings"
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
	StartedAt   time.Time             `json:"started_at,omitempty"`
	CompletedAt time.Time             `json:"completed_at,omitempty"`
	Error       *TokenCompletionError `json:"error,omitempty"`
}

type TokenCompletionSchedulerPolicy struct {
	MinSpacing  time.Duration
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Cooldown    time.Duration
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
		if attempt.State == TokenCompletionStateRunning {
			plan.Allowed = false
			plan.BlockedReason = &TokenCompletionError{
				Kind:    TokenCompletionErrorKindEmailConflict,
				Message: "token completion already running for email",
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
	Account      TokenCompletionAccount   `json:"account"`
	Attempts     []TokenCompletionAttempt `json:"attempts,omitempty"`
	ContinueURL  string                   `json:"continue_url,omitempty"`
	CallbackURL  string                   `json:"callback_url,omitempty"`
	PageType     string                   `json:"page_type,omitempty"`
	AccountID    string                   `json:"account_id,omitempty"`
	WorkspaceID  string                   `json:"workspace_id,omitempty"`
	Inbox        mail.Inbox               `json:"inbox,omitempty"`
	MailProvider mail.Provider            `json:"-"`
	AuthClient   *auth.Client             `json:"-"`
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
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
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
	NextEligibleAt *time.Time                    `json:"next_eligible_at,omitempty"`
	Error          *TokenCompletionError         `json:"error,omitempty"`
	Provider       TokenCompletionProviderResult `json:"provider,omitempty"`
}

type TokenCompletionCoordinatorOptions struct {
	Scheduler *TokenCompletionScheduler
	Provider  TokenCompletionProvider
	Now       func() time.Time
}

type TokenCompletionCoordinator struct {
	scheduler *TokenCompletionScheduler
	provider  TokenCompletionProvider
	now       func() time.Time
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
		scheduler: scheduler,
		provider:  options.Provider,
		now:       now,
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
	plan := c.scheduler.Plan(now, command.Account, command.Attempts)
	result := TokenCompletionResult{
		Email:          strings.TrimSpace(command.Account.Email),
		Strategy:       plan.Strategy,
		NextEligibleAt: plan.NextEligibleAt,
	}
	if !plan.Allowed {
		result.State = TokenCompletionStateBlocked
		result.Error = plan.BlockedReason
		return result, nil
	}

	providerResult, err := c.provider.CompleteToken(ctx, TokenCompletionRequest{
		Email:        strings.TrimSpace(command.Account.Email),
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
	if err != nil {
		result.State = TokenCompletionStateFailed
		result.Error = classifyTokenCompletionError(err)
		return result, nil
	}

	result.State = TokenCompletionStateCompleted
	result.Provider = providerResult
	return result, nil
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

func tokenCompletionAttemptTimestamp(attempt TokenCompletionAttempt) time.Time {
	if !attempt.CompletedAt.IsZero() && attempt.CompletedAt.After(attempt.StartedAt) {
		return attempt.CompletedAt
	}
	if !attempt.StartedAt.IsZero() {
		return attempt.StartedAt
	}
	return attempt.CompletedAt
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
