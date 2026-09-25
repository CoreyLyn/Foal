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
		"nvidia_gl_cache",
		"amd_gpu_shader_caches",
		"intel_gpu_shader_cache",
		"nvidia_installer_cache",
		"lghub-cache",
		"thunder-update-download",
		"windows-temp",
		"windows-update-download-cache",
		"winsxs_component_store",
		"browser_cache",
		"vscode_cache",
		"cursor_cache",
		"vscode_insiders_cache",
		"vscodium_cache",
		"windsurf_cache",
		"trae_cache",
		"npm-cache",
		"pnpm-cache",
		"yarn-cache",
		"go-cache",
		"go-modcache",
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
		"jetbrains-ide-caches",
		"visual-studio-caches",
		"grok-build-update-residue",
		"obsidian_cache",
		"vrchat_cache",
		"electron-updater-residue",
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
	validExecutable := clean.CleanupCategoryDefinition{
		Identifier:               "exec",
		Label:                    "Executable",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
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
		{name: "executable missing planned action", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-action", Label: "Missing action", ReportCategory: clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "executable unknown planned action", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "unknown-action", Label: "Unknown action", ReportCategory: clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityDefault,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            "shred",
		}}},
		{name: "non-executable with planned action", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "boundary-with-action", Label: "Boundary", ReportCategory: clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityPermissionBoundary,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionMoveToRecycleBin,
		}}},
		// Ensure a valid executable definition still constructs (control case uses
		// a separate positive test below; this only lists rejection cases).
		{name: "duplicate with executable", definitions: []clean.CleanupCategoryDefinition{validExecutable, validExecutable}},
		{name: "unknown selection group", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "unknown-group", Label: "Unknown group", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityOptIn, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction: clean.PlannedActionMoveToRecycleBin, SelectionGroup: "other-caches",
		}}},
		{name: "exact-only joins a group", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "exact-grouped", Label: "Exact grouped", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityOptIn, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction: clean.PlannedActionMoveToRecycleBin, SelectionPolicy: clean.CategorySelectionPolicyExactOnly,
			SelectionGroup: clean.CategorySelectionGroupDevCaches,
		}}},
		{name: "non-executable joins a group", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "boundary-grouped", Label: "Boundary grouped", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityPermissionBoundary, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			SelectionGroup: clean.CategorySelectionGroupAppCaches,
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

// TestDeletionRuleMatrixDerivesFromCatalog checks action, initial selection, and
// the eager queue against the catalog. Counts are derived; adding a category
// does not update a parallel length lock.
func TestDeletionRuleMatrixDerivesFromCatalog(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	var executable []string
	for _, definition := range catalog.Definitions() {
		summary, ok := catalog.Summary(definition.Identifier)
		if !ok {
			t.Fatalf("summary missing for %q", definition.Identifier)
		}
		if summary.PlannedAction != definition.PlannedAction || summary.SelectionGroup != definition.SelectionGroup ||
			summary.SelectionPolicy != definition.SelectionPolicy {
			t.Fatalf("summary drifted from definition for %q: summary=%#v definition=%#v", definition.Identifier, summary, definition)
		}
		switch definition.Eligibility {
		case clean.CategoryEligibilityDefault, clean.CategoryEligibilityOptIn:
			executable = append(executable, definition.Identifier)
			switch definition.PlannedAction {
			case clean.PlannedActionDeletePermanently, clean.PlannedActionMoveToRecycleBin, clean.PlannedActionInvokeWindowsServicing:
			default:
				t.Fatalf("executable %q has unsupported planned_action %q", definition.Identifier, definition.PlannedAction)
			}
			wantSelected := definition.SelectionPolicy != clean.CategorySelectionPolicyExactOnly &&
				(definition.Eligibility == clean.CategoryEligibilityDefault || definition.PlannedAction == clean.PlannedActionDeletePermanently)
			if clean.InitiallySelectedCategory(summary) != wantSelected {
				t.Fatalf("%s initially selected = %v, want %v", definition.Identifier, clean.InitiallySelectedCategory(summary), wantSelected)
			}
		case clean.CategoryEligibilityPermissionBoundary, clean.CategoryEligibilityReviewOnly:
			if definition.PlannedAction != "" || definition.SelectionGroup != "" {
				t.Fatalf("non-executable %q must be actionless and ungrouped, got action %q group %q",
					definition.Identifier, definition.PlannedAction, definition.SelectionGroup)
			}
			if clean.InitiallySelectedCategory(summary) {
				t.Fatalf("%s must not start selected", definition.Identifier)
			}
		default:
			t.Fatalf("unexpected eligibility %q on %q", definition.Eligibility, definition.Identifier)
		}
	}

	boundary, ok := catalog.Summary("administrator_only_caches")
	if !ok || boundary.Eligibility != clean.CategoryEligibilityPermissionBoundary || boundary.PlannedAction != "" {
		t.Fatalf("administrator_only_caches = %#v, want actionless permission boundary", boundary)
	}

	queue := clean.EagerPreviewQueue()
	if len(queue) != len(executable) {
		t.Fatalf("EagerPreviewQueue length = %d, want %d executable categories", len(queue), len(executable))
	}
	for i, summary := range queue {
		if summary.Identifier != executable[i] {
			t.Fatalf("queue[%d] = %q, want %q", i, summary.Identifier, executable[i])
		}
		if summary.Identifier == "administrator_only_caches" {
			t.Fatal("permission boundary must not enter the eager queue")
		}
	}
}

func TestCanonicalExecutableCategoriesDeclareExplicitPlannedActions(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	for _, definition := range catalog.Definitions() {
		summary, ok := catalog.Summary(definition.Identifier)
		if !ok {
			t.Fatalf("summary missing for %q", definition.Identifier)
		}
		if summary.PlannedAction != definition.PlannedAction {
			t.Fatalf("summary planned_action for %q = %q, want %q", definition.Identifier, summary.PlannedAction, definition.PlannedAction)
		}
		switch definition.Eligibility {
		case clean.CategoryEligibilityDefault, clean.CategoryEligibilityOptIn:
			switch definition.PlannedAction {
			case clean.PlannedActionMoveToRecycleBin, clean.PlannedActionDeletePermanently, clean.PlannedActionInvokeWindowsServicing:
			default:
				t.Fatalf("executable category %q has unsupported planned_action %q", definition.Identifier, definition.PlannedAction)
			}
			if definition.PlannedAction == clean.PlannedActionInvokeWindowsServicing &&
				definition.Identifier != clean.CategoryWinSxSComponentStore {
				t.Fatalf("unexpected servicing category %q", definition.Identifier)
			}
		case clean.CategoryEligibilityPermissionBoundary, clean.CategoryEligibilityReviewOnly:
			if definition.PlannedAction != "" {
				t.Fatalf("non-executable category %q must be actionless, got %q", definition.Identifier, definition.PlannedAction)
			}
		default:
			t.Fatalf("unexpected eligibility %q on %q", definition.Eligibility, definition.Identifier)
		}
	}

	// No parallel permanent-delete eligibility boolean on public catalog types.
	summaryType := reflect.TypeOf(clean.CleanupCategorySummary{})
	for i := 0; i < summaryType.NumField(); i++ {
		name := summaryType.Field(i).Name
		if strings.Contains(strings.ToLower(name), "permanent") && name != "PlannedAction" {
			t.Fatalf("summary exposes permanent-eligibility field %q; planned_action must be sole source", name)
		}
		if strings.EqualFold(name, "CanPermanentDelete") || strings.EqualFold(name, "PermanentDeleteEligible") {
			t.Fatalf("summary exposes parallel eligibility boolean %q", name)
		}
	}
}

func TestSelectionGroupExpansionMatchesCatalogField(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	groups := []struct {
		token string
		group clean.CategorySelectionGroup
	}{
		{token: clean.DevCacheCategoryAll, group: clean.CategorySelectionGroupDevCaches},
		{token: clean.ApplicationCacheCategoryGroup, group: clean.CategorySelectionGroupAppCaches},
		{token: clean.CLIAgentCategoryGroup, group: clean.CategorySelectionGroupCLIAgents},
	}
	for _, tc := range groups {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{tc.token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", tc.token, invalid)
		}
		var want []string
		for _, summary := range catalog.Summaries() {
			if summary.SelectionGroup != tc.group {
				continue
			}
			if summary.Eligibility != clean.CategoryEligibilityOptIn || summary.SelectionPolicy == clean.CategorySelectionPolicyExactOnly {
				t.Fatalf("%s carries selection group %s but cannot expand", summary.Identifier, tc.group)
			}
			want = append(want, summary.Identifier)
			if !enabled[summary.Identifier] {
				t.Fatalf("%s missing %q", tc.token, summary.Identifier)
			}
		}
		if len(enabled) != len(want) {
			t.Fatalf("%s enabled %#v, want %#v", tc.token, enabled, want)
		}
	}

	pins := map[string]clean.CategorySelectionGroup{
		clean.OpportunityCategoryVSCodeCache:   clean.CategorySelectionGroupDevCaches,
		clean.OpportunityCategoryObsidianCache: clean.CategorySelectionGroupAppCaches,
		clean.CategoryElectronUpdaterResidue:   clean.CategorySelectionGroupAppCaches,
		clean.CategoryGrokBuildUpdateResidue:   clean.CategorySelectionGroupCLIAgents,
		clean.OpportunityCategoryUserTemp:      "",
		clean.CategoryNVIDIAInstallerCache:     "",
	}
	for id, want := range pins {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if summary.SelectionGroup != want {
			t.Fatalf("%s selection_group = %q, want %q", id, summary.SelectionGroup, want)
		}
	}
}

func TestCategoryCatalogAcceptsSupportedPlannedActions(t *testing.T) {
	for _, action := range []clean.PlannedAction{
		clean.PlannedActionMoveToRecycleBin,
		clean.PlannedActionDeletePermanently,
	} {
		catalog, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{{
			Identifier:               "sample",
			Label:                    "Sample",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            action,
		}})
		if err != nil {
			t.Fatalf("action %q: %v", action, err)
		}
		summary, ok := catalog.Summary("sample")
		if !ok || summary.PlannedAction != action {
			t.Fatalf("summary = %#v, want planned_action %q", summary, action)
		}
	}
}

