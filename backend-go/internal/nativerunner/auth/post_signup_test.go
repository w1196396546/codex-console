package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientVerifyEmailOTPAdvancesToAboutYouState(t *testing.T) {
	t.Parallel()

	const otpCode = "123456"

	var verifyRequests int
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/email-otp/validate":
			verifyRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/email-otp/validate, got %s", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected verify accept header, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != serverURL {
				t.Fatalf("expected verify origin header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/email-verification" {
				t.Fatalf("expected verify referer header, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode verify payload: %v", err)
			}
			if got := payload["code"]; got != otpCode {
				t.Fatalf("expected otp code, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"continue_url":"` + serverURL + `/about-you"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.VerifyEmailOTP(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/email-verification",
		FinalPath: "/email-verification",
		PageType:  "email_otp_verification",
	}, otpCode)
	if err != nil {
		t.Fatalf("verify email otp: %v", err)
	}
	if verifyRequests != 1 {
		t.Fatalf("expected one verify request, got %d", verifyRequests)
	}
	if result.FinalURL != server.URL+"/about-you" {
		t.Fatalf("expected about-you final url, got %q", result.FinalURL)
	}
	if result.FinalPath != "/about-you" {
		t.Fatalf("expected about-you final path, got %q", result.FinalPath)
	}
	if result.ContinueURL != server.URL+"/about-you" {
		t.Fatalf("expected about-you continue url, got %q", result.ContinueURL)
	}
	if result.PageType != "about_you" {
		t.Fatalf("expected about_you page type, got %q", result.PageType)
	}
}

func TestClientCreateAccountExtractsPrimaryRegistrationFields(t *testing.T) {
	t.Parallel()

	var createRequests int
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/create_account":
			createRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/create_account, got %s", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected create account accept header, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != serverURL {
				t.Fatalf("expected create account origin header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/about-you" {
				t.Fatalf("expected create account referer header, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create account payload: %v", err)
			}
			if got := payload["name"]; got != "Teammate Example" {
				t.Fatalf("expected create account name, got %q", got)
			}
			if got := payload["birthdate"]; got != "1990-01-02" {
				t.Fatalf("expected create account birthdate, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"/api/auth/callback/openai?code=callback-code",
				"refresh_token":"refresh-123",
				"account":{"id":"account-123"},
				"workspaces":[{"id":"workspace-123"}]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.CreateAccount(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/about-you",
		FinalPath: "/about-you",
		PageType:  "about_you",
	}, "Teammate", "Example", "1990-01-02")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if createRequests != 1 {
		t.Fatalf("expected one create account request, got %d", createRequests)
	}
	if result.ContinueURL != server.URL+"/api/auth/callback/openai?code=callback-code" {
		t.Fatalf("expected continue url, got %q", result.ContinueURL)
	}
	if result.CallbackURL != server.URL+"/api/auth/callback/openai?code=callback-code" {
		t.Fatalf("expected callback url, got %q", result.CallbackURL)
	}
	if result.AccountID != "account-123" {
		t.Fatalf("expected account id, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-123" {
		t.Fatalf("expected workspace id, got %q", result.WorkspaceID)
	}
	if result.RefreshToken != "refresh-123" {
		t.Fatalf("expected refresh token, got %q", result.RefreshToken)
	}
	if result.PageType != "callback" {
		t.Fatalf("expected callback page type, got %q", result.PageType)
	}
}

func TestClientCreateAccountCapturesExplicitCallbackURL(t *testing.T) {
	t.Parallel()

	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/create_account":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"callback_url":"` + serverURL + `/api/auth/callback/openai?code=callback-only",
				"workspace_id":"workspace-123",
				"account":{"id":"account-123"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.CreateAccount(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/about-you",
		FinalPath: "/about-you",
		PageType:  "about_you",
	}, "Teammate", "Example", "1990-01-02")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=callback-only" {
		t.Fatalf("expected callback url, got %q", result.CallbackURL)
	}
	if result.ContinueURL != serverURL+"/api/auth/callback/openai?code=callback-only" {
		t.Fatalf("expected continue url to fall back to callback, got %q", result.ContinueURL)
	}
	if result.PageType != "callback" {
		t.Fatalf("expected callback page type, got %q", result.PageType)
	}
}

func TestClientCreateAccountReturnsTypedUserExistsError(t *testing.T) {
	t.Parallel()

	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/create_account":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{
				"error":{"code":"user_exists","message":"Email already exists"},
				"continue_url":"` + serverURL + `/u/continue?state=existing",
				"account":{"id":"account-existing"},
				"workspace":{"id":"workspace-existing"}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.CreateAccount(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/about-you",
		FinalPath: "/about-you",
		PageType:  "about_you",
	}, "Teammate", "Example", "1990-01-02")
	if err == nil {
		t.Fatal("expected create account user_exists error")
	}

	var userExistsErr *CreateAccountUserExistsError
	if !errors.As(err, &userExistsErr) {
		t.Fatalf("expected CreateAccountUserExistsError, got %T: %v", err, err)
	}
	if userExistsErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected typed error status 409, got %d", userExistsErr.StatusCode)
	}
	if userExistsErr.Code != "user_exists" {
		t.Fatalf("expected typed error code user_exists, got %q", userExistsErr.Code)
	}
	if result.AccountID != "account-existing" {
		t.Fatalf("expected partial account id, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-existing" {
		t.Fatalf("expected partial workspace id, got %q", result.WorkspaceID)
	}
	if result.ContinueURL != serverURL+"/u/continue?state=existing" {
		t.Fatalf("expected partial continue url, got %q", result.ContinueURL)
	}
}

func TestClientReadSessionReturnsNormalizedTokens(t *testing.T) {
	t.Parallel()

	var sessionRequests int

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
			"chatgpt_user_id":    "user-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/session":
			sessionRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /api/auth/session, got %s", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected session accept header, got %q", got)
			}
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected next-auth cookie: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected next-auth cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"expires":"2026-04-04T12:00:00Z",
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  "__Secure-next-auth.session-token",
		Value: "session-cookie",
		Path:  "/",
	}})

	result, err := client.ReadSession(context.Background())
	if err != nil {
		t.Fatalf("read session: %v", err)
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
		t.Fatalf("expected account id, got %q", result.AccountID)
	}
	if result.UserID != "user-from-jwt" {
		t.Fatalf("expected user id, got %q", result.UserID)
	}
	if result.WorkspaceID != "account-from-jwt" {
		t.Fatalf("expected workspace id fallback, got %q", result.WorkspaceID)
	}
}

