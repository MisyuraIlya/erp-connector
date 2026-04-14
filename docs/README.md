# Documentation Index (erp-connector)

## What this app is

`erp-connector` is a configuration UI and a background daemon (`erp-connectord`) that exposes a local REST API used by the main application to:
- Run **READ-only** SQL queries against ERP databases
- Read images from configured folders
- Run ERP-specific handlers (price/stock)
- (Hasavshevet only) generate “send order” output files into a configured folder

The GUI uses **walk** (native Win32 controls) and runs on Windows without any GPU or OpenGL requirement — including Hyper-V VMs and machines with Microsoft Basic Display Adapter.

## Reading order (required)

1. `architecture.md` – components and data flow
2. `security.md` – auth model, token handling, threat model
3. `sql-validation.md` – rules and approach for SELECT-only enforcement
4. `api.md` – endpoints and payload contracts
5. `config.md` – config schema and persistence rules
6. `autostart.md` – Windows Service / systemd / launchd
7. `hasavshevet-send-order.md` – send-order queue, IMOVEIN format, runbook
8. `pdf-email.md` – PDF generation, auto-print, email, Test Print, known pitfalls
9. `gui-migration.md` – why Fyne was replaced with walk (history/context)

## Non-goals

- No public internet exposure (bind to localhost by default).
- No arbitrary SQL execution.
- No file access outside configured folders only.

## Headless / CLI configuration

When a display is not available at all (e.g., fully headless server), configure the app from the command line:

```text
erp-connector.exe --headless --show
erp-connector.exe --headless --generate-token --api-listen 127.0.0.1:8080
erp-connector.exe --headless --db-host myserver --db-port 1433 --db-user sa --db-name ERP
erp-connector.exe --headless --db-password “secret”
erp-connector.exe --headless --test-connection
```

See `erp-connector.exe --help` for the full flag list.
