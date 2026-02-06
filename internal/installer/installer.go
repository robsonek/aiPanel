// Package installer provides the one-shot Debian 13 installer orchestrator.
package installer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/robsonek/aiPanel/internal/installer/steps"
	"github.com/robsonek/aiPanel/internal/modules/iam"
	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/logger"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
	"github.com/robsonek/aiPanel/internal/platform/systemd"
)

// Options controls installer behavior.
type Options struct {
	Addr             string
	Env              string
	ConfigPath       string
	DataDir          string
	PanelBinaryPath  string
	SourceBinaryPath string
	UnitFilePath     string
	StateFilePath    string
	ReportFilePath   string
	LogFilePath      string
	AdminEmail       string
	AdminPassword    string

	OSReleasePath string
	MemInfoPath   string
	Proc1ExePath  string
	RootFSPath    string

	NginxSitesAvailableDir string
	NginxSitesEnabledDir   string
	PHPBaseDir             string
	PanelVhostTemplatePath string
	CatchAllTemplatePath   string

	MinCPU      int
	MinMemoryMB int
	MinDiskGB   int

	SkipHealthcheck bool
}

// DefaultOptions returns production defaults for installer phase 1.
func DefaultOptions() Options {
	return Options{
		Addr:                   ":8080",
		Env:                    "prod",
		ConfigPath:             "/etc/aipanel/panel.yaml",
		DataDir:                "/var/lib/aipanel",
		PanelBinaryPath:        "/usr/local/bin/aipanel",
		UnitFilePath:           "/etc/systemd/system/aipanel.service",
		StateFilePath:          "/var/lib/aipanel/.installer-state.json",
		ReportFilePath:         "/var/lib/aipanel/install-report.json",
		LogFilePath:            "/var/log/aipanel/install.log",
		AdminEmail:             "admin@example.com",
		AdminPassword:          "ChangeMe12345!",
		OSReleasePath:          "/etc/os-release",
		MemInfoPath:            "/proc/meminfo",
		Proc1ExePath:           "/proc/1/exe",
		RootFSPath:             "/",
		NginxSitesAvailableDir: "/etc/nginx/sites-available",
		NginxSitesEnabledDir:   "/etc/nginx/sites-enabled",
		PHPBaseDir:             "/etc/php",
		PanelVhostTemplatePath: "configs/templates/nginx_panel_vhost.conf.tmpl",
		CatchAllTemplatePath:   "configs/templates/nginx_catchall.conf.tmpl",
		MinCPU:                 2,
		MinMemoryMB:            1024,
		MinDiskGB:              10,
		SkipHealthcheck:        false,
		SourceBinaryPath:       "",
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if strings.TrimSpace(o.Addr) == "" {
		o.Addr = d.Addr
	}
	if strings.TrimSpace(o.Env) == "" {
		o.Env = d.Env
	}
	if strings.TrimSpace(o.ConfigPath) == "" {
		o.ConfigPath = d.ConfigPath
	}
	if strings.TrimSpace(o.DataDir) == "" {
		o.DataDir = d.DataDir
	}
	if strings.TrimSpace(o.PanelBinaryPath) == "" {
		o.PanelBinaryPath = d.PanelBinaryPath
	}
	if strings.TrimSpace(o.UnitFilePath) == "" {
		o.UnitFilePath = d.UnitFilePath
	}
	if strings.TrimSpace(o.StateFilePath) == "" {
		o.StateFilePath = d.StateFilePath
	}
	if strings.TrimSpace(o.ReportFilePath) == "" {
		o.ReportFilePath = d.ReportFilePath
	}
	if strings.TrimSpace(o.LogFilePath) == "" {
		o.LogFilePath = d.LogFilePath
	}
	if strings.TrimSpace(o.AdminEmail) == "" {
		o.AdminEmail = d.AdminEmail
	}
	if strings.TrimSpace(o.AdminPassword) == "" {
		o.AdminPassword = d.AdminPassword
	}
	if strings.TrimSpace(o.OSReleasePath) == "" {
		o.OSReleasePath = d.OSReleasePath
	}
	if strings.TrimSpace(o.MemInfoPath) == "" {
		o.MemInfoPath = d.MemInfoPath
	}
	if strings.TrimSpace(o.Proc1ExePath) == "" {
		o.Proc1ExePath = d.Proc1ExePath
	}
	if strings.TrimSpace(o.RootFSPath) == "" {
		o.RootFSPath = d.RootFSPath
	}
	if strings.TrimSpace(o.NginxSitesAvailableDir) == "" {
		o.NginxSitesAvailableDir = d.NginxSitesAvailableDir
	}
	if strings.TrimSpace(o.NginxSitesEnabledDir) == "" {
		o.NginxSitesEnabledDir = d.NginxSitesEnabledDir
	}
	if strings.TrimSpace(o.PHPBaseDir) == "" {
		o.PHPBaseDir = d.PHPBaseDir
	}
	if strings.TrimSpace(o.PanelVhostTemplatePath) == "" {
		o.PanelVhostTemplatePath = d.PanelVhostTemplatePath
	}
	if strings.TrimSpace(o.CatchAllTemplatePath) == "" {
		o.CatchAllTemplatePath = d.CatchAllTemplatePath
	}
	if o.MinCPU <= 0 {
		o.MinCPU = d.MinCPU
	}
	if o.MinMemoryMB <= 0 {
		o.MinMemoryMB = d.MinMemoryMB
	}
	if o.MinDiskGB <= 0 {
		o.MinDiskGB = d.MinDiskGB
	}
	return o
}

// StepResult captures one installation step outcome.
type StepResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// Report is the installer JSON report format.
type Report struct {
	InstalledAt string       `json:"installed_at"`
	Status      string       `json:"status"`
	ConfigPath  string       `json:"config_path"`
	DataDir     string       `json:"data_dir"`
	Steps       []StepResult `json:"steps"`
}

type checkpointState struct {
	Completed map[string]bool `json:"completed"`
}

// Installer orchestrates phase 1 setup on Debian 13.
type Installer struct {
	opts   Options
	runner systemd.Runner
	now    func() time.Time
}

// New returns a configured installer.
func New(opts Options, runner systemd.Runner) *Installer {
	opts = opts.withDefaults()
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	return &Installer{
		opts:   opts,
		runner: runner,
		now:    time.Now,
	}
}

// Run executes installer phase 1 with checkpoint-based idempotency.
func (i *Installer) Run(ctx context.Context) (*Report, error) {
	report := &Report{
		InstalledAt: i.now().UTC().Format(time.RFC3339),
		Status:      "in_progress",
		ConfigPath:  i.opts.ConfigPath,
		DataDir:     i.opts.DataDir,
		Steps:       make([]StepResult, 0, len(steps.Ordered)),
	}

	state, err := i.loadState()
	if err != nil {
		return nil, err
	}
	if state.Completed == nil {
		state.Completed = map[string]bool{}
	}

	execStep := func(name string, fn func(context.Context) error) error {
		started := i.now().UTC()
		step := StepResult{
			Name:      name,
			StartedAt: started.Format(time.RFC3339),
		}

		if state.Completed[name] {
			step.Status = "skipped"
			step.FinishedAt = i.now().UTC().Format(time.RFC3339)
			report.Steps = append(report.Steps, step)
			i.logf("[%s] skipped (checkpoint exists)", name)
			return nil
		}

		i.logf("[%s] started", name)
		err := fn(ctx)
		step.FinishedAt = i.now().UTC().Format(time.RFC3339)
		if err != nil {
			step.Status = "failed"
			step.Error = err.Error()
			report.Steps = append(report.Steps, step)
			i.logf("[%s] failed: %v", name, err)
			return err
		}

		step.Status = "ok"
		report.Steps = append(report.Steps, step)
		state.Completed[name] = true
		if err := i.saveState(state); err != nil {
			return fmt.Errorf("save installer checkpoint: %w", err)
		}
		i.logf("[%s] completed", name)
		return nil
	}

	runErr := execStep(steps.Preflight, i.runPreflight)
	if runErr == nil {
		runErr = execStep(steps.SystemUpdate, i.runSystemUpdate)
	}
	if runErr == nil {
		runErr = execStep(steps.AddRepos, i.addRepositories)
	}
	if runErr == nil {
		runErr = execStep(steps.InstallPkgs, i.installPackages)
	}
	if runErr == nil {
		runErr = execStep(steps.PrepareDirs, i.prepareDirectories)
	}
	if runErr == nil {
		runErr = execStep(steps.CopyBinary, i.copyBinary)
	}
	if runErr == nil {
		runErr = execStep(steps.WriteConfig, i.writeConfig)
	}
	if runErr == nil {
		runErr = execStep(steps.CreateUser, i.createServiceUser)
	}
	if runErr == nil {
		runErr = execStep(steps.InstallNginx, i.installNginx)
	}
	if runErr == nil {
		runErr = execStep(steps.InitDatabases, i.initDatabases)
	}
	if runErr == nil {
		runErr = execStep(steps.ConfigureNginx, i.configureNginx)
	}
	if runErr == nil {
		runErr = execStep(steps.ConfigurePHP, i.configurePHPFPM)
	}
	if runErr == nil {
		runErr = execStep(steps.WriteUnit, i.writeUnitFile)
	}
	if runErr == nil {
		runErr = execStep(steps.StartPanel, i.startPanelService)
	}
	if runErr == nil {
		runErr = execStep(steps.CreateAdmin, i.createAdminUser)
	}
	if runErr == nil {
		runErr = execStep(steps.Healthcheck, i.runHealthcheck)
	}

	if runErr != nil {
		report.Status = "failed"
		_ = i.writeReport(report)
		return report, runErr
	}

	report.Status = "ok"
	if err := i.writeReport(report); err != nil {
		return report, err
	}
	i.logf("installation finished successfully")
	return report, nil
}

func (i *Installer) runPreflight(_ context.Context) error {
	release, err := parseOSRelease(i.opts.OSReleasePath)
	if err != nil {
		return fmt.Errorf("read os-release: %w", err)
	}
	if !isDebian13(release) {
		return fmt.Errorf("unsupported OS: installer requires Debian 13 (trixie)")
	}

	target, err := os.Readlink(i.opts.Proc1ExePath)
	if err != nil {
		return fmt.Errorf("read init system link: %w", err)
	}
	if !strings.Contains(strings.ToLower(target), "systemd") {
		return fmt.Errorf("unsupported init system: expected systemd, got %s", target)
	}

	if runtime.NumCPU() < i.opts.MinCPU {
		return fmt.Errorf("insufficient CPU: need at least %d cores", i.opts.MinCPU)
	}

	memMB, err := totalMemoryMB(i.opts.MemInfoPath)
	if err != nil {
		return fmt.Errorf("read memory info: %w", err)
	}
	if memMB < i.opts.MinMemoryMB {
		return fmt.Errorf("insufficient memory: need at least %d MB", i.opts.MinMemoryMB)
	}

	freeGB, err := freeDiskGB(i.opts.RootFSPath)
	if err != nil {
		return fmt.Errorf("read disk stats: %w", err)
	}
	if freeGB < i.opts.MinDiskGB {
		return fmt.Errorf("insufficient disk: need at least %d GB free", i.opts.MinDiskGB)
	}
	return nil
}

func (i *Installer) runSystemUpdate(ctx context.Context) error {
	if _, err := i.runner.Run(ctx, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}
	if _, err := i.runner.Run(ctx, "apt-get", "upgrade", "-y"); err != nil {
		return fmt.Errorf("apt upgrade: %w", err)
	}
	return nil
}

func (i *Installer) addRepositories(ctx context.Context) error {
	if _, err := i.runner.Run(ctx, "apt-get", "install", "-y", "ca-certificates", "curl", "gnupg", "lsb-release"); err != nil {
		i.logf("[add_repositories] fallback to distro packages: %v", err)
		return nil
	}
	if _, err := i.runner.Run(ctx, "mkdir", "-p", "/etc/apt/keyrings"); err != nil {
		i.logf("[add_repositories] cannot create keyring dir, skipping sury repo: %v", err)
		return nil
	}
	if _, err := i.runner.Run(ctx, "bash", "-lc", "curl -fsSL https://packages.sury.org/php/apt.gpg | gpg --dearmor -o /etc/apt/keyrings/sury-php.gpg"); err != nil {
		i.logf("[add_repositories] sury key setup failed, using distro packages: %v", err)
		return nil
	}
	if _, err := i.runner.Run(ctx, "bash", "-lc", "echo \"deb [signed-by=/etc/apt/keyrings/sury-php.gpg] https://packages.sury.org/php/ $(lsb_release -sc) main\" > /etc/apt/sources.list.d/sury-php.list"); err != nil {
		i.logf("[add_repositories] sury repo setup failed, using distro packages: %v", err)
		return nil
	}
	if _, err := i.runner.Run(ctx, "apt-get", "update"); err != nil {
		i.logf("[add_repositories] apt update after sury failed, using distro packages: %v", err)
		return nil
	}
	return nil
}

func (i *Installer) installPackages(ctx context.Context) error {
	primary := []string{
		"apt-get", "install", "-y",
		"nginx",
		"php8.3-fpm",
		"php8.4-fpm",
		"mariadb-server",
		"acl",
		"curl",
		"git",
	}
	if _, err := i.runner.Run(ctx, primary[0], primary[1:]...); err == nil {
		return nil
	}
	// Fallback for environments where php8.4 package is unavailable.
	if _, err := i.runner.Run(
		ctx,
		"apt-get",
		"install",
		"-y",
		"nginx",
		"php8.3-fpm",
		"mariadb-server",
		"acl",
		"curl",
		"git",
	); err != nil {
		return fmt.Errorf("apt install packages: %w", err)
	}
	return nil
}

func (i *Installer) prepareDirectories(_ context.Context) error {
	dirs := map[string]struct{}{
		filepath.Dir(i.opts.ConfigPath):      {},
		i.opts.DataDir:                       {},
		filepath.Dir(i.opts.PanelBinaryPath): {},
		filepath.Dir(i.opts.UnitFilePath):    {},
		filepath.Dir(i.opts.StateFilePath):   {},
		filepath.Dir(i.opts.ReportFilePath):  {},
		filepath.Dir(i.opts.LogFilePath):     {},
	}
	for dir := range dirs {
		if strings.TrimSpace(dir) == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func (i *Installer) copyBinary(_ context.Context) error {
	source := strings.TrimSpace(i.opts.SourceBinaryPath)
	if source == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve current binary: %w", err)
		}
		source = exe
	}
	if source == i.opts.PanelBinaryPath {
		return nil
	}

	// Open source once, copy and hash in a single pass to avoid TOCTOU.
	// Installer controls both source and target paths.
	//nolint:gosec // G304
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source binary: %w", err)
	}
	defer func() {
		_ = src.Close()
	}()

	tmp := i.opts.PanelBinaryPath + ".tmp"
	// Destination path is installer-owned and derived from trusted options.
	//nolint:gosec // G304
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}

	srcHash := sha256.New()
	if _, err := io.Copy(dst, io.TeeReader(src, srcHash)); err != nil {
		_ = dst.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close temp binary: %w", err)
	}

	// Compare with existing destination to skip needless replace.
	dstHash, err := fileSHA256(i.opts.PanelBinaryPath)
	if err == nil && hex.EncodeToString(srcHash.Sum(nil)) == dstHash {
		_ = os.Remove(tmp)
		return nil
	}

	if err := os.Rename(tmp, i.opts.PanelBinaryPath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	// Runtime binary must be executable.
	//nolint:gosec // G302
	if err := os.Chmod(i.opts.PanelBinaryPath, 0o755); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}
	return nil
}

