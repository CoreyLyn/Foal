package clean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SelectionMode distinguishes CLI additive planning from exact TUI planning.
type SelectionMode string

const (
	// SelectionModeAdditive always includes default categories and adds
	// explicitly opted-in categories (CLI dry-run / execute contract).
	SelectionModeAdditive SelectionMode = "additive"
	// SelectionModeExact authorizes only the listed canonical categories.
	// Omitted default categories remain omitted (TUI confirmation contract).
	SelectionModeExact SelectionMode = "exact"
)

// CategoryPlan is a compiled, path-free set of canonical cleanup categories
// for one Clean preview or execution run. It never carries candidate paths.
type CategoryPlan struct {
	Mode SelectionMode
	// Categories are unique canonical identifiers in stable catalog order.
	// Additive plans always list every default category first (catalog order),
	// then opted-in categories. Exact plans list only accepted identifiers.
	Categories []string
}

// CategoryPlanRejection describes one rejected exact-plan identifier.
type CategoryPlanRejection struct {
	Identifier string
	// Reason is a stable rejection code: unknown, alias, group,
	// permission_boundary, review_only, path_bearing, or empty.
	// Duplicates are normalized away rather than rejected.
	Reason string
}

func (r CategoryPlanRejection) Error() string {
	return fmt.Sprintf("invalid category identifier %q (%s)", r.Identifier, r.Reason)
}

// CategoryPlanError aggregates exact-plan rejections.
type CategoryPlanError struct {
	Rejections []CategoryPlanRejection
}

func (e *CategoryPlanError) Error() string {
	if e == nil || len(e.Rejections) == 0 {
		return "invalid category plan"
	}
	parts := make([]string, 0, len(e.Rejections))
	for _, rejection := range e.Rejections {
		parts = append(parts, rejection.Error())
	}
	return strings.Join(parts, "; ")
}

// CategoryResolution is the fresh resolution of exactly one canonical category.
// It may carry path-bearing discovery evidence for shared projection; it is not
// an execution authorization and never writes history.
type CategoryResolution struct {
	Identifier      string
	Eligibility     CategoryEligibility
	Candidates      []CandidatePreview
	OptInCandidates []OptInCandidate
	Skipped         []SkippedItem
	RunningStates   []RunningApplicationState
	Diagnostics     []StructuredIssue
	// SuppressedProtectionPaths are protection-rule paths suppressed during
	// opt-in resolution (same meaning as optInResolution).
	SuppressedProtectionPaths []string
}

// CompileAdditiveCategoryPlan builds the CLI-compatible plan: every default
// category plus opt-in tokens resolved through NormalizedOptInSet (aliases and
// group tokens such as dev-caches / all are accepted).
func CompileAdditiveCategoryPlan(optInTokens []string) (CategoryPlan, []string) {
	enabled, invalid, _ := NormalizedOptInSet(optInTokens)
	categories := make([]string, 0, len(canonicalCategoryEntries))
	for _, entry := range canonicalCategoryEntries {
		switch entry.definition.Eligibility {
		case CategoryEligibilityDefault:
			categories = append(categories, entry.definition.Identifier)
		case CategoryEligibilityOptIn:
			if enabled[entry.definition.Identifier] {
				categories = append(categories, entry.definition.Identifier)
			}
		}
	}
	return CategoryPlan{Mode: SelectionModeAdditive, Categories: categories}, invalid
}

