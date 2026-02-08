package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/robsonek/aiPanel/internal/installer"
	"github.com/robsonek/aiPanel/internal/modules/iam"
	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/logger"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
)

func TestNewHandler_ServesHealth(t *testing.T) {
	cfg := config.Config{
		Addr:              ":8080",
		Env:               "test",
		DataDir:           t.TempDir(),
		DevFrontendProxy:  "",
		SessionCookieName: "aipanel_session",
		SessionTTL:        24 * time.Hour,
	}
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite: %v", err)
	}
	iamSvc := iam.NewService(store, cfg, logger.New("test"))
	handler := newHandler(cfg, logger.New("test"), iamSvc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid health json: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", payload["status"])
	}
}

func TestNewHandler_ServesIndexHTML(t *testing.T) {
	cfg := config.Config{
		Addr:              ":8080",
		Env:               "prod",
		DataDir:           t.TempDir(),
		DevFrontendProxy:  "",
		SessionCookieName: "aipanel_session",
		SessionTTL:        24 * time.Hour,
	}
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite: %v", err)
	}
	iamSvc := iam.NewService(store, cfg, logger.New("test"))
	handler := newHandler(cfg, logger.New("test"), iamSvc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("response body does not contain <html")
	}
}

func TestNewHandler_ProtectsAPI(t *testing.T) {
	cfg := config.Config{
		Addr:              ":8080",
		Env:               "test",
		DataDir:           t.TempDir(),
		DevFrontendProxy:  "",
		SessionCookieName: "aipanel_session",
		SessionTTL:        24 * time.Hour,
	}
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite: %v", err)
	}
	iamSvc := iam.NewService(store, cfg, logger.New("test"))
	handler := newHandler(cfg, logger.New("test"), iamSvc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestIsHelpArg(t *testing.T) {
	tests := map[string]bool{
		"-h":     true,
		"--help": true,
		"help":   true,
		"serve":  false,
		"":       false,
	}
	for arg, want := range tests {
		if got := isHelpArg(arg); got != want {
			t.Fatalf("isHelpArg(%q)=%v want %v", arg, got, want)
		}
	}
}

func TestMissingTools(t *testing.T) {
	originalLookup := lookupCommandPath
	defer func() { lookupCommandPath = originalLookup }()

	lookupCommandPath = func(name string) (string, error) {
		if name == "sqlite3" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}

	missing := missingTools([]string{"bash", "sqlite3", ""})
	if len(missing) != 1 || missing[0] != "sqlite3" {
		t.Fatalf("unexpected missing tools: %v", missing)
	}
}

func TestEnsureRequiredTools(t *testing.T) {
	originalLookup := lookupCommandPath
	defer func() { lookupCommandPath = originalLookup }()

	lookupCommandPath = func(name string) (string, error) {
		return "", errors.New("not found")
	}

	err := ensureRequiredTools("serve", []string{"sqlite3", "gnupg"})
	if err == nil {
		t.Fatal("expected missing tools error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing required system tools for serve:") {
		t.Fatalf("unexpected error message: %s", msg)
	}
	if !strings.Contains(msg, "install with: sudo apt-get update && sudo apt-get install -y") {
		t.Fatalf("expected install hint in error message: %s", msg)
	}
}

func TestPromptInstallOptions_UsesDefaults(t *testing.T) {
	defaults := installer.DefaultOptions()
	input := "\n\n\n"
	out := &bytes.Buffer{}

	opts, dryRun, err := promptInstallOptions(defaults, strings.NewReader(input), out)
	if err != nil {
		t.Fatalf("promptInstallOptions error: %v", err)
	}
	if dryRun {
		t.Fatal("expected dryRun=false by default")
	}
	if opts.Addr != defaults.Addr {
		t.Fatalf("addr mismatch: got %q want %q", opts.Addr, defaults.Addr)
	}
	if opts.RuntimeChannel != defaults.RuntimeChannel {
		t.Fatalf("runtime channel mismatch: got %q want %q", opts.RuntimeChannel, defaults.RuntimeChannel)
	}
	if !opts.VerifyUpstreamSources {
		t.Fatal("expected VerifyUpstreamSources=true")
	}
}

func TestPromptInstallOptions_Cancel(t *testing.T) {
	defaults := installer.DefaultOptions()
	input := "\n\nn\n"

	_, _, err := promptInstallOptions(defaults, strings.NewReader(input), &bytes.Buffer{})
	if !errors.Is(err, errInstallCancelled) {
		t.Fatalf("expected errInstallCancelled, got %v", err)
	}
}

func TestPromptInstallOptions_CustomMode(t *testing.T) {
	defaults := installer.DefaultOptions()
	input := strings.Join([]string{
		"n",
		":18080",
		"ops@aipanel.dev",
		"VeryStrongPass123!",
		"edge",
		"y",
		"y",
		"panel.example.com",
		"y",
		"tls-admin@aipanel.dev",
		"n",
		"y",
	}, "\n") + "\n"
	out := &bytes.Buffer{}

	opts, dryRun, err := promptInstallOptions(defaults, strings.NewReader(input), out)
	if err != nil {
		t.Fatalf("promptInstallOptions error: %v", err)
	}
	if dryRun {
		t.Fatal("expected dryRun=false")
	}
	if opts.Addr != "127.0.0.1:18080" {
		t.Fatalf("addr mismatch: got %q", opts.Addr)
	}
	if opts.AdminEmail != "ops@aipanel.dev" {
		t.Fatalf("admin email mismatch: got %q", opts.AdminEmail)
	}
	if opts.AdminPassword != "VeryStrongPass123!" {
		t.Fatalf("admin password mismatch: got %q", opts.AdminPassword)
	}
	if opts.RuntimeChannel != "edge" {
		t.Fatalf("runtime channel mismatch: got %q", opts.RuntimeChannel)
	}
	if !opts.ReverseProxy {
		t.Fatal("expected reverse proxy enabled")
	}
	if opts.PanelDomain != "panel.example.com" {
		t.Fatalf("panel domain mismatch: got %q", opts.PanelDomain)
	}
	if !opts.EnableLetsEncrypt {
		t.Fatal("expected letsencrypt enabled")
	}
	if opts.LetsEncryptEmail != "tls-admin@aipanel.dev" {
		t.Fatalf("letsencrypt email mismatch: got %q", opts.LetsEncryptEmail)
	}
}

func TestPromptInstallOptions_CustomModeRePromptsShortPassword(t *testing.T) {
	defaults := installer.DefaultOptions()
	input := strings.Join([]string{
		"n",
		":18080",
		"ops@aipanel.dev",
		"short",
		"VeryStrongPass123!",
		"stable",
		"n",
		"n",
		"n",
		"y",
	}, "\n") + "\n"
	out := &bytes.Buffer{}

	opts, dryRun, err := promptInstallOptions(defaults, strings.NewReader(input), out)
	if err != nil {
		t.Fatalf("promptInstallOptions error: %v", err)
	}
	if dryRun {
		t.Fatal("expected dryRun=false")
	}
	if opts.AdminPassword != "VeryStrongPass123!" {
		t.Fatalf("admin password mismatch: got %q", opts.AdminPassword)
	}
	if !strings.Contains(out.String(), "admin password must be at least") {
		t.Fatalf("expected validation message in output, got: %q", out.String())
	}
}

func TestPromptInstallOptions_RePromptsInvalidLetsEncryptEmail(t *testing.T) {
	defaults := installer.DefaultOptions()
	input := strings.Join([]string{
		"y",
		"y",
		"aipanel.onee.my",
		"y",
		"",
		"admin@example.com",
		"ops@aipanel.dev",
		"y",
	}, "\n") + "\n"
	out := &bytes.Buffer{}

	opts, dryRun, err := promptInstallOptions(defaults, strings.NewReader(input), out)
	if err != nil {
		t.Fatalf("promptInstallOptions error: %v", err)
	}
	if dryRun {
		t.Fatal("expected dryRun=false")
	}
	if opts.LetsEncryptEmail != "ops@aipanel.dev" {
		t.Fatalf("letsencrypt email mismatch: got %q", opts.LetsEncryptEmail)
	}
	if !strings.Contains(strings.ToLower(out.String()), "letsencrypt email is required") {
		t.Fatalf("expected empty email validation message in output, got: %q", out.String())
	}
	if !strings.Contains(strings.ToLower(out.String()), "placeholder domain") {
		t.Fatalf("expected placeholder email validation message in output, got: %q", out.String())
	}
}

func TestInstallFlagValuesToOptions_RejectsShortAdminPassword(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	if err := fs.Parse([]string{"--admin-password", "short"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, _, err := values.toOptions(defaults)
	if err == nil || !strings.Contains(err.Error(), "admin password must be at least") {
		t.Fatalf("expected short password validation error, got %v", err)
	}
}

func TestInstallFlagValuesToOptions_OnlyStep(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	if err := fs.Parse([]string{"--only", "install_phpmyadmin"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	opts, _, err := values.toOptions(defaults)
	if err != nil {
		t.Fatalf("toOptions error: %v", err)
	}
	if opts.OnlyStep != "install_phpmyadmin" {
		t.Fatalf("only step mismatch: got %q", opts.OnlyStep)
	}
}

func TestInstallFlagValuesToOptions_OnlyStepPGAdminEnablesPGAdmin(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	if err := fs.Parse([]string{"--only", "install_pgadmin"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	opts, _, err := values.toOptions(defaults)
	if err != nil {
		t.Fatalf("toOptions error: %v", err)
	}
	if opts.OnlyStep != "install_pgadmin" {
		t.Fatalf("only step mismatch: got %q", opts.OnlyStep)
	}
	if opts.SkipPGAdmin {
		t.Fatal("expected pgAdmin to be enabled for install_pgadmin only step")
	}
}

func TestInstallFlagValuesToOptions_RuntimeLockURL(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	if err := fs.Parse([]string{
		"--runtime-lock-path", "",
		"--runtime-lock-url", "https://example.com/custom-lock.json",
	}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	opts, _, err := values.toOptions(defaults)
	if err != nil {
		t.Fatalf("toOptions error: %v", err)
	}
	if opts.RuntimeLockPath != "" {
		t.Fatalf("expected runtime lock path to be empty, got %q", opts.RuntimeLockPath)
	}
	if opts.RuntimeLockURL != "https://example.com/custom-lock.json" {
		t.Fatalf("runtime lock URL mismatch: got %q", opts.RuntimeLockURL)
	}
}

func TestInstallFlagValuesToOptions_LetsEncrypt(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	args := []string{
		"--reverse-proxy",
		"--panel-domain", "panel.example.com",
		"--lets-encrypt",
		"--lets-encrypt-email", "ops@aipanel.dev",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	opts, _, err := values.toOptions(defaults)
	if err != nil {
		t.Fatalf("toOptions error: %v", err)
	}
	if !opts.EnableLetsEncrypt {
		t.Fatal("expected letsencrypt enabled")
	}
	if opts.LetsEncryptEmail != "ops@aipanel.dev" {
		t.Fatalf("letsencrypt email mismatch: got %q", opts.LetsEncryptEmail)
	}
}

func TestInstallFlagValuesToOptions_LetsEncryptRequiresReverseProxy(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	args := []string{
		"--lets-encrypt",
		"--lets-encrypt-email", "ops@aipanel.dev",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, _, err := values.toOptions(defaults)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "letsencrypt requires --reverse-proxy") {
		t.Fatalf("expected letsencrypt reverse proxy validation error, got %v", err)
	}
}

func TestInstallFlagValuesToOptions_LetsEncryptRequiresEmail(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	args := []string{
		"--reverse-proxy",
		"--panel-domain", "panel.example.com",
		"--lets-encrypt",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, _, err := values.toOptions(defaults)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "letsencrypt email is required") {
		t.Fatalf("expected letsencrypt email validation error, got %v", err)
	}
}

func TestInstallFlagValuesToOptions_LetsEncryptRejectsPlaceholderEmail(t *testing.T) {
	defaults := installer.DefaultOptions()
	fs, values := newInstallFlagSet(defaults)
	args := []string{
		"--reverse-proxy",
		"--panel-domain", "panel.example.com",
		"--lets-encrypt",
		"--lets-encrypt-email", "admin@example.com",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	_, _, err := values.toOptions(defaults)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "placeholder domain") {
		t.Fatalf("expected placeholder email validation error, got %v", err)
	}
}

func TestApplyReverseProxySettings_RequiresDomain(t *testing.T) {
	opts := installer.DefaultOptions()
	err := applyReverseProxySettings(&opts, true, "")
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}
