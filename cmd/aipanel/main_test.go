package main

import (
	"context"
	"encoding/json"
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
	handler := newHandler(cfg, logger.New("test"), iamSvc)

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
	handler := newHandler(cfg, logger.New("test"), iamSvc)

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
	handler := newHandler(cfg, logger.New("test"), iamSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
