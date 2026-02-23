# REST API Contract

Base:
- `http://127.0.0.1:<port>/api`

Headers:
- `Authorization: Bearer <token>`
- `Content-Type: application/json` (except file response)

## Health
- `GET /api/health`

Response:
```json
{ "status": "ok" }
```
Notes:
- Performs a DB connection check; on failure returns `503` with error code `DB_UNAVAILABLE`.

## SQL
- `POST /api/sql`

Request:
```json
{ "query": "...", "params": { "name": "value" } }
```

Response:
```json
{
  "api": "...",
  "status": "success",
  "rowCount": 10,
  "rows": [ { "...": "..." } ],
  "recordsets": [
    [
      { "...": "..." }
    ]
  ]
}
```

Errors:
```json
{ "error": "Query rejected", "code": "SQL_NOT_READ_ONLY" }
```

## Image folders
- `GET /api/folders/list`

Response:
```json
{
  "folders": [
    { "folderPath": "...", "files": ["..."] }
  ]
}
```

## File fetch
- `POST /api/file`

Request:
```json
{ "folderPath": "...", "fileName": "..." }
```

Response:
- Streams binary content
- Adds `Content-Type` and `Content-Length` where possible

## Hasavshevet: sendOrder
- `POST /api/sendOrder`

Request:
```json
{
  "dbName": "MYDB",
  "documentType": "ORDER",
  "userExtId": "CUST001",
  "dueDate": "2026-03-01",
  "createdDate": "2026-02-23",
  "comment": "optional free text",
  "discount": 0.0,
  "historyId": "HID-001",
  "total": 150.0,
  "currency": "ש\"ח",
  "details": [
    {
      "title": "Item name",
      "sku": "SKU-001",
      "quantity": 2.0,
      "originalPrice": 75.0,
      "singlePrice": 75.0,
      "totalPrice": 150.0,
      "discount": 0.0
    }
  ]
}
```

Notes:
- `documentType`: `ORDER` | `QUOATE` | `RETURN`
- `discount` and `total` are required even when `0`.
- `quantity` must not be zero (Hasavshevet line23 mandatory field spec).
- Processing is **asynchronous**: API returns `202` immediately; IMOVEIN files are
  written and `has.exe` is invoked in a single background worker.

Response `202 Accepted`:
```json
{ "status": "queued", "jobId": "a1b2c3d4e5f6g7h8", "meta": { "durationMs": 2 } }
```

Errors:
```json
{ "error": "Missing required fields: documentType, historyId", "code": "VALIDATION_ERROR" }
{ "error": "Order queue full; try again later", "code": "QUEUE_FULL" }
```

See `docs/hasavshevet-send-order.md` for full runbook, file format details, and config.

## priceAndStockHandler

- `POST /api/sap/priceAndStockHandler`
- `POST /api/hasavshevet/priceAndStockHandler`

Request:
```json
{
  "skuList": ["..."],
  "priceList": ["..."],
  "warehouses": ["..."],
  "userExtId": "..."
}
```

Notes:
- For Hasavshevet, `priceList` is optional and ignored.
- For Hasavshevet, pricing uses `DocumentID = 1` internally.

Response (example):
```json
{
  "items": [
    { "sku":"...", "prices": { "1": 12.34 }, "stockByWarehouse": { "1": 5 } }
  ],
  "meta": { "durationMs": 18 }
}
```

## Error format (standard)

All JSON errors should be:
```json
{
  "error": "Human readable message",
  "code": "MACHINE_READABLE_CODE",
  "details": {}
}
```
