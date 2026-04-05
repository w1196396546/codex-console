package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const postgresBackupFormat = "codex-console-postgres-backup.v1"

type PostgresDatabaseAdmin struct {
	pool        *pgxpool.Pool
	databaseURL string
	backupDir   string
	now         func() time.Time
}

func NewPostgresDatabaseAdmin(pool *pgxpool.Pool, databaseURL string, backupDir string) *PostgresDatabaseAdmin {
	backupDir = strings.TrimSpace(backupDir)
	if backupDir == "" {
		backupDir = "backups"
	}

	return &PostgresDatabaseAdmin{
		pool:        pool,
		databaseURL: strings.TrimSpace(databaseURL),
		backupDir:   backupDir,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (a *PostgresDatabaseAdmin) GetInfo(ctx context.Context) (DatabaseInfoResponse, error) {
	if a == nil || a.pool == nil {
		return DatabaseInfoResponse{}, ErrDatabaseAdminNotConfigured
	}

	var databaseSizeBytes int64
	if err := a.pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseSizeBytes); err != nil {
		return DatabaseInfoResponse{}, fmt.Errorf("query database size: %w", err)
	}

	accountsCount, err := a.tableCount(ctx, "accounts")
	if err != nil {
		return DatabaseInfoResponse{}, err
	}
	emailServicesCount, err := a.tableCount(ctx, "email_services")
	if err != nil {
		return DatabaseInfoResponse{}, err
	}
	tasksCount, err := a.tableCount(ctx, "jobs")
	if err != nil {
		return DatabaseInfoResponse{}, err
	}

	return DatabaseInfoResponse{
		DatabaseURL:        a.databaseURL,
		DatabaseSizeBytes:  databaseSizeBytes,
		DatabaseSizeMB:     float64(databaseSizeBytes) / (1024 * 1024),
		AccountsCount:      accountsCount,
		EmailServicesCount: emailServicesCount,
		TasksCount:         tasksCount,
	}, nil
}

func (a *PostgresDatabaseAdmin) Backup(ctx context.Context) (DatabaseBackupResponse, error) {
	if a == nil || a.pool == nil {
		return DatabaseBackupResponse{}, ErrDatabaseAdminNotConfigured
	}

	payload := postgresBackupFile{
		Format:      postgresBackupFormat,
		Driver:      "postgres",
		DatabaseURL: a.databaseURL,
		GeneratedAt: a.now(),
		Tables:      map[string]json.RawMessage{},
	}

	for _, table := range managedBackupTables() {
		exists, err := a.tableExists(ctx, table)
		if err != nil {
			return DatabaseBackupResponse{}, err
		}
		if !exists {
			continue
		}

		columns, err := a.tableColumns(ctx, table)
		if err != nil {
			return DatabaseBackupResponse{}, err
		}
		if len(columns) == 0 {
			continue
		}

		tableJSON, err := a.exportTable(ctx, table, columns)
		if err != nil {
			return DatabaseBackupResponse{}, err
		}
		payload.Tables[table] = tableJSON
	}

	if err := os.MkdirAll(a.backupDir, 0o755); err != nil {
		return DatabaseBackupResponse{}, fmt.Errorf("create backup dir: %w", err)
	}

	filePath := filepath.Join(a.backupDir, fmt.Sprintf("database_backup_%s.json", a.now().Format("20060102_150405")))
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return DatabaseBackupResponse{}, fmt.Errorf("marshal backup payload: %w", err)
	}
	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		return DatabaseBackupResponse{}, fmt.Errorf("write backup file: %w", err)
	}

	return DatabaseBackupResponse{
		Success:    true,
		Message:    "数据库备份成功",
		BackupPath: filePath,
	}, nil
}

