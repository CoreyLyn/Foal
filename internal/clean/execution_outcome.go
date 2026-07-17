package clean

import "strings"

// CategoryExecutionState is the path-free lifecycle for one selected category
// during shared Clean execution and its terminal projection from the final
// Result. In-progress values (rechecking, ready, cleaning) come from shared
// phase observations; terminal values come only from the authoritative Result.
type CategoryExecutionState string

const (
	CategoryExecutionRechecking CategoryExecutionState = "rechecking"
	CategoryExecutionReady      CategoryExecutionState = "ready"
	CategoryExecutionCleaning   CategoryExecutionState = "cleaning"
	CategoryExecutionEmpty      CategoryExecutionState = "empty"
	CategoryExecutionCleaned    CategoryExecutionState = "cleaned"
	CategoryExecutionPartial    CategoryExecutionState = "partial"
	CategoryExecutionSkipped    CategoryExecutionState = "skipped"
	CategoryExecutionFailed     CategoryExecutionState = "failed"
	CategoryExecutionCanceled   CategoryExecutionState = "canceled"
)

// CategoryExecutionOutcome is the path-free projection of one selected category
// after shared Clean returns its final Result. It never carries candidate paths,
// protected paths, or raw path-bearing errors. Item-level Result and History
// remain authoritative for executed, skipped, and context_canceled items.
type CategoryExecutionOutcome struct {
	Identifier string
	Label      string
	State      CategoryExecutionState
	// RecycleBinMovedBytes is successful Recycle Bin work for this category.
	RecycleBinMovedBytes int64
	// PermanentlyDeletedBytes is successful permanent deletion for this category.
	PermanentlyDeletedBytes int64
	// AffectedBytes is RecycleBinMovedBytes + PermanentlyDeletedBytes (processed
	// content, not released disk space).
	AffectedBytes int64
	DeletedCount  int
	SkippedCount  int
}

// IsTerminalExecutionState reports whether state is a final category outcome.
func IsTerminalExecutionState(state CategoryExecutionState) bool {
	switch state {
	case CategoryExecutionEmpty, CategoryExecutionCleaned,
		CategoryExecutionPartial, CategoryExecutionSkipped,
		CategoryExecutionFailed, CategoryExecutionCanceled:
		return true
	default:
		return false
	}
}

// InProgressExecutionState projects the shared execution phase onto every
// selected category while work is still in flight. It does not invent
// byte-derived progress or per-item state.
func InProgressExecutionState(phase ExecutionPhase) CategoryExecutionState {
	switch phase {
	case ExecutionPhaseRecycleBinSafety:
		return CategoryExecutionReady
	case ExecutionPhaseRecycleBinOperations, ExecutionPhasePermanentOperations, ExecutionPhaseComplete:
		return CategoryExecutionCleaning
	case ExecutionPhaseScanning:
		return CategoryExecutionRechecking
	default:
		// Before the first observation arrives, treat work as fresh scanning.
		return CategoryExecutionRechecking
	}
}

