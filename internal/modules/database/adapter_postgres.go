package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/robsonek/aiPanel/internal/platform/systemd"
)

var postgresNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

const (
	defaultPostgreSQLCommandPath = "/opt/aipanel/runtime/postgresql/current/bin/psql"
	defaultPostgreSQLService     = "aipanel-runtime-postgresql.service"
	defaultPostgreSQLUser        = "postgres"
)

// PostgreSQLAdapterOptions controls runtime command paths used by the adapter.
type PostgreSQLAdapterOptions struct {
	CommandPath string
	ServiceName string
	RunAsUser   string
}

// PostgreSQLAdapter executes PostgreSQL commands through system runner.
type PostgreSQLAdapter struct {
	runner      systemd.Runner
	commandPath string
	serviceName string
	runAsUser   string
}

// NewPostgreSQLAdapter creates a PostgreSQL adapter.
func NewPostgreSQLAdapter(runner systemd.Runner, opts ...PostgreSQLAdapterOptions) *PostgreSQLAdapter {
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	cfg := PostgreSQLAdapterOptions{}
	if len(opts) > 0 {
		cfg = opts[0]
	}
	if strings.TrimSpace(cfg.CommandPath) == "" {
		cfg.CommandPath = defaultPostgreSQLCommandPath
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = defaultPostgreSQLService
	}
	if strings.TrimSpace(cfg.RunAsUser) == "" {
		cfg.RunAsUser = defaultPostgreSQLUser
	}
	return &PostgreSQLAdapter{
		runner:      runner,
		commandPath: cfg.CommandPath,
		serviceName: cfg.ServiceName,
		runAsUser:   cfg.RunAsUser,
	}
}

// CreateDatabase creates a PostgreSQL database.
func (a *PostgreSQLAdapter) CreateDatabase(ctx context.Context, dbName string) error {
	dbName = strings.TrimSpace(dbName)
	if !postgresNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	sql := fmt.Sprintf("CREATE DATABASE \"%s\";", dbName)
	if err := a.runPSQL(ctx, sql); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}
	return nil
}

// DropDatabase drops a PostgreSQL database.
func (a *PostgreSQLAdapter) DropDatabase(ctx context.Context, dbName string) error {
	dbName = strings.TrimSpace(dbName)
	if !postgresNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	sql := strings.Join([]string{
		fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", dbName),
		fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\";", dbName),
	}, " ")
	if err := a.runPSQL(ctx, sql); err != nil {
		return fmt.Errorf("drop database %s: %w", dbName, err)
	}
	return nil
}

// CreateUser creates user and grants privileges for database.
func (a *PostgreSQLAdapter) CreateUser(ctx context.Context, username, password, dbName string) error {
	username = strings.TrimSpace(username)
	dbName = strings.TrimSpace(dbName)
	if !postgresNamePattern.MatchString(username) {
		return fmt.Errorf("invalid username")
	}
	if !postgresNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("password is required")
	}
	password = strings.ReplaceAll(password, "\\", "\\\\")
	password = strings.ReplaceAll(password, "'", "''")

	createRoleSQL := fmt.Sprintf(
		"DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '%s') THEN CREATE ROLE \"%s\" LOGIN PASSWORD '%s'; ELSE ALTER ROLE \"%s\" LOGIN PASSWORD '%s'; END IF; END $$;",
		username,
		username,
		password,
		username,
		password,
	)
	if err := a.runPSQL(ctx, createRoleSQL); err != nil {
		return fmt.Errorf("create user %s: %w", username, err)
	}
	grantSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE \"%s\" TO \"%s\";", dbName, username)
	if err := a.runPSQL(ctx, grantSQL); err != nil {
		return fmt.Errorf("grant privileges to %s: %w", username, err)
	}
	return nil
}

// DropUser drops PostgreSQL role.
func (a *PostgreSQLAdapter) DropUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if !postgresNamePattern.MatchString(username) {
		return fmt.Errorf("invalid username")
	}
	sql := strings.Join([]string{
		fmt.Sprintf("REASSIGN OWNED BY \"%s\" TO postgres;", username),
		fmt.Sprintf("DROP OWNED BY \"%s\";", username),
		fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", username),
	}, " ")
	if err := a.runPSQL(ctx, sql); err != nil {
		return fmt.Errorf("drop user %s: %w", username, err)
	}
	return nil
}

// IsRunning reports whether postgresql unit is active.
func (a *PostgreSQLAdapter) IsRunning(ctx context.Context) (bool, error) {
	out, err := a.runner.Run(ctx, "systemctl", "is-active", a.serviceName)
	if err != nil {
		trimmed := strings.TrimSpace(strings.ToLower(out + " " + err.Error()))
		if strings.Contains(trimmed, "inactive") || strings.Contains(trimmed, "failed") || strings.Contains(trimmed, "unknown") {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) == "active", nil
}

func (a *PostgreSQLAdapter) runPSQL(ctx context.Context, sql string) error {
	args := []string{
		"-u", a.runAsUser, "--",
		a.commandPath, "-v", "ON_ERROR_STOP=1",
		"-d", "postgres",
		"-c", sql,
	}
	if _, err := a.runner.Run(ctx, "runuser", args...); err != nil {
		return err
	}
	return nil
}
