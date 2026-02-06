# aiPanel Installer Contract

**Version:** 0.1 (draft)
**Date:** 2026-02-06
**Status:** Draft — aligned with PRD v0.7
**Applies to:** aiPanel installer for Debian 13 (Trixie)

---

## 1. Minimum Host Requirements

| Resource | Minimum | Recommended | Notes |
|----------|---------|-------------|-------|
| OS | Debian 13 (Trixie) — clean install, no desktop environment | — | Only supported target OS for MVP |
| CPU | 1 vCPU | 2+ vCPU | Installer itself is not CPU-intensive; recommendation accounts for hosted workloads |
| RAM | 1 GB | 2+ GB | Panel steady-state target: <= 1.5 GB (NFR-PERF-004) |
| Disk | 10 GB free | 20+ GB free | Covers OS + panel + DB engines + initial backups |
| Network | Public IPv4, internet access | — | Required for package installation, TLS issuance (ACME HTTP-01), and feed sync |
| Existing software | No web server, no DB server, no other hosting panel installed | — | Pre-flight checks enforce this (see Section 2) |

**Not supported (installer will abort):**

- Any OS other than Debian 13
- Systems with an active desktop environment (GNOME, KDE, XFCE, etc.)
- Containers (Docker, LXC) — not validated for MVP
- OpenVZ virtualization (missing kernel features for nftables)

---

## 2. Definition of "Clean Debian 13"

A system is considered "clean" when **all** of the following are true:

| Condition | How the installer validates it |
|-----------|-------------------------------|
| Debian 13 (Trixie) release | Parse `/etc/os-release`: `VERSION_CODENAME=trixie` |
| No third-party APT repositories | Scan `/etc/apt/sources.list` and `/etc/apt/sources.list.d/` for non-Debian origins |
| No web server running | Check for listening ports 80/443 (`ss -tlnp`) and known packages (`nginx`, `apache2`, `lighttpd`, `caddy`) |
| No database server running | Check for listening DB ports and known packages (`mariadb-server`, `mysql-server`, `postgresql`) |
| No other hosting panel installed | Check for known panel binaries/services (`cpanel`, `directadmin`, `hestiacp`, `cyberpanel`, `ispconfig`, `webmin`, `froxlor`, `virtualmin`) |
| Root or sudo access | Verify effective UID 0 or `sudo -v` succeeds |
| systemd as init system | Verify PID 1 is systemd (`readlink /proc/1/exe`) |
| Standard system utilities only | Verify default Debian 13 tasksel profile (no `web-server`, `database-server`, `mail-server` tasks selected) |
| Sufficient resources | Check CPU count, total RAM, free disk space against minimums from Section 1 |
| Network connectivity | Resolve and reach `deb.debian.org` and `acme-v02.api.letsencrypt.org` |

**Pre-flight behavior:**

- All checks run before any system modification.
- Each check produces a `PASS` / `FAIL` / `WARN` result.
- Any `FAIL` aborts installation with a clear error message and remediation hint.
- `WARN` results are logged and displayed but do not block installation (e.g., low disk space above minimum but below recommended).

---

## 3. Installation Modes

### 3.1 Interactive Mode (default)

The installer prompts the user for configuration when run without flags:

```bash
curl -fsSL https://get.aipanel.io | bash
# or
./aipanel-installer
```

**Prompts:**

| Prompt | Default | Validation |
|--------|---------|------------|
| Database engine | MariaDB | `mariadb`, `postgresql`, `both` (INS-008) |
| Admin username | `admin` | 3-32 chars, alphanumeric + underscore |
| Admin password | _(generated if skipped)_ | Minimum 12 chars, complexity enforced |
| Admin email | _(required)_ | Valid email format |
| Panel HTTPS port | `8443` | 1024-65535, not in use |
| Panel domain/hostname | Server's FQDN or IP | Valid FQDN or IPv4 |
| Enable Let's Encrypt for panel | `no` (self-signed) | Requires valid FQDN pointing to server |

### 3.2 Non-Interactive Mode (INS-005)

All parameters provided via CLI flags or environment variables. Designed for CI/CD, Ansible, Terraform, and other automation tools.

```bash
./aipanel-installer --non-interactive \
  --db-engine=both \
  --admin-user=admin \
  --admin-password='S3cureP@ss!' \
  --admin-email=admin@example.com \
  --panel-port=8443 \
  --panel-domain=panel.example.com \
  --letsencrypt=yes
```

