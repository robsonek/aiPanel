// Package installer provides the one-shot Debian 13 installer orchestrator.
package installer

import (
	"bufio"
	"context"
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
	"time"

	"github.com/robsonek/aiPanel/internal/installer/steps"
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

	OSReleasePath string
	MemInfoPath   string
	Proc1ExePath  string
	RootFSPath    string

	MinCPU      int
	MinMemoryMB int
	MinDiskGB   int

	SkipHealthcheck bool
}

// DefaultOptions returns production defaults for installer phase 1.
func DefaultOptions() Options {
	return Options{
		Addr:             ":8080",
		Env:              "prod",
		ConfigPath:       "/etc/aipanel/panel.yaml",
		DataDir:          "/var/lib/aipanel",
		PanelBinaryPath:  "/usr/local/bin/aipanel",
		UnitFilePath:     "/etc/systemd/system/aipanel.service",
		StateFilePath:    "/var/lib/aipanel/.installer-state.json",
		ReportFilePath:   "/var/lib/aipanel/install-report.json",
		LogFilePath:      "/var/log/aipanel/install.log",
		OSReleasePath:    "/etc/os-release",
		MemInfoPath:      "/proc/meminfo",
		Proc1ExePath:     "/proc/1/exe",
		RootFSPath:       "/",
		MinCPU:           2,
		MinMemoryMB:      1024,
		MinDiskGB:        10,
		SkipHealthcheck:  false,
		SourceBinaryPath: "",
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
		runErr = execStep(steps.WriteUnit, i.writeUnitFile)
	}
	if runErr == nil {
		runErr = execStep(steps.StartPanel, i.startPanelService)
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
	if _, err := i.runner.Run(ctx, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}
	if _, err := i.runner.Run(ctx, "apt-get", "install", "-y", "nginx"); err != nil {
		return fmt.Errorf("apt install nginx: %w", err)
	}
	if err := systemd.EnableNow(ctx, i.runner, "nginx"); err != nil {
		return fmt.Errorf("start nginx: %w", err)
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
		"User=aipanel",
		"Group=aipanel",
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
