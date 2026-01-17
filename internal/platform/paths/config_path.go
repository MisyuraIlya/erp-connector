package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const AppName = "erp-connector"

func ConfigFilePath() (string, error) {
	switch runtime.GOOS {
	case "winsows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, AppName, "config.yaml"), nil
	case "linux":
		return filepath.Join("/etc", AppName, "config.yaml"), nil
	default:
		return "", errors.New("unsupported OS for machine-wide config")
	}
}
