package sqlite

import (
	"context"
	"testing"
)

func TestStoreInit_MigratesSiteDatabasesUniqueConstraint(t *testing.T) {
	ctx := context.Background()
	store := New(t.TempDir())

	legacySchema := `
CREATE TABLE IF NOT EXISTS sites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  domain TEXT NOT NULL UNIQUE,
  root_dir TEXT NOT NULL,
  php_version TEXT NOT NULL DEFAULT '8.5',
  system_user TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at)
VALUES('legacy.example.com', '/var/www/legacy.example.com/public_html', '8.5', 'site_legacy', 'active', 1, 1);
CREATE TABLE IF NOT EXISTS site_databases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id INTEGER NOT NULL,
  db_name TEXT NOT NULL UNIQUE,
  db_user TEXT NOT NULL,
  db_engine TEXT NOT NULL DEFAULT 'mariadb',
  created_at INTEGER NOT NULL,
  FOREIGN KEY(site_id) REFERENCES sites(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_site_databases_site_id ON site_databases(site_id);
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(1, 'shared_db', 'legacy_user', 'mariadb', 1);
`
	if err := store.exec(ctx, store.PanelDB, legacySchema); err != nil {
		t.Fatalf("seed legacy panel schema: %v", err)
	}

	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if err := store.exec(ctx, store.PanelDB, `
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(1, 'shared_db', 'postgres_user', 'postgres', 2);`); err != nil {
		t.Fatalf("insert same db_name for postgres should pass after migration: %v", err)
	}

	rows, err := store.queryJSON(ctx, store.PanelDB, `
SELECT db_engine, db_name
FROM site_databases
WHERE db_name = 'shared_db'
ORDER BY id;`)
	if err != nil {
		t.Fatalf("query migrated rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for shared_db across engines, got %d", len(rows))
	}
}
