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
// %USERPROFILE%\AppData\LocalLow. The Known Folder must lie inside the current
// user profile: one outside it, or an unknown profile, yields empty (silent
// absence), so a process whose profile is redirected — such as an isolated
// test binary — never resolves the signed-in user's real LocalLow.
func resolveLocalAppDataLowDir() string {
	home := localAppDataLowProfileDir()
	if home == "" {
		return ""
	}
	path, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppDataLow, 0)
	if err == nil {
		path = strings.TrimSpace(path)
		if path != "" {
			if !isStrictDescendantPath(home, path) {
				return ""
			}
			return path
		}
	}
	return filepath.Join(home, "AppData", "LocalLow")
}

func localAppDataLowProfileDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Clean(home)
}
