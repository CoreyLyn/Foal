package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestEagerCleanModelQueueIsCatalogDerived(t *testing.T) {
	model := newEagerCleanModel(120, 40)
	queue := clean.EagerPreviewQueue()
	if len(model.rows) != len(queue) {
		t.Fatalf("rows = %d, queue = %d (must not hard-code catalog size)", len(model.rows), len(queue))
	}
	for i, row := range model.rows {
		if row.Identifier != queue[i].Identifier {
			t.Fatalf("row[%d] = %q, queue = %q", i, row.Identifier, queue[i].Identifier)
		}
		if row.State != clean.CategoryPreviewWaiting {
			t.Fatalf("initial state[%d] = %q, want waiting", i, row.State)
		}
	}
	if len(model.rows) == 0 {
		t.Fatal("empty queue")
	}
}

func TestEagerCleanModelStartRendersFullQueueImmediately(t *testing.T) {
	model := newEagerCleanModel(120, 40)
	fixed := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	model.now = func() time.Time { return fixed }

	original := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options { return clean.Options{} }
	t.Cleanup(func() { buildEagerPreviewOptions = original })

	cmd := model.start()
	if cmd == nil {
		t.Fatal("start must return commands without another user action")
	}
	if model.generation != 1 || model.startedAt != fixed {
		t.Fatalf("generation/start = %d %#v", model.generation, model.startedAt)
	}

	content := model.content()
	if !strings.Contains(content, "Scanning 1/") {
		t.Fatalf("header missing Scanning n/N:\n%s", content)
	}
	if !strings.Contains(content, "Confirmation available after scan completes") {
		t.Fatalf("header missing confirmation gate notice:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "Scanning") && strings.Contains(line, "%") {
			t.Fatalf("byte-derived percentage in header: %s", line)
		}
	}
	for _, summary := range clean.EagerPreviewQueue() {
		if !strings.Contains(content, summary.Label) {
			t.Fatalf("queue label %q missing from initial render:\n%s", summary.Label, content)
		}
	}
	for _, group := range []string{"User essentials", "System", "Browsers", "Developer tools"} {
		if !strings.Contains(content, group) {
			t.Fatalf("group %q missing:\n%s", group, content)
		}
	}
	if !strings.Contains(content, "Focused:") {
		t.Fatalf("focused detail panel missing:\n%s", content)
	}
}

func TestEagerCleanModelStreamsOneCategoryAtATime(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	model.startedAt = time.Now()
	queue := clean.EagerPreviewQueue()

	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[0].Identifier,
			Label:      queue[0].Label,
			State:      clean.CategoryPreviewScanning,
		},
	})
	if model.activeIndex != 0 || model.rows[0].State != clean.CategoryPreviewScanning {
		t.Fatalf("active = %d state0 = %q", model.activeIndex, model.rows[0].State)
	}
	for i := 1; i < len(model.rows); i++ {
		if model.rows[i].State != clean.CategoryPreviewWaiting {
			t.Fatalf("row %d state = %q, want waiting while first scans", i, model.rows[i].State)
		}
	}
	content := model.content()
	if !strings.Contains(content, fmt.Sprintf("Scanning 1/%d", len(queue))) {
		t.Fatalf("header:\n%s", content)
	}
	if !strings.Contains(content, "…") {
		t.Fatal("waiting marker missing")
	}

	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:     queue[0].Identifier,
			Label:          queue[0].Label,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 2,
			Bytes:          2048,
		},
	})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[1].Identifier,
			Label:      queue[1].Label,
			State:      clean.CategoryPreviewScanning,
		},
	})
	if model.activeIndex != 1 || model.rows[0].State != clean.CategoryPreviewComplete || model.rows[1].State != clean.CategoryPreviewScanning {
		t.Fatalf("after second start: active=%d s0=%q s1=%q", model.activeIndex, model.rows[0].State, model.rows[1].State)
	}
	for i := 2; i < len(model.rows); i++ {
		if model.rows[i].State != clean.CategoryPreviewWaiting {
			t.Fatalf("row %d should still wait", i)
		}
	}
	content = model.content()
	if !strings.Contains(content, "✓") || !strings.Contains(content, "2 item(s)") {
		t.Fatalf("complete row missing:\n%s", content)
	}
	assertNoPath(t, content)
}

