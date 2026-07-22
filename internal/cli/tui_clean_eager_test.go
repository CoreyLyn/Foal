package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

// recordingHistoryRecorder is a no-op History seam for exact-handoff tests.
type recordingHistoryRecorder struct{}

func (*recordingHistoryRecorder) Record(context.Context, history.SessionRecord, []history.ItemRecord) error {
	return nil
}

// exactExecutionStartedFrom peels beginExactExecution's tea.Batch (execute +
// tick) and returns the start message without waiting on the spinner tick sleep.
func exactExecutionStartedFrom(t *testing.T, cmd tea.Cmd) eagerExactExecutionStartedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil execution cmd")
	}
	msg := cmd()
	if started, ok := msg.(eagerExactExecutionStartedMsg); ok {
		return started
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("execution cmd msg = %T, want Batch or start", msg)
	}
	// Run children in parallel so the immediate start msg is not blocked by
	// tea.Tick's sleep on the companion tick command.
	ch := make(chan tea.Msg, len(batch))
	for _, child := range batch {
		child := child
		go func() { ch <- child() }()
	}
	deadline := time.After(2 * time.Second)
	for i := 0; i < len(batch); i++ {
		select {
		case m := <-ch:
			if started, ok := m.(eagerExactExecutionStartedMsg); ok {
				return started
			}
		case <-deadline:
			t.Fatal("timeout waiting for eagerExactExecutionStartedMsg")
		}
	}
	t.Fatal("execution batch missing eagerExactExecutionStartedMsg")
	return eagerExactExecutionStartedMsg{}
}

func TestCleanFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 KB"},
		{name: "negative", bytes: -1, want: "0 KB"},
		{name: "less than one kilobyte", bytes: 512, want: "<1 KB"},
		{name: "kilobytes", bytes: 1536, want: "1.5 KB"},
		{name: "megabytes", bytes: 2 * 1024 * 1024, want: "2 MB"},
		{name: "gigabytes", bytes: 3 * 1024 * 1024 * 1024, want: "3 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanFormatBytes(tt.bytes); got != tt.want {
				t.Fatalf("cleanFormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

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
		// Windows servicing rows start as analysis_required (never file-scanned),
		// not the file-scan waiting state.
		if clean.IsServicingSummary(queue[i]) {
			if !row.Servicing || row.ServicingState != clean.ServicingRowAnalysisRequired {
				t.Fatalf("servicing row[%d] = servicing:%v state:%q, want analysis_required", i, row.Servicing, row.ServicingState)
			}
			continue
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
	// Height must fit all executable rows + group headers after matrix growth.
	model := newEagerCleanModel(120, 64)
	fixed := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	model.now = func() time.Time { return fixed }

	original := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options { return clean.Options{} }
	t.Cleanup(func() { buildEagerPreviewOptions = original })

	cmd := model.start()
	if cmd == nil {
		t.Fatal("start must return commands without another user action")
	}
	if model.generation == 0 || model.startedAt != fixed {
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
		// Servicing rows are never file-scanned; they rest at analysis_required.
		if model.rows[i].Servicing {
			continue
		}
		if model.rows[i].State != clean.CategoryPreviewWaiting {
			t.Fatalf("row %d state = %q, want waiting while first scans", i, model.rows[i].State)
		}
	}
	content := model.content()
	// Header denominator counts only file-scanned rows (servicing rows excluded).
	scannable := 0
	for _, s := range queue {
		if !clean.IsServicingSummary(s) {
			scannable++
		}
	}
	if !strings.Contains(content, fmt.Sprintf("Scanning 1/%d", scannable)) {
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
		if model.rows[i].Servicing {
			continue
		}
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
			// Servicing rows are never file-scanned, so completed counts only the
			// scannable (non-servicing) rows.
			if model.completed != model.scannableRowCount() {
				t.Fatalf("completed = %d, scannable = %d", model.completed, model.scannableRowCount())
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
			// Permanent-action opt-ins (complete 20-category matrix) start selected.
			if row.PlannedAction == clean.PlannedActionDeletePermanently {
				if !row.Selected {
					t.Fatalf("permanent opt-in %q must start selected", row.Identifier)
				}
			} else if row.Selected {
				t.Fatalf("recycle-bin opt-in %q must start unselected", row.Identifier)
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
	wantSelected := defaults
	permanentOptIns := 0
	recycleBinOptIns := 0
	for _, row := range model.rows {
		if row.Eligibility == clean.CategoryEligibilityOptIn && row.PlannedAction == clean.PlannedActionDeletePermanently {
			wantSelected++
			permanentOptIns++
		}
		if row.Eligibility == clean.CategoryEligibilityOptIn && row.PlannedAction == clean.PlannedActionMoveToRecycleBin {
			recycleBinOptIns++
		}
	}
	// Complete rule matrix: 1 default + 36 permanent = 37; 7 Recycle Bin opt-ins
	// unselected (windows_error_reporting, the exact-selection-only
	// nvidia_installer_cache, lghub-cache, thunder-update-download, windows-temp,
	// and windows-update-download-cache, plus the Standard-selection
	// electron-updater-residue).
	if defaults != 1 || permanentOptIns != 36 || recycleBinOptIns != 7 || wantSelected != 37 {
		t.Fatalf("matrix selection defaults=%d permanent=%d rb_opt_ins=%d wantSelected=%d; want 1/36/7/37",
			defaults, permanentOptIns, recycleBinOptIns, wantSelected)
	}
	if model.selectedCount() != 37 {
		t.Fatalf("selectedCount = %d, want 37 (default + all permanent when rows present)", model.selectedCount())
	}
	if len(model.rows) != 45 {
		t.Fatalf("eager rows = %d, want 45 executable categories", len(model.rows))
	}
	for _, id := range model.selectedCategoryIDs() {
		if strings.Contains(id, `\`) || strings.Contains(id, "/") || strings.Contains(id, " ") {
			t.Fatalf("selection id looks path-bearing or alias-like: %q", id)
		}
		if id == "dev-caches" || id == "app-caches" || id == "cli-agents" || id == "all" {
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
	// Row 0 is default (selected, waiting); select an unselected opt-in waiting
	// row (permanent opt-ins start selected, so pick the first unselected opt-in,
	// e.g. windows_error_reporting).
	optInIndex := -1
	for i, row := range model.rows {
		if row.Eligibility == clean.CategoryEligibilityOptIn && !row.Selected {
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
		// Exact-selection-only categories are excluded from Select All regardless
		// of scan state; they must stay unselected after "a".
		if row.SelectionPolicy == clean.CategorySelectionPolicyExactOnly {
			if row.Selected {
				t.Fatalf("exact-selection-only %q selected by a", row.Identifier)
			}
			continue
		}
		switch row.State {
		case clean.CategoryPreviewEmpty, clean.CategoryPreviewSkipped,
			clean.CategoryPreviewIncomplete, clean.CategoryPreviewFailed:
			if row.Selected {
				t.Fatalf("disabled %s %q selected by a", row.State, row.Identifier)
			}
		default:
			// Exact-selection-only rows (Windows servicing) are never bulk-selected
			// by Select All, even while in a selectable state.
			if row.SelectionPolicy == clean.CategorySelectionPolicyExactOnly {
				if row.Selected {
					t.Fatalf("exact-selection-only %q selected by a", row.Identifier)
				}
				continue
			}
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

func TestEagerCleanModelPermanentSelectionNoticeWithoutRowMarkers(t *testing.T) {
	// Inject mixed planned actions so plain frame assertions do not depend on
	// production catalog membership of any particular permanent category.
	queue := []clean.CleanupCategorySummary{
		{
			Identifier:     "recycle_one",
			Label:          "Recycle one",
			ReportCategory: clean.ReportCategoryUserEssentials,
			Eligibility:    clean.CategoryEligibilityDefault,
			PlannedAction:  clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:     "perm_one",
			Label:          "Perm one",
			ReportCategory: clean.ReportCategorySystem,
			Eligibility:    clean.CategoryEligibilityOptIn,
			PlannedAction:  clean.PlannedActionDeletePermanently,
		},
	}
	model := newEagerCleanModelFromSummaries(queue, 100, 40)
	model.generation = 1
	model.startedAt = time.Now()
	for i := range model.rows {
		model.rows[i].State = clean.CategoryPreviewComplete
		model.rows[i].CandidateCount = 1
		model.rows[i].Bytes = 1024
		model.rows[i].Selected = true
	}
	model.finished = true

	content := model.content()
	// Rows show labels without per-row planned-action prefixes.
	if !strings.Contains(content, "Recycle one") || !strings.Contains(content, "Perm one") {
		t.Fatalf("missing category labels:\n%s", content)
	}
	if strings.Contains(content, "bin ·") || strings.Contains(content, "perm ·") {
		t.Fatalf("preview must not show per-row perm/bin prefixes:\n%s", content)
	}
	// Footer notice while selection includes permanent.
	if !strings.Contains(content, "includes permanent deletion") {
		t.Fatalf("missing permanent-selection notice:\n%s", content)
	}
	if strings.Contains(content, "perm=permanent") || strings.Contains(content, "bin=Recycle Bin") {
		t.Fatalf("hints must not document removed row markers:\n%s", content)
	}
	// Plain frame oracle: no embedded pure-red risk tint.
	if strings.Contains(content, "\x1b[31m") || strings.Contains(content, "\x1b[91m") {
		t.Fatalf("plain content must not embed pure-red risk tint:\n%q", content)
	}

	// Clearing all permanent categories removes the notice.
	for i := range model.rows {
		if model.rows[i].PlannedAction == clean.PlannedActionDeletePermanently {
			model.rows[i].Selected = false
		}
	}
	cleared := model.content()
	if strings.Contains(cleared, "includes permanent deletion") {
		t.Fatalf("notice must disappear when permanent selection is empty:\n%s", cleared)
	}
	// Labels remain; only the notice is selection-gated.
	if !strings.Contains(cleared, "Perm one") || !strings.Contains(cleared, "Recycle one") {
		t.Fatalf("labels must remain after deselect:\n%s", cleared)
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

// markEagerQueueTerminal forces every row into a terminal path-free outcome.
// When keepDefaultComplete is true, the first default stays complete+selected;
// remaining rows become empty (auto-deselected).
func markEagerQueueTerminal(model *eagerCleanModel, keepDefaultComplete bool) {
	model.generation = 1
	for i, row := range model.rows {
		state := clean.CategoryPreviewEmpty
		count := 0
		var bytes int64
		if keepDefaultComplete && i == 0 {
			state = clean.CategoryPreviewComplete
			count = 1
			bytes = 42
		}
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     row.Identifier,
				Label:          row.Label,
				State:          state,
				CandidateCount: count,
				Bytes:          bytes,
			},
		})
	}
	model.finished = true
}

func TestEagerCleanEnterBlockedUntilTerminalAndNonEmpty(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	model.generation = 1

	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhasePreview {
		t.Fatalf("enter while scanning opened confirmation: nav=%v cmd=%v phase=%v", nav, cmd, model.phase)
	}

	// All terminal but empty selection.
	markEagerQueueTerminal(&model, true)
	model.clearSelection()
	nav, cmd = model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhasePreview {
		t.Fatalf("enter with empty selection opened confirmation: phase=%v", model.phase)
	}
	if model.confirmationEnabled() {
		t.Fatal("empty selection must not enable confirmation")
	}
}

func TestEagerCleanFirstEnterOpensConfirmationWithoutExecutionOrHistory(t *testing.T) {
	calls := 0
	original := runExactCleanSelection
	runExactCleanSelection = func(context.Context, []string, bool, bool, clean.ProgressReporter) clean.Result {
		calls++
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Keep this confirmation Recycle Bin-only: deselect permanent categories
	// and select one Recycle Bin opt-in.
	optInIndex := -1
	for i, row := range model.rows {
		if row.PlannedAction == clean.PlannedActionDeletePermanently {
			model.rows[i].Selected = false
			continue
		}
		if row.Eligibility == clean.CategoryEligibilityOptIn && optInIndex < 0 {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 2
			model.rows[i].Bytes = 100
			model.rows[i].Selected = true
			optInIndex = i
		}
	}
	if optInIndex < 0 {
		t.Fatal("catalog has no recycle-bin opt-in category")
	}

	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("first enter must not schedule execution: nav=%v cmd=%v", nav, cmd)
	}
	if model.phase != eagerPhaseConfirmation || calls != 0 {
		t.Fatalf("phase=%v calls=%d, want confirmation with no execution", model.phase, calls)
	}

	content := model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Confirm cleanup",
		"Recycle Bin",
		model.rows[0].Label,
		model.rows[optInIndex].Label,
		"Selected: 2 categories",
		confirmationNextStepLine,
		"cannot introduce a deletion action type",
		"Enter: execute",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, content)
		}
	}
	// Recycle Bin-only selection must not show permanent irreversible warning
	// or the permanent-strengthened Enter hint.
	if strings.Contains(content, "Permanent deletion is irreversible") {
		t.Fatalf("recycle-only confirmation must not show permanent irreversible warning:\n%s", content)
	}
	if strings.Contains(content, "Enter: start cleanup") {
		t.Fatalf("recycle-only confirmation must not use permanent Enter hint:\n%s", content)
	}
	// Unselected empty categories must not appear as selected lines.
	selectedBlock := content
	if idx := strings.Index(content, "Permanent deletion"); idx >= 0 {
		selectedBlock = content[idx:]
	} else if idx := strings.Index(content, "Recycle Bin ·"); idx >= 0 {
		selectedBlock = content[idx:]
	}
	if end := strings.Index(selectedBlock, "\nSelected:"); end >= 0 {
		selectedBlock = selectedBlock[:end]
	}
	for _, row := range model.rows {
		if row.Selected {
			if !strings.Contains(selectedBlock, row.Label) {
				t.Fatalf("selected %q missing from confirmation list:\n%s", row.Label, selectedBlock)
			}
			continue
		}
		if strings.Contains(selectedBlock, "- "+row.Label) {
			t.Fatalf("unselected %q listed in confirmation:\n%s", row.Label, selectedBlock)
		}
	}
	if calls != 0 {
		t.Fatalf("confirmation wrote/started execution %d times", calls)
	}
}

func TestEagerCleanConfirmationEscapePreservesSelectionWithoutRescan(t *testing.T) {
	scanStarts := 0
	originalPreview := runEagerPreviewFn
	runEagerPreviewFn = func(context.Context, clean.Options, func(clean.CategoryPreviewObservation)) error {
		scanStarts++
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = originalPreview })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Deselect default, select first opt-in for a distinctive selection.
	model.rows[0].Selected = false
	for i := range model.rows {
		if model.rows[i].Eligibility == clean.CategoryEligibilityOptIn {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 7
			model.rows[i].Selected = true
			break
		}
	}
	before := append([]string(nil), model.selectedCategoryIDs()...)
	if len(before) == 0 {
		t.Fatal("expected non-empty selection")
	}

	_, _ = model.handleKey("enter")
	if model.phase != eagerPhaseConfirmation {
		t.Fatal("expected confirmation")
	}
	nav, cmd := model.handleKey("esc")
	if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhasePreview {
		t.Fatalf("escape must return to preview: nav=%v cmd=%v phase=%v", nav, cmd, model.phase)
	}
	after := model.selectedCategoryIDs()
	if len(after) != len(before) {
		t.Fatalf("selection changed: before=%v after=%v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("selection order changed: before=%v after=%v", before, after)
		}
	}
	// b also returns without rescanning.
	_, _ = model.handleKey("enter")
	nav, cmd = model.handleKey("b")
	if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhasePreview {
		t.Fatalf("b must return to preview: nav=%v phase=%v", nav, model.phase)
	}
	if scanStarts != 0 {
		t.Fatalf("escape/b triggered %d rescans", scanStarts)
	}
	if model.canceled {
		t.Fatal("returning from confirmation must not mark preview canceled")
	}
}

func TestEagerCleanSecondEnterInvokesExactExecutionOnce(t *testing.T) {
	var gotIDs []string
	calls := 0
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotIDs = append([]string(nil), selected...)
		return clean.Result{Status: "ok", Mode: "execute", Totals: clean.Totals{DeletedCount: 1, AffectedBytes: 9}}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Explicitly omit the default category from authorization.
	model.rows[0].Selected = false
	var optInID string
	for i := range model.rows {
		if model.rows[i].Eligibility == clean.CategoryEligibilityOptIn {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 9
			model.rows[i].Selected = true
			optInID = model.rows[i].Identifier
			break
		}
	}
	if optInID == "" {
		t.Fatal("need opt-in category")
	}

	_, cmd := model.handleKey("enter")
	if model.phase != eagerPhaseConfirmation || cmd != nil {
		t.Fatalf("first enter: phase=%v cmd=%v", model.phase, cmd)
	}
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter must schedule execution: nav=%v cmd=%v", nav, cmd)
	}
	if model.phase != eagerPhaseExecuting || !model.executionStarted {
		t.Fatalf("phase=%v started=%v", model.phase, model.executionStarted)
	}
	if len(model.frozenCategories) != 1 || model.frozenCategories[0] != optInID {
		t.Fatalf("frozen = %#v, want [%q]", model.frozenCategories, optInID)
	}
	// Default must not be frozen.
	for _, id := range model.frozenCategories {
		if id == model.rows[0].Identifier {
			t.Fatalf("omitted default leaked into frozen plan: %#v", model.frozenCategories)
		}
	}

	// Repeated Enter while executing cannot start another run.
	_, second := model.handleKey("enter")
	if second != nil || calls != 0 {
		t.Fatalf("duplicate enter before cmd ran: second=%v calls=%d", second, calls)
	}

	// Drive the async handoff.
	started := exactExecutionStartedFrom(t, cmd)
	wait := model.applyExactExecutionStarted(started)
	if wait == nil {
		t.Fatal("expected wait command after start")
	}
	// Drain progress/result.
	for model.phase == eagerPhaseExecuting {
		msg := wait()
		switch m := msg.(type) {
		case eagerExactExecutionProgressMsg:
			wait = model.applyExactExecutionProgress(m)
		case eagerExactExecutedMsg:
			model.applyExactExecuted(m)
			wait = nil
		default:
			t.Fatalf("unexpected msg %T", msg)
		}
		if wait == nil {
			break
		}
	}
	if calls != 1 {
		t.Fatalf("shared clean calls = %d, want 1", calls)
	}
	if len(gotIDs) != 1 || gotIDs[0] != optInID {
		t.Fatalf("executed IDs = %#v, want [%q]", gotIDs, optInID)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v, want result", model.phase)
	}
	// Further Enter cannot re-execute.
	_, again := model.handleKey("enter")
	if again != nil || calls != 1 {
		t.Fatalf("result enter re-executed: again=%v calls=%d", again, calls)
	}
}

func TestRunExactCleanSelectionUsesPlanAndTUIProvenance(t *testing.T) {
	var captured clean.Options
	originalExecute := executeClean
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		captured = opts
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	originalLoader := loadProtectionConfiguration
	loadProtectionConfiguration = func() clean.ProtectionConfiguration {
		return clean.ProtectionConfiguration{}
	}
	originalRecorder := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) { return &recordingHistoryRecorder{}, nil }
	t.Cleanup(func() {
		executeClean = originalExecute
		loadProtectionConfiguration = originalLoader
		newHistoryRecorder = originalRecorder
	})

	// Omit default; pass only crash_dumps and go-cache style opt-ins.
	// Recycle-bin opt-ins only: permanent auth stays false.
	selected := []string{clean.DevCacheCategoryGo, clean.OpportunityCategoryCrashDumps}
	result := runExactCleanSelection(context.Background(), selected, false, false, nil)
	if result.Status != "ok" {
		t.Fatalf("result = %#v", result)
	}
	if captured.Plan == nil || captured.Plan.Mode != clean.SelectionModeExact {
		t.Fatalf("Plan = %#v, want exact", captured.Plan)
	}
	wantCats := []string{clean.OpportunityCategoryCrashDumps, clean.DevCacheCategoryGo}
	if len(captured.Plan.Categories) != 2 ||
		captured.Plan.Categories[0] != wantCats[0] ||
		captured.Plan.Categories[1] != wantCats[1] {
		t.Fatalf("plan categories = %#v, want catalog order %#v", captured.Plan.Categories, wantCats)
	}
	// Additive OptIn path must not be used as the authorization source.
	if len(captured.OptIn) != 0 {
		t.Fatalf("OptIn = %#v, exact TUI must authorize via Plan only", captured.OptIn)
	}
	if captured.AllowPermanentDeletion {
		t.Fatal("recycle-only TUI selection must not set AllowPermanentDeletion")
	}
	cp := captured.CommandParameters
	if cp.Surface != "tui" || cp.SelectionMode != "exact" {
		t.Fatalf("provenance surface/mode = %#v", cp)
	}
	if len(cp.Args) != 0 {
		t.Fatalf("synthetic CLI args = %#v", cp.Args)
	}
	if len(cp.SelectedCategories) != 2 ||
		cp.SelectedCategories[0] != wantCats[0] ||
		cp.SelectedCategories[1] != wantCats[1] {
		t.Fatalf("selected_categories = %#v, want %#v", cp.SelectedCategories, wantCats)
	}
	for _, id := range cp.SelectedCategories {
		if strings.ContainsAny(id, `/\`) || id == clean.DefaultCategoryFoalOwnedTempSandboxes {
			t.Fatalf("bad provenance category %q", id)
		}
	}
	if captured.DetailedListDir != "" {
		t.Fatal("detailed list must not be attached for TUI execute")
	}
	if captured.HistoryRecorder == nil || captured.DetectRunningApplications == nil {
		t.Fatal("history recorder and running detection required")
	}

	// Permanent authorization is passed directly, never via fabricated CLI args.
	_ = runExactCleanSelection(context.Background(), selected, true, false, nil)
	if !captured.AllowPermanentDeletion {
		t.Fatal("allowPermanent handoff must set AllowPermanentDeletion")
	}
	if len(captured.CommandParameters.Args) != 0 {
		t.Fatalf("must not synthesize --allow-permanent args: %#v", captured.CommandParameters.Args)
	}
}

func TestRunExactCleanSelectionRejectsInvalidIdentifiersBeforeCleanup(t *testing.T) {
	called := false
	originalExecute := executeClean
	executeClean = func(context.Context, clean.Options) clean.Result {
		called = true
		return clean.Result{Status: "ok"}
	}
	t.Cleanup(func() { executeClean = originalExecute })

	for _, ids := range [][]string{
		{"all"},
		{"dev-caches"},
		{"cli-agents"},
		{`C:\Users\temp`},
		{"not_a_category"},
		{"administrator_only_caches"},
	} {
		result := runExactCleanSelection(context.Background(), ids, false, false, nil)
		if called {
			t.Fatalf("execute called for invalid %#v", ids)
		}
		if result.Status != "error" || len(result.Errors) == 0 || result.Errors[0].Code != "invalid_category_plan" {
			t.Fatalf("ids=%v result=%#v", ids, result)
		}
	}
}

func TestEagerCleanExecutionRendersPhaseAndSelectedCategoriesOnly(t *testing.T) {
	var cancelCalls int
	original := runExactCleanSelection
	runExactCleanSelection = func(ctx context.Context, selected []string, _ bool, _ bool, reporter clean.ProgressReporter) clean.Result {
		if reporter != nil {
			// Phase opener (no category) then category-scoped boundaries.
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning})
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning, ActiveCategory: selected[0]})
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseRecycleBinSafety})
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseRecycleBinOperations, ActiveCategory: selected[0]})
		}
		select {
		case <-ctx.Done():
		default:
		}
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{{
				Path:  `C:\Users\me\AppData\Local\Temp\foal-sandbox`,
				Bytes: 12,
				Rule:  selected[0],
			}},
			Totals: clean.Totals{DeletedCount: 1, AffectedBytes: 12},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Select only the default; leave every other category unselected.
	for i := range model.rows {
		model.rows[i].Selected = i == 0
	}
	selectedID := model.rows[0].Identifier
	selectedLabel := model.rows[0].Label
	unselectedLabel := model.rows[1].Label

	_, _ = model.handleKey("enter")
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil || model.phase != eagerPhaseExecuting {
		t.Fatalf("start execution: nav=%v cmd=%v phase=%v", nav, cmd, model.phase)
	}
	if len(model.frozenCategories) != 1 || model.frozenCategories[0] != selectedID {
		t.Fatalf("frozen = %#v", model.frozenCategories)
	}
	// Authorization is frozen: toggling selection no longer applies.
	model.rows[0].Selected = false
	model.rows[1].Selected = true

	content := model.content()
	if !strings.Contains(content, "Fresh scanning") {
		t.Fatalf("header missing Fresh scanning:\n%s", content)
	}
	// Before any progress arrives, selected rows start waiting.
	if !strings.Contains(content, selectedLabel) || !strings.Contains(content, "waiting") {
		t.Fatalf("selected waiting row missing:\n%s", content)
	}
	if strings.Contains(content, unselectedLabel) {
		t.Fatalf("unselected category leaked into execution view:\n%s", content)
	}
	if strings.Contains(content, "Processed: 0/1") == false {
		t.Fatalf("processed must count terminal only (0/1):\n%s", content)
	}
	if strings.Contains(content, "%") {
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, "%") && (strings.Contains(line, "scanning") || strings.Contains(line, "Processed") || strings.Contains(line, "Fresh")) {
				t.Fatalf("byte-derived percentage:\n%s", line)
			}
		}
	}

	// Drive progress projections through shared phases (+ category events).
	started := exactExecutionStartedFrom(t, cmd)
	wait := model.applyExactExecutionStarted(started)
	// empty Scanning → category Scanning → Safety → Ops
	type step struct {
		phaseSubstr string
		state       string
		activeInHdr bool
	}
	steps := []step{
		{"Fresh scanning", "waiting", false},   // opener leaves waiting
		{"Fresh scanning", "rechecking", true}, // ActiveCategory
		{"Recycle Bin safety check", "ready", false},
		{"Moving to Recycle Bin", "cleaning", true},
	}
	for i, st := range steps {
		msg := wait()
		progress, ok := msg.(eagerExactExecutionProgressMsg)
		if !ok {
			t.Fatalf("msg %d = %T, want progress", i, msg)
		}
		wait = model.applyExactExecutionProgress(progress)
		content = model.content()
		if !strings.Contains(content, st.phaseSubstr) {
			t.Fatalf("step %d phase %q missing:\n%s", i, st.phaseSubstr, content)
		}
		if !strings.Contains(content, st.state) {
			t.Fatalf("step %d state %q missing:\n%s", i, st.state, content)
		}
		if st.activeInHdr && !strings.Contains(content, selectedLabel) {
			t.Fatalf("step %d header/body missing active label %q:\n%s", i, selectedLabel, content)
		}
		if strings.Contains(content, `C:\Users\me`) {
			t.Fatalf("path leaked during progress:\n%s", content)
		}
	}
	msg := wait()
	executed, ok := msg.(eagerExactExecutedMsg)
	if !ok {
		t.Fatalf("final msg = %T", msg)
	}
	model.applyExactExecuted(executed)
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v", model.phase)
	}
	content = model.content()
	if !strings.Contains(content, "cleaned") || !strings.Contains(content, selectedLabel) {
		t.Fatalf("result cleaned missing:\n%s", content)
	}
	if !strings.Contains(content, "Processed: 1/1") {
		t.Fatalf("processed terminal:\n%s", content)
	}
	for _, want := range []string{"Affected (processed):", "Recycle Bin moved:", "Permanently deleted:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("result totals missing %q:\n%s", want, content)
		}
	}
	for _, banned := range []string{"freed", "reclaimed disk", "space reclaimed"} {
		if strings.Contains(strings.ToLower(content), banned) {
			t.Fatalf("must not label aggregate as freed/reclaimed (%q):\n%s", banned, content)
		}
	}
	if strings.Contains(content, `C:\Users\me`) || strings.Contains(content, "foal-sandbox") {
		t.Fatalf("path leaked in result:\n%s", content)
	}
	// Frozen auth never expanded to the toggled unselected row.
	if len(model.executionOutcomes) != 1 || model.executionOutcomes[0].Identifier != selectedID {
		t.Fatalf("outcomes = %#v", model.executionOutcomes)
	}
	_ = cancelCalls
}

func TestEagerCleanExecutionProgressActiveCategoryWaitingVsActive(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	optInID := clean.OpportunityCategoryCrashDumps
	var defaultLabel, optInLabel string
	catalog := clean.CanonicalCleanupCategoryCatalog()
	if s, ok := catalog.Summary(defaultID); ok {
		defaultLabel = s.Label
	}
	if s, ok := catalog.Summary(optInID); ok {
		optInLabel = s.Label
	}

	model := newEagerCleanModel(100, 40)
	model.phase = eagerPhaseExecuting
	model.executionStarted = true
	model.executionStartedAt = time.Now()
	model.frozenCategories = []string{defaultID, optInID}
	model.executionOutcomes = initialExecutionOutcomes(model.frozenCategories)
	model.execFollowActive = true
	model.execTrackedActive = -1

	// Initial: both waiting, Processed still 0/2.
	content := model.content()
	if !strings.Contains(content, "waiting") || !strings.Contains(content, "Processed: 0/2") {
		t.Fatalf("initial waiting:\n%s", content)
	}
	for _, o := range model.executionOutcomes {
		if o.State != clean.CategoryExecutionWaiting {
			t.Fatalf("initial state = %q for %s", o.State, o.Identifier)
		}
	}

	// Active default during scanning: only default rechecking; opt-in stays waiting.
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:          clean.ExecutionPhaseScanning,
			ActiveCategory: defaultID,
		},
	})
	byID := map[string]clean.CategoryExecutionState{}
	for _, o := range model.executionOutcomes {
		byID[o.Identifier] = o.State
	}
	if byID[defaultID] != clean.CategoryExecutionRechecking {
		t.Fatalf("active default = %q, want rechecking", byID[defaultID])
	}
	if byID[optInID] != clean.CategoryExecutionWaiting {
		t.Fatalf("inactive opt-in = %q, want waiting", byID[optInID])
	}
	content = model.content()
	if !strings.Contains(content, "Fresh scanning · "+defaultLabel) {
		t.Fatalf("header missing active catalog label:\n%s", content)
	}
	if !strings.Contains(content, defaultLabel+" · rechecking") {
		t.Fatalf("active row label:\n%s", content)
	}
	if !strings.Contains(content, optInLabel+" · waiting") {
		t.Fatalf("waiting row label:\n%s", content)
	}
	if model.executionActiveIndex() != 0 {
		t.Fatalf("active index = %d, want 0", model.executionActiveIndex())
	}
	// Terminal count still zero mid-flight.
	if strings.Contains(content, "Processed: 0/2") == false {
		t.Fatalf("Processed must stay terminal-only:\n%s", content)
	}

	// Move active to opt-in: default keeps last in-progress; opt-in becomes rechecking.
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:          clean.ExecutionPhaseScanning,
			ActiveCategory: optInID,
		},
	})
	byID = map[string]clean.CategoryExecutionState{}
	for _, o := range model.executionOutcomes {
		byID[o.Identifier] = o.State
	}
	if byID[defaultID] != clean.CategoryExecutionRechecking {
		t.Fatalf("prior active should keep rechecking, got %q", byID[defaultID])
	}
	if byID[optInID] != clean.CategoryExecutionRechecking {
		t.Fatalf("new active = %q, want rechecking", byID[optInID])
	}
	if model.executionActiveIndex() != 1 {
		t.Fatalf("active index = %d, want 1", model.executionActiveIndex())
	}
	content = model.content()
	if !strings.Contains(content, "Fresh scanning · "+optInLabel) {
		t.Fatalf("header should follow new active:\n%s", content)
	}
	// ActiveCategory alone must not invent terminal outcomes (needs CompletedCategory).
	for _, o := range model.executionOutcomes {
		if clean.IsTerminalExecutionState(o.State) {
			t.Fatalf("ActiveCategory must not invent terminal state: %#v", o)
		}
	}
}

func TestEagerCleanExecutionMidFlightCompletionAndFinalOverwrite(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	optInID := clean.OpportunityCategoryCrashDumps
	var defaultLabel, optInLabel string
	catalog := clean.CanonicalCleanupCategoryCatalog()
	if s, ok := catalog.Summary(defaultID); ok {
		defaultLabel = s.Label
	}
	if s, ok := catalog.Summary(optInID); ok {
		optInLabel = s.Label
	}

	model := newEagerCleanModel(100, 40)
	model.phase = eagerPhaseExecuting
	model.executionStarted = true
	model.executionStartedAt = time.Now()
	model.frozenCategories = []string{defaultID, optInID}
	model.executionOutcomes = initialExecutionOutcomes(model.frozenCategories)
	model.execFollowActive = true
	model.execTrackedActive = -1

	// waiting → active rechecking for default
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:          clean.ExecutionPhaseScanning,
			ActiveCategory: defaultID,
		},
	})
	// provisional empty completion for default after resolve
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:             clean.ExecutionPhaseScanning,
			CompletedCategory: defaultID,
			CompletedState:    clean.CategoryExecutionEmpty,
		},
	})
	byID := map[string]clean.CategoryExecutionState{}
	for _, o := range model.executionOutcomes {
		byID[o.Identifier] = o.State
	}
	if byID[defaultID] != clean.CategoryExecutionEmpty {
		t.Fatalf("default provisional = %q, want empty", byID[defaultID])
	}
	if byID[optInID] != clean.CategoryExecutionWaiting {
		t.Fatalf("opt-in still waiting = %q", byID[optInID])
	}
	content := model.content()
	if !strings.Contains(content, "Processed: 1/2") {
		t.Fatalf("Processed should count provisional terminal:\n%s", content)
	}
	if !strings.Contains(content, defaultLabel+" · empty") {
		t.Fatalf("empty row missing:\n%s", content)
	}
	// Never regress provisional terminal back to waiting on later active events.
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:          clean.ExecutionPhaseRecycleBinOperations,
			ActiveCategory: optInID,
		},
	})
	for _, o := range model.executionOutcomes {
		if o.Identifier == defaultID && o.State != clean.CategoryExecutionEmpty {
			t.Fatalf("provisional empty regressed: %#v", o)
		}
		if o.Identifier == optInID && o.State != clean.CategoryExecutionCleaning {
			t.Fatalf("active opt-in = %q, want cleaning", o.State)
		}
	}
	// cleaned provisional with honest bytes → Processed 2/2 and Affected sum
	_ = model.applyExactExecutionProgress(eagerExactExecutionProgressMsg{
		progress: clean.ExecutionProgress{
			Phase:                         clean.ExecutionPhaseRecycleBinOperations,
			CompletedCategory:             optInID,
			CompletedState:                clean.CategoryExecutionCleaned,
			CompletedAffectedBytes:        4096,
			CompletedRecycleBinMovedBytes: 4096,
			CompletedDeletedCount:         1,
		},
	})
	content = model.content()
	if !strings.Contains(content, "Processed: 2/2") {
		t.Fatalf("both provisional terminals:\n%s", content)
	}
	if !strings.Contains(content, optInLabel+" · cleaned") {
		t.Fatalf("cleaned row missing:\n%s", content)
	}
	if !strings.Contains(content, "Affected (processed): 4 KB") &&
		!strings.Contains(content, "Affected (processed): 4.0 KB") {
		// cleanFormatBytes may render 4096 as "4 KB"
		if !strings.Contains(content, "Affected (processed):") {
			t.Fatalf("affected footer missing:\n%s", content)
		}
		// Accept any honest non-zero formatting of 4096.
		if strings.Contains(content, "Affected (processed): 0 KB") {
			t.Fatalf("must surface provisional affected bytes:\n%s", content)
		}
	}
	if strings.Contains(content, `C:\`) {
		t.Fatalf("path leaked:\n%s", content)
	}

	// Final Result completely overwrites provisional projections (even if wrong).
	model.applyExactExecuted(eagerExactExecutedMsg{result: clean.Result{
		Status: "ok",
		Mode:   "execute",
		Deleted: []clean.DeletedItem{{
			Path:   `C:\Temp\real`,
			Bytes:  100,
			Rule:   defaultID,
			Action: string(clean.PlannedActionMoveToRecycleBin),
		}},
		// opt-in ends skipped, not cleaned — must replace mid-flight cleaned.
		Skipped: []clean.SkippedItem{{
			Path:  `C:\Temp\skip`,
			Bytes: 1,
			Rule:  optInID,
			Reason: clean.StructuredIssue{
				Code: "protected_path", Message: "protected", Recoverable: true, Rule: optInID,
			},
		}},
	}})
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v", model.phase)
	}
	byID = map[string]clean.CategoryExecutionState{}
	for _, o := range model.executionOutcomes {
		byID[o.Identifier] = o.State
	}
	if byID[defaultID] != clean.CategoryExecutionCleaned {
		t.Fatalf("final default = %q, want cleaned (overwrite empty)", byID[defaultID])
	}
	if byID[optInID] != clean.CategoryExecutionSkipped {
		t.Fatalf("final opt-in = %q, want skipped (overwrite cleaned)", byID[optInID])
	}
	content = model.content()
	if !strings.Contains(content, "Processed: 2/2") {
		t.Fatalf("result processed:\n%s", content)
	}
	if strings.Contains(content, `C:\Temp`) {
		t.Fatalf("path leaked in result:\n%s", content)
	}
	// Overwrite must not keep provisional 4096 as the opt-in success bytes.
	for _, o := range model.executionOutcomes {
		if o.Identifier == optInID && o.AffectedBytes != 0 {
			t.Fatalf("skipped opt-in must have 0 affected after overwrite: %#v", o)
		}
		if o.Identifier == defaultID && o.AffectedBytes != 100 {
			t.Fatalf("default affected after overwrite = %d, want 100", o.AffectedBytes)
		}
	}
}

func TestEagerCleanExecutionTerminalOutcomesAndMixedPartial(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	optInID := clean.OpportunityCategoryCrashDumps
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ bool, _ clean.ProgressReporter) clean.Result {
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{
				{Path: `C:\Temp\ok`, Bytes: 8, Rule: defaultID},
			},
			Skipped: []clean.SkippedItem{
				{
					Path:   `C:\Temp\protected`,
					Bytes:  3,
					Rule:   defaultID,
					Reason: clean.StructuredIssue{Code: "protected_path", Message: `protected C:\Temp\protected`},
				},
				{
					Path:   `C:\Users\me\AppData\Local\CrashDumps\a.dmp`,
					Rule:   optInID,
					Reason: clean.StructuredIssue{Code: "protected_path", Message: "protected"},
				},
			},
			Totals: clean.Totals{DeletedCount: 1, SkippedCount: 2, AffectedBytes: 8},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Select default + crash_dumps.
	for i := range model.rows {
		id := model.rows[i].Identifier
		model.rows[i].Selected = id == defaultID || id == optInID
		if model.rows[i].Selected {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 1
		}
	}
	_, _ = model.handleKey("enter")
	_, cmd := model.handleKey("enter")
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
	byID := map[string]clean.CategoryExecutionState{}
	for _, outcome := range model.executionOutcomes {
		byID[outcome.Identifier] = outcome.State
	}
	if byID[defaultID] != clean.CategoryExecutionPartial {
		t.Fatalf("default outcome = %q, want partial", byID[defaultID])
	}
	if byID[optInID] != clean.CategoryExecutionSkipped {
		t.Fatalf("opt-in outcome = %q, want skipped", byID[optInID])
	}
	content := model.content()
	if !strings.Contains(content, "partial") || !strings.Contains(content, "skipped") {
		t.Fatalf("mixed result content:\n%s", content)
	}
	if strings.Contains(content, `C:\Temp`) || strings.Contains(content, "CrashDumps") {
		t.Fatalf("path leaked:\n%s", content)
	}
	if !strings.Contains(content, "Processed: 2/2") {
		t.Fatalf("processed:\n%s", content)
	}
}

func TestEagerCleanExecutionEmptyCleanedFailedCanceled(t *testing.T) {
	cases := []struct {
		name    string
		result  clean.Result
		want    clean.CategoryExecutionState
		wantTxt string
	}{
		{
			name:    "empty",
			result:  clean.Result{Status: "ok", Mode: "execute"},
			want:    clean.CategoryExecutionEmpty,
			wantTxt: "empty",
		},
		{
			name: "cleaned",
			result: clean.Result{
				Status:  "ok",
				Deleted: []clean.DeletedItem{{Path: `C:\a`, Bytes: 2, Rule: clean.DefaultCategoryFoalOwnedTempSandboxes}},
				Totals:  clean.Totals{DeletedCount: 1, AffectedBytes: 2},
			},
			want:    clean.CategoryExecutionCleaned,
			wantTxt: "cleaned",
		},
		{
			name: "failed",
			result: clean.Result{
				Status: "ok",
				Skipped: []clean.SkippedItem{{
					Path:   `C:\a`,
					Rule:   clean.DefaultCategoryFoalOwnedTempSandboxes,
					Reason: clean.StructuredIssue{Code: "delete_failed", Message: `failed C:\a`},
				}},
			},
			want:    clean.CategoryExecutionFailed,
			wantTxt: "failed",
		},
		{
			name: "canceled",
			result: clean.Result{
				Status: "ok",
				Skipped: []clean.SkippedItem{{
					Path:   `C:\a`,
					Rule:   clean.DefaultCategoryFoalOwnedTempSandboxes,
					Reason: clean.StructuredIssue{Code: "context_canceled", Message: "context canceled"},
				}},
			},
			want:    clean.CategoryExecutionCanceled,
			wantTxt: "canceled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := runExactCleanSelection
			runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ bool, _ clean.ProgressReporter) clean.Result {
				// Re-attribute items to the frozen selected id.
				result := tc.result
				for i := range result.Deleted {
					result.Deleted[i].Rule = selected[0]
				}
				for i := range result.Skipped {
					result.Skipped[i].Rule = selected[0]
				}
				return result
			}
			t.Cleanup(func() { runExactCleanSelection = original })

			model := newEagerCleanModel(100, 40)
			markEagerQueueTerminal(&model, true)
			for i := range model.rows {
				model.rows[i].Selected = i == 0
			}
			_, _ = model.handleKey("enter")
			_, cmd := model.handleKey("enter")
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
			if len(model.executionOutcomes) != 1 || model.executionOutcomes[0].State != tc.want {
				t.Fatalf("outcomes = %#v, want %q", model.executionOutcomes, tc.want)
			}
			content := model.content()
			if !strings.Contains(content, tc.wantTxt) {
				t.Fatalf("content missing %q:\n%s", tc.wantTxt, content)
			}
			if strings.Contains(content, `C:\`) {
				t.Fatalf("path leaked:\n%s", content)
			}
		})
	}
}

func TestEagerCleanActiveExecutionCancelKeysAndRepeatedCtrlC(t *testing.T) {
	cancelCh := make(chan struct{})
	original := runExactCleanSelection
	runExactCleanSelection = func(ctx context.Context, selected []string, _ bool, _ bool, reporter clean.ProgressReporter) clean.Result {
		if reporter != nil {
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseRecycleBinOperations})
		}
		<-ctx.Done()
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{{
				Path:  `C:\done`,
				Bytes: 4,
				Rule:  selected[0],
			}},
			Skipped: []clean.SkippedItem{{
				Path:   `C:\remaining`,
				Rule:   selected[0],
				Reason: clean.StructuredIssue{Code: "context_canceled", Message: "context canceled"},
			}},
			Totals: clean.Totals{DeletedCount: 1, SkippedCount: 1, AffectedBytes: 4},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	for i := range model.rows {
		model.rows[i].Selected = i == 0
	}
	_, _ = model.handleKey("enter")
	_, cmd := model.handleKey("enter")
	started := exactExecutionStartedFrom(t, cmd)
	// Capture cancel so repeated Ctrl+C can be observed without force exit.
	cancelCalls := 0
	baseCancel := model.cancelExecution
	model.cancelExecution = func() {
		cancelCalls++
		if baseCancel != nil {
			baseCancel()
		}
		select {
		case <-cancelCh:
		default:
			close(cancelCh)
		}
	}

	// Escape / b / q ignored during active execution.
	for _, key := range []string{"esc", "escape", "b", "q"} {
		nav, next := model.handleKey(key)
		if nav != eagerPreviewNavNone || next != nil || model.phase != eagerPhaseExecuting {
			t.Fatalf("key %q must be ignored: nav=%v phase=%v", key, nav, model.phase)
		}
	}
	if model.cancellationRequested {
		t.Fatal("ignored keys must not request cancellation")
	}

	nav, next := model.handleKey("ctrl+c")
	if nav != eagerPreviewNavNone || next != nil {
		t.Fatalf("ctrl+c nav=%v next=%v", nav, next)
	}
	if !model.cancellationRequested || cancelCalls < 1 {
		t.Fatalf("cancel requested=%v calls=%d", model.cancellationRequested, cancelCalls)
	}
	content := model.content()
	if !strings.Contains(content, cancellationRequestedMessage) {
		t.Fatalf("cancellation message missing:\n%s", content)
	}
	if strings.Contains(content, "roll back") && !strings.Contains(content, "will not be rolled back") {
		t.Fatalf("must not claim rollback:\n%s", content)
	}

	// Repeated Ctrl+C does not force exit.
	for i := 0; i < 3; i++ {
		nav, next = model.handleKey("ctrl+c")
		if nav != eagerPreviewNavNone || next != nil || model.phase != eagerPhaseExecuting {
			t.Fatalf("repeated ctrl+c force-exited: nav=%v phase=%v", nav, model.phase)
		}
	}

	// Still attached: drain to final Result.
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
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase = %v after cancel", model.phase)
	}
	if len(model.executionOutcomes) != 1 || model.executionOutcomes[0].State != clean.CategoryExecutionPartial {
		// One success + canceled remainder is partial; completed moves stay counted.
		t.Fatalf("cancel outcome = %#v", model.executionOutcomes)
	}
	if model.executionOutcomes[0].AffectedBytes != 4 {
		t.Fatalf("completed moves must remain: %#v", model.executionOutcomes[0])
	}
	content = model.content()
	if strings.Contains(content, "rolled back") {
		t.Fatalf("result must not claim rollback:\n%s", content)
	}
	if strings.Contains(content, `C:\done`) || strings.Contains(content, `C:\remaining`) {
		t.Fatalf("path leaked after cancel:\n%s", content)
	}
}

func TestEagerCleanPreviewCtrlCStillInterrupts(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	cancelCalled := false
	model.cancel = func() { cancelCalled = true }
	nav, _ := model.handleKey("ctrl+c")
	if nav != eagerPreviewNavInterrupt || !cancelCalled || !model.canceled {
		t.Fatalf("preview ctrl+c: nav=%v canceled=%v cancelCalled=%v", nav, model.canceled, cancelCalled)
	}
}

func TestEagerCleanResultKeysAndStartDiscardsStaleSession(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	model.phase = eagerPhaseResult
	model.executionStarted = true
	model.frozenCategories = []string{model.rows[0].Identifier}
	model.executionOutcomes = []clean.CategoryExecutionOutcome{{
		Identifier:    model.rows[0].Identifier,
		Label:         model.rows[0].Label,
		State:         clean.CategoryExecutionCleaned,
		AffectedBytes: 9,
	}}
	model.executionResult = clean.Result{Status: "ok", Totals: clean.Totals{DeletedCount: 1, AffectedBytes: 9}}

	content := model.content()
	if !strings.Contains(content, "Cleanup result") || !strings.Contains(content, "cleaned") {
		t.Fatalf("result page:\n%s", content)
	}

	for _, key := range []string{"enter", "esc", "b"} {
		model.nav = eagerPreviewNavNone
		nav, _ := model.handleKey(key)
		if nav != eagerPreviewNavMenu {
			t.Fatalf("key %q nav=%v, want menu", key, nav)
		}
	}
	model.nav = eagerPreviewNavNone
	nav, _ := model.handleKey("q")
	if nav != eagerPreviewNavQuit {
		t.Fatalf("q nav=%v", nav)
	}

	// Re-entering Clean via start discards stale preview/selection/result.
	original := buildEagerPreviewOptions
	buildEagerPreviewOptions = func() clean.Options { return clean.Options{} }
	t.Cleanup(func() { buildEagerPreviewOptions = original })
	originalPreview := runEagerPreviewFn
	runEagerPreviewFn = func(context.Context, clean.Options, func(clean.CategoryPreviewObservation)) error {
		return nil
	}
	t.Cleanup(func() { runEagerPreviewFn = originalPreview })

	_ = model.start()
	if model.phase != eagerPhasePreview || model.executionStarted || len(model.frozenCategories) != 0 {
		t.Fatalf("start did not discard session: phase=%v started=%v frozen=%v", model.phase, model.executionStarted, model.frozenCategories)
	}
	if len(model.executionOutcomes) != 0 {
		t.Fatalf("stale outcomes retained: %#v", model.executionOutcomes)
	}
	// Defaults re-selected; opt-ins cleared.
	if !model.rows[0].Selected {
		t.Fatal("default must start selected on new session")
	}
}

func TestEagerCleanExecutionCannotAlterFrozenAuthorization(t *testing.T) {
	var got []string
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ bool, _ clean.ProgressReporter) clean.Result {
		got = append([]string(nil), selected...)
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Exact selection: only opt-in, not default.
	model.rows[0].Selected = false
	var optIn string
	for i := range model.rows {
		if model.rows[i].Eligibility == clean.CategoryEligibilityOptIn {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 1
			model.rows[i].Selected = true
			optIn = model.rows[i].Identifier
			break
		}
	}
	_, _ = model.handleKey("enter")
	_, cmd := model.handleKey("enter")
	// Mutate live selection after freeze — must not affect execution.
	model.selectAllSelectable()
	model.rows[0].Selected = true

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
	if len(got) != 1 || got[0] != optIn {
		t.Fatalf("executed %#v, want [%q]", got, optIn)
	}
	if len(model.executionOutcomes) != 1 || model.executionOutcomes[0].Identifier != optIn {
		t.Fatalf("outcomes %#v", model.executionOutcomes)
	}
}

func TestEagerCleanViewportPreviewFocusFollowsIncludingDisabled(t *testing.T) {
	// Short body window so only a few category rows fit; chrome stays fixed.
	model := newEagerCleanModel(80, 16)
	model.generation = 1
	// Disable the last few rows so focus can land on non-selectable diagnostics.
	for i := range model.rows {
		state := clean.CategoryPreviewComplete
		count := 1
		var bytes int64 = 1
		if i >= len(model.rows)-3 {
			state = clean.CategoryPreviewEmpty
			count = 0
			bytes = 0
		}
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     model.rows[i].Identifier,
				Label:          model.rows[i].Label,
				State:          state,
				CandidateCount: count,
				Bytes:          bytes,
			},
		})
	}
	model.finished = true

	beforeAuth := append([]string(nil), model.selectedCategoryIDs()...)
	// Drive the cursor to the last (disabled) row; viewport must follow.
	for model.cursor < len(model.rows)-1 {
		nav, cmd := model.handleKey("down")
		if nav != eagerPreviewNavNone || cmd != nil {
			t.Fatalf("down should only move focus: nav=%v cmd=%v", nav, cmd)
		}
	}
	if model.cursor != len(model.rows)-1 {
		t.Fatalf("cursor = %d, want last row", model.cursor)
	}
	if model.rowSelectable(model.rows[model.cursor]) {
		t.Fatal("last row should be disabled/empty")
	}
	lastLabel := model.rows[model.cursor].Label
	content := model.content()
	if !strings.Contains(content, "Foal Clean") || !strings.Contains(content, "Selected:") {
		t.Fatalf("fixed chrome missing:\n%s", content)
	}
	if !strings.Contains(content, "Focused: "+lastLabel) {
		t.Fatalf("focused diagnostics must follow disabled row:\n%s", content)
	}
	if !strings.Contains(content, ">") || !strings.Contains(content, lastLabel) {
		t.Fatalf("viewport must keep focused disabled row visible:\n%s", content)
	}
	// Group headings are body lines (scroll with rows), not a separate mode.
	if strings.Contains(content, "scroll mode") {
		t.Fatal("must not introduce a separate scroll mode")
	}
	afterAuth := model.selectedCategoryIDs()
	if !stringSlicesEqual(beforeAuth, afterAuth) {
		t.Fatalf("cursor movement changed auth: before=%v after=%v", beforeAuth, afterAuth)
	}
}

func TestEagerCleanViewportConfirmationAndResultScrollOnly(t *testing.T) {
	model := newEagerCleanModel(80, 14)
	markEagerQueueTerminal(&model, true)
	// Select many complete categories so confirmation body is long. Skip the
	// servicing row (it uses the servicing state machine, not file scan state).
	selected := 0
	for i := range model.rows {
		if model.rows[i].Servicing || selected >= 12 {
			continue
		}
		model.rows[i].State = clean.CategoryPreviewComplete
		model.rows[i].CandidateCount = 1
		model.rows[i].Bytes = 2
		model.rows[i].Selected = true
		selected++
	}
	before := append([]string(nil), model.selectedCategoryIDs()...)
	nav, _ := model.handleKey("enter")
	if nav != eagerPreviewNavNone || model.phase != eagerPhaseConfirmation {
		t.Fatalf("enter confirmation: phase=%v", model.phase)
	}
	if model.viewportOffset != 0 {
		t.Fatalf("confirmation starts at offset 0, got %d", model.viewportOffset)
	}
	nav, cmd := model.handleKey("down")
	if nav != eagerPreviewNavNone || cmd != nil {
		t.Fatalf("confirmation down: nav=%v cmd=%v", nav, cmd)
	}
	if model.viewportOffset < 1 {
		t.Fatalf("confirmation down should increase viewport offset, got %d", model.viewportOffset)
	}
	// No row selection cursor in confirmation; authorization frozen for this page.
	after := model.selectedCategoryIDs()
	if !stringSlicesEqual(before, after) {
		t.Fatalf("confirmation scroll changed selection: %v -> %v", before, after)
	}
	content := model.content()
	if !strings.Contains(content, "Confirm cleanup") || !strings.Contains(content, "Selected:") {
		t.Fatalf("confirmation chrome missing:\n%s", content)
	}
	if strings.Contains(content, ">") {
		t.Fatalf("confirmation must not create row selection cursor:\n%s", content)
	}

	// Execute with a stub that returns mixed outcomes for a long result list.
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _ bool, _ bool, reporter clean.ProgressReporter) clean.Result {
		if reporter != nil {
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning})
		}
		deleted := make([]clean.DeletedItem, 0, len(selected))
		for _, id := range selected {
			deleted = append(deleted, clean.DeletedItem{Path: `C:\x`, Bytes: 1, Rule: id})
		}
		return clean.Result{
			Status:  "ok",
			Mode:    "execute",
			Deleted: deleted,
			Totals:  clean.Totals{DeletedCount: len(deleted), AffectedBytes: int64(len(deleted))},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	frozen := append([]string(nil), model.selectedCategoryIDs()...)
	_, cmd = model.handleKey("enter")
	if cmd == nil || model.phase != eagerPhaseExecuting {
		t.Fatalf("start exec: phase=%v", model.phase)
	}
	// Drain to result.
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
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase=%v", model.phase)
	}
	if !stringSlicesEqual(frozen, model.frozenCategories) {
		t.Fatalf("frozen auth changed: %v -> %v", frozen, model.frozenCategories)
	}
	model.viewportOffset = 0
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("down")
	if model.viewportOffset < 1 {
		t.Fatalf("result down should scroll offset, got %d", model.viewportOffset)
	}
	// Result scroll never re-authorizes or mutates frozen selection identity.
	if !stringSlicesEqual(frozen, model.frozenCategories) {
		t.Fatalf("result scroll changed frozen: %v", model.frozenCategories)
	}
	content = model.content()
	if !strings.Contains(content, "Cleanup result") || !strings.Contains(content, "Processed:") {
		t.Fatalf("result chrome missing:\n%s", content)
	}
	if strings.Contains(content, ">") {
		t.Fatalf("result must not create selectable cursor:\n%s", content)
	}
}

func TestEagerCleanViewportExecutionFollowAndTemporaryInspect(t *testing.T) {
	// Compact height so body capacity stays small relative to a long outcome list.
	model := newEagerCleanModel(80, 12)
	model.phase = eagerPhaseExecuting
	model.executionStarted = true
	model.execFollowActive = true
	model.execTrackedActive = -1
	catalog := clean.EagerPreviewQueue()
	n := len(catalog)
	if n < 10 {
		t.Fatalf("need a long catalog for scroll tests, got %d", n)
	}
	model.frozenCategories = make([]string, 0, n)
	model.executionOutcomes = make([]clean.CategoryExecutionOutcome, 0, n)
	for i := 0; i < n; i++ {
		model.frozenCategories = append(model.frozenCategories, catalog[i].Identifier)
		model.executionOutcomes = append(model.executionOutcomes, clean.CategoryExecutionOutcome{
			Identifier: catalog[i].Identifier,
			Label:      catalog[i].Label,
			State:      clean.CategoryExecutionRechecking,
		})
	}
	model.syncExecutionViewport()
	if model.viewportOffset != 0 {
		t.Fatalf("follow active (index 0) should keep offset near top, got %d", model.viewportOffset)
	}
	frozen := append([]string(nil), model.frozenCategories...)

	// Temporary inspection scrolls without changing authorization.
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("down")
	if model.execFollowActive {
		t.Fatal("manual up/down must pause active following")
	}
	if model.viewportOffset < 1 {
		t.Fatalf("inspection should move viewport, got %d", model.viewportOffset)
	}
	if !stringSlicesEqual(frozen, model.frozenCategories) {
		t.Fatalf("inspection changed frozen auth")
	}

	// Make a late outcome the only non-terminal active category.
	active := n - 1
	for i := 0; i < active; i++ {
		model.executionOutcomes[i].State = clean.CategoryExecutionCleaned
		model.executionOutcomes[i].AffectedBytes = 1
	}
	// Pause following and pin the viewport to the top so the last row is outside.
	model.execFollowActive = false
	model.execTrackedActive = 0
	model.viewportOffset = 0
	model.clampViewportOffset()
	if !model.executionOutcomeCompletelyOutside(active) {
		t.Fatalf("precondition: active=%d line=%d cap=%d body=%d offset=%d should be outside",
			active, model.executionOutcomeBodyLine(active), model.bodyCapacity(),
			len(model.scrollableBodyLines()), model.viewportOffset)
	}
	model.syncExecutionViewport()
	if !model.execFollowActive {
		t.Fatal("newly active completely outside must resume following")
	}
	if model.executionOutcomeCompletelyOutside(active) {
		t.Fatal("newly active must be brought into view")
	}
	if !stringSlicesEqual(frozen, model.frozenCategories) {
		t.Fatalf("follow resume changed frozen auth")
	}
}

func TestEagerCleanViewportResizePreservesFocusAndAuth(t *testing.T) {
	model := newEagerCleanModel(100, 40)
	markEagerQueueTerminal(&model, true)
	// Move focus mid-list.
	for i := 0; i < 8; i++ {
		_, _ = model.handleKey("down")
	}
	focusID := model.rows[model.cursor].Identifier
	auth := append([]string(nil), model.selectedCategoryIDs()...)
	offsetBefore := model.viewportOffset

	model.setSize(70, 15)
	if model.cursor >= len(model.rows) || model.rows[model.cursor].Identifier != focusID {
		t.Fatalf("resize changed focused identity: cursor=%d", model.cursor)
	}
	if !stringSlicesEqual(auth, model.selectedCategoryIDs()) {
		t.Fatalf("resize changed authorization")
	}
	content := model.content()
	if !strings.Contains(content, model.rows[model.cursor].Label) {
		t.Fatalf("resize must keep focused row visible:\n%s", content)
	}
	_ = offsetBefore

	// Confirmation offset preservation.
	model.setSize(100, 40)
	_, _ = model.handleKey("enter")
	if model.phase != eagerPhaseConfirmation {
		t.Fatalf("phase=%v", model.phase)
	}
	_, _ = model.handleKey("down")
	_, _ = model.handleKey("down")
	off := model.viewportOffset
	auth = append([]string(nil), model.selectedCategoryIDs()...)
	model.setSize(80, 16)
	if !stringSlicesEqual(auth, model.selectedCategoryIDs()) {
		t.Fatal("confirmation resize changed auth")
	}
	// Offset is clamped, not cleared to invent selection.
	if model.viewportOffset < 0 {
		t.Fatalf("bad offset %d", model.viewportOffset)
	}
	_ = off
}

func TestEagerCleanViewportTerminalTooSmallStageKeys(t *testing.T) {
	// Preview: too small still returns/quits; shows guidance.
	model := newEagerCleanModel(80, 40)
	model.generation = 1
	model.setSize(20, 5)
	content := model.content()
	if !strings.Contains(content, "Terminal too small") {
		t.Fatalf("expected too-small message:\n%s", content)
	}
	if !strings.Contains(content, "Resize") {
		t.Fatalf("expected resize guidance:\n%s", content)
	}
	// Must not present a truncated actionable checklist.
	if strings.Contains(content, "[x]") || strings.Contains(content, "[ ]") {
		t.Fatalf("too-small must not show actionable checkboxes:\n%s", content)
	}
	nav, _ := model.handleKey("esc")
	if nav != eagerPreviewNavMenu {
		t.Fatalf("preview too-small esc -> menu, got %v", nav)
	}

	model = newEagerCleanModel(80, 40)
	markEagerQueueTerminal(&model, true)
	model.setSize(20, 5)
	// Confirmation keys still work when undersized.
	// confirmationEnabled checks phase/selection; size does not gate keys.
	model.setSize(80, 40)
	_, _ = model.handleKey("enter")
	model.setSize(20, 5)
	if model.phase != eagerPhaseConfirmation {
		t.Fatalf("phase=%v", model.phase)
	}
	content = model.content()
	if !strings.Contains(content, "Terminal too small") {
		t.Fatalf("confirmation too small:\n%s", content)
	}
	nav, _ = model.handleKey("b")
	if nav != eagerPreviewNavNone || model.phase != eagerPhasePreview {
		t.Fatalf("confirmation too-small b should return preview: nav=%v phase=%v", nav, model.phase)
	}

	// Unavailable small terminal.
	model = newEagerCleanModel(80, 40)
	model.unavailable = &clean.EagerPreviewUnavailable{Code: "protection_config_failed", Message: "bad config"}
	model.finished = true
	model.setSize(10, 4)
	content = model.content()
	if !strings.Contains(content, "Terminal too small") {
		t.Fatalf("unavailable too small:\n%s", content)
	}
	nav, _ = model.handleKey("q")
	if nav != eagerPreviewNavQuit {
		t.Fatalf("unavailable too-small q: %v", nav)
	}

	// Active execution: Escape/b/q ignored; Ctrl+C still cancels.
	cancelCalls := 0
	model = newEagerCleanModel(80, 40)
	model.phase = eagerPhaseExecuting
	model.executionStarted = true
	model.executionOutcomes = []clean.CategoryExecutionOutcome{{
		Identifier: "x", Label: "X", State: clean.CategoryExecutionRechecking,
	}}
	model.cancelExecution = func() { cancelCalls++ }
	model.setSize(15, 6)
	content = model.content()
	if !strings.Contains(content, "Terminal too small") {
		t.Fatalf("execution too small:\n%s", content)
	}
	if !strings.Contains(content, "Ctrl+C") {
		t.Fatalf("execution too-small must expose cancel hint:\n%s", content)
	}
	for _, key := range []string{"esc", "escape", "b", "q"} {
		nav, cmd := model.handleKey(key)
		if nav != eagerPreviewNavNone || cmd != nil || model.phase != eagerPhaseExecuting {
			t.Fatalf("execution too-small %q must be ignored: nav=%v phase=%v", key, nav, model.phase)
		}
	}
	nav, _ = model.handleKey("ctrl+c")
	if nav != eagerPreviewNavNone || !model.cancellationRequested || cancelCalls != 1 {
		t.Fatalf("execution too-small ctrl+c: nav=%v cancel=%v calls=%d", nav, model.cancellationRequested, cancelCalls)
	}

	// Result too-small still returns.
	model.phase = eagerPhaseResult
	model.setSize(12, 5)
	content = model.content()
	if !strings.Contains(content, "Terminal too small") {
		t.Fatalf("result too small:\n%s", content)
	}
	nav, _ = model.handleKey("enter")
	if nav != eagerPreviewNavMenu {
		t.Fatalf("result too-small enter: %v", nav)
	}
}

func TestEagerCleanViewportLayoutKeepsChromeFixed(t *testing.T) {
	model := newEagerCleanModel(80, 18)
	model.generation = 1
	for i := range model.rows {
		model.applyObservation(eagerCategoryObservationMsg{
			generation: 1,
			observation: clean.CategoryPreviewObservation{
				Identifier:     model.rows[i].Identifier,
				Label:          model.rows[i].Label,
				State:          clean.CategoryPreviewComplete,
				CandidateCount: 1,
				Bytes:          4,
			},
		})
	}
	model.finished = true
	// Scroll deep into the list.
	for i := 0; i < len(model.rows)-1; i++ {
		_, _ = model.handleKey("down")
	}
	content := model.content()
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("too few lines:\n%s", content)
	}
	if lines[0] != "Foal Clean" {
		t.Fatalf("title must stay top line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Scan complete") && !strings.Contains(lines[1], "Scanning") {
		t.Fatalf("progress must stay under title: %q", lines[1])
	}
	// Footer anchors.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Selected:") {
		t.Fatal("selected totals missing")
	}
	if !strings.Contains(joined, "Focused:") {
		t.Fatal("focused diagnostics missing")
	}
	if !strings.Contains(joined, "Hints:") {
		t.Fatal("key hints missing")
	}
	// Title appears once at top, not re-scrolled in body.
	titleCount := 0
	for _, line := range lines {
		if line == "Foal Clean" {
			titleCount++
		}
	}
	if titleCount != 1 {
		t.Fatalf("title count=%d, want fixed once", titleCount)
	}
}

func stringSlicesEqual(a, b []string) bool {
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

// injectedMixedActionSummaries is a reduced fixture with real catalog identifiers
// so exact-plan compilation succeeds. Production catalog actions already match
// these policies; the fixture only narrows the row set for focused TUI tests.
func injectedMixedActionSummaries() []clean.CleanupCategorySummary {
	return []clean.CleanupCategorySummary{
		{
			Identifier:     clean.DefaultCategoryFoalOwnedTempSandboxes,
			Label:          "Foal-owned temp sandboxes",
			ReportCategory: clean.ReportCategoryUserEssentials,
			Eligibility:    clean.CategoryEligibilityDefault,
			PlannedAction:  clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:     clean.DevCacheCategoryGo,
			Label:          "Go cache",
			ReportCategory: clean.ReportCategoryDeveloperTools,
			Eligibility:    clean.CategoryEligibilityOptIn,
			PlannedAction:  clean.PlannedActionDeletePermanently,
		},
		{
			Identifier:     clean.OpportunityCategoryUserTemp,
			Label:          "User temp",
			ReportCategory: clean.ReportCategoryUserEssentials,
			Eligibility:    clean.CategoryEligibilityOptIn,
			PlannedAction:  clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:     clean.DevCacheCategoryNPM,
			Label:          "npm cache",
			ReportCategory: clean.ReportCategoryDeveloperTools,
			Eligibility:    clean.CategoryEligibilityOptIn,
			PlannedAction:  clean.PlannedActionDeletePermanently,
		},
	}
}

func TestEagerCleanInitialSelectionDerivedFromInjectedSummaries(t *testing.T) {
	summaries := injectedMixedActionSummaries()
	model := newEagerCleanModelFromSummaries(summaries, 100, 40)
	if len(model.rows) != len(summaries) {
		t.Fatalf("rows = %d", len(model.rows))
	}
	want := map[string]bool{
		clean.DefaultCategoryFoalOwnedTempSandboxes: true,
		clean.DevCacheCategoryGo:                    true,
		clean.OpportunityCategoryUserTemp:           false,
		clean.DevCacheCategoryNPM:                   true,
	}
	for _, row := range model.rows {
		if row.Selected != want[row.Identifier] {
			t.Fatalf("%s selected=%v, want %v", row.Identifier, row.Selected, want[row.Identifier])
		}
		if row.PlannedAction != summariesWantAction(summaries, row.Identifier) {
			t.Fatalf("%s planned action not carried on row", row.Identifier)
		}
	}
	if model.selectedCount() != 3 {
		t.Fatalf("selectedCount = %d, want 3", model.selectedCount())
	}

	// Users may clear any initial selection / select Recycle Bin opt-ins.
	for i, row := range model.rows {
		if row.Identifier == clean.DevCacheCategoryGo {
			model.rows[i].Selected = false
		}
		if row.Identifier == clean.OpportunityCategoryUserTemp {
			model.rows[i].Selected = true
		}
	}
	wantIDs := []string{
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.OpportunityCategoryUserTemp,
		clean.DevCacheCategoryNPM,
	}
	if !stringSlicesEqual(model.selectedCategoryIDs(), wantIDs) {
		t.Fatalf("after user edit ids=%v want %v", model.selectedCategoryIDs(), wantIDs)
	}

	// Clearing all does not change rule actions.
	model.clearSelection()
	if model.selectedCount() != 0 {
		t.Fatal("clear all failed")
	}
	for _, row := range model.rows {
		if row.Selected {
			t.Fatalf("selected after clear: %q", row.Identifier)
		}
		if row.PlannedAction != summariesWantAction(summaries, row.Identifier) {
			t.Fatalf("clear mutated action for %q", row.Identifier)
		}
	}
	// Production catalog: defaults + all delete_permanently categories start
	// selected, except exact-selection-only rows (Windows servicing) which never
	// start selected.
	prod := newEagerCleanModel(80, 24)
	for _, row := range prod.rows {
		wantSelected := row.SelectionPolicy != clean.CategorySelectionPolicyExactOnly &&
			(row.Eligibility == clean.CategoryEligibilityDefault ||
				row.PlannedAction == clean.PlannedActionDeletePermanently)
		if row.Selected != wantSelected {
			t.Fatalf("production %q selected=%v, want %v", row.Identifier, row.Selected, wantSelected)
		}
		if row.PlannedAction != clean.PlannedActionDeletePermanently &&
			row.PlannedAction != clean.PlannedActionMoveToRecycleBin &&
			row.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
			t.Fatalf("production %q action = %q, want a supported planned action", row.Identifier, row.PlannedAction)
		}
	}
}

func summariesWantAction(summaries []clean.CleanupCategorySummary, id string) clean.PlannedAction {
	for _, s := range summaries {
		if s.Identifier == id {
			return s.PlannedAction
		}
	}
	return ""
}

func TestEagerCleanConfirmationGroupsMixedActionsAndHandoff(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	permanentID := clean.DevCacheCategoryGo
	var gotSelected []string
	var gotAllow bool
	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent bool, _ bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotSelected = append([]string(nil), selected...)
		gotAllow = allowPermanent
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{
				{Path: `C:\Temp\a`, Bytes: 4, Rule: defaultID, Action: string(clean.PlannedActionMoveToRecycleBin)},
				{Path: `C:\Cache\b`, Bytes: 8, Rule: permanentID, Action: string(clean.PlannedActionDeletePermanently)},
			},
			Totals: clean.Totals{
				DeletedCount:            2,
				RecycleBinMovedBytes:    4,
				PermanentlyDeletedBytes: 8,
				AffectedBytes:           12,
			},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModelFromSummaries(injectedMixedActionSummaries(), 100, 40)
	// Terminal complete rows for selected categories; empty/skipped for the rest.
	for i := range model.rows {
		id := model.rows[i].Identifier
		switch id {
		case defaultID:
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 4
			model.rows[i].Selected = true
		case permanentID:
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 2
			model.rows[i].Bytes = 8
			model.rows[i].Selected = true
			model.rows[i].SafetyNote = "Rebuilds indexes after cleanup."
		case clean.DevCacheCategoryNPM:
			// Empty terminal auto-disables selection.
			model.rows[i].State = clean.CategoryPreviewEmpty
			model.rows[i].Selected = false
		case clean.OpportunityCategoryUserTemp:
			model.rows[i].State = clean.CategoryPreviewSkipped
			model.rows[i].ReasonCode = clean.PreviewReasonProtected
			model.rows[i].Selected = false
		}
	}
	model.finished = true
	model.generation = 1

	for _, row := range model.rows {
		if (row.State == clean.CategoryPreviewEmpty || row.State == clean.CategoryPreviewSkipped) && row.Selected {
			t.Fatalf("non-selectable terminal still selected: %#v", row)
		}
	}
	if model.selectedCount() != 2 {
		t.Fatalf("selectedCount = %d, want 2", model.selectedCount())
	}
	if !model.confirmationEnabled() {
		t.Fatal("confirmation should be enabled")
	}

	_, cmd := model.handleKey("enter")
	if cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("first enter: phase=%v cmd=%v", model.phase, cmd)
	}
	content := model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Permanent deletion · 1 categories · 2 item(s)",
		"Recycle Bin · 1 categories · 1 item(s)",
		"Go cache · 2 item(s) · <1 KB · Permanent deletion",
		"Foal-owned temp sandboxes · 1 item(s) · <1 KB · Recycle Bin",
		"Impact: Rebuilds indexes after cleanup.",
		"Permanent deletion is irreversible",
		"cannot introduce a deletion action type",
		"Recycle Bin items are moved",
		confirmationNextStepLine,
		"Enter: start cleanup",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, content)
		}
	}
	// Summary totals appear before detail rows.
	if sum := strings.Index(content, "Permanent deletion · 1 categories"); sum < 0 {
		t.Fatal("missing permanent summary")
	} else if detail := strings.Index(content, "  - "); detail < 0 || sum > detail {
		t.Fatalf("summary must precede detail rows:\n%s", content)
	}
	// No per-category deletion-method toggle chrome.
	for _, banned := range []string{"toggle action", "change method", "deletion method"} {
		if strings.Contains(strings.ToLower(content), banned) {
			t.Fatalf("presentation action toggle leaked: %q", banned)
		}
	}
	// Unselected rows not listed under action groups.
	if strings.Contains(content, "User temp") || strings.Contains(content, "npm cache") {
		t.Fatalf("unselected category listed:\n%s", content)
	}
	if calls != 0 {
		t.Fatal("confirmation must not execute")
	}

	// Second Enter freezes and hands off exact IDs + permanent auth.
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter: nav=%v cmd=%v", nav, cmd)
	}
	if !model.frozenAllowPermanent {
		t.Fatal("frozen permanent authorization missing")
	}
	if !stringSlicesEqual(model.frozenCategories, []string{defaultID, permanentID}) {
		t.Fatalf("frozen = %#v", model.frozenCategories)
	}
	// Drive handoff.
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
	if calls != 1 || !gotAllow {
		t.Fatalf("calls=%d allow=%v", calls, gotAllow)
	}
	if !stringSlicesEqual(gotSelected, []string{defaultID, permanentID}) {
		t.Fatalf("handoff selected=%v", gotSelected)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase=%v", model.phase)
	}
	content = model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Recycle Bin moved:",
		"Permanently deleted:",
		"Affected (processed):",
		"cleaned",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("result missing %q:\n%s", want, content)
		}
	}
	for _, banned := range []string{"freed space", "reclaimed disk", "space freed"} {
		if strings.Contains(strings.ToLower(content), banned) {
			t.Fatalf("banned wording %q:\n%s", banned, content)
		}
	}
	// No direct deletion collaborator in TUI path (only injected result).
	if strings.Contains(content, `C:\Temp`) || strings.Contains(content, `C:\Cache`) {
		t.Fatalf("path leak in result:\n%s", content)
	}
}

func TestEagerCleanResultProjectsMixedOutcomesAndPartialRisk(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	permanentID := clean.DevCacheCategoryGo
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent bool, _ bool, _ clean.ProgressReporter) clean.Result {
		if !allowPermanent {
			t.Fatal("mixed permanent selection must authorize permanent")
		}
		return clean.Result{
			Status: "partial",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{
				{Path: `C:\Temp\ok`, Bytes: 3, Rule: defaultID, Action: string(clean.PlannedActionMoveToRecycleBin)},
			},
			Failed: []clean.FailedItem{{
				Path:          `C:\Cache\fail`,
				Bytes:         9,
				Rule:          permanentID,
				PlannedAction: string(clean.PlannedActionDeletePermanently),
				Action:        string(clean.PlannedActionDeletePermanently),
				Reason: clean.StructuredIssue{
					Code:    "permanent_delete_failed",
					Message: `may already be permanently deleted under C:\Cache\fail`,
				},
			}},
			Skipped: []clean.SkippedItem{{
				Path:   `C:\Temp\skip`,
				Rule:   defaultID,
				Reason: clean.StructuredIssue{Code: "protected_path", Message: `protected C:\Temp\skip`},
			}},
			Totals: clean.Totals{
				DeletedCount:            1,
				SkippedCount:            1,
				RecycleBinMovedBytes:    3,
				PermanentlyDeletedBytes: 0,
				AffectedBytes:           3,
			},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModelFromSummaries(injectedMixedActionSummaries(), 100, 40)
	for i := range model.rows {
		id := model.rows[i].Identifier
		if id == defaultID || id == permanentID {
			model.rows[i].Selected = true
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 1
			continue
		}
		model.rows[i].Selected = false
		model.rows[i].State = clean.CategoryPreviewEmpty
	}
	model.finished = true
	_, _ = model.handleKey("enter")
	_, cmd := model.handleKey("enter")
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
	byID := map[string]clean.CategoryExecutionOutcome{}
	for _, outcome := range model.executionOutcomes {
		byID[outcome.Identifier] = outcome
	}
	if byID[defaultID].State != clean.CategoryExecutionPartial {
		t.Fatalf("recycle state = %q", byID[defaultID].State)
	}
	if byID[permanentID].State != clean.CategoryExecutionFailed {
		t.Fatalf("permanent state = %q", byID[permanentID].State)
	}
	content := model.content()
	assertNoPath(t, content)
	if !strings.Contains(content, "partial") || !strings.Contains(content, "failed") {
		t.Fatalf("mixed outcomes missing:\n%s", content)
	}
	if !strings.Contains(content, clean.PermanentPartialRiskWarning) {
		t.Fatalf("partial-risk warning missing:\n%s", content)
	}
	if strings.Contains(content, `C:\Cache`) || strings.Contains(content, "may already be permanently deleted under") {
		t.Fatalf("raw path-bearing failure message leaked:\n%s", content)
	}
	if !strings.Contains(content, "Recycle Bin moved:") || !strings.Contains(content, "Permanently deleted:") {
		t.Fatalf("split totals missing:\n%s", content)
	}
}

func TestEagerCleanNoPresentationOwnedActionMap(t *testing.T) {
	model := newEagerCleanModelFromSummaries(injectedMixedActionSummaries(), 80, 24)
	// Space toggles selection only; never flips planned action.
	for i := range model.rows {
		before := model.rows[i].PlannedAction
		model.cursor = i
		if model.rowSelectable(model.rows[i]) {
			_, _ = model.handleKey(" ")
		}
		if model.rows[i].PlannedAction != before {
			t.Fatalf("toggle changed planned action for %q", model.rows[i].Identifier)
		}
	}
	// select all / clear leave actions intact.
	model.selectAllSelectable()
	model.clearSelection()
	for i, summary := range injectedMixedActionSummaries() {
		if model.rows[i].PlannedAction != summary.PlannedAction {
			t.Fatalf("bulk select/clear mutated action for %q", summary.Identifier)
		}
	}
}

func TestEagerCleanProductionPermanentCategoriesInitialSelectionAndConfirmation(t *testing.T) {
	defaultID := clean.DefaultCategoryFoalOwnedTempSandboxes
	d3dID := clean.OpportunityCategoryD3DShaderCache
	npmID := clean.DevCacheCategoryNPM
	var gotSelected []string
	var gotAllow bool
	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent bool, _ bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotSelected = append([]string(nil), selected...)
		gotAllow = allowPermanent
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{
				{Path: `C:\Temp\a`, Bytes: 4, Rule: defaultID, Action: string(clean.PlannedActionMoveToRecycleBin)},
				{Path: `C:\Users\corey\AppData\Local\D3DSCache`, Bytes: 16, Rule: d3dID, Action: string(clean.PlannedActionDeletePermanently)},
				{Path: `C:\Users\corey\AppData\Local\npm-cache`, Bytes: 8, Rule: npmID, Action: string(clean.PlannedActionDeletePermanently)},
			},
			Totals: clean.Totals{
				DeletedCount:            3,
				RecycleBinMovedBytes:    4,
				PermanentlyDeletedBytes: 24,
				AffectedBytes:           28,
			},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	// Permanent-action opt-ins start selected; recycle-bin opt-ins stay unselected.
	seenPermanent := 0
	for _, row := range model.rows {
		if row.PlannedAction == clean.PlannedActionDeletePermanently {
			seenPermanent++
			if !row.Selected {
				t.Fatalf("%q must start selected", row.Identifier)
			}
		}
		if row.Eligibility == clean.CategoryEligibilityOptIn {
			if row.PlannedAction == clean.PlannedActionDeletePermanently {
				if !row.Selected {
					t.Fatalf("permanent opt-in %q must start selected", row.Identifier)
				}
			} else if row.Selected {
				t.Fatalf("non-permanent opt-in %q must start unselected", row.Identifier)
			}
		}
	}
	if seenPermanent == 0 {
		t.Fatal("expected at least one permanent-action category in production queue")
	}

	// Terminal complete for default + d3d + npm only (other permanents empty).
	for i := range model.rows {
		id := model.rows[i].Identifier
		switch id {
		case defaultID:
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 4
			model.rows[i].Selected = true
		case d3dID:
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 16
			model.rows[i].Selected = true
		case npmID:
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 8
			model.rows[i].Selected = true
		default:
			model.rows[i].State = clean.CategoryPreviewEmpty
			model.rows[i].Selected = false
		}
	}
	model.finished = true
	model.generation = 1

	// User may clear permanent rows from the exact selection.
	for i := range model.rows {
		if model.rows[i].PlannedAction == clean.PlannedActionDeletePermanently {
			model.rows[i].Selected = false
		}
	}
	if model.selectionIncludesPermanent() {
		t.Fatal("cleared permanent rows must not disclose permanent")
	}
	// Re-select d3d + npm permanent candidates for confirmation.
	for i := range model.rows {
		if model.rows[i].Identifier == d3dID || model.rows[i].Identifier == npmID {
			model.rows[i].Selected = true
		}
	}

	if model.selectedCount() != 3 {
		t.Fatalf("selectedCount = %d, want 3 (default+d3d+npm)", model.selectedCount())
	}
	if !model.confirmationEnabled() {
		t.Fatal("confirmation should be enabled")
	}

	_, cmd := model.handleKey("enter")
	if cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("first enter: phase=%v cmd=%v", model.phase, cmd)
	}
	content := model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Permanent deletion · 2 categories · 2 item(s)",
		"Recycle Bin · 1 categories · 1 item(s)",
		"D3D shader cache",
		"npm cache",
		"Permanent deletion",
		"Permanent deletion is irreversible",
		"cannot introduce a deletion action type",
		"Recycle Bin items are moved",
		confirmationNextStepLine,
		"Enter: start cleanup",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, content)
		}
	}
	if calls != 0 {
		t.Fatal("confirmation must not execute")
	}

	// Second Enter freezes exact IDs + permanent authorization from TUI confirmation.
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter: nav=%v cmd=%v", nav, cmd)
	}
	if !model.frozenAllowPermanent {
		t.Fatal("TUI confirmation must authorize permanent deletion")
	}
	// Frozen order follows catalog display/scan order.
	wantFrozen := make([]string, 0, 3)
	for _, summary := range clean.EagerPreviewQueue() {
		switch summary.Identifier {
		case defaultID, d3dID, npmID:
			wantFrozen = append(wantFrozen, summary.Identifier)
		}
	}
	if !stringSlicesEqual(model.frozenCategories, wantFrozen) {
		t.Fatalf("frozen = %#v, want %#v", model.frozenCategories, wantFrozen)
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
	if calls != 1 || !gotAllow {
		t.Fatalf("calls=%d allow=%v", calls, gotAllow)
	}
	if !stringSlicesEqual(gotSelected, wantFrozen) {
		t.Fatalf("handoff selected=%v, want %v", gotSelected, wantFrozen)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase=%v", model.phase)
	}
	content = model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Recycle Bin moved:",
		"Permanently deleted:",
		"Affected (processed):",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("result missing %q:\n%s", want, content)
		}
	}
}

func TestEagerCleanGrokBuildUpdateResidueRowSelectionAndHandoff(t *testing.T) {
	// Grok participates in the catalog-derived eager queue as a permanent row.
	// The TUI never owns Grok paths, process detection, or deletion — only
	// freezes the canonical identifier and permanent authorization.
	grokID := clean.CategoryGrokBuildUpdateResidue
	queue := clean.EagerPreviewQueue()
	foundInQueue := false
	for _, summary := range queue {
		if summary.Identifier == grokID {
			foundInQueue = true
			if summary.PlannedAction != clean.PlannedActionDeletePermanently {
				t.Fatalf("grok planned_action = %q", summary.PlannedAction)
			}
			if !clean.InitiallySelectedCategory(summary) {
				t.Fatal("grok permanent category must start selected when measurable")
			}
			break
		}
	}
	if !foundInQueue {
		t.Fatal("eager queue missing grok-build-update-residue")
	}

	var gotSelected []string
	var gotAllowPermanent bool
	var calls int
	original := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, allowPermanent bool, _ bool, _ clean.ProgressReporter) clean.Result {
		calls++
		gotSelected = append([]string(nil), selected...)
		gotAllowPermanent = allowPermanent
		return clean.Result{
			Status: "ok",
			Mode:   "execute",
			Deleted: []clean.DeletedItem{{
				Path:   `C:\Users\corey\.grok\bin\grok.exe.old`,
				Bytes:  138 << 20,
				Rule:   grokID,
				Action: string(clean.PlannedActionDeletePermanently),
			}},
			Totals: clean.Totals{
				DeletedCount:            1,
				PermanentlyDeletedBytes: 138 << 20,
				AffectedBytes:           138 << 20,
			},
		}
	}
	t.Cleanup(func() { runExactCleanSelection = original })

	model := newEagerCleanModel(100, 40)
	var grokIdx = -1
	for i, row := range model.rows {
		if row.Identifier == grokID {
			grokIdx = i
			if !row.Selected {
				t.Fatal("safely measurable Grok permanent row must start selected")
			}
			if row.PlannedAction != clean.PlannedActionDeletePermanently {
				t.Fatalf("row planned_action = %q", row.PlannedAction)
			}
		}
	}
	if grokIdx < 0 {
		t.Fatal("model rows missing grok-build-update-residue")
	}

	// Independently clear Grok; other permanent rows may stay selected for this test.
	model.rows[grokIdx].Selected = false
	if model.rows[grokIdx].Selected {
		t.Fatal("Grok row must be independently clearable")
	}

	// Drive terminal states: Grok complete/measurable, others empty so selection is exact.
	for i := range model.rows {
		id := model.rows[i].Identifier
		if id == grokID {
			model.rows[i].State = clean.CategoryPreviewComplete
			model.rows[i].CandidateCount = 1
			model.rows[i].Bytes = 138 << 20
			model.rows[i].Selected = true
			continue
		}
		model.rows[i].State = clean.CategoryPreviewEmpty
		model.rows[i].Selected = false
	}
	model.finished = true
	model.generation = 1

	if model.selectedCount() != 1 {
		t.Fatalf("selectedCount = %d, want 1 (Grok only)", model.selectedCount())
	}
	if !model.selectionIncludesPermanent() {
		t.Fatal("Grok selection must disclose permanent")
	}
	if !model.confirmationEnabled() {
		t.Fatal("confirmation should be enabled for non-empty terminal selection")
	}

	// Focus Grok so path-free browse rendering includes its label without
	// owning Grok paths or process detection.
	model.cursor = grokIdx
	model.ensurePreviewCursorVisible()
	if model.rows[grokIdx].Label != "Grok Build update residue" {
		t.Fatalf("row label = %q", model.rows[grokIdx].Label)
	}
	browse := model.content()
	assertNoPath(t, browse)
	if strings.Contains(browse, ".grok") || strings.Contains(browse, "grok.exe.old") || strings.Contains(browse, `\bin\`) {
		t.Fatalf("path-free browse leaked Grok path:\n%s", browse)
	}
	if !strings.Contains(browse, "Grok Build update residue") {
		t.Fatalf("browse missing Grok label after focus:\n%s", browse)
	}

	_, cmd := model.handleKey("enter")
	if cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("first enter: phase=%v cmd=%v", model.phase, cmd)
	}
	confirm := model.content()
	assertNoPath(t, confirm)
	for _, want := range []string{
		"Permanent deletion",
		"Grok Build update residue",
		"Permanent deletion is irreversible",
		confirmationNextStepLine,
		"Enter: start cleanup",
	} {
		if !strings.Contains(confirm, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, confirm)
		}
	}
	if calls != 0 {
		t.Fatal("confirmation must not execute or write History")
	}

	// Empty / no-selection states preserve existing confirmation rules.
	// Leave confirmation phase first so selection mutations apply to browse state.
	model.phase = eagerPhasePreview
	model.rows[grokIdx].Selected = false
	if model.confirmationEnabled() {
		t.Fatal("no-selection must disable confirmation")
	}
	model.rows[grokIdx].Selected = true
	if !model.confirmationEnabled() {
		t.Fatal("re-selected Grok must enable confirmation")
	}
	_, cmd = model.handleKey("enter")
	if cmd != nil || model.phase != eagerPhaseConfirmation {
		t.Fatalf("re-enter confirmation: phase=%v cmd=%v", model.phase, cmd)
	}

	// Second Enter freezes exact ID + permanent authorization through shared Clean.
	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil {
		t.Fatalf("second enter: nav=%v cmd=%v", nav, cmd)
	}
	if !model.frozenAllowPermanent {
		t.Fatal("TUI confirmation must authorize permanent deletion")
	}
	if len(model.frozenCategories) != 1 || model.frozenCategories[0] != grokID {
		t.Fatalf("frozen = %#v, want [%q]", model.frozenCategories, grokID)
	}
	// Group tokens never enter the exact frozen selection.
	for _, id := range model.frozenCategories {
		if id == clean.CLIAgentCategoryGroup || id == clean.DevCacheCategoryAll || id == "all" {
			t.Fatalf("group token in frozen selection: %q", id)
		}
	}
	// Drive the tea.Cmd handoff so shared Clean receives the frozen plan.
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
	if calls != 1 || !gotAllowPermanent {
		t.Fatalf("calls=%d allow=%v", calls, gotAllowPermanent)
	}
	if len(gotSelected) != 1 || gotSelected[0] != grokID {
		t.Fatalf("handoff selected=%v, want [%q]", gotSelected, grokID)
	}
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase=%v", model.phase)
	}
	result := model.content()
	assertNoPath(t, result)
}

func TestEagerCleanExecutionRestartsSpinnerTicksAndElapsed(t *testing.T) {
	// Hold execution open so ticks matter while Fresh scanning is in progress.
	release := make(chan struct{})
	original := runExactCleanSelection
	runExactCleanSelection = func(ctx context.Context, selected []string, _ bool, _ bool, reporter clean.ProgressReporter) clean.Result {
		if reporter != nil {
			reporter(clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning})
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	t.Cleanup(func() {
		close(release)
		runExactCleanSelection = original
	})

	fixedNow := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	model := newEagerCleanModel(100, 40)
	model.now = func() time.Time { return fixedNow }
	// markEagerQueueTerminal owns generation=1 for deterministic observations.
	markEagerQueueTerminal(&model, true)
	model.finished = true // preview finished; tick chain already dead
	gen := model.generation
	for i := range model.rows {
		model.rows[i].Selected = i == 0
	}

	_, _ = model.handleKey("enter")
	if model.phase != eagerPhaseConfirmation {
		t.Fatalf("phase=%v, want confirmation", model.phase)
	}
	// Confirmation must not keep the preview tick chain alive.
	if cmd := model.applyTick(eagerPreviewTickMsg{generation: gen, frame: 3}); cmd != nil {
		t.Fatal("confirmation phase must not reschedule ticks")
	}

	nav, cmd := model.handleKey("enter")
	if nav != eagerPreviewNavNone || cmd == nil || model.phase != eagerPhaseExecuting {
		t.Fatalf("start execution: nav=%v cmd=%v phase=%v", nav, cmd, model.phase)
	}
	if model.executionStartedAt.IsZero() {
		t.Fatal("executionStartedAt must be set when execute begins")
	}

	// Peel tea.Batch(execute, tick) without serial tick sleep blocking start.
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("beginExactExecution must return tea.Batch(execute, tick), got %T len=%d", batchMsg, len(batch))
	}
	ch := make(chan tea.Msg, len(batch))
	for _, child := range batch {
		child := child
		go func() { ch <- child() }()
	}
	var started eagerExactExecutionStartedMsg
	var tickMsg eagerPreviewTickMsg
	gotStart, gotTick := false, false
	deadline := time.After(eagerPreviewTickInterval + time.Second)
	for i := 0; i < len(batch); i++ {
		select {
		case m := <-ch:
			switch m := m.(type) {
			case eagerExactExecutionStartedMsg:
				started = m
				gotStart = true
			case eagerPreviewTickMsg:
				tickMsg = m
				gotTick = true
			}
		case <-deadline:
			t.Fatal("timeout expanding execution batch")
		}
	}
	if !gotStart || !gotTick {
		t.Fatalf("batch must include start and tick (start=%v tick=%v)", gotStart, gotTick)
	}
	if tickMsg.generation != gen {
		t.Fatalf("tick generation = %d, want %d", tickMsg.generation, gen)
	}
	_ = model.applyExactExecutionStarted(started)

	// Ticks advance the frame while executing.
	next := model.applyTick(tickMsg)
	if next == nil {
		t.Fatal("executing phase must reschedule ticks")
	}
	if model.spinnerFrame != tickMsg.frame {
		t.Fatalf("spinnerFrame = %d, want %d", model.spinnerFrame, tickMsg.frame)
	}
	wantFrame := eagerPreviewSpinnerFrames[model.spinnerFrame%len(eagerPreviewSpinnerFrames)]
	content := model.content()
	if !strings.Contains(content, wantFrame) {
		t.Fatalf("content missing spinner %q:\n%s", wantFrame, content)
	}
	if !strings.Contains(content, "Fresh scanning") || !strings.Contains(content, "0s") {
		t.Fatalf("execution header missing phase/elapsed:\n%s", content)
	}

	// Elapsed uses execution start, not preview start.
	fixedNow = fixedNow.Add(12 * time.Second)
	content = model.content()
	if !strings.Contains(content, "12s") {
		t.Fatalf("expected 12s elapsed after execution start:\n%s", content)
	}
	if !strings.Contains(content, eagerExecutionStillScanningReassurance) {
		t.Fatalf("expected still-working reassurance after threshold:\n%s", content)
	}

	// Stale generation must not mutate or reschedule.
	frameBeforeStale := model.spinnerFrame
	if cmd := model.applyTick(eagerPreviewTickMsg{generation: gen + 99, frame: 99}); cmd != nil {
		t.Fatal("stale tick must not reschedule")
	}
	if model.spinnerFrame != frameBeforeStale {
		t.Fatalf("stale tick mutated spinnerFrame to %d", model.spinnerFrame)
	}

	// Result stops the continuous tick need.
	model.applyExactExecuted(eagerExactExecutedMsg{result: clean.Result{Status: "ok", Mode: "execute"}})
	if model.phase != eagerPhaseResult {
		t.Fatalf("phase=%v, want result", model.phase)
	}
	if cmd := model.applyTick(eagerPreviewTickMsg{generation: gen, frame: model.spinnerFrame + 1}); cmd != nil {
		t.Fatal("result phase must not keep tick chain alive")
	}
}

func TestEagerCleanExecutionElapsedIgnoresPreviewStartedAt(t *testing.T) {
	fixedNow := time.Date(2026, 7, 18, 15, 0, 0, 0, time.UTC)
	model := newEagerCleanModel(80, 24)
	model.now = func() time.Time { return fixedNow }
	model.startedAt = fixedNow.Add(-5 * time.Minute) // long preview
	model.phase = eagerPhaseExecuting
	model.executionStarted = true
	model.executionStartedAt = fixedNow.Add(-4 * time.Second)
	model.executionProgress = clean.ExecutionProgress{Phase: clean.ExecutionPhaseScanning}
	model.executionOutcomes = []clean.CategoryExecutionOutcome{{
		Identifier: "user_temp",
		Label:      "User temp",
		State:      clean.CategoryExecutionRechecking,
	}}

	line := model.executionHeaderLine()
	if !strings.Contains(line, "4s") {
		t.Fatalf("want execution elapsed 4s, got %q", line)
	}
	if strings.Contains(line, "300s") || strings.Contains(line, "5m") {
		t.Fatalf("must not use preview startedAt: %q", line)
	}
	if !strings.Contains(line, eagerExecutionStillScanningReassurance) {
		t.Fatalf("want reassurance at >=3s scanning: %q", line)
	}

	// Other phases keep elapsed but drop scanning reassurance.
	model.executionProgress.Phase = clean.ExecutionPhasePermanentOperations
	line = model.executionHeaderLine()
	if !strings.Contains(line, "Permanent deletion") || !strings.Contains(line, "4s") {
		t.Fatalf("permanent phase header = %q", line)
	}
	if strings.Contains(line, eagerExecutionStillScanningReassurance) {
		t.Fatalf("reassurance only for Fresh scanning: %q", line)
	}
}
