package hosting

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/robsonek/aiPanel/internal/platform/systemd"
	"github.com/robsonek/aiPanel/pkg/adapter"
)

const (
	defaultNginxVhostTemplate  = "/etc/aipanel/templates/nginx_vhost.conf.tmpl"
	defaultNginxSitesAvailDir  = "/etc/nginx/sites-available"
	defaultNginxSitesEnableDir = "/etc/nginx/sites-enabled"
	defaultNginxBinaryPath     = "/opt/aipanel/runtime/nginx/current/sbin/nginx"
	defaultNginxConfigPath     = "/opt/aipanel/runtime/nginx/current/conf/nginx.conf"
	defaultNginxServiceName    = "aipanel-runtime-nginx.service"
)

// NginxAdapterOptions controls filesystem locations used by the adapter.
type NginxAdapterOptions struct {
	TemplatePath      string
	SitesAvailableDir string
	SitesEnabledDir   string
	NginxBinaryPath   string
	NginxConfigPath   string
	ServiceName       string
}

// NginxAdapter manages per-site Nginx vhost files.
type NginxAdapter struct {
	runner            systemd.Runner
	templatePath      string
	sitesAvailableDir string
	sitesEnabledDir   string
	nginxBinaryPath   string
	nginxConfigPath   string
	serviceName       string
}

// NewNginxAdapter constructs a Nginx adapter with sane defaults.
func NewNginxAdapter(runner systemd.Runner, opts NginxAdapterOptions) *NginxAdapter {
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	if opts.TemplatePath == "" {
		opts.TemplatePath = defaultNginxVhostTemplate
	}
	if opts.SitesAvailableDir == "" {
		opts.SitesAvailableDir = defaultNginxSitesAvailDir
	}
	if opts.SitesEnabledDir == "" {
		opts.SitesEnabledDir = defaultNginxSitesEnableDir
	}
	if opts.NginxBinaryPath == "" {
		opts.NginxBinaryPath = defaultNginxBinaryPath
	}
	if opts.NginxConfigPath == "" {
		opts.NginxConfigPath = defaultNginxConfigPath
	}
	if opts.ServiceName == "" {
		opts.ServiceName = defaultNginxServiceName
	}
	return &NginxAdapter{
		runner:            runner,
		templatePath:      opts.TemplatePath,
		sitesAvailableDir: opts.SitesAvailableDir,
		sitesEnabledDir:   opts.SitesEnabledDir,
		nginxBinaryPath:   opts.NginxBinaryPath,
		nginxConfigPath:   opts.NginxConfigPath,
		serviceName:       opts.ServiceName,
	}
}

// WriteVhost renders and writes a site vhost config and ensures sites-enabled symlink exists.
func (a *NginxAdapter) WriteVhost(_ context.Context, site adapter.SiteConfig) error {
	domain, err := normalizeDomain(site.Domain)
	if err != nil {
		return err
	}
	if site.RootDir == "" {
		return fmt.Errorf("root_dir is required")
	}
	model := map[string]string{
		"Domain":     domain,
		"RootDir":    site.RootDir,
		"PHPVersion": site.PHPVersion,
		"SystemUser": site.SystemUser,
		"SocketPath": socketPath(domain, site.PHPVersion),
	}

	content, err := renderTemplateFile(a.templatePath, model)
	if err != nil {
		return fmt.Errorf("render nginx vhost template: %w", err)
	}

	availablePath := filepath.Join(a.sitesAvailableDir, domain+".conf")
	enabledPath := filepath.Join(a.sitesEnabledDir, domain+".conf")

	if err := os.MkdirAll(a.sitesAvailableDir, 0o750); err != nil {
		return fmt.Errorf("create sites-available dir: %w", err)
	}
	if err := os.MkdirAll(a.sitesEnabledDir, 0o750); err != nil {
		return fmt.Errorf("create sites-enabled dir: %w", err)
	}
	if err := os.WriteFile(availablePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write vhost config: %w", err)
	}
	if err := os.Remove(enabledPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old vhost symlink: %w", err)
	}
	if err := os.Symlink(availablePath, enabledPath); err != nil {
		return fmt.Errorf("create vhost symlink: %w", err)
	}
	return nil
}

// RemoveVhost removes sites-enabled symlink and sites-available config.
func (a *NginxAdapter) RemoveVhost(_ context.Context, domain string) error {
	domain, err := normalizeDomain(domain)
	if err != nil {
		return err
	}
	availablePath := filepath.Join(a.sitesAvailableDir, domain+".conf")
	enabledPath := filepath.Join(a.sitesEnabledDir, domain+".conf")
	if err := os.Remove(enabledPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove vhost symlink: %w", err)
	}
	if err := os.Remove(availablePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove vhost config: %w", err)
	}
	return nil
}

// TestConfig runs "nginx -t".
func (a *NginxAdapter) TestConfig(ctx context.Context) error {
	if _, err := a.runner.Run(ctx, a.nginxBinaryPath, "-t", "-c", a.nginxConfigPath); err != nil {
		return fmt.Errorf("nginx config test failed: %w", err)
	}
	return nil
}

// Reload reloads the configured Nginx systemd service.
func (a *NginxAdapter) Reload(ctx context.Context) error {
	if _, err := a.runner.Run(ctx, "systemctl", "reload", a.serviceName); err != nil {
		return fmt.Errorf("nginx reload failed: %w", err)
	}
	return nil
}

func renderTemplateFile(path string, data any) (string, error) {
	source, err := os.ReadFile(path) //nolint:gosec // Adapter reads installer-controlled template paths.
	if err != nil {
		return "", err
	}
	tpl, err := template.New(filepath.Base(path)).Parse(string(source))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
