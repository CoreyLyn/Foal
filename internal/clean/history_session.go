package clean

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
	result = withoutOpportunityReviewData(result)
	session := history.SessionRecord{
		ID:        newHistorySessionID(result.Mode, endedAt),
		Command:   opts.CommandParameters,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt.UTC(),
		Mode:      result.Mode,
		Aggregate: history.AggregateOutcomes{
			CandidateCount:           result.Totals.CandidateCount,
			DeletedCount:             result.Totals.DeletedCount,
			SkippedCount:             result.Totals.SkippedCount,
			ErrorCount:               len(result.Errors) + len(result.Failed),
			OpportunityCount:         result.Totals.OpportunityCount,
			CandidateBytes:           result.Totals.CandidateBytes,
			OpportunityObservedBytes: result.Totals.OpportunityObservedBytes,
			OptInDeletedCount:        result.Totals.OptInDeletedCount,
			OptInAffectedBytes:       result.Totals.OptInAffectedBytes,
			RecycleBinMovedBytes:     result.Totals.RecycleBinMovedBytes,
			PermanentlyDeletedBytes:  result.Totals.PermanentlyDeletedBytes,
			AffectedBytes:            result.Totals.AffectedBytes,
		},
	}
	// Cancellation stops remaining cleanup work, but must not erase the outcomes
	// already produced. Persist those outcomes without extending cancellation to
	// the bounded history write.
	_ = opts.HistoryRecorder.Record(context.WithoutCancel(ctx), session, historyItems(session.ID, result))
}

func withoutOpportunityReviewData(result Result) Result {
	result.RunningApplications = nil
	if len(result.IncompleteOpportunityInspections) == 0 {
		result.Errors = withoutRunningApplicationDiagnostics(result.Errors)
		result.Opportunities = nil
		return result
	}
	incompleteIssues := make(map[string]struct{}, len(result.IncompleteOpportunityInspections))
	for _, incomplete := range result.IncompleteOpportunityInspections {
		incompleteIssues[structuredIssueKey(incomplete.Reason)] = struct{}{}
	}
	errorsForHistory := make([]StructuredIssue, 0, len(result.Errors))
	for _, issue := range result.Errors {
		if _, reviewOnly := incompleteIssues[structuredIssueKey(issue)]; reviewOnly {
			continue
		}
		if issue.Code == runningApplicationDetectionIssueCode {
			continue
		}
		errorsForHistory = append(errorsForHistory, issue)
	}
	result.Opportunities = nil
	result.IncompleteOpportunityInspections = nil
	result.Errors = errorsForHistory
	return result
}

func withoutRunningApplicationDiagnostics(issues []StructuredIssue) []StructuredIssue {
	if len(issues) == 0 {
		return issues
	}
	filtered := make([]StructuredIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Code == runningApplicationDetectionIssueCode {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

func structuredIssueKey(issue StructuredIssue) string {
	return issue.Path + "\x00" + issue.Code + "\x00" + issue.Message
}

func newHistorySessionID(mode string, at time.Time) string {
	return fmt.Sprintf("clean-%s-%s", mode, at.UTC().Format("20060102T150405.000000000Z"))
}

func historyItems(sessionID string, result Result) []history.ItemRecord {
	items := make([]history.ItemRecord, 0, len(result.Candidates)+len(result.Deleted)+len(result.Failed)+len(result.Skipped)+len(result.Errors))
	if result.Mode == "dry_run" {
		for _, candidate := range result.Candidates {
			bytes := candidate.Bytes
			items = append(items, history.ItemRecord{
				SessionID:     sessionID,
				Path:          candidate.Path,
				Rule:          candidate.Rule,
				PlannedAction: candidate.PlannedAction,
				Bytes:         &bytes,
				Result:        "candidate",
			})
		}
	}
	for _, deleted := range result.Deleted {
		bytes := deleted.Bytes
		action := deleted.Action
		if action == "" {
			// Successful Recycle Bin path always records actual action.
			action = plannedRecycleBinAction
		}
		items = append(items, history.ItemRecord{
			SessionID: sessionID,
			Path:      deleted.Path,
			Rule:      deleted.Rule,
			Action:    action,
			Bytes:     &bytes,
			Result:    "deleted",
		})
	}
	for _, failed := range result.Failed {
		bytes := failed.Bytes
		action := failed.Action
		if action == "" {
			action = string(DeletionActionDeletePermanently)
		}
		planned := failed.PlannedAction
		if planned == "" {
			planned = action
		}
		items = append(items, history.ItemRecord{
			SessionID:     sessionID,
			Path:          failed.Path,
			Rule:          failed.Rule,
			PlannedAction: planned,
			Action:        action,
			Bytes:         &bytes,
			Result:        "failed",
			Error:         historyIssue(failed.Reason),
		})
	}
	for _, skipped := range result.Skipped {
		planned := skipped.PlannedAction
		if planned == "" {
			planned = plannedActionForCategory(skipped.Rule)
		}
		item := history.ItemRecord{
			SessionID:     sessionID,
			Path:          skipped.Path,
			Rule:          skipped.Rule,
			PlannedAction: planned,
			Result:        "skipped",
			SkippedReason: historyIssue(skipped.Reason),
		}
		if skipped.Bytes > 0 {
			bytes := skipped.Bytes
			item.Bytes = &bytes
		}
		items = append(items, item)
	}
	for _, err := range result.Errors {
		items = append(items, history.ItemRecord{
			SessionID: sessionID,
			Path:      err.Path,
			Rule:      err.Rule,
			Result:    "error",
			Error:     historyIssue(err),
		})
	}
	return items
}

func historyIssue(issue StructuredIssue) *history.Issue {
	return &history.Issue{
		Code:        issue.Code,
		Message:     issue.Message,
		Recoverable: issue.Recoverable,
	}
}
