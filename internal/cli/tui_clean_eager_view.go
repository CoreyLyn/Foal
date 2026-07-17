package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Execution chrome: keep the header visibly alive during long Fresh scanning /
// permanent deletion without inventing percentages or path-backed progress.
const (
	eagerExecutionStillWorkingThreshold    = 3 * time.Second
	eagerExecutionStillScanningReassurance = "Still re-checking selected categories…"
)

// tui_clean_eager_view.go holds pure path-free presentation and selection
// view-model helpers for category-first Clean. These functions take plain data
// (rows, outcomes, flags) and return derived presentation state. They must not
// import Bubble Tea, touch I/O, resolve candidates, delete, stop processes, or
// elevate. Domain classification stays in internal/clean; this file only
// projects and formats.

// --- Selection / confirmation math ---

// eagerAllCategoriesTerminal reports whether every scannable category has a
// terminal path-free outcome. Empty queues are not terminal.
func eagerAllCategoriesTerminal(rows []eagerCategoryRow) bool {
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if !clean.IsTerminalPreviewState(row.State) {
			return false
		}
	}
	return true
}

// eagerSelectedCount returns how many categories are currently authorized.
func eagerSelectedCount(rows []eagerCategoryRow) int {
	n := 0
	for _, row := range rows {
		if row.Selected {
			n++
		}
	}
	return n
}

// eagerSelectedCategoryIDs returns canonical identifiers in stable display/scan
// order. Contains only selected identifiers — no aliases, group tokens, or paths.
func eagerSelectedCategoryIDs(rows []eagerCategoryRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Selected {
			ids = append(ids, row.Identifier)
		}
	}
	return ids
}

// eagerSelectionTotals returns selected category count, safely measured bytes
// for complete/partial selected rows, and selected waiting/scanning pending
// count. Unfinished, empty, skipped, incomplete, and failed work contributes
// no bytes.
func eagerSelectionTotals(rows []eagerCategoryRow) (categories int, measuredBytes int64, pending int) {
	for _, row := range rows {
		if !row.Selected {
			continue
		}
		categories++
		switch row.State {
		case clean.CategoryPreviewWaiting, clean.CategoryPreviewScanning:
			pending++
		case clean.CategoryPreviewComplete, clean.CategoryPreviewPartial:
			measuredBytes += row.Bytes
		}
	}
	return categories, measuredBytes, pending
}

// eagerRowSelectable reports whether Space may toggle the row.
func eagerRowSelectable(row eagerCategoryRow) bool {
	return clean.SelectablePreviewOutcome(row.State)
}

// eagerSelectionIncludesPermanent reports whether the exact selection discloses
// any delete_permanently planned action (catalog-owned).
func eagerSelectionIncludesPermanent(rows []eagerCategoryRow) bool {
	for _, row := range rows {
		if row.Selected && row.PlannedAction == clean.DeletionActionDeletePermanently {
			return true
		}
	}
	return false
}

// eagerConfirmationActionGroups splits the exact selection into Permanent
// deletion and Recycle Bin work. Empty groups are omitted by callers.
// Action is catalog-owned; unknown/missing actions present as Recycle Bin only.
func eagerConfirmationActionGroups(rows []eagerCategoryRow) (permanent, recycle []eagerCategoryRow) {
	for _, row := range rows {
		if !row.Selected {
			continue
		}
		switch row.PlannedAction {
		case clean.DeletionActionDeletePermanently:
			permanent = append(permanent, row)
		default:
			recycle = append(recycle, row)
		}
	}
	return permanent, recycle
}

// confirmationGroupTotals aggregates category/candidate/byte counts for one
// confirmation action group.
func confirmationGroupTotals(rows []eagerCategoryRow) (categories, candidates int, bytes int64) {
	for _, row := range rows {
		categories++
		candidates += row.CandidateCount
		bytes += row.Bytes
	}
	return categories, candidates, bytes
}

