package clean

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
	Identifier    string
	Label         string
	State         CategoryExecutionState
	// AffectedBytes counts successful Recycle Bin moves for this category only.
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
	case ExecutionPhaseRecycleBinOperations, ExecutionPhaseComplete:
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
		deleted        int
		skipped        int
		affectedBytes  int64
		hasCancel      bool
		hasOperational bool
		hasSafetySkip  bool
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
			b.affectedBytes += item.Bytes
		}
	}
	for _, item := range result.Skipped {
		b := byID[item.Rule]
		if b == nil {
			continue
		}
		b.skipped++
		switch classifyExecutionIssueCode(item.Reason.Code) {
		case executionIssueCancel:
			b.hasCancel = true
		case executionIssueOperational:
			b.hasOperational = true
		default:
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
		switch classifyExecutionIssueCode(issue.Code) {
		case executionIssueCancel:
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
		switch classifyExecutionIssueCode(issue.Code) {
		case executionIssueCancel:
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
		state := projectCategoryExecutionState(b.deleted, b.skipped, hasCancel, hasOperational, b.hasSafetySkip)
		out = append(out, CategoryExecutionOutcome{
			Identifier:    id,
			Label:         label,
			State:         state,
			AffectedBytes: b.affectedBytes,
			DeletedCount:  b.deleted,
			SkippedCount:  b.skipped,
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

// SumExecutionAffectedBytes sums successful Recycle Bin move bytes only.
func SumExecutionAffectedBytes(outcomes []CategoryExecutionOutcome) int64 {
	var total int64
	for _, outcome := range outcomes {
		total += outcome.AffectedBytes
	}
	return total
}

type executionIssueKind int

const (
	executionIssueSafety executionIssueKind = iota
	executionIssueOperational
	executionIssueCancel
)

func classifyExecutionIssueCode(code string) executionIssueKind {
	switch code {
	case "context_canceled":
		return executionIssueCancel
	case "delete_failed", "permission_denied", "unsupported_target",
		"invalid_category_plan", PreviewReasonInspectionFailed,
		"protection_file_load_failed", "protection_file_invalid_utf8":
		return executionIssueOperational
	default:
		// Safety skips and path-safety rejections (protected_path, recycle_bin_*,
		// running application, reparse_point, hardlink, etc.).
		return executionIssueSafety
	}
}

func projectCategoryExecutionState(deleted, skipped int, hasCancel, hasOperational, hasSafetySkip bool) CategoryExecutionState {
	if deleted > 0 {
		if skipped > 0 || hasCancel || hasOperational {
			return CategoryExecutionPartial
		}
		return CategoryExecutionCleaned
	}
	// Zero successes.
	if hasCancel {
		return CategoryExecutionCanceled
	}
	if hasOperational {
		return CategoryExecutionFailed
	}
	if hasSafetySkip || skipped > 0 {
		return CategoryExecutionSkipped
	}
	return CategoryExecutionEmpty
}
