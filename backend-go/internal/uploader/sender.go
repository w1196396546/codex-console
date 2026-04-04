package uploader

import (
	"context"
	"fmt"
	"net/http"
)

type Sender interface {
	Kind() UploadKind
	Send(ctx context.Context, req SendRequest) ([]UploadResult, error)
}

func NewSender(kind UploadKind, doer HTTPDoer) (Sender, error) {
	switch kind {
	case UploadKindCPA:
		return NewCPASender(doer), nil
	case UploadKindSub2API:
		return NewSub2APISender(doer), nil
	case UploadKindTM:
		return NewTMSender(doer), nil
	default:
		return nil, ErrUploadKindInvalid
	}
}

type cpaSender struct {
	client *httpClient
}

func NewCPASender(doer HTTPDoer) Sender {
	return &cpaSender{client: newHTTPClient(doer)}
}

func (s *cpaSender) Kind() UploadKind {
	return UploadKindCPA
}

func (s *cpaSender) Send(ctx context.Context, req SendRequest) ([]UploadResult, error) {
	service, err := validateSendRequest(req, UploadKindCPA)
	if err != nil {
		return nil, err
	}

	uploadURL := normalizeCPAAuthFilesURL(service.BaseURL)
	headers := map[string]string{
		"Authorization": "Bearer " + service.Credential,
	}

	results := make([]UploadResult, 0, len(req.Accounts))
	for _, account := range req.Accounts {
		file, err := BuildCPAAuthFile(account)
		if err != nil {
			return nil, err
		}

		httpReq, err := newMultipartFileRequest(ctx, http.MethodPost, uploadURL, "file", file, headers)
		if err != nil {
			return nil, err
		}

		statusCode, body, err := s.client.do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("send cpa upload request: %w", err)
		}

		success, message := parseUploadResponse(statusCode, body, "上传成功")
		if !success && (statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed || statusCode == http.StatusUnsupportedMediaType) {
			fallbackReq, err := newRawJSONRequest(ctx, http.MethodPost, uploadURL, file, headers)
			if err != nil {
				return nil, err
			}
			statusCode, body, err = s.client.do(fallbackReq)
			if err != nil {
				return nil, fmt.Errorf("send cpa fallback request: %w", err)
			}
			success, message = parseUploadResponse(statusCode, body, "上传成功")
		}

		normalizedAccount := account.Normalized()
		results = append(results, UploadResult{
			Kind:         UploadKindCPA,
			ServiceID:    service.ID,
			AccountEmail: normalizedAccount.Email,
			Success:      success,
			Message:      message,
		})
	}

	return results, nil
}

type sub2apiSender struct {
	client *httpClient
}

func NewSub2APISender(doer HTTPDoer) Sender {
	return &sub2apiSender{client: newHTTPClient(doer)}
}

func (s *sub2apiSender) Kind() UploadKind {
	return UploadKindSub2API
}

func (s *sub2apiSender) Send(ctx context.Context, req SendRequest) ([]UploadResult, error) {
	service, err := validateSendRequest(req, UploadKindSub2API)
	if err != nil {
		return nil, err
	}

	targetAccounts := accountsWithAccessToken(req.Accounts)
	payload, err := BuildSub2APIBatchPayload(service, req.Accounts, req.Sub2API)
	if err != nil {
		return nil, err
	}

	httpReq, err := newJSONRequest(ctx, http.MethodPost, joinURLPath(service.BaseURL, "/api/v1/admin/accounts/data"), payload, map[string]string{
		"x-api-key":       service.Credential,
		"Idempotency-Key": "import-" + payload.Data.ExportedAt,
	})
	if err != nil {
		return nil, err
	}

	statusCode, body, err := s.client.do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send sub2api upload request: %w", err)
	}

	success, message := parseUploadResponse(statusCode, body, fmt.Sprintf("成功上传 %d 个账号", len(targetAccounts)))
	return buildUploadResults(UploadKindSub2API, service.ID, targetAccounts, success, message), nil
}

type tmSender struct {
	client *httpClient
}

func NewTMSender(doer HTTPDoer) Sender {
	return &tmSender{client: newHTTPClient(doer)}
}

func (s *tmSender) Kind() UploadKind {
	return UploadKindTM
}

func (s *tmSender) Send(ctx context.Context, req SendRequest) ([]UploadResult, error) {
	service, err := validateSendRequest(req, UploadKindTM)
	if err != nil {
		return nil, err
	}

	var (
		payload        any
		targetAccounts []UploadAccount
		successMessage = "上传成功"
	)

	if len(req.Accounts) == 1 {
		payload, err = BuildTMSinglePayload(req.Accounts[0])
		if err != nil {
			return nil, err
		}
		targetAccounts = append(targetAccounts, req.Accounts[0].Normalized())
	} else {
		payload, err = BuildTMBatchPayload(req.Accounts)
		if err != nil {
			return nil, err
		}
		targetAccounts = accountsWithAccessToken(req.Accounts)
		successMessage = "批量上传成功"
	}

	httpReq, err := newJSONRequest(ctx, http.MethodPost, joinURLPath(service.BaseURL, "/admin/teams/import"), payload, map[string]string{
		"X-API-Key": service.Credential,
	})
	if err != nil {
		return nil, err
	}

	statusCode, body, err := s.client.do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send tm upload request: %w", err)
	}

	success, message := parseUploadResponse(statusCode, body, successMessage)
	return buildUploadResults(UploadKindTM, service.ID, targetAccounts, success, message), nil
}

func validateSendRequest(req SendRequest, expectedKind UploadKind) (ServiceConfig, error) {
	service := req.Service.Normalized()
	if service.Kind != expectedKind {
		return ServiceConfig{}, ErrUploadKindInvalid
	}
	if service.BaseURL == "" {
		return ServiceConfig{}, ErrUploadServiceBaseURLEmpty
	}
	if service.Credential == "" {
		return ServiceConfig{}, ErrUploadCredentialMissing
	}
	if len(req.Accounts) == 0 {
		return ServiceConfig{}, ErrUploadAccountsEmpty
	}
	return service, nil
}

func accountsWithAccessToken(accounts []UploadAccount) []UploadAccount {
	filtered := make([]UploadAccount, 0, len(accounts))
	for _, account := range accounts {
		normalized := account.Normalized()
		if normalized.AccessToken == "" {
			continue
		}
		filtered = append(filtered, normalized)
	}
	return filtered
}

func buildUploadResults(kind UploadKind, serviceID int, accounts []UploadAccount, success bool, message string) []UploadResult {
	results := make([]UploadResult, 0, len(accounts))
	for _, account := range accounts {
		results = append(results, UploadResult{
			Kind:         kind,
			ServiceID:    serviceID,
			AccountEmail: account.Email,
			Success:      success,
			Message:      message,
		})
	}
	return results
}
