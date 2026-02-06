# Sprint 1 Plan — aiPanel

| Field            | Value                                                                                         |
|------------------|-----------------------------------------------------------------------------------------------|
| Sprint           | 1                                                                                             |
| Duration         | 2 weeks (10 working days)                                                                     |
| Start date       | TBD                                                                                           |
| Sprint goal      | "Working panel skeleton: installer bootstraps Debian 13, panel binary serves authenticated UI shell with dark/light theme." |

---

## 1. Task Breakdown

| ID   | Task                        | Effort | Dependencies | Acceptance Criteria |
|------|-----------------------------|--------|--------------|---------------------|
| SP1-1 | Project Scaffolding        | S (1-2d) | none        | `make build` produces a single Go binary with embedded frontend. `make dev` starts the Vite dev server with HMR and the Go backend in parallel. Go module initialized as `github.com/aiPanel/aipanel`. React app bootstrapped with Vite + TypeScript + TailwindCSS 4 + pnpm. GitHub Actions CI workflow runs lint + test + build on every PR. Pre-commit hooks (lefthook) enforce gofmt, golangci-lint, eslint, prettier. Folder structure matches `docs/project-structure.md` (all directories created, placeholder files where needed). |
| SP1-2 | Platform Foundation        | M (3-4d) | SP1-1       | Panel binary starts and listens on configured port. Config loader reads `configs/defaults/panel.yaml` and allows env var overrides (`AIPANEL_*` prefix). Structured logger outputs JSON via Go `slog`. Three SQLite files (`panel.db`, `audit.db`, `queue.db`) are created at startup in WAL mode. Goose migrations run automatically on startup (initial schema: `users`, `sessions`, `audit_events`, `jobs` tables). Chi HTTP server boots with middleware stack: request ID, structured logging, panic recovery, CORS. `GET /health` returns `200 OK` with JSON payload (`{"status":"ok"}`). In production mode, frontend is served from `embed.FS`; in dev mode, Vite proxy is used. |
| SP1-3 | IAM Module — Basic Auth    | M (3-4d) | SP1-2       | CLI command `aipanel admin create --email <email> --password <password>` creates an admin user with Argon2id-hashed password in `panel.db`. `POST /api/auth/login` accepts `{email, password}`, validates credentials, creates a session row in `sessions` table, and returns a secure cookie (`HttpOnly`, `Secure`, `SameSite=Strict`). Auth middleware protects all `/api/*` routes except `POST /api/auth/login` and `GET /health`. Unauthenticated requests to protected routes return `401 Unauthorized`. User model has a `role` field (`admin` or `user`); RBAC middleware skeleton checks role (admin-only routes return `403` for non-admin users). `POST /api/auth/logout` invalidates the session (deletes row from `sessions` table) and clears the cookie. |
| SP1-4 | App Shell — UI             | M (3-4d) | SP1-3       | Design tokens defined as CSS custom properties in `:root` (`tokens.css`), with separate light (`light.css`) and dark (`dark.css`) theme maps implementing all semantic tokens from PRD 20.3 (`--bg-canvas`, `--bg-surface`, `--text-primary`, `--text-secondary`, `--border-subtle`, `--accent-primary`, `--state-success`, `--state-warning`, `--state-danger`, `--focus-ring`). Theme engine: `data-theme="light|dark"` attribute on `<html>`, toggle component persists preference to `localStorage`, respects `prefers-color-scheme` as fallback. App shell layout: left sidebar with 7 navigation items (Dashboard, Sites & Domains, Databases, Updates & Versions, Backup & Restore, Security & Audit, Settings), topbar (search placeholder, host status placeholder, theme toggle, account dropdown), scrollable content area. Login page with email/password form, client-side validation, error states for invalid credentials and network errors. Authenticated dashboard page displays "Welcome, {user}" placeholder. i18n configured with `i18next` + `react-i18next`: `en.json` with ~50 initial keys covering nav items, login form labels, common actions (Save, Cancel, Delete, Confirm, etc.), and placeholder strings. Responsive layout: sidebar visible on desktop (>=1024px), collapsible drawer with hamburger toggle on mobile/tablet. Fonts loaded: IBM Plex Sans (body text) and Space Grotesk (headings). |
| SP1-5 | Installer Phase 1          | M (3-4d) | SP1-2       | Pre-flight checks: script detects Debian 13 (`/etc/os-release`), rejects non-Debian or older versions, validates clean state (no conflicting packages), checks minimum resources (2 CPU cores, 1 GB RAM, 10 GB free disk). Nginx installed from official Debian 13 repo via apt adapter. Panel binary copied to `/usr/local/bin/aipanel`. systemd unit installed at `/etc/systemd/system/aipanel.service` (`ExecStart=/usr/local/bin/aipanel serve`, `Restart=on-failure`, `After=network.target`). Basic Nginx adapter implemented: install, start, stop, status, health check (verify process running + port open) via Go `os/exec`. Post-install health check: panel responds on configured port (`GET /health` returns 200). Installation report written as JSON to `/var/log/aipanel/install-report.json` (timestamp, OS version, installed components, status per step). Full installation log written to `/var/log/aipanel/install.log`. Installer is runnable on a clean Debian 13 VM: panel starts, Nginx runs, `/health` returns 200, report file exists and is valid JSON. |

