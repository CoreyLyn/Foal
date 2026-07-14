package clean

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// puppeteerBrowserProducts is the fail-closed allowlist of product directory
// names under a Puppeteer cache root. Unknown products never become candidates.
// Names match @puppeteer/browsers Browser enum values used as cache folders.
var puppeteerBrowserProducts = map[string]struct{}{
	"chrome":                {}, // Chrome for Testing
	"chrome-headless-shell": {},
	"firefox":               {},
}

// resolvePuppeteerCachePaths resolves the global Puppeteer browser cache root
// from a non-blank PUPPETEER_CACHE_DIR, otherwise the current user's home
// .cache\puppeteer root. Blank/whitespace overrides fall back to the default.
// Never reads project configuration, package.json, CWD state, or runs Puppeteer.
func resolvePuppeteerCachePaths(deps devCachePathDependencies) []string {
	if path, ok := deps.lookupEnv("PUPPETEER_CACHE_DIR"); ok {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			return []string{trimmed}
		}
	}
	if userProfile, ok := deps.lookupEnv("USERPROFILE"); ok && strings.TrimSpace(userProfile) != "" {
		return []string{deps.joinPath(userProfile, ".cache", "puppeteer")}
	}
	if home, err := deps.userHomeDir(); err == nil && home != "" {
		return []string{deps.joinPath(home, ".cache", "puppeteer")}
	}
	return nil
}

// discoverPuppeteerBrowserChildren enumerates independent Windows
// platform-version installation directories under one resolved Puppeteer root.
// Layout: <root>/<product>/<win32|win64>-<buildId>/. The root and product
// parents are never returned. Unknown products, metadata, non-Windows
// platforms, malformed names, files, and reparse points are excluded.
func discoverPuppeteerBrowserChildren(ctx context.Context, root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	if !isPuppeteerSafeDirectory(root) {
		return nil
	}

	productEntries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	sort.Slice(productEntries, func(i, j int) bool {
		return productEntries[i].Name() < productEntries[j].Name()
	})

	var children []string
	for _, productEntry := range productEntries {
		select {
		case <-ctx.Done():
			return children
		default:
		}

		productName := productEntry.Name()
		if _, ok := puppeteerBrowserProducts[productName]; !ok {
			continue
		}
		productPath := filepath.Join(root, productName)
		if !isPuppeteerSafeDirectory(productPath) {
			continue
		}

		versionEntries, err := os.ReadDir(productPath)
		if err != nil {
			continue
		}
		sort.Slice(versionEntries, func(i, j int) bool {
			return versionEntries[i].Name() < versionEntries[j].Name()
		})

		for _, versionEntry := range versionEntries {
			select {
			case <-ctx.Done():
				return children
			default:
			}

			versionName := versionEntry.Name()
			if !isPuppeteerWindowsPlatformVersionDir(versionName) {
				continue
			}
			installPath := filepath.Join(productPath, versionName)
			if !isPuppeteerSafeDirectory(installPath) {
				continue
			}
			children = append(children, installPath)
		}
	}
	return children
}

// isPuppeteerWindowsPlatformVersionDir reports whether name is a Puppeteer
// installation folder for a Windows platform: exactly one hyphen separating a
// known Windows platform token from a non-empty build id (same shape as
// @puppeteer/browsers Cache.parseFolderPath, restricted to win32/win64).
func isPuppeteerWindowsPlatformVersionDir(name string) bool {
	parts := strings.Split(name, "-")
	if len(parts) != 2 {
		return false
	}
	platform, buildID := parts[0], parts[1]
	if platform == "" || buildID == "" {
		return false
	}
	switch platform {
	case "win32", "win64":
		return true
	default:
		return false
	}
}

// isPuppeteerSafeDirectory reports whether path exists as a real directory
// (not a regular file, symlink, junction, or other reparse point).
func isPuppeteerSafeDirectory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.IsDir()
}
