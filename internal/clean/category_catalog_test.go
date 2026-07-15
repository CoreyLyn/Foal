package clean_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestCanonicalCleanupCategoryCatalogProvidesStableCompleteSummaries(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summaries := catalog.Summaries()
	definitions := catalog.Definitions()

	wantIdentifiers := []string{
		"foal_owned_temp_sandboxes",
		"user_temp",
		"crash_dumps",
		"windows_error_reporting",
		"explorer_thumbnail_cache",
		"inet_cache",
		"d3d_shader_cache",
		"nvidia_dx_cache",
		"browser_cache",
		"vscode_cache",
		"cursor_cache",
		"npm-cache",
		"go-cache",
		"pip-cache",
		"cargo-cache",
		"nuget-cache",
		"nuget-global-packages",
		"corepack-cache",
		"uv-cache",
		"bun-cache",
		"playwright-browsers",
		"puppeteer-browsers",
		"electron-cache",
		"administrator_only_caches",
	}
	gotIdentifiers := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		gotIdentifiers = append(gotIdentifiers, summary.Identifier)
		if strings.TrimSpace(summary.Label) == "" || strings.TrimSpace(string(summary.ReportCategory)) == "" ||
			strings.TrimSpace(string(summary.Eligibility)) == "" || strings.TrimSpace(string(summary.RunningApplicationPolicy)) == "" {
			t.Fatalf("incomplete category summary: %#v", summary)
		}
	}
	if !reflect.DeepEqual(gotIdentifiers, wantIdentifiers) {
		t.Fatalf("category order = %#v, want %#v", gotIdentifiers, wantIdentifiers)
	}
	if len(definitions) != len(summaries) {
		t.Fatalf("definitions/summaries = %d/%d", len(definitions), len(summaries))
	}
	for _, definition := range definitions {
		if definition.Aliases == nil {
			t.Fatalf("category %q does not expose accepted aliases", definition.Identifier)
		}
	}
	permissionBoundary, ok := catalog.Summary("administrator_only_caches")
	if !ok || permissionBoundary.ReportCategory != clean.ReportCategorySystem ||
		permissionBoundary.Eligibility != clean.CategoryEligibilityPermissionBoundary {
		t.Fatalf("permission boundary summary = %#v, %t", permissionBoundary, ok)
	}

	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"candidate_paths", "cache_path", "local_app_data_path", "roots"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("category summaries expose executable path data: %s", encoded)
		}
	}
}

func TestCategoryCatalogRejectsInvalidDefinitions(t *testing.T) {
	valid := clean.CleanupCategoryDefinition{
		Identifier:               "first",
		Label:                    "First",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityReviewOnly,
		Aliases:                  []string{"first-alias"},
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
	}

	tests := []struct {
		name        string
		definitions []clean.CleanupCategoryDefinition
	}{
		{name: "duplicate identifier", definitions: []clean.CleanupCategoryDefinition{valid, valid}},
		{name: "duplicate alias", definitions: []clean.CleanupCategoryDefinition{valid, {
			Identifier: "second", Label: "Second", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly, Aliases: []string{"first-alias"},
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "alias shadows identifier", definitions: []clean.CleanupCategoryDefinition{valid, {
			Identifier: "second", Label: "Second", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly, Aliases: []string{"first"},
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing label", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-label", ReportCategory: clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityReviewOnly,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing grouping", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-group", Label: "Missing group",
			Eligibility:              clean.CategoryEligibilityReviewOnly,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing eligibility", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-eligibility", Label: "Missing eligibility", ReportCategory: clean.ReportCategorySystem,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing running policy", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-policy", Label: "Missing policy", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly,
		}}},
		{name: "unsupported metadata", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "unsupported", Label: "Unsupported", ReportCategory: "Other",
			Eligibility: "sometimes", RunningApplicationPolicy: "best-effort",
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := clean.NewCleanupCategoryCatalog(tt.definitions); err == nil {
				t.Fatal("NewCleanupCategoryCatalog() error = nil")
			}
		})
	}
}

func TestFixedPathOpportunityUsesCanonicalCatalogVocabulary(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summary, ok := catalog.Summary(clean.OpportunityCategoryCrashDumps)
	if !ok {
		t.Fatal("crash_dumps missing from canonical catalog")
	}
	if summary.Label != "Crash dumps" || summary.ReportCategory != clean.ReportCategorySystem ||
		summary.Eligibility != clean.CategoryEligibilityOptIn ||
		summary.RunningApplicationPolicy != clean.RunningApplicationPolicyNotApplicable {
		t.Fatalf("crash_dumps summary = %#v", summary)
	}

	enabled, invalid, _ := clean.NormalizedOptInSet([]string{"CRASH_DUMPS"})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryCrashDumps] {
		t.Fatalf("NormalizedOptInSet() = %#v, %#v; want canonical crash_dumps", enabled, invalid)
	}
}

