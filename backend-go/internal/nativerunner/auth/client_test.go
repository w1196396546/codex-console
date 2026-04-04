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
		case "/":
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
	if len(client.Cookies()) != 1 {
		t.Fatalf("expected one persisted cookie, got %d", len(client.Cookies()))
	}
}

func TestClientPersistsBootstrapCookieForFollowUpRequests(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
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
		rootRequests      int
		csrfRequests      int
		signinRequests    int
		authorizeRequests int
		finalRequests     int
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			rootRequests++
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET /, got %s", r.Method)
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
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("expected form content type, got %q", got)
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

	if rootRequests != 1 || csrfRequests != 1 || signinRequests != 1 || authorizeRequests != 1 || finalRequests != 1 {
		t.Fatalf("unexpected request counts root=%d csrf=%d signin=%d authorize=%d final=%d", rootRequests, csrfRequests, signinRequests, authorizeRequests, finalRequests)
	}
	if result.CSRFToken != "csrf-token" {
		t.Fatalf("expected csrf token, got %q", result.CSRFToken)
	}
	if result.AuthorizeURL != server.URL+"/authorize?state=signup" {
		t.Fatalf("expected authorize url, got %q", result.AuthorizeURL)
	}
	if result.FinalURL != server.URL+"/u/continue?state=signup" {
		t.Fatalf("expected final url, got %q", result.FinalURL)
	}
	if result.FinalPath != "/u/continue" {
		t.Fatalf("expected final path /u/continue, got %q", result.FinalPath)
	}
	if result.ContinueURL != server.URL+"/u/continue?state=signup" {
		t.Fatalf("expected continue url, got %q", result.ContinueURL)
	}
	if result.PageType != "continue" {
		t.Fatalf("expected page type continue, got %q", result.PageType)
	}
	if len(client.Cookies()) != 2 {
		t.Fatalf("expected persisted cookies after authorize flow, got %d", len(client.Cookies()))
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
	if len(client.Cookies()) != 3 {
		t.Fatalf("expected persisted cookies after register and otp send, got %d", len(client.Cookies()))
	}
}
