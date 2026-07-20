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

func TestProjectInProgressCategoryStateWaitingVsActive(t *testing.T) {
	active := "npm-cache"
	other := "pnpm-cache"
	cases := []struct {
		name   string
		phase  clean.ExecutionPhase
		active string
		id     string
		want   clean.CategoryExecutionState
	}{
		{"active scanning", clean.ExecutionPhaseScanning, active, active, clean.CategoryExecutionRechecking},
		{"other waiting", clean.ExecutionPhaseScanning, active, other, clean.CategoryExecutionWaiting},
		{"empty active applies phase", clean.ExecutionPhaseRecycleBinSafety, "", active, clean.CategoryExecutionReady},
		{"empty active other also phase", clean.ExecutionPhaseRecycleBinSafety, "", other, clean.CategoryExecutionReady},
		{"active cleaning", clean.ExecutionPhasePermanentOperations, active, active, clean.CategoryExecutionCleaning},
		{"non-active during permanent waits", clean.ExecutionPhasePermanentOperations, active, other, clean.CategoryExecutionWaiting},
	}
	for _, tc := range cases {
		got := clean.ProjectInProgressCategoryState(tc.phase, tc.active, tc.id)
		if got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
		if clean.IsTerminalExecutionState(got) {
			t.Fatalf("%s: must not be terminal", tc.name)
		}
	}
	if clean.IsTerminalExecutionState(clean.CategoryExecutionWaiting) {
		t.Fatal("waiting must not be terminal")
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
			{Path: `C:\Users\me\AppData\Local\Temp\foal-a`, Bytes: 10, Rule: id, Action: string(clean.PlannedActionMoveToRecycleBin)},
			{Path: `C:\Users\me\AppData\Local\Temp\foal-b`, Bytes: 20, Rule: id, Action: string(clean.PlannedActionMoveToRecycleBin)},
		},
		Totals: clean.Totals{DeletedCount: 2, AffectedBytes: 30, RecycleBinMovedBytes: 30},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) != 1 || outcomes[0].State != clean.CategoryExecutionCleaned {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if outcomes[0].AffectedBytes != 30 || outcomes[0].DeletedCount != 2 {
		t.Fatalf("bytes/count = %#v", outcomes[0])
	}
	if outcomes[0].RecycleBinMovedBytes != 30 || outcomes[0].PermanentlyDeletedBytes != 0 {
		t.Fatalf("split bytes = %#v", outcomes[0])
	}
	assertOutcomePathFree(t, outcomes, `C:\Users\me`)
}

func TestProjectCategoryExecutionOutcomesSplitsMixedActions(t *testing.T) {
	recycleID := clean.DefaultCategoryFoalOwnedTempSandboxes
	permanentID := clean.DevCacheCategoryGo
	result := clean.Result{
		Status: "ok",
		Mode:   "execute",
		Deleted: []clean.DeletedItem{
			{Path: `C:\Temp\a`, Bytes: 5, Rule: recycleID, Action: string(clean.PlannedActionMoveToRecycleBin)},
			{Path: `C:\Cache\b`, Bytes: 7, Rule: permanentID, Action: string(clean.PlannedActionDeletePermanently)},
		},
		Totals: clean.Totals{
			DeletedCount:            2,
			RecycleBinMovedBytes:    5,
			PermanentlyDeletedBytes: 7,
			AffectedBytes:           12,
		},
	}
	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{recycleID, permanentID}, result)
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	byID := map[string]clean.CategoryExecutionOutcome{}
	for _, outcome := range outcomes {
		byID[outcome.Identifier] = outcome
	}
	if byID[recycleID].RecycleBinMovedBytes != 5 || byID[recycleID].PermanentlyDeletedBytes != 0 || byID[recycleID].AffectedBytes != 5 {
		t.Fatalf("recycle outcome = %#v", byID[recycleID])
	}
	if byID[permanentID].PermanentlyDeletedBytes != 7 || byID[permanentID].RecycleBinMovedBytes != 0 || byID[permanentID].AffectedBytes != 7 {
		t.Fatalf("permanent outcome = %#v", byID[permanentID])
	}
	if clean.SumExecutionRecycleBinMovedBytes(outcomes) != 5 {
		t.Fatalf("sum recycle = %d", clean.SumExecutionRecycleBinMovedBytes(outcomes))
	}
	if clean.SumExecutionPermanentlyDeletedBytes(outcomes) != 7 {
		t.Fatalf("sum permanent = %d", clean.SumExecutionPermanentlyDeletedBytes(outcomes))
	}
	if clean.SumExecutionAffectedBytes(outcomes) != 12 {
		t.Fatalf("sum affected = %d", clean.SumExecutionAffectedBytes(outcomes))
	}
	assertOutcomePathFree(t, outcomes, `C:\Temp`, `C:\Cache`)
}

