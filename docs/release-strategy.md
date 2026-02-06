# aiPanel Release Strategy

## 1. Versioning

aiPanel follows **Semantic Versioning (SemVer)**: `vMAJOR.MINOR.PATCH`

### Pre-1.0 (MVP phase)

- Format: `v0.x.y`
- Breaking changes are permitted in minor bumps (e.g., `v0.3.0` may break `v0.2.x` APIs or config).
- Patch bumps (`v0.x.1` -> `v0.x.2`) remain backward-compatible.

### Post-1.0

- Standard SemVer guarantees apply:
  - **MAJOR** — incompatible API or config changes.
  - **MINOR** — new functionality, backward-compatible.
  - **PATCH** — backward-compatible bug fixes.

### Version visibility

- Panel version displayed in the UI footer and Settings page.
- Every API response includes the `X-AiPanel-Version` header:
  ```
  X-AiPanel-Version: 0.4.2
  ```
- CLI reports version via `aipanel version`:
  ```
  $ aipanel version
  aiPanel v0.4.2 (stable) build 2026-02-06T14:00:00Z commit abc1234
  ```

---

## 2. Release Channels

| Channel   | Audience           | Stability        | Update frequency       |
|-----------|--------------------|------------------|------------------------|
| `stable`  | All production use | Production-ready | Every 2-4 weeks        |
| `beta`    | Early adopters     | Feature-complete, may have known issues | As features land |
| `nightly` | Developers/testing | No guarantees    | Every night from `main`|

### Channel selection

Channel is selected during installation and can be changed at any time:

```bash
# During install
curl -fsSL https://get.aipanel.dev | bash -s -- --channel=beta

# Change channel later
aipanel config set update.channel beta
```

The active channel is also configurable from **Settings > Updates** in the UI.

### Channel promotion flow

```
main (nightly) --> beta --> stable
```

Every stable release has been a beta first. Every beta was built from main.

---

## 3. Release Cadence

| Release type    | Schedule                          | Trigger                                    |
|-----------------|-----------------------------------|--------------------------------------------|
| **Patch**       | As needed                         | Security fix, critical bug                 |
| **Minor**       | Target every 2-4 weeks            | New features, non-critical improvements    |
| **Major**       | When necessary                    | Breaking changes (with migration guide)    |
| **Security hotfix** | Within 48h of CVE disclosure | CVE affecting aiPanel or its dependencies  |

The 48-hour security SLA aligns with NFR-SEC-006 from the PRD.

During active MVP development, minor releases may ship more frequently. After v1.0, the cadence stabilizes.

---

## 4. Hotfix Procedure

When a critical bug or security vulnerability is found in a released version:

1. **Branch** from the latest release tag:
   ```bash
   git checkout -b hotfix/v0.4.3 v0.4.2
   ```

2. **Minimal diff** — only the fix, no feature code. Every line in the diff must be directly related to the issue.

3. **Expedited review** — minimum 1 reviewer (normal PRs require 2). Reviewer must verify the fix addresses the issue and introduces no regressions.

4. **All quality gates still apply** — CI pipeline, tests, linting, build verification. No skipping.

5. **Merge and tag**:
   ```bash
   git tag v0.4.3
   git push origin v0.4.3
   ```

6. **Immediate release** — binaries built and published to GitHub Releases within minutes of merge.

7. **Notification** — all installed instances pick up the new version on their next update check (every 6h) or immediately via:
   ```bash
   aipanel update
   ```

---

## 5. Update Distribution

### Self-update mechanism

Updates can be triggered from CLI or UI:

```bash
# Check for updates
aipanel update check

# Apply available update
aipanel update

# Update to a specific version
aipanel update --version v0.4.3
```

The UI provides an equivalent **"Update Available"** banner with a one-click update button in **Settings > Updates**.

### Automatic update checks

The panel checks for new versions every **6 hours** (aligned with NFR-VER-001 feed sync interval). This is configurable:

```bash
aipanel config set update.check-interval 12h
```

### Download source

- Primary: **GitHub Releases** (signed artifacts).
- Future: optional self-hosted mirror/proxy for air-gapped or high-scale environments (post-MVP, see PRD section 18.6).

### Verification

Every downloaded binary is verified before installation:

1. **cosign signature** — cryptographic proof the binary was built by the aiPanel CI pipeline.
2. **SHA-256 checksum** — integrity verification against the published checksum file.

