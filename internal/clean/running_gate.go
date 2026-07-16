package clean

import (
	"context"
	"fmt"
	"strings"
)

// runningGate centralizes running-application gating: the tier table (which
// opt-in categories need which check), the three-state fail-closed rule
// (unknown never means idle), browser pre/discover/post, and distinctive-
// process developer-cache pre/measure/post. Discovery and measurement are
// injected so the gate is testable without filesystem access.
//
// Two consumers cross this seam: the opt-in candidate resolver and the dry-run
// review projection. The running/idle/unknown rule and the tier policy live
// here once, not re-derived at each call site.
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

// devCacheGateOutcome is the result of gating a developer-cache root through
// the tier-appropriate running-application checks around measurement.
type devCacheGateOutcome struct {
	// proceed is true when the root may become an Opt-in candidate: either the
	// category has no running-application tier, or every related application
	// was idle before and after a successful measurement.
	proceed bool
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
	// postStates is the post-measurement detector snapshot when a post re-check
	// ran. Callers may project product-scoped identities from it; nil when the
	// post re-check did not run.
	postStates []RunningApplicationState
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
) devCacheGateOutcome {
	if devCacheGateTier(category) == runningGateTierNone {
		return g.gateDevCacheApplications(ctx, category, path, nil, false, preStates, measure)
	}
	return g.gateDevCacheApplications(ctx, category, path, devCacheCategoryToApplications(category), true, preStates, measure)
}

// gateDevCacheApplications applies pre/measure/post gating for an explicit
// application list. When useGate is false the measurement runs immediately.
// postStates are returned when a post re-check ran so callers can project the
// latest authoritative observation for product-scoped identities.
func (g runningGate) gateDevCacheApplications(
	ctx context.Context,
	category, path string,
	apps []string,
	useGate bool,
	preStates []RunningApplicationState,
	measure func() (int64, error),
) devCacheGateOutcome {
	if !useGate {
		bytes, err := measure()
		if err != nil {
			return devCacheGateOutcome{measureErr: err}
		}
		return devCacheGateOutcome{proceed: true, bytes: bytes}
	}

	if idle, reason := appsIdleForApplications(apps, path, category, preStates); !idle {
		return devCacheGateOutcome{skipReason: reason}
	}

	bytes, err := measure()
	if err != nil {
		// Incomplete measurement: do not run the post re-check and do not
		// surface partial bytes as reclaimable evidence.
		return devCacheGateOutcome{measureErr: err}
	}

	var postStates []RunningApplicationState
	if g.detect != nil {
		postStates = g.detect(ctx)
	}
	if idle, reason := appsIdleForApplications(apps, path, category, postStates); !idle {
		// Discard the successful measurement: no candidate bytes.
		return devCacheGateOutcome{skipReason: reason, postStates: postStates}
	}
	return devCacheGateOutcome{proceed: true, bytes: bytes, postStates: postStates}
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

// browserGateOutcome is the result of gating a browser cache discovery.
type browserGateOutcome struct {
	// preIdle is false when the browser was not idle before discovery
	// (running, unknown, or absent from preStates). When false, discovery was
	// not run and the other fields are zero.
	preIdle bool
	// discovery is the injected discovery result. Zero-valued when preIdle is
	// false.
	discovery browserCacheDiscoveryResult
	// postIdle is false when the browser was not idle after discovery. Only
	// meaningful when preIdle is true and discovery was clean (not suppressed,
	// no diagnostic, no incomplete); true otherwise (no post re-check ran).
	postIdle bool
	// postState is the post-discovery state when postIdle is false and the
	// state was present. nil when the state was absent or postIdle is true.
	postState *RunningApplicationState
	// postDiagnostic is the unknown-state error when postIdle is false and the
	// post state was unknown. nil otherwise.
	postDiagnostic *StructuredIssue
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
func (g runningGate) gateBrowser(ctx context.Context, application string, preStates []RunningApplicationState, discover func() browserCacheDiscoveryResult) browserGateOutcome {
	preState, ok := runningApplicationStateFor(preStates, application)
	if !ok || preState.State != RunningApplicationStateIdle {
		return browserGateOutcome{preIdle: false}
	}
	discovery := discover()
	if discovery.suppressed || discovery.diagnostic != nil || discovery.incomplete != nil {
		return browserGateOutcome{preIdle: true, discovery: discovery, postIdle: true}
	}
	postStates := g.detect(ctx)
	postState, ok := runningApplicationStateFor(postStates, application)
	if !ok {
		return browserGateOutcome{preIdle: true, discovery: discovery, postIdle: false}
	}
	if postState.State != RunningApplicationStateIdle {
		outcome := browserGateOutcome{preIdle: true, discovery: discovery, postIdle: false, postState: &postState}
		if postState.State == RunningApplicationStateUnknown {
			diagnostic := runningApplicationUnknownIssue(postState)
			outcome.postDiagnostic = &diagnostic
		}
		return outcome
	}
	return browserGateOutcome{preIdle: true, discovery: discovery, postIdle: true}
}

// applicationCacheGateOutcome is the result of gating idle Application cache
// discovery with pre/inspect/post application process checks.
type applicationCacheGateOutcome struct {
	// preIdle is false when the application was not idle before discovery.
	// Discovery was not run and other fields are zero.
	preIdle bool
	// discovery is the injected discovery result when preIdle is true.
	discovery applicationCacheDiscoveryResult
	// postIdle is false when the application was not idle after discovery.
	// When discovery is canceled, post re-check is skipped (postIdle false,
	// postState nil) so measured roots cannot be reauthorized.
	postIdle bool
	// postState is set when postIdle is false and a post state was present.
	postState *RunningApplicationState
	// postDiagnostic is set for unknown post state.
	postDiagnostic *StructuredIssue
}

// gateApplicationCache runs pre/discover/post idle gating around an injected
// Application cache discovery for one logical application.
func (g runningGate) gateApplicationCache(
	ctx context.Context,
	application string,
	preStates []RunningApplicationState,
	discover func() applicationCacheDiscoveryResult,
) applicationCacheGateOutcome {
	preState, ok := runningApplicationStateFor(preStates, application)
	if !ok || preState.State != RunningApplicationStateIdle {
		return applicationCacheGateOutcome{preIdle: false}
	}
	discovery := discover()
	if discovery.canceled {
		// Incomplete/canceled scan: do not post-check reauthorize.
		return applicationCacheGateOutcome{preIdle: true, discovery: discovery, postIdle: false}
	}
	// Post re-check always runs after a non-canceled discovery so an app that
	// starts during root inspection discards every measured root.
	var postStates []RunningApplicationState
	if g.detect != nil {
		postStates = g.detect(ctx)
	}
	postState, ok := runningApplicationStateFor(postStates, application)
	if !ok {
		return applicationCacheGateOutcome{preIdle: true, discovery: discovery, postIdle: false}
	}
	if postState.State != RunningApplicationStateIdle {
		outcome := applicationCacheGateOutcome{preIdle: true, discovery: discovery, postIdle: false, postState: &postState}
		if postState.State == RunningApplicationStateUnknown {
			diagnostic := runningApplicationUnknownIssue(postState)
			outcome.postDiagnostic = &diagnostic
		}
		return outcome
	}
	return applicationCacheGateOutcome{preIdle: true, discovery: discovery, postIdle: true}
}
