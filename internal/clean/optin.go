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

// optedInOpportunityCategories returns the non-browser opportunity categories
// enabled by the plan.
func optedInOpportunityCategories(plan map[string]bool) []string {
	var enabled []string
	for _, c := range opportunityCategoryIDs(false) {
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
// review projection scans the rest separately. Running-application gating is
// delegated to runningGate. The plan is computed once by the caller.
func resolveOptInCandidates(ctx context.Context, opts Options, plan map[string]bool) optInResolution {
	var res optInResolution
	if len(plan) == 0 {
		return res
	}

	hasDetector := opts.DetectRunningApplications != nil
	// Distinctive-process developer caches need a pre-measurement snapshot and
	// a post-measurement re-check. Shared-runtime selections alone must not
	// trigger developer-tool detection (ADR-0008 attribution policy). Product-
	// scoped roots may also require detection; load lazily on first gated root.
	needsDistinctiveDetection := hasDetector && planNeedsDistinctiveProcessDetection(plan)
	var devCachePreStates []RunningApplicationState
	devCacheDetectLoaded := false
	devCacheGate := runningGate{}
	ensureDevCacheDetection := func() {
		if !hasDetector || devCacheDetectLoaded {
			return
		}
		// Pre-states gate distinctive and product-scoped roots. Category-wide
		// distinctive gates keep current reporting policy (no projection into
		// RunningApplications). Product-scoped gates project only their identities.
		devCachePreStates = opts.DetectRunningApplications(ctx)
		devCacheGate.detect = opts.DetectRunningApplications
		devCacheDetectLoaded = true
	}
	if needsDistinctiveDetection {
		ensureDevCacheDetection()
	}

	// Developer-tool caches. Each root scope is gated and measured independently
	// so discarding one scope never authorizes or double-counts another.
	// Categories with structured child discovery (or injected structured mode)
	// never treat the root as a candidate; each surviving child is independent.
	// Product-scoped scopes (non-empty Application) apply idle-before-and-after
	// only for that logical application identity.
	for _, category := range developerCacheCategoryIDs() {
		if !plan[category] {
			continue
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
				res.suppressedProtectionPaths = append(
					res.suppressedProtectionPaths,
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
				resolveStructuredDevCacheRoot(ctx, opts, &res, category, scope, children, apps, applyGate, productScoped, devCacheGate, devCachePreStates)
				continue
			}

			// Whole-root mode: the resolved root is the single candidate.
			if applyGate {
				if productScoped {
					// Project pre observation for product identity (scoped, latest).
					recordProductScopedRunningStates(&res, apps, devCachePreStates)
				}
				outcome := devCacheGate.gateDevCacheApplications(ctx, category, path, apps, true, devCachePreStates, func() (int64, error) {
					return measureBytes(ctx, path)
				})
				if productScoped && len(outcome.postStates) > 0 {
					recordProductScopedRunningStates(&res, apps, outcome.postStates)
				}
				if outcome.measureErr != nil {
					if ctx.Err() != nil {
						res.diagnostics = append(res.diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
					}
					continue
				}
				if !outcome.proceed {
					if outcome.skipReason != nil {
						// Structured safety skip without reclaimable bytes: pre
						// gate never measured; post gate discarded the measure.
						res.skipped = append(res.skipped, SkippedItem{
							Path:          path,
							Bytes:         0,
							Rule:          category,
							PlannedAction: plannedActionForCategory(category),
							Reason:        *outcome.skipReason,
						})
					}
					continue
				}
				res.candidates = append(res.candidates, OptInCandidate{
					Path:          path,
					Bytes:         outcome.bytes,
					Category:      category,
					PlannedAction: plannedActionForCategory(category),
				})
				continue
			}

			bytes, err := measureBytes(ctx, path)
			if err != nil {
				// Failed or canceled measurement yields no candidate; non-canceled
				// unrelated roots continue. Cancellation shows as recoverable diagnostic.
				if ctx.Err() != nil {
					res.diagnostics = append(res.diagnostics, issue("context_canceled", ctx.Err().Error(), true, path, category))
				}
				continue
			}
			res.candidates = append(res.candidates, OptInCandidate{
				Path:          path,
				Bytes:         bytes,
				Category:      category,
				PlannedAction: plannedActionForCategory(category),
			})
		}
	}

	// Opportunity categories (opted-in only).
	optedInCats := optedInOpportunityCategories(plan)
	if len(optedInCats) > 0 {
		discovery := discoverOpportunitiesForCategories(ctx, opts, optedInCats)
		for _, opportunity := range discovery.Opportunities {
			opportunity.Category = normalizedOpportunityCategory(opportunity.Category)
			if opts.Validator.IsUserProtected(opportunity.Path) {
				// Retain path-free exclusion evidence for eager preview; the
				// protected path itself never leaves ProjectCategoryPreview.
				res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, opportunity.Path)
				continue
			}
			candidate := OptInCandidate{
				Path:          opportunity.Path,
				Bytes:         opportunity.Bytes,
				Category:      opportunity.Category,
				PlannedAction: plannedActionForCategory(opportunity.Category),
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
				res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, incomplete.Path)
				continue
			}
			res.diagnostics = append(res.diagnostics, incomplete.Reason)
		}
	}

	// Browser cache (opted-in only) - individual cache directories.
	if plan[OpportunityCategoryBrowserCache] && hasDetector {
		resolveBrowserOptInCandidates(ctx, opts, &res)
	}

	// Idle Application cache categories (opted-in only) - one candidate per root.
	if hasDetector {
		for _, category := range applicationCacheCategoryIDs() {
			if plan[category] {
				resolveApplicationCacheOptInCandidates(ctx, opts, category, &res)
			}
		}
	}

	return res
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
	res *optInResolution,
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
		if productScoped {
			recordProductScopedRunningStates(res, apps, devCachePreStates)
		}
		if idle, reason := appsIdleForApplications(apps, root, category, devCachePreStates); !idle {
			if reason != nil {
				res.skipped = append(res.skipped, SkippedItem{
					Path:          root,
					Bytes:         0,
					Rule:          category,
					PlannedAction: plannedActionForCategory(category),
					Reason:        *reason,
				})
			}
			return
		}
	}

	start := len(res.candidates)
	appendStructuredDevCacheCandidates(ctx, opts, res, category, root, children, structuredDevCacheMeasureDependencies{})
	if len(res.candidates) == start {
		return
	}

	if !applyGate {
		return
	}
	var postStates []RunningApplicationState
	if devCacheGate.detect != nil {
		postStates = devCacheGate.detect(ctx)
	}
	if productScoped {
		recordProductScopedRunningStates(res, apps, postStates)
	}
	if idle, reason := appsIdleForApplications(apps, root, category, postStates); !idle {
		// Discard all children measured under this root scope.
		res.candidates = res.candidates[:start]
		if reason != nil {
			res.skipped = append(res.skipped, SkippedItem{
				Path:          root,
				Bytes:         0,
				Rule:          category,
				PlannedAction: plannedActionForCategory(category),
				Reason:        *reason,
			})
		}
	}
}