If either check fails, the update is rejected and an alert is logged.

### Rollback

The previous binary is always preserved locally. If an update causes issues:

```bash
# Rollback to previous version
aipanel rollback

# Rollback to a specific version
aipanel rollback --version v0.4.1
```

Rollback restores the previous binary and runs any necessary reverse database migrations. Target: rollback completes in under 15 minutes (NFR-VER-004).

---

## 6. Maintenance Windows

### Default behavior

By default, no maintenance window is configured. Security patches apply immediately.

### Configuring a maintenance window

Admins can define a preferred window for non-critical updates:

```bash
# Set maintenance window: Sunday 02:00-06:00 server time
aipanel config set update.maintenance-window "Sun 02:00-06:00"

# Set multiple windows
aipanel config set update.maintenance-window "Sat 03:00-05:00,Sun 02:00-06:00"
```

Also configurable from **Settings > Updates > Maintenance Window** in the UI.

### Behavior

- **Non-critical updates** (minor, patch without security flag): queued until the next maintenance window.
- **Security patches**: bypass the maintenance window and apply immediately by default.
- **Emergency override**: the admin can configure security patches to also respect the maintenance window (not recommended):
  ```bash
  aipanel config set update.security-bypass-window false
  ```

### How queued updates work

1. Update is detected and downloaded.
2. Preflight checks run immediately (compatibility, health, resources).
3. Binary is staged but not activated.
4. At the start of the next maintenance window, the update is applied automatically.
5. Post-update health checks run; auto-rollback triggers if thresholds are exceeded.

---

## 7. Migration and Breaking Changes

### Database migrations

- Managed by **goose** (Go migration tool).
- Migrations run automatically before the new binary starts serving requests.
- Migration files are embedded in the binary — no external dependencies.
- Each migration is idempotent and includes a rollback function.

```
migrations/
  001_initial_schema.sql
  002_add_audit_index.sql
  003_version_manager_tables.sql
```

### Configuration migrations

- Config schema is versioned (e.g., `config_version: 3`).
- When the panel starts with an older config version, automatic transformation runs:
  - Old keys are mapped to new keys.
  - Removed keys are archived to `config.backup.json`.
  - New required keys receive documented defaults.

### Breaking change documentation

All breaking changes are documented in two places:

- **CHANGELOG.md** — entry under the relevant version with a `BREAKING` label.
- **UPGRADING.md** — step-by-step migration instructions per major version.

Example UPGRADING.md entry:

```markdown
## Upgrading from v1.x to v2.0

### Breaking changes

1. **Config key renamed**: `server.ssl_mode` -> `server.tls.mode`
   - Automatic migration handles this. No action needed.

2. **API endpoint removed**: `POST /api/v1/sites` replaced by `POST /api/v2/sites`
   - Update any automation scripts to use the v2 endpoint.

### Migration steps

1. Run `aipanel update` — migrations apply automatically.
2. Verify with `aipanel doctor`.
```

### Deprecation policy

- Features slated for removal receive a **deprecation warning** for at least **1 minor version** before actual removal.
- Deprecated API endpoints return a `Sunset` header and `Deprecation` header per RFC 8594.
- Deprecated config keys log a warning on startup.

---

## 8. Changelog and Communication

### CHANGELOG.md

Generated from **conventional commits**:

```
feat(sites): add bulk domain import
fix(backup): correct timezone handling in scheduled backups
perf(dashboard): reduce P95 load time from 1.4s to 0.9s
security(auth): patch brute-force bypass in MFA flow
BREAKING(config): rename ssl_mode to tls.mode
```

The changelog is generated automatically during the release process using commit metadata.

### GitHub Releases

Each release includes:

- Human-readable release notes summarizing changes by category (features, fixes, security, breaking).
- Links to relevant issues and PRs.
- Signed binaries for supported architectures (linux/amd64, linux/arm64).
- SHA-256 checksum file.
- cosign signature bundle.

### In-panel notifications

When the update check detects a new version:

- A non-intrusive banner appears at the top of the dashboard.
- The banner includes a summary of changes (pulled from release notes).
- One-click access to full release notes and the update action.
- Security updates are highlighted with the danger state indicator.

Notification preferences are configurable per admin user in **Settings > Notifications**.
