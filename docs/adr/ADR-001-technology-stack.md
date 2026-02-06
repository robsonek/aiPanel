# ADR-001: Technology Stack Selection

- **Status:** Accepted
- **Date:** 2026-02-06

## Context

aiPanel is a hosting panel targeting single-server Debian 13 deployments with security-first and performance-first design principles. The project needs a technology stack that satisfies several competing constraints:

1. **Single binary deployment** — the installer must produce a self-contained artifact with no runtime dependencies (no JVM, no Node.js, no interpreter).
2. **Low resource overhead** — the panel must stay under 10% CPU and 1.5 GB RAM in steady-state (NFR-PERF-004).
3. **Infrastructure tooling fit** — the backend must interact heavily with system services (Nginx, PHP-FPM, MariaDB/PostgreSQL, systemd, nftables, fail2ban, apt) through exec calls and config file generation.
4. **Native concurrency** — the job queue, backup scheduler, feed sync, and canary rollout orchestration all require concurrent execution without external process managers.
5. **Fast MVP development** — the project is open-source with no funding; development speed matters.
6. **Modern admin UI** — the panel requires a responsive SPA with dark/light theming, accessible components, i18n, and real-time state management.
7. **Pure Go SQLite driver** — cross-compilation and installer simplicity demand no CGO dependency.

## Decision

### Backend: Go 1.24+ with Chi router

- **Language:** Go 1.24+ for the entire backend, installer, and system adapters.
- **HTTP router:** Chi — lightweight, idiomatic, fully compatible with `net/http`, no framework lock-in.
- **Template engine:** `text/template` from the standard library for Nginx vhosts, PHP-FPM pools, nftables rulesets, and systemd units.
- **SQLite driver:** `modernc.org/sqlite` — pure Go implementation, zero CGO dependency.
- **Schema migrations:** goose.
- **TLS/ACME:** lego (Go ACME library used by Caddy and Traefik).

### Frontend: React 19 + TypeScript + Vite + TailwindCSS 4 + Shadcn/ui

- **Framework:** React 19 with TypeScript for type safety and ecosystem breadth.
- **Build tool:** Vite for fast HMR during development and optimized production builds.
- **Styling:** TailwindCSS 4 (utility-first, design token implementation via CSS custom properties).
- **Component library:** Shadcn/ui (built on Radix UI) for accessible primitives with ARIA, keyboard navigation, and focus management out of the box.
- **Server state:** TanStack Query for cache, refetch, and optimistic updates.
- **Routing:** TanStack Router for type-safe routes with code splitting.
- **i18n:** i18next + react-i18next with lazy-loaded JSON translation files.

### Build and deployment model

- Frontend is compiled by Vite into `web/dist/` and embedded into the Go binary via `//go:embed`.
- The final artifact is a single statically-linked binary with no external runtime dependencies.
- Node.js (Active LTS) and pnpm are used only at build time, never in production.

## Consequences

### Positive

- **Single binary** simplifies the installer, eliminates runtime dependency management, and makes rollback trivial (swap one file).
- **Go's goroutines** provide lightweight concurrency for the job queue, feed sync, health checks, and rollout orchestration without threads or external queue infrastructure.
- **Low memory footprint** of Go keeps steady-state RAM well within the 1.5 GB target even with hundreds of managed sites.
- **Chi's `net/http` compatibility** means any standard Go HTTP middleware works without adaptation; there is no framework-specific middleware interface to learn or maintain.
- **`modernc.org/sqlite`** eliminates the need for gcc/CGO in the build and cross-compile toolchain, which directly simplifies the installer and CI pipeline.
- **React 19 + Shadcn/ui** provides the largest ecosystem of admin UI components, accessibility primitives, and community support.
- **TailwindCSS 4** maps directly to the design token system defined in the UI Spec (semantic tokens for both themes).
- **Vite** delivers sub-second HMR and optimized tree-shaken bundles, keeping the embedded frontend small.
- **Go is the industry standard** for infrastructure tooling (Docker, Caddy, Terraform, Portainer, Traefik), which means proven patterns for system interaction and strong hiring/contribution signal.

### Negative

