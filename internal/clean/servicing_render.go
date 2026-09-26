package clean

import (
	"fmt"
	"strings"
)

// servicingReportLabel is the path-free human label for the Windows
// component-store servicing category. It never names a WinSxS path.
const servicingReportLabel = "Windows component store"

// driverPackagesReportLabel is the human label for the superseded display
// driver category. It never names a driver-store path.
const driverPackagesReportLabel = "Superseded display drivers"

// ServicingReportLabel returns the path-free human label for a servicing
// operation's category.
func ServicingReportLabel(category string) string {
	if isDriverPackageServicingCategory(category) {
		return driverPackagesReportLabel
	}
	return servicingReportLabel
}

// ServicingOperationLines renders path-free human lines for Windows servicing
// operations with raw byte counts. See ServicingOperationLinesWithBytes.
func ServicingOperationLines(operations []ServicingOperation) []string {
	return ServicingOperationLinesWithBytes(operations, formatBytes)
}

// ServicingOperationLinesWithBytes renders path-free human lines for Windows
// servicing operations. Component-store lines carry only the outcome, parsed
// reclaimable package count, restart-required flag, and cancellation state —
// never a WinSxS path, byte estimate, raw DISM output, or package identifier.
// Superseded display driver lines add each package's published name, original
// INF name, provider, DriverVer, measured size, and outcome. Returns nil when
// there are no operations.
func ServicingOperationLinesWithBytes(operations []ServicingOperation, formatBytes func(int64) string) []string {
	if len(operations) == 0 {
		return nil
	}
	lines := make([]string, 0, len(operations))
	for _, op := range operations {
		if isDriverPackageServicingCategory(op.Category) {
			lines = append(lines, driverPackageOperationLines(op, formatBytes)...)
			continue
		}
		lines = append(lines, servicingOperationLine(op))
	}
	return lines
}

func servicingOperationLine(op ServicingOperation) string {
	switch op.Outcome {
	case ServicingOutcomeCompleted:
		line := fmt.Sprintf("%s: cleanup completed (%d reclaimable packages).", servicingReportLabel, op.ReclaimablePackages)
		if op.RestartRequired {
			line += " A restart is required to finish."
		}
		if op.CancelRequested {
			line += " Cancellation was requested after cleanup started; Windows completed the transaction."
		}
		return line
	case ServicingOutcomeNoWork:
		return fmt.Sprintf("%s: no cleanup needed.", servicingReportLabel)
	case ServicingOutcomeReady:
		return fmt.Sprintf("%s: ready for cleanup (%d reclaimable packages).", servicingReportLabel, op.ReclaimablePackages)
	case ServicingOutcomeSkipped:
		return fmt.Sprintf("%s: skipped (%s).", servicingReportLabel, servicingReasonText(op.Reason))
	case ServicingOutcomeFailed:
		line := fmt.Sprintf("%s: failed (%s).", servicingReportLabel, servicingReasonText(op.Reason))
		if hint := ServicingCleanupExitHint(op.ExitCode); hint != "" {
			line += " " + hint
		}
		if op.RestartRequired {
			line += " A restart is required."
		}
		if op.CancelRequested {
			line += " Cancellation was requested after cleanup started."
		}
		return line
	case ServicingOutcomeCanceled:
		return fmt.Sprintf("%s: canceled before servicing started.", servicingReportLabel)
	default:
		return fmt.Sprintf("%s: %s.", servicingReportLabel, op.Outcome)
	}
}

// DriverPackagesImpactNotice is the path-free impact disclosure for superseded
// display driver removal, shared by CLI and TUI surfaces.
const DriverPackagesImpactNotice = "Removed driver versions can no longer be restored with Device Manager Roll Back Driver and must be downloaded again if needed; removal requests administrator consent (UAC)."

