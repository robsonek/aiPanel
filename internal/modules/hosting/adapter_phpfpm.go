package hosting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/robsonek/aiPanel/internal/platform/systemd"
	"github.com/robsonek/aiPanel/pkg/adapter"
)

const (
	defaultPHPFPMTemplate = "configs/templates/phpfpm_pool.conf.tmpl"
	defaultPHPBaseDir     = "/etc/php"
)

var phpVersionPattern = regexp.MustCompile(`^\d+\.\d+$`)

// PHPFPMAdapterOptions controls filesystem locations used by the adapter.
type PHPFPMAdapterOptions struct {
	TemplatePath string
	PHPBaseDir   string
}

// PHPFPMAdapter manages per-site PHP-FPM pools.
type PHPFPMAdapter struct {
	runner       systemd.Runner
	templatePath string
	phpBaseDir   string
}

// NewPHPFPMAdapter constructs a PHP-FPM adapter with sane defaults.
func NewPHPFPMAdapter(runner systemd.Runner, opts PHPFPMAdapterOptions) *PHPFPMAdapter {
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	if opts.TemplatePath == "" {
		opts.TemplatePath = defaultPHPFPMTemplate
	}
	if opts.PHPBaseDir == "" {
		opts.PHPBaseDir = defaultPHPBaseDir
	}
	return &PHPFPMAdapter{
		runner:       runner,
		templatePath: opts.TemplatePath,
		phpBaseDir:   opts.PHPBaseDir,
	}
}

// WritePool renders and writes a PHP-FPM pool config for the site.
func (a *PHPFPMAdapter) WritePool(_ context.Context, site adapter.SiteConfig) error {
	domain, err := normalizeDomain(site.Domain)
	if err != nil {
		return err
	}
	if !phpVersionPattern.MatchString(site.PHPVersion) {
		return fmt.Errorf("invalid php version")
	}
	if site.SystemUser == "" {
		return fmt.Errorf("system user is required")
	}
	pool := poolName(domain, site.PHPVersion)
	targetDir := filepath.Join(a.phpBaseDir, site.PHPVersion, "fpm", "pool.d")
	targetPath := filepath.Join(targetDir, pool+".conf")

	model := map[string]string{
		"Domain":     domain,
		"RootDir":    site.RootDir,
		"PHPVersion": site.PHPVersion,
		"SystemUser": site.SystemUser,
		"PoolName":   pool,
		"SocketPath": socketPath(domain, site.PHPVersion),
	}
	content, err := renderTemplateFile(a.templatePath, model)
	if err != nil {
		return fmt.Errorf("render php-fpm pool template: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return fmt.Errorf("create php-fpm pool dir: %w", err)
	}
	if err := os.WriteFile(targetPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write php-fpm pool file: %w", err)
	}
	return nil
}

// RemovePool removes a per-site PHP-FPM pool config.
func (a *PHPFPMAdapter) RemovePool(_ context.Context, domain, phpVersion string) error {
	domain, err := normalizeDomain(domain)
	if err != nil {
		return err
	}
	if !phpVersionPattern.MatchString(phpVersion) {
		return fmt.Errorf("invalid php version")
	}
	path := filepath.Join(a.phpBaseDir, phpVersion, "fpm", "pool.d", poolName(domain, phpVersion)+".conf")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove php-fpm pool file: %w", err)
	}
	return nil
}

// Restart restarts the given PHP-FPM systemd unit.
func (a *PHPFPMAdapter) Restart(ctx context.Context, phpVersion string) error {
	if !phpVersionPattern.MatchString(phpVersion) {
		return fmt.Errorf("invalid php version")
	}
	if _, err := a.runner.Run(ctx, "systemctl", "restart", "php"+phpVersion+"-fpm"); err != nil {
		return fmt.Errorf("restart php-fpm %s: %w", phpVersion, err)
	}
	return nil
}

// ListVersions returns installed PHP versions detected under the base directory.
func (a *PHPFPMAdapter) ListVersions(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(a.phpBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read php base dir: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		v := strings.TrimSpace(entry.Name())
		if phpVersionPattern.MatchString(v) {
			versions = append(versions, v)
		}
	}
	slices.Sort(versions)
	return versions, nil
}
