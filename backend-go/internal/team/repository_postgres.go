package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListTeams(ctx context.Context, req ListTeamsRequest) ([]TeamRecord, int, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	perPage := req.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	where, args := buildTeamListWhere(req)
	countSQL := "SELECT COUNT(*) FROM teams" + where
	var total int
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count teams: %w", err)
	}

	listArgs := append(append([]any(nil), args...), perPage, (page-1)*perPage)
	listSQL := `
SELECT id, owner_account_id, upstream_team_id, upstream_account_id, team_name, plan_type,
       subscription_plan, account_role_snapshot, status, current_members, max_members,
       seats_available, expires_at, last_sync_at, sync_status, sync_error, created_at, updated_at
FROM teams` + where + `
ORDER BY updated_at DESC
LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := r.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	records := make([]TeamRecord, 0)
	for rows.Next() {
		record, scanErr := scanTeamRecord(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate teams: %w", err)
	}
	return records, total, nil
}

func (r *PostgresRepository) GetTeam(ctx context.Context, teamID int64) (TeamRecord, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, owner_account_id, upstream_team_id, upstream_account_id, team_name, plan_type,
       subscription_plan, account_role_snapshot, status, current_members, max_members,
       seats_available, expires_at, last_sync_at, sync_status, sync_error, created_at, updated_at
FROM teams
WHERE id = $1`, teamID)
	record, err := scanTeamRecord(row)
	if err != nil {
		return TeamRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) GetAccount(ctx context.Context, accountID int64) (AccountRecord, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, email, status, COALESCE(access_token, '')
FROM accounts
WHERE id = $1`, accountID)
	record, err := scanAccountRecord(row)
	if err != nil {
		return AccountRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) ListAccountsByIDs(ctx context.Context, accountIDs []int64) (map[int64]AccountRecord, error) {
	result := make(map[int64]AccountRecord, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
SELECT id, email, status, COALESCE(access_token, '')
FROM accounts
WHERE id = ANY($1)`, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		record, scanErr := scanAccountRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListAccountsByEmails(ctx context.Context, emails []string) (map[string]AccountRecord, error) {
	result := make(map[string]AccountRecord, len(emails))
	normalizedEmails := make([]string, 0, len(emails))
	seen := map[string]struct{}{}
	for _, email := range emails {
		normalized := normalizeEmail(email)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		normalizedEmails = append(normalizedEmails, normalized)
	}
	if len(normalizedEmails) == 0 {
		return result, nil
	}

	rows, err := r.db.Query(ctx, `
SELECT id, email, status, COALESCE(access_token, '')
FROM accounts
WHERE LOWER(BTRIM(email)) = ANY($1)`, normalizedEmails)
	if err != nil {
		return nil, fmt.Errorf("list accounts by email: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		record, scanErr := scanAccountRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[normalizeEmail(record.Email)] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts by email: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListMembershipsByTeam(ctx context.Context, teamID int64) ([]TeamMembershipRecord, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, team_id, local_account_id, member_email, upstream_user_id, member_role,
       membership_status, invited_at, joined_at, removed_at, last_seen_at, source,
       sync_error, created_at, updated_at
FROM team_memberships
WHERE team_id = $1
ORDER BY updated_at DESC, id DESC`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()

	records := make([]TeamMembershipRecord, 0)
	for rows.Next() {
		record, scanErr := scanMembershipRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memberships: %w", err)
	}
	return records, nil
}

func (r *PostgresRepository) GetMembership(ctx context.Context, membershipID int64) (TeamMembershipRecord, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, team_id, local_account_id, member_email, upstream_user_id, member_role,
       membership_status, invited_at, joined_at, removed_at, last_seen_at, source,
       sync_error, created_at, updated_at
FROM team_memberships
WHERE id = $1`, membershipID)
	record, err := scanMembershipRecord(row)
	if err != nil {
		return TeamMembershipRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) SaveMembership(ctx context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error) {
	row := r.db.QueryRow(ctx, `
UPDATE team_memberships
SET local_account_id = $2,
    member_email = $3,
    upstream_user_id = $4,
    member_role = $5,
    membership_status = $6,
    invited_at = $7,
    joined_at = $8,
    removed_at = $9,
    last_seen_at = $10,
    source = $11,
    sync_error = $12,
    updated_at = NOW()
WHERE id = $1
RETURNING id, team_id, local_account_id, member_email, upstream_user_id, member_role,
          membership_status, invited_at, joined_at, removed_at, last_seen_at, source,
          sync_error, created_at, updated_at`,
		membership.ID,
		membership.LocalAccountID,
		membership.MemberEmail,
		nullIfEmpty(membership.UpstreamUserID),
		defaultIfEmpty(membership.MemberRole, "member"),
		defaultIfEmpty(membership.MembershipStatus, "pending"),
		membership.InvitedAt,
		membership.JoinedAt,
		membership.RemovedAt,
		membership.LastSeenAt,
		defaultIfEmpty(membership.Source, "sync"),
		nullIfEmpty(membership.SyncError),
	)
	record, err := scanMembershipRecord(row)
	if err != nil {
		return TeamMembershipRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) UpsertMembership(ctx context.Context, membership TeamMembershipRecord) (TeamMembershipRecord, error) {
	row := r.db.QueryRow(ctx, `
INSERT INTO team_memberships (
    team_id, local_account_id, member_email, upstream_user_id, member_role,
    membership_status, invited_at, joined_at, removed_at, last_seen_at, source,
    sync_error, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
ON CONFLICT (team_id, member_email)
DO UPDATE SET
    local_account_id = EXCLUDED.local_account_id,
    upstream_user_id = EXCLUDED.upstream_user_id,
    member_role = EXCLUDED.member_role,
    membership_status = EXCLUDED.membership_status,
    invited_at = EXCLUDED.invited_at,
    joined_at = EXCLUDED.joined_at,
    removed_at = EXCLUDED.removed_at,
    last_seen_at = EXCLUDED.last_seen_at,
    source = EXCLUDED.source,
    sync_error = EXCLUDED.sync_error,
    updated_at = NOW()
RETURNING id, team_id, local_account_id, member_email, upstream_user_id, member_role,
          membership_status, invited_at, joined_at, removed_at, last_seen_at, source,
          sync_error, created_at, updated_at`,
		membership.TeamID,
		membership.LocalAccountID,
		normalizeEmail(membership.MemberEmail),
		nullIfEmpty(membership.UpstreamUserID),
		defaultIfEmpty(membership.MemberRole, "member"),
		defaultIfEmpty(membership.MembershipStatus, "pending"),
		membership.InvitedAt,
		membership.JoinedAt,
		membership.RemovedAt,
		membership.LastSeenAt,
		defaultIfEmpty(membership.Source, "sync"),
		nullIfEmpty(membership.SyncError),
	)
	record, err := scanMembershipRecord(row)
	if err != nil {
		return TeamMembershipRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) SaveTeam(ctx context.Context, team TeamRecord) (TeamRecord, error) {
	row := r.db.QueryRow(ctx, `
UPDATE teams
SET owner_account_id = $2,
    upstream_team_id = $3,
    upstream_account_id = $4,
    team_name = $5,
    plan_type = $6,
    subscription_plan = $7,
    account_role_snapshot = $8,
    status = $9,
    current_members = $10,
    max_members = $11,
    seats_available = $12,
    expires_at = $13,
    last_sync_at = $14,
    sync_status = $15,
    sync_error = $16,
    updated_at = NOW()
WHERE id = $1
RETURNING id, owner_account_id, upstream_team_id, upstream_account_id, team_name, plan_type,
          subscription_plan, account_role_snapshot, status, current_members, max_members,
          seats_available, expires_at, last_sync_at, sync_status, sync_error, created_at, updated_at`,
		team.ID,
		team.OwnerAccountID,
		nullIfEmpty(team.UpstreamTeamID),
		team.UpstreamAccountID,
		team.TeamName,
		team.PlanType,
		nullIfEmpty(team.SubscriptionPlan),
		nullIfEmpty(team.AccountRoleSnapshot),
		defaultIfEmpty(team.Status, "pending"),
		team.CurrentMembers,
		team.MaxMembers,
		team.SeatsAvailable,
		team.ExpiresAt,
		team.LastSyncAt,
		defaultIfEmpty(team.SyncStatus, "pending"),
		nullIfEmpty(team.SyncError),
	)
	record, err := scanTeamRecord(row)
	if err != nil {
		return TeamRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) UpsertTeam(ctx context.Context, team TeamRecord) (TeamRecord, error) {
	row := r.db.QueryRow(ctx, `
INSERT INTO teams (
    owner_account_id, upstream_team_id, upstream_account_id, team_name, plan_type,
    subscription_plan, account_role_snapshot, status, current_members, max_members,
    seats_available, expires_at, last_sync_at, sync_status, sync_error, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
ON CONFLICT (owner_account_id, upstream_account_id)
DO UPDATE SET
    upstream_team_id = EXCLUDED.upstream_team_id,
    team_name = EXCLUDED.team_name,
    plan_type = EXCLUDED.plan_type,
    subscription_plan = EXCLUDED.subscription_plan,
    account_role_snapshot = EXCLUDED.account_role_snapshot,
    status = EXCLUDED.status,
    current_members = EXCLUDED.current_members,
    max_members = EXCLUDED.max_members,
    seats_available = EXCLUDED.seats_available,
    expires_at = EXCLUDED.expires_at,
    last_sync_at = EXCLUDED.last_sync_at,
    sync_status = EXCLUDED.sync_status,
    sync_error = EXCLUDED.sync_error,
    updated_at = NOW()
RETURNING id, owner_account_id, upstream_team_id, upstream_account_id, team_name, plan_type,
          subscription_plan, account_role_snapshot, status, current_members, max_members,
          seats_available, expires_at, last_sync_at, sync_status, sync_error, created_at, updated_at`,
		team.OwnerAccountID,
		nullIfEmpty(team.UpstreamTeamID),
		team.UpstreamAccountID,
		team.TeamName,
		defaultIfEmpty(team.PlanType, "team"),
		nullIfEmpty(team.SubscriptionPlan),
		nullIfEmpty(team.AccountRoleSnapshot),
		defaultIfEmpty(team.Status, "pending"),
		team.CurrentMembers,
		team.MaxMembers,
		team.SeatsAvailable,
		team.ExpiresAt,
		team.LastSyncAt,
		defaultIfEmpty(team.SyncStatus, "pending"),
		nullIfEmpty(team.SyncError),
	)
	record, err := scanTeamRecord(row)
	if err != nil {
		return TeamRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) ListTasks(ctx context.Context, req ListTasksRequest) ([]TeamTaskRecord, error) {
	sql := `
SELECT id, team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
       task_type, status, request_payload, result_payload, error_message, logs,
       created_at, started_at, completed_at, updated_at
FROM team_tasks`
	args := []any{}
	if req.TeamID != nil {
		sql += " WHERE team_id = $1"
		args = append(args, *req.TeamID)
	}
	sql += " ORDER BY created_at DESC, id DESC"

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	records := make([]TeamTaskRecord, 0)
	for rows.Next() {
		record, scanErr := scanTaskRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return records, nil
}

func (r *PostgresRepository) GetTaskByUUID(ctx context.Context, taskUUID string) (TeamTaskRecord, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
       task_type, status, request_payload, result_payload, error_message, logs,
       created_at, started_at, completed_at, updated_at
FROM team_tasks
WHERE task_uuid = $1`, taskUUID)
	record, err := scanTaskRecord(row)
	if err != nil {
		return TeamTaskRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) CreateTask(ctx context.Context, task TeamTaskRecord) (TeamTaskRecord, error) {
	requestPayload, err := json.Marshal(cloneMap(task.RequestPayload))
	if err != nil {
		return TeamTaskRecord{}, fmt.Errorf("marshal request payload: %w", err)
	}
	resultPayload, err := marshalOptionalJSON(task.ResultPayload)
	if err != nil {
		return TeamTaskRecord{}, fmt.Errorf("marshal result payload: %w", err)
	}

	row := r.db.QueryRow(ctx, `
INSERT INTO team_tasks (
    team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
    task_type, status, request_payload, result_payload, error_message, logs,
    created_at, started_at, completed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), $13, $14, NOW())
RETURNING id, team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
          task_type, status, request_payload, result_payload, error_message, logs,
          created_at, started_at, completed_at, updated_at`,
		task.TeamID,
		task.OwnerAccountID,
		task.TaskUUID,
		task.ScopeType,
		task.ScopeID,
		task.ActiveScopeKey,
		task.TaskType,
		defaultIfEmpty(task.Status, "pending"),
		requestPayload,
		resultPayload,
		nullIfEmpty(task.ErrorMessage),
		task.Logs,
		task.StartedAt,
		task.CompletedAt,
	)
	record, err := scanTaskRecord(row)
	if err != nil {
		if isActiveScopeConflict(err) {
			return TeamTaskRecord{}, ErrActiveScopeConflict
		}
		return TeamTaskRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) SaveTask(ctx context.Context, task TeamTaskRecord) (TeamTaskRecord, error) {
	requestPayload, err := json.Marshal(cloneMap(task.RequestPayload))
	if err != nil {
		return TeamTaskRecord{}, fmt.Errorf("marshal request payload: %w", err)
	}
	resultPayload, err := marshalOptionalJSON(task.ResultPayload)
	if err != nil {
		return TeamTaskRecord{}, fmt.Errorf("marshal result payload: %w", err)
	}

	row := r.db.QueryRow(ctx, `
UPDATE team_tasks
SET team_id = $2,
    owner_account_id = $3,
    scope_type = $4,
    scope_id = $5,
    active_scope_key = $6,
    task_type = $7,
    status = $8,
    request_payload = $9,
    result_payload = $10,
    error_message = $11,
    logs = $12,
    started_at = $13,
    completed_at = $14,
    updated_at = NOW()
WHERE task_uuid = $1
RETURNING id, team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
          task_type, status, request_payload, result_payload, error_message, logs,
          created_at, started_at, completed_at, updated_at`,
		task.TaskUUID,
		task.TeamID,
		task.OwnerAccountID,
		task.ScopeType,
		task.ScopeID,
		task.ActiveScopeKey,
		task.TaskType,
		defaultIfEmpty(task.Status, "pending"),
		requestPayload,
		resultPayload,
		nullIfEmpty(task.ErrorMessage),
		task.Logs,
		task.StartedAt,
		task.CompletedAt,
	)
	record, err := scanTaskRecord(row)
	if err != nil {
		if isActiveScopeConflict(err) {
			return TeamTaskRecord{}, ErrActiveScopeConflict
		}
		return TeamTaskRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) ListTaskItems(ctx context.Context, taskID int64) ([]TeamTaskItemRecord, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, task_id, target_email, item_status, before, after, message, error_message,
       created_at, started_at, completed_at, updated_at
FROM team_task_items
WHERE task_id = $1
ORDER BY id ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task items: %w", err)
	}
	defer rows.Close()

	records := make([]TeamTaskItemRecord, 0)
	for rows.Next() {
		record, scanErr := scanTaskItemRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task items: %w", err)
	}
	return records, nil
}

func (r *PostgresRepository) SaveTaskItem(ctx context.Context, item TeamTaskItemRecord) (TeamTaskItemRecord, error) {
	beforePayload, err := marshalOptionalJSON(item.Before)
	if err != nil {
		return TeamTaskItemRecord{}, fmt.Errorf("marshal before payload: %w", err)
	}
	afterPayload, err := marshalOptionalJSON(item.After)
	if err != nil {
		return TeamTaskItemRecord{}, fmt.Errorf("marshal after payload: %w", err)
	}

	row := r.db.QueryRow(ctx, `
INSERT INTO team_task_items (
    task_id, target_email, item_status, before, after, message, error_message,
    created_at, started_at, completed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9, NOW())
ON CONFLICT (task_id, target_email) DO UPDATE
SET item_status = EXCLUDED.item_status,
    before = EXCLUDED.before,
    after = EXCLUDED.after,
    message = EXCLUDED.message,
    error_message = EXCLUDED.error_message,
    started_at = EXCLUDED.started_at,
    completed_at = EXCLUDED.completed_at,
    updated_at = NOW()
RETURNING id, task_id, target_email, item_status, before, after, message, error_message,
          created_at, started_at, completed_at, updated_at`,
		item.TaskID,
		item.TargetEmail,
		defaultIfEmpty(item.ItemStatus, "pending"),
		beforePayload,
		afterPayload,
		nullIfEmpty(item.Message),
		nullIfEmpty(item.ErrorMessage),
		item.StartedAt,
		item.CompletedAt,
	)
	record, err := scanTaskItemRecord(row)
	if err != nil {
		return TeamTaskItemRecord{}, err
	}
	return record, nil
}

func (r *PostgresRepository) FindActiveTask(ctx context.Context, scopeType string, scopeID string, taskType string) (TeamTaskRecord, error) {
	query := `
SELECT id, team_id, owner_account_id, task_uuid, scope_type, scope_id, active_scope_key,
       task_type, status, request_payload, result_payload, error_message, logs,
       created_at, started_at, completed_at, updated_at
FROM team_tasks
WHERE scope_type = $1
  AND scope_id = $2
  AND status NOT IN ('completed', 'failed', 'cancelled')`
	args := []any{scopeType, scopeID}
	if strings.TrimSpace(taskType) != "" {
		args = append(args, taskType)
		query += fmt.Sprintf(" AND task_type = $%d", len(args))
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT 1"

	row := r.db.QueryRow(ctx, query, args...)
	record, err := scanTaskRecord(row)
	if err != nil {
		return TeamTaskRecord{}, err
	}
	return record, nil
}

type teamRowScanner interface {
	Scan(dest ...any) error
}

func scanTeamRecord(scanner teamRowScanner) (TeamRecord, error) {
	var record TeamRecord
	var upstreamTeamID, subscriptionPlan, accountRoleSnapshot, syncError *string
	if err := scanner.Scan(
		&record.ID,
		&record.OwnerAccountID,
		&upstreamTeamID,
		&record.UpstreamAccountID,
		&record.TeamName,
		&record.PlanType,
		&subscriptionPlan,
		&accountRoleSnapshot,
		&record.Status,
		&record.CurrentMembers,
		&record.MaxMembers,
		&record.SeatsAvailable,
		&record.ExpiresAt,
		&record.LastSyncAt,
		&record.SyncStatus,
		&syncError,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamRecord{}, ErrNotFound
		}
		return TeamRecord{}, fmt.Errorf("scan team: %w", err)
	}
	record.UpstreamTeamID = derefString(upstreamTeamID)
	record.SubscriptionPlan = derefString(subscriptionPlan)
	record.AccountRoleSnapshot = derefString(accountRoleSnapshot)
	record.SyncError = derefString(syncError)
	return record, nil
}

func scanAccountRecord(scanner teamRowScanner) (AccountRecord, error) {
	var record AccountRecord
	if err := scanner.Scan(&record.ID, &record.Email, &record.Status, &record.AccessToken); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountRecord{}, ErrNotFound
		}
		return AccountRecord{}, fmt.Errorf("scan account: %w", err)
	}
	return record, nil
}

func scanMembershipRecord(scanner teamRowScanner) (TeamMembershipRecord, error) {
	var record TeamMembershipRecord
	var upstreamUserID, syncError *string
	if err := scanner.Scan(
		&record.ID,
		&record.TeamID,
		&record.LocalAccountID,
		&record.MemberEmail,
		&upstreamUserID,
		&record.MemberRole,
		&record.MembershipStatus,
		&record.InvitedAt,
		&record.JoinedAt,
		&record.RemovedAt,
		&record.LastSeenAt,
		&record.Source,
		&syncError,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamMembershipRecord{}, ErrNotFound
		}
		return TeamMembershipRecord{}, fmt.Errorf("scan membership: %w", err)
	}
	record.UpstreamUserID = derefString(upstreamUserID)
	record.SyncError = derefString(syncError)
	return record, nil
}

func scanTaskRecord(scanner teamRowScanner) (TeamTaskRecord, error) {
	var record TeamTaskRecord
	var requestPayload, resultPayload []byte
	var activeScopeKey, errorMessage *string
	if err := scanner.Scan(
		&record.ID,
		&record.TeamID,
		&record.OwnerAccountID,
		&record.TaskUUID,
		&record.ScopeType,
		&record.ScopeID,
		&activeScopeKey,
		&record.TaskType,
		&record.Status,
		&requestPayload,
		&resultPayload,
		&errorMessage,
		&record.Logs,
		&record.CreatedAt,
		&record.StartedAt,
		&record.CompletedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamTaskRecord{}, ErrNotFound
		}
		return TeamTaskRecord{}, fmt.Errorf("scan task: %w", err)
	}
	record.ActiveScopeKey = activeScopeKey
	record.ErrorMessage = derefString(errorMessage)
	record.RequestPayload = decodeJSONMap(requestPayload)
	record.ResultPayload = decodeJSONMap(resultPayload)
	return record, nil
}

func scanTaskItemRecord(scanner teamRowScanner) (TeamTaskItemRecord, error) {
	var record TeamTaskItemRecord
	var beforePayload, afterPayload []byte
	var message, errorMessage *string
	if err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.TargetEmail,
		&record.ItemStatus,
		&beforePayload,
		&afterPayload,
		&message,
		&errorMessage,
		&record.CreatedAt,
		&record.StartedAt,
		&record.CompletedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamTaskItemRecord{}, ErrNotFound
		}
		return TeamTaskItemRecord{}, fmt.Errorf("scan task item: %w", err)
	}
	record.Before = decodeJSONMap(beforePayload)
	record.After = decodeJSONMap(afterPayload)
	record.Message = derefString(message)
	record.ErrorMessage = derefString(errorMessage)
	return record, nil
}

func buildTeamListWhere(req ListTeamsRequest) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)

	if status := strings.TrimSpace(req.Status); status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if req.OwnerAccountID > 0 {
		args = append(args, req.OwnerAccountID)
		clauses = append(clauses, fmt.Sprintf("owner_account_id = $%d", len(args)))
	}
	if search := strings.TrimSpace(req.Search); search != "" {
		args = append(args, "%"+search+"%")
		index := len(args)
		clauses = append(clauses, fmt.Sprintf("(team_name ILIKE $%d OR upstream_account_id ILIKE $%d)", index, index))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func decodeJSONMap(payload []byte) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil
	}
	return decoded
}

func marshalOptionalJSON(payload map[string]any) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func defaultIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isActiveScopeConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "active_scope_key")
	}
	return false
}
