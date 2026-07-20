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

// ServicingExecuteRequest is the path-free, argument-free request the shared
// Clean boundary hands to a ServicingGateway for confirmed component-store
// cleanup. Like the analysis request it names only the canonical category and
// the fixed composite capability; the elevated helper derives the fixed DISM
// invocations (fresh analysis then StartComponentCleanup) from the capability
// alone.
type ServicingExecuteRequest struct {
	Category   string
	Capability ServicingCapability
}

// ServicingExecuteResult is a gateway's structured outcome of one composite
// execute_component_store_cleanup invocation. The gateway performs a fresh
// analysis, enforces the guard, and starts cleanup only when a positive
// reclaimable-package count is explicitly recommended; the result reports what
// actually happened. It is path-free and byte-free.
type ServicingExecuteResult struct {
	// Outcome is a stable mutation outcome: completed, no_work, skipped, failed,
	// or pre-mutation canceled. A ready outcome is never valid here.
	Outcome ServicingOutcome
	// Reason is a stable Foal-owned reason, required for skipped, failed, and
	// pre-mutation canceled; empty for completed and no_work.
	Reason string
	// ReclaimablePackages and CleanupRecommended are the fresh-analysis fields.
	// Meaningful for completed and no_work; zero-valued for skips and failures.
	ReclaimablePackages int
	CleanupRecommended  bool
	// ExitCode is present only when DISM actually ran to exit (analysis or
	// cleanup). Authorization skips and pre-launch failures leave it nil.
	ExitCode *int
	// RestartRequired is derived from DISM cleanup exit semantics (3010 or 3017).
	RestartRequired bool
	// CancelRequested records that cancellation arrived after cleanup started.
	// It never replaces the actual completed or failed outcome; a pre-mutation
	// cancellation instead uses Outcome canceled with CancelRequested false.
	CancelRequested bool
}

// ServicingGateway is the high-level seam for Windows component-store servicing.
// Shared Clean depends only on this interface: production wires an isolated
// elevated helper coordinator (nonce-bound named pipe + fixed DISM capability),
// while tests inject canned outcomes so ordinary core tests never launch UAC or
// DISM. AnalyzeComponentStore performs one read-only component-store analysis;
// ExecuteComponentStoreCleanup performs the composite fresh-analysis-then-cleanup
// transaction. Both return structured, path-free results and must never expose
// raw tool output or run an arbitrary command. There is no standalone
// start-cleanup capability, so analysis can never be asserted across processes.
type ServicingGateway interface {
	AnalyzeComponentStore(ctx context.Context, req ServicingAnalysisRequest) ServicingAnalysisResult
	ExecuteComponentStoreCleanup(ctx context.Context, req ServicingExecuteRequest) ServicingExecuteResult
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

// appendServicingExecution runs the composite execute_component_store_cleanup
// capability for each opted-in servicing category and appends a path-free
// ServicingOperation to the result. It is the final mutation phase of Execute
// (ADR 0029): it runs only after Recycle Bin and Permanent deletion work, and
// once a servicing operation starts no later action begins. Missing servicing
// authorization skips with windows_servicing_not_authorized and never opens
// UAC; a run canceled before servicing starts records a pre-mutation cancel.
func appendServicingExecution(ctx context.Context, opts Options, servicingCategories []string, result *Result) {
	if result == nil {
		return
	}
	for _, category := range servicingCategories {
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseServicingOperations, category)
		result.ServicingOperations = append(result.ServicingOperations, executeServicingCategory(ctx, opts, category))
	}
}

func executeServicingCategory(ctx context.Context, opts Options, category string) ServicingOperation {
	op := ServicingOperation{
		Category:      category,
		PlannedAction: PlannedActionInvokeWindowsServicing,
		Capability:    ServicingCapabilityExecuteComponentStoreCleanup,
	}
	// Independent per-run authorization: without --allow-servicing there is no
	// UAC, no analysis, and no gateway call. This is distinct from and never
	// implied by permanent-deletion authorization.
	if !opts.AllowServicing {
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = ServicingReasonNotAuthorized
		return op
	}
	// Pre-mutation cancellation: never begin a Windows servicing action after
	// cancellation. No UAC, no DISM.
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
	res := gateway.ExecuteComponentStoreCleanup(ctx, ServicingExecuteRequest{
		Category:   category,
		Capability: ServicingCapabilityExecuteComponentStoreCleanup,
	})
	return applyServicingExecuteResult(op, res)
}

// applyServicingExecuteResult maps a composite gateway result onto the operation
// record with a fail-closed guard. It copies the DISM exit code, restart-required
// flag, and cancellation-request state, then classifies the outcome. A read-only
// ready outcome or any unknown value from a mutation capability is downgraded to
// a stable helper failure so ambiguous state can never be read as success.
func applyServicingExecuteResult(op ServicingOperation, res ServicingExecuteResult) ServicingOperation {
	op.ExitCode = res.ExitCode
	op.RestartRequired = res.RestartRequired
	op.CancelRequested = res.CancelRequested
	switch res.Outcome {
	case ServicingOutcomeCompleted:
		op.Outcome = ServicingOutcomeCompleted
		op.ReclaimablePackages = res.ReclaimablePackages
		op.CleanupRecommended = res.CleanupRecommended
	case ServicingOutcomeNoWork:
		op.Outcome = ServicingOutcomeNoWork
		op.ReclaimablePackages = res.ReclaimablePackages
		op.CleanupRecommended = res.CleanupRecommended
	case ServicingOutcomeFailed:
		op.Outcome = ServicingOutcomeFailed
		op.ReclaimablePackages = res.ReclaimablePackages
		op.CleanupRecommended = res.CleanupRecommended
		op.Reason = normalizeServicingReason(ServicingOutcomeFailed, res.Reason)
	case ServicingOutcomeSkipped:
		op.Outcome = ServicingOutcomeSkipped
		op.Reason = normalizeServicingReason(ServicingOutcomeSkipped, res.Reason)
	case ServicingOutcomeCanceled:
		// Pre-mutation cancellation only: a post-start cancel keeps the actual
		// completed/failed outcome with CancelRequested set instead.
		op.Outcome = ServicingOutcomeCanceled
		op.Reason = normalizeServicingReason(ServicingOutcomeCanceled, res.Reason)
	default:
		// Unknown, or a read-only ready outcome returned from a mutation
		// capability: fail closed.
		op.Outcome = ServicingOutcomeFailed
		op.Reason = ServicingReasonHelperFailed
	}
	return op
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
