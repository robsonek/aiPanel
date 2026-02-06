# Backup / Restore Contract — aiPanel

> Authoritative contract for the Backup/Restore module of aiPanel.
> Derived from PRD v0.7 (NFR-REL-003, NFR-REL-004, FR-007, FR-018) and ADR-002 (SQLite WAL backup).

---

## 1. RPO / RTO Targets

| Metric | MVP Target | Source |
|--------|-----------|--------|
| **RPO** (Recovery Point Objective) | <= 24 hours | NFR-REL-004 |
| **RTO** (Recovery Time Objective) | <= 60 minutes | NFR-REL-004 |

### What Counts as "Recovered"

A restore is considered complete when **all** of the following are true:

- [ ] Site files (document root) are present and intact
- [ ] Associated databases are restored and connectable
- [ ] Panel configuration is restored (SQLite databases, adapter configs)
- [ ] TLS certificates are in place and valid
- [ ] The site responds to HTTP(S) requests with expected content
- [ ] Health check passes (see Section 6)

---

## 2. Backup Scope

### 2.1 Per-Site Backup

| Component | What Is Included | Notes |
|-----------|-----------------|-------|
| Site files | Entire document root directory | Excludes cache/temp/logs (see Section 5) |
| Database(s) | All databases associated with the site | MariaDB and/or PostgreSQL depending on setup |
| Nginx vhost config | `/etc/nginx/sites-available/<site>.conf` | Includes any included snippets |
| PHP-FPM pool config | `/etc/php/<version>/fpm/pool.d/<site>.conf` | Per-site pool definition |
| TLS certificates | Certificate + private key + chain | Let's Encrypt certs + ACME account data |

### 2.2 Panel Backup

| Component | What Is Included | Notes |
|-----------|-----------------|-------|
| `panel.db` | Config, sessions, version states | WAL checkpoint required before copy (ADR-002) |
| `audit.db` | Append-only audit log | WAL checkpoint required before copy |
| `queue.db` | Job queue state | WAL checkpoint required before copy |
| Panel config | Main panel configuration file(s) | Encrypted secrets remain encrypted |
| System adapter configs | Adapter-specific config files | Nginx global, PHP global, firewall rules, fail2ban jails |

### 2.3 Full Server Backup

Combines **all per-site backups** + **panel backup** into a single restorable archive. Used for full disaster recovery or server migration.

---

## 3. Backup Types

### 3.1 Scheduled Backups

