package clean

import (
	"context"
)

// optInResolution is the result of resolving opt-in candidates for a run.
// Both dry-run preview and execute consume the same resolution so preview and
// execute agree on what is cleanable. It is produced fresh per run; execute
// never trusts dry-run's resolved paths (ADR-0008 fresh-scan).
type optInResolution struct {
	// candidates are the opt-in candidate paths that survived running-app
	// gating and protection, fresh-measured. A Browser cache candidate is an
	// individual regenerating cache directory per profile, not the User Data
	// root, because only those directories are deletable.
	candidates []OptInCandidate
	// skipped holds path-backed items gated out by a running application
	// (distinctive-process dev caches). Both modes surface these.
	skipped []SkippedItem
	// runningStates are the running-application states consulted for browser
	// gating (pre and post). Both modes surface these, so a running browser
	// is reported at execute as well as dry-run.
	runningStates []RunningApplicationState
	// diagnostics are recoverable errors: unknown browser process state,
	// incomplete opted-in opportunity inspection, or browser catalog errors.
	// Both modes surface these.
	diagnostics []StructuredIssue
	// suppressedProtectionPaths are protection-rule paths suppressed during
	// resolution. dry-run applies them to its ProtectionRules display;
	// execute ignores them.
	suppressedProtectionPaths []string
}

// nonBrowserOpportunityCategories is the fixed set of opportunity categories
// discovered through DiscoverOpportunities (browser cache is discovered
// separately through discoverBrowserCache).
var nonBrowserOpportunityCategories = []string{
	OpportunityCategoryUserTemp,
	OpportunityCategoryCrashDumps,
	OpportunityCategoryWindowsErrorReporting,
	OpportunityCategoryExplorerThumbnailCache,
	OpportunityCategoryINetCache,
	OpportunityCategoryD3DShaderCache,
	OpportunityCategoryNVIDIADXCache,
}

// optedInOpportunityCategories returns the non-browser opportunity categories
// enabled by the plan.
func optedInOpportunityCategories(plan map[string]bool) []string {
	var enabled []string
	for _, c := range nonBrowserOpportunityCategories {
		if plan[c] {
			enabled = append(enabled, c)
		}
	}
	return enabled
}

// optedOutOpportunityCategories returns the non-browser opportunity categories
// NOT enabled by the plan - the categories the dry-run review projection scans.
func optedOutOpportunityCategories(plan map[string]bool) []string {
	var disabled []string
	for _, c := range nonBrowserOpportunityCategories {
		if !plan[c] {
			disabled = append(disabled, c)
		}
	}
	return disabled
}

// resolveOptInCandidates turns an opt-in plan into the concrete deletable
// Opt-in candidate paths for a run. Only opted-in categories are scanned
// (ADR-0008: non-opted-in categories stay omitted from execute); the dry-run
// review projection scans the rest separately. Running-application gating is
// delegated to runningGate. The plan is computed once by the caller.
func resolveOptInCandidates(ctx context.Context, opts Options, plan map[string]bool) optInResolution {
	var res optInResolution
	if len(plan) == 0 {
		return res
	}

	hasDetector := opts.DetectRunningApplications != nil
	var devCacheStates []RunningApplicationState
	if hasDetector {
		// Dev-cache states are used only for gating; they are not reported in
		// RunningApplications (only browser states are).
		devCacheStates = opts.DetectRunningApplications(ctx)
	}

	resolveDevCache := opts.DevCachePathResolver
	if resolveDevCache == nil {
		resolveDevCache = ResolveDevCachePath
	}

	// Developer-tool caches.
	for category := range plan {
		if !isDevCacheCategory(category) {
			continue
		}
		path := resolveDevCache(category)
		if path == "" || opts.Validator.IsUserProtected(path) {
			continue
		}
		if hasDetector {
			if outcome := (runningGate{}).gateDevCache(category, path, devCacheStates); !outcome.proceed {
				if outcome.skipReason != nil {
					if bytes, err := measureBytes(path); err == nil {
						res.skipped = append(res.skipped, SkippedItem{
							Path:   path,
							Bytes:  bytes,
							Rule:   category,
							Reason: *outcome.skipReason,
						})
					}
				}
				continue
			}
		}
		bytes, err := measureBytes(path)
		if err != nil {
			continue
		}
		res.candidates = append(res.candidates, OptInCandidate{
			Path:          path,
			Bytes:         bytes,
			Category:      category,
			PlannedAction: plannedRecycleBinAction,
		})
	}

	// Opportunity categories (opted-in only).
	optedInCats := optedInOpportunityCategories(plan)
	if len(optedInCats) > 0 {
		discovery := discoverOpportunitiesForCategories(ctx, opts, optedInCats)
		for _, opportunity := range discovery.Opportunities {
			opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
			if opts.Validator.IsUserProtected(opportunity.Path) {
				continue
			}
			candidate := OptInCandidate{
				Path:          opportunity.Path,
				Bytes:         opportunity.Bytes,
				Category:      opportunity.Category,
				PlannedAction: plannedRecycleBinAction,
			}
			if opportunity.Category == OpportunityCategoryUserTemp {
				candidate.IsUserTemp = true
				candidate.LatestModified = opportunity.LatestModifiedAt.Unix()
				candidate.IdleDays = opportunity.IdleDays
			}
			res.candidates = append(res.candidates, candidate)
		}
		for _, incomplete := range discovery.Incomplete {
			incomplete.Category = normalizedOpportunityCategory(incomplete.Category)
			if opts.Validator.IsUserProtected(incomplete.Path) {
				continue
			}
			res.diagnostics = append(res.diagnostics, incomplete.Reason)
		}
	}

	// Browser cache (opted-in only) - individual cache directories.
	if plan[OpportunityCategoryBrowserCache] && hasDetector {
		resolveBrowserOptInCandidates(ctx, opts, &res)
	}

	return res
}

