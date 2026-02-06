# aiPanel — Project Structure

> Monorepo layout for a modular-monolith hosting panel.
> Single binary backend (Go 1.24+), embedded SPA frontend (React 19 + Vite), SQLite storage.

---

## 1. Repository Root

```text
aiPanel/
├── cmd/
│   └── aipanel/                    # Main entrypoint — single binary
│       └── main.go                 # Wires modules, starts HTTP server
│
├── internal/                       # Private application code (Go)
│   ├── installer/                  # One-shot Debian 13 installer logic
│   │   ├── steps/                  # Ordered install steps (validate, deps, configure…)
│   │   ├── report.go               # Post-install report generator
│   │   └── installer.go            # Orchestrator
│   │
│   ├── modules/                    # Domain modules (modular monolith)
│   │   ├── iam/                    # Identity & Access Management
│   │   │   ├── handler.go          # HTTP handlers (Chi routes)
│   │   │   ├── service.go          # Business logic
│   │   │   ├── repository.go       # SQLite persistence
│   │   │   ├── model.go            # Domain entities
│   │   │   └── iam_test.go         # Unit / integration tests
│   │   │
│   │   ├── hosting/                # Sites, domains, vhosts, TLS, PHP runtime
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   ├── model.go
│   │   │   ├── adapter_nginx.go    # Nginx vhost generation, reload
│   │   │   ├── adapter_phpfpm.go   # PHP-FPM pool config, version switch
│   │   │   ├── adapter_certbot.go  # TLS cert provisioning (lego/ACME)
│   │   │   └── hosting_test.go
│   │   │
│   │   ├── database/               # Database Management (MariaDB + PostgreSQL)
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   ├── model.go
│   │   │   ├── adapter_mariadb.go
│   │   │   ├── adapter_postgres.go
│   │   │   └── database_test.go
│   │   │
│   │   ├── backup/                 # Backup & Restore
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   ├── model.go
│   │   │   ├── adapter_fs.go       # File-level backup
│   │   │   ├── adapter_dbdump.go   # DB dump / restore
│   │   │   └── backup_test.go
│   │   │
│   │   ├── audit/                  # Audit & Compliance (append-only event log)
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go       # Writes to audit.db
│   │   │   ├── model.go
│   │   │   └── audit_test.go
│   │   │
│   │   ├── versionmgr/            # Version Manager (feed sync, policy, rollout)
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   ├── model.go
│   │   │   ├── feed.go             # Upstream version feed sync
│   │   │   ├── policy.go           # Policy engine (auto-patch/minor/major)
│   │   │   ├── preflight.go        # Compatibility matrix & health checks
│   │   │   ├── rollout.go          # Canary / wave orchestration
│   │   │   └── versionmgr_test.go
│   │   │
│   │   ├── monitoring/             # Monitoring & Health
│   │   │   ├── handler.go
│   │   │   ├── service.go
│   │   │   ├── collector.go        # CPU / RAM / disk metrics
│   │   │   ├── healthcheck.go      # Service health probes
│   │   │   ├── model.go
│   │   │   └── monitoring_test.go
│   │   │
│   │   └── filemanager/            # File Manager (browse, edit, upload)
│   │       ├── handler.go
│   │       ├── service.go
│   │       ├── model.go
│   │       └── filemanager_test.go
│   │
│   └── platform/                   # Shared infrastructure (cross-cutting)
│       ├── config/                 # App config loading (env, YAML, flags)
│       │   └── config.go
│       ├── logger/                 # Structured logging (slog-based)
│       │   └── logger.go
│       ├── httpserver/             # Chi server bootstrap, graceful shutdown
│       │   └── server.go
│       ├── sqlite/                 # SQLite connection manager (WAL mode)
│       │   ├── sqlite.go           # Opens panel.db, audit.db, queue.db
│       │   └── sqlite_test.go
│       ├── jobqueue/               # SQLite-based async job queue
│       │   ├── queue.go            # Enqueue / dequeue / retry logic
│       │   ├── worker.go           # Background worker pool
│       │   └── jobqueue_test.go
│       ├── middleware/             # HTTP middleware stack
│       │   ├── auth.go             # JWT / session validation
│       │   ├── audit.go            # Auto audit-log for mutating requests
│       │   ├── ratelimit.go        # Brute-force / abuse protection
│       │   ├── cors.go
│       │   └── recover.go
│       └── systemd/               # Helpers: exec wrappers, systemctl, nftables…
│           ├── exec.go             # Safe command execution
│           ├── adapter_systemd.go  # Service start/stop/enable
│           ├── adapter_nftables.go # Firewall rule management
│           ├── adapter_fail2ban.go # Jail configuration
│           ├── adapter_apt.go      # Package install / upgrade
│           └── systemd_test.go
│
├── pkg/                            # Public contracts between modules
│   ├── dto/                        # Shared DTOs (request / response structs)
│   │   ├── site.go
│   │   ├── database.go
│   │   ├── backup.go
│   │   ├── user.go
│   │   ├── job.go
│   │   └── version.go
│   ├── iface/                      # Module interfaces (ports)
│   │   ├── iam.go                  # type IAMService interface { … }
│   │   ├── hosting.go
│   │   ├── database.go
│   │   ├── backup.go
│   │   ├── audit.go
│   │   ├── versionmgr.go
│   │   ├── monitoring.go
│   │   ├── filemanager.go
│   │   └── jobqueue.go
│   └── adapter/                    # System adapter interfaces
│       ├── nginx.go                # type NginxAdapter interface { … }
│       ├── phpfpm.go
│       ├── mariadb.go
│       ├── postgres.go
│       ├── systemd.go
│       ├── nftables.go
│       ├── fail2ban.go
│       └── apt.go
│
├── migrations/                     # Goose SQL migrations
│   ├── panel/                      # Migrations for panel.db
│   │   ├── 001_init_schema.sql
│   │   └── 002_iam_tables.sql
│   ├── audit/                      # Migrations for audit.db
│   │   └── 001_audit_log.sql
│   └── queue/                      # Migrations for queue.db
│       └── 001_job_queue.sql
│
├── configs/                        # System configuration management
│   ├── templates/                  # Go text/template files
│   │   ├── nginx_vhost.conf.tmpl
│   │   ├── nginx_ssl.conf.tmpl
│   │   ├── phpfpm_pool.conf.tmpl
│   │   ├── systemd_unit.service.tmpl
│   │   └── nftables.conf.tmpl
│   └── defaults/                   # Default config values
│       ├── panel.yaml              # Panel defaults (ports, paths, limits)
│       ├── nginx.yaml              # Nginx tuning defaults
│       ├── phpfpm.yaml             # PHP-FPM pool defaults
│       └── security.yaml           # SSH hardening, firewall baseline
│
├── web/                            # Frontend — React 19 SPA (Vite)
│   ├── index.html
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── tailwind.config.ts
│   ├── package.json
│   ├── pnpm-lock.yaml
│   │
│   ├── public/
│   │   └── favicon.svg
│   │
│   └── src/
│       ├── main.tsx                # App entrypoint
│       ├── App.tsx                 # Root component, router, providers
│       │
│       ├── components/
│       │   ├── ui/                 # Shadcn/ui primitives (Button, Dialog, Table…)
│       │   │   ├── button.tsx
│       │   │   ├── dialog.tsx
│       │   │   ├── table.tsx
│       │   │   ├── toast.tsx
│       │   │   └── ...
│       │   └── shared/             # App-level reusable components
│       │       ├── AppShell.tsx     # Sidebar + Topbar + Content layout
│       │       ├── MetricCard.tsx
│       │       ├── StatusBadge.tsx
│       │       ├── ConfirmModal.tsx
│       │       └── AuditTimeline.tsx
│       │
│       ├── features/               # Feature modules (page-level)
│       │   ├── dashboard/
│       │   │   ├── DashboardPage.tsx
│       │   │   ├── HealthScore.tsx
│       │   │   ├── ServiceStatus.tsx
│       │   │   └── dashboard.test.tsx
│       │   ├── sites/
│       │   │   ├── SitesPage.tsx
│       │   │   ├── SiteDetail.tsx
│       │   │   ├── CreateSiteForm.tsx
│       │   │   └── sites.test.tsx
│       │   ├── databases/
│       │   │   ├── DatabasesPage.tsx
│       │   │   └── databases.test.tsx
│       │   ├── backups/
│       │   │   ├── BackupsPage.tsx
│       │   │   ├── RestoreWizard.tsx
│       │   │   └── backups.test.tsx
│       │   ├── security/
│       │   │   ├── SecurityPage.tsx
│       │   │   ├── AuditLog.tsx
│       │   │   └── security.test.tsx
│       │   ├── updates/
│       │   │   ├── UpdatesPage.tsx
│       │   │   ├── RolloutPlan.tsx
│       │   │   └── updates.test.tsx
│       │   ├── settings/
│       │   │   ├── SettingsPage.tsx
│       │   │   ├── ProfileSettings.tsx
│       │   │   ├── ThemeToggle.tsx
│       │   │   ├── LanguageSelector.tsx
│       │   │   └── settings.test.tsx
│       │   └── filemanager/
│       │       ├── FileManagerPage.tsx
│       │       ├── CodeEditor.tsx
│       │       └── filemanager.test.tsx
│       │
│       ├── lib/                    # Utilities and API layer
│       │   ├── api.ts              # HTTP client (fetch wrapper, auth headers)
│       │   ├── queryClient.ts      # TanStack Query configuration
│       │   ├── router.ts           # TanStack Router definition
│       │   ├── utils.ts            # General helpers
│       │   └── hooks/
│       │       ├── useAuth.ts
│       │       ├── useTheme.ts
│       │       └── useI18n.ts
│       │
│       ├── locales/                # i18next translation files
│       │   ├── en.json             # English (default)
│       │   ├── pl.json             # Polish
│       │   └── de.json             # German (example)
│       │
│       └── theme/                  # Design tokens and theming
│           ├── tokens.css          # CSS custom properties (semantic tokens)
│           ├── light.css           # Light theme map
│           ├── dark.css            # Dark theme map
│           └── fonts.css           # IBM Plex Sans, Space Grotesk
│
├── test/                           # Cross-cutting test infrastructure
│   ├── e2e/                        # Playwright end-to-end tests
│   │   ├── playwright.config.ts
│   │   ├── dashboard.spec.ts
│   │   ├── sites.spec.ts
│   │   ├── auth.spec.ts
│   │   └── visual/                 # Visual regression snapshots (dark + light)
│   │       ├── dashboard-light.png
│   │       └── dashboard-dark.png
│   ├── fixtures/                   # Shared test data
│   │   ├── sites.json
│   │   ├── users.json
│   │   └── seed.sql
│   └── vm/                         # Installer integration tests
│       ├── Vagrantfile             # Debian 13 clean VM
│       ├── provision.sh            # Automated install test script
│       └── verify.sh               # Post-install verification
│
├── scripts/                        # Build, release, and dev helpers
│   ├── build.sh                    # Build single binary (embed frontend)
│   ├── dev.sh                      # Run backend + Vite dev server
│   ├── release.sh                  # Tag, build, package release
│   ├── migrate.sh                  # Run goose migrations
│   ├── generate.sh                 # Code generation (if any)
│   └── install.sh                  # Production installer entry script
│
├── docs/                           # Project documentation
│   ├── PRD-hosting-panel.md     # Product Requirements Document
│   ├── project-structure.md        # This file
│   └── adr/                        # Architecture Decision Records
│       ├── 001-go-chi-monolith.md
│       ├── 002-sqlite-panel-db.md
│       └── 003-react-vite-frontend.md
│
├── .github/
│   └── workflows/
│       ├── ci.yml                  # Lint, test, build (Go + frontend)
│       ├── e2e.yml                 # Playwright E2E suite
│       ├── release.yml             # Build binary, create GitHub release
│       └── security.yml            # Dependency audit, SBOM generation
│
├── .gitignore
├── .golangci.yml                   # Go linter configuration
├── go.mod
├── go.sum
├── Makefile                        # Primary build orchestration
├── Taskfile.yml                    # Alternative task runner (go-task)
├── LICENSE                         # MIT
└── README.md
```

