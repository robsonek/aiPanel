# PRD: Hosting Panel (Security + Performance First)

## 1. Document Information
- Product name: aiPanel (working name)
- PRD version: 0.7 (draft)
- Date: 2026-02-06
- Status: MVP scope and architecture/UX decisions confirmed
- Target OS: Debian 13 (clean install)
- Product model: open source, free

## 2. Summary
The goal is to build a cPanel/DirectAdmin-class hosting panel that prioritizes:
1. Server and data security.
2. Website performance.
3. Simple server/service management through UI and API.

The panel must support installation on a clean Debian 13 host and automatically install all required dependencies and components.

## 3. Problem and Context
Existing hosting panels are often:
- operationally heavy and over-installed,
- not secure-by-default,
- weak in measurable performance guarantees,
- limited in auditing and change traceability.

aiPanel addresses these gaps with a secure-by-default and performance-by-default architecture.

## 4. Product Goals
## 4.1 Primary Goals
- Deliver secure web hosting on a single Debian 13 server.
- Reduce time to provision a new server and website.
- Maintain high performance for typical PHP and static workloads.

## 4.2 Measurable MVP Goals
- Install panel on clean Debian 13 in <= 20 minutes (reference server).
- Provision a new website in <= 2 minutes.
- Main dashboard load time <= 1.5 s (P95).
- Automatic TLS issuance and renewal for new domains.
- 100% of administrative mutating operations recorded in audit logs.

## 5. Scope
## 5.1 In-scope (MVP)
- One-shot installer for clean Debian 13:
  - environment validation,
  - dependency installation,
  - component configuration,
  - panel startup.
