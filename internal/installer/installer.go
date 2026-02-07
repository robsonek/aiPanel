// Package installer provides the one-shot Debian 13 installer orchestrator.
package installer

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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
	Addr                  string
	Env                   string
	ConfigPath            string
	DataDir               string
	PanelBinaryPath       string
	SourceBinaryPath      string
	UnitFilePath          string
	StateFilePath         string
	ReportFilePath        string
	LogFilePath           string
	AdminEmail            string
	AdminPassword         string
	InstallMode           string
	RuntimeChannel        string
	RuntimeLockPath       string
	RuntimeManifestURL    string
	RuntimeInstallDir     string
	VerifyUpstreamSources bool

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

const (
	// InstallModeSourceBuild compiles runtime directly from upstream sources.
	InstallModeSourceBuild = "source-build"
)

const (
	// RuntimeChannelStable is the default pinned release channel.
	RuntimeChannelStable = "stable"
	// RuntimeChannelEdge tracks the newest validated runtime source pins.
	RuntimeChannelEdge = "edge"
)

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
		InstallMode:            InstallModeSourceBuild,
		RuntimeChannel:         RuntimeChannelStable,
		RuntimeLockPath:        "/etc/aipanel/sources.lock.json",
		RuntimeManifestURL:     "",
		RuntimeInstallDir:      "/opt/aipanel/runtime",
		VerifyUpstreamSources:  true,
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
	if strings.TrimSpace(o.InstallMode) == "" {
		o.InstallMode = d.InstallMode
	}
	if strings.TrimSpace(o.RuntimeChannel) == "" {
		o.RuntimeChannel = d.RuntimeChannel
	}
	if strings.TrimSpace(o.RuntimeLockPath) == "" {
		o.RuntimeLockPath = d.RuntimeLockPath
	}
	if strings.TrimSpace(o.RuntimeManifestURL) == "" {
		o.RuntimeManifestURL = d.RuntimeManifestURL
	}
	if strings.TrimSpace(o.RuntimeInstallDir) == "" {
		o.RuntimeInstallDir = d.RuntimeInstallDir
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

func (o Options) validate() error {
	mode := strings.ToLower(strings.TrimSpace(o.InstallMode))
	switch mode {
	case InstallModeSourceBuild:
	default:
		return fmt.Errorf("invalid install mode: %s", o.InstallMode)
	}

	channel := strings.ToLower(strings.TrimSpace(o.RuntimeChannel))
	switch channel {
	case RuntimeChannelStable, RuntimeChannelEdge:
	default:
		return fmt.Errorf("invalid runtime channel: %s", o.RuntimeChannel)
	}

	if isRuntimeSourceMode(mode) &&
		strings.TrimSpace(o.RuntimeLockPath) == "" &&
		strings.TrimSpace(o.RuntimeManifestURL) == "" {
		return fmt.Errorf("%s mode requires runtime lock path or runtime manifest URL", mode)
	}
	if isRuntimeSourceMode(mode) && strings.TrimSpace(o.RuntimeInstallDir) == "" {
		return fmt.Errorf("%s mode requires runtime install dir", mode)
	}
	return nil
}

func isRuntimeSourceMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), InstallModeSourceBuild)
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

type commandLoggingRunner struct {
	delegate systemd.Runner
	logf     func(string, ...any)
}

func (r commandLoggingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	command := strings.TrimSpace(name + " " + strings.Join(args, " "))
	startedAt := time.Now()
	if r.logf != nil {
		r.logf("[command] start: %s", command)
	}
	out, err := r.delegate.Run(ctx, name, args...)
	duration := time.Since(startedAt).Round(time.Millisecond)
	trimmedOut := strings.TrimSpace(out)
	if err != nil {
		if r.logf != nil {
			r.logf("[command] failed after %s: %s", duration, command)
			if trimmedOut != "" {
				r.logf("[command] output:\n%s", trimmedOut)
			}
			r.logf("[command] error: %v", err)
		}
		return out, fmt.Errorf("command %q failed after %s: %w", command, duration, err)
	}
	if r.logf != nil {
		r.logf("[command] ok after %s: %s", duration, command)
		if trimmedOut != "" {
			r.logf("[command] output:\n%s", trimmedOut)
		}
	}
	return out, nil
}

// Installer orchestrates phase 1 setup on Debian 13.
type Installer struct {
	opts        Options
	runner      systemd.Runner
	now         func() time.Time
	runtimeLock *RuntimeSourceLock
}

// New returns a configured installer.
func New(opts Options, runner systemd.Runner) *Installer {
	opts = opts.withDefaults()
	if runner == nil {
		runner = systemd.ExecRunner{}
	}
	ins := &Installer{
		opts: opts,
		now:  time.Now,
	}
	ins.runner = commandLoggingRunner{
		delegate: runner,
		logf:     ins.logf,
	}
	return ins
}

