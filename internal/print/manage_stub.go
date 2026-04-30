//go:build !windows

package print

import (
	"context"
	"errors"
)

// PrinterInfo mirrors the windows build so callers in non-build-tagged files
// (or future cross-platform UI) can compile against a consistent type.
type PrinterInfo struct {
	Name          string
	PortName      string
	DriverName    string
	PrinterStatus string
}

func (PrinterInfo) IsWSD() bool { return false }

func IsWSDPort(string) bool { return false }

var errNotSupported = errors.New("printer management is only supported on Windows")

// ListPrinters always returns errNotSupported on non-Windows. The GUI is
// windows-only via build tags, so this stub exists only to keep Linux daemon
// builds green when shared code transitively imports the package.
func ListPrinters(_ context.Context) ([]PrinterInfo, error) {
	return nil, errNotSupported
}

func ListPrinterDrivers(_ context.Context) ([]string, error) {
	return nil, errNotSupported
}

func InstallTCPPrinter(_ context.Context, _, _, _ string) error {
	return errNotSupported
}