func TestInitiallySelectedCategoryDerivedFromEligibilityAndAction(t *testing.T) {
	cases := []struct {
		name    string
		summary clean.CleanupCategorySummary
		want    bool
	}{
		{
			name: "default recycle bin",
			summary: clean.CleanupCategorySummary{
				Identifier:    "foal_owned_temp_sandboxes",
				Eligibility:   clean.CategoryEligibilityDefault,
				PlannedAction: clean.PlannedActionMoveToRecycleBin,
			},
			want: true,
		},
		{
			name: "opt-in permanent",
			summary: clean.CleanupCategorySummary{
				Identifier:    "go-cache",
				Eligibility:   clean.CategoryEligibilityOptIn,
				PlannedAction: clean.PlannedActionDeletePermanently,
			},
			want: true,
		},
		{
			name: "opt-in recycle bin",
			summary: clean.CleanupCategorySummary{
				Identifier:    "user_temp",
				Eligibility:   clean.CategoryEligibilityOptIn,
				PlannedAction: clean.PlannedActionMoveToRecycleBin,
			},
			want: false,
		},
		{
			name: "permission boundary",
			summary: clean.CleanupCategorySummary{
				Identifier:  "administrator_only_caches",
				Eligibility: clean.CategoryEligibilityPermissionBoundary,
			},
			want: false,
		},
		{
			name: "review only",
			summary: clean.CleanupCategorySummary{
				Identifier:  "review_only_tool",
				Eligibility: clean.CategoryEligibilityReviewOnly,
			},
			want: false,
		},
		{
			name: "default permanent still selected",
			summary: clean.CleanupCategorySummary{
				Identifier:    "future_default",
				Eligibility:   clean.CategoryEligibilityDefault,
				PlannedAction: clean.PlannedActionDeletePermanently,
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clean.InitiallySelectedCategory(tc.summary); got != tc.want {
				t.Fatalf("InitiallySelectedCategory(%#v) = %v, want %v", tc.summary, got, tc.want)
			}
		})
	}
}

