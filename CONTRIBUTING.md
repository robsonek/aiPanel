# Contributing to aiPanel

Thank you for your interest in contributing to aiPanel! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [How to Contribute](#how-to-contribute)
- [Development Setup](#development-setup)
- [Code Style](#code-style)
- [Commit Messages](#commit-messages)
- [Branch Naming](#branch-naming)
- [Pull Request Requirements](#pull-request-requirements)
- [Review Process](#review-process)
- [Issue Reporting](#issue-reporting)

---

## How to Contribute

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<your-username>/aiPanel.git
   cd aiPanel
   ```
3. **Create a branch** from `main` following the [branch naming convention](#branch-naming):
   ```bash
   git checkout -b feat/my-new-feature
   ```
4. **Make your changes**, ensuring they follow the [code style](#code-style) guidelines.
5. **Write tests** for your changes (unit, integration, or E2E as appropriate).
6. **Commit** using [Conventional Commits](#commit-messages) format.
7. **Push** your branch to your fork:
   ```bash
   git push origin feat/my-new-feature
   ```
8. **Open a Pull Request** against the `main` branch of the upstream repository.

---

## Development Setup

### Prerequisites

| Tool       | Version       | Purpose                          |
|------------|---------------|----------------------------------|
| Go         | 1.24+         | Backend development              |
| Node.js    | 22 LTS        | Frontend build toolchain         |
| pnpm       | Latest stable | Frontend package manager         |
| Git        | 2.40+         | Version control                  |
| Docker     | Latest stable | Integration tests (optional)     |
| lefthook   | Latest stable | Git hooks                        |

### Getting Started

```bash
# Install Go dependencies
go mod download

# Install frontend dependencies
cd web && pnpm install && cd ..

# Install git hooks
lefthook install

# Run the development environment (backend + frontend with HMR)
make dev

# Run all tests
make test        # Go unit tests
make test-fe     # Frontend unit tests (Vitest)
make lint        # Linters (golangci-lint + eslint)
```

### Building

```bash
# Full production build (frontend + Go binary)
make build

# The resulting binary is at ./aipanel
```

---

## Code Style

### Go (Backend)

- **Formatter:** All Go code must be formatted with `gofmt` (enforced by CI).
- **Linter:** All code must pass `golangci-lint` with the project configuration (`.golangci.yml`).
- **Conventions:**
  - Follow [Effective Go](https://go.dev/doc/effective_go) guidelines.
  - Use table-driven tests.
  - Keep functions short and focused.
  - Document all exported types, functions, and methods.

```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run
```

### TypeScript / React (Frontend)

- **Linter:** ESLint with the project configuration.
- **Formatter:** Prettier with the project configuration.
- **Conventions:**
  - Use functional components with hooks.
  - All text must come from i18n translation files (no hardcoded strings in UI).
  - Use TypeScript strict mode.
  - Components must support both dark and light themes via design tokens.

```bash
# Lint frontend code
cd web && pnpm lint

# Format frontend code
cd web && pnpm format
```

---

## Commit Messages

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. Every commit message must follow this format:

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type       | Description                                          |
|------------|------------------------------------------------------|
| `feat`     | A new feature                                        |
| `fix`      | A bug fix                                            |
| `docs`     | Documentation changes only                           |
| `chore`    | Maintenance tasks (deps, CI config, scripts)         |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test`     | Adding or updating tests                             |
| `perf`     | Performance improvement                              |
| `ci`       | CI/CD configuration changes                          |
| `style`    | Code style changes (formatting, semicolons, etc.)    |

### Examples

```
feat(iam): add MFA enrollment endpoint
fix(hosting): correct Nginx vhost template for wildcard domains
docs: update development setup instructions in CONTRIBUTING.md
chore(deps): bump Go to 1.24.2
refactor(backup): extract snapshot logic into separate service
test(installer): add idempotency integration test
```

---

## Branch Naming

Use the following prefixes for branch names:

| Prefix    | Purpose                          | Example                          |
|-----------|----------------------------------|----------------------------------|
| `feat/`   | New features                     | `feat/mfa-enrollment`            |
| `fix/`    | Bug fixes                        | `fix/nginx-reload-race`          |
| `docs/`   | Documentation changes            | `docs/api-endpoint-reference`    |
| `chore/`  | Maintenance and housekeeping     | `chore/update-ci-pipeline`       |

Branch names should be lowercase, use hyphens as separators, and be descriptive but concise.

---

## Pull Request Requirements

Before submitting a PR, ensure the following:

1. **Description:** Provide a clear description of what the PR does and why.
2. **Tests:** All new and modified code must have appropriate tests:
   - Unit tests for business logic (>= 80% coverage target).
   - Integration tests for adapter and API changes.
   - E2E tests for critical workflow changes.
3. **CI passing:** All CI checks must pass (lint, test, build, security scan).
4. **No failing tests:** The PR must not introduce any test failures.
5. **Security:** No high or critical findings from `gosec`, `govulncheck`, or `pnpm audit`.
6. **Documentation:** API endpoints and configuration options must be documented.
7. **i18n:** All UI text must use translation keys from locale files.
8. **Themes:** UI changes must work correctly in both dark and light modes.

### PR Template

When opening a PR, include:

- **Summary:** What changed and why (1-3 sentences).
- **Related issue:** Link to the GitHub issue (if applicable).
- **Type of change:** feat / fix / docs / chore / refactor / test.
- **Testing:** Describe how the changes were tested.
- **Screenshots:** For UI changes, include screenshots of both dark and light modes.

---

## Review Process

- Every PR requires **at least 1 approving review** before merge.
- Reviewers should check for:
  - Correctness and completeness of the implementation.
  - Test coverage and quality.
  - Code style and consistency.
  - Security implications.
  - Performance impact.
- The PR author is responsible for addressing review feedback.
- Use "Request changes" for blocking issues and "Comment" for suggestions.
- Once approved, the PR author merges (squash merge preferred for clean history).

---

## Issue Reporting

### Bug Reports

When reporting a bug, please include:

1. **Description:** A clear and concise description of the bug.
2. **Steps to reproduce:** Detailed steps to reproduce the behavior.
3. **Expected behavior:** What you expected to happen.
4. **Actual behavior:** What actually happened.
5. **Environment:**
   - OS and version (e.g., Debian 13)
   - aiPanel version
   - Browser (if UI-related)
6. **Logs:** Relevant log output (sanitize any sensitive data).
7. **Screenshots:** If applicable, especially for UI issues.

### Feature Requests

When requesting a feature:

1. **Problem:** Describe the problem or need the feature addresses.
2. **Proposed solution:** Describe your proposed approach.
3. **Alternatives considered:** Any alternative solutions you have considered.
4. **Additional context:** Any other relevant information.

### Security Vulnerabilities

**Do NOT report security vulnerabilities as public issues.** Please follow the process described in [SECURITY.md](SECURITY.md).

---

## License

By contributing to aiPanel, you agree that your contributions will be licensed under the [MIT License](LICENSE).
