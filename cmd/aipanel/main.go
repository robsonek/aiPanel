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

	"github.com/robsonek/aiPanel/internal/modules/iam"
	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/httpserver"
	"github.com/robsonek/aiPanel/internal/platform/logger"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
)

func newHandler(cfg config.Config, log *slog.Logger, iamSvc *iam.Service) http.Handler {
	return httpserver.NewHandler(cfg, log, iamSvc)
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
		}
	}
	runServer()
}

func runServer() {
	cfg, err := config.Load("configs/defaults/panel.yaml")
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}
	log := logger.New(cfg.Env)
	store := sqlite.New(cfg.DataDir)
	if err := store.Init(context.Background()); err != nil {
		panic(fmt.Errorf("init sqlite: %w", err))
	}
	iamSvc := iam.NewService(store, cfg, log)

	log.Info("aiPanel starting", "addr", cfg.Addr, "env", cfg.Env)

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      newHandler(cfg, log, iamSvc),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

	cfg, err := config.Load("configs/defaults/panel.yaml")
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
