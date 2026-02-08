package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/robsonek/aiPanel/internal/installer/steps"
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

type fakeRunnerFailOnce struct {
	commands []string
	failOnce map[string]bool
}

func (r *fakeRunnerFailOnce) Run(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, cmd)
	if r.failOnce[cmd] {
		delete(r.failOnce, cmd)
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

type fakeLiveRunner struct {
	runCalled     bool
	runLiveCalled bool
}

func (r *fakeLiveRunner) Run(_ context.Context, _ string, _ ...string) (string, error) {
	r.runCalled = true
	return "non-stream output", nil
}

func (r *fakeLiveRunner) RunLive(
	_ context.Context,
	_ string,
	_ []string,
	onLine func(string, bool),
) (string, error) {
	r.runLiveCalled = true
	if onLine != nil {
		onLine("line from stdout", false)
		onLine("line from stderr", true)
	}
	return "line from stdout\nline from stderr", nil
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
	opts.PanelVhostTemplatePath = filepath.Join(root, "configs", "templates", "nginx_panel_vhost.conf.tmpl")
	opts.CatchAllTemplatePath = filepath.Join(root, "configs", "templates", "nginx_catchall.conf.tmpl")
	opts.AdminEmail = "admin@example.com"
	opts.AdminPassword = "supersecret123"
	opts.SkipPHPMyAdmin = true
	opts.SkipHealthcheck = true
	opts.MinCPU = 1
	opts.InstallMode = InstallModeSourceBuild
	opts.RuntimeLockURL = ""

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
	opts.RuntimeLockURL = ""
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
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends") || !strings.Contains(joined, "build-essential") {
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

func TestInstallPackages_IncludesCertbotWhenLetsEncryptEnabled(t *testing.T) {
	runner := &fakeRunner{}
	opts := DefaultOptions()
	opts.EnableLetsEncrypt = true

	ins := &Installer{
		opts:   opts,
		runner: runner,
		now:    time.Now,
	}
	if err := ins.installPackages(context.Background()); err != nil {
		t.Fatalf("installPackages failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends") {
		t.Fatalf("expected apt-get install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "certbot") {
		t.Fatalf("expected certbot package in apt install command, got:\n%s", joined)
	}
}

func TestConfigureTLS_IssuesCertificateAndWritesRenewHook(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{}
	opts := DefaultOptions()
	opts.RootFSPath = root
	opts.ReverseProxy = true
	opts.PanelDomain = "panel.example.com"
	opts.EnableLetsEncrypt = true
	opts.LetsEncryptEmail = "ops@example.com"
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")
	opts.NginxSitesAvailableDir = filepath.Join(root, "etc", "nginx", "sites-available")
	opts.NginxSitesEnabledDir = filepath.Join(root, "etc", "nginx", "sites-enabled")
	opts.PanelVhostTemplatePath = filepath.Join(root, "configs", "templates", "nginx_panel_vhost.conf.tmpl")
	opts.CatchAllTemplatePath = filepath.Join(root, "configs", "templates", "nginx_catchall.conf.tmpl")

	ins := &Installer{
		opts:   opts,
		runner: runner,
		now:    time.Now,
	}
	if err := ins.configureTLS(context.Background()); err != nil {
		t.Fatalf("configureTLS failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "certbot certonly --webroot --webroot-path /var/www/letsencrypt --domain panel.example.com --email ops@example.com --agree-tos --non-interactive --keep-until-expiring") {
		t.Fatalf("expected certbot command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "nginx -t") {
		t.Fatalf("expected nginx config test command, got:\n%s", joined)
	}
	hookPath := filepath.Join(root, "etc", "letsencrypt", "renewal-hooks", "deploy", "aipanel-reload-nginx.sh")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected letsencrypt hook file, got %v", err)
	}
}

func TestEnsureCertbotInstalled_InstallsWhenMissing(t *testing.T) {
	runner := &fakeRunnerFailOnce{
		failOnce: map[string]bool{
			"certbot --version": true,
		},
	}
	ins := &Installer{
		opts:   DefaultOptions(),
		runner: runner,
		now:    time.Now,
	}
	if err := ins.ensureCertbotInstalled(context.Background()); err != nil {
		t.Fatalf("ensureCertbotInstalled failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "apt-get update") {
		t.Fatalf("expected apt-get update when certbot missing, got:\n%s", joined)
	}
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends certbot") {
		t.Fatalf("expected certbot install command, got:\n%s", joined)
	}
}

func TestCommandLoggingRunner_UsesLiveStreaming(t *testing.T) {
	streamRunner := &fakeLiveRunner{}
	var logs []string
	runner := commandLoggingRunner{
		delegate: streamRunner,
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}

	out, err := runner.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("command run failed: %v", err)
	}
	if out == "" {
		t.Fatal("expected output from live runner")
	}
	if !streamRunner.runLiveCalled {
		t.Fatal("expected RunLive to be used")
	}
	if streamRunner.runCalled {
		t.Fatal("did not expect fallback Run call when live runner is available")
	}

	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "[command][stdout] line from stdout") {
		t.Fatalf("expected streamed stdout log, got:\n%s", joined)
	}
	if !strings.Contains(joined, "[command][stderr] line from stderr") {
		t.Fatalf("expected streamed stderr log, got:\n%s", joined)
	}
	if strings.Contains(joined, "[command] output:") {
		t.Fatalf("did not expect buffered output log when live streaming is used, got:\n%s", joined)
	}
}

func TestVerifyRuntimeSourceSignature_UsesKeyserverFallback(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "runtime.tar.gz")
	signaturePath := filepath.Join(root, "runtime.tar.gz.asc")
	if err := os.WriteFile(archivePath, []byte("runtime"), 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}
	if err := os.WriteFile(signaturePath, []byte("signature"), 0o600); err != nil {
		t.Fatalf("write signature fixture: %v", err)
	}

	runner := &fakeRunner{}
	ins := &Installer{
		opts:   DefaultOptions(),
		runner: runner,
		now:    time.Now,
	}
	component := RuntimeComponentLock{
		SignatureURL:         "file://" + signaturePath,
		PublicKeyFingerprint: "177F4010FE56CA3336300305F1656F24C74CD1D8",
	}
	if err := ins.verifyRuntimeSourceSignature(context.Background(), "mariadb", component, archivePath); err != nil {
		t.Fatalf("verifyRuntimeSourceSignature failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "keys.openpgp.org --recv-keys") {
		t.Fatalf("expected keys.openpgp.org import command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "keyserver.ubuntu.com --recv-keys") {
		t.Fatalf("expected keyserver.ubuntu.com fallback import command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "gpg --batch --verify") {
		t.Fatalf("expected gpg verify command, got:\n%s", joined)
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
	opts.PanelVhostTemplatePath = filepath.Join(root, "configs", "templates", "nginx_panel_vhost.conf.tmpl")
	opts.CatchAllTemplatePath = filepath.Join(root, "configs", "templates", "nginx_catchall.conf.tmpl")
	opts.AdminEmail = "admin@example.com"
	opts.AdminPassword = "supersecret123"
	opts.SkipPHPMyAdmin = true
	opts.SkipHealthcheck = true
	opts.MinCPU = 1
	opts.InstallMode = InstallModeSourceBuild
	opts.RuntimeChannel = RuntimeChannelStable
	opts.RuntimeLockPath = lockPath
	opts.RuntimeLockURL = ""
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
	if !strings.Contains(joined, "apt-get install -y --no-install-recommends") || !strings.Contains(joined, "build-essential") {
		t.Fatalf("expected apt-get install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "cp ./bin/nginx") {
		t.Fatalf("expected build copy command, got:\n%s", joined)
	}
}

func TestInstallerRun_OnlyRuntimeComponentsInstallsSelectedComponent(t *testing.T) {
	root := t.TempDir()

	nginxTar := filepath.Join(root, "runtime", "nginx-source.tar.gz")
	if err := os.MkdirAll(filepath.Dir(nginxTar), 0o750); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := writeTarGzArtifact(nginxTar, "nginx-src/bin/nginx", []byte("compiled-nginx")); err != nil {
		t.Fatalf("write nginx source artifact: %v", err)
	}
	nginxSum, err := fileSHA256(nginxTar)
	if err != nil {
		t.Fatalf("nginx source sha: %v", err)
	}

	pgTar := filepath.Join(root, "runtime", "postgres-source.tar.gz")
	if err := writeTarGzArtifactEntries(pgTar, map[string][]byte{
		"postgres-src/bin/psql":   []byte("compiled-psql"),
		"postgres-src/bin/initdb": []byte("compiled-initdb"),
	}); err != nil {
		t.Fatalf("write postgres source artifact: %v", err)
	}
	pgSum, err := fileSHA256(pgTar)
	if err != nil {
		t.Fatalf("postgres source sha: %v", err)
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
        "version": "1.29.5",
        "source_url": "file://%s",
        "source_sha256": "%s",
        "signature_url": "https://nginx.org/download/nginx-1.29.5.tar.gz.asc",
        "public_key_fingerprint": "43387825DDB1BB97EC36BA5D007C8D7C15D87369",
        "build": {
          "commands": [
            "mkdir -p {{install_dir}}/bin",
            "cp ./bin/nginx {{install_dir}}/bin/nginx"
          ]
        },
        "systemd": {
          "name": "aipanel-runtime-nginx.service",
          "exec_start": "{{runtime_dir}}/nginx/current/bin/nginx"
        }
      },
      "postgresql": {
        "version": "18.1",
        "source_url": "file://%s",
        "source_sha256": "%s",
        "signature_url": "",
        "public_key_fingerprint": "",
        "build": {
          "commands": [
            "mkdir -p {{install_dir}}/bin",
            "cp ./bin/psql {{install_dir}}/bin/psql",
            "cp ./bin/initdb {{install_dir}}/bin/initdb"
          ]
        },
        "systemd": {
          "name": "aipanel-runtime-postgresql.service",
          "user": "postgres",
          "group": "postgres",
          "exec_start": "{{install_dir}}/bin/postgres -D {{install_dir}}/data"
        }
      }
    }
  }
}`, nginxTar, nginxSum, pgTar, pgSum)
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write runtime lock: %v", err)
	}

	opts := DefaultOptions()
	opts.OnlyStep = "postgresql"
	opts.RootFSPath = root
	opts.InstallMode = InstallModeSourceBuild
	opts.RuntimeChannel = RuntimeChannelStable
	opts.RuntimeLockPath = lockPath
	opts.RuntimeLockURL = ""
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")
	opts.PanelBinaryPath = filepath.Join(root, "usr", "local", "bin", "aipanel")
	opts.UnitFilePath = filepath.Join(root, "etc", "systemd", "system", "aipanel.service")
	opts.StateFilePath = filepath.Join(root, "var", "lib", "aipanel", ".installer-state.json")
	opts.ReportFilePath = filepath.Join(root, "var", "lib", "aipanel", "install-report.json")
	opts.LogFilePath = filepath.Join(root, "var", "log", "aipanel", "install.log")
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
	if len(report.Steps) != 3 {
		t.Fatalf("expected three runtime component steps, got %d", len(report.Steps))
	}
	if report.Steps[0].Name != steps.InstallPkgs+"[postgresql]" {
		t.Fatalf("expected first alias step %s, got %s", steps.InstallPkgs+"[postgresql]", report.Steps[0].Name)
	}

	postgresPath := filepath.Join(opts.RuntimeInstallDir, "postgresql", "18.1", "bin", "psql")
	if _, err := os.Stat(postgresPath); err != nil {
		t.Fatalf("expected postgres runtime binary at %s: %v", postgresPath, err)
	}
	nginxPath := filepath.Join(opts.RuntimeInstallDir, "nginx", "1.29.5", "bin", "nginx")
	if _, err := os.Stat(nginxPath); !os.IsNotExist(err) {
		t.Fatalf("expected nginx runtime to be untouched, got err=%v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "systemctl enable --now aipanel-runtime-postgresql.service") {
		t.Fatalf("expected enable postgresql runtime unit, got:\n%s", joined)
	}
	if !strings.Contains(joined, "systemctl restart aipanel-runtime-postgresql.service") {
		t.Fatalf("expected restart postgresql runtime unit, got:\n%s", joined)
	}
	if strings.Contains(joined, "aipanel-runtime-nginx.service") {
		t.Fatalf("did not expect nginx runtime activation in postgresql-only mode, got:\n%s", joined)
	}
}

func TestInstallerRun_OnlyInstallPHPMyAdmin(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "phpmyadmin.tar.gz")
	if err := writeTarGzArtifact(
		archivePath,
		"phpMyAdmin-5.2.3-all-languages/index.php",
		[]byte("<?php echo 'ok';"),
	); err != nil {
		t.Fatalf("write phpmyadmin archive: %v", err)
	}
	sum, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatalf("checksum phpmyadmin archive: %v", err)
	}
	checksumPath := filepath.Join(root, "phpmyadmin.tar.gz.sha256")
	if err := os.WriteFile(
		checksumPath,
		[]byte(sum+"  phpMyAdmin-5.2.3-all-languages.tar.gz\n"),
		0o600,
	); err != nil {
		t.Fatalf("write phpmyadmin checksum file: %v", err)
	}

	opts := DefaultOptions()
	opts.OnlyStep = steps.InstallPHPMyAdmin
	opts.RootFSPath = root
	opts.StateFilePath = filepath.Join(root, "var", "lib", "aipanel", ".installer-state.json")
	opts.ReportFilePath = filepath.Join(root, "var", "lib", "aipanel", "install-report.json")
	opts.LogFilePath = filepath.Join(root, "var", "log", "aipanel", "install.log")
	opts.PHPMyAdminURL = "file://" + archivePath
	opts.PHPMyAdminSHA256URL = "file://" + checksumPath
	opts.PHPMyAdminInstallDir = "/usr/share/phpmyadmin"
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")
	opts.NginxSitesAvailableDir = filepath.Join(root, "etc", "nginx", "sites-available")
	opts.NginxSitesEnabledDir = filepath.Join(root, "etc", "nginx", "sites-enabled")

	runner := &fakeRunner{}
	ins := New(opts, runner)
	report, err := ins.Run(context.Background())
	if err != nil {
		t.Fatalf("installer run failed: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("expected report status ok, got %q", report.Status)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("expected exactly one step in report, got %d", len(report.Steps))
	}
	if report.Steps[0].Name != steps.InstallPHPMyAdmin {
		t.Fatalf("expected only %s step, got %s", steps.InstallPHPMyAdmin, report.Steps[0].Name)
	}

	installedIndex := filepath.Join(root, "usr", "share", "phpmyadmin", "index.php")
	body, err := os.ReadFile(installedIndex) //nolint:gosec // test reads fixture under temp dir.
	if err != nil {
		t.Fatalf("read installed phpmyadmin index: %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("unexpected phpmyadmin index content: %q", string(body))
	}

	joined := strings.Join(runner.commands, "\n")
	if strings.Contains(joined, "apt-get update") {
		t.Fatalf("did not expect full install step in only mode, got:\n%s", joined)
	}
	if !strings.Contains(joined, "chown -R root:www-data") {
		t.Fatalf("expected phpmyadmin permissions command, got:\n%s", joined)
	}
}

func TestInstallerRun_OnlyInstallPHPMyAdminRequiresRoot(t *testing.T) {
	opts := DefaultOptions()
	opts.OnlyStep = steps.InstallPHPMyAdmin

	ins := New(opts, &fakeRunner{})
	ins.geteuid = func() int { return 1000 }

	_, err := ins.Run(context.Background())
	if err == nil {
		t.Fatal("expected root privileges error")
	}
	if !strings.Contains(err.Error(), "sudo aipanel install --only install_phpmyadmin") {
		t.Fatalf("expected sudo hint in error, got %v", err)
	}
}

func TestInstallerRun_OnlyInstallPGAdmin(t *testing.T) {
	root := t.TempDir()
	wheelPath := filepath.Join(root, "pgadmin.whl")
	if err := os.WriteFile(wheelPath, []byte("dummy-wheel"), 0o600); err != nil {
		t.Fatalf("write pgadmin wheel: %v", err)
	}
	sum, err := fileSHA256(wheelPath)
	if err != nil {
		t.Fatalf("checksum pgadmin wheel: %v", err)
	}

	opts := DefaultOptions()
	opts.OnlyStep = steps.InstallPGAdmin
	opts.RootFSPath = root
	opts.StateFilePath = filepath.Join(root, "var", "lib", "aipanel", ".installer-state.json")
	opts.ReportFilePath = filepath.Join(root, "var", "lib", "aipanel", "install-report.json")
	opts.LogFilePath = filepath.Join(root, "var", "log", "aipanel", "install.log")
	opts.PGAdminURL = "file://" + wheelPath
	opts.PGAdminSHA256 = sum
	opts.PGAdminSignatureURL = ""
	opts.PGAdminFingerprint = ""
	opts.VerifyUpstreamSources = false
	opts.PGAdminInstallDir = "/var/lib/aipanel/pgadmin4"
	opts.PGAdminVenvDir = "/var/lib/aipanel/pgadmin4-venv"
	opts.PGAdminDataDir = "/var/lib/aipanel/pgadmin-data"
	opts.PGAdminListenAddr = "127.0.0.1:5050"
	opts.PGAdminRoutePath = "/pgadmin"
	opts.RuntimeInstallDir = filepath.Join(root, "opt", "aipanel", "runtime")
	opts.NginxSitesAvailableDir = filepath.Join(root, "etc", "nginx", "sites-available")
	opts.NginxSitesEnabledDir = filepath.Join(root, "etc", "nginx", "sites-enabled")

	runner := &fakeRunner{}
	ins := New(opts, runner)
	report, err := ins.Run(context.Background())
	if err != nil {
		t.Fatalf("installer run failed: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("expected report status ok, got %q", report.Status)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("expected exactly one step in report, got %d", len(report.Steps))
	}
	if report.Steps[0].Name != steps.InstallPGAdmin {
		t.Fatalf("expected only %s step, got %s", steps.InstallPGAdmin, report.Steps[0].Name)
	}

	configLocal := filepath.Join(root, "var", "lib", "aipanel", "pgadmin4", "pgadmin4", "config_local.py")
	if _, err := os.Stat(configLocal); err != nil {
		t.Fatalf("expected config_local.py at %s: %v", configLocal, err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "python3 -m venv") {
		t.Fatalf("expected virtualenv setup command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "pip' install --target") && !strings.Contains(joined, "pip install --target") {
		t.Fatalf("expected wheel target install command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "setup-db") {
		t.Fatalf("expected pgAdmin setup-db command, got:\n%s", joined)
	}
	if !strings.Contains(joined, "systemctl enable --now aipanel-pgadmin.service") {
		t.Fatalf("expected pgAdmin service enable command, got:\n%s", joined)
	}
}

func TestResolveRuntimeSourceLock_DownloadsFromURLAndPersists(t *testing.T) {
	lockJSON := `{
  "schema_version": 1,
  "channels": {
    "stable": {
      "nginx": {
        "version": "1.29.5",
        "source_url": "https://nginx.org/download/nginx-1.29.5.tar.gz",
        "source_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
        "signature_url": "https://nginx.org/download/nginx-1.29.5.tar.gz.asc",
        "public_key_fingerprint": "43387825DDB1BB97EC36BA5D007C8D7C15D87369"
      }
    }
  }
}`

	root := t.TempDir()
	sourceLockPath := filepath.Join(root, "source.lock.json")
	if err := os.WriteFile(sourceLockPath, []byte(lockJSON), 0o600); err != nil {
		t.Fatalf("write source runtime lock: %v", err)
	}
	lockPath := filepath.Join(root, "sources.lock.json")

	opts := DefaultOptions()
	opts.RuntimeLockPath = lockPath
	opts.RuntimeLockURL = "file://" + sourceLockPath

	ins := New(opts, &fakeRunner{})
	lock, err := ins.resolveRuntimeSourceLock(context.Background())
	if err != nil {
		t.Fatalf("resolve runtime lock from URL: %v", err)
	}
	if lock == nil {
		t.Fatal("expected runtime lock to be loaded")
	}
	nginx, ok := lock.Channels["stable"]["nginx"]
	if !ok {
		t.Fatalf("expected nginx component in stable channel, got %+v", lock.Channels)
	}
	if nginx.Version != "1.29.5" {
		t.Fatalf("unexpected nginx version: %s", nginx.Version)
	}

	persisted, err := os.ReadFile(lockPath) //nolint:gosec // test reads file generated under temp dir.
	if err != nil {
		t.Fatalf("read persisted runtime lock: %v", err)
	}
	if !strings.Contains(string(persisted), "\"schema_version\": 1") {
		t.Fatalf("unexpected persisted runtime lock content: %s", string(persisted))
	}
	if err := os.Remove(sourceLockPath); err != nil {
		t.Fatalf("remove source runtime lock fixture: %v", err)
	}

	_, err = ins.resolveRuntimeSourceLock(context.Background())
	if err != nil {
		t.Fatalf("second resolve should use cache: %v", err)
	}
}

func writeTarGzArtifact(path string, name string, content []byte) error {
	return writeTarGzArtifactEntries(path, map[string][]byte{
		name: content,
	})
}

func writeTarGzArtifactEntries(path string, entries map[string][]byte) error {
	f, err := os.Create(path) //nolint:gosec // Test helper writes fixture file under t.TempDir.
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := entries[name]
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
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
