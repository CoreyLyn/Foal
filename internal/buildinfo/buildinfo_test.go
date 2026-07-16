package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrentReturnsSanitizedLinkerAndRuntimeMetadata(t *testing.T) {
	originalVersion, originalCommit := Version, Commit
	t.Cleanup(func() {
		Version, Commit = originalVersion, originalCommit
	})

	Version = "  v0.1.0-rc.1  "
	Commit = "  abc123  "

	got := Current()
	if got.Version != "v0.1.0-rc.1" {
		t.Fatalf("Version = %q, want v0.1.0-rc.1", got.Version)
	}
	if got.Commit != "abc123" {
		t.Fatalf("Commit = %q, want abc123", got.Commit)
	}
	if got.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
	}
	if got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
		t.Fatalf("platform = %s/%s, want %s/%s", got.OS, got.Arch, runtime.GOOS, runtime.GOARCH)
	}
}

func TestCurrentFallsBackForBlankLinkerMetadata(t *testing.T) {
	originalVersion, originalCommit := Version, Commit
	t.Cleanup(func() {
		Version, Commit = originalVersion, originalCommit
	})

	Version = " "
	Commit = " "

	got := Current()
	if got.Version != "dev" {
		t.Fatalf("Version = %q, want dev", got.Version)
	}
	if got.Commit != "unknown" {
		t.Fatalf("Commit = %q, want unknown", got.Commit)
	}
}
