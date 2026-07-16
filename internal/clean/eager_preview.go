package clean

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CategoryPreviewState is the path-free scan lifecycle for one eager-preview category.
// Waiting and scanning are non-terminal; the rest are terminal outcomes.
type CategoryPreviewState string

const (
	CategoryPreviewWaiting    CategoryPreviewState = "waiting"
	CategoryPreviewScanning   CategoryPreviewState = "scanning"
	CategoryPreviewComplete   CategoryPreviewState = "complete"
	CategoryPreviewEmpty      CategoryPreviewState = "empty"
	CategoryPreviewPartial    CategoryPreviewState = "partial"
	CategoryPreviewSkipped    CategoryPreviewState = "skipped"
	CategoryPreviewIncomplete CategoryPreviewState = "incomplete"
	CategoryPreviewFailed     CategoryPreviewState = "failed"
)

// Stable path-free reason codes retained for TUI diagnostics. Codes never embed
// candidate paths, protected paths, or raw operating-system messages.
const (
	PreviewReasonProtected              = "protected"
	PreviewReasonApplicationRunning     = "application_running"
	PreviewReasonDevToolRunning         = "dev_tool_running"
	PreviewReasonRunningStateUnknown    = "running_application_detection_unknown"
	PreviewReasonInspectionLimit        = "inspection_limit_exceeded"
	PreviewReasonContextCanceled        = "context_canceled"
	PreviewReasonInspectionFailed       = "inspection_failed"
	PreviewReasonEmpty                  = "empty"
	PreviewReasonProtectionConfigFailed = "protection_file_load_failed"
	PreviewReasonProtectionInvalidUTF8  = "protection_file_invalid_utf8"
)

// EagerPreviewUnavailable is a global pre-scan safety or configuration failure.
// It is path-free: Code is a stable identifier and Message never embeds paths.
type EagerPreviewUnavailable struct {
	Code    string
	Message string
}

