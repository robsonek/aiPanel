# aiPanel

[![Status](https://img.shields.io/badge/status-planning-yellow)](docs/PRD-hosting-panel.md)
[![MVP](https://img.shields.io/badge/mvp-ready%20to%20start-blue)](docs/sprint-1-plan.md)
[![OS](https://img.shields.io/badge/os-Debian%2013-red)](docs/installer-contract.md)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

aiPanel is an open-source hosting panel for Debian 13, designed with a security-first and performance-first approach.

## Install on Fresh Debian 13 (Copy/Paste)

Run all commands below on a clean Debian 13 server.

### 1. Download latest aiPanel binary from GitHub Releases

```bash
set -euo pipefail

sudo apt-get update
sudo apt-get install -y ca-certificates curl jq tar

REPO="robsonek/aiPanel"
ARCH="$(dpkg --print-architecture)"

case "${ARCH}" in
  amd64) ASSET_ARCH="amd64" ;;
  arm64) ASSET_ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: ${ARCH}. Supported: amd64, arm64"
    exit 1
    ;;
esac

TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" | jq -r '[.[] | select(.prerelease == true)][0].tag_name')"
if [ -z "${TAG}" ] || [ "${TAG}" = "null" ]; then
  echo "No prerelease found in ${REPO}"
  exit 1
fi

ASSET="aipanel-linux-${ASSET_ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

curl -fsSL "${BASE_URL}/${ASSET}" -o "/tmp/${ASSET}"
curl -fsSL "${BASE_URL}/${ASSET}.sha256" -o "/tmp/${ASSET}.sha256"
(cd /tmp && sha256sum -c "${ASSET}.sha256")

tar -xzf "/tmp/${ASSET}" -C /tmp
sudo install -d -m 755 /etc/aipanel
sudo install -m 755 /tmp/aipanel /usr/local/bin/aipanel
sudo curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/configs/sources/lock.json" -o /etc/aipanel/sources.lock.json
sudo chmod 644 /etc/aipanel/sources.lock.json

/usr/local/bin/aipanel --help >/dev/null
echo "aiPanel binary installed successfully"
```

### 2. Run installer (interactive, recommended)

```bash
sudo aipanel install
```

### 3. Run installer (non-interactive example with reverse proxy)

```bash
set -euo pipefail

PANEL_DOMAIN="panel.example.com"
ADMIN_EMAIL="admin@example.com"
ADMIN_PASSWORD='ChangeMe12345!'

sudo aipanel install \
  --admin-email "${ADMIN_EMAIL}" \
  --admin-password "${ADMIN_PASSWORD}" \
  --reverse-proxy \
  --panel-domain "${PANEL_DOMAIN}"
```

### 4. Verify installation

```bash
sudo systemctl is-active aipanel aipanel-runtime-nginx.service aipanel-runtime-php-fpm.service aipanel-runtime-mariadb.service
curl -fsS http://127.0.0.1:8080/health
sudo tail -n 100 /var/log/aipanel/install.log
sudo cat /var/lib/aipanel/install-report.json
```

Login URL:

- reverse proxy enabled: `http://<your-panel-domain>`
- reverse proxy disabled: `http://<your-server-ip>:8080`

Installer artifacts:

- log: `/var/log/aipanel/install.log`
- report: `/var/lib/aipanel/install-report.json`
- runtime lock: `/etc/aipanel/sources.lock.json`

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
