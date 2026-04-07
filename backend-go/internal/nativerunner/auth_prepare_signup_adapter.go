package nativerunner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
)

type AuthPrepareSignupClient interface {
	PrepareSignup(ctx context.Context, email string) (auth.PrepareSignupResult, error)
	RegisterPasswordAndSendOTP(ctx context.Context, prepared auth.PrepareSignupResult, email string, password string) (auth.PrepareSignupResult, error)
}

type AuthSignupPasswordGenerator func() (string, error)

type AuthSignupPreparerOption func(*authSignupPreparer)

func WithAuthSignupPasswordGenerator(generator AuthSignupPasswordGenerator) AuthSignupPreparerOption {
	return func(preparer *authSignupPreparer) {
		if preparer == nil || generator == nil {
			return
		}
		preparer.passwordGenerator = generator
	}
}

type authSignupPreparer struct {
	client            AuthPrepareSignupClient
	passwordGenerator AuthSignupPasswordGenerator
}

func NewAuthSignupPreparer(client AuthPrepareSignupClient, options ...AuthSignupPreparerOption) SignupPreparer {
	if client == nil {
		return nil
	}

	preparer := &authSignupPreparer{
		client:            client,
		passwordGenerator: defaultAuthSignupPasswordGenerator,
	}
	for _, option := range options {
		if option != nil {
			option(preparer)
		}
	}
	if preparer.passwordGenerator == nil {
		preparer.passwordGenerator = defaultAuthSignupPasswordGenerator
	}

	return *preparer
}

func (p authSignupPreparer) PrepareSignup(ctx context.Context, email string) (SignupPreparation, error) {
	if p.client == nil {
		return SignupPreparation{}, errors.New("auth signup preparer client is required")
	}
	if p.passwordGenerator == nil {
		return SignupPreparation{}, errors.New("auth signup password generator is required")
	}

	prepared, err := p.client.PrepareSignup(ctx, email)
	if err != nil {
		return SignupPreparation{}, err
	}
	pageType := strings.TrimSpace(prepared.PageType)
	if isDirectExistingAccountPreparation(pageType) {
		return SignupPreparation{
			CSRFToken:          prepared.CSRFToken,
			AuthorizeURL:       prepared.AuthorizeURL,
			FinalURL:           prepared.FinalURL,
			FinalPath:          prepared.FinalPath,
			ContinueURL:        prepared.ContinueURL,
			PageType:           "existing_account_detected",
			RegisterStatusCode: prepared.RegisterStatusCode,
			SendOTPStatusCode:  prepared.SendOTPStatusCode,
		}, nil
	}
	if pageType != "create_account_password" && pageType != "password" {
		return SignupPreparation{}, fmt.Errorf(
			"prepare signup did not reach password registration stage: page_type=%s final_url=%s final_path=%s",
			pageType,
			strings.TrimSpace(prepared.FinalURL),
			strings.TrimSpace(prepared.FinalPath),
		)
	}

	password, err := p.passwordGenerator()
	if err != nil {
		return SignupPreparation{}, fmt.Errorf("generate signup password: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return SignupPreparation{}, errors.New("signup password is required")
	}

	result, err := p.client.RegisterPasswordAndSendOTP(ctx, prepared, email, password)
	if err != nil {
		return SignupPreparation{}, err
	}
	pageType = strings.TrimSpace(result.PageType)
	if pageType != stageEmailOTPVerification && !isExistingAccountPageType(pageType) {
		return SignupPreparation{}, fmt.Errorf("unexpected signup stage after sending otp: %s", pageType)
	}

	return SignupPreparation{
		CSRFToken:          result.CSRFToken,
		AuthorizeURL:       result.AuthorizeURL,
		FinalURL:           result.FinalURL,
		FinalPath:          result.FinalPath,
		ContinueURL:        result.ContinueURL,
		PageType:           pageType,
		RegisterStatusCode: result.RegisterStatusCode,
		SendOTPStatusCode:  result.SendOTPStatusCode,
		Password:           password,
	}, nil
}

func defaultAuthSignupPasswordGenerator() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	for i, value := range buffer {
		buffer[i] = alphabet[int(value)%len(alphabet)]
	}

	return "Aa1!" + string(buffer), nil
}

func isDirectExistingAccountPreparation(pageType string) bool {
	switch strings.TrimSpace(pageType) {
	case "email_otp_verification", "login_password", "existing_account_detected", "user_exists":
		return true
	default:
		return false
	}
}
