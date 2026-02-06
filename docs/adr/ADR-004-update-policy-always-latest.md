# ADR-004: "Always Latest" Update Policy with Canary Rollout and Auto-Rollback

- **Status:** Accepted
- **Date:** 2026-02-06

## Context

aiPanel manages infrastructure components (Nginx, PHP-FPM, MariaDB, PostgreSQL, cert manager, backup tools) and hosted applications on behalf of the administrator. Keeping these components up-to-date is critical for:

1. **Security** — unpatched components are the leading cause of server compromise. The PRD mandates critical CVE patching within 48 hours (NFR-SEC-006) and automatic security updates within 24 hours (NFR-VER-003).
2. **Compliance** — hosting panels that fall behind on updates lose trust. Admins need clear visibility into version compliance status.
3. **Reliability** — updates must not break running sites. Automated rollback within 15 minutes is required (NFR-VER-004) with a 98% success rate target (NFR-VER-005).

Traditional hosting panels either ignore updates (leaving servers vulnerable) or require manual intervention for every update (creating operational burden). aiPanel needs a middle ground: automated by default, safe by design, with full admin control when needed.

## Decision

### Default policy: latest stable

The panel's default update policy is **"latest stable by default."** Every managed component targets the latest stable version from its official upstream source. This is not a blind auto-update — it is a governed pipeline with safety gates at every stage.

### Update channels

Each managed component can be assigned to one of three update channels:

| Channel | Behavior |
|---------|----------|
| **Latest stable** (default) | Target the newest stable release. Security patches applied within 24h. Minor/major updates applied after canary validation. |
| **Conservative** | Delay major version adoption until the latest stable channel has validated it for a configurable period (default: 7 days). Minor and patch updates follow normal pipeline. |
| **Rapid** | Fastest path to new stable versions. Reduced canary observation period. Intended for non-production or testing environments. |

### Update policy modes per component

Each component's update behavior is controlled by a policy mode:

| Mode | Behavior |
|------|----------|
| **manual** | No automatic updates. Admin is notified of available updates and must explicitly trigger them. |
| **auto-patch** | Patch versions (e.g., 8.3.5 -> 8.3.7) are applied automatically through the full pipeline. Minor and major updates require manual approval. |
| **auto-minor** | Patch and minor versions are applied automatically. Major updates require manual approval. |
| **auto-major** | All updates are applied automatically, including major versions, provided the compatibility matrix passes and canary validation succeeds. |

Default policy per component type:
- Security patches: `auto-patch` (always, regardless of component policy)
- Infrastructure components (Nginx, PHP-FPM, DB): `auto-minor`
- Panel self-updates: `auto-minor`
- Managed applications: `auto-patch`

### Update pipeline

The update pipeline is a 10-stage process executed by the Version Manager module:

```
1. Feed sync (every 6h)
   ↓
2. Policy evaluation (target version determination)
   ↓
3. Scheduler check (maintenance window + pin/hold)
   ↓
4. Preflight checks (compatibility matrix + health + resources)
   ↓
5. Artifact download + signature/checksum validation
   ↓
6. Snapshot creation (files + DB + config → rollback point)
   ↓
7. Canary rollout (limited scope, e.g., 10% of affected sites)
   ↓
8. Wave rollout (progressive expansion: 25% → 50% → 100%)
   ↓
9. Post-check (endpoint tests, error rate, performance metrics)
   ↓
10. Auto-rollback (if thresholds breached) or completion
```

#### Stage 1: Feed sync

- Metadata for new versions is fetched from official upstream sources every 6 hours.
- Only signed or checksum-verified metadata is accepted.
- New versions are visible in the panel within 24 hours of upstream publication (NFR-VER-002).

#### Stage 2: Policy evaluation

- The Policy Engine compares each component's installed version against the latest available version.
- It applies the component's policy mode (manual/auto-patch/auto-minor/auto-major) and channel to determine the target version.
- Components with `manual` policy are flagged as "update available" but not processed further.

#### Stage 3: Scheduler check

- Updates are only executed during configured maintenance windows (per-server or per-account).
- Exception: security patches bypass maintenance windows (configurable opt-out for extreme cases).
- Pinned/held components are skipped (see pin/hold section below).

#### Stage 4: Preflight checks

Before any update is applied, the preflight module validates:
- **Compatibility matrix:** PHP extensions, DB version compatibility, runtime dependencies.
- **Service health:** All related services must be healthy (green) before proceeding.
- **Resource availability:** Sufficient disk space for snapshot + new artifacts, adequate CPU/RAM headroom.
- **Configuration conflicts:** Template drift detection for managed config files.

If any preflight check fails, the update is blocked, the admin is notified, and the failure is logged.

#### Stage 5: Artifact download and validation

- Artifacts are downloaded from official upstream sources (direct-upstream model in MVP).
- Every artifact must pass cryptographic signature verification and checksum validation (FR-022).
- Invalid artifacts are rejected and the download is retried from a different mirror if available.
- Successfully validated artifacts are cached locally for rollback purposes (minimum: current version + one previous version).

#### Stage 6: Snapshot creation

- The Snapshot Engine creates a rollback point capturing:
  - Configuration files for the affected component.
  - Database state (if relevant).
  - The current binary/package version.
- Snapshots are stored locally with a TTL-based retention policy.

#### Stage 7: Canary rollout

- For components affecting multiple hosted sites (e.g., PHP version upgrade), the update is first applied to a canary group (default: 10% of affected sites, minimum 1).
- The canary group is observed for a configurable period (default: 15 minutes for patches, 1 hour for minor/major).
- Health checks run continuously during the observation period.

#### Stage 8: Wave rollout