---

## 2. Dependency Diagram

```text
                    +-----------+
                    |  SP1-1    |
                    | Scaffolding|
                    +-----+-----+
                          |
                          v
                    +-----------+
                    |  SP1-2    |
                    | Platform  |
                    | Foundation|
                    +-----+-----+
                          |
                +---------+---------+
                |                   |
                v                   v
          +-----------+       +-----------+
          |  SP1-3    |       |  SP1-5    |
          | IAM/Auth  |       | Installer |
          +-----+-----+       +-----------+
                |
                v
          +-----------+
          |  SP1-4    |
          | App Shell |
          +-----------+
```

**Critical path:** SP1-1 --> SP1-2 --> SP1-3 --> SP1-4

SP1-5 (Installer Phase 1) runs in parallel with SP1-3 and SP1-4 since it only depends on SP1-2 (the platform foundation that produces the binary).

---

## 3. Sprint Risks and Mitigations

| #  | Risk                                                                                          | Impact | Mitigation                                                                                                                                                    |
|----|-----------------------------------------------------------------------------------------------|--------|---------------------------------------------------------------------------------------------------------------------------------------------------------------|
| R1 | **Critical path is 4 tasks deep (SP1-1 through SP1-4), leaving little slack for delays.**     | High   | SP1-1 (Scaffolding) is timeboxed to 2 days max. If it runs over, cut scope to bare minimum (Go module + React app + Makefile; defer CI and pre-commit hooks to day 1 of SP1-2). SP1-5 is parallelized off the critical path to absorb time. |
| R2 | **SQLite pure-Go driver (`modernc.org/sqlite`) may have edge cases with WAL mode or goose migrations on first integration.** | Medium | Spike the SQLite + goose + WAL integration on day 1 of SP1-2. If `modernc.org/sqlite` causes issues, fall back to `mattn/go-sqlite3` (CGO) for Sprint 1 and revisit in Sprint 2. Write an integration test that opens all 3 DB files and runs a migration before proceeding. |
| R3 | **Debian 13 (Trixie) may not yet be fully stable, causing installer failures on clean VMs.**  | Medium | Pin the Debian 13 image version used for testing (specific cloud image or Vagrant box SHA). Document any Debian 13-specific workarounds. If Debian 13 stable is not available at sprint start, test against Debian 13 testing/RC and gate final validation on the stable release. |

---

## 4. Definition of Done — Sprint 1

All of the following must be true for the sprint to be considered complete:

1. **All five tasks (SP1-1 through SP1-5) are merged to `main`.**
2. **CI is green on `main`:** lint (golangci-lint + eslint + prettier), all unit tests pass, `make build` succeeds and produces a single binary.
3. **Demo scenario passes on a clean Debian 13 VM:**
   - Run the installer on a fresh Debian 13 instance.
   - Panel binary starts via systemd and Nginx is running.
   - `GET /health` returns `200 OK`.
   - Create an admin account via `aipanel admin create`.
   - Log in through the web UI (email + password).
   - App shell renders: sidebar with 7 nav items, topbar with theme toggle, content area showing dashboard placeholder.
   - Toggle dark/light theme; preference persists across page reloads.
   - Log out; protected pages return to login screen.
   - Installation report JSON exists at `/var/log/aipanel/install-report.json`.
4. **No known critical or high-severity bugs remain open** against sprint 1 tasks.

---

## 5. Effort Summary

| Effort | Count | Estimated Days |
|--------|-------|----------------|
| S      | 1     | 1-2            |
| M      | 4     | 12-16          |
| **Total** | **5** | **13-18 (fits 2-week sprint with parallelization of SP1-3/SP1-5)** |

---

## 6. Notes

- **Parallelism opportunity:** SP1-3 (IAM) and SP1-5 (Installer) are independent of each other and can be worked on simultaneously by different developers once SP1-2 is complete. This reduces the effective timeline from 18 days sequential to approximately 14 days with 2 developers.
- **Frontend dev workflow:** During SP1-4, the frontend developer can work against a mocked API (MSW or static JSON) while the auth endpoints from SP1-3 are finalized. Integration testing happens in the last 1-2 days of SP1-4.
- **Scope guard:** No work outside the five defined tasks should be pulled into this sprint. Feature requests or bugs discovered during implementation should be logged as backlog items for Sprint 2.
- **Reference documents:** PRD (`docs/PRD-hosting-panel.md`), project structure (`docs/project-structure.md`).
