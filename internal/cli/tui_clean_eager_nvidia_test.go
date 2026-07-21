package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// tui_clean_eager_nvidia_test.go pins the category-first Clean TUI behavior for
// the exact-selection-only, Not-proven nvidia_installer_cache row (issue #311).
// The TUI is an adapter over shared Clean: it owns no NVIDIA candidate
// resolution, status/process/service control, elevation, or deletion. Every
// behavior below is projected from the shared catalog, the shared eager
// resolution, and the shared exact executor. Tests drive the model with
// path-free observations and a stubbed shared executor so they never touch a
// real NVIDIA Downloader tree.

// nvidiaCatalogSummary returns the faithful catalog summary for the NVIDIA
// installer-cache row so tests exercise the real planned action and selection
// policy rather than a hand-built stand-in.
func nvidiaCatalogSummary(t *testing.T) clean.CleanupCategorySummary {
	t.Helper()
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryNVIDIAInstallerCache)
	if !ok {
		t.Fatal("catalog missing nvidia_installer_cache summary")
	}
	return summary
}

// nvidiaObservation projects a synthetic single-candidate NVIDIA resolution
// through the shared preview projection so the observation carries the real
// impact safety note and Complete classification (path-free). The candidate
// path is intentionally present in the resolution to prove the projection
// discards it.
func nvidiaCompleteObservation(bytes int64) clean.CategoryPreviewObservation {
	res := clean.CategoryResolution{
		Identifier:  clean.CategoryNVIDIAInstallerCache,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:          `C:\ProgramData\NVIDIA Corporation\Downloader\0123456789abcdef0123456789abcdef\driver.exe`,
			Bytes:         bytes,
			Category:      clean.CategoryNVIDIAInstallerCache,
			PlannedAction: string(clean.PlannedActionMoveToRecycleBin),
		}},
	}
	return clean.ProjectCategoryPreview(res)
}

// TestEagerCleanNVIDIAInstallerCacheInitialSelectionAndSelectAllExclusion proves
// the NVIDIA row participates in the catalog-derived eager queue as an
// exact-selection-only move_to_recycle_bin category, starts unselected, and is
// excluded from Select All while standard opt-ins are selected.
func TestEagerCleanNVIDIAInstallerCacheInitialSelectionAndSelectAllExclusion(t *testing.T) {
	id := clean.CategoryNVIDIAInstallerCache

	// Wiring: present in the eager queue with the exact catalog metadata.
	var summary clean.CleanupCategorySummary
	found := false
	for _, s := range clean.EagerPreviewQueue() {
		if s.Identifier == id {
			summary = s
			found = true
			break
		}
	}
	if !found {
		t.Fatal("eager preview queue missing nvidia_installer_cache")
	}
	if summary.SelectionPolicy != clean.CategorySelectionPolicyExactOnly {
		t.Fatalf("selection policy = %q, want exact-selection-only", summary.SelectionPolicy)
	}
	if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
		t.Fatalf("planned action = %q, want move_to_recycle_bin", summary.PlannedAction)
	}
	// Shared initial-selection policy: exact-only recycle categories never start
	// selected (no TUI-owned permanent list).
	if clean.InitiallySelectedCategory(summary) {
		t.Fatal("nvidia_installer_cache must not start selected")
	}

	model := newEagerCleanModel(120, 60)
	nvidiaIdx := -1
	standardOptInIdx := -1
	for i, row := range model.rows {
		switch {
		case row.Identifier == id:
			nvidiaIdx = i
			if row.Selected {
				t.Fatal("nvidia_installer_cache row must start unselected")
			}
		case standardOptInIdx < 0 &&
			row.Eligibility == clean.CategoryEligibilityOptIn &&
			row.SelectionPolicy != clean.CategorySelectionPolicyExactOnly &&
			row.PlannedAction == clean.PlannedActionMoveToRecycleBin:
			standardOptInIdx = i
		}
	}
	if nvidiaIdx < 0 {
		t.Fatal("model rows missing nvidia_installer_cache")
	}
	if standardOptInIdx < 0 {
		t.Fatal("expected at least one standard recycle-bin opt-in sibling")
	}

	// Select All authorizes standard selectable opt-ins but never the exact-only
	// NVIDIA row (rows are Waiting, which is otherwise selectable).
	model.selectAllSelectable()
	if model.rows[nvidiaIdx].Selected {
		t.Fatal("Select All must exclude the exact-selection-only nvidia row")
	}
	if !model.rows[standardOptInIdx].Selected {
		t.Fatal("Select All should select a standard recycle-bin opt-in sibling")
	}
}

