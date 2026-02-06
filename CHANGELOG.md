# Changelog

All notable changes to this project are documented in this file.

## v0.1.1-alpha - 2026-02-06

### Fixed
- Fixed auth routing for `/api/auth/*` to avoid false 400/401 behavior on login and protected endpoints.
- Added explicit auth wrapping per route (`/api/auth/me`, `/api/auth/logout`, `/api/admin/ping`) and kept `/api/auth/login` public.
- Updated API protection test to validate a real protected path (`/api/auth/me`).

