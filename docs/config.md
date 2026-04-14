# Configuration

## Config file location
- Windows: `%PROGRAMDATA%\erp-connector\config.yaml`
- Linux: `/etc/erp-connector/config.yaml`

## Current schema (implemented)

```yaml
erp: "hasavshevet"           # or "sap" / "priority"
apiListen: "127.0.0.1:8080"
debug: false
bearerToken: "CHANGE_ME"
erpUser: ""
imageFolders:
  - 'P:\images'
sendOrderDir: 'P:\send-orders'  # required only when erp=hasavshevet
hasBatFile:   'C:\Hash7\digi.bat'
db:
  driver: "mssql"
  host: "localhost"
  port: 1433
  user: "sa"
  database: "ERPDB"
  # DB password stored in OS secrets (Windows DPAPI), not here
pdf:
  companyName:     "My Company Ltd."
  companyAddress:  "123 Main St, Tel Aviv"
  companyPhone:    "03-1234567"
  companyFax:      "03-7654321"
  companyEmail:    "office@mycompany.co.il"
  logoPath:        'C:\images\logo.png'  # PNG/JPEG/GIF/WebP/BMP
  footerHTML:      ""
  chromePath:      ""          # auto-detected if empty
  sumatraPdfPath:  ""          # auto-detected if empty
  printAfterOrder: false
  printerName:     ""          # empty = Windows default printer
  emailAfterOrder: false
smtp:
  host:        "smtp.gmail.com"
  port:        587
  user:        "me@gmail.com"
  fromAddress: "me@gmail.com"
  useTLS:      true
  # SMTP password stored in OS secrets (Windows DPAPI), not here
```

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
- DB password is stored separately from config; the UI leaves the password field blank unless you enter a new value.
- Do not log them.
- Prefer OS-restricted permissions for the config file.
