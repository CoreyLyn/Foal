package clean_test

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestNVIDIAInstallerCache_CatalogAndSelectionContract(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryNVIDIAInstallerCache)
	if !ok {
		t.Fatal("nvidia_installer_cache missing from catalog")
	}
	if summary.Label != "NVIDIA installer cache" {
		t.Fatalf("label = %q", summary.Label)
	}
	if summary.ReportCategory != clean.ReportCategorySystem {
		t.Fatalf("report category = %q, want System", summary.ReportCategory)
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q, want opt-in", summary.Eligibility)
	}
	if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
		t.Fatalf("planned_action = %q, want move_to_recycle_bin", summary.PlannedAction)
	}
	if summary.SelectionPolicy != clean.CategorySelectionPolicyExactOnly {
		t.Fatalf("selection policy = %q, want exact-selection-only", summary.SelectionPolicy)
	}
	if !clean.ExactSelectionOnlyCategory(summary) {
		t.Fatal("category must be exact-selection-only")
	}
	if clean.InitiallySelectedCategory(summary) {
		t.Fatal("exact-selection-only category must start unselected")
	}
}

func TestNVIDIAInstallerCache_ExcludedFromAggregateAndGroupTokens(t *testing.T) {
	// Exact name is a valid opt-in and selects only this category.
	enabled, invalid, valid := clean.NormalizedOptInSet([]string{clean.CategoryNVIDIAInstallerCache})
	if len(invalid) != 0 {
		t.Fatalf("exact name invalid = %#v", invalid)
	}
	if len(enabled) != 1 || !enabled[clean.CategoryNVIDIAInstallerCache] {
		t.Fatalf("exact name enabled = %#v, want only nvidia_installer_cache", enabled)
	}
	foundValid := false
	for _, name := range valid {
		if name == clean.CategoryNVIDIAInstallerCache {
			foundValid = true
		}
	}
	if !foundValid {
		t.Fatal("nvidia_installer_cache should be a valid opt-in name")
	}

	// Aggregate and group tokens must never select it.
	for _, token := range []string{"all", clean.DevCacheCategoryAll, clean.ApplicationCacheCategoryGroup, clean.CLIAgentCategoryGroup} {
		enabled, _, _ := clean.NormalizedOptInSet([]string{token})
		if enabled[clean.CategoryNVIDIAInstallerCache] {
			t.Fatalf("token %q must not enable nvidia_installer_cache", token)
		}
	}
}

func TestNVIDIAInstallerCache_ExactPlanAcceptsAdditivePlanExcludesFromAll(t *testing.T) {
	// Exact plan accepts the exact identifier.
	plan, err := clean.CompileExactCategoryPlan([]string{clean.CategoryNVIDIAInstallerCache})
	if err != nil {
		t.Fatalf("exact plan error: %v", err)
	}
	if len(plan.Categories) != 1 || plan.Categories[0] != clean.CategoryNVIDIAInstallerCache {
		t.Fatalf("exact plan categories = %#v", plan.Categories)
	}

	// Additive "all" plan excludes the exact-selection-only category.
	additive, invalid := clean.CompileAdditiveCategoryPlan([]string{"all"})
	if len(invalid) != 0 {
		t.Fatalf("additive all invalid = %#v", invalid)
	}
	for _, id := range additive.Categories {
		if id == clean.CategoryNVIDIAInstallerCache {
			t.Fatal("all token must not include nvidia_installer_cache")
		}
	}
}

func TestNVIDIAInstallerCache_DryRunReportsRecycleBinAndImpact(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaResolveOptions(fx)
	opts.OptIn = []string{clean.CategoryNVIDIAInstallerCache}

	result := clean.DryRun(context.Background(), opts)

	found := false
	for _, candidate := range result.OptInCandidates {
		if candidate.Category == clean.CategoryNVIDIAInstallerCache {
			found = true
			if candidate.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("dry-run planned action = %q, want move_to_recycle_bin", candidate.PlannedAction)
			}
		}
	}
	if !found {
		t.Fatalf("dry-run did not report the NVIDIA candidate: %#v", result.OptInCandidates)
	}

	model := clean.NewPreviewReadModel(result)
	impact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "NVIDIA driver download") {
			impact = true
			if strings.Contains(strings.ToLower(notice.Message), "permanent deletion moves") {
				t.Fatal("impact notice must not imply permanent-delete eligibility")
			}
		}
	}
	if !impact {
		t.Fatalf("dry-run missing NVIDIA re-download/offline impact notice: %#v", model.Notices)
	}
}

func TestNVIDIAInstallerCache_ExecuteDoesNotMutate(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaResolveOptions(fx)
	opts.OptIn = []string{clean.CategoryNVIDIAInstallerCache}
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter

	result := clean.Execute(context.Background(), opts)

	if len(adapter.paths) != 0 {
		t.Fatalf("execution must not move NVIDIA candidates, adapter saw %v", adapter.paths)
	}
	for _, deleted := range result.Deleted {
		if deleted.Rule == clean.CategoryNVIDIAInstallerCache {
			t.Fatalf("NVIDIA candidate was unexpectedly deleted: %#v", deleted)
		}
	}
	skipped := false
	for _, item := range result.Skipped {
		if item.Rule == clean.CategoryNVIDIAInstallerCache {
			skipped = true
			if item.Reason.Code != "nvidia_installer_cache_execution_pending" {
				t.Fatalf("skip reason = %q, want nvidia_installer_cache_execution_pending", item.Reason.Code)
			}
			if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("skip planned action = %q, want move_to_recycle_bin", item.PlannedAction)
			}
		}
	}
	if !skipped {
		t.Fatalf("expected NVIDIA candidate to be fail-closed skipped, skipped = %#v", result.Skipped)
	}
}
