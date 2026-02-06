package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands []string
	outputs  map[string]string
	errs     map[string]error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, cmd)
	if r.errs != nil {
		if err, ok := r.errs[cmd]; ok {
			out := ""
			if r.outputs != nil {
				out = r.outputs[cmd]
			}
			return out, err
		}
	}
	if r.outputs != nil {
		if out, ok := r.outputs[cmd]; ok {
			return out, nil
		}
	}
	return "", nil
}

func TestMariaDBAdapter_CommandSequence(t *testing.T) {
	r := &fakeRunner{}
	ad := NewMariaDBAdapter(r)

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
	if !strings.Contains(joined, "mariadb -e CREATE DATABASE IF NOT EXISTS `site_db`") {
		t.Fatalf("missing create database command:\n%s", joined)
	}
	if !strings.Contains(joined, "GRANT ALL PRIVILEGES ON `site_db`.* TO 'site_user'@'localhost';") {
		t.Fatalf("missing grant command:\n%s", joined)
	}
	if !strings.Contains(joined, "DROP USER IF EXISTS 'site_user'@'localhost'") {
		t.Fatalf("missing drop user command:\n%s", joined)
	}
}

func TestMariaDBAdapter_IsRunning(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"systemctl is-active mariadb": "active\n",
		},
	}
	ad := NewMariaDBAdapter(r)
	ok, err := ad.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("is running: %v", err)
	}
	if !ok {
		t.Fatal("expected running status true")
	}
}

func TestMariaDBAdapter_IsRunningInactive(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"systemctl is-active mariadb": "inactive\n",
		},
		errs: map[string]error{
			"systemctl is-active mariadb": fmt.Errorf("exit status 3"),
		},
	}
	ad := NewMariaDBAdapter(r)
	ok, err := ad.IsRunning(context.Background())
	if err != nil {
		t.Fatalf("is running should not fail on inactive: %v", err)
	}
	if ok {
		t.Fatal("expected running status false")
	}
}
