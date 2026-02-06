# Testing Strategy — aiPanel

## 1. Goals and Principles

### Core principles

- **Security-first testing.** Every feature touching auth, RBAC, MFA, firewall rules, or file access gets dedicated security test cases before merge.
- **Shift-left.** Catch defects at the lowest cost: static analysis and unit tests run on every save/commit; integration tests on every PR; E2E and security scans nightly and on release.
- **Fail-fast.** CI pipelines abort on the first critical failure. Flaky tests are quarantined, not skipped.
- **Reproducibility.** Tests must produce identical results on any machine given the same commit. No reliance on external network services in unit/integration tests.
- **Test isolation.** Each test creates and tears down its own state. No shared mutable fixtures across test cases.

---

## 2. Test Pyramid

```
          ┌──────────┐
          │   E2E    │  Playwright on Debian 13 VM
         ─┤  (few)   ├─
        ┌──┴──────────┴──┐
        │  Integration   │  Go httptest + real SQLite / testcontainers
       ─┤  (moderate)    ├─
      ┌──┴────────────────┴──┐
      │       Unit tests     │  Go testing+testify / Vitest+RTL
      │       (many)         │
      └──────────────────────┘
```

### 2.1 Unit Tests

| Layer | Tool | Notes |
|-------|------|-------|
| Go business logic | `testing` + `github.com/stretchr/testify` | Table-driven tests, `assert`/`require` |
| Go adapters (interfaces) | `testing` + `github.com/stretchr/testify/mock` | Mock system interfaces, never call real binaries |
| React components | `vitest` + `@testing-library/react` | jsdom environment, test behavior not implementation |
| React hooks / stores | `vitest` + `@testing-library/react` `renderHook` | Tanstack Query wrapper with `QueryClientProvider` |
| i18n keys | `vitest` | Validate all keys in `en.json` exist, no orphaned keys |

**Commands:**
```bash
# Go unit tests (exclude integration tags)
go test ./... -short -count=1 -race

# Frontend unit tests
pnpm vitest run --reporter=verbose
```

### 2.2 Integration Tests

| Scope | Tool | Notes |
|-------|------|-------|
| API endpoints | Go `net/http/httptest` + real Chi router | Full request/response cycle, real middleware chain |
| Database (SQLite) | Real `modernc.org/sqlite` in-memory or temp file | Run goose migrations, test repository layer against real SQL |
| DB adapters (MariaDB/PostgreSQL) | `testcontainers-go` | Spin up MariaDB/PgSQL containers, test create/drop/grant flows |
| Job queue | Real SQLite queue with test harness | Enqueue, dequeue, retry, dead-letter scenarios |
| API auth flow | `httptest` + test JWT/session tokens | Validate RBAC enforcement, MFA challenge flow |

**Commands:**
```bash
# Go integration tests (tagged)
go test ./... -tags=integration -count=1 -race -timeout=5m

# With testcontainers (requires Docker)
go test ./... -tags=integration,docker -count=1 -timeout=10m
```

### 2.3 E2E Tests

| Scope | Tool | Notes |
|-------|------|-------|
| Critical workflows | Playwright (TypeScript) | Runs against real aiPanel on Debian 13 VM/container |
| Cross-browser | Playwright Chromium + Firefox | WebKit optional post-MVP |
| Accessibility | `@axe-core/playwright` | Automated WCAG AA checks per page |

**Target workflows:**
- Installer: clean install on Debian 13 completes successfully
- Site provisioning: create domain + TLS + DB + PHP runtime
- Backup & restore: trigger backup, destroy site, restore, verify
- Login + MFA: login flow, MFA enrollment, MFA challenge
- RBAC: admin vs user permission boundaries
- File Manager: upload, edit, delete, permission changes within site boundary
- Theme: dark/light toggle persists, no FOUC

**Commands:**
```bash
pnpm playwright test --project=chromium
pnpm playwright test --project=firefox
```

### 2.4 Security Tests