---

## 2. Backend Layout (Go)

### 2.1 Entrypoint — `cmd/aipanel/`

The single `main.go` file is responsible for:

- Loading configuration (`internal/platform/config`).
- Opening SQLite connections (panel.db, audit.db, queue.db).
- Running goose migrations at startup.
- Initializing each domain module and injecting dependencies.
- Registering all Chi routes.
- Embedding the built frontend via `embed.FS` and serving it from the root path.
- Starting the HTTP server with graceful shutdown.

```go
//go:embed all:web/dist
var frontendFS embed.FS
```

### 2.2 Domain Modules — `internal/modules/`

Each module follows a consistent internal layout:

| File              | Responsibility                                           |
|-------------------|----------------------------------------------------------|
| `handler.go`      | HTTP handlers; maps routes, validates input, returns JSON |
| `service.go`      | Business logic; orchestrates repositories and adapters    |
| `repository.go`   | Data access layer (SQLite); implements `pkg/iface` repo   |
| `model.go`        | Domain entities and value objects                         |
| `adapter_*.go`    | System adapter implementations (Nginx, PHP-FPM, etc.)    |
| `*_test.go`       | Unit and integration tests colocated with source          |

Module list:

| Module          | Domain                                                      |
|-----------------|-------------------------------------------------------------|
| `iam`           | Authentication, authorization (RBAC), MFA, sessions         |
| `hosting`       | Sites, domains, vhosts, TLS certs, PHP runtime, deployment  |
| `database`      | MariaDB and PostgreSQL DB/user management                   |
| `backup`        | Scheduled backups, restore wizard, snapshot management       |
| `audit`         | Append-only audit event log, export, filtering              |
| `versionmgr`   | Feed sync, policy engine, preflight, canary/wave rollout    |
| `monitoring`    | CPU/RAM/disk metrics, service health checks, alerting       |
| `filemanager`   | File browse, upload, download, edit, chmod/chown            |