func TestEagerCleanModelRendersEveryTerminalMarkerAndFocusedDiagnostic(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	model.startedAt = time.Now()
	queue := clean.EagerPreviewQueue()
	if len(queue) < 6 {
		t.Fatalf("need at least 6 queue rows, got %d", len(queue))
	}

	cases := []struct {
		state    clean.CategoryPreviewState
		count    int
		bytes    int64
		excluded int
		reason   string
		note     string
		marker   string
		rowText  string
		detail   string
	}{
		{clean.CategoryPreviewComplete, 1, 0, 0, "", "", "✓", "item(s)", "Complete · 1 item(s) · 0 KB"},
		{clean.CategoryPreviewPartial, 2, 4096, 3, clean.PreviewReasonProtected, "shared safety", "!", "partial", "3 excluded"},
		{clean.CategoryPreviewEmpty, 0, 0, 0, clean.PreviewReasonEmpty, "", "–", "empty", "Empty · no candidates found"},
		{clean.CategoryPreviewSkipped, 0, 0, 1, clean.PreviewReasonProtected, "", "⊘", "skipped", "protected by Protection rules"},
		{clean.CategoryPreviewIncomplete, 0, 0, 0, clean.PreviewReasonInspectionLimit, "", "!", "incomplete", "inspection limit exceeded"},
		{clean.CategoryPreviewFailed, 0, 0, 0, clean.PreviewReasonInspectionFailed, "", "!", "failed", "measurement failed"},
	}
	for i, tc := range cases {
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:           queue[i].Identifier,
				Label:                queue[i].Label,
				State:                tc.state,
				CandidateCount:       tc.count,
				Bytes:                tc.bytes,
				ExcludedSiblingCount: tc.excluded,
				ReasonCode:           tc.reason,
				SafetyNote:           tc.note,
			},
		})
	}

	// Focus each terminal row and assert marker + contextual panel.
	for i, tc := range cases {
		model.cursor = i
		content := model.content()
		if !strings.Contains(content, tc.marker) {
			t.Fatalf("marker %q missing for %s:\n%s", tc.marker, tc.state, content)
		}
		if !strings.Contains(content, tc.rowText) {
			t.Fatalf("row text %q missing for %s:\n%s", tc.rowText, tc.state, content)
		}
		if !strings.Contains(content, "Focused: "+queue[i].Label) {
			t.Fatalf("focused label missing for %s:\n%s", tc.state, content)
		}
		if !strings.Contains(content, tc.detail) {
			t.Fatalf("detail %q missing for %s:\n%s", tc.detail, tc.state, content)
		}
		if tc.note != "" && !strings.Contains(content, "Safety: "+tc.note) {
			t.Fatalf("safety note missing for %s:\n%s", tc.state, content)
		}
		assertNoPath(t, content)
		assertNoSentinelLeak(t, content)
	}

	if model.confirmationEnabled() {
		t.Fatal("confirmation must stay disabled while categories remain non-terminal")
	}
}

func TestEagerCleanModelDisabledRowsRemainFocusable(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:           queue[0].Identifier,
			Label:                queue[0].Label,
			State:                clean.CategoryPreviewSkipped,
			ReasonCode:           clean.PreviewReasonProtected,
			ExcludedSiblingCount: 2,
		},
	})
	model.cursor = 0
	content := model.content()
	if !strings.Contains(content, "Focused: "+queue[0].Label) {
		t.Fatalf("disabled row not focused:\n%s", content)
	}
	if !strings.Contains(content, "Skipped") || !strings.Contains(content, "2 excluded") {
		t.Fatalf("disabled diagnostic missing:\n%s", content)
	}
	// Cursor can still move onto later waiting rows.
	nav, _ := model.handleKey("down")
	if nav != eagerPreviewNavNone || model.cursor != 1 {
		t.Fatalf("nav=%v cursor=%d", nav, model.cursor)
	}
	content = model.content()
	if !strings.Contains(content, "Focused: "+queue[1].Label) {
		t.Fatalf("cursor did not follow to next row:\n%s", content)
	}
}

func TestEagerCleanModelConfirmationRequiresAllTerminal(t *testing.T) {
	model := newEagerCleanModel(80, 24)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	if model.confirmationEnabled() || model.allCategoriesTerminal() {
		t.Fatal("initial waiting queue must not enable confirmation")
	}
	for i, summary := range queue {
		state := clean.CategoryPreviewEmpty
		if i == 0 {
			state = clean.CategoryPreviewComplete
		}
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				State:          state,
				CandidateCount: map[bool]int{true: 1, false: 0}[i == 0],
				Bytes:          map[bool]int64{true: 10, false: 0}[i == 0],
			},
		})
	}
	if !model.allCategoriesTerminal() || !model.confirmationEnabled() {
		t.Fatal("full terminal queue should enable confirmation gate")
	}
	// Leave one non-terminal.
	model.rows[len(model.rows)-1].State = clean.CategoryPreviewScanning
	if model.confirmationEnabled() {
		t.Fatal("scanning row must block confirmation")
	}
}

