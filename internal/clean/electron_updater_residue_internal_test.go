package clean

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// electronInternalNow is the injected clock for electron-updater-residue
// identity revalidation tests.
var electronInternalNow = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

// buildElectronIdentityFixture creates a conforming -updater directory under
// root with every allowlisted file stamped 25 hours old, returning the root and
// the installer.exe candidate path.
func buildElectronIdentityFixture(t *testing.T) (root, updaterDir, installer string) {
	t.Helper()
	root = t.TempDir()
	updaterDir = filepath.Join(root, "obsidian-updater")
	pendingDir := filepath.Join(updaterDir, "pending")
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	installer = filepath.Join(updaterDir, "installer.exe")
	blockmap := filepath.Join(updaterDir, "current.blockmap")
	pendingExe := filepath.Join(pendingDir, "Obsidian 1.5.exe")
	pendingInfo := filepath.Join(pendingDir, "update-info.json")
	pendingMap := filepath.Join(pendingDir, "current.blockmap")
	for _, p := range []string{installer, blockmap, pendingExe, pendingInfo, pendingMap} {
		if err := os.WriteFile(p, []byte("payload-"+filepath.Base(p)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := electronInternalNow.Add(-25 * time.Hour)
	for _, p := range []string{installer, blockmap, pendingExe, pendingInfo, pendingMap} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	return root, updaterDir, installer
}

func electronIdentityCandidate(root, path string, size int64) CategoryIdentityCandidate {
	return CategoryIdentityCandidate{
		Path:     path,
		Bytes:    size,
		Category: CategoryElectronUpdaterResidue,
		electronUpdaterResidueDiscovery: ElectronUpdaterResidueDiscoveryOptions{
			Root: root,
			Now:  electronInternalNow,
		},
	}
}

func TestValidateElectronUpdaterResidueIdentity_AcceptsStableCandidate(t *testing.T) {
	root, _, installer := buildElectronIdentityFixture(t)
	info, err := os.Lstat(installer)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, installer, info.Size())); !ok {
		t.Fatal("stable conforming candidate must revalidate")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsWrongCategory(t *testing.T) {
	root, _, installer := buildElectronIdentityFixture(t)
	info, err := os.Lstat(installer)
	if err != nil {
		t.Fatal(err)
	}
	candidate := electronIdentityCandidate(root, installer, info.Size())
	candidate.Category = "user_temp"
	reason, ok := validateElectronUpdaterResidueIdentity(candidate)
	if ok || reason.Code != "identity_mismatch" {
		t.Fatalf("wrong category must fail with identity_mismatch, got %#v ok=%v", reason, ok)
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsPathOutsideUpdaterDir(t *testing.T) {
	root, _, _ := buildElectronIdentityFixture(t)
	// A loose file directly under LOCALAPPDATA is not within an updater dir.
	outside := filepath.Join(root, "loose.exe")
	if err := os.WriteFile(outside, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, outside, 1)); ok {
		t.Fatal("path outside an updater directory must be rejected")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsReparseCandidate(t *testing.T) {
	root, updaterDir, installer := buildElectronIdentityFixture(t)
	if err := os.Remove(installer); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(updaterDir, "link.exe")
	if err := os.Symlink(installer, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, link, 1)); ok {
		t.Fatal("reparse-point candidate must be rejected at revalidation")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsStructuralDrift(t *testing.T) {
	root, updaterDir, installer := buildElectronIdentityFixture(t)
	info, err := os.Lstat(installer)
	if err != nil {
		t.Fatal(err)
	}
	// An unknown child appeared after resolution: the directory no longer conforms.
	if err := os.WriteFile(filepath.Join(updaterDir, "stray.dat"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, installer, info.Size())); ok {
		t.Fatal("structural drift (unknown child) must be rejected at revalidation")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsMissingCandidate(t *testing.T) {
	root, _, installer := buildElectronIdentityFixture(t)
	if err := os.Remove(installer); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, installer, 1)); ok {
		t.Fatal("missing candidate must be rejected at revalidation")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsSizeDrift(t *testing.T) {
	root, _, installer := buildElectronIdentityFixture(t)
	info, err := os.Lstat(installer)
	if err != nil {
		t.Fatal(err)
	}
	// Reported size does not match the on-disk size: the file was replaced.
	if _, ok := validateElectronUpdaterResidueIdentity(electronIdentityCandidate(root, installer, info.Size()+1024)); ok {
		t.Fatal("size drift must be rejected at revalidation")
	}
}

func TestValidateElectronUpdaterResidueIdentity_RejectsUnresolvableRoot(t *testing.T) {
	_, _, installer := buildElectronIdentityFixture(t)
	info, err := os.Lstat(installer)
	if err != nil {
		t.Fatal(err)
	}
	candidate := CategoryIdentityCandidate{
		Path:     installer,
		Bytes:    info.Size(),
		Category: CategoryElectronUpdaterResidue,
		electronUpdaterResidueDiscovery: ElectronUpdaterResidueDiscoveryOptions{
			Root: "", // unresolvable LOCALAPPDATA
			Now:  electronInternalNow,
		},
	}
	if _, ok := validateElectronUpdaterResidueIdentity(candidate); ok {
		t.Fatal("unresolvable root must be rejected at revalidation")
	}
}
