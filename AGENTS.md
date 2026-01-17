# Agent Guidelines (erp-connector)

This repository contains a Go + Fyne application with a background REST API daemon.

## Mandatory reading order (before any code changes)

1. `docs/README.md`
2. `docs/architecture.md`
3. `docs/security.md`
4. `docs/sql-validation.md`
5. `docs/api.md`

Do not start implementing features before understanding the constraints in these docs.

## Project principles

- **Security first**
  - All API routes require `Authorization: Bearer ...`.
  - File access must prevent path traversal and must be restricted to configured folders only.
  - SQL endpoint must enforce **READ-only** queries.

- **Performance**
  - Reuse DB connections (pooling).
  - Avoid unnecessary allocations in hot paths.
  - Use streaming for file responses; do not load entire files into memory.

- **Stability**
  - Daemon must start reliably on boot and remain running without UI.
  - Config load must validate fields and fail with actionable errors.

- **Cross-platform**
  - Windows is the primary target, but Linux/macOS must not break.
  - OS-specific code must live under `internal/platform/...`.

## Repository layout rules

- Entry points only under:
  - `cmd/erp-connector` (Fyne UI)
  - `cmd/erp-connectord` (daemon/service)

- Business logic must be in `internal/`:
  - API handlers: `internal/api/handlers`
  - DB access: `internal/db`
  - ERP implementations: `internal/erp/<erp-name>`
  - File operations: `internal/files`
  - Auth: `internal/auth`
  - Platform autostart: `internal/platform/autostart`

Do not add new top-level folders unless necessary.

## API contract rules

- Backward-compatible changes only unless explicitly requested.
- All responses must be JSON except file streaming endpoint.
- Validate request bodies and return clear error objects:
  - `{"error":"...", "code":"...", "details":{...}}`

## SQL endpoint hard constraints

- Only SELECT queries allowed.
- Disallow keywords: INSERT/UPDATE/DELETE/MERGE/TRUNCATE/DROP/ALTER/EXEC/GRANT/REVOKE, etc.
- Parameters must be bound; no string concatenation.
- Add server-side timeouts and max-row limits.

See `docs/sql-validation.md`.

## File endpoints hard constraints

- Allowed folders are only those configured in UI.
- Reject absolute filename overrides and traversal (`..`, drive switching).
- Prefer allow-list checks on canonical paths.

## Change discipline

When implementing a change:
- Update docs first if the change affects architecture, API, config, or security.
- Add tests for validators (SQL + file path).
- Keep diffs small and localized to the appropriate package.

## Prohibited

- Storing secrets in logs
- Disabling auth “for debugging”
- Executing arbitrary SQL (non-SELECT)
- Returning raw DB driver errors directly to clients