func TestEagerCleanModelNoWorkStatesRemainDistinct(t *testing.T) {
	queue := clean.EagerPreviewQueue()

	allEmpty := newEagerCleanModel(80, 24)
	allEmpty.generation = 1
	for _, summary := range queue {
		allEmpty.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier: summary.Identifier,
				Label:      summary.Label,
				State:      clean.CategoryPreviewEmpty,
			},
		})
	}
	if allEmpty.noWorkState() != clean.EagerPreviewNoWorkAllEmpty {
		t.Fatalf("all empty = %q", allEmpty.noWorkState())
	}
	if !strings.Contains(allEmpty.content(), "Nothing to clean.") {
		t.Fatalf("all-empty message missing:\n%s", allEmpty.content())
	}

	diagnostics := newEagerCleanModel(80, 24)
	diagnostics.generation = 1
	for i, summary := range queue {
		state := clean.CategoryPreviewEmpty
		if i == 0 {
			state = clean.CategoryPreviewSkipped
		}
		if i == 1 {
			state = clean.CategoryPreviewFailed
		}
		diagnostics.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier: summary.Identifier,
				Label:      summary.Label,
				State:      state,
			},
		})
	}
	if diagnostics.noWorkState() != clean.EagerPreviewNoWorkDiagnostics {
		t.Fatalf("diagnostics = %q", diagnostics.noWorkState())
	}
	if !strings.Contains(diagnostics.content(), "No selectable cleanup found") {
		t.Fatalf("diagnostics message missing:\n%s", diagnostics.content())
	}

	needSelection := newEagerCleanModel(80, 24)
	needSelection.generation = 1
	for i, summary := range queue {
		state := clean.CategoryPreviewEmpty
		count := 0
		var bytes int64
		if i == 0 {
			state = clean.CategoryPreviewComplete
			count = 1
			bytes = 4
		}
		needSelection.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				State:          state,
				CandidateCount: count,
				Bytes:          bytes,
			},
		})
	}
	// User cleared the surviving selectable default; empty selection is distinct.
	needSelection.clearSelection()
	if needSelection.noWorkState() != clean.EagerPreviewNoWorkNeedSelection {
		t.Fatalf("need selection = %q", needSelection.noWorkState())
	}
	if !strings.Contains(needSelection.content(), "Select at least one category to continue.") {
		t.Fatalf("need-selection message missing:\n%s", needSelection.content())
	}
	if needSelection.confirmationEnabled() {
		t.Fatal("empty selection must not enable confirmation")
	}
}

func TestEagerCleanModelUnavailableFromProtectionFailure(t *testing.T) {
	original := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options {
		return clean.Options{
			ProtectionLoadError: &clean.StructuredIssue{
				Code:    clean.PreviewReasonProtectionConfigFailed,
				Message: `open C:\Users\corey\AppData\Roaming\Foal\protection.txt: access denied`,
				Path:    `C:\Users\corey\AppData\Roaming\Foal\protection.txt`,
			},
		}
	}
	t.Cleanup(func() { buildEagerPreviewOptions = original })

	model := newEagerCleanModel(100, 40)
	cmd := model.start()
	if cmd != nil {
		t.Fatal("unavailable start must not schedule category scan commands")
	}
	if model.unavailable == nil || model.unavailable.Code != clean.PreviewReasonProtectionConfigFailed {
		t.Fatalf("unavailable = %#v", model.unavailable)
	}
	if len(model.rows) != 0 {
		t.Fatalf("unavailable must clear category queue, rows=%d", len(model.rows))
	}
	if model.confirmationEnabled() {
		t.Fatal("unavailable must not enable confirmation")
	}
	content := model.content()
	if !strings.Contains(content, "Clean unavailable") {
		t.Fatalf("title missing:\n%s", content)
	}
	if !strings.Contains(content, clean.PreviewReasonProtectionConfigFailed) {
		t.Fatalf("stable code missing:\n%s", content)
	}
	assertNoPath(t, content)
	assertNoSentinelLeak(t, content)

	nav, _ := model.handleKey("enter")
	if nav != eagerPreviewNavMenu {
		t.Fatalf("enter nav = %v", nav)
	}
	model.nav = eagerPreviewNavNone
	nav, _ = model.handleKey("esc")
	if nav != eagerPreviewNavMenu {
		t.Fatalf("esc nav = %v", nav)
	}
	model.nav = eagerPreviewNavNone
	nav, _ = model.handleKey("b")
	if nav != eagerPreviewNavMenu {
		t.Fatalf("b nav = %v", nav)
	}
	model.nav = eagerPreviewNavNone
	nav, _ = model.handleKey("q")
	if nav != eagerPreviewNavQuit {
		t.Fatalf("q nav = %v", nav)
	}
}

func TestEagerCleanModelNavigationDoesNotRestartQueue(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 7
	model.startedAt = time.Now()
	queue := clean.EagerPreviewQueue()
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 7,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[0].Identifier,
			State:      clean.CategoryPreviewScanning,
		},
	})
	genBefore := model.generation
	activeBefore := model.activeIndex

	nav, cmd := model.handleKey("down")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("browse nav = %v cmd = %v", nav, cmd)
	}
	if model.cursor != 1 {
		t.Fatalf("cursor = %d", model.cursor)
	}
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("up")
	if model.cursor != 1 {
		t.Fatalf("cursor after up = %d", model.cursor)
	}
	if model.generation != genBefore || model.activeIndex != activeBefore {
		t.Fatal("browsing restarted or altered the scan queue")
	}
	if model.rows[0].State != clean.CategoryPreviewScanning {
		t.Fatal("active scan row changed by browsing")
	}
}