### 2.3 Shared Platform — `internal/platform/`

Cross-cutting concerns shared by all modules:

| Package       | Purpose                                                          |
|---------------|------------------------------------------------------------------|
| `config`      | Load config from env vars, YAML files, and CLI flags             |
| `logger`      | Structured logging via `slog` (JSON output in production)        |
| `httpserver`   | Chi router bootstrap, TLS, graceful shutdown                     |
| `sqlite`      | Connection pool for 3 SQLite files; WAL mode enforcement         |
| `jobqueue`    | SQLite-backed async queue: enqueue, dequeue, retry, dead-letter  |
| `middleware`   | Auth, audit trail, rate limiting, CORS, panic recovery           |
| `systemd`     | Safe `exec.Command` wrappers and system adapter implementations  |

### 2.4 Installer — `internal/installer/`

Self-contained logic for the one-shot Debian 13 installer:

- Environment validation (OS, architecture, clean state).
- Dependency installation via apt adapter.
- Service configuration (Nginx, PHP-FPM, MariaDB/PostgreSQL, firewall).
- Panel bootstrapping (DB migration, admin account creation).
- Post-install report and rollback point.
- Idempotent re-run support (INS-007).

### 2.5 Public Contracts — `pkg/`

| Package        | Contents                                                     |
|----------------|--------------------------------------------------------------|
| `pkg/dto`      | Shared request/response structs used across module boundaries |
| `pkg/iface`    | Go interfaces defining module service contracts               |
| `pkg/adapter`  | Go interfaces for system adapters (Nginx, PHP-FPM, etc.)     |

