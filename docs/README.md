# Documentation Index (erp-connector)

## What this app is

`erp-connector` is a configuration UI (Fyne) and a background daemon (`erp-connectord`) that exposes a local REST API used by the main application to:
- Run **READ-only** SQL queries against ERP databases
- Read images from configured folders
- Run ERP-specific handlers (price/stock)
- (Hasavshevet only) generate “send order” output files into a configured folder

## Reading order (required)

1. `architecture.md` – components and data flow
2. `security.md` – auth model, token handling, threat model
3. `sql-validation.md` – rules and approach for SELECT-only enforcement
4. `api.md` – endpoints and payload contracts
5. `config.md` – config schema and persistence rules
6. `autostart.md` – Windows Service / systemd / launchd

## Non-goals

- No public internet exposure (bind to localhost by default).
- No arbitrary SQL execution.
- No file access outside configured folders only.

## Headless configuration (no OpenGL)

If the GUI cannot run (for example, Hyper-V "Microsoft Hyper-V Video" with no OpenGL), you can configure the app using CLI mode:

```text
erp-connector.exe --headless --show
erp-connector.exe --headless --generate-token --api-listen 127.0.0.1:8080
```