func driverPackageOperationLines(op ServicingOperation, formatBytes func(int64) string) []string {
	label := driverPackagesReportLabel
	var head string
	switch op.Outcome {
	case ServicingOutcomeReady:
		head = fmt.Sprintf("%s: %d package(s) ready for removal (%s). %s", label, len(op.DriverPackages), packageBytesText(op.PackageBytes, formatBytes), DriverPackagesImpactNotice)
	case ServicingOutcomeNoWork:
		if op.Capability == ServicingCapabilityExecuteDriverPackageCleanup && len(op.DriverPackages) > 0 {
			head = fmt.Sprintf("%s: no package was removed.", label)
		} else {
			head = fmt.Sprintf("%s: no superseded packages found.", label)
		}
	case ServicingOutcomeCompleted:
		head = fmt.Sprintf("%s: removed %d of %d package(s) (%s).", label, countDriverOutcome(op.DriverPackages, DriverPackageOutcomeRemoved), len(op.DriverPackages), packageBytesText(op.PackageBytes, formatBytes))
		if op.CancelRequested {
			head += " Cancellation was requested after removal started; Windows completed the requested removals."
		}
	case ServicingOutcomeSkipped:
		head = fmt.Sprintf("%s: skipped (%s).", label, driverPackageReasonText(op.Reason))
	case ServicingOutcomeFailed:
		head = fmt.Sprintf("%s: failed (%s).", label, driverPackageReasonText(op.Reason))
		if removed := countDriverOutcome(op.DriverPackages, DriverPackageOutcomeRemoved); removed > 0 {
			head += fmt.Sprintf(" %d package(s) were removed before the failure.", removed)
		}
		if unknown := countDriverOutcome(op.DriverPackages, DriverPackageOutcomeUnknown); unknown > 0 {
			head += fmt.Sprintf(" The outcome of %d package(s) is unknown; preview again to see what remains.", unknown)
		}
	case ServicingOutcomeCanceled:
		head = fmt.Sprintf("%s: canceled before removal started.", label)
	default:
		head = fmt.Sprintf("%s: %s.", label, op.Outcome)
	}
	lines := []string{head}
	for _, pkg := range op.DriverPackages {
		line := fmt.Sprintf("  - %s · %s · %s · %s (%s) · %s", pkg.PublishedName, pkg.OriginalName, pkg.Provider, pkg.DriverVersion, pkg.DriverDate, formatBytes(pkg.Bytes))
		if pkg.Outcome != DriverPackageOutcomeCandidate || op.Capability == ServicingCapabilityExecuteDriverPackageCleanup {
			line += " · " + driverPackageOutcomeText(pkg.Outcome)
		}
		lines = append(lines, line)
	}
	return lines
}

func packageBytesText(bytes *int64, formatBytes func(int64) string) string {
	if bytes == nil {
		return "size unknown"
	}
	return formatBytes(*bytes)
}

func countDriverOutcome(packages []ServicingDriverPackage, outcome string) int {
	n := 0
	for _, pkg := range packages {
		if pkg.Outcome == outcome {
			n++
		}
	}
	return n
}

func driverPackageOutcomeText(outcome string) string {
	switch outcome {
	case DriverPackageOutcomeRemoved:
		return "removed"
	case DriverPackageOutcomeInUse:
		return "kept (in use by a device)"
	case DriverPackageOutcomeNotEligible:
		return "kept (no longer eligible)"
	case DriverPackageOutcomeFailed:
		return "removal failed"
	case DriverPackageOutcomeUnknown:
		return "outcome unknown"
	case DriverPackageOutcomeCandidate:
		return "not attempted"
	default:
		return strings.ReplaceAll(outcome, "_", " ")
	}
}

// driverPackageReasonText adapts the shared servicing reasons to driver-store
// wording; everything else falls back to the shared text.
func driverPackageReasonText(reason string) string {
	switch reason {
	case ServicingReasonAnalysisFailed:
		return "driver store inventory failed"
	case ServicingReasonAnalysisOutputInvalid:
		return "driver store inventory could not be interpreted"
	case ServicingReasonCleanupFailed:
		return "one or more packages could not be removed"
	case ServicingReasonUnsupportedPlatform:
		return "the Windows driver store is unavailable on this platform"
	default:
		return servicingReasonText(reason)
	}
}

// servicingReasonText maps a stable servicing reason to path-free human text. An
// unknown reason falls back to a generic message so raw or invented text never
// reaches the user.
func servicingReasonText(reason string) string {
	switch reason {
	case ServicingReasonNotAuthorized:
		return "servicing not authorized; rerun with --allow-servicing"
	case ServicingReasonElevationDenied:
		return "administrator consent was declined"
	case ServicingReasonElevationFailed:
		return "the elevated helper could not be started"
	case ServicingReasonToolUnavailable:
		return "the Windows servicing tool was unavailable"
	case ServicingReasonHelperFailed:
		return "the servicing helper failed"
	case ServicingReasonAnalysisFailed:
		return "component-store analysis failed"
	case ServicingReasonAnalysisOutputInvalid:
		return "component-store analysis output could not be interpreted"
	case ServicingReasonCleanupFailed:
		return "component cleanup failed"
	case ServicingReasonContextCanceled:
		return "the run was canceled"
	case ServicingReasonUnsupportedPlatform:
		return "Windows servicing is unavailable on this platform"
	default:
		return "servicing did not complete"
	}
}

// ServicingCleanupExitHint returns optional path-free failure guidance for a
// DISM StartComponentCleanup exit code, aligned with ADR 0029: lock,
// pending-operation, or servicing-conflict failures map to the stable cleanup
// failure reason, after which the user may finish Windows Update or restart and
// preview again. It never copies DISM/CBS log content or paths. Empty when no
// specific hint applies.
func ServicingCleanupExitHint(exitCode *int) string {
	if exitCode == nil {
		return ""
	}
	switch *exitCode {
	case 5:
		// Win32 ERROR_ACCESS_DENIED: in the StartComponentCleanup context this is
		// typically a transient CBS lock, a pending Windows Update operation, or
		// background maintenance holding the component store.
		return "This may be a transient lock or pending Windows Update operation; finish Windows Update or restart Windows and try again."
	default:
		return ""
	}
}
