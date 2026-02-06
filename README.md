# aiPanel

[![Status](https://img.shields.io/badge/status-planning-yellow)](docs/PRD-hosting-panel.md)
[![MVP](https://img.shields.io/badge/mvp-ready%20to%20start-blue)](docs/sprint-1-plan.md)
[![OS](https://img.shields.io/badge/os-Debian%2013-red)](docs/installer-contract.md)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

aiPanel is an open-source hosting panel for Debian 13, designed with a security-first and performance-first approach.

## Current Status

This repository is currently in the planning and architecture phase.

- PRD, ADRs, security model, CI quality gates, and Sprint 1 plan are defined.
- MVP implementation is ready to start.

## Product Direction

aiPanel targets the core hosting panel use case (cPanel/DirectAdmin style) with:

- one-shot installation on clean Debian 13,
- secure-by-default baseline (SSH hardening, firewall, fail2ban, audit logging),
- fast website provisioning and runtime management (Nginx, PHP-FPM, MariaDB/PostgreSQL),
- built-in backup/restore,
- internal API + modern responsive UI,
- dark mode and light mode from v1,
- always-latest update policy with canary rollout and auto-rollback.

## Core Decisions (MVP)

- Architecture: modular monolith.
- Backend: Go (Chi), single-binary deployment model.
- Frontend: React + TypeScript + Vite.
- Internal panel storage: SQLite (with clear PostgreSQL migration thresholds).
- Firewall layer: nftables (canonical on Debian 13).
- Default language: English, i18n-ready from day one.

## Documentation

- PRD: [`docs/PRD-hosting-panel.md`](docs/PRD-hosting-panel.md)
- Architecture decisions: [`docs/adr/`](docs/adr)
- Threat model: [`docs/threat-model.md`](docs/threat-model.md)
- Installer contract: [`docs/installer-contract.md`](docs/installer-contract.md)
- Backup/restore contract: [`docs/backup-restore-contract.md`](docs/backup-restore-contract.md)
- CI quality gates: [`docs/ci-quality-gates.md`](docs/ci-quality-gates.md)
- Observability: [`docs/observability.md`](docs/observability.md)
- Definition of Done: [`docs/definition-of-done.md`](docs/definition-of-done.md)
- Sprint 1 plan: [`docs/sprint-1-plan.md`](docs/sprint-1-plan.md)

## Contributing

Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a pull request.

## Security

Do not report vulnerabilities in public issues.

- Security policy: [`SECURITY.md`](SECURITY.md)
- Private reporting channel: `security@aipanel.dev`

## Code of Conduct

This project follows the Contributor Covenant:

- [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)

## License

MIT License. See [`LICENSE`](LICENSE).