// recordProductScopedRunningStates projects product-scoped application
// observations into the resolution. Match is by canonical identity only;
// later observations replace earlier ones in place without reordering.
func recordProductScopedRunningStates(res *optInResolution, apps []string, states []RunningApplicationState) {
	if res == nil || len(apps) == 0 || len(states) == 0 {
		return
	}
	for _, app := range apps {
		if state, found := runningApplicationStateFor(states, app); found {
			res.runningStates = mergeRunningApplicationStates(res.runningStates, state)
		}
	}
}

// resolveApplicationCacheOptInCandidates gates one Application cache category
// and appends independent Opt-in candidates for each measured root.
func resolveApplicationCacheOptInCandidates(ctx context.Context, opts Options, category string, res *optInResolution) {
	entry, ok := applicationCacheEntry(category)
	if !ok || len(entry.runningApplications) == 0 {
		return
	}
	application := entry.runningApplications[0]
	policyID, policy, ok := applicationCachePolicyForCategory(category)
	if !ok {
		return
	}
	if roaming := applicationCacheRoamingAppDataDir(opts.ApplicationCacheDiscoveryOptions); roaming != "" {
		userDataRoot := applicationCacheUserDataRoot(roaming, policy)
		if opts.Validator.IsUserProtected(userDataRoot) {
			res.suppressedProtectionPaths = append(
				res.suppressedProtectionPaths,
				applicationCacheProtectedRulePaths(userDataRoot, opts.Validator)...,
			)
			return
		}
	}

	gate := runningGate{detect: opts.DetectRunningApplications}
	preStates := opts.DetectRunningApplications(ctx)
	// Surface only the application identity owned by this selected category.
	if state, found := runningApplicationStateFor(preStates, application); found {
		res.runningStates = mergeRunningApplicationStates(res.runningStates, state)
	}

	outcome := gate.gateApplicationCache(ctx, application, preStates, func() applicationCacheDiscoveryResult {
		return resolveApplicationCacheDiscovery(ctx, opts, policyID)
	})
	if !outcome.preIdle {
		return
	}
	discovery := outcome.discovery
	res.suppressedProtectionPaths = append(res.suppressedProtectionPaths, discovery.suppressedProtectionPaths...)
	for _, incomplete := range discovery.incompletes {
		if opts.Validator.IsUserProtected(incomplete.Path) {
			continue
		}
		res.diagnostics = append(res.diagnostics, incomplete.Reason)
	}
	if discovery.canceled || !outcome.postIdle {
		if outcome.postState != nil {
			// Post-measurement supersedes pre-measurement for this identity.
			res.runningStates = mergeRunningApplicationStates(res.runningStates, *outcome.postState)
		}
		if outcome.postDiagnostic != nil {
			res.diagnostics = append(res.diagnostics, *outcome.postDiagnostic)
		}
		return
	}
	for _, opportunity := range discovery.opportunities {
		if opts.Validator.IsUserProtected(opportunity.Path) {
			continue
		}
		res.candidates = append(res.candidates, OptInCandidate{
			Path:          opportunity.Path,
			Bytes:         opportunity.Bytes,
			Category:      category,
			PlannedAction: plannedActionForCategory(category),
		})
	}
}

// resolveBrowserOptInCandidates gates each supported browser's cache discovery
// on running-application state and appends individual cache-directory
// candidates (pre-state, discover, post-state). Suppressed, diagnostic, and
// incomplete outcomes become diagnostics; running/unknown outcomes become
// running states and diagnostics. All artifacts surface in both modes.
func resolveBrowserOptInCandidates(ctx context.Context, opts Options, res *optInResolution) {
	gate := runningGate{detect: opts.DetectRunningApplications}
	preStates := opts.DetectRunningApplications(ctx)
	// Report only supported browser identities gated by this path, even when
	// the shared detector returns developer-tool or editor states.
	res.runningStates = mergeRunningApplicationStates(
		res.runningStates,
		projectRunningApplicationStates(preStates, browserRunningApplicationIdentities()...)...,
	)
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
				res.runningStates = mergeRunningApplicationStates(res.runningStates, *outcome.postState)
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
				PlannedAction: plannedActionForCategory(OpportunityCategoryBrowserCache),
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
