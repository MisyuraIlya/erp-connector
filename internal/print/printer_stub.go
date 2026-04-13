//go:build !windows

package print

import (
	"context"
	"errors"
)

// PrintPDF is not supported on non-Windows platforms.
func PrintPDF(_ context.Context, _, _, _ string) error {
	return errors.New("printing is only supported on Windows")
}

// DetectSumatraPDF always returns empty on non-Windows.
func DetectSumatraPDF() string {
	return ""
}
