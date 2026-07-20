//go:build windows

package servicing

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestResolveSystemDISMFromSystemDirectory(t *testing.T) {
	dir, err := systemDirectory()
	if err != nil {
		t.Fatalf("systemDirectory: %v", err)
	}
	if dir == "" || !strings.Contains(strings.ToLower(dir), "system32") {
		t.Fatalf("system directory = %q, want a System32 path", dir)
	}
	path, err := resolveSystemDISM()
	if err != nil {
		t.Fatalf("resolveSystemDISM: %v", err)
	}
	if !strings.EqualFold(filepath.Dir(path), dir) {
		t.Fatalf("resolved DISM %q not under system directory %q", path, dir)
	}
	if !strings.EqualFold(filepath.Base(path), dismExecutableName) {
		t.Fatalf("resolved DISM base = %q, want %q", filepath.Base(path), dismExecutableName)
	}
	// The resolved DISM must be an ordinary, non-reparse file.
	if err := validateOrdinaryExecutable(path); err != nil {
		t.Fatalf("system DISM failed ordinary-executable validation: %v", err)
	}
}

func TestValidateOrdinaryExecutableRejectsDirectoryAndMissing(t *testing.T) {
	dir, err := systemDirectory()
	if err != nil {
		t.Fatalf("systemDirectory: %v", err)
	}
	if err := validateOrdinaryExecutable(dir); err == nil {
		t.Fatal("directory accepted as ordinary executable")
	}
	missing := filepath.Join(dir, "foal-nonexistent-servicing-target.exe")
	if err := validateOrdinaryExecutable(missing); err == nil {
		t.Fatal("missing path accepted as ordinary executable")
	}
}

func TestAnalyzeArgsAreFixedEnglishNoRestart(t *testing.T) {
	want := []string{"/Online", "/Cleanup-Image", "/AnalyzeComponentStore", "/English", "/NoRestart"}
	if !reflect.DeepEqual(analyzeComponentStoreArgs, want) {
		t.Fatalf("analysis args = %v, want %v", analyzeComponentStoreArgs, want)
	}
}

func TestBoundedBufferTruncates(t *testing.T) {
	var b boundedBuffer
	chunk := strings.Repeat("a", maxAnalysisOutputBytes/2)
	for i := 0; i < 4; i++ {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("write %d: n=%d err=%v", i, n, err)
		}
	}
	if len(b.String()) != maxAnalysisOutputBytes {
		t.Fatalf("bounded buffer length = %d, want %d", len(b.String()), maxAnalysisOutputBytes)
	}
}

func TestValidatePeerExecutableAcceptsSelfRejectsOther(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	pid := uint32(windows.GetCurrentProcessId())
	if err := validatePeerExecutable(pid, self); err != nil {
		t.Fatalf("self peer rejected: %v", err)
	}
	if err := validatePeerExecutable(pid, `C:\definitely\not\this\foal.exe`); err == nil {
		t.Fatal("foreign executable accepted as peer")
	}
}

func TestServicingPipeSDDLGrantsRestrictedPrincipals(t *testing.T) {
	sddl, err := servicingPipeSDDL()
	if err != nil {
		t.Fatalf("servicingPipeSDDL: %v", err)
	}
	// Protected DACL granting only SYSTEM, Administrators, and the invoking user.
	for _, want := range []string{"D:P", "(A;;GA;;;SY)", "(A;;GA;;;BA)"} {
		if !strings.Contains(sddl, want) {
			t.Fatalf("SDDL %q missing %q", sddl, want)
		}
	}
	sid, err := currentUserSID()
	if err != nil {
		t.Fatalf("currentUserSID: %v", err)
	}
	if !strings.Contains(sddl, sid) {
		t.Fatalf("SDDL %q does not grant invoking user SID %q", sddl, sid)
	}
	// The descriptor must be well-formed.
	if _, err := windows.SecurityDescriptorFromString(sddl); err != nil {
		t.Fatalf("SDDL does not parse: %v", err)
	}
}