// Run executes installer phase 1 with checkpoint-based idempotency.
func (i *Installer) Run(ctx context.Context) (*Report, error) {
	if err := i.opts.validate(); err != nil {
		return nil, err
	}
	if isRuntimeSourceMode(i.opts.InstallMode) {
		if _, err := i.resolveRuntimeSourceLock(ctx); err != nil {
			return nil, fmt.Errorf("load runtime source lock: %w", err)
		}
	}
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
		runErr = execStep(steps.InstallRuntime, i.installRuntimeArtifacts)
	}
	if runErr == nil {
		runErr = execStep(steps.ActivateRuntime, i.activateRuntimeServices)
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
	return nil
}

func (i *Installer) addRepositories(ctx context.Context) error {
	i.logf("[add_repositories] skipped in source-build mode")
	return nil
}

func (i *Installer) installPackages(ctx context.Context) error {
	packages := []string{
		"build-essential",
		"ca-certificates",
		"cmake",
		"gnupg",
		"libncurses-dev",
		"libpcre2-dev",
		"libreadline-dev",
		"libsqlite3-dev",
		"libssl-dev",
		"libxml2-dev",
		"pkg-config",
		"sqlite3",
		"zlib1g-dev",
	}
	i.logf("[install_packages] apt prerequisites: %s", strings.Join(packages, ", "))
	installArgs := append([]string{"install", "-y", "--no-install-recommends"}, packages...)
	if _, err := i.runner.Run(ctx, "apt-get", installArgs...); err != nil {
		return fmt.Errorf("apt install installer prerequisites: %w", err)
	}
	return nil
}

func (i *Installer) installRuntimeArtifacts(ctx context.Context) error {
	if !isRuntimeSourceMode(i.opts.InstallMode) {
		return nil
	}
	return i.installRuntimeFromSources(ctx)
}

func (i *Installer) installRuntimeFromSources(ctx context.Context) error {
	lock, err := i.resolveRuntimeSourceLock(ctx)
	if err != nil {
		return err
	}
	channel, err := i.runtimeChannel(lock)
	if err != nil {
		return err
	}

	componentNames := make([]string, 0, len(channel))
	for name := range channel {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)

	for _, componentName := range componentNames {
		component := channel[componentName]
		if err := i.installRuntimeComponentFromSource(ctx, componentName, component); err != nil {
			return err
		}
	}
	return nil
}

func (i *Installer) installRuntimeComponentFromSource(
	ctx context.Context,
	componentName string,
	component RuntimeComponentLock,
) error {
	componentName = strings.TrimSpace(componentName)
	if componentName == "" {
		return fmt.Errorf("runtime component name is empty")
	}
	if len(component.Build.Commands) == 0 {
		return fmt.Errorf("runtime build commands are missing for %s", componentName)
	}
	i.logf(
		"[install_runtime] component=%s version=%s source=%s",
		componentName,
		component.Version,
		component.SourceURL,
	)

	versionDir := filepath.Join(i.opts.RuntimeInstallDir, componentName, component.Version)
	currentLink := filepath.Join(i.opts.RuntimeInstallDir, componentName, "current")
	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("reset runtime component dir %s: %w", componentName, err)
	}
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		return fmt.Errorf("create runtime component dir %s: %w", componentName, err)
	}

	sourceArchivePath, err := i.downloadRuntimeArtifact(ctx, component.SourceURL)
	if err != nil {
		return fmt.Errorf("download runtime source %s: %w", componentName, err)
	}
	defer func() {
		_ = os.Remove(sourceArchivePath)
	}()

	sourceHash, err := fileSHA256(sourceArchivePath)
	if err != nil {
		return fmt.Errorf("checksum runtime source %s: %w", componentName, err)
	}
	if !strings.EqualFold(sourceHash, component.SourceSHA256) {
		return fmt.Errorf(
			"runtime source checksum mismatch for %s: expected %s got %s",
			componentName,
			component.SourceSHA256,
			sourceHash,
		)
	}
	i.logf("[install_runtime] checksum verified for %s: %s", componentName, sourceHash)

	if i.opts.VerifyUpstreamSources {
		if err := i.verifyRuntimeSourceSignature(ctx, componentName, component, sourceArchivePath); err != nil {
			return err
		}
	}

	buildRoot, err := os.MkdirTemp("", "aipanel-source-build-"+componentName+"-*")
	if err != nil {
		return fmt.Errorf("create build dir for %s: %w", componentName, err)
	}
	defer func() {
		_ = os.RemoveAll(buildRoot)
	}()

	if err := extractArchive(sourceArchivePath, buildRoot); err != nil {
		return fmt.Errorf("extract runtime source %s: %w", componentName, err)
	}

	sourceDir, err := detectSourceDir(buildRoot)
	if err != nil {
		return fmt.Errorf("resolve source dir for %s: %w", componentName, err)
	}

	for idx, command := range component.Build.Commands {
		rendered := renderRuntimeBuildCommand(i.opts, componentName, component.Version, command)
		i.logf(
			"[install_runtime] %s build command %d/%d: %s",
			componentName,
			idx+1,
			len(component.Build.Commands),
			rendered,
		)
		shellCommand := "cd " + shellQuote(sourceDir) + " && " + rendered
		if _, err := i.runner.Run(ctx, "bash", "-lc", shellCommand); err != nil {
			return fmt.Errorf("build %s command %d failed: %w", componentName, idx+1, err)
		}
	}

	hasFiles, err := directoryHasEntries(versionDir)
	if err != nil {
		return fmt.Errorf("inspect runtime install dir for %s: %w", componentName, err)
	}
	if !hasFiles {
		return fmt.Errorf("runtime build output is empty for %s: %s", componentName, versionDir)
	}

	if err := os.Remove(currentLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove current runtime symlink for %s: %w", componentName, err)
	}
	if err := os.Symlink(versionDir, currentLink); err != nil {
		return fmt.Errorf("create current runtime symlink for %s: %w", componentName, err)
	}
	i.logf("[install_runtime] activated %s current -> %s", componentName, versionDir)
	return nil
}

