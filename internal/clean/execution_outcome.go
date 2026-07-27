package clean

import "strings"

// CategoryExecutionState is the path-free lifecycle for one selected category
// during shared Clean execution and its terminal projection. In-progress values
// (waiting, rechecking, ready, cleaning) come from shared phase observations
// plus ActiveCategory. Provisional terminal values may also arrive mid-flight
// via ExecutionProgress.CompletedCategory once that category's work for the
// run is finished; the authoritative final Result always overwrites them.
type CategoryExecutionState string

const (
	// CategoryExecutionWaiting is selected but not yet the ActiveCategory
	// during shared execution. Distinct from preview waiting. Progress promotes
	// waiting to a provisional terminal only after shared Clean finishes that
	// category's resolve+mutate (or empty/skip with no further work).
	CategoryExecutionWaiting    CategoryExecutionState = "waiting"
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
	// ReasonCode is the stable path-free issue code that best explains a
	// non-successful outcome (failed, partial, skipped, canceled). It is chosen
	// from the category's own Failed/Skipped/Errors evidence, falling back to a
	// run-level issue when the category itself recorded none. It is only ever a
	// stable Foal-owned code — never StructuredIssue.Message or .Path, which may
	// embed raw OS text and candidate paths. Empty when no issue applies.
	ReasonCode string
	// ServicingReason is the stable path-free reason for a servicing category's
	// skipped or failed execution outcome. Empty for non-servicing categories and
	// for servicing outcomes that carry no reason (completed, no_work). It is a
	// stable Foal-owned code, never raw DISM text.
	ServicingReason string
	// ServicingExitCode is the DISM exit code for a servicing category's
	// execution outcome, present only when DISM ran to exit. Nil for
	// non-servicing categories and authorization skips.
	ServicingExitCode *int
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

// InProgressExecutionState projects the shared execution phase onto the active
// category while work is still in flight. It does not invent byte-derived
// progress, per-item state, or terminal outcomes.
func InProgressExecutionState(phase ExecutionPhase) CategoryExecutionState {
	switch phase {
	case ExecutionPhaseRecycleBinSafety:
		return CategoryExecutionReady
	case ExecutionPhaseRecycleBinOperations, ExecutionPhasePermanentOperations, ExecutionPhaseComplete:
		return CategoryExecutionCleaning
	case ExecutionPhaseServicingOperations:
		// Windows servicing is a mutating final action group; show it as active
		// cleaning while the elevated helper runs (no byte-derived progress).
		return CategoryExecutionCleaning
	case ExecutionPhaseScanning:
		return CategoryExecutionRechecking
	default:
		// Before the first observation arrives, treat work as fresh scanning.
		return CategoryExecutionRechecking
	}
}

// ProjectInProgressCategoryState maps one path-free progress observation onto a
// single selected category. When ActiveCategory is non-empty, only that
// category receives the phase-mapped in-progress state; other categories stay
// waiting unless already advanced (callers preserve prior non-waiting state).
// Empty ActiveCategory means the phase is not category-scoped (e.g. aggregate
// Recycle Bin safety): every still-open category receives the phase mapping.
// This helper never invents terminal outcomes; use CompletedCategory fields or
// ProjectCategoryExecutionOutcomes for terminals.
func ProjectInProgressCategoryState(phase ExecutionPhase, activeCategory, categoryID string) CategoryExecutionState {
	if activeCategory != "" && activeCategory != categoryID {
		return CategoryExecutionWaiting
	}
	return InProgressExecutionState(phase)
}

// ProjectProvisionalCategoryOutcome maps items already recorded on result for
// one category onto a mid-flight terminal projection. It reuses the same
// evidence mapping as the final Result projection so provisional states match
// final rules for that category's finished work. Call only after that
// category's resolve+mutate path for this run is done (or resolve proved empty
// with no candidates queued). The final Result still overwrites at the end.
func ProjectProvisionalCategoryOutcome(categoryID string, result Result) CategoryExecutionOutcome {
	id := strings.TrimSpace(categoryID)
	if id == "" {
		return CategoryExecutionOutcome{}
	}
	outcomes := ProjectCategoryExecutionOutcomes([]string{id}, result)
	if len(outcomes) == 0 {
		return CategoryExecutionOutcome{Identifier: id, State: CategoryExecutionEmpty}
	}
	return outcomes[0]
}

// HasCategoryCompletion reports whether progress carries a usable mid-flight
// terminal observation for one category.
func HasCategoryCompletion(progress ExecutionProgress) bool {
	return strings.TrimSpace(progress.CompletedCategory) != "" &&
		IsTerminalExecutionState(progress.CompletedState)
}

// executionReasonPriority ranks issue classes when a category recorded more
// than one issue code. An operational failure explains a failed outcome better
// than a deliberate safety skip, and cancellation outranks ordinary safety.
// Equal priority keeps the first code observed so projection stays deterministic.
func executionReasonPriority(class evidenceIssueClass) int {
	switch class {
	case evidenceIssueOperational:
		return 3
	case evidenceIssueCancel:
		return 2
	default:
		return 1
	}
}

// ProjectCategoryExecutionOutcomes maps the authoritative final Result onto the
// frozen exact selection in selection order. Categories absent from the Result
// with no attributed items are empty. Mid-flight progress never feeds this
// projection; only Deleted, Failed, Skipped, and Errors (by Rule) do. Callers
// must replace any provisional progress outcomes with this projection when the
// final Result arrives.
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
		// reasonCode is the representative path-free issue code for this
		// category; reasonPriority guards which code wins. Only codes are kept —
		// never issue Message or Path text.
		reasonCode     string
		reasonPriority int
	}
	// noteReason records the highest-priority issue code seen for one category.
	noteReason := func(b *bucket, code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		priority := executionReasonPriority(classifyEvidenceIssueCode(code))
		if priority > b.reasonPriority {
			b.reasonCode = code
			b.reasonPriority = priority
		}
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
			switch PlannedAction(item.Action) {
			case PlannedActionDeletePermanently:
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
		noteReason(b, item.Reason.Code)
	}
	for _, item := range result.Skipped {
		b := byID[item.Rule]
		if b == nil {
			continue
		}
		b.skipped++
		noteReason(b, item.Reason.Code)
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
		noteReason(b, issue.Code)
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
	runLevelReasonCode := ""
	runLevelReasonPriority := 0
	for _, issue := range result.Errors {
		if issue.Rule != "" {
			continue
		}
		class := classifyEvidenceIssueCode(issue.Code)
		if code := strings.TrimSpace(issue.Code); code != "" {
			if priority := executionReasonPriority(class); priority > runLevelReasonPriority {
				runLevelReasonCode = code
				runLevelReasonPriority = priority
			}
		}
		switch class {
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
		// Windows servicing categories carry no file items: their lifecycle comes
		// from the path-free ServicingOperations, never Deleted/Failed/Skipped
		// buckets, and they contribute no bytes.
		if isServicingCategory(id) {
			out = append(out, projectServicingExecutionOutcome(catalog, id, result))
			continue
		}
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
		// A category with no evidence of its own can only be explained by the
		// run-level issue; one that recorded its own items keeps its own code.
		reasonCode := b.reasonCode
		if reasonCode == "" && b.deleted == 0 && b.skipped == 0 {
			reasonCode = runLevelReasonCode
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
			ReasonCode:              reasonCode,
		})
	}
	return out
}

// projectServicingExecutionOutcome maps the final Result's path-free servicing
// operation for one servicing category onto a byte-free execution outcome. A
// category with no recorded operation (for example a run that ended before the
// final servicing phase) fails closed as skipped.
func projectServicingExecutionOutcome(catalog CleanupCategoryCatalog, id string, result Result) CategoryExecutionOutcome {
	label := id
	if summary, ok := catalog.Summary(id); ok {
		label = summary.Label
	}
	state := CategoryExecutionSkipped
	var reason string
	var exitCode *int
	for _, op := range result.ServicingOperations {
		if op.Category == id {
			state = ServicingExecutionState(op.Outcome)
			reason = op.Reason
			exitCode = op.ExitCode
			break
		}
	}
	return CategoryExecutionOutcome{Identifier: id, Label: label, State: state, ServicingReason: reason, ServicingExitCode: exitCode}
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
			PlannedAction(item.Action) == PlannedActionDeletePermanently ||
			PlannedAction(item.PlannedAction) == PlannedActionDeletePermanently {
			return true
		}
	}
	for _, item := range result.Skipped {
		if PlannedAction(item.PlannedAction) != PlannedActionDeletePermanently {
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