func (a *PostgresDatabaseAdmin) Import(ctx context.Context, req DatabaseImportRequest) (DatabaseImportResponse, error) {
	if a == nil || a.pool == nil {
		return DatabaseImportResponse{}, ErrDatabaseAdminNotConfigured
	}
	if len(req.Content) == 0 {
		return DatabaseImportResponse{}, errors.New("导入文件无效或为空")
	}

	var payload postgresBackupFile
	if err := json.Unmarshal(req.Content, &payload); err != nil {
		return DatabaseImportResponse{}, errors.New("当前 PostgreSQL 路径仅支持导入 Go 生成的 JSON 备份")
	}
	if payload.Format != postgresBackupFormat {
		return DatabaseImportResponse{}, errors.New("当前 PostgreSQL 路径仅支持导入 Go 生成的 JSON 备份")
	}

	preBackup, err := a.Backup(ctx)
	if err != nil {
		return DatabaseImportResponse{}, fmt.Errorf("create pre-import backup: %w", err)
	}

	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DatabaseImportResponse{}, fmt.Errorf("begin import tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	truncateTables := intersectOrderedTables(payload.Tables)
	if len(truncateTables) > 0 {
		if _, err := tx.Exec(ctx, buildTruncateSQL(truncateTables)); err != nil {
			return DatabaseImportResponse{}, fmt.Errorf("truncate backup tables: %w", err)
		}
	}

	for _, table := range restoreOrder() {
		tableJSON, ok := payload.Tables[table]
		if !ok {
			continue
		}

		columns, err := a.tableColumnsTx(ctx, tx, table)
		if err != nil {
			return DatabaseImportResponse{}, err
		}
		if len(columns) == 0 {
			continue
		}
		if err := a.importTable(ctx, tx, table, columns, tableJSON); err != nil {
			return DatabaseImportResponse{}, err
		}
		if err := a.resetIDSequence(ctx, tx, table); err != nil {
			return DatabaseImportResponse{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return DatabaseImportResponse{}, fmt.Errorf("commit import tx: %w", err)
	}

	return DatabaseImportResponse{
		Success:    true,
		Message:    "数据库导入成功",
		BackupPath: preBackup.BackupPath,
	}, nil
}

func (a *PostgresDatabaseAdmin) Cleanup(ctx context.Context, req DatabaseCleanupRequest) (DatabaseCleanupResponse, error) {
	if a == nil || a.pool == nil {
		return DatabaseCleanupResponse{}, ErrDatabaseAdminNotConfigured
	}

	cutoff := a.now().Add(-time.Duration(req.Days) * 24 * time.Hour)
	query := `
DELETE FROM jobs
WHERE created_at < $1
  AND status = ANY($2)
`
	statuses := []string{"completed", "cancelled"}
	if !req.KeepFailed {
		query = `
DELETE FROM jobs
WHERE created_at < $1
  AND status <> ALL($2)
`
		statuses = []string{"failed"}
	}

	result, err := a.pool.Exec(ctx, query, cutoff, statuses)
	if err != nil {
		if isMissingRelation(err) {
			return DatabaseCleanupResponse{
				Success:      true,
				Message:      "已清理 0 条过期任务记录",
				DeletedCount: 0,
			}, nil
		}
		return DatabaseCleanupResponse{}, fmt.Errorf("cleanup jobs: %w", err)
	}

	deletedCount := int(result.RowsAffected())
	return DatabaseCleanupResponse{
		Success:      true,
		Message:      fmt.Sprintf("已清理 %d 条过期任务记录", deletedCount),
		DeletedCount: deletedCount,
	}, nil
}

type postgresBackupFile struct {
	Format      string                     `json:"format"`
	Driver      string                     `json:"driver"`
	DatabaseURL string                     `json:"database_url"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Tables      map[string]json.RawMessage `json:"tables"`
}

type tableColumn struct {
	Name string
	Type string
}

func (a *PostgresDatabaseAdmin) tableCount(ctx context.Context, table string) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteIdentifier(table))
	var count int
	if err := a.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		if isMissingRelation(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}

func (a *PostgresDatabaseAdmin) tableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	if err := a.pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = current_schema()
      AND table_name = $1
)
`, table).Scan(&exists); err != nil {
		return false, fmt.Errorf("check table existence for %s: %w", table, err)
	}
	return exists, nil
}

func (a *PostgresDatabaseAdmin) tableColumns(ctx context.Context, table string) ([]tableColumn, error) {
	return a.queryTableColumns(ctx, a.pool, table)
}

func (a *PostgresDatabaseAdmin) tableColumnsTx(ctx context.Context, tx pgx.Tx, table string) ([]tableColumn, error) {
	return a.queryTableColumns(ctx, tx, table)
}

type pgxQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (a *PostgresDatabaseAdmin) queryTableColumns(ctx context.Context, queryer pgxQueryer, table string) ([]tableColumn, error) {
	rows, err := queryer.Query(ctx, `
SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relname = $1
  AND n.nspname = current_schema()
  AND a.attnum > 0
  AND NOT a.attisdropped
  AND a.attgenerated = ''
ORDER BY a.attnum
`, table)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", table, err)
	}
	defer rows.Close()

	columns := make([]tableColumn, 0)
	for rows.Next() {
		var column tableColumn
		if err := rows.Scan(&column.Name, &column.Type); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns for %s: %w", table, err)
	}
	return columns, nil
}

func (a *PostgresDatabaseAdmin) exportTable(ctx context.Context, table string, columns []tableColumn) (json.RawMessage, error) {
	selectList := make([]string, 0, len(columns))
	for _, column := range columns {
		selectList = append(selectList, quoteIdentifier(column.Name))
	}

	query := fmt.Sprintf(`
SELECT COALESCE(jsonb_agg(to_jsonb(src)), '[]'::jsonb)
FROM (
    SELECT %s
    FROM %s
) AS src
`, strings.Join(selectList, ", "), quoteIdentifier(table))

	var raw []byte
	if err := a.pool.QueryRow(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("export %s: %w", table, err)
	}
	return raw, nil
}

func (a *PostgresDatabaseAdmin) importTable(ctx context.Context, tx pgx.Tx, table string, columns []tableColumn, payload json.RawMessage) error {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}

	columnNames := make([]string, 0, len(columns))
	columnDefs := make([]string, 0, len(columns))
	selectCols := make([]string, 0, len(columns))
	for _, column := range columns {
		columnNames = append(columnNames, quoteIdentifier(column.Name))
		columnDefs = append(columnDefs, fmt.Sprintf("%s %s", quoteIdentifier(column.Name), column.Type))
		selectCols = append(selectCols, quoteIdentifier(column.Name))
	}

	query := fmt.Sprintf(`
INSERT INTO %s (%s)
SELECT %s
FROM jsonb_to_recordset($1::jsonb) AS rows(%s)
`, quoteIdentifier(table), strings.Join(columnNames, ", "), strings.Join(selectCols, ", "), strings.Join(columnDefs, ", "))

	if _, err := tx.Exec(ctx, query, payload); err != nil {
		return fmt.Errorf("import %s: %w", table, err)
	}
	return nil
}

