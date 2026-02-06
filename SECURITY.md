# Security Policy

## Supported Versions

The following versions of aiPanel receive security updates:

| Version | Supported          |
|---------|--------------------|
| 1.x.x   | :white_check_mark: Currently supported |
| 0.x.x   | :white_check_mark: Pre-release, actively developed |
| < 0.1.0 | :x: Not supported  |

Only the latest release within each supported major version receives security patches. We strongly recommend always running the latest available version.

---

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

### How to Report

Send a detailed report to: **security@aipanel.dev**

### What to Include

Your report should include:

1. **Description** of the vulnerability.
2. **Steps to reproduce** the issue (proof of concept if possible).
3. **Affected component** (e.g., IAM module, Nginx adapter, installer, file manager).
4. **Impact assessment** — what an attacker could achieve by exploiting this vulnerability.
5. **Suggested fix** (optional, but appreciated).
6. **Your contact information** for follow-up questions.

### What NOT to Do

- Do not publicly disclose the vulnerability before it has been addressed.
- Do not exploit the vulnerability beyond what is necessary to demonstrate it.
- Do not access, modify, or delete data belonging to other users.

---

## Response Timeline

| Stage | Timeframe | Description |
|-------|-----------|-------------|
| **Acknowledgment** | Within 48 hours | We will confirm receipt of your report and assign a tracking ID. |
| **Assessment** | Within 7 days | We will evaluate the severity, impact, and affected versions. |
| **Fix target** | Within 30 days | We aim to develop, test, and release a fix for confirmed vulnerabilities. |
| **Notification** | At fix release | You will be notified when the fix is released. |

For critical vulnerabilities (remote code execution, authentication bypass, data exfiltration), we will prioritize and aim to release a fix as quickly as possible, potentially ahead of the 30-day target.

---

## Disclosure Policy

We follow a **coordinated disclosure** process:

1. **Reporter submits** the vulnerability via the private channel (email).
2. **We acknowledge** the report within 48 hours.
3. **We investigate and develop a fix** in a private branch.
4. **We release the fix** and publish a security advisory.
5. **Public disclosure** occurs after the fix is released and users have had reasonable time to update (typically 7 days after the fix release).
6. **Credit** is given to the reporter in the security advisory (unless they prefer to remain anonymous).

We will coordinate with the reporter on the disclosure timeline. If a vulnerability is already being actively exploited in the wild, we may accelerate the disclosure process.

---

## Security Update Process

### How We Handle Security Updates

1. **Vulnerability confirmed:** A member of the core team validates the report.
2. **Severity assessment:** We classify the vulnerability using CVSS v3.1 scoring:
   - **Critical (9.0-10.0):** Patch within 48 hours, emergency release.
   - **High (7.0-8.9):** Patch within 7 days.
   - **Medium (4.0-6.9):** Patch in the next scheduled release.
   - **Low (0.1-3.9):** Patch in the next scheduled release.
3. **Fix development:** The fix is developed and tested in a private branch.
4. **Release:** A new patch version is released with the fix.
5. **Advisory:** A GitHub Security Advisory is published with:
   - Description of the vulnerability.
   - Affected versions.
   - Fixed versions.
   - Mitigation steps (if applicable).
   - Credit to the reporter.

### Automated Security Measures

aiPanel's CI/CD pipeline includes automated security scanning:

- **gosec** — Static analysis for Go security issues (every PR).
- **govulncheck** — Known CVE scanning for Go dependencies (every PR + nightly).
- **semgrep** — Custom SAST rules for exec injection, template injection, RBAC bypass (every PR).
- **pnpm audit** — Frontend dependency vulnerability scanning (every PR + nightly).
- **trivy** — Container image scanning (on release).

---

## Security Best Practices for Users

- Always run the latest version of aiPanel.
- Enable MFA for all administrative accounts.
- Review the audit log regularly for unauthorized changes.
- Do not expose the panel port to the public internet without firewall rules.
- Use strong, unique passwords for all accounts.
- Keep the underlying Debian system updated.

---

## Contact

For security-related inquiries: **security@aipanel.dev**

For general questions and support, please use [GitHub Discussions](https://github.com/aiPanel/aiPanel/discussions).
