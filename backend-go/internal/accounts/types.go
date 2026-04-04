package accounts

import "time"

const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 100
)

type Account struct {
	ID           int        `json:"id"`
	Email        string     `json:"email"`
	Password     string     `json:"password"`
	Status       string     `json:"status"`
	RegisteredAt *time.Time `json:"registered_at,omitempty"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

type ListAccountsRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type AccountListResponse struct {
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int       `json:"total"`
	Accounts []Account `json:"accounts"`
}

func (r ListAccountsRequest) Normalized() ListAccountsRequest {
	normalized := r
	if normalized.Page <= 0 {
		normalized.Page = DefaultPage
	}
	if normalized.PageSize <= 0 {
		normalized.PageSize = DefaultPageSize
	}
	if normalized.PageSize > MaxPageSize {
		normalized.PageSize = MaxPageSize
	}

	return normalized
}

func (r ListAccountsRequest) Offset() int {
	normalized := r.Normalized()
	return (normalized.Page - 1) * normalized.PageSize
}
