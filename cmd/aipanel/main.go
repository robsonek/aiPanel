package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
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
const minAdminPasswordLength = installer.MinAdminPasswordLength

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
	_, _ = fmt.Fprintln(w, "  aipanel install")
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
	fs, values := newInstallFlagSet(defaults)
	if len(args) == 1 && isHelpArg(args[0]) {
		printInstallUsage(os.Stdout, fs)
		return
	}

	if len(args) == 0 {
		opts, dryRun, err := promptInstallOptions(defaults, os.Stdin, os.Stdout)
		if err != nil {
			if errors.Is(err, errInstallCancelled) || errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr, "installation cancelled")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "interactive installer failed: %v\n", err)
			os.Exit(1)
		}
		runInstaller(opts, dryRun)
		return
	}

	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	opts, dryRun, err := values.toOptions(defaults)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	runInstaller(opts, dryRun)
}

type installFlagValues struct {
	addr            *string
	env             *string
	configPath      *string
	dataDir         *string
	panelBinary     *string
	unitFile        *string
	stateFile       *string
	reportFile      *string
	logFile         *string
	adminEmail      *string
	adminPassword   *string
	installMode     *string
	runtimeChannel  *string
	runtimeLockPath *string
	runtimeManifest *string
	runtimeInstall  *string
	reverseProxy    *bool
	panelDomain     *string
	skipHealthcheck *bool
	dryRun          *bool
}

func newInstallFlagSet(defaults installer.Options) (*flag.FlagSet, *installFlagValues) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	values := &installFlagValues{
		addr:            fs.String("addr", defaults.Addr, "panel listen address"),
		env:             fs.String("env", defaults.Env, "panel environment"),
		configPath:      fs.String("config", defaults.ConfigPath, "panel config file path"),
		dataDir:         fs.String("data-dir", defaults.DataDir, "panel data directory"),
		panelBinary:     fs.String("panel-binary", defaults.PanelBinaryPath, "target panel binary path"),
		unitFile:        fs.String("unit-file", defaults.UnitFilePath, "systemd unit file path"),
		stateFile:       fs.String("state-file", defaults.StateFilePath, "installer checkpoint state path"),
		reportFile:      fs.String("report-file", defaults.ReportFilePath, "installer report path"),
		logFile:         fs.String("log-file", defaults.LogFilePath, "installer log path"),
		adminEmail:      fs.String("admin-email", defaults.AdminEmail, "initial admin email"),
		adminPassword:   fs.String("admin-password", defaults.AdminPassword, "initial admin password"),
		installMode:     fs.String("install-mode", defaults.InstallMode, "runtime install mode: source-build"),
		runtimeChannel:  fs.String("runtime-channel", defaults.RuntimeChannel, "runtime release channel: stable|edge"),
		runtimeLockPath: fs.String("runtime-lock-path", defaults.RuntimeLockPath, "runtime source lock file path"),
		runtimeManifest: fs.String("runtime-manifest-url", defaults.RuntimeManifestURL, "runtime manifest URL (optional)"),
		runtimeInstall:  fs.String("runtime-install-dir", defaults.RuntimeInstallDir, "runtime install directory for source runtime modes"),
		reverseProxy:    fs.Bool("reverse-proxy", defaults.ReverseProxy, "bind panel to loopback and expose via nginx reverse proxy"),
		panelDomain:     fs.String("panel-domain", "", "panel domain for nginx server_name (required with --reverse-proxy)"),
		skipHealthcheck: fs.Bool("skip-healthcheck", false, "skip final /health check"),
		dryRun:          fs.Bool("dry-run", false, "do not execute system commands"),
	}
	return fs, values
}

