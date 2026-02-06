# aiPanel Observability — v1

> Minimal observability specification for aiPanel MVP.
> Covers logging, metrics, health checks, alerting, the built-in operational dashboard, audit trail, and post-MVP external integrations.

---

## 1. Logging

### 1.1 Format

Every log entry is a single line of structured JSON written to stdout.

```json
{
  "ts": "2026-02-06T14:32:07.482Z",
  "level": "info",
  "module": "backup",
  "msg": "Backup completed for site-a",
  "request_id": "req_8f3a2c",
  "user_id": "usr_admin_01",
  "duration_ms": 4521,
  "error": null
}
```

```json
{
  "ts": "2026-02-06T14:33:11.109Z",
  "level": "error",
  "module": "tls",
  "msg": "ACME challenge failed for example.com",
  "request_id": "req_d91ef4",
  "user_id": null,
  "duration_ms": 12044,
  "error": "dns timeout after 10s"
}
```

### 1.2 Fields

| Field        | Type              | Required | Description                                      |
|------------- |------------------ |--------- |------------------------------------------------- |
| `ts`         | ISO 8601 string   | yes      | UTC timestamp with millisecond precision          |
| `level`      | string            | yes      | One of: `debug`, `info`, `warn`, `error`          |
| `module`     | string            | yes      | Logical module name (e.g. `iam`, `backup`, `tls`) |
| `msg`        | string            | yes      | Human-readable description of the event           |
| `request_id` | string / null     | yes      | Correlation ID for HTTP requests and job runs     |
| `user_id`    | string / null     | yes      | Authenticated user ID; `null` for system actions  |
| `duration_ms`| integer / null    | no       | Operation wall-clock time in milliseconds         |
| `error`      | string / null     | no       | Error details; `null` when no error occurred      |

### 1.3 Log Levels

| Level   | Usage                                                                 |
|-------- |---------------------------------------------------------------------- |
| `debug` | Verbose development traces; disabled in production by default         |
| `info`  | Normal operational events (request handled, backup started, etc.)     |
| `warn`  | Degraded conditions that do not block operations (high latency, retry)|
| `error` | Failures requiring attention (service down, operation failed)         |

### 1.4 Output & Rotation

| Destination                      | Details                                                        |
|--------------------------------- |--------------------------------------------------------------- |
| **stdout**                       | Captured by systemd journal (`journalctl -u aipanel`)          |
| **File**: `/var/log/aipanel/`    | Rotated via `logrotate`; default: daily, 14-day retention, gzip|

### 1.5 Managed Service Logs

The panel reads logs from managed services for display in the UI and for alert evaluation.

| Service       | Log Location (default)                           |
|-------------- |------------------------------------------------- |
| Nginx         | `/var/log/nginx/access.log`, `/var/log/nginx/error.log` |
| PHP-FPM       | `/var/log/php{version}-fpm.log`                  |
| MariaDB       | `/var/log/mysql/error.log`                       |
| PostgreSQL    | `/var/log/postgresql/postgresql-*-main.log`      |
| fail2ban      | `/var/log/fail2ban.log`                          |

### 1.6 Sensitive Data Policy

- **NEVER** log passwords, API tokens, private keys, session secrets, or database credentials.
- Mask fields that may contain sensitive data (e.g. `Authorization` header, `password` form field).
- If a full request body must be logged for debugging, redact known sensitive keys and limit log level to `debug`.

### 1.7 Library

Go stdlib `slog` (structured logging, built-in since Go 1.21).

- JSON handler writing to stdout.
- Module-scoped loggers created via `slog.With("module", "<name>")`.
- `request_id` and `user_id` injected through middleware context.

---

## 2. Metrics

### 2.1 Exposure

Prometheus-compatible endpoint at `GET /metrics`.

- **Internal only** — bound to `127.0.0.1` or protected by authentication; not exposed to the public internet.
- Wire format: Prometheus text exposition format (`text/plain; version=0.0.4`).

### 2.2 Key Metrics

#### Panel Metrics

| Metric Name                  | Type      | Labels              | Description                            |
|----------------------------- |---------- |-------------------- |--------------------------------------- |
| `aipanel_request_duration_seconds` | histogram | `method`, `path`, `status` | HTTP request latency distribution |
| `aipanel_request_total`      | counter   | `method`, `path`, `status` | Total HTTP requests by status code |
| `aipanel_active_sessions`    | gauge     | —                   | Currently active authenticated sessions|

#### Host Metrics

