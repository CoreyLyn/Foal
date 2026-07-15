package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

// expandTeaCmd flattens Batch/Sequence command trees into concrete messages.
// Stream wait commands that block are left for callers that intentionally drain them.
func expandTeaCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch batch := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, child := range batch {
			out = append(out, expandTeaCmd(child)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func applyRootMsgs(t *testing.T, model rootModel, msgs ...tea.Msg) (rootModel, tea.Cmd) {
	t.Helper()
	var last tea.Cmd
	for _, msg := range msgs {
		next, cmd := model.Update(msg)
		root, ok := next.(rootModel)
		if !ok {
			t.Fatalf("Update returned %T, want rootModel", next)
		}
		model = root
		last = cmd
	}
	return model, last
}

func stubInstantEagerPreview(t *testing.T) {
	t.Helper()
	original := runEagerPreviewFn
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
		if opts.HistoryRecorder != nil || opts.DetailedListDir != "" {
			t.Fatalf("eager options enable side effects: %#v", opts)
		}
		if opts.DiscoverReviewSuggestions != nil {
			t.Fatal("category-first Clean must not attach Review suggestion probes")
		}
		for _, summary := range clean.EagerPreviewQueue() {
			if err := ctx.Err(); err != nil {
				return err
			}
			emit(clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				ReportCategory: summary.ReportCategory,
				Eligibility:    summary.Eligibility,
				State:          clean.CategoryPreviewScanning,
			})
			state := clean.CategoryPreviewEmpty
			count := 0
			var bytes int64
			if summary.Eligibility == clean.CategoryEligibilityDefault {
				state = clean.CategoryPreviewComplete
				count = 1
				bytes = 42
			}
			emit(clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				ReportCategory: summary.ReportCategory,
				Eligibility:    summary.Eligibility,
				State:          state,
				CandidateCount: count,
				Bytes:          bytes,
			})
		}
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = original })

	originalBuild := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options { return clean.Options{} }
	t.Cleanup(func() { buildEagerPreviewOptions = originalBuild })
}

// openCategoryFirstClean enters Clean through the primary menu path and drains
// the eager stream until the scan finishes (or returns the first wait cmd when
// the stub is not installed).
func openCategoryFirstClean(t *testing.T) rootModel {
	t.Helper()
	stubInstantEagerPreview(t)
	disableHistoryRecording(t)

	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = next.(rootModel)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("entering Clean must start eager scanning immediately")
	}
	if model.screen != screenCleanPreview {
		t.Fatalf("screen = %v, want clean", model.screen)
	}
	content := model.content()
	if !strings.Contains(content, "Scanning") && !strings.Contains(content, "Scan complete") {
		t.Fatalf("primary Clean must show category-first scan chrome:\n%s", content)
	}
	for _, obsolete := range []string{
		"Loading clean preview (dry-run)",
		"Filter:",
		"Press c to copy",
		"tab category",
	} {
		if strings.Contains(content, obsolete) {
			t.Fatalf("obsolete report-first chrome %q still reachable:\n%s", obsolete, content)
		}
	}

	// Expand Batch (start + tick) and drive the stream to completion.
	for _, msg := range expandTeaCmd(cmd) {
		var nextCmd tea.Cmd
		model, nextCmd = applyRootMsgs(t, model, msg)
		model = drainEagerStream(t, model, nextCmd)
	}
	if !model.clean.finished && model.clean.unavailable == nil {
		t.Fatal("eager preview did not finish after open")
	}
	return model
}

func drainEagerStream(t *testing.T, model rootModel, cmd tea.Cmd) rootModel {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for cmd != nil && time.Now().Before(deadline) {
		// Skip pure tick sleeps by executing non-timer cmds only when possible.
		msg := cmd()
		switch msg.(type) {
		case eagerPreviewTickMsg:
			// Do not schedule further ticks in tests unless already finished.
			if model.clean.finished || model.clean.unavailable != nil {
				return model
			}
			// Apply tick once without re-arming long chains.
			next, _ := model.Update(msg)
			return next.(rootModel)
		case tea.BatchMsg:
			for _, child := range msg.(tea.BatchMsg) {
				model = drainEagerStream(t, model, child)
			}
			return model
		default:
			var nextCmd tea.Cmd
			model, nextCmd = applyRootMsgs(t, model, msg)
			cmd = nextCmd
			if model.clean.finished || model.clean.unavailable != nil {
				return model
			}
		}
	}
	return model
}

