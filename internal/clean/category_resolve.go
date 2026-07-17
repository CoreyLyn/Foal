package clean

import (
	"context"
	"fmt"
)

// categoryCoreResult is the internal fresh resolution of exactly one canonical
// cleanup category after running-application gates, protection suppression, and
// measurement. Public surfaces project this artifact; it never authorizes
// execution and never writes History.
//
// Dual projections (when both apply):
//   - OptInCandidates: executable-shaped reclaimable work for opt-in / execute
//   - Opportunities + IncompleteInspections: dry-run review-only shapes
//
// Developer-cache categories fill OptInCandidates only. Defaults fill
// DefaultCandidates only. Review suggestion tool probes are never part of this
// core (ADR-0004: dry-run Review suggestions only).
type categoryCoreResult struct {
	Identifier  string
	Eligibility CategoryEligibility

	DefaultCandidates []CandidatePreview
	OptInCandidates   []OptInCandidate

	// Opportunities and IncompleteInspections are review-capable shapes for
	// existence opportunities, browser aggregate discovery, and Application
	// cache roots. Empty for developer caches and defaults.
	Opportunities         []Opportunity
	IncompleteInspections []IncompleteOpportunityInspection

	Skipped                   []SkippedItem
	RunningStates             []RunningApplicationState
	Diagnostics               []StructuredIssue
	SuppressedProtectionPaths []string
}

// resolveCategoryCore is the single deep module for one-category fresh
// resolution. Call-scope differs by surface (selected-only vs full scannable
// queue); the category work is always this core.
func resolveCategoryCore(ctx context.Context, opts Options, identifier string) (categoryCoreResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := CompileExactCategoryPlan([]string{identifier})
	if err != nil {
		return categoryCoreResult{}, err
	}
	if len(plan.Categories) != 1 {
		return categoryCoreResult{}, fmt.Errorf("category %q is not independently resolvable", identifier)
	}
	canonicalID := plan.Categories[0]
	entry, ok := canonicalCategoryEntry(canonicalID)
	if !ok || entry.resolver == nil {
		return categoryCoreResult{}, fmt.Errorf("category %q is not independently resolvable", identifier)
	}

	core := categoryCoreResult{
		Identifier:  canonicalID,
		Eligibility: entry.definition.Eligibility,
	}
	entry.resolver.resolve(ctx, opts, canonicalID, &core)
	return core, nil
}

func resolveDefaultCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	result := newCleanResultSkeleton("dry_run", opts)
	appendDefaultCandidates(ctx, opts, map[string]bool{category: true}, true, &result)
	core.DefaultCandidates = append(core.DefaultCandidates, result.Candidates...)
	core.Skipped = append(core.Skipped, result.Skipped...)
	core.Diagnostics = append(core.Diagnostics, result.Errors...)
}

// projectCategoryCoreToResolution maps the internal core to the public
// CategoryResolution used by eager preview and ResolveCategory callers.
func projectCategoryCoreToResolution(core categoryCoreResult) CategoryResolution {
	return CategoryResolution{
		Identifier:                core.Identifier,
		Eligibility:               core.Eligibility,
		Candidates:                append([]CandidatePreview(nil), core.DefaultCandidates...),
		OptInCandidates:           append([]OptInCandidate(nil), core.OptInCandidates...),
		Skipped:                   append([]SkippedItem(nil), core.Skipped...),
		RunningStates:             append([]RunningApplicationState(nil), core.RunningStates...),
		Diagnostics:               append([]StructuredIssue(nil), core.Diagnostics...),
		SuppressedProtectionPaths: append([]string(nil), core.SuppressedProtectionPaths...),
	}
}

// mergeCategoryCoreIntoOptInResolution projects executable-shaped core fields
// into a multi-category opt-in resolution. Opportunities are intentionally
// omitted so review-only shapes never become executable.
func mergeCategoryCoreIntoOptInResolution(res *optInResolution, core categoryCoreResult) {
	if res == nil {
		return
	}
	res.candidates = append(res.candidates, core.OptInCandidates...)
	res.skipped = append(res.skipped, core.Skipped...)
	res.runningStates = mergeRunningApplicationStates(res.runningStates, core.RunningStates...)
	res.diagnostics = append(res.diagnostics, core.Diagnostics...)
	res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, core.SuppressedProtectionPaths...)
}

// categoryReviewProjection controls dry-run review-only projection of a core.
type categoryReviewProjection struct {
	// EmitUnknownDiagnostics adds recoverable unknown-state diagnostics for
	// RunningApplicationStateUnknown entries (browser / Application cache
	// review contract). Opt-in projection never uses this.
	EmitUnknownDiagnostics bool
	// ApplyProtectionSuppress rewrites Result.ProtectionRules when the core
	// recorded suppressed protection paths. Browser and Application cache
	// review apply this; existence-opportunity review historically does not.
	ApplyProtectionSuppress bool
}

// projectCategoryCoreToReview maps one non-opted-in category core onto dry-run
// review surfaces: Opportunities and IncompleteOpportunityInspections only.
// OptInCandidates are never written — review-only results stay non-executable.
func projectCategoryCoreToReview(result *Result, core categoryCoreResult, proj categoryReviewProjection) {
	if result == nil {
		return
	}
	for _, state := range core.RunningStates {
		replaceRunningApplicationState(result, state)
		if proj.EmitUnknownDiagnostics && state.State == RunningApplicationStateUnknown {
			result.Errors = append(result.Errors, runningApplicationUnknownIssue(state))
		}
	}
	// Incomplete reasons are already in core.Diagnostics; project inspections
	// separately then append every diagnostic once to Errors.
	result.IncompleteOpportunityInspections = append(result.IncompleteOpportunityInspections, core.IncompleteInspections...)
	result.Opportunities = append(result.Opportunities, core.Opportunities...)
	for _, diagnostic := range core.Diagnostics {
		// When unknown-state diagnostics are emitted from RunningStates, skip
		// the matching post-gate copies already stored on the core so review
		// does not double-count the same recoverable issue.
		if proj.EmitUnknownDiagnostics && diagnostic.Code == runningApplicationDetectionIssueCode {
			continue
		}
		result.Errors = append(result.Errors, diagnostic)
	}
	if proj.ApplyProtectionSuppress && len(core.SuppressedProtectionPaths) > 0 {
		suppressProtectionRules(result, core.SuppressedProtectionPaths)
	}
}

// orderedPlanCategories returns plan-enabled identifiers in catalog order so
// multi-category merges stay deterministic.
func orderedPlanCategories(plan map[string]bool) []string {
	if len(plan) == 0 {
		return nil
	}
	out := make([]string, 0, len(plan))
	for _, entry := range canonicalCategoryEntries {
		id := entry.definition.Identifier
		if plan[id] {
			out = append(out, id)
		}
	}
	return out
}