// TestEagerCleanNVIDIAInstallerCacheUnavailableStates proves that an active or
// undetermined NVIDIA process/service state renders the row unavailable
// (non-selectable) with a stable, readable, path-free reason, and that toggling
// cannot authorize it.
func TestEagerCleanNVIDIAInstallerCacheUnavailableStates(t *testing.T) {
	model := newEagerCleanModelFromSummaries(
		[]clean.CleanupCategorySummary{nvidiaCatalogSummary(t)}, 100, 40)
	model.generation = 1

	// Busy AND unknown NVIDIA state share one stable skip reason
	// (PreviewReasonNVIDIAActivity). The resolver never guesses idle.
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: clean.CategoryNVIDIAInstallerCache,
			Label:      "NVIDIA installer cache",
			State:      clean.CategoryPreviewSkipped,
			ReasonCode: clean.PreviewReasonNVIDIAActivity,
		},
	})
	model.finished = true

	row := model.rows[0]
	if eagerRowSelectable(row) {
		t.Fatal("skipped NVIDIA row must not be selectable")
	}
	if row.Selected {
		t.Fatal("skipped NVIDIA row must not be selected")
	}

	// Space cannot authorize an unavailable row.
	model.cursor = 0
	model.toggleFocusedSelection()
	if model.rows[0].Selected {
		t.Fatal("toggling an unavailable NVIDIA row must be a no-op")
	}

	content := model.content()
	assertNoPath(t, content)
	if !strings.Contains(content, "· skipped") {
		t.Fatalf("row should render skipped marker:\n%s", content)
	}
	if !strings.Contains(content, "NVIDIA activity detected or state unknown") {
		t.Fatalf("focused detail should show readable stable reason:\n%s", content)
	}
	// An unavailable-only queue offers no work: confirmation stays disabled.
	if model.confirmationEnabled() {
		t.Fatal("confirmation must be disabled when the only category is unavailable")
	}
}

// TestEagerCleanNVIDIAInstallerCacheImpactDisclosure proves the shared impact
// note (offline install / rollback convenience lost, re-download required) is
// surfaced when browsing the selectable row and again in confirmation, and that
// no permanent deletion action or authorization is presented for this category.
func TestEagerCleanNVIDIAInstallerCacheImpactDisclosure(t *testing.T) {
	model := newEagerCleanModelFromSummaries(
		[]clean.CleanupCategorySummary{nvidiaCatalogSummary(t)}, 100, 40)
	model.generation = 1

	obs := nvidiaCompleteObservation(512 << 20)
	if obs.State != clean.CategoryPreviewComplete {
		t.Fatalf("projected state = %q, want complete", obs.State)
	}
	if obs.SafetyNote == "" {
		t.Fatal("shared projection must attach the NVIDIA impact note for a present candidate")
	}
	// The impact vocabulary must disclose re-download and lost offline/rollback
	// convenience without promising rollback is unaffected.
	for _, want := range []string{"Recycle Bin", "download", "rollback", "offline"} {
		if !strings.Contains(obs.SafetyNote, want) {
			t.Fatalf("impact note missing %q: %q", want, obs.SafetyNote)
		}
	}
	if strings.Contains(strings.ToLower(obs.SafetyNote), "permanent deletion") &&
		!strings.Contains(obs.SafetyNote, "not permanent deletion") {
		t.Fatalf("impact note must not imply permanent deletion: %q", obs.SafetyNote)
	}

	model.applyObservation(eagerCategoryObservationMsg{generation: 1, observation: obs})
	model.finished = true

	// Exact, individual selection of the row.
	model.cursor = 0
	model.toggleFocusedSelection()
	if !model.rows[0].Selected {
		t.Fatal("selectable NVIDIA row must toggle on")
	}

	browse := model.content()
	assertNoPath(t, browse)
	if !strings.Contains(browse, "Safety: ") || !strings.Contains(browse, "download") {
		t.Fatalf("browse focused detail should disclose the impact note:\n%s", browse)
	}

	// Confirmation groups the exact selection as Recycle Bin only, repeats the
	// impact note, and never shows a permanent-deletion group or warning.
	if _, cmd := model.handleKey("enter"); cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("enter should open confirmation: phase=%v cmd=%v", model.phase, cmd)
	}
	confirm := model.content()
	assertNoPath(t, confirm)
	for _, want := range []string{"Recycle Bin", "Impact: ", "download"} {
		if !strings.Contains(confirm, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, confirm)
		}
	}
	if strings.Contains(confirm, "Permanent deletion") {
		t.Fatalf("NVIDIA-only confirmation must not present permanent deletion:\n%s", confirm)
	}
	if model.selectionIncludesPermanent() {
		t.Fatal("NVIDIA move_to_recycle_bin selection must not disclose permanent")
	}
	if !strings.Contains(confirm, "Enter: execute") {
		t.Fatalf("recycle-only confirmation should use execute wording:\n%s", confirm)
	}
}

