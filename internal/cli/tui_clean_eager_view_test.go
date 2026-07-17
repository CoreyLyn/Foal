package cli

import (
	"strings"
	"testing"

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
		{Identifier: "recycle", Selected: true, PlannedAction: clean.DeletionActionMoveToRecycleBin, CandidateCount: 2, Bytes: 100},
		{Identifier: "perm", Selected: true, PlannedAction: clean.DeletionActionDeletePermanently, CandidateCount: 1, Bytes: 50},
		{Identifier: "off", Selected: false, PlannedAction: clean.DeletionActionDeletePermanently, CandidateCount: 9, Bytes: 9},
		{Identifier: "unknown", Selected: true, PlannedAction: clean.DeletionAction(""), CandidateCount: 3, Bytes: 30},
	}
	permanent, recycle := eagerConfirmationActionGroups(rows)
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
		PlannedAction:  clean.DeletionActionMoveToRecycleBin,
	}
	if got := eagerPreviewRowLabel(row); got != "bin · Temp · 2 item(s) · 2 KB" {
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

func TestEagerPlannedActionMarkersProjectCatalogOnly(t *testing.T) {
	if got := eagerPlannedActionMarker(clean.DeletionActionDeletePermanently); got != "perm" {
		t.Fatalf("permanent marker = %q", got)
	}
	if got := eagerPlannedActionMarker(clean.DeletionActionMoveToRecycleBin); got != "bin" {
		t.Fatalf("recycle marker = %q", got)
	}
	// Unknown/missing project as bin (same default as confirmation grouping).
	if got := eagerPlannedActionMarker(clean.DeletionAction("")); got != "bin" {
		t.Fatalf("empty marker = %q", got)
	}

	perm := eagerCategoryRow{
		Label:          "D3D",
		State:          clean.CategoryPreviewComplete,
		CandidateCount: 1,
		Bytes:          1024,
		PlannedAction:  clean.DeletionActionDeletePermanently,
	}
	if got := eagerPreviewRowLabel(perm); got != "perm · D3D · 1 item(s) · 1 KB" {
		t.Fatalf("perm complete = %q", got)
	}
	partial := perm
	partial.State = clean.CategoryPreviewPartial
	if got := eagerPreviewRowLabel(partial); got != "perm · D3D · 1 item(s) · 1 KB · partial" {
		t.Fatalf("perm partial = %q", got)
	}
	// Non-measured states still project catalog action; no invented bytes.
	for _, state := range []clean.CategoryPreviewState{
		clean.CategoryPreviewWaiting,
		clean.CategoryPreviewScanning,
		clean.CategoryPreviewEmpty,
		clean.CategoryPreviewSkipped,
		clean.CategoryPreviewIncomplete,
		clean.CategoryPreviewFailed,
	} {
		row := eagerCategoryRow{Label: "Cache", State: state, PlannedAction: clean.DeletionActionDeletePermanently}
		got := eagerPreviewRowLabel(row)
		if !strings.HasPrefix(got, "perm · ") {
			t.Fatalf("state %q missing perm marker: %q", state, got)
		}
		if strings.Contains(got, "KB") || strings.Contains(got, "MB") || strings.Contains(got, "GB") {
			t.Fatalf("state %q invented byte token: %q", state, got)
		}
	}
	binWaiting := eagerCategoryRow{
		Label:         "Temp",
		State:         clean.CategoryPreviewWaiting,
		PlannedAction: clean.DeletionActionMoveToRecycleBin,
	}
	if got := eagerPreviewRowLabel(binWaiting); got != "bin · Temp · waiting" {
		t.Fatalf("bin waiting = %q", got)
	}
}

func TestEagerPermanentSelectionNoticePresence(t *testing.T) {
	withPerm := []eagerCategoryRow{
		{Identifier: "a", Selected: true, PlannedAction: clean.DeletionActionDeletePermanently},
		{Identifier: "b", Selected: true, PlannedAction: clean.DeletionActionMoveToRecycleBin},
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
		{Identifier: "a", Selected: false, PlannedAction: clean.DeletionActionDeletePermanently},
		{Identifier: "b", Selected: true, PlannedAction: clean.DeletionActionMoveToRecycleBin},
	}
	if got := eagerPermanentSelectionNotice(eagerSelectionIncludesPermanent(permOff)); got != "" {
		t.Fatalf("cleared permanent selection must remove notice, got %q", got)
	}
	// Empty selection.
	if got := eagerPermanentSelectionNotice(false); got != "" {
		t.Fatalf("no permanent flag must yield empty notice, got %q", got)
	}
}

func TestEagerFooterHintsDocumentPlannedActionLegend(t *testing.T) {
	const legend = "perm=permanent · bin=Recycle Bin"
	base := eagerFooterHints(false, clean.EagerPreviewNoWorkNone, false)
	if !strings.Contains(base, legend) {
		t.Fatalf("in-scan hints missing legend: %q", base)
	}
	ready := eagerFooterHints(true, clean.EagerPreviewNoWorkNone, true)
	if !strings.Contains(ready, legend) || !strings.Contains(ready, "enter confirm") {
		t.Fatalf("ready hints missing legend or enter: %q", ready)
	}
	// Legend documents markers; does not authorize cleanup.
	for _, forbidden := range []string{"execute now", "authorized", "will delete"} {
		if strings.Contains(strings.ToLower(base), forbidden) {
			t.Fatalf("hints must not authorize cleanup (%q): %s", forbidden, base)
		}
	}
}

func TestEagerExecutionMarkersAndLabelsPure(t *testing.T) {
	if got := eagerExecutionRowMarker(clean.CategoryExecutionCleaning, 2); got != eagerPreviewSpinnerFrames[2] {
		t.Fatalf("cleaning marker = %q", got)
	}
	if got := eagerExecutionRowMarker(clean.CategoryExecutionCleaned, 0); got != "✓" {
		t.Fatalf("cleaned marker = %q", got)
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

	base := eagerFooterHints(false, clean.EagerPreviewNoWorkNone, false)
	if !strings.Contains(base, "space toggle") || strings.Contains(base, "enter confirm") {
		t.Fatalf("in-scan footer = %q", base)
	}
	if !strings.Contains(base, "perm=permanent · bin=Recycle Bin") {
		t.Fatalf("in-scan footer missing planned-action legend: %q", base)
	}
	if got := eagerFooterHints(true, clean.EagerPreviewNoWorkNeedSelection, false); !strings.Contains(got, "Select at least one") {
		t.Fatalf("need selection footer = %q", got)
	}
	if got := eagerFooterHints(true, clean.EagerPreviewNoWorkNone, true); !strings.Contains(got, "enter confirm") {
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
		{Label: "Short", State: clean.CategoryPreviewComplete, CandidateCount: 1, Bytes: 2048, PlannedAction: clean.DeletionActionMoveToRecycleBin},
		{Label: "Much Longer Category Label", State: clean.CategoryPreviewPartial, CandidateCount: 12, Bytes: 3 * 1024 * 1024 * 1024, PlannedAction: clean.DeletionActionDeletePermanently},
		{Label: "Waiting", State: clean.CategoryPreviewWaiting, PlannedAction: clean.DeletionActionMoveToRecycleBin},
		{Label: "Empty", State: clean.CategoryPreviewEmpty, PlannedAction: clean.DeletionActionDeletePermanently},
	}
	leftWidth, byteWidth := eagerPreviewByteColumnWidths(rows)
	if leftWidth == 0 || byteWidth == 0 {
		t.Fatalf("widths left=%d byte=%d", leftWidth, byteWidth)
	}

	complete := eagerPreviewRowLabelAligned(rows[0], leftWidth, byteWidth)
	partial := eagerPreviewRowLabelAligned(rows[1], leftWidth, byteWidth)
	// Plain fragments remain findable (oracle), including planned-action markers.
	if !strings.Contains(complete, "bin · Short") || !strings.Contains(complete, "2 KB") {
		t.Fatalf("complete = %q", complete)
	}
	if !strings.Contains(partial, "perm ·") || !strings.Contains(partial, "3 GB") || !strings.Contains(partial, "partial") {
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
	if !strings.Contains(empty, "perm · Empty") {
		t.Fatalf("empty missing planned-action marker: %q", empty)
	}
}

func TestStyleMagnitudeTokenNoColorAndHues(t *testing.T) {
	attention := styleMagnitudeTokenWithColor("100 MB", cleanMagnitudeAttention, true)
	strong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, true)
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

	noColorAttention := styleMagnitudeTokenWithColor("100 MB", cleanMagnitudeAttention, false)
	noColorStrong := styleMagnitudeTokenWithColor("1 GB", cleanMagnitudeStrong, false)
	if !strings.Contains(noColorAttention, "100 MB") || !strings.Contains(noColorStrong, "1 GB") {
		t.Fatalf("NO_COLOR path lost fragments: %q %q", noColorAttention, noColorStrong)
	}
	// Without color, no amber/orange foreground; bold may remain.
	if strings.Contains(noColorAttention, "214") || strings.Contains(noColorStrong, "208") {
		t.Fatalf("NO_COLOR path leaked magnitude hues: %q %q", noColorAttention, noColorStrong)
	}

	// Zero/none and neutral stay plain (no invented magnitude color).
	if got := styleMagnitudeTokenWithColor("0 KB", cleanMagnitudeNone, true); got != "0 KB" {
		t.Fatalf("none tier = %q", got)
	}
	if got := styleMagnitudeTokenWithColor("2 KB", cleanMagnitudeNeutral, true); got != "2 KB" {
		t.Fatalf("neutral tier = %q", got)
	}
}

func TestStylizeFrameMagnitudeOnPreviewAndSelected(t *testing.T) {
	plain := strings.Join([]string{
		"Foal Clean",
		"  > [x] ✓ perm · Big · 1 item(s) · 1 GB",
		"    [ ] ✓ bin · Mid · 1 item(s) · 100 MB",
		"    [ ] … bin · Wait · waiting",
		"Selected: 1 categories · 1 GB",
		"Selection includes permanent deletion.",
		"Hints: space toggle · perm=permanent · bin=Recycle Bin",
	}, "\n")
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("plain frame must stay free of escapes")
	}
	styled := stylizeFrame(plain)
	// Plain fragments remain the contract (markers, notice, legend, bytes).
	for _, want := range []string{
		"perm · Big", "bin · Mid", "1 GB", "100 MB", "waiting",
		"Selected: 1 categories", "includes permanent deletion",
		"perm=permanent · bin=Recycle Bin",
	} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled frame missing plain fragment %q:\n%q", want, styled)
		}
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
	// Planned-action markers must not receive pure-red whole-row risk tint.
	for _, line := range strings.Split(styled, "\n") {
		if !strings.Contains(line, "perm ·") && !strings.Contains(line, "bin ·") {
			continue
		}
		if strings.Contains(line, "[31m") || strings.Contains(line, "[91m") {
			t.Fatalf("preview risk marker row must not use pure red: %q", line)
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
