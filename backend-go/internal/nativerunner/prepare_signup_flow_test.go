package nativerunner

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestPrepareSignupFlowCompletesRegistrationAndBuildsPersistence(t *testing.T) {
	t.Parallel()

	var capturedEmail string
	var capturedInput FlowRequest
	var logs []string
	mailProvider := &stubPrepareSignupFlowMailProvider{code: "123456"}
	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:   200,
			FinalURL:     "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			FinalPath:    "/api/auth/callback/openai",
			PageType:     "callback",
			ContinueURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			AccountID:    "account-created",
			WorkspaceID:  "workspace-created",
			RefreshToken: "refresh-123",
		},
		continueResult: auth.ContinueCreateAccountResult{
			StatusCode:   200,
			FinalURL:     "https://auth.example.com/u/continue?state=done",
			FinalPath:    "/u/continue",
			PageType:     "continue",
			CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			AccountID:    "account-123",
			WorkspaceID:  "workspace-123",
			RefreshToken: "refresh-123",
			AccessToken:  "access-123",
			SessionToken: "session-123",
			AuthProvider: "openai",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		sessionErr: errors.New("unexpected read session call"),
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(_ context.Context, input FlowRequest) (SignupPreparer, error) {
			capturedInput = input
			return SignupPreparerFunc(func(_ context.Context, email string) (SignupPreparation, error) {
				capturedEmail = email
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(_ context.Context, input FlowRequest) (AuthPostSignupClient, error) {
			if input.MailProvider == nil {
				t.Fatal("expected flow request mail provider to be forwarded to post-signup factory")
			}
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{
				FirstName: "Teammate",
				LastName:  "Example",
				Birthdate: "1990-01-02",
			}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{TaskUUID: "task-prepare-signup"},
		MailProvider:  mailProvider,
		Inbox: mail.Inbox{
			Email: "signup@example.com",
			Token: "mail-token-1",
		},
		Logf: func(level string, message string) error {
			logs = append(logs, level+":"+message)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if capturedInput.RunnerRequest.TaskUUID != "task-prepare-signup" {
		t.Fatalf("expected flow request task uuid, got %+v", capturedInput.RunnerRequest)
	}
	if capturedEmail != "signup@example.com" {
		t.Fatalf("expected preparer email signup@example.com, got %q", capturedEmail)
	}

	if got := result.Result["success"]; got != true {
		t.Fatalf("expected success=true, got %#v", got)
	}
	if got := result.Result["stage"]; got != "completed" {
		t.Fatalf("expected stage completed, got %#v", got)
	}
	if got := result.Result["email"]; got != "signup@example.com" {
		t.Fatalf("expected email in result, got %#v", got)
	}
	if got := result.Result["account_id"]; got != "account-123" {
		t.Fatalf("expected account id in result, got %#v", got)
	}
	if got := result.Result["workspace_id"]; got != "workspace-123" {
		t.Fatalf("expected workspace id in result, got %#v", got)
	}
	if got := result.Result["access_token"]; got != "access-123" {
		t.Fatalf("expected access token in result, got %#v", got)
	}
	if got := result.Result["refresh_token"]; got != "refresh-123" {
		t.Fatalf("expected refresh token in result, got %#v", got)
	}
	if got := result.Result["session_token"]; got != "session-123" {
		t.Fatalf("expected session token in result, got %#v", got)
	}
	metadata, ok := result.Result["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", result.Result["metadata"])
	}
	if metadata["auth_provider"] != "openai" {
		t.Fatalf("expected auth_provider metadata, got %#v", metadata)
	}
	if metadata["refresh_token_source"] != "create_account" {
		t.Fatalf("expected create_account refresh token source, got %#v", metadata)
	}
	if metadata["has_session_token"] != true {
		t.Fatalf("expected has_session_token=true, got %#v", metadata)
	}
	if metadata["device_id"] != "device-123" {
		t.Fatalf("expected device_id metadata, got %#v", metadata)
	}

	inboxResult, ok := result.Result["inbox"].(map[string]any)
	if !ok {
		t.Fatalf("expected inbox map, got %#v", result.Result["inbox"])
	}
	if inboxResult["email"] != "signup@example.com" || inboxResult["token"] != "mail-token-1" {
		t.Fatalf("unexpected inbox result: %#v", inboxResult)
	}

	preparationResult, ok := result.Result["signup_preparation"].(map[string]any)
	if !ok {
		t.Fatalf("expected signup_preparation map, got %#v", result.Result["signup_preparation"])
	}
	if preparationResult["csrf_token"] != "csrf-123" {
		t.Fatalf("expected csrf token in result, got %#v", preparationResult)
	}
	if preparationResult["page_type"] != "email_otp_verification" {
		t.Fatalf("expected page type email_otp_verification, got %#v", preparationResult)
	}
	if preparationResult["final_path"] != "/email-verification" {
		t.Fatalf("expected final path /email-verification, got %#v", preparationResult)
	}
	if mailProvider.waitedInbox.Email != "signup@example.com" || mailProvider.waitedInbox.Token != "mail-token-1" {
		t.Fatalf("expected mail provider wait inbox, got %+v", mailProvider.waitedInbox)
	}
	if mailProvider.waitedInbox.OTPSentAt.IsZero() {
		t.Fatalf("expected otp sent timestamp on waited inbox, got %+v", mailProvider.waitedInbox)
	}
	if mailProvider.waitedPattern == nil || !mailProvider.waitedPattern.MatchString("123456") {
		t.Fatalf("expected default otp pattern, got %#v", mailProvider.waitedPattern)
	}
	if postSignupClient.verifyCode != "123456" {
		t.Fatalf("expected otp code forwarded to auth verifier, got %q", postSignupClient.verifyCode)
	}
	if postSignupClient.verifyPrepared.PageType != "email_otp_verification" {
		t.Fatalf("expected verify prepared result forwarded, got %#v", postSignupClient.verifyPrepared)
	}
	if postSignupClient.createPrepared.PageType != "about_you" {
		t.Fatalf("expected create-account prepared result forwarded, got %#v", postSignupClient.createPrepared)
	}
	if postSignupClient.continueCreated.CallbackURL != "https://auth.example.com/api/auth/callback/openai?code=callback-code" {
		t.Fatalf("expected continue create-account callback url, got %#v", postSignupClient.continueCreated)
	}
	if postSignupClient.continueCreated.AccountID != "account-created" || postSignupClient.continueCreated.WorkspaceID != "workspace-created" {
		t.Fatalf("expected continue step to receive raw create-account result, got %#v", postSignupClient.continueCreated)
	}
	if postSignupClient.createFirstName != "Teammate" || postSignupClient.createLastName != "Example" || postSignupClient.createBirthdate != "1990-01-02" {
		t.Fatalf("expected account profile forwarded, got %q %q %q", postSignupClient.createFirstName, postSignupClient.createLastName, postSignupClient.createBirthdate)
	}

	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if got := result.AccountPersistence; got.Email != "signup@example.com" ||
		got.EmailService != "tempmail" ||
		got.AccessToken != "access-123" ||
		got.RefreshToken != "refresh-123" ||
		got.SessionToken != "session-123" ||
		got.ClientID != "client-123" ||
		got.Status != accounts.DefaultAccountStatus ||
		got.Source != accounts.DefaultAccountSource {
		t.Fatalf("unexpected account persistence payload: %+v", got)
	}
	if result.AccountPersistence.ExtraData["refresh_token_source"] != "create_account" {
		t.Fatalf("expected create_account refresh token source in persistence extra data, got %#v", result.AccountPersistence.ExtraData)
	}
	if result.AccountPersistence.Cookies != "__Secure-next-auth.session-token=session-cookie; oai-did=device-123" {
		t.Fatalf("expected persisted auth cookies, got %#v", result.AccountPersistence.Cookies)
	}
	if result.AccountPersistence.ExtraData["device_id"] != "device-123" {
		t.Fatalf("expected device_id in persistence extra data, got %#v", result.AccountPersistence.ExtraData)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %#v", logs)
	}
	if !strings.Contains(logs[0], "prepare signup started") {
		t.Fatalf("expected start log, got %#v", logs)
	}
	if !strings.Contains(logs[1], "prepare signup completed") {
		t.Fatalf("expected completed log, got %#v", logs)
	}
}

func TestPrepareSignupFlowRejectsMissingInboxEmail(t *testing.T) {
	t.Parallel()

	factoryCalled := false
	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			factoryCalled = true
			return nil, nil
		}),
	})

	_, err := flow.Run(context.Background(), FlowRequest{
		Inbox: mail.Inbox{Token: "mail-token-1"},
	})
	if err == nil {
		t.Fatal("expected missing inbox email error")
	}
	if err.Error() != "signup inbox email is required" {
		t.Fatalf("unexpected error: %v", err)
	}
	if factoryCalled {
		t.Fatal("expected factory not called when inbox email is missing")
	}
}

