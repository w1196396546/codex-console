package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

type Headers map[string]string

type FlowRequestKind string

const (
	FlowRequestKindRegisterPassword FlowRequestKind = "register_password"
	FlowRequestKindSendEmailOTP     FlowRequestKind = "send_email_otp"
	FlowRequestKindVerifyEmailOTP   FlowRequestKind = "verify_email_otp"
	FlowRequestKindCreateAccount    FlowRequestKind = "create_account"
)

type RequestHeadersInput struct {
	Kind          FlowRequestKind
	Email         string
	Password      string
	PrepareSignup PrepareSignupResult
}

type RequestHeadersProvider func(context.Context, RequestHeadersInput) (Headers, error)

type Options struct {
	BaseURL                string
	HTTPClient             *http.Client
	UserAgent              string
	DefaultHeaders         Headers
	RequestHeadersProvider RequestHeadersProvider
}

type Request struct {
	Method  string
	Path    string
	Headers Headers
	Body    io.Reader
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	FinalURL   string
	FinalPath  string
}

type Client struct {
	baseURL                *url.URL
	httpClient             *http.Client
	userAgent              string
	defaultHeaders         Headers
	requestHeadersProvider RequestHeadersProvider
}

func NewClient(options Options) (*Client, error) {
	baseURLText := strings.TrimSpace(options.BaseURL)
	if baseURLText == "" {
		return nil, errors.New("auth base url is required")
	}

	baseURL, err := url.Parse(baseURLText)
	if err != nil {
		return nil, fmt.Errorf("parse auth base url: %w", err)
	}
	if !baseURL.IsAbs() {
		return nil, errors.New("auth base url must be absolute")
	}

	httpClient, err := cloneHTTPClient(options.HTTPClient)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:                baseURL,
		httpClient:             httpClient,
		userAgent:              strings.TrimSpace(options.UserAgent),
		defaultHeaders:         cloneHeaders(options.DefaultHeaders),
		requestHeadersProvider: options.RequestHeadersProvider,
	}, nil
}

func (c *Client) Bootstrap(ctx context.Context) (BootstrapResult, error) {
	return c.BootstrapWith(ctx, BootstrapOptions{})
}

func (c *Client) Get(ctx context.Context, path string, headers Headers) (Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodGet,
		Path:    path,
		Headers: headers,
	})
}

func (c *Client) Do(ctx context.Context, request Request) (Response, error) {
	if c == nil {
		return Response{}, errors.New("auth client is required")
	}

	method := strings.TrimSpace(request.Method)
	if method == "" {
		method = http.MethodGet
	}

	targetURL, err := c.resolveURL(request.Path)
	if err != nil {
		return Response{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, method, targetURL.String(), request.Body)
	if err != nil {
		return Response{}, fmt.Errorf("build auth request: %w", err)
	}

	applyHeaders(httpRequest.Header, c.defaultHeaders)
	applyHeaders(httpRequest.Header, request.Headers)
	if c.userAgent != "" && httpRequest.Header.Get("User-Agent") == "" {
		httpRequest.Header.Set("User-Agent", c.userAgent)
	}

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("perform auth request: %w", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read auth response: %w", err)
	}

	finalURL := httpResponse.Request.URL.String()
	finalPath := httpResponse.Request.URL.Path
	if finalPath == "" {
		finalPath = "/"
	}

	return Response{
		StatusCode: httpResponse.StatusCode,
		Header:     httpResponse.Header.Clone(),
		Body:       body,
		FinalURL:   finalURL,
		FinalPath:  finalPath,
	}, nil
}

func (c *Client) Cookies() []*http.Cookie {
	if c == nil || c.httpClient == nil || c.httpClient.Jar == nil || c.baseURL == nil {
		return nil
	}
	return c.httpClient.Jar.Cookies(c.baseURL)
}

func (c *Client) resolveURL(path string) (*url.URL, error) {
	if c == nil || c.baseURL == nil {
		return nil, errors.New("auth client base url is required")
	}

	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}

	target, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse auth path: %w", err)
	}
	return c.baseURL.ResolveReference(target), nil
}

func cloneHTTPClient(source *http.Client) (*http.Client, error) {
	var client *http.Client
	if source == nil {
		client = &http.Client{Timeout: defaultTimeout}
	} else {
		cloned := *source
		client = &cloned
		if client.Timeout <= 0 {
			client.Timeout = defaultTimeout
		}
	}

	if client.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create auth cookie jar: %w", err)
		}
		client.Jar = jar
	}

	return client, nil
}

func applyHeaders(target http.Header, headers Headers) {
	for key, value := range headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		target.Set(key, value)
	}
}

func cloneHeaders(headers Headers) Headers {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(Headers, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func (c *Client) flowRequestHeaders(ctx context.Context, input RequestHeadersInput) (Headers, error) {
	if c == nil || c.requestHeadersProvider == nil {
		return nil, nil
	}

	headers, err := c.requestHeadersProvider(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("resolve %s headers: %w", input.Kind, err)
	}
	return cloneHeaders(headers), nil
}