func (i *Installer) verifyRuntimeSourceSignature(
	ctx context.Context,
	componentName string,
	component RuntimeComponentLock,
	archivePath string,
) error {
	signatureURL := strings.TrimSpace(component.SignatureURL)
	if signatureURL == "" {
		return fmt.Errorf("runtime signature_url is missing for %s", componentName)
	}
	fingerprint := strings.TrimSpace(component.PublicKeyFingerprint)
	if fingerprint == "" {
		return fmt.Errorf("runtime public_key_fingerprint is missing for %s", componentName)
	}

	signatureData, err := i.downloadBytes(ctx, signatureURL)
	if err != nil {
		return fmt.Errorf("download runtime signature %s: %w", componentName, err)
	}
	signaturePath, err := writeTempBytes("aipanel-runtime-signature-*", signatureData)
	if err != nil {
		return fmt.Errorf("write runtime signature %s: %w", componentName, err)
	}
	defer func() {
		_ = os.Remove(signaturePath)
	}()

	gnupgHome, err := os.MkdirTemp("", "aipanel-gpg-*")
	if err != nil {
		return fmt.Errorf("create gpg home for %s: %w", componentName, err)
	}
	defer func() {
		_ = os.RemoveAll(gnupgHome)
	}()

	verifyCmd := strings.Join([]string{
		"export GNUPGHOME=" + shellQuote(gnupgHome),
		"gpg --batch --keyserver hkps://keys.openpgp.org --recv-keys " + shellQuote(fingerprint),
		"gpg --batch --verify " + shellQuote(signaturePath) + " " + shellQuote(archivePath),
	}, " && ")
	if _, err := i.runner.Run(ctx, "bash", "-lc", verifyCmd); err != nil {
		return fmt.Errorf("verify upstream signature for %s: %w", componentName, err)
	}
	return nil
}

func (i *Installer) activateRuntimeServices(ctx context.Context) error {
	if !isRuntimeSourceMode(i.opts.InstallMode) {
		return nil
	}
	lock, err := i.resolveRuntimeSourceLock(ctx)
	if err != nil {
		return err
	}
	channel, err := i.runtimeChannel(lock)
	if err != nil {
		return err
	}
	unitDir := filepath.Dir(i.opts.UnitFilePath)
	if err := os.MkdirAll(unitDir, 0o750); err != nil {
		return fmt.Errorf("create systemd unit dir: %w", err)
	}

	componentNames := make([]string, 0, len(channel))
	for name := range channel {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)

	if err := i.prepareRuntimeCompatibility(ctx, channel, componentNames); err != nil {
		return err
	}

	unitNames := make([]string, 0, len(componentNames))
	for _, componentName := range componentNames {
		component := channel[componentName]
		unitName := strings.TrimSpace(component.Systemd.Name)
		execStart := strings.TrimSpace(component.Systemd.ExecStart)
		if unitName == "" || execStart == "" {
			i.logf("[activate_runtime_services] skipping component %s (no systemd unit spec)", componentName)
			continue
		}
		rendered := renderRuntimeSystemdUnit(i.opts, componentName, component)
		unitPath := filepath.Join(unitDir, unitName)
		if err := writeTextFile(unitPath, rendered, 0o644); err != nil {
			return fmt.Errorf("write runtime unit for %s: %w", componentName, err)
		}
		unitNames = append(unitNames, unitName)
	}
	if err := i.installRuntimeSystemdAliases(channel, unitDir); err != nil {
		return err
	}

	if len(unitNames) == 0 {
		i.logf("[activate_runtime_services] no runtime units declared in lockfile")
		return nil
	}

	if err := systemd.DaemonReload(ctx, i.runner); err != nil {
		return fmt.Errorf("systemd daemon-reload for runtime units: %w", err)
	}
	for _, unitName := range unitNames {
		if err := systemd.EnableNow(ctx, i.runner, unitName); err != nil {
			return fmt.Errorf("enable runtime unit %s: %w", unitName, err)
		}
	}
	return nil
}