| Tool | Target | Stage |
|------|--------|-------|
| `gosec` | Go source — detect hardcoded secrets, SQL injection, path traversal | Pre-commit + PR |
| `govulncheck` | Go dependencies — known CVEs | PR + nightly |
| `npm audit` / `pnpm audit` | Frontend dependencies | PR + nightly |
| `semgrep` (SAST) | Custom rules for exec calls, template injection, RBAC bypass | PR |
| `trivy` | Container image scanning (if distributing Docker image) | Release |

**Commands:**
```bash
gosec ./...
govulncheck ./...
pnpm audit --audit-level=high
semgrep --config=auto --config=.semgrep/ .
```

---

## 3. Minimum Coverage Requirements

| Category | Target | Measurement |
|----------|--------|-------------|
| **Unit — Go business logic** | >= 80% line coverage | `go test -coverprofile` per module (IAM, Hosting, Backup, VersionManager, Audit) |
| **Unit — React components** | >= 80% branch coverage | `vitest --coverage` (v8 provider) for `src/` excluding generated code |
| **Integration — system adapters** | 100% adapter interfaces covered | Every adapter (Nginx, PHP-FPM, MariaDB, PostgreSQL, systemd, nftables, fail2ban, apt) has integration tests |
| **Integration — API** | Every endpoint has at least one happy-path + one auth-rejection test | Tracked via test inventory spreadsheet or code annotations |
| **E2E — critical workflows** | All 4 critical workflows pass | Installer, site provisioning, backup/restore, login+MFA |
| **Security — zero high/critical** | 0 high/critical findings from gosec, govulncheck, npm audit | Gate in CI — build fails on any high/critical |

**Coverage enforcement:**
```bash
# Go — fail if below 80%
go test ./internal/iam/... -coverprofile=cover.out
go tool cover -func=cover.out | grep total | awk '{if ($3+0 < 80.0) exit 1}'

# Frontend — vitest config
# vitest.config.ts → coverage.thresholds.branches = 80
```

---

## 4. System Adapter Testing Strategy

### Problem

System adapters execute real commands (`nginx -t`, `systemctl reload`, `nft add rule`, `apt install`, etc.). Running these in CI requires root on a real Debian system.

### Solution: Two-layer approach

#### Layer 1 — Interface mocking (unit + integration, runs everywhere)

Every adapter implements a Go interface. Business logic depends only on the interface, never on the concrete implementation.

```go
// internal/adapter/nginx/nginx.go
type NginxAdapter interface {
    CreateVhost(ctx context.Context, site Site) error
    TestConfig(ctx context.Context) error
    Reload(ctx context.Context) error
    Status(ctx context.Context) (ServiceStatus, error)
}
```

```go
// internal/adapter/systemd/systemd.go
type SystemdAdapter interface {
    Start(ctx context.Context, unit string) error
    Stop(ctx context.Context, unit string) error
    Reload(ctx context.Context, unit string) error
    Enable(ctx context.Context, unit string) error
    IsActive(ctx context.Context, unit string) (bool, error)
}
```

For exec-based adapters, extract command execution behind an `Executor` interface:

```go
type CommandExecutor interface {
    Run(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)
}
```

In tests, inject a `FakeExecutor` that returns predefined stdout/stderr/exit codes. This lets you test:
- Config file generation (Nginx vhosts, PHP-FPM pools, nftables rules) — assert output content.
- Correct command arguments and sequencing.
- Error handling for non-zero exit codes, timeouts, permission denied.
- Retry logic and rollback on partial failure.

#### Layer 2 — Smoke tests on real Debian 13 VM (nightly + release)

Run the actual adapter implementations against a real Debian 13 environment:

```bash
# Provisioned via Vagrant, cloud-init, or CI runner with Debian 13 image
# Smoke test suite tagged "smoke"
go test ./internal/adapter/... -tags=smoke -count=1 -timeout=15m
```

Smoke tests verify:
- Nginx vhost creation + `nginx -t` + reload succeeds.
- PHP-FPM pool creation + restart produces a working pool.
- MariaDB/PostgreSQL database + user creation works end-to-end.
- systemd start/stop/enable/disable cycles.
- nftables rule insertion + listing + deletion.
- fail2ban jail configuration + ban/unban cycle.
- apt package install + removal.

---

