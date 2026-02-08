package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/robsonek/aiPanel/internal/platform/systemd"
)

var mariadbNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

const (
	defaultMariaDBBinaryPath = "/opt/aipanel/runtime/mariadb/current/bin/mariadb"
	defaultMariaDBService    = "aipanel-runtime-mariadb.service"
)

// MariaDBAdapterOptions controls runtime command paths used by the adapter.
type MariaDBAdapterOptions struct {
	BinaryPath  string
	ServiceName string
}

// MariaDBAdapter executes MariaDB commands through system runner.
type MariaDBAdapter struct {
	runner      systemd.Runner
	binaryPath  string
	serviceName string
}

// NewMariaDBAdapter creates a MariaDB adapter.
func NewMariaDBAdapter(runner systemd.Runner, opts ...MariaDBAdapterOptions) *MariaDBAdapter {
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	cfg := MariaDBAdapterOptions{}
	if len(opts) > 0 {
		cfg = opts[0]
	}
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		cfg.BinaryPath = defaultMariaDBBinaryPath
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = defaultMariaDBService
	}
	return &MariaDBAdapter{
		runner:      runner,
		binaryPath:  cfg.BinaryPath,
		serviceName: cfg.ServiceName,
	}
}

// CreateDatabase creates a MariaDB database.
func (a *MariaDBAdapter) CreateDatabase(ctx context.Context, dbName string) error {
	dbName = strings.TrimSpace(dbName)
	if !mariadbNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", dbName)
	if _, err := a.runner.Run(ctx, a.binaryPath, "-e", sql); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}
	return nil
}

// DropDatabase drops a MariaDB database.
func (a *MariaDBAdapter) DropDatabase(ctx context.Context, dbName string) error {
	dbName = strings.TrimSpace(dbName)
	if !mariadbNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", dbName)
	if _, err := a.runner.Run(ctx, a.binaryPath, "-e", sql); err != nil {
		return fmt.Errorf("drop database %s: %w", dbName, err)
	}
	return nil
}

// CreateUser creates user and grants privileges for database.
func (a *MariaDBAdapter) CreateUser(ctx context.Context, username, password, dbName string) error {
	username = strings.TrimSpace(username)
	dbName = strings.TrimSpace(dbName)
	if !mariadbNamePattern.MatchString(username) {
		return fmt.Errorf("invalid username")
	}
	if !mariadbNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid database name")
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("password is required")
	}
	password = strings.ReplaceAll(password, "\\", "\\\\")
	password = strings.ReplaceAll(password, "'", "''")

	sql := strings.Join([]string{
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';", username, password),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", dbName, username),
		"FLUSH PRIVILEGES;",
	}, " ")
	if _, err := a.runner.Run(ctx, a.binaryPath, "-e", sql); err != nil {
		return fmt.Errorf("create user %s: %w", username, err)
	}
	return nil
}

// DropUser drops database user.
func (a *MariaDBAdapter) DropUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if !mariadbNamePattern.MatchString(username) {
		return fmt.Errorf("invalid username")
	}
	sql := fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'; FLUSH PRIVILEGES;", username)
	if _, err := a.runner.Run(ctx, a.binaryPath, "-e", sql); err != nil {
		return fmt.Errorf("drop user %s: %w", username, err)
	}
	return nil
}

// IsRunning reports whether mariadb unit is active.
func (a *MariaDBAdapter) IsRunning(ctx context.Context) (bool, error) {
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
