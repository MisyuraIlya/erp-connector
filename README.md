# erp-connector

Cross-platform desktop + background service that exposes a local REST API used by a “main app” to talk to ERP systems (SAP, Hasavshevet). Optimized for Windows environments but runs on Linux/macOS as well.

## Goals

- Provide a **high-performance local REST API** that:
  - Connects to an ERP database using credentials configured in the UI.
  - Returns query results for **READ-only** SQL requests (SELECT-only).
  - Exposes file endpoints for ERP-related image folders configured in the UI.
  - Implements ERP-specific logic for `priceAndStockHandler` (SAP and Hasavshevet).
  - (Hasavshevet only) Generates “send order” files into a configured folder.

- Provide a **Fyne UI** to configure:
  1. ERP selection (SAP / Hasavshevet)
  2. Database settings (username/password/host/db/etc) + “Test connection”
  3. REST API port configuration
  4. Image folders (N folders with “+” to add more)
  5. Hasavshevet only: “sendOrder” output folder

- Ensure the REST API **starts automatically after server restart**:
  - Service/daemon runs in background; UI does **not** need to open.
  - Supported via Windows Service / systemd / launchd (see docs).

## High-level behavior

- `erp-connectord` is the background process:
  - Loads config from OS-specific application data folder.
  - Starts REST API on configured port.
  - Uses connection pooling for DB access.
  - Validates incoming Bearer token and rejects unauthorized requests.

- `erp-connector` is the GUI:
  - Reads/writes the same config file.
  - Provides “Test DB connection”.
  - Provides “Service status” and helpful error messages (optional).

## Configuration storage

Config is stored in the OS app-data directory:
- Windows: `%AppData%\\erp-connector\\config.yaml`
- Linux: `~/.config/erp-connector/config.yaml`
- macOS: `~/Library/Application Support/erp-connector/config.yaml`

See `docs/config.md`.

## File structure
```
erp-connector/
├─ AGENTS.md
├─ README.md
├─ go.mod
├─ go.sum
├─ assets/
│  ├─ icon.png
│  └─ ui/
│     └─ screenshots/
├─ cmd/
│  ├─ erp-connector/          # GUI (Fyne) – configure & manage the service
│  │  └─ main.go
│  └─ erp-connectord/         # Background REST API daemon/service
│     └─ main.go
├─ internal/
│  ├─ app/                    # Composition root (wire dependencies)
│  │  └─ app.go
│  ├─ config/                 # Load/save config, validation, defaults
│  │  ├─ model.go
│  │  ├─ load.go
│  │  └─ save.go
│  ├─ api/
│  │  ├─ server.go
│  │  ├─ middleware/
│  │  │  ├─ auth.go
│  │  │  ├─ logging.go
│  │  │  └─ recover.go
│  │  ├─ handlers/
│  │  │  ├─ health.go
│  │  │  ├─ sql.go
│  │  │  ├─ folders.go
│  │  │  ├─ file.go
│  │  │  ├─ send_order.go          # Hasavshevet only
│  │  │  ├─ price_stock_sap.go
│  │  │  └─ price_stock_hasav.go
│  │  └─ dto/
│  │     ├─ sql.go
│  │     ├─ folders.go
│  │     ├─ send_order.go
│  │     └─ price_stock.go
│  ├─ auth/
│  │  ├─ token.go              # Bearer token validation (and rotation logic if added)
│  │  └─ errors.go
│  ├─ db/
│  │  ├─ connect.go            # Shared connection factory + pooling
│  │  ├─ query_validator.go    # Read-only SQL validation
│  │  └─ drivers/
│  │     ├─ mssql/             # Common in Windows ERP installs
│  │     └─ hana/              # Optional, if SAP HANA is used
│  ├─ erp/
│  │  ├─ types.go
│  │  ├─ sap/
│  │  │  ├─ price_stock.go
│  │  │  └─ repo.go
│  │  └─ hasavshevet/
│  │     ├─ price_stock.go
│  │     ├─ repo.go
│  │     └─ send_order.go
│  ├─ files/
│  │  ├─ folders.go            # Folder registry from config
│  │  ├─ list.go               # List files by folder
│  │  └─ open.go               # Safe file open/stream (no traversal)
│  ├─ platform/
│  │  ├─ autostart/
│  │  │  ├─ windows.go         # Windows service registration helpers
│  │  │  ├─ linux.go           # systemd unit helpers
│  │  │  └─ darwin.go          # launchd helpers
│  │  └─ paths/
│  │     └─ appdata.go         # Cross-platform config/log paths
│  ├─ service/
│  │  ├─ daemon.go             # Long-running background logic
│  │  └─ lifecycle.go          # start/stop, graceful shutdown
│  └─ ui/
│     ├─ app.go
│     ├─ screens/
│     │  ├─ erp_select.go
│     │  ├─ database.go
│     │  ├─ configuration.go
│     │  ├─ files.go
│     │  └─ hasav_send_order.go
│     └─ widgets/
│        └─ folder_picker.go
├─ docs/
│  ├─ README.md
│  ├─ architecture.md
│  ├─ api.md
│  ├─ config.md
│  ├─ security.md
│  ├─ autostart.md
│  ├─ sql-validation.md
│  └─ agents.md
└─ scripts/
   ├─ install-service-windows.ps1
   ├─ uninstall-service-windows.ps1
   ├─ systemd-install.sh
   └─ systemd-uninstall.sh
```

