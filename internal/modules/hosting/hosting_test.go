package hosting

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
	"github.com/robsonek/aiPanel/pkg/adapter"
)

type fakeNginxAdapter struct {
	writeCalls  []adapter.SiteConfig
	removeCalls []string
	testCalls   int
	reloadCalls int
	failWrite   error
	failTest    error
}

func (f *fakeNginxAdapter) WriteVhost(_ context.Context, site adapter.SiteConfig) error {
	f.writeCalls = append(f.writeCalls, site)
	if f.failWrite != nil {
		return f.failWrite
	}
	return nil
}

func (f *fakeNginxAdapter) RemoveVhost(_ context.Context, domain string) error {
	f.removeCalls = append(f.removeCalls, domain)
	return nil
}

func (f *fakeNginxAdapter) TestConfig(_ context.Context) error {
	f.testCalls++
	return f.failTest
}

func (f *fakeNginxAdapter) Reload(_ context.Context) error {
	f.reloadCalls++
	return nil
}

type fakePHPFPMAdapter struct {
	writeCalls  []adapter.SiteConfig
	removeCalls []string
	restarts    []string
	versions    []string
	failWrite   error
}

func (f *fakePHPFPMAdapter) WritePool(_ context.Context, site adapter.SiteConfig) error {
	f.writeCalls = append(f.writeCalls, site)
	return f.failWrite
}

func (f *fakePHPFPMAdapter) RemovePool(_ context.Context, domain, phpVersion string) error {
	f.removeCalls = append(f.removeCalls, domain+"@"+phpVersion)
	return nil
}

func (f *fakePHPFPMAdapter) Restart(_ context.Context, phpVersion string) error {
	f.restarts = append(f.restarts, phpVersion)
	return nil
}

func (f *fakePHPFPMAdapter) ListVersions(_ context.Context) ([]string, error) {
	if len(f.versions) == 0 {
		return []string{"8.3", "8.4"}, nil
	}
	return f.versions, nil
}

func TestService_CreateSite(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	runner := &fakeRunner{
		errs: map[string]error{
			"id site_test_example_com": fmt.Errorf("no such user"),
		},
	}
	nginx := &fakeNginxAdapter{}
	phpfpm := &fakePHPFPMAdapter{}
	svc := NewService(store, config.Config{}, slog.Default(), runner, nginx, phpfpm)
	svc.webRoot = t.TempDir()

	site, err := svc.CreateSite(ctx, CreateSiteRequest{
		Domain:     "test.example.com",
		PHPVersion: "8.3",
		Actor:      "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if site.Domain != "test.example.com" {
		t.Fatalf("unexpected domain: %s", site.Domain)
	}
	if len(nginx.writeCalls) != 1 {
		t.Fatalf("expected nginx write once, got %d", len(nginx.writeCalls))
	}
	if len(phpfpm.writeCalls) != 1 {
		t.Fatalf("expected phpfpm write once, got %d", len(phpfpm.writeCalls))
	}
	if !containsCommand(runner.commands, "useradd --system --create-home --home-dir "+svc.webRoot+"/test.example.com --shell /usr/sbin/nologin site_test_example_com") {
		t.Fatalf("expected useradd command, got %v", runner.commands)
	}
	list, err := svc.ListSites(ctx)
	if err != nil {
		t.Fatalf("list sites: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one site, got %d", len(list))
	}
}

func TestService_CreateSiteRollbackOnNginxFailure(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	runner := &fakeRunner{
		errs: map[string]error{
			"id site_test_example_com": fmt.Errorf("no such user"),
		},
	}
	nginx := &fakeNginxAdapter{failWrite: fmt.Errorf("boom")}
	phpfpm := &fakePHPFPMAdapter{}
	svc := NewService(store, config.Config{}, slog.Default(), runner, nginx, phpfpm)
	svc.webRoot = t.TempDir()

	_, err := svc.CreateSite(ctx, CreateSiteRequest{
		Domain:     "test.example.com",
		PHPVersion: "8.3",
	})
	if err == nil {
		t.Fatal("expected create site to fail")
	}
	if len(phpfpm.removeCalls) != 1 {
		t.Fatalf("expected pool rollback, got %+v", phpfpm.removeCalls)
	}
	if !containsCommand(runner.commands, "userdel --remove site_test_example_com") {
		t.Fatalf("expected user cleanup, got %v", runner.commands)
	}
}

func TestService_DeleteSite(t *testing.T) {
	ctx := context.Background()
	store := sqlite.New(t.TempDir())
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	runner := &fakeRunner{
		errs: map[string]error{
			"id site_test_example_com": fmt.Errorf("no such user"),
		},
	}
	nginx := &fakeNginxAdapter{}
	phpfpm := &fakePHPFPMAdapter{}
	svc := NewService(store, config.Config{}, slog.Default(), runner, nginx, phpfpm)
	svc.webRoot = t.TempDir()

	site, err := svc.CreateSite(ctx, CreateSiteRequest{
		Domain:     "test.example.com",
		PHPVersion: "8.3",
		Actor:      "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := svc.DeleteSite(ctx, site.ID, "admin@example.com"); err != nil {
		t.Fatalf("delete site: %v", err)
	}
	list, err := svc.ListSites(ctx)
	if err != nil {
		t.Fatalf("list sites: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no sites after delete, got %d", len(list))
	}
	if len(nginx.removeCalls) == 0 {
		t.Fatal("expected nginx remove call")
	}
	if len(phpfpm.removeCalls) == 0 {
		t.Fatal("expected php-fpm remove call")
	}
}