var majorMinorVersionPattern = regexp.MustCompile(`^\d+\.\d+`)

func (i *Installer) prepareRuntimeCompatibility(
	ctx context.Context,
	channel RuntimeChannelLock,
	componentNames []string,
) error {
	for _, componentName := range componentNames {
		component := channel[componentName]
		switch componentName {
		case "nginx":
			if err := i.ensureRuntimeNginxConfig(); err != nil {
				return err
			}
			if err := i.ensureSymlink(
				filepath.Join(filepath.Dir(i.opts.PanelBinaryPath), "nginx"),
				filepath.Join(i.opts.RuntimeInstallDir, "nginx", "current", "sbin", "nginx"),
			); err != nil {
				return fmt.Errorf("install nginx command alias: %w", err)
			}
		case "php-fpm":
			version := majorMinorVersion(component.Version)
			if version == "" {
				return fmt.Errorf("invalid php-fpm version in runtime lock: %q", component.Version)
			}
			if err := i.ensureRuntimePHPFPMConfig(version); err != nil {
				return err
			}
		case "mariadb":
			if err := i.ensureRuntimeMariaDBBootstrap(ctx); err != nil {
				return err
			}
			if err := i.ensureSymlink(
				filepath.Join(filepath.Dir(i.opts.PanelBinaryPath), "mariadb"),
				filepath.Join(i.opts.RuntimeInstallDir, "mariadb", "current", "bin", "mariadb"),
			); err != nil {
				return fmt.Errorf("install mariadb command alias: %w", err)
			}
		}
	}
	return nil
}

func (i *Installer) installRuntimeSystemdAliases(channel RuntimeChannelLock, unitDir string) error {
	if component, ok := channel["nginx"]; ok {
		unitName := strings.TrimSpace(component.Systemd.Name)
		if unitName != "" {
			if err := i.ensureSymlink(
				filepath.Join(unitDir, "nginx.service"),
				filepath.Join(unitDir, unitName),
			); err != nil {
				return fmt.Errorf("install nginx systemd alias: %w", err)
			}
		}
	}
	if component, ok := channel["mariadb"]; ok {
		unitName := strings.TrimSpace(component.Systemd.Name)
		if unitName != "" {
			if err := i.ensureSymlink(
				filepath.Join(unitDir, "mariadb.service"),
				filepath.Join(unitDir, unitName),
			); err != nil {
				return fmt.Errorf("install mariadb systemd alias: %w", err)
			}
		}
	}
	if component, ok := channel["php-fpm"]; ok {
		unitName := strings.TrimSpace(component.Systemd.Name)
		version := majorMinorVersion(component.Version)
		if unitName != "" && version != "" {
			alias := "php" + version + "-fpm.service"
			if err := i.ensureSymlink(
				filepath.Join(unitDir, alias),
				filepath.Join(unitDir, unitName),
			); err != nil {
				return fmt.Errorf("install php-fpm systemd alias: %w", err)
			}
		}
	}
	return nil
}

func (i *Installer) ensureSymlink(path, target string) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(target) == "" {
		return fmt.Errorf("path and target are required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create symlink parent dir: %w", err)
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			i.logf("[activate_runtime_services] skip symlink %s -> %s (path exists and is not symlink)", path, target)
			return nil
		}
		currentTarget, readErr := os.Readlink(path)
		if readErr == nil && currentTarget == target {
			return nil
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("remove old symlink %s: %w", path, removeErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect symlink path %s: %w", path, err)
	}
	if err := os.Symlink(target, path); err != nil {
		return fmt.Errorf("create symlink %s -> %s: %w", path, target, err)
	}
	return nil
}

func majorMinorVersion(version string) string {
	return majorMinorVersionPattern.FindString(strings.TrimSpace(version))
}

