# ADR-002: SQLite as Panel Database with PostgreSQL Migration Path

- **Status:** Accepted
- **Date:** 2026-02-06

## Context

aiPanel needs an internal database to store panel configuration, user sessions, RBAC data, version state, audit events, and the job queue. This database is separate from the databases managed for hosted sites (MariaDB/PostgreSQL chosen by the user during installation).

The key constraints are:

1. **Zero additional services** — the panel should not require a separate database server process for its own operation. This simplifies the installer, reduces resource consumption, and eliminates a dependency that could fail independently.
2. **Simple backup** — the panel's own data must be trivially backupable (NFR-REL-003: daily backup of panel configuration).
3. **Write contention** — the audit log (append-only, high write volume) and job queue (frequent insert/delete) have different write patterns than the main panel data (mixed read/write). A single-writer SQLite model could bottleneck under concurrent operations.
4. **Future scalability** — while the MVP targets single-server deployments, the architecture must not prevent migration to a full RDBMS if scale demands it.
5. **Consistency guarantees** — backups of SQLite files must be consistent. Naive file copying of a SQLite database in WAL mode can produce a corrupt backup.

## Decision

### SQLite as the MVP panel database

aiPanel uses SQLite (via `modernc.org/sqlite`, pure Go) for all internal panel data. All databases run in WAL (Write-Ahead Logging) mode for concurrent read/write access.

### Three separate SQLite files

To prevent write contention between domains with fundamentally different access patterns, the panel uses three separate SQLite database files from the start:

| File | Contents | Access pattern |
|------|----------|----------------|
| `panel.db` | Configuration, user sessions, sites, users, RBAC, version state | Mixed read/write, moderate volume |
| `audit.db` | Append-only audit event log (FR-009) | High write volume, sequential append, rare deletes |
| `queue.db` | Job queue — pending, running, completed, dead-letter jobs | Frequent insert/delete, high churn |

This separation eliminates mutual write blocking between domains without changing the overall architecture. Each file has its own connection pool and its own goose migration directory (`migrations/panel/`, `migrations/audit/`, `migrations/queue/`).

### WAL mode enforcement

All three SQLite databases run with `PRAGMA journal_mode=WAL` set at connection open time. WAL mode allows concurrent readers while a single writer is active, which is essential for the panel's HTTP handlers reading data while the job queue and audit logger write concurrently.

### Backup strategy

SQLite backups must be consistent. Two approaches are supported:

1. **Checkpoint + file copy:** Execute `PRAGMA wal_checkpoint(TRUNCATE)` to flush the WAL into the main database file, then copy the `.db` file. This briefly blocks writes during the checkpoint.
2. **SQLite Online Backup API** (`sqlite3_backup`): Performs an atomic copy of the database without stopping operations. This is the preferred approach for zero-downtime backups.

Inconsistent backup (raw file copy without checkpoint or backup API) is explicitly prohibited.

### Hard migration thresholds to PostgreSQL

The following thresholds define when the panel must migrate its storage layer from SQLite to PostgreSQL. If **any** of these conditions is sustained, migration becomes mandatory:

| Threshold | Value | Rationale |
|-----------|-------|-----------|
| Job queue throughput | > 500 jobs/min sustained | SQLite single-writer model cannot sustain this rate across the queue.db file without unacceptable latency |
| Audit log size | > 10 GB | SQLite performance degrades with very large single-table databases; query planning and vacuum become expensive |
| P95 write latency | > 200 ms | Indicates write contention has exceeded what WAL mode and file separation can absorb |
| Concurrent panel sessions | > 100 | High concurrent session count amplifies read/write conflicts on panel.db (session refresh, RBAC checks) |

These thresholds are monitoring targets. The panel's monitoring module tracks these metrics, and the compliance dashboard alerts when any threshold reaches 80% of the limit.

### Repository pattern mandatory from day one

All data access is abstracted behind repository interfaces defined in `pkg/iface/`. Business logic (service layer) depends only on these interfaces, never on SQLite directly.