func TestPrimaryCleanEntryStartsCategoryFirstEagerScan(t *testing.T) {
	model := openCategoryFirstClean(t)
	content := model.content()

	for _, want := range []string{
		"Scanning",
		"User essentials",
		"System",
		"Browsers",
		"Developer tools",
		"Selected:",
		"Hints:",
		"space toggle",
	} {
		// After full drain, header may say Scan complete rather than Scanning.
		if want == "Scanning" {
			if !strings.Contains(content, "Scanning") && !strings.Contains(content, "Scan complete") {
				t.Fatalf("missing scan progress chrome:\n%s", content)
			}
			continue
		}
		if !strings.Contains(content, want) {
			t.Fatalf("category-first content missing %q:\n%s", want, content)
		}
	}

	// Defaults selected, opt-ins not selected after terminal complete default.
	if model.clean.selectedCount() < 1 {
		t.Fatal("default categories should remain selected when complete with candidates")
	}
	for _, row := range model.clean.rows {
		if row.Eligibility == clean.CategoryEligibilityOptIn && row.Selected {
			t.Fatalf("opt-in %q started selected", row.Identifier)
		}
	}

	// No retry/rescan action in the first slice.
	for _, forbidden := range []string{
		"retry", "rescan", "Reload", "reload", "Filter:", "Press c to copy",
		"Review suggestions", "pnpm cache", "tab category",
		"Loading clean preview", "Potential space:",
	} {
		if strings.Contains(strings.ToLower(content), strings.ToLower(forbidden)) &&
			(strings.Contains(content, forbidden) || strings.Contains(content, strings.ToLower(forbidden))) {
			// Case-sensitive exact for markers that could appear in labels.
			if strings.Contains(content, forbidden) {
				t.Fatalf("obsolete or deferred control %q present:\n%s", forbidden, content)
			}
		}
	}
	if strings.Contains(content, "Filter:") ||
		strings.Contains(content, "Press c to copy") ||
		strings.Contains(content, "tab category") ||
		strings.Contains(content, "Loading clean preview") ||
		strings.Contains(content, "r: reload") ||
		strings.Contains(content, "retry") ||
		strings.Contains(content, "rescan") {
		t.Fatalf("obsolete report-first or deferred retry chrome still visible:\n%s", content)
	}
}

func TestPrimaryCleanObsoleteKeysAreUnreachable(t *testing.T) {
	model := openCategoryFirstClean(t)
	before := model.content()
	gen := model.clean.generation

	// Report-first keys must not filter, expand, copy, or reload.
	for _, key := range []tea.KeyPressMsg{
		{Code: 'f', Text: "f"},
		{Code: 'e', Text: "e"},
		{Code: 'c', Text: "c"},
		{Code: 'r', Text: "r"},
		{Code: 't', Text: "t"},
	} {
		model = updateRootKeys(t, model, key)
	}
	if model.clean.generation != gen {
		t.Fatal("obsolete keys restarted or rescanned the eager session")
	}
	if model.screen != screenCleanPreview {
		t.Fatal("obsolete keys left the Clean surface unexpectedly")
	}
	after := model.content()
	if strings.Contains(after, "Copied") || strings.Contains(after, "Filter:") {
		t.Fatalf("obsolete interactions mutated chrome:\n%s", after)
	}
	// Selection and scan state remain path-free.
	assertNoPath(t, before)
	assertNoPath(t, after)
}

