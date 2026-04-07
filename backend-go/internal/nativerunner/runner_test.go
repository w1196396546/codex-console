package nativerunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/mail"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
)

func TestRunnerRunNativeCreatesTempmailInboxAndPassesRequestToFlow(t *testing.T) {
	t.Parallel()

	var createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/inbox/create" {
			t.Fatalf("expected /inbox/create, got %s", r.URL.Path)
		}
		createCalls++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"address": "native@example.com",
			"token":   "token-123",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	req := registration.RunnerRequest{
		TaskUUID: "task-native",
		StartRequest: registration.StartRequest{
			EmailServiceType: "tempmail",
		},
		Plan: registration.ExecutionPlan{
			EmailService: registration.PreparedEmailService{
				Prepared: true,
				Type:     "tempmail",
				Config: map[string]any{
					"base_url": server.URL,
				},
			},
		},
	}

	var flowInput FlowRequest
	runner := New(Options{
		Flow: FlowFunc(func(_ context.Context, input FlowRequest) (registration.NativeRunnerResult, error) {
			flowInput = input
			return registration.NativeRunnerResult{
				Result: map[string]any{
					"email": input.Inbox.Email,
					"token": input.Inbox.Token,
				},
			}, nil
		}),
	})

	result, err := runner.RunNative(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("run native: %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("expected provider create called once, got %d", createCalls)
	}
	if flowInput.RunnerRequest.TaskUUID != "task-native" {
		t.Fatalf("expected flow request task-native, got %+v", flowInput.RunnerRequest)
	}
	if flowInput.Inbox.Email != "native@example.com" || flowInput.Inbox.Token != "token-123" {
		t.Fatalf("expected flow inbox from provider, got %+v", flowInput.Inbox)
	}
	if flowInput.MailProvider == nil {
		t.Fatalf("expected flow to receive mail provider")
	}
	if result.Result["email"] != "native@example.com" || result.Result["token"] != "token-123" {
		t.Fatalf("expected flow result returned, got %#v", result.Result)
	}
}

func TestRunnerRunNativeRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	flowCalled := false
	runner := New(Options{
		Flow: FlowFunc(func(_ context.Context, input FlowRequest) (registration.NativeRunnerResult, error) {
			flowCalled = true
			return registration.NativeRunnerResult{}, nil
		}),
	})

	_, err := runner.RunNative(context.Background(), registration.RunnerRequest{
		TaskUUID: "task-unsupported",
		StartRequest: registration.StartRequest{
			EmailServiceType: "unknown",
		},
		Plan: registration.ExecutionPlan{
			EmailService: registration.PreparedEmailService{
				Prepared: true,
				Type:     "unknown",
			},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if err.Error() != "create native mail provider: unsupported native mail provider: unknown" {
		t.Fatalf("unexpected error: %v", err)
	}
	if flowCalled {
		t.Fatal("expected flow not called when provider is unsupported")
	}
}

func TestRunnerWorksThroughRegistrationAdapter(t *testing.T) {
	t.Parallel()

	runner := New(Options{
		Flow: FlowFunc(func(_ context.Context, input FlowRequest) (registration.NativeRunnerResult, error) {
			return registration.NativeRunnerResult{
				Result: map[string]any{
					"email": input.Inbox.Email,
				},
			}, nil
		}),
		ProviderFactory: providerFactoryFunc(func(_ string, _ map[string]any) (mail.Provider, error) {
			return stubMailProvider{
				inbox: mail.Inbox{Email: "adapter@example.com", Token: "adapter-token"},
			}, nil
		}),
	})

	adapted := registration.NewNativeRunner(runner)
	result, err := adapted.Run(context.Background(), registration.RunnerRequest{
		TaskUUID: "task-adapter",
		StartRequest: registration.StartRequest{
			EmailServiceType: "tempmail",
		},
	}, nil)
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Result["email"] != "adapter@example.com" {
		t.Fatalf("expected adapter result email, got %#v", result)
	}
}

func TestNewDefaultBuildsPrepareSignupFlowWithDefaults(t *testing.T) {
	t.Parallel()

	runner := NewDefault(DefaultOptions{})
	if runner == nil {
		t.Fatal("expected default runner")
	}

	flow, ok := runner.flow.(*PrepareSignupFlow)
	if !ok || flow == nil {
		t.Fatalf("expected default runner to use prepare signup flow, got %#v", runner.flow)
	}
	if flow.preparerFactory == nil {
		t.Fatal("expected default preparer factory")
	}
	if flow.postSignupClientFactory == nil {
		t.Fatal("expected default post-signup client factory")
	}
	if flow.accountProfileProvider == nil {
		t.Fatal("expected default account profile provider")
	}
	if flow.clientIDResolver == nil {
		t.Fatal("expected default client id resolver")
	}

	profile, err := flow.accountProfileProvider.ResolveAccountProfile(context.Background(), FlowRequest{})
	if err != nil {
		t.Fatalf("resolve default account profile: %v", err)
	}
	if strings.TrimSpace(profile.FirstName) == "" || strings.TrimSpace(profile.LastName) == "" || strings.TrimSpace(profile.Birthdate) == "" {
		t.Fatalf("expected populated default account profile, got %+v", profile)
	}

	clientID, err := flow.clientIDResolver.ResolveClientID(context.Background(), FlowRequest{})
	if err != nil {
		t.Fatalf("resolve default client id: %v", err)
	}
	if clientID != defaultOpenAIClientID {
		t.Fatalf("expected default client id %q, got %q", defaultOpenAIClientID, clientID)
	}
}

func TestDefaultPrepareSignupFlowFactoriesReuseSingleAuthClient(t *testing.T) {
	t.Parallel()

	var bootstrapCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("expected bootstrap request to root path, got %s", r.URL.Path)
		}
		bootstrapCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	flow := NewDefaultPrepareSignupFlow(DefaultPrepareSignupFlowOptions{
		AuthBaseURL: server.URL,
	})
	input := FlowRequest{runtime: &flowRuntime{}}

	preparer, err := flow.preparerFactory.NewSignupPreparer(context.Background(), input)
	if err != nil {
		t.Fatalf("create signup preparer: %v", err)
	}
	authPreparer, ok := preparer.(authSignupPreparer)
	if !ok {
		t.Fatalf("expected auth signup preparer, got %T", preparer)
	}

	postSignupClient, err := flow.postSignupClientFactory.NewPostSignupClient(context.Background(), input)
	if err != nil {
		t.Fatalf("create post-signup client: %v", err)
	}
	authClient, ok := postSignupClient.(*auth.Client)
	if !ok {
		t.Fatalf("expected auth client, got %T", postSignupClient)
	}
	if authPreparer.client != authClient {
		t.Fatal("expected signup preparer and post-signup factory to reuse one auth client")
	}

	if _, err := authClient.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap auth client: %v", err)
	}
	if bootstrapCalls != 1 {
		t.Fatalf("expected bootstrap to hit override base url once, got %d", bootstrapCalls)
	}
}

