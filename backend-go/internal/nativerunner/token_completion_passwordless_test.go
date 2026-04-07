package nativerunner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
)

func TestAuthPasswordlessTokenCompletionProviderCompletesCallbackFlow(t *testing.T) {
	t.Parallel()

	var (
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=provider", http.StatusFound)
		case "/u/continue":
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on continue request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on session request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:       "native@example.com",
		Strategy:    TokenCompletionStrategyPasswordless,
		CallbackURL: serverURL + "/api/auth/callback/openai?code=callback-code",
		AccountID:   "account-created",
		WorkspaceID: "workspace-created",
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.SessionToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id to be retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
	if result.Source != "login" {
		t.Fatalf("expected login source, got %q", result.Source)
	}
	if result.AuthProvider != "openai" {
		t.Fatalf("expected openai auth provider, got %q", result.AuthProvider)
	}
	if result.AccessTokenSource != "session" {
		t.Fatalf("expected session access token source, got %q", result.AccessTokenSource)
	}
	if result.SessionTokenSource != "session" {
		t.Fatalf("expected session token source, got %q", result.SessionTokenSource)
	}
}

func TestDefaultPrepareSignupFlowUsesRealPasswordlessTokenCompletionProvider(t *testing.T) {
	t.Parallel()

	var (
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=default", http.StatusFound)
		case "/u/continue":
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "__Secure-next-auth.session-token=session-cookie") {
				t.Fatalf("expected session cookie in continue request, got %q", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "__Secure-next-auth.session-token=session-cookie") {
				t.Fatalf("expected session cookie in session request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	flow := NewDefaultPrepareSignupFlow(DefaultPrepareSignupFlowOptions{
		AuthBaseURL: server.URL,
	})

	result, err := flow.tokenCompletionCoordinator.Complete(context.Background(), TokenCompletionCommand{
		Account: TokenCompletionAccount{
			Email: "native@example.com",
		},
		CallbackURL: serverURL + "/api/auth/callback/openai?code=default-code",
		AccountID:   "account-created",
		WorkspaceID: "workspace-created",
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if result.Provider.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.Provider.AccessToken)
	}
	if result.Provider.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.Provider.SessionToken)
	}
}

func TestAuthPasswordlessTokenCompletionProviderInitiatesSignupAndStopsAtEmailOTPVerification(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		rootRequests      int
		csrfRequests      int
		signinRequests    int
		authorizeRequests int
		registerRequests  int
		otpRequests       int
		serverURL         string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			rootRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected csrf accept header, got %q", got)
			}
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on csrf request: %v", err)
			}
			if cookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on csrf request, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			query := r.URL.Query()
			if got := query.Get("login_hint"); got != email {
				t.Fatalf("expected login_hint %q, got %q", email, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "authorizeid",
				Value: "authorize-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			registerRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected register referer header, got %q", got)
			}
			cookie, err := r.Cookie("authorizeid")
			if err != nil {
				t.Fatalf("expected authorize cookie on register request: %v", err)
			}
			if cookie.Value != "authorize-cookie" {
				t.Fatalf("expected authorize cookie value on register request, got %q", cookie.Value)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode register payload: %v", err)
			}
			if payload["username"] != email {
				t.Fatalf("expected register username %q, got %#v", email, payload["username"])
			}
			if strings.TrimSpace(payload["password"]) == "" {
				t.Fatal("expected generated password in register payload")
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "registerid",
				Value: "register-cookie",
				Path:  "/",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			otpRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected otp referer header, got %q", got)
			}
			registerCookie, err := r.Cookie("registerid")
			if err != nil {
				t.Fatalf("expected register cookie on otp request: %v", err)
			}
			if registerCookie.Value != "register-cookie" {
				t.Fatalf("expected register cookie value on otp request, got %q", registerCookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	_, err = provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:    email,
		Strategy: TokenCompletionStrategyPasswordless,
	})
	if err == nil {
		t.Fatal("expected interactive-step error")
	}

	typed, ok := err.(*TokenCompletionError)
	if !ok {
		t.Fatalf("expected token completion error, got %T", err)
	}
	if typed.Kind != TokenCompletionErrorKindInteractiveStepRequired {
		t.Fatalf("expected interactive_step_required kind, got %+v", typed)
	}
	if !strings.Contains(typed.Message, "email_otp_verification") {
		t.Fatalf("expected page type in error message, got %q", typed.Message)
	}
	if !strings.Contains(typed.Message, "/email-verification") {
		t.Fatalf("expected final path in error message, got %q", typed.Message)
	}

	if rootRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || registerRequests != 1 || otpRequests != 1 {
		t.Fatalf("unexpected request counts root=%d csrf=%d signin=%d authorize=%d register=%d otp=%d",
			rootRequests, csrfRequests, signinRequests, authorizeRequests, registerRequests, otpRequests)
	}
}

