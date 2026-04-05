package nativerunner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

const stageEmailOTPVerification = "email_otp_verification"
const stageCompleted = "completed"

type SignupPreparation struct {
	CSRFToken          string
	AuthorizeURL       string
	FinalURL           string
	FinalPath          string
	ContinueURL        string
	PageType           string
	RegisterStatusCode int
	SendOTPStatusCode  int
	Password           string
}

type SignupPreparer interface {
	PrepareSignup(ctx context.Context, email string) (SignupPreparation, error)
}

type SignupPreparerFunc func(ctx context.Context, email string) (SignupPreparation, error)

func (f SignupPreparerFunc) PrepareSignup(ctx context.Context, email string) (SignupPreparation, error) {
	return f(ctx, email)
}

type SignupPreparerFactory interface {
	NewSignupPreparer(ctx context.Context, input FlowRequest) (SignupPreparer, error)
}

type SignupPreparerFactoryFunc func(ctx context.Context, input FlowRequest) (SignupPreparer, error)

func (f SignupPreparerFactoryFunc) NewSignupPreparer(ctx context.Context, input FlowRequest) (SignupPreparer, error) {
	return f(ctx, input)
}

type AuthPostSignupClient interface {
	VerifyEmailOTP(ctx context.Context, prepared auth.PrepareSignupResult, code string) (auth.PrepareSignupResult, error)
	CreateAccount(ctx context.Context, prepared auth.PrepareSignupResult, firstName string, lastName string, birthdate string) (auth.CreateAccountResult, error)
	ContinueCreateAccount(ctx context.Context, created auth.CreateAccountResult) (auth.ContinueCreateAccountResult, error)
	ReadSession(ctx context.Context) (auth.SessionResult, error)
}

type AuthPostSignupClientFactory interface {
	NewPostSignupClient(ctx context.Context, input FlowRequest) (AuthPostSignupClient, error)
}

type AuthPostSignupClientFactoryFunc func(ctx context.Context, input FlowRequest) (AuthPostSignupClient, error)

func (f AuthPostSignupClientFactoryFunc) NewPostSignupClient(ctx context.Context, input FlowRequest) (AuthPostSignupClient, error) {
	return f(ctx, input)
}

type AccountProfile struct {
	FirstName string
	LastName  string
	Birthdate string
}

type AccountProfileProvider interface {
	ResolveAccountProfile(ctx context.Context, input FlowRequest) (AccountProfile, error)
}

type AccountProfileProviderFunc func(ctx context.Context, input FlowRequest) (AccountProfile, error)

func (f AccountProfileProviderFunc) ResolveAccountProfile(ctx context.Context, input FlowRequest) (AccountProfile, error) {
	return f(ctx, input)
}

type ClientIDResolver interface {
	ResolveClientID(ctx context.Context, input FlowRequest) (string, error)
}

type ClientIDResolverFunc func(ctx context.Context, input FlowRequest) (string, error)

func (f ClientIDResolverFunc) ResolveClientID(ctx context.Context, input FlowRequest) (string, error) {
	return f(ctx, input)
}

type HistoricalPasswordProvider interface {
	ResolveHistoricalPassword(ctx context.Context, input FlowRequest, email string) (string, error)
}

type HistoricalPasswordProviderFunc func(ctx context.Context, input FlowRequest, email string) (string, error)

func (f HistoricalPasswordProviderFunc) ResolveHistoricalPassword(ctx context.Context, input FlowRequest, email string) (string, error) {
	return f(ctx, input, email)
}

type TokenCompletionCooldownProvider interface {
	ResolveTokenCompletionCooldown(ctx context.Context, input FlowRequest, email string) (*time.Time, error)
}

type TokenCompletionCooldownProviderFunc func(ctx context.Context, input FlowRequest, email string) (*time.Time, error)

func (f TokenCompletionCooldownProviderFunc) ResolveTokenCompletionCooldown(ctx context.Context, input FlowRequest, email string) (*time.Time, error) {
	return f(ctx, input, email)
}

