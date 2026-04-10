package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// expandPath expands a leading ~ to the user's home directory.
func expandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand path: resolve home dir: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}
