//go:build windows

package driverstore

import (
	"context"
	"strings"
	"testing"
)

// TestInspectDisplayPackagesIsReadOnlyAndWellFormed runs the real, read-only
// inventory. It never removes anything; hosts without third-party display
// packages simply report none.
func TestInspectDisplayPackagesIsReadOnlyAndWellFormed(t *testing.T) {
	inv, err := InspectDisplayPackages(context.Background())
	if err != nil {
		t.Fatalf("InspectDisplayPackages: %v", err)
	}
	for _, pkg := range inv.Packages {
		if !ValidPublishedName(pkg.PublishedName) {
			t.Fatalf("invalid published name %q", pkg.PublishedName)
		}
		if !strings.EqualFold(pkg.ClassGUID, DisplayClassGUID) {
			t.Fatalf("%s class = %q, want Display", pkg.PublishedName, pkg.ClassGUID)
		}
		if pkg.OriginalName == "" || strings.ContainsAny(pkg.OriginalName, `\/`) {
			t.Fatalf("%s original name = %q, want a bare INF name", pkg.PublishedName, pkg.OriginalName)
		}
		if pkg.Bytes < 0 {
			t.Fatalf("%s bytes = %d", pkg.PublishedName, pkg.Bytes)
		}
	}
	candidates := SelectSuperseded(inv)
	var total int64
	for _, pkg := range candidates {
		if pkg.InUse {
			t.Fatalf("in-use package %s selected", pkg.PublishedName)
		}
		total += pkg.Bytes
		t.Logf("candidate %s %s %s %s %s bytes=%d", pkg.PublishedName, pkg.OriginalName, pkg.Provider, pkg.DriverDate, pkg.DriverVersion, pkg.Bytes)
	}
	for _, pkg := range inv.Packages {
		if pkg.InUse {
			t.Logf("in use %s %s %s", pkg.PublishedName, pkg.OriginalName, pkg.DriverVersion)
		}
	}
	t.Logf("display packages=%d superseded candidates=%d bytes=%d", len(inv.Packages), len(candidates), total)
}

func TestRemovePackageRejectsInvalidNamesWithoutCallingWindows(t *testing.T) {
	for _, name := range []string{"", "nv_dispi.inf", `..\oem1.inf`, "oem1.inf.bak"} {
		if got := RemovePackage(name); got != RemoveOutcomeFailed {
			t.Fatalf("RemovePackage(%q) = %q, want failed", name, got)
		}
	}
}