type TokenCompletionAttemptProvider interface {
	ResolveTokenCompletionAttempts(ctx context.Context, input FlowRequest, email string) ([]TokenCompletionAttempt, error)
}

type TokenCompletionAttemptProviderFunc func(ctx context.Context, input FlowRequest, email string) ([]TokenCompletionAttempt, error)

func (f TokenCompletionAttemptProviderFunc) ResolveTokenCompletionAttempts(ctx context.Context, input FlowRequest, email string) ([]TokenCompletionAttempt, error) {
	return f(ctx, input, email)
}

type TokenCompletionDispatcher interface {
	Complete(ctx context.Context, command TokenCompletionCommand) (TokenCompletionResult, error)
}

type PrepareSignupFlowOptions struct {
	PreparerFactory                 SignupPreparerFactory
	PostSignupClientFactory         AuthPostSignupClientFactory
	AccountProfileProvider          AccountProfileProvider
	ClientIDResolver                ClientIDResolver
	HistoricalPasswordProvider      HistoricalPasswordProvider
	TokenCompletionCooldownProvider TokenCompletionCooldownProvider
	TokenCompletionAttemptProvider  TokenCompletionAttemptProvider
	TokenCompletionCoordinator      TokenCompletionDispatcher
}

type PrepareSignupFlow struct {
	preparerFactory                 SignupPreparerFactory
	postSignupClientFactory         AuthPostSignupClientFactory
	accountProfileProvider          AccountProfileProvider
	clientIDResolver                ClientIDResolver
	historicalPasswordProvider      HistoricalPasswordProvider
	tokenCompletionCooldownProvider TokenCompletionCooldownProvider
	tokenCompletionAttemptProvider  TokenCompletionAttemptProvider
	tokenCompletionCoordinator      TokenCompletionDispatcher
}

var _ Flow = (*PrepareSignupFlow)(nil)

func NewPrepareSignupFlow(options PrepareSignupFlowOptions) *PrepareSignupFlow {
	return &PrepareSignupFlow{
		preparerFactory:                 options.PreparerFactory,
		postSignupClientFactory:         options.PostSignupClientFactory,
		accountProfileProvider:          options.AccountProfileProvider,
		clientIDResolver:                options.ClientIDResolver,
		historicalPasswordProvider:      options.HistoricalPasswordProvider,
		tokenCompletionCooldownProvider: options.TokenCompletionCooldownProvider,
		tokenCompletionAttemptProvider:  options.TokenCompletionAttemptProvider,
		tokenCompletionCoordinator:      options.TokenCompletionCoordinator,
	}
}

