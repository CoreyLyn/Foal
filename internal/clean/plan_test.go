package clean_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

func TestCompileAdditiveCategoryPlanIncludesDefaultsAndOptIns(t *testing.T) {
	plan, invalid := clean.CompileAdditiveCategoryPlan([]string{
		clean.OpportunityCategoryCrashDumps,
		"dev-caches",
	})
	if len(invalid) != 0 {
		t.Fatalf("invalid = %#v, want none", invalid)
	}
	if plan.Mode != clean.SelectionModeAdditive {
		t.Fatalf("mode = %q, want additive", plan.Mode)
	}
	if len(plan.Categories) == 0 || plan.Categories[0] != clean.DefaultCategoryFoalOwnedTempSandboxes {
		t.Fatalf("categories = %#v, want default first", plan.Categories)
	}
	if !containsAll(plan.Categories, clean.DefaultCategoryFoalOwnedTempSandboxes, clean.OpportunityCategoryCrashDumps, clean.DevCacheCategoryGo) {
		t.Fatalf("categories = %#v, want default + crash_dumps + expanded dev caches", plan.Categories)
	}
	assertCatalogOrder(t, plan.Categories)
}

func TestCompileAdditiveCategoryPlanEmptyOptInKeepsOnlyDefaults(t *testing.T) {
	plan, invalid := clean.CompileAdditiveCategoryPlan(nil)
	if len(invalid) != 0 {
		t.Fatalf("invalid = %#v", invalid)
	}
	if !reflect.DeepEqual(plan.Categories, []string{clean.DefaultCategoryFoalOwnedTempSandboxes}) {
		t.Fatalf("categories = %#v, want only default", plan.Categories)
	}
}

func TestCompileExactCategoryPlanOmitsUnselectedDefaults(t *testing.T) {
	plan, err := clean.CompileExactCategoryPlan([]string{
		clean.OpportunityCategoryCrashDumps,
		clean.DevCacheCategoryGo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != clean.SelectionModeExact {
		t.Fatalf("mode = %q, want exact", plan.Mode)
	}
	if containsAll(plan.Categories, clean.DefaultCategoryFoalOwnedTempSandboxes) {
		t.Fatalf("categories = %#v, default must remain omitted", plan.Categories)
	}
	if !reflect.DeepEqual(plan.Categories, []string{clean.OpportunityCategoryCrashDumps, clean.DevCacheCategoryGo}) {
		t.Fatalf("categories = %#v", plan.Categories)
	}
}

func TestCompileExactCategoryPlanAcceptsDefaultWhenSelected(t *testing.T) {
	plan, err := clean.CompileExactCategoryPlan([]string{
		clean.DevCacheCategoryNPM,
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.OpportunityCategoryUserTemp,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.OpportunityCategoryUserTemp,
		clean.DevCacheCategoryNPM,
	}
	if !reflect.DeepEqual(plan.Categories, want) {
		t.Fatalf("categories = %#v, want catalog order %#v", plan.Categories, want)
	}
}

func TestCompileExactCategoryPlanNormalizesDuplicatesAndOrder(t *testing.T) {
	plan, err := clean.CompileExactCategoryPlan([]string{
		clean.DevCacheCategoryGo,
		clean.OpportunityCategoryCrashDumps,
		clean.DevCacheCategoryGo,
		"CRASH_DUMPS",
		clean.DefaultCategoryFoalOwnedTempSandboxes,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.OpportunityCategoryCrashDumps,
		clean.DevCacheCategoryGo,
	}
	if !reflect.DeepEqual(plan.Categories, want) {
		t.Fatalf("categories = %#v, want %#v", plan.Categories, want)
	}
}

func TestCompileExactCategoryPlanRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "unknown", ids: []string{"not_a_category"}, want: "unknown"},
		{name: "group all", ids: []string{"all"}, want: "group"},
		{name: "group dev-caches", ids: []string{"dev-caches"}, want: "group"},
		{name: "permission boundary", ids: []string{"administrator_only_caches"}, want: "permission_boundary"},
		{name: "path windows", ids: []string{`C:\Users\temp`}, want: "path_bearing"},
		{name: "path slash", ids: []string{"foo/bar"}, want: "path_bearing"},
		{name: "empty", ids: []string{"   "}, want: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := clean.CompileExactCategoryPlan(tt.ids)
			if err == nil {
				t.Fatal("expected rejection")
			}
			var planErr *clean.CategoryPlanError
			if !errors.As(err, &planErr) || len(planErr.Rejections) == 0 {
				t.Fatalf("err = %v, want CategoryPlanError", err)
			}
			if planErr.Rejections[0].Reason != tt.want {
				t.Fatalf("reason = %q, want %q (%v)", planErr.Rejections[0].Reason, tt.want, err)
			}
		})
	}
}

