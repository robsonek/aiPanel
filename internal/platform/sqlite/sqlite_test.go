package sqlite

import (
	"context"
	"testing"
)

func TestStoreInit_AllowsSameDatabaseNameAcrossEngines(t *testing.T) {
	ctx := context.Background()
	store := New(t.TempDir())

	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.exec(ctx, store.PanelDB, `
INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at)
VALUES('fresh.example.com', '/var/www/fresh.example.com/public_html', '8.5', 'site_fresh', 'active', 1, 1);`); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	if err := store.exec(ctx, store.PanelDB, `
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(1, 'shared_db', 'maria_user', 'mariadb', 1);`); err != nil {
		t.Fatalf("insert mariadb db: %v", err)
	}
	if err := store.exec(ctx, store.PanelDB, `
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(1, 'shared_db', 'postgres_user', 'postgres', 2);`); err != nil {
		t.Fatalf("insert postgres db with same name: %v", err)
	}

	rows, err := store.queryJSON(ctx, store.PanelDB, `
SELECT db_engine, db_name
FROM site_databases
WHERE db_name = 'shared_db'
ORDER BY id;`)
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for shared_db across engines, got %d", len(rows))
	}
}