func (f *PrepareSignupFlow) Run(ctx context.Context, input FlowRequest) (registration.NativeRunnerResult, error) {
	if f == nil {
		return registration.NativeRunnerResult{}, errors.New("prepare signup flow is required")
	}
	if f.preparerFactory == nil {
		return registration.NativeRunnerResult{}, errors.New("signup preparer factory is required")
	}
	if input.runtime == nil {
		input.runtime = &flowRuntime{}
	}

	email := strings.TrimSpace(input.Inbox.Email)
	if email == "" {
		return registration.NativeRunnerResult{}, errors.New("signup inbox email is required")
	}
	if input.MailProvider == nil {
		return registration.NativeRunnerResult{}, errors.New("mail provider is required")
	}

	logf := input.Logf
	if logf == nil {
		logf = func(string, string) error { return nil }
	}
	if err := logf("info", fmt.Sprintf("prepare signup started for %s", email)); err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("log prepare signup start: %w", err)
	}

	preparer, err := f.preparerFactory.NewSignupPreparer(ctx, input)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("create signup preparer: %w", err)
	}
	if preparer == nil {
		return registration.NativeRunnerResult{}, errors.New("signup preparer is required")
	}

	preparation, err := preparer.PrepareSignup(ctx, email)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("prepare signup: %w", err)
	}
	if f.postSignupClientFactory == nil {
		return registration.NativeRunnerResult{}, errors.New("post-signup auth client factory is required")
	}
	if f.accountProfileProvider == nil {
		return registration.NativeRunnerResult{}, errors.New("account profile provider is required")
	}
	if f.clientIDResolver == nil {
		return registration.NativeRunnerResult{}, errors.New("account client id resolver is required")
	}

	postSignupClient, err := f.postSignupClientFactory.NewPostSignupClient(ctx, input)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("create post-signup auth client: %w", err)
	}
	if postSignupClient == nil {
		return registration.NativeRunnerResult{}, errors.New("post-signup auth client is required")
	}

	inbox := input.Inbox
	if inbox.OTPSentAt.IsZero() {
		inbox.OTPSentAt = time.Now().UTC()
	}

	otpCode, err := input.MailProvider.WaitCode(ctx, inbox, mail.DefaultCodePattern)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("read inbox email otp: %w", err)
	}
	otpCode = strings.TrimSpace(otpCode)
	if otpCode == "" {
		return registration.NativeRunnerResult{}, errors.New("read inbox email otp: otp code is required")
	}

	verifiedPreparation, err := postSignupClient.VerifyEmailOTP(ctx, toAuthPrepareSignupResult(preparation), otpCode)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("verify email otp: %w", err)
	}
	if strings.TrimSpace(verifiedPreparation.PageType) != "about_you" {
		return registration.NativeRunnerResult{}, fmt.Errorf("verify email otp result expected page type about_you, got %s", strings.TrimSpace(verifiedPreparation.PageType))
	}

	accountProfile, err := f.accountProfileProvider.ResolveAccountProfile(ctx, input)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("resolve account profile: %w", err)
	}
	if err := validateAccountProfile(accountProfile); err != nil {
		return registration.NativeRunnerResult{}, err
	}

	createAccountResult, err := postSignupClient.CreateAccount(
		ctx,
		verifiedPreparation,
		accountProfile.FirstName,
		accountProfile.LastName,
		accountProfile.Birthdate,
	)
	if err != nil {
		var userExistsErr *auth.CreateAccountUserExistsError
		if !errors.As(err, &userExistsErr) {
			return registration.NativeRunnerResult{}, fmt.Errorf("create account: %w", err)
		}
		if createAccountResult.StatusCode == 0 &&
			strings.TrimSpace(createAccountResult.PageType) == "" &&
			strings.TrimSpace(createAccountResult.ContinueURL) == "" &&
			strings.TrimSpace(createAccountResult.CallbackURL) == "" &&
			strings.TrimSpace(createAccountResult.AccountID) == "" &&
			strings.TrimSpace(createAccountResult.WorkspaceID) == "" &&
			strings.TrimSpace(createAccountResult.RefreshToken) == "" {
			createAccountResult = userExistsErr.Result
		}
	}

	clientID, err := f.clientIDResolver.ResolveClientID(ctx, input)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("resolve account client id: %w", err)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return registration.NativeRunnerResult{}, errors.New("resolve account client id: client id is required")
	}

	if shouldDispatchTokenCompletion(createAccountResult) {
		return f.completeExistingAccountToken(
			ctx,
			input,
			email,
			preparation,
			verifiedPreparation,
			postSignupClient,
			createAccountResult,
			clientID,
			logf,
		)
	}
	if strings.TrimSpace(createAccountResult.RefreshToken) == "" {
		return registration.NativeRunnerResult{}, errors.New("create account result missing refresh token")
	}

	continueCreateAccountResult, err := postSignupClient.ContinueCreateAccount(ctx, createAccountResult)
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("continue create account: %w", err)
	}
	if strings.TrimSpace(continueCreateAccountResult.AccessToken) == "" {
		return registration.NativeRunnerResult{}, errors.New("continue create account result missing access token")
	}
	if strings.TrimSpace(continueCreateAccountResult.SessionToken) == "" {
		return registration.NativeRunnerResult{}, errors.New("continue create account result missing session token")
	}

	workspaceID := strings.TrimSpace(continueCreateAccountResult.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(createAccountResult.WorkspaceID)
	}
	if workspaceID == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing workspace id")
	}

	accountID := strings.TrimSpace(continueCreateAccountResult.AccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(createAccountResult.AccountID)
	}
	if accountID == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing account id")
	}

	refreshToken := strings.TrimSpace(continueCreateAccountResult.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(createAccountResult.RefreshToken)
	}
	if refreshToken == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing refresh token")
	}

	emailServiceType, _ := resolveMailProvider(input.RunnerRequest)
	resultMetadata := map[string]any{
		"auth_provider":               strings.TrimSpace(continueCreateAccountResult.AuthProvider),
		"refresh_token_source":        "create_account",
		"has_session_token":           strings.TrimSpace(continueCreateAccountResult.SessionToken) != "",
		"create_account_callback_url": strings.TrimSpace(createAccountResult.CallbackURL),
		"create_account_continue_url": strings.TrimSpace(createAccountResult.ContinueURL),
	}
	accountPersistence := &accounts.UpsertAccountRequest{
		Email:          email,
		Password:       preparation.Password,
		ClientID:       clientID,
		SessionToken:   continueCreateAccountResult.SessionToken,
		EmailService:   strings.TrimSpace(emailServiceType),
		EmailServiceID: resolveEmailServiceID(input.RunnerRequest),
		AccountID:      accountID,
		WorkspaceID:    workspaceID,
		AccessToken:    continueCreateAccountResult.AccessToken,
		RefreshToken:   refreshToken,
		ProxyUsed:      strings.TrimSpace(input.RunnerRequest.Plan.Proxy.Selected),
		Status:         accounts.DefaultAccountStatus,
		Source:         accounts.DefaultAccountSource,
		ExtraData: map[string]any{
			"auth_provider":        strings.TrimSpace(continueCreateAccountResult.AuthProvider),
			"task_uuid":            strings.TrimSpace(input.RunnerRequest.TaskUUID),
			"flow":                 "native_runner",
			"refresh_token_source": "create_account",
		},
	}
	attachAuthSessionArtifacts(postSignupClient, accountPersistence, resultMetadata)

	if err := logf("info", fmt.Sprintf("prepare signup completed for %s page_type=%s final_path=%s", email, strings.TrimSpace(continueCreateAccountResult.PageType), strings.TrimSpace(continueCreateAccountResult.FinalPath))); err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("log prepare signup completion: %w", err)
	}

	return registration.NativeRunnerResult{
		Result: map[string]any{
			"success":       true,
			"stage":         stageCompleted,
			"email":         email,
			"account_id":    accountID,
			"workspace_id":  workspaceID,
			"access_token":  continueCreateAccountResult.AccessToken,
			"refresh_token": refreshToken,
			"session_token": continueCreateAccountResult.SessionToken,
			"password":      preparation.Password,
			"metadata":      resultMetadata,
			"inbox": map[string]any{
				"email": email,
				"token": input.Inbox.Token,
			},
			"signup_preparation": map[string]any{
				"csrf_token":           preparation.CSRFToken,
				"authorize_url":        preparation.AuthorizeURL,
				"final_url":            preparation.FinalURL,
				"final_path":           preparation.FinalPath,
				"continue_url":         preparation.ContinueURL,
				"page_type":            preparation.PageType,
				"register_status_code": preparation.RegisterStatusCode,
				"send_otp_status_code": preparation.SendOTPStatusCode,
			},
			"email_otp": map[string]any{
				"code": otpCode,
			},
			"email_verification": map[string]any{
				"continue_url": verifiedPreparation.ContinueURL,
				"final_url":    verifiedPreparation.FinalURL,
				"final_path":   verifiedPreparation.FinalPath,
				"page_type":    verifiedPreparation.PageType,
			},
			"create_account": map[string]any{
				"account_id":    createAccountResult.AccountID,
				"workspace_id":  createAccountResult.WorkspaceID,
				"refresh_token": refreshToken,
				"continue_url":  createAccountResult.ContinueURL,
				"callback_url":  createAccountResult.CallbackURL,
				"page_type":     createAccountResult.PageType,
			},
			"session": map[string]any{
				"access_token":  continueCreateAccountResult.AccessToken,
				"session_token": continueCreateAccountResult.SessionToken,
				"account_id":    continueCreateAccountResult.AccountID,
				"workspace_id":  continueCreateAccountResult.WorkspaceID,
				"auth_provider": continueCreateAccountResult.AuthProvider,
			},
		},
		AccountPersistence: accountPersistence,
	}, nil
}

