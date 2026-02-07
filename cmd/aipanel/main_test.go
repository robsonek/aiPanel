package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