func (a *PostgresDatabaseAdmin) resetIDSequence(ctx context.Context, tx pgx.Tx, table string) error {
	columns, err := a.tableColumnsTx(ctx, tx, table)
	if err != nil {
		return err
	}
	hasID := false
	for _, column := range columns {
		if column.Name == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		return nil
	}

	var sequenceName *string
	if err := tx.QueryRow(ctx, `SELECT pg_get_serial_sequence($1, 'id')`, table).Scan(&sequenceName); err != nil {
		return fmt.Errorf("lookup serial sequence for %s: %w", table, err)
	}
	if sequenceName == nil || strings.TrimSpace(*sequenceName) == "" {
		return nil
	}

	query := fmt.Sprintf(`
SELECT setval($1::regclass, COALESCE((SELECT MAX(id) FROM %s), 0) + 1, FALSE)
`, quoteIdentifier(table))
	if _, err := tx.Exec(ctx, query, *sequenceName); err != nil {
		return fmt.Errorf("reset id sequence for %s: %w", table, err)
	}
	return nil
}

func managedBackupTables() []string {
	return []string{
		"settings",
		"proxies",
		"email_services",
		"accounts",
		"cpa_services",
		"sub2api_services",
		"tm_services",
		"jobs",
		"job_runs",
		"job_logs",
		"app_logs",
	}
}

func restoreOrder() []string {
	return []string{
		"settings",
		"proxies",
		"email_services",
		"accounts",
		"cpa_services",
		"sub2api_services",
		"tm_services",
		"jobs",
		"job_runs",
		"job_logs",
		"app_logs",
	}
}

func intersectOrderedTables(tables map[string]json.RawMessage) []string {
	result := make([]string, 0, len(tables))
	seen := make(map[string]struct{}, len(tables))
	for _, table := range restoreOrder() {
		if _, ok := tables[table]; ok {
			result = append(result, table)
			seen[table] = struct{}{}
		}
	}
	extras := make([]string, 0)
	for table := range tables {
		if _, ok := seen[table]; ok {
			continue
		}
		extras = append(extras, table)
	}
	sort.Strings(extras)
	return append(result, extras...)
}

func buildTruncateSQL(tables []string) string {
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		quoted = append(quoted, quoteIdentifier(table))
	}
	return fmt.Sprintf("TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(quoted, ", "))
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func isMissingRelation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}