func TestInitiallySelectedCategoryUsesInjectedCatalogSummariesWithoutHardCodedList(t *testing.T) {
	catalog, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{
		{
			Identifier:               "default_recycle",
			Label:                    "Default recycle",
			ReportCategory:           clean.ReportCategoryUserEssentials,
			Eligibility:              clean.CategoryEligibilityDefault,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:               "permanent_cache",
			Label:                    "Permanent cache",
			ReportCategory:           clean.ReportCategoryDeveloperTools,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionDeletePermanently,
		},
		{
			Identifier:               "recycle_opt_in",
			Label:                    "Recycle opt-in",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:               "admin_boundary",
			Label:                    "Admin boundary",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityPermissionBoundary,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSelected := map[string]bool{
		"default_recycle": true,
		"permanent_cache": true,
		"recycle_opt_in":  false,
		"admin_boundary":  false,
	}
	for _, summary := range catalog.Summaries() {
		got := clean.InitiallySelectedCategory(summary)
		if got != wantSelected[summary.Identifier] {
			t.Fatalf("%s selected=%v, want %v", summary.Identifier, got, wantSelected[summary.Identifier])
		}
	}
	// Production catalog: defaults + every permanent-action category start selected.
	for _, summary := range clean.EagerPreviewQueue() {
		want := summary.Eligibility == clean.CategoryEligibilityDefault ||
			summary.PlannedAction == clean.PlannedActionDeletePermanently
		if clean.InitiallySelectedCategory(summary) != want {
			t.Fatalf("production %q selected=%v, want %v",
				summary.Identifier, clean.InitiallySelectedCategory(summary), want)
		}
	}
}

func TestPlannedActionLabel(t *testing.T) {
	if clean.PlannedActionLabel(clean.PlannedActionMoveToRecycleBin) != "Recycle Bin" {
		t.Fatal("recycle label")
	}
	if clean.PlannedActionLabel(clean.PlannedActionDeletePermanently) != "Permanent deletion" {
		t.Fatal("permanent label")
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
		clean.DevCacheCategoryPNPM,
		clean.DevCacheCategoryYarn,
		clean.DevCacheCategoryGo,
		clean.DevCacheCategoryGoModCache,
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
		clean.DevCacheCategoryJetBrainsIDECaches,
		clean.DevCacheCategoryVisualStudioCaches,
	}
	// Catalog Developer tools opt-in rows (includes CLI-agent residue after caches).
	wantDeveloperToolsOptIn := append(
		[]string{
			clean.OpportunityCategoryVSCodeCache,
			clean.OpportunityCategoryCursorCache,
			clean.OpportunityCategoryVSCodeInsidersCache,
			clean.OpportunityCategoryVSCodiumCache,
			clean.OpportunityCategoryWindsurfCache,
			clean.OpportunityCategoryTraeCache,
		},
		wantDevCaches...,
	)
	wantDeveloperToolsOptIn = append(wantDeveloperToolsOptIn, clean.CategoryGrokBuildUpdateResidue)

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
		clean.DevCacheCategoryPNPM:                clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryYarn:                clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryGo:                  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryGoModCache:          clean.RunningApplicationPolicyDistinctiveProcessIdle,
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
		clean.DevCacheCategoryJetBrainsIDECaches:  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryVisualStudioCaches:  clean.RunningApplicationPolicyDistinctiveProcessIdle,
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
		"uv.exe", "uvx.exe", "bun.exe", "bunx.exe", "Code.exe", "Cursor.exe", "Trae.exe",
		"idea64.exe", "pycharm64.exe", "webstorm64.exe", "phpstorm64.exe",
		"rubymine64.exe", "clion64.exe", "datagrip64.exe", "dataspell64.exe",
		"goland64.exe", "rustrover64.exe", "aqua64.exe", "mps64.exe", "writerside64.exe",
		"rider64.exe", "devenv.exe",
		"IntelliJIdea", "IdeaIC", "PyCharmCE", "WebStorm", "PhpStorm", "RubyMine",
		"ComponentModelCache",
		"CLion", "DataGrip", "DataSpell", "GoLand", "RustRover", "Writerside",
		"resolvePaths", "lookupEnv", "LOCALAPPDATA", "APPDATA", "ms-playwright",
		"CachedData", "CachedExtensionVSIXs", "INSTALLATION_COMPLETE",
		"chromium_headless_shell", "chrome-headless-shell", "discoverChildren",
		"resolveRootScopes", "LocalHistory", "resharper-host",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("path-free catalog projection exposes %q: %s", forbidden, encoded)
		}
	}

	// Unknown categories never resolve through the registry-backed default.
	if paths := clean.ResolveDevCachePaths("not-a-real-cache"); len(paths) != 0 {
		t.Fatalf("unknown category resolved paths: %#v", paths)
	}

	// dev-caches expands developer-cache + application-cache rows only.
	// CLI-agent residue is Developer tools but excluded from the cache group.
	wantDevCachesGroup := append(
		[]string{clean.OpportunityCategoryVSCodeCache, clean.OpportunityCategoryCursorCache, clean.OpportunityCategoryTraeCache},
		wantDevCaches...,
	)
	enabled, invalid, valid := clean.NormalizedOptInSet([]string{"dev-caches"})
	if len(invalid) != 0 {
		t.Fatalf("dev-caches invalid = %#v", invalid)
	}
	for _, id := range wantDevCachesGroup {
		if !enabled[id] {
			t.Fatalf("dev-caches missing %q", id)
		}
	}
	if enabled[clean.CategoryGrokBuildUpdateResidue] {
		t.Fatal("dev-caches must not enable grok-build-update-residue")
	}
	if enabled[clean.OpportunityCategoryObsidianCache] {
		t.Fatal("dev-caches must not enable obsidian_cache (Applications report category; use app-caches)")
	}
	foundDevCachesGroup := false
	foundCLIAgentsGroup := false
	foundAppCachesGroup := false
	for _, name := range valid {
		if name == clean.DevCacheCategoryAll {
			foundDevCachesGroup = true
		}
		if name == clean.CLIAgentCategoryGroup {
			foundCLIAgentsGroup = true
		}
		if name == clean.ApplicationCacheCategoryGroup {
			foundAppCachesGroup = true
		}
	}
	if !foundDevCachesGroup {
		t.Fatal("valid names missing dev-caches group token")
	}
	if !foundCLIAgentsGroup {
		t.Fatal("valid names missing cli-agents group token")
	}
	if !foundAppCachesGroup {
		t.Fatal("valid names missing app-caches group token")
	}

	// app-caches expands Applications report-category application caches only.
	appEnabled, appInvalid, _ := clean.NormalizedOptInSet([]string{clean.ApplicationCacheCategoryGroup})
	if len(appInvalid) != 0 {
		t.Fatalf("app-caches invalid = %#v", appInvalid)
	}
	if len(appEnabled) != 3 || !appEnabled[clean.OpportunityCategoryObsidianCache] || !appEnabled[clean.OpportunityCategoryVRChatCache] || !appEnabled[clean.CategoryElectronUpdaterResidue] {
		t.Fatalf("app-caches enabled = %#v, want obsidian_cache, vrchat_cache, and electron-updater-residue", appEnabled)
	}
	for _, id := range wantDevCachesGroup {
		if appEnabled[id] {
			t.Fatalf("app-caches must not enable developer-tools cache category %q", id)
		}
	}

	// cli-agents expands product-scoped CLI-agent residue only (catalog order).
	cliEnabled, cliInvalid, _ := clean.NormalizedOptInSet([]string{clean.CLIAgentCategoryGroup})
	if len(cliInvalid) != 0 {
		t.Fatalf("cli-agents invalid = %#v", cliInvalid)
	}
	if len(cliEnabled) != 1 || !cliEnabled[clean.CategoryGrokBuildUpdateResidue] {
		t.Fatalf("cli-agents enabled = %#v, want only grok-build-update-residue", cliEnabled)
	}
	for _, id := range wantDevCachesGroup {
		if cliEnabled[id] {
			t.Fatalf("cli-agents must not enable cache category %q", id)
		}
	}
}
