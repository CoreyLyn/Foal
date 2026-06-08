//go:build windows

package uninstall

import "testing"

func TestOrphanedResidueRootsUseOnlyAppDataAndLocalAppData(t *testing.T) {
	t.Setenv("APPDATA", "APPDATA_ROOT")
	t.Setenv("LOCALAPPDATA", "LOCAL_ROOT")
	t.Setenv("PROGRAMDATA", "PROGRAMDATA_ROOT")

	roots := orphanedResidueRoots()

	if len(roots) != 2 {
		t.Fatalf("roots = %#v, want APPDATA and LOCALAPPDATA only", roots)
	}
	if roots[0] != "APPDATA_ROOT" || roots[1] != "LOCAL_ROOT" {
		t.Fatalf("roots = %#v, want APPDATA then LOCALAPPDATA", roots)
	}
	for _, root := range roots {
		if root == "PROGRAMDATA_ROOT" {
			t.Fatalf("roots = %#v, must not include PROGRAMDATA", roots)
		}
	}
}
