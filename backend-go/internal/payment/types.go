package payment

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
)

const (
	StatusLinkReady         = "link_ready"
	StatusOpened            = "opened"
	StatusWaitingUserAction = "waiting_user_action"
	StatusVerifying         = "verifying"
	StatusPaidPendingSync   = "paid_pending_sync"
	StatusCompleted         = "completed"
	StatusFailed            = "failed"
)

var (
	ErrBindCardTaskNotFound            = errors.New("payment: bind_card_task not found")
	ErrRepositoryNotConfigured         = errors.New("payment: repository not configured")
	ErrAccountsRepositoryNotConfigured = errors.New("payment: accounts repository not configured")
	ErrCheckoutLinkGeneratorMissing    = errors.New("payment: checkout link generator not configured")
	ErrBillingProfileGeneratorMissing  = errors.New("payment: billing profile generator not configured")
	ErrSessionAdapterMissing           = errors.New("payment: session adapter not configured")
	ErrSubscriptionCheckerMissing      = errors.New("payment: subscription checker not configured")
	ErrBrowserOpenerMissing            = errors.New("payment: browser opener not configured")
	ErrAutoBinderMissing               = errors.New("payment: auto binder not configured")
)

type statusError struct {
	status int
	detail string
}

func (e *statusError) Error() string {
	if e == nil {
		return ""
	}
	return e.detail
}

func (e *statusError) StatusCode() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	return e.status
}

func newStatusError(status int, detail string) error {
	return &statusError{status: status, detail: detail}
}

func StatusCode(err error) int {
	var target *statusError
	if errors.As(err, &target) {
		return target.StatusCode()
	}
	return http.StatusInternalServerError
}

func Detail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type BindCardTask struct {
	ID                int        `json:"id"`
	AccountID         int        `json:"account_id"`
	AccountEmail      string     `json:"account_email,omitempty"`
	PlanType          string     `json:"plan_type"`
	WorkspaceName     string     `json:"workspace_name,omitempty"`
	PriceInterval     string     `json:"price_interval,omitempty"`
	SeatQuantity      int        `json:"seat_quantity,omitempty"`
	Country           string     `json:"country,omitempty"`
	Currency          string     `json:"currency,omitempty"`
	CheckoutURL       string     `json:"checkout_url"`
	CheckoutSessionID string     `json:"checkout_session_id,omitempty"`
	PublishableKey    string     `json:"publishable_key,omitempty"`
	ClientSecret      string     `json:"client_secret,omitempty"`
	CheckoutSource    string     `json:"checkout_source,omitempty"`
	BindMode          string     `json:"bind_mode,omitempty"`
	Status            string     `json:"status"`
	LastError         string     `json:"last_error,omitempty"`
	OpenedAt          *time.Time `json:"opened_at,omitempty"`
	LastCheckedAt     *time.Time `json:"last_checked_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Repository interface {
	CreateBindCardTask(ctx context.Context, params CreateBindCardTaskParams) (BindCardTask, error)
	GetBindCardTask(ctx context.Context, taskID int) (BindCardTask, error)
	ListBindCardTasks(ctx context.Context, req ListBindCardTasksRequest) (ListBindCardTasksResponse, error)
	UpdateBindCardTask(ctx context.Context, task BindCardTask) (BindCardTask, error)
	DeleteBindCardTask(ctx context.Context, taskID int) error
}

type AccountsRepository interface {
	GetAccountByID(ctx context.Context, accountID int) (accounts.Account, error)
	ListAccountsBySelection(ctx context.Context, req accounts.AccountSelectionRequest) ([]accounts.Account, error)
	UpsertAccount(ctx context.Context, account accounts.Account) (accounts.Account, error)
}

type CheckoutLinkGenerator interface {
	GenerateCheckoutLink(ctx context.Context, account accounts.Account, req GenerateLinkRequest, proxy string) (CheckoutLinkResult, error)
}

type BillingProfileGenerator interface {
	GenerateRandomBillingProfile(ctx context.Context, country string, proxy string) (map[string]any, error)
}

type BrowserOpener interface {
	OpenIncognito(ctx context.Context, url string, cookies string) (bool, error)
}

type SessionAdapter interface {
	BootstrapSessionToken(ctx context.Context, account accounts.Account, proxy string) (SessionBootstrapResult, error)
	ProbeSession(ctx context.Context, account accounts.Account, proxy string) (*SessionProbeResult, error)
}

type SubscriptionChecker interface {
	CheckSubscription(ctx context.Context, account accounts.Account, proxy string, allowRefresh bool) (SubscriptionCheckDetail, error)
}

type AutoBinder interface {
	AutoBindThirdParty(ctx context.Context, task BindCardTask, account accounts.Account, req ThirdPartyAutoBindRequest) (AutoBindResult, error)
	AutoBindLocal(ctx context.Context, task BindCardTask, account accounts.Account, req LocalAutoBindRequest) (AutoBindResult, error)
}

type CreateBindCardTaskParams struct {
	AccountID         int
	PlanType          string
	WorkspaceName     string
	PriceInterval     string
	SeatQuantity      int
	Country           string
	Currency          string
	CheckoutURL       string
	CheckoutSessionID string
	PublishableKey    string
	ClientSecret      string
	CheckoutSource    string
	BindMode          string
	Status            string
	LastError         string
	OpenedAt          *time.Time
	LastCheckedAt     *time.Time
	CompletedAt       *time.Time
}

type ListBindCardTasksRequest struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Status   string `json:"status,omitempty"`
	Search   string `json:"search,omitempty"`
}

type ListBindCardTasksResponse struct {
	Total int            `json:"total"`
	Tasks []BindCardTask `json:"tasks"`
}

type CheckoutRequestBase struct {
	AccountID     int    `json:"account_id"`
	PlanType      string `json:"plan_type"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	PriceInterval string `json:"price_interval,omitempty"`
	SeatQuantity  int    `json:"seat_quantity,omitempty"`
	Country       string `json:"country,omitempty"`
	Currency      string `json:"currency,omitempty"`
	Proxy         string `json:"proxy,omitempty"`
	AutoOpen      bool   `json:"auto_open,omitempty"`
}

