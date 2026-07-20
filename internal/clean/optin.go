package clean

import (
	"context"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// optInResolution is the result of resolving opt-in candidates for a run.
// Both dry-run preview and execute consume the same resolution so preview and
// execute agree on what is cleanable. It is produced fresh per run; execute
// never trusts dry-run's resolved paths (ADR-0008 fresh-scan).
//
// Construction composes single-category resolveCategoryCore results so gate,
// protection, and measurement policy live in one place.
type optInResolution struct {
	// candidates are the opt-in candidate paths that survived running-app
	// gating and protection, fresh-measured. A Browser cache candidate is an
	// individual regenerating cache directory per profile, not the User Data
	// root, because only those directories are deletable.
	candidates []OptInCandidate
	// skipped holds path-backed items gated out by a running application
	// (distinctive-process dev caches). Both modes surface these.
	skipped []SkippedItem
	// runningStates are the scoped, deduplicated running-application states
	// consulted by gates that participated in this plan (browser and/or
	// Application-cache identities; developer-cache gates keep their own
	// reporting policy and do not append shared-runtime noise here). Both
	// modes surface these, so a running browser/editor is reported at
	// execute as well as dry-run.
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

// optedOutOpportunityCategories returns the non-browser opportunity categories
// NOT enabled by the plan - the categories the dry-run review projection scans.
func optedOutOpportunityCategories(plan map[string]bool) []string {
	var disabled []string
	for _, c := range opportunityCategoryIDs(false) {
		if !plan[c] {
			disabled = append(disabled, c)
		}
	}
	return disabled
}

// normalizeAndDeduplicatePaths normalizes paths and removes duplicates while
// preserving order. Uses Windows path identity for deduplication and discards
// empty/whitespace-only paths before cleaning.
func normalizeAndDeduplicatePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if pathsafe.IsEmptyOrWhitespacePath(path) {
			continue
		}
		identity := pathsafe.NormalizePathForIdentity(path)
		if !seen[identity] {
			seen[identity] = true
			// Keep the first-seen spelling for display/execution, but clean it
			result = append(result, filepath.Clean(path))
		}
	}
	return result
}

// resolveOptInCandidates turns an opt-in plan into the concrete deletable
// Opt-in candidate paths for a run. Only opted-in categories are scanned
// (ADR-0008: non-opted-in categories stay omitted from execute); the dry-run
// review projection scans the rest separately via the same single-category
// core. Each planned category is resolved independently then merged.
func resolveOptInCandidates(ctx context.Context, opts Options, plan map[string]bool) optInResolution {
	var res optInResolution
	if len(plan) == 0 {
		return res
	}
	for _, category := range orderedPlanCategories(plan) {
		// Observation-only category boundary during execute. Dry-run leaves
		// ProgressReporter nil so this is a no-op on preview paths.
		reportExecutionProgress(opts.ProgressReporter, ExecutionPhaseScanning, category)
		core, err := resolveCategoryCore(ctx, opts, category)
		if err != nil {
			// Plan keys are canonical opt-in IDs from planning; unexpected
			// rejection is reported as a recoverable diagnostic and skipped.
			res.diagnostics = append(res.diagnostics, issue(
				"category_resolution_failed",
				err.Error(),
				true,
				"",
				category,
			))
			continue
		}
		mergeCategoryCoreIntoOptInResolution(&res, core)
	}
	return res
}