| Metric Name                  | Type      | Labels              | Description                            |
|----------------------------- |---------- |-------------------- |--------------------------------------- |
| `host_cpu_usage_percent`     | gauge     | `cpu`               | Per-CPU and aggregate usage percentage |
| `host_memory_usage_bytes`    | gauge     | `type` (`used`, `available`, `total`) | Memory consumption   |
| `host_disk_usage_bytes`      | gauge     | `mountpoint`        | Disk space used per mount              |
| `host_disk_io_bytes`         | counter   | `device`, `direction` (`read`, `write`) | Cumulative disk I/O |

#### Service Health Metrics

| Metric Name                  | Type      | Labels              | Description                            |
|----------------------------- |---------- |-------------------- |--------------------------------------- |
| `service_nginx_up`           | gauge     | —                   | 1 = running, 0 = down                 |
| `service_php_fpm_up`         | gauge     | `version`           | 1 = running, 0 = down                 |
| `service_mariadb_up`         | gauge     | —                   | 1 = running, 0 = down                 |
| `service_postgresql_up`      | gauge     | —                   | 1 = running, 0 = down                 |
| `service_fail2ban_up`        | gauge     | —                   | 1 = running, 0 = down                 |

#### Business Metrics

| Metric Name                          | Type      | Labels      | Description                                  |
|------------------------------------- |---------- |------------ |--------------------------------------------- |
| `aipanel_sites_total`                | gauge     | —           | Total number of configured sites             |
| `aipanel_sites_active`               | gauge     | —           | Sites with active traffic (non-suspended)    |
| `aipanel_backups_total`              | counter   | `site`      | Total backups created                        |
| `aipanel_backup_last_success_timestamp` | gauge  | `site`      | Unix timestamp of last successful backup     |
| `aipanel_updates_pending`            | gauge     | `component` | Number of pending updates per component      |

#### Job Queue Metrics

| Metric Name                          | Type      | Labels       | Description                                 |
|------------------------------------- |---------- |------------- |-------------------------------------------- |
| `aipanel_jobs_queued`                | gauge     | `type`       | Jobs waiting in queue                       |
| `aipanel_jobs_processing`            | gauge     | `type`       | Jobs currently being executed               |
| `aipanel_jobs_completed_total`       | counter   | `type`       | Total successfully completed jobs           |
| `aipanel_jobs_failed_total`          | counter   | `type`       | Total failed jobs                           |
| `aipanel_job_duration_seconds`       | histogram | `type`       | Job execution time distribution             |

### 2.3 Collection

- **Panel/HTTP metrics**: instrumented via `prometheus/client_golang` middleware.
- **Go runtime metrics**: exposed via `expvar` or the Prometheus Go collector (goroutines, GC, memory).
- **Host metrics**: read directly from `/proc/stat`, `/proc/meminfo`, `/proc/diskstats`, and `/sys/fs/cgroup/` (where applicable).
- **Service health**: checked via `systemctl is-active <unit>` or equivalent D-Bus call.

---

## 3. Health Checks

### 3.1 Endpoints

#### `GET /health` — Liveness

Returns `200 OK` if the panel process is alive and able to serve HTTP.

```json
{
  "status": "ok",
  "ts": "2026-02-06T14:32:07.482Z"
}
```

No external dependencies are checked. This endpoint is used by systemd watchdog and load balancers to determine process liveness.

#### `GET /health/ready` — Readiness

Returns `200 OK` only when all critical dependencies are reachable. Returns `503 Service Unavailable` if any component is unhealthy.

**Healthy response (200):**

```json
{
  "status": "ready",
  "ts": "2026-02-06T14:32:07.482Z",
  "checks": {
    "sqlite": { "status": "ok", "latency_ms": 1 },
    "nginx": { "status": "ok" },
    "php_fpm": { "status": "ok" },
    "mariadb": { "status": "ok", "latency_ms": 3 },
    "postgresql": { "status": "ok", "latency_ms": 4 }
  }
}
```

**Degraded response (503):**

```json
{
  "status": "not_ready",
  "ts": "2026-02-06T14:32:07.482Z",
  "checks": {
    "sqlite": { "status": "ok", "latency_ms": 1 },
    "nginx": { "status": "ok" },
    "php_fpm": { "status": "down", "error": "connect ECONNREFUSED /run/php/php8.3-fpm.sock" },
    "mariadb": { "status": "ok", "latency_ms": 3 },
    "postgresql": { "status": "skip", "reason": "not installed" }
  }
}
```

### 3.2 Component Status Values

| Status   | Meaning                                                |
|--------- |------------------------------------------------------- |
| `ok`     | Component is healthy and responding within thresholds  |
| `down`   | Component is unreachable or returning errors           |
| `degraded` | Component is responding but outside normal parameters|
| `skip`   | Component is not installed or not applicable           |

### 3.3 Consumers

