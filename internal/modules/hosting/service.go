package hosting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
	"github.com/robsonek/aiPanel/internal/platform/systemd"
	"github.com/robsonek/aiPanel/pkg/adapter"
)

var (
	// ErrSiteNotFound indicates missing site row.
	ErrSiteNotFound = errors.New("site not found")
)

const defaultPHPVersion = "8.5"

// Service orchestrates site CRUD against adapters and panel.db.
type Service struct {
	store   *sqlite.Store
	cfg     config.Config
	log     *slog.Logger
	runner  systemd.Runner
	nginx   adapter.Nginx
	phpfpm  adapter.PHPFPM
	webRoot string
}

// NewService creates a hosting service.
func NewService(
	store *sqlite.Store,
	cfg config.Config,
	log *slog.Logger,
	runner systemd.Runner,
	nginx adapter.Nginx,
	phpfpm adapter.PHPFPM,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	return &Service{
		store:   store,
		cfg:     cfg,
		log:     log,
		runner:  runner,
		nginx:   nginx,
		phpfpm:  phpfpm,
		webRoot: "/var/www",
	}
}

// CreateSite creates system user, docroot, PHP pool, Nginx vhost and DB row.
func (s *Service) CreateSite(ctx context.Context, req CreateSiteRequest) (Site, error) {
	if s.store == nil || s.nginx == nil || s.phpfpm == nil {
		return Site{}, fmt.Errorf("hosting service is not fully configured")
	}

	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		return Site{}, err
	}
	versions, err := s.phpfpm.ListVersions(ctx)
	if err != nil {
		return Site{}, fmt.Errorf("list php versions: %w", err)
	}
	phpVersion := strings.TrimSpace(req.PHPVersion)
	if phpVersion == "" {
		if len(versions) > 0 {
			availableVersions := slices.Clone(versions)
			slices.Sort(availableVersions)
			phpVersion = availableVersions[len(availableVersions)-1]
		} else {
			phpVersion = defaultPHPVersion
		}
	}
	if !phpVersionPattern.MatchString(phpVersion) {
		return Site{}, fmt.Errorf("invalid php version")
	}
	if len(versions) > 0 && !slices.Contains(versions, phpVersion) {
		return Site{}, fmt.Errorf("php version %s is not installed", phpVersion)
	}

	rootBaseDir := filepath.Join(s.webRoot, domain)
	rootDir := filepath.Join(rootBaseDir, "public_html")
	systemUser := systemUserForDomain(domain)
	siteCfg := adapter.SiteConfig{
		Domain:     domain,
		RootDir:    rootDir,
		PHPVersion: phpVersion,
		SystemUser: systemUser,
	}

	var createdUser bool
	var createdRootBase bool
	var poolWritten bool
	var vhostWritten bool

	defer func() {
		if err == nil {
			return
		}
		if vhostWritten {
			_ = s.nginx.RemoveVhost(ctx, domain)
		}
		if poolWritten {
			_ = s.phpfpm.RemovePool(ctx, domain, phpVersion)
			_ = s.phpfpm.Restart(ctx, phpVersion)
		}
		if createdUser {
			_, _ = s.runner.Run(ctx, "userdel", "--remove", systemUser)
		}
		if createdRootBase {
			_ = os.RemoveAll(rootBaseDir)
		}
	}()

	if _, statErr := os.Stat(rootBaseDir); os.IsNotExist(statErr) {
		createdRootBase = true
	}
	if err = os.MkdirAll(rootDir, 0o750); err != nil {
		return Site{}, fmt.Errorf("create docroot: %w", err)
	}

	if _, runErr := s.runner.Run(ctx, "id", systemUser); runErr != nil {
		if _, runErr = s.runner.Run(ctx,
			"useradd",
			"--system",
			"--create-home",
			"--home-dir", rootBaseDir,
			"--shell", "/usr/sbin/nologin",
			systemUser,
		); runErr != nil {
			return Site{}, fmt.Errorf("create system user: %w", runErr)
		}
		createdUser = true
	}
	if _, runErr := s.runner.Run(ctx, "chown", "-R", systemUser+":"+systemUser, rootBaseDir); runErr != nil {
		return Site{}, fmt.Errorf("chown site directory: %w", runErr)
	}

	if err = s.phpfpm.WritePool(ctx, siteCfg); err != nil {
		return Site{}, fmt.Errorf("write php-fpm pool: %w", err)
	}
	poolWritten = true
	if err = s.phpfpm.Restart(ctx, phpVersion); err != nil {
		return Site{}, fmt.Errorf("restart php-fpm: %w", err)
	}

	if err = s.nginx.WriteVhost(ctx, siteCfg); err != nil {
		return Site{}, fmt.Errorf("write nginx vhost: %w", err)
	}
	vhostWritten = true
	if err = s.nginx.TestConfig(ctx); err != nil {
		return Site{}, fmt.Errorf("test nginx config: %w", err)
	}
	if err = s.nginx.Reload(ctx); err != nil {
		return Site{}, fmt.Errorf("reload nginx: %w", err)
	}

	nowUnix := time.Now().Unix()
	insert := fmt.Sprintf(`
INSERT INTO sites(domain, root_dir, php_version, system_user, status, created_at, updated_at)
VALUES('%s','%s','%s','%s','active',%d,%d);`,
		sqlEscape(domain),
		sqlEscape(rootDir),
		sqlEscape(phpVersion),
		sqlEscape(systemUser),
		nowUnix,
		nowUnix,
	)
	if err = s.store.ExecPanel(ctx, insert); err != nil {
		return Site{}, fmt.Errorf("insert site: %w", err)
	}
	_ = s.writeAudit(ctx, req.Actor, "hosting.site.create", "domain="+domain)

	site, err := s.getSiteByDomain(ctx, domain)
	if err != nil {
		return Site{}, err
	}
	return site, nil
}

