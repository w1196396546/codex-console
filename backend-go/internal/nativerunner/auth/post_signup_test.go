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
