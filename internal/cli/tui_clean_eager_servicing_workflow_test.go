package cli

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Windows servicing Clean TUI workflow contract tests (issue #308, ADR 0029).
//
// These exercise the category-first servicing row state machine and its handoff
// to shared Clean. Analysis and execution are injected through the
// runServicingAnalysisFn and runExactCleanSelection seams so no test launches
// UAC, DISM, or the elevated helper. The TUI owns no DISM invocation, helper
// protocol, elevation, cancellation enforcement, or path-safety logic; it only
// requests analysis, gates selection, discloses the action, freezes the exact
// selection, and renders path-free servicing outcomes.

func newServicingWorkflowModel(t *testing.T) *eagerCleanModel {
	t.Helper()
	standard := clean.CleanupCategorySummary{
		Identifier:               clean.OpportunityCategoryUserTemp,
		Label:                    "User temp",
		ReportCategory:           clean.ReportCategoryUserEssentials,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
	}
	winsxs := clean.CleanupCategorySummary{
		Identifier:               clean.CategoryWinSxSComponentStore,
		Label:                    "Windows component store",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionInvokeWindowsServicing,
		SelectionPolicy:          clean.CategorySelectionPolicyExactOnly,
	}
	model := newEagerCleanModelFromSummaries([]clean.CleanupCategorySummary{standard, winsxs}, 100, 40)
	model.generation = 1
	model.finished = true
	// Standard row terminates empty so confirmation gating depends only on the
	// servicing selection under test.
	for i := range model.rows {
		if !model.rows[i].Servicing {
			model.rows[i].State = clean.CategoryPreviewEmpty
			model.rows[i].Selected = false
		}
	}
	return &model
}

func servicingRowIndex(t *testing.T, model *eagerCleanModel) int {
	t.Helper()
	for i, row := range model.rows {
		if row.Servicing {
			return i
		}
	}
	t.Fatal("no servicing row in model")
	return -1
}

// runServicingAnalysis focuses the servicing row, presses the explicit `a`
// action, and applies the injected analysis outcome. It asserts the row moves to
// analyzing before the (injected) analysis resolves.
func runServicingAnalysis(t *testing.T, model *eagerCleanModel, op clean.ServicingOperation) {
	t.Helper()
	model.cursor = servicingRowIndex(t, model)
	orig := runServicingAnalysisFn
	called := 0
	runServicingAnalysisFn = func(context.Context, string) clean.ServicingOperation {
		called++
		return op
	}
	defer func() { runServicingAnalysisFn = orig }()

	nav, cmd := model.handleKey("a")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("`a` on servicing row must return an analysis command: nav=%v cmd=%v", nav, cmd)
	}
	if got := model.rows[model.cursor].ServicingState; got != clean.ServicingRowAnalyzing {
		t.Fatalf("row must be analyzing before result: %q", got)
	}
	msg := cmd()
	analyzed, ok := msg.(eagerServicingAnalyzedMsg)
	if !ok {
		t.Fatalf("analysis command msg = %T, want eagerServicingAnalyzedMsg", msg)
	}
	if called != 1 {
		t.Fatalf("analysis seam calls = %d, want 1", called)
	}
	model.applyServicingAnalyzed(analyzed)
}

func TestServicingRowStartsAnalysisRequiredUnselectableExcludedFromSelectAll(t *testing.T) {
	model := newServicingWorkflowModel(t)
	idx := servicingRowIndex(t, model)
	row := model.rows[idx]
	if !row.Servicing || row.ServicingState != clean.ServicingRowAnalysisRequired {
		t.Fatalf("servicing row = servicing:%v state:%q, want analysis_required", row.Servicing, row.ServicingState)
	}
	if row.Selected {
		t.Fatal("servicing row must start unselected")
	}
	if model.rowSelectable(row) {
		t.Fatal("analysis_required row must not be selectable")
	}

	// Space cannot select an unanalyzed servicing row.
	model.cursor = idx
	model.toggleFocusedSelection()
	if model.rows[idx].Selected {
		t.Fatal("space must not select an analysis_required servicing row")
	}

	// Select All excludes it while still selecting the standard opt-in.
	model.selectAllSelectable()
	if model.rows[idx].Selected {
		t.Fatal("Select All must exclude the servicing row")
	}
}

