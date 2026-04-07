package nativerunner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
)

type authPasswordlessTokenCompletionProvider struct {
	client *auth.Client
}

func NewAuthPasswordlessTokenCompletionProvider(client *auth.Client) TokenCompletionProvider {
	return &authPasswordlessTokenCompletionProvider{client: client}
}

func (p *authPasswordlessTokenCompletionProvider) CompleteToken(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	if request.Strategy != TokenCompletionStrategyPasswordless {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindUnsupported,
			Message: "passwordless token completion provider only supports passwordless strategy",
		}
	}

	client := p.client
	if request.AuthClient != nil {
		client = request.AuthClient
	}
	if client == nil {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindProviderUnavailable,
			Message: "passwordless token completion auth client is required",
		}
	}

	continueURL := strings.TrimSpace(request.ContinueURL)
	callbackURL := strings.TrimSpace(request.CallbackURL)
	if continueURL == "" && callbackURL == "" {
		email := strings.TrimSpace(request.Email)
		if email == "" {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:    TokenCompletionErrorKindProviderUnavailable,
				Message: "passwordless token completion email is required to initiate auth flow",
			}
		}

		preparer := NewAuthSignupPreparer(client)
		if preparer == nil {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:    TokenCompletionErrorKindProviderUnavailable,
				Message: "passwordless token completion auth signup preparer is required",
			}
		}

		preparation, err := preparer.PrepareSignup(ctx, email)
		if err != nil {
			return TokenCompletionProviderResult{}, fmt.Errorf("initiate passwordless token completion: %w", err)
		}
		preparation = normalizePasswordlessPreparation(preparation)
		if strings.TrimSpace(preparation.PageType) != stageEmailOTPVerification {
			return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
				"passwordless token completion requires email otp verification before automatic completion",
				preparation.PageType,
				preparation.FinalPath,
			)
		}

		inbox := request.Inbox
		if strings.TrimSpace(inbox.Email) == "" {
			inbox.Email = email
		}
		if inbox.OTPSentAt.IsZero() {
			inbox.OTPSentAt = time.Now().UTC()
		}
		if request.MailProvider == nil {
			return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
				"passwordless token completion requires mail provider to consume email otp automatically",
				preparation.PageType,
				preparation.FinalPath,
			)
		}

		otpCode, err := request.MailProvider.WaitCode(ctx, inbox, mail.DefaultCodePattern)
		if err != nil {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Message:   fmt.Sprintf("passwordless token completion wait email otp: %v", err),
				Retryable: true,
			}
		}
		otpCode = strings.TrimSpace(otpCode)
		if otpCode == "" {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Message:   "passwordless token completion wait email otp: otp code is required",
				Retryable: true,
			}
		}

		verifiedPreparation, err := verifyPreparedEmailOTPWithRetries(ctx, client, request.MailProvider, inbox, toAuthPrepareSignupResult(preparation), otpCode)
		if err != nil {
			return TokenCompletionProviderResult{}, fmt.Errorf("verify passwordless email otp: %w", err)
		}
		if strings.TrimSpace(verifiedPreparation.PageType) != "about_you" {
			if shouldDispatchTokenCompletionAfterEmailOTP(verifiedPreparation) {
				completed, err := client.ContinueCreateAccount(ctx, createAccountResultFromVerifiedPreparation(verifiedPreparation))
				if err != nil {
					return TokenCompletionProviderResult{}, fmt.Errorf("continue passwordless existing account flow: %w", err)
				}
				if passwordlessInteractivePageType(completed.PageType) {
					return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
						"passwordless token completion requires interactive post-otp step after existing account verification",
						completed.PageType,
						firstNonEmptyTrimmed(completed.FinalPath, completed.FinalURL),
					)
				}
				return passwordlessCompletionResult(completed, auth.CreateAccountResult{}), nil
			}
			return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
				"passwordless token completion requires about_you after email otp verification",
				verifiedPreparation.PageType,
				verifiedPreparation.FinalPath,
			)
		}

		accountProfile, err := defaultAccountProfile()
		if err != nil {
			return TokenCompletionProviderResult{}, fmt.Errorf("resolve default passwordless account profile: %w", err)
		}

		createAccountResult, err := client.CreateAccount(
			ctx,
			verifiedPreparation,
			accountProfile.FirstName,
			accountProfile.LastName,
			accountProfile.Birthdate,
		)
		if err != nil {
			return TokenCompletionProviderResult{}, fmt.Errorf("create passwordless account: %w", err)
		}
		if strings.TrimSpace(createAccountResult.CallbackURL) == "" && strings.TrimSpace(createAccountResult.ContinueURL) == "" {
			return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
				"passwordless token completion requires callback or continue url after create account",
				createAccountResult.PageType,
				createAccountResult.FinalPath,
			)
		}

		completed, err := client.ContinueCreateAccount(ctx, createAccountResult)
		if err != nil {
			return TokenCompletionProviderResult{}, fmt.Errorf("continue passwordless create account: %w", err)
		}
		if passwordlessInteractivePageType(completed.PageType) {
			return TokenCompletionProviderResult{}, passwordlessInteractiveStepError(
				"passwordless token completion requires interactive post-signup step after create account",
				completed.PageType,
				firstNonEmptyTrimmed(completed.FinalPath, completed.FinalURL, createAccountResult.FinalPath),
			)
		}

		return passwordlessCompletionResult(completed, createAccountResult), nil
	}

	completed, err := client.ContinueCreateAccount(ctx, auth.CreateAccountResult{
		ContinueURL: continueURL,
		CallbackURL: callbackURL,
		PageType:    strings.TrimSpace(request.PageType),
		AccountID:   strings.TrimSpace(request.AccountID),
		WorkspaceID: strings.TrimSpace(request.WorkspaceID),
	})
	if err != nil {
		return TokenCompletionProviderResult{}, err
	}

	return passwordlessCompletionResult(completed, auth.CreateAccountResult{}), nil
}

