package clean

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var wudcInternalNow = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

func wudcDaysAgo(days int) time.Time {
	return wudcInternalNow.Add(-time.Duration(days) * 24 * time.Hour)
}

func TestResolveWindowsUpdateDownloadCacheRootFromSystemRoot(t *testing.T) {
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
			got, ok := resolveWindowsUpdateDownloadCacheRootFromSystemRoot(tc.systemRoot)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if tc.wantOK {
				want := filepath.Join(`C:\Windows`, "SoftwareDistribution", "Download")
				if got != want {
					t.Fatalf("root = %q, want %q", got, want)
				}
			}
		})
	}
}

// wudcMaterializeStaleChild creates a direct child under root whose whole subtree
// is stamped the given age and returns its path.
func wudcMaterializeStaleChild(t *testing.T, root, name string, ageDays int) string {
	t.Helper()
	child := filepath.Join(root, name)
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := wudcDaysAgo(ageDays)
	for _, p := range []string{filepath.Join(child, "f.cab"), child} {
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	return child
}

func wudcIdentityCandidate(root, path string) CategoryIdentityCandidate {
	return CategoryIdentityCandidate{
		Path:     path,
		Category: CategoryWindowsUpdateDownloadCache,
		fixedRootDiscovery: FixedRootDiscoveryOptions{
			Now:   wudcInternalNow,
			Roots: map[string]string{CategoryWindowsUpdateDownloadCache: root},
		},
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_AcceptsStableCandidate(t *testing.T) {
	root := t.TempDir()
	child := wudcMaterializeStaleChild(t, root, "old", 40)
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, child)); !ok {
		t.Fatal("stable stale direct child must revalidate")
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsWrongCategory(t *testing.T) {
	root := t.TempDir()
	child := wudcMaterializeStaleChild(t, root, "old", 40)
	candidate := wudcIdentityCandidate(root, child)
	candidate.Category = "user_temp"
	reason, ok := validateFixedRootIdentity(candidate)
	if ok || reason.Code != "identity_mismatch" {
		t.Fatalf("wrong category must fail with identity_mismatch, got %#v ok=%v", reason, ok)
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	child := wudcMaterializeStaleChild(t, root, "old", 40)
	// A grandchild is not a direct child of the root.
	drifted := filepath.Join(child, "f.cab")
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, drifted)); ok {
		t.Fatal("non-direct-child path must be rejected")
	}
	// A sibling outside the root entirely.
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, outside)); ok {
		t.Fatal("path outside the resolved root must be rejected")
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsReparsePoint(t *testing.T) {
	root := t.TempDir()
	target := wudcMaterializeStaleChild(t, root, "target", 40)
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, link)); ok {
		t.Fatal("reparse-point candidate must be rejected at revalidation")
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsFreshDrift(t *testing.T) {
	root := t.TempDir()
	// A child that is now inside the stability window (5 days old) must fail
	// revalidation even if it once qualified.
	child := wudcMaterializeStaleChild(t, root, "fresh", 5)
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, child)); ok {
		t.Fatal("candidate drifted inside the stability window must be rejected")
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsMissingPath(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "gone")
	if _, ok := validateFixedRootIdentity(wudcIdentityCandidate(root, missing)); ok {
		t.Fatal("missing candidate must be rejected")
	}
}

func TestValidateWindowsUpdateDownloadCacheIdentity_RejectsUnresolvableRoot(t *testing.T) {
	candidate := CategoryIdentityCandidate{
		Path:     `C:\Windows\SoftwareDistribution\Download\x`,
		Category: CategoryWindowsUpdateDownloadCache,
		fixedRootDiscovery: FixedRootDiscoveryOptions{
			SystemRoot: `relative-not-abs`,
			Now:        wudcInternalNow,
		},
	}
	if _, ok := validateFixedRootIdentity(candidate); ok {
		t.Fatal("unresolvable root must be rejected")
	}
}