func (v *installFlagValues) toOptions(defaults installer.Options) (installer.Options, bool, error) {
	opts := defaults
	opts.Addr = strings.TrimSpace(*v.addr)
	opts.Env = strings.TrimSpace(*v.env)
	opts.ConfigPath = strings.TrimSpace(*v.configPath)
	opts.DataDir = strings.TrimSpace(*v.dataDir)
	opts.PanelBinaryPath = strings.TrimSpace(*v.panelBinary)
	opts.UnitFilePath = strings.TrimSpace(*v.unitFile)
	opts.StateFilePath = strings.TrimSpace(*v.stateFile)
	opts.ReportFilePath = strings.TrimSpace(*v.reportFile)
	opts.LogFilePath = strings.TrimSpace(*v.logFile)
	opts.AdminEmail = strings.TrimSpace(*v.adminEmail)
	opts.AdminPassword = strings.TrimSpace(*v.adminPassword)
	opts.InstallMode = strings.TrimSpace(*v.installMode)
	opts.RuntimeChannel = strings.TrimSpace(*v.runtimeChannel)
	opts.RuntimeLockPath = strings.TrimSpace(*v.runtimeLockPath)
	opts.RuntimeManifestURL = strings.TrimSpace(*v.runtimeManifest)
	opts.RuntimeInstallDir = strings.TrimSpace(*v.runtimeInstall)
	if err := applyReverseProxySettings(&opts, *v.reverseProxy, strings.TrimSpace(*v.panelDomain)); err != nil {
		return installer.Options{}, false, err
	}
	if err := validateAdminPassword(opts.AdminPassword); err != nil {
		return installer.Options{}, false, err
	}
	opts.VerifyUpstreamSources = true
	opts.SkipHealthcheck = *v.skipHealthcheck
	return opts, *v.dryRun, nil
}

func printInstallUsage(w io.Writer, fs *flag.FlagSet) {
	_, _ = fmt.Fprintln(w, "usage: aipanel install")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Interactive mode (recommended):")
	_, _ = fmt.Fprintln(w, "  aipanel install")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Legacy non-interactive mode:")
	_, _ = fmt.Fprintln(w, "  aipanel install [flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "flags:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

var errInstallCancelled = errors.New("installation cancelled")

type promptValidator func(string) error

func promptInstallOptions(defaults installer.Options, in io.Reader, out io.Writer) (installer.Options, bool, error) {
	reader := bufio.NewReader(in)
	opts := defaults
	dryRun := false

	_, _ = fmt.Fprintln(out, "aiPanel installer interactive setup")
	_, _ = fmt.Fprintln(out, "Quick mode is recommended. For advanced flags use: aipanel install --help")
	_, _ = fmt.Fprintln(out)

	var err error
	useDefaults, err := promptBool(reader, out, "Use default installation settings", true)
	if err != nil {
		return installer.Options{}, false, err
	}
	if !useDefaults {
		_, _ = fmt.Fprintln(out, "Custom mode: provide only key settings; advanced options are available via flags.")
		if opts.Addr, err = promptString(reader, out, "Panel listen address", defaults.Addr, nonEmptyValidator("addr")); err != nil {
			return installer.Options{}, false, err
		}
		if opts.AdminEmail, err = promptString(reader, out, "Initial admin email", defaults.AdminEmail, nonEmptyValidator("admin-email")); err != nil {
			return installer.Options{}, false, err
		}
		if opts.AdminPassword, err = promptString(reader, out, "Initial admin password", defaults.AdminPassword, adminPasswordValidator()); err != nil {
			return installer.Options{}, false, err
		}
		if opts.RuntimeChannel, err = promptString(reader, out, "Runtime channel", defaults.RuntimeChannel, allowedValidator("runtime-channel", installer.RuntimeChannelStable, installer.RuntimeChannelEdge)); err != nil {
			return installer.Options{}, false, err
		}
		opts.RuntimeChannel = strings.ToLower(opts.RuntimeChannel)
	}
	enableReverseProxy, err := promptBool(reader, out, "Enable nginx reverse proxy for panel", false)
	if err != nil {
		return installer.Options{}, false, err
	}
	panelDomain := ""
	if enableReverseProxy {
		if panelDomain, err = promptString(reader, out, "Panel domain (e.g. panel.example.com)", "", nonEmptyValidator("panel domain")); err != nil {
			return installer.Options{}, false, err
		}
	}
	if err := applyReverseProxySettings(&opts, enableReverseProxy, panelDomain); err != nil {
		return installer.Options{}, false, err
	}
	if !useDefaults {
		if dryRun, err = promptBool(reader, out, "Dry run (do not execute commands)", false); err != nil {
			return installer.Options{}, false, err
		}
	}
	confirm, err := promptBool(reader, out, "Start installation now", true)
	if err != nil {
		return installer.Options{}, false, err
	}
	if !confirm {
		return installer.Options{}, false, errInstallCancelled
	}

	opts.VerifyUpstreamSources = true
	return opts, dryRun, nil
}

func applyReverseProxySettings(opts *installer.Options, enabled bool, domain string) error {
	if opts == nil {
		return fmt.Errorf("installer options are required")
	}
	opts.ReverseProxy = enabled
	if !enabled {
		opts.PanelDomain = "_"
		return nil
	}
	panelDomain := strings.TrimSpace(domain)
	if panelDomain == "" {
		return fmt.Errorf("panel domain is required when reverse proxy is enabled")
	}
	opts.PanelDomain = panelDomain
	opts.Addr = net.JoinHostPort("127.0.0.1", parseListenPort(opts.Addr))
	return nil
}

func parseListenPort(addr string) string {
	a := strings.TrimSpace(addr)
	if a == "" {
		return "8080"
	}
	if strings.HasPrefix(a, ":") && len(a) > 1 {
		return strings.TrimPrefix(a, ":")
	}
	_, port, err := net.SplitHostPort(a)
	if err == nil && strings.TrimSpace(port) != "" {
		return port
	}
	return "8080"
}

func nonEmptyValidator(field string) promptValidator {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", field)
		}
		return nil
	}
}