## Authentication

All API calls require:
- `Authorization: Bearer <token>`

Token is configured in the UI and stored in config.
See `docs/security.md`.

## REST API

Base URL:
- `http://127.0.0.1:<port>/api`

### 1) SQL (READ-only)

`POST /api/sql`

Executes a SELECT-only query with named parameters.

Request body example:
```json
{
  "query": "SELECT COUNT(1) OVER() AS TotalRows, * FROM dbo.Stock WHERE ValueDate >= TRY_CONVERT(date, @dateFrom) AND ValueDate <= TRY_CONVERT(date, @dateTo) AND (@documentType IS NULL OR DocumentID = @documentType) AND (@userExtId IS NULL OR AccountKey = @userExtId) AND (@search IS NULL OR DocNumber LIKE @search OR AccountKey LIKE @search OR AccountName LIKE @search) ORDER BY ValueDate ASC OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY",
  "params": {
    "dateFrom": "2025-12-01",
    "dateTo": "2025-12-18",
    "offset": 0,
    "pageSize": 10,
    "documentType": 3,
    "userExtId": "52074",
    "search": null
  }
}
```

Response:
```json
{
  "columns": ["TotalRows", "..."],
  "rows": [
    { "TotalRows": 123, "...": "..." }
  ],
  "meta": {
    "rowCount": 10,
    "durationMs": 12
  }
}
```

**Important rules**
- Query must be **SELECT-only** (no INSERT/UPDATE/DELETE/MERGE/TRUNCATE/DROP/ALTER/EXEC).
- Must be parameterized; parameters come from `params`.
- Server applies additional safety constraints (timeouts, max rows, etc.).

See `docs/sql-validation.md`.

### 2) Folders API (images)

A) `GET /api/folders/images`
- Returns configured image folders from UI.

Response:
```json
{
  "folders": ["P:\\\\images", "D:\\\\more-images"]
}
```

B) `POST /api/folders/list`
- Lists files under a specific folder path.

Request:
```json
{ "folderPath": "P:\\images" }
```

Response:
```json
{
  "folderPath": "P:\\\\images",
  "files": ["a.jpg", "b.png", "sub\\c.webp"]
}
```

C) `POST /api/file/{filename}`
- Returns file content (stream) for a file under a provided folder path.
- The server validates that:
  - folderPath is configured by the user
  - filename does not escape the folder (no traversal)

Request body:
```json
{ "folderPath": "P:\\images" }
```

Response:
- Binary stream with correct `Content-Type`.

### 3) Hasavshevet only: Send Order

`POST /api/sendOrder`

Receives an order payload, runs Hasavshevet-specific workflow, and writes output files into the configured “sendOrder folder” from UI.

Response:
```json
{
  "status": "ok",
  "writtenFiles": ["ORDER_12345.txt"],
  "meta": { "durationMs": 45 }
}
```

### 4) priceAndStockHandler

Same body shape for both ERPs, different internal implementation.

- `POST /api/sap/priceAndStockHandler`
- `POST /api/hasavshevet/priceAndStockHandler`

Request:
```json
{
  "skuList": ["KCP-1", "CBT-1", "MAM-1"],
  "priceList": ["1", "2"],
  "warehouses": ["1", "2", "3"],
  "userExtId": "50290"
}
```

Response (example shape; ERP-specific fields allowed):
```json
{
  "items": [
    {
      "sku": "KCP-1",
      "prices": { "1": 12.34, "2": 10.00 },
      "stockByWarehouse": { "1": 5, "2": 0, "3": 7 }
    }
  ],
  "meta": { "durationMs": 18 }
}
```

See `docs/api.md`.

## Docs

Start here:
- `docs/README.md` (index + reading order)
- `docs/architecture.md` (components + data flow)

## License

Proprietary (internal use).