- **Go lacks generics maturity** compared to languages like Rust or TypeScript; some patterns (e.g., generic repository) require more boilerplate.
- **Two languages** (Go + TypeScript) require developers to be proficient in both, unlike a full-stack Node.js approach.
- **React 19 is relatively new;** some third-party libraries may lag behind on compatibility. Mitigation: Shadcn/ui and TanStack already support React 19.
- **`modernc.org/sqlite` is slower than `mattn/go-sqlite3`** (CGO-based) by roughly 2-3x in write-heavy benchmarks. Mitigation: the panel workload is read-heavy; write-intensive paths (audit, queue) are separated into dedicated SQLite files with WAL mode, and migration thresholds to PostgreSQL are defined (see ADR-002).

## Alternatives Considered

### Backend language alternatives

| Language | Strengths | Why rejected |
|----------|-----------|--------------|
| **Rust** | Best raw performance, memory safety guarantees, no GC pauses | Significantly slower development velocity for MVP. Complex async ecosystem (tokio). Steeper learning curve limits community contributions. The performance delta over Go is unnecessary for a hosting panel workload. |
| **Node.js (TypeScript)** | Full-stack TypeScript (shared types with frontend), large ecosystem, fast prototyping | Higher memory consumption (V8 heap), weaker fit for system-level operations (exec, file management), no single-binary without bundlers like `pkg`/`bun compile` which add complexity. Not the industry standard for infrastructure tools. |
| **Python** | Fastest prototyping, extensive libraries | Insufficient performance for NFR-PERF targets (P95 dashboard <= 1.5s at 200 sessions). GIL limits true concurrency. Requires interpreter and virtualenv in production, complicating the installer. |
| **PHP** | Good web ecosystem, familiar to hosting audience | Not a standard for infrastructure tooling. Requires FPM or Swoole for concurrency. No single-binary deployment. Would create a circular dependency (panel written in PHP managing PHP). |

### Go HTTP router: Chi vs Echo

| Criterion | Chi | Echo |
|-----------|-----|------|
| `net/http` compatibility | Full — handlers are `http.Handler`, middleware is `func(http.Handler) http.Handler` | Partial — uses its own `echo.Context` wrapper; standard middleware requires adaptation |
| Framework weight | Minimal — router + middleware chain only | Heavier — includes its own context, binder, renderer, validator |
| Long-term maintenance | Lower risk — tracks Go stdlib evolution naturally | Higher risk — framework-specific abstractions may diverge from stdlib |
| Performance | Comparable (both are fast enough; router is not the bottleneck) | Comparable |
| Community adoption for infra | Used in production by Terraform Cloud, go-chi ecosystem | Popular for web APIs, less common in infra tools |

**Decision:** Chi was chosen because its complete `net/http` compatibility eliminates a class of long-term maintenance problems. When Go's stdlib adds features (e.g., enhanced routing in Go 1.22+), Chi benefits immediately without framework adapter layers. Echo's custom `Context` creates framework lock-in that provides no meaningful benefit for this project.

### SQLite driver: modernc.org/sqlite vs mattn/go-sqlite3

| Criterion | `modernc.org/sqlite` | `mattn/go-sqlite3` |
|-----------|----------------------|---------------------|
| CGO required | No (pure Go) | Yes (requires gcc, CGO_ENABLED=1) |
| Cross-compilation | Trivial (`GOOS=linux GOARCH=amd64 go build`) | Requires cross-compiler toolchain for target OS/arch |
| Installer simplicity | No gcc dependency on target system | gcc must be available or binary must be pre-compiled for exact target |
| Performance (writes) | ~2-3x slower than CGO version | Fastest (direct C bindings) |
| Performance (reads) | Comparable | Slightly faster |
| CI build time | Standard Go build | Requires CGO setup in CI, slower builds |

**Decision:** `modernc.org/sqlite` was chosen because the panel's deployment model (single binary installed via `curl | bash` on a clean Debian 13) demands zero build-time dependencies on the target system. The 2-3x write performance difference is acceptable because: (1) the panel workload is read-dominant, (2) write-heavy paths are isolated in separate SQLite files (audit.db, queue.db) to prevent contention, and (3) hard migration thresholds to PostgreSQL are defined for when SQLite becomes a bottleneck (see ADR-002).