// ListSites returns all sites ordered by newest first.
func (s *Service) ListSites(ctx context.Context) ([]Site, error) {
	if s.store == nil {
		return nil, fmt.Errorf("hosting service is not configured")
	}
	rows, err := s.store.QueryPanelJSON(ctx, `
SELECT id, domain, root_dir, php_version, system_user, status, created_at, updated_at
FROM sites
ORDER BY id DESC;`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	sites := make([]Site, 0, len(rows))
	for _, row := range rows {
		site, convErr := mapRowToSite(row)
		if convErr != nil {
			return nil, convErr
		}
		sites = append(sites, site)
	}
	return sites, nil
}

// GetSite returns a site by id.
func (s *Service) GetSite(ctx context.Context, id int64) (Site, error) {
	if s.store == nil {
		return Site{}, fmt.Errorf("hosting service is not configured")
	}
	query := fmt.Sprintf(`
SELECT id, domain, root_dir, php_version, system_user, status, created_at, updated_at
FROM sites
WHERE id = %d
LIMIT 1;`, id)
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return Site{}, fmt.Errorf("get site: %w", err)
	}
	if len(rows) == 0 {
		return Site{}, ErrSiteNotFound
	}
	return mapRowToSite(rows[0])
}

// DeleteSite removes vhost, PHP pool, system user, content and DB row.
func (s *Service) DeleteSite(ctx context.Context, id int64, actor string) error {
	if s.store == nil || s.nginx == nil || s.phpfpm == nil {
		return fmt.Errorf("hosting service is not fully configured")
	}
	site, err := s.GetSite(ctx, id)
	if err != nil {
		return err
	}

	siteCfg := adapter.SiteConfig{
		Domain:     site.Domain,
		RootDir:    site.RootDir,
		PHPVersion: site.PHPVersion,
		SystemUser: site.SystemUser,
	}

	if err = s.nginx.RemoveVhost(ctx, site.Domain); err != nil {
		return fmt.Errorf("remove nginx vhost: %w", err)
	}
	if err = s.phpfpm.RemovePool(ctx, site.Domain, site.PHPVersion); err != nil {
		_ = s.nginx.WriteVhost(ctx, siteCfg)
		return fmt.Errorf("remove php-fpm pool: %w", err)
	}
	if err = s.nginx.TestConfig(ctx); err != nil {
		_ = s.nginx.WriteVhost(ctx, siteCfg)
		_ = s.phpfpm.WritePool(ctx, siteCfg)
		_ = s.phpfpm.Restart(ctx, site.PHPVersion)
		return fmt.Errorf("test nginx config: %w", err)
	}
	if err = s.phpfpm.Restart(ctx, site.PHPVersion); err != nil {
		return fmt.Errorf("restart php-fpm: %w", err)
	}
	if err = s.nginx.Reload(ctx); err != nil {
		return fmt.Errorf("reload nginx: %w", err)
	}

	_, _ = s.runner.Run(ctx, "userdel", "--remove", site.SystemUser)

	rootBaseDir := filepath.Dir(site.RootDir)
	if withinBase(rootBaseDir, s.webRoot) {
		_ = os.RemoveAll(rootBaseDir)
	}

	del := fmt.Sprintf("DELETE FROM sites WHERE id = %d;", id)
	if err = s.store.ExecPanel(ctx, del); err != nil {
		return fmt.Errorf("delete site row: %w", err)
	}
	_ = s.writeAudit(ctx, actor, "hosting.site.delete", "domain="+site.Domain)
	return nil
}

