package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresProxySelector struct {
	pool       *pgxpool.Pool
	settings   SettingsProvider
	httpClient *http.Client
}

func NewPostgresProxySelector(pool *pgxpool.Pool, settings SettingsProvider) *PostgresProxySelector {
	return &PostgresProxySelector{
		pool:     pool,
		settings: settings,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *PostgresProxySelector) SelectProxy(ctx context.Context, req StartRequest) (ProxySelection, error) {
	requested := strings.TrimSpace(req.Proxy)
	if requested != "" {
		return ProxySelection{
			Requested: requested,
			Selected:  requested,
			Source:    "request",
		}, nil
	}

	if selection, ok, err := s.selectFromLegacyProxyPool(ctx); err != nil {
		return ProxySelection{}, fmt.Errorf("select proxy from pool: %w", err)
	} else if ok {
		return selection, nil
	}

	return s.selectFromSettings(ctx)
}

func (s *PostgresProxySelector) selectFromLegacyProxyPool(ctx context.Context) (ProxySelection, bool, error) {
	if s == nil || s.pool == nil {
		return ProxySelection{}, false, nil
	}

	row := s.pool.QueryRow(ctx, `
SELECT id, proxy_url
FROM proxies
WHERE enabled = TRUE
  AND COALESCE(proxy_url, '') <> ''
ORDER BY is_default DESC, last_used ASC NULLS FIRST, id ASC
LIMIT 1
`)

	var (
		id       int
		proxyURL string
	)
	if err := row.Scan(&id, &proxyURL); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProxySelection{}, false, nil
		}

		if isMissingLegacyProxyRelation(err) {
			return ProxySelection{}, false, nil
		}
		return ProxySelection{}, false, err
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ProxySelection{}, false, nil
	}

	return ProxySelection{
		Selected: proxyURL,
		Source:   "proxy_pool",
		Note:     fmt.Sprintf("selected enabled proxy id=%d", id),
	}, true, nil
}

func (s *PostgresProxySelector) selectFromSettings(ctx context.Context) (ProxySelection, error) {
	if s == nil || s.settings == nil {
		return ProxySelection{
			Source: "unassigned",
			Note:   "proxy selector settings are not configured",
		}, nil
	}

	settings, err := s.settings.GetSettings(ctx, []string{
		"proxy.enabled",
		"proxy.type",
		"proxy.host",
		"proxy.port",
		"proxy.username",
		"proxy.password",
		"proxy.dynamic_enabled",
		"proxy.dynamic_api_url",
		"proxy.dynamic_api_key",
		"proxy.dynamic_api_key_header",
		"proxy.dynamic_result_field",
	})
	if err != nil {
		return ProxySelection{}, fmt.Errorf("load proxy settings: %w", err)
	}

	if parseBoolSetting(settings["proxy.dynamic_enabled"]) && strings.TrimSpace(settings["proxy.dynamic_api_url"]) != "" {
		proxyURL, dynamicErr := s.fetchDynamicProxy(ctx, settings)
		if proxyURL != "" {
			return ProxySelection{
				Selected: proxyURL,
				Source:   "dynamic_proxy",
			}, nil
		}

		if staticProxyURL := buildStaticProxyURL(settings); staticProxyURL != "" {
			note := "dynamic proxy unavailable; using static proxy"
			if dynamicErr != nil {
				note = note + ": " + dynamicErr.Error()
			}
			return ProxySelection{
				Selected: staticProxyURL,
				Source:   "settings_static",
				Note:     note,
			}, nil
		}

		note := "dynamic proxy unavailable"
		if dynamicErr != nil {
			note = note + ": " + dynamicErr.Error()
		}
		return ProxySelection{
			Source: "unassigned",
			Note:   note,
		}, nil
	}

	if staticProxyURL := buildStaticProxyURL(settings); staticProxyURL != "" {
		return ProxySelection{
			Selected: staticProxyURL,
			Source:   "settings_static",
		}, nil
	}

	return ProxySelection{
		Source: "unassigned",
		Note:   "no proxy configured",
	}, nil
}

