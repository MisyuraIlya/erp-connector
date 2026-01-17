# Configuration

## Config file location
- Windows: `%AppData%\\erp-connector\\config.yaml`
- Linux: `~/.config/erp-connector/config.yaml`
- macOS: `~/Library/Application Support/erp-connector/config.yaml`

## Config schema (recommended)

```yaml
erp:
  type: "sap"  # or "hasavshevet"

api:
  host: "127.0.0.1"
  port: 8088

auth:
  bearerToken: "CHANGE_ME"

database:
  driver: "mssql"            # e.g. mssql / hana (as implemented)
  host: "localhost"
  port: 1433
  name: "ERPDB"
  username: "sa"
  password: "..."
  params:                    # optional driver-specific
    encrypt: "disable"
    trustServerCertificate: "true"

files:
  imageFolders:
    - "P:\\images"
    - "D:\\more-images"

hasavshevet:
  sendOrderFolder: "P:\\send-orders"
```

## Validation rules

- `api.port` must be 1..65535
- `auth.bearerToken` must be non-empty (minimum length recommended)
- `files.imageFolders` can be empty, but file endpoints must still enforce allow-list
- `hasavshevet.sendOrderFolder` required only when `erp.type=hasavshevet`

## Secrets

- DB password and bearer token are secrets.
- Do not log them.
- Prefer OS-restricted permissions for the config file.