func TestDefaultAuthClientUsesBrowserUserAgentDuringPrepareSignup(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		loginRequests  int
		csrfRequests   int
		signinRequests int
		passwordVisits int
		serverURL      string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			loginRequests++
			if r.URL.Path != "/" {
				t.Fatalf("expected default auth client to bootstrap via homepage, got %s", r.URL.Path)
			}
			if got := r.Header.Get("User-Agent"); got != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36" {
				t.Fatalf("expected browser user agent on bootstrap request, got %q", got)
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
			if got := r.Header.Get("User-Agent"); got != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36" {
				t.Fatalf("expected browser user agent on csrf request, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/" {
				t.Fatalf("expected homepage referer on csrf request, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != serverURL {
				t.Fatalf("expected origin header on csrf request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			signinRequests++
			if got := r.Header.Get("User-Agent"); got != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36" {
				t.Fatalf("expected browser user agent on signin request, got %q", got)
			}
			if got := r.Header.Get("Referer"); got != serverURL+"/" {
				t.Fatalf("expected homepage referer on signin request, got %q", got)
			}
			if got := r.URL.Query().Get("ext-oai-did"); got == "" {
				t.Fatalf("expected ext-oai-did query on signin request, got %q", got)
			}
			if got := r.URL.Query().Get("auth_session_logging_id"); got == "" {
				t.Fatalf("expected auth_session_logging_id query on signin request, got %q", got)
			}
			if got := r.Header.Get("Origin"); got != serverURL {
				t.Fatalf("expected origin header on signin request, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + serverURL + `/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "/u/continue?state=signup", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/create-account/password":
			passwordVisits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Password</body></html>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := defaultAuthClient(FlowRequest{runtime: &flowRuntime{}}, DefaultPrepareSignupFlowOptions{
		AuthBaseURL: serverURL,
	})
	if err != nil {
		t.Fatalf("create default auth client: %v", err)
	}

	if _, err := client.PrepareSignup(context.Background(), email); err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if loginRequests != 1 || csrfRequests != 1 || signinRequests != 1 || passwordVisits != 1 {
		t.Fatalf("unexpected request counts login=%d csrf=%d signin=%d password=%d", loginRequests, csrfRequests, signinRequests, passwordVisits)
	}
}

func TestDefaultAuthClientUsesSelectedProxyDuringPrepareSignup(t *testing.T) {
	t.Parallel()

	const email = "teammate@example.com"

	var (
		proxyLoginRequests  int
		proxyCSRFRequests   int
		proxySigninRequests int
		proxyPasswordVisits int
	)

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			proxyLoginRequests++
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionid",
				Value: "bootstrap-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("bootstrap"))
		case "/api/auth/csrf":
			proxyCSRFRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"csrfToken":"csrf-token"}`))
		case "/api/auth/signin/openai":
			proxySigninRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"http://auth-proxy-test.invalid/authorize?state=signup"}`))
		case "/authorize":
			http.Redirect(w, r, "http://auth-proxy-test.invalid/u/continue?state=signup", http.StatusFound)
		case "/u/continue":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="continue">Continue</body></html>`))
		case "/create-account/password":
			proxyPasswordVisits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body data-page="password">Password</body></html>`))
		default:
			t.Fatalf("unexpected proxy path: %s", r.URL.Path)
		}
	}))
	defer proxyServer.Close()

	client, err := defaultAuthClient(FlowRequest{
		RunnerRequest: registration.RunnerRequest{
			Plan: registration.ExecutionPlan{
				Proxy: registration.ProxySelection{
					Selected: proxyServer.URL,
				},
			},
		},
		runtime: &flowRuntime{},
	}, DefaultPrepareSignupFlowOptions{
		AuthBaseURL: "http://auth-proxy-test.invalid",
	})
	if err != nil {
		t.Fatalf("create default auth client: %v", err)
	}

	if _, err := client.PrepareSignup(context.Background(), email); err != nil {
		t.Fatalf("prepare signup: %v", err)
	}
	if proxyLoginRequests != 1 || proxyCSRFRequests != 1 || proxySigninRequests != 1 || proxyPasswordVisits != 1 {
		t.Fatalf("unexpected proxy request counts login=%d csrf=%d signin=%d password=%d", proxyLoginRequests, proxyCSRFRequests, proxySigninRequests, proxyPasswordVisits)
	}
}

type stubMailProvider struct {
	inbox mail.Inbox
}

func (s stubMailProvider) Create(context.Context) (mail.Inbox, error) {
	return s.inbox, nil
}

func (stubMailProvider) WaitCode(context.Context, mail.Inbox, *regexp.Regexp) (string, error) {
	return "", nil
}
