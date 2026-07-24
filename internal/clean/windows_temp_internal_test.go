package clean

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var wtInternalNow = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

func wtDaysAgo(days int) time.Time {
	return wtInternalNow.Add(-time.Duration(days) * 24 * time.Hour)
}

func TestResolveWindowsTempRootFromSystemRoot(t *testing.T) {
	for _, tc := range []struct {
		name       string
		systemRoot string
		wantOK     bool
	}{
		{"blank", "", false},
		{"whitespace", "   ", false},
		{"relative", `Windows`, false},
		{"dotted-relative", `.\Windows`, false},
		{"unc", `\\server\share`, false},
		{"absolute", `C:\Windows`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveWindowsTempRootFromSystemRoot(tc.systemRoot)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if tc.wantOK {
				want := filepath.Join(`C:\Windows`, "Temp")
				if got != want {
					t.Fatalf("root = %q, want %q", got, want)
				}
			}
		})
	}
}

// materializeStaleChild creates a direct child under root whose whole subtree is
// stamped the given age and returns its path.
func materializeStaleChild(t *testing.T, root, name string, ageDays int) string {
	t.Helper()
	child := filepath.Join(root, name)
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.tmp"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := wtDaysAgo(ageDays)
	for _, p := range []string{filepath.Join(child, "f.tmp"), child} {
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	return child
}

func wtIdentityCandidate(root, path string) CategoryIdentityCandidate {
	return CategoryIdentityCandidate{
		Path:     path,
		Category: CategoryWindowsTemp,
		fixedRootDiscovery: FixedRootDiscoveryOptions{
			Now:   wtInternalNow,
			Roots: map[string]string{CategoryWindowsTemp: root},
		},
	}
}

func TestValidateWindowsTempIdentity_AcceptsStableCandidate(t *testing.T) {
	root := t.TempDir()
	child := materializeStaleChild(t, root, "old", 40)
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, child)); !ok {
		t.Fatal("stable stale direct child must revalidate")
	}
}

func TestValidateWindowsTempIdentity_RejectsWrongCategory(t *testing.T) {
	root := t.TempDir()
	child := materializeStaleChild(t, root, "old", 40)
	candidate := wtIdentityCandidate(root, child)
	candidate.Category = "user_temp"
	reason, ok := validateFixedRootIdentity(candidate)
	if ok || reason.Code != "identity_mismatch" {
		t.Fatalf("wrong category must fail with identity_mismatch, got %#v ok=%v", reason, ok)
	}
}

func TestValidateWindowsTempIdentity_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	child := materializeStaleChild(t, root, "old", 40)
	// A grandchild is not a direct child of the root.
	drifted := filepath.Join(child, "f.tmp")
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, drifted)); ok {
		t.Fatal("non-direct-child path must be rejected")
	}
	// A sibling outside the root entirely.
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, outside)); ok {
		t.Fatal("path outside the resolved root must be rejected")
	}
}

func TestValidateWindowsTempIdentity_RejectsReparsePoint(t *testing.T) {
	root := t.TempDir()
	target := materializeStaleChild(t, root, "target", 40)
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, link)); ok {
		t.Fatal("reparse-point candidate must be rejected at revalidation")
	}
}

func TestValidateWindowsTempIdentity_RejectsFreshDrift(t *testing.T) {
	root := t.TempDir()
	// A child that is now inside the stability window (5 days old) must fail
	// revalidation even if it once qualified.
	child := materializeStaleChild(t, root, "fresh", 5)
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, child)); ok {
		t.Fatal("candidate drifted inside the stability window must be rejected")
	}
}

func TestValidateWindowsTempIdentity_RejectsMissingPath(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "gone")
	if _, ok := validateFixedRootIdentity(wtIdentityCandidate(root, missing)); ok {
		t.Fatal("missing candidate must be rejected")
	}
}

func TestValidateWindowsTempIdentity_RejectsUnresolvableRoot(t *testing.T) {
	candidate := CategoryIdentityCandidate{
		Path:     `C:\Windows\Temp\x`,
		Category: CategoryWindowsTemp,
		fixedRootDiscovery: FixedRootDiscoveryOptions{
			SystemRoot: `relative-not-abs`,
			Now:        wtInternalNow,
		},
	}
	if _, ok := validateFixedRootIdentity(candidate); ok {
		t.Fatal("unresolvable root must be rejected")
	}
}