All inter-module communication goes through interfaces declared in `pkg/iface`.
No module may import another module's `internal` package.

### 2.6 Migrations — `migrations/`

Managed by [goose](https://github.com/pressly/goose). Separate directories per SQLite database:

- `migrations/panel/` — schema for config, sessions, sites, users, version state.
- `migrations/audit/` — schema for the append-only audit event log.
- `migrations/queue/` — schema for the job queue tables.

---

## 3. Frontend Layout (`web/`)

### 3.1 Stack

- **React 19** + **TypeScript** — type-safe component development.
- **Vite** — fast builds, HMR, optimized production output.
- **TailwindCSS 4** — utility-first styling driven by design tokens.
- **Shadcn/ui** (Radix UI) — accessible UI primitives in `components/ui/`.
- **TanStack Query** — server-state management (cache, refetch, optimistic updates).
- **TanStack Router** — type-safe file-based routing with code splitting.
- **i18next + react-i18next** — internationalization with lazy-loaded JSON files.

### 3.2 Directory Overview

| Path                      | Purpose                                                 |
|---------------------------|---------------------------------------------------------|
| `src/components/ui/`      | Shadcn/ui primitives (Button, Dialog, Table, Toast...)  |
| `src/components/shared/`  | App-level reusable components (AppShell, MetricCard...) |
| `src/features/`           | Feature modules, one directory per page/domain          |
| `src/lib/`                | API client, TanStack Query config, router, utilities    |
| `src/lib/hooks/`          | Custom React hooks (auth, theme, i18n)                  |
| `src/locales/`            | i18next JSON files — one per language                   |
| `src/theme/`              | CSS custom properties, light/dark maps, font imports    |

### 3.3 Feature Modules

Each feature directory contains page components, sub-components, and colocated test files (`*.test.tsx`):

- `dashboard` — Health score, service status, alerts, recent deployments.
- `sites` — Site list, site detail, create/edit forms, TLS status.
- `databases` — DB list, create DB/user, connection info.
- `backups` — Backup schedules, restore points, restore wizard.
- `security` — Threat summary, login activity, audit timeline.
- `updates` — Version compliance table, rollout plan, rollback history.
- `settings` — Profile, theme toggle, language selector, account limits.
- `filemanager` — File browser, code editor with syntax highlighting.

### 3.4 Theming

Design tokens are defined as CSS custom properties in `src/theme/tokens.css`. Two theme maps (`light.css`, `dark.css`) provide concrete values. The active theme is set via `data-theme="light|dark"` on the `<html>` element before app init to prevent FOUC.

### 3.5 Internationalization

- Default language: English (`en.json`).
- One JSON file per language in `src/locales/`.
- Namespaced keys per feature (e.g., `dashboard.healthScore`, `sites.createSite`).
- Lazy loading — only the active language bundle is fetched.
- Adding a new language requires only a new JSON file; no code changes needed.

---

## 4. System Configs — `configs/`

### 4.1 Templates (`configs/templates/`)

Go `text/template` files used by system adapters to generate configuration for managed services:

- `nginx_vhost.conf.tmpl` — Per-site Nginx virtual host.
- `nginx_ssl.conf.tmpl` — TLS-specific Nginx directives.
- `phpfpm_pool.conf.tmpl` — Per-site PHP-FPM pool.
- `systemd_unit.service.tmpl` — Custom systemd unit for panel or managed services.
- `nftables.conf.tmpl` — Firewall ruleset.

### 4.2 Defaults (`configs/defaults/`)

YAML files with sensible default values for panel configuration, Nginx tuning, PHP-FPM pool sizing, and security baselines (SSH hardening, firewall rules).

---

## 5. Tests

### 5.1 Unit and Integration Tests

Tests live alongside the source code they cover:

- **Go**: `*_test.go` files in the same package (e.g., `internal/modules/iam/iam_test.go`).
- **React**: `*.test.tsx` files in the same feature directory (e.g., `web/src/features/dashboard/dashboard.test.tsx`).

### 5.2 End-to-End Tests (`test/e2e/`)

Playwright-based browser tests covering critical user workflows:

- Dashboard loading and navigation.
- Site creation with TLS provisioning.
- Authentication and RBAC enforcement.
- Visual regression snapshots for both dark and light themes across three breakpoints (desktop, tablet, mobile).

### 5.3 Test Fixtures (`test/fixtures/`)

Shared test data (JSON seed files, SQL seed scripts) used by both Go integration tests and Playwright specs.

### 5.4 VM Tests (`test/vm/`)

Vagrant-based integration environment for testing the installer on a clean Debian 13 image:

- `Vagrantfile` — Defines the VM (Debian 13, minimal).
- `provision.sh` — Runs the installer inside the VM.
- `verify.sh` — Post-install checks (services running, ports open, panel accessible).

---

## 6. Build and CI

### 6.1 Scripts (`scripts/`)

| Script           | Purpose                                             |
|------------------|-----------------------------------------------------|
| `build.sh`       | Build frontend, embed into Go binary, compile       |
| `dev.sh`         | Start Go backend + Vite dev server (with proxy)     |
| `release.sh`     | Tag version, build release binary, create archive   |
| `migrate.sh`     | Run goose migrations against local SQLite files     |
| `generate.sh`    | Run code generators (mocks, OpenAPI, etc.)          |
| `install.sh`     | Production installer entry point (curl-pipe target) |

### 6.2 Makefile / Taskfile.yml

Both provided for flexibility. Common targets:

```makefile
build        # Full production build (frontend + Go binary)
dev          # Start dev environment (backend + Vite HMR)
test         # Run all Go tests
test-fe      # Run frontend tests (Vitest)
test-e2e     # Run Playwright E2E suite
lint         # golangci-lint + eslint
migrate      # Run DB migrations
clean        # Remove build artifacts
```

### 6.3 GitHub Actions (`.github/workflows/`)

| Workflow         | Trigger           | Steps                                          |
|------------------|-------------------|-------------------------------------------------|
| `ci.yml`         | Push / PR         | Lint, unit tests, build (Go + frontend)         |
| `e2e.yml`        | Push to main / PR | Playwright E2E against built binary             |
| `release.yml`    | Tag `v*`          | Build binary, create GitHub Release with assets |
| `security.yml`   | Schedule / PR     | `govulncheck`, `npm audit`, SBOM generation     |

---

## 7. Design Principles

### 7.1 Module Isolation

- Modules communicate **only** through interfaces defined in `pkg/iface`.
- No module imports another module's `internal` package.
- Shared data structures live in `pkg/dto`.

### 7.2 Adapter Pattern

Every system-level integration (Nginx, PHP-FPM, MariaDB, PostgreSQL, systemd, nftables, fail2ban, apt) is defined as an interface in `pkg/adapter` and implemented inside the relevant module or `internal/platform/systemd`.

This enables:
- Testing with mock adapters (no real system calls in unit tests).
- Swapping implementations without changing business logic.

### 7.3 Single Binary Build

The entire application ships as one statically-linked binary:

```bash
go build -o aipanel ./cmd/aipanel
```

The frontend is compiled by Vite into `web/dist/` and embedded at compile time using Go's `embed.FS`. No separate web server or Node.js runtime is required in production.

### 7.4 SQLite Separation

Three separate SQLite files prevent write contention between domains:

| File         | Contents                                      | Access Pattern       |
|--------------|-----------------------------------------------|----------------------|
| `panel.db`   | Config, sessions, sites, users, version state | Mixed read/write     |
| `audit.db`   | Append-only audit event log                   | High write volume    |
| `queue.db`   | Job queue (pending, running, completed jobs)  | Frequent write/delete|

All databases run in WAL mode for concurrent read/write access.

### 7.5 Repository Pattern

Data access is abstracted behind repository interfaces, allowing a future migration from SQLite to PostgreSQL without changes to business logic (per PRD migration thresholds).

### 7.6 Frontend Embedding

The build pipeline:

1. `pnpm --dir web build` — Vite compiles the React app into `web/dist/`.
2. `go build ./cmd/aipanel` — Go embeds `web/dist/` via `//go:embed` directive.
3. At runtime, the Chi router serves the SPA from the embedded filesystem, with API routes mounted under `/api/v1/`.
