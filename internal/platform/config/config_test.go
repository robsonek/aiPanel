package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ConfigFileAndEnvOverride(t *testing.T) {
	t.Setenv("AIPANEL_ADDR", ":9999")
	t.Setenv("AIPANEL_ENV", "prod")
	t.Setenv("AIPANEL_SESSION_TTL_HOURS", "48")

	dir := t.TempDir()
	path := filepath.Join(dir, "panel.yaml")
	err := os.WriteFile(path, []byte(`
addr: ":8081"
env: "dev"
data_dir: "./test-data"
session_cookie_name: "test_cookie"
session_ttl_hours: 12
	`), 0o600)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Addr != ":9999" {
		t.Fatalf("expected addr from env, got %q", cfg.Addr)
	}
	if cfg.Env != "prod" {
		t.Fatalf("expected env from env var, got %q", cfg.Env)
	}
	if cfg.DataDir != "./test-data" {
		t.Fatalf("expected data_dir from file, got %q", cfg.DataDir)
	}
	if cfg.SessionCookieName != "test_cookie" {
		t.Fatalf("expected cookie name from file, got %q", cfg.SessionCookieName)
	}
	if got := int(cfg.SessionTTL.Hours()); got != 48 {
		t.Fatalf("expected ttl from env to be 48h, got %dh", got)
	}
}