func resolveSignupStage(preparation SignupPreparation) string {
	stage := strings.TrimSpace(preparation.PageType)
	if stage == "" {
		return stageEmailOTPVerification
	}
	return stage
}

func toAuthPrepareSignupResult(preparation SignupPreparation) auth.PrepareSignupResult {
	return auth.PrepareSignupResult{
		CSRFToken:          preparation.CSRFToken,
		AuthorizeURL:       preparation.AuthorizeURL,
		FinalURL:           preparation.FinalURL,
		FinalPath:          preparation.FinalPath,
		ContinueURL:        preparation.ContinueURL,
		PageType:           preparation.PageType,
		RegisterStatusCode: preparation.RegisterStatusCode,
		SendOTPStatusCode:  preparation.SendOTPStatusCode,
	}
}

func validateAccountProfile(profile AccountProfile) error {
	if strings.TrimSpace(profile.FirstName) == "" {
		return errors.New("account profile first name is required")
	}
	if strings.TrimSpace(profile.LastName) == "" {
		return errors.New("account profile last name is required")
	}
	if strings.TrimSpace(profile.Birthdate) == "" {
		return errors.New("account profile birthdate is required")
	}
	return nil
}

func resolveEmailServiceID(req registration.RunnerRequest) string {
	if req.Plan.EmailService.ServiceID != nil {
		return strconv.Itoa(*req.Plan.EmailService.ServiceID)
	}
	if req.StartRequest.EmailServiceID != nil {
		return strconv.Itoa(*req.StartRequest.EmailServiceID)
	}
	return ""
}