func TestServicingSelectAllKeyAnalyzesFocusedRowNotSelectAll(t *testing.T) {
	model := newServicingWorkflowModel(t)
	// On a non-servicing row, `a` selects all selectable non-servicing categories.
	model.rows[0].State = clean.CategoryPreviewComplete
	model.rows[0].CandidateCount = 1
	model.rows[0].Bytes = 4
	model.cursor = 0
	nav, cmd := model.handleKey("a")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("`a` on standard row must select all (no cmd): nav=%v cmd=%v", nav, cmd)
	}
	if !model.rows[0].Selected {
		t.Fatal("standard row should be selected by Select All")
	}
	if model.rows[servicingRowIndex(t, model)].Selected {
		t.Fatal("servicing row must never be selected by Select All")
	}
}

func TestServicingAnalysisReadyBecomesSelectable(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 7,
		CleanupRecommended:  true,
	})
	idx := servicingRowIndex(t, model)
	row := model.rows[idx]
	if row.ServicingState != clean.ServicingRowReady {
		t.Fatalf("state = %q, want ready", row.ServicingState)
	}
	if row.ServicingReclaimablePackages != 7 {
		t.Fatalf("reclaimable packages = %d, want 7", row.ServicingReclaimablePackages)
	}
	if row.Selected {
		t.Fatal("ready row must not auto-select; selection stays explicit")
	}
	if !model.rowSelectable(row) {
		t.Fatal("ready row must be selectable")
	}
	// Explicit space selects it.
	model.cursor = idx
	model.toggleFocusedSelection()
	if !model.rows[idx].Selected {
		t.Fatal("space must select a ready servicing row")
	}
	content := model.content()
	if !strings.Contains(content, "7 reclaimable package(s)") {
		t.Fatalf("preview must disclose package count:\n%s", content)
	}
}

func TestServicingAnalysisNoWorkNotSelectable(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{Outcome: clean.ServicingOutcomeNoWork})
	idx := servicingRowIndex(t, model)
	if model.rows[idx].ServicingState != clean.ServicingRowNoWork {
		t.Fatalf("state = %q, want no_work", model.rows[idx].ServicingState)
	}
	if model.rowSelectable(model.rows[idx]) {
		t.Fatal("no_work row must not be selectable")
	}
	model.cursor = idx
	model.toggleFocusedSelection()
	if model.rows[idx].Selected {
		t.Fatal("space must not select a no_work row")
	}
}

func TestServicingAnalysisUACDenialSkippedRetryable(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome: clean.ServicingOutcomeSkipped,
		Reason:  clean.ServicingReasonElevationDenied,
	})
	idx := servicingRowIndex(t, model)
	if model.rows[idx].ServicingState != clean.ServicingRowSkipped {
		t.Fatalf("state = %q, want skipped", model.rows[idx].ServicingState)
	}
	if model.rowSelectable(model.rows[idx]) {
		t.Fatal("skipped row must not be selectable")
	}
	// Focused detail discloses the stable, path-free reason and retry hint.
	model.cursor = idx
	content := model.content()
	if !strings.Contains(content, "administrator consent was declined") {
		t.Fatalf("denial reason must show:\n%s", content)
	}
	if !strings.Contains(content, "press a to analyze again") {
		t.Fatalf("retry hint must show:\n%s", content)
	}
	// Retry only through another explicit analysis; a fresh ready result recovers.
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 3,
		CleanupRecommended:  true,
	})
	if model.rows[idx].ServicingState != clean.ServicingRowReady {
		t.Fatalf("after retry state = %q, want ready", model.rows[idx].ServicingState)
	}
}

func TestServicingAnalysisFailureRetryable(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome: clean.ServicingOutcomeFailed,
		Reason:  clean.ServicingReasonAnalysisOutputInvalid,
	})
	idx := servicingRowIndex(t, model)
	if model.rows[idx].ServicingState != clean.ServicingRowFailed {
		t.Fatalf("state = %q, want failed", model.rows[idx].ServicingState)
	}
	if model.rowSelectable(model.rows[idx]) {
		t.Fatal("failed row must not be selectable")
	}
	// A failed analysis is retryable only through another explicit `a` action.
	if !clean.ServicingRowAnalyzable(model.rows[idx].ServicingState) {
		t.Fatal("failed row must be analyzable for retry")
	}
	runServicingAnalysis(t, model, clean.ServicingOperation{Outcome: clean.ServicingOutcomeNoWork})
	if model.rows[idx].ServicingState != clean.ServicingRowNoWork {
		t.Fatalf("after retry state = %q, want no_work", model.rows[idx].ServicingState)
	}
}

