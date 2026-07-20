package uninstall

import (
	"fmt"
	"strings"
)

// RenderExecuteReport formats a human execute summary without claiming
// free-space recovery, secure erasure, or leftover deletion. It mirrors the
// Purge execute report style so users see one consistent shape across
// mutating commands.
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
	if len(result.Applications) == 0 {
		b.WriteString("No applications were selected for uninstall.\n")
	}
	for _, app := range result.Applications {
		b.WriteString("\n  - ")
		b.WriteString(valueOrUnknown(app.Name))
		b.WriteString("\n")
		writeField(&b, "planned class", plannedClassLabel(app.PlannedClass))
		writeField(&b, "action", app.Action)
		writeField(&b, "result", app.Result)
		writeField(&b, "command mode", app.CommandMode)
		writeField(&b, "attempted command", app.AttemptedCommand)
		writeField(&b, "skipped reason", app.SkippedReason)
		writeField(&b, "detail", app.Detail)
	}
	b.WriteString("\nLeftover deletion is not performed in this slice. A failed or canceled uninstaller does not delete leftovers.\n")
	return b.String()
}
