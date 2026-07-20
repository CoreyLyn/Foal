package clean

import (
	"strings"
	"testing"
)

func TestCanonicalExecutableCategoriesBindResolvers(t *testing.T) {
	for _, entry := range canonicalCategoryEntries {
		executable := isExecutableCategoryEligibility(entry.definition.Eligibility)
		if executable && entry.resolver == nil {
			t.Errorf("executable category %q has no bound resolver", entry.definition.Identifier)
		}
		if !executable && entry.resolver != nil {
			t.Errorf("non-executable category %q has a bound resolver", entry.definition.Identifier)
		}
	}
}

func TestValidateCategoryResolverRegistryRejectsInvalidBindings(t *testing.T) {
	executable := categoryDefinition(
		"future-cache", "Future cache", ReportCategoryDeveloperTools,
		CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime,
		PlannedActionDeletePermanently,
	)
	nonExecutable := categoryDefinition(
		"future-notice", "Future notice", ReportCategorySystem,
		CategoryEligibilityPermissionBoundary, RunningApplicationPolicyNotApplicable, "",
	)

	tests := []struct {
		name  string
		entry categoryCatalogEntry
	}{
		{
			name:  "executable without resolver",
			entry: categoryCatalogEntry{definition: executable, resolverKind: categoryResolverDeveloperCache},
		},
		{
			name: "non-executable with resolver",
			entry: categoryCatalogEntry{
				definition: nonExecutable, resolverKind: categoryResolverNonExecutable,
				resolver: developerCacheResolver{},
			},
		},
		{
			name: "opt-in with default resolver",
			entry: categoryCatalogEntry{
				definition: executable, resolverKind: categoryResolverDefault,
				resolver: defaultCategoryResolver{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCategoryResolverRegistry([]categoryCatalogEntry{tt.entry}); err == nil {
				t.Fatal("expected invalid resolver binding")
			}
		})
	}
}

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
		PlannedActionMoveToRecycleBin,
	)

	t.Run("missing resolver", func(t *testing.T) {
		entries := []categoryCatalogEntry{{
			definition:            baseDef,
			resolverKind:          categoryResolverDeveloperCache,
			resolver:              developerCacheResolver{},
			runningApplications:   []string{ApplicationGo},
			reviewSuggestionTools: []string{"go"},
		}}
		if err := validateDeveloperCacheRegistry(entries, apps, allowlist); err == nil {
			t.Fatal("expected missing resolver error")
		} else if !strings.Contains(err.Error(), "missing a path or root-scope resolver") {
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

	want := map[string][]string{
		DevCacheCategoryNPM:                 {`C:\Users\test\AppData\Local\npm-cache`},
		DevCacheCategoryPNPM:                {`C:\Users\test\AppData\Local\pnpm\store`},
		DevCacheCategoryYarn:                {`C:\Users\test\AppData\Local\Yarn\Cache`},
		DevCacheCategoryGo:                  {`C:\Users\test\AppData\Local\go-build`},
		DevCacheCategoryGoModCache:          {`C:\Users\test\go\pkg\mod`},
		DevCacheCategoryPip:                 {`C:\Users\test\AppData\Local\pip\Cache`},
		DevCacheCategoryCargo:               {`C:\Users\test\.cargo\registry\cache`, `C:\Users\test\.cargo\registry\src`},
		DevCacheCategoryNuGet:               {`C:\Users\test\AppData\Local\NuGet\v3-cache`},
		DevCacheCategoryNuGetGlobalPackages: {`C:\Users\test\.nuget\packages`},
		DevCacheCategoryCorepack:            {`C:\Users\test\AppData\Local\node\corepack\v1`},
		DevCacheCategoryUV:                  {`C:\Users\test\AppData\Local\uv\cache`},
		DevCacheCategoryBun:                 {`C:\Users\test\.bun\install\cache`},
		DevCacheCategoryPlaywright:          {`C:\Users\test\AppData\Local\ms-playwright`},
		DevCacheCategoryPuppeteerBrowsers:   {`C:\Users\test\.cache\puppeteer`},
		DevCacheCategoryElectron:            {`C:\Users\test\AppData\Local\electron\Cache`},
	}
	for category, wantPaths := range want {
		paths := resolveDevCachePaths(category, deps)
		if len(paths) != len(wantPaths) {
			t.Fatalf("%s paths = %#v, want %#v", category, paths, wantPaths)
		}
		for i := range wantPaths {
			if paths[i] != wantPaths[i] {
				t.Fatalf("%s paths = %#v, want %#v", category, paths, wantPaths)
			}
		}
		entry, ok := canonicalCategoryEntry(category)
		if !ok || entry.resolverKind != categoryResolverDeveloperCache || entry.resolvePaths == nil {
			t.Fatalf("%s missing registered developer-cache rule", category)
		}
	}
	if paths := resolveDevCachePaths("unknown-cache", deps); len(paths) != 0 {
		t.Fatalf("unknown category paths = %#v, want nil", paths)
	}
}
