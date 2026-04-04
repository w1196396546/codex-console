package registration

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrOutlookBatchServiceUnavailable  = errors.New("outlook batch service unavailable")
	ErrOutlookAccountSelectionRequired = errors.New("at least one outlook account is required")
	ErrInvalidOutlookInterval          = errors.New("outlook batch interval is invalid")
	ErrInvalidOutlookConcurrency       = errors.New("outlook batch concurrency must be between 1 and 50")
	ErrInvalidOutlookMode              = errors.New("outlook batch mode must be parallel or pipeline")
)

type RegisteredAccountRecord struct {
	ID           int
	Email        string
	RefreshToken string
}

type OutlookAccount struct {
	ID                     int    `json:"id"`
	Email                  string `json:"email"`
	Name                   string `json:"name"`
	HasOAuth               bool   `json:"has_oauth"`
	IsRegistered           bool   `json:"is_registered"`
	HasRefreshToken        bool   `json:"has_refresh_token"`
	NeedsTokenRefresh      bool   `json:"needs_token_refresh"`
	IsRegistrationComplete bool   `json:"is_registration_complete"`
	RegisteredAccountID    *int   `json:"registered_account_id"`
}

type OutlookAccountsListResponse struct {
	Total             int              `json:"total"`
	RegisteredCount   int              `json:"registered_count"`
	UnregisteredCount int              `json:"unregistered_count"`
	Accounts          []OutlookAccount `json:"accounts"`
}

type OutlookBatchStartRequest struct {
	ServiceIDs        []int  `json:"service_ids"`
	Proxy             string `json:"proxy,omitempty"`
	IntervalMin       int    `json:"interval_min,omitempty"`
	IntervalMax       int    `json:"interval_max,omitempty"`
	Concurrency       int    `json:"concurrency,omitempty"`
	Mode              string `json:"mode,omitempty"`
	AutoUploadCPA     bool   `json:"auto_upload_cpa,omitempty"`
	CPAServiceIDs     []int  `json:"cpa_service_ids,omitempty"`
	AutoUploadSub2API bool   `json:"auto_upload_sub2api,omitempty"`
	Sub2APIServiceIDs []int  `json:"sub2api_service_ids,omitempty"`
	AutoUploadTM      bool   `json:"auto_upload_tm,omitempty"`
	TMServiceIDs      []int  `json:"tm_service_ids,omitempty"`
}

type OutlookBatchStartResponse struct {
	BatchID    string `json:"batch_id"`
	Total      int    `json:"total"`
	Skipped    int    `json:"skipped"`
	ToRegister int    `json:"to_register"`
	ServiceIDs []int  `json:"service_ids"`
}

type OutlookBatchStatusResponse struct {
	BatchID       string   `json:"batch_id"`
	Count         int      `json:"count,omitempty"`
	Status        string   `json:"status"`
	Total         int      `json:"total"`
	Completed     int      `json:"completed"`
	Success       int      `json:"success"`
	Failed        int      `json:"failed"`
	Skipped       int      `json:"skipped"`
	Paused        bool     `json:"paused"`
	Cancelled     bool     `json:"cancelled"`
	Finished      bool     `json:"finished"`
	Logs          []string `json:"logs"`
	LogOffset     int      `json:"log_offset"`
	LogNextOffset int      `json:"log_next_offset"`
	Progress      string   `json:"progress"`
}

type outlookRepository interface {
	ListOutlookServices(ctx context.Context) ([]EmailServiceRecord, error)
	ListAccountsByEmails(ctx context.Context, emails []string) ([]RegisteredAccountRecord, error)
}

type outlookBatchMetadata struct {
	Skipped int
}

type OutlookService struct {
	repo    outlookRepository
	batches *BatchService

	mu          sync.RWMutex
	batchStates map[string]outlookBatchMetadata
}

func NewOutlookService(repo outlookRepository, batches *BatchService) *OutlookService {
	return &OutlookService{
		repo:        repo,
		batches:     batches,
		batchStates: make(map[string]outlookBatchMetadata),
	}
}

func (s *OutlookService) ListOutlookAccounts(ctx context.Context) (OutlookAccountsListResponse, error) {
	if s.repo == nil {
		return OutlookAccountsListResponse{Accounts: make([]OutlookAccount, 0)}, nil
	}

	services, err := s.repo.ListOutlookServices(ctx)
	if err != nil {
		return OutlookAccountsListResponse{}, err
	}

	emails := make([]string, 0, len(services))
	seen := make(map[string]struct{}, len(services))
	for _, service := range services {
		email := resolveOutlookServiceEmail(service)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}

	registeredAccounts := make(map[string]RegisteredAccountRecord, len(emails))
	if len(emails) > 0 {
		accountRows, err := s.repo.ListAccountsByEmails(ctx, emails)
		if err != nil {
			return OutlookAccountsListResponse{}, err
		}
		for _, account := range accountRows {
			registeredAccounts[strings.TrimSpace(account.Email)] = account
		}
	}

	response := OutlookAccountsListResponse{
		Accounts: make([]OutlookAccount, 0, len(services)),
	}
	for _, service := range services {
		email := resolveOutlookServiceEmail(service)
		account, found := registeredAccounts[email]
		hasRefreshToken := found && strings.TrimSpace(account.RefreshToken) != ""
		isRegistered := found
		isComplete := hasRefreshToken
		needsTokenRefresh := found && !isComplete

		var registeredAccountID *int
		if found {
			accountID := account.ID
			registeredAccountID = &accountID
			response.RegisteredCount++
		} else {
			response.UnregisteredCount++
		}

		response.Accounts = append(response.Accounts, OutlookAccount{
			ID:                     service.ID,
			Email:                  email,
			Name:                   service.Name,
			HasOAuth:               hasOutlookOAuth(service.Config),
			IsRegistered:           isRegistered,
			HasRefreshToken:        hasRefreshToken,
			NeedsTokenRefresh:      needsTokenRefresh,
			IsRegistrationComplete: isComplete,
			RegisteredAccountID:    registeredAccountID,
		})
	}
	response.Total = len(response.Accounts)

	return response, nil
}

