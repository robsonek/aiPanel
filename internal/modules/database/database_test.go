package database

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
)

type fakeMariaDB struct {
	createDBCalls   []string
	dropDBCalls     []string
	createUserCalls []string
	dropUserCalls   []string
	failCreateDB    error
	failCreateUser  error
}

func (f *fakeMariaDB) CreateDatabase(_ context.Context, dbName string) error {
	f.createDBCalls = append(f.createDBCalls, dbName)
	return f.failCreateDB
}

func (f *fakeMariaDB) DropDatabase(_ context.Context, dbName string) error {
	f.dropDBCalls = append(f.dropDBCalls, dbName)
	return nil
}

func (f *fakeMariaDB) CreateUser(_ context.Context, username, password, dbName string) error {
	f.createUserCalls = append(f.createUserCalls, username+"@"+dbName+":"+password)
	return f.failCreateUser
}

func (f *fakeMariaDB) DropUser(_ context.Context, username string) error {
	f.dropUserCalls = append(f.dropUserCalls, username)
	return nil
}

func (f *fakeMariaDB) IsRunning(_ context.Context) (bool, error) {
	return true, nil
}

type fakePostgreSQL struct {
	createDBCalls   []string
	dropDBCalls     []string
	createUserCalls []string
	dropUserCalls   []string
	failCreateDB    error
	failCreateUser  error
}

func (f *fakePostgreSQL) CreateDatabase(_ context.Context, dbName string) error {
	f.createDBCalls = append(f.createDBCalls, dbName)
	return f.failCreateDB
}

func (f *fakePostgreSQL) DropDatabase(_ context.Context, dbName string) error {
	f.dropDBCalls = append(f.dropDBCalls, dbName)
	return nil
}

func (f *fakePostgreSQL) CreateUser(_ context.Context, username, password, dbName string) error {
	f.createUserCalls = append(f.createUserCalls, username+"@"+dbName+":"+password)
	return f.failCreateUser
}

func (f *fakePostgreSQL) DropUser(_ context.Context, username string) error {
	f.dropUserCalls = append(f.dropUserCalls, username)
	return nil
}

func (f *fakePostgreSQL) IsRunning(_ context.Context) (bool, error) {
	return true, nil
}

func TestService_CreateListDeleteDatabase(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	mariadb := &fakeMariaDB{}
	postgres := &fakePostgreSQL{}
	svc := NewService(store, config.Config{}, slog.Default(), mariadb, postgres)

	res, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID: 1,
		DBName: "test_db",
		Actor:  "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if res.Password == "" {
		t.Fatal("expected generated password")
	}
	if len(mariadb.createDBCalls) != 1 || mariadb.createDBCalls[0] != "test_db" {
		t.Fatalf("unexpected create db calls: %+v", mariadb.createDBCalls)
	}

	list, err := svc.ListDatabases(ctx, 1)
	if err != nil {
		t.Fatalf("list dbs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one db, got %d", len(list))
	}

	if err := svc.DeleteDatabase(ctx, list[0].ID, "admin@example.com"); err != nil {
		t.Fatalf("delete db: %v", err)
	}
	if len(mariadb.dropUserCalls) != 1 {
		t.Fatalf("expected user drop call, got %+v", mariadb.dropUserCalls)
	}
	if len(mariadb.dropDBCalls) != 1 || mariadb.dropDBCalls[0] != "test_db" {
		t.Fatalf("unexpected drop db calls: %+v", mariadb.dropDBCalls)
	}
}

func TestService_CreateDatabaseRollbackOnCreateUserFailure(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	mariadb := &fakeMariaDB{failCreateUser: fmt.Errorf("boom")}
	postgres := &fakePostgreSQL{}
	svc := NewService(store, config.Config{}, slog.Default(), mariadb, postgres)

	_, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID: 1,
		DBName: "test_db",
	})
	if err == nil {
		t.Fatal("expected create db to fail")
	}
	if len(mariadb.dropDBCalls) != 1 || mariadb.dropDBCalls[0] != "test_db" {
		t.Fatalf("expected db rollback call, got %+v", mariadb.dropDBCalls)
	}
}

