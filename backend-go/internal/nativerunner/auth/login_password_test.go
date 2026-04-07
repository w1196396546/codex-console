package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClientCompletePasswordLoginFallsBackToExplicitCallbackURL(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	var (
		callbackRequests       int
		passwordVerifyRequests int
		authorizeContinueCalls int
		sessionRequests        int
		signinRequests         int
		csrfRequests           int
		bootstrapRequests      int
		prepareAuthorizeVisits int
		serverURL              string
	)

	accessToken := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-from-jwt",
			"chatgpt_user_id":    "user-from-jwt",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			bootstrapRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize/openai"}`))
		case "/authorize/openai":
			prepareAuthorizeVisits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>login</body></html>`))
		case "/api/accounts/authorize/continue":
			authorizeContinueCalls++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/authorize/continue, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/authorize/openai" {
				t.Fatalf("expected authorize continue referer, got %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode authorize continue payload: %v", err)
			}
			username, _ := payload["username"].(map[string]any)
			if got := extractString(username["value"]); got != email {
				t.Fatalf("expected authorize continue email, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"login_password",
				"page":{"url":"` + serverURL + `/login/password"}
			}`))
		case "/api/accounts/password/verify":
			passwordVerifyRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/password/verify, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/login/password" {
				t.Fatalf("expected password verify referer, got %q", got)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode password verify payload: %v", err)
			}
			if got := payload["password"]; got != password {
				t.Fatalf("expected password verify payload, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"callback_url":"` + serverURL + `/api/auth/callback/openai?code=password-code",
				"account":{"id":"account-created"},
				"workspace":{"id":"workspace-created"}
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			if got := r.URL.Query().Get("code"); got != "password-code" {
				t.Fatalf("expected callback code, got %q", got)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=password", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
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

	result, err := client.CompletePasswordLogin(context.Background(), email, password)
	if err != nil {
		t.Fatalf("complete password login: %v", err)
	}
	if bootstrapRequests != 1 || csrfRequests != 1 || signinRequests != 1 || prepareAuthorizeVisits != 1 {
		t.Fatalf("expected prepare signup flow once, got bootstrap=%d csrf=%d signin=%d authorize=%d", bootstrapRequests, csrfRequests, signinRequests, prepareAuthorizeVisits)
	}
	if authorizeContinueCalls != 1 {
		t.Fatalf("expected one authorize continue call, got %d", authorizeContinueCalls)
	}
	if passwordVerifyRequests != 1 {
		t.Fatalf("expected one password verify call, got %d", passwordVerifyRequests)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if result.CallbackURL != serverURL+"/api/auth/callback/openai?code=password-code" {
		t.Fatalf("expected callback url, got %q", result.CallbackURL)
	}
	if result.PageType != "continue" {
		t.Fatalf("expected continue page type after callback, got %q", result.PageType)
	}
	if result.AccountID != "account-created" {
		t.Fatalf("expected account id retained, got %q", result.AccountID)
	}
	if result.WorkspaceID != "workspace-created" {
		t.Fatalf("expected workspace id retained, got %q", result.WorkspaceID)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token, got %q", result.AccessToken)
	}
}

func TestAuthPasswordLoginFallsBackToOAuthTokenWhenSessionMissingRefreshToken(t *testing.T) {
	t.Parallel()

	const (
		email         = "teammate@example.com"
		password      = "Password123!"
		codeVerifier  = "pkce-verifier"
		redirectURI   = "http://localhost:1455/auth/callback"
		oauthClientID = "app-test-client"
		callbackCode  = "password-code"
		refreshToken  = "refresh-from-exchange"
	)

	var (
		callbackRequests   int
		sessionRequests    int
		oauthTokenRequests int
		serverURL          string
	)

	accessToken := testJWT(t, map[string]any{
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
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize/openai?client_id=` + url.QueryEscape(oauthClientID) + `&redirect_uri=` + url.QueryEscape(redirectURI) + `"}`))
		case "/authorize/openai":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>login</body></html>`))
		case "/api/accounts/authorize/continue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"page_type":"login_password",
				"page":{"url":"` + serverURL + `/login/password"}
			}`))
		case "/api/accounts/password/verify":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"callback_url":"` + serverURL + `/api/auth/callback/openai?code=` + callbackCode + `",
				"account":{"id":"account-created"},
				"workspace":{"id":"workspace-created"}
			}`))
		case "/api/auth/callback/openai":
			callbackRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "__Secure-next-auth.session-token",
				Value: "session-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=password", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/api/auth/session":
			sessionRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"accessToken":"` + accessToken + `",
				"user":{},
				"account":{},
				"authProvider":"openai"
			}`))
		case "/oauth/token":
			oauthTokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse oauth token form: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("expected grant_type authorization_code, got %q", got)
			}
			if got := r.Form.Get("code"); got != callbackCode {
				t.Fatalf("expected authorization code, got %q", got)
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
				"refresh_token":"` + refreshToken + `",
				"id_token":"id-from-exchange"
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

	result, err := client.CompletePasswordLogin(context.Background(), email, password)
	if err != nil {
		t.Fatalf("complete password login: %v", err)
	}
	if callbackRequests != 1 {
		t.Fatalf("expected one callback request, got %d", callbackRequests)
	}
	if sessionRequests != 1 {
		t.Fatalf("expected one session request, got %d", sessionRequests)
	}
	if oauthTokenRequests != 1 {
		t.Fatalf("expected one oauth token request, got %d", oauthTokenRequests)
	}
	if result.RefreshToken != refreshToken {
		t.Fatalf("expected refresh token from oauth token fallback, got %q", result.RefreshToken)
	}
	if result.AccessToken != accessToken {
		t.Fatalf("expected access token from session, got %q", result.AccessToken)
	}
}