- If a required parameter is missing, the installer exits with error code `1` and lists missing parameters.
- No stdin prompts are issued.
- Output is machine-parseable (JSON report at the end).

### 3.3 Resume Mode (INS-007)

If the installer is interrupted (crash, network loss, manual Ctrl+C), re-running it detects the incomplete installation and resumes from the last successful checkpoint.

```bash
./aipanel-installer          # resumes automatically
./aipanel-installer --resume # explicit resume
./aipanel-installer --restart # discard progress, start from scratch
```

- Checkpoint state is stored in `/var/lib/aipanel/.installer-state.json`.
- Each completed step writes its checkpoint before the next step begins.
- Resume re-validates the completed steps (lightweight check) before continuing.

---

## 4. Installation Steps

Steps are executed in strict order. Each step has:
- A **pre-condition check** (skip if already satisfied — idempotency).
- A **checkpoint marker** written on success.
- A **defined failure behavior**.

| # | Step | Description | Failure behavior |
|---|------|-------------|-----------------|
| 1 | **Pre-flight validation** | OS, resources, clean state, network (Section 2) | Abort with diagnostic report |
| 2 | **System update** | `apt update && apt upgrade -y` | Abort — network or repo issue |
| 3 | **Add required repositories** | Add Sury PHP repo, aiPanel repo; import GPG keys | Abort — cannot proceed without packages |
| 4 | **Install system packages** | Install: Nginx, PHP-FPM (multiple versions), selected DB engine(s), nftables, fail2ban, certbot dependencies, acl, curl, git, jq, openssl | Abort — dependency resolution failed |
| 5 | **Create system users** | Create `aipanel` service user (nologin); create per-site user template in `/etc/aipanel/skel/` | Abort — permission issue |
| 6 | **Configure nftables** | Apply default ruleset: allow 22 (SSH), 80 (HTTP), 443 (HTTPS), panel port; deny all other inbound; backup original rules | Abort — rollback nftables config |
| 7 | **Configure SSH hardening** | Disable root password login, disable empty passwords, set `MaxAuthTries 3`, configure `AllowGroups aipanel-ssh`; backup original `sshd_config` | Abort — rollback SSH config, warn operator |
| 8 | **Configure fail2ban** | Install jails: `sshd`, `aipanel-auth`; set ban time, find time, max retry; backup original config | Abort — rollback fail2ban config |
| 9 | **Install panel binary** | Download or copy Go single binary to `/usr/local/bin/aipanel`; verify checksum + signature | Abort — integrity check failed |
| 10 | **Initialize SQLite databases** | Create `/var/lib/aipanel/panel.db`, `audit.db`, `queue.db` with WAL mode enabled | Abort — filesystem issue |
| 11 | **Run database migrations** | Apply all pending migrations via goose (embedded in binary) | Abort — migration failure, databases untouched (transaction rollback) |
| 12 | **Configure Nginx** | Write panel vhost (`/etc/nginx/sites-available/aipanel.conf`), default catch-all config; symlink to sites-enabled; `nginx -t && systemctl reload nginx` | Abort — rollback Nginx config |
| 13 | **Configure PHP-FPM** | Write default pool config for each installed PHP version; set `listen`, `user`, `group`, resource limits; restart PHP-FPM | Abort — rollback PHP-FPM config |
| 14 | **Create admin account** | Insert admin user into `panel.db` with bcrypt-hashed password | Abort — DB write failure |
| 15 | **Generate TLS for panel** | Generate self-signed certificate for panel HTTPS; optionally issue Let's Encrypt certificate if `--letsencrypt=yes` and domain is valid | Warn on LE failure — fall back to self-signed, continue |
| 16 | **Start panel service** | Write `/etc/systemd/system/aipanel.service`; `systemctl daemon-reload && systemctl enable --now aipanel` | Abort — service failed to start |
| 17 | **Post-install validation** | Health checks: panel HTTP response, DB connectivity, Nginx status, PHP-FPM status, nftables loaded, fail2ban running | Warn on non-critical failures, abort on panel unreachable |
| 18 | **Generate installation report** | Write JSON + human-readable summary (INS-003) | Warn — report generation is best-effort |
| 19 | **Write installation log** | Flush full output to `/var/log/aipanel/install.log` (INS-006) | Warn — log write is best-effort |

