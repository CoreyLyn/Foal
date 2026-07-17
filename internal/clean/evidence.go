package clean

import "strings"

// evidenceIssueClass is the shared residual taxonomy for structured issue codes
// used by path-free preview and execution projections. Public label sets stay
// separate (preview vs execution product vocabulary).
type evidenceIssueClass int

const (
	// evidenceIssueSafety covers path-safety and policy exclusions
	// (protected_path, recycle_bin_*, permanent_deletion_not_authorized,
	// running application skips, reparse_point as an execute skip, etc.).
	evidenceIssueSafety evidenceIssueClass = iota
	// evidenceIssueOperational covers mutation/inspection failures that should
	// surface as failed rather than deliberate safety skips.
	evidenceIssueOperational
	// evidenceIssueIncomplete covers measurement limits and similar partial
	// inspection residuals (preview incomplete; execution treats as safety).
	evidenceIssueIncomplete
	// evidenceIssueCancel covers cooperative cancellation.
	evidenceIssueCancel
	// evidenceIssueRunningUnknown is fail-closed unknown process state. Preview
	// does not count it as an operational failed sibling; execution treats it
	// as a safety residual when it appears as a skip/error code.
	evidenceIssueRunningUnknown
)

// classifyEvidenceIssueCode maps a stable issue code onto the shared residual
// taxonomy. Unknown codes default to safety so path-safety rejections stay
// non-operational unless explicitly listed.
func classifyEvidenceIssueCode(code string) evidenceIssueClass {
	switch strings.TrimSpace(code) {
	case PreviewReasonContextCanceled: // "context_canceled"
		return evidenceIssueCancel
	case PreviewReasonInspectionLimit, "reparse_point":
		return evidenceIssueIncomplete
	case runningApplicationDetectionIssueCode:
		return evidenceIssueRunningUnknown
	case "delete_failed", "permission_denied", "unsupported_target",
		permanentDeleteFailedIssueCode,
		"invalid_category_plan", PreviewReasonInspectionFailed,
		PreviewReasonProtectionConfigFailed, // "protection_file_load_failed"
		PreviewReasonProtectionInvalidUTF8:  // "protection_file_invalid_utf8"
		return evidenceIssueOperational
	default:
		return evidenceIssueSafety
	}
}

// categoryEvidenceFactors is the path-free intermediate form of one category's
// residual evidence. Surfaces extract factors from path-bearing resolution or
// Result items; mappers produce public preview vs execution labels without
// re-reading paths (ADR 0015 / ADR 0009).
type categoryEvidenceFactors struct {
	// SuccessCount is safe candidate count (preview) or deleted count (execution).
	SuccessCount int
	// SkippedCount is path-backed skip items. Execution also counts Failed items
	// here so residual skips drive partial/failed taxonomy consistently.
	SkippedCount int
	// ProtectedCount is protection-suppressed discovery paths (preview only).
	ProtectedCount int
	// IncompleteCount is incomplete inspection residual siblings (preview).
	IncompleteCount int
	// OperationalCount is operational failed residual siblings from diagnostics
	// (preview). Execution uses HasOperationalFail instead.
	OperationalCount int
	// HasAnyDiagnostic preserves preview's "any remaining diagnostic → failed"
	// edge when operational counts alone would miss running-unknown-only rows.
	HasAnyDiagnostic bool

	Canceled           bool
	RunningBlocked     bool
	HasSafetySkip      bool
	HasOperationalFail bool

	SkipReasonCode   string
	DiagnosticReason string
}

// exclusionCount returns the path-free excluded-sibling total used by preview
// partial/skipped projection. A running/unknown gate with no path-backed skip
// still counts as one excluded sibling.
func (f categoryEvidenceFactors) exclusionCount() int {
	n := f.ProtectedCount + f.SkippedCount + f.IncompleteCount + f.OperationalCount
	if f.RunningBlocked && f.SkippedCount == 0 {
		n++
	}
	return n
}

// mapPreviewOutcome maps evidence factors onto Clean TUI category scan labels:
// complete, partial, empty, skipped, incomplete, failed.
func mapPreviewOutcome(f categoryEvidenceFactors) (state CategoryPreviewState, reason string, excluded int) {
	excluded = f.exclusionCount()
	switch {
	case f.SuccessCount > 0 && excluded > 0:
		return CategoryPreviewPartial, previewPartialReason(f), excluded
	case f.SuccessCount > 0:
		return CategoryPreviewComplete, "", 0
	case f.Canceled:
		return CategoryPreviewIncomplete, PreviewReasonContextCanceled, 0
	case f.IncompleteCount > 0 && f.ProtectedCount == 0 && f.SkippedCount == 0 && !f.RunningBlocked:
		reason = f.DiagnosticReason
		if reason == "" {
			reason = PreviewReasonInspectionLimit
		}
		return CategoryPreviewIncomplete, reason, f.IncompleteCount
	case f.ProtectedCount > 0 && f.SkippedCount == 0 && !f.RunningBlocked && f.IncompleteCount == 0 && f.OperationalCount == 0:
		// All-protected evidence with no other residual work.
		return CategoryPreviewSkipped, PreviewReasonProtected, f.ProtectedCount
	case f.SkippedCount > 0 || f.RunningBlocked:
		reason = f.SkipReasonCode
		if reason == "" {
			reason = PreviewReasonApplicationRunning
		}
		return CategoryPreviewSkipped, reason, excluded
	case f.ProtectedCount > 0:
		// Protected plus residual diagnostics without safe candidates: still a
		// path-free safety exclusion rather than proven absence.
		return CategoryPreviewSkipped, PreviewReasonProtected, f.ProtectedCount
	case f.IncompleteCount > 0:
		reason = f.DiagnosticReason
		if reason == "" {
			reason = PreviewReasonInspectionLimit
		}
		return CategoryPreviewIncomplete, reason, f.IncompleteCount
	case f.OperationalCount > 0 || f.HasAnyDiagnostic:
		reason = f.DiagnosticReason
		if reason == "" {
			reason = PreviewReasonInspectionFailed
		}
		return CategoryPreviewFailed, reason, 0
	default:
		return CategoryPreviewEmpty, PreviewReasonEmpty, 0
	}
}

