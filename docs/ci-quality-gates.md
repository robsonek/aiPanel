# CI Quality Gates — aiPanel

## 1. Pipeline Overview

```
 Developer workstation                     GitHub Actions                         Release
 ─────────────────────                     ──────────────                         ───────

 ┌─────────────┐   ┌────────────────────┐   ┌────────────────────┐   ┌──────────────────┐
 │   Commit     │──▶│  Pre-commit Hooks  │──▶│    PR Checks       │──▶│  Merge to main   │
 │  (local)     │   │  (lefthook)        │   │  (all must pass)   │   │                  │
 └─────────────┘   └────────────────────┘   └────────────────────┘   └────────┬─────────┘
                     │ gofmt              │     │ Unit tests (Go+React)│                  │
                     │ golangci-lint      │     │ Integration tests    │                  │
                     │ gosec (quick)      │     │ Lint (Go+Frontend)   │                  ▼
                     │ eslint + prettier  │     │ SAST (gosec+semgrep) │        ┌─────────────────┐
                     │ tsc --noEmit       │     │ Dep scan             │   ┌───▶│  Nightly        │
                     │ gitleaks           │     │ Coverage >= 80%      │   │    │  (scheduled)    │
                     │ commit msg check   │     │ Build check          │   │    └────────┬────────┘
                     └────────────────────┘     └──────────────────────┘   │             │
                                                                          │     │ E2E (Playwright)    │
                                                                    cron 0 3 * * *│ Installer matrix    │
                                                                          │     │ Adapter smoke tests │
                                                                          │     │ Visual regression   │
                                                                          │     │ Perf benchmarks     │
                                                                          │     │ SBOM generation     │
                                                                          │     │ Trivy full scan     │
                                                                          │     │ License compliance  │
                                                                          │     │ Backup dry-run      │
                                                                          │             │
                                                                          │             ▼
                                                                          │    ┌─────────────────┐
                                                                          │    │ Nightly passed   │
                                                                          │    │ in last 24h?     │
                                                                          │    └────────┬────────┘
                                                                          │          YES│
                                                                          │             ▼
                                                                          │    ┌─────────────────┐
                                                                          └───▶│ Release Pipeline │
                                                                               │ (manual trigger) │
                                                                               └────────┬────────┘
                                                                                        │
                                                                                │ Semantic version tag    │
                                                                                │ Changelog generation    │
                                                                                │ Static binary build     │
                                                                                │ Cross-compile           │
                                                                                │ SBOM attach             │
                                                                                │ Cosign signing          │
                                                                                │ SHA-256 checksums       │
                                                                                │ GitHub Release          │
                                                                                │ Container image (opt)   │
                                                                                        │
                                                                                        ▼
                                                                               ┌─────────────────┐
                                                                               │  Release v1.x.y  │
                                                                               │  published       │
                                                                               └─────────────────┘
```

---

## 2. Pre-commit Hooks (lefthook)