func TestEagerCleanModelCancelKeysAndStaleMessages(t *testing.T) {
	queue := clean.EagerPreviewQueue()

	model := newEagerCleanModel(100, 40)
	model.generation = 3
	canceled := false
	model.cancel = func() { canceled = true }

	nav, _ := model.handleKey("esc")
	if nav != eagerPreviewNavMenu || !model.canceled || !canceled {
		t.Fatalf("esc: nav=%v canceled=%v cancelCalled=%v", nav, model.canceled, canceled)
	}
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 3,
		observation: clean.CategoryPreviewObservation{
			Identifier:     queue[0].Identifier,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 9,
			Bytes:          99,
		},
	})
	if model.rows[0].State != clean.CategoryPreviewWaiting || model.rows[0].CandidateCount != 0 {
		t.Fatalf("stale after cancel mutated row: %#v", model.rows[0])
	}

	model = newEagerCleanModel(100, 40)
	model.generation = 4
	canceled = false
	model.cancel = func() { canceled = true }
	nav, _ = model.handleKey("b")
	if nav != eagerPreviewNavMenu || !canceled {
		t.Fatalf("b: nav=%v", nav)
	}

	model = newEagerCleanModel(100, 40)
	model.generation = 5
	canceled = false
	model.cancel = func() { canceled = true }
	nav, _ = model.handleKey("q")
	if nav != eagerPreviewNavQuit || !canceled {
		t.Fatalf("q: nav=%v", nav)
	}

	model = newEagerCleanModel(100, 40)
	model.generation = 6
	canceled = false
	model.cancel = func() { canceled = true }
	nav, _ = model.handleKey("ctrl+c")
	if nav != eagerPreviewNavInterrupt || !canceled {
		t.Fatalf("ctrl+c: nav=%v", nav)
	}
}

func TestEagerCleanModelIgnoresSupersededGeneration(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 2
	queue := clean.EagerPreviewQueue()
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:     queue[0].Identifier,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 3,
			Bytes:          30,
		},
	})
	if model.rows[0].State != clean.CategoryPreviewWaiting {
		t.Fatalf("stale generation applied: %#v", model.rows[0])
	}
	model.applyFinished(eagerPreviewFinishedMsg{generation: 1, canceled: false})
	if model.finished {
		t.Fatal("stale finished applied")
	}
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 2,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[0].Identifier,
			State:      clean.CategoryPreviewEmpty,
		},
	})
	if model.rows[0].State != clean.CategoryPreviewEmpty {
		t.Fatal("current generation not applied")
	}
}

func TestEagerCleanModelAnimationTickIsDeterministic(t *testing.T) {
	model := newEagerCleanModel(80, 24)
	model.generation = 1
	model.startedAt = time.Now()
	queue := clean.EagerPreviewQueue()
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[0].Identifier,
			State:      clean.CategoryPreviewScanning,
		},
	})

	cmd := model.applyTick(eagerPreviewTickMsg{generation: 1, frame: 1})
	if cmd == nil {
		t.Fatal("tick should schedule the next tick while scanning")
	}
	if model.spinnerFrame != 1 {
		t.Fatalf("spinnerFrame = %d", model.spinnerFrame)
	}
	content := model.content()
	wantFrame := eagerPreviewSpinnerFrames[1%len(eagerPreviewSpinnerFrames)]
	if !strings.Contains(content, wantFrame) {
		t.Fatalf("expected spinner %q in:\n%s", wantFrame, content)
	}
	_ = model.applyTick(eagerPreviewTickMsg{generation: 1, frame: 2})
	content2 := model.content()
	wantFrame2 := eagerPreviewSpinnerFrames[2%len(eagerPreviewSpinnerFrames)]
	if !strings.Contains(content2, wantFrame2) {
		t.Fatalf("expected spinner %q in:\n%s", wantFrame2, content2)
	}

	model.generation = 2
	if cmd := model.applyTick(eagerPreviewTickMsg{generation: 1, frame: 9}); cmd != nil {
		t.Fatal("stale tick must not reschedule")
	}
	if model.spinnerFrame == 9 {
		t.Fatal("stale tick mutated frame")
	}
}

