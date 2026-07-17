package purge

import (
	"context"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
)

// Execute rediscovers allowlisted project artifacts under the same root policy
// as DryRun, then permanently deletes them only when per-run permanent
// authorization is present. It never trusts a prior dry-run path list alone:
// discovery is always fresh, and path-safe validation runs immediately before
// each delete. Cancellation does not roll back completed permanent deletions.
func Execute(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	preview, errResult, ok := discover(ctx, opts, ModeExecute, start)
	if !ok {
		// Discovery error/cancel: still attempt history for audit of canceled/error runs
		// only when we have a root; record when recorder is set.
		if errResult.Root != "" || errResult.Status == StatusCanceled {
			recordHistorySession(ctx, opts, errResult, start, time.Now())
		}
		return errResult
	}

	result := Result{
		Status:     StatusOK,
		Mode:       ModeExecute,
		Root:       preview.root,
		Candidates: preview.candidates,
		Deleted:    []DeletedItem{},
		Failed:     []FailedItem{},
		Skipped:    append([]Skipped(nil), preview.skipped...),
		Notices:    highImpactNotices(true),
	}

	if len(preview.candidates) == 0 {
		result.Notices = nil
		result.Totals = computeExecuteTotals(result)
		result.ElapsedMS = time.Since(start).Milliseconds()
		recordHistorySession(ctx, opts, result, start, time.Now())
		return result
	}

	if !opts.AllowPermanentDeletion {
		for _, c := range preview.candidates {
			result.Skipped = append(result.Skipped, Skipped{
				Path:          c.Path,
				Reason:        IssuePermanentDeletionNotAuthorized,
				Detail:        "permanent deletion is not authorized for this run; planned action is unchanged",
				Kind:          c.Kind,
				Bytes:         c.Bytes,
				PlannedAction: PlannedActionDeletePermanently,
			})
		}
		result.Totals = computeExecuteTotals(result)
		result.ElapsedMS = time.Since(start).Milliseconds()
		recordHistorySession(ctx, opts, result, start, time.Now())
		return result
	}

	remover := opts.PermanentRemover
	if remover == nil {
		remover = delete.FilesystemPermanentRemover{}
	}

	deleteCandidates := make([]delete.Candidate, 0, len(preview.candidates))
	byPath := make(map[string]Candidate, len(preview.candidates))
	for _, c := range preview.candidates {
		deleteCandidates = append(deleteCandidates, delete.Candidate{Path: c.Path, Bytes: c.Bytes})
		byPath[c.Path] = c
	}

	permanentResult := delete.ExecutePermanentWithValidator(ctx, deleteCandidates, remover, opts.Validator)
	canceled := false
	for _, item := range permanentResult.Items {
		c := byPath[item.Path]
		switch item.Kind {
		case delete.PermanentOutcomeDeleted:
			result.Deleted = append(result.Deleted, DeletedItem{
				Kind:         c.Kind,
				Path:         item.Path,
				RelativePath: c.RelativePath,
				Bytes:        item.Bytes,
				Action:       PlannedActionDeletePermanently,
			})
		case delete.PermanentOutcomeFailed:
			result.Failed = append(result.Failed, FailedItem{
				Kind:          c.Kind,
				Path:          item.Path,
				RelativePath:  c.RelativePath,
				Bytes:         item.Bytes,
				PlannedAction: PlannedActionDeletePermanently,
				Action:        PlannedActionDeletePermanently,
				Reason: Issue{
					Code:        IssuePermanentDeleteFailed,
					Message:     item.Reason.Message,
					Recoverable: true,
					Path:        item.Path,
				},
			})
		case delete.PermanentOutcomeCanceled:
			canceled = true
			result.Skipped = append(result.Skipped, Skipped{
				Path:          item.Path,
				Reason:        IssueContextCanceled,
				Detail:        item.Reason.Message,
				Kind:          c.Kind,
				Bytes:         item.Bytes,
				PlannedAction: PlannedActionDeletePermanently,
			})
		default:
			// Pre-mutation skips (pathsafe validation, protection, reparse, …).
			reason := item.Reason.Code
			if reason == "" {
				reason = "validation_failed"
			}
			result.Skipped = append(result.Skipped, Skipped{
				Path:          item.Path,
				Reason:        reason,
				Detail:        item.Reason.Message,
				Kind:          c.Kind,
				Bytes:         item.Bytes,
				PlannedAction: PlannedActionDeletePermanently,
			})
		}
	}

	if canceled && len(result.Deleted) == 0 && len(result.Failed) == 0 {
		// Entire batch canceled before any mutation outcome — keep status canceled
		// only when nothing succeeded or failed; otherwise remain ok with honest partials.
		if allCanceled(permanentResult) {
			result.Status = StatusCanceled
			result.Message = "purge execution canceled; completed permanent deletions are not rolled back"
		}
	} else if canceled {
		result.Message = "purge execution canceled after partial progress; completed permanent deletions are not rolled back"
	}

	result.Totals = computeExecuteTotals(result)
	result.ElapsedMS = time.Since(start).Milliseconds()
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

func allCanceled(result delete.PermanentResult) bool {
	if len(result.Items) == 0 {
		return false
	}
	for _, item := range result.Items {
		if item.Kind != delete.PermanentOutcomeCanceled {
			return false
		}
	}
	return true
}

func computeExecuteTotals(result Result) Totals {
	var candidateBytes int64
	for _, c := range result.Candidates {
		candidateBytes += c.Bytes
	}
	var permanentlyDeleted int64
	for _, d := range result.Deleted {
		permanentlyDeleted += d.Bytes
	}
	// Discovery skips that are not candidate authorization/validation skips still
	// count toward SkippedCount; failed permanent items are counted separately
	// (and not as permanently deleted bytes).
	return Totals{
		CandidateCount:          len(result.Candidates),
		Bytes:                   candidateBytes,
		DeletedCount:            len(result.Deleted),
		SkippedCount:            len(result.Skipped),
		FailedCount:             len(result.Failed),
		PermanentlyDeletedBytes: permanentlyDeleted,
		AffectedBytes:           permanentlyDeleted,
	}
}