// resolveBrowserOptInCandidates gates each supported browser's cache discovery
// on running-application state and appends individual cache-directory
// candidates (pre-state, discover, post-state). Suppressed, diagnostic, and
// incomplete outcomes become diagnostics; running/unknown outcomes become
// running states and diagnostics. All artifacts surface in both modes.
func resolveBrowserOptInCandidates(ctx context.Context, opts Options, res *optInResolution) {
	gate := runningGate{detect: opts.DetectRunningApplications}
	preStates := opts.DetectRunningApplications(ctx)
	res.runningStates = append(res.runningStates, preStates...)
	for _, config := range browserCacheConfigs {
		if localAppDataDir := browserCacheLocalAppDataDir(opts.BrowserCacheDiscoveryOptions); localAppDataDir != "" {
			if suppressed, protectedRulePaths := browserDiscoverySuppressed(browserUserDataRoot(localAppDataDir, config), opts.Validator); suppressed {
				res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, protectedRulePaths...)
				continue
			}
		}
		outcome := gate.gateBrowser(ctx, config.application, preStates, func() browserCacheDiscoveryResult {
			return discoverBrowserCache(ctx, config, opts.BrowserCacheDiscoveryOptions, opts.Validator)
		})
		if !outcome.preIdle {
			continue
		}
		discovery := outcome.discovery
		if discovery.suppressed {
			res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, discovery.suppressedProtectionPaths...)
			continue
		}
		if discovery.diagnostic != nil {
			res.diagnostics = append(res.diagnostics, *discovery.diagnostic)
			continue
		}
		if discovery.incomplete != nil {
			if !browserOpportunityPathProtected(opts.Validator, discovery.incomplete) {
				res.diagnostics = append(res.diagnostics, discovery.incomplete.Reason)
			}
			continue
		}
		if !outcome.postIdle {
			if outcome.postState != nil {
				res.runningStates = append(res.runningStates, *outcome.postState)
			}
			if outcome.postDiagnostic != nil {
				res.diagnostics = append(res.diagnostics, *outcome.postDiagnostic)
			}
			continue
		}
		if discovery.opportunity == nil || browserOpportunityProtected(opts.Validator, *discovery.opportunity) {
			continue
		}
		appendBrowserCacheCandidates(res, discovery.opportunity)
	}
}

// appendBrowserCacheCandidates appends one Opt-in candidate per non-empty
// regenerating cache directory across the browser's profiles.
func appendBrowserCacheCandidates(res *optInResolution, opportunity *Opportunity) {
	if opportunity.BrowserCache == nil {
		return
	}
	for _, profile := range opportunity.BrowserCache.Profiles {
		for _, cache := range profile.Caches {
			if cache.Bytes <= 0 {
				continue
			}
			res.candidates = append(res.candidates, OptInCandidate{
				Path:          cache.Path,
				Bytes:         cache.Bytes,
				Category:      OpportunityCategoryBrowserCache,
				PlannedAction: plannedRecycleBinAction,
			})
		}
	}
}

// discoverOpportunitiesForCategories runs opportunity discovery scoped to the
// given categories. When a discovery function is injected (tests), its results
// are filtered to the categories; otherwise the real DiscoverOpportunities is
// called with a Categories filter so non-opted-in categories are not scanned
// at execute (ADR-0008).
func discoverOpportunitiesForCategories(ctx context.Context, opts Options, categories []string) OpportunityDiscoveryResult {
	discover := opts.DiscoverOpportunities
	if discover == nil && opts.DiscoverUserTempOpportunities != nil {
		discover = opts.DiscoverUserTempOpportunities
	}
	if discover != nil {
		return filterOpportunityDiscovery(discover(ctx), categories)
	}
	discoveryOptions := opts.OpportunityDiscoveryOptions
	discoveryOptions.Categories = categories
	if discoveryOptions.TempDir == "" {
		discoveryOptions.TempDir = opts.UserTempDiscoveryOptions.TempDir
	}
	if discoveryOptions.Now.IsZero() {
		discoveryOptions.Now = opts.UserTempDiscoveryOptions.Now
	}
	return DiscoverOpportunities(ctx, discoveryOptions)
}

// filterOpportunityDiscovery keeps only opportunities and incompletes whose
// category is in the filter. An empty filter keeps everything.
func filterOpportunityDiscovery(result OpportunityDiscoveryResult, categories []string) OpportunityDiscoveryResult {
	if len(categories) == 0 {
		return result
	}
	allowed := make(map[string]bool, len(categories))
	for _, c := range categories {
		allowed[c] = true
	}
	filtered := OpportunityDiscoveryResult{
		Opportunities: []Opportunity{},
		Incomplete:    []IncompleteOpportunityInspection{},
	}
	for _, opp := range result.Opportunities {
		if allowed[normalizedOpportunityCategory(opp.Category)] {
			filtered.Opportunities = append(filtered.Opportunities, opp)
		}
	}
	for _, inc := range result.Incomplete {
		if allowed[normalizedOpportunityCategory(inc.Category)] {
			filtered.Incomplete = append(filtered.Incomplete, inc)
		}
	}
	return filtered
}
