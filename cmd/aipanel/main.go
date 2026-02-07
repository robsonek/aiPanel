package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/robsonek/aiPanel/internal/installer"
	"github.com/robsonek/aiPanel/internal/modules/database"
	"github.com/robsonek/aiPanel/internal/modules/hosting"
	"github.com/robsonek/aiPanel/internal/modules/iam"
	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/httpserver"
	"github.com/robsonek/aiPanel/internal/platform/logger"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
	"github.com/robsonek/aiPanel/internal/platform/systemd"
)

func newHandler(
	cfg config.Config,
	log *slog.Logger,
	iamSvc *iam.Service,
	hostingSvc *hosting.Service,
	databaseSvc *database.Service,
) http.Handler {
	return httpserver.NewHandler(cfg, log, iamSvc, hostingSvc, databaseSvc)
}

var lookupCommandPath = exec.LookPath

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runServer()
		return
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return
	case "serve":
		if len(args) > 1 && isHelpArg(args[1]) {
			_, _ = fmt.Fprintln(os.Stdout, "usage: aipanel serve")
			return
		}
		runServer()
		return
	case "admin":
		runAdmin(args[1:])
		return
	case "install":
		runInstall(args[1:])
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

func resolveConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("AIPANEL_CONFIG")); p != "" {
		return p
	}
	return "configs/defaults/panel.yaml"
}

func isHelpArg(arg string) bool {
	arg = strings.TrimSpace(strings.ToLower(arg))
	return arg == "-h" || arg == "--help" || arg == "help"
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: aipanel [command]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "commands:")
	_, _ = fmt.Fprintln(w, "  serve          start panel server (default when no command is provided)")
	_, _ = fmt.Fprintln(w, "  admin create   create admin user")
	_, _ = fmt.Fprintln(w, "  install        run installer")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "examples:")
	_, _ = fmt.Fprintln(w, "  aipanel serve")
	_, _ = fmt.Fprintln(w, "  aipanel admin create --email admin@example.com --password Secret123!")
	_, _ = fmt.Fprintln(w, "  aipanel install --runtime-lock-path /etc/aipanel/sources.lock.json")
}

func ensureRequiredTools(scope string, required []string) error {
	missing := missingTools(required)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"missing required system tools for %s: %s\ninstall with: sudo apt-get update && sudo apt-get install -y %s",
		scope,
		strings.Join(missing, ", "),
		strings.Join(missing, " "),
	)
}

func missingTools(required []string) []string {
	missing := make([]string, 0)
	for _, tool := range required {
		name := strings.TrimSpace(tool)
		if name == "" {
			continue
		}
		if _, err := lookupCommandPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func runServer() {
	if err := ensureRequiredTools("serve", []string{"sqlite3"}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}
	log := logger.New(cfg.Env)
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		panic(fmt.Errorf("init sqlite: %w", err))
	}
	iamSvc := iam.NewService(store, cfg, log)
	runner := systemd.ExecRunner{}
	nginxAdapter := hosting.NewNginxAdapter(runner, hosting.NginxAdapterOptions{})
	phpfpmAdapter := hosting.NewPHPFPMAdapter(runner, hosting.PHPFPMAdapterOptions{})
	hostingSvc := hosting.NewService(store, cfg, log, runner, nginxAdapter, phpfpmAdapter)
	mariadbAdapter := database.NewMariaDBAdapter(runner)
	databaseSvc := database.NewService(store, cfg, log, mariadbAdapter)

	log.Info("aiPanel starting", "addr", cfg.Addr, "env", cfg.Env, "config_path", cfgPath, "data_dir", cfg.DataDir)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newHandler(cfg, log, iamSvc, hostingSvc, databaseSvc),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Error("server exited", "error", err.Error())
		os.Exit(1)
	}
}