func TestServicingReanalysisClearsStaleSelection(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 5,
		CleanupRecommended:  true,
	})
	idx := servicingRowIndex(t, model)
	model.cursor = idx
	model.toggleFocusedSelection()
	if !model.rows[idx].Selected {
		t.Fatal("ready row should be selectable and selected")
	}
	// Re-analyzing a selected ready row drops the stale selection immediately.
	orig := runServicingAnalysisFn
	runServicingAnalysisFn = func(context.Context, string) clean.ServicingOperation {
		return clean.ServicingOperation{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 5, CleanupRecommended: true}
	}
	defer func() { runServicingAnalysisFn = orig }()
	_, cmd := model.handleKey("a")
	if cmd == nil {
		t.Fatal("re-analysis must return a command")
	}
	if model.rows[idx].Selected {
		t.Fatal("re-analysis must clear a stale selection")
	}
	if model.rows[idx].ServicingState != clean.ServicingRowAnalyzing {
		t.Fatalf("re-analysis state = %q, want analyzing", model.rows[idx].ServicingState)
	}
	// A second `a` while analyzing must not launch another analysis.
	nav, cmd2 := model.handleKey("a")
	if nav != eagerPreviewNavNone || cmd2 != nil {
		t.Fatalf("`a` while analyzing must be inert: nav=%v cmd=%v", nav, cmd2)
	}
}

func selectReadyServicing(t *testing.T, model *eagerCleanModel, packages int) int {
	t.Helper()
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: packages,
		CleanupRecommended:  true,
	})
	idx := servicingRowIndex(t, model)
	model.cursor = idx
	model.toggleFocusedSelection()
	if !model.rows[idx].Selected {
		t.Fatal("failed to select ready servicing row")
	}
	return idx
}

func TestServicingConfirmationDisclosesActionAndAuthorization(t *testing.T) {
	model := newServicingWorkflowModel(t)
	selectReadyServicing(t, model, 9)
	if !model.confirmationEnabled() {
		t.Fatal("confirmation must be enabled with a ready servicing selection")
	}
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("first enter should open confirmation: nav=%v cmd=%v phase=%v", nav, cmd, model.phase)
	}
	content := model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Windows servicing · 1 categories · 9 reclaimable package(s) · size unknown",
		confirmationServicingAuthorizationLine,
		confirmationServicingUACLine,
		confirmationServicingNoRestartLine,
		confirmationServicingNonInterruptLine,
		"--allow-servicing",
		"/NoRestart",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, content)
		}
	}
	// Servicing must never present a byte estimate for reclaimable work.
	if strings.Contains(content, "WinSxS") {
		t.Fatalf("confirmation must be path-free:\n%s", content)
	}
}

func TestServicingExecutionFreezesSelectionAndAuthorizes(t *testing.T) {
	model := newServicingWorkflowModel(t)
	selectReadyServicing(t, model, 4)

	var gotSelected []string
	var gotServicing, gotPermanent bool
	calls := 0
	orig := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent, allowServicing bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotSelected = append([]string(nil), selected...)
		gotServicing = allowServicing
		gotPermanent = allowPermanent
		return clean.Result{Status: "ok", Mode: "execute", ServicingOperations: []clean.ServicingOperation{{
			Category:      clean.CategoryWinSxSComponentStore,
			PlannedAction: clean.PlannedActionInvokeWindowsServicing,
			Capability:    clean.ServicingCapabilityExecuteComponentStoreCleanup,
			Outcome:       clean.ServicingOutcomeCompleted,
		}}}
	}
	t.Cleanup(func() { runExactCleanSelection = orig })

	// First enter opens confirmation; second enter freezes and hands off.
	model.handleKey("enter")
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter should hand off: nav=%v cmd=%v", nav, cmd)
	}
	if !model.frozenAllowServicing {
		t.Fatal("frozen servicing authorization must be set")
	}
	if model.frozenAllowPermanent {
		t.Fatal("servicing selection must not authorize permanent deletion")
	}
	if !stringSlicesEqual(model.frozenCategories, []string{clean.CategoryWinSxSComponentStore}) {
		t.Fatalf("frozen categories = %#v", model.frozenCategories)
	}
	driveExecutionToResult(t, model, cmd)
	if calls != 1 {
		t.Fatalf("execute seam calls = %d, want 1", calls)
	}
	if !gotServicing {
		t.Fatal("execution must pass servicing authorization equivalent to --allow-servicing")
	}
	if gotPermanent {
		t.Fatal("execution must not pass permanent authorization")
	}
	if !stringSlicesEqual(gotSelected, []string{clean.CategoryWinSxSComponentStore}) {
		t.Fatalf("frozen exact selection = %#v", gotSelected)
	}
	content := model.content()
	if !strings.Contains(content, "servicing completed") {
		t.Fatalf("result must render servicing completion:\n%s", content)
	}
}

