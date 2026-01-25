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
- `GET /api/folders/images`

Response:
```json
{ "folders": ["..."] }
```

- `POST /api/folders/list`

Request:
```json
{ "folderPath": "..." }
```

Response:
```json
{ "folderPath": "...", "files": ["..."] }
```

## File fetch
- `POST /api/file/{filename}`

Request:
```json
{ "folderPath": "..." }
```

Response:
- Streams binary content
- Adds `Content-Type` and `Content-Length` where possible

## Hasavshevet: sendOrder
- `POST /api/sendOrder`

Request: (app-defined order schema)

Response:
```json
{ "status":"ok", "writtenFiles":["..."], "meta":{"durationMs":45} }
```

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
