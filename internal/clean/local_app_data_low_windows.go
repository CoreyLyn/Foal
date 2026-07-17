//go:build windows

package clean

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// resolveLocalAppDataLowDir returns the current user's LocalLow AppData base.
// Prefers Known Folder FOLDERID_LocalAppDataLow; falls back to
// %USERPROFILE%\AppData\LocalLow. Empty when neither is available.
func resolveLocalAppDataLowDir() string {
	path, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppDataLow, 0)
	if err == nil {
		path = strings.TrimSpace(path)
		if path != "" {
			return path
		}
	}
	return localAppDataLowDirFallback()
}

func localAppDataLowDirFallback() string {
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
