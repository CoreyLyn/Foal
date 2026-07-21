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
	// PreviewReasonNVIDIAActivity is the stable skip reason surfaced when the
	// nvidia_installer_cache category is skipped because NVIDIA process/service
	// state is active or could not be determined. It equals nvidiaActivitySkipCode
	// so preview projection and the resolver share one code.
	PreviewReasonNVIDIAActivity = "nvidia_application_running"
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
	Identifier     string              `json:"identifier"`
	Label          string              `json:"label"`
	ReportCategory ReportCategory      `json:"report_category"`
	Eligibility    CategoryEligibility `json:"eligibility"`
	// PlannedAction is the catalog-owned deletion action for this executable
	// category. Non-executable categories never appear in the eager queue.
	PlannedAction        PlannedAction        `json:"planned_action,omitempty"`
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
// incomplete, or failed. Classification lives in the shared evidence module;
// this projection only attaches safe candidate totals and optional safety notes.
// Only safe candidate count/bytes, excluded sibling count, a stable reason
// code, and optional shared safety guidance survive.
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
		PlannedAction:  summary.PlannedAction,
	}

	var safeBytes int64
	for _, candidate := range resolution.Candidates {
		safeBytes += candidate.Bytes
	}
	for _, candidate := range resolution.OptInCandidates {
		safeBytes += candidate.Bytes
	}

	factors := categoryEvidenceFromResolution(resolution)
	state, reason, excluded := mapPreviewOutcome(factors)
	obs.State = state
	obs.ReasonCode = reason
	obs.ExcludedSiblingCount = excluded
	if factors.SuccessCount > 0 {
		obs.CandidateCount = factors.SuccessCount
		obs.Bytes = safeBytes
		obs.SafetyNote = categoryPreviewSafetyNote(summary.Identifier, resolution)
	}
	return obs
}

// categoryPreviewSafetyNote dispatches through the canonical private
// registration. Categories without a registered note remain silent.
func categoryPreviewSafetyNote(identifier string, resolution CategoryResolution) string {
	entry, ok := canonicalCategoryEntry(identifier)
	if !ok || entry.previewSafetyNote == nil {
		return ""
	}
	return entry.previewSafetyNote(resolution)
}

func applicationCachePreviewSafetyNote(resolution CategoryResolution) string {
	if resolutionIncludesCachedExtensionVSIXs(resolution) {
		return applicationCacheCachedExtensionVSIXsImpactNotice
	}
	return ""
}

func resolutionIncludesCachedExtensionVSIXs(resolution CategoryResolution) bool {
	for _, candidate := range resolution.Candidates {
		if isCachedExtensionVSIXsPath(candidate.Path) {
			return true
		}
	}
	for _, candidate := range resolution.OptInCandidates {
		if isCachedExtensionVSIXsPath(candidate.Path) {
			return true
		}
	}
	return false
}

// ErrEagerPreviewUnavailable is returned when RunEagerPreview refuses to scan
// because of a global pre-scan failure. Callers should prefer
// CheckEagerPreviewAvailability for the path-free payload.
var ErrEagerPreviewUnavailable = errors.New("clean unavailable")

// RunEagerPreview sequentially measures every scannable cleanup category through
// shared ResolveCategory (which projects resolveCategoryCore). It is a
// TUI-specific read-only operation: it never writes History or a detailed list,
// never calls a cleanup adapter, and never runs external Review suggestion
// probes. emit receives scanning then terminal observations for each queue
// entry; cooperative cancellation stops further work.
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