func shouldDispatchTokenCompletion(createAccountResult auth.CreateAccountResult) bool {
	return isExistingAccountPageType(createAccountResult.PageType) &&
		strings.TrimSpace(createAccountResult.RefreshToken) == ""
}

func (f *PrepareSignupFlow) completeExistingAccountToken(
	ctx context.Context,
	input FlowRequest,
	email string,
	preparation SignupPreparation,
	verifiedPreparation auth.PrepareSignupResult,
	postSignupClient AuthPostSignupClient,
	createAccountResult auth.CreateAccountResult,
	clientID string,
	logf func(level string, message string) error,
) (registration.NativeRunnerResult, error) {
	if f.tokenCompletionCoordinator == nil {
		return registration.NativeRunnerResult{}, errors.New("token completion coordinator is required")
	}
	emailServiceType, _ := resolveMailProvider(input.RunnerRequest)

	historicalPassword := ""
	if f.historicalPasswordProvider != nil {
		resolved, err := f.historicalPasswordProvider.ResolveHistoricalPassword(ctx, input, email)
		if err != nil {
			return registration.NativeRunnerResult{}, fmt.Errorf("resolve historical password: %w", err)
		}
		historicalPassword = strings.TrimSpace(resolved)
	}

	var cooldownUntil *time.Time
	if f.tokenCompletionCooldownProvider != nil {
		resolved, err := f.tokenCompletionCooldownProvider.ResolveTokenCompletionCooldown(ctx, input, email)
		if err != nil {
			return registration.NativeRunnerResult{}, fmt.Errorf("resolve token completion cooldown: %w", err)
		}
		if resolved != nil {
			cloned := resolved.UTC()
			cooldownUntil = &cloned
		}
	}

	var attempts []TokenCompletionAttempt
	if f.tokenCompletionAttemptProvider != nil {
		resolved, err := f.tokenCompletionAttemptProvider.ResolveTokenCompletionAttempts(ctx, input, email)
		if err != nil {
			return registration.NativeRunnerResult{}, fmt.Errorf("resolve token completion attempts: %w", err)
		}
		attempts = append(attempts, resolved...)
	}

	completionResult, err := f.tokenCompletionCoordinator.Complete(ctx, TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email:    email,
			Password: historicalPassword,
		},
		CooldownUntil: cooldownUntil,
		Attempts:      attempts,
		ContinueURL:   createAccountResult.ContinueURL,
		CallbackURL:   createAccountResult.CallbackURL,
		PageType:      createAccountResult.PageType,
		AccountID:     createAccountResult.AccountID,
		WorkspaceID:   createAccountResult.WorkspaceID,
		AuthClient:    authClientFromPostSignupClient(postSignupClient),
	})
	if err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("complete token: %w", err)
	}
	attemptsExtraData, cooldownExtraData := tokenCompletionPersistenceState(attempts, completionResult, time.Now().UTC())
	if completionResult.State != TokenCompletionStateCompleted {
		failureAccountPersistence := &accounts.UpsertAccountRequest{
			Email:          email,
			Password:       historicalPassword,
			EmailService:   strings.TrimSpace(emailServiceType),
			EmailServiceID: resolveEmailServiceID(input.RunnerRequest),
			AccountID:      firstNonEmptyTrimmed(createAccountResult.AccountID),
			WorkspaceID:    firstNonEmptyTrimmed(createAccountResult.WorkspaceID),
			ProxyUsed:      strings.TrimSpace(input.RunnerRequest.Plan.Proxy.Selected),
			Status:         tokenCompletionFailureStatus(completionResult),
			Source:         "login",
			ExtraData: map[string]any{
				"task_uuid":                    strings.TrimSpace(input.RunnerRequest.TaskUUID),
				"flow":                         "native_runner",
				"token_completion":             true,
				"signup_page_type":             strings.TrimSpace(createAccountResult.PageType),
				"token_completion_mode":        string(completionResult.Strategy),
				"existing_account_detected":    true,
				"refresh_token_cooldown_until": cooldownExtraData,
				"token_completion_attempts":    attemptsExtraData,
				"token_pending":                completionResult.Error != nil && completionResult.Error.Retryable,
				"login_incomplete":             completionResult.Error == nil || !completionResult.Error.Retryable,
				"account_status_reason":        tokenCompletionStatusReason(completionResult),
			},
		}
		attachAuthSessionArtifacts(postSignupClient, failureAccountPersistence, nil)
		return registration.NativeRunnerResult{}, newTokenCompletionPersistenceError(
			tokenCompletionDispatchError(completionResult),
			failureAccountPersistence,
		)
	}

	accessToken := strings.TrimSpace(completionResult.Provider.AccessToken)
	if accessToken == "" {
		return registration.NativeRunnerResult{}, errors.New("token completion result missing access token")
	}
	sessionToken := strings.TrimSpace(completionResult.Provider.SessionToken)
	if sessionToken == "" {
		return registration.NativeRunnerResult{}, errors.New("token completion result missing session token")
	}

	accountID := firstNonEmptyTrimmed(completionResult.Provider.AccountID, createAccountResult.AccountID)
	if accountID == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing account id")
	}
	workspaceID := firstNonEmptyTrimmed(completionResult.Provider.WorkspaceID, createAccountResult.WorkspaceID)
	if workspaceID == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing workspace id")
	}
	refreshToken := firstNonEmptyTrimmed(completionResult.Provider.RefreshToken, createAccountResult.RefreshToken)
	if refreshToken == "" {
		return registration.NativeRunnerResult{}, errors.New("registration result missing refresh token")
	}

	refreshTokenSource := "oauth_passwordless"
	if completionResult.Strategy == TokenCompletionStrategyPassword {
		refreshTokenSource = "oauth_password"
	}
	resultMetadata := map[string]any{
		"existing_account_detected":   true,
		"refresh_token_source":        refreshTokenSource,
		"has_session_token":           sessionToken != "",
		"create_account_callback_url": strings.TrimSpace(createAccountResult.CallbackURL),
		"create_account_continue_url": strings.TrimSpace(createAccountResult.ContinueURL),
	}
	accountPersistence := &accounts.UpsertAccountRequest{
		Email:          email,
		Password:       historicalPassword,
		ClientID:       clientID,
		SessionToken:   sessionToken,
		EmailService:   strings.TrimSpace(emailServiceType),
		EmailServiceID: resolveEmailServiceID(input.RunnerRequest),
		AccountID:      accountID,
		WorkspaceID:    workspaceID,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		ProxyUsed:      strings.TrimSpace(input.RunnerRequest.Plan.Proxy.Selected),
		Status:         accounts.DefaultAccountStatus,
		Source:         "login",
		ExtraData: map[string]any{
			"task_uuid":                    strings.TrimSpace(input.RunnerRequest.TaskUUID),
			"flow":                         "native_runner",
			"token_completion":             true,
			"signup_page_type":             strings.TrimSpace(createAccountResult.PageType),
			"token_completion_mode":        string(completionResult.Strategy),
			"existing_account_detected":    true,
			"refresh_token_source":         refreshTokenSource,
			"refresh_token_cooldown_until": cooldownExtraData,
			"token_completion_attempts":    attemptsExtraData,
		},
	}
	attachAuthSessionArtifacts(postSignupClient, accountPersistence, resultMetadata)

	if err := logf("info", fmt.Sprintf("prepare signup completed via token completion for %s page_type=%s", email, strings.TrimSpace(createAccountResult.PageType))); err != nil {
		return registration.NativeRunnerResult{}, fmt.Errorf("log prepare signup completion: %w", err)
	}

	return registration.NativeRunnerResult{
		Result: map[string]any{
			"success":       true,
			"source":        "login",
			"stage":         stageCompleted,
			"email":         email,
			"account_id":    accountID,
			"workspace_id":  workspaceID,
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"session_token": sessionToken,
			"password":      historicalPassword,
			"metadata":      resultMetadata,
			"inbox": map[string]any{
				"email": email,
				"token": input.Inbox.Token,
			},
			"signup_preparation": map[string]any{
				"csrf_token":           preparation.CSRFToken,
				"authorize_url":        preparation.AuthorizeURL,
				"final_url":            preparation.FinalURL,
				"final_path":           preparation.FinalPath,
				"continue_url":         preparation.ContinueURL,
				"page_type":            preparation.PageType,
				"register_status_code": preparation.RegisterStatusCode,
				"send_otp_status_code": preparation.SendOTPStatusCode,
			},
			"email_verification": map[string]any{
				"continue_url": verifiedPreparation.ContinueURL,
				"final_url":    verifiedPreparation.FinalURL,
				"final_path":   verifiedPreparation.FinalPath,
				"page_type":    verifiedPreparation.PageType,
			},
			"create_account": map[string]any{
				"account_id":    createAccountResult.AccountID,
				"workspace_id":  createAccountResult.WorkspaceID,
				"refresh_token": createAccountResult.RefreshToken,
				"continue_url":  createAccountResult.ContinueURL,
				"callback_url":  createAccountResult.CallbackURL,
				"page_type":     createAccountResult.PageType,
			},
			"token_completion": tokenCompletionResultMap(completionResult),
			"session": map[string]any{
				"access_token":  accessToken,
				"session_token": sessionToken,
				"account_id":    accountID,
				"workspace_id":  workspaceID,
			},
		},
		AccountPersistence: accountPersistence,
	}, nil
}

