package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultProxyVerificationURL   = "https://api.ipify.org?format=json"
	defaultDynamicProxyTestTimout = 10 * time.Second
	defaultProxyTestTimeout       = 3 * time.Second
)

type HTTPProxyTesterOptions struct {
	VerifyURL           string
	DynamicFetchClient  *http.Client
	VerificationClient  *http.Client
	DynamicFetchTimeout time.Duration
	VerificationTimeout time.Duration
}

type HTTPProxyTester struct {
	verifyURL           string
	dynamicFetchClient  *http.Client
	verificationClient  *http.Client
	dynamicFetchTimeout time.Duration
	verifyTimeout       time.Duration
}

func NewHTTPProxyTester(options HTTPProxyTesterOptions) *HTTPProxyTester {
	verifyURL := strings.TrimSpace(options.VerifyURL)
	if verifyURL == "" {
		verifyURL = defaultProxyVerificationURL
	}

	dynamicFetchTimeout := options.DynamicFetchTimeout
	if dynamicFetchTimeout <= 0 {
		dynamicFetchTimeout = defaultDynamicProxyTestTimout
	}

	verifyTimeout := options.VerificationTimeout
	if verifyTimeout <= 0 {
		verifyTimeout = defaultProxyTestTimeout
	}

	return &HTTPProxyTester{
		verifyURL:           verifyURL,
		dynamicFetchClient:  options.DynamicFetchClient,
		verificationClient:  options.VerificationClient,
		dynamicFetchTimeout: dynamicFetchTimeout,
		verifyTimeout:       verifyTimeout,
	}
}

func (t *HTTPProxyTester) TestDynamicProxy(ctx context.Context, req UpdateDynamicProxySettingsRequest) (DynamicProxyTestResponse, error) {
	apiURL := strings.TrimSpace(req.APIURL)
	if apiURL == "" {
		return DynamicProxyTestResponse{
			Success: false,
			Message: "请填写动态代理 API 地址",
		}, nil
	}

	proxyURL, message := t.fetchDynamicProxy(ctx, req)
	if proxyURL == "" {
		return DynamicProxyTestResponse{
			Success:  false,
			ProxyURL: proxyURL,
			Message:  message,
		}, nil
	}

	result := t.verifyProxy(ctx, proxyURL, t.verifyTimeout)
	if !result.Success {
		return DynamicProxyTestResponse{
			Success:      false,
			ProxyURL:     proxyURL,
			IP:           result.IP,
			ResponseTime: result.ResponseTime,
			Message:      result.Message,
		}, nil
	}

	return DynamicProxyTestResponse{
		Success:      true,
		ProxyURL:     proxyURL,
		IP:           result.IP,
		ResponseTime: result.ResponseTime,
		Message:      fmt.Sprintf("动态代理可用，出口 IP: %s，响应时间: %dms", result.IP, result.ResponseTime),
	}, nil
}

func (t *HTTPProxyTester) TestProxy(ctx context.Context, proxy ProxyRecord) (ProxyTestResult, error) {
	proxyURL := buildProxyRecordURL(proxy)
	if proxyURL == "" {
		return ProxyTestResult{
			ID:      proxy.ID,
			Name:    proxy.Name,
			Success: false,
			Message: "代理配置不完整",
		}, nil
	}

	result := t.verifyProxy(ctx, proxyURL, t.verifyTimeout)
	result.ID = proxy.ID
	result.Name = proxy.Name
	if result.Success {
		result.Message = fmt.Sprintf("代理连接成功，出口 IP: %s", result.IP)
	}
	return result, nil
}

func (t *HTTPProxyTester) fetchDynamicProxy(ctx context.Context, req UpdateDynamicProxySettingsRequest) (string, string) {
	requestCtx, cancel := context.WithTimeout(ctx, t.dynamicFetchTimeout)
	defer cancel()

	httpClient := cloneHTTPClientWithTimeout(t.dynamicFetchClient, t.dynamicFetchTimeout)
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimSpace(req.APIURL), nil)
	if err != nil {
		return "", fmt.Sprintf("动态代理 API 请求构建失败: %v", err)
	}

	apiKeyHeader := strings.TrimSpace(req.APIKeyHeader)
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}
	if req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "" {
		httpRequest.Header.Set(apiKeyHeader, strings.TrimSpace(*req.APIKey))
	}

	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Sprintf("动态代理 API 请求失败: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("动态代理 API 返回错误状态码: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Sprintf("动态代理 API 响应读取失败: %v", err)
	}

	proxyURL := extractDynamicProxyURL(body, req.ResultField)
	if proxyURL == "" {
		return "", "动态代理 API 返回为空或请求失败"
	}

	return proxyURL, ""
}

