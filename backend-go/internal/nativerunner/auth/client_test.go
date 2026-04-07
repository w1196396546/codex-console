package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientBootstrapEstablishesCookieSessionAndFollowsRedirect(t *testing.T) {
	t.Parallel()

	var rootRequests int
	var homeRequests int

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			rootRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /, got %s", r.Method)
			}
			if got := r.Header.Get("User-Agent"); got != "codex-native-auth/0.1" {
				t.Fatalf("expected user agent header, got %q", got)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/homepage", http.StatusFound)
		case "/homepage":
			homeRequests++
			if got := r.Header.Get("User-Agent"); got != "codex-native-auth/0.1" {
				t.Fatalf("expected user agent on redirect request, got %q", got)
			}
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on redirected request: %v", err)
			}
			if cookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value, got %q", cookie.Value)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
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

	result, err := client.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if rootRequests != 1 {
		t.Fatalf("expected one root request, got %d", rootRequests)
	}
	if homeRequests != 1 {
		t.Fatalf("expected one homepage request, got %d", homeRequests)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if result.FinalPath != "/homepage" {
		t.Fatalf("expected final path /homepage, got %q", result.FinalPath)
	}
	if len(client.Cookies()) != 2 {
		t.Fatalf("expected bootstrap cookie plus oai-did, got %d cookies", len(client.Cookies()))
	}
}

func TestClientPersistsBootstrapCookieForFollowUpRequests(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login", "/":
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "persisted-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/homepage", http.StatusFound)
		case "/homepage":
			w.WriteHeader(http.StatusOK)
		case "/api/session":
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected persisted cookie on follow-up request: %v", err)
			}
			if cookie.Value != "persisted-cookie" {
				t.Fatalf("expected persisted cookie value, got %q", cookie.Value)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected accept header, got %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
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

	if _, err := client.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	response, err := client.Get(context.Background(), "/api/session", Headers{
		"Accept": "application/json",
	})
	if err != nil {
		t.Fatalf("get follow-up session: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", response.StatusCode)
	}
}