## 5. Installer Testing Strategy

### Requirements covered
- INS-001 through INS-008: clean Debian 13, idempotency, non-interactive mode, DB engine choice.

### Test environment

| Environment | Tool | Purpose |
|-------------|------|---------|
| Debian 13 VM | Vagrant + libvirt/VirtualBox or cloud VM (Hetzner/GCP) | Realistic hardware, clean snapshots |
| Debian 13 container | Docker `debian:trixie` (limited — no systemd) | Fast unit tests of installer logic (package lists, validation) |
| CI runner | GitHub Actions + self-hosted Debian 13 runner | Nightly full install |

### Test matrix

| Test | What it verifies | Frequency |
|------|------------------|-----------|
| **Clean install — MariaDB** | Full install on clean Debian 13 choosing MariaDB | Nightly |
| **Clean install — PostgreSQL** | Full install on clean Debian 13 choosing PostgreSQL | Nightly |
| **Clean install — both DB** | Full install on clean Debian 13 choosing both engines | Nightly |
| **Idempotency** | Run installer twice on same VM — no errors, no config destruction | Nightly |
| **Interrupted install resume** | Kill installer mid-run (after package install, before config), re-run — completes | Weekly |
| **Pre-requisite rejection** | Run on Ubuntu, CentOS, or modified Debian — installer rejects cleanly | PR (container-based) |
| **Non-interactive mode** | `./install.sh --non-interactive --db=mariadb` runs without prompts | PR (container-based) |
| **Post-install validation** | After install: panel responds on port, services running, TLS obtainable | Nightly |
| **Rollback** | Trigger rollback after partial install — system returns to pre-install state | Weekly |
| **Install report** | Verify final report contains all installed components and versions | Every install test |

### Implementation

```bash
# Vagrant-based test (example)
vagrant up debian13-clean
vagrant ssh -c "curl -sSL https://install.aipanel.dev | bash -s -- --non-interactive --db=mariadb"
vagrant ssh -c "aipanel health-check"

# Idempotency test
vagrant ssh -c "curl -sSL https://install.aipanel.dev | bash -s -- --non-interactive --db=mariadb"
vagrant ssh -c "aipanel health-check"  # must still pass

vagrant destroy -f
```

---

## 6. CI Pipeline

### Pipeline stages by trigger

| Trigger | Stage | Tests | Timeout |
|---------|-------|-------|---------|
| **Pre-commit** (local, via `lefthook`) | Lint + format | `golangci-lint run`, `pnpm lint`, `gosec ./...` | 60s |
| **PR opened/updated** | Unit | Go unit tests + React unit tests | 5m |
| | Integration | Go integration tests (SQLite in-memory, httptest) | 10m |
| | Security scan | `gosec`, `govulncheck`, `pnpm audit`, `semgrep` | 5m |
| | Build | `go build`, `pnpm build` — verify compilation | 3m |
| | Coverage gate | Fail if coverage < 80% | — |
| **Nightly (scheduled)** | E2E | Playwright on Debian 13 VM | 30m |
| | Installer | Full install matrix (3 DB configs + idempotency) | 60m |
| | Adapter smoke | Real adapter tests on Debian 13 VM | 15m |
| | Security deep | `trivy` image scan, full `semgrep` ruleset | 10m |
| | Performance | Benchmark suite (see section 8) | 15m |
| | Visual regression | Playwright screenshots dark/light (see section 7) | 10m |
| **Release tag** | All of the above | Full matrix | 120m |
| | Installer on fresh cloud VM | Real cloud provisioning test | 30m |
| | Signed artifact verification | Checksum + signature validation | 2m |

### Pre-commit setup (lefthook)

```yaml
# .lefthook.yml
pre-commit:
  parallel: true
  commands:
    go-lint:
      glob: "*.go"
      run: golangci-lint run --new-from-rev=HEAD~1
    go-test:
      glob: "*.go"
      run: go test ./... -short -count=1 -race -timeout=60s
    frontend-lint:
      glob: "*.{ts,tsx}"
      run: pnpm lint
    frontend-test:
      glob: "*.{ts,tsx}"
      run: pnpm vitest run --changed
    security:
      glob: "*.go"
      run: gosec ./...
```

