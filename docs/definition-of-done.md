# Definition of Done — aiPanel

> This document defines the acceptance criteria that must be met before any epic can be considered complete.
> It consists of a **Global DoD** (applies to every epic) and **Per-Epic DoD** sections with specific criteria.

---

## 1. Global Definition of Done

The following checklist is **mandatory for every epic** before it can be closed:

- [ ] **Code**: Written, reviewed by at least 1 reviewer, and merged to `main`
- [ ] **Tests — Unit**: >= 80% coverage for business logic (Go packages, React components with logic)
- [ ] **Tests — Integration**: Integration tests for all system adapters (Nginx, PHP-FPM, DB, systemd, nftables, fail2ban, apt)
- [ ] **Security**: `gosec` + `semgrep` pass with zero high/critical findings
- [ ] **UI (if applicable)**: Works correctly in both dark and light mode
- [ ] **UI (if applicable)**: WCAG AA contrast verified for all interactive and text elements
- [ ] **UI (if applicable)**: Responsive — functional on desktop, tablet, and mobile breakpoints
- [ ] **Audit**: All mutating operations logged to `audit.db` (append-only)
- [ ] **i18n**: All UI-facing text sourced from translation files (`locales/*.json`), zero hardcoded strings
- [ ] **Performance**: Meets the relevant NFR-PERF thresholds defined in the PRD (dashboard P95 <= 1.5 s, CRUD P95 <= 800 ms, provisioning P95 <= 2 min, panel overhead <= 10% CPU / <= 1.5 GB RAM)
- [ ] **Documentation**: API endpoints documented; configuration options described
- [ ] **CI**: All PR quality gates pass (lint, test, security scan, build)

---

## 2. Per-Epic Definition of Done

### 2.1 Installer

- [ ] Clean Debian 13 install completes in <= 20 minutes on the reference server
- [ ] Idempotency test passes — re-running the installer does not break existing configuration
- [ ] Resume after interruption works — installer can be restarted after a partial run and complete successfully
- [ ] Non-interactive mode works with all flags (suitable for CI/provisioning pipelines)
- [ ] Installation report is generated at the end, listing all installed/configured components
- [ ] Installer logs are written to a persistent location on the server
- [ ] DB engine selection works (MariaDB, PostgreSQL, or both) via flag/prompt
- [ ] Rollback/uninstall removes the panel and its configuration cleanly, leaving the system in a usable state
- [ ] Environment validation rejects unsupported OS or unmet prerequisites with a clear error message

---

### 2.2 IAM (Auth, RBAC, MFA)

- [ ] Admin and user roles are enforced on all API endpoints — no unprotected mutating routes
- [ ] Login with bcrypt or argon2 password verification works correctly
- [ ] Session management uses secure, HTTP-only, SameSite cookies
- [ ] Rate limiting on login endpoint: max 5 failed attempts per 15 minutes, then temporary lockout
- [ ] MFA (TOTP) enrollment flow works: QR code generation, secret storage, verification
- [ ] MFA verification is required on login when enrolled
- [ ] Privilege escalation test: a user-role account cannot access admin-only endpoints (returns 403)
- [ ] Password hashes and secrets are never exposed in API responses, logs, or error messages
- [ ] Session invalidation works on logout and password change

---

### 2.3 Hosting (Domains, Vhost, TLS, PHP Runtime)

- [ ] Create domain + vhost + TLS certificate completes in <= 2 minutes (P95)
- [ ] PHP version switching per site works without affecting other sites
- [ ] At least 2 PHP versions are available and assignable per site
- [ ] Let's Encrypt certificate auto-renewal works (via lego ACME library)
- [ ] Certificate expiry < 7 days triggers an alert
- [ ] Site isolation: user A cannot access user B's files, DB, or configuration
- [ ] Nginx vhost configuration is generated correctly from template and passes `nginx -t` validation
- [ ] Subdomain creation and mapping to directories works
- [ ] HTTP/2 is enabled by default; HTTP/3 is available as an option
- [ ] FastCGI cache can be toggled per site

---

### 2.4 Database Management

- [ ] Create and delete databases for both MariaDB and PostgreSQL
- [ ] Create and delete database users with appropriate grants for both engines
- [ ] Per-site database access isolation — user A's DB credentials cannot access user B's databases
- [ ] Database credentials are never exposed in logs, API responses, or error messages
- [ ] DB adapter works correctly for whichever engine(s) were selected during installation
- [ ] Dump and restore operations work for both engines

---

### 2.5 Backup / Restore

- [ ] Scheduled backup runs on time according to the configured schedule
- [ ] On-demand backup can be triggered from the UI and completes successfully
- [ ] Backup includes both files and database dumps
- [ ] Restore from backup produces a fully working site (files + DB + config)
- [ ] Dry-run restore test passes — used as a release gate
- [ ] RPO <= 24 hours verified (backup frequency ensures no more than 24h of data loss)
- [ ] RTO <= 60 minutes verified (restore completes within 60 min)
- [ ] Panel configuration backup runs at least once daily (NFR-REL-003)
- [ ] SQLite backup uses WAL checkpoint or Online Backup API — inconsistent file copy is not acceptable