- Web service management:
  - domains/subdomains,
  - vhosts,
  - TLS (Let's Encrypt),
  - PHP runtime (multi-version + per-site assignment),
  - database engines (MariaDB, PostgreSQL, or both chosen during install).
- Account management:
  - admin and user roles,
  - RBAC.
- Security baseline:
  - SSH hardening,
  - firewall,
  - fail2ban,
  - per-user site isolation,
  - per-account limits (CPU/RAM/IO).
- Performance baseline:
  - Nginx as frontend,
  - static cache,
  - optional per-site FastCGI cache,
  - HTTP/2 by default (HTTP/3 optional).
- Backup and restore:
  - backup scheduling,
  - single-site/single-database restore.
- Monitoring and audit:
  - service status,
  - host metrics (CPU/RAM/disk),
  - panel operation audit log.
- Internal API (for UI + system automation) and web UI.
- UI/UX:
  - modern interface for desktop and mobile,
  - dark mode + light mode in v1,
  - manual theme toggle + per-user preference persistence.
- Application deployment:
  - Git-based deployment for sites (MVP).
- File Manager (built-in UI):
  - browse website directories,
  - upload/download files,
  - edit text/code files,
  - create/delete/move files and folders,
  - change permissions (chmod/chown),
  - operations restricted to the assigned site home directory.

## 5.2 Out-of-scope (MVP)
- Multi-node orchestration / HA cluster.
- Windows Server management.
- Full Kubernetes orchestration.
- Third-party plugin marketplace.
- Advanced billing (invoices, payments).
- Public stable API for third parties.
- Reseller role.
- DNS zone management (A/CNAME/MX/TXT).
- Mail server and webmail.

## 6. Users and Personas
- Server administrator:
  - requires full control, auditability, hardening, and fast diagnostics.
- Website owner / developer:
  - manages domains, databases, deployment, backups, and certificates.

## 7. Key Scenarios (Use Cases)
1. Admin installs panel on a fresh Debian 13 server using one installer flow.
2. User creates a new website with domain, TLS certificate, and database.
3. User switches PHP version for a specific site.
4. Admin restores a site after a failed deployment.
5. Admin reviews audit logs to see who changed what and when.

## 8. Functional Requirements
- FR-001: Installer must support clean Debian 13 and reject unmet prerequisites.
- FR-002: Installer must be idempotent (rerun does not break configuration).
- FR-003: Panel must manage domains, subdomains, and directory mappings.
- FR-004: Panel must automatically issue and renew TLS certificates.
- FR-005: Panel must support DB/user management for MariaDB and PostgreSQL based on install-time selection.
- FR-006: Panel must support at least 2 PHP versions with per-site assignment.
- FR-007: Panel must support backup and restore (files + DB).
- FR-008: Panel must provide RBAC and MFA for admin accounts.
- FR-009: Panel must log all mutating operations (audit trail).
- FR-010: Panel must provide token-based internal API for UI and automation (no public API SLA in MVP).
- FR-011: Panel must expose service status and basic host metrics.
- FR-012: Panel must provide secure panel component updates.
- FR-013: Panel must support configuring and enforcing per-account limits (CPU/RAM/IO).
- FR-014: Panel must support Git-based site deployment (pull + checkout + deploy hook).
- FR-015: Panel must include a Version Manager module for detecting new versions from official sources.
- FR-016: Panel must support per-component/app update policies: manual, auto-patch, auto-minor, auto-major (latest stable).
- FR-017: Every update must pass preflight checks (PHP/DB/extension compatibility, service health, free resources).
- FR-018: Before update, panel must create a restore point (files + DB + config) and support automatic rollback on failure.
- FR-019: Panel must support phased rollout (canary -> next waves) for hosted app updates.
- FR-020: Panel must report version compliance status (up-to-date/lagging/unsupported) and alert on delays.
- FR-021: Panel must support two artifact distribution modes: direct-upstream (default) and optional mirror/proxy.
- FR-022: Panel must verify cryptographic signatures and checksums before installing updates.
- FR-023: Panel must support version pin/hold with TTL and forced rollback to previous stable version.
- FR-024: Panel must support dark mode and light mode with manual switching.
- FR-025: Panel must persist per-user theme preference and use system preference on first login.
- FR-026: UI must be responsive (desktop/tablet/mobile) while preserving full admin functionality.
- FR-027: Panel must provide a design-token-based design system shared across dark and light themes.
- FR-028: Default panel language must be English (`en`).
- FR-029: Panel must support i18n from v1: all UI text comes from translation files (no hardcoded UI strings).
- FR-030: Each language must have a separate translation file (e.g., `en.json`, `pl.json`, `de.json`).
- FR-031: Panel must support per-user language switching with preference persistence.
- FR-032: Adding a new language must not require source code changes; only a new translation file.
- FR-033: Panel must include a built-in file manager in the UI.
- FR-034: File manager must support browsing, upload, download, edit, create, delete, and move for files/folders.
- FR-035: File manager must include syntax highlighting for common formats (PHP, HTML, CSS, JS, JSON, YAML, `.conf`).
- FR-036: File manager operations must be limited to the user's assigned site home directory (strict per-user isolation).
- FR-037: File manager must support permission changes (`chmod`/`chown`) within allowed policies.

## 9. Non-Functional Requirements
## 9.1 Security
- NFR-SEC-001: Security-by-default after install (firewall enabled, SSH hardening enabled).
- NFR-SEC-002: Passwords and secrets stored hashed/encrypted.
- NFR-SEC-003: Brute-force protection for panel login (rate limiting + temporary lockout).
- NFR-SEC-004: Least-privilege process permissions.
- NFR-SEC-005: Audit logs must be non-modifiable by non-privileged accounts.
- NFR-SEC-006: Security update process with SLA for critical CVEs <= 48h.

## 9.2 Performance
- NFR-PERF-001: Dashboard P95 <= 1.5 s at 200 concurrent panel sessions.
- NFR-PERF-002: Hosting CRUD operations P95 <= 800 ms (excluding async jobs).
- NFR-PERF-003: New site provisioning <= 2 minutes (P95).
- NFR-PERF-004: Panel overhead <= 10% CPU and <= 1.5 GB RAM steady-state (reference host).

## 9.3 Reliability and Operations
- NFR-REL-001: Automatic restart of critical services after failure.
- NFR-REL-002: Centralized logs for panel and managed components.
- NFR-REL-003: Daily backup of panel configuration.
- NFR-REL-004: RPO <= 24h, RTO <= 60 min (MVP target).

## 9.4 Version Management (Always Latest)
- NFR-VER-001: New-version catalog refresh at least every 6h.
- NFR-VER-002: New stable component version visible in panel <= 24h from upstream release.
- NFR-VER-003: Critical security updates must be deployable automatically <= 24h.
- NFR-VER-004: Any update must be rollback-capable <= 15 min after failure detection.
- NFR-VER-005: Update success rate (without rollback) >= 98% in production release cycles.

## 9.5 UX and Design System
- NFR-UX-001: Both themes (dark/light) must meet WCAG AA contrast on key screens.
- NFR-UX-002: Theme switching must not require re-login or full application reload.
- NFR-UX-003: Critical workflows (domain creation, TLS, backup/restore, update) must be clear in both themes.
- NFR-UX-004: UI must keep a consistent visual language based on design tokens and a component library.

## 10. Installer Requirements (Debian 13)
- INS-001: Installer must detect whether the system is clean and compatible (Debian 13).
- INS-002: Installer must install dependencies only from official repos or trusted signed sources.
- INS-003: Installer must generate a final installation report.
- INS-004: Installer must create rollback point (or provide documented rollback procedure).
- INS-005: Installer must support non-interactive mode (CI/provisioning).
- INS-006: Installer must write logs to a persistent server location.
- INS-007: Installer must support resume/re-run after interruption.
- INS-008: Installer must support DB engine selection: MariaDB, PostgreSQL, or both.

## 11. High-Level Architecture (MVP)
- Web UI (SPA) + backend API.
- Recommended backend architecture: modular monolith (single deployable process, clear domain boundaries).
- System operation orchestrator (job runner + queue).
- Service adapters:
  - Nginx,
  - PHP-FPM,
  - DB engine,
  - backup engine,
  - cert manager,
  - file manager adapter.
- IAM module (auth, RBAC, MFA).
- Audit module (append-only event log).
- Monitoring module (metrics + health checks).
- Version Manager module (feed sync, update policy, preflight, rollout, rollback, compliance reporting).
- UI Design System module (theme tokens, component library, dark/light theme engine).

## 12. Telemetry and Success Metrics
- Security:
  - failed login count,
  - blocked brute-force attempts,
  - time from CVE publication to patch deployment.
- Performance:
  - dashboard load P50/P95,
  - provisioning P95,
  - average TTFB for hosted apps (reference benchmark).
- Operational:
  - successful installation percentage,
  - MTTR for critical incidents,
  - successful backup/restore test rate.

## 13. Roadmap
1. Phase 0 - Discovery/Architecture (2-3 weeks):
   - finalize technology stack,
   - threat model,
   - baseline benchmark,
   - Version Manager specification.
2. Phase 1 - Core Installer + Hosting Basics (4-6 weeks):
   - Debian 13 installer,
   - domains, TLS, vhost, DB (MariaDB/PostgreSQL), basic UI/internal API,
   - design system v1 + dark/light mode.
3. Phase 2 - Security Hardening + Audit (3-4 weeks):
   - MFA, RBAC, fail2ban, firewall profile, audit trail, per-account limits,
   - signed package sources and update integrity validation.
4. Phase 3 - Performance + Backup/Restore (3-4 weeks):
   - cache policy, tuning, backup scheduler, restore workflow, Git-based deployment,
   - preflight checks + rollback automation for updates.
5. Phase 4 - MVP Stabilization (2-3 weeks):
   - performance tests, security review, hardened release,
   - phased rollout (canary/wave) and version compliance dashboard.

## 14. MVP Acceptance Criteria
- Debian 13 server can be prepared with one installation flow.
- User can create and configure a site (domain + TLS + DB + PHP) via UI.
- Installer supports MariaDB, PostgreSQL, or both.
- Panel enforces baseline security and audits operations.
- Panel enforces per-account CPU/RAM/IO limits.
- Panel supports Git-based website deployment.
- Panel runs component/app updates in latest-stable mode with preflight and rollback.
- Admin has update policies (manual/auto-patch/auto-minor/auto-major) and maintenance windows.
- Panel works in dark and light mode with per-user preference persistence.
- Key admin workflows pass readability/contrast checks in both themes.
- Built-in file manager works with strict per-user directory isolation.
- Backup and restore work for at least one failure scenario.
- Required P95 metrics for panel and provisioning are met in QA.

## 15. Risks and Mitigations
- Risk: MVP scope too broad.
  - Mitigation: strict out-of-scope boundaries and phased delivery.
- Risk: conflict with manual system edits.
  - Mitigation: "managed by panel" policy + drift detection warnings.
- Risk: performance regressions after updates.
  - Mitigation: benchmark smoke tests in release pipeline.
- Risk: vulnerabilities in dependencies.
  - Mitigation: SBOM + periodic scans + fast patch policy.
- Risk: added complexity from supporting two DB engines.
  - Mitigation: shared DB adapter interface + full test matrix.
- Risk: open-source model without funding slows development.
  - Mitigation: clear roadmap, governance, and contribution policy.
- Risk: automated updates can break compatibility.
  - Mitigation: preflight, canary/wave rollout, mandatory health checks, auto-rollback.
- Risk: inconsistent UX between dark and light mode.
  - Mitigation: design tokens + visual regression in both themes.

## 16. Product Decisions (MVP)
1. Roles: admin + user only (no reseller in MVP).
2. DB engines: installer choice of MariaDB, PostgreSQL, or both.
3. API: internal API only in MVP (no public API).
4. Limits: required per-account CPU/RAM/IO limits.
5. Deployment: Git-based deployment in MVP.
6. Model: open source, free.
7. Backend architecture: modular monolith (final decision).
8. Updates: latest-stable-by-default policy for panel-managed components and apps.
9. UX: modern MVP interface with full dark/light support.
10. Visual direction: "Security Control Room".
11. Language: English default, i18n-ready architecture from MVP.

## 17. Backend Architecture: Final Decision
- Selected architecture: modular monolith.
- Reason: best tradeoff between delivery speed and strict domain boundaries.
- Target module boundaries:
  - Installer,
  - IAM,
  - Hosting (domains/vhost/runtime),
  - Database Management,
  - Backup/Restore,
  - Audit/Compliance,
  - Version Manager,
  - Monitoring/Health,
  - File Manager.
- Design rule: modules communicate through explicit application contracts (no internal-layer shortcuts).

## 17.1 Technology Stack: Final Decision
### Backend: Go (Golang) 1.24+
- Rationale: best balance of delivery speed, performance, and infrastructure domain fit.
- Single binary simplifies installer and deployment (INS-001..INS-007).
- Low RAM footprint supports NFR-PERF-004.
- Native concurrency (goroutines) for job runner and system operations.
- Industry-proven for infrastructure tools (Docker, Caddy, Terraform, Portainer).
- HTTP router: Chi (lightweight, idiomatic, `net/http`-compatible).
- Config template rendering: `text/template` (stdlib).

### Frontend: React 19 + TypeScript + Vite
- React 19 + TypeScript for typed UI development and ecosystem maturity.
- Vite for fast HMR and production builds.
- TailwindCSS 4 for design-token implementation from UI Spec 20.3.
- Shadcn/ui (Radix-based) for accessible primitives (ARIA, keyboard nav, focus management).
  - Note: WCAG AA still depends on implementation and testing.
- TanStack Query for server state/caching.
- TanStack Router for type-safe routing and code splitting.
- i18next + react-i18next for localization:
  - one JSON file per language,
  - feature namespaces,
  - lazy-loaded translations,
  - variable interpolation and pluralization,
  - new language can be added without source changes.

### Internal panel database: SQLite
- Stores panel metadata only (configuration, audit, sessions, version state), not hosted app data.
- No separate DB service required.
- Single-file model enables simple config backup.
- Driver: `modernc.org/sqlite` (pure Go, no CGO).
- Schema migrations: `goose`.
- Hard migration thresholds to PostgreSQL:
  - queue throughput > 500 jobs/min sustained,
  - audit log > 10 GB,
  - write contention causes P95 write latency > 200 ms,
  - concurrent panel sessions > 100.
- Repository-pattern separation is mandatory so SQLite -> PostgreSQL migration does not impact business logic.
- Split SQLite files at higher traffic (before PostgreSQL migration if needed):
  - `panel.db` for config/sessions/version state,
  - `audit.db` for append-only audit events,
  - `queue.db` for job queue.

### SQLite backup in WAL mode
- SQLite runs in WAL mode for read/write concurrency.
- Backup must run `PRAGMA wal_checkpoint(TRUNCATE)` before snapshot to guarantee consistency.
- Alternative: SQLite Online Backup API (`sqlite3_backup`) for atomic copy without service stop.
- Inconsistent backups (raw file copy without checkpoint/backup API) are not allowed.

### Job Queue
- MVP: built-in SQLite-based queue.
- Post-MVP alternatives if scale requires: Asynq (Redis) or River (PostgreSQL).
- Used for async operations (provisioning, backup, updates, cert renewal).

### TLS Certificates
- ACME library: `lego`.
- Supported challenges: HTTP-01 and DNS-01.
- Automatic certificate renewal (FR-004).

### System Adapters (Go)
- Dedicated adapters per managed component:
  - Nginx (vhost generation/reload/status),
  - PHP-FPM (per-site pools/restart/version switching),
  - MariaDB/PostgreSQL (DB/user/grants/dump/restore),
  - systemd (service lifecycle),
  - nftables (canonical firewall layer on Debian 13),
  - fail2ban,
  - apt (package install and system updates).
- Adapter communication via explicit interfaces per module contract.

### Stack Diagram
```text
┌─────────────────────────────────────────┐
│              Frontend (SPA)             │
│  React 19 + TypeScript + Vite           │
│  TailwindCSS 4 + Shadcn/ui             │
├─────────────────────────────────────────┤
│            Backend API (Go)             │
│  Chi + modular monolith                 │
├──────────┬──────────┬───────────────────┤
│ SQLite   │ Job Queue│ Lego (ACME/TLS)   │
│ (panel)  │ (built-in)│                  │
├──────────┴──────────┴───────────────────┤
│         System Adapters (Go)            │
│  Nginx · PHP-FPM · MariaDB/PgSQL       │
│  systemd · nftables · fail2ban · apt   │
└─────────────────────────────────────────┘
```

### Build Tooling Version Policy
- Go: latest stable patch (e.g. 1.24.x); patch updates immediately, minor after compatibility validation.
- Node.js: Active LTS (build-time only, not production runtime).
- pnpm: committed lockfile (`pnpm-lock.yaml`) for reproducible builds.
- Vite/Tailwind/React: pinned in `package.json` (minor-level constraints), upgraded through tested PRs.
- Goal: same commit produces identical artifact.

### Rejected Alternatives
- Rust: excellent performance/memory safety, but slower MVP velocity.
- Node.js/TypeScript backend: strong DX, weaker for system-level operations, higher RAM.
- Python: very fast prototyping, weaker performance for target NFRs.
- PHP backend: strong web ecosystem, weaker fit for infra tooling.

## 18. Always Latest Plan
## 18.1 Update Scope
- Panel components: backend, UI, job runner, migrations.
- Managed infrastructure components: Nginx, PHP-FPM, MariaDB, PostgreSQL, cert manager, backup tools.
- Managed applications deployed by panel (Git + recipe).

## 18.2 Version Model and Channels
- Default channel: latest stable.
- Optional channels:
  - conservative (delayed major adoption),
  - rapid (fastest stable adoption).
- Sources: official signed repositories or verified vendor artifacts only.

## 18.3 Update Pipeline
1. Feed sync every 6h.
2. Compatibility evaluation (PHP/DB/extensions, deprecations).
3. Per-instance preflight (health/resources/config conflicts).
4. Backup/snapshot before change.
5. Canary rollout.
6. Wave rollout.
7. Post-check (endpoints/logs/perf).
8. Auto-rollback on thresholds breach.

## 18.4 Automation Rules
- Security patches: auto, target <= 24h.
- Minor releases: auto (with canary + rollback).
- Major releases: auto only if matrix + canary pass; otherwise admin approval required.
- Maintenance windows configurable per server and per account.

## 18.5 Visibility and Enforcement
- Version compliance dashboard:
  - up-to-date,
  - lagging,
  - unsupported.
- Alerts:
  - delayed security updates,
  - failed rollout,
  - frequent rollback for same component.
- Weekly report:
  - current-instance percentage,
  - average upstream-to-deployed lead time,
  - rollback count and root causes.

## 18.6 Artifact and Source Strategy (Do we host versions ourselves?)
- Default: we do not need to host all external component versions ourselves.
- Recommended hybrid model:
  - aiPanel-owned components: yes, maintain signed aiPanel artifact repository.
  - system/runtime components: use official signed upstream sources.
  - local cache on managed server: keep latest artifacts + at least one previous version for rollback.
  - central mirror/proxy: optional for scale, poor connectivity, or offline environments.
- MVP conclusion: direct-upstream + local cache is sufficient; central mirror after MVP.

## 18.7 Update Installation Flow
1. Version Manager detects new release.
2. Policy Engine calculates target version.
3. Scheduler validates maintenance window and pin/hold status.
4. Preflight validates compatibility, health, and resources.
5. Downloader fetches artifacts into staging and validates signature/checksum.
6. Snapshot engine creates rollback point.
7. Update adapters install updates:
   - system packages via apt,
   - aiPanel components via aiPanel artifact repo,
   - managed apps via recipe update.
8. Post-check runs smoke tests and metrics validation.
9. On failure, auto-rollback is triggered and incident flagged.
10. Audit + telemetry record full execution trace and outcome.

## 18.8 Version State Management Model
- Every component has:
  - Desired State (channel, target version, policy, maintenance window, pin/hold),
  - Observed State (installed version, compliance status, last attempt, last rollback).
- Minimum version record fields:
  - `component_id`,
  - `installed_version`,
  - `target_version`,
  - `channel`,
  - `policy_mode`,
  - `pinned_until`,
  - `rollback_version`,
  - `update_status`,
  - `last_check_at`,
  - `last_update_at`.
- Admin operations:
  - pin version,
  - unpin,
  - force update,
  - rollback to previous stable.

## 18.9 Hosted Application Update Policy
- Type A: Managed Apps (recipe + panel adapter):
  - panel updates app and dependencies according to policy.
- Type B: Custom Git Apps:
  - panel updates runtime (PHP/Nginx/DB), app code stays under user repo control.
  - optional dependency update signaling without forced code updates.
- Both types require canary + wave + rollback safeguards.

## 18.10 Update Governance
- Security patching: daily execution cycle.
- Minor/major updates: based on maintenance windows and health signals.
- Hard stop thresholds:
  - error rate increase above threshold,
  - P95 degradation above SLA threshold,
  - failed health check of critical component.
- Every rollout must have explicit rollback plan and named owner.

## 19. Always Latest Implementation Backlog
## 19.1 EPIC-AL-01: Version Catalog and Trusted Sources
- Goal: trustworthy version discovery and authenticity validation.
- Tasks:
  - feed sync every 6h,
  - version metadata parsers,
  - metadata storage,
  - signature/checksum validation.
- DoD: new version detected, verified, and visible in panel <= 24h.

## 19.2 EPIC-AL-02: Policy Engine and Version Model
- Goal: automatic target-version decisions.
- Tasks:
  - Desired/Observed state model,
  - policy engine,
  - pin/hold with TTL,
  - maintenance windows.
- DoD: every component has computed target version and decision state.

## 19.3 EPIC-AL-03: Preflight and Compatibility Matrix
- Goal: reduce failed update risk.
- Tasks:
  - PHP/DB/extension compatibility matrix,
  - resource/service health checks,
  - update dry-run plan.
- DoD: update blocked when preflight fails.

## 19.4 EPIC-AL-04: Update Adapters
- Goal: unified update execution across component types.
- Tasks:
  - apt adapter,
  - aiPanel artifact adapter,
  - managed app recipe adapter,
  - standardized error codes.
- DoD: all supported components update through shared adapter interface.

## 19.5 EPIC-AL-05: Rollout and Rollback
- Goal: safe incremental deployment.
- Tasks:
  - canary -> wave orchestration,
  - snapshot and restore automation,
  - auto-rollback thresholds,
  - rollback/update loop protection.
- DoD: failed update rolls back <= 15 min.

## 19.6 EPIC-AL-06: Compliance Dashboard and Alerting
- Goal: full compliance visibility and risk transparency.
- Tasks:
  - compliance dashboard,
  - security-delay and rollout-failure alerts,
  - weekly KPI reporting.
- DoD: admin can see compliance state and root causes of delays.

## 19.7 EPIC-AL-07: Mirror/Proxy Mode (Post-MVP)
- Goal: large-scale and constrained-network support.
- Tasks:
  - mirror/proxy integration,
  - artifact retention policies,
  - direct-upstream fallback.
- DoD: updates can run without direct per-host upstream fetches.

## 19.8 Delivery Sequence
1. EPIC-AL-01
2. EPIC-AL-02
3. EPIC-AL-03
4. EPIC-AL-04
5. EPIC-AL-05
6. EPIC-AL-06
7. EPIC-AL-07 (post-MVP)

## 20. UI Spec v1 (Dark + Light, Modern)
## 20.1 Visual Concept
- Style: "Security Control Room".
- Characteristics:
  - high readability of operational data,
  - explicit state signaling (OK/Warn/Critical),
  - modern aesthetic without marketing clutter.
- Priority: admin can act on critical events in 1-2 clicks.

## 20.2 Information Architecture and Layout
- Desktop layout:
  - left navigation sidebar,
  - top contextual bar (search, host status, theme toggle, account),
  - workspace based on card grid + operational tables.
- Mobile layout:
  - drawer navigation,
  - sticky top bar with critical actions,
  - vertical cards with alert prioritization.
- Navigation modules v1:
  - Dashboard,
  - Sites & Domains,
  - Databases,
  - Updates & Versions,
  - Backup & Restore,
  - Security & Audit,
  - File Manager,
  - Settings.

## 20.3 Design Tokens and Color Palette
- Rule: one semantic token set, two theme maps (light/dark).
- Semantic tokens:
  - `--bg-canvas`,
  - `--bg-surface`,
  - `--text-primary`,
  - `--text-secondary`,
  - `--border-subtle`,
  - `--accent-primary`,
  - `--state-success`,
  - `--state-warning`,
  - `--state-danger`,
  - `--focus-ring`.
- Light mode direction:
  - light graphite-white background,
  - cool cyan/blue accent,
  - high-contrast red for critical states.
- Dark mode direction:
  - deep navy/graphite background,
  - cyan/teal accent with high readability,
  - subtle layered surfaces for depth.

## 20.4 Typography and Icons
- UI font: `IBM Plex Sans`.
- Heading/metrics font: `Space Grotesk`.
- Type scale:
  - H1: 32/40,
  - H2: 24/32,
  - H3: 18/26,
  - Body: 14/22,
  - Meta: 12/18.
- Icons: line-based style, consistent stroke weight, dual-theme variants.

## 20.5 Base Components v1
- App shell (sidebar, topbar, content).
- Metric card (status, trend, action).
- Operational table (sort/filter/bulk actions/status badges).
- Admin forms:
  - inline validation,
  - security hints,
  - advanced sections.
- Audit timeline.
- Risk-action confirmation modals.
- Priority toasts and alerts.

## 20.6 Motion and Microinteractions
- Animation timing:
  - fast: 120-160 ms,
  - standard: 180-240 ms.
- Rules:
  - no decorative-only animation,
  - motion must support orientation and state change,
  - support `prefers-reduced-motion`.
- Required animations:
  - subtle page reveal,
  - loading skeletons for cards/tables,
  - success/error operation feedback.

## 20.7 Accessibility and Readability
- WCAG AA minimum contrast on key screens.
- Visible focus for all interactive elements.
- Full keyboard support for critical workflows.
- ARIA semantics for tables/forms/status feedback.
- Both themes tested as separate QA cases.

## 20.8 Key Screens v1
- Dashboard:
  - server health score,
  - service status (Nginx/PHP/DB),
  - security alerts + update compliance.
- Updates & Versions:
  - up-to-date/lagging/unsupported status,
  - rollout plan + rollback history,
  - quick actions: pin/unpin/force update.
- Security & Audit:
  - event timeline,
  - login attempts,
  - configuration changes with actor identity.
- Sites & Domains:
  - site list,
  - TLS status,
  - runtime (PHP), limits, deployment.
- Backup & Restore:
  - schedules,
  - restore points,
  - restore flow with impact validation.
- File Manager:
  - tree/list views,
  - built-in code editor,
  - safe file operations inside assigned directory.

## 20.9 Technical UI Implementation
- Theme engine:
  - CSS variables at root,
  - `data-theme="light|dark"` on `html`,
  - single source of truth token mapping.
- Theme preference source priority:
  - user profile setting in panel,
  - local fallback,
  - `prefers-color-scheme`.
- FOUC minimization:
  - apply active theme before app initialization.
- Tests:
  - visual snapshots for both themes,
  - contrast and focus-state tests,
  - responsive tests (desktop/tablet/mobile).

## 20.10 UI Backlog (MVP)
1. EPIC-UI-01: Design Tokens and Theme Engine.
2. EPIC-UI-02: App Shell (sidebar/topbar/routing shell).
3. EPIC-UI-03: Operational component library.
4. EPIC-UI-04: Dashboard + critical screens (Updates/Security/Backup/File Manager).
5. EPIC-UI-05: Accessibility QA + dual-theme visual regression.
6. EPIC-UI-06: Mobile UX polish and render performance.

## 21. Low-Fi Spec for Key Screens (v1)
## 21.1 Screen: Dashboard
- Screen goal:
  - show server/service health in 5 seconds,
  - route admin to critical actions in max 2 clicks.

- Desktop low-fi:
```text
+--------------------------------------------------------------------------------------+
| TOPBAR: [Search] [Host Status: OK/WARN/CRIT] [Theme Toggle] [User]                  |
+----------------------+---------------------------------------------------------------+
| SIDEBAR              | PAGE: DASHBOARD                                               |
| - Dashboard          | +-------------------+ +-------------------+ +--------------+ |
| - Sites & Domains    | | Health Score      | | CPU/RAM/Disk      | | Alerts       | |
| - Databases          | | 92 / 100          | | CPU 43% RAM 61%   | | 2 WARN       | |
| - Updates & Versions | +-------------------+ +-------------------+ +--------------+ |
| - Backup & Restore   | +-----------------------------------------------------------+ |
| - Security & Audit   | | Services: Nginx OK | PHP-FPM OK | MariaDB WARN | LE OK   | |
| - File Manager       | +-----------------------------------------------------------+ |
| - Settings           | +---------------------------+ +----------------------------+  |
|                      | | Latest Deployments        | | Backup Status             |  |
|                      | | #site-a success           | | Last backup 02:14 OK      |  |
|                      | | #site-b failed (view)     | | Next: 03:00               |  |
|                      | +---------------------------+ +----------------------------+  |
+----------------------+---------------------------------------------------------------+
```

- Mobile low-fi:
```text
+--------------------------------------+
| TOPBAR [Menu] Dashboard [Theme]      |
+--------------------------------------+
| Health Score 92/100                  |
| CPU 43% | RAM 61% | Disk 58%         |
+--------------------------------------+
| Alerts (2) [View all]                |
| - MariaDB latency high               |
| - Update pending security patch      |
+--------------------------------------+
| Services                             |
| Nginx OK | PHP-FPM OK | DB WARN      |
+--------------------------------------+
| Backup Status                        |
| Last: OK 02:14 | Next: 03:00         |
+--------------------------------------+
```

- Main actions:
  - `View Alert`,
  - `Restart Service`,
  - `Open Update Plan`,
  - `Run Backup Now`.

- States:
  - loading: card skeletons + trend placeholders,
  - empty: onboarding "create your first website",
  - error: fallback card with `Retry`.

- Acceptance criteria:
  - key metrics and service status visible above fold on desktop,
  - mobile first screen prioritizes alerts and server health,
  - dark and light modes keep identical information hierarchy.

## 21.2 Screen: Updates and Versions
- Screen goal:
  - control version compliance and risk,
  - run safe updates (canary/wave/rollback).

- Desktop low-fi:
```text
+--------------------------------------------------------------------------------------+
| TOPBAR: [Search] [Global Compliance: 84%] [Theme Toggle] [User]                     |
+----------------------+---------------------------------------------------------------+
| SIDEBAR              | PAGE: UPDATES & VERSIONS                                      |
| ...                  | +-----------------------------------------------------------+ |
|                      | | Filters: [Component] [Status] [Channel] [Policy]        | |
|                      | +-----------------------------------------------------------+ |
|                      | +-----------------------------------------------------------+ |
|                      | | Component | Installed | Target | Status   | Actions      | |
|                      | | Nginx     | 1.25.3    | 1.25.5 | Lagging  | Pin Update   | |
|                      | | PHP 8.3   | 8.3.5     | 8.3.7  | Pending  | Force Update | |
|                      | | MariaDB   | 11.3.1    | 11.3.1 | UpToDate | -            | |
|                      | +-----------------------------------------------------------+ |
|                      | +---------------------------+ +----------------------------+  |
|                      | | Rollout Plan              | | Rollback History           |  |
|                      | | Stage 1: canary (10%)     | | 2026-02-02 PHP rollback    |  |
|                      | | Stage 2: wave (50%)       | | cause: error-rate spike    |  |
|                      | +---------------------------+ +----------------------------+  |
+----------------------+---------------------------------------------------------------+
```

- Mobile low-fi:
```text
+--------------------------------------+
| TOPBAR [Menu] Updates [Theme]        |
+--------------------------------------+
| Compliance 84%                       |
| [Filter] [Policy] [Channel]          |
+--------------------------------------+
| Nginx                                |
| Installed 1.25.3 -> Target 1.25.5    |
| Status: Lagging                      |
| [Pin] [Update] [Details]             |
+--------------------------------------+
| PHP 8.3                              |
| Installed 8.3.5 -> Target 8.3.7      |
| Status: Pending                      |
| [Force Update] [Details]             |
+--------------------------------------+
| Rollout: Canary 10% -> Wave 50%      |
| [Open plan] [Rollback history]       |
+--------------------------------------+
```

- Main actions:
  - `Pin/Unpin`,
  - `Force Update`,
  - `Approve Major`,
  - `Rollback`.

- States:
  - loading: table + rollout panel skeletons,
  - empty: "everything up-to-date" + last check timestamp,
  - error: "feed error" + `Retry feed sync`.

- Acceptance criteria:
  - admin can inspect installed/target/status/policy per component,
  - rollout plan and rollback history visible from one screen,
  - risky actions require confirmation modal.

## 21.3 Screen: Security and Audit
- Screen goal:
  - quickly detect incidents and config changes,
  - provide explicit who/what/when traceability.

- Desktop low-fi:
```text
+--------------------------------------------------------------------------------------+
| TOPBAR: [Search] [Security Level: Elevated] [Theme Toggle] [User]                   |
+----------------------+---------------------------------------------------------------+
| SIDEBAR              | PAGE: SECURITY & AUDIT                                        |
| ...                  | +------------------------+ +-------------------------------+  |
|                      | | Threat Summary         | | Login Activity                |  |
|                      | | Brute-force blocked:12 | | Failed logins: 38 (24h)      |  |
|                      | | CVE pending: 1         | | MFA bypass: 0                |  |
|                      | +------------------------+ +-------------------------------+  |
|                      | +-----------------------------------------------------------+ |
|                      | | AUDIT TIMELINE                                            | |
|                      | | 12:05 user:admin changed php.ini (site-a) [View diff]    | |
|                      | | 11:43 system auto-applied security patch [Details]        | |
|                      | | 10:18 user:ops executed rollback nginx [Reason]           | |
|                      | +-----------------------------------------------------------+ |
|                      | +-----------------------------------------------------------+ |
|                      | | Filters: [User] [Action] [Severity] [Date range]         | |
|                      | +-----------------------------------------------------------+ |
+----------------------+---------------------------------------------------------------+
```

- Mobile low-fi:
```text
+--------------------------------------+
| TOPBAR [Menu] Security [Theme]       |
+--------------------------------------+
| Threat Summary                       |
| Brute-force blocked: 12              |
| CVE pending: 1                       |
+--------------------------------------+
| Login Activity                       |
| Failed logins: 38 / 24h              |
| MFA bypass: 0                        |
+--------------------------------------+
| Audit Timeline                       |
| 12:05 admin changed php.ini          |
| [View diff]                          |
| 11:43 system patched component       |
| [Details]                            |
+--------------------------------------+
```

- Main actions:
  - `View Diff`,
  - `Export Audit`,
  - `Filter by Severity`,
  - `Open Incident`.

- States:
  - loading: timeline + summary skeleton,
  - empty: "no events for selected period",
  - error: "audit service unavailable" + `Retry`.

- Acceptance criteria:
  - timeline allows one-click drilldown to event detail,
  - filters support user/action/date narrowing,
  - audit export supports date range and event type selection.

## 21.4 Shared Rules for All 3 Screens
- all critical buttons must support `disabled/loading/success/error` states,
- fixed layout slot for system alerts (no layout shift),
- full dark/light consistency through shared semantic tokens,
- visual snapshot tests in both themes and all 3 breakpoints (mobile/tablet/desktop).