func (t *HTTPProxyTester) verifyProxy(ctx context.Context, proxyURL string, timeout time.Duration) ProxyTestResult {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpClient, err := newProxyHTTPClient(t.verificationClient, proxyURL, timeout)
	if err != nil {
		return ProxyTestResult{
			Success: false,
			Message: fmt.Sprintf("代理连接失败: %v", err),
		}
	}

	startedAt := time.Now()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, t.verifyURL, nil)
	if err != nil {
		return ProxyTestResult{
			Success: false,
			Message: fmt.Sprintf("代理连接失败: %v", err),
		}
	}

	response, err := httpClient.Do(httpRequest)
	elapsed := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		return ProxyTestResult{
			Success:      false,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("代理连接失败: %v", err),
		}
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return ProxyTestResult{
			Success:      false,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("代理连接失败: HTTP %d", response.StatusCode),
		}
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return ProxyTestResult{
			Success:      false,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("代理连接失败: %v", err),
		}
	}

	return ProxyTestResult{
		Success:      true,
		IP:           extractIP(body),
		ResponseTime: elapsed,
	}
}

func extractDynamicProxyURL(body []byte, resultField string) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}

	proxyURL := ""
	if strings.TrimSpace(resultField) != "" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var payload any
		if err := json.Unmarshal(body, &payload); err == nil {
			if strings.TrimSpace(resultField) != "" {
				proxyURL = extractValueByPath(payload, resultField)
			} else {
				proxyURL = extractCommonProxyValue(payload)
			}
		}
	}

	if proxyURL == "" {
		proxyURL = trimmed
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ""
	}
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + proxyURL
	}
	return proxyURL
}

func extractValueByPath(payload any, fieldPath string) string {
	current := payload
	for _, key := range strings.Split(strings.TrimSpace(fieldPath), ".") {
		if key == "" {
			return ""
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[key]
		case []any:
			return ""
		default:
			return ""
		}
	}
	if current == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", current))
}

func extractCommonProxyValue(payload any) string {
	typed, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"proxy", "url", "proxy_url", "data", "ip"} {
		value, exists := typed[key]
		if !exists || value == nil {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			if proxyURL := extractCommonProxyValue(nested); proxyURL != "" {
				return proxyURL
			}
		}
		if proxyURL := strings.TrimSpace(fmt.Sprintf("%v", value)); proxyURL != "" {
			return proxyURL
		}
	}
	return ""
}

func extractIP(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", payload["ip"]))
}

func buildProxyRecordURL(proxy ProxyRecord) string {
	if proxyURL := strings.TrimSpace(proxy.ProxyURL); proxyURL != "" {
		if strings.Contains(proxyURL, "://") {
			return proxyURL
		}
		return "http://" + proxyURL
	}

	host := strings.TrimSpace(proxy.Host)
	if host == "" || proxy.Port <= 0 {
		return ""
	}

	scheme := strings.TrimSpace(proxy.Type)
	if scheme == "" {
		scheme = "http"
	}

	proxyURL := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, proxy.Port),
	}
	if proxy.Username != nil && strings.TrimSpace(*proxy.Username) != "" {
		password := ""
		if proxy.Password != nil {
			password = *proxy.Password
		}
		proxyURL.User = url.UserPassword(strings.TrimSpace(*proxy.Username), password)
	}
	return proxyURL.String()
}

func cloneHTTPClientWithTimeout(source *http.Client, timeout time.Duration) *http.Client {
	if source == nil {
		return &http.Client{Timeout: timeout}
	}
	cloned := *source
	if timeout > 0 {
		cloned.Timeout = timeout
	}
	return &cloned
}

func newProxyHTTPClient(source *http.Client, proxyText string, timeout time.Duration) (*http.Client, error) {
	proxyURL, err := url.Parse(strings.TrimSpace(proxyText))
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}

	client := cloneHTTPClientWithTimeout(source, timeout)
	var transport *http.Transport
	switch typed := client.Transport.(type) {
	case nil:
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("default transport has unexpected type %T", http.DefaultTransport)
		}
		transport = defaultTransport.Clone()
	case *http.Transport:
		transport = typed.Clone()
	default:
		return nil, fmt.Errorf("proxy transport requires *http.Transport, got %T", typed)
	}

	transport.Proxy = http.ProxyURL(proxyURL)
	client.Transport = transport
	return client, nil
}
