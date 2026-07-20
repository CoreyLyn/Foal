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
			// Uninstall does not reclaim bytes in this slice; leftover
			// deletion (#292) will populate affected bytes later.
		},
	}
	// Cancellation must not erase already-produced outcomes; history write is
	// bounded and not extended by the caller's cancel.
	_ = opts.HistoryRecorder.Record(context.WithoutCancel(ctx), session, uninstallHistoryItems(session.ID, result))
}

func newUninstallHistorySessionID(mode string, at time.Time) string {
	return fmt.Sprintf("uninstall-%s-%s", mode, at.UTC().Format("20060102T150405.000000000Z"))
}

// uninstallHistoryItems builds one ItemRecord per selected application. The
// Rule field carries the app's planned class so the audit trail records what
// Foal attempted, and SkippedReason/Error capture the stable failure code.
func uninstallHistoryItems(sessionID string, result ExecuteResult) []history.ItemRecord {
	items := make([]history.ItemRecord, 0, len(result.Applications))
	for _, app := range result.Applications {
		item := history.ItemRecord{
			SessionID:     sessionID,
			Path:          "", // Uninstall targets applications, not paths, in this slice.
			Rule:          app.PlannedClass,
			PlannedAction: app.Action,
			Action:        app.Action,
			Result:        app.Result,
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
	}
	return items
}