func TestEagerPreviewStreamUsesSharedSeamWithoutSideEffects(t *testing.T) {
	original := runEagerPreviewFn
	var optsSeen clean.Options
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
		optsSeen = opts
		if opts.HistoryRecorder != nil || opts.DetailedListDir != "" || opts.RecycleBinAdapter != nil {
			t.Fatalf("eager options enable side effects: %#v", opts)
		}
		if opts.DiscoverReviewSuggestions != nil {
			t.Fatal("eager preview must not attach Review suggestion probes")
		}
		queue := clean.EagerPreviewQueue()
		first := queue[0]
		emit(clean.CategoryPreviewObservation{
			Identifier:     first.Identifier,
			Label:          first.Label,
			ReportCategory: first.ReportCategory,
			Eligibility:    first.Eligibility,
			State:          clean.CategoryPreviewScanning,
		})
		emit(clean.ProjectCategoryPreview(clean.CategoryResolution{
			Identifier:  first.Identifier,
			Eligibility: first.Eligibility,
			Candidates:  []clean.CandidatePreview{{Path: `C:\private\only-in-resolution`, Bytes: 4}},
		}))
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = original })

	model := newEagerCleanModel(100, 40)
	model.generation = 99
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := startEagerPreviewCmd(ctx, 99, clean.Options{})().(eagerPreviewStartedMsg)
	wait := waitEagerPreviewCmd(99, started.stream)

	scanMsg := wait().(eagerCategoryObservationMsg)
	model.applyObservation(scanMsg)
	if scanMsg.observation.State != clean.CategoryPreviewScanning {
		t.Fatalf("first = %#v", scanMsg.observation)
	}
	wait = waitEagerPreviewCmd(99, started.stream)

	termMsg := wait().(eagerCategoryObservationMsg)
	model.applyObservation(termMsg)
	if termMsg.observation.State != clean.CategoryPreviewComplete || termMsg.observation.CandidateCount != 1 || termMsg.observation.Bytes != 4 {
		t.Fatalf("terminal = %#v", termMsg.observation)
	}
	if strings.Contains(fmt.Sprintf("%#v", termMsg.observation), `C:\private`) {
		t.Fatalf("path in observation: %#v", termMsg.observation)
	}
	assertNoPath(t, model.content())
	_ = optsSeen
}

func TestEagerPreviewFinishedAfterFullQueue(t *testing.T) {
	original := runEagerPreviewFn
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
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
			emit(clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				ReportCategory: summary.ReportCategory,
				Eligibility:    summary.Eligibility,
				State:          clean.CategoryPreviewEmpty,
			})
		}
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = original })

	model := newEagerCleanModel(100, 40)
	model.generation = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := startEagerPreviewCmd(ctx, 1, clean.Options{})().(eagerPreviewStartedMsg)
	wait := waitEagerPreviewCmd(1, started.stream)
	for {
		msg := wait()
		switch m := msg.(type) {
		case eagerCategoryObservationMsg:
			model.applyObservation(m)
			wait = waitEagerPreviewCmd(1, started.stream)
		case eagerPreviewFinishedMsg:
			model.applyFinished(m)
			if model.canceled || !model.finished {
				t.Fatalf("finished state = finished=%v canceled=%v", model.finished, model.canceled)
			}
			if model.completed != len(model.rows) {
				t.Fatalf("completed = %d, rows = %d", model.completed, len(model.rows))
			}
			content := model.content()
			if !strings.Contains(content, "Scan complete") {
				t.Fatalf("content:\n%s", content)
			}
			if !strings.Contains(content, "Nothing to clean.") {
				t.Fatalf("all-empty finished message missing:\n%s", content)
			}
			assertNoPath(t, content)
			return
		default:
			t.Fatalf("unexpected %T", msg)
		}
	}
}

func TestEagerPreviewCancelStopsFurtherMutations(t *testing.T) {
	original := runEagerPreviewFn
	block := make(chan struct{})
	release := make(chan struct{})
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
		queue := clean.EagerPreviewQueue()
		first := queue[0]
		emit(clean.CategoryPreviewObservation{
			Identifier: first.Identifier,
			Label:      first.Label,
			State:      clean.CategoryPreviewScanning,
		})
		close(block)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		emit(clean.CategoryPreviewObservation{
			Identifier:     first.Identifier,
			Label:          first.Label,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 5,
			Bytes:          50,
		})
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

	model := newEagerCleanModel(100, 40)
	model.generation = 1
	ctx, cancel := context.WithCancel(context.Background())
	model.cancel = cancel
	started := startEagerPreviewCmd(ctx, 1, clean.Options{})().(eagerPreviewStartedMsg)
	wait := waitEagerPreviewCmd(1, started.stream)

	msg := wait()
	obs, ok := msg.(eagerCategoryObservationMsg)
	if !ok || obs.observation.State != clean.CategoryPreviewScanning {
		t.Fatalf("first msg = %#v", msg)
	}
	model.applyObservation(obs)

	<-block
	nav, _ := model.handleKey("esc")
	if nav != eagerPreviewNavMenu || !model.canceled {
		t.Fatalf("nav=%v canceled=%v", nav, model.canceled)
	}
	close(release)

	wait = waitEagerPreviewCmd(1, started.stream)
	for i := 0; i < 4; i++ {
		msg := wait()
		switch m := msg.(type) {
		case eagerCategoryObservationMsg:
			model.applyObservation(m)
			if model.rows[0].State == clean.CategoryPreviewComplete {
				t.Fatal("post-cancel terminal observation mutated the model")
			}
			wait = waitEagerPreviewCmd(1, started.stream)
		case eagerPreviewFinishedMsg:
			model.applyFinished(m)
			if model.rows[0].CandidateCount != 0 {
				t.Fatalf("row after cancel: %#v", model.rows[0])
			}
			return
		default:
			t.Fatalf("unexpected %T", msg)
		}
	}
}

