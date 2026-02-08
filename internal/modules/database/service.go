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

const (
	// DBEngineMariaDB marks MariaDB-backed site database metadata.
	DBEngineMariaDB = "mariadb"
	// DBEnginePostgreSQL marks PostgreSQL-backed site database metadata.
	DBEnginePostgreSQL = "postgres"
)

type databaseProvisioner interface {
	CreateDatabase(ctx context.Context, dbName string) error
	DropDatabase(ctx context.Context, dbName string) error
	CreateUser(ctx context.Context, username, password, dbName string) error
	DropUser(ctx context.Context, username string) error
}

// Service orchestrates database engine CRUD and panel metadata persistence.
type Service struct {
	store      *sqlite.Store
	cfg        config.Config
	log        *slog.Logger
	mariadb    adapter.MariaDB
	postgresql adapter.PostgreSQL
}

// NewService creates a database service.
func NewService(
	store *sqlite.Store,
	cfg config.Config,
	log *slog.Logger,
	mariadb adapter.MariaDB,
	postgresql adapter.PostgreSQL,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:      store,
		cfg:        cfg,
		log:        log,
		mariadb:    mariadb,
		postgresql: postgresql,
	}
}

// CreateDatabase provisions DB + user in selected engine and stores metadata.
func (s *Service) CreateDatabase(ctx context.Context, req CreateDatabaseRequest) (CreateDatabaseResult, error) {
	if s.store == nil {
		return CreateDatabaseResult{}, fmt.Errorf("database service is not fully configured")
	}
	if req.SiteID <= 0 {
		return CreateDatabaseResult{}, fmt.Errorf("site_id is required")
	}
	dbName, err := normalizeDatabaseName(req.DBName)
	if err != nil {
		return CreateDatabaseResult{}, err
	}
	if exists, err := s.siteExists(ctx, req.SiteID); err != nil {
		return CreateDatabaseResult{}, err
	} else if !exists {
		return CreateDatabaseResult{}, fmt.Errorf("site not found")
	}

	engine, err := normalizeDatabaseEngine(req.DBEngine)
	if err != nil {
		return CreateDatabaseResult{}, err
	}
	provisioner, err := s.provisionerForEngine(engine)
	if err != nil {
		return CreateDatabaseResult{}, err
	}

	dbUser := dbUserForName(engine, dbName)
	password, err := randomHex(12)
	if err != nil {
		return CreateDatabaseResult{}, fmt.Errorf("generate password: %w", err)
	}

	if err = provisioner.CreateDatabase(ctx, dbName); err != nil {
		return CreateDatabaseResult{}, err
	}
	userCreated := false
	defer func() {
		if err == nil {
			return
		}
		if userCreated {
			_ = provisioner.DropUser(ctx, dbUser)
		}
		_ = provisioner.DropDatabase(ctx, dbName)
	}()

	if err = provisioner.CreateUser(ctx, dbUser, password, dbName); err != nil {
		return CreateDatabaseResult{}, err
	}
	userCreated = true

	nowUnix := time.Now().Unix()
	insert := fmt.Sprintf(`
INSERT INTO site_databases(site_id, db_name, db_user, db_engine, created_at)
VALUES(%d,'%s','%s','%s',%d);`,
		req.SiteID,
		sqlEscape(dbName),
		sqlEscape(dbUser),
		sqlEscape(engine),
		nowUnix,
	)
	if err = s.store.ExecPanel(ctx, insert); err != nil {
		return CreateDatabaseResult{}, fmt.Errorf("insert database row: %w", err)
	}
	_ = s.writeAudit(ctx, req.Actor, "database.create", "db="+dbName+",engine="+engine)

	db, err := s.getByNameAndEngine(ctx, dbName, engine)
	if err != nil {
		return CreateDatabaseResult{}, err
	}

	return CreateDatabaseResult{
		Database: db,
		Password: password,
	}, nil
}

func normalizeDatabaseName(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("invalid database name")
	}
	value = strings.Map(func(r rune) rune {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			return r
		case r == '-' || r == '.' || r == ' ':
			return '_'
		default:
			return '_'
		}
	}, value)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "_")
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	if len(value) > 64 {
		value = value[:64]
	}
	if !databaseNamePattern.MatchString(value) {
		return "", fmt.Errorf("invalid database name")
	}
	return value, nil
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
	if s.store == nil {
		return fmt.Errorf("database service is not fully configured")
	}
	db, err := s.getByID(ctx, id)
	if err != nil {
		return err
	}
	engine, err := normalizeDatabaseEngine(db.DBEngine)
	if err != nil {
		return err
	}
	provisioner, err := s.provisionerForEngine(engine)
	if err != nil {
		return err
	}
	switch engine {
	case DBEnginePostgreSQL:
		if err = provisioner.DropDatabase(ctx, db.DBName); err != nil {
			return err
		}
		if err = provisioner.DropUser(ctx, db.DBUser); err != nil {
			return err
		}
	default:
		if err = provisioner.DropUser(ctx, db.DBUser); err != nil {
			return err
		}
		if err = provisioner.DropDatabase(ctx, db.DBName); err != nil {
			return err
		}
	}
	del := fmt.Sprintf("DELETE FROM site_databases WHERE id = %d;", id)
	if err = s.store.ExecPanel(ctx, del); err != nil {
		return fmt.Errorf("delete database row: %w", err)
	}
	_ = s.writeAudit(ctx, actor, "database.delete", "db="+db.DBName+",engine="+engine)
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

func (s *Service) getByNameAndEngine(ctx context.Context, dbName, dbEngine string) (SiteDatabase, error) {
	query := fmt.Sprintf(`
SELECT id, site_id, db_name, db_user, db_engine, created_at
FROM site_databases
WHERE db_name = '%s' AND db_engine = '%s'
LIMIT 1;`, sqlEscape(dbName), sqlEscape(dbEngine))
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return SiteDatabase{}, fmt.Errorf("get database by name and engine: %w", err)
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
	if strings.TrimSpace(dbEngine) == "" {
		dbEngine = DBEngineMariaDB
	}
	return SiteDatabase{
		ID:        id,
		SiteID:    siteID,
		DBName:    dbName,
		DBUser:    dbUser,
		DBEngine:  dbEngine,
		CreatedAt: time.Unix(createdAtUnix, 0).UTC(),
	}, nil
}

func dbUserForName(engine, dbName string) string {
	base := strings.ToLower(strings.TrimSpace(dbName))
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, base)
	limit := 18
	prefix := "u_"
	if strings.EqualFold(engine, DBEnginePostgreSQL) {
		prefix = "p_"
		limit = 16
	}
	if len(base) > limit {
		base = base[:limit]
	}
	suffix, _ := randomHex(3)
	return prefix + base + "_" + suffix
}

func normalizeDatabaseEngine(raw string) (string, error) {
	engine := strings.ToLower(strings.TrimSpace(raw))
	if engine == "" {
		return DBEngineMariaDB, nil
	}
	switch engine {
	case DBEngineMariaDB, DBEnginePostgreSQL:
		return engine, nil
	default:
		return "", fmt.Errorf("invalid database engine")
	}
}

func (s *Service) provisionerForEngine(engine string) (databaseProvisioner, error) {
	switch engine {
	case DBEngineMariaDB:
		if s.mariadb == nil {
			return nil, fmt.Errorf("database engine mariadb is not configured")
		}
		return s.mariadb, nil
	case DBEnginePostgreSQL:
		if s.postgresql == nil {
			return nil, fmt.Errorf("database engine postgres is not configured")
		}
		return s.postgresql, nil
	default:
		return nil, fmt.Errorf("invalid database engine")
	}
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