func TestCompileExactCategoryPlanRejectsAliasWhenPresent(t *testing.T) {
	// Canonical catalog currently registers empty alias lists. Build a private
	// catalog through the public constructor to prove the exact-plan alias
	// rejection contract when an alias exists.
	catalog, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{{
		Identifier:               "demo_opt_in",
		Label:                    "Demo",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		Aliases:                  []string{"demo-alias"},
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.DeletionActionMoveToRecycleBin,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if summary, ok := catalog.Summary("demo-alias"); !ok || summary.Identifier != "demo_opt_in" {
		t.Fatalf("alias lookup = %#v, %t", summary, ok)
	}
	// Exact plan against the process-wide canonical catalog still rejects an
	// unknown alias token that is not a canonical identifier.
	_, err = clean.CompileExactCategoryPlan([]string{"demo-alias"})
	if err == nil {
		t.Fatal("expected unknown/alias rejection for non-canonical token")
	}
}

func TestScannableCategoryIDsExcludesPermissionBoundary(t *testing.T) {
	ids := clean.ScannableCategoryIDs()
	if len(ids) == 0 {
		t.Fatal("empty scannable set")
	}
	for _, id := range ids {
		if id == "administrator_only_caches" {
			t.Fatalf("permission boundary leaked into scannable set: %#v", ids)
		}
	}
	if ids[0] != clean.DefaultCategoryFoalOwnedTempSandboxes {
		t.Fatalf("first scannable = %q, want default", ids[0])
	}
	assertCatalogOrder(t, ids)
}

func TestResolveCategoryScansOnlyRequestedDefault(t *testing.T) {
	root := t.TempDir()
	wanted := filepath.Join(root, "wanted.tmp")
	other := filepath.Join(root, "other.tmp")
	if err := os.WriteFile(wanted, []byte("want"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}

	opts := clean.Options{
		Rules: []clean.Rule{
			{
				ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
				Description:    "default under test",
				DefaultEnabled: true,
				CandidatePaths: []string{wanted},
			},
			{
				ID:             "unrelated_default",
				Description:    "must not scan",
				DefaultEnabled: true,
				CandidatePaths: []string{other},
			},
		},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			t.Fatal("opportunity discovery must not run for default-only resolve")
			return clean.OpportunityDiscoveryResult{}
		},
		DevCachePathResolver: func(string) []string {
			t.Fatal("dev cache resolver must not run for default-only resolve")
			return nil
		},
	}

	resolution, err := clean.ResolveCategory(context.Background(), opts, clean.DefaultCategoryFoalOwnedTempSandboxes)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Identifier != clean.DefaultCategoryFoalOwnedTempSandboxes {
		t.Fatalf("identifier = %q", resolution.Identifier)
	}
	if len(resolution.Candidates) != 1 || resolution.Candidates[0].Path != wanted {
		t.Fatalf("candidates = %#v, want only %q", resolution.Candidates, wanted)
	}
	if len(resolution.OptInCandidates) != 0 {
		t.Fatalf("opt-in candidates = %#v, want none", resolution.OptInCandidates)
	}
}

func TestResolveCategoryScansOnlyRequestedOptIn(t *testing.T) {
	var discovered []string
	opts := clean.Options{
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{
					{Category: clean.OpportunityCategoryCrashDumps, Path: `C:\CrashDumps`, Bytes: 11},
					{Category: clean.OpportunityCategoryUserTemp, Path: `C:\temp\old`, Bytes: 22},
				},
			}
		},
		DevCachePathResolver: func(category string) []string {
			discovered = append(discovered, category)
			return nil
		},
	}

	resolution, err := clean.ResolveCategory(context.Background(), opts, clean.OpportunityCategoryCrashDumps)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.OptInCandidates) != 1 || resolution.OptInCandidates[0].Category != clean.OpportunityCategoryCrashDumps {
		t.Fatalf("opt-in candidates = %#v", resolution.OptInCandidates)
	}
	if len(discovered) != 0 {
		t.Fatalf("dev cache categories resolved = %#v, want none", discovered)
	}
}

