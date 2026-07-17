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
			RecycleBinMovedBytes:     10,
			PermanentlyDeletedBytes:  20,
			AffectedBytes:            30,
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