func adminPasswordValidator() promptValidator {
	return func(value string) error {
		return validateAdminPassword(value)
	}
}

func validateAdminPassword(password string) error {
	if len(strings.TrimSpace(password)) < minAdminPasswordLength {
		return fmt.Errorf("admin password must be at least %d characters", minAdminPasswordLength)
	}
	return nil
}

func allowedValidator(field string, allowed ...string) promptValidator {
	allowedMap := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedMap[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	return func(value string) error {
		if _, ok := allowedMap[strings.ToLower(strings.TrimSpace(value))]; ok {
			return nil
		}
		return fmt.Errorf("%s must be one of: %s", field, strings.Join(allowed, ", "))
	}
}

func promptString(
	reader *bufio.Reader,
	out io.Writer,
	label string,
	defaultValue string,
	validator promptValidator,
) (string, error) {
	for {
		if strings.TrimSpace(defaultValue) == "" {
			_, _ = fmt.Fprintf(out, "%s: ", label)
		} else {
			_, _ = fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
		}
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}
		if line == "" {
			line = defaultValue
		}
		line = strings.TrimSpace(line)
		if validator != nil {
			if err := validator(line); err != nil {
				if errors.Is(readErr, io.EOF) {
					return "", readErr
				}
				_, _ = fmt.Fprintf(out, "invalid value: %v\n", err)
				continue
			}
		}
		if errors.Is(readErr, io.EOF) && line == "" {
			return "", io.EOF
		}
		return line, nil
	}
}

func promptBool(reader *bufio.Reader, out io.Writer, label string, defaultValue bool) (bool, error) {
	defaultLabel := "y/N"
	if defaultValue {
		defaultLabel = "Y/n"
	}
	for {
		_, _ = fmt.Fprintf(out, "%s [%s]: ", label, defaultLabel)
		line, readErr := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return false, readErr
		}
		if line == "" {
			if errors.Is(readErr, io.EOF) {
				return false, io.EOF
			}
			return defaultValue, nil
		}
		switch line {
		case "y", "yes", "true", "1":
			return true, nil
		case "n", "no", "false", "0":
			return false, nil
		default:
			if errors.Is(readErr, io.EOF) {
				return false, io.EOF
			}
			_, _ = fmt.Fprintln(out, "invalid value: enter y or n")
		}
	}
}

func runInstaller(opts installer.Options, dryRun bool) {
	runner := systemd.ExecRunner{DryRun: dryRun}
	ins := installer.New(opts, runner)
	fmt.Printf(
		"installer start: mode=%s channel=%s lock=%s runtime_dir=%s verify_signatures=%t dry_run=%t\n",
		opts.InstallMode,
		opts.RuntimeChannel,
		opts.RuntimeLockPath,
		opts.RuntimeInstallDir,
		opts.VerifyUpstreamSources,
		dryRun,
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