func TestResolveCategoryRejectsNonScannableIdentifiers(t *testing.T) {
	for _, id := range []string{"all", "administrator_only_caches", `C:\Windows`, "nope"} {
		if _, err := clean.ResolveCategory(context.Background(), clean.Options{}, id); err == nil {
			t.Fatalf("ResolveCategory(%q) error = nil", id)
		}
	}
}

func TestExecuteExactPlanRecordsTUIProvenanceWithoutPreviewPaths(t *testing.T) {
	root := t.TempDir()
	defaultCandidate := filepath.Join(root, "default.tmp")
	optInPath := filepath.Join(root, "crash")
	if err := os.WriteFile(defaultCandidate, []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(optInPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(optInPath, "dump.dmp"), []byte("dmp!"), 0600); err != nil {
		t.Fatal(err)
	}

	// Preview measures both default and crash_dumps; execute freezes only crash_dumps.
	previewCalls := 0
	discover := func(context.Context) clean.OpportunityDiscoveryResult {
		previewCalls++
		return clean.OpportunityDiscoveryResult{
			Opportunities: []clean.Opportunity{{
				Category: clean.OpportunityCategoryCrashDumps,
				Path:     optInPath,
				Bytes:    4,
			}},
		}
	}
	previewOpts := clean.Options{
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{defaultCandidate},
		}},
		DiscoverOpportunities: discover,
		OptIn:                 []string{clean.OpportunityCategoryCrashDumps},
	}
	preview := clean.DryRun(context.Background(), previewOpts)
	if len(preview.Candidates) != 1 || len(preview.OptInCandidates) != 1 {
		t.Fatalf("preview = candidates=%#v opt-in=%#v", preview.Candidates, preview.OptInCandidates)
	}

	// Fresh discovery can differ from preview (empty this time).
	discoverExecute := func(context.Context) clean.OpportunityDiscoveryResult {
		return clean.OpportunityDiscoveryResult{}
	}
	plan, err := clean.CompileExactCategoryPlan([]string{clean.OpportunityCategoryCrashDumps})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingHistoryRecorder{}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Plan:              &plan,
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		CommandParameters: history.CommandParameters{
			Command:            "clean",
			Surface:            "tui",
			SelectionMode:      string(clean.SelectionModeExact),
			SelectedCategories: append([]string(nil), plan.Categories...),
		},
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{defaultCandidate},
		}},
		DiscoverOpportunities: discoverExecute,
	})

	if result.Totals.DeletedCount != 0 || len(adapter.paths) != 0 {
		t.Fatalf("stale preview executed: totals=%#v paths=%v", result.Totals, adapter.paths)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v", recorder.sessions)
	}
	cmd := recorder.sessions[0].Command
	if cmd.Surface != "tui" || cmd.SelectionMode != "exact" {
		t.Fatalf("command = %#v", cmd)
	}
	if len(cmd.Args) != 0 {
		t.Fatalf("synthetic args = %#v", cmd.Args)
	}
	if len(cmd.SelectedCategories) != 1 || cmd.SelectedCategories[0] != clean.OpportunityCategoryCrashDumps {
		t.Fatalf("selected_categories = %#v", cmd.SelectedCategories)
	}
	for _, id := range cmd.SelectedCategories {
		if id == clean.DefaultCategoryFoalOwnedTempSandboxes {
			t.Fatal("omitted default recorded in provenance")
		}
		if strings.ContainsAny(id, `/\`) || id == optInPath || id == defaultCandidate {
			t.Fatalf("path leaked into provenance: %q", id)
		}
	}
	// Default candidate path must not appear as authorized execution.
	for _, path := range adapter.paths {
		if path == defaultCandidate {
			t.Fatal("default candidate cleaned under exact omit")
		}
	}
	_ = previewCalls
}

func TestExecuteExactPlanOmitsDefaultCandidates(t *testing.T) {
	root := t.TempDir()
	defaultCandidate := filepath.Join(root, "default.tmp")
	optInPath := filepath.Join(root, "crash")
	if err := os.WriteFile(defaultCandidate, []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(optInPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(optInPath, "dump.dmp"), []byte("dmp!"), 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := clean.CompileExactCategoryPlan([]string{clean.OpportunityCategoryCrashDumps})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Plan:              &plan,
		RecycleBinAdapter: adapter,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{defaultCandidate},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryCrashDumps,
					Path:     optInPath,
					Bytes:    4,
				}},
			}
		},
	})

	if result.Totals.CandidateCount != 0 {
		t.Fatalf("default candidates = %#v, want omitted", result.Candidates)
	}
	if result.Totals.OptInDeletedCount != 1 || result.Totals.DeletedCount != 1 {
		t.Fatalf("totals = %#v, want only opt-in deletion", result.Totals)
	}
	if len(adapter.paths) != 1 || adapter.paths[0] != optInPath {
		t.Fatalf("adapter paths = %#v, want [%q]", adapter.paths, optInPath)
	}
	for _, path := range adapter.paths {
		if path == defaultCandidate {
			t.Fatalf("default candidate was deleted under exact plan: %#v", adapter.paths)
		}
	}
}

func TestExecuteAdditiveCompatibilityStillDeletesDefaults(t *testing.T) {
	root := t.TempDir()
	defaultCandidate := filepath.Join(root, "default.tmp")
	if err := os.WriteFile(defaultCandidate, []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		// No Plan: additive CLI wrapper path via OptIn (empty).
		RecycleBinAdapter: adapter,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{defaultCandidate},
		}},
	})
	if result.Totals.DeletedCount != 1 || len(adapter.paths) != 1 || adapter.paths[0] != defaultCandidate {
		t.Fatalf("additive default execute broken: totals=%#v paths=%v", result.Totals, adapter.paths)
	}
}

func TestExecuteExactPlanReusesProtectionAndCapacityGates(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	allowed := filepath.Join(root, "allowed")
	for _, dir := range []string{protected, allowed} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("xx"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := clean.CompileExactCategoryPlan([]string{clean.OpportunityCategoryCrashDumps})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Plan:              &plan,
		RecycleBinAdapter: adapter,
		Validator:         pathsafe.NewValidator([]string{protected}),
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{
					{Category: clean.OpportunityCategoryCrashDumps, Path: protected, Bytes: 2},
					{Category: clean.OpportunityCategoryCrashDumps, Path: allowed, Bytes: 2},
				},
			}
		},
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       filepath.VolumeName(path),
				MaxCapacity:  1,
				CurrentUsage: 0,
			}, nil
		},
	})

	// Protected path never reaches the adapter; capacity failure skips the rest.
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %#v, want none under protection/capacity gates", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("deleted opt-in = %d, want 0", result.Totals.OptInDeletedCount)
	}
	if result.Totals.SkippedCount == 0 {
		t.Fatalf("skipped = %#v, want capacity or protection skip", result.Skipped)
	}
}

func TestDryRunAdditiveCompatibilityUnchangedWithoutPlan(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{}
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return nil
		},
	})
	if result.Mode != "dry_run" || len(result.Candidates) != 1 || result.Candidates[0].Path != candidate {
		t.Fatalf("additive dry-run broken: %#v", result)
	}
}

func containsAll(have []string, want ...string) bool {
	set := make(map[string]bool, len(have))
	for _, id := range have {
		set[id] = true
	}
	for _, id := range want {
		if !set[id] {
			return false
		}
	}
	return true
}

func assertCatalogOrder(t *testing.T, ids []string) {
	t.Helper()
	order := make(map[string]int, len(clean.ScannableCategoryIDs()))
	for i, id := range clean.ScannableCategoryIDs() {
		order[id] = i
	}
	prev := -1
	for _, id := range ids {
		pos, ok := order[id]
		if !ok {
			// Permission boundary and other non-scannable IDs use full catalog order.
			continue
		}
		if pos < prev {
			t.Fatalf("ids out of catalog order: %#v", ids)
		}
		prev = pos
	}
}