func TestAuthPasswordlessTokenCompletionProviderConsumesOTPAndCompletesSignupFlow(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		rootRequests        int
		csrfRequests        int
		signinRequests      int
		authorizeRequests   int
		registerRequests    int
		sendOTPRequests     int
		validateOTPRequests int
		createRequests      int
		callbackRequests    int
		continueRequests    int
		sessionRequests     int
		waitCodeCalls       int
		waitedInbox         mail.Inbox
		serverURL           string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			rootRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected csrf accept header, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if got := r.URL.Query().Get("login_hint"); got != email {
				t.Fatalf("expected login hint %q, got %q", email, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "authorizeid",
				Value: "authorize-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			registerRequests++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode register payload: %v", err)
			}
			if payload["username"] != email {
				t.Fatalf("expected register username %q, got %#v", email, payload["username"])
			}
			if strings.TrimSpace(payload["password"]) == "" {
				t.Fatal("expected generated password in register payload")
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "registerid",
				Value: "register-cookie",
				Path:  "/",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			sendOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/email-verification" {
				t.Fatalf("expected otp verify referer header, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode otp validate payload: %v", err)
			}
			if payload["code"] != "654321" {
				t.Fatalf("expected otp code 654321, got %#v", payload["code"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/about-you" {
				t.Fatalf("expected create account referer header, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create account payload: %v", err)
			}
			if strings.TrimSpace(payload["name"]) == "" {
				t.Fatal("expected generated account name")
			}
			if strings.TrimSpace(payload["birthdate"]) == "" {
				t.Fatal("expected generated birthdate")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=passwordless-code",
				"account_id":"account-created",
				"workspace_id":"workspace-created",
				"refresh_token":"refresh-created"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=passwordless", http.StatusFound)
		case "/u/continue":
			continueRequests++
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on continue request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on session request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321", calls: &waitCodeCalls, waitedInbox: &waitedInbox},
		Inbox: mail.Inbox{
			Email: email,
			Token: "inbox-token",
		},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if waitCodeCalls != 1 {
		t.Fatalf("expected wait code called once, got %d", waitCodeCalls)
	}
	if waitedInbox.OTPSentAt.IsZero() {
		t.Fatalf("expected otp sent timestamp propagated to mail provider, got %+v", waitedInbox)
	}
	if rootRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || registerRequests != 1 || sendOTPRequests != 1 || validateOTPRequests != 1 || createRequests != 1 || callbackRequests != 1 || continueRequests != 1 || sessionRequests != 1 {
		t.Fatalf("unexpected request counts root=%d csrf=%d signin=%d authorize=%d register=%d send_otp=%d validate_otp=%d create=%d callback=%d continue=%d session=%d",
			rootRequests, csrfRequests, signinRequests, authorizeRequests, registerRequests, sendOTPRequests, validateOTPRequests, createRequests, callbackRequests, continueRequests, sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-created" {
		t.Fatalf("expected refresh token from create account, got %q", result.RefreshToken)
	}
	if result.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.SessionToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id to be retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
	if result.Source != "register" {
		t.Fatalf("expected register source, got %q", result.Source)
	}
	if result.AuthProvider != "openai" {
		t.Fatalf("expected openai auth provider, got %q", result.AuthProvider)
	}
	if result.RefreshTokenSource != "create_account" {
		t.Fatalf("expected create_account refresh token source, got %q", result.RefreshTokenSource)
	}
	if result.AccessTokenSource != "session" {
		t.Fatalf("expected session access token source, got %q", result.AccessTokenSource)
	}
	if result.SessionTokenSource != "session" {
		t.Fatalf("expected session token source, got %q", result.SessionTokenSource)
	}
}

func TestAuthPasswordlessTokenCompletionProviderConsumesOTPForExistingAccountFlow(t *testing.T) {
	t.Parallel()

	const email = "existing@example.com"

	var (
		rootRequests        int
		csrfRequests        int
		signinRequests      int
		authorizeRequests   int
		validateOTPRequests int
		callbackRequests    int
		continueRequests    int
		sessionRequests     int
		waitCodeCalls       int
		waitedInbox         mail.Inbox
		serverURL           string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "existing-account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			rootRequests++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if got := r.URL.Query().Get("login_hint"); got != email {
				t.Fatalf("expected login hint %q, got %q", email, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=existing"}`))
		case "/authorize":
			authorizeRequests++
			http.Redirect(w, r, "/email-verification", http.StatusFound)
		case "/email-verification":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Email verification</body></html>`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/email-verification" {
				t.Fatalf("expected email verification referer, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/api/auth/callback/openai?code=existing-code"}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			if got := r.URL.Query().Get("code"); got != "existing-code" {
				t.Fatalf("expected callback code existing-code, got %q", got)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=provider", http.StatusFound)
		case "/u/continue":
			continueRequests++
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on continue request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected session cookie on session request: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:    email,
		Strategy: TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{
			waitCode:    "123456",
			calls:       &waitCodeCalls,
			waitedInbox: &waitedInbox,
		},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if rootRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 {
		t.Fatalf("unexpected auth bootstrap counts root=%d csrf=%d signin=%d authorize=%d",
			rootRequests, csrfRequests, signinRequests, authorizeRequests)
	}
	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validation request, got %d", validateOTPRequests)
	}
	if callbackRequests != 1 || continueRequests != 1 || sessionRequests != 1 {
		t.Fatalf("expected callback/continue/session once each, got callback=%d continue=%d session=%d",
			callbackRequests, continueRequests, sessionRequests)
	}
	if waitCodeCalls != 1 {
		t.Fatalf("expected wait code called once, got %d", waitCodeCalls)
	}
	if waitedInbox.Email != email {
		t.Fatalf("expected waited inbox email %q, got %+v", email, waitedInbox)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.SessionToken)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace from session, got %q", result.WorkspaceID)
	}
	if result.Source != "login" {
		t.Fatalf("expected login source, got %+v", result)
	}
}

func TestAuthPasswordlessTokenCompletionProviderRetriesWhenOTPCodeIsInvalidOrStale(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		validateOTPRequests int
		waitCodeCalls       int
		serverURL           string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			if validateOTPRequests == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_code","message":"The code is incorrect or expired"}}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"page_type":"about_you",
				"continue_url":"` + serverURL + `/about-you"
			}`))
		case "/api/accounts/create_account":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"account_id":"account-created",
				"workspace_id":"workspace-created",
				"refresh_token":"refresh-created",
				"callback_url":"` + serverURL + `/api/auth/callback/openai?code=callback-code",
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=callback-code",
				"page_type":"callback"
			}`))
		case "/api/auth/callback/openai":
			http.SetCookie(w, &http.Cookie{Name: "__Secure-next-auth.session-token", Value: "session-cookie", Path: "/"})
			http.Redirect(w, r, "/u/continue?state=done", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)
	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCodes: []string{"111111", "222222"}, calls: &waitCodeCalls},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if waitCodeCalls != 2 {
		t.Fatalf("expected two wait-code calls, got %d", waitCodeCalls)
	}
	if validateOTPRequests != 2 {
		t.Fatalf("expected two otp validation attempts, got %d", validateOTPRequests)
	}
	if result.AccessToken != accessToken || result.SessionToken != "session-cookie" || result.RefreshToken != "refresh-created" {
		t.Fatalf("unexpected retry result: %+v", result)
	}
}

func TestAuthPasswordlessTokenCompletionProviderReturnsTypedBoundaryWhenAboutYouLeadsToAddPhone(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		sessionRequests     int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">Add phone</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			t.Fatal("did not expect session request while passwordless flow is blocked on add_phone")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	_, err = provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err == nil {
		t.Fatal("expected interactive-step boundary")
	}

	typed, ok := err.(*TokenCompletionError)
	if !ok {
		t.Fatalf("expected token completion error, got %T", err)
	}
	if typed.Kind != TokenCompletionErrorKindInteractiveStepRequired {
		t.Fatalf("expected interactive step required, got %+v", typed)
	}
	if !strings.Contains(typed.Message, "add_phone") {
		t.Fatalf("expected add_phone in boundary message, got %q", typed.Message)
	}
	if !strings.Contains(typed.Message, "/add-phone") {
		t.Fatalf("expected /add-phone final path in boundary message, got %q", typed.Message)
	}
	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if sessionRequests != 0 {
		t.Fatalf("expected zero session requests, got %d", sessionRequests)
	}
}

func TestAuthPasswordlessTokenCompletionProviderCompletesSignupWhenAddPhoneProvidesSkipURL(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		continueRequests    int
		callbackRequests    int
		sessionRequests     int
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone"><a href="` + serverURL + `/u/continue?state=skip-phone">Skip for now</a></body></html>`))
		case "/u/continue":
			continueRequests++
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/welcome", http.StatusFound)
		case "/welcome":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Welcome</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{},
				"workspace":{"id":"workspace-from-session"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if continueRequests != 1 {
		t.Fatalf("expected one add-phone continue request, got %d", continueRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
	if result.Source != "register" {
		t.Fatalf("expected register source, got %q", result.Source)
	}
	if result.AuthProvider != "openai" {
		t.Fatalf("expected openai auth provider, got %q", result.AuthProvider)
	}
	if result.RefreshTokenSource != "session" {
		t.Fatalf("expected session refresh token source, got %q", result.RefreshTokenSource)
	}
}

func TestAuthPasswordlessTokenCompletionProviderCompletesSignupWhenAddPhoneProvidesSkipFormPost(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		formPostRequests    int
		callbackRequests    int
		sessionRequests     int
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone"><form method="post" action="` + serverURL + `/add-phone/skip"><input type="hidden" name="state" value="skip-phone"><input type="hidden" name="intent" value="skip"></form></body></html>`))
		case "/add-phone/skip":
			formPostRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /add-phone/skip, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("expected form content type on add-phone submit, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/add-phone" {
				t.Fatalf("expected add-phone referer on form submit, got %q", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse add-phone submit form: %v", err)
			}
			if got := r.Form.Get("state"); got != "skip-phone" {
				t.Fatalf("expected hidden state, got %q", got)
			}
			if got := r.Form.Get("intent"); got != "skip" {
				t.Fatalf("expected hidden intent, got %q", got)
			}
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-form-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/welcome", http.StatusFound)
		case "/welcome":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Welcome</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"authProvider":"openai",
				"user":{},
				"account":{},
				"workspace":{"id":"workspace-from-session"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if formPostRequests != 1 {
		t.Fatalf("expected one add-phone form submit, got %d", formPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
	if result.Source != "register" {
		t.Fatalf("expected register source, got %q", result.Source)
	}
	if result.AuthProvider != "openai" {
		t.Fatalf("expected openai auth provider, got %q", result.AuthProvider)
	}
	if result.RefreshTokenSource != "session" {
		t.Fatalf("expected session refresh token source, got %q", result.RefreshTokenSource)
	}
}

func TestAuthPasswordlessTokenCompletionProviderCompletesSignupWhenAddPhonePrefersSkipFormWithButtonAndCSRF(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		verifyRequests      int
		formPostRequests    int
		callbackRequests    int
		sessionRequests     int
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">
				<form id="verify-phone" method="post" action="` + serverURL + `/add-phone/verify">
					<input type="hidden" name="csrf_token" value="verify-csrf">
					<input type="hidden" name="state" value="verify-phone">
					<button type="submit" name="intent" value="verify_phone">Continue</button>
				</form>
				<form id="skip-phone" method="post" action="` + serverURL + `/add-phone/skip">
					<input type="hidden" name="csrf_token" value="skip-csrf">
					<input type="hidden" name="state" value="skip-phone">
					<input type="hidden" name="screen_hint" value="add_phone">
					<button type="submit" name="intent" value="skip_phone">Skip for now</button>
				</form>
			</body></html>`))
		case "/add-phone/verify":
			verifyRequests++
			t.Fatalf("did not expect verify form to be submitted")
		case "/add-phone/skip":
			formPostRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /add-phone/skip, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("expected form content type on add-phone submit, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/add-phone" {
				t.Fatalf("expected add-phone referer on form submit, got %q", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse add-phone submit form: %v", err)
			}
			if got := r.Form.Get("csrf_token"); got != "skip-csrf" {
				t.Fatalf("expected skip csrf token, got %q", got)
			}
			if got := r.Form.Get("state"); got != "skip-phone" {
				t.Fatalf("expected skip state, got %q", got)
			}
			if got := r.Form.Get("screen_hint"); got != "add_phone" {
				t.Fatalf("expected screen_hint, got %q", got)
			}
			if got := r.Form.Get("intent"); got != "skip_phone" {
				t.Fatalf("expected skip button value, got %q", got)
			}
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-form-priority-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/welcome", http.StatusFound)
		case "/welcome":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Welcome</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"user":{},
				"account":{},
				"workspace":{"id":"workspace-from-session"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if verifyRequests != 0 {
		t.Fatalf("expected verify form to be skipped, got %d requests", verifyRequests)
	}
	if formPostRequests != 1 {
		t.Fatalf("expected one add-phone form submit, got %d", formPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
}

func TestAuthPasswordlessTokenCompletionProviderCompletesSignupWhenAddPhoneUsesDataActionJSONPayload(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		verifyRequests      int
		formPostRequests    int
		callbackRequests    int
		sessionRequests     int
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">
				<button
					type="button"
					data-action="` + serverURL + `/add-phone/verify"
					data-method="post"
					data-payload='{"state":"verify-phone","intent":"verify_phone","csrf_token":"verify-csrf"}'
				>Verify phone</button>
				<div
					role="button"
					data-action="` + serverURL + `/add-phone/skip"
					data-method="post"
					data-payload='{"state":"skip-phone","intent":"skip_phone","screen_hint":"add_phone","csrf_token":"skip-csrf"}'
				>Skip for now</div>
			</body></html>`))
		case "/add-phone/verify":
			verifyRequests++
			t.Fatalf("did not expect verify data action to be submitted")
		case "/add-phone/skip":
			formPostRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /add-phone/skip, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("expected form content type on add-phone submit, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/add-phone" {
				t.Fatalf("expected add-phone referer on form submit, got %q", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse add-phone submit form: %v", err)
			}
			if got := r.Form.Get("csrf_token"); got != "skip-csrf" {
				t.Fatalf("expected skip csrf token, got %q", got)
			}
			if got := r.Form.Get("state"); got != "skip-phone" {
				t.Fatalf("expected skip state, got %q", got)
			}
			if got := r.Form.Get("screen_hint"); got != "add_phone" {
				t.Fatalf("expected screen_hint, got %q", got)
			}
			if got := r.Form.Get("intent"); got != "skip_phone" {
				t.Fatalf("expected skip intent, got %q", got)
			}
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-data-action-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/welcome", http.StatusFound)
		case "/welcome":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Welcome</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"user":{},
				"account":{},
				"workspace":{"id":"workspace-from-session"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if verifyRequests != 0 {
		t.Fatalf("expected verify action to be skipped, got %d requests", verifyRequests)
	}
	if formPostRequests != 1 {
		t.Fatalf("expected one add-phone data action submit, got %d", formPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
}

func TestAuthPasswordlessTokenCompletionProviderCompletesSignupWhenAddPhoneUsesStaticFetchJSONPayload(t *testing.T) {
	t.Parallel()

	const email = "native@example.com"

	var (
		serverURL           string
		validateOTPRequests int
		createRequests      int
		addPhoneRequests    int
		verifyRequests      int
		fetchPostRequests   int
		callbackRequests    int
		sessionRequests     int
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/validate":
			validateOTPRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		case "/api/accounts/create_account":
			createRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/add-phone",
				"account_id":"account-created"
			}`))
		case "/add-phone":
			addPhoneRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">
				<script>
					fetch("` + serverURL + `/add-phone/verify", {
						method: "POST",
						headers: {"Content-Type": "application/json"},
						body: JSON.stringify({"state":"verify-phone","intent":"verify_phone","csrf_token":"verify-csrf"})
					});
					fetch("` + serverURL + `/add-phone/skip", {
						method: "POST",
						headers: {"Content-Type": "application/json"},
						body: JSON.stringify({
							"state":"skip-phone",
							"intent":"skip_phone",
							"screen_hint":"add_phone",
							"csrf_token":"skip-csrf",
							"remember_me":true
						})
					});
				</script>
			</body></html>`))
		case "/add-phone/verify":
			verifyRequests++
			t.Fatalf("did not expect verify fetch branch to be submitted")
		case "/add-phone/skip":
			fetchPostRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /add-phone/skip, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("expected json content type on add-phone fetch submit, got %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode add-phone fetch payload: %v", err)
			}
			if got := payload["csrf_token"]; got != "skip-csrf" {
				t.Fatalf("expected skip csrf token, got %#v", got)
			}
			if got := payload["state"]; got != "skip-phone" {
				t.Fatalf("expected skip state, got %#v", got)
			}
			if got := payload["screen_hint"]; got != "add_phone" {
				t.Fatalf("expected screen_hint, got %#v", got)
			}
			if got := payload["intent"]; got != "skip_phone" {
				t.Fatalf("expected skip intent, got %#v", got)
			}
			if got := payload["remember_me"]; got != true {
				t.Fatalf("expected remember_me bool, got %#v", got)
			}
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-static-fetch-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/welcome", http.StatusFound)
		case "/welcome":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Welcome</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"user":{},
				"account":{},
				"workspace":{"id":"workspace-from-session"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:        email,
		Strategy:     TokenCompletionStrategyPasswordless,
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321"},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if validateOTPRequests != 1 {
		t.Fatalf("expected one otp validate request, got %d", validateOTPRequests)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if verifyRequests != 0 {
		t.Fatalf("expected verify fetch branch to be skipped, got %d requests", verifyRequests)
	}
	if fetchPostRequests != 1 {
		t.Fatalf("expected one add-phone fetch submit, got %d", fetchPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
}

func TestAuthPasswordlessTokenCompletionProviderReturnsTypedBoundaryWhenOTPConsumptionUnavailable(t *testing.T) {
	t.Parallel()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "bootstrap-cookie", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
		case "/api/accounts/user/register":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordlessTokenCompletionProvider(client)

	_, err = provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:    "native@example.com",
		Strategy: TokenCompletionStrategyPasswordless,
	})
	if err == nil {
		t.Fatal("expected typed boundary error")
	}

	typed, ok := err.(*TokenCompletionError)
	if !ok {
		t.Fatalf("expected token completion error, got %T", err)
	}
	if typed.Kind != TokenCompletionErrorKindInteractiveStepRequired {
		t.Fatalf("expected interactive step required, got %+v", typed)
	}
	if !strings.Contains(typed.Message, "mail provider") {
		t.Fatalf("expected mail provider boundary in message, got %q", typed.Message)
	}
	if !strings.Contains(typed.Message, "email_otp_verification") {
		t.Fatalf("expected page type in boundary message, got %q", typed.Message)
	}
}

func testTokenCompletionJWT(t *testing.T, payload map[string]any) string {
	t.Helper()

	headerJSON, err := json.Marshal(map[string]string{
		"alg": "none",
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("marshal jwt header: %v", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}

	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString(headerJSON),
		base64.RawURLEncoding.EncodeToString(payloadJSON),
		"signature",
	}, ".")
}

type stubPasswordlessMailProvider struct {
	waitCode    string
	waitCodes   []string
	err         error
	calls       *int
	waitedInbox *mail.Inbox
}

func (s stubPasswordlessMailProvider) Create(context.Context) (mail.Inbox, error) {
	return mail.Inbox{}, nil
}

func (s stubPasswordlessMailProvider) WaitCode(_ context.Context, inbox mail.Inbox, pattern *regexp.Regexp) (string, error) {
	if s.calls != nil {
		*s.calls++
	}
	if s.waitedInbox != nil {
		*s.waitedInbox = inbox
	}
	if strings.TrimSpace(inbox.Email) == "" {
		return "", nil
	}
	if pattern == nil {
		return "", nil
	}
	if len(s.waitCodes) != 0 {
		index := 0
		if s.calls != nil {
			index = *s.calls - 1
		}
		if index < len(s.waitCodes) {
			return s.waitCodes[index], s.err
		}
		return s.waitCodes[len(s.waitCodes)-1], s.err
	}
	return s.waitCode, s.err
}