func (e *EagerPreviewUnavailable) Error() string {
	if e == nil {
		return "clean unavailable"
	}
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// CategoryPreviewObservation is path-free evidence for one category in the
// Clean TUI eager preview. It never carries candidate paths, protected paths,
// private resolver details, or raw path-bearing operating-system errors.
type CategoryPreviewObservation struct {
	Identifier           string               `json:"identifier"`
	Label                string               `json:"label"`
	ReportCategory       ReportCategory       `json:"report_category"`
	Eligibility          CategoryEligibility  `json:"eligibility"`
	State                CategoryPreviewState `json:"state"`
	CandidateCount       int                  `json:"candidate_count"`
	Bytes                int64                `json:"bytes"`
	ExcludedSiblingCount int                  `json:"excluded_sibling_count,omitempty"`
	ReasonCode           string               `json:"reason_code,omitempty"`
	SafetyNote           string               `json:"safety_note,omitempty"`
}

// IsTerminalPreviewState reports whether state finishes measurement for one
// category. Waiting and scanning are non-terminal.
func IsTerminalPreviewState(state CategoryPreviewState) bool {
	switch state {
	case CategoryPreviewComplete, CategoryPreviewEmpty,
		CategoryPreviewPartial, CategoryPreviewSkipped,
		CategoryPreviewIncomplete, CategoryPreviewFailed:
		return true
	default:
		return false
	}
}

// SelectablePreviewOutcome reports whether a terminal outcome may remain in a
// cleanup selection. Empty, skipped, incomplete, and failed cannot; complete
// and partial can. Non-terminal waiting/scanning are provisionally selectable.
func SelectablePreviewOutcome(state CategoryPreviewState) bool {
	switch state {
	case CategoryPreviewWaiting, CategoryPreviewScanning,
		CategoryPreviewComplete, CategoryPreviewPartial:
		return true
	default:
		return false
	}
}

// EagerPreviewNoWorkState classifies a fully terminal scan with no current
// selection for the selection ticket. The three empty-authorization cases stay
// distinct so confirmation messaging never confuses deliberate empty selection
// with proven absence or diagnostic-only outcomes.
type EagerPreviewNoWorkState string

const (
	// EagerPreviewNoWorkNone means selection work remains possible (scan still
	// running, or selectable categories exist for the user to choose).
	EagerPreviewNoWorkNone EagerPreviewNoWorkState = ""
	// EagerPreviewNoWorkNeedSelection means at least one category is selectable
	// but the current selection is empty.
	EagerPreviewNoWorkNeedSelection EagerPreviewNoWorkState = "need_selection"
	// EagerPreviewNoWorkAllEmpty means every scannable category terminated empty.
	EagerPreviewNoWorkAllEmpty EagerPreviewNoWorkState = "all_empty"
	// EagerPreviewNoWorkDiagnostics means no category is selectable and at least
	// one terminated as skipped, incomplete, or failed.
	EagerPreviewNoWorkDiagnostics EagerPreviewNoWorkState = "diagnostics"
)

// ClassifyEagerPreviewNoWork distinguishes empty-selection, proven all-empty,
// and diagnostic-only finished scans. selectedCount is the current cleanup
// selection size (0 for this ticket's no-work classification). observations
// must include every scannable category's latest state.
func ClassifyEagerPreviewNoWork(observations []CategoryPreviewObservation, selectedCount int) EagerPreviewNoWorkState {
	if len(observations) == 0 {
		return EagerPreviewNoWorkNone
	}
	for _, obs := range observations {
		if !IsTerminalPreviewState(obs.State) {
			return EagerPreviewNoWorkNone
		}
	}
	if selectedCount > 0 {
		return EagerPreviewNoWorkNone
	}

	selectable := 0
	allEmpty := true
	diagnostic := false
	for _, obs := range observations {
		if SelectablePreviewOutcome(obs.State) {
			selectable++
		}
		if obs.State != CategoryPreviewEmpty {
			allEmpty = false
		}
		switch obs.State {
		case CategoryPreviewSkipped, CategoryPreviewIncomplete, CategoryPreviewFailed:
			diagnostic = true
		}
	}
	if selectable > 0 {
		return EagerPreviewNoWorkNeedSelection
	}
	if allEmpty {
		return EagerPreviewNoWorkAllEmpty
	}
	if diagnostic {
		return EagerPreviewNoWorkDiagnostics
	}
	// Terminal but nothing selectable and nothing diagnostic (unexpected);
	// treat as all-empty for fail-closed confirmation messaging.
	return EagerPreviewNoWorkAllEmpty
}

// CheckEagerPreviewAvailability returns a path-free global failure when Clean
// must not start category scanning. Callers open Clean unavailable instead of
// emitting one failed row per category.
func CheckEagerPreviewAvailability(opts Options) *EagerPreviewUnavailable {
	if opts.ProtectionLoadError == nil {
		return nil
	}
	code := strings.TrimSpace(opts.ProtectionLoadError.Code)
	if code == "" {
		code = PreviewReasonProtectionConfigFailed
	}
	return &EagerPreviewUnavailable{
		Code:    code,
		Message: pathFreeProtectionLoadMessage(code),
	}
}

func pathFreeProtectionLoadMessage(code string) string {
	switch code {
	case PreviewReasonProtectionInvalidUTF8:
		return "Protection configuration is not valid UTF-8. Fix the protection file and re-enter Clean."
	default:
		return "Protection configuration could not be loaded. Fix the protection file and re-enter Clean."
	}
}

// EagerPreviewQueue returns every canonical default and opt-in cleanup category
// in catalog registration order (display and scan order). Permission-boundary
// notices and review-only entries are excluded.
func EagerPreviewQueue() []CleanupCategorySummary {
	return EagerPreviewQueueFrom(CanonicalCleanupCategoryCatalog())
}

// EagerPreviewQueueFrom builds the scan queue from an arbitrary path-free
// catalog. Callers and tests use this so queue size tracks the catalog rather
// than a TUI-owned fixed list.
func EagerPreviewQueueFrom(catalog CleanupCategoryCatalog) []CleanupCategorySummary {
	definitions := catalog.Definitions()
	queue := make([]CleanupCategorySummary, 0, len(definitions))
	for _, definition := range definitions {
		switch definition.Eligibility {
		case CategoryEligibilityDefault, CategoryEligibilityOptIn:
			queue = append(queue, summaryFromDefinition(definition))
		}
	}
	return queue
}

// ProjectCategoryPreview maps one shared CategoryResolution into a path-free
// observation. Terminal outcomes are complete, partial, empty, skipped,
// incomplete, or failed. Only safe candidate count/bytes, excluded sibling
// count, a stable reason code, and optional shared safety guidance survive.
func ProjectCategoryPreview(resolution CategoryResolution) CategoryPreviewObservation {
	summary, ok := CanonicalCleanupCategoryCatalog().Summary(resolution.Identifier)
	if !ok {
		summary = CleanupCategorySummary{
			Identifier:     resolution.Identifier,
			Label:          resolution.Identifier,
			Eligibility:    resolution.Eligibility,
			ReportCategory: categoryReportGroup(resolution.Identifier),
		}
	}

	obs := CategoryPreviewObservation{
		Identifier:     summary.Identifier,
		Label:          summary.Label,
		ReportCategory: summary.ReportCategory,
		Eligibility:    summary.Eligibility,
	}

	safeCount := 0
	var safeBytes int64
	for _, candidate := range resolution.Candidates {
		safeCount++
		safeBytes += candidate.Bytes
	}
	for _, candidate := range resolution.OptInCandidates {
		safeCount++
		safeBytes += candidate.Bytes
	}

	protectedCount := len(resolution.SuppressedProtectionPaths)
	skippedCount := len(resolution.Skipped)
	canceled := resolutionIndicatesCanceled(resolution)
	runningBlocked := runningApplicationBlocked(resolution.RunningStates)
	incompleteSiblingCount, failedSiblingCount, diagnosticReason := classifyResolutionDiagnostics(resolution.Diagnostics)
	skipReason := firstSkippedReasonCode(resolution.Skipped)

	excluded := protectedCount + skippedCount + incompleteSiblingCount + failedSiblingCount
	if runningBlocked && skippedCount == 0 {
		// Running/unknown application blocked discovery without a path-backed
		// skipped item; still counts as one excluded sibling for partial.
		excluded++
	}

	switch {
	case safeCount > 0 && excluded > 0:
		obs.State = CategoryPreviewPartial
		obs.CandidateCount = safeCount
		obs.Bytes = safeBytes
		obs.ExcludedSiblingCount = excluded
		obs.ReasonCode = partialReasonCode(protectedCount, incompleteSiblingCount, failedSiblingCount, skippedCount, runningBlocked, skipReason, diagnosticReason)
		obs.SafetyNote = categoryPreviewSafetyNote(summary.Identifier, true)
	case safeCount > 0:
		obs.State = CategoryPreviewComplete
		obs.CandidateCount = safeCount
		obs.Bytes = safeBytes
		obs.SafetyNote = categoryPreviewSafetyNote(summary.Identifier, true)
	case canceled:
		obs.State = CategoryPreviewIncomplete
		obs.ReasonCode = PreviewReasonContextCanceled
		// Incomplete contributes no invent reclaimable bytes.
	case incompleteSiblingCount > 0 && protectedCount == 0 && skippedCount == 0 && !runningBlocked:
		obs.State = CategoryPreviewIncomplete
		obs.ExcludedSiblingCount = incompleteSiblingCount
		if diagnosticReason != "" {
			obs.ReasonCode = diagnosticReason
		} else {
			obs.ReasonCode = PreviewReasonInspectionLimit
		}
	case protectedCount > 0 && skippedCount == 0 && !runningBlocked && incompleteSiblingCount == 0 && failedSiblingCount == 0:
		// All-protected evidence with no other residual work.
		obs.State = CategoryPreviewSkipped
		obs.ExcludedSiblingCount = protectedCount
		obs.ReasonCode = PreviewReasonProtected
	case skippedCount > 0 || runningBlocked:
		obs.State = CategoryPreviewSkipped
		obs.ExcludedSiblingCount = excluded
		if skipReason != "" {
			obs.ReasonCode = skipReason
		} else if runningBlocked {
			obs.ReasonCode = PreviewReasonApplicationRunning
		} else {
			obs.ReasonCode = PreviewReasonApplicationRunning
		}
	case protectedCount > 0:
		// Protected plus residual diagnostics without safe candidates: still a
		// path-free safety exclusion rather than proven absence.
		obs.State = CategoryPreviewSkipped
		obs.ExcludedSiblingCount = protectedCount
		obs.ReasonCode = PreviewReasonProtected
	case incompleteSiblingCount > 0:
		obs.State = CategoryPreviewIncomplete
		obs.ExcludedSiblingCount = incompleteSiblingCount
		if diagnosticReason != "" {
			obs.ReasonCode = diagnosticReason
		} else {
			obs.ReasonCode = PreviewReasonInspectionLimit
		}
	case failedSiblingCount > 0 || len(resolution.Diagnostics) > 0:
		obs.State = CategoryPreviewFailed
		if diagnosticReason != "" {
			obs.ReasonCode = diagnosticReason
		} else {
			obs.ReasonCode = PreviewReasonInspectionFailed
		}
	default:
		obs.State = CategoryPreviewEmpty
		obs.ReasonCode = PreviewReasonEmpty
	}
	return obs
}

func partialReasonCode(protected, incomplete, failed, skipped int, running bool, skipReason, diagnosticReason string) string {
	if protected > 0 {
		return PreviewReasonProtected
	}
	if incomplete > 0 {
		if diagnosticReason != "" {
			return diagnosticReason
		}
		return PreviewReasonInspectionLimit
	}
	if skipped > 0 || running {
		if skipReason != "" {
			return skipReason
		}
		return PreviewReasonApplicationRunning
	}
	if failed > 0 {
		if diagnosticReason != "" {
			return diagnosticReason
		}
		return PreviewReasonInspectionFailed
	}
	return PreviewReasonInspectionFailed
}

func classifyResolutionDiagnostics(diagnostics []StructuredIssue) (incompleteCount, failedCount int, reason string) {
	for _, diagnostic := range diagnostics {
		code := strings.TrimSpace(diagnostic.Code)
		switch {
		case code == PreviewReasonContextCanceled || strings.Contains(strings.ToLower(diagnostic.Message), "context canceled"):
			// Cancellation is handled separately; do not count as sibling failure.
			continue
		case code == PreviewReasonInspectionLimit || code == "reparse_point":
			incompleteCount++
			if reason == "" {
				reason = code
			}
		case code == runningApplicationDetectionIssueCode:
			// Unknown process state is a safety skip, not operational failure.
			// Counted via running states / skip path instead when present.
			if reason == "" {
				reason = code
			}
		default:
			failedCount++
			if reason == "" {
				if code != "" {
					reason = code
				} else {
					reason = PreviewReasonInspectionFailed
				}
			}
		}
	}
	return incompleteCount, failedCount, reason
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

// categoryPreviewSafetyNote returns optional shared impact vocabulary for
// categories that already publish notices on the dry-run surface. Empty when
// there is no shared note or no safe candidates to associate it with.
func categoryPreviewSafetyNote(identifier string, hasSafeCandidates bool) string {
	if !hasSafeCandidates {
		return ""
	}
	switch identifier {
	case DevCacheCategoryUV:
		return uvCacheOptInImpactNotice
	case DevCacheCategoryBun:
		return bunCacheOptInImpactNotice
	case DevCacheCategoryPlaywright:
		return playwrightBrowsersOptInImpactNotice
	case DevCacheCategoryPuppeteerBrowsers:
		return puppeteerBrowsersOptInImpactNotice
	case DevCacheCategoryElectron:
		return electronCacheOptInImpactNotice
	case DevCacheCategoryJetBrainsIDECaches:
		return jetbrainsIDECachesOptInImpactNotice
	default:
		return ""
	}
}

// ErrEagerPreviewUnavailable is returned when RunEagerPreview refuses to scan
// because of a global pre-scan failure. Callers should prefer
// CheckEagerPreviewAvailability for the path-free payload.
var ErrEagerPreviewUnavailable = errors.New("clean unavailable")

// RunEagerPreview sequentially measures every scannable cleanup category through
// shared ResolveCategory. It is a TUI-specific read-only operation: it never
// writes History or a detailed list, never calls a cleanup adapter, and never
// runs external Review suggestion probes. emit receives scanning then terminal
// observations for each queue entry; cooperative cancellation stops further work.
//
// A global protection-configuration failure returns without emitting category
// rows so the TUI can open Clean unavailable.
func RunEagerPreview(ctx context.Context, opts Options, emit func(CategoryPreviewObservation)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if unavailable := CheckEagerPreviewAvailability(opts); unavailable != nil {
		return fmt.Errorf("%w: %s", ErrEagerPreviewUnavailable, unavailable.Code)
	}
	// Strip side-effect and execution collaborators; this surface is measurement only.
	opts.HistoryRecorder = nil
	opts.DetailedListDir = ""
	opts.RecycleBinAdapter = nil
	opts.ProgressReporter = nil
	// Review suggestion probes are intentionally not invoked: eager preview does
	// not surface Review suggestions and must not pay their latency.

	for _, summary := range EagerPreviewQueue() {
		if err := ctx.Err(); err != nil {
			return err
		}

		if emit != nil {
			emit(CategoryPreviewObservation{
				Identifier:     summary.Identifier,
				Label:          summary.Label,
				ReportCategory: summary.ReportCategory,
				Eligibility:    summary.Eligibility,
				State:          CategoryPreviewScanning,
			})
		}

		resolution, err := ResolveCategory(ctx, opts, summary.Identifier)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if emit != nil {
				emit(CategoryPreviewObservation{
					Identifier:     summary.Identifier,
					Label:          summary.Label,
					ReportCategory: summary.ReportCategory,
					Eligibility:    summary.Eligibility,
					State:          CategoryPreviewFailed,
					ReasonCode:     PreviewReasonInspectionFailed,
				})
			}
			continue
		}

		if emit != nil {
			emit(ProjectCategoryPreview(resolution))
		}
	}
	return ctx.Err()
}

func resolutionIndicatesCanceled(resolution CategoryResolution) bool {
	for _, diagnostic := range resolution.Diagnostics {
		if diagnostic.Code == PreviewReasonContextCanceled || diagnostic.Code == "context_canceled" {
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
