# Agent Guidelines — erp-connector

## What this is

A Go application providing a local HTTP REST API gateway between the main app and ERP systems. Two binaries:
- **`erp-connectord`** — Background daemon/Windows service; runs the REST API server
- **`erp-connector`** — Windows-only native UI (`walk` library) for config management and service control

**Supported ERPs:** Hasavshevet (complete), SAP (skeleton — not yet implemented)
**Primary target OS:** Windows. Linux supported for daemon only.

## Mandatory reading before any code changes

1. `docs/CLAUDE.md`
2. `docs/architecture.md`
3. `docs/security.md`
4. `docs/sql-validation.md`
5. `docs/api.md`

Do not start implementing features before understanding the constraints in these docs.

## Project structure

```
cmd/
  erp-connector/       ← Windows GUI (walk library)
    main.go            ← Win32 form, config UI, service control buttons
    helpers.go         ← UI helper functions
    ui_guard_windows.go
    ui_log.go
  erp-connectord/      ← Background daemon/service
    main.go            ← Signal handling, service mode detection
    app.go             ← serverApp lifecycle (Start/Stop, DB, queue, HTTP server)
    service_windows.go ← Windows Service API integration
    service_stub.go    ← Non-Windows stub

internal/
  api/
    server.go          ← HTTP mux, route registration, timeouts
    handlers/          ← One handler per endpoint
      health.go        ← GET /api/health
      sql.go           ← POST /api/sql (SQL validator + executor)
      folders.go       ← GET /api/folders/list
      file.go          ← POST /api/file (path-safe file streaming)
      price_stock.go   ← POST /api/priceAndStockHandler
      send_order.go    ← POST /api/sendOrder (async queue)
      sql_test.go      ← SQL validation tests
      send_order_test.go
    middleware/
      auth.go          ← Bearer token validation (all routes)
      logging.go       ← Request/response logging (no secrets)
    utils/responses.go ← WriteJSON / WriteError helpers
    dto/               ← Request/response structs per endpoint
  auth/                ← Stub (future token rotation)
  config/
    model.go           ← Config struct + field validation
    io.go              ← Load/Save YAML (atomic write, 0o600 permissions)
  db/
    connect.go         ← Open() with pooling (10 open, 10 idle, 30min lifetime)
    driver_mssql.go    ← MSSQL driver registration
  erp/
    types.go           ← PriceStockRequest, PriceStockItem, PriceStockResult
    hasavshevet/       ← Complete implementation
      price_stock.go   ← FetchPriceAndStock (GPRICE_Bulk + GetOnHandStockForSkus)
      send_order.go    ← Sender.ProcessOrder pipeline
      queue.go         ← Single-worker async OrderQueue
      imovein.go       ← IMOVEIN.doc/.prm binary format (Windows-1255)
      order_number.go  ← OrderNumberStore (JSON file + mutex)
      exec_windows.go  ← Windows: has.exe execution
      exec_stub.go     ← Non-Windows stub
    sap/
      price_stock.go   ← NOT IMPLEMENTED (returns ErrNotImplemented)
  files/
    files.go           ← Path traversal prevention + ListFiles
    files_test.go
  logger/logger.go     ← LoggerService interface (Info, Error, Warn, Success, Close)
  platform/
    autostart/         ← Windows service + Linux systemd registration
    paths/             ← OS-specific config/log file paths
  secrets/             ← OS-level encrypted storage (Windows DPAPI, Unix keyring)
```

## API endpoints (all require `Authorization: Bearer <token>`)

| Route | Method | Timeout | What it does |
|-------|--------|---------|--------------|
| `/api/health` | GET | 3s | Pings DB; returns `{"status":"ok"}` or 503 |
| `/api/sql` | POST | 8s | Validates SELECT-only query, binds params, executes, returns rows + recordsets |
| `/api/folders/list` | GET | — | Returns all configured image folders with file lists |
| `/api/file` | POST | — | Path-validates `{folderPath, fileName}` against allow-list, streams binary |
| `/api/priceAndStockHandler` | POST | 12s | Routes to Hasavshevet or SAP price/stock fetch |
| `/api/sendOrder` | POST | — | Validates order, enqueues to OrderQueue, returns `202 Accepted + jobId` |

## SQL endpoint hard constraints — NEVER bypass

- **SELECT or WITH only** — any other keyword causes `SQL_NOT_READ_ONLY` error
- **No semicolons** — multi-statement queries rejected
- **No comments** — `--`, `/*`, `*/` rejected
- **Keyword blocklist:** INSERT, UPDATE, DELETE, MERGE, TRUNCATE, DROP, ALTER, CREATE, EXEC, EXECUTE, GRANT, REVOKE
- **Param binding only** — no string concatenation; all user values must be bound via `sql.Named()`
- **Row limit:** 10,000 rows max per query
- **Body limit:** 1 MiB request body

Implementation: `validateReadOnlySQL()` in `handlers/sql.go` strips string literals first (prevents false positives on literal values), then applies keyword regex. Tests in `sql_test.go`.

## File endpoint hard constraints — NEVER bypass

- `folderPath` must exactly match (after canonicalization) one of the configured `imageFolders`
- `fileName` must not contain `.`, `..`, or absolute paths
- Final resolved path re-validated with `filepath.Rel(base, target)` — reject if starts with `..`
- Symlinks in final path resolved and re-checked against base folder
- Implementation: `ResolveFilePath()` in `internal/files/files.go`. Tests in `files_test.go`.

## Authentication