func TestClientPrepareSignupAlignsHappyPathAndPersistsCookies(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		loginRequests     int
		csrfRequests      int
		signinRequests    int
		authorizeRequests int
		finalRequests     int
		passwordRequests  int
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			loginRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /, got %s", r.Method)
			}
			if r.URL.Path != "/" {
				t.Fatalf("expected homepage bootstrap path, got %s", r.URL.Path)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /api/auth/csrf, got %s", r.Method)
			}
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on csrf request: %v", err)
			}
			if cookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on csrf request, got %q", cookie.Value)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected csrf accept header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/" {
				t.Fatalf("expected homepage referer header, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != server.URL {
				t.Fatalf("expected csrf origin header, got %q", got)
			}
			if got := r.Header.Get("User-Agent"); got != "codex-native-auth/0.1" {
				t.Fatalf("expected csrf user agent header, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/auth/signin/openai, got %s", r.Method)
			}
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on signin request: %v", err)
			}
			if cookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on signin request, got %q", cookie.Value)
			}
			query := r.URL.Query()
			if got := query.Get("prompt"); got != "login" {
				t.Fatalf("expected signin prompt query, got %q", got)
			}
			if got := query.Get("screen_hint"); got != "login_or_signup" {
				t.Fatalf("expected signin screen_hint query, got %q", got)
			}
			if got := query.Get("login_hint"); got != email {
				t.Fatalf("expected signin login_hint query, got %q", got)
			}
			if got := query.Get("ext-oai-did"); got == "" {
				t.Fatalf("expected ext-oai-did query to be present, got %q", got)
			}
			if got := query.Get("auth_session_logging_id"); got == "" {
				t.Fatalf("expected auth_session_logging_id query to be present, got %q", got)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("expected form content type, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != server.URL {
				t.Fatalf("expected signin origin header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/" {
				t.Fatalf("expected signin referer header, got %q", got)
			}
			if got := r.Header.Get("User-Agent"); got != "codex-native-auth/0.1" {
				t.Fatalf("expected signin user agent header, got %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read signin body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse signin form: %v", err)
			}
			if got := values.Get("csrfToken"); got != "csrf-token" {
				t.Fatalf("expected csrf token in signin form, got %q", got)
			}
			if got := values.Get("callbackUrl"); got != server.URL+"/" {
				t.Fatalf("expected callback url in signin form, got %q", got)
			}
			if got := values.Get("json"); got != "true" {
				t.Fatalf("expected json=true in signin form, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + server.URL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /authorize, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/" {
				t.Fatalf("expected authorize referer header, got %q", got)
			}
			cookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on authorize request: %v", err)
			}
			if cookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on authorize request, got %q", cookie.Value)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "authorizeid",
				Value: "authorize-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=signup", http.StatusFound)
		case "/u/continue":
			finalRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /u/continue, got %s", r.Method)
			}
			sessionCookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected bootstrap cookie on final request: %v", err)
			}
			if sessionCookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on final request, got %q", sessionCookie.Value)
			}
			authorizeCookie, err := r.Cookie("authorizeid")
			if err != nil {
				t.Fatalf("expected authorize cookie on final request: %v", err)
			}
			if authorizeCookie.Value != "authorize-cookie" {
				t.Fatalf("expected authorize cookie value on final request, got %q", authorizeCookie.Value)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/create-account/password":
			passwordRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /create-account/password, got %s", r.Method)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/u/continue?state=signup" {
				t.Fatalf("expected fallback referer from continue page, got %q", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Password</body></html>`))
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

	result, err := client.PrepareSignup(context.Background(), email)
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}

	if loginRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || finalRequests != 1 || passwordRequests != 1 {
		t.Fatalf("unexpected request counts login=%d csrf=%d signin=%d authorize=%d final=%d password=%d", loginRequests, csrfRequests, signinRequests, authorizeRequests, finalRequests, passwordRequests)
	}
	if result.CSRFToken != "csrf-token" {
		t.Fatalf("expected csrf token, got %q", result.CSRFToken)
	}
	if result.AuthorizeURL != server.URL+"/authorize?state=signup" {
		t.Fatalf("expected authorize url, got %q", result.AuthorizeURL)
	}
	if result.FinalURL != server.URL+"/create-account/password" {
		t.Fatalf("expected final url, got %q", result.FinalURL)
	}
	if result.FinalPath != "/create-account/password" {
		t.Fatalf("expected final path /create-account/password, got %q", result.FinalPath)
	}
	if result.ContinueURL != "" {
		t.Fatalf("expected continue url cleared after fallback, got %q", result.ContinueURL)
	}
	if result.PageType != "create_account_password" {
		t.Fatalf("expected page type create_account_password, got %q", result.PageType)
	}
	if len(client.Cookies()) != 3 {
		t.Fatalf("expected persisted cookies after authorize flow plus oai-did, got %d", len(client.Cookies()))
	}
}

func TestClientPrepareSignupContinuesAuthorizeFlowIntoCreateAccountPasswordPage(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		loginRequests     int
		csrfRequests      int
		signinRequests    int
		authorizeRequests int
		continueRequests  int
		passwordRequests  int
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			loginRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if got := r.URL.Query().Get("login_hint"); got != email {
				t.Fatalf("expected login_hint %q, got %q", email, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + server.URL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "authorizeid",
				Value: "authorize-cookie",
				Path:  "/",
			})
			http.Redirect(w, r, "/u/continue?state=signup", http.StatusFound)
		case "/u/continue":
			continueRequests++
			if _, err := r.Cookie("sessionid"); err != nil {
				t.Fatalf("expected bootstrap cookie on continue request: %v", err)
			}
			if _, err := r.Cookie("authorizeid"); err != nil {
				t.Fatalf("expected authorize cookie on continue request: %v", err)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue"><a href="/create-account/password">Continue</a></body></html>`))
		case "/create-account/password":
			passwordRequests++
			if _, err := r.Cookie("sessionid"); err != nil {
				t.Fatalf("expected bootstrap cookie on password page request: %v", err)
			}
			if _, err := r.Cookie("authorizeid"); err != nil {
				t.Fatalf("expected authorize cookie on password page request: %v", err)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>Create account</body></html>`))
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

	result, err := client.PrepareSignup(context.Background(), email)
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}

	if loginRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || continueRequests != 1 || passwordRequests != 1 {
		t.Fatalf(
			"unexpected request counts login=%d csrf=%d signin=%d authorize=%d continue=%d password=%d",
			loginRequests,
			csrfRequests,
			signinRequests,
			authorizeRequests,
			continueRequests,
			passwordRequests,
		)
	}
	if result.AuthorizeURL != server.URL+"/authorize?state=signup" {
		t.Fatalf("expected authorize url, got %q", result.AuthorizeURL)
	}
	if result.FinalURL != server.URL+"/create-account/password" {
		t.Fatalf("expected final url create-account/password, got %q", result.FinalURL)
	}
	if result.FinalPath != "/create-account/password" {
		t.Fatalf("expected final path create-account/password, got %q", result.FinalPath)
	}
	if result.PageType != "create_account_password" {
		t.Fatalf("expected create_account_password page type, got %q", result.PageType)
	}
	if result.ContinueURL != "" {
		t.Fatalf("expected empty continue url after reaching password page, got %q", result.ContinueURL)
	}
}

func TestClientPrepareSignupRetriesAuthorizeWhenInterceptedByCloudflareInterstitial(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		authorizeRequests int
		serverURL         string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			if authorizeRequests == 1 {
				http.Redirect(w, r, "/api/accounts/authorize?state=signup", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/api/accounts/authorize":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>blocked by cloudflare</body></html>`))
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Password</body></html>`))
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

	result, err := client.PrepareSignup(context.Background(), email)
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if authorizeRequests != 2 {
		t.Fatalf("expected authorize retried twice after interstitial, got %d", authorizeRequests)
	}
	if result.FinalURL != server.URL+"/create-account/password" {
		t.Fatalf("expected final password url after retry, got %q", result.FinalURL)
	}
	if result.PageType != "create_account_password" {
		t.Fatalf("expected create_account_password after retry, got %#v", result)
	}
}

func TestClientPrepareSignupRetriesAuthorizeWhenChallengePageReturnedWithoutRedirect(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		authorizeRequests int
		serverURL         string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if authorizeRequests == 1 {
				_, _ = w.Write([]byte(`<html><head><title>Just a moment...</title></head><body>Checking your browser before accessing auth.openai.com __cf_chl_tk cloudflare</body></html>`))
				return
			}
			http.Redirect(w, r, "/create-account/password", http.StatusFound)
		case "/create-account/password":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Password</body></html>`))
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

	result, err := client.PrepareSignup(context.Background(), email)
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if authorizeRequests != 2 {
		t.Fatalf("expected authorize retried after challenge page, got %d", authorizeRequests)
	}
	if result.FinalURL != server.URL+"/create-account/password" {
		t.Fatalf("expected final password url after retry, got %q", result.FinalURL)
	}
	if result.PageType != "create_account_password" {
		t.Fatalf("expected create_account_password after retry, got %#v", result)
	}
}

