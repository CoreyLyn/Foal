// Package testenv isolates a test binary from the host user's real profile,
// caches, and tool data.
//
// Clean resolves production default locations (user profile, AppData, tool
// caches, %SystemDrive%\Espressif, ...) from the environment. A test that
// forgets an inject seam must only ever see empty sandbox directories, never
// real user files. Locations Windows resolves without the environment, such as
// Known Folders, must be confined by their resolvers (Clean's LocalLow resolver
// requires the folder to lie inside the current, here sandboxed, profile).
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
)

// redirected are environment variables pointed at sandbox directories.
var redirected = map[string][]string{
	"temp":        {"TEMP", "TMP", "TMPDIR"},
	"profile":     {"USERPROFILE", "HOME"},
	"local":       {"LOCALAPPDATA"},
	"roaming":     {"APPDATA"},
	"systemdrive": {"SystemDrive"},
}

// unset are tool-location overrides that would otherwise point production
// resolvers outside the sandbox.
var unset = []string{
	"IDF_TOOLS_PATH", "UPM_CACHE_ROOT",
	"GOCACHE", "GOMODCACHE", "GOPATH",
	"CARGO_HOME",
	"NUGET_PACKAGES", "NUGET_HTTP_CACHE_PATH", "NUGET_SCRATCH",
	"npm_config_cache", "NPM_CONFIG_CACHE",
	"YARN_CACHE_FOLDER", "PNPM_HOME",
	"UV_CACHE_DIR", "BUN_INSTALL", "BUN_INSTALL_CACHE_DIR",
	"COREPACK_HOME", "PIP_CACHE_DIR",
	"PLAYWRIGHT_BROWSERS_PATH", "PUPPETEER_CACHE_DIR", "electron_config_cache",
	"GROK_HOME", "XDG_CACHE_HOME",
}

// Isolate creates an empty sandbox, redirects the process environment into it,
// and returns a cleanup that removes the sandbox. Call it from TestMain before
// m.Run.
func Isolate(prefix string) (func(), error) {
	root, err := os.MkdirTemp("", prefix)
	if err != nil {
		return nil, fmt.Errorf("testenv: create sandbox: %w", err)
	}
	dirs := map[string]string{
		"temp":        filepath.Join(root, "temp"),
		"profile":     filepath.Join(root, "profile"),
		"local":       filepath.Join(root, "profile", "AppData", "Local"),
		"roaming":     filepath.Join(root, "profile", "AppData", "Roaming"),
		"systemdrive": filepath.Join(root, "systemdrive"),
	}
	for key, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("testenv: create %s: %w", key, err)
		}
		for _, name := range redirected[key] {
			if err := os.Setenv(name, dir); err != nil {
				_ = os.RemoveAll(root)
				return nil, fmt.Errorf("testenv: set %s: %w", name, err)
			}
		}
	}
	for _, name := range unset {
		if err := os.Unsetenv(name); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("testenv: unset %s: %w", name, err)
		}
	}
	return func() { _ = os.RemoveAll(root) }, nil
}