// CompileExactCategoryPlan builds an exact plan from canonical default and
// opt-in identifiers only. Unknown identifiers, catalog aliases, aggregate
// group tokens, permission-boundary notices, review-only entries, empty
// tokens, and path-bearing inputs are rejected. Duplicates are dropped while
// preserving membership; the resulting Categories list is always sorted into
// deterministic catalog order.
func CompileExactCategoryPlan(identifiers []string) (CategoryPlan, error) {
	selected := make(map[string]bool, len(identifiers))
	var rejections []CategoryPlanRejection
	seenInput := make(map[string]bool, len(identifiers))

	for _, raw := range identifiers {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			rejections = append(rejections, CategoryPlanRejection{Identifier: raw, Reason: "empty"})
			continue
		}
		if isPathBearingCategoryInput(trimmed) {
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "path_bearing"})
			continue
		}
		normalized := strings.ToLower(trimmed)
		if seenInput[normalized] {
			// Explicit contract: duplicates are normalized away, not rejected.
			continue
		}
		seenInput[normalized] = true

		if isGroupCategoryToken(normalized) {
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "group"})
			continue
		}

		summary, foundAsCanonical, foundAsAlias := lookupExactCategoryIdentifier(normalized)
		if foundAsAlias {
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "alias"})
			continue
		}
		if !foundAsCanonical {
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "unknown"})
			continue
		}
		switch summary.Eligibility {
		case CategoryEligibilityDefault, CategoryEligibilityOptIn:
			selected[summary.Identifier] = true
		case CategoryEligibilityPermissionBoundary:
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "permission_boundary"})
		case CategoryEligibilityReviewOnly:
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "review_only"})
		default:
			rejections = append(rejections, CategoryPlanRejection{Identifier: trimmed, Reason: "unknown"})
		}
	}

	if len(rejections) > 0 {
		return CategoryPlan{}, &CategoryPlanError{Rejections: rejections}
	}

	categories := make([]string, 0, len(selected))
	for _, entry := range canonicalCategoryEntries {
		if selected[entry.definition.Identifier] {
			categories = append(categories, entry.definition.Identifier)
		}
	}
	return CategoryPlan{Mode: SelectionModeExact, Categories: categories}, nil
}

// ScannableCategoryIDs returns every canonical default and opt-in cleanup
// category identifier in catalog registration order. Permission-boundary and
// review-only entries are omitted.
func ScannableCategoryIDs() []string {
	ids := make([]string, 0, len(canonicalCategoryEntries))
	for _, entry := range canonicalCategoryEntries {
		switch entry.definition.Eligibility {
		case CategoryEligibilityDefault, CategoryEligibilityOptIn:
			ids = append(ids, entry.definition.Identifier)
		}
	}
	return ids
}

// ResolveCategory resolves exactly one canonical default or opt-in category
// without scanning unrelated categories. It reuses shared protection, running-
// application gates, env/default-root resolution, and measurement policy.
// Preview paths returned here never authorize execution.
func ResolveCategory(ctx context.Context, opts Options, identifier string) (CategoryResolution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := CompileExactCategoryPlan([]string{identifier})
	if err != nil {
		return CategoryResolution{}, err
	}
	if len(plan.Categories) != 1 {
		return CategoryResolution{}, fmt.Errorf("category %q is not independently resolvable", identifier)
	}
	canonicalID := plan.Categories[0]
	summary, _ := canonicalCleanupCategoryCatalog.Summary(canonicalID)

	resolution := CategoryResolution{
		Identifier:  canonicalID,
		Eligibility: summary.Eligibility,
	}

	switch summary.Eligibility {
	case CategoryEligibilityDefault:
		result := newCleanResultSkeleton("dry_run", opts)
		appendDefaultCandidates(ctx, opts, map[string]bool{canonicalID: true}, true, &result)
		resolution.Candidates = append(resolution.Candidates, result.Candidates...)
		resolution.Skipped = append(resolution.Skipped, result.Skipped...)
		resolution.Diagnostics = append(resolution.Diagnostics, result.Errors...)
	case CategoryEligibilityOptIn:
		optIn := resolveOptInCandidates(ctx, opts, map[string]bool{canonicalID: true})
		resolution.OptInCandidates = append(resolution.OptInCandidates, optIn.candidates...)
		resolution.Skipped = append(resolution.Skipped, optIn.skipped...)
		resolution.RunningStates = mergeRunningApplicationStates(resolution.RunningStates, optIn.runningStates...)
		resolution.Diagnostics = append(resolution.Diagnostics, optIn.diagnostics...)
		resolution.SuppressedProtectionPaths = append(resolution.SuppressedProtectionPaths, optIn.suppressedProtectionPaths...)
	default:
		return CategoryResolution{}, fmt.Errorf("category %q is not independently resolvable", identifier)
	}
	return resolution, nil
}

// effectiveCategoryPlan returns opts.Plan when set; otherwise compiles the
// additive CLI plan from OptIn so existing callers stay compatible.
func effectiveCategoryPlan(opts Options) CategoryPlan {
	if opts.Plan != nil {
		return *opts.Plan
	}
	plan, _ := CompileAdditiveCategoryPlan(opts.OptIn)
	return plan
}