// eagerConfirmationEnabled requires every scannable category terminal and a
// non-empty exact selection before the first Enter may open confirmation.
func eagerConfirmationEnabled(unavailable, canceled bool, phase eagerCleanPhase, rows []eagerCategoryRow, executionStarted bool) bool {
	return !unavailable && !canceled && phase == eagerPhasePreview &&
		eagerAllCategoriesTerminal(rows) && eagerSelectedCount(rows) > 0 && !executionStarted
}

// eagerNoWorkState classifies finished zero-selection presentation via domain
// ClassifyEagerPreviewNoWork. Unavailable/canceled yield no no-work banner.
func eagerNoWorkState(rows []eagerCategoryRow, selectedCount int, unavailable, canceled bool) clean.EagerPreviewNoWorkState {
	if unavailable || canceled {
		return clean.EagerPreviewNoWorkNone
	}
	observations := make([]clean.CategoryPreviewObservation, len(rows))
	for i, row := range rows {
		observations[i] = clean.CategoryPreviewObservation{
			Identifier:     row.Identifier,
			State:          row.State,
			CandidateCount: row.CandidateCount,
			Bytes:          row.Bytes,
		}
	}
	return clean.ClassifyEagerPreviewNoWork(observations, selectedCount)
}

// --- Path-free marker / label projection ---

// eagerPermanentSelectionNotice returns the path-free footer sentence shown
// only while the exact selection includes at least one permanent-delete
// category. Empty when no permanent work is selected. Preview rows do not
// show per-row perm/bin markers; risk stays on this notice + confirmation.
func eagerPermanentSelectionNotice(includesPermanent bool) string {
	if !includesPermanent {
		return ""
	}
	return "Selection includes permanent deletion."
}

// eagerFooterRuleLine is the horizontal rule framing the preview selection /
// focus info block. Uses terminal width when known; otherwise a stable 70-col
// default matching the documented mockup.
func eagerFooterRuleLine(width int) string {
	n := 70
	if width > 0 {
		n = width
	}
	if n < 1 {
		n = 1
	}
	return strings.Repeat("=", n)
}

// eagerPreviewRowMarker maps a preview state to a single-glyph marker.
// spinnerFrame is only used for the scanning state.
func eagerPreviewRowMarker(state clean.CategoryPreviewState, spinnerFrame int) string {
	switch state {
	case clean.CategoryPreviewWaiting:
		return "…"
	case clean.CategoryPreviewScanning:
		return eagerPreviewSpinnerFrames[spinnerFrame%len(eagerPreviewSpinnerFrames)]
	case clean.CategoryPreviewComplete:
		return "✓"
	case clean.CategoryPreviewEmpty:
		return "–"
	case clean.CategoryPreviewSkipped:
		return "⊘"
	case clean.CategoryPreviewPartial, clean.CategoryPreviewIncomplete, clean.CategoryPreviewFailed:
		return "!"
	default:
		return "!"
	}
}

// eagerPreviewRowLabel formats one preview category row without paths.
// Byte fields are unpadded; use eagerPreviewRowLabelAligned for list alignment.
func eagerPreviewRowLabel(row eagerCategoryRow) string {
	return eagerPreviewRowLabelAligned(row, 0, 0)
}

// eagerPreviewRowLabelAligned formats one preview category row and, for
// complete/partial outcomes, column-aligns the trusted byte token using the
// supplied left and byte field widths (runes/bytes of the plain token text).
// Widths of 0 disable padding. Waiting/scanning/empty/skipped/incomplete/failed
// never invent a byte token or magnitude field. Planned deletion action is not
// shown as a per-row prefix (risk channel: footer notice + confirmation).
func eagerPreviewRowLabelAligned(row eagerCategoryRow, leftWidth, byteWidth int) string {
	switch row.State {
	case clean.CategoryPreviewComplete:
		return formatEagerPreviewMeasuredLabel(row.Label, row.CandidateCount, row.Bytes, leftWidth, byteWidth, "")
	case clean.CategoryPreviewPartial:
		return formatEagerPreviewMeasuredLabel(row.Label, row.CandidateCount, row.Bytes, leftWidth, byteWidth, " · partial")
	case clean.CategoryPreviewEmpty:
		return fmt.Sprintf("%s · empty", row.Label)
	case clean.CategoryPreviewSkipped:
		return fmt.Sprintf("%s · skipped", row.Label)
	case clean.CategoryPreviewIncomplete:
		return fmt.Sprintf("%s · incomplete", row.Label)
	case clean.CategoryPreviewFailed:
		return fmt.Sprintf("%s · failed", row.Label)
	case clean.CategoryPreviewWaiting:
		return fmt.Sprintf("%s · waiting", row.Label)
	case clean.CategoryPreviewScanning:
		return fmt.Sprintf("%s · scanning", row.Label)
	default:
		return row.Label
	}
}