func (i *Installer) runtimePHPMajorMinorVersion(ctx context.Context) (string, error) {
	lock, err := i.resolveRuntimeSourceLock(ctx)
	if err != nil {
		return "", err
	}
	channel, err := i.runtimeChannel(lock)
	if err != nil {
		return "", err
	}
	component, ok := channel["php-fpm"]
	if !ok {
		return "", nil
	}
	version := majorMinorVersion(component.Version)
	if version == "" {
		return "", fmt.Errorf("invalid php-fpm version in runtime lock: %q", component.Version)
	}
	return version, nil
}

func (i *Installer) ensureRuntimeNginxConfig() error {
	confDir := filepath.Join(i.opts.RuntimeInstallDir, "nginx", "current", "conf")
	if err := os.MkdirAll(confDir, 0o750); err != nil {
		return fmt.Errorf("create runtime nginx conf dir: %w", err)
	}
	snippetsDir := filepath.Join(confDir, "snippets")
	if err := os.MkdirAll(snippetsDir, 0o750); err != nil {
		return fmt.Errorf("create runtime nginx snippets dir: %w", err)
	}
	if err := writeTextFile(
		filepath.Join(snippetsDir, "fastcgi-php.conf"),
		sourceRuntimeFastCGIPHPConf,
		0o644,
	); err != nil {
		return fmt.Errorf("write runtime nginx fastcgi snippet: %w", err)
	}
	for _, dir := range []string{
		"/var/log/nginx",
		"/var/lib/nginx/body",
		"/var/lib/nginx/proxy",
		"/var/lib/nginx/fastcgi",
		"/var/lib/nginx/uwsgi",
		"/var/lib/nginx/scgi",
	} {
		if err := os.MkdirAll(pathInRootFS(i.opts.RootFSPath, dir), 0o750); err != nil {
			return fmt.Errorf("create runtime nginx dir %s: %w", dir, err)
		}
	}
	confPath := filepath.Join(confDir, "nginx.conf")
	if err := writeTextFile(confPath, sourceRuntimeNginxConf, 0o644); err != nil {
		return fmt.Errorf("write runtime nginx config: %w", err)
	}
	return nil
}

func (i *Installer) ensureRuntimePHPFPMConfig(version string) error {
	runtimeEtcDir := filepath.Join(i.opts.RuntimeInstallDir, "php-fpm", "current", "etc")
	if err := os.MkdirAll(runtimeEtcDir, 0o750); err != nil {
		return fmt.Errorf("create runtime php-fpm etc dir: %w", err)
	}
	confPath := filepath.Join(runtimeEtcDir, "php-fpm.conf")
	defaultConfPath := confPath + ".default"
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		if body, readErr := os.ReadFile(defaultConfPath); readErr == nil { //nolint:gosec // Installer controls runtime path.
			if err := writeBinaryFile(confPath, body, 0o644); err != nil {
				return fmt.Errorf("write runtime php-fpm.conf: %w", err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("inspect runtime php-fpm.conf: %w", err)
	}

	runtimePoolDir := filepath.Join(runtimeEtcDir, "php-fpm.d")
	if err := os.MkdirAll(runtimePoolDir, 0o750); err != nil {
		return fmt.Errorf("create runtime php-fpm pool dir: %w", err)
	}
	defaultPool := filepath.Join(runtimePoolDir, "www.conf")
	defaultPoolTemplate := filepath.Join(runtimePoolDir, "www.conf.default")
	if _, err := os.Stat(defaultPool); os.IsNotExist(err) {
		if body, readErr := os.ReadFile(defaultPoolTemplate); readErr == nil { //nolint:gosec // Installer controls runtime path.
			if err := writeBinaryFile(defaultPool, body, 0o644); err != nil {
				return fmt.Errorf("write runtime php-fpm default pool: %w", err)
			}
		}
	}

	compatPoolDir := filepath.Join(i.opts.PHPBaseDir, version, "fpm", "pool.d")
	if err := ensureParentDir(compatPoolDir); err != nil {
		return fmt.Errorf("create php compatibility dir: %w", err)
	}
	if err := i.ensureSymlink(compatPoolDir, runtimePoolDir); err != nil {
		return fmt.Errorf("link php compatibility pool dir: %w", err)
	}
	return nil
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o750)
}

func pathInRootFS(rootFSPath, absolutePath string) string {
	root := strings.TrimSpace(rootFSPath)
	if root == "" || root == "/" {
		return absolutePath
	}
	return filepath.Join(root, strings.TrimPrefix(absolutePath, "/"))
}

func (i *Installer) ensureRuntimeMariaDBBootstrap(ctx context.Context) error {
	runtimeDir := filepath.Join(i.opts.RuntimeInstallDir, "mariadb", "current")
	mysqlDir := filepath.Join(runtimeDir, "data", "mysql")
	if _, err := os.Stat(mysqlDir); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect runtime mariadb data dir: %w", err)
	}

	bootstrapScript := fmt.Sprintf(`
set -e
runtime_root=%s
data_dir="$runtime_root/data"
mkdir -p "$data_dir"
{
  echo "create database if not exists mysql;"
  echo "use mysql;"
  echo "SET @auth_root_socket=NULL;"
  for file in \
    "$runtime_root/share/mariadb_system_tables.sql" \
    "$runtime_root/share/mariadb_performance_tables.sql" \
    "$runtime_root/share/mariadb_system_tables_data.sql" \
    "$runtime_root/share/fill_help_tables.sql" \
    "$runtime_root/share/maria_add_gis_sp_bootstrap.sql" \
    "$runtime_root/share/mariadb_sys_schema.sql"; do
    if [ -f "$file" ]; then
      cat "$file"
    fi
  done
} | "$runtime_root/bin/mariadbd" --bootstrap --basedir="$runtime_root" --datadir="$data_dir" --log-warnings=0 --enforce-storage-engine="" --plugin-dir="$runtime_root/lib/plugin" --max_allowed_packet=8M --net_buffer_length=16K --user=root
`, shellQuote(runtimeDir))
	if _, err := i.runner.Run(ctx, "bash", "-lc", bootstrapScript); err != nil {
		return fmt.Errorf("bootstrap runtime mariadb data dir: %w", err)
	}
	return nil
}

