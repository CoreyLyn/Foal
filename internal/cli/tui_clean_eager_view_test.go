package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Pure view-model tests: no tea Model, no streams, no I/O.

func TestEagerSelectionTotalsPure(t *testing.T) {
	rows := []eagerCategoryRow{
		{Identifier: "a", Selected: true, State: clean.CategoryPreviewComplete, Bytes: 2048},
		{Identifier: "b", Selected: true, State: clean.CategoryPreviewPartial, Bytes: 1024},
		{Identifier: "c", Selected: true, State: clean.CategoryPreviewWaiting},
		{Identifier: "d", Selected: true, State: clean.CategoryPreviewScanning},
		{Identifier: "e", Selected: false, State: clean.CategoryPreviewComplete, Bytes: 99999},
		{Identifier: "f", Selected: true, State: clean.CategoryPreviewEmpty, Bytes: 0},
	}
	n, measured, pending := eagerSelectionTotals(rows)
	if n != 5 {
		t.Fatalf("categories = %d, want 5", n)
	}
	if measured != 3072 {
		t.Fatalf("measured = %d, want 3072", measured)
	}
	if pending != 2 {
		t.Fatalf("pending = %d, want 2", pending)
	}
}

func TestEagerConfirmationEnabledPure(t *testing.T) {
	terminal := []eagerCategoryRow{
		{Identifier: "a", Selected: true, State: clean.CategoryPreviewComplete, Bytes: 1},
		{Identifier: "b", Selected: false, State: clean.CategoryPreviewEmpty},
	}
	if !eagerConfirmationEnabled(false, false, eagerPhasePreview, terminal, false) {
		t.Fatal("want enabled for terminal non-empty selection")
	}
	if eagerConfirmationEnabled(true, false, eagerPhasePreview, terminal, false) {
		t.Fatal("unavailable must disable")
	}
	if eagerConfirmationEnabled(false, true, eagerPhasePreview, terminal, false) {
		t.Fatal("canceled must disable")
	}
	if eagerConfirmationEnabled(false, false, eagerPhaseConfirmation, terminal, false) {
		t.Fatal("confirmation phase must disable re-entry gate")
	}
	if eagerConfirmationEnabled(false, false, eagerPhasePreview, terminal, true) {
		t.Fatal("executionStarted must disable")
	}
	none := []eagerCategoryRow{
		{Identifier: "a", Selected: false, State: clean.CategoryPreviewComplete},
	}
	if eagerConfirmationEnabled(false, false, eagerPhasePreview, none, false) {
		t.Fatal("empty selection must disable")
	}
	scanning := []eagerCategoryRow{
		{Identifier: "a", Selected: true, State: clean.CategoryPreviewScanning},
	}
	if eagerConfirmationEnabled(false, false, eagerPhasePreview, scanning, false) {
		t.Fatal("non-terminal must disable")
	}
	if eagerAllCategoriesTerminal(nil) {
		t.Fatal("empty queue is not terminal")
	}
}

func TestEagerNoWorkStatePure(t *testing.T) {
	allEmpty := []eagerCategoryRow{
		{Identifier: "a", State: clean.CategoryPreviewEmpty},
		{Identifier: "b", State: clean.CategoryPreviewEmpty},
	}
	if got := eagerNoWorkState(allEmpty, 0, false, false); got != clean.EagerPreviewNoWorkAllEmpty {
		t.Fatalf("all empty = %q", got)
	}
	diagnostics := []eagerCategoryRow{
		{Identifier: "a", State: clean.CategoryPreviewSkipped},
		{Identifier: "b", State: clean.CategoryPreviewFailed},
	}
	if got := eagerNoWorkState(diagnostics, 0, false, false); got != clean.EagerPreviewNoWorkDiagnostics {
		t.Fatalf("diagnostics = %q", got)
	}
	need := []eagerCategoryRow{
		{Identifier: "a", State: clean.CategoryPreviewComplete, CandidateCount: 1, Bytes: 10},
	}
	if got := eagerNoWorkState(need, 0, false, false); got != clean.EagerPreviewNoWorkNeedSelection {
		t.Fatalf("need selection = %q", got)
	}
	if got := eagerNoWorkState(need, 0, true, false); got != clean.EagerPreviewNoWorkNone {
		t.Fatalf("unavailable = %q", got)
	}
	if got := eagerNoWorkState(need, 1, false, false); got != clean.EagerPreviewNoWorkNone {
		t.Fatalf("with selection = %q", got)
	}
}