func TestPrepareSignupFlowRejectsMissingSessionToken(t *testing.T) {
	t.Parallel()

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return &stubAuthPostSignupClient{
				verifyResult: auth.PrepareSignupResult{
					FinalURL:    "https://auth.example.com/about-you",
					FinalPath:   "/about-you",
					ContinueURL: "https://auth.example.com/about-you",
					PageType:    "about_you",
				},
				createResult: auth.CreateAccountResult{
					AccountID:    "account-123",
					WorkspaceID:  "workspace-123",
					RefreshToken: "refresh-123",
				},
				continueResult: auth.ContinueCreateAccountResult{
					AccessToken: "access-123",
					AccountID:   "account-123",
					WorkspaceID: "workspace-123",
				},
				sessionErr: errors.New("unexpected read session call"),
			}, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	_, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-missing-session-token",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err == nil {
		t.Fatal("expected missing session token error")
	}
	if err.Error() != "continue create account result missing session token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareSignupFlowRetriesEmailOTPWhenVerifierRequiresNewCode(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyErrs: []error{
			&auth.RetryableEmailOTPError{},
			nil,
		},
		verifyResults: []auth.PrepareSignupResult{
			{},
			{
				FinalURL:    "https://auth.example.com/about-you",
				FinalPath:   "/about-you",
				ContinueURL: "https://auth.example.com/about-you",
				PageType:    "about_you",
			},
		},
		createResult: auth.CreateAccountResult{
			FinalURL:     "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			FinalPath:    "/api/auth/callback/openai",
			PageType:     "callback",
			ContinueURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			AccountID:    "account-123",
			WorkspaceID:  "workspace-123",
			RefreshToken: "refresh-123",
		},
		continueResult: auth.ContinueCreateAccountResult{
			FinalURL:     "https://auth.example.com/u/continue?state=done",
			FinalPath:    "/u/continue",
			PageType:     "continue",
			CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
			AccountID:    "account-123",
			WorkspaceID:  "workspace-123",
			RefreshToken: "refresh-123",
			AccessToken:  "access-123",
			SessionToken: "session-123",
			AuthProvider: "openai",
		},
		sessionErr: errors.New("unexpected read session call"),
	}
	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	mailProvider := &stubPrepareSignupFlowMailProvider{codes: []string{"111111", "222222"}}
	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-retry-otp",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: mailProvider,
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if postSignupClient.verifyCalls != 2 {
		t.Fatalf("expected two otp verification attempts, got %d", postSignupClient.verifyCalls)
	}
	if postSignupClient.verifyCode != "222222" {
		t.Fatalf("expected second otp code used on retry, got %q", postSignupClient.verifyCode)
	}
	if mailProvider.waitCalls != 2 {
		t.Fatalf("expected mail provider waited twice, got %d", mailProvider.waitCalls)
	}
	if got := result.Result["access_token"]; got != "access-123" {
		t.Fatalf("expected final success after otp retry, got %#v", got)
	}
}

func TestPrepareSignupFlowUsesContinueCreateAccountWorkspaceFallback(t *testing.T) {
	t.Parallel()

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return &stubAuthPostSignupClient{
				verifyResult: auth.PrepareSignupResult{
					FinalURL:    "https://auth.example.com/about-you",
					FinalPath:   "/about-you",
					ContinueURL: "https://auth.example.com/about-you",
					PageType:    "about_you",
				},
				createResult: auth.CreateAccountResult{
					ContinueURL:  "https://auth.example.com/sign-in-with-chatgpt/codex/consent",
					PageType:     "workspace_selection",
					RefreshToken: "refresh-123",
				},
				continueResult: auth.ContinueCreateAccountResult{
					FinalURL:     "https://auth.example.com/u/continue?state=workspace",
					FinalPath:    "/u/continue",
					PageType:     "continue",
					AccountID:    "account-from-continue",
					WorkspaceID:  "workspace-from-continue",
					RefreshToken: "refresh-123",
					AccessToken:  "access-123",
					SessionToken: "session-123",
					AuthProvider: "openai",
				},
				sessionErr: errors.New("unexpected read session call"),
			}, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-workspace-fallback",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if got := result.Result["account_id"]; got != "account-from-continue" {
		t.Fatalf("expected account id from continue fallback, got %#v", got)
	}
	if got := result.Result["workspace_id"]; got != "workspace-from-continue" {
		t.Fatalf("expected workspace id from continue fallback, got %#v", got)
	}
	if got := result.Result["session_token"]; got != "session-123" {
		t.Fatalf("expected session token from continue fallback, got %#v", got)
	}
	if got := result.Result["access_token"]; got != "access-123" {
		t.Fatalf("expected access token from continue fallback, got %#v", got)
	}
}

func TestPrepareSignupFlowUsesContinueCreateAccountRefreshTokenWhenCreateAccountMissingRefreshToken(t *testing.T) {
	t.Parallel()

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return &stubAuthPostSignupClient{
				verifyResult: auth.PrepareSignupResult{
					FinalURL:    "https://auth.example.com/about-you",
					FinalPath:   "/about-you",
					ContinueURL: "https://auth.example.com/about-you",
					PageType:    "about_you",
				},
				createResult: auth.CreateAccountResult{
					FinalURL:    "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					FinalPath:   "/api/auth/callback/openai",
					PageType:    "callback",
					ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					CallbackURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					AccountID:   "account-created",
					WorkspaceID: "workspace-created",
				},
				continueResult: auth.ContinueCreateAccountResult{
					FinalURL:     "https://auth.example.com/u/continue?state=done",
					FinalPath:    "/u/continue",
					PageType:     "continue",
					CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					AccountID:    "account-123",
					WorkspaceID:  "workspace-123",
					RefreshToken: "refresh-from-continue",
					AccessToken:  "access-123",
					SessionToken: "session-123",
					AuthProvider: "openai",
				},
				sessionErr: errors.New("unexpected read session call"),
			}, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-continue-refresh-token",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if got := result.Result["refresh_token"]; got != "refresh-from-continue" {
		t.Fatalf("expected continue step refresh token, got %#v", got)
	}
	if result.AccountPersistence == nil || result.AccountPersistence.RefreshToken != "refresh-from-continue" {
		t.Fatalf("expected persistence refresh token from continue step, got %+v", result.AccountPersistence)
	}
}