func normalizePasswordlessPreparation(preparation SignupPreparation) SignupPreparation {
	if strings.TrimSpace(preparation.PageType) != "existing_account_detected" {
		return preparation
	}

	finalTarget := strings.ToLower(firstNonEmptyTrimmed(
		preparation.FinalPath,
		preparation.FinalURL,
		preparation.ContinueURL,
	))
	switch {
	case strings.Contains(finalTarget, "/email-verification"), strings.Contains(finalTarget, "/email-otp"):
		preparation.PageType = stageEmailOTPVerification
	case strings.Contains(finalTarget, "/log-in/password"):
		preparation.PageType = "login_password"
	}
	return preparation
}

func passwordlessCompletionResult(completed auth.ContinueCreateAccountResult, created auth.CreateAccountResult) TokenCompletionProviderResult {
	refreshToken := firstNonEmptyTrimmed(completed.RefreshToken, created.RefreshToken)
	refreshTokenSource := firstNonEmptyTrimmed(strings.TrimSpace(completed.RefreshTokenSource))
	if refreshTokenSource == "" && refreshToken != "" {
		if strings.TrimSpace(created.RefreshToken) != "" {
			refreshTokenSource = "create_account"
		} else if strings.TrimSpace(completed.RefreshToken) != "" {
			refreshTokenSource = "session"
		}
	}
	source := "login"
	if strings.TrimSpace(created.AccountID) != "" || strings.TrimSpace(created.WorkspaceID) != "" || strings.TrimSpace(created.RefreshToken) != "" {
		source = "register"
	}
	return TokenCompletionProviderResult{
		AccessToken:        strings.TrimSpace(completed.AccessToken),
		RefreshToken:       refreshToken,
		SessionToken:       strings.TrimSpace(completed.SessionToken),
		AccountID:          firstNonEmptyTrimmed(completed.AccountID, created.AccountID),
		WorkspaceID:        firstNonEmptyTrimmed(completed.WorkspaceID, created.WorkspaceID),
		Source:             source,
		AuthProvider:       strings.TrimSpace(completed.AuthProvider),
		AccessTokenSource:  "session",
		SessionTokenSource: "session",
		RefreshTokenSource: refreshTokenSource,
	}
}

func passwordlessInteractiveStepError(message string, pageType string, finalPath string) *TokenCompletionError {
	return &TokenCompletionError{
		Kind: TokenCompletionErrorKindInteractiveStepRequired,
		Message: fmt.Sprintf(
			"%s (page_type=%s final_path=%s)",
			strings.TrimSpace(message),
			strings.TrimSpace(pageType),
			strings.TrimSpace(finalPath),
		),
	}
}

func passwordlessInteractivePageType(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "about_you", "add_phone", "email_otp_verification", "create_account_password", "login_password":
		return true
	default:
		return false
	}
}