// TestEagerCleanNVIDIAInstallerCacheConfirmationFreezesExactRecycleHandoff proves
// the second Enter freezes exactly the NVIDIA identifier, requests no permanent
// authorization, delegates to the shared exact executor once, and preserves the
// item-level Recycle Bin Result semantics on the result page. It also proves the
// selection never implicitly authorizes a sibling category.
func TestEagerCleanNVIDIAInstallerCacheConfirmationFreezesExactRecycleHandoff(t *testing.T) {
	id := clean.CategoryNVIDIAInstallerCache

	var gotSelected []string
	var gotAllowPermanent bool
	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotSelected = append([]string(nil), selected...)
		gotAllowPermanent = allowPermanent
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{{
				Path:    `C:\ProgramData\NVIDIA Corporation\Downloader\0123456789abcdef0123456789abcdef\driver.exe`,
				Bytes:   512 << 20,
				Rule:    id,
				Action:  string(clean.PlannedActionMoveToRecycleBin),
				IsOptIn: true,
			}},
			Totals: clean.Totals{
				DeletedCount:         1,
				RecycleBinMovedBytes: 512 << 20,
				AffectedBytes:        512 << 20,
			},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	// A sibling recycle opt-in is present to prove NVIDIA selection never
	// implicitly selects it.
	sibling := clean.CleanupCategorySummary{
		Identifier:               "user_temp",
		Label:                    "User temp",
		ReportCategory:           clean.ReportCategoryUserEssentials,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
	}
	model := newEagerCleanModelFromSummaries(
		[]clean.CleanupCategorySummary{nvidiaCatalogSummary(t), sibling}, 100, 40)
	model.generation = 1

	// NVIDIA measurable/complete; sibling empty so the exact selection is NVIDIA only.
	model.applyObservation(eagerCategoryObservationMsg{generation: 1, observation: nvidiaCompleteObservation(512 << 20)})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: sibling.Identifier,
			Label:      sibling.Label,
			State:      clean.CategoryPreviewEmpty,
		},
	})
	model.finished = true

	// Select NVIDIA only.
	model.cursor = 0
	model.toggleFocusedSelection()
	if !model.rows[0].Selected {
		t.Fatal("NVIDIA row must be selectable")
	}
	if model.rows[1].Selected {
		t.Fatal("selecting NVIDIA must not implicitly select a sibling category")
	}
	if model.selectedCount() != 1 {
		t.Fatalf("selectedCount = %d, want 1", model.selectedCount())
	}
	if !model.confirmationEnabled() {
		t.Fatal("confirmation should be enabled for a non-empty terminal selection")
	}

	// Toggling NVIDIA never returns a scan command (no rescan).
	if _, cmd := model.handleKey("enter"); cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("first enter opens confirmation: phase=%v cmd=%v", model.phase, cmd)
	}
	if calls != 0 {
		t.Fatal("confirmation must not execute or write History")
	}

	// Second Enter freezes exactly the NVIDIA identifier with no permanent auth.
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter: nav=%v cmd=%v", nav, cmd)
	}
	if model.frozenAllowPermanent {
		t.Fatal("NVIDIA-only selection must not authorize permanent deletion")
	}
	if len(model.frozenCategories) != 1 || model.frozenCategories[0] != id {
		t.Fatalf("frozen = %#v, want [%q]", model.frozenCategories, id)
	}

	// Drive the shared executor handoff to completion.
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
			t.Fatalf("unexpected %T", msg)
		}
		if wait == nil {
			break
		}
	}
	if calls != 1 {
		t.Fatalf("shared executor calls = %d, want 1", calls)
	}
	if gotAllowPermanent {
		t.Fatal("handoff must not pass permanent authorization for the NVIDIA category")
	}
	if len(gotSelected) != 1 || gotSelected[0] != id {
		t.Fatalf("handoff selected = %v, want [%q]", gotSelected, id)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v, want result", model.phase)
	}
	result := model.content()
	assertNoPath(t, result)
	if !strings.Contains(result, "Recycle Bin moved") {
		t.Fatalf("result should preserve Recycle Bin action semantics:\n%s", result)
	}
	if !strings.Contains(result, "cleaned") {
		t.Fatalf("result should show the item-level cleaned outcome:\n%s", result)
	}
}

