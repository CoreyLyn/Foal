package clean

import (
	"context"
	"fmt"
	"strings"
)

// runningGate centralizes running-application gating: the tier table (which
// opt-in categories need which check), the three-state fail-closed rule
// (unknown never means idle), browser pre/discover/post, Application cache
// pre/inspect/post, and distinctive-process developer-cache pre/measure/post.
// Discovery and measurement are injected so the gate is testable without
// filesystem access.
//
// Outcomes already carry projected RunningStates for the scoped applications
// and structured unknown Diagnostics where the surface reports them as Errors.
// Callers merge/append via runningGateOutcome.apply; they must not re-derive
// fail-closed policy or re-project post states ad hoc.
//
// Single consumer of the gate semantics is resolveCategoryCore (and its
// per-category helpers). Opt-in merge and dry-run review only project the
// resulting categoryCoreResult. The running/idle/unknown rule and the tier
// policy live here once, not re-derived at each surface.
//
// State merge/project helpers in running_states.go are part of this module.
type runningGate struct {
	// detect returns the current running-application states. Used for the
	// browser post-discovery re-check and the distinctive-process developer-
	// cache post-measurement re-check. Callers that only need a single-shot
	// state list may leave it nil when they never enter those paths.
	detect func(context.Context) []RunningApplicationState
}

// runningGateTier describes how a category is gated against running apps.
type runningGateTier int

const (
	// runningGateTierNone: shared-runtime tools (npm/pip/corepack) run under a
	// shared runtime and cannot be attributed to the tool, so no check is made.
	runningGateTierNone runningGateTier = iota
	// runningGateTierBeforeAfter: distinctive-process tools (go/cargo/nuget)
	// must be idle before measurement and again after measurement before the
	// root becomes an Opt-in candidate.
	runningGateTierBeforeAfter
)

// runningGateOutcome is the shared base of every running-application gate
// result. Gate methods fill projected RunningStates and (when applicable)
// unknown Diagnostics so consumers stop re-implementing merge/project policy.
//
// Three-state semantics (ADR 0007 / 0010): running, idle, and unknown are
// distinct; unknown never means idle. Shared-runtime categories use
// runningGateTierNone and never spuriously trigger distinctive-process
// detection (ADR 0008). Product-scoped JetBrains gates pass a single
// application identity so each logical product stays independent (ADR 0017).
type runningGateOutcome struct {
	// proceed is true when the injected work may surface reclaimable evidence.
	// False for pre fail-closed, post discard, measurement failure, or
	// discovery that is not clean enough to reclaim.
	proceed bool
	// runningStates are the authoritative projected observations for the
	// applications this gate scoped. When a post re-check ran, post
	// supersedes pre for matching identities without reordering.
	// Callers merge as-is; empty when the gate did not observe any scoped app.
	runningStates []RunningApplicationState
	// diagnostics are recoverable unknown-state issues already built for
	// scoped applications observed as Unknown when the surface reports them
	// as Result.Errors (browser / Application cache). Distinctive-process
	// developer-cache gates leave this empty and use skipReason (SkippedItem)
	// instead for the same fail-closed signal.
	diagnostics []StructuredIssue
}

// apply merges projected running states and unknown diagnostics into the
// destinations. Nil destinations are skipped. Safe on a zero outcome.
func (o runningGateOutcome) apply(dstStates *[]RunningApplicationState, dstDiag *[]StructuredIssue) {
	if dstStates != nil && len(o.runningStates) > 0 {
		*dstStates = mergeRunningApplicationStates(*dstStates, o.runningStates...)
	}
	if dstDiag != nil && len(o.diagnostics) > 0 {
		*dstDiag = append(*dstDiag, o.diagnostics...)
	}
}

// measureGateOutcome is the gate result around an injected size measurement
// (whole-root developer caches and product-scoped roots that measure one path).
type measureGateOutcome struct {
	runningGateOutcome
	// bytes is the measured size when proceed is true. Post-gate discards do
	// not expose measured bytes here so callers cannot reclaim them.
	bytes int64
	// measureErr is set when measurement failed or was canceled. When set,
	// proceed is false, skipReason is nil, and the post re-check did not run
	// (cancellation must not be re-authorized by a second idle check).
	measureErr error
	// skipReason is the structured skip reason when a tool is running, unknown,
	// or missing required state on the pre- or post-check. nil when proceed is
	// true or when measurement failed without a gate skip.
	skipReason *StructuredIssue
}