---

## 7. Visual Regression

### Approach

Capture Playwright screenshots of every key screen in both themes at 3 breakpoints. Compare against baseline using pixel diff.

### Configuration

```typescript
// playwright.config.ts (visual regression project)
{
  projects: [
    { name: 'desktop-light', use: { viewport: { width: 1440, height: 900 }, colorScheme: 'light' } },
    { name: 'desktop-dark',  use: { viewport: { width: 1440, height: 900 }, colorScheme: 'dark' } },
    { name: 'tablet-light',  use: { viewport: { width: 768, height: 1024 }, colorScheme: 'light' } },
    { name: 'tablet-dark',   use: { viewport: { width: 768, height: 1024 }, colorScheme: 'dark' } },
    { name: 'mobile-light',  use: { viewport: { width: 375, height: 812 }, colorScheme: 'light' } },
    { name: 'mobile-dark',   use: { viewport: { width: 375, height: 812 }, colorScheme: 'dark' } },
  ],
}
```

### Screens to capture

- Dashboard (empty state, populated state, alert state)
- Sites & Domains list
- Site detail / provisioning form
- Updates & Versions table
- Security & Audit timeline
- Backup & Restore
- Settings / theme toggle
- Login page
- File Manager (directory view, editor view)

### Workflow

1. Playwright `toHaveScreenshot()` with `maxDiffPixelRatio: 0.01`.
2. Baseline screenshots committed in `tests/visual/snapshots/`.
3. On diff detection, CI uploads comparison artifacts (expected / actual / diff).
4. Developer reviews diff and either updates baseline (`pnpm playwright test --update-snapshots`) or fixes the regression.

```bash
# Run visual regression tests
pnpm playwright test --project='desktop-*' --project='tablet-*' --project='mobile-*' tests/visual/

# Update baselines after intentional changes
pnpm playwright test --update-snapshots tests/visual/
```

---

## 8. Performance Testing

### Benchmark targets (from PRD NFR-PERF)

| Metric | Target | Test method |
|--------|--------|-------------|
| Dashboard P95 load | <= 1.5s at 200 concurrent sessions | `k6` load test |
| CRUD operations P95 | <= 800ms | `k6` API test |
| Site provisioning P95 | <= 2 min | E2E timing in Playwright |
| Panel overhead (steady-state) | <= 10% CPU, <= 1.5 GB RAM | `prometheus` + `node_exporter` during load test |
| Installer completion | <= 20 min on reference server | Timed installer test |

### Tools

| Tool | Purpose |
|------|---------|
| `k6` (Grafana) | HTTP load testing — scripted scenarios against API |
| Go `testing.B` | Microbenchmarks for hot paths (template rendering, DB queries, job queue throughput) |
| Playwright `performance.timing` | Frontend page load metrics |
| `prometheus` + `node_exporter` | Host resource monitoring during load tests |

### k6 scenario example

```javascript
// tests/performance/dashboard-load.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '60s', target: 200 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<1500'],  // NFR-PERF-001
  },
};

export default function () {
  const res = http.get('https://panel.test/api/v1/dashboard', {
    headers: { Authorization: `Bearer ${__ENV.TOKEN}` },
  });
  check(res, { 'status 200': (r) => r.status === 200 });
  sleep(1);
}
```

### Go microbenchmarks

```bash
# Run all benchmarks
go test ./... -bench=. -benchmem -count=5 -run=^$

# Compare with baseline (benchstat)
go test ./... -bench=. -benchmem -count=10 -run=^$ > new.txt
benchstat old.txt new.txt
```

### Performance regression detection

- Nightly: run `k6` and Go benchmarks, store results.
- Compare P95 against previous nightly. Alert if regression > 15%.
- Release gate: all NFR-PERF thresholds must pass.

---

## 9. Test Naming Conventions and File Structure

### Go

**Naming:**
```
func TestServiceName_MethodName_Scenario(t *testing.T)
func TestServiceName_MethodName_ErrorCase(t *testing.T)
func BenchmarkServiceName_MethodName(b *testing.B)
```