func TestPrepareSignupFlowSupportsAccessTokenOnlyWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return &stubAuthPostSignupClient{
				verifyResult: auth.PrepareSignupResult{
					FinalURL:    "https://auth.example.com/about-you",
					FinalPath:   "/about-you",
					ContinueURL: "https://auth.example.com/about-you",
					PageType:    "about_you",
				},
				createResult: auth.CreateAccountResult{
					FinalURL:    "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					FinalPath:   "/api/auth/callback/openai",
					PageType:    "callback",
					ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					CallbackURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					AccountID:   "account-created",
					WorkspaceID: "workspace-created",
				},
				continueResult: auth.ContinueCreateAccountResult{
					FinalURL:     "https://auth.example.com/u/continue?state=done",
					FinalPath:    "/u/continue",
					PageType:     "continue",
					CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=callback-code",
					AccountID:    "account-123",
					WorkspaceID:  "workspace-123",
					AccessToken:  "access-123",
					SessionToken: "session-123",
					AuthProvider: "openai",
				},
				sessionErr: errors.New("unexpected read session call"),
			}, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-access-token-only",
			StartRequest: registration.StartRequest{
				EmailServiceType:        "tempmail",
				ChatGPTRegistrationMode: "access_token_only",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if got := result.Result["access_token"]; got != "access-123" {
		t.Fatalf("expected access token in result, got %#v", got)
	}
	if got := result.Result["session_token"]; got != "session-123" {
		t.Fatalf("expected session token in result, got %#v", got)
	}
	if got := result.Result["refresh_token"]; got != nil {
		t.Fatalf("expected refresh token omitted in access_token_only mode, got %#v", got)
	}
	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if result.AccountPersistence.AccessToken != "access-123" || result.AccountPersistence.SessionToken != "session-123" {
		t.Fatalf("expected persisted session/access tokens, got %+v", result.AccountPersistence)
	}
	if strings.TrimSpace(result.AccountPersistence.RefreshToken) != "" {
		t.Fatalf("expected empty persisted refresh token in access_token_only mode, got %+v", result.AccountPersistence)
	}
}

