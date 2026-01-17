# Architecture

## Components

### 1) GUI: `cmd/erp-connector` (Fyne)
Responsibilities:
- Let the user select ERP type (SAP or Hasavshevet).
- Configure DB connection parameters + “Test connection”.
- Configure REST API port.
- Configure N image folders.
- Hasavshevet only: configure sendOrder output folder.
- Write config to disk.

### 2) Daemon: `cmd/erp-connectord`
Responsibilities:
- Load config from disk.
- Start HTTP server on configured port.
- Enforce auth (Bearer token).
- Route requests to handlers:
  - SQL read-only executor
  - folders/files
  - ERP handlers
  - sendOrder (Hasavshevet only)

### 3) Internal packages
- `internal/config`: config model, validation, persistence
- `internal/api`: HTTP server + routing + middleware + handlers + DTO
- `internal/auth`: token validation
- `internal/db`: DB connections, query execution, SQL validation
- `internal/files`: folder registry, file listing, secure file open/stream
- `internal/erp`: ERP-specific logic (SAP/Hasavshevet)
- `internal/platform`: autostart helpers and OS paths

## Data flow

1. User configures settings in UI → config persisted.
2. Daemon starts (boot/service) → loads config → starts REST server.
3. Main app calls REST endpoints with Bearer token.
4. Daemon validates token:
   - Reject unauthorized requests.
5. Daemon executes logic:
   - SQL: validate SELECT-only → bind params → query → return rows
   - Files: validate folder allow-list → list/stream
   - ERP handlers: run ERP-specific DB queries and mapping
   - sendOrder: run Hasavshevet workflow → write file(s) to configured folder

## Key constraints

- Daemon must run without UI.
- SQL endpoint must be SELECT-only.
- File access must be restricted to configured folders only.
- Prefer localhost binding unless explicitly configured otherwise.
