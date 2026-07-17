package purge

import (
	"context"
	"fmt"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

func recordHistorySession(ctx context.Context, opts Options, result Result, startedAt, endedAt time.Time) {
	if opts.HistoryRecorder == nil {
		return
	}
	command := opts.CommandParameters
	if command.Command == "" {
		command.Command = "purge"
	}
	session := history.SessionRecord{
		ID:        newHistorySessionID(result.Mode, endedAt),
		Command:   command,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt.UTC(),
		Mode:      result.Mode,
		Aggregate: history.AggregateOutcomes{
			CandidateCount:          result.Totals.CandidateCount,
			DeletedCount:            result.Totals.DeletedCount,
			SkippedCount:            result.Totals.SkippedCount + result.Totals.FailedCount,
			ErrorCount:              result.Totals.FailedCount,
			CandidateBytes:          result.Totals.Bytes,
			PermanentlyDeletedBytes: result.Totals.PermanentlyDeletedBytes,
			AffectedBytes:           result.Totals.AffectedBytes,
		},
	}
	// Cancellation must not erase already-produced outcomes; history write is
	// bounded and not extended by the caller's cancel.
	_ = opts.HistoryRecorder.Record(context.WithoutCancel(ctx), session, historyItems(session.ID, result))
}

func newHistorySessionID(mode string, at time.Time) string {
	return fmt.Sprintf("purge-%s-%s", mode, at.UTC().Format("20060102T150405.000000000Z"))
}

func historyItems(sessionID string, result Result) []history.ItemRecord {
	items := make([]history.ItemRecord, 0, len(result.Candidates)+len(result.Deleted)+len(result.Failed)+len(result.Skipped))
	if result.Mode == ModeDryRun {
		for _, candidate := range result.Candidates {
			bytes := candidate.Bytes
			items = append(items, history.ItemRecord{
				SessionID:     sessionID,
				Path:          candidate.Path,
				Rule:          candidate.Kind,
				PlannedAction: PlannedActionDeletePermanently,
				Bytes:         &bytes,
				Result:        "candidate",
			})
		}
	}
	for _, deleted := range result.Deleted {
		bytes := deleted.Bytes
		items = append(items, history.ItemRecord{
			SessionID: sessionID,
			Path:      deleted.Path,
			Rule:      deleted.Kind,
			Action:    PlannedActionDeletePermanently,
			Bytes:     &bytes,
			Result:    "deleted",
		})
	}
	for _, failed := range result.Failed {
		bytes := failed.Bytes
		items = append(items, history.ItemRecord{
			SessionID:     sessionID,
			Path:          failed.Path,
			Rule:          failed.Kind,
			PlannedAction: PlannedActionDeletePermanently,
			Action:        PlannedActionDeletePermanently,
			Bytes:         &bytes,
			Result:        "failed",
			Error: &history.Issue{
				Code:        failed.Reason.Code,
				Message:     failed.Reason.Message,
				Recoverable: failed.Reason.Recoverable,
			},
		})
	}
	for _, skipped := range result.Skipped {
		item := history.ItemRecord{
			SessionID:     sessionID,
			Path:          skipped.Path,
			Rule:          skipped.Kind,
			PlannedAction: skipped.PlannedAction,
			Result:        "skipped",
			SkippedReason: &history.Issue{
				Code:        skipped.Reason,
				Message:     skipped.Detail,
				Recoverable: true,
			},
		}
		if skipped.Bytes > 0 {
			bytes := skipped.Bytes
			item.Bytes = &bytes
		}
		if item.PlannedAction == "" && result.Mode == ModeExecute {
			item.PlannedAction = PlannedActionDeletePermanently
		}
		items = append(items, item)
	}
	return items
}
