# aiPanel Threat Model & Hardening Checklist v1

- **Product**: aiPanel (hosting panel)
- **Version**: 0.7 (aligned with PRD v0.7)
- **Date**: 2026-02-06
- **Status**: Draft
- **Target OS**: Debian 13 (clean install)

---

## 1. Scope

This threat model covers the following assets:

| Asset | Description |
|---|---|
| **Panel API** | Go backend (Chi router, modular monolith) serving internal API for UI and system automation |
| **Panel UI** | React 19 SPA served via Nginx, communicating with the API |
| **Hosted websites** | Customer sites managed by the panel (files, PHP runtime, databases) |
| **User data** | Panel accounts, credentials, RBAC assignments, session tokens, MFA secrets |
| **System configurations** | Nginx vhosts, PHP-FPM pools, nftables rules, fail2ban jails, systemd units |
| **Secrets** | Database passwords, API tokens, TLS private keys, session signing keys, ACME account keys |
| **SQLite databases** | `panel.db` (config, sessions, version state), `audit.db` (append-only audit log), `queue.db` (job queue) |
| **Backups** | Scheduled and on-demand backup archives (files + DB dumps + panel config) |
| **TLS certificates** | Let's Encrypt certificates and private keys for managed domains |

**Out of scope for this version**: multi-node clusters, DNS management, mail services, marketplace plugins, billing, public third-party API.

---

## 2. Trust Boundaries

```
                         ┌──────────────────────────────────────────────────────────┐
                         │                    INTERNET (Untrusted)                   │
                         │   Browsers, bots, scanners, attackers, ACME servers      │
                         └─────────────────────────┬────────────────────────────────┘
                                                   │
                              ─────────── TB1: Network Edge ───────────
                                                   │
                                                   ▼
                         ┌──────────────────────────────────────────────────────────┐
                         │                    NGINX (Reverse Proxy)                  │
                         │   Ports 80/443 — TLS termination, rate limiting,         │
                         │   static file serving, security headers                  │
                         └──────────┬───────────────────────────┬───────────────────┘
                                    │                           │
               ─── TB2: Panel App ──┘                           └── TB3: Hosted Sites ──
                                    │                           │
                                    ▼                           ▼
                  ┌─────────────────────────┐   ┌──────────────────────────────────┐
                  │      Panel Backend      │   │       Hosted Site Runtime         │
                  │   (Go API + Job Queue)  │   │   PHP-FPM pool (per-user)        │
                  │                         │   │   Site files (per-user dir)       │
                  │   Auth / RBAC / MFA     │   │   open_basedir enforced          │
                  │   CSRF / Rate Limiting  │   │                                  │
                  └─────────┬───────────────┘   └──────────────┬───────────────────┘
                            │                                  │
               ─── TB4: Data Layer ──                ─── TB5: DB Access ──
                            │                                  │
                            ▼                                  ▼
                  ┌─────────────────────────┐   ┌──────────────────────────────────┐
                  │   SQLite (WAL mode)     │   │   MariaDB / PostgreSQL           │
                  │   panel.db              │   │   (localhost only)               │
                  │   audit.db              │   │   Per-site DB users              │
                  │   queue.db              │   │   Least-privilege grants         │
                  └─────────────────────────┘   └──────────────────────────────────┘
                            │
               ─── TB6: System Adapters ──
                            │
                            ▼
                  ┌──────────────────────────────────────────────────────────────────┐
                  │                    Managed System Services                       │
                  │   Nginx · PHP-FPM · MariaDB · PostgreSQL · systemd              │
                  │   nftables · fail2ban · apt · Lego (ACME) · backup engine       │
                  └──────────────────────────────────────────────────────────────────┘
                            │
               ─── TB7: OS Boundary ──
                            │
                            ▼
                  ┌──────────────────────────────────────────────────────────────────┐
                  │                    Debian 13 (Kernel / OS)                       │
                  │   cgroups · namespaces · filesystem permissions · SSH            │
                  └──────────────────────────────────────────────────────────────────┘

User Access Flow:
  Browser ──[HTTPS]──► Nginx ──► Panel UI (SPA static)
                                 Panel API ──[auth token]──► RBAC check ──► Operation
                                                                              │
                                                              ┌───────────────┼────────────┐
                                                              ▼               ▼            ▼
                                                         System Adapter   SQLite DB   Job Queue
```

### Trust Boundary Summary

