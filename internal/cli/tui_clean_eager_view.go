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
// terminal path-free outcome. Empty queues are not terminal. Windows servicing
// rows are at rest unless an analysis is in flight (analyzing), so a servicing
// row never blocks confirmation of other categories.
func eagerAllCategoriesTerminal(rows []eagerCategoryRow) bool {
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if row.Servicing {
			if !clean.ServicingRowTerminalForConfirmation(row.ServicingState) {
				return false
			}
			continue
		}
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
// no bytes. Servicing rows contribute a category count only (no bytes, never
// pending): servicing has no reclaimable-byte measurement.
func eagerSelectionTotals(rows []eagerCategoryRow) (categories int, measuredBytes int64, pending int) {
	for _, row := range rows {
		if !row.Selected {
			continue
		}
		categories++
		if row.Servicing {
			continue
		}
		switch row.State {
		case clean.CategoryPreviewWaiting, clean.CategoryPreviewScanning:
			pending++
		case clean.CategoryPreviewComplete, clean.CategoryPreviewPartial:
			measuredBytes += row.Bytes
		}
	}
	return categories, measuredBytes, pending
}

// eagerRowSelectable reports whether Space may toggle the row. Servicing rows
// are selectable only when a fresh analysis has reported work (ready).
func eagerRowSelectable(row eagerCategoryRow) bool {
	if row.Servicing {
		return clean.ServicingRowSelectable(row.ServicingState)
	}
	return clean.SelectablePreviewOutcome(row.State)
}

// eagerSelectionIncludesServicing reports whether the exact selection includes
// any Windows servicing (invoke_windows_servicing) category.
func eagerSelectionIncludesServicing(rows []eagerCategoryRow) bool {
	for _, row := range rows {
		if row.Selected && row.Servicing {
			return true
		}
	}
	return false
}

// eagerSelectionIncludesPermanent reports whether the exact selection discloses
// any delete_permanently planned action (catalog-owned).
func eagerSelectionIncludesPermanent(rows []eagerCategoryRow) bool {
	for _, row := range rows {
		if row.Selected && row.PlannedAction == clean.PlannedActionDeletePermanently {
			return true
		}
	}
	return false
}

// eagerConfirmationActionGroups splits the exact selection into Permanent
// deletion, Recycle Bin, and Windows servicing work. Empty groups are omitted by
// callers. Action is catalog-owned; unknown/missing file actions present as
// Recycle Bin, while servicing rows form their own group.
func eagerConfirmationActionGroups(rows []eagerCategoryRow) (permanent, recycle, servicing []eagerCategoryRow) {
	for _, row := range rows {
		if !row.Selected {
			continue
		}
		if row.Servicing {
			servicing = append(servicing, row)
			continue
		}
		switch row.PlannedAction {
		case clean.PlannedActionDeletePermanently:
			permanent = append(permanent, row)
		default:
			recycle = append(recycle, row)
		}
	}
	return permanent, recycle, servicing
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

// Confirmation plain-frame copy (path-free). Tests assert these fragments.
const (
	// confirmationNextStepLine sets expectation before Enter executes: shared
	// Clean re-checks selected categories, then mutates. No path, no ETA.
	confirmationNextStepLine = "Next: re-check selected categories, then clean. This may take a while."
	// confirmationActionTypeCaveat is the permanent safety note that fresh
	// execution cannot introduce an undisclosed deletion action type.
	confirmationActionTypeCaveat = "Fresh execution cannot introduce a deletion action type that was not disclosed above."
	// confirmationRecycleRecoverabilityNote when the Recycle Bin group is present.
	confirmationRecycleRecoverabilityNote = "Recycle Bin items are moved, not permanently erased."
)

// Windows servicing confirmation disclosure copy (path-free, byte-free). Shown
// only when the exact selection includes a servicing category (ADR 0029). Each
// fragment is asserted by confirmation contract tests.
const (
	confirmationServicingAuthorizationLine = "Confirming grants this run's Windows servicing authorization (equivalent to --allow-servicing)."
	confirmationServicingUACLine           = "Windows servicing requests administrator consent (UAC) after ordinary cleanup finishes."
	confirmationServicingNoRestartLine     = "Foal runs component cleanup with /NoRestart and never reboots; a restart may still be required."
	confirmationServicingNonInterruptLine  = "Once servicing starts it cannot be canceled; Windows finishes the transaction."
	confirmationServicingBytesLine         = "Windows servicing reclaims components measured in packages, not bytes; size is unknown."
)

// confirmationServicingDisclosureLines are the ordered servicing disclosures for
// the confirmation footer.
func confirmationServicingDisclosureLines() []string {
	return []string{
		confirmationServicingAuthorizationLine,
		confirmationServicingUACLine,
		confirmationServicingNoRestartLine,
		confirmationServicingNonInterruptLine,
		confirmationServicingBytesLine,
	}
}

// confirmationServicingSummaryLine is the compact servicing action-group total
// used at the top of the confirmation body. Package count is disclosed; bytes
// are never estimated.
func confirmationServicingSummaryLine(rows []eagerCategoryRow) string {
	cats := len(rows)
	packages := 0
	for _, row := range rows {
		packages += row.ServicingReclaimablePackages
	}
	return fmt.Sprintf("Windows servicing · %d categories · %d reclaimable package(s) · size unknown", cats, packages)
}

// confirmationGroupSummaryLine is the compact action-group total used at the
// top of the confirmation body (summary-first). Empty groups must not call this.
func confirmationGroupSummaryLine(title string, rows []eagerCategoryRow) (line string, bytes int64) {
	cats, candidates, bytes := confirmationGroupTotals(rows)
	return fmt.Sprintf("%s · %d categories · %d item(s) · %s", title, cats, candidates, cleanFormatBytes(bytes)), bytes
}

// confirmationBodyEntriesFromGroups builds summary-first confirmation body lines:
// high-level Permanent / Recycle / Windows servicing totals first, then
// scrollable detail sections. Empty action groups are omitted entirely.
// Presentation only.
func confirmationBodyEntriesFromGroups(permanent, recycle, servicing []eagerCategoryRow) []eagerBodyLine {
	lines := make([]eagerBodyLine, 0, len(permanent)+len(recycle)+len(servicing)+10)
	appendSummary := func(title string, rows []eagerCategoryRow) {
		if len(rows) == 0 {
			return
		}
		text, bytes := confirmationGroupSummaryLine(title, rows)
		lines = append(lines, eagerBodyLine{
			text:              text,
			rowIndex:          -1,
			outcomeIndex:      -1,
			magnitudeBytes:    bytes,
			hasMagnitudeBytes: true,
		})
	}
	appendSummary("Permanent deletion", permanent)
	appendSummary("Recycle Bin", recycle)
	if len(servicing) > 0 {
		// Servicing summary is byte-free: it discloses a package count only.
		lines = append(lines, eagerBodyLine{
			text:         confirmationServicingSummaryLine(servicing),
			rowIndex:     -1,
			outcomeIndex: -1,
		})
	}

	if len(permanent) > 0 || len(recycle) > 0 || len(servicing) > 0 {
		lines = append(lines, eagerBodyLine{text: "", rowIndex: -1, outcomeIndex: -1})
	}

	appendDetails := func(title string, rows []eagerCategoryRow) {
		if len(rows) == 0 {
			return
		}
		lines = append(lines, eagerBodyLine{text: title, rowIndex: -1, outcomeIndex: -1})
		for _, row := range rows {
			action := clean.PlannedActionLabel(row.PlannedAction)
			lines = append(lines, eagerBodyLine{
				text: fmt.Sprintf("  - %s · %d item(s) · %s · %s",
					row.Label, row.CandidateCount, cleanFormatBytes(row.Bytes), action),
				rowIndex:          -1,
				outcomeIndex:      -1,
				magnitudeBytes:    row.Bytes,
				hasMagnitudeBytes: true,
			})
			if row.SafetyNote != "" {
				lines = append(lines, eagerBodyLine{
					text:         "      Impact: " + row.SafetyNote,
					rowIndex:     -1,
					outcomeIndex: -1,
				})
			}
		}
	}
	appendDetails("Permanent deletion", permanent)
	appendDetails("Recycle Bin", recycle)
	appendServicingDetails(&lines, servicing)
	return lines
}

// appendServicingDetails renders the byte-free Windows servicing detail section.
// Each row discloses its reclaimable-package count (never bytes), the planned
// Windows servicing action, and the UAC / non-interruptible impact.
func appendServicingDetails(lines *[]eagerBodyLine, servicing []eagerCategoryRow) {
	if len(servicing) == 0 {
		return
	}
	*lines = append(*lines, eagerBodyLine{text: "Windows servicing", rowIndex: -1, outcomeIndex: -1})
	for _, row := range servicing {
		*lines = append(*lines, eagerBodyLine{
			text: fmt.Sprintf("  - %s · %d reclaimable package(s) · size unknown · %s",
				row.Label, row.ServicingReclaimablePackages, clean.PlannedActionLabel(row.PlannedAction)),
			rowIndex:     -1,
			outcomeIndex: -1,
		})
		*lines = append(*lines, eagerBodyLine{
			text:         "      Impact: administrator consent (UAC) required; cannot be canceled once started.",
			rowIndex:     -1,
			outcomeIndex: -1,
		})
	}
}

// confirmationExecuteHintLine is the Enter / back key hint on confirmation.
// Permanent selections use slightly stronger "start cleanup" wording; recycle-
// only keeps "execute". No typed phrase or countdown.
func confirmationExecuteHintLine(includesPermanent bool) string {
	if includesPermanent {
		return "Enter: start cleanup | b/Esc: back to preview"
	}
	return "Enter: execute | b/Esc: back to preview"
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
	observations := make([]clean.CategoryPreviewObservation, 0, len(rows))
	for _, row := range rows {
		// Servicing rows are not file-scanned and never contribute to no-work
		// classification; they carry no CategoryPreviewState.
		if row.Servicing {
			continue
		}
		observations = append(observations, clean.CategoryPreviewObservation{
			Identifier:     row.Identifier,
			State:          row.State,
			CandidateCount: row.CandidateCount,
			Bytes:          row.Bytes,
		})
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
	// Shared with tui_style permanentSelectionNotice for risk-channel styling.
	return permanentSelectionNotice
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

// eagerServicingRowMarker maps a servicing row state to a single-glyph marker.
// spinnerFrame animates only the analyzing state.
func eagerServicingRowMarker(state clean.ServicingRowState, spinnerFrame int) string {
	switch state {
	case clean.ServicingRowAnalysisRequired:
		return "?"
	case clean.ServicingRowAnalyzing:
		return eagerPreviewSpinnerFrames[spinnerFrame%len(eagerPreviewSpinnerFrames)]
	case clean.ServicingRowReady:
		return "✓"
	case clean.ServicingRowNoWork:
		return "–"
	case clean.ServicingRowSkipped:
		return "⊘"
	case clean.ServicingRowFailed:
		return "!"
	default:
		return "?"
	}
}

// eagerServicingRowLabel formats one Windows servicing preview row without paths
// or bytes. Ready discloses the reclaimable-package count; other states disclose
// their lifecycle only.
func eagerServicingRowLabel(row eagerCategoryRow) string {
	switch row.ServicingState {
	case clean.ServicingRowAnalysisRequired:
		return row.Label + " · analysis required (press a)"
	case clean.ServicingRowAnalyzing:
		return row.Label + " · analyzing…"
	case clean.ServicingRowReady:
		return fmt.Sprintf("%s · ready · %d reclaimable package(s) · size unknown", row.Label, row.ServicingReclaimablePackages)
	case clean.ServicingRowNoWork:
		return row.Label + " · no cleanup needed"
	case clean.ServicingRowSkipped:
		return row.Label + " · skipped"
	case clean.ServicingRowFailed:
		return row.Label + " · failed"
	default:
		return row.Label
	}
}

// eagerServicingFocusedDetailBody is the path-free focused diagnostic for a
// servicing row. Skipped and failed states surface their stable reason text.
func eagerServicingFocusedDetailBody(row eagerCategoryRow) string {
	switch row.ServicingState {
	case clean.ServicingRowAnalysisRequired:
		return "Analysis required · press a to analyze the component store (requests administrator consent)."
	case clean.ServicingRowAnalyzing:
		return "Analyzing… requesting administrator consent for read-only component-store analysis."
	case clean.ServicingRowReady:
		return fmt.Sprintf("Ready · %d reclaimable package(s) · cleanup recommended · press space to select.", row.ServicingReclaimablePackages)
	case clean.ServicingRowNoWork:
		return "No cleanup needed · component store reports nothing to reclaim."
	case clean.ServicingRowSkipped:
		body := "Skipped"
		if row.ServicingReasonCode != "" {
			body += " · " + servicingReasonText(row.ServicingReasonCode)
		}
		return body + " · press a to analyze again."
	case clean.ServicingRowFailed:
		body := "Failed"
		if row.ServicingReasonCode != "" {
			body += " · " + servicingReasonText(row.ServicingReasonCode)
		}
		return body + " · press a to analyze again."
	default:
		return "Unknown servicing state"
	}
}

// eagerServicingExecutionRowLabel formats a servicing execution/result outcome
// without a byte token. Servicing processes no file bytes; its lifecycle is
// reported directly.
func eagerServicingExecutionRowLabel(outcome clean.CategoryExecutionOutcome) string {
	switch outcome.State {
	case clean.CategoryExecutionWaiting:
		return outcome.Label + " · waiting"
	case clean.CategoryExecutionRechecking, clean.CategoryExecutionReady, clean.CategoryExecutionCleaning:
		return outcome.Label + " · servicing…"
	case clean.CategoryExecutionCleaned:
		return outcome.Label + " · servicing completed"
	case clean.CategoryExecutionEmpty:
		return outcome.Label + " · no cleanup needed"
	case clean.CategoryExecutionSkipped:
		return outcome.Label + " · skipped"
	case clean.CategoryExecutionFailed:
		line := outcome.Label + " · failed"
		if outcome.ServicingReason != "" {
			line += " · " + servicingReasonText(outcome.ServicingReason)
		}
		if hint := clean.ServicingCleanupExitHint(outcome.ServicingExitCode); hint != "" {
			line += " · " + hint
		}
		return line
	case clean.CategoryExecutionCanceled:
		return outcome.Label + " · canceled"
	default:
		return outcome.Label
	}
}

// servicingReasonText maps a stable servicing reason code to short path-free
// presentation text. Unknown codes fall back to a generic message so raw or
// invented text never reaches the user.
func servicingReasonText(code string) string {
	switch code {
	case clean.ServicingReasonNotAuthorized:
		return "servicing not authorized"
	case clean.ServicingReasonElevationDenied:
		return "administrator consent was declined"
	case clean.ServicingReasonElevationFailed:
		return "the elevated helper could not be started"
	case clean.ServicingReasonToolUnavailable:
		return "the Windows servicing tool was unavailable"
	case clean.ServicingReasonHelperFailed:
		return "the servicing helper failed"
	case clean.ServicingReasonAnalysisFailed:
		return "component-store analysis failed"
	case clean.ServicingReasonAnalysisOutputInvalid:
		return "analysis output could not be interpreted"
	case clean.ServicingReasonCleanupFailed:
		return "component cleanup failed"
	case clean.ServicingReasonContextCanceled:
		return "the analysis was canceled"
	case clean.ServicingReasonUnsupportedPlatform:
		return "Windows servicing is unavailable on this platform"
	default:
		return "analysis did not complete"
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
// spinnerFrame is only used for active in-progress states (not waiting).
func eagerExecutionRowMarker(state clean.CategoryExecutionState, spinnerFrame int) string {
	switch state {
	case clean.CategoryExecutionWaiting:
		return "…"
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
	case clean.CategoryExecutionWaiting:
		return outcome.Label + " · waiting"
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
	case clean.PreviewReasonNVIDIAActivity:
		// Busy or undetermined NVIDIA process/service state; the whole
		// nvidia_installer_cache category is skipped and cannot be selected.
		return "NVIDIA activity detected or state unknown"
	case "recycle_bin_capacity":
		return "did not fit remaining Recycle Bin capacity"
	case "recycle_bin_disabled":
		return "Recycle Bin disabled for this volume"
	case "recycle_bin_capacity_probe_failed":
		return "Recycle Bin capacity state unknown"
	case "permanent_delete_partial":
		return "some locked files skipped; rest deleted"
	case "permanent_delete_failed":
		return "permanent deletion failed"
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
// terminal/selection state. When the focused row is an analyzable Windows
// servicing row, the `a` hint reads "analyze" (the context-sensitive analysis
// action) rather than "select all". Never authorizes cleanup.
func eagerFooterHints(allTerminal bool, noWork clean.EagerPreviewNoWorkState, confirmationEnabled, focusedServicingAnalyzable bool) string {
	aHint := "a select all"
	if focusedServicingAnalyzable {
		aHint = "a analyze"
	}
	base := "Hints: up/down browse · space toggle · " + aHint + " · x clear · b/Esc back · q quit"
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
			return "Hints: enter confirm · up/down browse · space toggle · " + aHint + " · x clear · b/Esc back · q quit"
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

// eagerResultServicingNotes returns path-free, byte-free result disclosures for
// Windows servicing operations in the authoritative Result: a restart
// requirement (DISM 3010/3017 semantics, preserved for the user's own decision)
// and a post-start cancellation notice (cancellation requested after servicing
// began never replaces the actual outcome). Order is stable; empty when no
// servicing operation carries either flag.
func eagerResultServicingNotes(result clean.Result) []string {
	restart := false
	postStartCancel := false
	for _, op := range result.ServicingOperations {
		if op.RestartRequired {
			restart = true
		}
		if op.CancelRequested {
			postStartCancel = true
		}
	}
	var notes []string
	if restart {
		notes = append(notes, "A restart is required to finish Windows servicing; Foal never reboots for you.")
	}
	if postStartCancel {
		notes = append(notes, "Cancellation was requested after servicing started; Windows finished the transaction and the actual outcome stands.")
	}
	return notes
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