func runAdmin(args []string) {
	if err := ensureRequiredTools("admin", []string{"sqlite3"}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: aipanel admin create --email <email> --password <password>")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("admin create", flag.ExitOnError)
	email := fs.String("email", "", "admin email")
	password := fs.String("password", "", "admin password")
	_ = fs.Parse(args[1:])

	if strings.TrimSpace(*email) == "" || strings.TrimSpace(*password) == "" {
		fmt.Fprintln(os.Stderr, "email and password are required")
		os.Exit(2)
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	log := logger.New(cfg.Env)
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "init sqlite: %v\n", err)
		os.Exit(1)
	}
	iamSvc := iam.NewService(store, cfg, log)
	if err := iamSvc.CreateAdmin(context.Background(), *email, *password); err != nil {
		fmt.Fprintf(os.Stderr, "create admin: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("admin user created")
}

func runInstall(args []string) {
	defaults := installer.DefaultOptions()
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	addr := fs.String("addr", defaults.Addr, "panel listen address")
	env := fs.String("env", defaults.Env, "panel environment")
	configPath := fs.String("config", defaults.ConfigPath, "panel config file path")
	dataDir := fs.String("data-dir", defaults.DataDir, "panel data directory")
	panelBinary := fs.String("panel-binary", defaults.PanelBinaryPath, "target panel binary path")
	sourceBinary := fs.String("source-binary", "", "source panel binary path (defaults to current executable)")
	unitFile := fs.String("unit-file", defaults.UnitFilePath, "systemd unit file path")
	stateFile := fs.String("state-file", defaults.StateFilePath, "installer checkpoint state path")
	reportFile := fs.String("report-file", defaults.ReportFilePath, "installer report path")
	logFile := fs.String("log-file", defaults.LogFilePath, "installer log path")
	adminEmail := fs.String("admin-email", defaults.AdminEmail, "initial admin email")
	adminPassword := fs.String("admin-password", defaults.AdminPassword, "initial admin password")
	installMode := fs.String("install-mode", defaults.InstallMode, "runtime install mode: source-build")
	runtimeChannel := fs.String("runtime-channel", defaults.RuntimeChannel, "runtime release channel: stable|edge")
	runtimeLockPath := fs.String("runtime-lock-path", defaults.RuntimeLockPath, "runtime source lock file path")
	runtimeManifestURL := fs.String("runtime-manifest-url", defaults.RuntimeManifestURL, "runtime manifest URL (optional)")
	runtimeInstallDir := fs.String("runtime-install-dir", defaults.RuntimeInstallDir, "runtime install directory for source runtime modes")
	skipHealthcheck := fs.Bool("skip-healthcheck", false, "skip final /health check")
	dryRun := fs.Bool("dry-run", false, "do not execute system commands")
	_ = fs.Parse(args)

	opts := installer.DefaultOptions()
	opts.Addr = strings.TrimSpace(*addr)
	opts.Env = strings.TrimSpace(*env)
	opts.ConfigPath = strings.TrimSpace(*configPath)
	opts.DataDir = strings.TrimSpace(*dataDir)
	opts.PanelBinaryPath = strings.TrimSpace(*panelBinary)
	opts.SourceBinaryPath = strings.TrimSpace(*sourceBinary)
	opts.UnitFilePath = strings.TrimSpace(*unitFile)
	opts.StateFilePath = strings.TrimSpace(*stateFile)
	opts.ReportFilePath = strings.TrimSpace(*reportFile)
	opts.LogFilePath = strings.TrimSpace(*logFile)
	opts.AdminEmail = strings.TrimSpace(*adminEmail)
	opts.AdminPassword = strings.TrimSpace(*adminPassword)
	opts.InstallMode = strings.TrimSpace(*installMode)
	opts.RuntimeChannel = strings.TrimSpace(*runtimeChannel)
	opts.RuntimeLockPath = strings.TrimSpace(*runtimeLockPath)
	opts.RuntimeManifestURL = strings.TrimSpace(*runtimeManifestURL)
	opts.RuntimeInstallDir = strings.TrimSpace(*runtimeInstallDir)
	opts.VerifyUpstreamSources = true
	opts.SkipHealthcheck = *skipHealthcheck

	runner := systemd.ExecRunner{DryRun: *dryRun}
	ins := installer.New(opts, runner)
	fmt.Printf(
		"installer start: mode=%s channel=%s lock=%s runtime_dir=%s verify_signatures=%t dry_run=%t\n",
		opts.InstallMode,
		opts.RuntimeChannel,
		opts.RuntimeLockPath,
		opts.RuntimeInstallDir,
		opts.VerifyUpstreamSources,
		*dryRun,
	)
	report, err := ins.Run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		if report != nil {
			fmt.Fprintln(os.Stderr, "steps:")
			for _, step := range report.Steps {
				if strings.TrimSpace(step.Error) == "" {
					fmt.Fprintf(os.Stderr, "- %s: %s\n", step.Name, step.Status)
					continue
				}
				fmt.Fprintf(os.Stderr, "- %s: %s (%s)\n", step.Name, step.Status, step.Error)
			}
			fmt.Fprintf(os.Stderr, "report: %s\n", opts.ReportFilePath)
		}
		os.Exit(1)
	}
	fmt.Println("installation finished successfully")
	fmt.Printf("report: %s\n", opts.ReportFilePath)
}