| ID | Boundary | Controls |
|---|---|---|
| TB1 | Internet to Nginx | TLS termination, nftables firewall, fail2ban, rate limiting |
| TB2 | Nginx to Panel App | Reverse proxy to localhost, authentication required |
| TB3 | Nginx to Hosted Sites | Per-site vhost isolation, separate PHP-FPM pools |
| TB4 | Panel App to Data Layer | Application-level access control, repository pattern |
| TB5 | Hosted Site to DB | Per-site DB credentials, localhost-only binding, least privilege |
| TB6 | Panel App to System Adapters | Adapter interface contracts, minimal privilege escalation |
| TB7 | Services to OS | cgroups resource limits, filesystem isolation, per-user processes |

---

## 3. Threat Categories (STRIDE)

### 3.1 Spoofing

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| S-1 | Attacker impersonates panel admin via stolen credentials | Full panel control, data exfiltration | Bcrypt/Argon2 password hashing, MFA (TOTP) mandatory for admin in hardened mode, session tokens with secure cookie flags |
| S-2 | Attacker forges session token | Unauthorized panel access | Cryptographically signed session tokens, HttpOnly + Secure + SameSite=Strict cookies, session timeout and rotation |
| S-3 | Attacker impersonates ACME server during TLS issuance | Rogue certificates, MitM | Lego library verifies ACME server identity via TLS, pin ACME directory URL |
| S-4 | Spoofed source IP bypasses fail2ban | Brute-force attacks evade blocking | nftables rules at kernel level (pre-application), fail2ban operates on connection IP, rate limiting at Nginx level as secondary control |
| S-5 | Attacker spoofs internal API requests from compromised hosted site | Lateral movement to panel control plane | Panel API binds to separate internal socket/port, hosted sites have no route to panel API, per-user process isolation |
| S-6 | Forged update artifacts from compromised upstream | Malicious code execution via update pipeline | Cryptographic signature and checksum verification before any installation (FR-022), trusted source pinning |

### 3.2 Tampering

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| T-1 | Modification of Nginx vhost configs to redirect traffic | Phishing, data theft | Config files owned by root, panel manages via adapter with atomic write + validation, drift detection |
| T-2 | Tampering with audit.db to hide attacker actions | Loss of forensic evidence | Append-only audit log (NFR-SEC-005), audit.db owned by panel service user with write-only append semantics, integrity verification via chained hashes |
| T-3 | Modification of PHP files in a hosted site after compromise | Defacement, malware, lateral movement | Per-user file ownership (750/640), PHP open_basedir per site, file integrity monitoring |
| T-4 | SQLite database corruption (panel.db) | Panel inoperability, config loss | WAL mode for crash safety, daily backups (NFR-REL-003), checkpoint before backup, integrity checks |
| T-5 | Tampering with nftables rules to open ports | Exposure of internal services | nftables config managed exclusively by panel adapter, periodic drift detection, rules file owned by root |
| T-6 | Binary replacement of panel executable | Complete system compromise | Panel binary integrity verification at startup, signed releases, file permissions (root-owned, 755) |
| T-7 | Modification of backup archives | Restore injects malicious content | Backup archives checksummed at creation, verification before restore, backup storage with restricted permissions |

### 3.3 Repudiation

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| R-1 | Admin denies performing a destructive operation (e.g., site deletion) | Inability to attribute actions, compliance failure | All mutating operations logged with user ID, timestamp, source IP, action type in append-only audit.db (FR-009) |
| R-2 | Attacker clears or modifies logs after compromise | Loss of incident timeline | Append-only log design, log integrity chain (hash of previous entry), audit.db file permissions prevent non-panel modification |
| R-3 | User denies changing PHP config that caused site outage | Support disputes, unclear accountability | Per-operation audit entries with before/after state (diff), user identity tied to authenticated session |
| R-4 | System automation (auto-update) results attributed to wrong actor | Confusion about manual vs. automated changes | Distinct system actor identity in audit trail, all automated operations tagged with job ID and trigger reason |