func authClientFromPostSignupClient(client AuthPostSignupClient) *auth.Client {
	authClient, _ := client.(*auth.Client)
	return authClient
}

func tokenCompletionDispatchError(result TokenCompletionResult) error {
	if result.Error != nil {
		return fmt.Errorf("token completion %s: %w", strings.TrimSpace(string(result.State)), result.Error)
	}
	if result.State == "" {
		return errors.New("token completion did not complete")
	}
	return fmt.Errorf("token completion %s", strings.TrimSpace(string(result.State)))
}

func tokenCompletionResultMap(result TokenCompletionResult) map[string]any {
	mapped := map[string]any{
		"state":    string(result.State),
		"email":    result.Email,
		"strategy": string(result.Strategy),
		"provider": map[string]any{
			"access_token":  strings.TrimSpace(result.Provider.AccessToken),
			"refresh_token": strings.TrimSpace(result.Provider.RefreshToken),
			"session_token": strings.TrimSpace(result.Provider.SessionToken),
			"account_id":    strings.TrimSpace(result.Provider.AccountID),
			"workspace_id":  strings.TrimSpace(result.Provider.WorkspaceID),
		},
	}
	if result.NextEligibleAt != nil {
		mapped["next_eligible_at"] = result.NextEligibleAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if result.Error != nil {
		mapped["error"] = map[string]any{
			"kind":      string(result.Error.Kind),
			"message":   result.Error.Message,
			"retryable": result.Error.Retryable,
		}
	}
	return mapped
}

type authCookieReader interface {
	Cookies() []*http.Cookie
}

func attachAuthSessionArtifacts(client AuthPostSignupClient, accountPersistence *accounts.UpsertAccountRequest, resultMetadata map[string]any) {
	cookieHeader, deviceID := authSessionArtifacts(client)

	if accountPersistence != nil {
		accountPersistence.Cookies = firstNonEmptyTrimmed(accountPersistence.Cookies, cookieHeader)
		if deviceID != "" {
			if accountPersistence.ExtraData == nil {
				accountPersistence.ExtraData = map[string]any{}
			}
			accountPersistence.ExtraData["device_id"] = deviceID
		}
	}

	if resultMetadata != nil && deviceID != "" {
		resultMetadata["device_id"] = deviceID
	}
}

func authSessionArtifacts(client AuthPostSignupClient) (string, string) {
	cookieReader, ok := client.(authCookieReader)
	if !ok || cookieReader == nil {
		return "", ""
	}

	cookies := cookieReader.Cookies()
	return serializeCookieHeader(cookies), deviceIDFromCookies(cookies)
}

func serializeCookieHeader(cookies []*http.Cookie) string {
	filtered := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		filtered = append(filtered, cookie)
	}
	if len(filtered) == 0 {
		return ""
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Name != filtered[j].Name {
			return filtered[i].Name < filtered[j].Name
		}
		return filtered[i].Value < filtered[j].Value
	})

	parts := make([]string, 0, len(filtered))
	for _, cookie := range filtered {
		parts = append(parts, strings.TrimSpace(cookie.Name)+"="+strings.TrimSpace(cookie.Value))
	}
	return strings.Join(parts, "; ")
}

