package hosting

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robsonek/aiPanel/pkg/adapter"
)

func TestNginxAdapter_WriteVhostAndSymlink(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "nginx_vhost.conf.tmpl")
	if err := os.WriteFile(templatePath, []byte("server_name {{ .Domain }};\nroot {{ .RootDir }};\nfastcgi_pass {{ .SocketPath }};"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	availDir := filepath.Join(root, "sites-available")
	enabledDir := filepath.Join(root, "sites-enabled")
	ad := NewNginxAdapter(&fakeRunner{}, NginxAdapterOptions{
		TemplatePath:      templatePath,
		SitesAvailableDir: availDir,
		SitesEnabledDir:   enabledDir,
	})

	site := adapter.SiteConfig{
		Domain:     "test.example.com",
		RootDir:    "/var/www/test.example.com/public_html",
		PHPVersion: "8.3",
		SystemUser: "site_test_example_com",
	}
	if err := ad.WriteVhost(context.Background(), site); err != nil {
		t.Fatalf("write vhost: %v", err)
	}

	confPath := filepath.Join(availDir, "test.example.com.conf")
	//nolint:gosec // test reads a file created within temp dir.
	b, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read vhost: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "server_name test.example.com;") {
		t.Fatalf("expected domain in template output, got %q", content)
	}
	if !strings.Contains(content, "/run/php/test-example-com-php83.sock") {
		t.Fatalf("expected socket path in template output, got %q", content)
	}

	linkPath := filepath.Join(enabledDir, "test.example.com.conf")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("read symlink: %v", err)
	}
	if target != confPath {
		t.Fatalf("expected symlink target %q, got %q", confPath, target)
	}
}

func TestNginxAdapter_WriteVhostWithFallbackTemplate(t *testing.T) {
	root := t.TempDir()
	availDir := filepath.Join(root, "sites-available")
	enabledDir := filepath.Join(root, "sites-enabled")
	ad := NewNginxAdapter(&fakeRunner{}, NginxAdapterOptions{
		TemplatePath:      filepath.Join(root, "missing-template.tmpl"),
		SitesAvailableDir: availDir,
		SitesEnabledDir:   enabledDir,
	})

	site := adapter.SiteConfig{
		Domain:     "test.example.com",
		RootDir:    "/var/www/test.example.com/public_html",
		PHPVersion: "8.3",
		SystemUser: "site_test_example_com",
	}
	if err := ad.WriteVhost(context.Background(), site); err != nil {
		t.Fatalf("write vhost with fallback template: %v", err)
	}

	confPath := filepath.Join(availDir, "test.example.com.conf")
	//nolint:gosec // test reads a file created within temp dir.
	b, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read vhost: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "server_name test.example.com;") {
		t.Fatalf("expected fallback template domain output, got %q", content)
	}
	if !strings.Contains(content, "fastcgi_pass unix:/run/php/test-example-com-php83.sock;") {
		t.Fatalf("expected fallback template socket output, got %q", content)
	}
}

func TestNginxAdapter_RemoveVhost(t *testing.T) {
	root := t.TempDir()
	availDir := filepath.Join(root, "sites-available")
	enabledDir := filepath.Join(root, "sites-enabled")
	if err := os.MkdirAll(availDir, 0o750); err != nil {
		t.Fatalf("mkdir avail: %v", err)
	}
	if err := os.MkdirAll(enabledDir, 0o750); err != nil {
		t.Fatalf("mkdir enabled: %v", err)
	}
	confPath := filepath.Join(availDir, "test.example.com.conf")
	linkPath := filepath.Join(enabledDir, "test.example.com.conf")
	if err := os.WriteFile(confPath, []byte("vhost"), 0o600); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	if err := os.Symlink(confPath, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	ad := NewNginxAdapter(&fakeRunner{}, NginxAdapterOptions{
		TemplatePath:      filepath.Join(root, "unused.tmpl"),
		SitesAvailableDir: availDir,
		SitesEnabledDir:   enabledDir,
	})
	if err := ad.RemoveVhost(context.Background(), "test.example.com"); err != nil {
		t.Fatalf("remove vhost: %v", err)
	}
	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Fatalf("expected config removed, got err=%v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("expected symlink removed, got err=%v", err)
	}
}

func TestNginxAdapter_TestConfigAndReload(t *testing.T) {
	r := &fakeRunner{}
	ad := NewNginxAdapter(r, NginxAdapterOptions{})

	if err := ad.TestConfig(context.Background()); err != nil {
		t.Fatalf("test config: %v", err)
	}
	if err := ad.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !containsCommand(r.commands, "nginx -t") {
		t.Fatalf("expected nginx -t command, got %v", r.commands)
	}
	if !containsCommand(r.commands, "systemctl reload nginx") {
		t.Fatalf("expected reload command, got %v", r.commands)
	}
}
