package clean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// DryRun previews cleanup candidates for the effective category plan without
// mutating the filesystem. It shares category resolution with Execute but adds
// dry-run-only review projection, detailed list, and opportunity history.
func DryRun(ctx context.Context, opts Options) Result {
	return dryRun(ctx, opts)
}

func dryRun(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	if opts.ProtectionLoadError != nil {
		return protectionLoadFailure("dry_run", opts, start)
	}
	categoryPlan := effectiveCategoryPlan(opts)
	result := newCleanResultSkeleton("dry_run", opts)
	exactDefaults := categoryPlan.Mode == SelectionModeExact
	appendDefaultCandidates(ctx, opts, planDefaultSet(categoryPlan), exactDefaults, &result)
	if ctx.Err() != nil {
		result.ElapsedMS = time.Since(start).Milliseconds()
		result.Totals = totals(result)
		return result
	}

	// Resolve opt-in candidates once; both modes share this resolution.
	optInPlan := planOptInSet(categoryPlan)
	resolution := resolveOptInCandidates(ctx, opts, optInPlan)
	applyOptInResolution(&result, resolution)

	// Review projection is CLI additive dry-run only: non-opted-in opportunities,
	// suggestions, and browser cache review. Exact plans skip the review fan-out
	// so unselected categories are not implicitly resolved.
	if categoryPlan.Mode != SelectionModeExact {
		applyOptInReviewProjection(ctx, opts, &result, optInPlan, resolution)
	}

	result.ElapsedMS = time.Since(start).Milliseconds()
	result.Totals = totals(result)
	if opts.DetailedListDir != "" {
		path, err := writeDetailedCandidateList(opts.DetailedListDir, result, time.Now())
		if err != nil {
			result.Errors = append(result.Errors, issue("detailed_list_write_failed", err.Error(), true, opts.DetailedListDir, ""))
			result.Totals = totals(result)
		} else {
			result.DetailedListPath = path
		}
	}
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

// applyOptInResolution writes the resolved opt-in candidates and gating
// artifacts onto the result. Both dry-run and execute call this so preview and
// execute surface the same candidates, skips, running states, and diagnostics.
// Execute uses a tighter inline path for mutation candidates; dry-run uses this
// helper to populate OptInCandidates on the public Result.
func applyOptInResolution(result *Result, res optInResolution) {
	result.OptInCandidates = append(result.OptInCandidates, res.candidates...)
	result.Skipped = append(result.Skipped, res.skipped...)
	result.RunningApplications = mergeRunningApplicationStates(result.RunningApplications, res.runningStates...)
	result.Errors = append(result.Errors, res.diagnostics...)
	if len(res.suppressedProtectionPaths) > 0 {
		suppressProtectionRules(result, res.suppressedProtectionPaths)
	}
}

// applyOptInReviewProjection is the dry-run-only review surface for categories
// the user did not opt in. Non-opted-in existence opportunities, browser cache,
// and Application cache categories reuse resolveCategoryCore and project only
// review shapes (Opportunities / incompletes) — never OptInCandidates.
// Review suggestions still use tool-query probes (ADR-0004) and remain outside
// the single-category core.
func applyOptInReviewProjection(ctx context.Context, opts Options, result *Result, plan map[string]bool, resolution optInResolution) {
	// Existence opportunities (non-browser, non-application-cache).
	for _, category := range optedOutOpportunityCategories(plan) {
		core, err := resolveCategoryCore(ctx, opts, category)
		if err != nil {
			continue
		}
		projectCategoryCoreToReview(result, core, categoryReviewProjection{
			// No protection-rule display rewrite for existence review.
			ApplyProtectionSuppress: false,
		})
	}

	optedInDevCacheIdentities := make(map[string]bool)
	for _, c := range resolution.candidates {
		if isDevCacheCategory(c.Category) {
			optedInDevCacheIdentities[pathsafe.NormalizePathForIdentity(c.Path)] = true
		}
	}
	discoverSuggestions := opts.DiscoverReviewSuggestions
	if discoverSuggestions == nil {
		discoverSuggestions = DiscoverReviewSuggestions
	}
	for _, suggestion := range discoverSuggestions(ctx) {
		if suggestion.CachePath != "" && opts.Validator.IsUserProtected(suggestion.CachePath) {
			continue
		}
		if suggestion.CachePath != "" && optedInDevCacheIdentities[pathsafe.NormalizePathForIdentity(suggestion.CachePath)] {
			continue
		}
		result.ReviewSuggestions = append(result.ReviewSuggestions, suggestion)
	}

	gatedReview := categoryReviewProjection{
		EmitUnknownDiagnostics:  true,
		ApplyProtectionSuppress: true,
	}
	if !plan[OpportunityCategoryBrowserCache] && opts.DetectRunningApplications != nil {
		if core, err := resolveCategoryCore(ctx, opts, OpportunityCategoryBrowserCache); err == nil {
			projectCategoryCoreToReview(result, core, gatedReview)
		}
	}
	if opts.DetectRunningApplications != nil {
		for _, category := range applicationCacheCategoryIDs() {
			if plan[category] {
				continue
			}
			core, err := resolveCategoryCore(ctx, opts, category)
			if err != nil {
				continue
			}
			projectCategoryCoreToReview(result, core, gatedReview)
		}
	}
}

func writeDetailedCandidateList(dir string, result Result, at time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("clean-dry-run-%s-detailed-candidates.txt", at.UTC().Format("20060102T150405.000000000Z")))
	var builder strings.Builder
	builder.WriteString("Foal clean detailed candidate list\n")
	builder.WriteString("Companion file only. This is not an execution manifest; foal clean --execute performs a fresh scan and path-safety validation before using the Recycle Bin.\n\n")

	builder.WriteString("Default candidates\n")
	if len(result.Candidates) == 0 {
		builder.WriteString("  No default candidates found.\n")
	} else {
		for _, candidate := range result.Candidates {
			builder.WriteString(fmt.Sprintf("  path: %s\n", candidate.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", candidate.Rule))
			builder.WriteString(fmt.Sprintf("    bytes: %d\n", candidate.Bytes))
			builder.WriteString(fmt.Sprintf("    planned action: %s (%s)\n", plannedActionLabel(candidate.PlannedAction), candidate.PlannedAction))
		}
	}

	builder.WriteString("\nSkipped-by-default opportunities\n")
	builder.WriteString("  This section is review-only and is not an execution manifest. Opportunity bytes are not counted as Potential space.\n")
	if len(result.Opportunities) == 0 {
		builder.WriteString("  No skipped-by-default opportunities found.\n")
	} else {
		for _, opportunity := range result.Opportunities {
			builder.WriteString(fmt.Sprintf("  path: %s\n", opportunity.Path))
			builder.WriteString(fmt.Sprintf("    category: %s\n", normalizedOpportunityCategory(opportunity.Category)))
			builder.WriteString(fmt.Sprintf("    bytes: %d\n", opportunity.Bytes))
			if normalizedOpportunityCategory(opportunity.Category) == OpportunityCategoryUserTemp {
				builder.WriteString(fmt.Sprintf("    latest modified: %s\n", opportunity.LatestModifiedAt.UTC().Format(time.RFC3339)))
				builder.WriteString(fmt.Sprintf("    idle days: %d\n", opportunity.IdleDays))
			}
			if opportunity.BrowserCache != nil {
				builder.WriteString(fmt.Sprintf("    browser: %s\n", applicationDisplayName(opportunity.BrowserCache.Browser)))
				builder.WriteString(fmt.Sprintf("    profiles: %d\n", opportunity.BrowserCache.ProfileCount))
				for _, profile := range opportunity.BrowserCache.Profiles {
					builder.WriteString(fmt.Sprintf("    profile: %s\n", profile.ID))
					if profile.Name != "" {
						builder.WriteString(fmt.Sprintf("      name: %s\n", profile.Name))
					}
					builder.WriteString(fmt.Sprintf("      path: %s\n", profile.Path))
					for _, cache := range profile.Caches {
						builder.WriteString(fmt.Sprintf("      cache: %s\n", cache.Kind))
						builder.WriteString(fmt.Sprintf("        path: %s\n", cache.Path))
						builder.WriteString(fmt.Sprintf("        bytes: %d\n", cache.Bytes))
					}
				}
			}
			builder.WriteString(fmt.Sprintf("    status: %s\n", opportunity.Status))
			builder.WriteString(fmt.Sprintf("    reason: %s\n", opportunity.Reason))
			builder.WriteString("    not counted as Potential space: true\n")
		}
	}

	builder.WriteString("\nSkipped items\n")
	if len(result.Skipped) == 0 {
		builder.WriteString("  No skipped cleanup paths reported.\n")
	} else {
		for _, skipped := range result.Skipped {
			builder.WriteString(fmt.Sprintf("  path: %s\n", skipped.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", skipped.Rule))
			if skipped.Bytes > 0 {
				builder.WriteString(fmt.Sprintf("    bytes: %d\n", skipped.Bytes))
			}
			builder.WriteString(fmt.Sprintf("    reason: %s\n", skipped.Reason.Code))
			if skipped.Reason.Message != "" {
				builder.WriteString(fmt.Sprintf("    message: %s\n", skipped.Reason.Message))
			}
			builder.WriteString(fmt.Sprintf("    recoverable: %t\n", skipped.Reason.Recoverable))
		}
	}

	builder.WriteString("\nRecoverable errors\n")
	if len(result.Errors) == 0 {
		builder.WriteString("  No recoverable inspection errors reported.\n")
	} else {
		for _, err := range result.Errors {
			builder.WriteString(fmt.Sprintf("  path: %s\n", err.Path))
			builder.WriteString(fmt.Sprintf("    rule: %s\n", err.Rule))
			builder.WriteString(fmt.Sprintf("    error: %s\n", err.Code))
			if err.Message != "" {
				builder.WriteString(fmt.Sprintf("    message: %s\n", err.Message))
			}
			builder.WriteString(fmt.Sprintf("    recoverable: %t\n", err.Recoverable))
		}
	}

	return path, os.WriteFile(path, []byte(builder.String()), 0600)
}

func plannedActionLabel(action string) string {
	switch action {
	case plannedRecycleBinAction:
		return "Recycle Bin"
	case string(DeletionActionDeletePermanently):
		return "Permanent deletion"
	default:
		return action
	}
}