### 3.4 Information Disclosure

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| I-1 | TLS private keys exposed via file read vulnerability | Domain impersonation, MitM | Key files owned by root (600), panel reads via dedicated adapter with minimal privilege, never logged or returned via API |
| I-2 | Database credentials leaked in logs or error messages | Unauthorized database access | Secrets never written to logs (structured logging with secret redaction), error messages sanitized before API response |
| I-3 | SQLite files (panel.db) accessible from hosted site | Panel config, session data, user hashes exposed | SQLite files stored outside web roots, owned by panel service user, per-user PHP open_basedir prevents access |
| I-4 | Stack traces or debug info exposed in production API responses | Internal architecture revealed to attacker | Production mode disables verbose errors, generic error responses with correlation IDs, detailed errors only in server logs |
| I-5 | Backup archives contain plaintext secrets | Offline credential extraction | Backup archives encrypted at rest, encryption key separate from backup storage, secrets in DB stored hashed/encrypted |
| I-6 | Session tokens leaked via Referer header | Session hijacking | SameSite=Strict cookies, no session tokens in URL parameters, Referrer-Policy header set to strict-origin-when-cross-origin |
| I-7 | Cross-site information leakage between hosted sites | Privacy violation, data breach | Per-user filesystem isolation, per-user PHP-FPM pools, separate DB credentials, open_basedir enforcement |

### 3.5 Denial of Service

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| D-1 | Brute-force login attempts exhaust panel resources | Panel unavailable for legitimate admins | Rate limiting (5 attempts / 15 min), fail2ban jail for panel auth, account lockout with notification |
| D-2 | Hosted site consumes all server CPU/RAM | Other sites and panel become unresponsive | Per-user cgroups resource limits (FR-013), PHP-FPM pool limits (max_children, memory_limit), process accounting |
| D-3 | Disk exhaustion via log flooding or upload abuse | Server-wide outage | Log rotation (logrotate), disk quota per site, monitoring with alerts at thresholds, separate partitions recommended |
| D-4 | SYN flood or HTTP flood against ports 80/443 | Nginx unresponsive, all sites down | nftables rate limiting, Nginx connection limits (limit_conn, limit_req), SYN cookies at kernel level |
| D-5 | Job queue flooding (many simultaneous backup/deploy requests) | Panel operations backlogged, degraded service | Queue concurrency limits, per-user operation rate limits, queue depth monitoring with alerts |
| D-6 | Fork bomb or runaway process from hosted site | Server crash | Per-user cgroups (pids limit), ulimit settings, PHP disable_functions (exec, system, proc_open...) |
| D-7 | SQLite write contention under load | Panel API latency spikes | WAL mode, split databases (panel.db/audit.db/queue.db), migration path to PostgreSQL at defined thresholds |

### 3.6 Elevation of Privilege

| # | Threat | Impact | Mitigation |
|---|---|---|---|
| E-1 | Regular user escalates to admin role via RBAC bypass | Full panel control | Server-side RBAC enforcement on every API endpoint, role checks in middleware, RBAC tests in CI |
| E-2 | Hosted site PHP code exploits PHP-FPM to gain system access | Server compromise | Per-user PHP-FPM pools, disable_functions (exec, system, passthru, shell_exec, popen, proc_open), open_basedir, no SUID binaries accessible |
| E-3 | Panel service account exploited to gain root | Complete server takeover | Panel runs as dedicated non-root user, sudo rules limited to specific commands with NOPASSWD only for required operations, no shell for service account |
| E-4 | SQL injection in panel API to manipulate RBAC tables | Privilege escalation within panel | Parameterized queries (no string concatenation for SQL), ORM/query builder with bind variables, input validation |
| E-5 | Path traversal in file manager to access files outside site directory | Cross-site data access, system file read | Strict path canonicalization and validation, chroot-like enforcement in file manager adapter, open_basedir as defense-in-depth |
| E-6 | Compromised hosted site pivots to attack panel API | Panel control plane compromise | Panel API not routable from hosted site network context, panel listens on localhost or unix socket, hosted sites have no knowledge of panel port |
| E-7 | Exploiting auto-update mechanism to deploy malicious package | Arbitrary code execution as root | Cryptographic signature verification (FR-022), preflight checks, canary rollout, admin approval for major updates, rollback automation |

---

## 4. Attack Surface Inventory

### 4.1 Panel Web UI (Port 443)

| Attribute | Detail |
|---|---|
| **Exposure** | Internet-facing |
| **Protocol** | HTTPS (TLS 1.2 minimum, 1.3 preferred) |
| **Authentication** | Session cookie (HttpOnly, Secure, SameSite=Strict) |
| **Input vectors** | Login form, site management forms, file manager, search, settings |
| **Key risks** | XSS, CSRF, session hijacking, brute-force |
| **Controls** | CSP header, CSRF tokens, input sanitization, rate limiting, secure cookie attributes |

### 4.2 Panel API (Internal, Same Port)

