package uploader

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresConfigRepositoryListCPAServiceConfigsUsesEnabledServicesByDefault(t *testing.T) {
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

func TestPostgresConfigRepositoryListSub2APIServiceConfigsMapsTargetType(t *testing.T) {
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

func TestPostgresConfigRepositoryListTMServiceConfigsReturnsScanError(t *testing.T) {
	db := &fakeUploaderDB{
		err: errors.New("boom"),
	}
	repo := newPostgresConfigRepository(db)

	_, err := repo.ListTMServiceConfigs(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "query tm services") {
		t.Fatalf("expected tm query error, got %v", err)
	}
}

type fakeUploaderDB struct {
	query string
	args  []any
	rows  pgx.Rows
	err   error
}

func (f *fakeUploaderDB) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	f.query = query
	f.args = args
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
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
