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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := clean.NewCleanupCategoryCatalog(tt.definitions); err == nil {
				t.Fatal("NewCleanupCategoryCatalog() error = nil")
			}
		})
	}
}

// lockedPermanentCategoryIDs is the complete production permanent matrix (36).
// Order matches catalog registration among permanent-action categories.
func lockedPermanentCategoryIDs() []string {
	return []string{
		clean.OpportunityCategoryUserTemp,
		clean.OpportunityCategoryCrashDumps,
		clean.OpportunityCategoryExplorerThumbnailCache,
		clean.OpportunityCategoryINetCache,
		clean.OpportunityCategoryD3DShaderCache,
		clean.OpportunityCategoryNVIDIADXCache,
		clean.OpportunityCategoryNVIDIAGLCache,
		clean.OpportunityCategoryAMDGPUShaderCaches,
		clean.OpportunityCategoryIntelGPUShaderCache,
		clean.OpportunityCategoryBrowserCache,
		clean.OpportunityCategoryVSCodeCache,
		clean.OpportunityCategoryCursorCache,
		clean.OpportunityCategoryVSCodeInsidersCache,
		clean.OpportunityCategoryVSCodiumCache,
		clean.OpportunityCategoryWindsurfCache,
		clean.OpportunityCategoryTraeCache,
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
		clean.CategoryGrokBuildUpdateResidue,
		clean.OpportunityCategoryObsidianCache,
		clean.OpportunityCategoryVRChatCache,
	}
}

// lockedRecycleBinCategoryIDs is the complete production Recycle Bin matrix (5).
func lockedRecycleBinCategoryIDs() []string {
	return []string{
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.OpportunityCategoryWindowsErrorReporting,
		clean.CategoryNVIDIAInstallerCache,
		clean.CategoryLGHUBCache,
		clean.CategoryThunderUpdateDownload,
	}
}

func productionPermanentCategoryIDs() map[string]bool {
	want := make(map[string]bool, len(lockedPermanentCategoryIDs()))
	for _, id := range lockedPermanentCategoryIDs() {
		want[id] = true
	}
	return want
}