Examples:
```go
func TestNginxAdapter_CreateVhost_Success(t *testing.T)
func TestNginxAdapter_CreateVhost_InvalidDomain(t *testing.T)
func TestNginxAdapter_Reload_ServiceDown(t *testing.T)
func TestIAMService_Login_ValidCredentials(t *testing.T)
func TestIAMService_Login_MFARequired(t *testing.T)
func TestIAMService_Login_BruteForceBlocked(t *testing.T)
func BenchmarkJobQueue_Enqueue(b *testing.B)
```

**File structure:**
```
internal/
├── iam/
│   ├── service.go
│   ├── service_test.go            # unit tests
│   ├── service_integration_test.go # integration tests (build tag)
│   └── testdata/                   # fixtures (JSON, SQL, config files)
├── hosting/
│   ├── site_service.go
│   ├── site_service_test.go
│   └── testdata/
├── adapter/
│   ├── nginx/
│   │   ├── adapter.go
│   │   ├── adapter_test.go         # unit tests with mocked executor
│   │   ├── adapter_smoke_test.go   # smoke tests (build tag: smoke)
│   │   └── testdata/
│   │       ├── vhost_expected.conf
│   │       └── nginx_error.txt
│   ├── phpfpm/
│   ├── mariadb/
│   ├── postgresql/
│   ├── systemd/
│   ├── nftables/
│   ├── fail2ban/
│   └── apt/
└── testutil/                       # shared test helpers
    ├── db.go                       # in-memory SQLite setup + migration
    ├── fixtures.go                 # common test data factories
    └── executor_mock.go            # fake CommandExecutor
```

**Build tags:**
```go
//go:build integration
// +build integration

//go:build smoke
// +build smoke
```

### React / TypeScript

**Naming:**
```
ComponentName.test.tsx
useHookName.test.ts
serviceName.test.ts
```

Examples:
```
DashboardPage.test.tsx
SiteProvisioningForm.test.tsx
useAuthSession.test.ts
apiClient.test.ts
```

**File structure:**
```
src/
├── components/
│   ├── dashboard/
│   │   ├── DashboardPage.tsx
│   │   ├── DashboardPage.test.tsx      # co-located unit test
│   │   ├── HealthScoreCard.tsx
│   │   └── HealthScoreCard.test.tsx
│   └── sites/
│       ├── SiteList.tsx
│       └── SiteList.test.tsx
├── hooks/
│   ├── useAuthSession.ts
│   └── useAuthSession.test.ts
├── lib/
│   ├── api-client.ts
│   └── api-client.test.ts
└── test/
    ├── setup.ts                        # vitest global setup (MSW, providers)
    ├── mocks/
    │   ├── handlers.ts                 # MSW request handlers
    │   └── server.ts                   # MSW server instance
    └── factories/
        ├── site.ts                     # test data factory for Site
        └── user.ts
```

### E2E (Playwright)

**File structure:**
```
tests/
├── e2e/
│   ├── installer.spec.ts
│   ├── site-provisioning.spec.ts
│   ├── backup-restore.spec.ts
│   ├── login-mfa.spec.ts
│   ├── file-manager.spec.ts
│   └── rbac.spec.ts
├── visual/
│   ├── dashboard.visual.ts
│   ├── sites.visual.ts
│   ├── updates.visual.ts
│   └── snapshots/                      # baseline screenshots (committed)
│       ├── dashboard-desktop-light.png
│       ├── dashboard-desktop-dark.png
│       └── ...
├── performance/
│   ├── dashboard-load.js              # k6 script
│   └── api-crud.js                    # k6 script
├── fixtures/
│   └── test-site.json
└── playwright.config.ts
```

### Test data conventions

- **Go:** `testdata/` directory in each package, loaded via `os.ReadFile("testdata/...")`.
- **React:** `src/test/factories/` for typed test data builders; `src/test/mocks/handlers.ts` for MSW API mocks.
- **E2E:** `tests/fixtures/` for shared test payloads.
- **No production credentials** in test fixtures. Use generated tokens, dummy passwords, and test domains (`*.test`, `*.example`).