**Total estimated time:** <= 20 minutes on reference hardware (2 vCPU, 2 GB RAM, SSD, 100 Mbps).

---

## 5. Rollback Contract

### 5.1 Checkpoint and Failure Behavior

- Every step (Section 4) writes a checkpoint marker to `/var/lib/aipanel/.installer-state.json` upon success.
- On failure, the installer:
  1. Stops immediately (no further steps).
  2. Logs the error with full context (step number, error message, system state).
  3. Reports which step failed, what the likely cause is, and suggested remediation.
  4. Does **not** automatically roll back completed steps (to avoid making things worse).

### 5.2 Uninstall Command

```bash
aipanel uninstall [--keep-data] [--keep-packages] [--force]
```

| Flag | Behavior |
|------|----------|
| _(no flags)_ | Full removal: panel binary, configs, system users, firewall rules, fail2ban jails, DB engines, SQLite databases |
| `--keep-data` | Remove panel but keep: hosted site files, database data directories, SSL certificates |
| `--keep-packages` | Remove panel configs but keep installed apt packages (Nginx, PHP, DB engines) |
| `--force` | Skip confirmation prompt |

### 5.3 Rollback Scope

| Component | Rolled back by `uninstall` | Notes |
|-----------|---------------------------|-------|
| Panel binary (`/usr/local/bin/aipanel`) | Yes | Removed |
| Panel SQLite databases (`/var/lib/aipanel/`) | Yes (unless `--keep-data`) | Removed or preserved |
| Panel systemd service | Yes | Stopped, disabled, unit file removed |
| Nginx panel vhost | Yes | Removed; original config restored from backup |
| Nginx default config | Yes | Restored from backup |
| PHP-FPM pool configs | Yes | Restored from backup |
| nftables rules | Yes | Restored from pre-install backup |
| SSH hardening (`sshd_config`) | Yes | Restored from pre-install backup |
| fail2ban jails | Yes | Removed; original config restored from backup |
| `aipanel` system user | Yes | Removed |
| Per-site system users | Yes (unless `--keep-data`) | Removed with home directories, or preserved |
| APT packages installed by installer | **No** | Documented — manual removal if needed |
| System updates (`apt upgrade`) | **No** | Cannot be safely reversed |
| Third-party APT repositories added | Yes | Repository files and GPG keys removed |

### 5.4 Config Backup Strategy

Before modifying any system configuration file, the installer:

1. Copies the original to `/var/lib/aipanel/backups/install/<filename>.pre-install`.
2. Records the backup path in the checkpoint state file.
3. On uninstall, restores from these backups.

This satisfies INS-004.

---

## 6. Idempotency Contract

**Requirement references:** INS-002, FR-002

### 6.1 Re-run on Already-Installed System

```
$ ./aipanel-installer
[INFO] Existing aiPanel installation detected (version 1.0.0).
[INFO] Choose an action:
  1. Upgrade to latest version
  2. Repair current installation
  3. Abort
```

- The installer reads `/var/lib/aipanel/.installer-state.json` and `/usr/local/bin/aipanel --version`.
- It will **never** overwrite a working installation without explicit confirmation.

### 6.2 Re-run After Interruption

- Detects incomplete state from checkpoint file.
- Re-validates all completed steps (lightweight — checks artifacts exist, configs are correct).
- Resumes from the first incomplete step.
- If a completed step's artifacts are missing or corrupt, re-executes that step.

### 6.3 Idempotency Guarantees

| Guarantee | Implementation |
|-----------|---------------|
| Users are never duplicated | Check `id <username>` before `useradd` |
| Firewall rules are never duplicated | Flush and rewrite the aiPanel nftables table, not append |
| Nginx configs are never duplicated | Write to known file paths (overwrite, not append) |
| PHP-FPM pools are never duplicated | Write to known file paths per PHP version |
| fail2ban jails are never duplicated | Write to `/etc/fail2ban/jail.d/aipanel.conf` (overwrite) |
| SQLite databases are not wiped on re-run | Migrations are additive; `goose` tracks applied versions |
| APT repositories are not added twice | Check existence before adding |
| systemd service is not duplicated | Overwrite unit file + `daemon-reload` |

### 6.4 Pre-Condition Pattern

Every step follows this pattern:

```
function executeStep(step):
    if step.preConditionMet():
        log("Step already satisfied, skipping")
        markCheckpoint(step)
        return
    step.execute()
    step.verify()
    markCheckpoint(step)
```