func TestDeveloperCacheRegistryConsistency(t *testing.T) {
	// Public seam: stable order, completeness, path-free projection, and
	// resolver dispatch through the registered rules. Does not inspect private
	// struct layout.
	wantDevCaches := []string{
		clean.DevCacheCategoryNPM,
		clean.DevCacheCategoryGo,
		clean.DevCacheCategoryPip,
		clean.DevCacheCategoryCargo,
		clean.DevCacheCategoryNuGet,
		clean.DevCacheCategoryNuGetGlobalPackages,
		clean.DevCacheCategoryCorepack,
		clean.DevCacheCategoryUV,
		clean.DevCacheCategoryBun,
		clean.DevCacheCategoryPlaywright,
		clean.DevCacheCategoryPuppeteerBrowsers,
		clean.DevCacheCategoryElectron,
	}
	wantDeveloperToolsOptIn := append(
		[]string{clean.OpportunityCategoryVSCodeCache, clean.OpportunityCategoryCursorCache},
		wantDevCaches...,
	)

	catalog := clean.CanonicalCleanupCategoryCatalog()
	summaries := catalog.Summaries()
	var gotDeveloperTools []string
	for _, summary := range summaries {
		if summary.ReportCategory == clean.ReportCategoryDeveloperTools &&
			summary.Eligibility == clean.CategoryEligibilityOptIn {
			gotDeveloperTools = append(gotDeveloperTools, summary.Identifier)
		}
	}
	if !reflect.DeepEqual(gotDeveloperTools, wantDeveloperToolsOptIn) {
		t.Fatalf("developer tools opt-in order = %#v, want %#v", gotDeveloperTools, wantDeveloperToolsOptIn)
	}

	vscodeSummary, ok := catalog.Summary(clean.OpportunityCategoryVSCodeCache)
	if !ok || vscodeSummary.Label != "VS Code cache" ||
		vscodeSummary.RunningApplicationPolicy != clean.RunningApplicationPolicyApplicationIdleBeforeAfter {
		t.Fatalf("vscode_cache summary = %#v, want application-idle opportunity", vscodeSummary)
	}
	cursorSummary, ok := catalog.Summary(clean.OpportunityCategoryCursorCache)
	if !ok || cursorSummary.Label != "Cursor cache" ||
		cursorSummary.RunningApplicationPolicy != clean.RunningApplicationPolicyApplicationIdleBeforeAfter {
		t.Fatalf("cursor_cache summary = %#v, want application-idle opportunity", cursorSummary)
	}

	policies := map[string]clean.RunningApplicationPolicy{
		clean.DevCacheCategoryNPM:                 clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryGo:                  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryPip:                 clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryCargo:               clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryNuGet:               clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryNuGetGlobalPackages: clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryCorepack:            clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryUV:                  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryBun:                 clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryPlaywright:          clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryPuppeteerBrowsers:   clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryElectron:            clean.RunningApplicationPolicySharedRuntime,
	}
	for id, wantPolicy := range policies {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("missing developer-cache category %q", id)
		}
		if summary.RunningApplicationPolicy != wantPolicy {
			t.Fatalf("%s policy = %q, want %q", id, summary.RunningApplicationPolicy, wantPolicy)
		}
		if summary.Eligibility != clean.CategoryEligibilityOptIn {
			t.Fatalf("%s eligibility = %q, want opt-in", id, summary.Eligibility)
		}
	}

	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"NPM_CONFIG_CACHE", "GOCACHE", "PIP_CACHE_DIR", "CARGO_HOME",
		"NUGET_HTTP_CACHE_PATH", "NUGET_PACKAGES", "COREPACK_HOME", "UV_CACHE_DIR",
		"BUN_INSTALL_CACHE_DIR", "PLAYWRIGHT_BROWSERS_PATH", "PUPPETEER_CACHE_DIR",
		"electron_config_cache",
		"go.exe", "cargo.exe", "dotnet.exe", "nuget.exe", "node.exe", "python.exe",
		"uv.exe", "uvx.exe", "bun.exe", "bunx.exe", "Code.exe", "Cursor.exe",
		"resolvePaths", "lookupEnv", "LOCALAPPDATA", "APPDATA", "ms-playwright",
		"CachedData", "CachedExtensionVSIXs", "INSTALLATION_COMPLETE",
		"chromium_headless_shell", "chrome-headless-shell", "discoverChildren",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("path-free catalog projection exposes %q: %s", forbidden, encoded)
		}
	}

	// Unknown categories never resolve through the registry-backed default.
	if paths := clean.ResolveDevCachePaths("not-a-real-cache"); len(paths) != 0 {
		t.Fatalf("unknown category resolved paths: %#v", paths)
	}

	// dev-caches / all selection order stays complete through NormalizedOptInSet.
	enabled, invalid, valid := clean.NormalizedOptInSet([]string{"dev-caches"})
	if len(invalid) != 0 {
		t.Fatalf("dev-caches invalid = %#v", invalid)
	}
	for _, id := range wantDeveloperToolsOptIn {
		if !enabled[id] {
			t.Fatalf("dev-caches missing %q", id)
		}
	}
	foundDevCachesGroup := false
	for _, name := range valid {
		if name == clean.DevCacheCategoryAll {
			foundDevCachesGroup = true
		}
	}
	if !foundDevCachesGroup {
		t.Fatal("valid names missing dev-caches group token")
	}
}