// resolveDeveloperCacheCategory resolves one developer-tool cache category into
// core. Each root scope is gated and measured independently so discarding one
// scope never authorizes or double-counts another. Structured child discovery
// never treats the root as a candidate; product-scoped scopes apply
// idle-before-and-after only for that logical application identity.
func resolveDeveloperCacheCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || !isDevCacheCategory(category) {
		return
	}
	hasDetector := opts.DetectRunningApplications != nil
	var devCachePreStates []RunningApplicationState
	devCacheDetectLoaded := false
	devCacheGate := runningGate{}
	ensureDevCacheDetection := func() {
		if !hasDetector || devCacheDetectLoaded {
			return
		}
		devCachePreStates = opts.DetectRunningApplications(ctx)
		devCacheGate.detect = opts.DetectRunningApplications
		devCacheDetectLoaded = true
	}
	// Distinctive-process and product-scoped roots load detection lazily.
	// Shared-runtime alone must not trigger developer-tool detection
	// (ADR-0008 attribution policy).
	if hasDetector && devCacheGateTier(category) == runningGateTierBeforeAfter {
		ensureDevCacheDetection()
	}

	scopes := resolveDevCacheRootScopes(opts, category)
	for _, scope := range scopes {
		path := scope.Path
		if path == "" {
			continue
		}
		if opts.Validator.IsUserProtected(path) {
			// Protected root: do not discover children or measure the root.
			// Protection never authorizes siblings under another root.
			core.SuppressedProtectionPaths = append(
				core.SuppressedProtectionPaths,
				structuredDevCacheProtectedRulePaths(path, opts.Validator)...,
			)
			continue
		}

		apps, useGate := gateApplicationsForDevCacheScope(category, scope)
		productScoped := scope.Application != ""
		if useGate && hasDetector {
			ensureDevCacheDetection()
		}
		// Product-scoped and category-wide distinctive gates require a loaded
		// detector snapshot when a detector is present. Without a detector,
		// shared-runtime and test paths measure directly (fail open only for
		// process state; protection/validation still apply).
		applyGate := useGate && hasDetector && devCacheDetectLoaded

		children, structured := resolveDevCacheChildCandidates(ctx, opts, category, path)
		if structured {
			resolveStructuredDevCacheRoot(ctx, opts, core, category, scope, children, apps, applyGate, productScoped, devCacheGate, devCachePreStates)
			continue
		}

		// Whole-root mode: the resolved root is the single candidate.
		if applyGate {
			outcome := devCacheGate.gateDevCacheApplications(ctx, category, path, apps, true, devCachePreStates, func() (int64, error) {
				return measureBytes(ctx, path)
			})
			// Product-scoped gates report only their logical application
			// identities; category-wide distinctive-process gates keep skip
			// reasons without projecting shared tool noise into RunningStates.
			if productScoped {
				outcome.apply(&core.RunningStates, nil)
			}
			if outcome.measureErr != nil {
				if ctx.Err() != nil {
					core.Diagnostics = append(core.Diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
				}
				continue
			}
			if !outcome.proceed {
				if outcome.skipReason != nil {
					// Structured safety skip without reclaimable bytes: pre
					// gate never measured; post gate discarded the measure.
					core.Skipped = append(core.Skipped, SkippedItem{
						Path:          path,
						Bytes:         0,
						Rule:          category,
						PlannedAction: plannedActionForOpts(opts, category),
						Reason:        *outcome.skipReason,
					})
				}
				continue
			}
			core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
				Path:          path,
				Bytes:         outcome.bytes,
				Category:      category,
				PlannedAction: plannedActionForOpts(opts, category),
			})
			continue
		}

		bytes, err := measureBytes(ctx, path)
		if err != nil {
			// Failed or canceled measurement yields no candidate; non-canceled
			// unrelated roots continue. Cancellation shows as recoverable diagnostic.
			if ctx.Err() != nil {
				core.Diagnostics = append(core.Diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
			}
			continue
		}
		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          path,
			Bytes:         bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}

// resolveStructuredDevCacheRoot discovers and measures independent child
// candidates under one resolved developer-cache root scope. When applyGate is
// true, apps must be idle before discovery and again after measurement; a
// post-gate failure discards every child from this scope without authorizing
// siblings under other scopes. Product-scoped gates project only their
// application identities into runningStates.
func resolveStructuredDevCacheRoot(
	ctx context.Context,
	opts Options,
	core *categoryCoreResult,
	category string,
	scope DevCacheRootScope,
	children []string,
	apps []string,
	applyGate bool,
	productScoped bool,
	devCacheGate runningGate,
	devCachePreStates []RunningApplicationState,
) {
	root := scope.Path
	if applyGate {
		// Pre observation uses the same project + fail-closed helper as the
		// whole-root measure gate so product-scoped reporting stays consistent.
		idle, projected, reason := observeDevCacheApps(apps, root, category, devCachePreStates)
		if productScoped {
			runningGateOutcome{runningStates: projected}.apply(&core.RunningStates, nil)
		}
		if !idle {
			if reason != nil {
				core.Skipped = append(core.Skipped, SkippedItem{
					Path:          root,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForOpts(opts, category),
					Reason:        *reason,
				})
			}
			return
		}
	}

	start := len(core.OptInCandidates)
	appendStructuredDevCacheCandidates(ctx, opts, core, category, root, children, structuredDevCacheMeasureDependencies{})
	if len(core.OptInCandidates) == start {
		return
	}

	if !applyGate {
		return
	}
	var postStates []RunningApplicationState
	if devCacheGate.detect != nil {
		postStates = devCacheGate.detect(ctx)
	}
	idle, projected, reason := observeDevCacheApps(apps, root, category, postStates)
	if productScoped {
		// Post supersedes pre for matching identities via merge-in-place.
		runningGateOutcome{runningStates: projected}.apply(&core.RunningStates, nil)
	}
	if !idle {
		// Discard all children measured under this root scope.
		core.OptInCandidates = core.OptInCandidates[:start]
		if reason != nil {
			core.Skipped = append(core.Skipped, SkippedItem{
				Path:          root,
				Bytes:         0,
				Rule:          category,
				PlannedAction: plannedActionForOpts(opts, category),
				Reason:        *reason,
			})
		}
	}
}

