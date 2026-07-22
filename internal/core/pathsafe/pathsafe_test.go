package pathsafe_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestValidateDeletePathRejectsUnsafeWindowsForms(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "long path protected root", path: `\\?\C:\Windows\System32`, code: "protected_path"},
		{name: "unc path", path: `\\server\share\cache`, code: "unc_path"},
		{name: "short name path", path: `C:\PROGRA~1\App`, code: "short_name_path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := pathsafe.ValidateDeletePath(tt.path)
			if ok {
				t.Fatalf("ValidateDeletePath(%q) ok = true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("reason.Code = %q, want %q", reason.Code, tt.code)
			}
			if reason.Message == "" {
				t.Fatal("reason.Message is empty")
			}
		})
	}
}

func TestValidatorRejectsUserProtectedPathAndDescendants(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "App")
	descendant := filepath.Join(protected, "Cache", "entry.tmp")
	sibling := filepath.Join(root, "Application", "entry.tmp")
	for _, path := range []string{descendant, sibling} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	validator := pathsafe.NewValidator([]string{strings.ToUpper(`\\?\` + protected)})

	for _, path := range []string{protected, descendant} {
		reason, ok := validator.ValidateDeletePath(path)
		if ok {
			t.Fatalf("ValidateDeletePath(%q) ok = true, want false", path)
		}
		if reason.Code != "protected_path" {
			t.Fatalf("reason.Code = %q, want protected_path", reason.Code)
		}
		if !strings.Contains(reason.Message, "user-defined Protection rule") {
			t.Fatalf("reason.Message = %q, want user-defined Protection rule", reason.Message)
		}
	}

	ordinaryValidator := pathsafe.NewValidator([]string{protected})
	if reason, ok := ordinaryValidator.ValidateDeletePath(`\\?\` + descendant); ok || reason.Code != "protected_path" {
		t.Fatalf("long-path descendant = %#v, %t; want protected_path", reason, ok)
	}

	if reason, ok := validator.ValidateDeletePath(sibling); !ok {
		t.Fatalf("ValidateDeletePath(%q) = %#v, false; want sibling valid", sibling, reason)
	}
}

func TestValidatorUserRulesCannotOverrideBuiltInSafety(t *testing.T) {
	validator := pathsafe.NewValidator([]string{t.TempDir()})

	reason, ok := validator.ValidateDeletePath(`C:\Windows\System32`)

	if ok || reason.Code != "protected_path" {
		t.Fatalf("ValidateDeletePath built-in protection = %#v, %t; want protected_path", reason, ok)
	}
}

func TestValidateUserScanRootRejectsDangerousRoots(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "volume root", path: `C:\`, code: "dangerous_root"},
		{name: "windows", path: `C:\Windows`, code: "dangerous_root"},
		{name: "windows child", path: `C:\Windows\Temp`, code: "dangerous_root"},
		{name: "program files", path: `C:\Program Files`, code: "dangerous_root"},
		{name: "program files child", path: `C:\Program Files\Git`, code: "dangerous_root"},
		{name: "program files x86", path: `C:\Program Files (x86)\App`, code: "dangerous_root"},
		{name: "unc", path: `\\server\share\proj`, code: "unc_path"},
		{name: "relative", path: `.\my-project`, code: "relative_path"},
		{name: "empty", path: `  `, code: "empty_path"},
		{name: "short name", path: `C:\PROGRA~1\App`, code: "short_name_path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := pathsafe.ValidateUserScanRoot(tt.path)
			if ok {
				t.Fatalf("ValidateUserScanRoot(%q) ok=true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("code = %q, want %q (message=%q)", reason.Code, tt.code, reason.Message)
			}
			if reason.Message == "" {
				t.Fatal("empty message")
			}
		})
	}
}

func TestValidateUserScanRootRejectsUserProfileRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("user home unavailable")
	}
	variants := []string{home, strings.ToUpper(home)}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		variants = append(variants, profile, `\\?\`+profile)
	}
	for _, path := range variants {
		reason, ok := pathsafe.ValidateUserScanRoot(path)
		if ok {
			t.Fatalf("ValidateUserScanRoot(%q) ok=true, want dangerous_root", path)
		}
		if reason.Code != "dangerous_root" {
			t.Fatalf("code = %q, want dangerous_root (message=%q)", reason.Code, reason.Message)
		}
		if !strings.Contains(strings.ToLower(reason.Message), "user profile") {
			t.Fatalf("message = %q, want user profile wording", reason.Message)
		}
	}
}

func TestValidateUserScanRootAllowsOrdinaryProjectPaths(t *testing.T) {
	root := t.TempDir()
	reason, ok := pathsafe.ValidateUserScanRoot(root)
	if !ok {
		t.Fatalf("ValidateUserScanRoot temp dir = %#v, false; want ok", reason)
	}
	// Nested project-style path under a user-ish absolute location.
	proj := filepath.Join(root, "src", "my-project")
	reason, ok = pathsafe.ValidateUserScanRoot(proj)
	if !ok {
		t.Fatalf("ValidateUserScanRoot project path = %#v, false; want ok", reason)
	}
	// Paths under the real user profile must still be accepted as project roots.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		underHome := filepath.Join(home, "Projects", "foal-sample")
		reason, ok = pathsafe.ValidateUserScanRoot(underHome)
		if !ok {
			t.Fatalf("ValidateUserScanRoot under home = %#v, false; want ok", reason)
		}
	}
}

func TestValidateAnalyzeReadRootAcceptsLocalVolumeAndWindowsManaged(t *testing.T) {
	tests := []string{`C:\`, `C:\Windows`, `C:\Program Files`}
	for _, path := range tests {
		reason, ok := pathsafe.ValidateAnalyzeReadRoot(path)
		if !ok {
			t.Fatalf("ValidateAnalyzeReadRoot(%q) = %#v, false; want ok", path, reason)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		reason, ok := pathsafe.ValidateAnalyzeReadRoot(home)
		if !ok {
			t.Fatalf("ValidateAnalyzeReadRoot(profile) = %#v, false; want ok", reason)
		}
	}
	root := t.TempDir()
	reason, ok := pathsafe.ValidateAnalyzeReadRoot(root)
	if !ok {
		t.Fatalf("ValidateAnalyzeReadRoot(temp) = %#v, false; want ok", reason)
	}
}

func TestValidateAnalyzeReadRootRejectsUnsupportedForms(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "unc", path: `\\server\share\proj`, code: "unc_path"},
		{name: "device path", path: `\\.\C:`, code: "device_path"},
		{name: "physical drive", path: `\\.\PhysicalDrive0`, code: "device_path"},
		{name: "empty", path: `  `, code: "empty_path"},
		{name: "relative", path: `.\my-project`, code: "relative_path"},
		{name: "short name", path: `C:\PROGRA~1`, code: "short_name_path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := pathsafe.ValidateAnalyzeReadRoot(tt.path)
			if ok {
				t.Fatalf("ValidateAnalyzeReadRoot(%q) ok=true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("code = %q, want %q (message=%q)", reason.Code, tt.code, reason.Message)
			}
		})
	}
}

func TestValidateAnalyzeReadRootDoesNotAuthorizeMutationRoots(t *testing.T) {
	// Analyze may accept volume roots; mutation-oriented root policies must still reject them.
	if reason, ok := pathsafe.ValidateUserScanRoot(`C:\`); ok || reason.Code != "dangerous_root" {
		t.Fatalf("ValidateUserScanRoot(C:\\) = %#v, %t; want dangerous_root", reason, ok)
	}
	if reason, ok := pathsafe.ValidateUserScanRoot(`C:\Windows`); ok || reason.Code != "dangerous_root" {
		t.Fatalf("ValidateUserScanRoot(C:\\Windows) = %#v, %t; want dangerous_root", reason, ok)
	}
	validator := pathsafe.NewValidator(nil)
	if reason, ok := validator.ValidatePortableRemovalPath(`C:\`); ok || reason.Code != "dangerous_root" {
		t.Fatalf("ValidatePortableRemovalPath(C:\\) = %#v, %t; want dangerous_root", reason, ok)
	}
}

func TestValidatorProtectsDescendantsOfVolumeRoot(t *testing.T) {
	validator := pathsafe.NewValidator([]string{`C:\`})

	reason, ok := validator.ValidateDeletePath(`C:\Users\corey\cache.tmp`)

	if ok || reason.Code != "protected_path" {
		t.Fatalf("ValidateDeletePath volume descendant = %#v, %t; want protected_path", reason, ok)
	}
}

func TestValidateDeletePathRejectsHardlink(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.txt")
	link := filepath.Join(dir, "linked.txt")
	if err := os.WriteFile(original, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, link); err != nil {
		t.Skipf("hardlink fixture unavailable: %v", err)
	}

	reason, ok := pathsafe.ValidateDeletePath(link)
	if ok {
		t.Fatal("ValidateDeletePath(hardlink) ok = true, want false")
	}
	if reason.Code != "hardlink_path" {
		t.Fatalf("reason.Code = %q, want hardlink_path", reason.Code)
	}
}

func TestValidateDeletePathRejectsReparsePoint(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}

	reason, ok := pathsafe.ValidateDeletePath(link)
	if ok {
		t.Fatal("ValidateDeletePath(symlink) ok = true, want false")
	}
	if reason.Code != "reparse_point" {
		t.Fatalf("reason.Code = %q, want reparse_point", reason.Code)
	}
}

func TestNormalizePathForIdentity(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty path", path: "", want: ""},
		{name: "whitespace only", path: "   \t  ", want: ""},
		{name: "lowercase preserved", path: `c:\users\corey\cache`, want: `c:\users\corey\cache`},
		{name: "uppercase normalized", path: `C:\Users\Corey\Cache`, want: `c:\users\corey\cache`},
		{name: "long path prefix stripped", path: `\\?\C:\Users\Corey\Cache`, want: `c:\users\corey\cache`},
		{name: "UNC long path not normalized (preserved as-is)", path: `\\?\UNC\server\share`, want: `\\server\share`},
		{name: "redundant separators cleaned", path: `C:\\Users\\Corey\\\Cache`, want: `c:\users\corey\cache`},
		{name: "trailing separator removed", path: `C:\Users\Corey\Cache\`, want: `c:\users\corey\cache`},
		{name: "forward slashes normalized", path: `C:/Users/Corey/Cache`, want: `c:\users\corey\cache`},
		{name: "dot segments cleaned", path: `C:\Users\.\Corey\..\Corey\Cache`, want: `c:\users\corey\cache`},
		{name: "mixed variations", path: `\\?\C:/Users/Corey\Cache/`, want: `c:\users\corey\cache`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathsafe.NormalizePathForIdentity(tt.path)
			if got != tt.want {
				t.Fatalf("NormalizePathForIdentity(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathsAreSameIdentity(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "exact match", a: `C:\Cache`, b: `C:\Cache`, want: true},
		{name: "case different", a: `C:\Cache`, b: `c:\cache`, want: true},
		{name: "with long prefix", a: `C:\Cache`, b: `\\?\C:\Cache`, want: true},
		{name: "trailing separator", a: `C:\Cache`, b: `C:\Cache\`, want: true},
		{name: "redundant separators", a: `C:\Users\Corey\Cache`, b: `C:\\Users\\Corey\\Cache`, want: true},
		{name: "forward vs back slashes", a: `C:\Users\Corey\Cache`, b: `C:/Users/Corey/Cache`, want: true},
		{name: "all variations combined", a: `C:\Users\Corey\Cache`, b: `\\?\C:/Users/Corey/Cache\`, want: true},
		{name: "different paths not same", a: `C:\Cache`, b: `C:\OtherCache`, want: false},
		{name: "sibling paths not same", a: `C:\Users\Corey\Cache`, b: `C:\Users\Other\Cache`, want: false},
		{name: "parent not same as child", a: `C:\Users\Corey`, b: `C:\Users\Corey\Cache`, want: false},
		{name: "empty vs empty", a: "", b: "   ", want: true},
		{name: "empty vs non-empty", a: "", b: `C:\Cache`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathsafe.PathsAreSameIdentity(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("PathsAreSameIdentity(%q, %q) = %t, want %t", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsEmptyOrWhitespacePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "empty string", path: "", want: true},
		{name: "spaces only", path: "   ", want: true},
		{name: "tabs and newlines", path: "\t\n  \t\n", want: true},
		{name: "non-empty path", path: `C:\Cache`, want: false},
		{name: "path with whitespace around", path: "  C:\\Cache  ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathsafe.IsEmptyOrWhitespacePath(tt.path)
			if got != tt.want {
				t.Fatalf("IsEmptyOrWhitespacePath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

// TestValidateDeletePathWindowsTempCarveOut verifies the narrow, category-owned
// carve-out for exactly %SystemRoot%\Temp: strict descendants pass the system-tree
// policy (and then fail only at Lstat because they do not exist), while the Temp
// root itself, the sibling %SystemRoot%\Logs, and the rest of the Windows tree
// stay rejected as protected_path. User Protection still applies inside the
// carve-out.
func TestValidateDeletePathWindowsTempCarveOut(t *testing.T) {
	t.Setenv("SystemRoot", `C:\Windows`)

	// A strict descendant of %SystemRoot%\Temp passes the protected-tree policy;
	// because it does not exist, it stops later at Lstat with stat_failed. The key
	// assertion is that it is NOT rejected as protected_path.
	reason, ok := pathsafe.ValidateDeletePath(`C:\Windows\Temp\nonexistent-foal-338`)
	if ok {
		t.Fatal("nonexistent carve-out descendant should still fail at Lstat")
	}
	if reason.Code == "protected_path" {
		t.Fatalf("carve-out descendant must pass the protected-tree policy, got %#v", reason)
	}
	if reason.Code != "stat_failed" {
		t.Fatalf("carve-out descendant reason = %#v, want stat_failed (passed carve-out, then missing)", reason)
	}

	for _, path := range []string{
		`C:\Windows\Temp`,                 // the carve-out root itself is never deletable
		`C:\Windows\Logs\nonexistent`,     // sibling stays rejected
		`C:\Windows\System32\nonexistent`, // rest of the tree stays rejected
		`C:\Windows\Temp2\nonexistent`,    // prefix look-alike is not the carve-out subtree
	} {
		reason, ok := pathsafe.ValidateDeletePath(path)
		if ok || reason.Code != "protected_path" {
			t.Fatalf("ValidateDeletePath(%q) = %#v, %t; want protected_path", path, reason, ok)
		}
	}

	// User Protection rules still apply inside the carve-out (deny-only).
	guarded := `C:\Windows\Temp\guarded`
	validator := pathsafe.NewValidator([]string{guarded})
	reason, ok = validator.ValidateDeletePath(guarded + `\child`)
	if ok || reason.Code != "protected_path" {
		t.Fatalf("protected descendant inside carve-out = %#v, %t; want protected_path", reason, ok)
	}
	if !strings.Contains(reason.Message, "Protection rule") {
		t.Fatalf("carve-out Protection message = %q, want user Protection rule wording", reason.Message)
	}
}

// TestWindowsTempCarveOutRequiresResolvableSystemRoot verifies that when
// SystemRoot is unusable (relative), no carve-out is granted and Temp descendants
// stay rejected as protected_path.
func TestWindowsTempCarveOutRequiresResolvableSystemRoot(t *testing.T) {
	t.Setenv("SystemRoot", `Windows`) // relative ⇒ no carve-out

	reason, ok := pathsafe.ValidateDeletePath(`C:\Windows\Temp\nonexistent-foal-338`)
	if ok || reason.Code != "protected_path" {
		t.Fatalf("without a resolvable SystemRoot the carve-out must not apply, got %#v, %t", reason, ok)
	}
}

// TestValidateDeletePathWindowsUpdateDownloadCarveOut verifies the narrow,
// category-owned carve-out for exactly %SystemRoot%\SoftwareDistribution\Download
// (ADR 0033): strict descendants pass the system-tree policy (and then fail only
// at Lstat because they do not exist), while the Download root itself, the
// SoftwareDistribution siblings DataStore and ReportingEvents, and the rest of
// the Windows tree stay rejected as protected_path. User Protection still applies
// inside the carve-out.
func TestValidateDeletePathWindowsUpdateDownloadCarveOut(t *testing.T) {
	t.Setenv("SystemRoot", `C:\Windows`)

	// A strict descendant of the Download subtree passes the protected-tree
	// policy; because it does not exist it stops later at Lstat with stat_failed.
	reason, ok := pathsafe.ValidateDeletePath(`C:\Windows\SoftwareDistribution\Download\nonexistent-foal-339`)
	if ok {
		t.Fatal("nonexistent carve-out descendant should still fail at Lstat")
	}
	if reason.Code == "protected_path" {
		t.Fatalf("carve-out descendant must pass the protected-tree policy, got %#v", reason)
	}
	if reason.Code != "stat_failed" {
		t.Fatalf("carve-out descendant reason = %#v, want stat_failed (passed carve-out, then missing)", reason)
	}

	for _, path := range []string{
		`C:\Windows\SoftwareDistribution\Download`,              // the carve-out root itself is never deletable
		`C:\Windows\SoftwareDistribution`,                       // the parent stays rejected
		`C:\Windows\SoftwareDistribution\DataStore\nonexistent`, // sibling stays rejected
		`C:\Windows\SoftwareDistribution\ReportingEvents\bad`,   // sibling stays rejected
		`C:\Windows\SoftwareDistribution\Download2\nonexistent`, // prefix look-alike is not the carve-out subtree
		`C:\Windows\System32\nonexistent`,                       // rest of the tree stays rejected
	} {
		reason, ok := pathsafe.ValidateDeletePath(path)
		if ok || reason.Code != "protected_path" {
			t.Fatalf("ValidateDeletePath(%q) = %#v, %t; want protected_path", path, reason, ok)
		}
	}

	// The Temp carve-out still coexists (both roots are carved out).
	reason, ok = pathsafe.ValidateDeletePath(`C:\Windows\Temp\nonexistent-foal-338`)
	if ok || reason.Code == "protected_path" {
		t.Fatalf("Temp carve-out must still apply alongside the Download carve-out, got %#v, %t", reason, ok)
	}

	// User Protection rules still apply inside the Download carve-out (deny-only).
	guarded := `C:\Windows\SoftwareDistribution\Download\guarded`
	validator := pathsafe.NewValidator([]string{guarded})
	reason, ok = validator.ValidateDeletePath(guarded + `\child`)
	if ok || reason.Code != "protected_path" {
		t.Fatalf("protected descendant inside carve-out = %#v, %t; want protected_path", reason, ok)
	}
	if !strings.Contains(reason.Message, "Protection rule") {
		t.Fatalf("carve-out Protection message = %q, want user Protection rule wording", reason.Message)
	}
}

// TestWindowsUpdateDownloadCarveOutRequiresResolvableSystemRoot verifies that when
// SystemRoot is unusable (relative), no Download carve-out is granted and its
// descendants stay rejected as protected_path.
func TestWindowsUpdateDownloadCarveOutRequiresResolvableSystemRoot(t *testing.T) {
	t.Setenv("SystemRoot", `Windows`) // relative ⇒ no carve-out

	reason, ok := pathsafe.ValidateDeletePath(`C:\Windows\SoftwareDistribution\Download\nonexistent-foal-339`)
	if ok || reason.Code != "protected_path" {
		t.Fatalf("without a resolvable SystemRoot the carve-out must not apply, got %#v, %t", reason, ok)
	}
}
