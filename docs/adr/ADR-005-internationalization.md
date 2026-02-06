# ADR-005: Internationalization Architecture

- **Status:** Accepted
- **Date:** 2026-02-06

## Context

aiPanel is an open-source hosting panel targeting a global audience. The PRD mandates:

1. **English as the default language** (FR-028).
2. **i18n-ready from the first version** — all UI text must come from translation files, not hardcoded strings (FR-029).
3. **One translation file per language** (FR-030).
4. **Per-user language preference** with persistence (FR-031).
5. **Adding a new language must not require code changes** — only a new translation file (FR-032).

These requirements must be satisfied while maintaining frontend performance (P95 dashboard load <= 1.5s per NFR-PERF-001) and keeping the developer experience practical for an open-source project where contributors may add translations without deep knowledge of the codebase.

The backend API also returns error messages and status strings that need to be displayed in the user's chosen language. The architecture must define a clear boundary between backend and frontend responsibilities for text localization.

## Decision

### Frontend: i18next + react-i18next

The frontend uses **i18next** with the **react-i18next** binding as the internationalization framework.

#### Translation file structure

One JSON file per language, stored in `web/src/locales/`:

```
web/src/locales/
├── en.json          # English (default, always complete)
├── pl.json          # Polish
├── de.json          # German
├── fr.json          # French
└── ...              # Additional languages
```

#### Namespaced keys per module

Translation keys are organized by feature namespace to prevent key collisions and enable selective loading:

```json
{
  "dashboard": {
    "healthScore": "Health Score",
    "serviceStatus": "Service Status",
    "alerts": "Alerts",
    "runBackupNow": "Run Backup Now"
  },
  "sites": {
    "createSite": "Create Site",
    "domainName": "Domain Name",
    "tlsStatus": "TLS Status",
    "phpVersion": "PHP Version"
  },
  "backup": {
    "scheduleBackup": "Schedule Backup",
    "restorePoint": "Restore Point",
    "lastBackup": "Last Backup"
  },
  "settings": {
    "language": "Language",
    "theme": "Theme",
    "darkMode": "Dark Mode",
    "lightMode": "Light Mode"
  },
  "errors": {
    "networkError": "Network error. Please try again.",
    "unauthorized": "You are not authorized to perform this action.",
    "notFound": "The requested resource was not found.",
    "validationFailed": "Validation failed. Please check the form."
  }
}
```

#### Lazy loading

Only the active language bundle is fetched at runtime. When a user switches language, the new bundle is loaded on demand via dynamic import. This prevents unused translations from increasing the initial bundle size.

```typescript
i18next.init({
  lng: userPreference || 'en',
  fallbackLng: 'en',
  ns: ['dashboard', 'sites', 'backup', 'security', 'updates', 'settings', 'errors', 'common'],
  defaultNS: 'common',
  backend: {
    loadPath: '/locales/{{lng}}.json',
  },
  interpolation: {
    escapeValue: false, // React handles XSS
  },
});
```

#### Features used

- **Interpolation:** Dynamic values in translations (e.g., `"sitesCount": "{{count}} sites"`).
- **Pluralization:** i18next's built-in plural rules (e.g., `"site": "{{count}} site", "site_plural": "{{count}} sites"`).
- **Context:** Gender or contextual variants where needed.
- **Fallback:** Missing keys in a non-English language fall back to the English value, ensuring the UI is never blank.

### Per-user language preference

The user's language preference is stored in the panel database (`panel.db`) as part of the user profile:

```sql
ALTER TABLE users ADD COLUMN preferred_language VARCHAR(10) DEFAULT 'en';
```

#### Preference resolution order

1. **User preference in database** (highest priority) — set explicitly by the user in Settings.
2. **Browser `Accept-Language` header** — detected on first login if no preference is stored.
3. **Default: English** — fallback when no preference is available.

When a user logs in, the backend returns the `preferred_language` field in the auth response. The frontend initializes i18next with this value. When the user changes language in Settings, the frontend:
1. Switches the active language immediately (lazy loads the new bundle).
2. Sends a PATCH request to update the user's `preferred_language` in the database.

No page reload or re-login is required to switch languages (consistent with NFR-UX-002 for theme switching).

### Backend: error keys, not translated strings

The backend API never returns user-facing translated strings. Instead, it returns machine-readable error keys that the frontend maps to translated text.

#### Backend response format

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "key": "errors.validationFailed",
    "details": [
      {
        "field": "domain",
        "key": "errors.validation.invalidDomain",
        "params": { "value": "not-a-domain" }
      }
    ]
  }
}
```

#### Frontend rendering

```typescript
const { t } = useTranslation();

// Simple error
t(error.key); // "Validation failed. Please check the form."

