package clean_test

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the process temp directory at an isolated, empty root for the
// whole test binary. The production default rule scans os.TempDir(), so real
// host entries (for example a recent %TEMP%\foal-install-* left by the
// installer) must never leak into tests that rely on the default rule catalog.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "clean-test-temp-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "clean tests: create isolated temp root:", err)
		os.Exit(1)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		if err := os.Setenv(name, root); err != nil {
			fmt.Fprintln(os.Stderr, "clean tests: set", name+":", err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