func (i *Installer) writeConfig(_ context.Context) error {
	content := renderPanelConfig(i.opts)
	if err := writeTextFile(i.opts.ConfigPath, content, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (i *Installer) createServiceUser(ctx context.Context) error {
	if _, err := i.runner.Run(ctx, "id", "aipanel"); err == nil {
		return nil // user already exists
	}
	if _, err := i.runner.Run(ctx, "useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "aipanel"); err != nil {
		return fmt.Errorf("create aipanel user: %w", err)
	}
	if _, err := i.runner.Run(ctx, "chown", "-R", "aipanel:aipanel", i.opts.DataDir); err != nil {
		return fmt.Errorf("chown data dir: %w", err)
	}
	return nil
}

func (i *Installer) installNginx(ctx context.Context) error {
	if err := systemd.EnableNow(ctx, i.runner, "nginx"); err != nil {
		return fmt.Errorf("start nginx: %w", err)
	}
	return nil
}

func (i *Installer) initDatabases(ctx context.Context) error {
	store := sqlite.New(i.opts.DataDir)
	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("init sqlite databases: %w", err)
	}
	return nil
}

func (i *Installer) configureNginx(ctx context.Context) error {
	panelPort := parsePort(i.opts.Addr, "8080")
	panelContent, err := renderTemplateWithFallback(
		i.opts.PanelVhostTemplatePath,
		defaultPanelVhostTemplate,
		map[string]string{"PanelPort": panelPort},
	)
	if err != nil {
		return fmt.Errorf("render panel vhost template: %w", err)
	}
	catchallContent, err := renderTemplateWithFallback(
		i.opts.CatchAllTemplatePath,
		defaultCatchallTemplate,
		nil,
	)
	if err != nil {
		return fmt.Errorf("render catchall template: %w", err)
	}

	availDir := i.opts.NginxSitesAvailableDir
	enableDir := i.opts.NginxSitesEnabledDir
	if err := os.MkdirAll(availDir, 0o750); err != nil {
		return fmt.Errorf("create nginx sites-available dir: %w", err)
	}
	if err := os.MkdirAll(enableDir, 0o750); err != nil {
		return fmt.Errorf("create nginx sites-enabled dir: %w", err)
	}

	panelPath := filepath.Join(availDir, "aipanel.conf")
	catchallPath := filepath.Join(availDir, "aipanel-catchall.conf")
	if err := writeTextFile(panelPath, panelContent, 0o644); err != nil {
		return fmt.Errorf("write panel vhost: %w", err)
	}
	if err := writeTextFile(catchallPath, catchallContent, 0o644); err != nil {
		return fmt.Errorf("write catchall vhost: %w", err)
	}

	// Remove default nginx site to avoid duplicate default_server conflict.
	defaultLink := filepath.Join(enableDir, "default")
	if err := os.Remove(defaultLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove default nginx site: %w", err)
	}

	panelLink := filepath.Join(enableDir, "aipanel.conf")
	catchallLink := filepath.Join(enableDir, "aipanel-catchall.conf")
	if err := os.Remove(panelLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old panel symlink: %w", err)
	}
	if err := os.Remove(catchallLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old catchall symlink: %w", err)
	}
	if err := os.Symlink(panelPath, panelLink); err != nil {
		return fmt.Errorf("create panel symlink: %w", err)
	}
	if err := os.Symlink(catchallPath, catchallLink); err != nil {
		return fmt.Errorf("create catchall symlink: %w", err)
	}

	if _, err := i.runner.Run(ctx, "nginx", "-t"); err != nil {
		return fmt.Errorf("test nginx config: %w", err)
	}
	if _, err := i.runner.Run(ctx, "systemctl", "reload", "nginx"); err != nil {
		return fmt.Errorf("reload nginx: %w", err)
	}
	return nil
}

func (i *Installer) configurePHPFPM(ctx context.Context) error {
	versions := []string{"8.3", "8.4"}
	for _, version := range versions {
		path := filepath.Join(i.opts.PHPBaseDir, version, "fpm", "pool.d", "aipanel-default.conf")
		content := fmt.Sprintf(phpPoolTemplate, version, version)
		if err := writeTextFile(path, content, 0o644); err != nil {
			return fmt.Errorf("write php-fpm default pool for %s: %w", version, err)
		}
		if _, err := i.runner.Run(ctx, "systemctl", "restart", "php"+version+"-fpm"); err != nil {
			// Keep installer resilient when only one version is available.
			i.logf("[configure_phpfpm] restart php%s-fpm failed: %v", version, err)
		}
	}
	return nil
}

func (i *Installer) createAdminUser(ctx context.Context) error {
	cfg := config.Config{
		Addr:              i.opts.Addr,
		Env:               i.opts.Env,
		DataDir:           i.opts.DataDir,
		SessionCookieName: "aipanel_session",
		SessionTTL:        24 * time.Hour,
	}
	store := sqlite.New(i.opts.DataDir)
	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("init sqlite before create admin: %w", err)
	}
	iamSvc := iam.NewService(store, cfg, logger.New(cfg.Env))
	email := strings.TrimSpace(i.opts.AdminEmail)
	password := strings.TrimSpace(i.opts.AdminPassword)
	if email == "" {
		email = "admin@example.com"
	}
	if password == "" {
		generated, err := randomPassword()
		if err != nil {
			return fmt.Errorf("generate admin password: %w", err)
		}
		password = generated
	}
	if err := iamSvc.CreateAdmin(ctx, email, password); err != nil {
		// Idempotent reruns can fail with unique email conflict.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			i.logf("[create_admin] admin %s already exists", email)
			return nil
		}
		return fmt.Errorf("create admin user: %w", err)
	}
	if strings.TrimSpace(i.opts.AdminPassword) == "" {
		i.logf("[create_admin] generated admin credentials email=%s password=%s", email, password)
	}
	return nil
}

