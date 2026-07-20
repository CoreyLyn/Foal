package clean

import "fmt"

// servicingReportLabel is the path-free human label for the Windows
// component-store servicing category. It never names a WinSxS path.
const servicingReportLabel = "Windows component store"

// ServicingOperationLines renders path-free, byte-free human lines for Windows
// servicing operations. It emits only the outcome, the parsed reclaimable
// package count, the cleanup recommendation, the restart-required flag, and the
// cancellation state — never a WinSxS path, byte estimate, raw DISM output, or
// package identifier. Returns nil when there are no operations.
func ServicingOperationLines(operations []ServicingOperation) []string {
	if len(operations) == 0 {
		return nil
	}
	lines := make([]string, 0, len(operations))
	for _, op := range operations {
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
