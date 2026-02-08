package installer

import (
	"strings"
	"testing"

	"github.com/robsonek/aiPanel/internal/installer/steps"
)

func TestOptionsValidate(t *testing.T) {
	t.Run("valid source-build defaults", func(t *testing.T) {
		opts := DefaultOptions()
		if err := opts.validate(); err != nil {
			t.Fatalf("expected valid options, got %v", err)
		}
	})

	t.Run("invalid install mode", func(t *testing.T) {
		opts := DefaultOptions()
		opts.InstallMode = "unknown"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "invalid install mode") {
			t.Fatalf("expected invalid install mode error, got %v", err)
		}
	})

	t.Run("invalid runtime channel", func(t *testing.T) {
		opts := DefaultOptions()
		opts.RuntimeChannel = "nightly"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "invalid runtime channel") {
			t.Fatalf("expected invalid runtime channel error, got %v", err)
		}
	})

	t.Run("source-build mode validates runtime lock dependency", func(t *testing.T) {
		opts := DefaultOptions()
		opts.InstallMode = InstallModeSourceBuild
		opts.RuntimeLockPath = ""
		opts.RuntimeLockURL = ""
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "requires runtime lock path or runtime lock URL") {
			t.Fatalf("expected source-build dependency validation error, got %v", err)
		}
	})

	t.Run("source-build mode accepts runtime lock URL without local path", func(t *testing.T) {
		opts := DefaultOptions()
		opts.InstallMode = InstallModeSourceBuild
		opts.RuntimeLockPath = ""
		opts.RuntimeLockURL = "https://raw.githubusercontent.com/robsonek/aiPanel/main/configs/sources/lock.json"
		if err := opts.validate(); err != nil {
			t.Fatalf("expected runtime lock URL to satisfy validation, got %v", err)
		}
	})

	t.Run("reverse proxy requires panel domain", func(t *testing.T) {
		opts := DefaultOptions()
		opts.ReverseProxy = true
		opts.PanelDomain = ""
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "panel domain is required") {
			t.Fatalf("expected reverse proxy panel domain validation error, got %v", err)
		}
	})

	t.Run("letsencrypt requires reverse proxy", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableLetsEncrypt = true
		opts.LetsEncryptEmail = "ops@aipanel.dev"
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "letsencrypt requires reverse proxy") {
			t.Fatalf("expected letsencrypt reverse proxy validation error, got %v", err)
		}
	})

	t.Run("letsencrypt requires email", func(t *testing.T) {
		opts := DefaultOptions()
		opts.ReverseProxy = true
		opts.PanelDomain = "panel.example.com"
		opts.EnableLetsEncrypt = true
		opts.LetsEncryptEmail = ""
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "letsencrypt email is required") {
			t.Fatalf("expected letsencrypt email validation error, got %v", err)
		}
	})

	t.Run("letsencrypt rejects placeholder email domain", func(t *testing.T) {
		opts := DefaultOptions()
		opts.ReverseProxy = true
		opts.PanelDomain = "panel.example.com"
		opts.EnableLetsEncrypt = true
		opts.LetsEncryptEmail = "admin@example.com"
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "placeholder domain") {
			t.Fatalf("expected placeholder email validation error, got %v", err)
		}
	})

	t.Run("letsencrypt rejects malformed email", func(t *testing.T) {
		opts := DefaultOptions()
		opts.ReverseProxy = true
		opts.PanelDomain = "panel.example.com"
		opts.EnableLetsEncrypt = true
		opts.LetsEncryptEmail = "invalid-email"
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid letsencrypt email") {
			t.Fatalf("expected malformed email validation error, got %v", err)
		}
	})

	t.Run("admin password must meet minimum length", func(t *testing.T) {
		opts := DefaultOptions()
		opts.AdminPassword = "short"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "admin password must be at least") {
			t.Fatalf("expected admin password length validation error, got %v", err)
		}
	})

	t.Run("phpmyadmin URLs are required when enabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.PHPMyAdminURL = ""
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "phpmyadmin source url") {
			t.Fatalf("expected phpmyadmin source validation error, got %v", err)
		}
	})

	t.Run("invalid only step", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = "not-a-step"
		err := opts.validate()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid installer step") {
			t.Fatalf("expected invalid only step validation error, got %v", err)
		}
	})

	t.Run("only phpmyadmin does not require runtime lock", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = steps.InstallPHPMyAdmin
		opts.RuntimeLockPath = ""
		opts.RuntimeInstallDir = ""
		if err := opts.validate(); err != nil {
			t.Fatalf("expected valid options for only phpmyadmin step, got %v", err)
		}
	})

	t.Run("only pgadmin does not require runtime lock", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = steps.InstallPGAdmin
		opts.RuntimeLockPath = ""
		opts.RuntimeInstallDir = ""
		if err := opts.validate(); err != nil {
			t.Fatalf("expected valid options for only pgadmin step, got %v", err)
		}
	})

	t.Run("runtime component names are valid in only mode", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = "postgresql,mysql,php-fpm,nginx,mariadb"
		if err := opts.validate(); err != nil {
			t.Fatalf("expected runtime component names to be valid, got %v", err)
		}
	})

	t.Run("runtime components require runtime lock", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = "postgresql"
		opts.RuntimeLockPath = ""
		opts.RuntimeLockURL = ""
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "requires runtime lock path or runtime lock URL") {
			t.Fatalf("expected runtime lock requirement for runtime component, got %v", err)
		}
	})

	t.Run("mysql and mariadb component names are distinct", func(t *testing.T) {
		components, runtimeOnly, err := parseRuntimeOnlyComponents("mysql,mariadb")
		if err != nil {
			t.Fatalf("expected mysql and mariadb names to parse, got %v", err)
		}
		if !runtimeOnly {
			t.Fatal("expected runtime component mode")
		}
		if len(components) != 2 {
			t.Fatalf("expected 2 distinct components, got %+v", components)
		}
		if components[0] != "mariadb" || components[1] != "mysql" {
			t.Fatalf("unexpected parsed runtime components: %+v", components)
		}
	})

	t.Run("component aliases are rejected", func(t *testing.T) {
		opts := DefaultOptions()
		opts.OnlyStep = "php"
		err := opts.validate()
		if err == nil || !strings.Contains(err.Error(), "invalid installer step") {
			t.Fatalf("expected invalid installer step for alias php, got %v", err)
		}
	})
}