| Attribute | Detail |
|---|---|
| **Exposure** | Behind Nginx reverse proxy (same port as UI) |
| **Protocol** | HTTPS (terminated at Nginx, internal plaintext to localhost Go process) |
| **Authentication** | Bearer token / session cookie, RBAC on every endpoint |
| **Input vectors** | JSON payloads, query parameters, path parameters |
| **Key risks** | Injection, RBAC bypass, mass assignment, IDOR |
| **Controls** | Input validation, parameterized queries, RBAC middleware, request size limits |

### 4.3 SSH (Port 22, Hardened)

| Attribute | Detail |
|---|---|
| **Exposure** | Internet-facing (port configurable) |
| **Protocol** | SSH v2 |
| **Authentication** | Key-based (password optional, disabled by default in hardened mode) |
| **Key risks** | Brute-force, key compromise, unauthorized access |
| **Controls** | Root login disabled, key-only auth, fail2ban jail, configurable port, AllowUsers/AllowGroups |

### 4.4 Managed Services (Local Ports)

| Service | Binding | Port | Controls |
|---|---|---|---|
| **Nginx** | 0.0.0.0:80, 0.0.0.0:443 | 80, 443 | Only externally exposed managed service |
| **PHP-FPM** | Unix socket per pool | N/A | Per-user pools, socket permissions 660 |
| **MariaDB** | 127.0.0.1 | 3306 | Localhost only, per-site users, strong passwords |
| **PostgreSQL** | 127.0.0.1 | 5432 | Localhost only, per-site users, md5/scram-sha-256 auth |
| **Panel Go process** | 127.0.0.1 | Internal | Not directly reachable from internet |

### 4.5 File System (Per-User Isolation)

| Path Pattern | Owner | Permissions | Purpose |
|---|---|---|---|
| `/home/<site-user>/` | site-user:site-group | 750 | Site home directory |
| `/home/<site-user>/public_html/` | site-user:www-data | 750 | Web root |
| Individual site files | site-user:site-group | 640 | PHP, HTML, assets |
| Upload directories | site-user:www-data | 770 | Writable by web server |
| Panel binary and config | root:root | 755 / 640 | Panel installation |

### 4.6 SQLite Files

| File | Purpose | Sensitivity | Protection |
|---|---|---|---|
| `panel.db` | Config, sessions, version state | High (session tokens, user hashes) | Owned by panel service user (600), outside web root, WAL mode |
| `audit.db` | Append-only audit log | High (forensic evidence) | Append-only semantics, integrity chain, owned by panel service user |
| `queue.db` | Job queue | Medium (operation metadata) | Owned by panel service user (600), purged after job completion |

---

## 5. Hardening Checklist v1 (Post-Install Defaults)

These controls are applied automatically by the aiPanel installer on a clean Debian 13 system. Items marked **[DEFAULT]** are enabled out of the box. Items marked **[OPTIONAL]** require explicit activation by the administrator.

### 5.1 SSH Hardening

- [ ] **[DEFAULT]** Disable root login (`PermitRootLogin no`)
- [ ] **[DEFAULT]** Key-only authentication (`PasswordAuthentication no`, `PubkeyAuthentication yes`)
- [ ] **[OPTIONAL]** Password authentication (can be re-enabled per admin preference)
- [ ] **[DEFAULT]** SSH port configurable during install (default: 22)
- [ ] **[DEFAULT]** fail2ban jail for SSH enabled (`maxretry=5`, `bantime=3600`, `findtime=600`)
- [ ] **[DEFAULT]** SSH protocol version 2 only
- [ ] **[DEFAULT]** Disable empty passwords (`PermitEmptyPasswords no`)
- [ ] **[DEFAULT]** Disable X11 forwarding (`X11Forwarding no`)
- [ ] **[DEFAULT]** Restrict SSH access to panel-created users (`AllowGroups ssh-users`)
- [ ] **[DEFAULT]** Idle session timeout (`ClientAliveInterval 300`, `ClientAliveCountMax 2`)

### 5.2 Firewall (nftables)