---

### 2.6 Security Hardening (Firewall, fail2ban, SSH, Audit)

- [ ] nftables default ruleset is active post-install (canonical firewall for Debian 13)
- [ ] SSH hardening is applied: root login disabled, key-based authentication enforced
- [ ] fail2ban jails are active for SSH and the panel login endpoint
- [ ] Audit trail records all mutating operations with who/what/when
- [ ] Audit log is append-only — non-admin accounts cannot modify or delete audit entries
- [ ] Brute-force protection is active for the panel (rate limit + lockout)
- [ ] Processes run with least-privilege permissions (NFR-SEC-004)
- [ ] All packages are installed from official or signed sources
- [ ] Per-account resource limits (CPU/RAM/IO) are enforced

---

### 2.7 Monitoring / Health

- [ ] `/health` endpoint returns overall system health status
- [ ] `/health/ready` endpoint returns readiness status (suitable for orchestration probes)
- [ ] Dashboard shows real-time CPU, RAM, disk usage, and service status
- [ ] Alert fires when a managed service goes down (Nginx, PHP-FPM, DB)
- [ ] Alert fires when disk usage exceeds 90%
- [ ] Alert fires when any TLS certificate expires in < 7 days
- [ ] All panel operations produce structured JSON logs
- [ ] Automatic restart of critical services after failure (NFR-REL-001)
- [ ] Centralized log aggregation for panel and managed components (NFR-REL-002)

---

### 2.8 File Manager

- [ ] Browse directories within the user's site home directory
- [ ] Upload files (single and multiple)
- [ ] Download files
- [ ] Edit text files with a code editor featuring syntax highlighting (PHP, HTML, CSS, JS, JSON, YAML, .conf)
- [ ] Create new files and directories
- [ ] Delete files and directories
- [ ] Move/rename files and directories
- [ ] All operations are restricted to the user's home directory — no path traversal or escape possible
- [ ] chmod/chown changes work within the allowed policy boundaries
- [ ] File manager respects per-user isolation — user A cannot see or modify user B's files

---

### 2.9 Version Manager (EPIC-AL-01 through EPIC-AL-06)

- [ ] Feed sync detects new upstream versions within 24 hours of publication
- [ ] Feed sync runs automatically at least every 6 hours
- [ ] Cryptographic signatures and checksums are validated before any update is installed
- [ ] Policy engine computes the correct target version based on channel and policy mode (manual / auto-patch / auto-minor / auto-major)
- [ ] Pin/hold with TTL works — pinned components are not updated until the pin expires or is removed
- [ ] Preflight check blocks incompatible updates (PHP/DB/extension matrix, service health, resource availability)
- [ ] Canary rollout deploys to a limited group first; wave rollout expands after positive health checks
- [ ] Auto-rollback triggers on failure and completes within 15 minutes
- [ ] Rollback-update loop protection is in place (prevents infinite retry cycles)
- [ ] Compliance dashboard shows up-to-date / lagging / unsupported status for every managed component
- [ ] Alerts fire for delayed security updates, failed rollouts, and frequent rollbacks
- [ ] Maintenance windows are configurable per server and per account
- [ ] Full update lifecycle is recorded in the audit trail

---

### 2.10 UI (EPIC-UI-01 through EPIC-UI-06)

- [ ] Design tokens (colors, typography, spacing) are applied consistently via CSS variables with a single source of truth
- [ ] Theme engine uses `data-theme="light|dark"` on `<html>` and switches without page reload or re-login
- [ ] Theme preference priority: user setting > local storage fallback > `prefers-color-scheme`
- [ ] No FOUC (flash of unstyled content) — active theme is injected before app initialization
- [ ] App shell is functional: sidebar navigation, top bar (search, host status, theme toggle, account), content area
- [ ] All key screens render correctly in both dark and light mode: Dashboard, Sites & Domains, Databases, Updates & Versions, Backup & Restore, Security & Audit, Settings
- [ ] Visual regression tests pass for both themes at 3 breakpoints (mobile / tablet / desktop)
- [ ] Keyboard navigation works for all critical workflows (create domain, TLS, backup/restore, update)
- [ ] Focus visible state is present on all interactive elements
- [ ] ARIA semantics are applied to tables, forms, and status messages
- [ ] `prefers-reduced-motion` is respected — no decorative animations without function
- [ ] Mobile layout is functional: drawer navigation, sticky top bar, vertically stacked cards with alarm prioritization
- [ ] Loading states use skeleton placeholders for cards and tables
- [ ] Risky actions (Force Update, Rollback, Delete) require confirmation via modal
- [ ] Fonts load correctly: IBM Plex Sans (UI), Space Grotesk (headings/metrics)