func (s *OutlookService) StartOutlookBatch(ctx context.Context, req OutlookBatchStartRequest) (OutlookBatchStartResponse, error) {
	if s.batches == nil {
		return OutlookBatchStartResponse{}, ErrOutlookBatchServiceUnavailable
	}
	if len(req.ServiceIDs) == 0 {
		return OutlookBatchStartResponse{}, ErrOutlookAccountSelectionRequired
	}
	options, err := normalizeBatchExecutionOptions(req.IntervalMin, req.IntervalMax, req.Concurrency, req.Mode)
	if err != nil {
		switch err {
		case ErrInvalidBatchInterval:
			return OutlookBatchStartResponse{}, ErrInvalidOutlookInterval
		case ErrInvalidBatchConcurrency:
			return OutlookBatchStartResponse{}, ErrInvalidOutlookConcurrency
		case ErrInvalidBatchMode:
			return OutlookBatchStartResponse{}, ErrInvalidOutlookMode
		default:
			return OutlookBatchStartResponse{}, err
		}
	}

	requests := make([]StartRequest, 0, len(req.ServiceIDs))
	for _, serviceID := range req.ServiceIDs {
		id := serviceID
		requests = append(requests, StartRequest{
			EmailServiceType:  "outlook",
			Proxy:             req.Proxy,
			EmailServiceID:    &id,
			IntervalMin:       options.IntervalMin,
			IntervalMax:       options.IntervalMax,
			Concurrency:       options.Concurrency,
			Mode:              options.Mode,
			AutoUploadCPA:     req.AutoUploadCPA,
			CPAServiceIDs:     append([]int(nil), req.CPAServiceIDs...),
			AutoUploadSub2API: req.AutoUploadSub2API,
			Sub2APIServiceIDs: append([]int(nil), req.Sub2APIServiceIDs...),
			AutoUploadTM:      req.AutoUploadTM,
			TMServiceIDs:      append([]int(nil), req.TMServiceIDs...),
		})
	}

	batchResponse, err := s.batches.startBatchRequests(ctx, requests)
	if err != nil {
		return OutlookBatchStartResponse{}, err
	}

	s.mu.Lock()
	s.batchStates[batchResponse.BatchID] = outlookBatchMetadata{Skipped: 0}
	s.mu.Unlock()

	return OutlookBatchStartResponse{
		BatchID:    batchResponse.BatchID,
		Total:      len(req.ServiceIDs),
		Skipped:    0,
		ToRegister: len(req.ServiceIDs),
		ServiceIDs: append([]int(nil), req.ServiceIDs...),
	}, nil
}

func (s *OutlookService) GetOutlookBatch(ctx context.Context, batchID string, logOffset int) (OutlookBatchStatusResponse, error) {
	if s.batches == nil {
		return OutlookBatchStatusResponse{}, ErrOutlookBatchServiceUnavailable
	}

	status, err := s.batches.GetBatch(ctx, batchID, logOffset)
	if err != nil {
		return OutlookBatchStatusResponse{}, err
	}

	s.mu.RLock()
	metadata := s.batchStates[batchID]
	s.mu.RUnlock()

	return OutlookBatchStatusResponse{
		BatchID:       status.BatchID,
		Count:         status.Count,
		Status:        status.Status,
		Total:         status.Total,
		Completed:     status.Completed,
		Success:       status.Success,
		Failed:        status.Failed,
		Skipped:       metadata.Skipped,
		Paused:        status.Paused,
		Cancelled:     status.Cancelled,
		Finished:      status.Finished,
		Logs:          append([]string(nil), status.Logs...),
		LogOffset:     status.LogOffset,
		LogNextOffset: status.LogNextOffset,
		Progress:      status.Progress,
	}, nil
}

func resolveOutlookServiceEmail(service EmailServiceRecord) string {
	email := stringConfig(service.Config, "email")
	if email != "" {
		return email
	}
	return strings.TrimSpace(service.Name)
}

func hasOutlookOAuth(config map[string]any) bool {
	return stringConfig(config, "client_id") != "" && stringConfig(config, "refresh_token") != ""
}
