package installer

import (
	"context"
	"fmt"
	"os"
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
	opts.SkipHealthcheck = true
	opts.MinCPU = 1

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