All routes protected by `middleware/auth.go` — validates `Authorization: Bearer <token>` against config `BearerToken`. Returns 401 if missing, malformed, or mismatched. Token is never logged.

## Config structure

```go
type Config struct {
  ERP          ERPType    // "sap" or "hasavshevet"
  APIListen    string     // "127.0.0.1:8080"
  Debug        bool
  BearerToken  string     // never log this
  ERPUser      string
  ImageFolders []string
  SendOrderDir string     // Hasavshevet only
  HasExePath   string     // path to has.exe (Windows)
  HasBatFile   string     // alternative: digi.bat
  DB           DBConfig   // Host, Port, User, Database, Driver
}
```

Config stored at:
- Windows: `%PROGRAMDATA%\erp-connector\config.yaml`
- Linux: `/etc/erp-connector/config.yaml`

DB password stored separately in `secrets/` (OS-level encrypted: Windows DPAPI, Unix keyring).

## Hasavshevet send-order flow

1. `OrderQueue.Submit(req)` → reserves order number via `OrderNumberStore.Next()` (mutex + JSON file) → enqueues → returns jobId immediately (202 Accepted)
2. Single-worker goroutine processes FIFO via `Sender.ProcessOrder()`:
   - Queries account details + currency rate from DB
   - Builds IMOVEIN.doc + IMOVEIN.prm (fixed-width binary, Windows-1255 encoding)
   - Writes to `SendOrderDir`, copies to history subfolder
   - Executes `has.exe` or `digi.bat`
3. Queue capacity: 64 jobs; returns 503 if full

**Single-worker constraint:** Never make the order queue concurrent — IMOVEIN format requires no concurrent file writes to `SendOrderDir`.

## Go patterns

**Error handling:**
- Sentinel errors for domain faults: `var ErrFolderNotAllowed = errors.New("...")`
- Typed validation errors: `sqlValidationError{code, msg, err}`
- Public API errors via `WriteError(w, status, message, code, details)` — never return raw DB errors

**Interfaces:**
- `LoggerService` — `Info()`, `Error()`, `Warn()`, `Success()`, `Close()`. Use `noopLogger{}` in tests.
- Middleware chain: `wrap := func(h http.Handler) http.Handler { return withLog(withAuth(h)) }`

**Context:** Request contexts passed through all call chains. Custom timeouts per endpoint. 10s graceful shutdown context.

**Concurrency:** `OrderNumberStore` uses mutex + JSON file for safe concurrent increment. `OrderQueue` uses a buffered channel with single goroutine consumer.

## Tests (5 test files)

| File | What it tests |
|------|--------------|
| `handlers/sql_test.go` | SQL keyword validation, integer param detection, param coercion |
| `handlers/send_order_test.go` | Order request validation, required fields, documentType enum |
| `files/files_test.go` | Path traversal rejection, allow-list enforcement, valid path resolution |
| `erp/hasavshevet/imovein_test.go` | IMOVEIN field lengths (2891 bytes), padding, Hebrew char handling |
| `erp/hasavshevet/order_number_test.go` | Sequential increment, file persistence, concurrent access |

Run tests: `go test ./...`

## Build

```bash
# Daemon (cross-platform)
go build -o erp-connectord ./cmd/erp-connectord

# GUI (Windows only)
go build -o erp-connector.exe ./cmd/erp-connector
```

## Prohibited (zero exceptions)

- Storing secrets in logs (DB password, bearer token, user credentials)
- Disabling auth "for testing" on any route
- Executing non-SELECT SQL through the `/api/sql` endpoint
- Returning raw DB driver errors directly to API clients
- Allowing absolute path `fileName` values in file endpoint
- Making the OrderQueue worker concurrent

## Known risks

- **SAP price/stock:** `sap.FetchPriceAndStock()` returns `ErrNotImplemented` — returns 501 to client (correct). Do not silently suppress this.
- **No rate limiting:** SQL and file-list endpoints have no per-token rate limiting. Docs recommend `127.0.0.1` binding; do not expose to LAN without adding rate limiting.
- **Config in plaintext:** `config.yaml` contains bearer token in plaintext (0o600 permissions). DB password is separately encrypted via `secrets/`.

## Architecture rules

- Business logic in `internal/` packages — never in `cmd/`
- API handlers in `internal/api/handlers/` — one file per endpoint
- DB access in `internal/db/`
- ERP logic in `internal/erp/{name}/`
- File operations in `internal/files/`
- Auth in `internal/api/middleware/auth.go`
- OS-specific code in `internal/platform/` (use build tags or stub files)

## Known AI Failure Patterns (Do Not Repeat)

### SQL safety
- ❌ Adding a new SQL execution path that bypasses `validateReadOnlySQL()` — all queries must go through the validator
- ❌ String-concatenating user input into SQL — parameters must always be bound via `sql.Named()`

### File path security
- ❌ Using `filepath.Join` on user-supplied paths without canonical path checking — `..` segments bypass `filepath.Join`; always use `ResolveFilePath()` from `internal/files/files.go`
- ❌ Accepting absolute paths from API requests — reject at handler level before any filesystem operation

### Architecture
- ❌ Adding business logic to `cmd/` entrypoints — belongs in `internal/`
- ❌ Removing Bearer token check from any route "for testing"
- ❌ Making the Hasavshevet OrderQueue concurrent — IMOVEIN file format requires single-writer

### Security
- ❌ Logging the bearer token, DB password, or any user credential in any log output
- ❌ Returning raw `error.Error()` strings from DB to API clients — use generic error messages with error codes
