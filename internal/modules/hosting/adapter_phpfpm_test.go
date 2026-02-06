package hosting

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/robsonek/aiPanel/pkg/adapter"
)

func TestPHPFPMAdapter_WritePoolAndRemovePool(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "pool.tmpl")
	if err := os.WriteFile(templatePath, []byte("[{{ .PoolName }}]\nlisten = {{ .SocketPath }}\nuser = {{ .SystemUser }}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	ad := NewPHPFPMAdapter(&fakeRunner{}, PHPFPMAdapterOptions{
		TemplatePath: templatePath,
		PHPBaseDir:   root,
	})
	site := adapter.SiteConfig{
		Domain:     "test.example.com",
		RootDir:    "/var/www/test.example.com/public_html",
		PHPVersion: "8.3",
		SystemUser: "site_test_example_com",
	}
	if err := ad.WritePool(context.Background(), site); err != nil {
		t.Fatalf("write pool: %v", err)
	}

	path := filepath.Join(root, "8.3", "fpm", "pool.d", "test-example-com-php83.conf")
	//nolint:gosec // test reads a file created within temp dir.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pool: %v", err)
	}
	if !strings.Contains(string(b), "listen = /run/php/test-example-com-php83.sock") {
		t.Fatalf("unexpected pool content: %s", string(b))
	}

	if err := ad.RemovePool(context.Background(), "test.example.com", "8.3"); err != nil {
		t.Fatalf("remove pool: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected pool removed, got err=%v", err)
	}
}

func TestPHPFPMAdapter_WritePoolWithFallbackTemplate(t *testing.T) {
	root := t.TempDir()
	ad := NewPHPFPMAdapter(&fakeRunner{}, PHPFPMAdapterOptions{
		TemplatePath: filepath.Join(root, "missing-template.tmpl"),
		PHPBaseDir:   root,
	})
	site := adapter.SiteConfig{
		Domain:     "test.example.com",
		RootDir:    "/var/www/test.example.com/public_html",
		PHPVersion: "8.3",
		SystemUser: "site_test_example_com",
	}
	if err := ad.WritePool(context.Background(), site); err != nil {
		t.Fatalf("write pool with fallback template: %v", err)
	}

	path := filepath.Join(root, "8.3", "fpm", "pool.d", "test-example-com-php83.conf")
	//nolint:gosec // test reads a file created within temp dir.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pool: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "[test-example-com-php83]") {
		t.Fatalf("expected fallback template pool name output, got %q", content)
	}
	if !strings.Contains(content, "php_admin_value[open_basedir] = /var/www/test.example.com/public_html:/tmp") {
		t.Fatalf("expected fallback template root dir output, got %q", content)
	}
}

func TestPHPFPMAdapter_Restart(t *testing.T) {
	r := &fakeRunner{}
	ad := NewPHPFPMAdapter(r, PHPFPMAdapterOptions{})
	if err := ad.Restart(context.Background(), "8.4"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !containsCommand(r.commands, "systemctl restart php8.4-fpm") {
		t.Fatalf("expected restart command, got %v", r.commands)
	}
}

func TestPHPFPMAdapter_ListVersions(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"8.4", "8.3", "invalid", "bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	ad := NewPHPFPMAdapter(&fakeRunner{}, PHPFPMAdapterOptions{PHPBaseDir: root})
	versions, err := ad.ListVersions(context.Background())
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if !slices.Equal(versions, []string{"8.3", "8.4"}) {
		t.Fatalf("unexpected versions: %v", versions)
	}
}