func (i *Installer) writeUnitFile(_ context.Context) error {
	content := renderSystemdUnit(i.opts)
	if err := writeTextFile(i.opts.UnitFilePath, content, 0o600); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	return nil
}

func (i *Installer) startPanelService(ctx context.Context) error {
	if err := systemd.DaemonReload(ctx, i.runner); err != nil {
		return fmt.Errorf("systemd daemon-reload: %w", err)
	}
	if err := systemd.EnableNow(ctx, i.runner, "aipanel"); err != nil {
		return fmt.Errorf("start aipanel service: %w", err)
	}
	return nil
}

func (i *Installer) runHealthcheck(ctx context.Context) error {
	if i.opts.SkipHealthcheck {
		return nil
	}
	for _, unit := range []string{"nginx", "php8.3-fpm", "php8.4-fpm", "mariadb"} {
		active, err := systemd.IsActive(ctx, i.runner, unit)
		if err != nil {
			return fmt.Errorf("check %s status: %w", unit, err)
		}
		if !active {
			return fmt.Errorf("%s is not active", unit)
		}
	}

	hctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	url := healthURL(i.opts.Addr)
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		req, err := http.NewRequestWithContext(hctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create healthcheck request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		select {
		case <-hctx.Done():
			return fmt.Errorf("healthcheck failed for %s: %w", url, lastErr)
		case <-ticker.C:
		}
	}
}

