package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const accountSelectColumns = `
		id,
		email,
		COALESCE(password, ''),
		COALESCE(client_id, ''),
		COALESCE(session_token, ''),
		COALESCE(email_service, ''),
		COALESCE(email_service_id, ''),
		COALESCE(account_id, ''),
		COALESCE(workspace_id, ''),
		COALESCE(access_token, ''),
		COALESCE(refresh_token, ''),
		COALESCE(id_token, ''),
		COALESCE(cookies, ''),
		COALESCE(proxy_used, ''),
		COALESCE(cpa_uploaded, FALSE),
		cpa_uploaded_at,
		COALESCE(sub2api_uploaded, FALSE),
		sub2api_uploaded_at,
		last_refresh,
		expires_at,
		COALESCE(extra_data::text, '{}'),
		COALESCE(status, ''),
		COALESCE(source, ''),
		COALESCE(subscription_type, ''),
		subscription_at,
		registered_at,
		created_at,
		updated_at
`

type accountFilters struct {
	Status            string
	EmailService      string
	RefreshTokenState string
	Search            string
}

type postgresQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PostgresRepository struct {
	db postgresQuerier
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return newPostgresRepository(pool)
}

func newPostgresRepository(db postgresQuerier) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListAccounts(ctx context.Context, req ListAccountsRequest) ([]Account, int, error) {
	normalized := req.Normalized()

	total, err := r.countAccounts(ctx, accountFilters{
		Status:            normalized.Status,
		EmailService:      normalized.EmailService,
		RefreshTokenState: normalized.RefreshTokenState,
		Search:            normalized.Search,
	})
	if err != nil {
		return nil, 0, err
	}

	whereClause, args := buildAccountFiltersSQL(accountFilters{
		Status:            normalized.Status,
		EmailService:      normalized.EmailService,
		RefreshTokenState: normalized.RefreshTokenState,
		Search:            normalized.Search,
	}, 1)
	args = append(args, normalized.PageSize, normalized.Offset())
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM accounts
		%s
		ORDER BY COALESCE(created_at, registered_at, updated_at) DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, accountSelectColumns, whereClause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, normalized.PageSize)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, total, nil
}

func (r *PostgresRepository) ListAccountsForOverview(ctx context.Context, req AccountOverviewCardsRequest) ([]Account, error) {
	normalized := req.Normalized()
	return r.listAccountsByFilters(ctx, accountFilters{
		Status:       normalized.Status,
		EmailService: normalized.EmailService,
		Search:       normalized.Search,
	})
}

func (r *PostgresRepository) ListAccountsForSelectable(ctx context.Context, req AccountOverviewSelectableRequest) ([]Account, error) {
	normalized := req.Normalized()
	return r.listAccountsByFilters(ctx, accountFilters{
		Status:       normalized.Status,
		EmailService: normalized.EmailService,
		Search:       normalized.Search,
	})
}

func (r *PostgresRepository) ListAccountsBySelection(ctx context.Context, req AccountSelectionRequest) ([]Account, error) {
	normalized := req.Normalized()
	if normalized.SelectAll {
		return r.listAccountsByFilters(ctx, accountFilters{
			Status:            normalized.StatusFilter,
			EmailService:      normalized.EmailServiceFilter,
			RefreshTokenState: normalized.RefreshTokenStateFilter,
			Search:            normalized.SearchFilter,
		})
	}
	if len(normalized.IDs) == 0 {
		return []Account{}, nil
	}

	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM accounts
		WHERE id = ANY($1)
		ORDER BY COALESCE(created_at, registered_at, updated_at) DESC, id DESC
	`, accountSelectColumns), toInt32Slice(normalized.IDs))
	if err != nil {
		return nil, fmt.Errorf("query accounts by ids: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, len(normalized.IDs))
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan selected account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate selected accounts: %w", err)
	}
	return accounts, nil
}

func (r *PostgresRepository) GetAccountByID(ctx context.Context, accountID int) (Account, error) {
	account, err := scanAccount(r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM accounts
		WHERE id = $1
	`, accountSelectColumns), accountID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, fmt.Errorf("get account by id: %w", err)
	}
	return account, nil
}

