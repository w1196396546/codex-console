package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

	total, err := r.countAccounts(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			email,
			COALESCE(password, ''),
			COALESCE(cpa_uploaded, FALSE),
			cpa_uploaded_at,
			COALESCE(sub2api_uploaded, FALSE),
			sub2api_uploaded_at,
			last_refresh,
			expires_at,
			COALESCE(status, ''),
			registered_at,
			created_at,
			updated_at
		FROM accounts
		ORDER BY COALESCE(registered_at, created_at, updated_at) DESC, id DESC
		LIMIT $1 OFFSET $2
	`, normalized.PageSize, normalized.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, normalized.PageSize)
	for rows.Next() {
		var (
			account           Account
			cpaUploaded       bool
			cpaUploadedAt     *time.Time
			sub2apiUploaded   bool
			sub2apiUploadedAt *time.Time
			lastRefresh       *time.Time
			expiresAt         *time.Time
			registeredAt      *time.Time
			createdAt         *time.Time
			updatedAt         *time.Time
		)
		if err := rows.Scan(
			&account.ID,
			&account.Email,
			&account.Password,
			&cpaUploaded,
			&cpaUploadedAt,
			&sub2apiUploaded,
			&sub2apiUploadedAt,
			&lastRefresh,
			&expiresAt,
			&account.Status,
			&registeredAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan account: %w", err)
		}

		account.CPAUploaded = cpaUploaded
		account.CPAUploadedAt = cpaUploadedAt
		account.Sub2APIUploaded = sub2apiUploaded
		account.Sub2APIUploadedAt = sub2apiUploadedAt
		account.LastRefresh = lastRefresh
		account.ExpiresAt = expiresAt
		account.RegisteredAt = registeredAt
		account.CreatedAt = createdAt
		account.UpdatedAt = updatedAt
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, total, nil
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

func (r *PostgresRepository) countAccounts(ctx context.Context) (int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&total); err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}

	return total, nil
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
