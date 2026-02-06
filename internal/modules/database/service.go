package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
	"github.com/robsonek/aiPanel/pkg/adapter"
)

var (
	// ErrDatabaseNotFound indicates missing database row.
	ErrDatabaseNotFound = errors.New("database not found")
	databaseNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
)

// Service orchestrates MariaDB CRUD and panel metadata persistence.
type Service struct {
	store   *sqlite.Store
	cfg     config.Config
	log     *slog.Logger
	mariadb adapter.MariaDB
}

// NewService creates a database service.
func NewService(store *sqlite.Store, cfg config.Config, log *slog.Logger, mariadb adapter.MariaDB) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:   store,
		cfg:     cfg,
		log:     log,
		mariadb: mariadb,
	}
}

// CreateDatabase provisions DB + user in MariaDB and stores metadata.
func (s *Service) CreateDatabase(ctx context.Context, req CreateDatabaseRequest) (CreateDatabaseResult, error) {
	if s.store == nil || s.mariadb == nil {
		return CreateDatabaseResult{}, fmt.Errorf("database service is not fully configured")
	}
	if req.SiteID <= 0 {
		return CreateDatabaseResult{}, fmt.Errorf("site_id is required")
	}
	dbName := strings.TrimSpace(req.DBName)
	if !databaseNamePattern.MatchString(dbName) {
		return CreateDatabaseResult{}, fmt.Errorf("invalid database name")
	}
	if exists, err := s.siteExists(ctx, req.SiteID); err != nil {
		return CreateDatabaseResult{}, err
	} else if !exists {
		return CreateDatabaseResult{}, fmt.Errorf("site not found")
	}

	dbUser := dbUserForName(dbName)
	password, err := randomHex(12)
	if err != nil {
		return CreateDatabaseResult{}, fmt.Errorf("generate password: %w", err)
	}

	if err = s.mariadb.CreateDatabase(ctx, dbName); err != nil {
		return CreateDatabaseResult{}, err
	}
	userCreated := false
	defer func() {
		if err == nil {
			return
		}
		if userCreated {
			_ = s.mariadb.DropUser(ctx, dbUser)
		}
		_ = s.mariadb.DropDatabase(ctx, dbName)
	}()

	if err = s.mariadb.CreateUser(ctx, dbUser, password, dbName); err != nil {
		return CreateDatabaseResult{}, err
	}
	userCreated = true

	nowUnix := time.Now().Unix()
	insert := fmt.Sprintf(`
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(%d,'%s','%s','mariadb',%d);`,
		req.SiteID,
		sqlEscape(dbName),
		sqlEscape(dbUser),
		nowUnix,
	)
	if err = s.store.ExecPanel(ctx, insert); err != nil {
		return CreateDatabaseResult{}, fmt.Errorf("insert database row: %w", err)
	}
	_ = s.writeAudit(ctx, req.Actor, "database.create", "db="+dbName)

	db, err := s.getByName(ctx, dbName)
	if err != nil {
		return CreateDatabaseResult{}, err
	}

	return CreateDatabaseResult{
		Database: db,
		Password: password,
	}, nil
}

// ListDatabases returns databases for a site.
func (s *Service) ListDatabases(ctx context.Context, siteID int64) ([]SiteDatabase, error) {
	if s.store == nil {
		return nil, fmt.Errorf("database service is not configured")
	}
	query := fmt.Sprintf(`
SELECT id, site_id, db_name, db_user, db_engine, created_at
FROM site_databases
WHERE site_id = %d
ORDER BY id DESC;`, siteID)
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	result := make([]SiteDatabase, 0, len(rows))
	for _, row := range rows {
		db, convErr := mapRowToDatabase(row)
		if convErr != nil {
			return nil, convErr
		}
		result = append(result, db)
	}
	return result, nil
}