| Consumer               | Endpoint Used    | Purpose                                      |
|----------------------- |----------------- |--------------------------------------------- |
| systemd watchdog       | `/health`        | Restart panel on liveness failure             |
| Monitoring dashboard   | `/health/ready`  | Display per-component status in UI            |
| Update preflight       | `/health/ready`  | Block updates when services are unhealthy     |

---

## 4. Alerts (Critical)

### 4.1 Alert Conditions

| ID   | Condition                                    | Check Interval | Severity |
|----- |--------------------------------------------- |--------------- |--------- |
| A-01 | Nginx service down                           | 30s            | critical |
| A-02 | PHP-FPM service down                         | 30s            | critical |
| A-03 | MariaDB service down                         | 30s            | critical |
| A-04 | PostgreSQL service down                      | 30s            | critical |
| A-05 | Disk usage > 90%                             | 60s            | critical |
| A-06 | Panel process crash (systemd restart count > 3 in 5 min) | on restart | critical |
| A-07 | Backup failure                               | on completion  | critical |
| A-08 | TLS certificate expiring in < 7 days         | 6h             | warning  |
| A-09 | Failed login attempts > threshold            | 60s            | warning  |
| A-10 | Update rollback occurred                     | on occurrence  | warning  |

### 4.2 Alert Payload

Each alert is recorded as a structured entry:

```json
{
  "alert_id": "A-05",
  "ts": "2026-02-06T14:32:07.482Z",
  "severity": "critical",
  "title": "Disk usage exceeds 90%",
  "detail": "/dev/sda1 at 93% (46.5 GB / 50 GB)",
  "component": "host",
  "resolved": false
}
```

### 4.3 Delivery

| Phase    | Channel                                                                       |
|--------- |------------------------------------------------------------------------------ |
| **MVP**  | Log-based: panel evaluates conditions, writes alert to audit log, displays in UI dashboard alert list |
| **Post-MVP** | Webhook notifications (Slack, Discord, PagerDuty), email notifications   |

### 4.4 Alert Lifecycle

1. **Firing** — condition is met; alert is created and displayed.
2. **Acknowledged** — admin marks alert as seen (optional, from UI).
3. **Resolved** — condition clears; alert is auto-resolved with timestamp.

---

## 5. Operational Dashboard (Built-in)

This section describes the main Dashboard screen as defined in UI Spec v1 (PRD section 20.8 / 21.1). The dashboard is the default landing page after login.

### 5.1 Widgets

| Widget                     | Data Source              | Description                                                   |
|--------------------------- |------------------------- |-------------------------------------------------------------- |
| Server Health Score        | Composite metric         | Weighted score from CPU + RAM + disk + service states (0-100) |
| Service Status Grid        | `/health/ready`          | Nginx, PHP-FPM, DB, fail2ban — up / down / degraded badges   |
| Active Alerts              | Alert engine             | Unresolved alerts sorted by severity                          |
| Recent Audit Log           | `audit.db`               | Last 10 mutating operations with user, action, result         |
| Resource Usage Graphs      | Host metrics (last 24h)  | CPU, RAM, disk — sparkline or area charts                     |
| Sites Overview             | `panel.db`               | Total sites, active sites, TLS certificate statuses           |
| Update Compliance Status   | Version Manager          | Components: up-to-date / lagging / unsupported counts         |
| Backup Status per Site     | Backup module            | Last backup timestamp, next scheduled, success/failure state  |

### 5.2 Health Score Calculation

```
score = w_cpu * (100 - cpu_percent)
      + w_ram * (100 - ram_percent)
      + w_disk * (100 - disk_percent)
      + w_svc * (services_up / services_total * 100)
```

Default weights: `w_cpu = 0.20`, `w_ram = 0.20`, `w_disk = 0.25`, `w_svc = 0.35`.

Score thresholds for display:

| Range   | Label     | Color Token          |
|-------- |---------- |--------------------- |
| 80-100  | Healthy   | `--state-success`    |
| 50-79   | Degraded  | `--state-warning`    |
| 0-49    | Critical  | `--state-danger`     |

---

## 6. Audit Trail

Implements PRD requirement FR-009: all mutating operations must be recorded.

### 6.1 Storage

- **Database**: `audit.db` (SQLite, separate file from `panel.db` per PRD section 17.1).
- **Mode**: append-only. Rows are never updated or deleted by application logic.
- **Integrity**: non-admin accounts cannot modify audit records (NFR-SEC-005).

### 6.2 Audit Entry Schema