// resolveExistenceOpportunityCategory discovers one non-browser, non-application
// opportunity category and dual-projects surviving paths into Opportunities
// (review) and OptInCandidates (execute). Protection removes candidates only.
func resolveExistenceOpportunityCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil {
		return
	}
	discovery := discoverOpportunitiesForCategories(ctx, opts, []string{category})
	for _, opportunity := range discovery.Opportunities {
		opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
		if opts.Validator.IsUserProtected(opportunity.Path) {
			// Retain path-free exclusion evidence for eager preview; the
			// protected path itself never leaves ProjectCategoryPreview.
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, opportunity.Path)
			continue
		}
		core.Opportunities = append(core.Opportunities, opportunity)
		candidate := OptInCandidate{
			Path:          opportunity.Path,
			Bytes:         opportunity.Bytes,
			Category:      opportunity.Category,
			PlannedAction: plannedActionForOpts(opts, opportunity.Category),
		}
		if opportunity.Category == OpportunityCategoryUserTemp {
			candidate.IsUserTemp = true
			candidate.LatestModified = opportunity.LatestModifiedAt.Unix()
			candidate.IdleDays = opportunity.IdleDays
		}
		core.OptInCandidates = append(core.OptInCandidates, candidate)
	}
	for _, incomplete := range discovery.Incomplete {
		incomplete.Category = normalizedOpportunityCategory(incomplete.Category)
		if opts.Validator.IsUserProtected(incomplete.Path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, incomplete.Path)
			continue
		}
		core.IncompleteInspections = append(core.IncompleteInspections, incomplete)
		core.Diagnostics = append(core.Diagnostics, incomplete.Reason)
	}
}

// resolveApplicationCacheCategory gates one Application cache category and
// dual-projects measured roots into Opportunities and OptInCandidates.
func resolveApplicationCacheCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || opts.DetectRunningApplications == nil {
		return
	}
	entry, ok := applicationCacheEntry(category)
	if !ok || len(entry.runningApplications) == 0 {
		return
	}
	application := entry.runningApplications[0]
	policyID, policy, ok := applicationCachePolicyForCategory(category)
	if !ok {
		return
	}
	preflight := preflightApplicationCacheRoot(
		opts.ApplicationCacheDiscoveryOptions,
		policy,
		opts.Validator,
		applicationCacheStat(opts.ApplicationCacheDiscoveryOptions),
	)
	if preflight.absent {
		return
	}
	if len(preflight.suppressedProtectionPaths) > 0 {
		core.SuppressedProtectionPaths = append(
			core.SuppressedProtectionPaths,
			preflight.suppressedProtectionPaths...,
		)
		return
	}

	gate := runningGate{detect: opts.DetectRunningApplications}
	preStates := opts.DetectRunningApplications(ctx)
	outcome := gate.gateApplicationCache(ctx, application, preStates, func() applicationCacheDiscoveryResult {
		return resolveApplicationCacheDiscovery(ctx, opts, policyID)
	})
	// Gate already projects the scoped editor identity (pre + post supersede)
	// and builds unknown diagnostics when post state is Unknown.
	outcome.apply(&core.RunningStates, &core.Diagnostics)
	if !outcome.discoveryRan {
		return
	}
	discovery := outcome.discovery
	core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, discovery.suppressedProtectionPaths...)
	for _, incomplete := range discovery.incompletes {
		if opts.Validator.IsUserProtected(incomplete.Path) {
			continue
		}
		core.IncompleteInspections = append(core.IncompleteInspections, incomplete)
		core.Diagnostics = append(core.Diagnostics, incomplete.Reason)
	}
	if discovery.canceled || !outcome.proceed {
		// States/diagnostics already applied; discard measured roots.
		return
	}
	for _, opportunity := range discovery.opportunities {
		if opts.Validator.IsUserProtected(opportunity.Path) {
			continue
		}
		core.Opportunities = append(core.Opportunities, opportunity)
		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          opportunity.Path,
			Bytes:         opportunity.Bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}

