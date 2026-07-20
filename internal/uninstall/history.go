package uninstall

import (
	"context"
	"fmt"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

// recordHistorySession writes one Uninstall execution session and one
// ItemRecord per selected application. It mirrors Purge/Clean history
// semantics so consumers can rely on one shape. Sessions are distinct from
// Clean and Purge: the command is "uninstall" and the session ID is prefixed
// "uninstall-" so the audit trail stays unambiguous.
//
// History is written even when the run was canceled or every app was
// skipped: the audit trail must reflect what the user asked Foal to do, not
// only what succeeded.
func recordHistorySession(ctx context.Context, opts ExecuteOptions, result ExecuteResult, startedAt, endedAt time.Time) {
	if opts.HistoryRecorder == nil {
		return
	}
	command := opts.CommandParameters
	if command.Command == "" {
		command.Command = "uninstall"
	}
	session := history.SessionRecord{
		ID:        newUninstallHistorySessionID(result.Mode, endedAt),
		Command:   command,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt.UTC(),
		Mode:      result.Mode,
		Aggregate: history.AggregateOutcomes{
			CandidateCount: result.Totals.SelectedCount,
			DeletedCount:   result.Totals.UninstalledCount,
			// Skipped and failed are disjoint: do not fold FailedCount into
			// SkippedCount. Consumers may sum skipped+error without
			// double-counting.
			SkippedCount: result.Totals.SkippedCount,
			ErrorCount:   result.Totals.FailedCount + result.Totals.CanceledCount,
			// Uninstall leftover deletion uses the Recycle Bin but does not
			// measure leftover path bytes (no size scan in this slice), so
			// AffectedBytes stays zero. Per-path outcomes are recorded as
			// individual ItemRecords below for path-level audit.
		},
	}
	// Cancellation must not erase already-produced outcomes; history write is
	// bounded and not extended by the caller's cancel.
	_ = opts.HistoryRecorder.Record(context.WithoutCancel(ctx), session, uninstallHistoryItems(session.ID, result))
}

func newUninstallHistorySessionID(mode string, at time.Time) string {
	return fmt.Sprintf("uninstall-%s-%s", mode, at.UTC().Format("20060102T150405.000000000Z"))
}

// uninstallHistoryItems builds one ItemRecord per selected application plus
// one ItemRecord per leftover path outcome. The Rule field carries the app's
// planned class (for app records) or "leftover" (for path records) so the
// audit trail records what Foal attempted, and SkippedReason/Error capture
// the stable failure code.
//
// Leftover path items are distinct from the app-level record so consumers
// can audit which paths were deleted via the Recycle Bin and which were
// skipped. They carry PlannedAction=recycle_bin and the actual per-path
// Action/Result; a skipped path records SkippedReason with the stable
// pathsafe.Reason code (protected_path, stat_failed, reparse_point, etc).
func uninstallHistoryItems(sessionID string, result ExecuteResult) []history.ItemRecord {
	items := make([]history.ItemRecord, 0, len(result.Applications))
	for _, app := range result.Applications {
		item := history.ItemRecord{
			SessionID:     sessionID,
			Path:          "", // App-level record targets the application, not a path.
			Rule:          app.PlannedClass,
			PlannedAction: app.Action,
			Action:        app.Action,
			Result:        app.Result,
		}
		// Portable removal targets a specific install location path and is a
		// permanent deletion (ordinary filesystem removal, NOT the Recycle
		// Bin). Record the targeted path and set PlannedAction to
		// portable_removal so the audit trail distinguishes a planned
		// permanent deletion from an actual skip. This lets consumers tell
		// portable removal (permanent deletion of the install tree) apart
		// from leftover deletion (Recycle Bin move) and from official
		// uninstaller invocation, matching the spec's "planned vs actual
		// permanent action" recording requirement.
		if app.PlannedClass == PlannedClassPortableDirectoryRemoval {
			item.Path = app.PortableRemovalPath
			item.PlannedAction = ActionPortableRemoval
		}
		switch app.Result {
		case ResultSkipped:
			item.SkippedReason = &history.Issue{
				Code:        app.SkippedReason,
				Message:     app.Detail,
				Recoverable: true,
			}
		case ResultFailed, ResultCanceled:
			item.Error = &history.Issue{
				Code:        app.SkippedReason,
				Message:     app.Detail,
				Recoverable: true,
			}
		}
		items = append(items, item)

		// One ItemRecord per leftover path outcome. These are populated
		// only after a successful uninstaller; on failure or cancel the
		// app has no LeftoverOutcomes and this loop is a no-op.
		for _, leftover := range app.LeftoverOutcomes {
			leftoverItem := history.ItemRecord{
				SessionID:     sessionID,
				Path:          leftover.Path,
				Rule:          "leftover",
				PlannedAction: ActionLeftoverRecycleBin,
				Action:        leftover.Action,
				Result:        leftover.Result,
			}
			if leftover.Result == ResultLeftoverSkipped {
				leftoverItem.SkippedReason = &history.Issue{
					Code:        leftover.Reason,
					Message:     leftover.Detail,
					Recoverable: true,
				}
			}
			items = append(items, leftoverItem)
		}
	}
	return items
}