// driveExecutionToResult runs the execution handoff loop to the result phase.
func driveExecutionToResult(t *testing.T, model *eagerCleanModel, cmd tea.Cmd) {
	t.Helper()
	started := exactExecutionStartedFrom(t, cmd)
	wait := model.applyExactExecutionStarted(started)
	for model.phase == eagerPhaseExecuting {
		msg := wait()
		switch m := msg.(type) {
		case eagerExactExecutionProgressMsg:
			wait = model.applyExactExecutionProgress(m)
		case eagerExactExecutedMsg:
			model.applyExactExecuted(m)
			wait = nil
		default:
			t.Fatalf("unexpected execution msg %T", msg)
		}
		if wait == nil {
			break
		}
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v, want result", model.phase)
	}
}

func TestServicingResultRestartRequiredDisclosed(t *testing.T) {
	content := servicingResultContent(t, clean.ServicingOperation{
		Category:        clean.CategoryWinSxSComponentStore,
		PlannedAction:   clean.PlannedActionInvokeWindowsServicing,
		Capability:      clean.ServicingCapabilityExecuteComponentStoreCleanup,
		Outcome:         clean.ServicingOutcomeCompleted,
		RestartRequired: true,
	})
	if !strings.Contains(content, "servicing completed") {
		t.Fatalf("restart result must still show completion:\n%s", content)
	}
	if !strings.Contains(content, "restart is required") {
		t.Fatalf("restart requirement must be disclosed:\n%s", content)
	}
}

func TestServicingResultPostStartCancellationPreservesOutcome(t *testing.T) {
	content := servicingResultContent(t, clean.ServicingOperation{
		Category:        clean.CategoryWinSxSComponentStore,
		PlannedAction:   clean.PlannedActionInvokeWindowsServicing,
		Capability:      clean.ServicingCapabilityExecuteComponentStoreCleanup,
		Outcome:         clean.ServicingOutcomeCompleted,
		CancelRequested: true,
	})
	// The actual completed outcome stands; cancellation is only a disclosure.
	if !strings.Contains(content, "servicing completed") {
		t.Fatalf("post-start cancel must preserve the actual outcome:\n%s", content)
	}
	if !strings.Contains(content, "Cancellation was requested after servicing started") {
		t.Fatalf("post-start cancellation must be disclosed:\n%s", content)
	}
	if strings.Contains(content, "· canceled") {
		t.Fatalf("post-start cancel must not render the row as canceled:\n%s", content)
	}
}

func TestServicingResultPreMutationCanceled(t *testing.T) {
	content := servicingResultContent(t, clean.ServicingOperation{
		Category:      clean.CategoryWinSxSComponentStore,
		PlannedAction: clean.PlannedActionInvokeWindowsServicing,
		Capability:    clean.ServicingCapabilityExecuteComponentStoreCleanup,
		Outcome:       clean.ServicingOutcomeCanceled,
		Reason:        clean.ServicingReasonContextCanceled,
	})
	if !strings.Contains(content, "· canceled") {
		t.Fatalf("pre-mutation cancel must render as canceled:\n%s", content)
	}
}

func TestServicingResultFailure(t *testing.T) {
	content := servicingResultContent(t, clean.ServicingOperation{
		Category:      clean.CategoryWinSxSComponentStore,
		PlannedAction: clean.PlannedActionInvokeWindowsServicing,
		Capability:    clean.ServicingCapabilityExecuteComponentStoreCleanup,
		Outcome:       clean.ServicingOutcomeFailed,
		Reason:        clean.ServicingReasonCleanupFailed,
	})
	if !strings.Contains(content, "· failed") {
		t.Fatalf("failed servicing must render as failed:\n%s", content)
	}
}

