// Package sqlite manages SQLite files (WAL mode) for panel.db, audit.db, queue.db.
package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Store holds paths for panel databases and provides basic SQL helpers.
type Store struct {
	DataDir string
	PanelDB string
	AuditDB string
	QueueDB string
}

// New returns a Store with normalized database file paths.
func New(dataDir string) *Store {
	return &Store{
		DataDir: dataDir,
		PanelDB: filepath.Join(dataDir, "panel.db"),
		AuditDB: filepath.Join(dataDir, "audit.db"),
		QueueDB: filepath.Join(dataDir, "queue.db"),
	}
}

// Init creates DB files, enforces WAL mode, and applies baseline schema.
func (s *Store) Init(ctx context.Context) error {
	if err := os.MkdirAll(s.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	for _, db := range []string{s.PanelDB, s.AuditDB, s.QueueDB} {
		if err := s.exec(ctx, db, "PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
			return fmt.Errorf("enable wal for %s: %w", filepath.Base(db), err)
		}
	}

	panelSchema := `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
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
CREATE INDEX IF NOT EXISTS idx_sites_domain ON sites(domain);
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
`
	if err := s.exec(ctx, s.PanelDB, panelSchema); err != nil {
		return fmt.Errorf("apply panel schema: %w", err)
	}

	auditSchema := `
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  details TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_events(created_at);
`
	if err := s.exec(ctx, s.AuditDB, auditSchema); err != nil {
		return fmt.Errorf("apply audit schema: %w", err)
	}

	queueSchema := `
CREATE TABLE IF NOT EXISTS jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
`
	if err := s.exec(ctx, s.QueueDB, queueSchema); err != nil {
		return fmt.Errorf("apply queue schema: %w", err)
	}

	return nil
}

// ExecPanel executes a write SQL statement against panel.db.
func (s *Store) ExecPanel(ctx context.Context, sql string) error {
	return s.exec(ctx, s.PanelDB, sql)
}

// QueryPanelJSON runs a SELECT against panel.db and parses JSON output.
func (s *Store) QueryPanelJSON(ctx context.Context, sql string) ([]map[string]any, error) {
	return s.queryJSON(ctx, s.PanelDB, sql)
}

// ExecAudit inserts/updates audit data.
func (s *Store) ExecAudit(ctx context.Context, sql string) error {
	return s.exec(ctx, s.AuditDB, sql)
}

func (s *Store) exec(ctx context.Context, dbPath, sql string) error {
	cmd := exec.CommandContext(ctx, "sqlite3", dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 exec: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Store) queryJSON(ctx context.Context, dbPath, sql string) ([]map[string]any, error) {
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite3 query: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var rows []map[string]any
	if len(out) == 0 {
		return rows, nil
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("decode sqlite json: %w", err)
	}
	return rows, nil
}
