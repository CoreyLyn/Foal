package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestSandboxIsolatesHostProfile guards TestMain: every location production
// resolvers read must point inside the sandbox, so no test can resolve or
// delete real user files.
func TestSandboxIsolatesHostProfile(t *testing.T) {
	sandbox := filepath.Dir(os.Getenv("TEMP"))
	if !strings.Contains(strings.ToLower(sandbox), "clean-test-env-") {
		t.Fatalf("TEMP = %q is not inside the clean test sandbox", os.Getenv("TEMP"))
	}
	for _, name := range []string{"USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA", "SystemDrive", "TEMP", "TMP"} {
		value := os.Getenv(name)
		if !strings.HasPrefix(strings.ToLower(value), strings.ToLower(sandbox)) {
			t.Fatalf("%s = %q escapes the sandbox %q", name, value, sandbox)
		}
	}
	for _, name := range []string{"IDF_TOOLS_PATH", "UPM_CACHE_ROOT", "GOCACHE", "GROK_HOME", "PLAYWRIGHT_BROWSERS_PATH"} {
		if value, ok := os.LookupEnv(name); ok {
			t.Fatalf("%s = %q must be unset in tests", name, value)
		}
	}
	if home, err := os.UserHomeDir(); err != nil || !strings.HasPrefix(strings.ToLower(home), strings.ToLower(sandbox)) {
		t.Fatalf("os.UserHomeDir() = %q, %v escapes the sandbox", home, err)
	}
}

// TestAllTokenSeesNothingOutsideSandbox proves every category the `all` token
// selects — which includes every permanent-delete category — resolves paths
// only inside the sandbox under default production options. A resolver that
// bypasses the environment (for example a Windows Known Folder) fails here
// before an execute test can reach real user files.
func TestAllTokenSeesNothingOutsideSandbox(t *testing.T) {
	sandbox := strings.ToLower(filepath.Dir(os.Getenv("TEMP")))
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{"all"},
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	var paths []string
	for _, item := range result.Candidates {
		paths = append(paths, item.Path)
	}
	for _, item := range result.OptInCandidates {
		paths = append(paths, item.Path)
	}
	for _, item := range result.Opportunities {
		paths = append(paths, item.Path)
	}
	for _, item := range result.IncompleteOpportunityInspections {
		paths = append(paths, item.Path)
	}
	for _, item := range result.Skipped {
		paths = append(paths, item.Path)
	}
	for _, item := range result.Errors {
		paths = append(paths, item.Path)
	}
	for _, path := range paths {
		if path != "" && !strings.HasPrefix(strings.ToLower(path), sandbox) {
			t.Fatalf("`all` resolved host path %q outside the sandbox %q", path, sandbox)
		}
	}
}