// browserGateOutcome is the result of gating a browser cache discovery.
type browserGateOutcome struct {
	runningGateOutcome
	// discoveryRan is true when the browser was idle before discovery and
	// discovery executed. When false, discovery is zero and proceed is false.
	discoveryRan bool
	// discovery is the injected discovery result when discoveryRan is true.
	discovery browserCacheDiscoveryResult
}

// applicationCacheGateOutcome is the result of gating idle Application cache
// discovery with pre/inspect/post application process checks.
type applicationCacheGateOutcome struct {
	runningGateOutcome
	// discoveryRan is true when the application was idle before discovery and
	// discovery executed. When false, discovery is zero and proceed is false.
	discoveryRan bool
	// discovery is the injected discovery result when discoveryRan is true.
	discovery applicationCacheDiscoveryResult
}

// devCacheGateTier returns the gate tier for a developer-tool cache category.
func devCacheGateTier(category string) runningGateTier {
	entry, ok := canonicalCategoryEntry(category)
	if ok && entry.definition.RunningApplicationPolicy == RunningApplicationPolicyDistinctiveProcessIdle {
		return runningGateTierBeforeAfter
	}
	return runningGateTierNone
}

// planNeedsDistinctiveProcessDetection reports whether the opt-in plan selects
// any distinctive-process developer-cache category. Shared-runtime selections
// alone must not trigger developer-tool process detection.
func planNeedsDistinctiveProcessDetection(plan map[string]bool) bool {
	for category, enabled := range plan {
		if !enabled {
			continue
		}
		if isDevCacheCategory(category) && devCacheGateTier(category) == runningGateTierBeforeAfter {
			return true
		}
	}
	return false
}

// devCacheCategoryToApplications maps a dev-cache category to the application(s)
// whose running state indicates the cache is in use.
func devCacheCategoryToApplications(category string) []string {
	entry, ok := canonicalCategoryEntry(category)
	if !ok {
		return nil
	}
	return append([]string(nil), entry.runningApplications...)
}

// appsIdleForDevCache reports whether every application tied to the category is
// idle in states. Missing state, running, and unknown all fail closed.
func appsIdleForDevCache(category, path string, states []RunningApplicationState) (bool, *StructuredIssue) {
	return appsIdleForApplications(devCacheCategoryToApplications(category), path, category, states)
}

// appsIdleForApplications reports whether every listed application is idle in
// states. Missing state, running, and unknown all fail closed. Used by both
// category-wide distinctive-process gates and product-scoped root gates.
func appsIdleForApplications(apps []string, path, category string, states []RunningApplicationState) (bool, *StructuredIssue) {
	for _, app := range apps {
		state, ok := runningApplicationStateFor(states, app)
		if !ok || state.State == RunningApplicationStateRunning || state.State == RunningApplicationStateUnknown {
			reason := devToolRunningSkipIssue(apps, path, category)
			return false, &reason
		}
	}
	return true, nil
}

// gateApplicationsForDevCacheScope selects the applications and whether
// idle-before-and-after gating applies for one resolved root scope.
//
// Product-scoped roots (non-empty Application) always use before/after for that
// single identity. Unscoped roots keep category-wide distinctive-process policy
// or shared-runtime none-tier behavior.
func gateApplicationsForDevCacheScope(category string, scope DevCacheRootScope) (apps []string, useGate bool) {
	if app := strings.TrimSpace(scope.Application); app != "" {
		return []string{app}, true
	}
	if devCacheGateTier(category) == runningGateTierBeforeAfter {
		return devCacheCategoryToApplications(category), true
	}
	return nil, false
}

// observeDevCacheApps projects scoped application identities from a detector
// snapshot and evaluates fail-closed idle. Used by multi-child structured
// developer-cache paths that cannot wrap a single measure function, and by
// whole-root measure gates for shared projection policy.
//
// Returns idle=false with skipReason when any app is running, unknown, or
// missing. runningStates always hold the projected observations so callers can
// report product-scoped identities even when the gate fails.
func observeDevCacheApps(apps []string, path, category string, states []RunningApplicationState) (idle bool, projected []RunningApplicationState, skipReason *StructuredIssue) {
	projected = projectRunningApplicationStates(states, apps...)
	if ok, reason := appsIdleForApplications(apps, path, category, states); !ok {
		return false, projected, reason
	}
	return true, projected, nil
}

