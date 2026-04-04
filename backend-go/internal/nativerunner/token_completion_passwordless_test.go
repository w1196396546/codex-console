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
	if result.AccountID != "account-from-jwt" {
		t.Fatalf("expected account id from session, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
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
		case "/":
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
		serverURL           string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
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
		MailProvider: stubPasswordlessMailProvider{waitCode: "654321", calls: &waitCodeCalls},
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
	if result.AccountID != "account-from-jwt" {
		t.Fatalf("expected account id from session, got %q", result.AccountID)
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
		case "/":
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
	waitCode string
	err      error
	calls    *int
}

func (s stubPasswordlessMailProvider) Create(context.Context) (mail.Inbox, error) {
	return mail.Inbox{}, nil
}

func (s stubPasswordlessMailProvider) WaitCode(_ context.Context, inbox mail.Inbox, pattern *regexp.Regexp) (string, error) {
	if s.calls != nil {
		*s.calls++
	}
	if strings.TrimSpace(inbox.Email) == "" {
		return "", nil
	}
	if pattern == nil {
		return "", nil
	}
	return s.waitCode, s.err
}