// resolveBrowserCacheCategory gates each supported browser's cache discovery
// on running-application state. Dual-projects: full Opportunity aggregates for
// review and individual regenerating cache directories for opt-in execution.
func resolveBrowserCacheCategory(ctx context.Context, opts Options, core *categoryCoreResult) {
	if core == nil || opts.DetectRunningApplications == nil {
		return
	}
	gate := runningGate{detect: opts.DetectRunningApplications}
	preStates := opts.DetectRunningApplications(ctx)
	// Seed all supported browser identities from the shared pre snapshot so
	// protected or never-gated browsers still appear. Per-browser gate outcomes
	// then supersede with post observations and unknown diagnostics.
	core.RunningStates = mergeRunningApplicationStates(
		core.RunningStates,
		projectRunningApplicationStates(preStates, browserRunningApplicationIdentities()...)...,
	)
	for _, config := range browserCacheConfigs {
		suppressedForBrowser := false
		for _, root := range browserDiscoveryRoots(config, opts.BrowserCacheDiscoveryOptions) {
			if suppressed, protectedRulePaths := browserDiscoverySuppressed(root, opts.Validator); suppressed {
				core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, protectedRulePaths...)
				suppressedForBrowser = true
				break
			}
		}
		if suppressedForBrowser {
			continue
		}
		outcome := gate.gateBrowser(ctx, config.application, preStates, func() browserCacheDiscoveryResult {
			return discoverBrowserCache(ctx, config, opts.BrowserCacheDiscoveryOptions, opts.Validator)
		})
		// Projected post state + unknown diagnostics come from the gate only.
		outcome.apply(&core.RunningStates, &core.Diagnostics)
		if !outcome.discoveryRan {
			continue
		}
		discovery := outcome.discovery
		if discovery.suppressed {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, discovery.suppressedProtectionPaths...)
			continue
		}
		if discovery.diagnostic != nil {
			core.Diagnostics = append(core.Diagnostics, *discovery.diagnostic)
			continue
		}
		if discovery.incomplete != nil {
			if !browserOpportunityPathProtected(opts.Validator, discovery.incomplete) {
				core.IncompleteInspections = append(core.IncompleteInspections, *discovery.incomplete)
				core.Diagnostics = append(core.Diagnostics, discovery.incomplete.Reason)
			}
			continue
		}
		if !outcome.proceed {
			// Post fail-closed: states/diagnostics already applied; no reclaim.
			continue
		}
		if discovery.opportunity == nil || browserOpportunityProtected(opts.Validator, *discovery.opportunity) {
			continue
		}
		core.Opportunities = append(core.Opportunities, *discovery.opportunity)
		appendBrowserCacheCandidates(opts, core, discovery.opportunity)
	}
}

// appendBrowserCacheCandidates appends one Opt-in candidate per non-empty
// regenerating cache directory across the browser's profiles.
func appendBrowserCacheCandidates(opts Options, core *categoryCoreResult, opportunity *Opportunity) {
	if core == nil || opportunity == nil || opportunity.BrowserCache == nil {
		return
	}
	for _, profile := range opportunity.BrowserCache.Profiles {
		for _, cache := range profile.Caches {
			if cache.Bytes <= 0 {
				continue
			}
			core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
				Path:          cache.Path,
				Bytes:         cache.Bytes,
				Category:      OpportunityCategoryBrowserCache,
				PlannedAction: plannedActionForOpts(opts, OpportunityCategoryBrowserCache),
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
