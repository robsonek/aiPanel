package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServer()
			return
		case "admin":
			runAdmin(os.Args[2:])
			return
		case "install":
			runInstall(os.Args[2:])
			return
		}
	}
	runServer()
}

func resolveConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("AIPANEL_CONFIG")); p != "" {
		return p
	}
	return "configs/defaults/panel.yaml"
}

func runServer() {
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
	opts.SkipHealthcheck = *skipHealthcheck

	runner := systemd.ExecRunner{DryRun: *dryRun}
	ins := installer.New(opts, runner)
	report, err := ins.Run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		if report != nil {
			fmt.Fprintf(os.Stderr, "report: %s\n", opts.ReportFilePath)
		}
		os.Exit(1)
	}
	fmt.Println("installation finished successfully")
	fmt.Printf("report: %s\n", opts.ReportFilePath)
}