// TestEagerCleanNVIDIAInstallerCacheExecutionFailureFlowsToResult proves a
// shared-executor failure is delegated (never invented by the TUI) and projected
// to the result page path-free, with the NVIDIA outcome marked failed.
func TestEagerCleanNVIDIAInstallerCacheExecutionFailureFlowsToResult(t *testing.T) {
	id := clean.CategoryNVIDIAInstallerCache

	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ clean.ProgressReporter) clean.Result {
		calls++
		return clean.Result{
			Status: "error",
			Mode:   "execute",
			Failed: []clean.FailedItem{{
				Path:   `C:\ProgramData\NVIDIA Corporation\Downloader\0123456789abcdef0123456789abcdef\driver.exe`,
				Rule:   id,
				Reason: clean.StructuredIssue{Code: "recycle_bin_move_failed", Message: "move failed", Recoverable: true},
			}},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModelFromSummaries(
		[]clean.CleanupCategorySummary{nvidiaCatalogSummary(t)}, 100, 40)
	model.generation = 1
	model.applyObservation(eagerCategoryObservationMsg{generation: 1, observation: nvidiaCompleteObservation(200 << 20)})
	model.finished = true
	model.cursor = 0
	model.toggleFocusedSelection()

	if _, cmd := model.handleKey("enter"); cmd != nil {
		t.Fatalf("first enter should not execute: cmd=%v", cmd)
	}
	_, cmd := model.handleKey("enter")
	if cmd == nil {
		t.Fatal("second enter should start execution")
	}
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
			t.Fatalf("unexpected %T", msg)
		}
		if wait == nil {
			break
		}
	}
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v, want result", model.phase)
	}
	result := model.content()
	assertNoPath(t, result)
	// The NVIDIA outcome is projected from the shared failed Result, not invented.
	if !strings.Contains(result, "NVIDIA installer cache") {
		t.Fatalf("result should name the NVIDIA category:\n%s", result)
	}
	if !strings.Contains(result, "failed") {
		t.Fatalf("result should reflect the failed shared outcome:\n%s", result)
	}
}

// TestEagerCleanNVIDIAInstallerCacheCancellation proves cooperative cancellation
// before execution keeps the category safe: Ctrl+C from confirmation interrupts
// without freezing or executing, and Ctrl+C during preview interrupts too.
func TestEagerCleanNVIDIAInstallerCacheCancellation(t *testing.T) {
	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(context.Context, []string, bool, clean.ProgressReporter) clean.Result {
		calls++
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	newModel := func(t *testing.T) eagerCleanModel {
		m := newEagerCleanModelFromSummaries(
			[]clean.CleanupCategorySummary{nvidiaCatalogSummary(t)}, 100, 40)
		m.generation = 1
		m.applyObservation(eagerCategoryObservationMsg{generation: 1, observation: nvidiaCompleteObservation(64 << 20)})
		m.finished = true
		m.cursor = 0
		m.toggleFocusedSelection()
		return m
	}

	// Preview cancellation.
	preview := newModel(t)
	nav, _ := preview.handleKey("ctrl+c")
	if nav != eagerPreviewNavInterrupt || !preview.canceled {
		t.Fatalf("preview ctrl+c: nav=%v canceled=%v", nav, preview.canceled)
	}

	// Confirmation cancellation: no freeze, no execution.
	confirm := newModel(t)
	if _, cmd := confirm.handleKey("enter"); cmd != nil || confirm.phase != eagerPhaseConfirmation {
		t.Fatalf("enter should open confirmation: phase=%v cmd=%v", confirm.phase, cmd)
	}
	nav, cmd := confirm.handleKey("ctrl+c")
	if nav != eagerPreviewNavInterrupt || !confirm.canceled {
		t.Fatalf("confirmation ctrl+c: nav=%v canceled=%v", nav, confirm.canceled)
	}
	if cmd != nil {
		t.Fatalf("confirmation ctrl+c must not start execution: cmd=%v", cmd)
	}
	if confirm.executionStarted || len(confirm.frozenCategories) != 0 {
		t.Fatalf("cancellation must not freeze or start execution: started=%v frozen=%#v",
			confirm.executionStarted, confirm.frozenCategories)
	}
	if calls != 0 {
		t.Fatalf("shared executor must not be called on cancellation: calls=%d", calls)
	}
}