func TestPrepareSignupFlowDispatchesTokenCompletionForExistingAccountWithoutRefreshTokenOrHistoricalPassword(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:  200,
			FinalURL:    "https://auth.example.com/existing-account",
			FinalPath:   "/existing-account",
			PageType:    "existing_account_detected",
			AccountID:   "account-123",
			WorkspaceID: "workspace-123",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-123",
				RefreshToken: "refresh-123",
				SessionToken: "session-123",
				AccountID:    "account-123",
				WorkspaceID:  "workspace-123",
			},
		},
	}

	historicalPasswordLookups := 0
	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		HistoricalPasswordProvider: HistoricalPasswordProviderFunc(func(context.Context, FlowRequest, string) (string, error) {
			historicalPasswordLookups++
			return "", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-existing-account-token-completion",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if historicalPasswordLookups != 1 {
		t.Fatalf("expected one historical password lookup, got %d", historicalPasswordLookups)
	}
	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion coordinator called once, got %d", tokenCompletion.calls)
	}
	if tokenCompletion.lastCommand.Account.Email != "signup@example.com" {
		t.Fatalf("expected token completion email signup@example.com, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.Account.Password != "" {
		t.Fatalf("expected passwordless token completion command, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.MailProvider == nil {
		t.Fatalf("expected mail provider forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.Inbox.Email != "signup@example.com" || tokenCompletion.lastCommand.Inbox.Token != "mail-token-1" {
		t.Fatalf("expected inbox forwarded to token completion, got %+v", tokenCompletion.lastCommand.Inbox)
	}
	if !createAccountResultsEqual(postSignupClient.continueCreated, auth.CreateAccountResult{}) {
		t.Fatalf("expected continue create account skipped, got %+v", postSignupClient.continueCreated)
	}
	if postSignupClient.readSessionCalls != 0 {
		t.Fatalf("expected read session skipped on token completion path, got %d calls", postSignupClient.readSessionCalls)
	}

	if got := result.Result["success"]; got != true {
		t.Fatalf("expected success=true, got %#v", got)
	}
	if got := result.Result["stage"]; got != "completed" {
		t.Fatalf("expected stage completed, got %#v", got)
	}
	if got := result.Result["access_token"]; got != "access-123" {
		t.Fatalf("expected access token in result, got %#v", got)
	}
	if got := result.Result["session_token"]; got != "session-123" {
		t.Fatalf("expected session token in result, got %#v", got)
	}

	tokenCompletionResult, ok := result.Result["token_completion"].(map[string]any)
	if !ok {
		t.Fatalf("expected token_completion map, got %#v", result.Result["token_completion"])
	}
	if tokenCompletionResult["state"] != "completed" {
		t.Fatalf("expected token completion state completed, got %#v", tokenCompletionResult)
	}
	if tokenCompletionResult["strategy"] != "passwordless" {
		t.Fatalf("expected passwordless token completion strategy, got %#v", tokenCompletionResult)
	}

	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if got := result.AccountPersistence; got.Password != "" ||
		got.AccessToken != "access-123" ||
		got.RefreshToken != "refresh-123" ||
		got.SessionToken != "session-123" ||
		got.AccountID != "account-123" ||
		got.WorkspaceID != "workspace-123" {
		t.Fatalf("unexpected account persistence payload: %+v", got)
	}
	if result.AccountPersistence.Cookies != "__Secure-next-auth.session-token=session-cookie; oai-did=device-123" {
		t.Fatalf("expected persisted auth cookies, got %#v", result.AccountPersistence.Cookies)
	}
	if result.AccountPersistence.ExtraData["device_id"] != "device-123" {
		t.Fatalf("expected device_id in persistence extra data, got %#v", result.AccountPersistence.ExtraData)
	}
	runtimeState, err := ParseTokenCompletionRuntimeState(result.AccountPersistence.ExtraData, "signup@example.com")
	if err != nil {
		t.Fatalf("parse persisted token completion runtime: %v", err)
	}
	if runtimeState.CooldownUntil != nil {
		t.Fatalf("expected completed token completion to clear cooldown, got %+v", runtimeState.CooldownUntil)
	}
	if len(runtimeState.Attempts) != 1 || runtimeState.Attempts[0].State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed runtime attempt, got %+v", runtimeState.Attempts)
	}
}

func TestPrepareSignupFlowSoftCompletesWhenTokenCompletionHitsAddPhone(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateFailed,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Error: &TokenCompletionError{
				Kind:    TokenCompletionErrorKindInteractiveStepRequired,
				Message: "interactive step required (page_type=add_phone final_path=/add-phone)",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:          "csrf-123",
					AuthorizeURL:       "https://auth.example.com/authorize",
					FinalURL:           "https://auth.example.com/api/auth/callback/openai?code=existing",
					FinalPath:          "/api/auth/callback/openai",
					ContinueURL:        "https://auth.example.com/api/auth/callback/openai?code=existing",
					PageType:           "user_exists",
					RegisterStatusCode: 409,
					Password:           "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-add-phone-soft-complete",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if got := result.Result["success"]; got != true {
		t.Fatalf("expected success=true, got %#v", got)
	}
	if got := result.Result["source"]; got != "login" {
		t.Fatalf("expected login source, got %#v", got)
	}
	metadata, ok := result.Result["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", result.Result["metadata"])
	}
	if metadata["phone_verification_required"] != true || metadata["token_pending"] != true {
		t.Fatalf("expected add_phone soft completion metadata, got %#v", metadata)
	}
	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if result.AccountPersistence.Status != "token_pending" {
		t.Fatalf("expected token_pending persistence status, got %+v", result.AccountPersistence)
	}
	if result.AccountPersistence.Cookies != "__Secure-next-auth.session-token=session-cookie; oai-did=device-123" {
		t.Fatalf("expected persisted auth cookies, got %#v", result.AccountPersistence.Cookies)
	}
	if result.AccountPersistence.ExtraData["phone_verification_required"] != true {
		t.Fatalf("expected persisted phone verification flag, got %#v", result.AccountPersistence.ExtraData)
	}
}

func TestPrepareSignupFlowDispatchesTokenCompletionWhenPreparationDetectsUserExists(t *testing.T) {
	t.Parallel()

	mailProvider := &stubPrepareSignupFlowMailProvider{code: "123456"}
	postSignupClient := &stubAuthPostSignupClient{
		verifyErr:   errors.New("unexpected verify email otp call"),
		createErr:   errors.New("unexpected create account call"),
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPassword,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-from-login",
				WorkspaceID:  "workspace-from-login",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:          "csrf-123",
					AuthorizeURL:       "https://auth.example.com/authorize",
					FinalURL:           "https://auth.example.com/api/auth/callback/openai?code=existing",
					FinalPath:          "/api/auth/callback/openai",
					ContinueURL:        "https://auth.example.com/api/auth/callback/openai?code=existing",
					PageType:           "user_exists",
					RegisterStatusCode: 409,
					Password:           "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		HistoricalPasswordProvider: HistoricalPasswordProviderFunc(func(context.Context, FlowRequest, string) (string, error) {
			return "known-pass", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-register-user-exists-token-completion",
			StartRequest: registration.StartRequest{
				EmailServiceType: "outlook",
			},
		},
		MailProvider: mailProvider,
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion coordinator called once, got %d", tokenCompletion.calls)
	}
	if tokenCompletion.lastCommand.Account.Password != "known-pass" {
		t.Fatalf("expected historical password forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.MailProvider == nil {
		t.Fatalf("expected mail provider forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.Inbox.Email != "signup@example.com" || tokenCompletion.lastCommand.Inbox.Token != "mail-token-1" {
		t.Fatalf("expected inbox forwarded to token completion, got %+v", tokenCompletion.lastCommand.Inbox)
	}
	if tokenCompletion.lastCommand.PageType != "user_exists" {
		t.Fatalf("expected user_exists page type forwarded, got %+v", tokenCompletion.lastCommand)
	}
	if mailProvider.waitedPattern != nil || mailProvider.waitedInbox.Email != "" {
		t.Fatalf("expected otp wait skipped on user_exists preparation, got inbox=%+v pattern=%v", mailProvider.waitedInbox, mailProvider.waitedPattern)
	}
	if postSignupClient.verifyCode != "" {
		t.Fatalf("expected verify email otp skipped, got %q", postSignupClient.verifyCode)
	}
	if postSignupClient.createFirstName != "" || postSignupClient.createLastName != "" || postSignupClient.createBirthdate != "" {
		t.Fatalf("expected create account skipped, got %+v", postSignupClient)
	}
	if got := result.Result["source"]; got != "login" {
		t.Fatalf("expected login source in result, got %#v", got)
	}
}

func TestPrepareSignupFlowSupportsExistingAccountAccessTokenOnlyWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyErr:   errors.New("unexpected verify email otp call"),
		createErr:   errors.New("unexpected create account call"),
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-only",
				SessionToken: "session-only",
				AccountID:    "account-only",
				WorkspaceID:  "workspace-only",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:          "csrf-123",
					AuthorizeURL:       "https://auth.example.com/authorize",
					FinalURL:           "https://auth.example.com/api/auth/callback/openai?code=existing",
					FinalPath:          "/api/auth/callback/openai",
					ContinueURL:        "https://auth.example.com/api/auth/callback/openai?code=existing",
					PageType:           "user_exists",
					RegisterStatusCode: 409,
					Password:           "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-existing-access-token-only",
			StartRequest: registration.StartRequest{
				EmailServiceType:        "tempmail",
				ChatGPTRegistrationMode: "access_token_only",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if got := result.Result["access_token"]; got != "access-only" {
		t.Fatalf("expected access token, got %#v", got)
	}
	if got := result.Result["session_token"]; got != "session-only" {
		t.Fatalf("expected session token, got %#v", got)
	}
	if got := result.Result["refresh_token"]; got != nil {
		t.Fatalf("expected no refresh token in access_token_only existing-account flow, got %#v", got)
	}
	metadata, ok := result.Result["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", result.Result["metadata"])
	}
	if metadata["chatgpt_registration_mode"] != "access_token_only" {
		t.Fatalf("expected access_token_only metadata, got %#v", metadata)
	}
	if metadata["refresh_token_skipped"] != true {
		t.Fatalf("expected refresh_token_skipped metadata, got %#v", metadata)
	}
	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if strings.TrimSpace(result.AccountPersistence.RefreshToken) != "" {
		t.Fatalf("expected empty persisted refresh token, got %+v", result.AccountPersistence)
	}
	if result.AccountPersistence.ExtraData["chatgpt_registration_mode"] != "access_token_only" {
		t.Fatalf("expected persisted registration mode, got %#v", result.AccountPersistence.ExtraData)
	}
}

func TestPrepareSignupFlowDispatchesTokenCompletionWhenEmailOTPVerificationLandsOnWorkspaceSelection(t *testing.T) {
	t.Parallel()

	mailProvider := &stubPrepareSignupFlowMailProvider{code: "123456"}
	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/workspace",
			FinalPath:    "/workspace",
			ContinueURL:  "https://auth.example.com/workspace",
			PageType:     "workspace_selection",
		},
		createErr:   errors.New("unexpected create account call"),
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPassword,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-from-login",
				WorkspaceID:  "workspace-from-login",
			},
		},
	}

	historicalPasswordLookups := 0
	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		HistoricalPasswordProvider: HistoricalPasswordProviderFunc(func(context.Context, FlowRequest, string) (string, error) {
			historicalPasswordLookups++
			return "known-pass", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-email-otp-workspace-token-completion",
			StartRequest: registration.StartRequest{
				EmailServiceType: "outlook",
			},
		},
		MailProvider: mailProvider,
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if historicalPasswordLookups != 1 {
		t.Fatalf("expected one historical password lookup, got %d", historicalPasswordLookups)
	}
	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion coordinator called once, got %d", tokenCompletion.calls)
	}
	if tokenCompletion.lastCommand.Account.Password != "known-pass" {
		t.Fatalf("expected historical password forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.PageType != "workspace_selection" {
		t.Fatalf("expected workspace_selection page type forwarded, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.ContinueURL != "https://auth.example.com/workspace" {
		t.Fatalf("expected workspace continue url forwarded, got %+v", tokenCompletion.lastCommand)
	}
	if postSignupClient.verifyCode != "123456" {
		t.Fatalf("expected verify email otp to consume mailbox code, got %q", postSignupClient.verifyCode)
	}
	if postSignupClient.createFirstName != "" || postSignupClient.createLastName != "" || postSignupClient.createBirthdate != "" {
		t.Fatalf("expected create account skipped, got %+v", postSignupClient)
	}
	if got := result.Result["source"]; got != "login" {
		t.Fatalf("expected login source in result, got %#v", got)
	}
}

func TestPrepareSignupFlowDispatchesTokenCompletionWhenPreparationDetectsExistingAccount(t *testing.T) {
	t.Parallel()

	mailProvider := &stubPrepareSignupFlowMailProvider{code: "123456"}
	postSignupClient := &stubAuthPostSignupClient{
		verifyErr:   errors.New("unexpected verify email otp call"),
		createErr:   errors.New("unexpected create account call"),
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-from-login",
				WorkspaceID:  "workspace-from-login",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:          "csrf-123",
					AuthorizeURL:       "https://auth.example.com/authorize",
					FinalURL:           "https://auth.example.com/existing-account",
					FinalPath:          "/existing-account",
					ContinueURL:        "https://auth.example.com/api/auth/callback/openai?code=existing-account",
					PageType:           "existing_account_detected",
					RegisterStatusCode: 409,
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-register-existing-account-token-completion",
			StartRequest: registration.StartRequest{
				EmailServiceType: "outlook",
			},
		},
		MailProvider: mailProvider,
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion coordinator called once, got %d", tokenCompletion.calls)
	}
	if tokenCompletion.lastCommand.MailProvider == nil {
		t.Fatalf("expected mail provider forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.Inbox.Email != "signup@example.com" || tokenCompletion.lastCommand.Inbox.Token != "mail-token-1" {
		t.Fatalf("expected inbox forwarded to token completion, got %+v", tokenCompletion.lastCommand.Inbox)
	}
	if tokenCompletion.lastCommand.PageType != "existing_account_detected" {
		t.Fatalf("expected existing_account_detected page type forwarded, got %+v", tokenCompletion.lastCommand)
	}
	if mailProvider.waitedPattern != nil || mailProvider.waitedInbox.Email != "" {
		t.Fatalf("expected otp wait skipped on existing_account_detected preparation, got inbox=%+v pattern=%v", mailProvider.waitedInbox, mailProvider.waitedPattern)
	}
	if postSignupClient.verifyCode != "" {
		t.Fatalf("expected verify email otp skipped, got %q", postSignupClient.verifyCode)
	}
	if postSignupClient.createFirstName != "" || postSignupClient.createLastName != "" || postSignupClient.createBirthdate != "" {
		t.Fatalf("expected create account skipped, got %+v", postSignupClient)
	}
	if got := result.Result["source"]; got != "login" {
		t.Fatalf("expected login source in result, got %#v", got)
	}
}

func TestPrepareSignupFlowDispatchesPasswordTokenCompletionForUserExistsWithHistoricalPassword(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:   200,
			FinalURL:     "https://auth.example.com/user-exists",
			FinalPath:    "/user-exists",
			PageType:     "user_exists",
			ContinueURL:  "https://auth.example.com/api/auth/callback/openai?code=user-exists",
			CallbackURL:  "https://auth.example.com/api/auth/callback/openai?code=user-exists",
			AccountID:    "account-created",
			WorkspaceID:  "workspace-created",
			RefreshToken: "",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPassword,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-from-login",
				WorkspaceID:  "workspace-from-login",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		HistoricalPasswordProvider: HistoricalPasswordProviderFunc(func(context.Context, FlowRequest, string) (string, error) {
			return "known-pass", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-user-exists-token-completion",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion coordinator called once, got %d", tokenCompletion.calls)
	}
	if tokenCompletion.lastCommand.Account.Password != "known-pass" {
		t.Fatalf("expected historical password forwarded to token completion, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.PageType != "user_exists" {
		t.Fatalf("expected user_exists page type forwarded, got %+v", tokenCompletion.lastCommand)
	}

	if got := result.Result["source"]; got != "login" {
		t.Fatalf("expected login source in result, got %#v", got)
	}
	if got := result.Result["refresh_token"]; got != "refresh-from-login" {
		t.Fatalf("expected refresh token in result, got %#v", got)
	}
	metadata, ok := result.Result["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map in result, got %#v", result.Result["metadata"])
	}
	if metadata["existing_account_detected"] != true {
		t.Fatalf("expected existing_account_detected metadata, got %#v", metadata)
	}
	if metadata["refresh_token_source"] != "oauth_password" {
		t.Fatalf("expected oauth_password refresh token source, got %#v", metadata)
	}
	if metadata["device_id"] != "device-123" {
		t.Fatalf("expected device_id metadata, got %#v", metadata)
	}

	if result.AccountPersistence == nil {
		t.Fatal("expected account persistence payload")
	}
	if result.AccountPersistence.Password != "known-pass" {
		t.Fatalf("expected persistence password from historical password, got %+v", result.AccountPersistence)
	}
	if result.AccountPersistence.Source != "login" {
		t.Fatalf("expected login source for existing-account persistence, got %+v", result.AccountPersistence)
	}
	if result.AccountPersistence.ExtraData["existing_account_detected"] != true {
		t.Fatalf("expected persistence existing_account_detected extra data, got %#v", result.AccountPersistence.ExtraData)
	}
	if result.AccountPersistence.ExtraData["token_completion_mode"] != "password" {
		t.Fatalf("expected password token completion mode in persistence extra data, got %#v", result.AccountPersistence.ExtraData)
	}
	if result.AccountPersistence.Cookies != "__Secure-next-auth.session-token=session-cookie; oai-did=device-123" {
		t.Fatalf("expected persisted auth cookies, got %#v", result.AccountPersistence.Cookies)
	}
	runtimeState, err := ParseTokenCompletionRuntimeState(result.AccountPersistence.ExtraData, "signup@example.com")
	if err != nil {
		t.Fatalf("parse persisted token completion runtime: %v", err)
	}
	if len(runtimeState.Attempts) != 1 || runtimeState.Attempts[0].State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed runtime attempt, got %+v", runtimeState.Attempts)
	}
}

