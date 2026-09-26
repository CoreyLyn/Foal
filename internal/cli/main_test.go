package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/CoreyLyn/Foal/internal/testenv"
)

// TestMain isolates the CLI test binary from the host user's profile, AppData,
// system-drive roots, and tool-cache overrides so Clean resolution inside CLI
// and TUI tests can never reach real user files.
func TestMain(m *testing.M) {
	cleanup, err := testenv.Isolate("cli-test-env-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cli tests:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