// gateDevCacheRoot applies the developer-cache running-application gate around
// an injected measurement for one resolved root using category-wide applications.
//
// None-tier categories measure immediately with no process check.
// Before/after-tier categories require all related applications idle in
// preStates before measuring; after a successful measurement they re-detect
// via g.detect and require idle again before proceed. A failed or canceled
// measurement never runs the post re-check.
func (g runningGate) gateDevCacheRoot(
	ctx context.Context,
	category, path string,
	preStates []RunningApplicationState,
	measure func() (int64, error),
) measureGateOutcome {
	if devCacheGateTier(category) == runningGateTierNone {
		return g.gateDevCacheApplications(ctx, category, path, nil, false, preStates, measure)
	}
	return g.gateDevCacheApplications(ctx, category, path, devCacheCategoryToApplications(category), true, preStates, measure)
}

// gateDevCacheApplications applies pre/measure/post gating for an explicit
// application list. When useGate is false the measurement runs immediately.
// Outcome.runningStates project the scoped apps (pre, then post supersedes)
// whenever useGate is true so product-scoped callers can apply them without
// re-projecting.
func (g runningGate) gateDevCacheApplications(
	ctx context.Context,
	category, path string,
	apps []string,
	useGate bool,
	preStates []RunningApplicationState,
	measure func() (int64, error),
) measureGateOutcome {
	if !useGate {
		bytes, err := measure()
		if err != nil {
			return measureGateOutcome{measureErr: err}
		}
		return measureGateOutcome{runningGateOutcome: runningGateOutcome{proceed: true}, bytes: bytes}
	}

	idle, projected, reason := observeDevCacheApps(apps, path, category, preStates)
	if !idle {
		return measureGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: projected},
			skipReason:         reason,
		}
	}

	bytes, err := measure()
	if err != nil {
		// Incomplete measurement: do not run the post re-check and do not
		// surface partial bytes as reclaimable evidence. Keep pre projection.
		return measureGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: projected},
			measureErr:         err,
		}
	}

	var postStates []RunningApplicationState
	if g.detect != nil {
		postStates = g.detect(ctx)
	}
	postIdle, postProjected, postReason := observeDevCacheApps(apps, path, category, postStates)
	// Post projection supersedes pre for matching identities; merge preserves
	// first-seen order among the scoped apps.
	merged := mergeRunningApplicationStates(projected, postProjected...)
	if !postIdle {
		// Discard the successful measurement: no candidate bytes.
		return measureGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: merged},
			skipReason:         postReason,
		}
	}
	return measureGateOutcome{
		runningGateOutcome: runningGateOutcome{proceed: true, runningStates: merged},
		bytes:              bytes,
	}
}

// devToolRunningSkipIssue builds the skip reason for a dev cache gated out by a
// running or unknown tool state.
func devToolRunningSkipIssue(apps []string, path, category string) StructuredIssue {
	appNames := make([]string, 0, len(apps))
	for _, app := range apps {
		appNames = append(appNames, applicationDisplayName(app))
	}
	var message string
	switch len(appNames) {
	case 1:
		message = fmt.Sprintf("%s is running or its state could not be determined; skipping dev cache cleanup", appNames[0])
	default:
		message = fmt.Sprintf("%s or %s is running or their state could not be determined; skipping dev cache cleanup", appNames[0], appNames[1])
	}
	return issue(devToolRunningIssueCode, message, true, path, category)
}

// projectSingleApplicationState returns the state for application when present.
func projectSingleApplicationState(states []RunningApplicationState, application string) []RunningApplicationState {
	if state, ok := runningApplicationStateFor(states, application); ok {
		return []RunningApplicationState{state}
	}
	return nil
}

// unknownDiagnosticsForStates builds recoverable unknown-state diagnostics for
// any projected state that is Unknown. Used by browser / Application cache
// gates so callers append diagnostics without re-deriving the issue shape.
func unknownDiagnosticsForStates(states []RunningApplicationState) []StructuredIssue {
	var out []StructuredIssue
	for _, state := range states {
		if state.State == RunningApplicationStateUnknown {
			out = append(out, runningApplicationUnknownIssue(state))
		}
	}
	return out
}

