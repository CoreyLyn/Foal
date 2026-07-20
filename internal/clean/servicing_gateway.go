package clean

import "context"

// ServicingAnalysisRequest is the path-free, argument-free request the shared
// Clean boundary hands to a ServicingGateway. It names only the canonical
// category and the fixed built-in capability. It never carries an executable,
// command line, DISM argument, path, or byte estimate: the elevated helper
// derives the fixed DISM invocation from the capability alone.
type ServicingAnalysisRequest struct {
	Category   string
	Capability ServicingCapability
}

// ServicingAnalysisResult is a gateway's structured outcome of one
// analyze_component_store invocation. It carries only a Foal-owned outcome and
// stable reason plus the two strictly parsed English analysis fields. It never
// exposes raw DISM output, an OS error string, a package identifier, a path, or
// a reclaimable-byte estimate.
type ServicingAnalysisResult struct {
	// Outcome is a stable read-only analysis outcome: ready, no_work, skipped,
	// failed, or canceled. Analysis never reports completed (that is a mutation
	// outcome).
	Outcome ServicingOutcome
	// Reason is a stable Foal-owned reason. It is empty for ready and no_work and
	// required for skipped, failed, and canceled.
	Reason string
	// ReclaimablePackages and CleanupRecommended are the strictly parsed English
	// analysis fields. They are meaningful only when DISM ran and parsed cleanly
	// (ready or no_work); they stay zero-valued for skips and failures.
	ReclaimablePackages int
	CleanupRecommended  bool
	// ExitCode is present only when DISM actually ran to exit. Authorization
	// skips, elevation failures, and helper failures before launch leave it nil.
	ExitCode *int
}

// ServicingGateway is the high-level seam for Windows component-store servicing.
// Shared Clean depends only on this interface: production wires an isolated
// elevated helper coordinator (nonce-bound named pipe + fixed DISM capability),
// while tests inject canned outcomes so ordinary core tests never launch UAC or
// DISM. AnalyzeComponentStore performs one read-only component-store analysis
// and returns a structured, path-free result. It must never mutate the
// filesystem or component store, expose raw tool output, or run an arbitrary
// command.
type ServicingGateway interface {
	AnalyzeComponentStore(ctx context.Context, req ServicingAnalysisRequest) ServicingAnalysisResult
}

// appendServicingAnalysis runs read-only component-store analysis for each
// opted-in servicing category and appends a path-free ServicingOperation to the
// result. It is invoked only from the dry-run preview: an exact CLI opt-in may
// request analysis (which may prompt UAC through the gateway). Default Dry-run,
// `all`, group tokens, and TUI entry never reach this function because servicing
// categories are exact-selection-only and absent from their plans.
func appendServicingAnalysis(ctx context.Context, opts Options, servicingCategories []string, result *Result) {
	if result == nil {
		return
	}
	for _, category := range servicingCategories {
		result.ServicingOperations = append(result.ServicingOperations, analyzeServicingCategory(ctx, opts, category))
	}
}

func analyzeServicingCategory(ctx context.Context, opts Options, category string) ServicingOperation {
	op := ServicingOperation{
		Category:      category,
		PlannedAction: PlannedActionInvokeWindowsServicing,
		Capability:    ServicingCapabilityAnalyzeComponentStore,
	}
	// Pre-mutation cancellation: never begin analysis after cancellation.
	if ctx != nil && ctx.Err() != nil {
		op.Outcome = ServicingOutcomeCanceled
		op.Reason = ServicingReasonContextCanceled
		return op
	}
	gateway := opts.ServicingGateway
	if gateway == nil {
		// No servicing gateway wired (unsupported platform or absent coordinator):
		// fail closed with a stable skip; never guess reclaimable work or open UAC.
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = ServicingReasonUnsupportedPlatform
		return op
	}
	res := gateway.AnalyzeComponentStore(ctx, ServicingAnalysisRequest{
		Category:   category,
		Capability: ServicingCapabilityAnalyzeComponentStore,
	})
	return applyServicingAnalysisResult(op, res)
}

// applyServicingAnalysisResult maps a gateway result onto the operation record
// with a fail-closed consistency guard. A ready outcome is honored only when the
// parsed evidence actually supports it (positive reclaimable packages and a
// positive cleanup recommendation); anything ambiguous, unknown, or a mutation
// outcome from a read-only analysis is downgraded to a stable failure so
// localization or format drift can never be read as approval.
func applyServicingAnalysisResult(op ServicingOperation, res ServicingAnalysisResult) ServicingOperation {
	op.ExitCode = res.ExitCode
	switch res.Outcome {
	case ServicingOutcomeReady:
		if res.ReclaimablePackages <= 0 || !res.CleanupRecommended {
			op.Outcome = ServicingOutcomeFailed
			op.Reason = ServicingReasonAnalysisOutputInvalid
			return op
		}
		op.Outcome = ServicingOutcomeReady
		op.ReclaimablePackages = res.ReclaimablePackages
		op.CleanupRecommended = res.CleanupRecommended
	case ServicingOutcomeNoWork:
		op.Outcome = ServicingOutcomeNoWork
		op.ReclaimablePackages = res.ReclaimablePackages
		op.CleanupRecommended = res.CleanupRecommended
	case ServicingOutcomeSkipped, ServicingOutcomeFailed, ServicingOutcomeCanceled:
		op.Outcome = res.Outcome
		op.Reason = normalizeServicingReason(res.Outcome, res.Reason)
	default:
		// Unknown or a completed (mutation) outcome from analysis: fail closed.
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
	}
	return op
}

// appendServicingExecuteSkips records each opted-in servicing category at
// execute time as a skip with windows_servicing_not_authorized and no exit
// code. Component-store mutation requires dedicated per-run authorization that
// this slice does not provide, so execute never opens UAC or analyzes WinSxS.
// The record keeps the composite execute capability so History and Result
// distinguish a would-be cleanup from a dry-run analysis.
func appendServicingExecuteSkips(servicingCategories []string, result *Result) {
	if result == nil {
		return
	}
	for _, category := range servicingCategories {
		result.ServicingOperations = append(result.ServicingOperations, ServicingOperation{
			Category:      category,
			PlannedAction: PlannedActionInvokeWindowsServicing,
			Capability:    ServicingCapabilityExecuteComponentStoreCleanup,
			Outcome:       ServicingOutcomeSkipped,
			Reason:        ServicingReasonNotAuthorized,
		})
	}
}

// normalizeServicingReason keeps only known stable reasons on a skip, failure,
// or cancellation. An empty or unrecognized reason from the gateway is replaced
// with a safe default so external surfaces never carry raw or invented text.
func normalizeServicingReason(outcome ServicingOutcome, reason string) string {
	if isKnownServicingReason(reason) {
		return reason
	}
	switch outcome {
	case ServicingOutcomeCanceled:
		return ServicingReasonContextCanceled
	case ServicingOutcomeSkipped:
		return ServicingReasonUnsupportedPlatform
	default:
		return ServicingReasonHelperFailed
	}
}

func isKnownServicingReason(reason string) bool {
	switch reason {
	case ServicingReasonNotAuthorized, ServicingReasonElevationDenied, ServicingReasonElevationFailed,
		ServicingReasonToolUnavailable, ServicingReasonHelperFailed, ServicingReasonAnalysisFailed,
		ServicingReasonAnalysisOutputInvalid, ServicingReasonCleanupFailed, ServicingReasonContextCanceled,
		ServicingReasonUnsupportedPlatform:
		return true
	default:
		return false
	}
}
