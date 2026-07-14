package clean

import (
	"strings"
	"testing"
)

func TestValidateDeveloperCacheRegistryRejectsIncompleteEntries(t *testing.T) {
	apps := []supportedApplicationDefinition{
		{id: ApplicationGo, displayName: "Go", executables: []string{"go.exe"}},
	}
	allowlist := map[string]reviewSuggestionTool{
		"go": {},
	}
	baseDef := categoryDefinition(
		DevCacheCategoryGo,
		"Go build cache",
		ReportCategoryDeveloperTools,
		CategoryEligibilityOptIn,
		RunningApplicationPolicyDistinctiveProcessIdle,
	)

	t.Run("missing resolver", func(t *testing.T) {
		entries := []categoryCatalogEntry{{
			definition:          baseDef,
			developerCache:      true,
			runningApplications: []string{ApplicationGo},
			reviewSuggestionTools: []string{"go"},
		}}
		if err := validateDeveloperCacheRegistry(entries, apps, allowlist); err == nil {
			t.Fatal("expected missing resolver error")
		} else if !strings.Contains(err.Error(), "missing a path resolver") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unknown review suggestion tool", func(t *testing.T) {
		entries := []categoryCatalogEntry{developerCacheEntry(baseDef, resolveGoCachePaths, []string{"not-a-tool"}, ApplicationGo)}
		if err := validateDeveloperCacheRegistry(entries, apps, allowlist); err == nil {
			t.Fatal("expected unknown Review suggestion error")
		} else if !strings.Contains(err.Error(), "unknown Review suggestion tool") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("distinctive-process without applications", func(t *testing.T) {
		entries := []categoryCatalogEntry{developerCacheEntry(baseDef, resolveGoCachePaths, []string{"go"})}
		if err := validateDeveloperCacheRegistry(entries, apps, allowlist); err == nil {
			t.Fatal("expected distinctive-process without apps error")
		} else if !strings.Contains(err.Error(), "without applications") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unknown application reference", func(t *testing.T) {
		entries := []categoryCatalogEntry{developerCacheEntry(baseDef, resolveGoCachePaths, []string{"go"}, "not-an-app")}
		if err := validateDeveloperCacheRegistry(entries, apps, allowlist); err == nil {
			t.Fatal("expected unknown application error")
		} else if !strings.Contains(err.Error(), "unknown application") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("ambiguous executable ownership", func(t *testing.T) {
		ambiguousApps := []supportedApplicationDefinition{
			{id: "tool-a", displayName: "A", executables: []string{"shared.exe"}},
			{id: "tool-b", displayName: "B", executables: []string{"shared.exe"}},
		}
		if err := validateDeveloperCacheRegistry(nil, ambiguousApps, allowlist); err == nil {
			t.Fatal("expected ambiguous executable error")
		} else if !strings.Contains(err.Error(), "ambiguous executable") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("duplicate application id", func(t *testing.T) {
		dupApps := []supportedApplicationDefinition{
			{id: ApplicationGo, displayName: "Go", executables: []string{"go.exe"}},
			{id: ApplicationGo, displayName: "Go again", executables: []string{"go2.exe"}},
		}
		if err := validateDeveloperCacheRegistry(nil, dupApps, allowlist); err == nil {
			t.Fatal("expected duplicate application id error")
		} else if !strings.Contains(err.Error(), "duplicate developer application id") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("canonical registry is valid", func(t *testing.T) {
		if err := validateDeveloperCacheRegistry(canonicalCategoryEntries, developerApplicationDefinitions, reviewSuggestionAllowlist); err != nil {
			t.Fatalf("canonical registry invalid: %v", err)
		}
	})
}

func TestCanonicalDeveloperCacheRegistryBindsResolvers(t *testing.T) {
	// Every registered developer-cache category dispatches through its bound
	// resolver rather than a free-standing switch.
	deps := devCachePathDependencies{
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case "LOCALAPPDATA":
				return `C:\Users\test\AppData\Local`, true
			case "USERPROFILE":
				return `C:\Users\test`, true
			default:
				return "", false
			}
		},
		joinPath: func(parts ...string) string {
			return strings.Join(parts, `\`)
		},
		goos: "windows",
	}

	want := map[string]string{
		DevCacheCategoryNPM:                 `C:\Users\test\AppData\Local\npm-cache`,
		DevCacheCategoryGo:                  `C:\Users\test\AppData\Local\go-build`,
		DevCacheCategoryPip:                 `C:\Users\test\AppData\Local\pip\Cache`,
		DevCacheCategoryCargo:               `C:\Users\test\.cargo\registry\cache`,
		DevCacheCategoryNuGet:               `C:\Users\test\AppData\Local\NuGet\v3-cache`,
		DevCacheCategoryNuGetGlobalPackages: `C:\Users\test\.nuget\packages`,
		DevCacheCategoryCorepack:            `C:\Users\test\AppData\Local\node\corepack\v1`,
	}
	for category, wantPath := range want {
		paths := resolveDevCachePaths(category, deps)
		if len(paths) != 1 || paths[0] != wantPath {
			t.Fatalf("%s paths = %#v, want [%q]", category, paths, wantPath)
		}
		entry, ok := canonicalCategoryEntry(category)
		if !ok || !entry.developerCache || entry.resolvePaths == nil {
			t.Fatalf("%s missing registered developer-cache rule", category)
		}
	}
	if paths := resolveDevCachePaths("unknown-cache", deps); len(paths) != 0 {
		t.Fatalf("unknown category paths = %#v, want nil", paths)
	}
}