- [ ] **[DEFAULT]** nftables enabled and active after install
- [ ] **[DEFAULT]** Default policy: drop all incoming traffic
- [ ] **[DEFAULT]** Whitelist: port 22 (or configured SSH port)
- [ ] **[DEFAULT]** Whitelist: port 80 (HTTP, for ACME challenges and redirect)
- [ ] **[DEFAULT]** Whitelist: port 443 (HTTPS, panel UI + hosted sites)
- [ ] **[DEFAULT]** Allow established/related connections
- [ ] **[DEFAULT]** Allow loopback interface
- [ ] **[DEFAULT]** Rate limiting for new connections (SYN flood protection)
- [ ] **[DEFAULT]** Log dropped packets (limited rate to prevent log flood)
- [ ] **[DEFAULT]** Outbound: allow all (required for apt, ACME, feed sync)
- [ ] **[OPTIONAL]** Outbound filtering (restrict to known destinations)

### 5.3 Panel Authentication & Sessions

- [ ] **[DEFAULT]** Passwords hashed with Argon2id (bcrypt as fallback)
- [ ] **[DEFAULT]** Rate limiting on login endpoint (5 failed attempts per 15 minutes per IP)
- [ ] **[DEFAULT]** Account lockout after 10 consecutive failures (unlock via CLI or time-based)
- [ ] **[DEFAULT]** fail2ban jail for panel login (ban IP after repeated failures)
- [ ] **[DEFAULT]** Session timeout: 30 minutes of inactivity
- [ ] **[DEFAULT]** Absolute session lifetime: 12 hours (re-authentication required)
- [ ] **[DEFAULT]** CSRF tokens on all mutating requests
- [ ] **[DEFAULT]** Secure cookie attributes: `HttpOnly`, `Secure`, `SameSite=Strict`
- [ ] **[DEFAULT]** Session ID regeneration after login
- [ ] **[DEFAULT]** Constant-time comparison for authentication tokens

### 5.4 Multi-Factor Authentication (MFA)

- [ ] **[OPTIONAL]** TOTP-based MFA available for all accounts
- [ ] **[DEFAULT]** MFA enrollment prompt on first admin login
- [ ] **[DEFAULT]** MFA required for admin accounts in hardened mode
- [ ] **[DEFAULT]** Recovery codes generated during MFA setup (one-time use, hashed storage)
- [ ] **[DEFAULT]** MFA validation rate-limited (prevent TOTP brute-force)

### 5.5 TLS Configuration

- [ ] **[DEFAULT]** Minimum TLS version: 1.2
- [ ] **[DEFAULT]** Preferred TLS version: 1.3
- [ ] **[DEFAULT]** Strong cipher suite order (AEAD ciphers preferred: AES-256-GCM, ChaCha20-Poly1305)
- [ ] **[DEFAULT]** HSTS header enabled (`max-age=31536000; includeSubDomains`)
- [ ] **[DEFAULT]** OCSP stapling enabled
- [ ] **[DEFAULT]** Automatic certificate issuance via Let's Encrypt (HTTP-01 challenge)
- [ ] **[DEFAULT]** Automatic certificate renewal (30 days before expiry)
- [ ] **[DEFAULT]** TLS private keys: 600 permissions, root-owned
- [ ] **[DEFAULT]** Redirect HTTP to HTTPS (port 80 to 443)
- [ ] **[OPTIONAL]** DNS-01 challenge support for wildcard certificates

### 5.6 PHP Hardening (Per-Site)

- [ ] **[DEFAULT]** `disable_functions`: `exec`, `system`, `passthru`, `shell_exec`, `popen`, `proc_open`, `proc_close`, `proc_get_status`, `proc_nice`, `proc_terminate`, `pcntl_exec`, `pcntl_fork`, `pcntl_signal`, `pcntl_alarm`, `dl`, `putenv`, `phpinfo`, `show_source`
- [ ] **[DEFAULT]** `open_basedir` set per site to site home directory + `/tmp` (isolated temp)
- [ ] **[DEFAULT]** `expose_php = Off`
- [ ] **[DEFAULT]** `display_errors = Off` (production)
- [ ] **[DEFAULT]** `log_errors = On` (to per-site error log)
- [ ] **[DEFAULT]** `session.cookie_httponly = 1`
- [ ] **[DEFAULT]** `session.cookie_secure = 1`
- [ ] **[DEFAULT]** `session.cookie_samesite = Strict`
- [ ] **[DEFAULT]** `session.use_strict_mode = 1`
- [ ] **[DEFAULT]** `session.use_only_cookies = 1`
- [ ] **[DEFAULT]** `upload_max_filesize` and `post_max_size` set to reasonable defaults (50M)
- [ ] **[DEFAULT]** `max_execution_time = 30`
- [ ] **[DEFAULT]** `memory_limit = 256M` (configurable per site)
- [ ] **[DEFAULT]** `allow_url_fopen = Off` (can be enabled per site if needed)
- [ ] **[DEFAULT]** `allow_url_include = Off`

