package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	commands []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.commands = append(r.commands, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	return "", nil
}

type fakeRunnerWithErrors struct {
	commands     []string
	failCommands map[string]bool
}

func (r *fakeRunnerWithErrors) Run(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, cmd)
	if r.failCommands[cmd] {
		return "", fmt.Errorf("command failed: %s", cmd)
	}
	return "", nil
}

type fakeRunnerShellBuild struct {
	commands []string
}

func (r *fakeRunnerShellBuild) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, cmd)
	if name == "bash" && len(args) >= 2 && args[0] == "-lc" {
		c := exec.CommandContext(ctx, "bash", "-lc", args[1]) //nolint:gosec // Test helper executes controlled build commands.
		out, err := c.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("build shell failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}
	return "", nil
}

func TestIsDebian13(t *testing.T) {
	if !isDebian13(map[string]string{"ID": "debian", "VERSION_CODENAME": "trixie"}) {
		t.Fatal("expected debian trixie to pass")
	}
	if !isDebian13(map[string]string{"ID": "debian", "VERSION_ID": "13"}) {
		t.Fatal("expected debian version_id=13 to pass")
	}
	if isDebian13(map[string]string{"ID": "ubuntu", "VERSION_ID": "24.04"}) {
		t.Fatal("expected non-debian to fail")
	}
}

func TestInstallerRun_Phase1DrySystem(t *testing.T) {
	root := t.TempDir()
	srcBinary := filepath.Join(root, "src", "aipanel")
	if err := os.MkdirAll(filepath.Dir(srcBinary), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(srcBinary, []byte("binary"), 0o600); err != nil {
		t.Fatalf("write src binary: %v", err)
	}

	osRelease := filepath.Join(root, "etc", "os-release")
	if err := os.MkdirAll(filepath.Dir(osRelease), 0o750); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}
	if err := os.WriteFile(osRelease, []byte("ID=debian\nVERSION_CODENAME=trixie\n"), 0o600); err != nil {
		t.Fatalf("write os-release: %v", err)
	}

	memInfo := filepath.Join(root, "proc", "meminfo")
	if err := os.MkdirAll(filepath.Dir(memInfo), 0o750); err != nil {
		t.Fatalf("mkdir proc: %v", err)
	}
	if err := os.WriteFile(memInfo, []byte("MemTotal:       4194304 kB\n"), 0o600); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	proc1 := filepath.Join(root, "proc", "1-exe")
	if err := os.Symlink("/lib/systemd/systemd", proc1); err != nil {
		t.Fatalf("write proc1 symlink: %v", err)
	}

	opts := DefaultOptions()
	opts.Addr = ":18080"
	opts.ConfigPath = filepath.Join(root, "etc", "aipanel", "panel.yaml")
	opts.DataDir = filepath.Join(root, "var", "lib", "aipanel")
	opts.PanelBinaryPath = filepath.Join(root, "usr", "local", "bin", "aipanel")
	opts.SourceBinaryPath = srcBinary
	opts.UnitFilePath = filepath.Join(root, "etc", "systemd", "system", "aipanel.service")
	opts.StateFilePath = filepath.Join(root, "var", "lib", "aipanel", ".installer-state.json")
	opts.ReportFilePath = filepath.Join(root, "var", "lib", "aipanel", "install-report.json")
	opts.LogFilePath = filepath.Join(root, "var", "log", "aipanel", "install.log")
	opts.OSReleasePath = osRelease
	opts.MemInfoPath = memInfo
	opts.Proc1ExePath = proc1
	opts.RootFSPath = root
	opts.NginxSitesAvailableDir = filepath.Join(root, "etc", "nginx", "sites-available")
	opts.NginxSitesEnabledDir = filepath.Join(root, "etc", "nginx", "sites-enabled")
	opts.PHPBaseDir = filepath.Join(root, "etc", "php")
	opts.PanelVhostTemplatePath = filepath.Join(root, "configs", "templates", "nginx_panel_vhost.conf.tmpl")
	opts.CatchAllTemplatePath = filepath.Join(root, "configs", "templates", "nginx_catchall.conf.tmpl")
	opts.AdminEmail = "admin@example.com"
	opts.AdminPassword = "supersecret123"
	opts.SkipHealthcheck = true
	opts.MinCPU = 1
	opts.InstallMode = InstallModeSourceBuild

	lockPath := filepath.Join(root, "configs", "sources", "lock.json")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockBody := `{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "https://nginx.org/download/nginx-1.27.4.tar.gz",
        "source_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62"
      }
    }
  }
}`
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write runtime lock: %v", err)
	}
	opts.RuntimeLockPath = lockPath
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")

	if err := os.MkdirAll(filepath.Dir(opts.StateFilePath), 0o750); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	stateBody := `{"completed":{"install_runtime":true,"activate_runtime_services":true}}`
	if err := os.WriteFile(opts.StateFilePath, []byte(stateBody), 0o600); err != nil {
		t.Fatalf("write installer state: %v", err)
	}

	runner := &fakeRunner{}
	ins := New(opts, runner)
	report, err := ins.Run(context.Background())
	if err != nil {
		t.Fatalf("installer run failed: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("expected report status ok, got %q", report.Status)
	}

	if _, err := os.Stat(opts.ConfigPath); err != nil {
		t.Fatalf("missing config file: %v", err)
	}
	if _, err := os.Stat(opts.UnitFilePath); err != nil {
		t.Fatalf("missing unit file: %v", err)
	}
	if _, err := os.Stat(opts.StateFilePath); err != nil {
		t.Fatalf("missing state file: %v", err)
	}
	if _, err := os.Stat(opts.ReportFilePath); err != nil {
		t.Fatalf("missing report file: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "id aipanel") {
		t.Fatalf("expected id aipanel command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "apt-get update") {
		t.Fatalf("expected apt-get update command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends build-essential") {
		t.Fatalf("expected apt-get install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "systemctl enable --now aipanel") {
		t.Fatalf("expected systemd enable command for aipanel, got:\n%s", joined)
	}
}

func TestHealthURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":8080", "http://127.0.0.1:8080/health"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080/health"},
		{"192.168.1.1:9090", "http://192.168.1.1:9090/health"},
		{"[::]:8080", "http://127.0.0.1:8080/health"},
		{"[::1]:8080", "http://[::1]:8080/health"},
		{"", "http://127.0.0.1:8080/health"},
	}
	for _, tt := range tests {
		got := healthURL(tt.addr)
		if got != tt.want {
			t.Errorf("healthURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestCreateServiceUser_NewUser(t *testing.T) {
	runner := &fakeRunnerWithErrors{
		failCommands: map[string]bool{"id aipanel": true},
	}
	ins := &Installer{
		opts:   DefaultOptions(),
		runner: runner,
		now:    time.Now,
	}
	if err := ins.createServiceUser(context.Background()); err != nil {
		t.Fatalf("createServiceUser failed: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "useradd") {
		t.Fatalf("expected useradd command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "chown") {
		t.Fatalf("expected chown command, got:\n%s", joined)
	}
}

func TestEnsureRuntimeNginxConfig_SetsTempDirPermissions(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{}
	opts := DefaultOptions()
	opts.RootFSPath = root
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")

	ins := &Installer{
		opts:   opts,
		runner: runner,
		now:    time.Now,
	}
	if err := ins.ensureRuntimeNginxConfig(context.Background()); err != nil {
		t.Fatalf("ensureRuntimeNginxConfig failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "id -u www-data") {
		t.Fatalf("expected www-data lookup command, got:\n%s", joined)
	}
	expectedProxyDir := filepath.Join(root, "var", "lib", "nginx", "proxy")
	if !strings.Contains(joined, "chown -R www-data:www-data") || !strings.Contains(joined, expectedProxyDir) {
		t.Fatalf("expected chown command for nginx temp dirs, got:\n%s", joined)
	}
}

func TestInstallerRun_SourceBuildCompilesRuntime(t *testing.T) {
	root := t.TempDir()
	srcBinary := filepath.Join(root, "src", "aipanel")
	if err := os.MkdirAll(filepath.Dir(srcBinary), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(srcBinary, []byte("binary"), 0o600); err != nil {
		t.Fatalf("write src binary: %v", err)
	}

	osRelease := filepath.Join(root, "etc", "os-release")
	if err := os.MkdirAll(filepath.Dir(osRelease), 0o750); err != nil {
		t.Fatalf("mkdir etc: %v", err)
	}
	if err := os.WriteFile(osRelease, []byte("ID=debian\nVERSION_CODENAME=trixie\n"), 0o600); err != nil {
		t.Fatalf("write os-release: %v", err)
	}

	memInfo := filepath.Join(root, "proc", "meminfo")
	if err := os.MkdirAll(filepath.Dir(memInfo), 0o750); err != nil {
		t.Fatalf("mkdir proc: %v", err)
	}
	if err := os.WriteFile(memInfo, []byte("MemTotal:       4194304 kB\n"), 0o600); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	proc1 := filepath.Join(root, "proc", "1-exe")
	if err := os.Symlink("/lib/systemd/systemd", proc1); err != nil {
		t.Fatalf("write proc1 symlink: %v", err)
	}

	sourceTar := filepath.Join(root, "runtime", "nginx-source.tar.gz")
	if err := os.MkdirAll(filepath.Dir(sourceTar), 0o750); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := writeTarGzArtifact(sourceTar, "nginx-src/bin/nginx", []byte("compiled-nginx")); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	sourceSum, err := fileSHA256(sourceTar)
	if err != nil {
		t.Fatalf("source sha: %v", err)
	}

	lockPath := filepath.Join(root, "configs", "sources", "lock-build.json")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockBody := fmt.Sprintf(`{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.27.4",
        "source_url": "file://%s",
        "source_sha256": "%s",
        "signature_url": "https://nginx.org/download/nginx-1.27.4.tar.gz.asc",
        "public_key_fingerprint": "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62",
        "build": {
          "commands": [
            "mkdir -p {{install_dir}}/bin",
            "cp ./bin/nginx {{install_dir}}/bin/nginx"
          ]
        },
        "systemd": {
          "name": "aipanel-runtime-nginx.service",
          "exec_start": "{{runtime_dir}}/nginx/current/sbin/nginx -g \"daemon off;\""
        }
      }
    }
  }
}`, sourceTar, sourceSum)
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write runtime lock: %v", err)
	}

	opts := DefaultOptions()
	opts.Addr = ":18080"
	opts.ConfigPath = filepath.Join(root, "etc", "aipanel", "panel.yaml")
	opts.DataDir = filepath.Join(root, "var", "lib", "aipanel")
	opts.PanelBinaryPath = filepath.Join(root, "usr", "local", "bin", "aipanel")
	opts.SourceBinaryPath = srcBinary
	opts.UnitFilePath = filepath.Join(root, "etc", "systemd", "system", "aipanel.service")
	opts.StateFilePath = filepath.Join(root, "var", "lib", "aipanel", ".installer-state.json")
	opts.ReportFilePath = filepath.Join(root, "var", "lib", "aipanel", "install-report.json")
	opts.LogFilePath = filepath.Join(root, "var", "log", "aipanel", "install.log")
	opts.OSReleasePath = osRelease
	opts.MemInfoPath = memInfo
	opts.Proc1ExePath = proc1
	opts.RootFSPath = root
	opts.NginxSitesAvailableDir = filepath.Join(root, "etc", "nginx", "sites-available")
	opts.NginxSitesEnabledDir = filepath.Join(root, "etc", "nginx", "sites-enabled")
	opts.PHPBaseDir = filepath.Join(root, "etc", "php")
	opts.PanelVhostTemplatePath = filepath.Join(root, "configs", "templates", "nginx_panel_vhost.conf.tmpl")
	opts.CatchAllTemplatePath = filepath.Join(root, "configs", "templates", "nginx_catchall.conf.tmpl")
	opts.AdminEmail = "admin@example.com"
	opts.AdminPassword = "supersecret123"
	opts.SkipHealthcheck = true
	opts.MinCPU = 1
	opts.InstallMode = InstallModeSourceBuild
	opts.RuntimeChannel = RuntimeChannelStable
	opts.RuntimeLockPath = lockPath
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")
	opts.VerifyUpstreamSources = false

	runner := &fakeRunnerShellBuild{}
	ins := New(opts, runner)
	report, err := ins.Run(context.Background())
	if err != nil {
		t.Fatalf("installer run failed: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("expected report status ok, got %q", report.Status)
	}

	installedPath := filepath.Join(opts.RuntimeInstallDir, "nginx", "1.27.4", "bin", "nginx")
	body, err := os.ReadFile(installedPath) //nolint:gosec // test reads file generated in temp dir.
	if err != nil {
		t.Fatalf("read installed runtime payload: %v", err)
	}
	if string(body) != "compiled-nginx" {
		t.Fatalf("unexpected installed payload: %q", string(body))
	}

	currentLink := filepath.Join(opts.RuntimeInstallDir, "nginx", "current")
	target, err := os.Readlink(currentLink)
	if err != nil {
		t.Fatalf("read current symlink: %v", err)
	}
	if target != filepath.Join(opts.RuntimeInstallDir, "nginx", "1.27.4") {
		t.Fatalf("unexpected current symlink target: %s", target)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends build-essential") {
		t.Fatalf("expected apt-get install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "cp ./bin/nginx") {
		t.Fatalf("expected build copy command, got:\n%s", joined)
	}
}

func writeTarGzArtifact(path string, name string, content []byte) error {
	f, err := os.Create(path) //nolint:gosec // Test helper writes fixture file under t.TempDir.
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