func (s *Service) getSiteByDomain(ctx context.Context, domain string) (Site, error) {
	query := fmt.Sprintf(`
SELECT id, domain, root_dir, php_version, system_user, status, created_at, updated_at
FROM sites
WHERE domain = '%s'
LIMIT 1;`, sqlEscape(domain))
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil {
		return Site{}, fmt.Errorf("get site by domain: %w", err)
	}
	if len(rows) == 0 {
		return Site{}, ErrSiteNotFound
	}
	return mapRowToSite(rows[0])
}

func mapRowToSite(row map[string]any) (Site, error) {
	id, err := toInt64(row["id"])
	if err != nil {
		return Site{}, err
	}
	domain, _ := row["domain"].(string)
	rootDir, _ := row["root_dir"].(string)
	phpVersion, _ := row["php_version"].(string)
	systemUser, _ := row["system_user"].(string)
	status, _ := row["status"].(string)
	createdAtUnix, err := toInt64(row["created_at"])
	if err != nil {
		return Site{}, err
	}
	updatedAtUnix, err := toInt64(row["updated_at"])
	if err != nil {
		return Site{}, err
	}
	return Site{
		ID:         id,
		Domain:     domain,
		RootDir:    rootDir,
		PHPVersion: phpVersion,
		SystemUser: systemUser,
		Status:     status,
		CreatedAt:  time.Unix(createdAtUnix, 0).UTC(),
		UpdatedAt:  time.Unix(updatedAtUnix, 0).UTC(),
	}, nil
}

func systemUserForDomain(domain string) string {
	token := strings.ReplaceAll(sanitizeToken(domain), "-", "_")
	if len(token) > 24 {
		token = token[:24]
	}
	return "site_" + token
}

func withinBase(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func sqlEscape(in string) string {
	return strings.ReplaceAll(in, "'", "''")
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported int conversion type %T", v)
	}
}

func (s *Service) writeAudit(ctx context.Context, actor, action, details string) error {
	if s.store == nil {
		return nil
	}
	if strings.TrimSpace(actor) == "" {
		actor = "system"
	}
	sql := fmt.Sprintf(
		"INSERT INTO audit_events(actor, action, details, created_at) VALUES('%s','%s','%s',%d);",
		sqlEscape(actor),
		sqlEscape(action),
		sqlEscape(details),
		time.Now().Unix(),
	)
	return s.store.ExecAudit(ctx, sql)
}