func TestOptionsWithDefaults(t *testing.T) {
	var opts Options
	opts = opts.withDefaults()

	if opts.InstallMode != InstallModeSourceBuild {
		t.Fatalf("expected install mode %q, got %q", InstallModeSourceBuild, opts.InstallMode)
	}
	if opts.RuntimeChannel != RuntimeChannelStable {
		t.Fatalf("expected runtime channel %q, got %q", RuntimeChannelStable, opts.RuntimeChannel)
	}
	if opts.RuntimeLockPath == "" {
		t.Fatal("expected runtime lock path default to be set")
	}
	if opts.RuntimeLockURL == "" {
		t.Fatal("expected runtime lock URL default to be set")
	}
	if opts.RuntimeInstallDir == "" {
		t.Fatal("expected runtime install dir default to be set")
	}
	if opts.PanelDomain == "" {
		t.Fatal("expected panel domain default to be set")
	}
	if opts.PHPMyAdminURL == "" || opts.PHPMyAdminSHA256URL == "" || opts.PHPMyAdminInstallDir == "" {
		t.Fatal("expected phpMyAdmin defaults to be set")
	}
	if opts.PGAdminURL == "" || opts.PGAdminInstallDir == "" || opts.PGAdminListenAddr == "" || opts.PGAdminRoutePath == "" {
		t.Fatal("expected pgAdmin defaults to be set")
	}
	if !opts.SkipPGAdmin {
		t.Fatal("expected pgAdmin to be disabled by default")
	}
	if opts.LetsEncryptWebroot == "" {
		t.Fatal("expected letsencrypt webroot default to be set")
	}
	if opts.OnlyStep != "" {
		t.Fatalf("expected default only step empty, got %q", opts.OnlyStep)
	}
}
