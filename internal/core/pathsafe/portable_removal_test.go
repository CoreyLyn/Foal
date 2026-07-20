package pathsafe_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// TestValidatePortableRemovalPathRejectsDangerousRoots verifies the portable
// removal root policy rejects volume roots, the Windows system tree, the
// Program Files roots, the user profile root, and the AppData roots. These are
// the "do NOT permanently delete a dangerous root" guarantees from the
// Uninstall execution spec: if in doubt, skip with a stable reason.
func TestValidatePortableRemovalPathRejectsDangerousRoots(t *testing.T) {
	// Build AppData root candidates from the real environment so the test
	// matches the current user's actual roots.
	var dangerousRoots []struct {
		name string
		path string
	}
	dangerousRoots = append(dangerousRoots,
		struct{ name, path string }{"volume root", `C:\`},
		struct{ name, path string }{"windows root", `C:\Windows`},
		struct{ name, path string }{"windows descendant", `C:\Windows\System32`},
		struct{ name, path string }{"program files root", `C:\Program Files`},
		struct{ name, path string }{"program files x86 root", `C:\Program Files (x86)`},
	)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dangerousRoots = append(dangerousRoots, struct{ name, path string }{
			"user profile root", home,
		})
	}
	for _, env := range []string{"LOCALAPPDATA", "APPDATA"} {
		if root := os.Getenv(env); root != "" {
			dangerousRoots = append(dangerousRoots, struct{ name, path string }{
				strings.ToLower(env) + " root", root,
			})
		}
	}

	validator := pathsafe.NewValidator(nil)
	for _, tt := range dangerousRoots {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := validator.ValidatePortableRemovalPath(tt.path)
			if ok {
				t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false (dangerous root)", tt.path)
			}
			if reason.Code != "dangerous_root" {
				t.Fatalf("reason.Code = %q, want dangerous_root", reason.Code)
			}
			if reason.Message == "" {
				t.Fatal("reason.Message is empty")
			}
		})
	}
}

// TestValidatePortableRemovalPathAllowsProgramFilesDescendant verifies that a
// per-app directory under Program Files is NOT rejected as dangerous_root. The
// portable root policy rejects the Program Files roots themselves but allows
// per-app descendants (e.g. C:\Program Files\MyApp), which are plausible
// portable install locations. The path does not exist on disk, so the validator
// returns stat_failed rather than ok; the assertion is that the code is
// stat_failed and NOT dangerous_root, proving the root policy allowed it.
func TestValidatePortableRemovalPathAllowsProgramFilesDescendant(t *testing.T) {
	validator := pathsafe.NewValidator(nil)
	candidate := `C:\Program Files\DefinitelyNotARealApp-Portable-Test`

	reason, ok := validator.ValidatePortableRemovalPath(candidate)
	if ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false (path does not exist)", candidate)
	}
	if reason.Code == "dangerous_root" {
		t.Fatalf("reason.Code = dangerous_root, want non-dangerous_root (Program Files descendant should be eligible); message = %q", reason.Message)
	}
	if reason.Code == "protected_path" {
		t.Fatalf("reason.Code = protected_path, want non-protected_path (Program Files descendant should not hit the built-in system root check); message = %q", reason.Message)
	}
	// The path does not exist, so we expect stat_failed.
	if reason.Code != "stat_failed" {
		t.Fatalf("reason.Code = %q, want stat_failed (path does not exist)", reason.Code)
	}
}

// TestValidatePortableRemovalPathAllowsAppDataDescendant verifies that a
// per-app directory under AppData is NOT rejected as dangerous_root. Same
// rationale as the Program Files descendant test: the roots are rejected but
// per-app descendants are eligible.
func TestValidatePortableRemovalPathAllowsAppDataDescendant(t *testing.T) {
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		t.Skip("LOCALAPPDATA is not set; cannot test AppData descendant policy")
	}
	validator := pathsafe.NewValidator(nil)
	candidate := filepath.Join(appData, "DefinitelyNotARealApp-Portable-Test")

	reason, ok := validator.ValidatePortableRemovalPath(candidate)
	if ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false (path does not exist)", candidate)
	}
	if reason.Code == "dangerous_root" {
		t.Fatalf("reason.Code = dangerous_root, want non-dangerous_root (AppData descendant should be eligible); message = %q", reason.Message)
	}
	if reason.Code != "stat_failed" {
		t.Fatalf("reason.Code = %q, want stat_failed (path does not exist)", reason.Code)
	}
}

// TestValidatePortableRemovalPathAllowsRealTempDirectory verifies that a real
// directory under the user profile (a temp dir) passes all checks and returns
// ok=true. This proves the portable removal validator accepts ordinary
// user-profile descendants when they exist and are not protected, reparse, or
// hardlinked.
func TestValidatePortableRemovalPathAllowsRealTempDirectory(t *testing.T) {
	dir := t.TempDir()
	validator := pathsafe.NewValidator(nil)

	reason, ok := validator.ValidatePortableRemovalPath(dir)
	if !ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) = %#v, false; want ok (real temp dir)", dir, reason)
	}
}

// TestValidatePortableRemovalPathEnforcesUserProtectionRules verifies that
// user-defined Protection rules (deny-only) suppress portable removal targets
// the same way as other path mutations. A protected path is skipped with
// protected_path, never force-deleted.
func TestValidatePortableRemovalPathEnforcesUserProtectionRules(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "ProtectedApp")
	free := filepath.Join(dir, "FreeApp")
	for _, p := range []string{protected, free} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	validator := pathsafe.NewValidator([]string{protected})

	if reason, ok := validator.ValidatePortableRemovalPath(protected); ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false (protected)", protected)
	} else if reason.Code != "protected_path" {
		t.Fatalf("protected reason.Code = %q, want protected_path", reason.Code)
	}

	if reason, ok := validator.ValidatePortableRemovalPath(free); !ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) = %#v, false; want ok (sibling not protected)", free, reason)
	}
}

// TestValidatePortableRemovalPathRejectsUnsafeForms verifies the shared format
// checks (empty, UNC, relative, 8.3 short name) apply to the portable removal
// path the same way as ValidateDeletePath.
func TestValidatePortableRemovalPathRejectsUnsafeForms(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "empty", path: "", code: "empty_path"},
		{name: "unc path", path: `\\server\share\app`, code: "unc_path"},
		{name: "relative path", path: `relative\app`, code: "relative_path"},
		{name: "short name path", path: `C:\PROGRA~1\App`, code: "short_name_path"},
	}
	validator := pathsafe.NewValidator(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := validator.ValidatePortableRemovalPath(tt.path)
			if ok {
				t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("reason.Code = %q, want %q", reason.Code, tt.code)
			}
		})
	}
}

// TestValidatePortableRemovalPathRejectsMissingPath verifies that a path which
// does not exist is rejected with stat_failed (the revalidation guarantee: a
// portable install location must be revalidated immediately before deletion).
func TestValidatePortableRemovalPathRejectsMissingPath(t *testing.T) {
	validator := pathsafe.NewValidator(nil)
	missing := filepath.Join(t.TempDir(), "DoesNotExist")

	reason, ok := validator.ValidatePortableRemovalPath(missing)
	if ok {
		t.Fatalf("ValidatePortableRemovalPath(%q) ok = true, want false (missing path)", missing)
	}
	if reason.Code != "stat_failed" {
		t.Fatalf("reason.Code = %q, want stat_failed", reason.Code)
	}
}