// formatEagerPreviewMeasuredLabel builds
// "Label · N item(s) · <bytes>[suffix]" with optional column alignment of the
// left prefix and the byte token.
func formatEagerPreviewMeasuredLabel(label string, candidates int, bytes int64, leftWidth, byteWidth int, suffix string) string {
	left := fmt.Sprintf("%s · %d item(s) · ", label, candidates)
	if leftWidth > len(left) {
		left = left + strings.Repeat(" ", leftWidth-len(left))
	}
	return left + cleanFormatBytesPadded(bytes, byteWidth) + suffix
}

// eagerPreviewByteColumnWidths returns the plain-text widths needed to
// column-align trusted complete/partial byte tokens across a preview list.
// Non-measured states do not contribute. Order is not changed.
func eagerPreviewByteColumnWidths(rows []eagerCategoryRow) (leftWidth, byteWidth int) {
	for _, row := range rows {
		switch row.State {
		case clean.CategoryPreviewComplete, clean.CategoryPreviewPartial:
			left := fmt.Sprintf("%s · %d item(s) · ", row.Label, row.CandidateCount)
			if len(left) > leftWidth {
				leftWidth = len(left)
			}
			formatted := cleanFormatBytes(row.Bytes)
			if len(formatted) > byteWidth {
				byteWidth = len(formatted)
			}
		}
	}
	return leftWidth, byteWidth
}

// cleanFormatBytesPadded right-aligns cleanFormatBytes within width. Width <= 0
// or already wider tokens return the unpadded form.
func cleanFormatBytesPadded(bytes int64, width int) string {
	formatted := cleanFormatBytes(bytes)
	if width <= len(formatted) {
		return formatted
	}
	return strings.Repeat(" ", width-len(formatted)) + formatted
}

// eagerExecutionRowMarker maps an execution state to a single-glyph marker.
// spinnerFrame is only used for in-progress states.
func eagerExecutionRowMarker(state clean.CategoryExecutionState, spinnerFrame int) string {
	switch state {
	case clean.CategoryExecutionRechecking, clean.CategoryExecutionReady, clean.CategoryExecutionCleaning:
		return eagerPreviewSpinnerFrames[spinnerFrame%len(eagerPreviewSpinnerFrames)]
	case clean.CategoryExecutionCleaned:
		return "✓"
	case clean.CategoryExecutionEmpty:
		return "–"
	case clean.CategoryExecutionSkipped:
		return "⊘"
	case clean.CategoryExecutionPartial, clean.CategoryExecutionFailed, clean.CategoryExecutionCanceled:
		return "!"
	default:
		return "!"
	}
}