---

## 7. Output Artifacts

### 7.1 Installation Report (INS-003)

**Location:** `/var/lib/aipanel/install-report.json` + summary printed to stdout.

**JSON structure:**

```json
{
  "version": "1.0.0",
  "installed_at": "2026-02-06T14:32:00Z",
  "duration_seconds": 487,
  "os": "Debian 13 (trixie)",
  "mode": "interactive",
  "steps": [
    {
      "number": 1,
      "name": "preflight_validation",
      "status": "passed",
      "duration_ms": 1200
    }
  ],
  "components": {
    "nginx": "1.27.x",
    "php_versions": ["8.3.x", "8.4.x"],
    "db_engine": "mariadb",
    "db_version": "11.x",
    "panel": "1.0.0"
  },
  "network": {
    "panel_url": "https://203.0.113.10:8443",
    "panel_domain": "panel.example.com",
    "tls_type": "self-signed"
  },
  "security": {
    "firewall": "nftables active",
    "fail2ban": "active",
    "ssh_hardening": "applied"
  }
}
```

**Human-readable summary** (printed to stdout at the end):

```
=== aiPanel Installation Complete ===

  Panel URL:      https://panel.example.com:8443
  Admin user:     admin
  Admin password: <displayed once, not stored in plaintext>

  Nginx:          1.27.x
  PHP:            8.3.x, 8.4.x
  Database:       MariaDB 11.x
  Firewall:       nftables (ports 22, 80, 443, 8443)
  fail2ban:       active
  TLS:            self-signed (run: aipanel tls setup --letsencrypt)

  Full report:    /var/lib/aipanel/install-report.json
  Install log:    /var/log/aipanel/install.log

  Next steps:
    1. Log in to the panel
    2. Change admin password
    3. Add your first site
```

### 7.2 Installation Log (INS-006)

**Location:** `/var/log/aipanel/install.log`

- Full verbose output of every command executed.
- Timestamps for each line.
- Log level annotations: `[INFO]`, `[WARN]`, `[ERROR]`, `[DEBUG]`.
- Rotated on subsequent installs (previous log moved to `install.log.1`).

### 7.3 Admin Credentials

- Displayed once on stdout at installation end.
- If auto-generated, the password is shown once and **never stored in plaintext**.
- Password hash (bcrypt) stored in `panel.db`.
- The installation report does **not** contain the password.

### 7.4 systemd Service

**Unit file:** `/etc/systemd/system/aipanel.service`

```ini
[Unit]
Description=aiPanel Hosting Control Panel
After=network-online.target nginx.service
Wants=network-online.target

[Service]
Type=simple
User=aipanel
Group=aipanel
ExecStart=/usr/local/bin/aipanel serve
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
Environment=AIPANEL_CONFIG=/etc/aipanel/config.toml

[Install]
WantedBy=multi-user.target
```

---

## 8. Environment Variables and CLI Flags

All configuration options for non-interactive mode (INS-005).

