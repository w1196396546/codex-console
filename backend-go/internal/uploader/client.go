package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type SendRequest struct {
	Service  ServiceConfig
	Accounts []UploadAccount
	Sub2API  Sub2APIBatchOptions
}

type httpClient struct {
	doer HTTPDoer
}

func newHTTPClient(doer HTTPDoer) *httpClient {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &httpClient{doer: doer}
}

func (c *httpClient) do(req *http.Request) (int, []byte, error) {
	resp, err := c.doer.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func newJSONRequest(ctx context.Context, method string, rawURL string, payload any, headers map[string]string) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

func newMultipartFileRequest(ctx context.Context, method string, rawURL string, fieldName string, file CPAAuthFile, headers map[string]string) (*http.Request, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile(fieldName, file.Filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(file.Content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

func newRawJSONRequest(ctx context.Context, method string, rawURL string, file CPAAuthFile, headers map[string]string) (*http.Request, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := parsed.Query()
	query.Set("name", file.Filename)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(file.Content))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", file.ContentType)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

func parseUploadResponse(statusCode int, body []byte, successMessage string) (bool, string) {
	if statusCode == http.StatusOK || statusCode == http.StatusCreated {
		return true, successMessage
	}

	message := fmt.Sprintf("upload failed: HTTP %d", statusCode)
	if len(body) == 0 {
		return false, message
	}

	var detail struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &detail); err == nil && strings.TrimSpace(detail.Message) != "" {
		return false, detail.Message
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		return false, message
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return false, message + " - " + text
}

func joinURLPath(baseURL string, path string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
}

func normalizeCPAAuthFilesURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lowerURL := strings.ToLower(normalized)

	switch {
	case normalized == "":
		return ""
	case strings.HasSuffix(lowerURL, "/auth-files"):
		return normalized
	case strings.HasSuffix(lowerURL, "/v0/management"), strings.HasSuffix(lowerURL, "/management"):
		return normalized + "/auth-files"
	case strings.HasSuffix(lowerURL, "/v0"):
		return normalized + "/management/auth-files"
	default:
		return normalized + "/v0/management/auth-files"
	}
}
