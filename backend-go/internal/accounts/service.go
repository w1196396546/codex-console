package accounts

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Repository interface {
	ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error)
	GetAccountByEmail(ctx context.Context, email string) (Account, bool, error)
	UpsertAccount(ctx context.Context, account Account) (Account, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListAccounts(ctx context.Context, req ListAccountsRequest) (AccountListResponse, error) {
	normalized := req.Normalized()
	if s == nil || s.repository == nil {
		return AccountListResponse{
			Page:     normalized.Page,
			PageSize: normalized.PageSize,
			Accounts: make([]Account, 0),
		}, nil
	}

	accounts, total, err := s.repository.ListAccounts(ctx, normalized)
	if err != nil {
		return AccountListResponse{}, err
	}

	return AccountListResponse{
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Total:    total,
		Accounts: accounts,
	}, nil
}

func (s *Service) UpsertAccount(ctx context.Context, req UpsertAccountRequest) (Account, error) {
	normalized, err := req.Normalized(time.Now().UTC())
	if err != nil {
		return Account{}, err
	}
	if s == nil || s.repository == nil {
		return Account{}, ErrRepositoryNotConfigured
	}

	existing, found, err := s.repository.GetAccountByEmail(ctx, normalized.Email)
	if err != nil {
		return Account{}, fmt.Errorf("lookup account by email: %w", err)
	}
	if !found && normalized.EmailService == "" {
		return Account{}, ErrAccountEmailServiceRequired
	}

	account := normalized.ToAccount()
	if found {
		account = mergeAccount(existing, normalized)
	}

	saved, err := s.repository.UpsertAccount(ctx, account)
	if err != nil {
		return Account{}, fmt.Errorf("upsert account: %w", err)
	}

	return saved, nil
}

func mergeAccount(existing Account, incoming UpsertAccountRequest) Account {
	merged := existing
	merged.Email = incoming.Email
	originalStatus := strings.TrimSpace(existing.Status)
	originalRefreshToken := strings.TrimSpace(existing.RefreshToken)

	mergeStringField(&merged.Password, incoming.Password)
	mergeStringField(&merged.ClientID, incoming.ClientID)
	mergeStringField(&merged.SessionToken, incoming.SessionToken)
	mergeStringField(&merged.EmailService, incoming.EmailService)
	mergeStringField(&merged.EmailServiceID, incoming.EmailServiceID)
	mergeStringField(&merged.AccountID, incoming.AccountID)
	mergeStringField(&merged.WorkspaceID, incoming.WorkspaceID)
	mergeStringField(&merged.AccessToken, incoming.AccessToken)
	mergeStringField(&merged.RefreshToken, incoming.RefreshToken)
	mergeStringField(&merged.IDToken, incoming.IDToken)
	mergeStringField(&merged.Cookies, incoming.Cookies)
	mergeStringField(&merged.ProxyUsed, incoming.ProxyUsed)
	mergeStringField(&merged.Status, incoming.Status)
	mergeStringField(&merged.Source, incoming.Source)
	mergeBoolField(&merged.CPAUploaded, incoming.CPAUploaded)
	mergeTimeField(&merged.CPAUploadedAt, incoming.CPAUploadedAt)
	mergeBoolField(&merged.Sub2APIUploaded, incoming.Sub2APIUploaded)
	mergeTimeField(&merged.Sub2APIUploadedAt, incoming.Sub2APIUploadedAt)

	extraData := cloneExtraData(existing.ExtraData)
	if len(incoming.ExtraData) > 0 {
		for key, value := range incoming.ExtraData {
			extraData[key] = value
		}
	}
	merged.ExtraData = extraData

	if shouldPreserveAccountStatus(originalStatus, originalRefreshToken, incoming.Status, incoming.RefreshToken) {
		merged.Status = originalStatus
		removeTemporaryAccountExtraData(merged.ExtraData)
	}

	if incoming.RegisteredAt != nil && shouldRefreshRegisteredAt(existing.Status, incoming.Status, existing.RegisteredAt) {
		merged.RegisteredAt = cloneTimePtr(incoming.RegisteredAt)
	}

	return merged
}

func mergeStringField(target *string, incoming string) {
	if incoming == "" {
		return
	}
	*target = incoming
}

func mergeBoolField(target *bool, incoming *bool) {
	if incoming == nil {
		return
	}
	*target = *incoming
}

func mergeTimeField(target **time.Time, incoming *time.Time) {
	if incoming == nil {
		return
	}
	*target = cloneTimePtr(incoming)
}

func shouldPreserveAccountStatus(originalStatus string, originalRefreshToken string, incomingStatus string, incomingRefreshToken string) bool {
	if strings.TrimSpace(originalRefreshToken) == "" || strings.TrimSpace(incomingRefreshToken) != "" {
		return false
	}
	if strings.TrimSpace(originalStatus) == "" {
		return false
	}

	switch strings.TrimSpace(incomingStatus) {
	case "token_pending", "login_incomplete":
		return true
	default:
		return false
	}
}

func shouldRefreshRegisteredAt(existingStatus string, incomingStatus string, existingRegisteredAt *time.Time) bool {
	if existingRegisteredAt == nil {
		return true
	}

	return strings.TrimSpace(existingStatus) == "failed" && strings.TrimSpace(incomingStatus) == "active"
}

func removeTemporaryAccountExtraData(extraData map[string]any) {
	if len(extraData) == 0 {
		return
	}

	delete(extraData, "token_pending")
	delete(extraData, "login_incomplete")
	delete(extraData, "account_status_reason")
}