| CLI Flag | Environment Variable | Type | Default | Required | Description |
|----------|---------------------|------|---------|----------|-------------|
| `--non-interactive` | `AIPANEL_NON_INTERACTIVE=1` | bool | `false` | No | Enable non-interactive mode; abort on missing required params |
| `--db-engine` | `AIPANEL_DB_ENGINE` | string | `mariadb` | No | Database engine to install: `mariadb`, `postgresql`, `both` |
| `--admin-user` | `AIPANEL_ADMIN_USER` | string | `admin` | No | Admin account username |
| `--admin-password` | `AIPANEL_ADMIN_PASSWORD` | string | _(auto-generated)_ | No | Admin account password (min 12 chars). If omitted in non-interactive mode, a secure random password is generated |
| `--admin-email` | `AIPANEL_ADMIN_EMAIL` | string | — | **Yes** (non-interactive) | Admin account email address |
| `--panel-port` | `AIPANEL_PANEL_PORT` | int | `8443` | No | HTTPS port for the panel UI |
| `--panel-domain` | `AIPANEL_PANEL_DOMAIN` | string | _(server FQDN or IP)_ | No | Domain or hostname for the panel |
| `--letsencrypt` | `AIPANEL_LETSENCRYPT` | bool | `false` | No | Request a Let's Encrypt certificate for the panel during install |
| `--le-email` | `AIPANEL_LE_EMAIL` | string | _(admin email)_ | No | Email for Let's Encrypt registration (defaults to admin email) |
| `--ssh-port` | `AIPANEL_SSH_PORT` | int | `22` | No | SSH port to allow in firewall rules |
| `--skip-system-update` | `AIPANEL_SKIP_SYSTEM_UPDATE=1` | bool | `false` | No | Skip `apt update/upgrade` (use when system is already up to date) |
| `--php-versions` | `AIPANEL_PHP_VERSIONS` | string | `8.3,8.4` | No | Comma-separated list of PHP versions to install |
| `--resume` | `AIPANEL_RESUME=1` | bool | `false` | No | Explicitly resume interrupted installation |
| `--restart` | — | bool | `false` | No | Discard previous progress and start from scratch |
| `--log-level` | `AIPANEL_LOG_LEVEL` | string | `info` | No | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--log-file` | `AIPANEL_LOG_FILE` | string | `/var/log/aipanel/install.log` | No | Path to installation log file |
| `--report-file` | `AIPANEL_REPORT_FILE` | string | `/var/lib/aipanel/install-report.json` | No | Path to JSON installation report |

**Precedence:** CLI flags override environment variables. Environment variables override defaults.

**Validation in non-interactive mode:**

```
$ AIPANEL_NON_INTERACTIVE=1 ./aipanel-installer
[ERROR] Missing required parameter: --admin-email (or AIPANEL_ADMIN_EMAIL)
[ERROR] Non-interactive mode requires all mandatory parameters.
Exit code: 1
```

---

## Appendix A: File System Layout

| Path | Purpose |
|------|---------|
| `/usr/local/bin/aipanel` | Panel binary (Go single binary) |
| `/etc/aipanel/config.toml` | Panel configuration |
| `/etc/aipanel/skel/` | Per-site user skeleton directory |
| `/var/lib/aipanel/panel.db` | Panel config and session database (SQLite WAL) |
| `/var/lib/aipanel/audit.db` | Audit log database (SQLite WAL) |
| `/var/lib/aipanel/queue.db` | Job queue database (SQLite WAL) |
| `/var/lib/aipanel/install-report.json` | Installation report |
| `/var/lib/aipanel/.installer-state.json` | Installer checkpoint state |
| `/var/lib/aipanel/backups/install/` | Pre-install config backups |
| `/var/log/aipanel/install.log` | Installation log |
| `/var/log/aipanel/panel.log` | Panel runtime log |
| `/etc/systemd/system/aipanel.service` | systemd unit file |
| `/etc/nginx/sites-available/aipanel.conf` | Panel Nginx vhost |
| `/etc/fail2ban/jail.d/aipanel.conf` | Panel fail2ban jail |

## Appendix B: Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Installation completed successfully |
| `1` | Invalid arguments or missing required parameters |
| `2` | Pre-flight validation failed (unsupported OS, insufficient resources, dirty state) |
| `3` | Network error (cannot reach repositories or ACME endpoint) |
| `4` | Package installation failed |
| `5` | Configuration error (Nginx, PHP-FPM, nftables, fail2ban, SSH) |
| `6` | Database initialization or migration error |
| `7` | Panel service failed to start |
| `8` | Post-install health check failed |
| `10` | Interrupted — re-run to resume |

## Appendix C: PRD Requirement Traceability

| PRD Requirement | Installer Contract Section |
|----------------|---------------------------|
| FR-001 | Section 2 (pre-flight), Section 4 Step 1 |
| FR-002 | Section 6 (idempotency) |
| INS-001 | Section 2 (clean Debian 13 definition) |
| INS-002 | Section 6 (idempotency) |
| INS-003 | Section 7.1 (installation report) |
| INS-004 | Section 5 (rollback contract) |
| INS-005 | Section 3.2 (non-interactive mode), Section 8 (env vars/flags) |
| INS-006 | Section 7.2 (installation log) |
| INS-007 | Section 3.3 (resume mode) |
| INS-008 | Section 3.1 (DB choice prompt), Section 8 (`--db-engine` flag) |
| NFR-SEC-001 | Section 4 Steps 6-8 (firewall, SSH hardening, fail2ban) |
| NFR-SEC-002 | Section 7.3 (admin credentials — bcrypt, no plaintext storage) |
| NFR-PERF-004 | Section 1 (RAM requirements aligned with steady-state target) |
