package clean_test

import (
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestInProgressExecutionStateProjectsSharedPhases(t *testing.T) {
	cases := []struct {
		phase clean.ExecutionPhase
		want  clean.CategoryExecutionState
	}{
		{clean.ExecutionPhaseScanning, clean.CategoryExecutionRechecking},
		{clean.ExecutionPhaseRecycleBinSafety, clean.CategoryExecutionReady},
		{clean.ExecutionPhaseRecycleBinOperations, clean.CategoryExecutionCleaning},
		{clean.ExecutionPhasePermanentOperations, clean.CategoryExecutionCleaning},
		{clean.ExecutionPhaseComplete, clean.CategoryExecutionCleaning},
		{"", clean.CategoryExecutionRechecking},
	}
	for _, tc := range cases {
		got := clean.InProgressExecutionState(tc.phase)
		if got != tc.want {
			t.Fatalf("phase %q = %q, want %q", tc.phase, got, tc.want)
		}
		if clean.IsTerminalExecutionState(got) {
			t.Fatalf("in-progress %q must not be terminal", got)
		}
	}
}

func TestProjectCategoryExecutionOutcomesEmptyWhenNoCandidates(t *testing.T) {
	selected := []string{clean.DefaultCategoryFoalOwnedTempSandboxes, clean.OpportunityCategoryCrashDumps}
	outcomes := clean.ProjectCategoryExecutionOutcomes(selected, clean.Result{Status: "ok", Mode: "execute"})
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	for _, outcome := range outcomes {
		if outcome.State != clean.CategoryExecutionEmpty {
			t.Fatalf("state = %q, want empty for %#v", outcome.State, outcome)
		}
		if outcome.AffectedBytes != 0 {
			t.Fatalf("empty affected = %d", outcome.AffectedBytes)
		}
		if outcome.Label == "" || outcome.Label == outcome.Identifier {
			// Labels come from the catalog; default/crash dumps both have labels.
			if outcome.Label == outcome.Identifier {
				t.Fatalf("missing catalog label for %q", outcome.Identifier)
			}
		}
	}
	if clean.CountTerminalExecutionOutcomes(outcomes) != 2 {
		t.Fatalf("terminal count = %d", clean.CountTerminalExecutionOutcomes(outcomes))
	}
}

func TestProjectCategoryExecutionOutcomesCleanedFullSuccess(t *testing.T) {
	id := clean.DefaultCategoryFoalOwnedTempSandboxes
	result := clean.Result{
		Status: "ok",
		Mode:   "execute",
		Deleted: []clean.DeletedItem{
			{Path: `C:\Users\me\AppData\Local\Temp\foal-a`, Bytes: 10, Rule: id},
			{Path: `C:\Users\me\AppData\Local\Temp\foal-b`, Bytes: 20, Rule: id},
		},
		Totals: clean.Totals{DeletedCount: 2, AffectedBytes: 30},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) != 1 || outcomes[0].State != clean.CategoryExecutionCleaned {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if outcomes[0].AffectedBytes != 30 || outcomes[0].DeletedCount != 2 {
		t.Fatalf("bytes/count = %#v", outcomes[0])
	}
	assertOutcomePathFree(t, outcomes, `C:\Users\me`)
}

func TestProjectCategoryExecutionOutcomesPartialMixedSuccessAndExclusion(t *testing.T) {
	id := clean.OpportunityCategoryUserTemp
	result := clean.Result{
		Status: "ok",
		Mode:   "execute",
		Deleted: []clean.DeletedItem{
			{Path: `C:\Users\me\AppData\Local\Temp\old`, Bytes: 4, Rule: id},
		},
		Skipped: []clean.SkippedItem{
			{
				Path:   `C:\Users\me\AppData\Local\Temp\protected-child`,
				Bytes:  8,
				Rule:   id,
				Reason: clean.StructuredIssue{Code: "protected_path", Message: "protected"},
			},
		},
		Totals: clean.Totals{DeletedCount: 1, SkippedCount: 1, AffectedBytes: 4},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) != 1 || outcomes[0].State != clean.CategoryExecutionPartial {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if outcomes[0].AffectedBytes != 4 {
		t.Fatalf("partial must count only successful moves: %#v", outcomes[0])
	}
	assertOutcomePathFree(t, outcomes, `C:\Users\me`, "protected-child")
}

func TestProjectCategoryExecutionOutcomesSkippedFailedCanceled(t *testing.T) {
	protectedID := clean.OpportunityCategoryCrashDumps
	failedID := clean.DevCacheCategoryGo
	canceledID := clean.OpportunityCategoryUserTemp

	result := clean.Result{
		Status: "ok",
		Mode:   "execute",
		Skipped: []clean.SkippedItem{
			{
				Path:   `C:\Users\me\AppData\Local\CrashDumps\a.dmp`,
				Rule:   protectedID,
				Reason: clean.StructuredIssue{Code: "protected_path", Message: `blocked C:\Users\me\AppData\Local\CrashDumps\a.dmp`},
			},
			{
				Path:   `C:\Users\me\AppData\Local\go-build\pkg`,
				Rule:   failedID,
				Reason: clean.StructuredIssue{Code: "delete_failed", Message: `move failed for C:\Users\me\AppData\Local\go-build\pkg`},
			},
			{
				Path:   `C:\Users\me\AppData\Local\Temp\idle`,
				Rule:   canceledID,
				Reason: clean.StructuredIssue{Code: "context_canceled", Message: "context canceled"},
			},
		},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes(
		[]string{protectedID, failedID, canceledID},
		result,
	)
	want := map[string]clean.CategoryExecutionState{
		protectedID: clean.CategoryExecutionSkipped,
		failedID:    clean.CategoryExecutionFailed,
		canceledID:  clean.CategoryExecutionCanceled,
	}
	if len(outcomes) != 3 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	for _, outcome := range outcomes {
		if outcome.State != want[outcome.Identifier] {
			t.Fatalf("%s state = %q, want %q", outcome.Identifier, outcome.State, want[outcome.Identifier])
		}
		if outcome.AffectedBytes != 0 {
			t.Fatalf("%s affected bytes must be 0: %#v", outcome.Identifier, outcome)
		}
	}
	assertOutcomePathFree(t, outcomes, `C:\Users\me`, "CrashDumps", "go-build")
}

func TestProjectCategoryExecutionOutcomesPartialWithCanceledItem(t *testing.T) {
	id := clean.DefaultCategoryFoalOwnedTempSandboxes
	result := clean.Result{
		Status: "ok",
		Deleted: []clean.DeletedItem{
			{Path: `C:\Temp\done`, Bytes: 5, Rule: id},
		},
		Skipped: []clean.SkippedItem{
			{
				Path:   `C:\Temp\remaining`,
				Rule:   id,
				Reason: clean.StructuredIssue{Code: "context_canceled", Message: "context canceled"},
			},
		},
		Totals: clean.Totals{DeletedCount: 1, SkippedCount: 1, AffectedBytes: 5},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) != 1 || outcomes[0].State != clean.CategoryExecutionPartial {
		t.Fatalf("success + cancel must be partial: %#v", outcomes)
	}
	if outcomes[0].AffectedBytes != 5 {
		t.Fatalf("completed moves remain counted: %#v", outcomes[0])
	}
}

func TestProjectCategoryExecutionOutcomesIgnoresUnselectedCategories(t *testing.T) {
	selected := clean.DefaultCategoryFoalOwnedTempSandboxes
	other := clean.OpportunityCategoryUserTemp
	result := clean.Result{
		Status: "ok",
		Deleted: []clean.DeletedItem{
			{Path: `C:\a`, Bytes: 1, Rule: selected},
			{Path: `C:\b`, Bytes: 99, Rule: other},
		},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{selected}, result)
	if len(outcomes) != 1 || outcomes[0].Identifier != selected {
		t.Fatalf("must project selected only: %#v", outcomes)
	}
	if outcomes[0].AffectedBytes != 1 {
		t.Fatalf("unselected bytes leaked: %#v", outcomes[0])
	}
}

func TestProjectCategoryExecutionOutcomesRunLevelFailureMarksEmptyCategoriesFailed(t *testing.T) {
	id := clean.DefaultCategoryFoalOwnedTempSandboxes
	result := clean.Result{
		Status: "error",
		Mode:   "execute",
		Errors: []clean.StructuredIssue{{
			Code:    "invalid_category_plan",
			Message: "invalid",
		}},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) != 1 || outcomes[0].State != clean.CategoryExecutionFailed {
		t.Fatalf("run-level failure: %#v", outcomes)
	}
}

func TestProjectCategoryExecutionOutcomesPreservesSelectionOrder(t *testing.T) {
	// Catalog order is crash_dumps before go-cache; selection order may differ.
	order := []string{clean.DevCacheCategoryGo, clean.OpportunityCategoryCrashDumps}
	outcomes := clean.ProjectCategoryExecutionOutcomes(order, clean.Result{Status: "ok"})
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if outcomes[0].Identifier != order[0] || outcomes[1].Identifier != order[1] {
		t.Fatalf("order = %#v, want %#v", []string{outcomes[0].Identifier, outcomes[1].Identifier}, order)
	}
}

func TestSumAndCountExecutionOutcomes(t *testing.T) {
	outcomes := []clean.CategoryExecutionOutcome{
		{State: clean.CategoryExecutionCleaned, AffectedBytes: 3},
		{State: clean.CategoryExecutionRechecking, AffectedBytes: 0},
		{State: clean.CategoryExecutionPartial, AffectedBytes: 7},
	}
	if clean.CountTerminalExecutionOutcomes(outcomes) != 2 {
		t.Fatalf("terminal = %d", clean.CountTerminalExecutionOutcomes(outcomes))
	}
	if clean.SumExecutionAffectedBytes(outcomes) != 10 {
		t.Fatalf("sum = %d", clean.SumExecutionAffectedBytes(outcomes))
	}
}

func assertOutcomePathFree(t *testing.T, outcomes []clean.CategoryExecutionOutcome, forbidden ...string) {
	t.Helper()
	for _, outcome := range outcomes {
		blob := strings.Join([]string{
			outcome.Identifier,
			outcome.Label,
			string(outcome.State),
		}, " ")
		for _, token := range forbidden {
			if strings.Contains(blob, token) {
				t.Fatalf("path token %q leaked in %#v", token, outcome)
			}
		}
		if strings.ContainsAny(outcome.Label, `/\`) {
			t.Fatalf("label looks path-bearing: %#v", outcome)
		}
	}
}