- Triggered by cron (managed by the panel's job queue).
- Configurable per site: **daily**, **weekly**, or **monthly**.
- Default schedule: **daily at 02:00 server time**.
- Panel backup: **daily** (NFR-REL-003).

### 3.2 On-Demand Backups

- Triggered from the UI (**Backup & Restore** screen) or via the internal API.
- No rate limit in MVP, but queued through the job runner (no parallel backups of the same site).

### 3.3 Pre-Operation Backups (FR-018)

Automatic backup created **before** any risky operation:

- [ ] Panel update / self-update
- [ ] PHP version change for a site
- [ ] Database engine upgrade
- [ ] Major config change (Nginx global, firewall rules)
- [ ] Managed app update (recipe update)

These backups are tagged `pre-operation` with a reference to the triggering operation for traceability.

### 3.4 Retention Policy

| Tier | Default Retention | Configurable |
|------|-------------------|-------------|
| Daily | **7** most recent | Yes |
| Weekly | **4** most recent | Yes |
| Monthly | **3** most recent | Yes |
| Pre-operation | Kept until the **next successful scheduled backup** after the operation completes | Yes |

Retention is enforced by a cleanup job that runs after each successful backup.

---

## 4. Backup Storage

### 4.1 Local Storage (MVP)

| Setting | Default | Configurable |
|---------|---------|-------------|
| Base path | `/var/backups/aipanel/` | Yes |
| Directory structure | `<base>/<site-slug>/<date>-<type>/` | Fixed |
| Panel backups | `<base>/_panel/<date>/` | Fixed |
| Permissions | `root:aipanel 0750` | Fixed |

### 4.2 Remote Storage (Post-MVP)

| Backend | Status | Protocol |
|---------|--------|----------|
| S3-compatible (AWS S3, MinIO, R2) | Planned | S3 API |
| SFTP | Planned | SFTP/SSH |

Remote backends will use the same archive format and manifest as local storage.

### 4.3 Encryption

| Property | Value |
|----------|-------|
| Algorithm | AES-256-GCM |
| Scope | Entire backup archive (tar.gz encrypted as a unit) |
| Key storage | Panel config (`panel.db`), encrypted at rest |
| Key rotation | Manual in MVP; automated rotation post-MVP |
| Encryption | **Enabled by default** — can be disabled per backup policy |

---

## 5. Backup Procedure (Technical)

### 5.1 SQLite Databases (ADR-002)

```
1. PRAGMA wal_checkpoint(TRUNCATE)   -- flush WAL to main DB file
2. Copy DB file using sqlite3_backup API (preferred)
   OR: file-level copy after checkpoint (acceptable fallback)
3. Verify copied DB: PRAGMA integrity_check
```

**Inconsistent copies (file copy without checkpoint) are prohibited.**

### 5.2 MariaDB

```bash
mysqldump \
  --single-transaction \
  --routines \
  --triggers \
  --events \
  --databases <db1> <db2> ... \
  > dump.sql
```

- `--single-transaction` ensures consistent snapshot without locking.
- Output: SQL dump file per site (or combined for full server backup).

### 5.3 PostgreSQL

```bash
pg_dump \
  --format=custom \
  --no-owner \
  --no-acl \
  <database> \
  > dump.pgcustom
```

- `--format=custom` enables selective restore and compression.
- Output: one `.pgcustom` file per database.

### 5.4 Files

```bash
tar czf site-files.tar.gz \
  --exclude='*/cache/*' \
  --exclude='*/tmp/*' \
  --exclude='*/logs/*' \
  --exclude='*.log' \
  --exclude='*/node_modules/*' \
  --exclude='*/vendor/*/.git/*' \
  -C /home/<user>/sites/ <site-slug>/
```

Additional excludes are configurable per site via the panel UI.

### 5.5 Manifest

Every backup produces a `manifest.json`:

```json
{
  "version": "1.0",
  "type": "site|panel|full",
  "site_slug": "example-com",
  "created_at": "2026-02-06T02:00:00Z",
  "trigger": "scheduled|on-demand|pre-operation",
  "operation_ref": "op-uuid (if pre-operation)",
  "panel_version": "0.1.0",
  "php_version": "8.3.7",
  "db_engine": "mariadb|postgresql|both",
  "components": [
    {
      "name": "site-files",
      "file": "site-files.tar.gz",
      "sha256": "abc123...",
      "size_bytes": 104857600
    },
    {
      "name": "database-mariadb",
      "file": "dump.sql.gz",
      "sha256": "def456...",
      "size_bytes": 5242880
    },
    {
      "name": "nginx-vhost",
      "file": "nginx-vhost.conf",
      "sha256": "ghi789...",
      "size_bytes": 2048
    },
    {
      "name": "php-fpm-pool",
      "file": "php-fpm-pool.conf",
      "sha256": "jkl012...",
      "size_bytes": 1024
    },
    {
      "name": "tls-certs",
      "file": "tls-certs.tar.gz",
      "sha256": "mno345...",
      "size_bytes": 8192
    }
  ],
  "checksum_algorithm": "sha256",
  "encrypted": true,
  "manifest_sha256": "pqr678..."
}
```

### 5.6 Atomicity

A backup is marked **valid** only when:

1. All components listed in the manifest are written successfully.
2. SHA-256 checksums of all component files match the manifest.
3. The manifest itself is written and its own checksum is recorded.

If any step fails, the backup is marked **failed**, the partial artifacts are cleaned up, and an alert is raised.

---

## 6. Restore Contract

### 6.1 Restore Targets

| Target | Description |
|--------|-------------|
| Same server | Restore in-place on the server that created the backup |
| New server | Restore onto a clean Debian 13 server with aiPanel installed |

### 6.2 Restore Granularity

| Level | What Is Restored |
|-------|-----------------|
| **Full site** | Files + DB + Nginx vhost + PHP-FPM pool + TLS certs |
| **Database only** | Database dump restored; files untouched |
| **Files only** | Document root restored; database untouched |
| **Panel config only** | Panel SQLite DBs + panel config + adapter configs |

### 6.3 Pre-Restore Validation

Before any restore operation executes, the following checks **must** pass:

| Check | Description | Failure Action |
|-------|-------------|----------------|
| Checksum verification | All component SHA-256 hashes match manifest | Abort restore |
| Disk space | Available space >= 2x backup size (for rollback margin) | Abort restore |
| PHP version compatibility | Target server has the PHP version recorded in manifest | Warn + require confirmation |
| DB engine compatibility | Target server runs the same DB engine as the backup | Abort restore |
| Panel version compatibility | Target panel version >= backup panel version | Warn + require confirmation |
| Encryption key | Decryption key is available and valid | Abort restore |

### 6.4 Restore Wizard (UI Flow)

```
Step 1: Select Backup Point
  └─ List available backups (date, type, size, status)
  └─ Filter by site, date range, trigger type

Step 2: Choose Restore Scope
  └─ Full site / DB only / Files only / Panel config
  └─ Show what will be overwritten

Step 3: Preview & Confirm
  └─ Display: components to restore, current vs. backup versions
  └─ Display: pre-restore validation results (all checks green)
  └─ Require explicit confirmation (modal with typed confirmation for destructive restores)

Step 4: Execute
  └─ Put site into maintenance mode
  └─ Create pre-restore snapshot (for rollback)
  └─ Restore components in order: config → DB → files → TLS
  └─ Run post-restore health check

Step 5: Result
  └─ Success: remove maintenance mode, show summary
  └─ Failure: auto-rollback to pre-restore state, show error details
```

### 6.5 During Restore

- The target site is placed into **maintenance mode** (Nginx returns 503 with a maintenance page).
- A **pre-restore snapshot** is taken of the current state before overwriting anything.
- Restore operations are logged in the audit trail with full details.

### 6.6 Post-Restore Health Check

After restore completes, the following checks run automatically:

- [ ] Site responds to HTTP request (status 200 on `/` or configured health endpoint)
- [ ] Database connection succeeds (SELECT 1)
- [ ] TLS certificate is valid and matches the domain
- [ ] PHP-FPM pool is running and responsive
- [ ] Nginx config test passes (`nginx -t`)

### 6.7 Rollback on Failed Restore

If any post-restore health check fails:

1. The restore is marked **failed**.
2. Automatic rollback restores the **pre-restore snapshot**.
3. Maintenance mode is removed after rollback.
4. An alert is raised with the failure reason.
5. The failed restore attempt is recorded in the audit log.

---

## 7. Dry-Run Restore Test as Release Gate

### 7.1 Requirement

Every aiPanel release **must** pass a dry-run backup/restore test before being published.

### 7.2 Test Procedure

```
1. Provision a test site (files + DB + TLS + config)
2. Create a full backup of the test site
3. Destroy the test site completely (rm files, drop DB, remove configs)
4. Restore from the backup created in step 2
5. Run the full post-restore health check (Section 6.6)
6. Validate: site serves correct content, DB data intact, TLS valid
```

### 7.3 CI Integration

| Property | Value |
|----------|-------|
| Schedule | **Nightly** (cron in CI pipeline) |
| Environment | Dedicated VM (Debian 13, clean install + aiPanel) |
| DB engines tested | MariaDB **and** PostgreSQL (matrix) |
| PHP versions tested | All supported versions |
| Failure behavior | **Release blocker** — pipeline fails, release is not published |

### 7.4 Metrics Tracked per Run

| Metric | Tracked | Alert Threshold |
|--------|---------|----------------|
| Backup size (bytes) | Yes | > 2x previous run (unexpected growth) |
| Backup duration (seconds) | Yes | > 300s for test site |
| Restore duration (seconds) | Yes | > RTO target (60 min) |
| Health check pass/fail | Yes | Any failure = release blocked |
| Checksum validation | Yes | Any mismatch = release blocked |

---

## 8. Monitoring and Alerts

### 8.1 Alert Rules

| Condition | Severity | Channel |
|-----------|----------|---------|
| Scheduled backup **failed** | Critical | Dashboard + notification |
| No backup exists for a site within **RPO threshold** (24h) | Critical | Dashboard + notification |
| Backup storage usage **> 80%** of allocated capacity | Warning | Dashboard + notification |
| Backup storage usage **> 95%** of allocated capacity | Critical | Dashboard + notification |
| Pre-operation backup failed (blocks the operation) | Critical | Dashboard + notification |
| Restore health check **failed** | Critical | Dashboard + notification |
| Dry-run restore test **failed** in CI | Critical | CI pipeline + team notification |
| Backup checksum mismatch detected during periodic integrity scan | Critical | Dashboard + notification |

### 8.2 Dashboard Widgets (Backup & Restore Screen)

| Widget | Content |
|--------|---------|
| **Last Backup** | Per-site: timestamp, type, status (OK/Failed), size |
| **Next Scheduled** | Per-site: next scheduled backup time |
| **Backup Size Trend** | Chart: backup size over time per site (spot unexpected growth) |
| **Storage Usage** | Total used / allocated, percentage bar |
| **Restore Test Status** | Last dry-run: date, result, duration |
| **Retention Summary** | Per site: number of backups kept (daily/weekly/monthly) |

### 8.3 Periodic Integrity Scan

- Runs **weekly** (configurable).
- Verifies SHA-256 checksums of all stored backups against their manifests.
- Flags corrupted backups and raises an alert.
- Corrupted backups are excluded from the restore selection list.

---

## Appendix A: Backup Directory Structure

```
/var/backups/aipanel/
├── _panel/
│   └── 2026-02-06-scheduled/
│       ├── panel.db
│       ├── audit.db
│       ├── queue.db
│       ├── panel-config.tar.gz
│       ├── adapter-configs.tar.gz
│       └── manifest.json
├── example-com/
│   ├── 2026-02-06-scheduled/
│   │   ├── site-files.tar.gz
│   │   ├── dump.sql.gz
│   │   ├── nginx-vhost.conf
│   │   ├── php-fpm-pool.conf
│   │   ├── tls-certs.tar.gz
│   │   └── manifest.json
│   ├── 2026-02-05-scheduled/
│   │   └── ...
│   └── 2026-02-04-pre-operation/
│       └── ...
└── my-app-net/
    └── ...
```

## Appendix B: Restore Order of Operations

For a full site restore, components are restored in this specific order to avoid dependency issues:

| Step | Component | Reason |
|------|-----------|--------|
| 1 | Nginx vhost config | Must be in place before reload |
| 2 | PHP-FPM pool config | Must be in place before pool restart |
| 3 | TLS certificates | Must be in place before Nginx reload with SSL |
| 4 | Database | Data must be available before site serves requests |
| 5 | Site files | Document root restored last (largest component) |
| 6 | Reload services | `nginx -t && systemctl reload nginx && systemctl restart php-fpm` |
| 7 | Remove maintenance mode | Site goes live |
| 8 | Post-restore health check | Final validation |

## Appendix C: Checklist for Adding Remote Storage Backend (Post-MVP)

- [ ] Implement storage adapter interface (local is the reference implementation)
- [ ] Add S3-compatible backend (PutObject, GetObject, ListObjects, DeleteObject)
- [ ] Add SFTP backend (upload, download, list, delete)
- [ ] Support mixed storage: local + remote (write to both, restore from either)
- [ ] Add bandwidth throttling for remote uploads (configurable)
- [ ] Add retry logic with exponential backoff for remote operations
- [ ] Update manifest to include storage location metadata
- [ ] Update restore wizard to show available backups from all configured backends
- [ ] Add monitoring: remote upload success rate, transfer duration