// Error with parameters
t(error.details[0].key, error.details[0].params);
// "Invalid domain: not-a-domain"
```

This approach ensures:
- The backend is language-agnostic — it does not need access to translation files.
- Error messages are always displayed in the user's current language, even if they switch language after the error occurred.
- Error keys are stable API contracts that do not change when translations are updated.

### Adding a new language

The process for adding a new language is:

1. Copy `web/src/locales/en.json` to `web/src/locales/{lang}.json`.
2. Translate all values in the new file.
3. The new language appears automatically in the language selector.

No code changes, no configuration changes, no recompilation. The language selector dynamically reads the available locale files.

### Translation quality safeguards

- **English as the reference:** `en.json` is the single source of truth. All other languages are measured against it for completeness.
- **Missing key detection:** In development mode, i18next logs warnings for missing keys. A Vitest test validates that all keys in `en.json` exist in every other translation file.
- **CI check:** A lint step in CI compares all locale files against `en.json` and fails if any translation file is missing keys or has orphaned keys.
- **Fallback rendering:** In production, missing translations fall back to English rather than showing raw keys.

## Consequences

### Positive

- **Zero-code language addition** — adding a new language is a single file commit. Community contributors can add translations without understanding React, Go, or the build system.
- **Performance preserved** — lazy loading ensures only one language bundle is loaded, keeping the initial bundle small and the P95 dashboard load within the 1.5s target.
- **Clean separation** — the backend never deals with translation, avoiding the complexity of server-side locale management. The frontend owns all user-facing text.
- **Stable API contract** — error keys are stable identifiers that decouple API changes from translation changes.
- **Consistent UX** — all UI text, including error messages, is always in the user's chosen language.
- **Open-source friendly** — the translation workflow (copy JSON, translate values, submit PR) is accessible to non-developers.

### Negative

- **Single large file per language** — as the panel grows, each locale file may become large (hundreds of keys). Mitigation: namespaced keys provide logical organization; splitting into multiple files per namespace is a future option if needed.
- **Translation drift** — when new English keys are added, other languages fall behind. Mitigation: CI checks detect missing keys; the missing key fallback to English ensures the UI remains functional.
- **No professional translation management** — i18next JSON files are not directly compatible with translation platforms (Crowdin, Transifex) without format conversion. Mitigation: JSON is a widely supported import/export format for these platforms; integration can be added post-MVP.
- **Error key maintenance** — developers must remember to use error keys (not strings) in backend responses, and must add corresponding translations for new error keys. Mitigation: code review checks and linting rules enforce this pattern.
- **RTL languages** — languages like Arabic or Hebrew require right-to-left layout support, which is not covered by i18next alone. Mitigation: RTL support is post-MVP; the CSS architecture (TailwindCSS + design tokens) supports `dir="rtl"` when needed.

## Alternatives Considered

### Server-side translation (Go templates with locale)

Rendering translated text on the backend using Go's `text/template` or a Go i18n library (e.g., `go-i18n`) was considered. However:
- aiPanel's frontend is a React SPA, not a server-rendered application. Backend-rendered translations would require the API to accept a locale parameter on every request, complicating the API contract.
- Switching language would require re-fetching all data from the API with the new locale, increasing load and latency.
- The frontend already has all the context needed to render translations (user preference, loaded translation file) without backend involvement.

### react-intl (FormatJS)

`react-intl` is another popular React i18n library backed by the ICU message format. It was considered but rejected because:
- The ICU message syntax is more powerful but also more complex than i18next's simple interpolation. For a hosting panel UI, i18next's feature set is sufficient.
- i18next has a larger ecosystem of plugins (lazy loading backends, language detection, pluralization rules) that map directly to the project's requirements.
- i18next's JSON format is simpler for community contributors to work with than ICU message syntax.

### Namespace-per-file (multiple JSON files per language)

Splitting translations into multiple files per namespace (e.g., `en/dashboard.json`, `en/sites.json`) was considered. However:
- The PRD explicitly states one file per language (FR-030).
- A single file per language simplifies the contribution workflow (one file to translate).
- i18next's namespace support still allows logical organization within a single file through key prefixes.
- If the single-file approach becomes unwieldy post-MVP, splitting into namespace files is backward-compatible.

### Inline translations with extraction (babel-plugin-react-intl / i18next-parser)

Writing English text directly in components and extracting keys via a build-time tool was considered. This approach:
- Provides a better developer experience (write natural English, keys are auto-generated).
- However, it creates a build-time dependency and complicates the "add a language file, no code changes" requirement.
- Auto-generated keys are less readable for translators than hand-crafted semantic keys.
- The extraction step adds fragility to the build pipeline.

The explicit key approach (`t('dashboard.healthScore')`) was chosen for its simplicity, predictability, and contributor-friendliness.
