# Autostart / Run on Boot

The REST API daemon (`erp-connectord`) must start automatically after machine restart.

## Windows (preferred)
Run daemon as a Windows Service.
Options:
1) Native Go Windows service implementation (`internal/platform/autostart/windows.go`)
2) Wrapper tools (less ideal, but fast) like NSSM (documented internally if used)

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