func TestPrepareSignupFlowContinuesWhenCreateAccountReturnsTypedUserExistsError(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:  409,
			FinalURL:    "https://auth.example.com/user-exists",
			FinalPath:   "/user-exists",
			PageType:    "user_exists",
			ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=user-exists",
			CallbackURL: "https://auth.example.com/api/auth/callback/openai?code=user-exists",
			AccountID:   "account-created",
			WorkspaceID: "workspace-created",
		},
		createErr: &auth.CreateAccountUserExistsError{
			StatusCode: 409,
			Code:       "user_exists",
			Message:    "Email already exists",
			Result: auth.CreateAccountResult{
				StatusCode:  409,
				PageType:    "user_exists",
				ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=user-exists",
				CallbackURL: "https://auth.example.com/api/auth/callback/openai?code=user-exists",
				AccountID:   "account-created",
				WorkspaceID: "workspace-created",
			},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-created",
				WorkspaceID:  "workspace-created",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	result, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-user-exists-typed-error",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}

	if tokenCompletion.calls != 1 {
		t.Fatalf("expected token completion to proceed after typed user_exists error, got %d calls", tokenCompletion.calls)
	}
	if got := result.Result["refresh_token"]; got != "refresh-from-login" {
		t.Fatalf("expected token-completion refresh token, got %#v", got)
	}
}

