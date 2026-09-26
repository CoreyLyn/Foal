//go:build windows

package clean

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestResolveLocalAppDataLowDirStaysInsideUserProfile(t *testing.T) {
	known, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppDataLow, 0)
	if err != nil || known == "" {
		t.Skip("LocalLow known folder is unavailable")
	}
	profile, err := windows.KnownFolderPath(windows.FOLDERID_Profile, 0)
	if err != nil || profile == "" {
		t.Skip("profile known folder is unavailable")
	}

	// A redirected profile (as in the test sandbox) never reaches the real LocalLow.
	t.Setenv("USERPROFILE", t.TempDir())
	if got := resolveLocalAppDataLowDir(); got != "" {
		t.Fatalf("redirected profile resolved LocalLow %q outside it", got)
	}

	// The real profile still resolves the Known Folder.
	t.Setenv("USERPROFILE", profile)
	if got := resolveLocalAppDataLowDir(); got != known {
		t.Fatalf("resolveLocalAppDataLowDir() = %q, want Known Folder %q", got, known)
	}
}