// eagerExecutionRowLabel formats one execution/result outcome without paths.
func eagerExecutionRowLabel(outcome clean.CategoryExecutionOutcome) string {
	switch outcome.State {
	case clean.CategoryExecutionRechecking:
		return outcome.Label + " · rechecking"
	case clean.CategoryExecutionReady:
		return outcome.Label + " · ready"
	case clean.CategoryExecutionCleaning:
		return outcome.Label + " · cleaning"
	case clean.CategoryExecutionEmpty:
		return outcome.Label + " · empty"
	case clean.CategoryExecutionCleaned:
		return fmt.Sprintf("%s · cleaned · %s", outcome.Label, cleanFormatBytes(outcome.AffectedBytes))
	case clean.CategoryExecutionPartial:
		return fmt.Sprintf("%s · partial · %s", outcome.Label, cleanFormatBytes(outcome.AffectedBytes))
	case clean.CategoryExecutionSkipped:
		return outcome.Label + " · skipped"
	case clean.CategoryExecutionFailed:
		return outcome.Label + " · failed"
	case clean.CategoryExecutionCanceled:
		return outcome.Label + " · canceled"
	default:
		return outcome.Label
	}
}

// eagerCheckbox formats the selection checkbox glyph.
func eagerCheckbox(selected bool) string {
	if selected {
		return "[x]"
	}
	return "[ ]"
}

// eagerSelectionSummaryLine shows live selected totals. Unfinished selected
// work is pending, never guessed as zero bytes.
func eagerSelectionSummaryLine(categories int, measuredBytes int64, pending int) string {
	if pending > 0 {
		return fmt.Sprintf("Selected: %d categories · %s measured · %d pending", categories, cleanFormatBytes(measuredBytes), pending)
	}
	return fmt.Sprintf("Selected: %d categories · %s", categories, cleanFormatBytes(measuredBytes))
}

// eagerFocusedDetailBody is the path-free focused diagnostic body for one row.
func eagerFocusedDetailBody(row eagerCategoryRow) string {
	switch row.State {
	case clean.CategoryPreviewWaiting:
		return "Waiting to scan."
	case clean.CategoryPreviewScanning:
		return "Scanning…"
	case clean.CategoryPreviewComplete:
		return fmt.Sprintf("Complete · %d item(s) · %s", row.CandidateCount, cleanFormatBytes(row.Bytes))
	case clean.CategoryPreviewPartial:
		body := fmt.Sprintf("Partial · %d item(s) · %s", row.CandidateCount, cleanFormatBytes(row.Bytes))
		if row.ExcludedSiblingCount > 0 {
			body += fmt.Sprintf(" · %d excluded", row.ExcludedSiblingCount)
		}
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		return body
	case clean.CategoryPreviewEmpty:
		return "Empty · no candidates found"
	case clean.CategoryPreviewSkipped:
		body := "Skipped"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		if row.ExcludedSiblingCount > 0 {
			body += fmt.Sprintf(" · %d excluded", row.ExcludedSiblingCount)
		}
		return body
	case clean.CategoryPreviewIncomplete:
		body := "Incomplete"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		return body
	case clean.CategoryPreviewFailed:
		body := "Failed"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		} else {
			body += " · measurement failed"
		}
		return body
	default:
		return "Unknown state"
	}
}

// pathFreeReasonExplanation maps stable reason codes to short presentation
// text. Never forwards raw OS text that may embed paths.
func pathFreeReasonExplanation(code string) string {
	switch code {
	case clean.PreviewReasonProtected:
		return "protected by Protection rules"
	case clean.PreviewReasonApplicationRunning, clean.PreviewReasonDevToolRunning:
		return "application is running"
	case clean.PreviewReasonRunningStateUnknown:
		return "application state unknown"
	case clean.PreviewReasonInspectionLimit:
		return "inspection limit exceeded"
	case clean.PreviewReasonContextCanceled:
		return "scan canceled"
	case clean.PreviewReasonInspectionFailed:
		return "measurement failed"
	case clean.PreviewReasonEmpty:
		return "no candidates found"
	case "reparse_point":
		return "reparse point excluded"
	default:
		if code == "" {
			return "see category state"
		}
		return code
	}
}

// --- Result projection ---