func TestResultHasPermanentPartialRisk(t *testing.T) {
	if clean.ResultHasPermanentPartialRisk(clean.Result{Status: "ok"}) {
		t.Fatal("empty result must not claim partial risk")
	}
	failed := clean.Result{
		Failed: []clean.FailedItem{{
			Path:          `C:\Cache\x`,
			Bytes:         4,
			Rule:          "go-cache",
			PlannedAction: string(clean.PlannedActionDeletePermanently),
			Action:        string(clean.PlannedActionDeletePermanently),
			Reason:        clean.StructuredIssue{Code: "permanent_delete_failed", Message: `may already be permanently deleted under C:\Cache\x`},
		}},
	}
	if !clean.ResultHasPermanentPartialRisk(failed) {
		t.Fatal("permanent_delete_failed must signal partial risk")
	}
	canceled := clean.Result{
		Skipped: []clean.SkippedItem{{
			Path:          `C:\Cache\y`,
			Rule:          "go-cache",
			PlannedAction: string(clean.PlannedActionDeletePermanently),
			Reason: clean.StructuredIssue{
				Code:    "context_canceled",
				Message: "context canceled; some content may already be permanently deleted",
			},
		}},
	}
	if !clean.ResultHasPermanentPartialRisk(canceled) {
		t.Fatal("permanent cancel with partial risk must signal")
	}
	// Untouched cancel without partial-risk wording is not a partial mutation warning.
	untouched := clean.Result{
		Skipped: []clean.SkippedItem{{
			Path:          `C:\Cache\z`,
			Rule:          "go-cache",
			PlannedAction: string(clean.PlannedActionDeletePermanently),
			Reason:        clean.StructuredIssue{Code: "context_canceled", Message: "context canceled"},
		}},
	}
	if clean.ResultHasPermanentPartialRisk(untouched) {
		t.Fatal("untouched cancel must not claim partial risk")
	}
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

func TestProjectProvisionalCategoryOutcomeMatchesFinalMapping(t *testing.T) {
	id := clean.DefaultCategoryFoalOwnedTempSandboxes
	result := clean.Result{
		Status: "ok",
		Deleted: []clean.DeletedItem{{
			Path: `C:\Temp\a`, Bytes: 5, Rule: id, Action: string(clean.PlannedActionMoveToRecycleBin),
		}},
		Skipped: []clean.SkippedItem{{
			Path: `C:\Temp\b`, Bytes: 1, Rule: id,
			Reason: clean.StructuredIssue{Code: "protected_path", Message: "protected", Recoverable: true, Rule: id},
		}},
	}
	provisional := clean.ProjectProvisionalCategoryOutcome(id, result)
	final := clean.ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(final) != 1 {
		t.Fatalf("final = %#v", final)
	}
	if provisional.State != final[0].State || provisional.State != clean.CategoryExecutionPartial {
		t.Fatalf("provisional=%#v final=%#v", provisional, final[0])
	}
	if provisional.AffectedBytes != 5 || provisional.DeletedCount != 1 || provisional.SkippedCount != 1 {
		t.Fatalf("metrics = %#v", provisional)
	}
	if clean.HasCategoryCompletion(clean.ExecutionProgress{}) {
		t.Fatal("empty progress must not report completion")
	}
	if !clean.HasCategoryCompletion(clean.ExecutionProgress{
		CompletedCategory: id,
		CompletedState:    clean.CategoryExecutionCleaned,
	}) {
		t.Fatal("terminal completion should report")
	}
	if clean.HasCategoryCompletion(clean.ExecutionProgress{
		CompletedCategory: id,
		CompletedState:    clean.CategoryExecutionCleaning,
	}) {
		t.Fatal("in-progress state must not count as completion")
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