func TestPrepareSignupFlowPassesStoredCooldownIntoTokenCompletionCommand(t *testing.T) {
	t.Parallel()

	cooldownUntil := time.Date(2026, time.April, 5, 10, 7, 0, 0, time.UTC)
	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:  200,
			FinalURL:    "https://auth.example.com/existing-account",
			FinalPath:   "/existing-account",
			PageType:    "existing_account_detected",
			AccountID:   "account-123",
			WorkspaceID: "workspace-123",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State: TokenCompletionStateBlocked,
			Email: "signup@example.com",
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindCooldownActive,
				Message:   "token completion cooldown active",
				Retryable: true,
			},
			NextEligibleAt: &cooldownUntil,
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCooldownProvider: TokenCompletionCooldownProviderFunc(func(context.Context, FlowRequest, string) (*time.Time, error) {
			return &cooldownUntil, nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	_, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-existing-account-cooldown",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	var persistenceErr *tokenCompletionPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("expected tokenCompletionPersistenceError, got %v", err)
	}
	if tokenCompletion.lastCommand.CooldownUntil == nil || !tokenCompletion.lastCommand.CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("expected cooldown_until forwarded into token completion command, got %+v", tokenCompletion.lastCommand)
	}
	if persistenceErr.AccountPersistenceRequest() == nil {
		t.Fatal("expected account persistence carrier for blocked token completion")
	}
	runtimeState, err := ParseTokenCompletionRuntimeState(persistenceErr.AccountPersistenceRequest().ExtraData, "signup@example.com")
	if err != nil {
		t.Fatalf("parse persisted token completion runtime: %v", err)
	}
	if runtimeState.CooldownUntil == nil || !runtimeState.CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("expected blocked cooldown persisted into runtime, got %+v", runtimeState.CooldownUntil)
	}
	if len(runtimeState.Attempts) != 0 {
		t.Fatalf("expected blocked result not to append attempts, got %+v", runtimeState.Attempts)
	}
}

func TestPrepareSignupFlowPassesPersistedRuntimeIntoTokenCompletionCommand(t *testing.T) {
	t.Parallel()

	cooldownUntil := time.Date(2026, time.April, 5, 10, 7, 0, 0, time.UTC)
	persistedAttempts := []TokenCompletionAttempt{
		{
			Email:       "signup@example.com",
			State:       TokenCompletionStateFailed,
			CompletedAt: time.Date(2026, time.April, 5, 9, 59, 0, 0, time.UTC),
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Message:   "temporary outage",
				Retryable: true,
			},
		},
	}
	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:  200,
			FinalURL:    "https://auth.example.com/existing-account",
			FinalPath:   "/existing-account",
			PageType:    "existing_account_detected",
			AccountID:   "account-123",
			WorkspaceID: "workspace-123",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State:    TokenCompletionStateCompleted,
			Email:    "signup@example.com",
			Strategy: TokenCompletionStrategyPasswordless,
			Provider: TokenCompletionProviderResult{
				AccessToken:  "access-from-login",
				RefreshToken: "refresh-from-login",
				SessionToken: "session-from-login",
				AccountID:    "account-123",
				WorkspaceID:  "workspace-123",
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCooldownProvider: TokenCompletionCooldownProviderFunc(func(context.Context, FlowRequest, string) (*time.Time, error) {
			return &cooldownUntil, nil
		}),
		TokenCompletionAttemptProvider: TokenCompletionAttemptProviderFunc(func(context.Context, FlowRequest, string) ([]TokenCompletionAttempt, error) {
			return persistedAttempts, nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	_, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-existing-account-runtime",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	if err != nil {
		t.Fatalf("run flow: %v", err)
	}
	if len(tokenCompletion.lastCommand.Attempts) != 1 {
		t.Fatalf("expected persisted attempts forwarded to token completion command, got %+v", tokenCompletion.lastCommand)
	}
	if tokenCompletion.lastCommand.Attempts[0].State != TokenCompletionStateFailed {
		t.Fatalf("expected failed attempt forwarded to token completion command, got %+v", tokenCompletion.lastCommand.Attempts)
	}
	if tokenCompletion.lastCommand.CooldownUntil == nil || !tokenCompletion.lastCommand.CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("expected persisted cooldown forwarded to token completion command, got %+v", tokenCompletion.lastCommand)
	}
}

func TestPrepareSignupFlowPersistsRuntimeWhenTokenCompletionFails(t *testing.T) {
	t.Parallel()

	postSignupClient := &stubAuthPostSignupClient{
		verifyResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/about-you",
			FinalPath:    "/about-you",
			ContinueURL:  "https://auth.example.com/about-you",
			PageType:     "about_you",
		},
		createResult: auth.CreateAccountResult{
			StatusCode:  200,
			FinalURL:    "https://auth.example.com/existing-account",
			FinalPath:   "/existing-account",
			PageType:    "existing_account_detected",
			AccountID:   "account-123",
			WorkspaceID: "workspace-123",
		},
		cookies: []*http.Cookie{
			{Name: "__Secure-next-auth.session-token", Value: "session-cookie"},
			{Name: "oai-did", Value: "device-123"},
		},
		continueErr: errors.New("unexpected continue create account call"),
		sessionErr:  errors.New("unexpected read session call"),
	}
	tokenCompletion := &stubPrepareSignupFlowTokenCompletionCoordinator{
		result: TokenCompletionResult{
			State: TokenCompletionStateFailed,
			Email: "signup@example.com",
			Error: &TokenCompletionError{
				Kind:      TokenCompletionErrorKindProviderUnavailable,
				Message:   "temporary outage",
				Retryable: true,
			},
		},
	}

	flow := NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(context.Context, FlowRequest) (SignupPreparer, error) {
			return SignupPreparerFunc(func(context.Context, string) (SignupPreparation, error) {
				return SignupPreparation{
					CSRFToken:    "csrf-123",
					AuthorizeURL: "https://auth.example.com/authorize",
					FinalURL:     "https://auth.example.com/email-verification",
					FinalPath:    "/email-verification",
					PageType:     "email_otp_verification",
					Password:     "Password123!",
				}, nil
			}), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(context.Context, FlowRequest) (AuthPostSignupClient, error) {
			return postSignupClient, nil
		}),
		AccountProfileProvider: AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return AccountProfile{FirstName: "Teammate", LastName: "Example", Birthdate: "1990-01-02"}, nil
		}),
		ClientIDResolver: ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return "client-123", nil
		}),
		TokenCompletionCoordinator: tokenCompletion,
	})

	_, err := flow.Run(context.Background(), FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			TaskUUID: "task-existing-account-failed-runtime",
			StartRequest: registration.StartRequest{
				EmailServiceType: "tempmail",
			},
		},
		MailProvider: &stubPrepareSignupFlowMailProvider{code: "123456"},
		Inbox:        mail.Inbox{Email: "signup@example.com", Token: "mail-token-1"},
	})
	var persistenceErr *tokenCompletionPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("expected tokenCompletionPersistenceError, got %v", err)
	}
	if persistenceErr.AccountPersistenceRequest() == nil {
		t.Fatal("expected account persistence carrier for failed token completion")
	}
	if persistenceErr.AccountPersistenceRequest().Status != "token_pending" {
		t.Fatalf("expected token_pending status for retryable failed token completion, got %+v", persistenceErr.AccountPersistenceRequest())
	}
	if persistenceErr.AccountPersistenceRequest().Cookies != "__Secure-next-auth.session-token=session-cookie; oai-did=device-123" {
		t.Fatalf("expected persisted auth cookies on failure, got %#v", persistenceErr.AccountPersistenceRequest().Cookies)
	}
	if persistenceErr.AccountPersistenceRequest().ExtraData["device_id"] != "device-123" {
		t.Fatalf("expected device_id in failure persistence extra data, got %#v", persistenceErr.AccountPersistenceRequest().ExtraData)
	}
	runtimeState, err := ParseTokenCompletionRuntimeState(persistenceErr.AccountPersistenceRequest().ExtraData, "signup@example.com")
	if err != nil {
		t.Fatalf("parse persisted token completion runtime: %v", err)
	}
	if len(runtimeState.Attempts) != 1 || runtimeState.Attempts[0].State != TokenCompletionStateFailed {
		t.Fatalf("expected failed attempt persisted into runtime, got %+v", runtimeState.Attempts)
	}
}

