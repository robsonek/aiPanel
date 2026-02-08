package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPostgreSQLAdapter_CommandSequence(t *testing.T) {
	r := &fakeRunner{}
	ad := NewPostgreSQLAdapter(r)

	if err := ad.CreateDatabase(context.Background(), "site_db"); err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := ad.CreateUser(context.Background(), "site_user", "secret123", "site_db"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := ad.DropUser(context.Background(), "site_user"); err != nil {
		t.Fatalf("drop user: %v", err)
	}
	if err := ad.DropDatabase(context.Background(), "site_db"); err != nil {
		t.Fatalf("drop db: %v", err)
	}

	joined := strings.Join(r.commands, "\n")
	if !strings.Contains(joined, "runuser -u postgres -- psql -v ON_ERROR_STOP=1 -d postgres -c CREATE DATABASE \"site_db\";") {
		t.Fatalf("missing create database command:\n%s", joined)
	}
	if !strings.Contains(joined, "GRANT ALL PRIVILEGES ON DATABASE \"site_db\" TO \"site_user\";") {
		t.Fatalf("missing grant command:\n%s", joined)
	}
	if !strings.Contains(joined, "DROP ROLE IF EXISTS \"site_user\";") {
		t.Fatalf("missing drop user command:\n%s", joined)
	}
}

func TestPostgreSQLAdapter_IsRunning(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"systemctl is-active postgresql": "active\n",
		},
	}
	ad := NewPostgreSQLAdapter(r)
	ok, err := ad.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("is running: %v", err)
	}
	if !ok {
		t.Fatal("expected running status true")
	}
}

func TestPostgreSQLAdapter_IsRunningInactive(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"systemctl is-active postgresql": "inactive\n",
		},
		errs: map[string]error{
			"systemctl is-active postgresql": fmt.Errorf("exit status 3"),
		},
	}
	ad := NewPostgreSQLAdapter(r)
	ok, err := ad.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("is running should not fail on inactive: %v", err)
	}
	if ok {
		t.Fatal("expected running status false")
	}
}
