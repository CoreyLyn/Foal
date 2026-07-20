package cli

import (
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestSelectAllExcludesExactSelectionOnlyRow proves the Clean TUI Select All
// action honors the catalog's exact-selection-only policy: a standard opt-in row
// is selected, while an exact-selection-only servicing row is not. The exact row
// also starts unselected.
func TestSelectAllExcludesExactSelectionOnlyRow(t *testing.T) {
	standard := clean.CleanupCategorySummary{
		Identifier:               "user_temp",
		Label:                    "User temp",
		ReportCategory:           clean.ReportCategoryUserEssentials,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
	}
	exactOnly := clean.CleanupCategorySummary{
		Identifier:               "winsxs_component_store",
		Label:                    "Windows component store",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionInvokeWindowsServicing,
		SelectionPolicy:          clean.CategorySelectionPolicyExactOnly,
	}

	model := newEagerCleanModelFromSummaries([]clean.CleanupCategorySummary{standard, exactOnly}, 80, 24)

	// Initial selection: neither the recycle opt-in nor the exact-only row starts selected.
	for _, row := range model.rows {
		if row.Selected {
			t.Fatalf("row %q started selected, want unselected", row.Identifier)
		}
	}

	model.selectAllSelectable()

	got := map[string]bool{}
	for _, row := range model.rows {
		got[row.Identifier] = row.Selected
	}
	if !got["user_temp"] {
		t.Fatal("Select All should select the standard opt-in row")
	}
	if got["winsxs_component_store"] {
		t.Fatal("Select All must exclude the exact-selection-only row")
	}
}