func TestClientContinueCreateAccountConsumesCallbackAndReadsSession(t *testing.T) {
	t.Parallel()

	var (
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
			"chatgpt_user_id":    "user-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/callback/openai":
			callbackRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /api/auth/callback/openai, got %s", r.Method)
			}
			if got := r.URL.Query().Get("code"); got != "callback-code" {
				t.Fatalf("expected callback code, got %q", got)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=done", http.StatusFound)
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
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /api/auth/session, got %s", r.Method)
			}
			cookie, err := r.Cookie("__Secure-next-auth.session-token")
			if err != nil {
				t.Fatalf("expected next-auth session cookie: %v", err)
			}
			if cookie.Value != "session-cookie" {
				t.Fatalf("expected next-auth session cookie value, got %q", cookie.Value)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL:  serverURL + "/api/auth/callback/openai?code=callback-code",
		CallbackURL:  serverURL + "/api/auth/callback/openai?code=callback-code",
		AccountID:    "account-created",
		WorkspaceID:  "workspace-created",
		RefreshToken: "refresh-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=callback-code" {
		t.Fatalf("expected callback url, got %q", result.CallbackURL)
	}
	if result.FinalPath != "/u/continue" {
		t.Fatalf("expected final path /u/continue, got %q", result.FinalPath)
	}
	if result.PageType != "continue" {
		t.Fatalf("expected page type continue, got %q", result.PageType)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.SessionToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained over claim fallback, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-created" {
		t.Fatalf("expected created workspace id retained over account fallback, got %q", result.WorkspaceID)
	}
	if result.RefreshToken != "refresh-created" {
		t.Fatalf("expected create-account refresh token to win, got %q", result.RefreshToken)
	}
}

func TestClientContinueCreateAccountFallsBackWhenSessionIsIncomplete(t *testing.T) {
	t.Parallel()

	var (
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_user_id": "user-from-jwt",
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
			http.Redirect(w, r, "/u/continue?state=fallback", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL:  serverURL + "/api/auth/callback/openai?code=fallback-code",
		CallbackURL:  serverURL + "/api/auth/callback/openai?code=fallback-code",
		AccountID:    "account-created",
		WorkspaceID:  "workspace-created",
		RefreshToken: "refresh-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
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
		t.Fatalf("expected session token from callback cookie, got %q", result.SessionToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected fallback account id, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-created" {
		t.Fatalf("expected fallback workspace id, got %q", result.WorkspaceID)
	}
	if result.RefreshToken != "refresh-created" {
		t.Fatalf("expected fallback refresh token, got %q", result.RefreshToken)
	}
	if result.UserID != "user-from-jwt" {
		t.Fatalf("expected user id from jwt, got %q", result.UserID)
	}
}

func TestClientContinueCreateAccountUsesSessionRefreshFallback(t *testing.T) {
	t.Parallel()

	var serverURL string

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_user_id": "user-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/callback/openai":
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=refresh-fallback", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/api/auth/callback/openai?code=session-refresh",
		CallbackURL: serverURL + "/api/auth/callback/openai?code=session-refresh",
		AccountID:   "account-created",
		WorkspaceID: "workspace-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session fallback, got %q", result.RefreshToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-created" {
		t.Fatalf("expected created workspace id retained, got %q", result.WorkspaceID)
	}
}

func TestClientContinueCreateAccountSelectsWorkspaceAndOrganization(t *testing.T) {
	t.Parallel()

	var (
		workspaceSelectRequests    int
		organizationSelectRequests int
		callbackRequests           int
		sessionRequests            int
		serverURL                  string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
			"chatgpt_user_id":    "user-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/workspace/select":
			workspaceSelectRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/workspace/select, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/sign-in-with-chatgpt/codex/consent" {
				t.Fatalf("expected workspace select referer, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode workspace select payload: %v", err)
			}
			if got := payload["workspace_id"]; got != "workspace-created" {
				t.Fatalf("expected workspace id, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/organization/select",
				"data":{"orgs":[{"id":"org-123","projects":[{"id":"project-456"}]}]}
			}`))
		case "/api/accounts/organization/select":
			organizationSelectRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/organization/select, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/organization/select" {
				t.Fatalf("expected organization select referer, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode organization select payload: %v", err)
			}
			if got := payload["org_id"]; got != "org-123" {
				t.Fatalf("expected org id, got %q", got)
			}
			if got := payload["project_id"]; got != "project-456" {
				t.Fatalf("expected project id, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=workspace-code"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token.0",
				Value: "chunk-a",
				Path:  "/",
			})
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token.1",
				Value: "chunk-b",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=workspace", http.StatusFound)
		case "/u/continue":
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "__Secure-next-auth.session-token.0=chunk-a") {
				t.Fatalf("expected chunked session token cookie in continue request, got %q", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "__Secure-next-auth.session-token.0=chunk-a") {
				t.Fatalf("expected chunked session token cookie in session request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace_id":"workspace-from-session",
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL:  serverURL + "/sign-in-with-chatgpt/codex/consent",
		PageType:     "workspace_selection",
		WorkspaceID:  "workspace-created",
		RefreshToken: "refresh-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if workspaceSelectRequests != 1 {
		t.Fatalf("expected one workspace select request, got %d", workspaceSelectRequests)
	}
	if organizationSelectRequests != 1 {
		t.Fatalf("expected one organization select request, got %d", organizationSelectRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.OrganizationID != "org-123" {
		t.Fatalf("expected organization id, got %q", result.OrganizationID)
	}
	if result.ProjectID != "project-456" {
		t.Fatalf("expected project id, got %q", result.ProjectID)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=workspace-code" {
		t.Fatalf("expected callback url from workspace flow, got %q", result.CallbackURL)
	}
	if result.SessionToken != "chunk-achunk-b" {
		t.Fatalf("expected chunked session token, got %q", result.SessionToken)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session, got %q", result.WorkspaceID)
	}
}

func TestClientContinueCreateAccountSelectsWorkspaceFromConsentCookie(t *testing.T) {
	t.Parallel()

	var (
		consentRequests         int
		workspaceSelectRequests int
		callbackRequests        int
		sessionRequests         int
		serverURL               string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sign-in-with-chatgpt/codex/consent":
			consentRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>consent</body></html>`))
		case "/api/accounts/workspace/select":
			workspaceSelectRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/sign-in-with-chatgpt/codex/consent" {
				t.Fatalf("expected workspace select referer, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode workspace select payload: %v", err)
			}
			if got := payload["workspace_id"]; got != "workspace-cookie" {
				t.Fatalf("expected workspace id from cookie, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=cookie-workspace-code"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=cookie-workspace", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			if callbackRequests == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"callback required"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	sessionJSON, err := json.Marshal(map[string]any{
		"workspaces": []map[string]string{{"id": "workspace-cookie"}},
	})
	if err != nil {
		t.Fatalf("marshal oauth session cookie: %v", err)
	}

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  "oai-client-auth-session",
		Value: base64.RawURLEncoding.EncodeToString(sessionJSON) + ".signature",
		Path:  "/",
	}})

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/sign-in-with-chatgpt/codex/consent",
		PageType:    "consent",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if consentRequests != 0 {
		t.Fatalf("expected workspace resolution to prefer cookie before consent html, got %d consent requests", consentRequests)
	}
	if workspaceSelectRequests != 1 {
		t.Fatalf("expected one workspace select request, got %d", workspaceSelectRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.WorkspaceID != "workspace-cookie" {
		t.Fatalf("expected resolved workspace id to persist, got %q", result.WorkspaceID)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=cookie-workspace-code" {
		t.Fatalf("expected callback url from workspace selection, got %q", result.CallbackURL)
	}
}

func TestClientContinueCreateAccountSelectsWorkspaceFromConsentHTML(t *testing.T) {
	t.Parallel()

	var (
		consentRequests         int
		workspaceSelectRequests int
		callbackRequests        int
		sessionRequests         int
		serverURL               string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sign-in-with-chatgpt/codex/consent":
			consentRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><script>streamController.enqueue("{\"workspaces\":[{\"id\":\"workspace-html\",\"kind\":\"personal\"}],\"openai_client_id\":\"client-123\"}")</script></body></html>`))
		case "/api/accounts/workspace/select":
			workspaceSelectRequests++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode workspace select payload: %v", err)
			}
			if got := payload["workspace_id"]; got != "workspace-html" {
				t.Fatalf("expected workspace id from consent html, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=html-workspace-code"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=html-workspace", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			if callbackRequests == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"callback required"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/sign-in-with-chatgpt/codex/consent",
		PageType:    "consent",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if consentRequests != 1 {
		t.Fatalf("expected one consent html fetch, got %d", consentRequests)
	}
	if workspaceSelectRequests != 1 {
		t.Fatalf("expected one workspace select request, got %d", workspaceSelectRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.WorkspaceID != "workspace-html" {
		t.Fatalf("expected resolved workspace id from consent html, got %q", result.WorkspaceID)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=html-workspace-code" {
		t.Fatalf("expected callback url from workspace selection, got %q", result.CallbackURL)
	}
}

func TestClientContinueCreateAccountPrefersConsentURLWhenPageTypeIsStale(t *testing.T) {
	t.Parallel()

	var (
		consentRequests         int
		workspaceSelectRequests int
		callbackRequests        int
		sessionRequests         int
		serverURL               string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sign-in-with-chatgpt/codex/consent":
			consentRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><script>streamController.enqueue("{\"workspaces\":[{\"id\":\"workspace-stale\",\"kind\":\"personal\"}],\"openai_client_id\":\"client-123\"}")</script></body></html>`))
		case "/api/accounts/workspace/select":
			workspaceSelectRequests++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode workspace select payload: %v", err)
			}
			if got := payload["workspace_id"]; got != "workspace-stale" {
				t.Fatalf("expected workspace id from consent html, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=stale-consent-code"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=stale-consent", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			if callbackRequests == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"callback required"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/sign-in-with-chatgpt/codex/consent",
		PageType:    "callback",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if consentRequests != 1 {
		t.Fatalf("expected one consent html fetch, got %d", consentRequests)
	}
	if workspaceSelectRequests != 1 {
		t.Fatalf("expected one workspace select request, got %d", workspaceSelectRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.WorkspaceID != "workspace-stale" {
		t.Fatalf("expected resolved workspace id from consent html, got %q", result.WorkspaceID)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=stale-consent-code" {
		t.Fatalf("expected callback url from workspace selection, got %q", result.CallbackURL)
	}
}

func TestClientContinueCreateAccountReturnsAddPhoneStateWithoutReadingSession(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		sessionRequests  int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">Add phone</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			t.Fatal("did not expect session request when flow is blocked on add_phone")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if sessionRequests != 0 {
		t.Fatalf("expected zero session requests, got %d", sessionRequests)
	}
	if result.FinalURL != serverURL+"/add-phone" {
		t.Fatalf("expected final url /add-phone, got %q", result.FinalURL)
	}
	if result.FinalPath != "/add-phone" {
		t.Fatalf("expected final path /add-phone, got %q", result.FinalPath)
	}
	if result.PageType != "add_phone" {
		t.Fatalf("expected add_phone page type, got %q", result.PageType)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected account id retained, got %q", result.AccountID)
	}
	if result.AccessToken != "" {
		t.Fatalf("expected empty access token for add_phone boundary, got %q", result.AccessToken)
	}
}

func TestClientContinueCreateAccountAutoContinuesAddPhoneSkipURL(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		continueRequests int
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone"><a href="` + serverURL + `/u/continue?state=skip-phone">Skip for now</a></body></html>`))
		case "/u/continue":
			continueRequests++
			if got := r.URL.Query().Get("state"); got != "skip-phone" {
				t.Fatalf("expected skip-phone state, got %q", got)
			}
			http.Redirect(w, r, "/api/auth/callback/openai?code=skip-phone-code", http.StatusFound)
		case "/api/auth/callback/openai":
			callbackRequests++
			if got := r.URL.Query().Get("code"); got != "skip-phone-code" {
				t.Fatalf("expected skip-phone code, got %q", got)
			}
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
				"sessionToken":"session-from-payload",
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

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if continueRequests != 1 {
		t.Fatalf("expected one continue request from add-phone skip link, got %d", continueRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request after add-phone skip link, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request after add-phone auto-continue, got %d", sessionRequests)
	}
	if result.FinalPath != "/welcome" {
		t.Fatalf("expected final path /welcome, got %q", result.FinalPath)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=skip-phone-code" {
		t.Fatalf("expected callback url from add-phone continuation, got %q", result.CallbackURL)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
}

func TestClientContinueCreateAccountAutoContinuesAddPhoneSkipFormPost(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		formPostRequests int
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
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
			if got := r.URL.Query().Get("code"); got != "skip-phone-form-code" {
				t.Fatalf("expected skip-phone-form-code, got %q", got)
			}
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
				"sessionToken":"session-from-payload",
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

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if formPostRequests != 1 {
		t.Fatalf("expected one form submit from add-phone page, got %d", formPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request after add-phone form submit, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request after add-phone form submit, got %d", sessionRequests)
	}
	if result.FinalPath != "/welcome" {
		t.Fatalf("expected final path /welcome, got %q", result.FinalPath)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=skip-phone-form-code" {
		t.Fatalf("expected callback url from add-phone form continuation, got %q", result.CallbackURL)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
}

func TestClientContinueCreateAccountAutoContinuesAddPhonePrefersSkipFormWithButtonAndCSRF(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		verifyRequests   int
		formPostRequests int
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
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
			if got := r.URL.Query().Get("code"); got != "skip-phone-form-priority-code" {
				t.Fatalf("expected skip-phone-form-priority-code, got %q", got)
			}
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
				"sessionToken":"session-from-payload",
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

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
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
		t.Fatalf("expected one callback request after add-phone form submit, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request after add-phone form submit, got %d", sessionRequests)
	}
	if result.FinalPath != "/welcome" {
		t.Fatalf("expected final path /welcome, got %q", result.FinalPath)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=skip-phone-form-priority-code" {
		t.Fatalf("expected callback url from prioritized add-phone form continuation, got %q", result.CallbackURL)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
}

func TestClientContinueCreateAccountAutoContinuesAddPhoneDataActionJSONPayload(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		verifyRequests   int
		formPostRequests int
		callbackRequests int
		sessionRequests  int
		serverURL        string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
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
			if got := r.URL.Query().Get("code"); got != "skip-phone-data-action-code" {
				t.Fatalf("expected skip-phone-data-action-code, got %q", got)
			}
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
				"sessionToken":"session-from-payload",
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

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
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
		t.Fatalf("expected one callback request after add-phone data action, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request after add-phone data action, got %d", sessionRequests)
	}
	if result.FinalPath != "/welcome" {
		t.Fatalf("expected final path /welcome, got %q", result.FinalPath)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=skip-phone-data-action-code" {
		t.Fatalf("expected callback url from add-phone data action, got %q", result.CallbackURL)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
}

func TestClientContinueCreateAccountAutoContinuesAddPhoneStaticFetchJSONBody(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests  int
		verifyRequests    int
		fetchPostRequests int
		callbackRequests  int
		sessionRequests   int
		serverURL         string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-session",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
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
			if got := r.Header.Get("Referer"); got != serverURL+"/add-phone" {
				t.Fatalf("expected add-phone referer on fetch submit, got %q", got)
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
			if got := r.URL.Query().Get("code"); got != "skip-phone-static-fetch-code" {
				t.Fatalf("expected skip-phone-static-fetch-code, got %q", got)
			}
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
				"sessionToken":"session-from-payload",
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

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if verifyRequests != 0 {
		t.Fatalf("expected verify fetch to be skipped, got %d requests", verifyRequests)
	}
	if fetchPostRequests != 1 {
		t.Fatalf("expected one add-phone fetch submit, got %d", fetchPostRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request after add-phone fetch, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request after add-phone fetch, got %d", sessionRequests)
	}
	if result.FinalPath != "/welcome" {
		t.Fatalf("expected final path /welcome, got %q", result.FinalPath)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=skip-phone-static-fetch-code" {
		t.Fatalf("expected callback url from add-phone static fetch, got %q", result.CallbackURL)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected created account id retained, got %q", result.AccountID)
	}
}

func TestClientContinueCreateAccountKeepsAddPhoneBoundaryWhenFetchBodyIsDynamic(t *testing.T) {
	t.Parallel()

	var (
		addPhoneRequests int
		skipRequests     int
		sessionRequests  int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add-phone":
			addPhoneRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /add-phone, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="add-phone">
				<script>
					const skipPayload = {
						state: "skip-phone",
						intent: "skip_phone",
						screen_hint: "add_phone",
						csrf_token: window.__csrfToken
					};
					fetch("` + serverURL + `/add-phone/skip", {
						method: "POST",
						headers: {"Content-Type": "application/json"},
						body: JSON.stringify(skipPayload)
					});
				</script>
			</body></html>`))
		case "/add-phone/skip":
			skipRequests++
			t.Fatal("did not expect dynamic fetch payload to be auto-submitted")
		case "/api/auth/session":
			sessionRequests++
			t.Fatal("did not expect session request when dynamic fetch payload cannot be inferred statically")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.ContinueCreateAccount(context.Background(), CreateAccountResult{
		ContinueURL: serverURL + "/add-phone",
		PageType:    "add_phone",
		AccountID:   "account-created",
	})
	if err != nil {
		t.Fatalf("continue create account: %v", err)
	}
	if addPhoneRequests != 1 {
		t.Fatalf("expected one add-phone request, got %d", addPhoneRequests)
	}
	if skipRequests != 0 {
		t.Fatalf("expected zero add-phone skip requests, got %d", skipRequests)
	}
	if sessionRequests != 0 {
		t.Fatalf("expected zero session requests, got %d", sessionRequests)
	}
	if result.FinalPath != "/add-phone" {
		t.Fatalf("expected final path /add-phone, got %q", result.FinalPath)
	}
	if result.PageType != "add_phone" {
		t.Fatalf("expected add_phone page type, got %q", result.PageType)
	}
	if result.AccessToken != "" {
		t.Fatalf("expected empty access token for dynamic fetch boundary, got %q", result.AccessToken)
	}
}

func TestClientReadSessionSupportsAlternateChunkedSessionCookies(t *testing.T) {
	t.Parallel()

	var sessionRequests int

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/session":
			sessionRequests++
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "_Secure-next-auth.session-token.0=alt-") {
				t.Fatalf("expected alternate chunked cookie in request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{
		{
			Name:  "_Secure-next-auth.session-token.0",
			Value: "alt-",
			Path:  "/",
		},
		{
			Name:  "_Secure-next-auth.session-token.1",
			Value: "session",
			Path:  "/",
		},
	})

	result, err := client.ReadSession(context.Background())
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.SessionToken != "alt-session" {
		t.Fatalf("expected alternate chunked session token, got %q", result.SessionToken)
	}
}

func TestClientReadSessionExtractsRefreshTokenAndExplicitWorkspace(t *testing.T) {
	t.Parallel()

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/session":
			if got := r.Header.Get("Cookie"); !strings.Contains(got, "__Secure-next-auth.session-token=session-cookie") {
				t.Fatalf("expected direct next-auth cookie in request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"refreshToken":"refresh-from-session",
				"workspace":{"id":"workspace-object"},
				"workspaces":[{"id":"workspace-list"}],
				"user":{},
				"account":{}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  "__Secure-next-auth.session-token",
		Value: "session-cookie",
		Path:  "/",
	}})

	result, err := client.ReadSession(context.Background())
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session payload, got %q", result.RefreshToken)
	}
	if result.WorkspaceID != "workspace-object" {
		t.Fatalf("expected explicit workspace object id, got %q", result.WorkspaceID)
	}
}

func testJWT(t *testing.T, payload map[string]any) string {
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
