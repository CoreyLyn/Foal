//go:build !windows

package clean

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveLocalAppDataLowDir returns a best-effort LocalLow path on non-Windows
// hosts (tests only). Production Clean targets Windows.
func resolveLocalAppDataLowDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, "AppData", "LocalLow")
}
