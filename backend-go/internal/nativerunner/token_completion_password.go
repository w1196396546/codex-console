package nativerunner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
)

type authPasswordTokenCompletionProvider struct {
	client *auth.Client
}

func NewAuthPasswordTokenCompletionProvider(client *auth.Client) TokenCompletionProvider {
	return &authPasswordTokenCompletionProvider{client: client}
}

func (p *authPasswordTokenCompletionProvider) CompleteToken(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	if request.Strategy != TokenCompletionStrategyPassword {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindUnsupported,
			Message: "password token completion provider only supports password strategy",
		}
	}
	if strings.TrimSpace(request.Password) == "" {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindMissingPassword,
			Message: "password token completion requires historical password",
		}
	}

	client := p.client
	if request.AuthClient != nil {
		client = request.AuthClient
	}
	if client == nil {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindProviderUnavailable,
			Message: "password token completion auth client is required",
		}
	}

	completed, err := client.CompletePasswordLogin(ctx, request.Email, request.Password)
	if err != nil {
		var stepErr *auth.PasswordLoginStepError
		if errors.As(err, &stepErr) {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:    TokenCompletionErrorKindInteractiveStepRequired,
				Message: stepErr.Error(),
			}
		}
		return TokenCompletionProviderResult{}, fmt.Errorf("complete password token login: %w", err)
	}

	return TokenCompletionProviderResult{
		AccessToken:        strings.TrimSpace(completed.AccessToken),
		RefreshToken:       strings.TrimSpace(completed.RefreshToken),
		SessionToken:       strings.TrimSpace(completed.SessionToken),
		AccountID:          strings.TrimSpace(completed.AccountID),
		WorkspaceID:        strings.TrimSpace(completed.WorkspaceID),
		Source:             "login",
		AuthProvider:       strings.TrimSpace(completed.AuthProvider),
		AccessTokenSource:  "session",
		SessionTokenSource: "session",
		RefreshTokenSource: firstNonEmptyTrimmed(strings.TrimSpace(completed.RefreshTokenSource), inferRefreshTokenSource(strings.TrimSpace(completed.RefreshToken))),
	}, nil
}

type strategyTokenCompletionProvider struct {
	passwordProvider     TokenCompletionProvider
	passwordlessProvider TokenCompletionProvider
}

func NewStrategyTokenCompletionProvider(passwordProvider TokenCompletionProvider, passwordlessProvider TokenCompletionProvider) TokenCompletionProvider {
	return &strategyTokenCompletionProvider{
		passwordProvider:     passwordProvider,
		passwordlessProvider: passwordlessProvider,
	}
}

func (p *strategyTokenCompletionProvider) CompleteToken(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
	if p == nil {
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindProviderUnavailable,
			Message: "token completion strategy provider is required",
		}
	}

	switch request.Strategy {
	case TokenCompletionStrategyPassword:
		if p.passwordProvider == nil {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:    TokenCompletionErrorKindProviderUnavailable,
				Message: "password token completion provider is required",
			}
		}
		result, err := p.passwordProvider.CompleteToken(ctx, request)
		if err == nil {
			return result, nil
		}

		var tokenErr *TokenCompletionError
		if errors.As(err, &tokenErr) &&
			(tokenErr.Kind == TokenCompletionErrorKindInteractiveStepRequired || tokenErr.Kind == TokenCompletionErrorKindMissingPassword) &&
			p.passwordlessProvider != nil {
			fallbackRequest := request
			fallbackRequest.Strategy = TokenCompletionStrategyPasswordless
			fallbackRequest.Password = ""
			return p.passwordlessProvider.CompleteToken(ctx, fallbackRequest)
		}
		return TokenCompletionProviderResult{}, err
	case TokenCompletionStrategyPasswordless:
		if p.passwordlessProvider == nil {
			return TokenCompletionProviderResult{}, &TokenCompletionError{
				Kind:    TokenCompletionErrorKindProviderUnavailable,
				Message: "passwordless token completion provider is required",
			}
		}
		return p.passwordlessProvider.CompleteToken(ctx, request)
	default:
		return TokenCompletionProviderResult{}, &TokenCompletionError{
			Kind:    TokenCompletionErrorKindUnsupported,
			Message: fmt.Sprintf("unsupported token completion strategy %q", strings.TrimSpace(string(request.Strategy))),
		}
	}
}

func inferRefreshTokenSource(refreshToken string) string {
	if strings.TrimSpace(refreshToken) == "" {
		return ""
	}
	return "session"
}
