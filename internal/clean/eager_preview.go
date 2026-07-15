package clean

import (
	"context"
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

// CategoryPreviewObservation is path-free evidence for one category in the
// Clean TUI eager preview. It never carries candidate paths, protected paths,
// private resolver details, or raw path-bearing operating-system errors.
type CategoryPreviewObservation struct {
	Identifier     string                `json:"identifier"`
	Label          string                `json:"label"`
	ReportCategory ReportCategory        `json:"report_category"`
	Eligibility    CategoryEligibility   `json:"eligibility"`
	State          CategoryPreviewState  `json:"state"`
	CandidateCount int                   `json:"candidate_count"`
	Bytes          int64                 `json:"bytes"`
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
// observation. Complete and empty carry measured evidence; every other terminal
// outcome is projected conservatively without paths or raw OS messages so the
// next ticket can refine the full outcome taxonomy without leaking privacy.
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

	count := 0
	var bytes int64
	for _, candidate := range resolution.Candidates {
		count++
		bytes += candidate.Bytes
	}
	for _, candidate := range resolution.OptInCandidates {
		count++
		bytes += candidate.Bytes
	}
	obs.CandidateCount = count
	obs.Bytes = bytes

	if count > 0 {
		// Ticket #188 distinguishes partial (safe siblings + exclusions). Until
		// then any positive candidate count is complete path-free evidence.
		obs.State = CategoryPreviewComplete
		return obs
	}

	if resolutionIndicatesCanceled(resolution) {
		obs.State = CategoryPreviewIncomplete
		return obs
	}
	if len(resolution.Skipped) > 0 || len(resolution.SuppressedProtectionPaths) > 0 ||
		runningApplicationBlocked(resolution.RunningStates) {
		obs.State = CategoryPreviewSkipped
		return obs
	}
	if len(resolution.Diagnostics) > 0 {
		obs.State = CategoryPreviewFailed
		return obs
	}
	obs.State = CategoryPreviewEmpty
	return obs
}

// RunEagerPreview sequentially measures every scannable cleanup category through
// shared ResolveCategory. It is a TUI-specific read-only operation: it never
// writes History or a detailed list, never calls a cleanup adapter, and never
// runs external Review suggestion probes. emit receives scanning then terminal
// observations for each queue entry; cooperative cancellation stops further work.
func RunEagerPreview(ctx context.Context, opts Options, emit func(CategoryPreviewObservation)) error {
	if ctx == nil {
		ctx = context.Background()
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
		if diagnostic.Code == "context_canceled" {
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