| Field          | Type            | Description                                              |
|--------------- |---------------- |--------------------------------------------------------- |
| `id`           | integer (PK)    | Auto-incrementing unique identifier                      |
| `ts`           | ISO 8601 string | UTC timestamp of the operation                           |
| `user_id`      | string          | User who performed the action (`system` for automated)   |
| `action`       | string          | Operation name (e.g. `site.create`, `backup.restore`)    |
| `resource_type`| string          | Target resource type (`site`, `database`, `user`, etc.)  |
| `resource_id`  | string / null   | Target resource identifier                               |
| `ip`           | string          | Source IP address of the request                         |
| `result`       | string          | `success` or `failure`                                   |
| `detail`       | JSON string     | Operation-specific context (params, error message, diff) |

### 6.3 Example Entry

```json
{
  "id": 10482,
  "ts": "2026-02-06T14:32:07.482Z",
  "user_id": "usr_admin_01",
  "action": "site.create",
  "resource_type": "site",
  "resource_id": "site_abc123",
  "ip": "192.168.1.42",
  "result": "success",
  "detail": {
    "domain": "example.com",
    "php_version": "8.3",
    "db_engine": "mariadb"
  }
}
```

### 6.4 Querying from UI

- **Filters**: user, action type, resource type, date range, result (success/failure).
- **Search**: free-text search across `action`, `resource_id`, and `detail`.
- **Pagination**: server-side, default 50 entries per page.

### 6.5 Retention & Export

| Setting          | Default         | Configurable |
|----------------- |---------------- |------------- |
| Retention period | 1 year          | Yes          |
| Cleanup method   | Soft-delete + periodic vacuum | — |
| Export formats   | JSON, CSV       | from UI      |

---

## 7. External Integration (Post-MVP)

These integrations are not part of the MVP delivery but are designed-for from v1 so that the interfaces are ready.

### 7.1 Prometheus Scraping

- The `/metrics` endpoint (section 2) is Prometheus-compatible from v1.
- Post-MVP: publish a sample `prometheus.yml` scrape config targeting `localhost:PORT/metrics`.

### 7.2 Grafana Dashboards

- Provide a JSON dashboard template (`grafana-aipanel.json`) covering:
  - Panel HTTP performance (request duration, error rate).
  - Host resources (CPU, RAM, disk).
  - Service uptime.
  - Job queue depth and throughput.
  - Backup success rate.

### 7.3 Webhook Alerts

Post-MVP alert delivery via outgoing webhooks:

| Target          | Payload Format             |
|---------------- |--------------------------- |
| Slack           | Slack Block Kit JSON       |
| Discord         | Discord embed JSON         |
| PagerDuty       | PagerDuty Events API v2    |
| Custom URL      | aiPanel alert JSON payload |

Configuration: per-alert-rule webhook URL + optional secret for HMAC signature verification.

### 7.4 Syslog Forwarding

- Forward structured logs to a remote syslog server (RFC 5424).
- Configuration: protocol (`tcp`/`udp`), host, port, facility, severity mapping.
- Use case: centralized log aggregation in environments with existing syslog infrastructure.

---

## Appendix A: Configuration Reference

All observability settings are managed in the panel configuration file.

| Setting                        | Default                  | Description                                    |
|------------------------------- |------------------------- |----------------------------------------------- |
| `logging.level`                | `info`                   | Minimum log level for stdout and file output   |
| `logging.file.enabled`         | `true`                   | Write logs to `/var/log/aipanel/`              |
| `logging.file.retention_days`  | `14`                     | Days to keep rotated log files                 |
| `metrics.enabled`              | `true`                   | Expose `/metrics` endpoint                     |
| `metrics.bind`                 | `127.0.0.1:9100`         | Address for metrics endpoint                   |
| `health.check_interval`        | `30s`                    | Interval for service health checks             |
| `alerts.disk_threshold`        | `90`                     | Disk usage percentage to trigger alert         |
| `alerts.cert_expiry_days`      | `7`                      | Days before cert expiry to trigger alert       |
| `alerts.login_fail_threshold`  | `10`                     | Failed logins in window before alert           |
| `audit.retention_days`         | `365`                    | Days to retain audit log entries               |

---

## Appendix B: Dependency Map

| Observability Feature       | Go Dependency                  | Notes                              |
|---------------------------- |------------------------------- |----------------------------------- |
| Structured logging          | `log/slog` (stdlib)            | Built-in since Go 1.21             |
| Prometheus metrics          | `prometheus/client_golang`     | De-facto standard Go metrics lib   |
| Host metrics (`/proc`)      | Custom readers or `prometheus/procfs` | Minimal, no CGO            |
| Health checks               | Custom HTTP handlers (Chi)     | No external dependency             |
| Audit trail                 | `modernc.org/sqlite`           | Pure-Go SQLite driver (no CGO)     |
| Job queue metrics           | Internal instrumentation       | Counters on the built-in queue     |