// gateBrowser runs the pre/discover/post running-application gate around an
// injected browser cache discovery. preStates are detected once by the caller
// and shared across browsers; the gate performs the post re-check itself.
//
// The post re-check is skipped when discovery is suppressed, diagnostic, or
// incomplete (there is no opportunity to act on). When discovery is clean the
// post re-check runs even when the opportunity is empty, so a browser that
// starts during discovery is still reported. The gate requires g.detect != nil;
// callers only gate browsers when a detector is present.
//
// Outcome.runningStates always project the scoped browser identity when present
// in preStates (and post supersedes on re-check). Outcome.diagnostics hold
// unknown-state issues when a post re-check observed Unknown.
func (g runningGate) gateBrowser(ctx context.Context, application string, preStates []RunningApplicationState, discover func() browserCacheDiscoveryResult) browserGateOutcome {
	preProjected := projectSingleApplicationState(preStates, application)
	preState, ok := runningApplicationStateFor(preStates, application)
	if !ok || preState.State != RunningApplicationStateIdle {
		// Pre fail-closed: still surface the projected pre state (running /
		// unknown) so category resolution can report it. Unknown pre-state
		// diagnostics for review are emitted from RunningStates by the review
		// projector; opt-in does not invent Errors from pre alone.
		return browserGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: preProjected},
		}
	}
	discovery := discover()
	if discovery.suppressed || discovery.diagnostic != nil || discovery.incomplete != nil {
		// Not-clean discovery: no post re-check, no reclaimable proceed.
		return browserGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: preProjected},
			discoveryRan:       true,
			discovery:          discovery,
		}
	}
	postStates := g.detect(ctx)
	postProjected := projectSingleApplicationState(postStates, application)
	merged := mergeRunningApplicationStates(preProjected, postProjected...)
	postState, ok := runningApplicationStateFor(postStates, application)
	if !ok {
		return browserGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: merged},
			discoveryRan:       true,
			discovery:          discovery,
		}
	}
	if postState.State != RunningApplicationStateIdle {
		return browserGateOutcome{
			runningGateOutcome: runningGateOutcome{
				runningStates: merged,
				diagnostics:   unknownDiagnosticsForStates(postProjected),
			},
			discoveryRan: true,
			discovery:    discovery,
		}
	}
	return browserGateOutcome{
		runningGateOutcome: runningGateOutcome{proceed: true, runningStates: merged},
		discoveryRan:       true,
		discovery:          discovery,
	}
}

// gateApplicationCache runs pre/discover/post idle gating around an injected
// Application cache discovery for one logical application.
//
// Outcome.runningStates project the scoped application (pre, then post
// supersedes). Outcome.diagnostics hold unknown post-state issues. When
// discovery is canceled, post re-check is skipped so measured roots cannot be
// reauthorized (proceed stays false).
func (g runningGate) gateApplicationCache(
	ctx context.Context,
	application string,
	preStates []RunningApplicationState,
	discover func() applicationCacheDiscoveryResult,
) applicationCacheGateOutcome {
	preProjected := projectSingleApplicationState(preStates, application)
	preState, ok := runningApplicationStateFor(preStates, application)
	if !ok || preState.State != RunningApplicationStateIdle {
		return applicationCacheGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: preProjected},
		}
	}
	discovery := discover()
	if discovery.canceled {
		// Incomplete/canceled scan: do not post-check reauthorize.
		return applicationCacheGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: preProjected},
			discoveryRan:       true,
			discovery:          discovery,
		}
	}
	// Post re-check always runs after a non-canceled discovery so an app that
	// starts during root inspection discards every measured root.
	var postStates []RunningApplicationState
	if g.detect != nil {
		postStates = g.detect(ctx)
	}
	postProjected := projectSingleApplicationState(postStates, application)
	merged := mergeRunningApplicationStates(preProjected, postProjected...)
	postState, ok := runningApplicationStateFor(postStates, application)
	if !ok {
		return applicationCacheGateOutcome{
			runningGateOutcome: runningGateOutcome{runningStates: merged},
			discoveryRan:       true,
			discovery:          discovery,
		}
	}
	if postState.State != RunningApplicationStateIdle {
		return applicationCacheGateOutcome{
			runningGateOutcome: runningGateOutcome{
				runningStates: merged,
				diagnostics:   unknownDiagnosticsForStates(postProjected),
			},
			discoveryRan: true,
			discovery:    discovery,
		}
	}
	return applicationCacheGateOutcome{
		runningGateOutcome: runningGateOutcome{proceed: true, runningStates: merged},
		discoveryRan:       true,
		discovery:          discovery,
	}
}