func (s *PostgresProxySelector) fetchDynamicProxy(ctx context.Context, settings map[string]string) (string, error) {
	if s == nil {
		return "", nil
	}

	apiURL := strings.TrimSpace(settings["proxy.dynamic_api_url"])
	if apiURL == "" {
		return "", nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("build dynamic proxy request: %w", err)
	}

	if apiKey := strings.TrimSpace(settings["proxy.dynamic_api_key"]); apiKey != "" {
		headerName := strings.TrimSpace(settings["proxy.dynamic_api_key_header"])
		if headerName == "" {
			headerName = "X-API-Key"
		}
		request.Header.Set(headerName, apiKey)
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("request dynamic proxy: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request dynamic proxy: unexpected status %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read dynamic proxy response: %w", err)
	}

	proxyURL := parseDynamicProxyResponse(strings.TrimSpace(string(body)), strings.TrimSpace(settings["proxy.dynamic_result_field"]))
	proxyURL = normalizeDynamicProxyURL(proxyURL)
	if proxyURL == "" {
		return "", errors.New("dynamic proxy response missing proxy url")
	}
	return proxyURL, nil
}

func buildStaticProxyURL(settings map[string]string) string {
	if !parseBoolSetting(settings["proxy.enabled"]) {
		return ""
	}

	scheme := strings.TrimSpace(settings["proxy.type"])
	switch scheme {
	case "", "http":
		scheme = "http"
	case "socks5":
	default:
		return ""
	}

	host := strings.TrimSpace(settings["proxy.host"])
	if host == "" {
		return ""
	}
	portText := strings.TrimSpace(settings["proxy.port"])
	if portText == "" {
		return ""
	}
	if _, err := strconv.Atoi(portText); err != nil {
		return ""
	}

	hostPort := net.JoinHostPort(host, portText)
	staticURL := &url.URL{
		Scheme: scheme,
		Host:   hostPort,
	}
	username := strings.TrimSpace(settings["proxy.username"])
	password := strings.TrimSpace(settings["proxy.password"])
	if username != "" {
		if password != "" {
			staticURL.User = url.UserPassword(username, password)
		} else {
			staticURL.User = url.User(username)
		}
	}
	return staticURL.String()
}

func parseDynamicProxyResponse(raw string, resultField string) string {
	if raw == "" {
		return ""
	}

	if resultField != "" || strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var payload any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if resultField != "" {
				if value, ok := lookupJSONPath(payload, resultField); ok {
					return strings.TrimSpace(fmt.Sprintf("%v", value))
				}
				return ""
			}

			if value, ok := lookupDynamicProxyCandidate(payload); ok {
				return strings.TrimSpace(value)
			}
		}
	}

	return raw
}

func lookupJSONPath(payload any, path string) (any, bool) {
	current := payload
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, false
		}

		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func lookupDynamicProxyCandidate(payload any) (string, bool) {
	switch typed := payload.(type) {
	case map[string]any:
		for _, key := range []string{"proxy", "url", "proxy_url", "data", "ip"} {
			value, ok := typed[key]
			if !ok {
				continue
			}
			if proxyURL, ok := lookupDynamicProxyCandidate(value); ok {
				return proxyURL, true
			}
			if proxyURL := strings.TrimSpace(fmt.Sprintf("%v", value)); proxyURL != "" {
				return proxyURL, true
			}
		}
	case []any:
		for _, item := range typed {
			if proxyURL, ok := lookupDynamicProxyCandidate(item); ok {
				return proxyURL, true
			}
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			return typed, true
		}
	}
	return "", false
}

func normalizeDynamicProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "socks5://") {
		return raw
	}
	return "http://" + raw
}

func isMissingLegacyProxyRelation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "42P01" || pgErr.Code == "42703"
}
