//go:build windows

package print

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"erp-connector/internal/logger"
)

// PrintPDF sends a PDF file to the printer via SumatraPDF.
// If printerName is empty, the system default printer is used.
// log may be nil; when non-nil, diagnostic info is recorded around the print call.
func PrintPDF(ctx context.Context, pdfPath, printerName, sumatraPDFPath string, log logger.LoggerService) error {
	sumatraPath := resolveSumatraPDF(sumatraPDFPath)
	if log != nil {
		printerDisplay := printerName
		if printerDisplay == "" {
			printerDisplay = "<system default>"
		}
		resolvedDisplay := sumatraPath
		if resolvedDisplay == "" {
			resolvedDisplay = "<not found>"
		}
		log.Info(fmt.Sprintf(
			"print.PrintPDF: pdfPath=%q printer=%s configuredSumatra=%q resolvedSumatra=%s",
			pdfPath, printerDisplay, sumatraPDFPath, resolvedDisplay,
		))
	}
	if sumatraPath == "" {
		return fmt.Errorf("SumatraPDF not found; install it or set the path in config")
	}

	args := []string{"-silent"}
	if printerName != "" {
		args = append(args, "-print-to", printerName)
	} else {
		args = append(args, "-print-to-default")
	}
	args = append(args, pdfPath)

	printCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if log != nil {
		log.Info(fmt.Sprintf("print.PrintPDF exec: %s %s", sumatraPath, strings.Join(args, " ")))
	}

	cmd := exec.CommandContext(printCtx, sumatraPath, args...)
	output, err := cmd.CombinedOutput()
	trimmedOutput := strings.TrimSpace(string(output))
	if err != nil {
		return fmt.Errorf("SumatraPDF print failed (exit: %v, output: %q): %w", cmd.ProcessState.ExitCode(), trimmedOutput, err)
	}
	if log != nil {
		log.Info(fmt.Sprintf("print.PrintPDF exec ok (exit=%d, output=%q)", cmd.ProcessState.ExitCode(), trimmedOutput))
	}
	return nil
}

// DetectSumatraPDF searches for SumatraPDF.exe in common locations.
func DetectSumatraPDF() string {
	return resolveSumatraPDF("")
}

func resolveSumatraPDF(configPath string) string {
	if configPath != "" {
		if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
			return configPath
		}
	}

	// Search next to our own executable
	if exePath, err := os.Executable(); err == nil {
		dir := filepath.Dir(exePath)
		candidate := filepath.Join(dir, "SumatraPDF.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	// Search working directory
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, "SumatraPDF.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	// Search PATH
	if p, err := exec.LookPath("SumatraPDF.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("SumatraPDF"); err == nil {
		return p
	}

	// Common install locations
	programFiles := os.Getenv("ProgramFiles")
	localAppData := os.Getenv("LOCALAPPDATA")
	candidates := []string{
		filepath.Join(programFiles, "SumatraPDF", "SumatraPDF.exe"),
		filepath.Join(localAppData, "SumatraPDF", "SumatraPDF.exe"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}

	return ""
}
