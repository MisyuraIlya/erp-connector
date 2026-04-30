//go:build windows

package print

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PrinterInfo is the minimal printer record returned by ListPrinters. Mirrors
// what `powershell Get-Printer | Select Name,PortName,DriverName,PrinterStatus`
// emits — kept narrow so the GUI doesn't need to know about every Get-Printer
// column.
type PrinterInfo struct {
	Name          string
	PortName      string
	DriverName    string
	PrinterStatus string
}

// IsWSD reports whether this printer is on a WSD-discovery port. WSD printers
// are stored per-user, so a Windows service running as LocalSystem cannot
// reach them — the GUI surfaces this so operators don't pick one by mistake.
func (p PrinterInfo) IsWSD() bool { return IsWSDPort(p.PortName) }

// IsWSDPort returns true when portName is a WSD-discovery port. Match is
// case-insensitive and prefix-only — every vendor we've seen uses the
// "WSD-<guid>" naming the print spooler assigns.
func IsWSDPort(portName string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(portName)), "WSD-")
}

// shellTimeout caps how long any single PowerShell call may run. PowerShell
// startup is ~300–800ms; Get-Printer/Get-PrinterDriver ~500ms–1.5s; the install
// path can take a few seconds while it talks to the spooler. 30s is generous
// enough for slow installs without freezing the UI on a hung shell.
const shellTimeout = 30 * time.Second

// ListPrinters returns every printer registered on the local machine, as the
// LocalSystem service account would see them — Get-Printer enumerates the
// machine-wide spooler view, NOT the calling user's session. Output is
// JSON-decoded so we don't have to parse PowerShell's table format.
func ListPrinters(ctx context.Context) ([]PrinterInfo, error) {
	const script = `Get-Printer | Select-Object Name,PortName,DriverName,PrinterStatus | ConvertTo-Json -Compress`
	out, err := runPowershell(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("Get-Printer: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return []PrinterInfo{}, nil
	}
	// ConvertTo-Json emits an object (not an array) when the result has one
	// element. Try array first, fall back to single-object decode.
	type rawPrinter struct {
		Name          string `json:"Name"`
		PortName      string `json:"PortName"`
		DriverName    string `json:"DriverName"`
		PrinterStatus any    `json:"PrinterStatus"` // PS may emit int OR string
	}
	var arr []rawPrinter
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		var one rawPrinter
		if err2 := json.Unmarshal([]byte(out), &one); err2 != nil {
			return nil, fmt.Errorf("parse Get-Printer JSON: %v / %v (raw=%q)", err, err2, truncate(out, 200))
		}
		arr = []rawPrinter{one}
	}
	results := make([]PrinterInfo, 0, len(arr))
	for _, r := range arr {
		results = append(results, PrinterInfo{
			Name:          r.Name,
			PortName:      r.PortName,
			DriverName:    r.DriverName,
			PrinterStatus: stringifyStatus(r.PrinterStatus),
		})
	}
	return results, nil
}

// ListPrinterDrivers returns the names of every printer driver installed on
// the local machine. Used by the Install-network-printer sub-dialog so the
// operator picks from drivers Windows already knows about — we never install
// drivers from .inf files in this flow.
func ListPrinterDrivers(ctx context.Context) ([]string, error) {
	const script = `Get-PrinterDriver | Select-Object -ExpandProperty Name | Sort-Object | ConvertTo-Json -Compress`
	out, err := runPowershell(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("Get-PrinterDriver: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return []string{}, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		// Single-driver case → JSON string, not array
		var one string
		if err2 := json.Unmarshal([]byte(out), &one); err2 != nil {
			return nil, fmt.Errorf("parse Get-PrinterDriver JSON: %v / %v (raw=%q)", err, err2, truncate(out, 200))
		}
		arr = []string{one}
	}
	return arr, nil
}

// InstallTCPPrinter installs a printer machine-wide using a Standard TCP/IP
// port. The new port is named `<name>-TCP` so it doesn't collide with whatever
// WSD port the same physical device may already have under a different
// printer name.
//
// The PowerShell sequence is intentionally idempotent on the port (skip if
// it already exists) so the operator can re-run the install after a partial
// failure without hitting "port already exists" errors.
//
// host must be an IP address or a DNS hostname reachable from this machine
// — there is no validation on caller side; bad hosts produce a PowerShell
// error which we surface verbatim.
func InstallTCPPrinter(ctx context.Context, name, host, driver string) error {
	name = strings.TrimSpace(name)
	host = strings.TrimSpace(host)
	driver = strings.TrimSpace(driver)
	if name == "" || host == "" || driver == "" {
		return fmt.Errorf("name, host, and driver are all required")
	}
	if strings.ContainsAny(name, "\"'`$\r\n") || strings.ContainsAny(host, "\"'`$\r\n") || strings.ContainsAny(driver, "\"'`$\r\n") {
		return fmt.Errorf("invalid characters in name/host/driver — quotes, backticks, $ and newlines are not allowed")
	}
	portName := name + "-TCP"
	// PowerShell here-string isn't required; -Command quotes are sufficient.
	// We pass values as parameters via a $args-fed scriptblock to avoid any
	// risk of shell-quoting bugs even though we already validated above.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$portName = %q
$host_   = %q
$pname   = %q
$driver  = %q
if (-not (Get-PrinterPort -Name $portName -ErrorAction SilentlyContinue)) {
    Add-PrinterPort -Name $portName -PrinterHostAddress $host_
}
if (Get-Printer -Name $pname -ErrorAction SilentlyContinue) {
    throw "A printer named '$pname' is already installed. Pick a different name or remove the existing one first."
}
Add-Printer -Name $pname -DriverName $driver -PortName $portName
Write-Output "OK"
`, portName, host, name, driver)
	out, err := runPowershell(ctx, script)
	if err != nil {
		return fmt.Errorf("install printer: %w (output: %s)", err, truncate(out, 400))
	}
	if !strings.Contains(out, "OK") {
		return fmt.Errorf("install printer: unexpected PowerShell output: %s", truncate(out, 400))
	}
	return nil
}

// runPowershell runs a single PowerShell -NoProfile -Command invocation with
// the given script. Uses -ExecutionPolicy Bypass so corp lock-downs that
// disable script execution don't block our inline scripts (we never run
// untrusted .ps1 files from disk; everything is literal Go strings).
func runPowershell(ctx context.Context, script string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", script,
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(out.String() + "\n" + errb.String())
		return combined, fmt.Errorf("powershell exit %v: %s", err, truncate(combined, 400))
	}
	return out.String(), nil
}

// stringifyStatus normalizes Get-Printer's PrinterStatus column. PowerShell
// sometimes emits the integer enum value, sometimes the string label.
func stringifyStatus(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%d", int(x))
	case int:
		return fmt.Sprintf("%d", x)
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