// TestCompleteDeletionRuleMatrixLocked is the end-state catalog contract for ADR 0018:
// exactly 36 delete_permanently, 5 move_to_recycle_bin, and one actionless permission boundary.
func TestCompleteDeletionRuleMatrixLocked(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	wantPermanent := lockedPermanentCategoryIDs()
	wantRecycleBin := lockedRecycleBinCategoryIDs()
	if len(wantPermanent) != 36 {
		t.Fatalf("locked permanent matrix length = %d, want 36", len(wantPermanent))
	}
	if len(wantRecycleBin) != 5 {
		t.Fatalf("locked Recycle Bin matrix length = %d, want 5", len(wantRecycleBin))
	}

	var permanent, recycleBin, servicing, executable []string
	for _, definition := range catalog.Definitions() {
		switch definition.Eligibility {
		case clean.CategoryEligibilityDefault, clean.CategoryEligibilityOptIn:
			executable = append(executable, definition.Identifier)
			switch definition.PlannedAction {
			case clean.PlannedActionDeletePermanently:
				permanent = append(permanent, definition.Identifier)
			case clean.PlannedActionMoveToRecycleBin:
				recycleBin = append(recycleBin, definition.Identifier)
			case clean.PlannedActionInvokeWindowsServicing:
				servicing = append(servicing, definition.Identifier)
			default:
				t.Fatalf("executable %q has unsupported planned_action %q", definition.Identifier, definition.PlannedAction)
			}
		case clean.CategoryEligibilityPermissionBoundary, clean.CategoryEligibilityReviewOnly:
			if definition.PlannedAction != "" {
				t.Fatalf("non-executable %q must be actionless, got %q", definition.Identifier, definition.PlannedAction)
			}
		default:
			t.Fatalf("unexpected eligibility %q on %q", definition.Eligibility, definition.Identifier)
		}
	}

	// ADR 0029 adds exactly one invoke_windows_servicing category
	// (winsxs_component_store); #309 adds one move_to_recycle_bin category
	// (nvidia_installer_cache); #325 adds lghub-cache and #326 adds
	// thunder-update-download (both move_to_recycle_bin); #323 adds
	// nvidia_gl_cache and #324 adds vrchat_cache (both delete_permanently).
	if len(servicing) != 1 || servicing[0] != clean.CategoryWinSxSComponentStore {
		t.Fatalf("servicing matrix = %#v, want [%q]", servicing, clean.CategoryWinSxSComponentStore)
	}
	if len(executable) != 42 {
		t.Fatalf("executable categories = %d (%v), want 42 (41 deletion + 1 servicing)", len(executable), executable)
	}
	if !reflect.DeepEqual(permanent, wantPermanent) {
		t.Fatalf("permanent matrix = %#v, want %#v", permanent, wantPermanent)
	}
	if !reflect.DeepEqual(recycleBin, wantRecycleBin) {
		t.Fatalf("Recycle Bin matrix = %#v, want %#v", recycleBin, wantRecycleBin)
	}

	boundary, ok := catalog.Summary("administrator_only_caches")
	if !ok {
		t.Fatal("administrator_only_caches missing")
	}
	if boundary.Eligibility != clean.CategoryEligibilityPermissionBoundary || boundary.PlannedAction != "" {
		t.Fatalf("administrator_only_caches = %#v, want actionless permission boundary", boundary)
	}
	if clean.InitiallySelectedCategory(boundary) {
		t.Fatal("administrator_only_caches must never start selected")
	}

	// TUI initial selection when every executable row is present: default + 36 permanent = 37.
	selected := 0
	for _, summary := range catalog.Summaries() {
		if !clean.InitiallySelectedCategory(summary) {
			continue
		}
		selected++
		if summary.Eligibility != clean.CategoryEligibilityDefault &&
			summary.PlannedAction != clean.PlannedActionDeletePermanently {
			t.Fatalf("unexpected initial selection %q eligibility=%q action=%q",
				summary.Identifier, summary.Eligibility, summary.PlannedAction)
		}
	}
	if selected != 37 {
		t.Fatalf("initially selected categories = %d, want 37 (default + 36 permanent)", selected)
	}
	for _, id := range []string{
		clean.OpportunityCategoryWindowsErrorReporting,
	} {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if clean.InitiallySelectedCategory(summary) {
			t.Fatalf("%s must start unselected (Recycle Bin opt-in)", id)
		}
		if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
			t.Fatalf("%s planned_action = %q, want move_to_recycle_bin", id, summary.PlannedAction)
		}
	}

	// Eager queue is all 42 executable rows; permission boundary is never scanned.
	queue := clean.EagerPreviewQueue()
	if len(queue) != 42 {
		t.Fatalf("EagerPreviewQueue length = %d, want 42 executable categories", len(queue))
	}
	for _, summary := range queue {
		if summary.Identifier == "administrator_only_caches" {
			t.Fatal("permission boundary must not enter the eager queue")
		}
		if summary.PlannedAction != clean.PlannedActionDeletePermanently &&
			summary.PlannedAction != clean.PlannedActionMoveToRecycleBin &&
			summary.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
			t.Fatalf("queue %q planned_action = %q", summary.Identifier, summary.PlannedAction)
		}
	}
}

func TestCanonicalExecutableCategoriesDeclareExplicitPlannedActions(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	wantPermanent := productionPermanentCategoryIDs()
	for _, definition := range catalog.Definitions() {
		switch definition.Eligibility {
		case clean.CategoryEligibilityDefault, clean.CategoryEligibilityOptIn:
			if definition.PlannedAction == clean.PlannedActionInvokeWindowsServicing {
				// Servicing categories declare invoke_windows_servicing rather than
				// a deletion action; they are validated by the servicing matrix.
				if definition.Identifier != clean.CategoryWinSxSComponentStore {
					t.Fatalf("unexpected servicing category %q", definition.Identifier)
				}
				summary, ok := catalog.Summary(definition.Identifier)
				if !ok || summary.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
					t.Fatalf("servicing summary for %q = %#v", definition.Identifier, summary)
				}
				continue
			}
			want := clean.PlannedActionMoveToRecycleBin
			if wantPermanent[definition.Identifier] {
				want = clean.PlannedActionDeletePermanently
			}
			if definition.PlannedAction != want {
				t.Fatalf("executable category %q planned_action = %q, want %q",
					definition.Identifier, definition.PlannedAction, want)
			}
			summary, ok := catalog.Summary(definition.Identifier)
			if !ok || summary.PlannedAction != definition.PlannedAction {
				t.Fatalf("summary planned_action for %q = %#v, want %#v", definition.Identifier, summary, definition.PlannedAction)
			}
		case clean.CategoryEligibilityPermissionBoundary, clean.CategoryEligibilityReviewOnly:
			if definition.PlannedAction != "" {
				t.Fatalf("non-executable category %q must be actionless, got %q", definition.Identifier, definition.PlannedAction)
			}
			summary, ok := catalog.Summary(definition.Identifier)
			if !ok || summary.PlannedAction != "" {
				t.Fatalf("non-executable summary %q planned_action = %#v", definition.Identifier, summary)
			}
		default:
			t.Fatalf("unexpected eligibility %q on %q", definition.Eligibility, definition.Identifier)
		}
	}

	// No parallel permanent-delete eligibility boolean on public catalog types.
	summaryType := reflect.TypeOf(clean.CleanupCategorySummary{})
	for i :=0; i < summaryType.NumField(); i++ {
		name := summaryType.Field(i).Name
		if strings.Contains(strings.ToLower(name), "permanent") && name != "PlannedAction" {
			t.Fatalf("summary exposes permanent-eligibility field %q; planned_action must be sole source", name)
		}
		if strings.EqualFold(name, "CanPermanentDelete") || strings.EqualFold(name, "PermanentDeleteEligible") {
			t.Fatalf("summary exposes parallel eligibility boolean %q", name)
		}
	}
}

