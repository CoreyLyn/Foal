package clean

// Windows servicing contract (ADR 0029). This file defines the action-neutral,
// path-free, byte-free servicing operation record shared by Result and History.
// It carries no filesystem path, raw DISM output, package identifier, or guessed
// byte estimate. Actual Windows servicing invocation is intentionally out of
// scope for this contract: no production category registers
// invoke_windows_servicing yet, and this package never launches DISM here.

// ServicingCapability is the fixed built-in helper capability a servicing
// operation used. There is no standalone start-cleanup capability: the composite
// execute capability performs a fresh analysis and guard before mutating.
type ServicingCapability string

const (
	ServicingCapabilityAnalyzeComponentStore        ServicingCapability = "analyze_component_store"
	ServicingCapabilityExecuteComponentStoreCleanup ServicingCapability = "execute_component_store_cleanup"
)

// ServicingOutcome is the stable outcome of a Windows servicing operation.
// canceled applies only before mutation begins; after cleanup starts the record
// keeps the actual completed/failed outcome and sets CancelRequested instead.
type ServicingOutcome string

const (
	ServicingOutcomeReady     ServicingOutcome = "ready"
	ServicingOutcomeNoWork    ServicingOutcome = "no_work"
	ServicingOutcomeCompleted ServicingOutcome = "completed"
	ServicingOutcomeSkipped   ServicingOutcome = "skipped"
	ServicingOutcomeFailed    ServicingOutcome = "failed"
	ServicingOutcomeCanceled  ServicingOutcome = "canceled"
)

// Stable servicing reasons. They are Foal-owned messages, never raw OS/DISM
// text. context_canceled and unsupported_platform are shared with other Clean
// surfaces.
const (
	ServicingReasonNotAuthorized         = "windows_servicing_not_authorized"
	ServicingReasonElevationDenied       = "windows_servicing_elevation_denied"
	ServicingReasonElevationFailed       = "windows_servicing_elevation_failed"
	ServicingReasonToolUnavailable       = "windows_servicing_tool_unavailable"
	ServicingReasonHelperFailed          = "windows_servicing_helper_failed"
	ServicingReasonAnalysisFailed        = "windows_servicing_analysis_failed"
	ServicingReasonAnalysisOutputInvalid = "windows_servicing_analysis_output_invalid"
	ServicingReasonCleanupFailed         = "windows_servicing_cleanup_failed"
	ServicingReasonContextCanceled       = "context_canceled"
	ServicingReasonUnsupportedPlatform   = "unsupported_platform"
)

// ServicingOperation is the path-free, byte-free record of one Windows servicing
// operation. It never enters file candidates, deleted/failed/skipped file items,
// detailed path lists, or path History, and never contributes candidate,
// affected, Recycle Bin, or Permanent deletion bytes. It carries only parsed
// analysis evidence (a reclaimable package count and cleanup recommendation),
// the outcome, cancellation-request state, an optional stable reason, an
// optional DISM exit code (present only when DISM actually ran), and the
// restart-required state derived from DISM exit semantics.
type ServicingOperation struct {
	Category            string              `json:"category"`
	PlannedAction       PlannedAction       `json:"planned_action"`
	Capability          ServicingCapability `json:"capability"`
	ReclaimablePackages int                 `json:"reclaimable_packages"`
	CleanupRecommended  bool                `json:"cleanup_recommended"`
	Outcome             ServicingOutcome    `json:"outcome"`
	CancelRequested     bool                `json:"cancel_requested"`
	Reason              string              `json:"reason,omitempty"`
	// ExitCode is present only when DISM actually ran to exit. Authorization
	// skips and pre-launch failures leave it nil.
	ExitCode        *int `json:"exit_code,omitempty"`
	RestartRequired bool `json:"restart_required"`
}

// ValidServicingOutcome reports whether outcome is a stable servicing outcome.
func ValidServicingOutcome(outcome ServicingOutcome) bool {
	switch outcome {
	case ServicingOutcomeReady, ServicingOutcomeNoWork, ServicingOutcomeCompleted,
		ServicingOutcomeSkipped, ServicingOutcomeFailed, ServicingOutcomeCanceled:
		return true
	default:
		return false
	}
}

// ValidServicingCapability reports whether capability is a fixed built-in
// servicing capability.
func ValidServicingCapability(capability ServicingCapability) bool {
	switch capability {
	case ServicingCapabilityAnalyzeComponentStore, ServicingCapabilityExecuteComponentStoreCleanup:
		return true
	default:
		return false
	}
}
