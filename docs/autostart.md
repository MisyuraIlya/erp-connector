# Autostart / Run on Boot

The REST API daemon (`erp-connectord`) must start automatically after machine restart.

## Windows (preferred)
Run daemon as a Windows Service.
Options:
1) Native Go Windows service mode (built into `erp-connectord`)
2) Wrapper tools (less ideal, but fast) like NSSM (documented internally if used)

Installer behavior (recommended):
- Install both `erp-connector.exe` (UI) and `erp-connectord.exe` (daemon) under `Program Files`.
- Register `erp-connectord` as a Windows Service set to `start=auto`.
- Create a desktop shortcut only for the UI.

Minimum behavior:
- Service starts on boot
- Restarts on failure
- Runs without UI session

## Linux
Use `systemd` unit:
- Start on boot
- Restart=always
- Runs under a dedicated user if possible

## macOS
Use `launchd` LaunchAgent/LaunchDaemon:
- Start at login or boot depending on use case

## UI relationship
- UI is a configuration tool.
- Daemon reads config from disk and starts independently.
- On Windows, the UI `Start server` action installs/updates `erp-connectord` as a Windows Service (`start=auto`) and starts it.
- On Windows, the UI `Stop server` action stops the `erp-connectord` Windows Service.