func TestClientPrepareSignupFollowsContinuePageToLoginPasswordVariant(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		loginRequests         int
		csrfRequests          int
		signinRequests        int
		authorizeRequests     int
		continueRequests      int
		loginPasswordRequests int
		legacyPasswordHits    int
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			loginRequests++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			csrfRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if got := r.URL.Query().Get("login_hint"); got != email {
				t.Fatalf("expected login_hint %q, got %q", email, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + server.URL + `/authorize?state=signup"}`))
		case "/authorize":
			authorizeRequests++
			http.Redirect(w, r, "/u/continue?state=signup", http.StatusFound)
		case "/u/continue":
			continueRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue"><a href="/log-in/password?state=existing">Continue</a></body></html>`))
		case "/create-account/password":
			legacyPasswordHits++
			http.NotFound(w, r)
		case "/log-in/password":
			loginPasswordRequests++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Existing account password</body></html>`))
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

	result, err := client.PrepareSignup(context.Background(), email)
	if err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if loginRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || continueRequests != 1 {
		t.Fatalf(
			"unexpected request counts login=%d csrf=%d signin=%d authorize=%d continue=%d",
			loginRequests,
			csrfRequests,
			signinRequests,
			authorizeRequests,
			continueRequests,
		)
	}
	if loginPasswordRequests != 1 {
		t.Fatalf("expected continue page to advance into login password variant, got %d requests", loginPasswordRequests)
	}
	if legacyPasswordHits > 1 {
		t.Fatalf("expected at most one legacy password fallback probe, got %d", legacyPasswordHits)
	}
	if result.FinalURL != server.URL+"/log-in/password?state=existing" {
		t.Fatalf("expected login-password final url, got %q", result.FinalURL)
	}
	if result.FinalPath != "/log-in/password" {
		t.Fatalf("expected login-password final path, got %q", result.FinalPath)
	}
	if result.PageType != "login_password" {
		t.Fatalf("expected login_password page type, got %#v", result)
	}
	if result.ContinueURL != "" {
		t.Fatalf("expected continue url cleared after landing on login password page, got %q", result.ContinueURL)
	}
}

func TestClientRegisterPasswordAndSendOTPAlignsHappyPathAndPersistsCookies(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	type registerPasswordAndSendOTPer interface {
		RegisterPasswordAndSendOTP(context.Context, PrepareSignupResult, string, string) (PrepareSignupResult, error)
	}

	var (
		registerRequests int
		otpRequests      int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/user/register":
			registerRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST /api/accounts/user/register, got %s", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("expected register accept header, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != serverURL {
				t.Fatalf("expected register origin header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected register referer header, got %q", got)
			}
			if got := r.Header.Get("oai-device-id"); got == "" {
				t.Fatalf("expected register device id header, got %q", got)
			}
			if got := r.Header.Get("User-Agent"); got != "codex-native-auth/0.1" {
				t.Fatalf("expected register user agent header, got %q", got)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("expected register content type, got %q", got)
			}
			sessionCookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected session cookie on register request: %v", err)
			}
			if sessionCookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on register request, got %q", sessionCookie.Value)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode register payload: %v", err)
			}
			if got := payload["username"]; got != email {
				t.Fatalf("expected register username, got %q", got)
			}
			if got := payload["password"]; got != password {
				t.Fatalf("expected register password, got %q", got)
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
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /api/accounts/email-otp/send, got %s", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json, text/plain, */*" {
				t.Fatalf("expected otp send accept header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected otp send referer header, got %q", got)
			}
			sessionCookie, err := r.Cookie("sessionid")
			if err != nil {
				t.Fatalf("expected session cookie on otp request: %v", err)
			}
			if sessionCookie.Value != "bootstrap-cookie" {
				t.Fatalf("expected bootstrap cookie value on otp request, got %q", sessionCookie.Value)
			}
			registerCookie, err := r.Cookie("registerid")
			if err != nil {
				t.Fatalf("expected register cookie on otp request: %v", err)
			}
			if registerCookie.Value != "register-cookie" {
				t.Fatalf("expected register cookie value on otp request, got %q", registerCookie.Value)
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "otpid",
				Value: "otp-cookie",
				Path:  "/",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
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

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  "sessionid",
		Value: "bootstrap-cookie",
		Path:  "/",
	}})

	runner, ok := any(client).(registerPasswordAndSendOTPer)
	if !ok {
		t.Fatalf("expected client to implement RegisterPasswordAndSendOTP")
	}

	result, err := runner.RegisterPasswordAndSendOTP(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/create-account/password",
		FinalPath: "/create-account/password",
		PageType:  "create_account_password",
	}, email, password)
	if err != nil {
		t.Fatalf("register password and send otp: %v", err)
	}

	if registerRequests != 1 {
		t.Fatalf("expected one register request, got %d", registerRequests)
	}
	if otpRequests != 1 {
		t.Fatalf("expected one otp request, got %d", otpRequests)
	}
	if result.FinalURL != server.URL+"/email-verification" {
		t.Fatalf("expected email verification final url, got %q", result.FinalURL)
	}
	if result.FinalPath != "/email-verification" {
		t.Fatalf("expected email verification final path, got %q", result.FinalPath)
	}
	if result.PageType != "email_otp_verification" {
		t.Fatalf("expected email verification page type, got %q", result.PageType)
	}
	if len(client.Cookies()) != 4 {
		t.Fatalf("expected persisted cookies after register and otp send plus oai-did, got %d", len(client.Cookies()))
	}
}

func TestClientRegisterPasswordAndSendOTPUsesPreparedFlowHost(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	type registerPasswordAndSendOTPer interface {
		RegisterPasswordAndSendOTP(context.Context, PrepareSignupResult, string, string) (PrepareSignupResult, error)
	}

	var authRequests int
	var authServer *httptest.Server
	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/user/register":
			authRequests++
			if got := r.Header.Get("Origin"); got != authServer.URL {
				t.Fatalf("expected auth host origin header, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != authServer.URL+"/create-account/password" {
				t.Fatalf("expected auth host referer header, got %q", got)
			}
			if _, err := r.Cookie("sessionid"); err != nil {
				t.Fatalf("expected auth-host cookie on register request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			authRequests++
			if got := r.Header.Get("Referer"); got != authServer.URL+"/create-account/password" {
				t.Fatalf("expected auth host referer on otp request, got %q", got)
			}
			if _, err := r.Cookie("sessionid"); err != nil {
				t.Fatalf("expected auth-host cookie on otp request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected auth-server path: %s", r.URL.Path)
		}
	}))
	defer authServer.Close()

	chatgptServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not hit base host once prepared flow moved to auth host: %s", r.URL.Path)
	}))
	defer chatgptServer.Close()

	client, err := NewClient(Options{
		BaseURL:   chatgptServer.URL,
		UserAgent: "codex-native-auth/0.1",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	authBaseURL, err := url.Parse(authServer.URL)
	if err != nil {
		t.Fatalf("parse auth server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(authBaseURL, []*http.Cookie{{
		Name:  "sessionid",
		Value: "auth-cookie",
		Path:  "/",
	}})

	runner, ok := any(client).(registerPasswordAndSendOTPer)
	if !ok {
		t.Fatalf("expected client to implement RegisterPasswordAndSendOTP")
	}

	result, err := runner.RegisterPasswordAndSendOTP(context.Background(), PrepareSignupResult{
		FinalURL:  authServer.URL + "/create-account/password",
		FinalPath: "/create-account/password",
		PageType:  "create_account_password",
	}, email, password)
	if err != nil {
		t.Fatalf("register password and send otp: %v", err)
	}
	if authRequests != 2 {
		t.Fatalf("expected register and otp requests against auth host, got %d", authRequests)
	}
	if result.FinalURL != authServer.URL+"/email-verification" {
		t.Fatalf("expected email verification to stay on auth host, got %q", result.FinalURL)
	}
}

func TestClientRegisterPasswordAndSendOTPToleratesOTPDispatchFailure(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	type registerPasswordAndSendOTPer interface {
		RegisterPasswordAndSendOTP(context.Context, PrepareSignupResult, string, string) (PrepareSignupResult, error)
	}

	var (
		registerRequests int
		otpRequests      int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/user/register":
			registerRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			otpRequests++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary otp dispatch failure"}`))
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

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  "sessionid",
		Value: "bootstrap-cookie",
		Path:  "/",
	}})

	runner, ok := any(client).(registerPasswordAndSendOTPer)
	if !ok {
		t.Fatalf("expected client to implement RegisterPasswordAndSendOTP")
	}

	result, err := runner.RegisterPasswordAndSendOTP(context.Background(), PrepareSignupResult{
		FinalURL:  serverURL + "/create-account/password",
		FinalPath: "/create-account/password",
		PageType:  "create_account_password",
	}, email, password)
	if err != nil {
		t.Fatalf("register password and send otp: %v", err)
	}
	if registerRequests != 1 || otpRequests != 1 {
		t.Fatalf("expected register and otp requests once, got register=%d otp=%d", registerRequests, otpRequests)
	}
	if result.PageType != "email_otp_verification" {
		t.Fatalf("expected otp verification page type after otp dispatch failure, got %#v", result)
	}
	if result.SendOTPStatusCode != http.StatusInternalServerError {
		t.Fatalf("expected send otp status recorded, got %#v", result)
	}
}