func deviceIDFromCookies(cookies []*http.Cookie) string {
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name != "oai-did" {
			continue
		}
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

type tokenCompletionPersistenceError struct {
	cause error
	req   *accounts.UpsertAccountRequest
}

func newTokenCompletionPersistenceError(cause error, req *accounts.UpsertAccountRequest) error {
	if cause == nil {
		return nil
	}
	return &tokenCompletionPersistenceError{cause: cause, req: req}
}

func (e *tokenCompletionPersistenceError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *tokenCompletionPersistenceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *tokenCompletionPersistenceError) AccountPersistenceRequest() *accounts.UpsertAccountRequest {
	if e == nil {
		return nil
	}
	return e.req
}

func tokenCompletionPersistenceState(existing []TokenCompletionAttempt, result TokenCompletionResult, now time.Time) ([]map[string]any, string) {
	state := BuildTokenCompletionRuntimeState(now, result.Email, TokenCompletionRuntimeState{
		Attempts: existing,
	}, result)
	return serializeTokenCompletionAttempts(state.Attempts), formatTokenCompletionCooldown(state.CooldownUntil)
}

func tokenCompletionFailureStatus(result TokenCompletionResult) string {
	if result.Error != nil && result.Error.Retryable {
		return "token_pending"
	}
	return "login_incomplete"
}

func tokenCompletionStatusReason(result TokenCompletionResult) string {
	if result.Error != nil && strings.TrimSpace(string(result.Error.Kind)) != "" {
		return strings.TrimSpace(string(result.Error.Kind))
	}
	return strings.TrimSpace(string(result.State))
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isExistingAccountPageType(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "existing_account_detected", "user_exists":
		return true
	default:
		return false
	}
}