func (r *PostgresRepository) DeleteAccount(ctx context.Context, accountID int) error {
	if _, scanErr := scanDeleteResult(r.db.QueryRow(ctx, `
		DELETE FROM accounts
		WHERE id = $1
		RETURNING id
	`, accountID)); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("delete account: %w", scanErr)
	}
	return nil
}

func (r *PostgresRepository) GetCurrentAccountID(ctx context.Context) (*int, error) {
	var raw string
	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(value, '')
		FROM settings
		WHERE key = $1
		LIMIT 1
	`, CurrentAccountSettingKey).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get current account id: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, nil
	}
	return &value, nil
}

func (r *PostgresRepository) GetAccountsStatsSummary(ctx context.Context) (AccountsStatsSummary, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&total); err != nil {
		return AccountsStatsSummary{}, fmt.Errorf("count accounts summary: %w", err)
	}

	byStatus, err := r.scanCountMap(ctx, `
		SELECT COALESCE(status, ''), COUNT(*)
		FROM accounts
		GROUP BY COALESCE(status, '')
	`)
	if err != nil {
		return AccountsStatsSummary{}, err
	}
	byEmailService, err := r.scanCountMap(ctx, `
		SELECT COALESCE(email_service, ''), COUNT(*)
		FROM accounts
		GROUP BY COALESCE(email_service, '')
	`)
	if err != nil {
		return AccountsStatsSummary{}, err
	}

	return AccountsStatsSummary{
		Total:          total,
		ByStatus:       byStatus,
		ByEmailService: byEmailService,
	}, nil
}

func (r *PostgresRepository) GetAccountsOverviewStats(ctx context.Context) (AccountsOverviewStats, error) {
	var (
		total             int
		activeCount       int
		withAccessToken   int
		withRefreshToken  int
		cpaUploadedCount  int
	)

	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&total); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("count overview accounts: %w", err)
	}
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE COALESCE(status, '') = 'active'`).Scan(&activeCount); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("count active accounts: %w", err)
	}
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE COALESCE(access_token, '') <> ''`).Scan(&withAccessToken); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("count accounts with access_token: %w", err)
	}
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE TRIM(COALESCE(refresh_token, '')) <> ''`).Scan(&withRefreshToken); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("count accounts with refresh_token: %w", err)
	}
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE COALESCE(cpa_uploaded, FALSE) = TRUE`).Scan(&cpaUploadedCount); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("count cpa uploaded accounts: %w", err)
	}

	byStatus, err := r.scanCountMap(ctx, `SELECT COALESCE(status, 'unknown'), COUNT(*) FROM accounts GROUP BY COALESCE(status, 'unknown')`)
	if err != nil {
		return AccountsOverviewStats{}, err
	}
	byEmailService, err := r.scanCountMap(ctx, `SELECT COALESCE(email_service, 'unknown'), COUNT(*) FROM accounts GROUP BY COALESCE(email_service, 'unknown')`)
	if err != nil {
		return AccountsOverviewStats{}, err
	}
	bySource, err := r.scanCountMap(ctx, `SELECT COALESCE(source, 'unknown'), COUNT(*) FROM accounts GROUP BY COALESCE(source, 'unknown')`)
	if err != nil {
		return AccountsOverviewStats{}, err
	}
	bySubscription, err := r.scanCountMap(ctx, `SELECT COALESCE(subscription_type, 'free'), COUNT(*) FROM accounts GROUP BY COALESCE(subscription_type, 'free')`)
	if err != nil {
		return AccountsOverviewStats{}, err
	}

	recentRows, err := r.db.Query(ctx, `
		SELECT
			id,
			email,
			COALESCE(status, ''),
			COALESCE(email_service, ''),
			COALESCE(source, ''),
			COALESCE(subscription_type, 'free'),
			created_at,
			last_refresh
		FROM accounts
		ORDER BY COALESCE(created_at, registered_at, updated_at) DESC, id DESC
		LIMIT 10
	`)
	if err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("query recent accounts: %w", err)
	}
	defer recentRows.Close()

	recentAccounts := make([]AccountOverviewRecentItem, 0, 10)
	for recentRows.Next() {
		var (
			item        AccountOverviewRecentItem
			createdAt   *time.Time
			lastRefresh *time.Time
		)
		if err := recentRows.Scan(&item.ID, &item.Email, &item.Status, &item.EmailService, &item.Source, &item.SubscriptionType, &createdAt, &lastRefresh); err != nil {
			return AccountsOverviewStats{}, fmt.Errorf("scan recent account: %w", err)
		}
		item.CreatedAt = formatTime(createdAt)
		item.LastRefresh = formatTime(lastRefresh)
		recentAccounts = append(recentAccounts, item)
	}
	if err := recentRows.Err(); err != nil {
		return AccountsOverviewStats{}, fmt.Errorf("iterate recent accounts: %w", err)
	}

	return AccountsOverviewStats{
		Total:       total,
		ActiveCount: activeCount,
		TokenStats: AccountTokenStats{
			WithAccessToken:    withAccessToken,
			WithRefreshToken:   withRefreshToken,
			WithoutAccessToken: max(total-withAccessToken, 0),
		},
		CPAUploadedCount: cpaUploadedCount,
		ByStatus:         byStatus,
		ByEmailService:   byEmailService,
		BySource:         bySource,
		BySubscription:   bySubscription,
		RecentAccounts:   recentAccounts,
	}, nil
}

func (r *PostgresRepository) GetAccountByEmail(ctx context.Context, email string) (Account, bool, error) {
	account, err := scanAccount(r.db.QueryRow(ctx, `
		SELECT
			id,
			email,
			COALESCE(password, ''),
			COALESCE(client_id, ''),
			COALESCE(session_token, ''),
			COALESCE(email_service, ''),
			COALESCE(email_service_id, ''),
			COALESCE(account_id, ''),
			COALESCE(workspace_id, ''),
			COALESCE(access_token, ''),
			COALESCE(refresh_token, ''),
			COALESCE(id_token, ''),
			COALESCE(cookies, ''),
			COALESCE(proxy_used, ''),
			COALESCE(cpa_uploaded, FALSE),
			cpa_uploaded_at,
			COALESCE(sub2api_uploaded, FALSE),
			sub2api_uploaded_at,
			last_refresh,
			expires_at,
			COALESCE(extra_data::text, '{}'),
			COALESCE(status, ''),
			COALESCE(source, ''),
			COALESCE(subscription_type, ''),
			subscription_at,
			registered_at,
			created_at,
			updated_at
		FROM accounts
		WHERE email = $1
	`, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, false, nil
		}
		return Account{}, false, fmt.Errorf("get account by email: %w", err)
	}

	return account, true, nil
}

func (r *PostgresRepository) UpsertAccount(ctx context.Context, account Account) (Account, error) {
	extraDataJSON, err := marshalExtraData(account.ExtraData)
	if err != nil {
		return Account{}, fmt.Errorf("marshal extra_data: %w", err)
	}

	saved, err := scanAccount(r.db.QueryRow(ctx, `
		INSERT INTO accounts (
			email,
			password,
			client_id,
			session_token,
			email_service,
			email_service_id,
			account_id,
			workspace_id,
			access_token,
			refresh_token,
			id_token,
			cookies,
			proxy_used,
			cpa_uploaded,
			cpa_uploaded_at,
			sub2api_uploaded,
			sub2api_uploaded_at,
			last_refresh,
			expires_at,
			extra_data,
			status,
			source,
			subscription_type,
			subscription_at,
			registered_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
		)
		ON CONFLICT (email) DO UPDATE SET
			password = EXCLUDED.password,
			client_id = EXCLUDED.client_id,
			session_token = EXCLUDED.session_token,
			email_service = EXCLUDED.email_service,
			email_service_id = EXCLUDED.email_service_id,
			account_id = EXCLUDED.account_id,
			workspace_id = EXCLUDED.workspace_id,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			id_token = EXCLUDED.id_token,
			cookies = EXCLUDED.cookies,
			proxy_used = EXCLUDED.proxy_used,
			cpa_uploaded = EXCLUDED.cpa_uploaded,
			cpa_uploaded_at = EXCLUDED.cpa_uploaded_at,
			sub2api_uploaded = EXCLUDED.sub2api_uploaded,
			sub2api_uploaded_at = EXCLUDED.sub2api_uploaded_at,
			last_refresh = EXCLUDED.last_refresh,
			expires_at = EXCLUDED.expires_at,
			extra_data = EXCLUDED.extra_data,
			status = EXCLUDED.status,
			source = EXCLUDED.source,
			subscription_type = EXCLUDED.subscription_type,
			subscription_at = EXCLUDED.subscription_at,
			registered_at = EXCLUDED.registered_at,
			updated_at = NOW()
		RETURNING
			id,
			email,
			COALESCE(password, ''),
			COALESCE(client_id, ''),
			COALESCE(session_token, ''),
			COALESCE(email_service, ''),
			COALESCE(email_service_id, ''),
			COALESCE(account_id, ''),
			COALESCE(workspace_id, ''),
			COALESCE(access_token, ''),
			COALESCE(refresh_token, ''),
			COALESCE(id_token, ''),
			COALESCE(cookies, ''),
			COALESCE(proxy_used, ''),
			COALESCE(cpa_uploaded, FALSE),
			cpa_uploaded_at,
			COALESCE(sub2api_uploaded, FALSE),
			sub2api_uploaded_at,
			last_refresh,
			expires_at,
			COALESCE(extra_data::text, '{}'),
			COALESCE(status, ''),
			COALESCE(source, ''),
			COALESCE(subscription_type, ''),
			subscription_at,
			registered_at,
			created_at,
			updated_at
	`, account.Email, account.Password, account.ClientID, account.SessionToken, account.EmailService, account.EmailServiceID, account.AccountID, account.WorkspaceID, account.AccessToken, account.RefreshToken, account.IDToken, account.Cookies, account.ProxyUsed, account.CPAUploaded, account.CPAUploadedAt, account.Sub2APIUploaded, account.Sub2APIUploadedAt, account.LastRefresh, account.ExpiresAt, extraDataJSON, account.Status, account.Source, account.SubscriptionType, account.SubscriptionAt, account.RegisteredAt))
	if err != nil {
		return Account{}, fmt.Errorf("upsert account: %w", err)
	}

	return saved, nil
}

func (r *PostgresRepository) CompareAndSwapTokenCompletionRuntime(
	ctx context.Context,
	email string,
	currentExtraData map[string]any,
	nextExtraData map[string]any,
	defaults Account,
) (Account, bool, error) {
	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" {
		return Account{}, false, ErrAccountEmailRequired
	}

	currentAttemptsJSON, err := marshalTokenCompletionCASValue(currentExtraData["token_completion_attempts"])
	if err != nil {
		return Account{}, false, fmt.Errorf("marshal current token completion attempts: %w", err)
	}
	nextRuntimeJSON, err := marshalExtraData(filterTokenCompletionRuntimeExtraData(nextExtraData))
	if err != nil {
		return Account{}, false, fmt.Errorf("marshal next token completion runtime extra_data: %w", err)
	}
	currentCooldown := strings.TrimSpace(fmt.Sprintf("%v", currentExtraData["refresh_token_cooldown_until"]))

	insertAccount := defaults
	insertAccount.Email = normalizedEmail
	insertExtraData := filterTokenCompletionRuntimeExtraData(nextExtraData)
	insertExtraDataJSON, err := marshalExtraData(insertExtraData)
	if err != nil {
		return Account{}, false, fmt.Errorf("marshal insert token completion runtime extra_data: %w", err)
	}

	if tokenCompletionRuntimeExtraDataEmpty(currentExtraData) {
		saved, err := scanAccount(r.db.QueryRow(ctx, `
			INSERT INTO accounts (
				email,
				extra_data,
				status,
				source
			) VALUES ($1, $2, $3, $4)
			ON CONFLICT (email) DO NOTHING
			RETURNING
				id,
				email,
				COALESCE(password, ''),
				COALESCE(client_id, ''),
				COALESCE(session_token, ''),
				COALESCE(email_service, ''),
				COALESCE(email_service_id, ''),
				COALESCE(account_id, ''),
				COALESCE(workspace_id, ''),
				COALESCE(access_token, ''),
				COALESCE(refresh_token, ''),
				COALESCE(id_token, ''),
				COALESCE(cookies, ''),
				COALESCE(proxy_used, ''),
				COALESCE(cpa_uploaded, FALSE),
				cpa_uploaded_at,
				COALESCE(sub2api_uploaded, FALSE),
				sub2api_uploaded_at,
				last_refresh,
				expires_at,
				COALESCE(extra_data::text, '{}'),
				COALESCE(status, ''),
				COALESCE(source, ''),
				COALESCE(subscription_type, ''),
				subscription_at,
				registered_at,
				created_at,
				updated_at
		`, insertAccount.Email, insertExtraDataJSON, insertAccount.Status, insertAccount.Source))
		if err == nil {
			return saved, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Account{}, false, fmt.Errorf("insert token completion runtime state: %w", err)
		}
	}

	saved, err := scanAccount(r.db.QueryRow(ctx, `
		UPDATE accounts
		SET
			extra_data = (COALESCE(extra_data, '{}'::jsonb) - 'token_completion_attempts' - 'refresh_token_cooldown_until') || $2::jsonb,
			updated_at = NOW()
		WHERE email = $1
			AND COALESCE(extra_data->'token_completion_attempts', 'null'::jsonb) = $3::jsonb
			AND COALESCE(extra_data->>'refresh_token_cooldown_until', '') = $4
		RETURNING
			id,
			email,
			COALESCE(password, ''),
			COALESCE(client_id, ''),
			COALESCE(session_token, ''),
			COALESCE(email_service, ''),
			COALESCE(email_service_id, ''),
			COALESCE(account_id, ''),
			COALESCE(workspace_id, ''),
			COALESCE(access_token, ''),
			COALESCE(refresh_token, ''),
			COALESCE(id_token, ''),
			COALESCE(cookies, ''),
			COALESCE(proxy_used, ''),
			COALESCE(cpa_uploaded, FALSE),
			cpa_uploaded_at,
			COALESCE(sub2api_uploaded, FALSE),
			sub2api_uploaded_at,
			last_refresh,
			expires_at,
			COALESCE(extra_data::text, '{}'),
			COALESCE(status, ''),
			COALESCE(source, ''),
			COALESCE(subscription_type, ''),
			subscription_at,
			registered_at,
			created_at,
			updated_at
	`, normalizedEmail, nextRuntimeJSON, currentAttemptsJSON, currentCooldown))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, false, nil
		}
		return Account{}, false, fmt.Errorf("compare and swap token completion runtime state: %w", err)
	}

	return saved, true, nil
}

func (r *PostgresRepository) countAccounts(ctx context.Context, filters accountFilters) (int, error) {
	whereClause, args := buildAccountFiltersSQL(filters, 1)
	var total int
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM accounts %s`, whereClause), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}

	return total, nil
}