func TestAuthSignupPreparerChainsPrepareSignupAndSendOTP(t *testing.T) {
	t.Parallel()

	client := &stubAuthPrepareSignupClient{
		prepareResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/create-account/password",
			FinalPath:    "/create-account/password",
			PageType:     "create_account_password",
		},
		registerResult: auth.PrepareSignupResult{
			CSRFToken:          "csrf-123",
			AuthorizeURL:       "https://auth.example.com/authorize",
			FinalURL:           "https://auth.example.com/email-verification",
			FinalPath:          "/email-verification",
			PageType:           "email_otp_verification",
			RegisterStatusCode: 200,
			SendOTPStatusCode:  200,
		},
	}
	preparer := NewAuthSignupPreparer(client, WithAuthSignupPasswordGenerator(func() (string, error) {
		return "Password123!", nil
	}))

	result, err := preparer.PrepareSignup(context.Background(), "signup@example.com")
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if client.prepareEmail != "signup@example.com" {
		t.Fatalf("expected prepare signup email, got %q", client.prepareEmail)
	}
	if client.registerEmail != "signup@example.com" {
		t.Fatalf("expected register email, got %q", client.registerEmail)
	}
	if client.registerPassword != "Password123!" {
		t.Fatalf("expected generated password to be forwarded, got %q", client.registerPassword)
	}
	if client.registerPrepared.PageType != "create_account_password" {
		t.Fatalf("expected prepared signup result forwarded to register step, got %#v", client.registerPrepared)
	}
	if result.CSRFToken != "csrf-123" {
		t.Fatalf("expected csrf token, got %#v", result)
	}
	if result.PageType != "email_otp_verification" || result.FinalPath != "/email-verification" {
		t.Fatalf("unexpected mapped result: %#v", result)
	}
	if result.RegisterStatusCode != 200 || result.SendOTPStatusCode != 200 {
		t.Fatalf("unexpected mapped result: %#v", result)
	}
}

func TestAuthSignupPreparerAllowsExistingAccountPageTypeFromRegisterStep(t *testing.T) {
	t.Parallel()

	client := &stubAuthPrepareSignupClient{
		prepareResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/create-account/password",
			FinalPath:    "/create-account/password",
			PageType:     "create_account_password",
		},
		registerResult: auth.PrepareSignupResult{
			CSRFToken:          "csrf-123",
			AuthorizeURL:       "https://auth.example.com/authorize",
			FinalURL:           "https://auth.example.com/api/auth/callback/openai?code=existing",
			FinalPath:          "/api/auth/callback/openai",
			ContinueURL:        "https://auth.example.com/api/auth/callback/openai?code=existing",
			PageType:           "user_exists",
			RegisterStatusCode: 409,
		},
	}
	preparer := NewAuthSignupPreparer(client, WithAuthSignupPasswordGenerator(func() (string, error) {
		return "Password123!", nil
	}))

	result, err := preparer.PrepareSignup(context.Background(), "signup@example.com")
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if result.PageType != "user_exists" {
		t.Fatalf("expected user_exists page type, got %#v", result)
	}
	if result.RegisterStatusCode != 409 {
		t.Fatalf("expected register status 409, got %#v", result)
	}
	if result.ContinueURL != "https://auth.example.com/api/auth/callback/openai?code=existing" {
		t.Fatalf("expected continue url to round-trip, got %#v", result)
	}
}

func TestAuthSignupPreparerTreatsDirectEmailOTPStateAsExistingAccount(t *testing.T) {
	t.Parallel()

	client := &stubAuthPrepareSignupClient{
		prepareResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/email-verification",
			FinalPath:    "/email-verification",
			PageType:     "email_otp_verification",
		},
	}
	preparer := NewAuthSignupPreparer(client, WithAuthSignupPasswordGenerator(func() (string, error) {
		return "Password123!", nil
	}))

	result, err := preparer.PrepareSignup(context.Background(), "signup@example.com")
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if client.registerEmail != "" || client.registerPassword != "" {
		t.Fatalf("expected register step skipped for direct email otp state, got %+v", client)
	}
	if result.PageType != "existing_account_detected" {
		t.Fatalf("expected existing_account_detected page type, got %#v", result)
	}
	if result.FinalPath != "/email-verification" {
		t.Fatalf("expected final path to round-trip, got %#v", result)
	}
}

func TestAuthSignupPreparerReturnsPreparedUserExistsWithoutRegistering(t *testing.T) {
	t.Parallel()

	passwordGeneratorCalls := 0
	client := &stubAuthPrepareSignupClient{
		prepareResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/api/auth/callback/openai?code=existing",
			FinalPath:    "/api/auth/callback/openai",
			ContinueURL:  "https://auth.example.com/api/auth/callback/openai?code=existing",
			PageType:     "user_exists",
		},
		registerErr: errors.New("unexpected register call"),
	}
	preparer := NewAuthSignupPreparer(client, WithAuthSignupPasswordGenerator(func() (string, error) {
		passwordGeneratorCalls++
		return "Password123!", nil
	}))

	result, err := preparer.PrepareSignup(context.Background(), "signup@example.com")
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if client.prepareEmail != "signup@example.com" {
		t.Fatalf("expected prepare signup email, got %q", client.prepareEmail)
	}
	if client.registerEmail != "" || client.registerPassword != "" {
		t.Fatalf("expected register step skipped for user_exists, got registerEmail=%q password=%q", client.registerEmail, client.registerPassword)
	}
	if passwordGeneratorCalls != 0 {
		t.Fatalf("expected password generator not called for user_exists, got %d calls", passwordGeneratorCalls)
	}
	if result.PageType != "existing_account_detected" {
		t.Fatalf("expected existing_account_detected page type, got %#v", result)
	}
	if result.ContinueURL != "https://auth.example.com/api/auth/callback/openai?code=existing" {
		t.Fatalf("expected continue url preserved, got %#v", result)
	}
	if result.Password != "" {
		t.Fatalf("expected no generated password when register is skipped, got %#v", result)
	}
}