func TestEagerCleanModelDefaultSelectionAndCursorIndependence(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	queue := clean.EagerPreviewQueue()
	if len(queue) < 2 {
		t.Fatal("need default and opt-in rows")
	}

	defaults := 0
	optIns := 0
	for i, row := range model.rows {
		switch row.Eligibility {
		case clean.CategoryEligibilityDefault:
			defaults++
			if !row.Selected {
				t.Fatalf("default %q must start selected", row.Identifier)
			}
		case clean.CategoryEligibilityOptIn:
			optIns++
			if row.Selected {
				t.Fatalf("opt-in %q must start unselected", row.Identifier)
			}
		default:
			t.Fatalf("queue row eligibility = %q", row.Eligibility)
		}
		// Selection IDs are canonical catalog identifiers only.
		if row.Identifier != queue[i].Identifier {
			t.Fatalf("non-canonical id %q", row.Identifier)
		}
	}
	if defaults == 0 || optIns == 0 {
		t.Fatalf("defaults=%d optIns=%d", defaults, optIns)
	}
	if model.selectedCount() != defaults {
		t.Fatalf("selectedCount = %d, want %d defaults", model.selectedCount(), defaults)
	}
	for _, id := range model.selectedCategoryIDs() {
		if strings.Contains(id, `\`) || strings.Contains(id, "/") || strings.Contains(id, " ") {
			t.Fatalf("selection id looks path-bearing or alias-like: %q", id)
		}
		if id == "dev-caches" || id == "all" {
			t.Fatalf("group token in selection: %q", id)
		}
	}

	// Cursor movement never changes selection.
	before := append([]bool(nil), selectedFlags(model)...)
	_, cmd := model.handleKey("down")
	if cmd != nil {
		t.Fatal("browse must not schedule a scan command")
	}
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("up")
	if !equalBools(before, selectedFlags(model)) {
		t.Fatal("cursor movement changed selection")
	}
	content := model.content()
	if !strings.Contains(content, "[x]") || !strings.Contains(content, "[ ]") {
		t.Fatalf("checkbox markers missing:\n%s", content)
	}
	if !strings.Contains(content, ">") {
		t.Fatalf("cursor missing:\n%s", content)
	}
}

func TestEagerCleanModelSpaceTogglesSelectableDuringScan(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	// Row 0 is default (selected, waiting); select an opt-in waiting row.
	optInIndex := -1
	for i, row := range model.rows {
		if row.Eligibility == clean.CategoryEligibilityOptIn {
			optInIndex = i
			break
		}
	}
	if optInIndex < 0 {
		t.Fatal("no opt-in row")
	}
	genBefore := model.generation
	activeBefore := model.activeIndex

	model.cursor = optInIndex
	nav, cmd := model.handleKey(" ")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("space nav=%v cmd=%v", nav, cmd)
	}
	if !model.rows[optInIndex].Selected {
		t.Fatal("space must select waiting opt-in")
	}
	if model.generation != genBefore || model.activeIndex != activeBefore {
		t.Fatal("selection restarted or reprioritized the scan")
	}

	// Space again clears provisional selection.
	_, cmd = model.handleKey(" ")
	if cmd != nil || model.rows[optInIndex].Selected {
		t.Fatalf("space toggle clear failed selected=%v cmd=%v", model.rows[optInIndex].Selected, cmd)
	}

	// Scanning row remains provisionally selectable.
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[optInIndex].Identifier,
			State:      clean.CategoryPreviewScanning,
		},
	})
	model.cursor = optInIndex
	_, cmd = model.handleKey(" ")
	if cmd != nil || !model.rows[optInIndex].Selected {
		t.Fatalf("scanning toggle selected=%v cmd=%v", model.rows[optInIndex].Selected, cmd)
	}
	// Active scan row must still be scanning after selection.
	if model.rows[optInIndex].State != clean.CategoryPreviewScanning {
		t.Fatal("selection altered scan state")
	}
}

func TestEagerCleanModelTerminalAutoClearAndPartialPreservation(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	if len(queue) < 5 {
		t.Fatal("need enough rows for terminal outcomes")
	}

	// Select several rows provisionally, including defaults already selected.
	for i := 0; i < 5; i++ {
		model.rows[i].Selected = true
	}

	terminalCases := []struct {
		state    clean.CategoryPreviewState
		count    int
		bytes    int64
		wantSel  bool
		excluded int
	}{
		{clean.CategoryPreviewComplete, 1, 0, true, 0},   // zero-byte complete stays
		{clean.CategoryPreviewPartial, 2, 4096, true, 1}, // partial stays with safe bytes
		{clean.CategoryPreviewEmpty, 0, 0, false, 0},
		{clean.CategoryPreviewSkipped, 0, 0, false, 1},
		{clean.CategoryPreviewFailed, 0, 0, false, 0},
	}
	for i, tc := range terminalCases {
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:           queue[i].Identifier,
				Label:                queue[i].Label,
				State:                tc.state,
				CandidateCount:       tc.count,
				Bytes:                tc.bytes,
				ExcludedSiblingCount: tc.excluded,
				ReasonCode:           clean.PreviewReasonProtected,
			},
		})
		if model.rows[i].Selected != tc.wantSel {
			t.Fatalf("%s selected=%v want %v", tc.state, model.rows[i].Selected, tc.wantSel)
		}
	}

	// Space on disabled skipped row changes nothing; diagnostic remains.
	model.cursor = 3
	before := model.rows[3].Selected
	_, cmd := model.handleKey(" ")
	if cmd != nil || model.rows[3].Selected != before {
		t.Fatalf("disabled space mutated selection cmd=%v", cmd)
	}
	model.cursor = 3
	content := model.content()
	if !strings.Contains(content, "Skipped") {
		t.Fatalf("disabled diagnostic missing:\n%s", content)
	}

	// Incomplete also auto-clears.
	model.rows[4].Selected = true
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[4].Identifier,
			State:      clean.CategoryPreviewIncomplete,
			ReasonCode: clean.PreviewReasonInspectionLimit,
		},
	})
	if model.rows[4].Selected {
		t.Fatal("incomplete must clear selection")
	}
}

func TestEagerCleanModelBulkSelectAndClear(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()

	// Mix of selectable and disabled terminal rows.
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:     queue[0].Identifier,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 1,
			Bytes:          100,
		},
	})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[1].Identifier,
			State:      clean.CategoryPreviewEmpty,
		},
	})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[2].Identifier,
			State:      clean.CategoryPreviewSkipped,
			ReasonCode: clean.PreviewReasonProtected,
		},
	})
	// Remaining stay waiting (selectable).

	genBefore := model.generation
	nav, cmd := model.handleKey("a")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("a nav=%v cmd=%v", nav, cmd)
	}
	if model.generation != genBefore {
		t.Fatal("bulk select restarted scan")
	}
	for _, row := range model.rows {
		switch row.State {
		case clean.CategoryPreviewEmpty, clean.CategoryPreviewSkipped,
			clean.CategoryPreviewIncomplete, clean.CategoryPreviewFailed:
			if row.Selected {
				t.Fatalf("disabled %s %q selected by a", row.State, row.Identifier)
			}
		default:
			if !row.Selected {
				t.Fatalf("selectable %s %q not selected by a", row.State, row.Identifier)
			}
		}
	}

	nav, cmd = model.handleKey("x")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("x nav=%v cmd=%v", nav, cmd)
	}
	if model.selectedCount() != 0 {
		t.Fatalf("x left %d selected", model.selectedCount())
	}
	// Defaults are also cleared — exact selection, not additive CLI defaults.
	for _, row := range model.rows {
		if row.Selected {
			t.Fatalf("default/opt-in still selected after x: %q", row.Identifier)
		}
	}
	if model.generation != genBefore {
		t.Fatal("bulk clear restarted scan")
	}
}

func TestEagerCleanModelSelectionTotalsMeasuredAndPending(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	if len(queue) < 4 {
		t.Fatal("need several rows")
	}

	// Clear then select a controlled set.
	model.clearSelection()
	model.rows[0].Selected = true // will complete with bytes
	model.rows[1].Selected = true // will be partial
	model.rows[2].Selected = true // remains waiting -> pending
	model.rows[3].Selected = true // scanning -> pending

	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:     queue[0].Identifier,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 1,
			Bytes:          2048,
		},
	})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier:           queue[1].Identifier,
			State:                clean.CategoryPreviewPartial,
			CandidateCount:       1,
			Bytes:                1024,
			ExcludedSiblingCount: 2,
		},
	})
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.CategoryPreviewObservation{
			Identifier: queue[3].Identifier,
			State:      clean.CategoryPreviewScanning,
		},
	})

	n, measured, pending := model.selectionTotals()
	if n != 4 {
		t.Fatalf("categories = %d, want 4", n)
	}
	if measured != 2048+1024 {
		t.Fatalf("measured = %d, want 3072", measured)
	}
	if pending != 2 {
		t.Fatalf("pending = %d, want 2", pending)
	}
	content := model.content()
	if !strings.Contains(content, "Selected: 4 categories · 3 KB measured · 2 pending") {
		t.Fatalf("pending summary missing:\n%s", content)
	}

	// Zero-byte complete contributes 0 bytes but stays in count.
	model.clearSelection()
	model.rows[0].Selected = true
	model.rows[0].State = clean.CategoryPreviewComplete
	model.rows[0].Bytes = 0
	model.rows[0].CandidateCount = 1
	n, measured, pending = model.selectionTotals()
	if n != 1 || measured != 0 || pending != 0 {
		t.Fatalf("zero-byte complete totals n=%d measured=%d pending=%d", n, measured, pending)
	}
	content = model.content()
	if !strings.Contains(content, "Selected: 1 categories · 0 KB") {
		t.Fatalf("collapsed summary missing:\n%s", content)
	}
	if strings.Contains(content, "pending") {
		t.Fatalf("pending must collapse when selection is terminal:\n%s", content)
	}

	// Unselected complete bytes never enter the total.
	model.rows[1].State = clean.CategoryPreviewComplete
	model.rows[1].Bytes = 99999
	model.rows[1].Selected = false
	_, measured, _ = model.selectionTotals()
	if measured != 0 {
		t.Fatalf("unselected bytes leaked into total: %d", measured)
	}
}

func TestEagerCleanModelConfirmationRequiresNonEmptySelection(t *testing.T) {
	model := newEagerCleanModel(80, 24)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	for i, summary := range queue {
		state := clean.CategoryPreviewEmpty
		count := 0
		var bytes int64
		if i == 0 {
			state = clean.CategoryPreviewComplete
			count = 1
			bytes = 10
		}
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				State:          state,
				CandidateCount: count,
				Bytes:          bytes,
			},
		})
	}
	// First default complete remains selected -> confirmation ready.
	if !model.confirmationEnabled() || model.selectedCount() != 1 {
		t.Fatalf("want ready selection: enabled=%v count=%d", model.confirmationEnabled(), model.selectedCount())
	}
	model.clearSelection()
	if model.confirmationEnabled() {
		t.Fatal("empty selection must block confirmation")
	}
	if model.noWorkState() != clean.EagerPreviewNoWorkNeedSelection {
		t.Fatalf("noWork = %q", model.noWorkState())
	}
}

func TestEagerCleanModelSelectionNeverInvokesExecutionOrHistory(t *testing.T) {
	original := runEagerPreviewFn
	var scanStarts int
	runEagerPreviewFn = func(ctx context.Context, opts clean.Options, emit func(clean.CategoryPreviewObservation)) error {
		scanStarts++
		if opts.HistoryRecorder != nil || opts.DetailedListDir != "" || opts.RecycleBinAdapter != nil {
			t.Fatalf("side-effect options present: %#v", opts)
		}
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = original })

	model := newEagerCleanModel(100, 40)
	// Drive selection without start(); keys must not call the preview seam.
	_, cmd := model.handleKey("a")
	if cmd != nil {
		t.Fatal("select-all scheduled a command")
	}
	_, cmd = model.handleKey("x")
	if cmd != nil {
		t.Fatal("clear scheduled a command")
	}
	model.cursor = 0
	_, cmd = model.handleKey(" ")
	if cmd != nil {
		t.Fatal("space scheduled a command")
	}
	if scanStarts != 0 {
		t.Fatalf("selection started %d scans", scanStarts)
	}
	// No retry/rescan key: 'r' is a no-op with no command.
	_, cmd = model.handleKey("r")
	if cmd != nil {
		t.Fatal("r must not rescan in the first slice")
	}
}

func selectedFlags(model eagerCleanModel) []bool {
	out := make([]bool, len(model.rows))
	for i, row := range model.rows {
		out[i] = row.Selected
	}
	return out
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEagerCleanModelNeverRendersSentinelPaths(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1
	queue := clean.EagerPreviewQueue()
	model.applyObservation(eagerCategoryObservationMsg{
		generation: 1,
		observation: clean.ProjectCategoryPreview(clean.CategoryResolution{
			Identifier:  queue[0].Identifier,
			Eligibility: queue[0].Eligibility,
			Candidates: []clean.CandidatePreview{{
				Path:  `C:\Users\private\sentinel-candidate`,
				Bytes: 12,
			}},
			SuppressedProtectionPaths: []string{`C:\Users\private\sentinel-protected`},
			Diagnostics: []clean.StructuredIssue{{
				Code:    clean.PreviewReasonInspectionFailed,
				Message: `open C:\Users\private\sentinel-error: boom`,
				Path:    `C:\Users\private\sentinel-error`,
			}},
		}),
	})
	model.cursor = 0
	content := model.content()
	assertNoPath(t, content)
	assertNoSentinelLeak(t, content)
	if !strings.Contains(content, "partial") && !strings.Contains(content, "Partial") {
		t.Fatalf("expected partial projection:\n%s", content)
	}
}

func assertNoPath(t *testing.T, content string) {
	t.Helper()
	for _, needle := range []string{`C:\`, `\\?\`, "/Users/", "AppData\\Local"} {
		if strings.Contains(content, needle) {
			t.Fatalf("path-like content %q in:\n%s", needle, content)
		}
	}
}

func assertNoSentinelLeak(t *testing.T, content string) {
	t.Helper()
	for _, needle := range []string{
		"sentinel-candidate", "sentinel-protected", "sentinel-error",
		"protection.txt", "private\\",
	} {
		if strings.Contains(content, needle) {
			t.Fatalf("sentinel %q leaked:\n%s", needle, content)
		}
	}
}
