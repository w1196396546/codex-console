package accounts

import "context"

type Repository interface {
	ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error)
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
