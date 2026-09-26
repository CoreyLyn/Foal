//go:build windows

package driverstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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

func TestInspectDisplayPackageIdentitiesMatchesMeasuredInventory(t *testing.T) {
	measured, err := InspectDisplayPackages(context.Background())
	if err != nil {
		t.Fatalf("InspectDisplayPackages: %v", err)
	}
	identities, err := InspectDisplayPackageIdentities(context.Background())
	if err != nil {
		t.Fatalf("InspectDisplayPackageIdentities: %v", err)
	}
	if len(identities.Packages) != len(measured.Packages) {
		t.Fatalf("identity inventory has %d packages, measured %d", len(identities.Packages), len(measured.Packages))
	}
	for i, pkg := range identities.Packages {
		if pkg.Bytes != 0 || !IdentityOf(measured.Packages[i]).Matches(pkg) || pkg.InUse != measured.Packages[i].InUse {
			t.Fatalf("identity %#v does not match measured %#v", pkg, measured.Packages[i])
		}
	}
}

// fakeUninstall replaces the SetupUninstallOEMInfW call for one test so no test
// ever reaches the real API.
func fakeUninstall(t *testing.T, removed bool, callErr error) *[]string {
	t.Helper()
	calls := &[]string{}
	original := uninstallOEMInf
	uninstallOEMInf = func(name *uint16) (bool, error) {
		*calls = append(*calls, windows.UTF16PtrToString(name))
		return removed, callErr
	}
	t.Cleanup(func() { uninstallOEMInf = original })
	return calls
}

func TestRemovePackageRejectsInvalidNamesWithoutCallingWindows(t *testing.T) {
	calls := fakeUninstall(t, true, nil)
	for _, name := range []string{"", "nv_dispi.inf", `..\oem1.inf`, "oem1.inf.bak"} {
		if got := RemovePackage(name); got != RemoveOutcomeFailed {
			t.Fatalf("RemovePackage(%q) = %q, want failed", name, got)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("invalid names reached the uninstall API: %v", *calls)
	}
}

func TestRemovePackageMapsUninstallOutcomes(t *testing.T) {
	for _, tc := range []struct {
		removed bool
		err     error
		want    RemoveOutcome
	}{
		{removed: true, want: RemoveOutcomeRemoved},
		{err: windows.ERROR_INF_IN_USE_BY_DEVICES, want: RemoveOutcomeInUse},
		{err: windows.ERROR_ACCESS_DENIED, want: RemoveOutcomeFailed},
	} {
		calls := fakeUninstall(t, tc.removed, tc.err)
		if got := RemovePackage("oem999999.inf"); got != tc.want {
			t.Fatalf("RemovePackage with (%v, %v) = %q, want %q", tc.removed, tc.err, got, tc.want)
		}
		if len(*calls) != 1 || (*calls)[0] != "oem999999.inf" {
			t.Fatalf("uninstall calls = %v", *calls)
		}
	}
}

func TestIsReparseDetectsJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v %s", err, out)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !isReparse(info) {
		t.Fatal("junction not reported as a reparse point")
	}
	if _, err := measurePackageDir(context.Background(), link); err == nil {
		t.Fatal("a junction was measured as a package directory")
	}
	plain, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if isReparse(plain) {
		t.Fatal("plain directory reported as a reparse point")
	}
}