// previewPartialReason prioritizes path-free partial reason codes. Order matches
// the historical partialReasonCode helper (protected > incomplete > skip/running > failed).
func previewPartialReason(f categoryEvidenceFactors) string {
	if f.ProtectedCount > 0 {
		return PreviewReasonProtected
	}
	if f.IncompleteCount > 0 {
		if f.DiagnosticReason != "" {
			return f.DiagnosticReason
		}
		return PreviewReasonInspectionLimit
	}
	if f.SkippedCount > 0 || f.RunningBlocked {
		if f.SkipReasonCode != "" {
			return f.SkipReasonCode
		}
		return PreviewReasonApplicationRunning
	}
	if f.OperationalCount > 0 {
		if f.DiagnosticReason != "" {
			return f.DiagnosticReason
		}
		return PreviewReasonInspectionFailed
	}
	return PreviewReasonInspectionFailed
}

// mapExecutionOutcome maps evidence factors onto Clean execution category labels:
// empty, cleaned, partial, skipped, failed, canceled.
func mapExecutionOutcome(f categoryEvidenceFactors) CategoryExecutionState {
	if f.SuccessCount > 0 {
		if f.SkippedCount > 0 || f.Canceled || f.HasOperationalFail {
			return CategoryExecutionPartial
		}
		return CategoryExecutionCleaned
	}
	// Zero successes.
	if f.Canceled {
		return CategoryExecutionCanceled
	}
	if f.HasOperationalFail {
		return CategoryExecutionFailed
	}
	if f.HasSafetySkip || f.SkippedCount > 0 {
		return CategoryExecutionSkipped
	}
	return CategoryExecutionEmpty
}

// categoryEvidenceFromResolution extracts path-free factors from one category
// resolution. Candidate paths, protected paths, and raw diagnostic messages are
// discarded; only counts, flags, and stable reason codes survive.
func categoryEvidenceFromResolution(resolution CategoryResolution) categoryEvidenceFactors {
	safeCount := 0
	for range resolution.Candidates {
		safeCount++
	}
	for range resolution.OptInCandidates {
		safeCount++
	}

	incomplete, operational, diagnosticReason := classifyDiagnosticResiduals(resolution.Diagnostics)
	return categoryEvidenceFactors{
		SuccessCount:     safeCount,
		SkippedCount:     len(resolution.Skipped),
		ProtectedCount:   len(resolution.SuppressedProtectionPaths),
		IncompleteCount:  incomplete,
		OperationalCount: operational,
		HasAnyDiagnostic: len(resolution.Diagnostics) > 0,
		Canceled:         resolutionIndicatesCanceled(resolution),
		RunningBlocked:   runningApplicationBlocked(resolution.RunningStates),
		SkipReasonCode:   firstSkippedReasonCode(resolution.Skipped),
		DiagnosticReason: diagnosticReason,
	}
}

// classifyDiagnosticResiduals tallies incomplete and operational residual
// siblings from diagnostics. Cancellation is handled separately and is not
// counted as a sibling failure. Running-unknown is not operational failure.
func classifyDiagnosticResiduals(diagnostics []StructuredIssue) (incompleteCount, operationalCount int, reason string) {
	for _, diagnostic := range diagnostics {
		code := strings.TrimSpace(diagnostic.Code)
		switch {
		case classifyEvidenceIssueCode(code) == evidenceIssueCancel ||
			strings.Contains(strings.ToLower(diagnostic.Message), "context canceled"):
			// Cancellation is handled separately; do not count as sibling failure.
			continue
		case classifyEvidenceIssueCode(code) == evidenceIssueIncomplete:
			incompleteCount++
			if reason == "" {
				reason = code
			}
		case classifyEvidenceIssueCode(code) == evidenceIssueRunningUnknown:
			// Unknown process state is a safety skip, not operational failure.
			// Counted via running states / skip path instead when present.
			if reason == "" {
				reason = code
			}
		default:
			operationalCount++
			if reason == "" {
				if code != "" {
					reason = code
				} else {
					reason = PreviewReasonInspectionFailed
				}
			}
		}
	}
	return incompleteCount, operationalCount, reason
}

func resolutionIndicatesCanceled(resolution CategoryResolution) bool {
	for _, diagnostic := range resolution.Diagnostics {
		if classifyEvidenceIssueCode(diagnostic.Code) == evidenceIssueCancel {
			return true
		}
		if strings.Contains(strings.ToLower(diagnostic.Message), "context canceled") {
			return true
		}
	}
	return false
}

func runningApplicationBlocked(states []RunningApplicationState) bool {
	for _, state := range states {
		switch state.State {
		case RunningApplicationStateRunning, RunningApplicationStateUnknown:
			return true
		}
	}
	return false
}

func firstSkippedReasonCode(skipped []SkippedItem) string {
	for _, item := range skipped {
		code := strings.TrimSpace(item.Reason.Code)
		if code != "" {
			return code
		}
	}
	return ""
}
