package secrets

import (
	"erp-connector/internal/platform/paths"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrNotFound = errors.New("secret not found")
var numR = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeKey(key string) string {
	key = strings.TrimSpace(key)
	key = numR.ReplaceAllString(key, "_")
	if key == "" {
		return "empty"
	}
	return key
}

func secretFilePath(key string) (string, error) {
	cfgPath, err := paths.ConfigFilePath()
	if err != nil {
		return "", err
	}

	baseDir := filepath.Dir(cfgPath)

	safe := sanitizeKey(key)

	return filepath.Join(baseDir, "secrets", safe+".bin"), nil

}

func Set(key string, value []byte) error {
	p, err := secretFilePath(key)
	if err != nil {
		return err
	}

	errFi := os.Mkdir(filepath.Dir(p), 0o755)

	if errFi != nil {
		return errFi
	}

}
