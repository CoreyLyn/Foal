//go:build windows

package pathsafe

import (
	"strings"

	"golang.org/x/sys/windows"
)

// isSupportedAnalyzeVolume reports whether root (e.g. c:\) is a local fixed or
// removable volume suitable for Analyze read roots. Remote, optical, and
// unknown volume types fail closed.
func isSupportedAnalyzeVolume(volumeRoot string) bool {
	root := strings.TrimSpace(volumeRoot)
	if root == "" {
		return false
	}
	// GetDriveType requires a trailing backslash for drive roots.
	if !strings.HasSuffix(root, `\`) {
		root += `\`
	}
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	switch windows.GetDriveType(ptr) {
	case windows.DRIVE_FIXED, windows.DRIVE_REMOVABLE:
		return true
	default:
		return false
	}
}