- After successful canary validation, the update progressively expands:
  - Wave 1: 25% of remaining sites.
  - Wave 2: 50% of remaining sites.
  - Wave 3: 100% of remaining sites.
- Each wave has its own observation period and health check gate.
- The rollout pauses automatically if error rates exceed thresholds.

#### Stage 9: Post-check

After full rollout, the post-check module validates:
- All affected endpoints respond correctly (HTTP 200 on health endpoints).
- Error rate has not increased beyond the configured threshold.
- P95 response time has not degraded beyond the configured threshold.
- All managed services are healthy.

#### Stage 10: Auto-rollback

If post-checks fail or any hard threshold is breached during rollout:
- The update is automatically rolled back using the snapshot from Stage 6.
- Rollback must complete within 15 minutes (NFR-VER-004).
- The incident is recorded in the audit log with full details (component, version, failure reason, rollback time).
- A loop-prevention mechanism prevents re-attempting the same update without admin intervention.

### Pin/hold with TTL

Administrators can pin a component to a specific version:

- **Pin:** Freezes a component at its current version. No updates are applied.
- **Hold duration (TTL):** Pins expire after a configurable time (default: 30 days). This prevents forgotten pins from leaving components permanently outdated.
- **Unpin:** Manually removes the pin, allowing the normal update pipeline to resume.
- **Force update:** Overrides the current version with the target version, bypassing the normal pipeline schedule (but still running preflight and snapshot).

### Signature and checksum validation

Every update artifact must pass validation before installation (FR-022):

1. **Package signatures:** apt packages are verified through official GPG-signed repositories.
2. **Panel artifacts:** aiPanel's own updates are signed with the project's release key.
3. **Checksum verification:** SHA-256 checksums are verified for all downloaded artifacts.
4. **Rejection on failure:** Any artifact that fails signature or checksum validation is rejected, the download is logged as a security event, and the admin is alerted.

### Desired State / Observed State model

Each managed component maintains two state records:

**Desired State:**
- `channel` — latest-stable / conservative / rapid
- `policy_mode` — manual / auto-patch / auto-minor / auto-major
- `target_version` — computed by Policy Engine
- `pinned_until` — timestamp (null if not pinned)
- `maintenance_window` — cron expression for allowed update times

**Observed State:**
- `installed_version` — currently running version
- `update_status` — up-to-date / pending / in-progress / rolling-back / failed
- `compliance_status` — up-to-date / lagging / unsupported
- `last_check_at` — timestamp of last feed sync
- `last_update_at` — timestamp of last successful update
- `rollback_version` — version available for immediate rollback

## Consequences

### Positive

- **Security posture** — critical security patches are applied automatically within 24 hours, dramatically reducing the window of vulnerability compared to manual update management.
- **Reduced operational burden** — administrators do not need to manually track, evaluate, and apply updates for every component. The pipeline handles the routine work.
- **Safe by design** — the canary/wave rollout with automatic rollback ensures that even a bad update affects only a small percentage of sites before being caught and reversed.
- **Full visibility** — the compliance dashboard shows the exact version status of every component with clear up-to-date/lagging/unsupported indicators.
- **Admin control preserved** — despite automation, administrators retain full control through policy modes, maintenance windows, pin/hold, and force update capabilities.
- **Auditability** — every update action (attempt, success, failure, rollback) is recorded in the audit log with full context.

### Negative

- **Pipeline complexity** — the 10-stage pipeline is the most complex subsystem in the panel. It requires thorough testing across all stages, including failure injection for rollback validation.
- **Observation delay** — canary and wave rollout add latency between update availability and full deployment. A security patch for a critical CVE may take 2-4 hours to fully roll out to all sites (canary observation + waves). Mitigation: security patches use reduced observation periods.
- **Snapshot storage** — maintaining rollback snapshots for every component consumes disk space. Mitigation: TTL-based retention with configurable limits.
- **False positives in post-checks** — health check flaps unrelated to the update could trigger unnecessary rollbacks. Mitigation: configurable thresholds with baseline comparison rather than absolute values.
- **Pin expiration surprises** — administrators who set pins and forget about the TTL may be surprised when updates resume. Mitigation: the panel sends alerts before pin expiration.

## Alternatives Considered

### Manual-only updates

Requiring all updates to be manually triggered and approved would give administrators maximum control. However:
- Most administrators do not monitor upstream releases daily.
- Security patches would be delayed by days or weeks, creating vulnerability windows.
- The PRD explicitly requires NFR-VER-003 (automatic security updates within 24h).
- The operational burden would negate one of the panel's core value propositions.

### Unattended-upgrades (Debian native)

Debian's `unattended-upgrades` package provides automatic security updates for apt packages. However:
- It only covers Debian packages, not PHP runtime versions, application updates, or panel self-updates.
- It has no canary/wave rollout mechanism — updates are applied to all packages simultaneously.
- It has no integrated rollback — if an update breaks a service, manual intervention is required.
- It cannot coordinate with the panel's maintenance windows or per-component policies.
- The panel uses `unattended-upgrades` for OS-level security patches but manages all panel-tracked components through its own pipeline.

### Pull-based update checks (on-demand only)

Checking for updates only when an administrator opens the dashboard would reduce complexity. However:
- NFR-VER-001 requires automatic catalog refresh at least every 6 hours.
- NFR-VER-002 requires new versions to be visible within 24 hours.
- On-demand checks would violate both requirements and leave administrators unaware of critical patches.

### Single-stage update (no canary/wave)

Applying updates to all affected sites simultaneously would be faster and simpler. However:
- A bad update would affect 100% of sites immediately.
- Rollback would be more disruptive (all sites down during rollback vs. only the canary group).
- The canary/wave model is explicitly required by the PRD (FR-019) and is a core safety mechanism.