### 5.7 File Permissions

- [ ] **[DEFAULT]** Site directories: `750` (owner: site-user, group: site-group)
- [ ] **[DEFAULT]** Site files: `640` (owner: site-user, group: site-group)
- [ ] **[DEFAULT]** Separate system user per site (no shared UIDs)
- [ ] **[DEFAULT]** Web root owned by site user, group includes `www-data` for read
- [ ] **[DEFAULT]** No world-readable or world-writable permissions in site directories
- [ ] **[DEFAULT]** Panel configuration files: `640` (root:panel-group)
- [ ] **[DEFAULT]** SQLite database files: `600` (panel-user:panel-group)
- [ ] **[DEFAULT]** TLS private keys: `600` (root:root)
- [ ] **[DEFAULT]** Log directories: `750` (respective service user)
- [ ] **[DEFAULT]** No SUID/SGID binaries in site directories (periodic scan)

### 5.8 Process Isolation

- [ ] **[DEFAULT]** Per-user PHP-FPM pools (separate pool config per site)
- [ ] **[DEFAULT]** PHP-FPM pool runs as site-specific user/group
- [ ] **[DEFAULT]** cgroups v2 resource limits per site (CPU weight, memory max, IO weight)
- [ ] **[DEFAULT]** `max_children` limit per PHP-FPM pool
- [ ] **[DEFAULT]** PIDs limit via cgroups (prevent fork bombs)
- [ ] **[DEFAULT]** Panel Go process runs as dedicated non-root service user
- [ ] **[DEFAULT]** Panel process uses specific sudo rules (limited commands, no shell)
- [ ] **[DEFAULT]** MariaDB/PostgreSQL run as dedicated system users
- [ ] **[OPTIONAL]** PrivateTmp and ProtectSystem in systemd unit files for managed services

### 5.9 HTTP Security Headers

