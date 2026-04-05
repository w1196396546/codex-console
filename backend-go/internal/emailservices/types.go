package emailservices

import (
	"errors"
	"time"
)

const (
	ServiceTypeTempmail  = "tempmail"
	ServiceTypeYYDSMail  = "yyds_mail"
	ServiceTypeOutlook   = "outlook"
	ServiceTypeMoeMail   = "moe_mail"
	ServiceTypeTempMail  = "temp_mail"
	ServiceTypeDuckMail  = "duck_mail"
	ServiceTypeFreemail  = "freemail"
	ServiceTypeIMAPMail  = "imap_mail"
	ServiceTypeCloudmail = "cloudmail"
	ServiceTypeLuckmail  = "luckmail"
)

var (
	ErrServiceNotFound      = errors.New("service not found")
	ErrDuplicateServiceName = errors.New("duplicate service name")
	ErrInvalidServiceType   = errors.New("invalid service type")
)

type ListServicesRequest struct {
	ServiceType string
	EnabledOnly bool
}

type EmailServiceRecord struct {
	ID          int
	ServiceType string
	Name        string
	Config      map[string]any
	Enabled     bool
	Priority    int
	LastUsed    *time.Time
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

type RegisteredAccountRecord struct {
	ID    int
	Email string
}

type EmailServiceResponse struct {
	ID                  int            `json:"id"`
	ServiceType         string         `json:"service_type"`
	Name                string         `json:"name"`
	Enabled             bool           `json:"enabled"`
	Priority            int            `json:"priority"`
	Config              map[string]any `json:"config,omitempty"`
	RegistrationStatus  string         `json:"registration_status,omitempty"`
	RegisteredAccountID *int           `json:"registered_account_id,omitempty"`
	LastUsed            string         `json:"last_used,omitempty"`
	CreatedAt           string         `json:"created_at,omitempty"`
	UpdatedAt           string         `json:"updated_at,omitempty"`
}

type EmailServiceListResponse struct {
	Total    int                    `json:"total"`
	Services []EmailServiceResponse `json:"services"`
}

type EmailServiceFullResponse struct {
	ID          int            `json:"id"`
	ServiceType string         `json:"service_type"`
	Name        string         `json:"name"`
	Enabled     bool           `json:"enabled"`
	Priority    int            `json:"priority"`
	Config      map[string]any `json:"config"`
	LastUsed    string         `json:"last_used,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
}

type StatsResponse struct {
	OutlookCount      int  `json:"outlook_count"`
	CustomCount       int  `json:"custom_count"`
	YYDSMailCount     int  `json:"yyds_mail_count"`
	TempMailCount     int  `json:"temp_mail_count"`
	DuckMailCount     int  `json:"duck_mail_count"`
	FreemailCount     int  `json:"freemail_count"`
	IMAPMailCount     int  `json:"imap_mail_count"`
	CloudmailCount    int  `json:"cloudmail_count"`
	LuckmailCount     int  `json:"luckmail_count"`
	TempmailAvailable bool `json:"tempmail_available"`
	YYDSMailAvailable bool `json:"yyds_mail_available"`
	EnabledCount      int  `json:"enabled_count"`
}

type ServiceTypeFieldDefinition struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

type ServiceTypeDefinition struct {
	Value        string                       `json:"value"`
	Label        string                       `json:"label"`
	Description  string                       `json:"description"`
	ConfigFields []ServiceTypeFieldDefinition `json:"config_fields"`
}

type ServiceTypesResponse struct {
	Types []ServiceTypeDefinition `json:"types"`
}
