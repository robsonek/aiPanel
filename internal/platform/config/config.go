// Package config handles app configuration loading (env, YAML-like defaults).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the runtime configuration for aiPanel.
type Config struct {
	Addr              string
	Env               string
	DataDir           string
	DevFrontendProxy  string
	SessionCookieName string
	SessionTTL        time.Duration
}

// Load reads defaults from a simple key/value YAML file and applies AIPANEL_* env overrides.
func Load(path string) (Config, error) {
	cfg := Config{
		Addr:              ":8080",
		Env:               "dev",
		DataDir:           "./data",
		DevFrontendProxy:  "http://localhost:5173",
		SessionCookieName: "aipanel_session",
		SessionTTL:        24 * time.Hour,
	}

	if path != "" {
		if err := mergeFromFile(&cfg, path); err != nil {
			return Config{}, err
		}
	}
	mergeFromEnv(&cfg)
	if err := normalizeDataDir(&cfg, path); err != nil {
		return Config{}, err
	}

	if cfg.Addr == "" {
		return Config{}, fmt.Errorf("addr cannot be empty")
	}
	if cfg.DataDir == "" {
		return Config{}, fmt.Errorf("data_dir cannot be empty")
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("session_ttl_hours must be > 0")
	}
	return cfg, nil
}

func normalizeDataDir(cfg *Config, configPath string) error {
	if cfg.DataDir == "" {
		return nil
	}
	if filepath.IsAbs(cfg.DataDir) {
		return nil
	}

	baseDir := "."
	if strings.TrimSpace(configPath) != "" {
		baseDir = filepath.Dir(configPath)
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolve config directory: %w", err)
	}
	cfg.DataDir = filepath.Clean(filepath.Join(absBaseDir, cfg.DataDir))
	return nil
}

func mergeFromFile(cfg *Config, path string) error {
	// Config path is controlled by the local installation/runtime setup.
	//nolint:gosec // G304
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open config file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		applyKey(cfg, key, val)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan config file: %w", err)
	}
	return nil
}

func mergeFromEnv(cfg *Config) {
	type envMap struct {
		key string
		set func(string)
	}
	maps := []envMap{
		{key: "AIPANEL_ADDR", set: func(v string) { cfg.Addr = v }},
		{key: "AIPANEL_ENV", set: func(v string) { cfg.Env = v }},
		{key: "AIPANEL_DATA_DIR", set: func(v string) { cfg.DataDir = v }},
		{key: "AIPANEL_DEV_FRONTEND_PROXY", set: func(v string) { cfg.DevFrontendProxy = v }},
		{key: "AIPANEL_SESSION_COOKIE_NAME", set: func(v string) { cfg.SessionCookieName = v }},
		{key: "AIPANEL_SESSION_TTL_HOURS", set: func(v string) {
			if h, err := strconv.Atoi(v); err == nil && h > 0 {
				cfg.SessionTTL = time.Duration(h) * time.Hour
			}
		}},
	}
	for _, m := range maps {
		if v, ok := os.LookupEnv(m.key); ok {
			m.set(strings.TrimSpace(v))
		}
	}
}

func applyKey(cfg *Config, key, val string) {
	switch key {
	case "addr":
		cfg.Addr = val
	case "env":
		cfg.Env = val
	case "data_dir":
		cfg.DataDir = val
	case "dev_frontend_proxy":
		cfg.DevFrontendProxy = val
	case "session_cookie_name":
		cfg.SessionCookieName = val
	case "session_ttl_hours":
		if h, err := strconv.Atoi(val); err == nil && h > 0 {
			cfg.SessionTTL = time.Duration(h) * time.Hour
		}
	}
}