// eagerResultTotals prefers authoritative Result totals and falls back to
// outcome projection when the Result has not yet populated byte fields.
func eagerResultTotals(result clean.Result, outcomes []clean.CategoryExecutionOutcome) (recycle, permanent, affected int64) {
	recycle = result.Totals.RecycleBinMovedBytes
	permanent = result.Totals.PermanentlyDeletedBytes
	affected = result.Totals.AffectedBytes
	if recycle == 0 && permanent == 0 && affected == 0 {
		recycle = clean.SumExecutionRecycleBinMovedBytes(outcomes)
		permanent = clean.SumExecutionPermanentlyDeletedBytes(outcomes)
		affected = clean.SumExecutionAffectedBytes(outcomes)
	}
	if affected == 0 {
		affected = recycle + permanent
	}
	return recycle, permanent, affected
}

// eagerExecutionElapsedLabel formats whole seconds since execution start.
func eagerExecutionElapsedLabel(elapsed time.Duration) string {
	seconds := int(elapsed.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%ds", seconds)
}

// eagerExecutionHeaderLine formats path-free execution chrome: spinner, phase,
// elapsed since execute began, and an optional still-working reassurance.
// No byte-derived percentages or candidate paths.
func eagerExecutionHeaderLine(spinnerFrame int, phaseLabel, elapsed, reassurance string) string {
	spinner := eagerPreviewSpinnerFrames[spinnerFrame%len(eagerPreviewSpinnerFrames)]
	line := fmt.Sprintf("%s %s · %s", spinner, phaseLabel, elapsed)
	if reassurance != "" {
		line += " · " + reassurance
	}
	return line
}

// eagerPreviewHeaderLine formats the path-free scanning/complete header without
// byte-derived percentages. n is the 1-based active or completed+1 index.
func eagerPreviewHeaderLine(canceled, finished, allTerminal bool, activeIndex, completed, total int, elapsed string) string {
	if canceled {
		return fmt.Sprintf("Canceled · %s", elapsed)
	}
	if finished && allTerminal {
		return fmt.Sprintf("Scan complete · %d/%d · %s", total, total, elapsed)
	}
	n := completed + 1
	if activeIndex >= 0 {
		n = activeIndex + 1
	}
	if n > total {
		n = total
	}
	if total == 0 {
		return fmt.Sprintf("Scanning 0/0 · %s", elapsed)
	}
	if !allTerminal {
		return fmt.Sprintf("Scanning %d/%d · Confirmation available after scan completes · %s", n, total, elapsed)
	}
	return fmt.Sprintf("Scanning %d/%d · %s", n, total, elapsed)
}

// eagerFooterHints returns path-free preview footer hints for the current
// terminal/selection state. Never authorizes cleanup.
func eagerFooterHints(allTerminal bool, noWork clean.EagerPreviewNoWorkState, confirmationEnabled bool) string {
	const base = "Hints: up/down browse · space toggle · a select all · x clear · b/Esc back · q quit"
	if !allTerminal {
		return base
	}
	switch noWork {
	case clean.EagerPreviewNoWorkNeedSelection:
		return "Select at least one category to continue.\n" + base
	case clean.EagerPreviewNoWorkAllEmpty:
		return "Nothing to clean.\n" + base
	case clean.EagerPreviewNoWorkDiagnostics:
		return "No selectable cleanup found. Some categories were skipped or could not be measured.\n" + base
	default:
		if confirmationEnabled {
			return "Hints: enter confirm · up/down browse · space toggle · a select all · x clear · b/Esc back · q quit"
		}
		return base
	}
}

// eagerUnavailableContent formats a path-free global unavailable surface.
func eagerUnavailableContent(code, message string) string {
	if code == "" {
		code = "unavailable"
	}
	if message == "" {
		message = "Clean cannot start."
	}
	return strings.Join([]string{
		"Clean unavailable",
		fmt.Sprintf("Code: %s", code),
		message,
		"",
		"Hints: Enter/Esc/b menu · q quit",
		"",
	}, "\n")
}

// cleanFormatBytes formats measured/affected sizes for Clean TUI chrome.
// Shared by category-first presentation only (no report-first surface remains).
func cleanFormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 KB"
	}
	if bytes < 1024 {
		return "<1 KB"
	}

	const (
		kilobyte = int64(1024)
		megabyte = 1024 * kilobyte
		gigabyte = 1024 * megabyte
	)

	value := float64(bytes) / float64(kilobyte)
	unit := "KB"
	if bytes >= gigabyte {
		value = float64(bytes) / float64(gigabyte)
		unit = "GB"
	} else if bytes >= megabyte {
		value = float64(bytes) / float64(megabyte)
		unit = "MB"
	}

	formatted := strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0")
	return formatted + " " + unit
}