func TestProductionPermanentCategoriesMatchActivationSet(t *testing.T) {
	// Compatibility alias for the locked permanent set; full matrix is TestCompleteDeletionRuleMatrixLocked.
	catalog := clean.CanonicalCleanupCategoryCatalog()
	want := productionPermanentCategoryIDs()
	var permanent []string
	for _, definition := range catalog.Definitions() {
		if definition.PlannedAction == clean.PlannedActionDeletePermanently {
			permanent = append(permanent, definition.Identifier)
			if !want[definition.Identifier] {
				t.Fatalf("unexpected permanent category %q", definition.Identifier)
			}
			summary, ok := catalog.Summary(definition.Identifier)
			if !ok {
				t.Fatalf("%s missing from catalog", definition.Identifier)
			}
			if summary.Eligibility != clean.CategoryEligibilityOptIn {
				t.Fatalf("%s eligibility = %q, want opt-in", definition.Identifier, summary.Eligibility)
			}
			if !clean.InitiallySelectedCategory(summary) {
				t.Fatalf("%s must initially select when permanently eligible", definition.Identifier)
			}
		}
	}
	if len(permanent) != 36 || len(permanent) != len(want) {
		t.Fatalf("production permanent categories = %v, want exactly 36", permanent)
	}
	for _, id := range lockedRecycleBinCategoryIDs() {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
			t.Fatalf("%s planned_action = %q, want move_to_recycle_bin", id, summary.PlannedAction)
		}
		if summary.Eligibility == clean.CategoryEligibilityOptIn && clean.InitiallySelectedCategory(summary) {
			t.Fatalf("%s must start unselected (Recycle Bin opt-in)", id)
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
	if len(invalid) !=0 || !enabled[clean.OpportunityCategoryCrashDumps] {
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
	if len(invalid) !=0 {
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
	if len(appInvalid) !=0 {
		t.Fatalf("app-caches invalid = %#v", appInvalid)
	}
	if len(appEnabled) != 2 || !appEnabled[clean.OpportunityCategoryObsidianCache] || !appEnabled[clean.OpportunityCategoryVRChatCache] {
		t.Fatalf("app-caches enabled = %#v, want obsidian_cache and vrchat_cache", appEnabled)
	}
	for _, id := range wantDevCachesGroup {
		if appEnabled[id] {
			t.Fatalf("app-caches must not enable developer-tools cache category %q", id)
		}
	}

	// cli-agents expands product-scoped CLI-agent residue only (catalog order).
	cliEnabled, cliInvalid, _ := clean.NormalizedOptInSet([]string{clean.CLIAgentCategoryGroup})
	if len(cliInvalid) !=0 {
		t.Fatalf("cli-agents invalid = %#v", cliInvalid)
	}
	if len(cliEnabled) !=1 || !cliEnabled[clean.CategoryGrokBuildUpdateResidue] {
		t.Fatalf("cli-agents enabled = %#v, want only grok-build-update-residue", cliEnabled)
	}
	for _, id := range wantDevCachesGroup {
		if cliEnabled[id] {
			t.Fatalf("cli-agents must not enable cache category %q", id)
		}
	}
}