// DeleteDatabase drops DB user + DB and removes metadata row.
func (s *Service) DeleteDatabase(ctx context.Context, id int64, actor string) error {
	if s.store == nil || s.mariadb == nil {
		return fmt.Errorf("database service is not fully configured")
	}
	db, err := s.getByID(ctx, id)
	if err != nil {
		return err
	}
	if err = s.mariadb.DropUser(ctx, db.DBUser); err != nil {
		return err
	}
	if err = s.mariadb.DropDatabase(ctx, db.DBName); err != nil {
		return err
	}
	del := fmt.Sprintf("DELETE FROM site_databases WHERE id = %d;", id)
	if err = s.store.ExecPanel(ctx, del); err != nil {
		return fmt.Errorf("delete database row: %w", err)
	}
	_ = s.writeAudit(ctx, actor, "database.delete", "db="+db.DBName)
	return nil
}

func (s *Service) siteExists(ctx context.Context, siteID int64) (bool, error) {
	query := fmt.Sprintf("SELECT id FROM sites WHERE id = %d LIMIT 1;", siteID)
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return false, fmt.Errorf("check site exists: %w", err)
	}
	return len(rows) > 0, nil
}

func (s *Service) getByID(ctx context.Context, id int64) (SiteDatabase, error) {
	query := fmt.Sprintf(`
SELECT id, site_id, db_name, db_user, db_engine, created_at
FROM site_databases
WHERE id = %d
LIMIT 1;`, id)
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return SiteDatabase{}, fmt.Errorf("get database by id: %w", err)
	}
	if len(rows) == 0 {
		return SiteDatabase{}, ErrDatabaseNotFound
	}
	return mapRowToDatabase(rows[0])
}

func (s *Service) getByName(ctx context.Context, dbName string) (SiteDatabase, error) {
	query := fmt.Sprintf(`
SELECT id, site_id, db_name, db_user, db_engine, created_at
FROM site_databases
WHERE db_name = '%s'
LIMIT 1;`, sqlEscape(dbName))
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return SiteDatabase{}, fmt.Errorf("get database by name: %w", err)
	}
	if len(rows) == 0 {
		return SiteDatabase{}, ErrDatabaseNotFound
	}
	return mapRowToDatabase(rows[0])
}

func mapRowToDatabase(row map[string]any) (SiteDatabase, error) {
	id, err := toInt64(row["id"])
	if err != nil {
		return SiteDatabase{}, err
	}
	siteID, err := toInt64(row["site_id"])
	if err != nil {
		return SiteDatabase{}, err
	}
	createdAtUnix, err := toInt64(row["created_at"])
	if err != nil {
		return SiteDatabase{}, err
	}
	dbName, _ := row["db_name"].(string)
	dbUser, _ := row["db_user"].(string)
	dbEngine, _ := row["db_engine"].(string)
	return SiteDatabase{
		ID:        id,
		SiteID:    siteID,
		DBName:    dbName,
		DBUser:    dbUser,
		DBEngine:  dbEngine,
		CreatedAt: time.Unix(createdAtUnix, 0).UTC(),
	}, nil
}

func dbUserForName(dbName string) string {
	base := strings.ToLower(strings.TrimSpace(dbName))
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, base)
	if len(base) > 18 {
		base = base[:18]
	}
	suffix, _ := randomHex(3)
	return "u_" + base + "_" + suffix
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sqlEscape(in string) string {
	return strings.ReplaceAll(in, "'", "''")
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported int conversion type %T", v)
	}
}

func (s *Service) writeAudit(ctx context.Context, actor, action, details string) error {
	if s.store == nil {
		return nil
	}
	if strings.TrimSpace(actor) == "" {
		actor = "system"
	}
	sql := fmt.Sprintf(
		"INSERT INTO audit_events(actor, action, details, created_at) VALUES('%s','%s','%s',%d);",
		sqlEscape(actor),
		sqlEscape(action),
		sqlEscape(details),
		time.Now().Unix(),
	)
	return s.store.ExecAudit(ctx, sql)
}