func (i *Installer) resolveRuntimeSourceLock(ctx context.Context) (*RuntimeSourceLock, error) {
	if i.runtimeLock != nil {
		return i.runtimeLock, nil
	}
	manifestURL := strings.TrimSpace(i.opts.RuntimeManifestURL)
	if manifestURL != "" {
		data, err := i.downloadBytes(ctx, manifestURL)
		if err != nil {
			return nil, fmt.Errorf("download runtime manifest: %w", err)
		}
		var lock RuntimeSourceLock
		if err := json.Unmarshal(data, &lock); err != nil {
			return nil, fmt.Errorf("decode runtime manifest: %w", err)
		}
		if err := lock.Validate(); err != nil {
			return nil, err
		}
		i.runtimeLock = &lock
		return i.runtimeLock, nil
	}

	if p := strings.TrimSpace(i.opts.RuntimeLockPath); p != "" {
		lock, err := LoadRuntimeSourceLock(p)
		if err != nil {
			return nil, err
		}
		i.runtimeLock = lock
		return i.runtimeLock, nil
	}
	return nil, fmt.Errorf("missing runtime lock path and runtime manifest URL")
}

func (i *Installer) runtimeChannel(lock *RuntimeSourceLock) (RuntimeChannelLock, error) {
	channelName := strings.ToLower(strings.TrimSpace(i.opts.RuntimeChannel))
	channel, ok := lock.Channels[channelName]
	if !ok {
		return nil, fmt.Errorf("runtime lock does not contain channel %s", channelName)
	}
	return channel, nil
}

func (i *Installer) downloadRuntimeArtifact(ctx context.Context, artifactURL string) (string, error) {
	data, err := i.downloadBytes(ctx, artifactURL)
	if err != nil {
		return "", err
	}
	suffix := archiveSuffix(artifactURL)
	tmp, err := os.CreateTemp("", "aipanel-runtime-*"+suffix)
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func writeTempBytes(pattern string, b []byte) (string, error) {
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func archiveSuffix(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(ref))
	switch {
	case strings.HasSuffix(ref, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(ref, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(ref, ".tar"):
		return ".tar"
	default:
		return ".tar"
	}
}

func (i *Installer) downloadBytes(ctx context.Context, ref string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty download reference")
	}
	i.logf("[download] start: %s", ref)
	if strings.HasPrefix(strings.ToLower(ref), "http://") || strings.HasPrefix(strings.ToLower(ref), "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<30))
		if err != nil {
			return nil, err
		}
		i.logf("[download] completed: %s (%d bytes)", ref, len(body))
		return body, nil
	}
	if strings.HasPrefix(strings.ToLower(ref), "file://") {
		u, err := url.Parse(ref)
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(u.Path) //nolint:gosec // Installer reads explicit runtime manifest/artifact location.
		if err != nil {
			return nil, err
		}
		i.logf("[download] loaded local file: %s (%d bytes)", u.Path, len(body))
		return body, nil
	}
	body, err := os.ReadFile(ref) //nolint:gosec // Installer reads explicit runtime manifest/artifact location.
	if err != nil {
		return nil, err
	}
	i.logf("[download] loaded local file: %s (%d bytes)", ref, len(body))
	return body, nil
}

func extractArchive(archivePath, destination string) error {
	f, err := os.Open(archivePath) //nolint:gosec // Installer reads generated temporary archive path.
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() {
			_ = gzr.Close()
		}()
		return extractTar(gzr, destination)
	case strings.HasSuffix(archivePath, ".tar"):
		return extractTar(f, destination)
	default:
		return fmt.Errorf("unsupported artifact format for %s", archivePath)
	}
}

func detectSourceDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(root, entries[0].Name()), nil
	}
	return root, nil
}

