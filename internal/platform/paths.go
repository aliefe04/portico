package platform

import (
	"os"
	"path/filepath"
)

func UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func DefaultSSHConfigPath(home string) string {
	if home == "" {
		return ""
	}

	return filepath.Join(home, ".ssh", "config")
}
