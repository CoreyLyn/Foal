package clean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Playwright browser component directories under the global browsers root use
// "<prefix>-<numeric-revision>" names. Only these controlled prefixes may become
// structured Opt-in candidates; everything else under the root is excluded.
var playwrightBrowserComponentPrefixes = []string{
	"chromium_headless_shell",
	"chromium",
	"firefox",
	"webkit",
	"ffmpeg",
	"winldd", // Windows dependency helper (PrintDeps)
}

const playwrightInstallationCompleteMarker = "INSTALLATION_COMPLETE"

// discoverPlaywrightBrowserChildren enumerates direct children of a resolved
// Playwright browsers root and returns only complete, allowlisted revision
// directories. The root itself is never returned. Fail-closed exclusions:
// mcp-* profiles/state, .links, b, unknown prefixes/suffixes, incomplete
// installs, regular files, and unreadable entries. Symlink/reparse rejection
// and strict containment are re-applied by shared structured measurement.
func discoverPlaywrightBrowserChildren(ctx context.Context, root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var children []string
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return children
		default:
		}
		name := entry.Name()
		if !entry.IsDir() {
			continue
		}
		if !isPlaywrightBrowserRevisionDirName(name) {
			continue
		}
		child := filepath.Join(root, name)
		if !playwrightRevisionHasInstallationEvidence(child) {
			continue
		}
		children = append(children, child)
	}
	sort.Strings(children)
	return children
}

// isPlaywrightBrowserRevisionDirName reports whether name is an allowlisted
// component prefix plus a pure numeric revision (for example chromium-1161).
// Matching is case-insensitive for Windows path identity; unknown prefixes and
// non-numeric revisions fail closed.
func isPlaywrightBrowserRevisionDirName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	// Permanent exclusions: MCP persistent profiles/state and internal metadata.
	if strings.HasPrefix(lower, "mcp-") || lower == ".links" || lower == "b" {
		return false
	}
	for _, prefix := range playwrightBrowserComponentPrefixes {
		// Prefer longer prefixes first (chromium_headless_shell before chromium).
		if !strings.HasPrefix(lower, prefix+"-") {
			continue
		}
		revision := lower[len(prefix)+1:]
		if revision == "" || !isAllASCIIDigits(revision) {
			return false
		}
		return true
	}
	return false
}

func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// playwrightRevisionHasInstallationEvidence requires Playwright's
// INSTALLATION_COMPLETE marker as a regular non-reparse file. Matching names
// without evidence are incomplete and must not become candidates.
func playwrightRevisionHasInstallationEvidence(dir string) bool {
	marker := filepath.Join(dir, playwrightInstallationCompleteMarker)
	info, err := os.Lstat(marker)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if info.IsDir() {
		return false
	}
	return true
}