func (i *Installer) loadState() (*checkpointState, error) {
	st := &checkpointState{Completed: map[string]bool{}}
	// Installer controls state file path.
	//nolint:gosec // G304
	b, err := os.ReadFile(i.opts.StateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}
	if len(b) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}
	return st, nil
}

func (i *Installer) saveState(st *checkpointState) error {
	if err := os.MkdirAll(filepath.Dir(i.opts.StateFilePath), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeBinaryFile(i.opts.StateFilePath, b, 0o600)
}

func (i *Installer) writeReport(report *Report) error {
	if err := os.MkdirAll(filepath.Dir(i.opts.ReportFilePath), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return writeBinaryFile(i.opts.ReportFilePath, b, 0o600)
}

func (i *Installer) logf(format string, args ...any) {
	if strings.TrimSpace(i.opts.LogFilePath) == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(i.opts.LogFilePath), 0o750)
	f, err := os.OpenFile(i.opts.LogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() {
		_ = f.Close()
	}()
	_, _ = fmt.Fprintf(f, "%s %s\n", i.now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func parseOSRelease(path string) (map[string]string, error) {
	// Installer owns the os-release path in runtime options.
	//nolint:gosec // G304
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	res := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		res[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func isDebian13(release map[string]string) bool {
	id := strings.ToLower(strings.TrimSpace(release["ID"]))
	codename := strings.ToLower(strings.TrimSpace(release["VERSION_CODENAME"]))
	versionID := strings.TrimSpace(release["VERSION_ID"])
	return id == "debian" && (codename == "trixie" || versionID == "13")
}

func totalMemoryMB(memInfoPath string) (int, error) {
	// Installer controls meminfo path in runtime options.
	//nolint:gosec // G304
	f, err := os.Open(memInfoPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("invalid MemTotal line")
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, err
		}
		return kb / 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("MemTotal not found")
}

func freeDiskGB(rootPath string) (int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(rootPath, &stat); err != nil {
		return 0, err
	}
	if stat.Bavail <= 0 {
		return 0, nil
	}
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	gb := free / (1024 * 1024 * 1024)
	maxInt := uint64(^uint(0) >> 1)
	if gb > maxInt {
		return int(maxInt), nil
	}
	return int(gb), nil
}

func healthURL(addr string) string {
	host := "127.0.0.1"
	port := "8080"

	a := strings.TrimSpace(addr)
	if a != "" {
		h, p, err := net.SplitHostPort(a)
		if err == nil {
			if p != "" {
				port = p
			}
			if h != "" {
				host = h
			}
		}
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s/health", net.JoinHostPort(host, port))
}

func parsePort(addr, fallback string) string {
	if strings.TrimSpace(addr) == "" {
		return fallback
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(port) == "" {
		return fallback
	}
	return port
}

func renderTemplateWithFallback(path, fallback string, data any) (string, error) {
	content, err := os.ReadFile(path) //nolint:gosec // Installer controls template path.
	if err != nil {
		content = []byte(fallback)
	}
	tpl, err := template.New(filepath.Base(path)).Parse(string(content))
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func randomPassword() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

const defaultPanelVhostTemplate = `server {
    listen 80;
    server_name _;

    access_log /var/log/nginx/aipanel.access.log;
    error_log /var/log/nginx/aipanel.error.log;

    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://127.0.0.1:{{ .PanelPort }};
    }
}
`

const defaultCatchallTemplate = `server {
    listen 80 default_server;
    server_name _;
    return 444;
}
`

// phpPoolTemplate uses two %s verb slots: PHP version for pool name and socket path.
const phpPoolTemplate = `[aipanel-default-%s]
user = www-data
group = www-data
listen = /run/php/aipanel-default-%s.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = ondemand
pm.max_children = 10
pm.process_idle_timeout = 10s
`

func renderPanelConfig(opts Options) string {
	return fmt.Sprintf(
		"addr: %q\nenv: %q\ndata_dir: %q\nsession_cookie_name: \"aipanel_session\"\nsession_ttl_hours: 24\n",
		opts.Addr,
		opts.Env,
		opts.DataDir,
	)
}

func renderSystemdUnit(opts Options) string {
	configPath := opts.ConfigPath
	if strings.TrimSpace(configPath) == "" {
		configPath = "/etc/aipanel/panel.yaml"
	}
	return strings.Join([]string{
		"[Unit]",
		"Description=aiPanel service",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		// Hosting provisioning requires privileged operations:
		// useradd/chown, writes under /etc and /var/www, and service reloads.
		"User=root",
		"Group=root",
		"WorkingDirectory=/",
		fmt.Sprintf("Environment=AIPANEL_CONFIG=%s", configPath),
		fmt.Sprintf("ExecStart=%s serve", opts.PanelBinaryPath),
		"Restart=on-failure",
		"RestartSec=2",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func writeTextFile(path, content string, mode os.FileMode) error {
	return writeBinaryFile(path, []byte(content), mode)
}

func writeBinaryFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// Installer controls output paths and file modes for generated artifacts.
	//nolint:gosec // G304
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func fileSHA256(path string) (string, error) {
	// Installer controls binary paths in runtime options.
	//nolint:gosec // G304
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