func planDefaultSet(plan CategoryPlan) map[string]bool {
	selected := make(map[string]bool)
	for _, id := range plan.Categories {
		summary, ok := canonicalCleanupCategoryCatalog.Summary(id)
		if ok && summary.Eligibility == CategoryEligibilityDefault {
			selected[id] = true
		}
	}
	return selected
}

func planOptInSet(plan CategoryPlan) map[string]bool {
	selected := make(map[string]bool)
	for _, id := range plan.Categories {
		summary, ok := canonicalCleanupCategoryCatalog.Summary(id)
		if ok && summary.Eligibility == CategoryEligibilityOptIn {
			selected[id] = true
		}
	}
	return selected
}

func lookupExactCategoryIdentifier(normalized string) (summary CleanupCategorySummary, asCanonical bool, asAlias bool) {
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Identifier == normalized {
			return summaryFromDefinition(entry.definition), true, false
		}
	}
	// Catalog Summary resolves aliases; reject those for exact planning.
	if summary, ok := canonicalCleanupCategoryCatalog.Summary(normalized); ok && summary.Identifier != normalized {
		return summary, false, true
	}
	return CleanupCategorySummary{}, false, false
}

func isGroupCategoryToken(normalized string) bool {
	return normalized == "all" || normalized == DevCacheCategoryAll
}

func isPathBearingCategoryInput(value string) bool {
	if strings.ContainsAny(value, `/\`) {
		return true
	}
	// Windows drive-qualified paths such as C:temp or C:\Windows.
	if len(value) >= 2 && value[1] == ':' {
		return true
	}
	return false
}

func newCleanResultSkeleton(mode string, opts Options) Result {
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}
	status := "preview"
	if mode == "execute" {
		status = "ok"
	}
	return Result{
		Status:                           status,
		Mode:                             mode,
		DefaultRuleCatalog:               defaultRuleSummaries(rules),
		ProtectionRules:                  protectionRules(opts.Validator),
		ProtectionDiagnostics:            append([]ProtectionDiagnostic(nil), opts.ProtectionDiagnostics...),
		Candidates:                       []CandidatePreview{},
		Deleted:                          []DeletedItem{},
		Failed:                           []FailedItem{},
		Skipped:                          []SkippedItem{},
		Errors:                           []StructuredIssue{},
		Opportunities:                    []Opportunity{},
		IncompleteOpportunityInspections: []IncompleteOpportunityInspection{},
		ReviewSuggestions:                []ReviewSuggestion{},
		RunningApplications:              []RunningApplicationState{},
		OptInCandidates:                  []OptInCandidate{},
	}
}

// appendDefaultCandidates scans default-enabled rules. When filterDefaults is
// true, only rules whose IDs appear in selectedDefaults are scanned (exact
// plans). When false, every DefaultEnabled rule is scanned (additive CLI /
// test rule catalogs). filterDefaults with an empty selectedDefaults scans
// nothing.
func appendDefaultCandidates(ctx context.Context, opts Options, selectedDefaults map[string]bool, filterDefaults bool, result *Result) {
	if filterDefaults && len(selectedDefaults) == 0 {
		return
	}
	rules := opts.Rules
	if len(rules) == 0 {
		rules = DefaultRuleCatalog()
	}
	for _, rule := range rules {
		if !rule.DefaultEnabled {
			continue
		}
		if filterDefaults && !selectedDefaults[rule.ID] {
			continue
		}
		for _, path := range rule.CandidatePaths {
			previewCandidate(ctx, opts, path, rule.ID, result)
		}
		for _, root := range rule.Roots {
			select {
			case <-ctx.Done():
				result.Errors = append(result.Errors, issue("context_canceled", ctx.Err().Error(), true, root, rule.ID))
				return
			default:
			}

			entries, err := os.ReadDir(root)
			if err != nil {
				result.Errors = append(result.Errors, issue(classifyError(err), err.Error(), true, root, rule.ID))
				continue
			}
			for _, entry := range entries {
				if !matchesRuleName(entry.Name(), rule.CandidateNamePrefixes) {
					continue
				}
				path := filepath.Join(root, entry.Name())
				previewCandidate(ctx, opts, path, rule.ID, result)
			}
		}
	}
}