```
pkg/iface/
  iam.go         → type IAMRepository interface { ... }
  hosting.go     → type HostingRepository interface { ... }
  audit.go       → type AuditRepository interface { ... }
  jobqueue.go    → type JobQueueRepository interface { ... }
  versionmgr.go  → type VersionRepository interface { ... }
```

This means migrating from SQLite to PostgreSQL requires only:
1. Writing new repository implementations that use `database/sql` with a PostgreSQL driver.
2. Swapping the injected dependency in `cmd/aipanel/main.go`.
3. Running equivalent goose migrations against PostgreSQL.

No changes to business logic, handlers, or service-layer code are needed.

## Consequences

### Positive

- **Zero operational overhead** — no database server process to install, configure, monitor, or restart. The installer is simpler and the panel's attack surface is smaller.
- **Trivial backup** — panel data is three files. Backup reduces to consistent file copy or Online Backup API call (NFR-REL-003).
- **Fast startup** — no connection negotiation or authentication overhead. SQLite opens in microseconds.
- **Write isolation** — separating audit.db and queue.db from panel.db prevents the high-churn audit log and job queue from blocking configuration reads and session checks.
- **Predictable migration path** — repository pattern ensures the migration to PostgreSQL is a bounded, well-defined task that does not touch business logic.
- **Resource efficiency** — SQLite adds zero RAM overhead beyond the page cache. This contributes directly to the NFR-PERF-004 target (panel overhead <= 1.5 GB RAM).

### Negative

- **Single-writer limitation** — even with three separate files, each file has a single writer at a time. Under high concurrent write load, writes queue up. Mitigation: WAL mode reduces the impact; migration thresholds catch the problem before it affects users.
- **No built-in replication** — SQLite has no native replication. If multi-node HA is added post-MVP, the panel database must migrate to PostgreSQL regardless of the thresholds. Mitigation: multi-node is explicitly out of scope for MVP.
- **`modernc.org/sqlite` performance** — the pure Go driver is ~2-3x slower on writes than the CGO-based `mattn/go-sqlite3`. Mitigation: see ADR-001 for the rationale; the write performance is acceptable for MVP workloads.
- **Three migration directories** — maintaining separate goose migration sets for three databases adds complexity compared to a single database. Mitigation: the migration directories are well-structured (`migrations/panel/`, `migrations/audit/`, `migrations/queue/`) and each has a focused, small schema.
- **Backup coordination** — backing up three files consistently requires either sequential checkpoints or three parallel Online Backup API calls. Mitigation: the backup module handles this as a single atomic operation.

## Alternatives Considered

### Single SQLite file

Using one SQLite file for all panel data (config, audit, queue) would simplify backup and migration management. However, the write contention between the append-heavy audit log, the high-churn job queue, and the mixed-access panel data would cause P95 write latency to exceed acceptable limits well before the 500 jobs/min threshold. The three-file approach was chosen as a low-cost architectural decision that buys significant headroom.

### PostgreSQL from the start

Using PostgreSQL as the panel database from day one would eliminate all SQLite limitations. However, it would:
- Require installing and managing a PostgreSQL server process for the panel itself, increasing installer complexity and resource overhead.
- Create a hard dependency on an external service that must be running for the panel to function.
- Add ~200-500 MB of base RAM consumption before any panel data is stored.
- Conflict with the goal of a single-binary, zero-dependency deployment.

PostgreSQL remains the planned migration target when the defined thresholds are exceeded.

### Embedded key-value store (BoltDB, BadgerDB)

Embedded key-value stores would share SQLite's advantage of zero external dependencies. However, they lack SQL query capabilities, which are valuable for the audit log (filtering, time-range queries, aggregation) and the job queue (priority ordering, status transitions). The impedance mismatch between a KV store and the panel's relational data model would increase development time without meaningful benefit.

### Redis for job queue

Using Redis for the job queue would provide excellent throughput and pub/sub capabilities. However, it introduces an additional service dependency (Redis server), increases installer complexity, and adds another component to monitor and secure. The SQLite-based queue in queue.db is sufficient for MVP workloads. Post-MVP alternatives like Asynq (Redis) or River (PostgreSQL) are documented as upgrade paths when the job queue throughput threshold is exceeded.