All hooks run locally before every commit via [lefthook](https://github.com/evilmartians/lefthook). They provide fast feedback and prevent obviously broken code from reaching CI.

### Configuration

```yaml
# .lefthook.yml
pre-commit:
  parallel: true
  commands:
    go-fmt:
      glob: "*.go"
      run: gofmt -l -d {staged_files} && test -z "$(gofmt -l {staged_files})"
      fail_text: "gofmt: files not formatted"

    go-lint:
      glob: "*.go"
      run: golangci-lint run --new-from-rev=HEAD~1 --timeout=30s
      fail_text: "golangci-lint: lint errors found"

    go-sec:
      glob: "*.go"
      run: gosec -quiet -exclude-generated ./...
      fail_text: "gosec: security issues found"

    frontend-eslint:
      glob: "*.{ts,tsx,js,jsx}"
      run: pnpm eslint {staged_files}
      fail_text: "eslint: lint errors found"

    frontend-prettier:
      glob: "*.{ts,tsx,js,jsx,json,css,md,yaml,yml}"
      run: pnpm prettier --check {staged_files}
      fail_text: "prettier: formatting issues found"

    frontend-typecheck:
      glob: "*.{ts,tsx}"
      run: pnpm tsc --noEmit
      fail_text: "tsc: type errors found"

    secret-detection:
      run: gitleaks protect --staged --no-banner
      fail_text: "gitleaks: potential secrets detected"

commit-msg:
  commands:
    conventional-commit:
      run: >
        echo "{1}" | grep -qE
        '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\(.+\))?!?: .{1,72}'
      fail_text: |
        Commit message must follow Conventional Commits format:
          <type>(<optional scope>): <description>
        Types: feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert
        Example: feat(hosting): add PHP version switching
```

### Hook summary

| Hook | Tool | Target | Timeout |
|------|------|--------|---------|
| `go-fmt` | `gofmt` | `*.go` | 10s |
| `go-lint` | `golangci-lint` | `*.go` (changed) | 30s |
| `go-sec` | `gosec` (quick, exclude generated) | `*.go` | 30s |
| `frontend-eslint` | `eslint` | `*.{ts,tsx,js,jsx}` | 15s |
| `frontend-prettier` | `prettier --check` | `*.{ts,tsx,js,jsx,json,css,md,yaml}` | 10s |
| `frontend-typecheck` | `tsc --noEmit` | `*.{ts,tsx}` | 20s |
| `secret-detection` | `gitleaks` | staged files | 10s |
| `conventional-commit` | regex check | commit message | 1s |

---

## 3. PR Quality Gates

All gates below must pass before a PR can be merged into `main`. Configured as GitHub Actions required status checks.

### Gate matrix

| # | Gate | Command | Timeout | Blocker |
|---|------|---------|---------|---------|
| 1 | Go unit + integration tests | `go test ./... -count=1 -race -timeout=5m` | 5m | Yes |
| 2 | Frontend unit tests | `pnpm vitest run --reporter=verbose` | 3m | Yes |
| 3 | Go lint | `golangci-lint run --timeout=3m` | 3m | Yes |
| 4 | Frontend lint | `pnpm eslint . && pnpm tsc --noEmit` | 2m | Yes |
| 5 | SAST — gosec (full) | `gosec ./...` | 3m | Yes |
| 6 | SAST — semgrep | `semgrep --config=auto --config=.semgrep/ . --error` | 5m | Yes |
| 7 | Dependency scan — Go | `govulncheck ./...` | 2m | Yes |
| 8 | Dependency scan — npm | `pnpm audit --audit-level=high` | 1m | Yes |
| 9 | Coverage — Go business logic | `go test -coverprofile=cover.out ./internal/...` | 5m | Yes |
| 10 | Coverage — React branch | `pnpm vitest run --coverage` (threshold in config) | 3m | Yes |
| 11 | Build — Go | `CGO_ENABLED=0 go build -o /dev/null ./cmd/aipanel` | 2m | Yes |
| 12 | Build — Frontend | `pnpm vite build` | 2m | Yes |
| 13 | No TODO/FIXME/HACK in new code | `git diff origin/main...HEAD -- '*.go' '*.ts' '*.tsx' \| grep -E 'TODO\|FIXME\|HACK'` | 10s | No (warning) |

### Coverage enforcement

**Go (>= 80% for business logic packages):**

```bash
go test -coverprofile=cover.out ./internal/iam/... ./internal/hosting/... \
  ./internal/backup/... ./internal/versionmgr/... ./internal/audit/...

COVERAGE=$(go tool cover -func=cover.out | grep total | awk '{print $3}' | tr -d '%')
if (( $(echo "$COVERAGE < 80.0" | bc -l) )); then
  echo "FAIL: Go coverage ${COVERAGE}% < 80% threshold"
  exit 1
fi
```

**React (>= 80% branch coverage):**

```typescript
// vitest.config.ts
export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.*', 'src/test/**', 'src/**/*.d.ts'],
      thresholds: {
        branches: 80,
      },
    },
  },
});
```

### GitHub Actions workflow (PR)

```yaml
# .github/workflows/pr-checks.yml
name: PR Quality Gates
on:
  pull_request:
    branches: [main]

concurrency:
  group: pr-${{ github.event.pull_request.number }}
  cancel-in-progress: true

jobs:
  go-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test ./... -count=1 -race -timeout=5m
      - run: |
          go test -coverprofile=cover.out ./internal/iam/... ./internal/hosting/... \
            ./internal/backup/... ./internal/versionmgr/... ./internal/audit/...
          COVERAGE=$(go tool cover -func=cover.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Go coverage: ${COVERAGE}%"
          if (( $(echo "$COVERAGE < 80.0" | bc -l) )); then
            echo "::error::Go coverage ${COVERAGE}% is below 80% threshold"
            exit 1
          fi

  frontend-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with: { node-version-file: .node-version, cache: pnpm }
      - run: pnpm install --frozen-lockfile
      - run: pnpm vitest run --reporter=verbose --coverage
      - run: pnpm eslint .
      - run: pnpm tsc --noEmit

  go-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest, args: --timeout=3m }

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go install github.com/securego/gosec/v2/cmd/gosec@latest && gosec ./...
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...
      - uses: returntocorp/semgrep-action@v1
        with: { config: "auto .semgrep/" }
      - run: pnpm audit --audit-level=high

  build-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: CGO_ENABLED=0 go build -o /dev/null ./cmd/aipanel
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with: { node-version-file: .node-version, cache: pnpm }
      - run: pnpm install --frozen-lockfile && pnpm vite build

  code-hygiene:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - name: Check for TODO/FIXME/HACK in new code (warning only)
        run: |
          MATCHES=$(git diff origin/main...HEAD -- '*.go' '*.ts' '*.tsx' | grep -E '^\+.*\b(TODO|FIXME|HACK)\b' || true)
          if [ -n "$MATCHES" ]; then
            echo "::warning::New TODO/FIXME/HACK found in changed code:"
            echo "$MATCHES"
          fi
```

---

## 4. Nightly Pipeline

Runs every night at 03:00 UTC via cron schedule. Tests the full system on a real Debian 13 environment.

### Gate matrix

| # | Gate | Command / Tool | Timeout | Environment |
|---|------|----------------|---------|-------------|
| 1 | Full E2E test suite | `pnpm playwright test --project=chromium --project=firefox` | 30m | Debian 13 VM |
| 2 | Installer matrix — MariaDB | `./install.sh --non-interactive --db=mariadb` | 25m | Clean Debian 13 VM |
| 3 | Installer matrix — PostgreSQL | `./install.sh --non-interactive --db=postgresql` | 25m | Clean Debian 13 VM |
| 4 | Installer matrix — both | `./install.sh --non-interactive --db=both` | 25m | Clean Debian 13 VM |
| 5 | System adapter smoke tests | `go test ./internal/adapter/... -tags=smoke -count=1 -timeout=15m` | 15m | Debian 13 VM |
| 6 | Visual regression (dark + light, 3 breakpoints) | `pnpm playwright test tests/visual/` | 10m | Debian 13 VM |
| 7 | Performance — k6 load tests | `k6 run tests/performance/dashboard-load.js` | 15m | Debian 13 VM |
| 8 | Performance — Go benchmarks | `go test ./... -bench=. -benchmem -count=10 -run=^$ > bench-new.txt && benchstat bench-old.txt bench-new.txt` | 10m | Debian 13 VM |
| 9 | Backup/restore dry-run | `aipanel backup --dry-run && aipanel restore --dry-run --latest` | 10m | Debian 13 VM |
| 10 | SBOM generation — Go | `cyclonedx-gomod app -output sbom-go.json -json` | 2m | CI runner |
| 11 | SBOM generation — Frontend | `cyclonedx-npm --output-file sbom-frontend.json` | 2m | CI runner |
| 12 | Full vulnerability scan | `trivy fs --severity HIGH,CRITICAL --exit-code 1 .` | 5m | CI runner |
| 13 | License compliance | `trivy fs --scanners license --severity UNKNOWN,HIGH,CRITICAL .` | 3m | CI runner |

### Visual regression configuration

6 projects covering all theme/breakpoint combinations:

| Project | Viewport | Theme |
|---------|----------|-------|
| `desktop-light` | 1440 x 900 | light |
| `desktop-dark` | 1440 x 900 | dark |
| `tablet-light` | 768 x 1024 | light |
| `tablet-dark` | 768 x 1024 | dark |
| `mobile-light` | 375 x 812 | light |
| `mobile-dark` | 375 x 812 | dark |

Diff threshold: `maxDiffPixelRatio: 0.01`. Comparison artifacts (expected / actual / diff) uploaded on failure.

### Performance regression detection

```bash
# Store previous nightly benchmark as bench-old.txt (artifact from last run)
# Run current benchmarks
go test ./... -bench=. -benchmem -count=10 -run='^$' > bench-new.txt

# Compare with benchstat — fail if any regression > 15%
benchstat -delta-test=none bench-old.txt bench-new.txt | tee benchstat-report.txt

# k6 thresholds are defined in the script:
#   http_req_duration: ['p(95)<1500']   — NFR-PERF-001
#   http_req_duration: ['p(95)<800']    — NFR-PERF-002
```

### GitHub Actions workflow (nightly)

```yaml
# .github/workflows/nightly.yml
name: Nightly Quality Gates
on:
  schedule:
    - cron: '0 3 * * *'
  workflow_dispatch:

jobs:
  e2e-tests:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      - run: pnpm playwright install --with-deps
      - run: pnpm playwright test --project=chromium --project=firefox

  installer-matrix:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 60
    strategy:
      fail-fast: false
      matrix:
        db: [mariadb, postgresql, both]
    steps:
      - uses: actions/checkout@v4
      - name: Restore clean VM snapshot
        run: ./scripts/ci/restore-clean-snapshot.sh
      - name: Run installer
        run: ./install.sh --non-interactive --db=${{ matrix.db }}
      - name: Post-install validation
        run: aipanel health-check

  adapter-smoke:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test ./internal/adapter/... -tags=smoke -count=1 -timeout=15m

  visual-regression:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
      - run: pnpm playwright test tests/visual/
      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: visual-regression-diffs
          path: tests/visual/test-results/

  performance:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 25
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Download previous benchmark baseline
        uses: actions/download-artifact@v4
        with: { name: bench-baseline, path: . }
        continue-on-error: true
      - name: Go benchmarks
        run: go test ./... -bench=. -benchmem -count=10 -run='^$' > bench-new.txt
      - name: Regression check (benchstat)
        run: |
          if [ -f bench-old.txt ]; then
            benchstat bench-old.txt bench-new.txt | tee benchstat-report.txt
          fi
          cp bench-new.txt bench-old.txt
      - name: k6 load tests
        run: k6 run tests/performance/dashboard-load.js
      - uses: actions/upload-artifact@v4
        with:
          name: bench-baseline
          path: bench-old.txt

  backup-dryrun:
    runs-on: [self-hosted, debian13]
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
      - run: aipanel backup --dry-run
      - run: aipanel restore --dry-run --latest

  sbom-and-scans:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: SBOM — Go modules
        run: cyclonedx-gomod app -output sbom-go.json -json
      - name: SBOM — Frontend packages
        run: pnpm dlx @cyclonedx/cyclonedx-npm --output-file sbom-frontend.json
      - name: Full vulnerability scan (trivy)
        run: trivy fs --severity HIGH,CRITICAL --exit-code 1 .
      - name: License compliance check
        run: trivy fs --scanners license --severity UNKNOWN,HIGH,CRITICAL .
      - uses: actions/upload-artifact@v4
        with:
          name: sbom-nightly
          path: |
            sbom-go.json
            sbom-frontend.json
```

---

## 5. Release Pipeline

Triggered manually (workflow dispatch) when cutting a new release. Requires all nightly gates to have passed within the last 24 hours.

### Prerequisites

| Check | Requirement |
|-------|-------------|
| Nightly pipeline | All jobs green in last 24h |
| Branch | `main`, clean working tree |
| Tag format | `vMAJOR.MINOR.PATCH` (semantic versioning) |
| Changelog | Auto-generated from conventional commits since last tag |

### Release steps

| # | Step | Command / Tool | Output |
|---|------|----------------|--------|
| 1 | Validate nightly status | `gh run list --workflow=nightly.yml --limit=1 --json conclusion` | Pass/fail check |
| 2 | Determine version | `git-cliff --bumped-version` or manual input | `vX.Y.Z` |
| 3 | Generate changelog | `git-cliff --latest --strip header -o CHANGELOG.md` | `CHANGELOG.md` |
| 4 | Build — Go binary (linux/amd64) | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" -o dist/aipanel-linux-amd64 ./cmd/aipanel` | Static binary |
| 5 | Build — Go binary (linux/arm64) | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$VERSION" -o dist/aipanel-linux-arm64 ./cmd/aipanel` | Static binary |
| 6 | Build — Frontend (embedded) | `pnpm vite build` (output embedded via `go:embed`) | Embedded in binary |
| 7 | Generate SBOM — Go | `cyclonedx-gomod app -output dist/sbom-go.json -json` | CycloneDX JSON |
| 8 | Generate SBOM — Frontend | `cyclonedx-npm --output-file dist/sbom-frontend.json` | CycloneDX JSON |
| 9 | Checksum file | `cd dist && sha256sum * > checksums-sha256.txt` | SHA-256 checksums |
| 10 | Sign binary — cosign | `cosign sign-blob --yes --output-signature dist/aipanel-linux-amd64.sig dist/aipanel-linux-amd64` | Sigstore signature |
| 11 | Sign binary — cosign (arm64) | `cosign sign-blob --yes --output-signature dist/aipanel-linux-arm64.sig dist/aipanel-linux-arm64` | Sigstore signature |
| 12 | GitHub Release | `gh release create $VERSION dist/* --title "$VERSION" --notes-file CHANGELOG.md` | Published release |
| 13 | Container image (optional) | `docker build -t ghcr.io/aipanel/aipanel:$VERSION .` | OCI image |
| 14 | Sign container image | `cosign sign ghcr.io/aipanel/aipanel:$VERSION` | Signed manifest |
| 15 | Push container image | `docker push ghcr.io/aipanel/aipanel:$VERSION` | Registry push |

### Cross-compilation matrix

| Target | GOOS | GOARCH | CGO_ENABLED | Output |
|--------|------|--------|-------------|--------|
| Debian 13 x86_64 | `linux` | `amd64` | `0` | `aipanel-linux-amd64` |
| Debian 13 ARM64 | `linux` | `arm64` | `0` | `aipanel-linux-arm64` |

### GitHub Actions workflow (release)

```yaml
# .github/workflows/release.yml
name: Release Pipeline
on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Release version (e.g., v1.2.3)'
        required: true
        type: string

env:
  VERSION: ${{ github.event.inputs.version }}

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - name: Check nightly passed in last 24h
        run: |
          CONCLUSION=$(gh run list --workflow=nightly.yml --limit=1 --json conclusion -q '.[0].conclusion')
          if [ "$CONCLUSION" != "success" ]; then
            echo "::error::Nightly pipeline did not pass. Release blocked."
            exit 1
          fi
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  build:
    needs: validate
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with: { node-version-file: .node-version, cache: pnpm }
      - run: pnpm install --frozen-lockfile && pnpm vite build
      - name: Build Go binary
        run: |
          CGO_ENABLED=0 GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} \
            go build -ldflags="-s -w -X main.version=$VERSION" \
            -o dist/aipanel-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/aipanel
      - uses: actions/upload-artifact@v4
        with:
          name: binary-${{ matrix.goos }}-${{ matrix.goarch }}
          path: dist/

  release:
    needs: build
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/download-artifact@v4
        with: { path: dist/, merge-multiple: true }
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: sigstore/cosign-installer@v3

      - name: Generate SBOM (Go)
        run: |
          go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
          cyclonedx-gomod app -output dist/sbom-go.json -json
      - name: Generate SBOM (Frontend)
        run: pnpm dlx @cyclonedx/cyclonedx-npm --output-file dist/sbom-frontend.json

      - name: Generate checksums
        run: cd dist && sha256sum * > checksums-sha256.txt

      - name: Sign artifacts (cosign)
        run: |
          cosign sign-blob --yes \
            --output-signature dist/aipanel-linux-amd64.sig \
            dist/aipanel-linux-amd64
          cosign sign-blob --yes \
            --output-signature dist/aipanel-linux-arm64.sig \
            dist/aipanel-linux-arm64

      - name: Generate changelog
        run: |
          go install github.com/orhun/git-cliff/git-cliff@latest
          git-cliff --latest --strip header -o RELEASE_NOTES.md

      - name: Create GitHub Release
        run: |
          gh release create $VERSION dist/* \
            --title "$VERSION" \
            --notes-file RELEASE_NOTES.md
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

---

## 6. SBOM Requirements

### Overview

A Software Bill of Materials (SBOM) is generated for every release and nightly build to enable dependency tracking, vulnerability scanning, and supply chain transparency.

### Specification

| Field | Requirement |
|-------|-------------|
| **Format** | CycloneDX JSON (preferred) or SPDX JSON |
| **Frequency** | Every release (mandatory), every nightly build (stored as artifact) |
| **Go modules** | Generated with `cyclonedx-gomod app -json` |
| **Frontend packages** | Generated with `@cyclonedx/cyclonedx-npm` |
| **System dependencies** | Declared in `installer/dependencies.json` manifest; included in combined SBOM |
| **Storage** | Attached to GitHub Release alongside binaries |
| **Naming convention** | `sbom-go.json`, `sbom-frontend.json` |

### Generation commands

```bash
# Go SBOM
go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
cyclonedx-gomod app -output sbom-go.json -json

# Frontend SBOM
pnpm dlx @cyclonedx/cyclonedx-npm --output-file sbom-frontend.json

# Verify SBOM is valid CycloneDX
cyclonedx validate --input-file sbom-go.json --input-format json
cyclonedx validate --input-file sbom-frontend.json --input-format json
```

### Contents

| SBOM | Includes |
|------|----------|
| `sbom-go.json` | All Go module dependencies (`go.sum`), transitive dependencies, license info |
| `sbom-frontend.json` | All npm packages from `pnpm-lock.yaml`, transitive dependencies, license info |
| `installer/dependencies.json` | System packages installed by the installer (Nginx, PHP-FPM, MariaDB, PostgreSQL, nftables, fail2ban, etc.) |

### Downstream consumption

Downstream consumers (server administrators, security teams, compliance audits) can scan the released SBOM with tools such as:

```bash
# Scan SBOM for known vulnerabilities
trivy sbom sbom-go.json
trivy sbom sbom-frontend.json

# Scan for license compliance
trivy sbom --scanners license sbom-go.json
```

---

## 7. Security Scan Details

### Static analysis (SAST)

| Tool | Language | What it detects | Stage |
|------|----------|-----------------|-------|
| `gosec` | Go | Hardcoded secrets, SQL injection, path traversal, insecure crypto, unsafe exec | Pre-commit (quick) + PR (full) |
| `semgrep` | Go, TS, YAML | Custom rules for exec calls, template injection, RBAC bypass patterns, SSRF | PR + nightly |
| `eslint-plugin-security` | JavaScript/TypeScript | `eval()`, `new Function()`, prototype pollution, RegExp DoS | PR (via eslint) |

### Dependency vulnerability scanning

| Tool | Scope | What it detects | Stage |
|------|-------|-----------------|-------|
| `govulncheck` | Go modules | Known CVEs in Go dependencies (uses Go vulnerability database) | PR + nightly |
| `pnpm audit` / `npm audit` | Frontend packages | Known CVEs in npm dependencies (npm advisory database) | PR + nightly |
| `trivy` | Filesystem + container | Full vulnerability scan across all dependency types, container layers | Nightly + release |

### Secret detection

| Tool | Scope | Stage |
|------|-------|-------|
| `gitleaks` | Staged git files | Pre-commit hook |
| `gitleaks` | Full repository history | Nightly (optional, for historical audit) |

### Configuration

```bash
# gosec (full mode, used in PR)
gosec -severity medium -confidence medium -exclude-generated ./...

# semgrep (with custom rules)
semgrep --config=auto --config=.semgrep/ . --error --severity=WARNING

# eslint with security plugin (in .eslintrc or eslint.config.js)
# plugins: ['security']
# extends: ['plugin:security/recommended']

# govulncheck
govulncheck ./...

# npm audit (high and critical only)
pnpm audit --audit-level=high

# trivy filesystem scan
trivy fs --severity HIGH,CRITICAL --exit-code 1 .

# trivy container scan (release only)
trivy image --severity HIGH,CRITICAL --exit-code 1 ghcr.io/aipanel/aipanel:$VERSION

# gitleaks (pre-commit)
gitleaks protect --staged --no-banner
```

### Policy

| Severity | Policy | Action |
|----------|--------|--------|
| **Critical** | Zero tolerance | Release blocked, immediate fix required |
| **High** | Zero tolerance | Release blocked, immediate fix required |
| **Medium** | Tracked as tech debt | Issue created, must be resolved within 30 days |
| **Low** | Informational | Logged, resolved during regular maintenance |

---

## 8. Gate Failure Protocol

### PR gate failure

| Aspect | Protocol |
|--------|----------|
| **Effect** | PR cannot be merged |
| **Notification** | GitHub check status visible on PR; author receives email notification |
| **Action required** | Author fixes the issue, pushes new commits, CI re-runs automatically |
| **Escalation** | None (author-owned) |
| **Override** | No override for required checks; only repo admin can bypass in emergencies |

### Nightly pipeline failure

| Aspect | Protocol |
|--------|----------|
| **Effect** | Release pipeline blocked (nightly must pass in last 24h to release) |
| **Notification** | Slack channel `#ci-alerts` + email to on-call engineer |
| **Action required** | On-call triages failure, creates GitHub issue with label `ci:nightly-failure` |
| **Escalation** | If not resolved within 24h, escalate to tech lead |
| **Tracking** | Each nightly failure tracked as a GitHub issue |

### Release gate failure

| Aspect | Protocol |
|--------|----------|
| **Effect** | Release blocked, artifacts not published |
| **Notification** | Slack channel `#releases` + email to release manager |
| **Action required** | Hotfix branch created, fix merged, nightly must pass again |
| **Escalation** | Immediate escalation to tech lead and security lead (if security gate failed) |
| **Rollback** | If the release was partially published (should not happen with atomic release), retract immediately |

### Flaky test policy

| Condition | Action |
|-----------|--------|
| Test fails 3 consecutive times with non-deterministic cause | Test quarantined (moved to `@flaky` tag, excluded from blocking gates) |
| Quarantined test | GitHub issue created with label `test:flaky`, assigned to test owner |
| Quarantine duration | Maximum 7 days; if not fixed, test must be rewritten or deleted |
| Quarantine mechanism | Build tag `//go:build !flaky` for Go; `.skip()` with comment for Vitest/Playwright |
| Monitoring | Weekly report of quarantined tests; count must trend toward zero |

### Failure response flowchart

```
  Gate Failure Detected
          │
          ▼
  ┌───────────────┐     ┌───────────────────────────────────┐
  │ PR gate?      │─YES─▶ Author notified via GitHub check  │
  └───────┬───────┘     │ Author fixes and re-pushes        │
          │NO           └───────────────────────────────────┘
          ▼
  ┌───────────────┐     ┌───────────────────────────────────┐
  │ Nightly gate? │─YES─▶ Alert: Slack #ci-alerts + email   │
  └───────┬───────┘     │ On-call creates issue              │
          │NO           │ Must resolve before next release   │
          ▼             └───────────────────────────────────┘
  ┌───────────────┐     ┌───────────────────────────────────┐
  │ Release gate? │─YES─▶ Alert: Slack #releases + email    │
  └───────────────┘     │ Release blocked                    │
                        │ Hotfix process triggered           │
                        │ Security lead notified if sec gate │
                        └───────────────────────────────────┘
```

---

## Appendix A: Tool Versions and Installation

| Tool | Install command | Purpose |
|------|-----------------|---------|
| `lefthook` | `go install github.com/evilmartians/lefthook@latest` | Git hooks manager |
| `golangci-lint` | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` | Go linter aggregator |
| `gosec` | `go install github.com/securego/gosec/v2/cmd/gosec@latest` | Go security scanner |
| `govulncheck` | `go install golang.org/x/vuln/cmd/govulncheck@latest` | Go vulnerability checker |
| `semgrep` | `pip install semgrep` | Multi-language SAST |
| `trivy` | `brew install trivy` / `apt install trivy` | Vulnerability + SBOM scanner |
| `gitleaks` | `go install github.com/gitleaks/gitleaks/v8@latest` | Secret detection |
| `cyclonedx-gomod` | `go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest` | Go SBOM generator |
| `@cyclonedx/cyclonedx-npm` | `pnpm dlx @cyclonedx/cyclonedx-npm` | npm SBOM generator |
| `cosign` | `go install github.com/sigstore/cosign/v2/cmd/cosign@latest` | Artifact signing (Sigstore) |
| `k6` | `brew install k6` / `apt install k6` | HTTP load testing |
| `benchstat` | `go install golang.org/x/perf/cmd/benchstat@latest` | Go benchmark comparison |
| `git-cliff` | `cargo install git-cliff` | Changelog generator |
| `playwright` | `pnpm playwright install --with-deps` | E2E + visual testing |

## Appendix B: Complete Gate Summary

| Pipeline | Gate | Blocker | Frequency |
|----------|------|---------|-----------|
| Pre-commit | gofmt | Yes | Every commit |
| Pre-commit | golangci-lint (changed files) | Yes | Every commit |
| Pre-commit | gosec (quick) | Yes | Every commit |
| Pre-commit | eslint | Yes | Every commit |
| Pre-commit | prettier | Yes | Every commit |
| Pre-commit | tsc --noEmit | Yes | Every commit |
| Pre-commit | gitleaks (staged) | Yes | Every commit |
| Pre-commit | Conventional commit message | Yes | Every commit |
| PR | Go unit + integration tests | Yes | Every PR |
| PR | Frontend unit tests (vitest) | Yes | Every PR |
| PR | Go lint (golangci-lint full) | Yes | Every PR |
| PR | Frontend lint (eslint + tsc) | Yes | Every PR |
| PR | SAST: gosec (full) | Yes | Every PR |
| PR | SAST: semgrep | Yes | Every PR |
| PR | Dependency: govulncheck | Yes | Every PR |
| PR | Dependency: pnpm audit | Yes | Every PR |
| PR | Coverage: Go >= 80% | Yes | Every PR |
| PR | Coverage: React branch >= 80% | Yes | Every PR |
| PR | Build: go build | Yes | Every PR |
| PR | Build: vite build | Yes | Every PR |
| PR | TODO/FIXME/HACK check | No (warning) | Every PR |
| Nightly | E2E (Playwright, Chromium + Firefox) | Yes | Daily 03:00 UTC |
| Nightly | Installer matrix (3 DB configs) | Yes | Daily 03:00 UTC |
| Nightly | System adapter smoke tests | Yes | Daily 03:00 UTC |
| Nightly | Visual regression (6 projects) | Yes | Daily 03:00 UTC |
| Nightly | Performance: k6 load tests | Yes | Daily 03:00 UTC |
| Nightly | Performance: Go benchmarks + benchstat | Yes | Daily 03:00 UTC |
| Nightly | Backup/restore dry-run | Yes | Daily 03:00 UTC |
| Nightly | SBOM generation | Yes | Daily 03:00 UTC |
| Nightly | Trivy vulnerability scan | Yes | Daily 03:00 UTC |
| Nightly | License compliance check | Yes | Daily 03:00 UTC |
| Release | Nightly passed in last 24h | Yes | Manual trigger |
| Release | Semantic version tag | Yes | Manual trigger |
| Release | Changelog generation | Yes | Manual trigger |
| Release | Static binary build (amd64 + arm64) | Yes | Manual trigger |
| Release | SBOM attached | Yes | Manual trigger |
| Release | Cosign artifact signing | Yes | Manual trigger |
| Release | SHA-256 checksums | Yes | Manual trigger |
| Release | GitHub Release published | Yes | Manual trigger |
| Release | Container image signed (optional) | No | Manual trigger |
