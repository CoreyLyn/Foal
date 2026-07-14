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