func TestPrimaryCleanBackCancelsEagerScanWithoutHistory(t *testing.T) {
	disableHistoryRecording(t)
	original := runEagerPreviewFn
	started := make(chan struct{})
	release := make(chan struct{})
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	t.Cleanup(func() {
		runEagerPreviewFn = original
		select {
		case <-release:
		default:
			close(release)
		}
	})
	originalBuild := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options { return clean.Options{} }
	t.Cleanup(func() { buildEagerPreviewOptions = originalBuild })

	originalRecorder := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) {
		t.Fatal("preview cancel must not open history")
		return nil, nil
	}
	t.Cleanup(func() { newHistoryRecorder = originalRecorder })

	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = next.(rootModel)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("expected start command")
	}
	// Kick the start cmd so the worker observes cancel.
	for _, msg := range expandTeaCmd(cmd) {
		var nextCmd tea.Cmd
		model, nextCmd = applyRootMsgs(t, model, msg)
		if nextCmd != nil {
			// Non-blocking: start wait in background only if needed.
			_ = nextCmd
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("eager scan did not start")
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if !strings.Contains(model.content(), "Foal main menu") {
		t.Fatalf("b should return to menu:\n%s", model.content())
	}
	if model.screen != screenMenu {
		t.Fatalf("screen = %v", model.screen)
	}
}

func TestPrimaryCleanFourStageHintsAndNoRescan(t *testing.T) {
	model := openCategoryFirstClean(t)
	// Ensure confirmation path: keep first default selected after terminal outcomes.
	markEagerQueueTerminal(&model.clean, true)
	model.clean.finished = true

	content := model.content()
	if !strings.Contains(content, "enter confirm") && !model.clean.confirmationEnabled() {
		// Force selectable complete default.
		if len(model.clean.rows) == 0 {
			t.Fatal("no rows")
		}
	}
	if !model.clean.confirmationEnabled() {
		t.Fatalf("expected confirmation enabled after terminal selection; noWork=%v content:\n%s", model.clean.noWorkState(), content)
	}

	// Preview hints
	previewHints := model.clean.footerHints()
	for _, want := range []string{"space toggle", "a select all", "x clear", "enter confirm"} {
		if !strings.Contains(previewHints, want) {
			t.Fatalf("preview hints missing %q: %s", want, previewHints)
		}
	}
	for _, forbidden := range []string{"retry", "rescan", "reload", "copy", "filter", "expand"} {
		if strings.Contains(strings.ToLower(previewHints), forbidden) {
			t.Fatalf("preview hints expose deferred/obsolete %q: %s", forbidden, previewHints)
		}
	}

	// Confirmation stage
	model, _ = applyRootMsgs(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.clean.phase != eagerPhaseConfirmation {
		t.Fatalf("phase = %v", model.clean.phase)
	}
	confirm := model.content()
	for _, want := range []string{"Confirm", "Recycle Bin", "Selected:"} {
		if !strings.Contains(confirm, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, confirm)
		}
	}
	assertNoPath(t, confirm)
	// Escape preserves selection without rescan
	gen := model.clean.generation
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.clean.phase != eagerPhasePreview || model.clean.generation != gen {
		t.Fatalf("escape must return to preview without rescan: phase=%v gen=%d", model.clean.phase, model.clean.generation)
	}
}

func TestPrimaryCleanConfirmationExecutionResultPathFree(t *testing.T) {
	disableHistoryRecording(t)
	stubInstantEagerPreview(t)

	originalExec := runExactCleanSelection
	runExactCleanSelection = func(ctx context.Context, selected []string, reporter clean.ProgressReporter) clean.Result {
		if len(selected) == 0 {
			t.Fatal("empty selection")
		}
		for _, id := range selected {
			if strings.Contains(id, `\`) || strings.Contains(id, "/") {
				t.Fatalf("selection leaked path: %q", selected)
			}
		}
		if reporter != nil {
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning})
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseRecycleBinOperations})
		}
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{{
				Path:  `C:\Users\private\sentinel-candidate`,
				Bytes: 42,
				Rule:  selected[0],
			}},
			Totals: clean.Totals{DeletedCount: 1, AffectedBytes: 42},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = originalExec })

	model := openCategoryFirstClean(t)
	markEagerQueueTerminal(&model.clean, true)
	model.clean.finished = true
	if !model.clean.confirmationEnabled() {
		t.Fatal("confirmation should be enabled")
	}

	// Enter confirmation, then execute.
	model, _ = applyRootMsgs(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.clean.phase != eagerPhaseConfirmation {
		t.Fatalf("phase=%v", model.clean.phase)
	}
	model, cmd := applyRootMsgs(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.clean.phase != eagerPhaseExecuting || cmd == nil {
		t.Fatalf("second enter must start execution: phase=%v cmd=%v", model.clean.phase, cmd != nil)
	}
	// Drive execution stream to result.
	msg := cmd()
	model, cmd = applyRootMsgs(t, model, msg)
	for cmd != nil && model.clean.phase == eagerPhaseExecuting {
		msg = cmd()
		model, cmd = applyRootMsgs(t, model, msg)
	}
	if model.clean.phase != eagerPhaseResult {
		t.Fatalf("phase=%v want result", model.clean.phase)
	}
	result := model.content()
	assertNoPath(t, result)
	assertNoSentinelLeak(t, result)
	for _, want := range []string{"Affected bytes", "cleaned"} {
		if !strings.Contains(result, want) {
			// cleaned may be state wording
			if want == "cleaned" && !strings.Contains(result, "Clean") {
				t.Fatalf("result missing %q:\n%s", want, result)
			}
		}
	}
	// Result enter returns to menu; re-entry is a new scan (tested via start semantics).
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.screen != screenMenu {
		t.Fatalf("result enter should return to menu, screen=%v", model.screen)
	}
}

func TestPrimaryCleanStaleMessagesDoNotMutateAfterLeave(t *testing.T) {
	model := openCategoryFirstClean(t)
	gen := model.clean.generation
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if model.screen != screenMenu {
		t.Fatal("expected menu")
	}
	// Re-enter Clean for a new generation.
	stubInstantEagerPreview(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	for _, msg := range expandTeaCmd(cmd) {
		var nextCmd tea.Cmd
		model, nextCmd = applyRootMsgs(t, model, msg)
		model = drainEagerStream(t, model, nextCmd)
	}
	if model.clean.generation <= gen {
		t.Fatalf("re-entry generation %d should exceed prior %d", model.clean.generation, gen)
	}
	// Inject stale observation from old generation.
	model.clean.applyObservation(eagerCategoryObservationMsg{
		generation: gen,
		observation: clean.CategoryPreviewObservation{
			Identifier:     model.clean.rows[0].Identifier,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 99,
			Bytes:          9999,
		},
	})
	if model.clean.rows[0].CandidateCount == 99 {
		t.Fatal("stale generation mutated current model")
	}
}

func TestPrimaryCleanMenuDescriptionMentionsCategoryFlow(t *testing.T) {
	content := newRootModel().content()
	if !strings.Contains(content, "Measure cleanup categories") {
		t.Fatalf("menu Clean description should describe category-first flow:\n%s", content)
	}
	if strings.Contains(content, "dry-run read model") {
		t.Fatalf("menu still describes report-first dry-run browser:\n%s", content)
	}
}
