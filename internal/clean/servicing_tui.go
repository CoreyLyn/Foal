package clean

import "context"

// ServicingRowState is the Clean TUI row lifecycle for an exact-selection-only
// Windows servicing category (ADR 0029). It is deliberately distinct from
// CategoryPreviewState: a servicing category is never file-scanned by the eager
// preview, so the row rests at analysis_required until the user requests an
// explicit analysis. Only a ready row (a fresh successful analysis that reports
// work) may be selected; every other state fails closed and non-selectable.
type ServicingRowState string

const (
	// ServicingRowAnalysisRequired is the initial resting state. The row is
	// unselected and cannot be selected until an explicit analysis reports work.
	ServicingRowAnalysisRequired ServicingRowState = "analysis_required"
	// ServicingRowAnalyzing means an explicit analysis is in flight (the only
	// non-terminal servicing state; it blocks confirmation like a scanning row).
	ServicingRowAnalyzing ServicingRowState = "analyzing"
	// ServicingRowReady means a fresh analysis reported a positive reclaimable
	// package count with a cleanup recommendation. Only this state is selectable.
	ServicingRowReady ServicingRowState = "ready"
	// ServicingRowNoWork means analysis proved there is nothing to clean.
	ServicingRowNoWork ServicingRowState = "no_work"
	// ServicingRowSkipped means analysis was denied, unavailable, or canceled.
	// It is non-selectable and retryable through another explicit analysis.
	ServicingRowSkipped ServicingRowState = "skipped"
	// ServicingRowFailed means analysis failed or returned uninterpretable
	// output. It is non-selectable and retryable through another analysis.
	ServicingRowFailed ServicingRowState = "failed"
)

// IsServicingSummary reports whether a catalog summary is a Windows servicing
// category (planned action invoke_windows_servicing). The Clean TUI presents
// such categories with the servicing row state machine and never file-scans
// them in the eager preview.
func IsServicingSummary(summary CleanupCategorySummary) bool {
	return summary.PlannedAction == PlannedActionInvokeWindowsServicing
}

// IsServicingCategory reports whether a canonical category delegates to Windows
// servicing instead of file deletion. Unknown identifiers report false.
func IsServicingCategory(identifier string) bool {
	return isServicingCategory(identifier)
}

// ServicingRowSelectable reports whether a servicing row may be added to the
// cleanup selection. Only a ready row is selectable; every other state
// (including the initial analysis_required and the terminal no_work) fails
// closed so unknown or empty servicing state can never be executed.
func ServicingRowSelectable(state ServicingRowState) bool {
	return state == ServicingRowReady
}

// ServicingRowAnalyzable reports whether an explicit analysis action may run
// from this state. Analysis may (re)start from any resting state — including a
// retry from failed, skipped, no_work, or a stale ready result — but never
// while an analysis is already in flight.
func ServicingRowAnalyzable(state ServicingRowState) bool {
	return state != ServicingRowAnalyzing
}

// ServicingRowTerminalForConfirmation reports whether a servicing row is at
// rest for confirmation gating. Only analyzing is non-terminal (like a scanning
// file category); every resting servicing state — including the initial
// analysis_required — must not block confirmation of other categories.
func ServicingRowTerminalForConfirmation(state ServicingRowState) bool {
	return state != ServicingRowAnalyzing
}

// ServicingRowStateFromAnalysis maps a shared servicing analysis operation onto
// a TUI row state. A ready outcome becomes ready (selectable); no_work becomes
// no_work; a canceled, denied, or otherwise skipped analysis becomes skipped;
// anything else — including an unexpected mutation outcome from a read-only
// analysis — fails closed as failed.
func ServicingRowStateFromAnalysis(op ServicingOperation) ServicingRowState {
	switch op.Outcome {
	case ServicingOutcomeReady:
		return ServicingRowReady
	case ServicingOutcomeNoWork:
		return ServicingRowNoWork
	case ServicingOutcomeSkipped, ServicingOutcomeCanceled:
		return ServicingRowSkipped
	default:
		return ServicingRowFailed
	}
}

// ServicingExecutionState maps a servicing operation outcome onto the shared
// category execution lifecycle for the Clean TUI execution/result surface.
// Servicing contributes no bytes; only its lifecycle is projected. A pre-mutation
// canceled outcome projects as canceled; an unexpected ready or unknown value
// fails closed as failed.
func ServicingExecutionState(outcome ServicingOutcome) CategoryExecutionState {
	switch outcome {
	case ServicingOutcomeCompleted:
		return CategoryExecutionCleaned
	case ServicingOutcomeNoWork:
		return CategoryExecutionEmpty
	case ServicingOutcomeSkipped:
		return CategoryExecutionSkipped
	case ServicingOutcomeCanceled:
		return CategoryExecutionCanceled
	default:
		return CategoryExecutionFailed
	}
}

// AnalyzeServicingCategory runs one read-only component-store analysis for a
// servicing category through the injected gateway and returns a path-free
// ServicingOperation. It is the shared entry point the Clean TUI's explicit
// analysis action uses so the TUI owns no DISM invocation, elevation, helper
// protocol, or output parsing. An absent gateway or unsupported platform fails
// closed without opening UAC.
func AnalyzeServicingCategory(ctx context.Context, opts Options, category string) ServicingOperation {
	return analyzeServicingCategory(ctx, opts, category)
}