- [ ] **[DEFAULT]** `X-Content-Type-Options: nosniff`
- [ ] **[DEFAULT]** `X-Frame-Options: DENY` (for panel; `SAMEORIGIN` for hosted sites)
- [ ] **[DEFAULT]** `Content-Security-Policy` baseline for panel: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'`
- [ ] **[DEFAULT]** `Referrer-Policy: strict-origin-when-cross-origin`
- [ ] **[DEFAULT]** `Permissions-Policy: camera=(), microphone=(), geolocation=()`
- [ ] **[DEFAULT]** `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- [ ] **[DEFAULT]** `X-XSS-Protection: 0` (rely on CSP instead of broken XSS filter)
- [ ] **[OPTIONAL]** Per-site CSP customization (via panel UI, with sane defaults)

### 5.10 Audit & Logging

- [ ] **[DEFAULT]** All mutating operations logged to `audit.db` (FR-009)
- [ ] **[DEFAULT]** Audit log fields: timestamp, user ID, source IP, action, resource, before/after state
- [ ] **[DEFAULT]** Append-only semantics for audit log (no UPDATE/DELETE on audit table)
- [ ] **[DEFAULT]** Log integrity verification via chained SHA-256 hashes (each entry references previous hash)
- [ ] **[DEFAULT]** Panel service user has INSERT-only permissions on audit table
- [ ] **[DEFAULT]** Nginx access/error logs with structured format
- [ ] **[DEFAULT]** PHP-FPM per-site error logs (separate log file per pool)
- [ ] **[DEFAULT]** Log rotation via logrotate (daily, 30 days retention)
- [ ] **[DEFAULT]** Failed authentication attempts logged with IP, timestamp, username
- [ ] **[DEFAULT]** Privilege-sensitive operations (RBAC changes, MFA changes) logged at elevated severity

---

## 6. Secrets Management

### 6.1 Secret Types

| Secret | Storage | Encryption | Rotation |
|---|---|---|---|
| User passwords | `panel.db` | Argon2id hash (irreversible) | User-initiated or admin-forced |
| MFA TOTP seeds | `panel.db` | AES-256-GCM encrypted at rest | On MFA re-enrollment |
| Session signing key | Environment / config | Generated at install, stored encrypted | On panel update or manual rotation |
| DB passwords (MariaDB/PgSQL) | `panel.db` + service config | AES-256-GCM encrypted in panel.db | Via panel UI (rotate + update service config) |
| TLS private keys | Filesystem (`/etc/ssl/private/`) | File permissions (600, root-owned) | On certificate renewal (every 60-90 days) |
| ACME account key | Filesystem (panel data dir) | File permissions (600, panel-user) | Rarely (manual rotation) |
| API tokens (internal) | `panel.db` | SHA-256 hashed (lookup by prefix) | Configurable expiry, manual revocation |
| Backup encryption key | Panel config (encrypted) | Derived from master key via HKDF | On admin rotation |

### 6.2 Principles

1. **Encrypted at rest**: All secrets stored in `panel.db` are encrypted using AES-256-GCM with a master key derived from a passphrase or hardware-backed source.
2. **Never in logs**: Structured logging framework with secret redaction. Secrets are tagged in code and automatically masked in log output. Log format audited in CI.
3. **Never in API responses**: Secrets are write-only via API. Read operations return masked values or metadata only.
4. **Minimal lifetime**: Session tokens have defined TTL. API tokens have configurable expiry. Temporary secrets (e.g., one-time recovery codes) are deleted after use.
5. **Rotation policy**:
   - Session signing key: rotate on every panel major update, or manually via CLI.
   - DB passwords: rotatable via panel (panel updates both `panel.db` record and service configuration atomically).
   - TLS keys: rotated on every certificate renewal cycle.
   - Backup encryption key: annual rotation recommended, with re-encryption of retained backups.
6. **Master key protection**: The master encryption key is derived at panel startup from a file on disk (`/etc/aipanel/master.key`, permissions 600, root-owned). In future versions, integration with HSM or system keyring is planned.

---

## 7. Security Monitoring

### 7.1 Monitored Events

| Category | Event | Source | Alert Threshold |
|---|---|---|---|
| **Authentication** | Failed login attempts | Panel auth, fail2ban | >5 failures from single IP in 15 min |
| **Authentication** | Successful login from new IP | Panel auth | Any (informational, logged) |
| **Authentication** | MFA bypass or failure | Panel auth | Any failure (logged), 3+ failures (alert) |
| **Privilege** | RBAC role change | Audit log | Any (alert to admin) |
| **Privilege** | New admin account created | Audit log | Any (alert to all existing admins) |
| **Privilege** | sudo usage by panel service | System journal | Unexpected commands (alert) |
| **Integrity** | File changes in critical paths | Periodic integrity check | Any change outside panel-managed operations |
| **Integrity** | Audit log hash chain broken | Integrity verifier | Any (critical alert) |
| **Integrity** | Panel binary modified | Startup check + periodic | Any (critical alert) |
| **Resources** | CPU/RAM/disk thresholds | Monitoring module | CPU >90% 5min, RAM >85%, Disk >90% |
| **Resources** | Unusual resource usage by site | cgroups accounting | Sustained usage above limits |
| **TLS** | Certificate expiry approaching | Cert manager | 14 days, 7 days, 3 days before expiry |
| **TLS** | Certificate renewal failure | Cert manager | Any failure (alert, retry) |
| **Network** | nftables rule change outside panel | Drift detection | Any (alert) |
| **Updates** | Security patch available but not applied | Version Manager | >24h delay (warning), >72h (critical) |
| **Updates** | Rollback triggered | Update pipeline | Any (alert with cause) |

### 7.2 Alert Channels

- **Panel dashboard**: Real-time alerts and threat summary (Security & Audit screen).
- **System notifications**: Alerts stored in panel database, displayed on next admin login.
- **Email** (optional, post-MVP): Configurable recipients for critical alerts.
- **Webhook** (optional, post-MVP): Integration point for external monitoring systems.

### 7.3 Periodic Security Tasks

| Task | Frequency | Method |
|---|---|---|
| Audit log integrity verification | Every 6 hours | Hash chain validation job |
| File integrity check (critical paths) | Daily | Checksum comparison against baseline |
| SUID/SGID binary scan in site dirs | Daily | find + report |
| nftables rule drift detection | Every hour | Compare running rules to expected config |
| Open port scan (internal) | Daily | Internal port check against whitelist |
| Backup integrity verification | After each backup | Checksum verification of archive |
| Expired session cleanup | Every hour | Purge expired sessions from panel.db |
| fail2ban status check | Every 15 minutes | Service health + active bans report |

---

## 8. Incident Response Basics

### 8.1 Severity Levels

| Level | Definition | Examples | Response Time |
|---|---|---|---|
| **P1 — Critical** | Active exploitation, data breach, complete service outage | Root compromise, audit log tampering, mass site defacement | Immediate |
| **P2 — High** | Confirmed vulnerability with exploit path, single site compromise | Privilege escalation confirmed, single site malware, credential leak | <1 hour |
| **P3 — Medium** | Suspicious activity, failed exploit attempts, degraded security | Brute-force surge, config drift detected, update failure | <4 hours |
| **P4 — Low** | Informational, minor policy violations | New login IP, low-rate failed logins, advisory CVE (no exploit) | Next business day |

### 8.2 Notification Matrix

| Event | Notified | Method |
|---|---|---|
| P1 incident | All admin accounts | Panel alert (immediate) + email (if configured) |
| P2 incident | Primary admin | Panel alert + email |
| Account lockout (brute-force) | Account owner + admins | Panel alert |
| Compromised site detected | Site owner + admins | Panel alert |
| Audit log integrity failure | All admin accounts | Panel alert (critical banner) |
| Security patch overdue (>72h) | All admin accounts | Panel alert (persistent warning) |

### 8.3 Account Lockout Procedure

1. **Automatic lockout**: After 10 consecutive failed login attempts, the account is locked.
2. **Lockout notification**: Admin accounts receive a panel alert with the locked account details, source IP, and timestamp.
3. **Unlock methods**:
   - Time-based: automatic unlock after configurable period (default: 30 minutes).
   - Admin action: another admin unlocks via panel UI or CLI.
   - CLI: `aipanel user unlock <username>` (requires server SSH access).
4. **Post-unlock**: Force password change if compromise is suspected. Review audit log for the affected account.

### 8.4 Compromised Site Isolation Procedure

When a hosted site is confirmed or suspected to be compromised:

1. **Immediate containment**:
   - Disable the site's Nginx vhost (`aipanel site disable <site-name>`)
   - Stop the site's PHP-FPM pool (`aipanel site stop-runtime <site-name>`)
   - Revoke the site's database user credentials and create new ones (not yet applied)
   - Result: site is offline, no PHP execution, no DB access

2. **Evidence preservation**:
   - Snapshot the site directory (read-only copy to quarantine path)
   - Export relevant audit log entries for the site and its user
   - Export relevant Nginx access/error logs
   - Record PHP-FPM pool logs for the affected site
   - Do not modify or delete any files in the original site directory

3. **Damage assessment**:
   - Check if the site user accessed other resources (review audit log)
   - Verify file integrity against last known-good backup
   - Scan for web shells, modified PHP files, unauthorized cron jobs
   - Check database for injected content or unauthorized accounts
   - Review outbound connections from the site user (if available)

4. **Remediation**:
   - Option A: Restore from last known-good backup (verify backup integrity first)
   - Option B: Manual cleanup with verification (for minor incidents)
   - Rotate all credentials for the site (DB password, FTP/SFTP keys if applicable)
   - Update PHP and dependencies if vulnerability was in runtime
   - Apply additional hardening if site-specific weakness identified

5. **Recovery**:
   - Re-enable the site with new credentials
   - Verify site functionality
   - Monitor closely for 48 hours (elevated logging)
   - Document the incident in audit log with resolution details

### 8.5 Panel-Level Compromise Response

If the panel itself (API, Go process, or panel.db) is suspected to be compromised:

1. **Isolate**: Disconnect the server from the network if remote attacker is active (physical/IPMI access required).
2. **Preserve**: Do not restart services. Copy all logs, SQLite databases, and panel binary to external storage.
3. **Assess**: Review audit.db integrity chain. Check panel binary hash against known-good release. Review systemd journal for panel service.
4. **Rebuild**: If compromise is confirmed, reinstall the panel from a verified release. Restore panel.db from last verified backup. Force password reset for all accounts. Re-enroll MFA.
5. **Notify**: Inform all panel users about the incident and required credential changes.

---

## Appendix A: Threat Model Maintenance

This threat model must be reviewed and updated:

- At every major version release of aiPanel.
- When new features are added that change the attack surface (e.g., public API, reseller role, DNS management).
- After any P1 or P2 security incident.
- At minimum once every 6 months.

## Appendix B: References

- PRD: `docs/PRD-hosting-panel.md` (v0.7)
- STRIDE: Microsoft Threat Modeling methodology
- OWASP: Application Security Verification Standard (ASVS)
- CIS Benchmarks: Debian Linux, Nginx, MariaDB, PostgreSQL
- NIST SP 800-123: Guide to General Server Security
