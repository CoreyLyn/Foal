package clean

import (
	"context"
	"fmt"
)

// runningGate centralizes running-application gating: the tier table (which
// opt-in categories need which check), the three-state fail-closed rule
// (unknown never means idle), and the browser pre/discover/post sequence.
// Discovery is injected so the gate is testable without filesystem access.
//
// Two consumers cross this seam: the opt-in candidate resolver and the dry-run
// review projection. The running/idle/unknown rule and the tier policy live
// here once, not re-derived at each call site.
type runningGate struct {
	// detect returns the current running-application states. It is used for
	// the browser post-discovery re-check. Callers that only gate dev caches
	// (single-check, against pre-detected states) may leave it nil.
	detect func(context.Context) []RunningApplicationState
}

// runningGateTier describes how a category is gated against running apps.
type runningGateTier int

const (
	// runningGateTierNone: shared-runtime tools (npm/pip/corepack) run under a
	// shared runtime and cannot be attributed to the tool, so no check is made.
	runningGateTierNone runningGateTier = iota
	// runningGateTierSingle: distinctive-process tools (go/cargo/nuget) get one
	// state check before cleanup.
	runningGateTierSingle
)

// devCacheGateTier returns the gate tier for a developer-tool cache category.
func devCacheGateTier(category string) runningGateTier {
	switch category {
	case DevCacheCategoryGo, DevCacheCategoryCargo, DevCacheCategoryNuGet:
		return runningGateTierSingle
	default:
		return runningGateTierNone
	}
}

// devCacheCategoryToApplications maps a dev-cache category to the application(s)
// whose running state indicates the cache is in use.
func devCacheCategoryToApplications(category string) []string {
	switch category {
	case DevCacheCategoryGo:
		return []string{ApplicationGo}
	case DevCacheCategoryCargo:
		return []string{ApplicationCargo}
	case DevCacheCategoryNuGet:
		// Both dotnet.exe and nuget.exe can use the nuget cache.
		return []string{ApplicationDotNet, ApplicationNuGet}
	default:
		// Runtime-hosted tools (npm, pip, corepack) don't need checks.
		return nil
	}
}

// devCacheGateOutcome is the result of gating a dev-cache category.
type devCacheGateOutcome struct {
	// proceed is true when the category may be cleaned (no tier, or the tool
	// is idle). When false, skipReason is set.
	proceed bool
	// skipReason is the structured skip reason when a tool is running or its
	// state is unknown. nil when proceed is true or the category has no tier.
	skipReason *StructuredIssue
}

// gateDevCache applies the single-check tier for a dev-cache category against
// already-detected states. None-tier categories always proceed. path and
// category are included in the skip reason so callers can build a SkippedItem
// directly. gateDevCache does not measure bytes; the caller fresh-measures.
func (g runningGate) gateDevCache(category, path string, states []RunningApplicationState) devCacheGateOutcome {
	if devCacheGateTier(category) == runningGateTierNone {
		return devCacheGateOutcome{proceed: true}
	}
	apps := devCacheCategoryToApplications(category)
	for _, app := range apps {
		state, ok := runningApplicationStateFor(states, app)
		if !ok || state.State == RunningApplicationStateRunning || state.State == RunningApplicationStateUnknown {
			reason := devToolRunningSkipIssue(apps, path, category)
			return devCacheGateOutcome{proceed: false, skipReason: &reason}
		}
	}
	return devCacheGateOutcome{proceed: true}
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
