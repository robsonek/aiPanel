package iam

import (
	"context"
	"testing"
	"time"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/logger"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
)

func TestIAM_CreateLoginAuthenticateLogout(t *testing.T) {
	cfg := config.Config{
		Addr:              ":8080",
		Env:               "test",
		DataDir:           t.TempDir(),
		DevFrontendProxy:  "",
		SessionCookieName: "aipanel_session",
		SessionTTL:        time.Hour,
	}
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite: %v", err)
	}

	svc := NewService(store, cfg, logger.New("test"))
	if err := svc.CreateAdmin(context.Background(), "admin@example.com", "supersecret123"); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	session, err := svc.Login(context.Background(), "admin@example.com", "supersecret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.Token == "" {
		t.Fatal("expected non-empty session token")
	}
	if session.User.Role != "admin" {
		t.Fatalf("expected role admin, got %q", session.User.Role)
	}

	user, err := svc.Authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.Email != "admin@example.com" {
		t.Fatalf("expected authenticated email admin@example.com, got %q", user.Email)
	}

	if err := svc.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), session.Token); err == nil {
		t.Fatal("expected auth to fail after logout")
	}
}
