package uninstall

import (
	"fmt"
	"strings"
)

// RenderExecuteReport formats a human execute summary without claiming
// free-space recovery, secure erasure, or permanent deletion for leftovers.
// It mirrors the Purge execute report style so users see one consistent
// shape across mutating commands. Leftover path outcomes are rendered per
// app when present. Elevation outcome (Uninstall-only, ADR 0028) is rendered
// so batch and elevation results are clear.
func RenderExecuteReport(result ExecuteResult) string {
	var b strings.Builder
	b.WriteString("Foal uninstall\n")
	if result.Status == StatusExecuteError {
		b.WriteString("Status: error\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
		return b.String()
	}
	if result.Status == StatusExecuteCanceled {
		b.WriteString("Status: canceled\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("Execution complete.\n")
	}
	b.WriteString(fmt.Sprintf("Selected: %d, uninstalled: %d, skipped: %d, failed: %d, canceled: %d.\n",
		result.Totals.SelectedCount,
		result.Totals.UninstalledCount,
		result.Totals.SkippedCount,
		result.Totals.FailedCount,
		result.Totals.CanceledCount,
	))
	renderElevationOutcome(&b, result.Elevation)
	if len(result.Applications) == 0 {
		b.WriteString("No applications were selected for uninstall.\n")
	}
	for _, app := range result.Applications {
		b.WriteString("\n  - ")
		b.WriteString(valueOrUnknown(app.Name))
		b.WriteString("\n")
		writeField(&b, "planned class", plannedClassLabel(app.PlannedClass))
		if app.RequiresAdmin {
			writeField(&b, "requires admin", "true (machine-wide install)")
		}
		writeField(&b, "action", app.Action)
		writeField(&b, "result", app.Result)
		writeField(&b, "command mode", app.CommandMode)
		writeField(&b, "attempted command", app.AttemptedCommand)
		writeField(&b, "skipped reason", app.SkippedReason)
		writeField(&b, "detail", app.Detail)
		renderLeftoverOutcomes(&b, app)
	}
	b.WriteString("\nLeftover deletion uses the Recycle Bin and runs only after the uninstaller reports success. A failed or canceled uninstaller does not delete leftovers.\n")
	return b.String()
}

// renderElevationOutcome writes the Uninstall-only elevation decision so the
// batch result makes clear whether UAC was requested and whether admin-
// required apps were allowed to proceed (ADR 0028). Clean/Purge reports never
// carry elevation state.
func renderElevationOutcome(b *strings.Builder, elevation ElevationOutcome) {
	if !elevation.Requested {
		return
	}
	b.WriteString("Elevation: ")
	if elevation.Granted {
		b.WriteString("granted")
	} else {
		b.WriteString("not granted (admin-required apps skipped)")
	}
	if elevation.Reason != "" {
		b.WriteString(" - ")
		b.WriteString(elevation.Reason)
	}
	b.WriteString("\n")
}

// renderLeftoverOutcomes writes the per-path leftover outcome list for one
// app. Paths that were deleted via the Recycle Bin and paths that were
// skipped (protected, missing, reparse, etc) are both surfaced so the user
// can audit what was and was not touched. Empty when the uninstaller did
// not report success or the confirmed set was empty.
func renderLeftoverOutcomes(b *strings.Builder, app AppOutcome) {
	if len(app.LeftoverOutcomes) == 0 {
		return
	}
	b.WriteString("    leftover paths:\n")
	for _, outcome := range app.LeftoverOutcomes {
		b.WriteString("      - ")
		b.WriteString(outcome.Path)
		b.WriteString("\n")
		writeField(b, "      action", outcome.Action)
		writeField(b, "      result", outcome.Result)
		writeField(b, "      reason", outcome.Reason)
		writeField(b, "      detail", outcome.Detail)
	}
}
