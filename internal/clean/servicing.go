package clean

// Windows servicing contract (ADR 0029, ADR 0036). This file defines the
// action-neutral, path-free servicing operation record shared by Result and
// History. It never carries a filesystem path, raw DISM output, or a guessed
// reclaimable-byte estimate. Component-store records carry parsed analysis
// evidence only; superseded display driver records additionally carry package
// identifiers, DriverVer metadata, and measured package sizes. Byte values are
// servicing evidence and observations only and never enter any deletion byte
// total.

// ServicingCapability is the fixed built-in helper capability a servicing
// operation used. There is no standalone start-cleanup capability: the composite
// execute capability performs a fresh analysis and guard before mutating.
type ServicingCapability string

const (
	ServicingCapabilityAnalyzeComponentStore        ServicingCapability = "analyze_component_store"
	ServicingCapabilityExecuteComponentStoreCleanup ServicingCapability = "execute_component_store_cleanup"
	// ServicingCapabilityAnalyzeDriverStore is the non-elevated, in-process
	// inventory of superseded display driver packages (ADR 0036).
	ServicingCapabilityAnalyzeDriverStore ServicingCapability = "analyze_driver_store"
	// ServicingCapabilityExecuteDriverPackageCleanup removes a bounded set of
	// confirmed superseded display driver packages through the elevated helper,
	// which re-derives eligibility itself before each removal (ADR 0036).
	ServicingCapabilityExecuteDriverPackageCleanup ServicingCapability = "execute_driver_package_cleanup"
)

// Per-package outcomes for superseded display driver packages. candidate after
// execution means the package was not attempted. unknown means the removal
// request reached the helper but no trustworthy outcome came back, so the
// package may or may not have been removed.
const (
	DriverPackageOutcomeCandidate   = "candidate"
	DriverPackageOutcomeRemoved     = "removed"
	DriverPackageOutcomeInUse       = "in_use"
	DriverPackageOutcomeNotEligible = "not_eligible"
	DriverPackageOutcomeFailed      = "failed"
	DriverPackageOutcomeUnknown     = "unknown"
)

// ServicingDriverPackage is one superseded display driver package in a
// servicing record. It carries package identifiers and DriverVer metadata only,
// never a driver-store path. Bytes is the measured logical size of the package
// directory: servicing evidence that never enters deletion byte totals.
type ServicingDriverPackage struct {
	PublishedName string `json:"published_name"`
	OriginalName  string `json:"original_name"`
	Provider      string `json:"provider"`
	DriverDate    string `json:"driver_date"`
	DriverVersion string `json:"driver_version"`
	Bytes         int64  `json:"bytes"`
	Outcome       string `json:"outcome"`
}

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

// ServicingOperation is the path-free record of one Windows servicing
// operation. It never enters file candidates, deleted/failed/skipped file items,
// detailed path lists, or path History, and never contributes candidate,
// affected, Recycle Bin, or Permanent deletion bytes. It carries parsed analysis
// evidence (a reclaimable package count and cleanup recommendation), the
// outcome, cancellation-request state, an optional stable reason, an optional
// DISM exit code (present only when DISM actually ran), the restart-required
// state derived from DISM exit semantics, an optional post-mutation free-space
// observation, and — for superseded display drivers — the packages involved.
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
	// ObservedFreeBytes is the approximate non-negative free-space increase on the
	// Windows volume, measured around a completed exit-0 StartComponentCleanup, or
	// around superseded display driver removals when at least one package was
	// removed (even if another failed). It is nil when not measured — no
	// completed mutation, a restart-required (3010) success whose reclaim happens
	// after reboot, or a negative delta. A measured zero is a legitimate value
	// distinct from nil. It is an external observation, never a reclaimable
	// estimate, and never enters affected, Recycle Bin, or Permanent deletion
	// byte totals.
	ObservedFreeBytes *int64 `json:"observed_free_bytes,omitempty"`
	// DriverPackages lists superseded display driver packages for the
	// superseded-display-drivers category: candidates after analysis, per-package
	// outcomes after execution. Omitted for component-store servicing.
	DriverPackages []ServicingDriverPackage `json:"driver_packages,omitempty"`
	// PackageBytes is servicing evidence that never enters deletion byte totals:
	// after analyze_driver_store, the measured size of the candidate packages;
	// after execute_driver_package_cleanup, the measured size of the packages
	// Windows removed.
	PackageBytes *int64 `json:"package_bytes,omitempty"`
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
	case ServicingCapabilityAnalyzeComponentStore, ServicingCapabilityExecuteComponentStoreCleanup,
		ServicingCapabilityAnalyzeDriverStore, ServicingCapabilityExecuteDriverPackageCleanup:
		return true
	default:
		return false
	}
}