func TestClientRegisterPasswordAndSendOTPReturnsExistingAccountPageTypeOnUserExists(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	type registerPasswordAndSendOTPer interface {
		RegisterPasswordAndSendOTP(context.Context, PrepareSignupResult, string, string) (PrepareSignupResult, error)
	}

	var (
		registerRequests int
		otpRequests      int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/user/register":
			registerRequests++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{
				"error":{"code":"user_exists","message":"Email already exists"},
				"continue_url":"` + serverURL + `/api/auth/callback/openai?code=existing"
			}`))
		case "/api/accounts/email-otp/send":
			otpRequests++
			t.Fatal("did not expect otp send after user_exists response")
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

	runner, ok := any(client).(registerPasswordAndSendOTPer)
	if !ok {
		t.Fatalf("expected client to implement RegisterPasswordAndSendOTP")
	}

	result, err := runner.RegisterPasswordAndSendOTP(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/create-account/password",
		FinalPath: "/create-account/password",
		PageType:  "create_account_password",
	}, email, password)
	if err != nil {
		t.Fatalf("register password and send otp: %v", err)
	}
	if registerRequests != 1 {
		t.Fatalf("expected one register request, got %d", registerRequests)
	}
	if otpRequests != 0 {
		t.Fatalf("expected no otp request after user_exists, got %d", otpRequests)
	}
	if result.PageType != "user_exists" {
		t.Fatalf("expected user_exists page type, got %#v", result)
	}
	if result.RegisterStatusCode != http.StatusConflict {
		t.Fatalf("expected register status 409, got %#v", result)
	}
	if result.ContinueURL != serverURL+"/api/auth/callback/openai?code=existing" {
		t.Fatalf("expected continue url from user_exists payload, got %#v", result)
	}
}

func TestClientRegisterPasswordAndSendOTPUsesCanonicalPasswordRefererForContinueState(t *testing.T) {
	t.Parallel()

	const (
		email    = "teammate@example.com"
		password = "Password123!"
	)

	type registerPasswordAndSendOTPer interface {
		RegisterPasswordAndSendOTP(context.Context, PrepareSignupResult, string, string) (PrepareSignupResult, error)
	}

	var (
		registerRequests int
		otpRequests      int
		serverURL        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/user/register":
			registerRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected canonical password referer, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/accounts/email-otp/send":
			otpRequests++
			if got := r.Header.Get("Referer"); got != serverURL+"/create-account/password" {
				t.Fatalf("expected canonical password referer on otp request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
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

	runner, ok := any(client).(registerPasswordAndSendOTPer)
	if !ok {
		t.Fatalf("expected client to implement RegisterPasswordAndSendOTP")
	}

	_, err = runner.RegisterPasswordAndSendOTP(context.Background(), PrepareSignupResult{
		FinalURL:  server.URL + "/u/continue?state=signup",
		FinalPath: "/u/continue",
		PageType:  "continue",
	}, email, password)
	if err != nil {
		t.Fatalf("register password and send otp: %v", err)
	}
	if registerRequests != 1 || otpRequests != 1 {
		t.Fatalf("expected register+otp requests once, got register=%d otp=%d", registerRequests, otpRequests)
	}
}
