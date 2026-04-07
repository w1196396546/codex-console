package nativerunner

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner/auth"
)

const (
	defaultAuthBaseURL    = "https://chatgpt.com"
	defaultOpenAIClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultAuthUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

func defaultAuthHeaders() auth.Headers {
	return auth.Headers{
		"Accept-Language":            "en-US,en;q=0.9",
		"sec-ch-ua":                  `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		"sec-ch-ua-mobile":           "?0",
		"sec-ch-ua-platform":         `"Windows"`,
		"sec-ch-ua-arch":             `"x86"`,
		"sec-ch-ua-bitness":          `"64"`,
		"sec-ch-ua-full-version":     `"131.0.0.0"`,
		"sec-ch-ua-platform-version": `"15.0.0"`,
	}
}

var defaultAccountProfileFirstNames = []string{
	"James", "Robert", "John", "Michael", "David", "William", "Richard",
	"Mary", "Jennifer", "Linda", "Elizabeth", "Susan", "Jessica", "Sarah",
	"Emily", "Emma", "Olivia", "Sophia", "Liam", "Noah", "Oliver", "Ethan",
}

var defaultAccountProfileLastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller",
	"Davis", "Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Martin",
}

const (
	defaultTokenCompletionMinSpacing  = 8 * time.Second
	defaultTokenCompletionBaseBackoff = 45 * time.Second
	defaultTokenCompletionMaxBackoff  = 15 * time.Minute
	defaultTokenCompletionCooldown    = 15 * time.Minute
)

type DefaultOptions struct {
	AuthBaseURL                     string
	OpenAIClientID                  string
	HTTPClient                      *http.Client
	RequestHeadersProvider          auth.RequestHeadersProvider
	ProviderFactory                 ProviderFactory
	SignupPasswordGenerator         AuthSignupPasswordGenerator
	AccountProfileProvider          AccountProfileProvider
	ClientIDResolver                ClientIDResolver
	HistoricalPasswordProvider      HistoricalPasswordProvider
	TokenCompletionCooldownProvider TokenCompletionCooldownProvider
	TokenCompletionAttemptProvider  TokenCompletionAttemptProvider
	TokenCompletionCoordinator      TokenCompletionDispatcher
}

type DefaultPrepareSignupFlowOptions struct {
	AuthBaseURL                     string
	OpenAIClientID                  string
	HTTPClient                      *http.Client
	RequestHeadersProvider          auth.RequestHeadersProvider
	SignupPasswordGenerator         AuthSignupPasswordGenerator
	AccountProfileProvider          AccountProfileProvider
	ClientIDResolver                ClientIDResolver
	HistoricalPasswordProvider      HistoricalPasswordProvider
	TokenCompletionCooldownProvider TokenCompletionCooldownProvider
	TokenCompletionAttemptProvider  TokenCompletionAttemptProvider
	TokenCompletionCoordinator      TokenCompletionDispatcher
}

type flowRuntime struct {
	authClient *auth.Client
}

func NewDefault(options DefaultOptions) *Runner {
	return New(Options{
		Flow: NewDefaultPrepareSignupFlow(DefaultPrepareSignupFlowOptions{
			AuthBaseURL:                     options.AuthBaseURL,
			OpenAIClientID:                  options.OpenAIClientID,
			HTTPClient:                      options.HTTPClient,
			RequestHeadersProvider:          options.RequestHeadersProvider,
			SignupPasswordGenerator:         options.SignupPasswordGenerator,
			AccountProfileProvider:          options.AccountProfileProvider,
			ClientIDResolver:                options.ClientIDResolver,
			HistoricalPasswordProvider:      options.HistoricalPasswordProvider,
			TokenCompletionCooldownProvider: options.TokenCompletionCooldownProvider,
			TokenCompletionAttemptProvider:  options.TokenCompletionAttemptProvider,
			TokenCompletionCoordinator:      options.TokenCompletionCoordinator,
		}),
		ProviderFactory: options.ProviderFactory,
	})
}

func NewDefaultPrepareSignupFlow(options DefaultPrepareSignupFlowOptions) *PrepareSignupFlow {
	accountProfileProvider := options.AccountProfileProvider
	if accountProfileProvider == nil {
		accountProfileProvider = AccountProfileProviderFunc(func(context.Context, FlowRequest) (AccountProfile, error) {
			return defaultAccountProfile()
		})
	}

	clientIDResolver := options.ClientIDResolver
	if clientIDResolver == nil {
		clientID := strings.TrimSpace(options.OpenAIClientID)
		if clientID == "" {
			clientID = defaultOpenAIClientID
		}
		clientIDResolver = ClientIDResolverFunc(func(context.Context, FlowRequest) (string, error) {
			return clientID, nil
		})
	}

	tokenCompletionCoordinator := options.TokenCompletionCoordinator
	if tokenCompletionCoordinator == nil {
		defaultProvider := TokenCompletionProviderFunc(func(ctx context.Context, request TokenCompletionRequest) (TokenCompletionProviderResult, error) {
			client := request.AuthClient
			if client == nil {
				var err error
				client, err = auth.NewClient(auth.Options{
					BaseURL:                firstNonEmptyTrimmed(options.AuthBaseURL, defaultAuthBaseURL),
					HTTPClient:             options.HTTPClient,
					UserAgent:              defaultAuthUserAgent,
					DefaultHeaders:         defaultAuthHeaders(),
					RequestHeadersProvider: options.RequestHeadersProvider,
				})
				if err != nil {
					return TokenCompletionProviderResult{}, &TokenCompletionError{
						Kind:    TokenCompletionErrorKindProviderUnavailable,
						Message: fmt.Sprintf("create token completion auth client: %v", err),
					}
				}
			}

			return NewStrategyTokenCompletionProvider(
				NewAuthPasswordTokenCompletionProvider(client),
				NewAuthPasswordlessTokenCompletionProvider(client),
			).CompleteToken(ctx, request)
		})

		tokenCompletionCoordinator = NewTokenCompletionCoordinator(TokenCompletionCoordinatorOptions{
			Scheduler: NewTokenCompletionScheduler(DefaultTokenCompletionSchedulerPolicy()),
			Provider:  defaultProvider,
		})
	}

	return NewPrepareSignupFlow(PrepareSignupFlowOptions{
		PreparerFactory: SignupPreparerFactoryFunc(func(_ context.Context, input FlowRequest) (SignupPreparer, error) {
			client, err := defaultAuthClient(input, options)
			if err != nil {
				return nil, err
			}

			var signupOptions []AuthSignupPreparerOption
			if options.SignupPasswordGenerator != nil {
				signupOptions = append(signupOptions, WithAuthSignupPasswordGenerator(options.SignupPasswordGenerator))
			}

			return NewAuthSignupPreparer(client, signupOptions...), nil
		}),
		PostSignupClientFactory: AuthPostSignupClientFactoryFunc(func(_ context.Context, input FlowRequest) (AuthPostSignupClient, error) {
			return defaultAuthClient(input, options)
		}),
		AccountProfileProvider:          accountProfileProvider,
		ClientIDResolver:                clientIDResolver,
		HistoricalPasswordProvider:      options.HistoricalPasswordProvider,
		TokenCompletionCooldownProvider: options.TokenCompletionCooldownProvider,
		TokenCompletionAttemptProvider:  options.TokenCompletionAttemptProvider,
		TokenCompletionCoordinator:      tokenCompletionCoordinator,
	})
}

func defaultAuthClient(input FlowRequest, options DefaultPrepareSignupFlowOptions) (*auth.Client, error) {
	runtime := input.runtime
	if runtime == nil {
		runtime = &flowRuntime{}
	}
	if runtime.authClient != nil {
		return runtime.authClient, nil
	}

	baseURL := strings.TrimSpace(options.AuthBaseURL)
	if baseURL == "" {
		baseURL = defaultAuthBaseURL
	}

	httpClient, err := proxyAwareHTTPClient(options.HTTPClient, selectedProxyURL(input))
	if err != nil {
		return nil, fmt.Errorf("create default native auth http client: %w", err)
	}

	client, err := auth.NewClient(auth.Options{
		BaseURL:                baseURL,
		HTTPClient:             httpClient,
		UserAgent:              defaultAuthUserAgent,
		DefaultHeaders:         defaultAuthHeaders(),
		RequestHeadersProvider: options.RequestHeadersProvider,
	})
	if err != nil {
		return nil, fmt.Errorf("create default native auth client: %w", err)
	}

	runtime.authClient = client
	return client, nil
}

func selectedProxyURL(input FlowRequest) string {
	if selected := strings.TrimSpace(input.RunnerRequest.Plan.Proxy.Selected); selected != "" {
		return selected
	}
	return strings.TrimSpace(input.RunnerRequest.StartRequest.Proxy)
}

func proxyAwareHTTPClient(source *http.Client, proxyText string) (*http.Client, error) {
	proxyText = strings.TrimSpace(proxyText)
	if proxyText == "" {
		return source, nil
	}

	proxyURL, err := url.Parse(proxyText)
	if err != nil {
		return nil, fmt.Errorf("parse auth proxy url: %w", err)
	}

	var client *http.Client
	if source == nil {
		client = &http.Client{}
	} else {
		cloned := *source
		client = &cloned
	}

	var transport *http.Transport
	switch typed := client.Transport.(type) {
	case nil:
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("default auth transport has unexpected type %T", http.DefaultTransport)
		}
		transport = defaultTransport.Clone()
	case *http.Transport:
		transport = typed.Clone()
	default:
		return nil, fmt.Errorf("auth proxy requires *http.Transport, got %T", typed)
	}

	transport.Proxy = http.ProxyURL(proxyURL)
	client.Transport = transport
	return client, nil
}

func defaultAccountProfile() (AccountProfile, error) {
	firstName, err := randomProfileValue(defaultAccountProfileFirstNames)
	if err != nil {
		return AccountProfile{}, err
	}
	lastName, err := randomProfileValue(defaultAccountProfileLastNames)
	if err != nil {
		return AccountProfile{}, err
	}

	year, err := randomIntInRange(1996, 2006)
	if err != nil {
		return AccountProfile{}, err
	}
	month, err := randomIntInRange(1, 12)
	if err != nil {
		return AccountProfile{}, err
	}
	day, err := randomIntInRange(1, 28)
	if err != nil {
		return AccountProfile{}, err
	}

	return AccountProfile{
		FirstName: firstName,
		LastName:  lastName,
		Birthdate: fmt.Sprintf("%04d-%02d-%02d", year, month, day),
	}, nil
}

func randomProfileValue(values []string) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("default profile values are required")
	}

	index, err := randomIntInRange(0, len(values)-1)
	if err != nil {
		return "", err
	}
	return values[index], nil
}

func randomIntInRange(minValue int, maxValue int) (int, error) {
	if maxValue < minValue {
		return 0, fmt.Errorf("invalid random range %d-%d", minValue, maxValue)
	}

	size := maxValue - minValue + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return minValue + int(n.Int64()), nil
}
