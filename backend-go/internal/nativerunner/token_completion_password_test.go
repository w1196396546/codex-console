package nativerunner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
)

func TestAuthPasswordTokenCompletionProviderCompletesPasswordLoginFlow(t *testing.T) {
	t.Parallel()

	var (
		authorizeContinueRequests  int
		passwordVerifyRequests     int
		workspaceSelectRequests    int
		organizationSelectRequests int
		callbackRequests           int
		sessionRequests            int
		serverURL                  string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=password-login"}`))
		case "/authorize":
			http.Redirect(w, r, "/log-in", http.StatusFound)
		case "/log-in":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="login">Log in</body></html>`))
		case "/api/accounts/authorize/continue":
			authorizeContinueRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"login_password",
				"continue_url":"` + serverURL + `/log-in/password"
			}`))
		case "/api/accounts/password/verify":
			passwordVerifyRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"workspace_selection",
				"continue_url":"` + serverURL + `/sign-in-with-chatgpt/codex/consent",
				"workspaces":[{"id":"workspace-created"}],
				"data":{"orgs":[{"id":"org-123","projects":[{"id":"project-456"}]}]}
			}`))
		case "/api/accounts/workspace/select":
			workspaceSelectRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/organization/select",
				"data":{"orgs":[{"id":"org-123","projects":[{"id":"project-456"}]}]}
			}`))
		case "/api/accounts/organization/select":
			organizationSelectRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=oauth-login"
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=done", http.StatusFound)
		case "/u/continue":
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
				"refreshToken":"refresh-from-session",
				"workspace":{"id":"workspace-from-session"},
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

	client, err := auth.NewClient(auth.Options{
		BaseURL:   server.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	provider := NewAuthPasswordTokenCompletionProvider(client)
	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:      "native@example.com",
		Password:   "known-pass",
		Strategy:   TokenCompletionStrategyPassword,
		AuthClient: client,
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}

	if authorizeContinueRequests != 1 || passwordVerifyRequests != 1 || workspaceSelectRequests != 1 || organizationSelectRequests != 1 || callbackRequests != 1 || sessionRequests != 1 {
		t.Fatalf("unexpected request counts authorize_continue=%d password_verify=%d workspace=%d organization=%d callback=%d session=%d",
			authorizeContinueRequests, passwordVerifyRequests, workspaceSelectRequests, organizationSelectRequests, callbackRequests, sessionRequests)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-from-session" {
		t.Fatalf("expected refresh token from session, got %q", result.RefreshToken)
	}
	if result.SessionToken != "session-cookie" {
		t.Fatalf("expected session token, got %q", result.SessionToken)
	}
	if result.AccountID != "account-from-jwt" {
		t.Fatalf("expected account id from jwt fallback, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-from-session" {
		t.Fatalf("expected workspace id from session payload, got %q", result.WorkspaceID)
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

func TestAuthPasswordTokenCompletionProviderFallsBackToOAuthTokenWhenSessionMissingRefreshToken(t *testing.T) {
	t.Parallel()

	const (
		codeVerifier  = "pkce-verifier"
		redirectURI   = "http://localhost:1455/auth/callback"
		oauthClientID = "app-test-client"
		callbackCode  = "oauth-login"
		refreshToken  = "refresh-from-exchange"
	)

	var (
		oauthTokenRequests int
		serverURL          string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.pkce.code_verifier",
				Value: codeVerifier,
				Path:  "/",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?client_id=` + url.QueryEscape(oauthClientID) + `&redirect_uri=` + url.QueryEscape(redirectURI) + `"}`))
		case "/authorize":
			http.Redirect(w, r, "/log-in", http.StatusFound)
		case "/log-in":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="login">Log in</body></html>`))
		case "/api/accounts/authorize/continue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"login_password",
				"continue_url":"` + serverURL + `/log-in/password"
			}`))
		case "/api/accounts/password/verify":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"callback",
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=` + callbackCode + `"
			}`))
		case "/api/auth/callback/openai":
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=done", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace":{"id":"workspace-from-session"},
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		case "/oauth/token":
			oauthTokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse oauth token form: %v", err)
			}
			if got := r.Form.Get("code"); got != callbackCode {
				t.Fatalf("expected callback code, got %q", got)
			}
			if got := r.Form.Get("code_verifier"); got != codeVerifier {
				t.Fatalf("expected code verifier, got %q", got)
			}
			if got := r.Form.Get("client_id"); got != oauthClientID {
				t.Fatalf("expected client id, got %q", got)
			}
			if got := r.Form.Get("redirect_uri"); got != redirectURI {
				t.Fatalf("expected redirect uri, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"access_token":"access-from-exchange",
				"refresh_token":"` + refreshToken + `"
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

	provider := NewAuthPasswordTokenCompletionProvider(client)
	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:      "native@example.com",
		Password:   "known-pass",
		Strategy:   TokenCompletionStrategyPassword,
		AuthClient: client,
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if oauthTokenRequests != 1 {
		t.Fatalf("expected one oauth token fallback request, got %d", oauthTokenRequests)
	}
	if result.RefreshToken != refreshToken {
		t.Fatalf("expected refresh token from oauth token fallback, got %q", result.RefreshToken)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected session access token, got %q", result.AccessToken)
	}
	if result.Source != "login" {
		t.Fatalf("expected login source, got %q", result.Source)
	}
	if result.AuthProvider != "openai" {
		t.Fatalf("expected openai auth provider, got %q", result.AuthProvider)
	}
}

func TestDefaultPrepareSignupFlowUsesRealPasswordTokenCompletionProvider(t *testing.T) {
	t.Parallel()

	var (
		oauthTokenRequests int
		serverURL          string
	)

	accessToken := testTokenCompletionJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.pkce.code_verifier",
				Value: "pkce-verifier",
				Path:  "/",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=password-login&client_id=` + url.QueryEscape("app-test-client") + `&redirect_uri=` + url.QueryEscape("http://localhost:1455/auth/callback") + `"}`))
		case "/authorize":
			http.Redirect(w, r, "/log-in", http.StatusFound)
		case "/log-in":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="login">Log in</body></html>`))
		case "/api/accounts/authorize/continue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"login_password",
				"continue_url":"` + serverURL + `/log-in/password"
			}`))
		case "/api/accounts/password/verify":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"callback",
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=oauth-login"
			}`))
		case "/api/auth/callback/openai":
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=done", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"workspace":{"id":"workspace-from-session"},
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		case "/oauth/token":
			oauthTokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse oauth token form: %v", err)
			}
			if got := r.Form.Get("code"); got != "oauth-login" {
				t.Fatalf("expected callback code, got %q", got)
			}
			if got := r.Form.Get("code_verifier"); got != "pkce-verifier" {
				t.Fatalf("expected code verifier, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"access_token":"access-from-exchange",
				"refresh_token":"refresh-from-exchange"
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
			Email:    "native@example.com",
			Password: "known-pass",
		},
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if result.State != TokenCompletionStateCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if result.Strategy != TokenCompletionStrategyPassword {
		t.Fatalf("expected password strategy, got %+v", result)
	}
	if oauthTokenRequests != 1 {
		t.Fatalf("expected one oauth token fallback request, got %d", oauthTokenRequests)
	}
	if result.Provider.RefreshToken != "refresh-from-exchange" {
		t.Fatalf("expected refresh token from default password provider, got %+v", result.Provider)
	}
}

func TestAuthPasswordTokenCompletionProviderRejectsMissingPassword(t *testing.T) {
	t.Parallel()

	provider := NewAuthPasswordTokenCompletionProvider(nil)
	_, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:    "native@example.com",
		Strategy: TokenCompletionStrategyPassword,
	})
	if err == nil {
		t.Fatal("expected missing password error")
	}

	tokenErr, ok := err.(*TokenCompletionError)
	if !ok {
		t.Fatalf("expected token completion error, got %T", err)
	}
	if tokenErr.Kind != TokenCompletionErrorKindMissingPassword {
		t.Fatalf("expected missing password kind, got %+v", tokenErr)
	}
}

func TestStrategyTokenCompletionProviderFallsBackToPasswordlessAfterInteractivePasswordStep(t *testing.T) {
	t.Parallel()

	passwordProvider := &stubTokenCompletionProvider{
		err: &TokenCompletionError{
			Kind:    TokenCompletionErrorKindInteractiveStepRequired,
			Message: "password flow hit add_phone",
		},
	}
	passwordlessProvider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken:  "access-from-passwordless",
			RefreshToken: "refresh-from-passwordless",
			SessionToken: "session-from-passwordless",
		},
	}

	provider := NewStrategyTokenCompletionProvider(passwordProvider, passwordlessProvider)
	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:       "native@example.com",
		Password:    "known-pass",
		Strategy:    TokenCompletionStrategyPassword,
		ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if passwordProvider.calls != 1 {
		t.Fatalf("expected password provider called once, got %d", passwordProvider.calls)
	}
	if passwordlessProvider.calls != 1 {
		t.Fatalf("expected passwordless provider fallback called once, got %d", passwordlessProvider.calls)
	}
	if passwordlessProvider.lastRequest.Strategy != TokenCompletionStrategyPasswordless {
		t.Fatalf("expected passwordless fallback strategy, got %+v", passwordlessProvider.lastRequest)
	}
	if passwordlessProvider.lastRequest.Password != "" {
		t.Fatalf("expected passwordless fallback to clear password, got %+v", passwordlessProvider.lastRequest)
	}
	if result.AccessToken != "access-from-passwordless" || result.RefreshToken != "refresh-from-passwordless" || result.SessionToken != "session-from-passwordless" {
		t.Fatalf("unexpected passwordless fallback result: %+v", result)
	}
}

func TestStrategyTokenCompletionProviderFallsBackToPasswordlessWhenPasswordMissing(t *testing.T) {
	t.Parallel()

	passwordProvider := &stubTokenCompletionProvider{
		err: &TokenCompletionError{
			Kind:    TokenCompletionErrorKindMissingPassword,
			Message: "password token completion requires historical password",
		},
	}
	passwordlessProvider := &stubTokenCompletionProvider{
		result: TokenCompletionProviderResult{
			AccessToken:  "access-from-passwordless",
			RefreshToken: "refresh-from-passwordless",
			SessionToken: "session-from-passwordless",
			Source:       "login",
		},
	}

	provider := NewStrategyTokenCompletionProvider(passwordProvider, passwordlessProvider)
	result, err := provider.CompleteToken(context.Background(), TokenCompletionRequest{
		Email:       "native@example.com",
		Strategy:    TokenCompletionStrategyPassword,
		PageType:    "existing_account_detected",
		ContinueURL: "https://auth.example.com/api/auth/callback/openai?code=callback-code",
	})
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if passwordProvider.calls != 1 {
		t.Fatalf("expected password provider called once, got %d", passwordProvider.calls)
	}
	if passwordlessProvider.calls != 1 {
		t.Fatalf("expected passwordless provider fallback called once, got %d", passwordlessProvider.calls)
	}
	if passwordlessProvider.lastRequest.Strategy != TokenCompletionStrategyPasswordless {
		t.Fatalf("expected passwordless fallback strategy, got %+v", passwordlessProvider.lastRequest)
	}
	if passwordlessProvider.lastRequest.PageType != "existing_account_detected" {
		t.Fatalf("expected existing account page type forwarded to fallback, got %+v", passwordlessProvider.lastRequest)
	}
	if result.Source != "login" {
		t.Fatalf("expected login source from fallback result, got %+v", result)
	}
}