func TestEagerConfirmationActionGroupsPure(t *testing.T) {
	rows := []eagerCategoryRow{
		{Identifier: "recycle", Selected: true, PlannedAction: clean.PlannedActionMoveToRecycleBin, CandidateCount: 2, Bytes: 100},
		{Identifier: "perm", Selected: true, PlannedAction: clean.PlannedActionDeletePermanently, CandidateCount: 1, Bytes: 50},
		{Identifier: "off", Selected: false, PlannedAction: clean.PlannedActionDeletePermanently, CandidateCount: 9, Bytes: 9},
		{Identifier: "unknown", Selected: true, PlannedAction: clean.PlannedAction(""), CandidateCount: 3, Bytes: 30},
	}
	permanent, recycle, _ := eagerConfirmationActionGroups(rows)
	if len(permanent) != 1 || permanent[0].Identifier != "perm" {
		t.Fatalf("permanent = %#v", permanent)
	}
	if len(recycle) != 2 {
		t.Fatalf("recycle len = %d, want 2", len(recycle))
	}
	if !eagerSelectionIncludesPermanent(rows) {
		t.Fatal("want permanent disclosed")
	}
	pc, pcand, pbytes := confirmationGroupTotals(permanent)
	if pc != 1 || pcand != 1 || pbytes != 50 {
		t.Fatalf("permanent totals %d %d %d", pc, pcand, pbytes)
	}
	rc, rcand, rbytes := confirmationGroupTotals(recycle)
	if rc != 2 || rcand != 5 || rbytes != 130 {
		t.Fatalf("recycle totals %d %d %d", rc, rcand, rbytes)
	}
	ids := eagerSelectedCategoryIDs(rows)
	if len(ids) != 3 || ids[0] != "recycle" || ids[1] != "perm" || ids[2] != "unknown" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestEagerPreviewMarkersAndLabelsPure(t *testing.T) {
	if got := eagerPreviewRowMarker(clean.CategoryPreviewWaiting, 0); got != "…" {
		t.Fatalf("waiting marker = %q", got)
	}
	if got := eagerPreviewRowMarker(clean.CategoryPreviewScanning, 1); got != eagerPreviewSpinnerFrames[1] {
		t.Fatalf("scanning marker = %q", got)
	}
	if got := eagerPreviewRowMarker(clean.CategoryPreviewComplete, 0); got != "✓" {
		t.Fatalf("complete marker = %q", got)
	}
	row := eagerCategoryRow{
		Label:          "Temp",
		State:          clean.CategoryPreviewComplete,
		CandidateCount: 2,
		Bytes:          2048,
		PlannedAction:  clean.PlannedActionMoveToRecycleBin,
	}
	if got := eagerPreviewRowLabel(row); got != "Temp · 2 item(s) · 2 KB" {
		t.Fatalf("label = %q", got)
	}
	if got := eagerCheckbox(true); got != "[x]" {
		t.Fatalf("checkbox selected = %q", got)
	}
	if got := eagerCheckbox(false); got != "[ ]" {
		t.Fatalf("checkbox clear = %q", got)
	}
	if !eagerRowSelectable(eagerCategoryRow{State: clean.CategoryPreviewComplete}) {
		t.Fatal("complete should be selectable")
	}
	if eagerRowSelectable(eagerCategoryRow{State: clean.CategoryPreviewSkipped}) {
		t.Fatal("skipped should not be selectable")
	}
}

func TestEagerPreviewRowLabelOmitsPlannedActionPrefix(t *testing.T) {
	perm := eagerCategoryRow{
		Label:          "D3D",
		State:          clean.CategoryPreviewComplete,
		CandidateCount: 1,
		Bytes:          1024,
		PlannedAction:  clean.PlannedActionDeletePermanently,
	}
	if got := eagerPreviewRowLabel(perm); got != "D3D · 1 item(s) · 1 KB" {
		t.Fatalf("complete = %q", got)
	}
	if got := eagerPreviewRowLabel(perm); strings.Contains(got, "perm") || strings.Contains(got, "bin") {
		t.Fatalf("row must not prefix planned-action marker: %q", got)
	}
	partial := perm
	partial.State = clean.CategoryPreviewPartial
	if got := eagerPreviewRowLabel(partial); got != "D3D · 1 item(s) · 1 KB · partial" {
		t.Fatalf("partial = %q", got)
	}
	// Non-measured states invent no byte token and no action prefix.
	for _, state := range []clean.CategoryPreviewState{
		clean.CategoryPreviewWaiting,
		clean.CategoryPreviewScanning,
		clean.CategoryPreviewEmpty,
		clean.CategoryPreviewSkipped,
		clean.CategoryPreviewIncomplete,
		clean.CategoryPreviewFailed,
	} {
		row := eagerCategoryRow{Label: "Cache", State: state, PlannedAction: clean.PlannedActionDeletePermanently}
		got := eagerPreviewRowLabel(row)
		if strings.HasPrefix(got, "perm · ") || strings.HasPrefix(got, "bin · ") {
			t.Fatalf("state %q has planned-action prefix: %q", state, got)
		}
		if strings.Contains(got, "KB") || strings.Contains(got, "MB") || strings.Contains(got, "GB") {
			t.Fatalf("state %q invented byte token: %q", state, got)
		}
	}
	waiting := eagerCategoryRow{
		Label:         "Temp",
		State:         clean.CategoryPreviewWaiting,
		PlannedAction: clean.PlannedActionMoveToRecycleBin,
	}
	if got := eagerPreviewRowLabel(waiting); got != "Temp · waiting" {
		t.Fatalf("waiting = %q", got)
	}
}

func TestEagerPermanentSelectionNoticePresence(t *testing.T) {
	withPerm := []eagerCategoryRow{
		{Identifier: "a", Selected: true, PlannedAction: clean.PlannedActionDeletePermanently},
		{Identifier: "b", Selected: true, PlannedAction: clean.PlannedActionMoveToRecycleBin},
	}
	notice := eagerPermanentSelectionNotice(eagerSelectionIncludesPermanent(withPerm))
	if notice == "" || !strings.Contains(notice, "includes permanent deletion") {
		t.Fatalf("want permanent-selection notice, got %q", notice)
	}
	// Full sentence, distinct from confirmation irreversible warning.
	if !strings.Contains(notice, "permanent") {
		t.Fatalf("notice too weak: %q", notice)
	}
	if strings.Contains(notice, "irreversible") {
		t.Fatalf("preview notice must not reuse confirmation irreversible copy: %q", notice)
	}

	// Unselected permanent categories do not count.
	permOff := []eagerCategoryRow{
		{Identifier: "a", Selected: false, PlannedAction: clean.PlannedActionDeletePermanently},
		{Identifier: "b", Selected: true, PlannedAction: clean.PlannedActionMoveToRecycleBin},
	}
	if got := eagerPermanentSelectionNotice(eagerSelectionIncludesPermanent(permOff)); got != "" {
		t.Fatalf("cleared permanent selection must remove notice, got %q", got)
	}
	// Empty selection.
	if got := eagerPermanentSelectionNotice(false); got != "" {
		t.Fatalf("no permanent flag must yield empty notice, got %q", got)
	}
}

func TestEagerFooterHintsDoNotAuthorizeCleanup(t *testing.T) {
	base := eagerFooterHints(false, clean.EagerPreviewNoWorkNone, false, false)
	if !strings.Contains(base, "space toggle") {
		t.Fatalf("in-scan hints missing browse chrome: %q", base)
	}
	if strings.Contains(base, "perm=") || strings.Contains(base, "bin=") {
		t.Fatalf("hints must not document removed per-row markers: %q", base)
	}
	ready := eagerFooterHints(true, clean.EagerPreviewNoWorkNone, true, false)
	if !strings.Contains(ready, "enter confirm") {
		t.Fatalf("ready hints missing enter: %q", ready)
	}
	for _, forbidden := range []string{"execute now", "authorized", "will delete"} {
		if strings.Contains(strings.ToLower(base), forbidden) {
			t.Fatalf("hints must not authorize cleanup (%q): %s", forbidden, base)
		}
	}
}

func TestEagerExecutionMarkersAndLabelsPure(t *testing.T) {
	if got := eagerExecutionRowMarker(clean.CategoryExecutionWaiting, 0); got != "…" {
		t.Fatalf("waiting marker = %q", got)
	}
	if got := eagerExecutionRowMarker(clean.CategoryExecutionCleaning, 2); got != eagerPreviewSpinnerFrames[2] {
		t.Fatalf("cleaning marker = %q", got)
	}
	if got := eagerExecutionRowMarker(clean.CategoryExecutionCleaned, 0); got != "✓" {
		t.Fatalf("cleaned marker = %q", got)
	}
	if got := eagerExecutionRowLabel(clean.CategoryExecutionOutcome{
		Label: "Cache",
		State: clean.CategoryExecutionWaiting,
	}); got != "Cache · waiting" {
		t.Fatalf("waiting label = %q", got)
	}
	outcome := clean.CategoryExecutionOutcome{
		Label:         "Cache",
		State:         clean.CategoryExecutionCleaned,
		AffectedBytes: 4096,
	}
	if got := eagerExecutionRowLabel(outcome); got != "Cache · cleaned · 4 KB" {
		t.Fatalf("label = %q", got)
	}
}

func TestEagerResultTotalsPure(t *testing.T) {
	result := clean.Result{
		Totals: clean.Totals{
			RecycleBinMovedBytes:    10,
			PermanentlyDeletedBytes: 20,
			AffectedBytes:           30,
		},
	}
	recycle, permanent, affected := eagerResultTotals(result, nil)
	if recycle != 10 || permanent != 20 || affected != 30 {
		t.Fatalf("from result: %d %d %d", recycle, permanent, affected)
	}
	outcomes := []clean.CategoryExecutionOutcome{
		{State: clean.CategoryExecutionCleaned, RecycleBinMovedBytes: 4, AffectedBytes: 4},
		{State: clean.CategoryExecutionCleaned, PermanentlyDeletedBytes: 8, AffectedBytes: 8},
	}
	recycle, permanent, affected = eagerResultTotals(clean.Result{}, outcomes)
	if recycle != 4 || permanent != 8 || affected != 12 {
		t.Fatalf("from outcomes: %d %d %d", recycle, permanent, affected)
	}
}

func TestEagerFooterRuleLine(t *testing.T) {
	if got := eagerFooterRuleLine(0); got != strings.Repeat("=", 70) {
		t.Fatalf("default width rule = %q", got)
	}
	if got := eagerFooterRuleLine(40); got != strings.Repeat("=", 40) {
		t.Fatalf("width-40 rule = %q", got)
	}
	if got := eagerFooterRuleLine(-1); got != strings.Repeat("=", 70) {
		t.Fatalf("negative width falls back to default: %q", got)
	}
}

func TestEagerPreviewFooterFramedByRules(t *testing.T) {
	model := newEagerCleanModel(50, 30)
	markEagerQueueTerminal(&model, true)
	// One permanent selected category so notice appears inside the frame.
	for i := range model.rows {
		model.rows[i].Selected = false
	}
	model.rows[0].Selected = true
	model.rows[0].State = clean.CategoryPreviewComplete
	model.rows[0].Bytes = 1 << 30
	model.rows[0].CandidateCount = 1
	model.rows[0].PlannedAction = clean.PlannedActionDeletePermanently
	model.cursor = 0

	footer := model.fixedFooterLines()
	rule := strings.Repeat("=", 50)
	if len(footer) < 5 {
		t.Fatalf("footer too short: %#v", footer)
	}
	// Leading blank, then rule, Selected…, optional permanent notice, Focused…, rule, Hints…
	if footer[0] != "" {
		t.Fatalf("want leading blank, got %q", footer[0])
	}
	if footer[1] != rule {
		t.Fatalf("top rule = %q, want %q", footer[1], rule)
	}
	if !strings.HasPrefix(footer[2], "Selected:") {
		t.Fatalf("Selected line after top rule = %q", footer[2])
	}
	// Locate bottom rule: last pure-equals line before Hints.
	bottom := -1
	for i, line := range footer {
		if line == rule {
			bottom = i
		}
	}
	if bottom <= 2 {
		t.Fatalf("bottom rule missing or not after content: %#v", footer)
	}
	joinedInside := strings.Join(footer[2:bottom], "\n")
	if !strings.Contains(joinedInside, "Selected:") {
		t.Fatalf("Selected must be inside frame:\n%s", joinedInside)
	}
	if !strings.Contains(joinedInside, "Selection includes permanent deletion.") {
		t.Fatalf("permanent notice must be inside frame:\n%s", joinedInside)
	}
	if !strings.Contains(joinedInside, "Focused:") {
		t.Fatalf("Focused must be inside frame:\n%s", joinedInside)
	}
	outside := strings.Join(footer[bottom+1:], "\n")
	if !strings.Contains(outside, "Hints:") {
		t.Fatalf("Hints must stay outside frame:\n%s", outside)
	}
	if strings.Contains(outside, "Selected:") || strings.Contains(outside, "Focused:") {
		t.Fatalf("Selected/Focused must not appear outside frame:\n%s", outside)
	}
}

func TestEagerPreviewHeaderAndFooterPure(t *testing.T) {
	if got := eagerPreviewHeaderLine(true, false, false, -1, 0, 5, "3s"); !strings.HasPrefix(got, "Canceled") {
		t.Fatalf("canceled header = %q", got)
	}
	if got := eagerPreviewHeaderLine(false, true, true, -1, 5, 5, "1s"); !strings.Contains(got, "Scan complete") {
		t.Fatalf("complete header = %q", got)
	}
	scanning := eagerPreviewHeaderLine(false, false, false, 0, 0, 10, "0s")
	if !strings.Contains(scanning, "Scanning 1/10") || !strings.Contains(scanning, "Confirmation available after scan completes") {
		t.Fatalf("scanning header = %q", scanning)
	}
	if strings.Contains(scanning, "%") {
		t.Fatalf("byte-derived percentage leaked: %q", scanning)
	}

	base := eagerFooterHints(false, clean.EagerPreviewNoWorkNone, false, false)
	if !strings.Contains(base, "space toggle") || strings.Contains(base, "enter confirm") {
		t.Fatalf("in-scan footer = %q", base)
	}
	if strings.Contains(base, "perm=") || strings.Contains(base, "bin=") {
		t.Fatalf("in-scan footer must not carry removed marker legend: %q", base)
	}
	if got := eagerFooterHints(true, clean.EagerPreviewNoWorkNeedSelection, false, false); !strings.Contains(got, "Select at least one") {
		t.Fatalf("need selection footer = %q", got)
	}
	if got := eagerFooterHints(true, clean.EagerPreviewNoWorkNone, true, false); !strings.Contains(got, "enter confirm") {
		t.Fatalf("ready footer = %q", got)
	}
}

func TestPathFreeReasonExplanationStable(t *testing.T) {
	if got := pathFreeReasonExplanation(clean.PreviewReasonProtected); got != "protected by Protection rules" {
		t.Fatalf("protected = %q", got)
	}
	if got := pathFreeReasonExplanation(`C:\Users\secret\path`); got != `C:\Users\secret\path` {
		// Unknown codes pass through as the stable code string only — callers
		// must never put OS path text into ReasonCode. This asserts the map
		// does not invent extra wording that could hide misuse.
		t.Fatalf("unknown code passthrough = %q", got)
	}
	if got := pathFreeReasonExplanation(""); got != "see category state" {
		t.Fatalf("empty = %q", got)
	}
}

func TestEagerFocusedDetailBodyPure(t *testing.T) {
	partial := eagerCategoryRow{
		State:                clean.CategoryPreviewPartial,
		CandidateCount:       2,
		Bytes:                1024,
		ExcludedSiblingCount: 3,
		ReasonCode:           clean.PreviewReasonProtected,
	}
	body := eagerFocusedDetailBody(partial)
	if !strings.Contains(body, "Partial") || !strings.Contains(body, "3 excluded") ||
		!strings.Contains(body, "protected by Protection rules") {
		t.Fatalf("partial body = %q", body)
	}
	if strings.Contains(strings.ToLower(body), `c:\`) {
		t.Fatalf("path leaked into detail: %q", body)
	}
}

func TestEagerSelectionSummaryLinePure(t *testing.T) {
	if got := eagerSelectionSummaryLine(4, 3072, 2); got != "Selected: 4 categories · 3 KB measured · 2 pending" {
		t.Fatalf("pending summary = %q", got)
	}
	if got := eagerSelectionSummaryLine(1, 0, 0); got != "Selected: 1 categories · 0 KB" {
		t.Fatalf("collapsed summary = %q", got)
	}
}

func TestEagerExecutionHeaderLinePure(t *testing.T) {
	if got := eagerExecutionElapsedLabel(12 * time.Second); got != "12s" {
		t.Fatalf("elapsed = %q", got)
	}
	if got := eagerExecutionElapsedLabel(-time.Second); got != "0s" {
		t.Fatalf("negative elapsed = %q", got)
	}
	line := eagerExecutionHeaderLine(1, "Fresh scanning", "12s", "")
	want := fmt.Sprintf("%s Fresh scanning · 12s", eagerPreviewSpinnerFrames[1%len(eagerPreviewSpinnerFrames)])
	if line != want {
		t.Fatalf("header = %q, want %q", line, want)
	}
	withNote := eagerExecutionHeaderLine(0, "Fresh scanning", "3s", eagerExecutionStillScanningReassurance)
	if !strings.Contains(withNote, "3s") || !strings.Contains(withNote, eagerExecutionStillScanningReassurance) {
		t.Fatalf("reassurance header = %q", withNote)
	}
	if strings.Contains(withNote, "%") {
		t.Fatalf("must not invent percentages: %q", withNote)
	}
}

func TestCleanMagnitudeTierBoundaries(t *testing.T) {
	const (
		mib = int64(1024 * 1024)
		gib = int64(1024 * 1024 * 1024)
	)
	tests := []struct {
		name  string
		bytes int64
		want  cleanMagnitudeTier
	}{
		{name: "zero", bytes: 0, want: cleanMagnitudeNone},
		{name: "negative", bytes: -1, want: cleanMagnitudeNone},
		{name: "one byte", bytes: 1, want: cleanMagnitudeNeutral},
		{name: "just below 100 MiB", bytes: 100*mib - 1, want: cleanMagnitudeNeutral},
		{name: "exactly 100 MiB", bytes: 100 * mib, want: cleanMagnitudeAttention},
		{name: "mid attention", bytes: 500 * mib, want: cleanMagnitudeAttention},
		{name: "just below 1 GiB", bytes: gib - 1, want: cleanMagnitudeAttention},
		{name: "exactly 1 GiB", bytes: gib, want: cleanMagnitudeStrong},
		{name: "multi GiB", bytes: 3 * gib, want: cleanMagnitudeStrong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanMagnitudeTierFromBytes(tt.bytes); got != tt.want {
				t.Fatalf("cleanMagnitudeTierFromBytes(%d) = %v, want %v", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestCleanMagnitudeTierFromFormattedToken(t *testing.T) {
	tests := []struct {
		token string
		want  cleanMagnitudeTier
	}{
		{token: "0 KB", want: cleanMagnitudeNone},
		{token: "<1 KB", want: cleanMagnitudeNeutral},
		{token: "2 KB", want: cleanMagnitudeNeutral},
		{token: "99.9 MB", want: cleanMagnitudeNeutral},
		{token: "100 MB", want: cleanMagnitudeAttention},
		{token: "512 MB", want: cleanMagnitudeAttention},
		{token: "1 GB", want: cleanMagnitudeStrong},
		{token: "3 GB", want: cleanMagnitudeStrong},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			if got := cleanMagnitudeTierFromFormattedToken(tt.token); got != tt.want {
				t.Fatalf("token %q tier = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestEagerPreviewByteColumnAlignment(t *testing.T) {
	rows := []eagerCategoryRow{
		{Label: "Short", State: clean.CategoryPreviewComplete, CandidateCount: 1, Bytes: 2048, PlannedAction: clean.PlannedActionMoveToRecycleBin},
		{Label: "Much Longer Category Label", State: clean.CategoryPreviewPartial, CandidateCount: 12, Bytes: 3 * 1024 * 1024 * 1024, PlannedAction: clean.PlannedActionDeletePermanently},
		{Label: "Waiting", State: clean.CategoryPreviewWaiting, PlannedAction: clean.PlannedActionMoveToRecycleBin},
		{Label: "Empty", State: clean.CategoryPreviewEmpty, PlannedAction: clean.PlannedActionDeletePermanently},
	}
	leftWidth, byteWidth := eagerPreviewByteColumnWidths(rows)
	if leftWidth == 0 || byteWidth == 0 {
		t.Fatalf("widths left=%d byte=%d", leftWidth, byteWidth)
	}

	complete := eagerPreviewRowLabelAligned(rows[0], leftWidth, byteWidth)
	partial := eagerPreviewRowLabelAligned(rows[1], leftWidth, byteWidth)
	// Plain fragments remain findable (oracle); no planned-action prefix.
	if !strings.Contains(complete, "Short") || !strings.Contains(complete, "2 KB") {
		t.Fatalf("complete = %q", complete)
	}
	if strings.Contains(complete, "bin ·") || strings.Contains(partial, "perm ·") {
		t.Fatalf("labels must not prefix planned-action markers: complete=%q partial=%q", complete, partial)
	}
	if !strings.Contains(partial, "3 GB") || !strings.Contains(partial, "partial") {
		t.Fatalf("partial = %q", partial)
	}
	// Byte tokens start at the same column in the plain frame.
	idxComplete := strings.Index(complete, "2 KB")
	idxPartial := strings.Index(partial, "3 GB")
	if idxComplete < 0 || idxPartial < 0 || idxComplete != idxPartial {
		t.Fatalf("byte columns misaligned: complete@%d %q partial@%d %q", idxComplete, complete, idxPartial, partial)
	}
	// Non-measured states invent no byte field.
	waiting := eagerPreviewRowLabelAligned(rows[2], leftWidth, byteWidth)
	if strings.Contains(waiting, "KB") || strings.Contains(waiting, "MB") || strings.Contains(waiting, "GB") {
		t.Fatalf("waiting invented bytes: %q", waiting)
	}
	empty := eagerPreviewRowLabelAligned(rows[3], leftWidth, byteWidth)
	if strings.Contains(empty, "0 KB") {
		t.Fatalf("empty invented zero magnitude field: %q", empty)
	}
	if got := empty; got != "Empty · empty" {
		t.Fatalf("empty label = %q", got)
	}
}

func TestStyleMagnitudeTokenNoColorAndHues(t *testing.T) {
	attention := styleMagnitudeTokenWithColor("100 MB", cleanMagnitudeAttention, true, false)
	strong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, true, false)
	if attention == "100 MB" || strong == "1 GB" {
		t.Fatal("colored path should decorate attention/strong tokens")
	}
	// Plain fragments must remain inside the styled token.
	if !strings.Contains(attention, "100 MB") || !strings.Contains(strong, "1 GB") {
		t.Fatalf("styled tokens lost plain fragments: %q %q", attention, strong)
	}
	// Strong must not use pure red CSI (31 / 91) as the size cue.
	if strings.Contains(strong, "[31m") || strings.Contains(strong, "[91m") {
		t.Fatalf("strong magnitude must not use pure red: %q", strong)
	}

	noColorAttention := styleMagnitudeTokenWithColor("100 MB", cleanMagnitudeAttention, false, false)
	noColorStrong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, false, false)
	if !strings.Contains(noColorAttention, "100 MB") || !strings.Contains(noColorStrong, "1 GB") {
		t.Fatalf("NO_COLOR path lost fragments: %q %q", noColorAttention, noColorStrong)
	}
	// Without color, no amber/orange foreground; bold may remain.
	if strings.Contains(noColorAttention, "214") || strings.Contains(noColorStrong, "208") {
		t.Fatalf("NO_COLOR path leaked magnitude hues: %q %q", noColorAttention, noColorStrong)
	}

	// Zero/none and neutral stay plain (no invented magnitude color).
	if got := styleMagnitudeTokenWithColor("0 KB", cleanMagnitudeNone, true, false); got != "0 KB" {
		t.Fatalf("none tier = %q", got)
	}
	if got := styleMagnitudeTokenWithColor("2 KB", cleanMagnitudeNeutral, true, false); got != "2 KB" {
		t.Fatalf("neutral tier = %q", got)
	}
}

func TestClassifyMagnitudeTierPrefersTrustedBytes(t *testing.T) {
	const (
		mib = int64(1024 * 1024)
		gib = int64(1024 * 1024 * 1024)
	)
	// Display rounding can make just-below thresholds look like the next unit
	// step; trusted int64 classification must still win on the production path.
	justBelowAttention := 100*mib - 1
	tokenBelowAttention := cleanFormatBytes(justBelowAttention)
	if tokenBelowAttention != "100 MB" {
		// Document the brittle display; if formatting changes, still assert tiers.
		t.Logf("cleanFormatBytes(%d) = %q (expected rounding to 100 MB historically)", justBelowAttention, tokenBelowAttention)
	}
	if got := classifyMagnitudeTier(tokenBelowAttention, justBelowAttention, true); got != cleanMagnitudeNeutral {
		t.Fatalf("trusted just-below 100 MiB tier = %v, want Neutral", got)
	}
	// Fallback reverse-parse of the rounded token is documented as imprecise.
	if tokenBelowAttention == "100 MB" {
		if got := classifyMagnitudeTier(tokenBelowAttention, 0, false); got != cleanMagnitudeAttention {
			t.Fatalf("fallback for rounded 100 MB token = %v, want Attention", got)
		}
	}

	justBelowStrong := gib - 1
	tokenBelowStrong := cleanFormatBytes(justBelowStrong)
	if got := classifyMagnitudeTier(tokenBelowStrong, justBelowStrong, true); got != cleanMagnitudeAttention {
		t.Fatalf("trusted just-below 1 GiB tier = %v, want Attention", got)
	}

	// Exact thresholds via int64 remain the authoritative boundary table.
	tests := []struct {
		name    string
		bytes   int64
		trusted bool
		token   string
		want    cleanMagnitudeTier
	}{
		{name: "zero trusted", bytes: 0, trusted: true, token: "0 KB", want: cleanMagnitudeNone},
		{name: "exactly 100 MiB", bytes: 100 * mib, trusted: true, token: cleanFormatBytes(100 * mib), want: cleanMagnitudeAttention},
		{name: "exactly 1 GiB", bytes: gib, trusted: true, token: cleanFormatBytes(gib), want: cleanMagnitudeStrong},
		{name: "fallback 1 GB token", bytes: 0, trusted: false, token: "1 GB", want: cleanMagnitudeStrong},
		{name: "fallback 0 KB token", bytes: 0, trusted: false, token: "0 KB", want: cleanMagnitudeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMagnitudeTier(tt.token, tt.bytes, tt.trusted); got != tt.want {
				t.Fatalf("classifyMagnitudeTier(%q, %d, trusted=%v) = %v, want %v",
					tt.token, tt.bytes, tt.trusted, got, tt.want)
			}
		})
	}
}

func TestStylizeStyleLinesUsesTrustedBytesNotTokenParse(t *testing.T) {
	// Line displays a token that reverse-parse would call Attention, but the
	// trusted count is Neutral (< 100 MiB). Production path must classify from
	// int64 so the token stays undecorated (Neutral = plain).
	const justBelow = int64(100*1024*1024 - 1)
	token := cleanFormatBytes(justBelow)
	plain := fmt.Sprintf("    [x] ✓ Edge · 1 item(s) · %s", token)

	// Tier decision: trusted bytes win over token text.
	if got := classifyMagnitudeTier(token, justBelow, true); got != cleanMagnitudeNeutral {
		t.Fatalf("trusted tier = %v, want Neutral for %d (%q)", got, justBelow, token)
	}
	if token == "100 MB" {
		if got := classifyMagnitudeTier(token, 0, false); got != cleanMagnitudeAttention {
			t.Fatalf("fallback tier for rounded %q = %v, want Attention", token, got)
		}
	}

	// Annotated style path: Neutral token remains plain (no magnitude style).
	styled := stylizeStyleLines([]tuiStyleLine{
		magnitudeStyleLine(plain, justBelow),
	})
	if !strings.Contains(styled, token) {
		t.Fatalf("lost plain token %q:\n%q", token, styled)
	}
	// Neutral leaves the token unstyled; the whole line may still be plain.
	if styled != plain {
		// Allow only non-magnitude whole-line styles; magnitude hues must not apply.
		if strings.Contains(styled, "214") || strings.Contains(styled, "208") {
			t.Fatalf("trusted Neutral must not apply attention/strong hues:\n%q", styled)
		}
	}

	// Explicit color core: Neutral stays plain; Attention (fallback tier) hues.
	if got := styleMagnitudeTokenWithColor(token, cleanMagnitudeNeutral, true, false); got != token {
		t.Fatalf("Neutral colored path should stay plain, got %q", got)
	}
	attention := styleMagnitudeTokenWithColor("100 MB", cleanMagnitudeAttention, true, false)
	if attention == "100 MB" || !strings.Contains(attention, "100 MB") {
		t.Fatalf("Attention colored path should decorate token: %q", attention)
	}
	if strings.Contains(attention, "[31m") || strings.Contains(attention, "[91m") {
		t.Fatalf("Attention must not use pure red: %q", attention)
	}
}

func TestSelectedRowMagnitudeStacking(t *testing.T) {
	// Focused preview row: reverse chrome + magnitude on the byte token.
	// Plain fragments remain the oracle; strong must not use pure red.
	const gib = int64(1024 * 1024 * 1024)
	plain := "  > [x] ✓ Big · 1 item(s) · 1 GB"
	styled := stylizeStyleLines([]tuiStyleLine{
		magnitudeStyleLine(plain, gib),
	})
	for _, want := range []string{"Big · 1 item(s)", "1 GB", "[x]"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("selected styled row missing plain fragment %q:\n%q", want, styled)
		}
	}
	if strings.Contains(styled, "[31m") || strings.Contains(styled, "[91m") {
		t.Fatalf("selected strong magnitude must not use pure red:\n%q", styled)
	}
	// Selection reverse is applied (left/token/right segments).
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("selected row should receive style escapes")
	}
	// Strip ANSI: plain frame fragments must still match the source line.
	if stripANSIForTest(styled) != plain {
		t.Fatalf("selected styled row plain projection mismatch:\n got %q\nwant %q", stripANSIForTest(styled), plain)
	}

	// Explicit color core: selected strong stacks reverse + orange, never pure red.
	selectedStrong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, true, true)
	if !strings.Contains(selectedStrong, "1 GB") {
		t.Fatalf("selected strong lost plain fragment: %q", selectedStrong)
	}
	if selectedStrong == "1 GB" {
		t.Fatal("selected strong should decorate token when color enabled")
	}
	if strings.Contains(selectedStrong, "[31m") || strings.Contains(selectedStrong, "[91m") {
		t.Fatalf("selected strong must not use pure red: %q", selectedStrong)
	}
	// Reverse bit (7) should be present for continuous selection on the token.
	if !strings.Contains(selectedStrong, "7") {
		t.Fatalf("selected strong should include reverse: %q", selectedStrong)
	}

	// Neutral selected: continuous reverse, no magnitude hue indexes required.
	neutralToken := styleMagnitudeTokenWithColor("2 KB", cleanMagnitudeNeutral, true, true)
	if !strings.Contains(neutralToken, "2 KB") {
		t.Fatalf("neutral selected lost fragment: %q", neutralToken)
	}
	// Explicit no-color selected strong: reverse/bold without magnitude hues.
	noColorStrong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, false, true)
	if !strings.Contains(noColorStrong, "1 GB") {
		t.Fatalf("NO_COLOR selected strong lost fragment: %q", noColorStrong)
	}
	if strings.Contains(noColorStrong, "208") || strings.Contains(noColorStrong, "214") {
		t.Fatalf("NO_COLOR selected strong leaked magnitude hues: %q", noColorStrong)
	}

	// Neutral selected full line still projects plain fragments.
	neutralPlain := "  > [x] ✓ Small · 1 item(s) · 2 KB"
	neutralStyled := stylizeStyleLines([]tuiStyleLine{
		magnitudeStyleLine(neutralPlain, 2048),
	})
	if stripANSIForTest(neutralStyled) != neutralPlain {
		t.Fatalf("neutral selected plain projection mismatch:\n got %q\nwant %q", stripANSIForTest(neutralStyled), neutralPlain)
	}
}

func TestStylizeFrameMagnitudeOnPreviewAndSelected(t *testing.T) {
	plain := strings.Join([]string{
		"Foal Clean",
		"  > [x] ✓ Big · 1 item(s) · 1 GB",
		"    [ ] ✓ Mid · 1 item(s) · 100 MB",
		"    [ ] … Wait · waiting",
		"Selected: 1 categories · 1 GB",
		"Selection includes permanent deletion.",
		"Hints: space toggle",
	}, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain frame must stay free of escapes")
	}
	styled := stylizeFrame(plain)
	// Plain fragments remain the contract (labels, notice, bytes).
	for _, want := range []string{
		"Big · 1 item(s)", "Mid · 1 item(s)", "1 GB", "100 MB", "waiting",
		"Selected: 1 categories", "includes permanent deletion",
	} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled frame missing plain fragment %q:\n%q", want, styled)
		}
	}
	if strings.Contains(styled, "perm ·") || strings.Contains(styled, "bin ·") {
		t.Fatalf("styled frame must not invent per-row action prefixes:\n%q", styled)
	}
	// Magnitude applied to measured preview/selected bytes, not waiting.
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("styled frame should include magnitude or reverse escapes when color enabled")
	}
	// Waiting line has no byte token to color.
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "waiting") && cleanByteTokenPattern.MatchString(line) {
			t.Fatalf("waiting line should not gain a byte token: %q", line)
		}
	}
	// Preview rows must not use pure-red whole-row risk tint for size or labels.
	for _, line := range strings.Split(styled, "\n") {
		if !strings.Contains(line, "item(s)") {
			continue
		}
		if strings.Contains(line, "[31m") || strings.Contains(line, "[91m") {
			t.Fatalf("preview magnitude/label row must not use pure red: %q", line)
		}
	}
}

func TestStylizeFrameMagnitudeRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	plain := "    [x] ✓ Cache · 1 item(s) · 1 GB\nSelected: 1 categories · 1 GB\n"
	styled := stylizeFrame(plain)
	if !strings.Contains(styled, "1 GB") {
		t.Fatalf("NO_COLOR path lost plain fragment:\n%q", styled)
	}
	// Hues (256-color orange/amber indexes used by magnitude styles) must not appear.
	if strings.Contains(styled, "208") || strings.Contains(styled, "214") {
		t.Fatalf("NO_COLOR path leaked magnitude hues:\n%q", styled)
	}
}

func TestIsConfirmationMeasuredLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{line: "Permanent deletion · 2 categories · 3 item(s) · 1 GB", want: true},
		{line: "Recycle Bin · 1 categories · 1 item(s) · 100 MB", want: true},
		{line: "  - Go cache · 2 item(s) · 512 MB · Permanent deletion", want: true},
		{line: "  - Foal-owned temp sandboxes · 1 item(s) · 2 KB · Recycle Bin", want: true},
		// Summary-first section headers (no totals) are not measured lines.
		{line: "Permanent deletion", want: false},
		{line: "Recycle Bin", want: false},
		// Risk warning / next-step copy is not a measured-byte confirmation row.
		{line: confirmationPermanentIrreversibleWarning, want: false},
		{line: confirmationNextStepLine, want: false},
		// Preview / execution / result must not match confirmation patterns.
		{line: "  > [x] ✓ Big · 1 item(s) · 1 GB", want: false},
		{line: "    [ ] ✓ Mid · 1 item(s) · 100 MB", want: false},
		{line: "  ✓ Cache · cleaned · 4 KB", want: false},
		{line: "Recycle Bin moved: 1 GB", want: false},
		{line: "Permanently deleted: 100 MB", want: false},
		{line: "Affected (processed): 1 GB", want: false},
		{line: "Selected: 2 categories · 1 GB", want: false},
		{line: "      Impact: Rebuilds indexes after cleanup.", want: false},
	}
	for _, tt := range cases {
		if got := isConfirmationMeasuredLine(tt.line); got != tt.want {
			t.Fatalf("isConfirmationMeasuredLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestConfirmationBodyEntriesSummaryFirst(t *testing.T) {
	// Summary-first: compact group totals precede detail section headers + rows.
	rows := []eagerCategoryRow{
		{
			Identifier:     "go-cache",
			Label:          "Go cache",
			Selected:       true,
			PlannedAction:  clean.PlannedActionDeletePermanently,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 2,
			Bytes:          1024 * 1024 * 1024,
			SafetyNote:     "Rebuilds indexes after cleanup.",
		},
		{
			Identifier:     "user_temp",
			Label:          "User temp",
			Selected:       true,
			PlannedAction:  clean.PlannedActionMoveToRecycleBin,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 1,
			Bytes:          100 * 1024 * 1024,
		},
		{
			Identifier:    "off",
			Label:         "Off row",
			Selected:      false,
			PlannedAction: clean.PlannedActionDeletePermanently,
			State:         clean.CategoryPreviewComplete,
			Bytes:         99,
		},
	}
	permanent, recycle, _ := eagerConfirmationActionGroups(rows)
	entries := confirmationBodyEntriesFromGroups(permanent, recycle, nil)
	if len(entries) == 0 {
		t.Fatal("expected confirmation body entries")
	}
	// First non-empty lines are the compact summaries (both groups present).
	if !strings.HasPrefix(entries[0].text, "Permanent deletion ·") {
		t.Fatalf("first body line must be permanent summary, got %q", entries[0].text)
	}
	if !entries[0].hasMagnitudeBytes || entries[0].magnitudeBytes != permanent[0].Bytes {
		t.Fatalf("permanent summary magnitude = %v %d", entries[0].hasMagnitudeBytes, entries[0].magnitudeBytes)
	}
	if !strings.HasPrefix(entries[1].text, "Recycle Bin ·") {
		t.Fatalf("second body line must be recycle summary, got %q", entries[1].text)
	}
	// Blank separator then section header before detail rows.
	blankIdx := -1
	for i, e := range entries {
		if e.text == "" {
			blankIdx = i
			break
		}
	}
	if blankIdx < 0 {
		t.Fatal("summary and details must be separated by a blank line")
	}
	if blankIdx >= len(entries)-1 || entries[blankIdx+1].text != "Permanent deletion" {
		t.Fatalf("detail section header after blank = %#v", entries[blankIdx+1:])
	}
	joined := ""
	for _, e := range entries {
		joined += e.text + "\n"
	}
	for _, want := range []string{
		"Permanent deletion · 1 categories · 2 item(s) · 1 GB",
		"Recycle Bin · 1 categories · 1 item(s) · 100 MB",
		"  - Go cache · 2 item(s) · 1 GB · Permanent deletion",
		"      Impact: Rebuilds indexes after cleanup.",
		"  - User temp · 1 item(s) · 100 MB · Recycle Bin",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("body missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Off row") {
		t.Fatalf("unselected category leaked:\n%s", joined)
	}
	// Empty groups omitted: permanent-only body has no Recycle Bin lines.
	permOnly := confirmationBodyEntriesFromGroups(permanent, nil, nil)
	for _, e := range permOnly {
		if strings.Contains(e.text, "Recycle Bin") {
			t.Fatalf("empty recycle group must be omitted: %q", e.text)
		}
	}
}

func TestConfirmationPlainFrameByteAndWarningCopy(t *testing.T) {
	// Plain confirmation composition is the contract oracle (no ANSI).
	rows := []eagerCategoryRow{
		{
			Identifier:     "go-cache",
			Label:          "Go cache",
			Selected:       true,
			PlannedAction:  clean.PlannedActionDeletePermanently,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 2,
			Bytes:          1024 * 1024 * 1024, // 1 GB
			SafetyNote:     "Rebuilds indexes after cleanup.",
		},
		{
			Identifier:     "user_temp",
			Label:          "User temp",
			Selected:       true,
			PlannedAction:  clean.PlannedActionMoveToRecycleBin,
			State:          clean.CategoryPreviewComplete,
			CandidateCount: 1,
			Bytes:          100 * 1024 * 1024, // 100 MB
		},
		{
			Identifier:    "off",
			Label:         "Off row",
			Selected:      false,
			PlannedAction: clean.PlannedActionDeletePermanently,
			State:         clean.CategoryPreviewComplete,
			Bytes:         99,
		},
	}
	permanent, recycle, _ := eagerConfirmationActionGroups(rows)
	if len(permanent) != 1 || len(recycle) != 1 {
		t.Fatalf("groups permanent=%d recycle=%d", len(permanent), len(recycle))
	}
	_, _, pbytes := confirmationGroupTotals(permanent)
	_, _, rbytes := confirmationGroupTotals(recycle)

	// Body: summary-first then details (same pure helper as production).
	body := confirmationBodyEntriesFromGroups(permanent, recycle, nil)
	bodyLines := make([]string, len(body))
	for i, e := range body {
		bodyLines[i] = e.text
	}

	// Footer fragments mirror confirmationFooterStyleLines ordering.
	plainParts := []string{
		"Foal Clean",
		"Confirm cleanup",
	}
	plainParts = append(plainParts, bodyLines...)
	plainParts = append(plainParts,
		fmt.Sprintf("Selected: %d categories · %s", 2, cleanFormatBytes(pbytes+rbytes)),
		confirmationPermanentIrreversibleWarning,
		confirmationRecycleRecoverabilityNote,
		confirmationActionTypeCaveat,
		confirmationNextStepLine,
		confirmationExecuteHintLine(true),
	)
	plain := strings.Join(plainParts, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain confirmation frame must stay free of escapes")
	}
	for _, want := range []string{
		"Permanent deletion · 1 categories · 2 item(s) · 1 GB",
		"Go cache · 2 item(s) · 1 GB · Permanent deletion",
		"Recycle Bin · 1 categories · 1 item(s) · 100 MB",
		"User temp · 1 item(s) · 100 MB · Recycle Bin",
		"Selected: 2 categories ·",
		"1 GB",
		"100 MB",
		confirmationPermanentIrreversibleWarning,
		confirmationNextStepLine,
		"Enter: start cleanup",
		"Impact: Rebuilds indexes after cleanup.",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("plain confirmation missing %q:\n%s", want, plain)
		}
	}
	// Groups stay separately reported; unselected categories stay out.
	if !strings.Contains(plain, "Permanent deletion ·") || !strings.Contains(plain, "Recycle Bin ·") {
		t.Fatalf("action groups not separately reported:\n%s", plain)
	}
	if strings.Contains(plain, "Off row") {
		t.Fatalf("unselected category leaked into confirmation:\n%s", plain)
	}
	// Summary lines appear before their detail rows.
	permSummary := strings.Index(plain, "Permanent deletion · 1 categories")
	permDetail := strings.Index(plain, "  - Go cache")
	if permSummary < 0 || permDetail < 0 || permSummary > permDetail {
		t.Fatalf("permanent summary must precede detail row:\n%s", plain)
	}
}

func TestConfirmationExecuteHintAndNextStepCopy(t *testing.T) {
	if confirmationExecuteHintLine(true) != "Enter: start cleanup | b/Esc: back to preview" {
		t.Fatalf("permanent hint = %q", confirmationExecuteHintLine(true))
	}
	if confirmationExecuteHintLine(false) != "Enter: execute | b/Esc: back to preview" {
		t.Fatalf("recycle-only hint = %q", confirmationExecuteHintLine(false))
	}
	if !strings.Contains(confirmationNextStepLine, "re-check selected categories") {
		t.Fatalf("next-step missing re-check fragment: %q", confirmationNextStepLine)
	}
	if !strings.Contains(confirmationNextStepLine, "may take a while") {
		t.Fatalf("next-step missing duration expectation: %q", confirmationNextStepLine)
	}
}

func TestStylizeFrameConfirmationMagnitudeAndRisk(t *testing.T) {
	plain := strings.Join([]string{
		"Foal Clean",
		"Confirm cleanup",
		"Permanent deletion · 1 categories · 2 item(s) · 1 GB",
		"Recycle Bin · 1 categories · 1 item(s) · 100 MB",
		"",
		"Permanent deletion",
		"  - Go cache · 2 item(s) · 1 GB · Permanent deletion",
		"Recycle Bin",
		"  - User temp · 1 item(s) · 2 KB · Recycle Bin",
		"Selected: 2 categories · 1 GB",
		confirmationPermanentIrreversibleWarning,
		confirmationRecycleRecoverabilityNote,
		confirmationActionTypeCaveat,
		confirmationNextStepLine,
		"Enter: start cleanup | b/Esc: back to preview",
	}, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain frame must stay free of escapes")
	}
	styled := stylizeFrame(plain)
	// Plain fragments remain the contract oracle.
	for _, want := range []string{
		"1 GB",
		"100 MB",
		"2 KB",
		confirmationPermanentIrreversibleWarning,
		confirmationNextStepLine,
		"Permanent deletion · 1 categories",
		"Recycle Bin · 1 categories",
		"Selected: 2 categories",
		"Enter: start cleanup",
	} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled confirmation missing plain fragment %q:\n%q", want, styled)
		}
	}
	// Magnitude applied on confirmation measured lines + Selected.
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("styled confirmation should include magnitude or risk escapes when color enabled")
	}

	// Risk warning uses pure red risk emphasis, not magnitude orange/amber.
	var warningLine string
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "irreversible") {
			warningLine = line
			break
		}
	}
	if warningLine == "" {
		t.Fatal("warning line missing from styled frame")
	}
	if !strings.Contains(warningLine, confirmationPermanentIrreversibleWarning) {
		t.Fatalf("warning lost plain copy: %q", warningLine)
	}
	// Pure red CSI (31 / 91) or lipgloss 256 red "1"/"9" may appear for risk.
	// Magnitude orange/amber (208/214) must not paint the irreversible warning.
	if strings.Contains(warningLine, "208") || strings.Contains(warningLine, "214") {
		t.Fatalf("risk warning must not use magnitude-orange: %q", warningLine)
	}
	if warningLine == confirmationPermanentIrreversibleWarning {
		t.Fatal("risk warning should receive emphasis when color enabled")
	}
}

func TestStyleRiskWarningNoColorAndRed(t *testing.T) {
	text := confirmationPermanentIrreversibleWarning
	colored := styleRiskWarningWithColor(text, true)
	if colored == text {
		t.Fatal("colored risk path should decorate warning")
	}
	if !strings.Contains(colored, text) {
		t.Fatalf("risk style lost plain fragment: %q", colored)
	}
	// Must not use magnitude orange/amber as the risk cue.
	if strings.Contains(colored, "208") || strings.Contains(colored, "214") {
		t.Fatalf("risk style used magnitude hues: %q", colored)
	}

	noColor := styleRiskWarningWithColor(text, false)
	if !strings.Contains(noColor, text) {
		t.Fatalf("NO_COLOR risk path lost fragment: %q", noColor)
	}
	if strings.Contains(noColor, "208") || strings.Contains(noColor, "214") {
		t.Fatalf("NO_COLOR risk path leaked magnitude hues: %q", noColor)
	}
	// Without color, hues for pure red should not appear either.
	if strings.Contains(noColor, "[31m") || strings.Contains(noColor, "[91m") {
		t.Fatalf("NO_COLOR risk path leaked red CSI: %q", noColor)
	}
}

func TestStylizeFrameConfirmationRiskRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	plain := strings.Join([]string{
		"Permanent deletion · 1 categories · 1 item(s) · 1 GB",
		confirmationPermanentIrreversibleWarning,
	}, "\n")
	styled := stylizeFrame(plain)
	if !strings.Contains(styled, "1 GB") || !strings.Contains(styled, confirmationPermanentIrreversibleWarning) {
		t.Fatalf("NO_COLOR confirmation lost plain fragments:\n%q", styled)
	}
	if strings.Contains(styled, "208") || strings.Contains(styled, "214") {
		t.Fatalf("NO_COLOR confirmation leaked magnitude hues:\n%q", styled)
	}
	if strings.Contains(styled, "[31m") || strings.Contains(styled, "[91m") {
		t.Fatalf("NO_COLOR confirmation leaked risk red CSI:\n%q", styled)
	}
}

// stripANSIForTest removes common CSI sequences so tests can compare plain copy
// without treating ANSI as the contract oracle.
func stripANSIForTest(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					break
				}
			}
			i = j - 1
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestStylizeFrameMagnitudeOnResultSuccessBytes(t *testing.T) {
	// Plain result frame: successful affected-style tokens only on cleaned/partial
	// rows and result totals. Non-success outcomes invent no success-byte field.
	plain := strings.Join([]string{
		"Foal Clean",
		"Cleanup result",
		"  ✓ Big cache · cleaned · 1 GB",
		"  ! Mid cache · partial · 100 MB",
		"  ⊘ Skip me · skipped",
		"  – Empty · empty",
		"  ! Fail · failed",
		"  ! Stop · canceled",
		"",
		"Processed: 5/6",
		"Recycle Bin moved: 100 MB",
		"Permanently deleted: 1 GB",
		"Affected (processed): 1 GB",
		"",
		"Enter/Esc/b: menu · q: quit",
	}, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain frame must stay free of escapes")
	}
	for _, banned := range []string{"freed", "reclaimed disk", "space reclaimed", "space freed"} {
		if strings.Contains(strings.ToLower(plain), banned) {
			t.Fatalf("must not label aggregate as freed/reclaimed (%q):\n%s", banned, plain)
		}
	}
	// Plain fragments are the contract oracle.
	for _, want := range []string{
		"cleaned · 1 GB",
		"partial · 100 MB",
		"Recycle Bin moved: 100 MB",
		"Permanently deleted: 1 GB",
		"Affected (processed): 1 GB",
		"skipped",
		"empty",
		"failed",
		"canceled",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("plain result missing fragment %q:\n%s", want, plain)
		}
	}

	styled := stylizeFrame(plain)
	for _, want := range []string{"1 GB", "100 MB", "cleaned", "partial", "Affected (processed):", "skipped"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled result missing plain fragment %q:\n%q", want, styled)
		}
	}
	// Magnitude applied to successful affected-style tokens when color enabled.
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("styled result should include magnitude escapes when color enabled")
	}
	// Strong tier must not use pure red as size cue.
	if strings.Contains(styled, "[31m") || strings.Contains(styled, "[91m") {
		t.Fatalf("result magnitude must not use pure red:\n%q", styled)
	}
	// No full success-green / fail-red result palette in this slice.
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "cleaned") || strings.Contains(line, "failed") {
			if strings.Contains(line, "[32m") || strings.Contains(line, "[92m") ||
				strings.Contains(line, "[31m") || strings.Contains(line, "[91m") {
				t.Fatalf("result must not use success-green/fail-red palette: %q", line)
			}
		}
	}
	// Non-success outcome lines invent no byte token for missing success bytes.
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "skipped") || strings.Contains(line, "empty") ||
			strings.Contains(line, "failed") || strings.Contains(line, "canceled") {
			if cleanByteTokenPattern.MatchString(line) {
				t.Fatalf("non-success outcome invented byte token: %q", line)
			}
		}
	}
}

func TestStylizeFrameResultZeroAndNonSuccessNoFalseMagnitude(t *testing.T) {
	plain := strings.Join([]string{
		"  ✓ Tiny · cleaned · 0 KB",
		"  ⊘ Skip · skipped",
		"  ! Fail · failed",
		"Recycle Bin moved: 0 KB",
		"Permanently deleted: 0 KB",
		"Affected (processed): 0 KB",
	}, "\n")
	styled := stylizeFrame(plain)
	for _, want := range []string{"0 KB", "skipped", "failed", "Affected (processed):"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled zero/non-success missing fragment %q:\n%q", want, styled)
		}
	}
	// Zero success bytes and non-success rows get no magnitude hues.
	if strings.Contains(styled, "208") || strings.Contains(styled, "214") {
		t.Fatalf("zero/non-success must not get magnitude hues:\n%q", styled)
	}
}

func TestStylizeFrameResultMagnitudeRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	plain := strings.Join([]string{
		"  ✓ Big · cleaned · 1 GB",
		"Recycle Bin moved: 100 MB",
		"Permanently deleted: 1 GB",
		"Affected (processed): 1 GB",
	}, "\n")
	styled := stylizeFrame(plain)
	for _, want := range []string{"1 GB", "100 MB", "Affected (processed):"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("NO_COLOR result lost plain fragment %q:\n%q", want, styled)
		}
	}
	if strings.Contains(styled, "208") || strings.Contains(styled, "214") {
		t.Fatalf("NO_COLOR result leaked magnitude hues:\n%q", styled)
	}
}

func TestIsMagnitudeEligibleLineResultSurfaces(t *testing.T) {
	eligible := []string{
		"  ✓ Big cache · cleaned · 1 GB",
		"  ! Mid · partial · 100 MB",
		"Recycle Bin moved: 100 MB",
		"Permanently deleted: 1 GB",
		"Affected (processed): 1 GB",
		"Selected: 1 categories · 1 GB",
		"    [x] ✓ Cache · 1 item(s) · 1 GB",
	}
	for _, line := range eligible {
		if !isMagnitudeEligibleLine(line) {
			t.Fatalf("expected eligible: %q", line)
		}
	}
	// Execution progress footer is mid-line and not a result total line.
	// Non-success outcomes and in-progress states are not magnitude surfaces.
	ineligible := []string{
		"Processed: 1/2 · Affected (processed): 0 KB",
		"  ⊘ Skip · skipped",
		"  – Empty · empty",
		"  ! Fail · failed",
		"  ⠋ Work · cleaning",
		"  ⠋ Work · rechecking",
		"Hints: Enter/Esc/b menu · q quit",
	}
	for _, line := range ineligible {
		if isMagnitudeEligibleLine(line) {
			t.Fatalf("expected ineligible: %q", line)
		}
	}
	// Confirmation measured lines remain eligible after #271 (separate surface).
	if !isMagnitudeEligibleLine("Permanent deletion · 1 categories · 1 item(s) · 1 GB") {
		t.Fatal("confirmation measured line should stay magnitude-eligible")
	}
}

func TestEagerUnavailableContentPure(t *testing.T) {
	content := eagerUnavailableContent("protection_load_failed", "cannot load protection")
	if !strings.Contains(content, "Clean unavailable") ||
		!strings.Contains(content, "protection_load_failed") ||
		!strings.Contains(content, "cannot load protection") {
		t.Fatalf("content = %q", content)
	}
	defaults := eagerUnavailableContent("", "")
	if !strings.Contains(defaults, "unavailable") || !strings.Contains(defaults, "Clean cannot start.") {
		t.Fatalf("defaults = %q", defaults)
	}
}

func TestStylizeFrameStateMarkersAndChrome(t *testing.T) {
	// Minimal polish: marker hues + heading/rule chrome. Plain remains oracle;
	// no success-green / pure-red on reliability markers (ADR 0023).
	plain := strings.Join([]string{
		"Foal Clean",
		"Scanning 1/2 · Confirmation available after scan completes · 3s",
		"  > [x] ✓ Done · 1 item(s) · 2 KB",
		"    [ ] ! Partial · 1 item(s) · 2 KB · partial",
		"    [ ] ⊘ Skip · skipped",
		"    [ ] … Wait · waiting",
		"    [ ] – Empty · empty",
		"  ✓ Cleaned · cleaned · 2 KB",
		"  ! Failed · failed",
		strings.Repeat("=", 12),
		permanentSelectionNotice,
		"Hints: space toggle",
	}, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain frame must stay free of escapes")
	}

	styled := stylizeFrame(plain)
	if stripANSIForTest(styled) != plain {
		t.Fatalf("styled plain projection mismatch:\n got %q\nwant %q", stripANSIForTest(styled), plain)
	}
	for _, want := range []string{"Foal Clean", "✓", "!", "⊘", "…", "–", permanentSelectionNotice, "Hints:"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled frame missing plain fragment %q:\n%q", want, styled)
		}
	}
	if !strings.Contains(styled, "\x1b[") {
		t.Fatal("styled frame should include chrome/marker escapes when color enabled")
	}
	// Reliability markers must not use pure red or classic success-green.
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "failed") || strings.Contains(line, "Partial") ||
			strings.Contains(line, "Done") || strings.Contains(line, "Cleaned") {
			if strings.Contains(line, "[31m") || strings.Contains(line, "[91m") ||
				strings.Contains(line, "[32m") || strings.Contains(line, "[92m") {
				t.Fatalf("marker line used risk-red or success-green: %q", line)
			}
		}
	}
	// Permanent notice is risk-channel red/bold.
	var noticeLine string
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, permanentSelectionNotice) {
			noticeLine = line
			break
		}
	}
	if noticeLine == "" || noticeLine == permanentSelectionNotice {
		t.Fatalf("permanent notice should receive risk emphasis: %q", noticeLine)
	}
}

func TestStylizeFrameStateMarkersRespectNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	plain := strings.Join([]string{
		"Foal Clean",
		"    [ ] ✓ Done · 1 item(s) · 2 KB",
		"    [ ] ! Partial · partial",
		"  ⊘ Skip · skipped",
		strings.Repeat("=", 8),
		permanentSelectionNotice,
	}, "\n")
	styled := stylizeFrame(plain)
	if stripANSIForTest(styled) != plain && !strings.Contains(stripANSIForTest(styled), "Foal Clean") {
		t.Fatalf("NO_COLOR path lost plain copy:\n%q", styled)
	}
	for _, hue := range []string{"208", "214", "240", "81", "14", "11", "8"} {
		// Hue indexes may appear as plain text only if labels contain them; assert
		// they do not appear as 256-color parameters for our chrome styles.
		if strings.Contains(styled, "38;5;"+hue) || strings.Contains(styled, "38;5;"+hue+"m") {
			t.Fatalf("NO_COLOR leaked hue %s:\n%q", hue, styled)
		}
	}
	if strings.Contains(styled, "[31m") || strings.Contains(styled, "[91m") {
		t.Fatalf("NO_COLOR leaked pure red CSI:\n%q", styled)
	}
}
