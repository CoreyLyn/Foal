package pathsafe_test

import (
	"os"
	"path/filepath"
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