func directoryHasEntries(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func extractTar(r io.Reader, destination string) error {
	const (
		maxExtractedBytes     int64 = 4 << 30
		maxExtractedFileBytes int64 = 1 << 30
	)

	var extractedBytes int64
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Size < 0 {
			return fmt.Errorf("invalid negative size for archive entry %s", header.Name)
		}
		if header.Size > maxExtractedFileBytes {
			return fmt.Errorf("archive entry too large: %s", header.Name)
		}
		if extractedBytes+header.Size > maxExtractedBytes {
			return fmt.Errorf("archive total extracted size exceeds limit")
		}
		// Archive paths are validated against destination immediately below.
		//nolint:gosec // G305
		targetPath := filepath.Join(destination, header.Name)
		cleanDestination := filepath.Clean(destination) + string(os.PathSeparator)
		cleanTarget := filepath.Clean(targetPath)
		if cleanTarget != filepath.Clean(destination) && !strings.HasPrefix(cleanTarget, cleanDestination) {
			return fmt.Errorf("archive path traversal detected: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			written, err := io.CopyN(out, tr, header.Size)
			if err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
			if written != header.Size {
				return fmt.Errorf("short write for archive entry %s", header.Name)
			}
			extractedBytes += written

			mode := secureArchiveFileMode(header.FileInfo().Mode(), false)
			if err := os.Chmod(cleanTarget, mode); err != nil { //nolint:gosec // G302: mode sanitized to max 0750.
				return err
			}
		default:
			// Skip unsupported entry types for now.
		}
	}
}

func secureArchiveFileMode(raw os.FileMode, isDir bool) os.FileMode {
	perm := raw & 0o777
	perm &= 0o750
	if isDir {
		if perm&0o700 == 0 {
			perm |= 0o700
		}
		return perm
	}
	if perm&0o600 == 0 {
		perm |= 0o600
	}
	return perm
}

func renderRuntimeSystemdUnit(opts Options, componentName string, component RuntimeComponentLock) string {
	unit := component.Systemd
	desc := strings.TrimSpace(unit.Description)
	if desc == "" {
		desc = "aiPanel runtime " + componentName
	}
	serviceType := strings.TrimSpace(unit.Type)
	if serviceType == "" {
		serviceType = "simple"
	}
	workingDir := strings.TrimSpace(unit.WorkingDirectory)
	if workingDir == "" {
		workingDir = "/"
	}
	execStart := renderRuntimePlaceholder(unit.ExecStart, opts, componentName, component.Version)
	execReload := renderRuntimePlaceholder(unit.ExecReload, opts, componentName, component.Version)
	execStop := renderRuntimePlaceholder(unit.ExecStop, opts, componentName, component.Version)

	after := append([]string(nil), unit.After...)
	if len(after) == 0 {
		after = []string{"network-online.target"}
	}
	wants := append([]string(nil), unit.Wants...)
	if len(wants) == 0 {
		wants = []string{"network-online.target"}
	}

	lines := []string{
		"[Unit]",
		"Description=" + desc,
		"After=" + strings.Join(after, " "),
		"Wants=" + strings.Join(wants, " "),
		"",
		"[Service]",
		"Type=" + serviceType,
		"User=root",
		"Group=root",
		"WorkingDirectory=" + workingDir,
		"ExecStart=" + execStart,
		"Restart=on-failure",
		"RestartSec=2",
	}
	if componentName == "php-fpm" {
		lines = append(lines, "RuntimeDirectory=php")
	}
	if strings.TrimSpace(execReload) != "" {
		lines = append(lines, "ExecReload="+execReload)
	}
	if strings.TrimSpace(execStop) != "" {
		lines = append(lines, "ExecStop="+execStop)
	}
	lines = append(lines,
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	)
	return strings.Join(lines, "\n")
}

func renderRuntimePlaceholder(in string, opts Options, component, version string) string {
	installDir := filepath.Join(strings.TrimSpace(opts.RuntimeInstallDir), strings.TrimSpace(component), strings.TrimSpace(version))
	replacer := strings.NewReplacer(
		"{{runtime_dir}}", strings.TrimSpace(opts.RuntimeInstallDir),
		"{{component}}", strings.TrimSpace(component),
		"{{version}}", strings.TrimSpace(version),
		"{{install_dir}}", installDir,
	)
	return replacer.Replace(strings.TrimSpace(in))
}

func renderRuntimeBuildCommand(opts Options, component, version, command string) string {
	return renderRuntimePlaceholder(command, opts, component, version)
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
	if isRuntimeSourceMode(i.opts.InstallMode) {
		dirs[i.opts.RuntimeInstallDir] = struct{}{}
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

func (i *Installer) installNginx(_ context.Context) error {
	i.logf("[install_nginx] skipped in source-build mode")
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
	if err := i.ensureRuntimeNginxConfig(); err != nil {
		return err
	}
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
	version, err := i.runtimePHPMajorMinorVersion(ctx)
	if err != nil {
		return err
	}
	if version == "" {
		i.logf("[configure_phpfpm] runtime php-fpm component not declared in lock")
		return nil
	}
	versions := []string{version}
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
	if err := i.checkRuntimeUnitsHealth(ctx); err != nil {
		return err
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

func (i *Installer) checkRuntimeUnitsHealth(ctx context.Context) error {
	lock, err := i.resolveRuntimeSourceLock(ctx)
	if err != nil {
		return err
	}
	channel, err := i.runtimeChannel(lock)
	if err != nil {
		return err
	}
	unitNames := make([]string, 0, len(channel))
	for _, component := range channel {
		name := strings.TrimSpace(component.Systemd.Name)
		if name != "" {
			unitNames = append(unitNames, name)
		}
	}
	sort.Strings(unitNames)
	if len(unitNames) == 0 {
		return nil
	}
	for _, unit := range unitNames {
		active, err := systemd.IsActive(ctx, i.runner, unit)
		if err != nil {
			return fmt.Errorf("check %s status: %w", unit, err)
		}
		if !active {
			return fmt.Errorf("%s is not active", unit)
		}
	}
	return nil
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
	ts := i.now().UTC().Format(time.RFC3339)
	message := fmt.Sprintf(format, args...)
	lines := strings.Split(strings.TrimSuffix(message, "\n"), "\n")

	var file io.Writer
	if strings.TrimSpace(i.opts.LogFilePath) != "" {
		_ = os.MkdirAll(filepath.Dir(i.opts.LogFilePath), 0o750)
		f, err := os.OpenFile(i.opts.LogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			defer func() {
				_ = f.Close()
			}()
			file = f
		}
	}

	for _, line := range lines {
		entry := fmt.Sprintf("%s %s\n", ts, line)
		_, _ = os.Stderr.WriteString(entry)
		if file != nil {
			_, _ = io.WriteString(file, entry)
		}
	}
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
	bavail, ok := new(big.Int).SetString(fmt.Sprint(stat.Bavail), 10)
	if !ok || bavail.Sign() <= 0 {
		return 0, nil
	}
	bsize, ok := new(big.Int).SetString(fmt.Sprint(stat.Bsize), 10)
	if !ok || bsize.Sign() <= 0 {
		return 0, nil
	}

	freeBytes := new(big.Int).Mul(bavail, bsize)
	bytesPerGB := big.NewInt(1024 * 1024 * 1024)
	gb := new(big.Int).Div(freeBytes, bytesPerGB)

	maxInt := int64(^uint(0) >> 1)
	maxIntBig := big.NewInt(maxInt)
	if gb.Cmp(maxIntBig) > 0 {
		return int(maxInt), nil
	}
	return int(gb.Int64()), nil
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

const sourceRuntimeNginxConf = `worker_processes auto;
pid /run/nginx.pid;
error_log /var/log/nginx/error.log warn;

events {
    worker_connections 1024;
}

http {
    include mime.types;
    default_type application/octet-stream;
    sendfile on;
    keepalive_timeout 65;
    client_body_temp_path /var/lib/nginx/body;
    proxy_temp_path /var/lib/nginx/proxy;
    fastcgi_temp_path /var/lib/nginx/fastcgi;
    uwsgi_temp_path /var/lib/nginx/uwsgi;
    scgi_temp_path /var/lib/nginx/scgi;
    include /etc/nginx/conf.d/*.conf;
    include /etc/nginx/sites-enabled/*.conf;
}
`

const sourceRuntimeFastCGIPHPConf = `fastcgi_split_path_info ^(.+\.php)(/.+)$;
try_files $fastcgi_script_name =404;
set $path_info $fastcgi_path_info;
fastcgi_param PATH_INFO $path_info;
fastcgi_index index.php;
include fastcgi.conf;
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
