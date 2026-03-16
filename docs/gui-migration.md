# GUI Migration: Fyne → walk

## Problem

On Windows machines without a proper GPU (Hyper-V VMs, Azure VMs, machines with
Microsoft Basic Display Adapter), Fyne failed at startup with:

```
OpenGL is not available on this machine (likely Microsoft Hyper-V Video or
Basic Display Adapter). The GUI cannot start.
```

Fyne requires OpenGL because it renders every widget with a GPU shader pipeline.
Win32 native controls (buttons, text fields, combo boxes) have no such requirement —
they are drawn by GDI, which works on any display including software renderers and RDP.

## Solution

Replaced Fyne with **walk** (`github.com/lxn/walk`), a Go library that wraps
Win32 native controls. Walk uses GDI only, with zero GPU dependency.

## What changed

### Files removed

| File | Why |
|---|---|
| `opengl_fallback_windows.go` | Detected OpenGL failure and launched a headless console. No longer needed. |
| `opengl_fallback_stub.go` | Non-Windows stub for the above. No longer needed. |
| `log_watcher.go` | Watched log output for OpenGL error strings to trigger the fallback. No longer needed. |

### Files added

| File | Purpose |
|---|---|
| `helpers.go` | Platform-agnostic helpers (`newBearerToken`, `dbPasswordKey`, `resolveDBPassword`) moved out of the Windows-only `main.go` so they compile on all platforms. |
| `main_stub.go` | `//go:build !windows` entry point. Runs headless/CLI mode on non-Windows; exits with an error if no `--headless` flag is given. |

### Files changed

| File | Change |
|---|---|
| `main.go` | Added `//go:build windows`. Replaced all Fyne imports and widgets with walk equivalents. UI operations that call the database (test connection, save with procedure init, start/stop server) now run in background goroutines and post results back to the UI thread via `f.Synchronize(...)`. |
| `go.mod` | Added `github.com/lxn/walk`. Removed `fyne.io/fyne/v2` and all its transitive GPU/OpenGL dependencies (`go-gl/gl`, `go-gl/glfw`, `fyne-io/gl-js`, etc.). |

## Walk vs Fyne — feature mapping

| Fyne | Walk equivalent |
|---|---|
| `fyne.io/fyne/v2/app.New()` / `app.NewWindow()` | `walk.MainWindow` (declarative) |
| `widget.NewEntry()` | `walk.LineEdit` |
| `widget.NewPasswordEntry()` | `walk.LineEdit{PasswordMode: true}` |
| `widget.NewLabel()` | `walk.Label` |
| `widget.NewButton()` | `walk.PushButton` |
| `widget.NewCheck()` | `walk.CheckBox` |
| `widget.NewSelect()` | `walk.ComboBox` |
| `widget.NewSeparator()` | `walk.HSeparator` |
| `container.NewVBox()` | `walk.Composite` with `walk.VBoxLayout` |
| `container.NewHBox()` | `walk.Composite` with `walk.HBoxLayout` |
| `container.NewBorder()` | `walk.Composite` with `walk.HBoxLayout` + fixed-width button |
| `container.NewVScroll()` | `walk.ScrollView` |
| `dialog.ShowFolderOpen()` | `walk.FileDialog{}.ShowBrowseFolder()` |
| `dialog.ShowFileOpen()` | `walk.FileDialog{}.ShowOpen()` |

## Threading model change

Fyne callbacks run on the main goroutine by default. Walk callbacks also run on
the UI (main OS) thread, so the same constraint applies. However, walk does not
block the UI during a callback — if a callback does I/O, the window freezes.

The new code separates UI reads from I/O:

1. **UI thread** — read all widget values, build a `config.Config` value.
2. **Background goroutine** — perform DB operations and file writes.
3. **UI thread** (via `f.Synchronize(...)`) — update status label with result.

This prevents the window from freezing during "Test connection", "Save", and
"Start server" operations.

## Windows manifest (optional)

Walk works without an application manifest but will use the classic Win32 look
(Windows XP–style buttons). To enable ComCtl32 v6 visual styles and per-monitor
DPI awareness, generate a `.syso` resource file:

```sh
go install github.com/akavel/rsrc@latest
rsrc -manifest app.manifest -o cmd/erp-connector/rsrc.syso
```

A sample `app.manifest` template:

```xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="*"
                    name="erp-connector" type="win32"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32"
        name="Microsoft.Windows.Common-Controls" version="6.0.0.0"
        processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">
        PerMonitorV2
      </dpiAwareness>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">
        True/PM
      </dpiAware>
    </windowsSettings>
  </application>
</assembly>
```