func TestService_CreateDatabaseNormalizesName(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	mariadb := &fakeMariaDB{}
	postgres := &fakePostgreSQL{}
	svc := NewService(store, config.Config{}, slog.Default(), mariadb, postgres)

	res, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID: 1,
		DBName: "fhdfgh.com-prod",
	})
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if res.Database.DBName != "fhdfgh_com_prod" {
		t.Fatalf("expected normalized db name, got %q", res.Database.DBName)
	}
	if len(mariadb.createDBCalls) != 1 || mariadb.createDBCalls[0] != "fhdfgh_com_prod" {
		t.Fatalf("unexpected create db calls: %+v", mariadb.createDBCalls)
	}
}

func TestService_CreateDeletePostgreSQLDatabase(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	mariadb := &fakeMariaDB{}
	postgres := &fakePostgreSQL{}
	svc := NewService(store, config.Config{}, slog.Default(), mariadb, postgres)

	res, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID:   1,
		DBName:   "pg_db",
		DBEngine: DBEnginePostgreSQL,
		Actor:    "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if res.Database.DBEngine != DBEnginePostgreSQL {
		t.Fatalf("expected postgres engine, got %q", res.Database.DBEngine)
	}
	if len(postgres.createDBCalls) != 1 || postgres.createDBCalls[0] != "pg_db" {
		t.Fatalf("unexpected postgres create db calls: %+v", postgres.createDBCalls)
	}
	if len(mariadb.createDBCalls) != 0 {
		t.Fatalf("unexpected mariadb calls: %+v", mariadb.createDBCalls)
	}

	list, err := svc.ListDatabases(ctx, 1)
	if err != nil {
		t.Fatalf("list dbs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one db, got %d", len(list))
	}
	if list[0].DBEngine != DBEnginePostgreSQL {
		t.Fatalf("expected stored postgres engine, got %q", list[0].DBEngine)
	}

	if err := svc.DeleteDatabase(ctx, list[0].ID, "admin@example.com"); err != nil {
		t.Fatalf("delete db: %v", err)
	}
	if len(postgres.dropUserCalls) != 1 {
		t.Fatalf("expected postgres user drop call, got %+v", postgres.dropUserCalls)
	}
	if len(postgres.dropDBCalls) != 1 || postgres.dropDBCalls[0] != "pg_db" {
		t.Fatalf("unexpected postgres drop db calls: %+v", postgres.dropDBCalls)
	}
}

func TestService_CreateDatabase_AllowsSameNameAcrossEngines(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	mariadb := &fakeMariaDB{}
	postgres := &fakePostgreSQL{}
	svc := NewService(store, config.Config{}, slog.Default(), mariadb, postgres)

	first, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID:   1,
		DBName:   "shared_db",
		DBEngine: DBEngineMariaDB,
	})
	if err != nil {
		t.Fatalf("create mariadb db: %v", err)
	}
	second, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID:   1,
		DBName:   "shared_db",
		DBEngine: DBEnginePostgreSQL,
	})
	if err != nil {
		t.Fatalf("create postgres db: %v", err)
	}
	if first.Database.ID == second.Database.ID {
		t.Fatalf("expected distinct records for same name across engines")
	}
	if first.Database.DBEngine != DBEngineMariaDB {
		t.Fatalf("expected mariadb for first record, got %q", first.Database.DBEngine)
	}
	if second.Database.DBEngine != DBEnginePostgreSQL {
		t.Fatalf("expected postgres for second record, got %q", second.Database.DBEngine)
	}
	if len(mariadb.createDBCalls) != 1 || mariadb.createDBCalls[0] != "shared_db" {
		t.Fatalf("unexpected mariadb create calls: %+v", mariadb.createDBCalls)
	}
	if len(postgres.createDBCalls) != 1 || postgres.createDBCalls[0] != "shared_db" {
		t.Fatalf("unexpected postgres create calls: %+v", postgres.createDBCalls)
	}
}

func TestService_CreateDatabaseRejectsInvalidEngine(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := store.ExecPanel(ctx, "INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at) VALUES('test.example.com','/var/www/test.example.com/public_html','8.3','site_test','active',1,1);"); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	svc := NewService(store, config.Config{}, slog.Default(), &fakeMariaDB{}, &fakePostgreSQL{})

	_, err := svc.CreateDatabase(ctx, CreateDatabaseRequest{
		SiteID:   1,
		DBName:   "app_db",
		DBEngine: "oracle",
	})
	if err == nil || err.Error() != "invalid database engine" {
		t.Fatalf("expected invalid engine error, got %v", err)
	}
}