func (r *PostgresRepository) listAccountsByFilters(ctx context.Context, filters accountFilters) ([]Account, error) {
	whereClause, args := buildAccountFiltersSQL(filters, 1)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM accounts
		%s
		ORDER BY COALESCE(created_at, registered_at, updated_at) DESC, id DESC
	`, accountSelectColumns, whereClause), args...)
	if err != nil {
		return nil, fmt.Errorf("query filtered accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan filtered account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate filtered accounts: %w", err)
	}
	return accounts, nil
}

func (r *PostgresRepository) scanCountMap(ctx context.Context, query string, args ...any) (map[string]int, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query count map: %w", err)
	}
	defer rows.Close()

	values := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("scan count map: %w", err)
		}
		values[key] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate count map: %w", err)
	}
	return values, nil
}

func buildAccountFiltersSQL(filters accountFilters, startIndex int) (string, []any) {
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	next := startIndex

	switch normalizeFilterText(filters.Status) {
	case "":
	case "failed", "invalid":
		conditions = append(conditions, `COALESCE(status, '') IN ('failed', 'expired', 'banned')`)
	default:
		conditions = append(conditions, fmt.Sprintf(`COALESCE(status, '') = $%d`, next))
		args = append(args, normalizeFilterText(filters.Status))
		next++
	}

	if emailService := normalizeFilterText(filters.EmailService); emailService != "" {
		conditions = append(conditions, fmt.Sprintf(`COALESCE(email_service, '') = $%d`, next))
		args = append(args, emailService)
		next++
	}

	switch normalizeFilterText(filters.RefreshTokenState) {
	case "", "all":
	case "has":
		conditions = append(conditions, `TRIM(COALESCE(refresh_token, '')) <> ''`)
	case "missing":
		conditions = append(conditions, `TRIM(COALESCE(refresh_token, '')) = ''`)
	}

	if search := strings.TrimSpace(filters.Search); search != "" {
		conditions = append(conditions, fmt.Sprintf(`(email ILIKE $%d OR COALESCE(account_id, '') ILIKE $%d)`, next, next))
		args = append(args, "%"+search+"%")
		next++
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func toInt32Slice(ids []int) []int32 {
	values := make([]int32, 0, len(ids))
	for _, id := range ids {
		values = append(values, int32(id))
	}
	return values
}

func scanDeleteResult(scanner accountScanner) (int, error) {
	var id int
	if err := scanner.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

type accountScanner interface {
	Scan(dest ...any) error
}

func scanAccount(scanner accountScanner) (Account, error) {
	var (
		account           Account
		extraDataRaw      string
		cpaUploaded       bool
		cpaUploadedAt     *time.Time
		sub2apiUploaded   bool
		sub2apiUploadedAt *time.Time
		lastRefresh       *time.Time
		expiresAt         *time.Time
		registeredAt      *time.Time
		subscriptionAt    *time.Time
		createdAt         *time.Time
		updatedAt         *time.Time
	)

	if err := scanner.Scan(
		&account.ID,
		&account.Email,
		&account.Password,
		&account.ClientID,
		&account.SessionToken,
		&account.EmailService,
		&account.EmailServiceID,
		&account.AccountID,
		&account.WorkspaceID,
		&account.AccessToken,
		&account.RefreshToken,
		&account.IDToken,
		&account.Cookies,
		&account.ProxyUsed,
		&cpaUploaded,
		&cpaUploadedAt,
		&sub2apiUploaded,
		&sub2apiUploadedAt,
		&lastRefresh,
		&expiresAt,
		&extraDataRaw,
		&account.Status,
		&account.Source,
		&account.SubscriptionType,
		&subscriptionAt,
		&registeredAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Account{}, err
	}

	extraData, err := parseExtraData(extraDataRaw)
	if err != nil {
		return Account{}, err
	}
	account.ExtraData = extraData
	account.CPAUploaded = cpaUploaded
	account.CPAUploadedAt = cpaUploadedAt
	account.Sub2APIUploaded = sub2apiUploaded
	account.Sub2APIUploadedAt = sub2apiUploadedAt
	account.LastRefresh = lastRefresh
	account.ExpiresAt = expiresAt
	account.RegisteredAt = registeredAt
	account.SubscriptionAt = subscriptionAt
	account.CreatedAt = createdAt
	account.UpdatedAt = updatedAt

	return account, nil
}

func marshalExtraData(value map[string]any) (string, error) {
	payload := cloneExtraData(value)
	if payload == nil {
		payload = map[string]any{}
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func parseExtraData(value string) (map[string]any, error) {
	if value == "" {
		return map[string]any{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return nil, fmt.Errorf("parse extra_data: %w", err)
	}
	if payload == nil {
		return map[string]any{}, nil
	}

	return payload, nil
}

func marshalTokenCompletionCASValue(value any) (string, error) {
	if value == nil {
		return "null", nil
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func filterTokenCompletionRuntimeExtraData(value map[string]any) map[string]any {
	filtered := map[string]any{
		"token_completion_attempts":    value["token_completion_attempts"],
		"refresh_token_cooldown_until": strings.TrimSpace(fmt.Sprintf("%v", value["refresh_token_cooldown_until"])),
	}
	if filtered["refresh_token_cooldown_until"] == "" || filtered["refresh_token_cooldown_until"] == "<nil>" {
		filtered["refresh_token_cooldown_until"] = ""
	}
	return filtered
}

func tokenCompletionRuntimeExtraDataEmpty(value map[string]any) bool {
	if len(value) == 0 {
		return true
	}
	return value["token_completion_attempts"] == nil && strings.TrimSpace(fmt.Sprintf("%v", value["refresh_token_cooldown_until"])) == ""
}