func TestAuthSignupPreparerIncludesContextWhenPreparedStageUnknown(t *testing.T) {
	t.Parallel()

	client := &stubAuthPrepareSignupClient{
		prepareResult: auth.PrepareSignupResult{
			CSRFToken:    "csrf-123",
			AuthorizeURL: "https://auth.example.com/authorize",
			FinalURL:     "https://auth.example.com/u/continue?state=opaque",
			FinalPath:    "/u/continue",
			PageType:     "",
		},
	}
	preparer := NewAuthSignupPreparer(client, WithAuthSignupPasswordGenerator(func() (string, error) {
		return "Password123!", nil
	}))

	_, err := preparer.PrepareSignup(context.Background(), "signup@example.com")
	if err == nil {
		t.Fatal("expected unknown signup stage error")
	}
	if !strings.Contains(err.Error(), "page_type=") {
		t.Fatalf("expected page_type in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "final_url=https://auth.example.com/u/continue?state=opaque") {
		t.Fatalf("expected final_url in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "final_path=/u/continue") {
		t.Fatalf("expected final_path in error, got %v", err)
	}
}

func TestAuthSignupPreparerRejectsNilClient(t *testing.T) {
	t.Parallel()

	preparer := NewAuthSignupPreparer(nil)
	if preparer != nil {
		t.Fatalf("expected nil preparer for nil client, got %#v", preparer)
	}
}

type stubAuthPrepareSignupClient struct {
	prepareResult    auth.PrepareSignupResult
	registerResult   auth.PrepareSignupResult
	err              error
	prepareErr       error
	registerErr      error
	prepareEmail     string
	registerPrepared auth.PrepareSignupResult
	registerEmail    string
	registerPassword string
}

func (s *stubAuthPrepareSignupClient) PrepareSignup(_ context.Context, email string) (auth.PrepareSignupResult, error) {
	if s.prepareErr != nil {
		return auth.PrepareSignupResult{}, s.prepareErr
	}
	if s.err != nil {
		return auth.PrepareSignupResult{}, s.err
	}
	s.prepareEmail = email
	return s.prepareResult, nil
}

func (s *stubAuthPrepareSignupClient) RegisterPasswordAndSendOTP(_ context.Context, prepared auth.PrepareSignupResult, email string, password string) (auth.PrepareSignupResult, error) {
	if s.registerErr != nil {
		return auth.PrepareSignupResult{}, s.registerErr
	}
	if s.err != nil {
		return auth.PrepareSignupResult{}, s.err
	}
	s.registerPrepared = prepared
	s.registerEmail = email
	s.registerPassword = password
	return s.registerResult, nil
}

type stubPrepareSignupFlowMailProvider struct {
	code          string
	codes         []string
	waitCalls     int
	waitedInbox   mail.Inbox
	waitedPattern *regexp.Regexp
	err           error
}

func (s *stubPrepareSignupFlowMailProvider) Create(context.Context) (mail.Inbox, error) {
	return mail.Inbox{}, errors.New("unexpected create call")
}

func (s *stubPrepareSignupFlowMailProvider) WaitCode(_ context.Context, inbox mail.Inbox, pattern *regexp.Regexp) (string, error) {
	s.waitCalls++
	s.waitedInbox = inbox
	s.waitedPattern = pattern
	if s.err != nil {
		return "", s.err
	}
	if len(s.codes) != 0 {
		index := s.waitCalls - 1
		if index < len(s.codes) {
			return s.codes[index], nil
		}
		return s.codes[len(s.codes)-1], nil
	}
	return s.code, nil
}

type stubAuthPostSignupClient struct {
	verifyPrepared   auth.PrepareSignupResult
	verifyCode       string
	verifyResult     auth.PrepareSignupResult
	verifyErr        error
	verifyResults    []auth.PrepareSignupResult
	verifyErrs       []error
	verifyCalls      int
	createPrepared   auth.PrepareSignupResult
	createFirstName  string
	createLastName   string
	createBirthdate  string
	createResult     auth.CreateAccountResult
	createErr        error
	continueCreated  auth.CreateAccountResult
	continueResult   auth.ContinueCreateAccountResult
	continueErr      error
	sessionResult    auth.SessionResult
	sessionErr       error
	readSessionCalls int
	cookies          []*http.Cookie
}

func (s *stubAuthPostSignupClient) VerifyEmailOTP(_ context.Context, prepared auth.PrepareSignupResult, code string) (auth.PrepareSignupResult, error) {
	s.verifyPrepared = prepared
	s.verifyCode = code
	s.verifyCalls++
	if len(s.verifyErrs) != 0 || len(s.verifyResults) != 0 {
		index := s.verifyCalls - 1
		if index < len(s.verifyErrs) && s.verifyErrs[index] != nil {
			return auth.PrepareSignupResult{}, s.verifyErrs[index]
		}
		if index < len(s.verifyResults) {
			return s.verifyResults[index], nil
		}
	}
	if s.verifyErr != nil {
		return auth.PrepareSignupResult{}, s.verifyErr
	}
	return s.verifyResult, nil
}

func (s *stubAuthPostSignupClient) CreateAccount(_ context.Context, prepared auth.PrepareSignupResult, firstName string, lastName string, birthdate string) (auth.CreateAccountResult, error) {
	s.createPrepared = prepared
	s.createFirstName = firstName
	s.createLastName = lastName
	s.createBirthdate = birthdate
	if s.createErr != nil {
		return auth.CreateAccountResult{}, s.createErr
	}
	return s.createResult, nil
}

func (s *stubAuthPostSignupClient) ContinueCreateAccount(_ context.Context, created auth.CreateAccountResult) (auth.ContinueCreateAccountResult, error) {
	s.continueCreated = created
	if s.continueErr != nil {
		return auth.ContinueCreateAccountResult{}, s.continueErr
	}
	return s.continueResult, nil
}

func (s *stubAuthPostSignupClient) ReadSession(context.Context) (auth.SessionResult, error) {
	s.readSessionCalls++
	if s.sessionErr != nil {
		return auth.SessionResult{}, s.sessionErr
	}
	return s.sessionResult, nil
}

func (s *stubAuthPostSignupClient) Cookies() []*http.Cookie {
	return s.cookies
}

type stubPrepareSignupFlowTokenCompletionCoordinator struct {
	result      TokenCompletionResult
	err         error
	calls       int
	lastCommand TokenCompletionCommand
}

func (s *stubPrepareSignupFlowTokenCompletionCoordinator) Complete(_ context.Context, command TokenCompletionCommand) (TokenCompletionResult, error) {
	s.calls++
	s.lastCommand = command
	if s.err != nil {
		return TokenCompletionResult{}, s.err
	}
	return s.result, nil
}

func createAccountResultsEqual(left auth.CreateAccountResult, right auth.CreateAccountResult) bool {
	return left.StatusCode == right.StatusCode &&
		left.FinalURL == right.FinalURL &&
		left.FinalPath == right.FinalPath &&
		left.PageType == right.PageType &&
		left.ContinueURL == right.ContinueURL &&
		left.CallbackURL == right.CallbackURL &&
		left.AccountID == right.AccountID &&
		left.WorkspaceID == right.WorkspaceID &&
		left.RefreshToken == right.RefreshToken
}
