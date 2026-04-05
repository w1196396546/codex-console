package team

import (
	"context"
	"time"
)

type AccountRecord struct {
	ID          int64
	Email       string
	Status      string
	AccessToken string
}

type TeamRecord struct {
	ID                  int64
	OwnerAccountID      int64
	UpstreamTeamID      string
	UpstreamAccountID   string
	TeamName            string
	PlanType            string
	SubscriptionPlan    string
	AccountRoleSnapshot string
	Status              string
	CurrentMembers      int
	MaxMembers          *int
	SeatsAvailable      *int
	ExpiresAt           *time.Time
	LastSyncAt          *time.Time
	SyncStatus          string
	SyncError           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type TeamMembershipRecord struct {
	ID               int64
	TeamID           int64
	LocalAccountID   *int64
	MemberEmail      string
	UpstreamUserID   string
	MemberRole       string
	MembershipStatus string
	InvitedAt        *time.Time
	JoinedAt         *time.Time
	RemovedAt        *time.Time
	LastSeenAt       *time.Time
	Source           string
	SyncError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamTaskRecord struct {
	ID             int64
	TeamID         *int64
	OwnerAccountID *int64
	TaskUUID       string
	ScopeType      string
	ScopeID        string
	ActiveScopeKey *string
	TaskType       string
	Status         string
	RequestPayload map[string]any
	ResultPayload  map[string]any
	ErrorMessage   string
	Logs           string
	CreatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	UpdatedAt      time.Time
}

type TeamTaskItemRecord struct {
	ID           int64
	TaskID       int64
	TargetEmail  string
	ItemStatus   string
	Before       map[string]any
	After        map[string]any
	Message      string
	ErrorMessage string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	UpdatedAt    time.Time
}

type ListTeamsRequest struct {
	Page           int
	PerPage        int
	Status         string
	OwnerAccountID int64
	Search         string
}

type TeamListItem struct {
	ID                  int64      `json:"id"`
	OwnerAccountID      int64      `json:"owner_account_id"`
	OwnerEmail          string     `json:"owner_email"`
	UpstreamAccountID   string     `json:"upstream_account_id"`
	TeamName            string     `json:"team_name"`
	AccountRoleSnapshot string     `json:"account_role_snapshot"`
	Status              string     `json:"status"`
	CurrentMembers      int        `json:"current_members"`
	MaxMembers          *int       `json:"max_members"`
	SeatsAvailable      *int       `json:"seats_available"`
	ExpiresAt           *time.Time `json:"expires_at"`
	LastSyncAt          *time.Time `json:"last_sync_at"`
	SyncStatus          string     `json:"sync_status"`
}

type ListTeamsResponse struct {
	Items   []TeamListItem `json:"items"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}

type TeamDetailResponse struct {
	TeamListItem
	ActiveMemberCount   int    `json:"active_member_count"`
	JoinedCount         int    `json:"joined_count"`
	InvitedCount        int    `json:"invited_count"`
	LocalMemberCount    int    `json:"local_member_count"`
	ExternalMemberCount int    `json:"external_member_count"`
	LastSyncError       string `json:"last_sync_error"`
	ActiveTaskCount     int    `json:"active_task_count"`
}

type ListMembershipsRequest struct {
	TeamID  int64
	Status  string
	Binding string
	Search  string
}

type TeamMembershipItem struct {
	ID                 int64      `json:"id"`
	MemberEmail        string     `json:"member_email"`
	LocalAccountID     *int64     `json:"local_account_id"`
	LocalAccountStatus string     `json:"local_account_status"`
	MemberRole         string     `json:"member_role"`
	MembershipStatus   string     `json:"membership_status"`
	UpstreamUserID     string     `json:"upstream_user_id"`
	InvitedAt          *time.Time `json:"invited_at"`
	JoinedAt           *time.Time `json:"joined_at"`
	LastSeenAt         *time.Time `json:"last_seen_at"`
}

type ListMembershipsResponse struct {
	Items []TeamMembershipItem `json:"items"`
	Total int                  `json:"total"`
}

type ListTasksRequest struct {
	TeamID *int64
}

type TeamTaskListItem struct {
	TaskUUID       string     `json:"task_uuid"`
	TaskType       string     `json:"task_type"`
	Status         string     `json:"status"`
	TeamID         *int64     `json:"team_id"`
	OwnerAccountID *int64     `json:"owner_account_id"`
	CreatedAt      *time.Time `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

type ListTasksResponse struct {
	Items []TeamTaskListItem `json:"items"`
	Total int                `json:"total"`
}

type TeamTaskDetailItem struct {
	TargetEmail          string `json:"target_email"`
	ItemStatus           string `json:"item_status"`
	RelationStatusBefore string `json:"relation_status_before"`
	RelationStatusAfter  string `json:"relation_status_after"`
	Message              string `json:"message"`
	ErrorMessage         string `json:"error_message"`
}

type TeamTaskDetailResponse struct {
	TaskUUID       string               `json:"task_uuid"`
	TaskType       string               `json:"task_type"`
	Status         string               `json:"status"`
	TeamID         *int64               `json:"team_id"`
	OwnerAccountID *int64               `json:"owner_account_id"`
	CreatedAt      *time.Time           `json:"created_at"`
	StartedAt      *time.Time           `json:"started_at"`
	CompletedAt    *time.Time           `json:"completed_at"`
	Logs           []string             `json:"logs"`
	GuardLogs      []string             `json:"guard_logs"`
	Summary        map[string]any       `json:"summary"`
	Items          []TeamTaskDetailItem `json:"items"`
}

type ApplyMembershipActionRequest struct {
	MembershipID int64
	Action       string
	AccountID    *int64
}

type MembershipActionResult struct {
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	TeamID          *int64 `json:"team_id"`
	MembershipID    int64  `json:"membership_id"`
	NextStatus      string `json:"next_status"`
	RefreshRequired bool   `json:"refresh_required"`
	ErrorCode       int    `json:"error_code,omitempty"`
	AccountID       *int64 `json:"account_id,omitempty"`
}

type MembershipGatewayRevokeInviteParams struct {
	TeamUpstreamAccountID string
	OwnerAccessToken      string
	MemberEmail           string
}

type MembershipGatewayRemoveMemberParams struct {
	TeamUpstreamAccountID string
	OwnerAccessToken      string
	UpstreamUserID        string
}

type MembershipGateway interface {
	RevokeInvite(ctx context.Context, params MembershipGatewayRevokeInviteParams) error
	RemoveMember(ctx context.Context, params MembershipGatewayRemoveMemberParams) error
}
