package clean_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/CoreyLyn/Foal/internal/testenv"
)

// TestMain isolates the whole test binary from the host: temp, user profile,
// AppData, system-drive roots, and tool-cache overrides all point at an empty
// sandbox, so a test that forgets an inject seam never sees real user files
// (for example a recent %TEMP%\foal-install-* entry or real developer-tool
// downloads). Test removers additionally only record, or remove paths inside
// fixtures the test created.
func TestMain(m *testing.M) {
	cleanup, err := testenv.Isolate("clean-test-env-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "clean tests:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
