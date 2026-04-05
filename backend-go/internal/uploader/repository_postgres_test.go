package uploader

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestServiceConfigRepositoryListCPAServiceConfigsUsesEnabledServicesByDefault(t *testing.T) {
	db := &fakeUploaderDB{
		rows: &fakeUploaderRows{
			values: [][]any{
				{int32(1), "CPA A", "https://cpa-a.example.com", "token-a", "http://proxy-a", true, int32(5)},
			},
		},
	}
	repo := newPostgresConfigRepository(db)

	configs, err := repo.ListCPAServiceConfigs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(db.query, "FROM cpa_services") || !strings.Contains(db.query, "enabled = TRUE") {
		t.Fatalf("expected enabled cpa service query, got %q", db.query)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Kind != UploadKindCPA || configs[0].Credential != "token-a" || configs[0].ProxyURL != "http://proxy-a" {
		t.Fatalf("unexpected mapped cpa config: %+v", configs[0])
	}
}

func TestServiceConfigRepositoryListSub2APIServiceConfigsMapsTargetType(t *testing.T) {
	db := &fakeUploaderDB{
		rows: &fakeUploaderRows{
			values: [][]any{
				{int32(2), "Sub2API A", "https://s2a.example.com", "key-a", " newapi ", true, int32(3)},
			},
		},
	}
	repo := newPostgresConfigRepository(db)

	configs, err := repo.ListSub2APIServiceConfigs(context.Background(), []int{2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(db.query, "FROM sub2api_services") || !strings.Contains(db.query, "id = ANY($1)") {
		t.Fatalf("expected id filtered sub2api query, got %q", db.query)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Kind != UploadKindSub2API || configs[0].TargetType != "newapi" || configs[0].Credential != "key-a" {
		t.Fatalf("unexpected mapped sub2api config: %+v", configs[0])
	}
}

func TestServiceConfigRepositoryListTMServiceConfigsReturnsScanError(t *testing.T) {
	db := &fakeUploaderDB{
		err: errors.New("boom"),
	}
	repo := newPostgresConfigRepository(db)

	_, err := repo.ListTMServiceConfigs(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "query tm services") {
		t.Fatalf("expected tm query error, got %v", err)
	}
}

func TestServiceConfigRepositoryCreateServiceConfigUsesKindSpecificColumns(t *testing.T) {
	db := &fakeUploaderDB{
		row: fakeUploaderRow{
			values: []any{
				int32(7),
				"Sub2API New",
				"https://sub2api.example.com",
				"sub2api-key",
				"newapi",
				true,
				int32(8),
				testUploaderTime,
				testUploaderTime,
			},
		},
	}
	repo := newPostgresConfigRepository(db)

	created, err := repo.CreateServiceConfig(context.Background(), ManagedServiceConfig{
		ServiceConfig: ServiceConfig{
			Kind:       UploadKindSub2API,
			Name:       "Sub2API New",
			BaseURL:    "https://sub2api.example.com",
			Credential: "sub2api-key",
			TargetType: "newapi",
			Enabled:    true,
			Priority:   8,
		},
	})
	if err != nil {
		t.Fatalf("create service config: %v", err)
	}

	if !strings.Contains(db.queryRowQuery, "INSERT INTO sub2api_services") || !strings.Contains(db.queryRowQuery, "target_type") {
		t.Fatalf("expected sub2api insert query, got %q", db.queryRowQuery)
	}
	if created.Kind != UploadKindSub2API || created.TargetType != "newapi" || created.Credential != "sub2api-key" {
		t.Fatalf("unexpected created config: %+v", created)
	}
}

func TestServiceConfigRepositoryUpdateAndDeleteServiceConfigPreserveKindSpecificFields(t *testing.T) {
	db := &fakeUploaderDB{
		row: fakeUploaderRow{
			values: []any{
				int32(9),
				"CPA Updated",
				"https://cpa.example.com",
				"cpa-token",
				"http://proxy.internal",
				false,
				int32(3),
				testUploaderTime,
				testUploaderTime,
			},
		},
	}
	repo := newPostgresConfigRepository(db)

	proxyURL := "http://proxy.internal"
	enabled := false
	priority := 3
	updated, found, err := repo.UpdateServiceConfig(context.Background(), UploadKindCPA, 9, ManagedServiceConfigPatch{
		Name:     stringPointer("CPA Updated"),
		BaseURL:  stringPointer("https://cpa.example.com"),
		ProxyURL: &proxyURL,
		Enabled:  &enabled,
		Priority: &priority,
	})
	if err != nil {
		t.Fatalf("update service config: %v", err)
	}
	if !found {
		t.Fatal("expected updated config to be found")
	}
	if !strings.Contains(db.queryRowQuery, "UPDATE cpa_services") || !strings.Contains(db.queryRowQuery, "proxy_url") {
		t.Fatalf("expected cpa update query, got %q", db.queryRowQuery)
	}
	if updated.ProxyURL != "http://proxy.internal" || updated.Enabled != false {
		t.Fatalf("unexpected updated config: %+v", updated)
	}

	db.row = fakeUploaderRow{
		values: []any{
			int32(9),
			"CPA Updated",
			"https://cpa.example.com",
			"cpa-token",
			"http://proxy.internal",
			false,
			int32(3),
			testUploaderTime,
			testUploaderTime,
		},
	}
	deleted, deletedFound, err := repo.DeleteServiceConfig(context.Background(), UploadKindCPA, 9)
	if err != nil {
		t.Fatalf("delete service config: %v", err)
	}
	if !deletedFound {
		t.Fatal("expected deleted config to be found")
	}
	if !strings.Contains(db.queryRowQuery, "DELETE FROM cpa_services") {
		t.Fatalf("expected cpa delete query, got %q", db.queryRowQuery)
	}
	if deleted.ID != 9 || deleted.Name != "CPA Updated" {
		t.Fatalf("unexpected deleted config: %+v", deleted)
	}
}

type fakeUploaderDB struct {
	query         string
	args          []any
	rows          pgx.Rows
	err           error
	queryRowQuery string
	queryRowArgs  []any
	row           fakeUploaderRow
}

func (f *fakeUploaderDB) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	f.query = query
	f.args = args
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func (f *fakeUploaderDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.queryRowQuery = query
	f.queryRowArgs = args
	if f.err != nil {
		return fakeUploaderRow{err: f.err}
	}
	return f.row
}

type fakeUploaderRow struct {
	values []any
	err    error
}

func (r fakeUploaderRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch out := dest[i].(type) {
		case *int32:
			*out = r.values[i].(int32)
		case *string:
			*out = r.values[i].(string)
		case *bool:
			*out = r.values[i].(bool)
		case **time.Time:
			value := r.values[i].(time.Time)
			cloned := value
			*out = &cloned
		default:
			return errors.New("unsupported row dest type")
		}
	}
	return nil
}

type fakeUploaderRows struct {
	values [][]any
	index  int
	err    error
}

func (r *fakeUploaderRows) Close() {}

func (r *fakeUploaderRows) Err() error { return r.err }

func (r *fakeUploaderRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeUploaderRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeUploaderRows) Next() bool {
	if r.err != nil {
		return false
	}
	return r.index < len(r.values)
}

func (r *fakeUploaderRows) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	row := r.values[r.index]
	r.index++
	for i := range dest {
		switch out := dest[i].(type) {
		case *int32:
			*out = row[i].(int32)
		case *string:
			*out = row[i].(string)
		case *bool:
			*out = row[i].(bool)
		default:
			return errors.New("unsupported dest type")
		}
	}
	return nil
}

func (r *fakeUploaderRows) Values() ([]any, error) { return nil, nil }

func (r *fakeUploaderRows) RawValues() [][]byte { return nil }

func (r *fakeUploaderRows) Conn() *pgx.Conn { return nil }

var testUploaderTime = time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
