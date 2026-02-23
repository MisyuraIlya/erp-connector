//go:build windows

package hasavshevet

import (
	"context"
	"os/exec"
	"strings"
)

// runImporter executes has.exe with the given parameter file in workDir.
// Returns the process exit code, combined stdout+stderr output, and any exec error.
// The single-worker OrderQueue guarantees only one invocation runs at a time.
func runImporter(ctx context.Context, hasExePath, paramFile, workDir string) (int, string, error) {
	args := []string{}
	if strings.TrimSpace(paramFile) != "" {
		args = append(args, paramFile)
	}
	cmd := exec.CommandContext(ctx, hasExePath, args...)
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	return exitCode, strings.TrimSpace(string(out)), err
}