// ProjectCategoryExecutionOutcomes maps the authoritative final Result onto the
// frozen exact selection in selection order. Categories absent from the Result
// with no attributed items are empty. Progress observations never feed this
// projection; only Deleted, Skipped, and Errors (by Rule) do.
func ProjectCategoryExecutionOutcomes(selected []string, result Result) []CategoryExecutionOutcome {
	if len(selected) == 0 {
		return nil
	}

	type bucket struct {
		deleted                 int
		skipped                 int
		recycleBinMovedBytes    int64
		permanentlyDeletedBytes int64
		hasCancel               bool
		hasOperational          bool
		hasSafetySkip           bool
	}
	byID := make(map[string]*bucket, len(selected))
	for _, id := range selected {
		byID[id] = &bucket{}
	}

	for _, item := range result.Deleted {
		b := byID[item.Rule]
		if b == nil {
			continue
		}
		b.deleted++
		if item.Bytes > 0 {
			switch DeletionAction(item.Action) {
			case DeletionActionDeletePermanently:
				b.permanentlyDeletedBytes += item.Bytes
			default:
				// Empty action (legacy/test fixtures) and move_to_recycle_bin
				// both count as Recycle Bin work.
				b.recycleBinMovedBytes += item.Bytes
			}
		}
	}
	for _, item := range result.Failed {
		b := byID[item.Rule]
		if b == nil {
			continue
		}
		// Permanent post-mutation failures count as operational skips for
		// category projection (failed/partial), not successful deletions.
		b.skipped++
		b.hasOperational = true
	}
	for _, item := range result.Skipped {
		b := byID[item.Rule]
		if b == nil {
			continue
		}
		b.skipped++
		switch classifyEvidenceIssueCode(item.Reason.Code) {
		case evidenceIssueCancel:
			b.hasCancel = true
		case evidenceIssueOperational:
			b.hasOperational = true
		default:
			// Safety, incomplete, and running-unknown residuals are safety skips
			// on the execution surface (not operational failure).
			b.hasSafetySkip = true
		}
	}
	for _, issue := range result.Errors {
		if issue.Rule == "" {
			// Run-level failures without a category Rule apply to every selected
			// category that still has zero success, so empty selections surface
			// as failed rather than silently empty.
			continue
		}
		b := byID[issue.Rule]
		if b == nil {
			continue
		}
		switch classifyEvidenceIssueCode(issue.Code) {
		case evidenceIssueCancel:
			b.hasCancel = true
		default:
			b.hasOperational = true
		}
	}

	runLevelFailed := result.Status == "error"
	runLevelCancel := false
	runLevelOperational := false
	for _, issue := range result.Errors {
		if issue.Rule != "" {
			continue
		}
		switch classifyEvidenceIssueCode(issue.Code) {
		case evidenceIssueCancel:
			runLevelCancel = true
		default:
			runLevelOperational = true
			runLevelFailed = true
		}
	}

	out := make([]CategoryExecutionOutcome, 0, len(selected))
	catalog := CanonicalCleanupCategoryCatalog()
	for _, id := range selected {
		b := byID[id]
		label := id
		if summary, ok := catalog.Summary(id); ok {
			label = summary.Label
		}
		hasCancel := b.hasCancel || runLevelCancel
		hasOperational := b.hasOperational || (runLevelOperational && b.deleted == 0 && b.skipped == 0)
		// A whole-run error with no attributed items fails every empty category.
		if runLevelFailed && b.deleted == 0 && b.skipped == 0 && !hasCancel {
			hasOperational = true
		}
		state := mapExecutionOutcome(categoryEvidenceFactors{
			SuccessCount:       b.deleted,
			SkippedCount:       b.skipped,
			Canceled:           hasCancel,
			HasOperationalFail: hasOperational,
			HasSafetySkip:      b.hasSafetySkip,
		})
		out = append(out, CategoryExecutionOutcome{
			Identifier:              id,
			Label:                   label,
			State:                   state,
			RecycleBinMovedBytes:    b.recycleBinMovedBytes,
			PermanentlyDeletedBytes: b.permanentlyDeletedBytes,
			AffectedBytes:           b.recycleBinMovedBytes + b.permanentlyDeletedBytes,
			DeletedCount:            b.deleted,
			SkippedCount:            b.skipped,
		})
	}
	return out
}

// CountTerminalExecutionOutcomes returns how many projected outcomes are terminal.
func CountTerminalExecutionOutcomes(outcomes []CategoryExecutionOutcome) int {
	n := 0
	for _, outcome := range outcomes {
		if IsTerminalExecutionState(outcome.State) {
			n++
		}
	}
	return n
}

// SumExecutionAffectedBytes sums successful processed content across outcomes
// (Recycle Bin moves plus permanent deletions). It is not released disk space.
func SumExecutionAffectedBytes(outcomes []CategoryExecutionOutcome) int64 {
	var total int64
	for _, outcome := range outcomes {
		total += outcome.AffectedBytes
	}
	return total
}

// SumExecutionRecycleBinMovedBytes sums successful Recycle Bin move bytes.
func SumExecutionRecycleBinMovedBytes(outcomes []CategoryExecutionOutcome) int64 {
	var total int64
	for _, outcome := range outcomes {
		total += outcome.RecycleBinMovedBytes
	}
	return total
}

// SumExecutionPermanentlyDeletedBytes sums successful permanent deletion bytes.
func SumExecutionPermanentlyDeletedBytes(outcomes []CategoryExecutionOutcome) int64 {
	var total int64
	for _, outcome := range outcomes {
		total += outcome.PermanentlyDeletedBytes
	}
	return total
}

// PermanentPartialRiskWarning is the path-free result notice shown when shared
// Clean reports permanent_delete_failed or a permanent cancel after mutation
// may have begun. Raw path-bearing issue text is never forwarded.
const PermanentPartialRiskWarning = "Some permanent deletion may have partially completed and cannot be undone."

// ResultHasPermanentPartialRisk reports whether the authoritative Result
// indicates permanent mutation may have partially completed.
func ResultHasPermanentPartialRisk(result Result) bool {
	for _, item := range result.Failed {
		if item.Reason.Code == permanentDeleteFailedIssueCode ||
			DeletionAction(item.Action) == DeletionActionDeletePermanently ||
			DeletionAction(item.PlannedAction) == DeletionActionDeletePermanently {
			return true
		}
	}
	for _, item := range result.Skipped {
		if DeletionAction(item.PlannedAction) != DeletionActionDeletePermanently {
			continue
		}
		if item.Reason.Code == "context_canceled" {
			// Cancel after possible permanent mutation is the only permanent
			// cancel that carries partial-risk semantics in shared Clean.
			msg := strings.ToLower(item.Reason.Message)
			if strings.Contains(msg, "may already") || strings.Contains(msg, "partial") {
				return true
			}
		}
	}
	for _, issue := range result.Errors {
		if issue.Code == permanentDeleteFailedIssueCode {
			return true
		}
	}
	return false
}