// TestServicingReadyRowDisclosesSizeUnknown proves the selection ready row
// carries the weak "size unknown" disclosure without inventing a byte figure.
func TestServicingReadyRowDisclosesSizeUnknown(t *testing.T) {
	model := newServicingWorkflowModel(t)
	runServicingAnalysis(t, model, clean.ServicingOperation{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 7,
		CleanupRecommended:  true,
	})
	content := model.content()
	if !strings.Contains(content, "reclaimable package(s) · size unknown") {
		t.Fatalf("ready row must disclose size unknown without a byte figure:\n%s", content)
	}
}

// TestServicingResultObservationShownWhenPositive proves a positive post-mutation
// observation renders an approximate free-space line plus a Mixed cleanup impact
// line on the result page.
func TestServicingResultObservationShownWhenPositive(t *testing.T) {
	observed := int64(1500)
	content := servicingResultContent(t, clean.ServicingOperation{
		Category:          clean.CategoryWinSxSComponentStore,
		PlannedAction:     clean.PlannedActionInvokeWindowsServicing,
		Capability:        clean.ServicingCapabilityExecuteComponentStoreCleanup,
		Outcome:           clean.ServicingOutcomeCompleted,
		ObservedFreeBytes: &observed,
	})
	if !strings.Contains(content, "observed free-space increase ≈") {
		t.Fatalf("positive observation must be shown:\n%s", content)
	}
	if !strings.Contains(content, "Mixed cleanup impact ≈") {
		t.Fatalf("mixed cleanup impact must be shown when observation present:\n%s", content)
	}
}

// TestServicingResultObservationHiddenWhenZeroOrAbsent proves a measured-zero or
// absent observation shows nothing (presentation treats zero as no observation).
func TestServicingResultObservationHiddenWhenZeroOrAbsent(t *testing.T) {
	zero := int64(0)
	cases := map[string]clean.ServicingOperation{
		"measured zero": {Category: clean.CategoryWinSxSComponentStore, PlannedAction: clean.PlannedActionInvokeWindowsServicing, Capability: clean.ServicingCapabilityExecuteComponentStoreCleanup, Outcome: clean.ServicingOutcomeCompleted, ObservedFreeBytes: &zero},
		"absent":        {Category: clean.CategoryWinSxSComponentStore, PlannedAction: clean.PlannedActionInvokeWindowsServicing, Capability: clean.ServicingCapabilityExecuteComponentStoreCleanup, Outcome: clean.ServicingOutcomeCompleted},
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			content := servicingResultContent(t, op)
			if strings.Contains(content, "observed free-space increase") || strings.Contains(content, "Mixed cleanup impact") {
				t.Fatalf("zero/absent observation must show nothing:\n%s", content)
			}
		})
	}
}

// TestServicingObservationLinesNotMagnitudeEligible proves the approximate
// observation and Mixed impact lines opt out of danger-aware magnitude emphasis.
func TestServicingObservationLinesNotMagnitudeEligible(t *testing.T) {
	lines := []string{
		"Windows component store: observed free-space increase ≈ 1.5 KB (approximate; not in Affected)",
		"Mixed cleanup impact ≈ 1.5 KB (approximate: Affected plus observed servicing free-space)",
	}
	for _, line := range lines {
		if isMagnitudeEligibleLine(line) {
			t.Fatalf("approximate observation line must not be magnitude-eligible: %q", line)
		}
	}
}

// servicingResultContent drives a ready+selected servicing row through the
// confirm/execute/result flow with an injected Result carrying the given
// servicing operation, then returns the rendered result-page content.
func servicingResultContent(t *testing.T, op clean.ServicingOperation) string {
	t.Helper()
	model := newServicingWorkflowModel(t)
	selectReadyServicing(t, model, 4)

	orig := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _, _ bool, _ clean.ProgressReporter) clean.Result {
		return clean.Result{Status: "ok", Mode: "execute", ServicingOperations: []clean.ServicingOperation{op}}
	}
	t.Cleanup(func() { runExactCleanSelection = orig })

	model.handleKey("enter")
	_, cmd := model.handleKey("enter")
	driveExecutionToResult(t, model, cmd)
	content := model.content()
	assertNoPath(t, content)
	return content
}