type GenerateLinkRequest struct {
	CheckoutRequestBase
}

type CreateBindCardTaskRequest struct {
	CheckoutRequestBase
	BindMode string `json:"bind_mode,omitempty"`
}

type OpenIncognitoRequest struct {
	URL       string `json:"url"`
	AccountID int    `json:"account_id,omitempty"`
}

type SaveSessionTokenRequest struct {
	SessionToken string `json:"session_token"`
	MergeCookie  bool   `json:"merge_cookie"`
}

type SyncBindCardTaskRequest struct {
	Proxy string `json:"proxy,omitempty"`
}

type MarkUserActionRequest struct {
	Proxy           string `json:"proxy,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
}

type ThirdPartyCardRequest struct {
	Number   string `json:"number"`
	ExpMonth string `json:"exp_month"`
	ExpYear  string `json:"exp_year"`
	CVC      string `json:"cvc"`
}

type ThirdPartyProfileRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Country string `json:"country,omitempty"`
	Line1   string `json:"line1,omitempty"`
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Postal  string `json:"postal,omitempty"`
}

type ThirdPartyAutoBindRequest struct {
	APIURL                        string                   `json:"api_url,omitempty"`
	APIKey                        string                   `json:"api_key,omitempty"`
	Proxy                         string                   `json:"proxy,omitempty"`
	TimeoutSeconds                int                      `json:"timeout_seconds,omitempty"`
	IntervalSeconds               int                      `json:"interval_seconds,omitempty"`
	ThirdPartyPollTimeoutSeconds  int                      `json:"third_party_poll_timeout_seconds,omitempty"`
	ThirdPartyPollIntervalSeconds int                      `json:"third_party_poll_interval_seconds,omitempty"`
	Card                          ThirdPartyCardRequest    `json:"card"`
	Profile                       ThirdPartyProfileRequest `json:"profile"`
}

type LocalAutoBindRequest struct {
	Proxy                 string                   `json:"proxy,omitempty"`
	BrowserTimeoutSeconds int                      `json:"browser_timeout_seconds,omitempty"`
	PostSubmitWaitSeconds int                      `json:"post_submit_wait_seconds,omitempty"`
	VerifyTimeoutSeconds  int                      `json:"verify_timeout_seconds,omitempty"`
	VerifyIntervalSeconds int                      `json:"verify_interval_seconds,omitempty"`
	Headless              bool                     `json:"headless,omitempty"`
	Card                  ThirdPartyCardRequest    `json:"card"`
	Profile               ThirdPartyProfileRequest `json:"profile"`
}

type MarkSubscriptionRequest struct {
	SubscriptionType string `json:"subscription_type"`
}

type BatchCheckSubscriptionRequest struct {
	IDs                     []int  `json:"ids"`
	Proxy                   string `json:"proxy,omitempty"`
	SelectAll               bool   `json:"select_all"`
	StatusFilter            string `json:"status_filter,omitempty"`
	EmailServiceFilter      string `json:"email_service_filter,omitempty"`
	SearchFilter            string `json:"search_filter,omitempty"`
	RefreshTokenStateFilter string `json:"refresh_token_state_filter,omitempty"`
}

type RandomBillingResponse struct {
	Success bool           `json:"success"`
	Profile map[string]any `json:"profile,omitempty"`
}

type CheckoutLinkResult struct {
	Link              string
	Source            string
	FallbackReason    string
	CheckoutSessionID string
	PublishableKey    string
	ClientSecret      string
}

type GenerateLinkResponse struct {
	Success            bool   `json:"success"`
	Link               string `json:"link,omitempty"`
	IsOfficialCheckout bool   `json:"is_official_checkout"`
	PlanType           string `json:"plan_type,omitempty"`
	Country            string `json:"country,omitempty"`
	Currency           string `json:"currency,omitempty"`
	AutoOpened         bool   `json:"auto_opened"`
	Source             string `json:"source,omitempty"`
	FallbackReason     string `json:"fallback_reason,omitempty"`
	CheckoutSessionID  string `json:"checkout_session_id,omitempty"`
	PublishableKey     string `json:"publishable_key,omitempty"`
	HasClientSecret    bool   `json:"has_client_secret"`
}

type CreateBindCardTaskResponse struct {
	Success            bool         `json:"success"`
	Task               BindCardTask `json:"task"`
	Link               string       `json:"link,omitempty"`
	IsOfficialCheckout bool         `json:"is_official_checkout"`
	Source             string       `json:"source,omitempty"`
	FallbackReason     string       `json:"fallback_reason,omitempty"`
	AutoOpened         bool         `json:"auto_opened"`
	CheckoutSessionID  string       `json:"checkout_session_id,omitempty"`
	PublishableKey     string       `json:"publishable_key,omitempty"`
	HasClientSecret    bool         `json:"has_client_secret"`
}

type OpenIncognitoResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type SessionProbeResult struct {
	OK                       bool   `json:"ok"`
	HTTPStatus               int    `json:"http_status,omitempty"`
	SessionTokenFound        bool   `json:"session_token_found"`
	AccessTokenInSessionJSON bool   `json:"access_token_in_session_json"`
	SessionTokenPreview      string `json:"session_token_preview,omitempty"`
	AccessTokenPreview       string `json:"access_token_preview,omitempty"`
	Error                    string `json:"error,omitempty"`
}

type SessionDiagnosticPayload struct {
	AccountID           int                 `json:"account_id"`
	Email               string              `json:"email"`
	TokenState          map[string]any      `json:"token_state"`
	CookieState         map[string]any      `json:"cookie_state"`
	BootstrapCapability map[string]any      `json:"bootstrap_capability"`
	Probe               *SessionProbeResult `json:"probe,omitempty"`
	Notes               []string            `json:"notes"`
	Recommendation      string              `json:"recommendation"`
	CheckedAt           string              `json:"checked_at"`
}

type SessionDiagnosticResponse struct {
	Success    bool                     `json:"success"`
	Diagnostic SessionDiagnosticPayload `json:"diagnostic"`
}

type SessionBootstrapResult struct {
	SessionToken string
	AccessToken  string
	Cookies      string
}

type SessionBootstrapResponse struct {
	Success             bool   `json:"success"`
	Message             string `json:"message"`
	AccountID           int    `json:"account_id"`
	Email               string `json:"email"`
	SessionTokenLen     int    `json:"session_token_len,omitempty"`
	SessionTokenPreview string `json:"session_token_preview,omitempty"`
}

type SaveSessionTokenResponse struct {
	Success             bool   `json:"success"`
	AccountID           int    `json:"account_id"`
	Email               string `json:"email"`
	SessionTokenLen     int    `json:"session_token_len"`
	SessionTokenPreview string `json:"session_token_preview"`
	Message             string `json:"message"`
}

type SubscriptionCheckDetail struct {
	Status         string         `json:"status"`
	Source         string         `json:"source,omitempty"`
	Confidence     string         `json:"confidence,omitempty"`
	Note           string         `json:"note,omitempty"`
	RefreshedToken bool           `json:"token_refreshed,omitempty"`
	Extra          map[string]any `json:"-"`
}

func (d SubscriptionCheckDetail) Map() map[string]any {
	payload := map[string]any{
		"status":          d.Status,
		"source":          d.Source,
		"confidence":      d.Confidence,
		"note":            d.Note,
		"token_refreshed": d.RefreshedToken,
	}
	for key, value := range d.Extra {
		payload[key] = value
	}
	return payload
}

type SyncBindCardTaskResponse struct {
	Success          bool           `json:"success"`
	Verified         bool           `json:"verified,omitempty"`
	Checks           int            `json:"checks,omitempty"`
	SubscriptionType string         `json:"subscription_type"`
	Detail           map[string]any `json:"detail,omitempty"`
	TokenRefreshUsed bool           `json:"token_refresh_used,omitempty"`
	Task             BindCardTask   `json:"task"`
	AccountID        int            `json:"account_id"`
	AccountEmail     string         `json:"account_email"`
}

type BindCardTaskActionResponse struct {
	Success bool         `json:"success"`
	Task    BindCardTask `json:"task"`
}

type AutoBindResult struct {
	Verified         bool           `json:"verified,omitempty"`
	PaidConfirmed    bool           `json:"paid_confirmed,omitempty"`
	Pending          bool           `json:"pending,omitempty"`
	NeedUserAction   bool           `json:"need_user_action,omitempty"`
	SubscriptionType string         `json:"subscription_type,omitempty"`
	Detail           map[string]any `json:"detail,omitempty"`
	ThirdParty       map[string]any `json:"third_party,omitempty"`
	LocalAuto        map[string]any `json:"local_auto,omitempty"`
	Task             BindCardTask   `json:"task,omitempty"`
	AccountID        int            `json:"account_id,omitempty"`
	AccountEmail     string         `json:"account_email,omitempty"`
}

type DeleteBindCardTaskResponse struct {
	Success bool `json:"success"`
	TaskID  int  `json:"task_id"`
}

type BatchCheckSubscriptionDetail struct {
	ID               int    `json:"id"`
	Email            string `json:"email,omitempty"`
	Success          bool   `json:"success"`
	SubscriptionType string `json:"subscription_type,omitempty"`
	Confidence       string `json:"confidence,omitempty"`
	Source           string `json:"source,omitempty"`
	TokenRefreshed   bool   `json:"token_refreshed,omitempty"`
	Error            string `json:"error,omitempty"`
}

type BatchCheckSubscriptionResponse struct {
	SuccessCount int                            `json:"success_count"`
	FailedCount  int                            `json:"failed_count"`
	Details      []BatchCheckSubscriptionDetail `json:"details"`
}

type MarkSubscriptionResponse struct {
	Success          bool   `json:"success"`
	SubscriptionType string `json:"subscription_type"`
}