// --- Magnitude emphasis (presentation only) ---

// cleanMagnitudeTier classifies a trusted measured/affected byte token for
// Clean TUI magnitude emphasis. Unfinished or missing sizes must not invent a
// tier; callers pass only trusted complete/partial measured values (or the
// measured portion of a selected total).
type cleanMagnitudeTier int

const (
	// cleanMagnitudeNone: zero, empty, skipped, or unfinished — no magnitude hue.
	cleanMagnitudeNone cleanMagnitudeTier = iota
	// cleanMagnitudeNeutral: trusted positive bytes strictly below 100 MiB.
	cleanMagnitudeNeutral
	// cleanMagnitudeAttention: >= 100 MiB and < 1 GiB (amber/yellow).
	cleanMagnitudeAttention
	// cleanMagnitudeStrong: >= 1 GiB (orange, never pure red for size).
	cleanMagnitudeStrong
)

const (
	// Absolute 1024-based thresholds aligned with cleanFormatBytes units.
	cleanMagnitudeAttentionBytes int64 = 100 * 1024 * 1024
	cleanMagnitudeStrongBytes    int64 = 1024 * 1024 * 1024
)

// cleanMagnitudeTierFromBytes classifies a trusted byte count.
// bytes <= 0 yields None (no magnitude color), not Neutral.
func cleanMagnitudeTierFromBytes(bytes int64) cleanMagnitudeTier {
	if bytes <= 0 {
		return cleanMagnitudeNone
	}
	if bytes >= cleanMagnitudeStrongBytes {
		return cleanMagnitudeStrong
	}
	if bytes >= cleanMagnitudeAttentionBytes {
		return cleanMagnitudeAttention
	}
	return cleanMagnitudeNeutral
}

// cleanMagnitudeTierFromFormattedToken classifies a cleanFormatBytes plain
// token by reverse-parsing the display unit scale. This is an explicit
// fallback for plain-only stylizeFrame callers that lack trusted int64
// metadata. Production Clean styling prefers cleanMagnitudeTierFromBytes via
// tuiStyleLine.HasMagnitudeBytes (see classifyMagnitudeTier).
func cleanMagnitudeTierFromFormattedToken(token string) cleanMagnitudeTier {
	token = strings.TrimSpace(token)
	if token == "" || token == "0 KB" {
		return cleanMagnitudeNone
	}
	approx, ok := parseCleanFormatBytesApprox(token)
	if !ok {
		return cleanMagnitudeNone
	}
	return cleanMagnitudeTierFromBytes(approx)
}

// parseCleanFormatBytesApprox maps a cleanFormatBytes token back to an
// approximate int64 for fallback tier classification only (not for accounting).
// Prefer cleanMagnitudeTierFromBytes with trusted counts on the production path.
func parseCleanFormatBytesApprox(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	if token == "0 KB" {
		return 0, true
	}
	if token == "<1 KB" {
		return 1, true
	}
	parts := strings.Split(token, " ")
	if len(parts) != 2 {
		return 0, false
	}
	var value float64
	if _, err := fmt.Sscanf(parts[0], "%f", &value); err != nil {
		return 0, false
	}
	if value < 0 {
		return 0, false
	}
	var mult int64
	switch parts[1] {
	case "KB":
		mult = 1024
	case "MB":
		mult = 1024 * 1024
	case "GB":
		mult = 1024 * 1024 * 1024
	default:
		return 0, false
	}
	return int64(value * float64(mult)), true
}
