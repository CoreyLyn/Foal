package clean

import (
	"context"
	"errors"
	"testing"
)

func TestDevCacheGateTier(t *testing.T) {
	for _, c := range []string{DevCacheCategoryNPM, DevCacheCategoryPip, DevCacheCategoryCorepack, DevCacheCategoryPlaywright, DevCacheCategoryElectron} {
		if got := devCacheGateTier(c); got != runningGateTierNone {
			t.Errorf("devCacheGateTier(%q) = %v, want none", c, got)
		}
	}
	for _, c := range []string{DevCacheCategoryGo, DevCacheCategoryCargo, DevCacheCategoryNuGet, DevCacheCategoryNuGetGlobalPackages, DevCacheCategoryUV, DevCacheCategoryBun, DevCacheCategoryJetBrainsIDECaches, DevCacheCategoryVisualStudioCaches} {
		if got := devCacheGateTier(c); got != runningGateTierBeforeAfter {
			t.Errorf("devCacheGateTier(%q) = %v, want before/after", c, got)
		}
	}
}

func TestPlanNeedsDistinctiveProcessDetection(t *testing.T) {
	if planNeedsDistinctiveProcessDetection(nil) {
		t.Fatal("empty plan should not need detection")
	}
	if planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryNPM: true}) {
		t.Fatal("shared-runtime only should not need distinctive-process detection")
	}
	if planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryPlaywright: true}) {
		t.Fatal("playwright-browsers shared-runtime must not trigger process detection")
	}
	if planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryElectron: true}) {
		t.Fatal("electron-cache shared-runtime must not trigger process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryGo: true}) {
		t.Fatal("go-cache should need distinctive-process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryUV: true}) {
		t.Fatal("uv-cache should need distinctive-process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryBun: true}) {
		t.Fatal("bun-cache should need distinctive-process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryJetBrainsIDECaches: true}) {
		t.Fatal("jetbrains-ide-caches should need distinctive-process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryVisualStudioCaches: true}) {
		t.Fatal("visual-studio-caches should need distinctive-process detection")
	}
	if !planNeedsDistinctiveProcessDetection(map[string]bool{
		DevCacheCategoryNPM: true,
		DevCacheCategoryGo:  true,
	}) {
		t.Fatal("mixed plan with go-cache should need detection")
	}
}

func TestGateDevCacheRootNoneTierAlwaysProceeds(t *testing.T) {
	// None-tier proceeds even when an unrelated tool is running; no detect call.
	detectCalled := false
	g := runningGate{detect: func(context.Context) []RunningApplicationState {
		detectCalled = true
		return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
	}}
	pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
	for _, category := range []string{DevCacheCategoryNPM, DevCacheCategoryPip, DevCacheCategoryCorepack} {
		detectCalled = false
		measureCalled := false
		outcome := g.gateDevCacheRoot(context.Background(), category, `C:\cache`, pre, func() (int64, error) {
			measureCalled = true
			return 42, nil
		})
		if !outcome.proceed || outcome.skipReason != nil || outcome.bytes != 42 {
			t.Fatalf("none-tier %q: outcome = %+v, want proceed with 42 bytes", category, outcome)
		}
		if len(outcome.runningStates) != 0 {
			t.Fatalf("none-tier %q: runningStates should be empty, got %#v", category, outcome.runningStates)
		}
		if !measureCalled {
			t.Fatalf("none-tier %q: measure should run", category)
		}
		if detectCalled {
			t.Fatalf("none-tier %q: post detect should not run", category)
		}
	}
}

func TestGateDevCacheRootStateSequences(t *testing.T) {
	const path = `C:\go-cache`
	const measured int64 = 100

	// Controllable detector sequences: each post-check call returns the next state.
	newGate := func(states ...RunningApplicationStatus) (runningGate, *int) {
		call := 0
		g := runningGate{detect: func(context.Context) []RunningApplicationState {
			state := states[len(states)-1]
			if call < len(states) {
				state = states[call]
			}
			call++
			return []RunningApplicationState{{Application: ApplicationGo, State: state}}
		}}
		return g, &call
	}

	t.Run("idle to idle proceeds with measured bytes", func(t *testing.T) {
		g, detectCalls := newGate(RunningApplicationStateIdle)
		// preStates supplied by caller (first check); post uses detect.
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		measureCalled := false
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			measureCalled = true
			return measured, nil
		})
		if !outcome.proceed || outcome.skipReason != nil || outcome.bytes != measured {
			t.Fatalf("idle→idle: outcome = %+v, want proceed with %d", outcome, measured)
		}
		if !measureCalled {
			t.Fatal("idle→idle: measure should run")
		}
		if *detectCalls != 1 {
			t.Fatalf("idle→idle: post detect calls = %d, want 1", *detectCalls)
		}
		if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateIdle {
			t.Fatalf("idle→idle: runningStates = %#v, want projected idle go", outcome.runningStates)
		}
	})

	t.Run("idle to running discards measurement", func(t *testing.T) {
		g, _ := newGate(RunningApplicationStateRunning)
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || outcome.bytes != 0 {
			t.Fatalf("idle→running: outcome = %+v, want skip discard bytes", outcome)
		}
		if outcome.skipReason.Code != devToolRunningIssueCode {
			t.Errorf("skip code = %q, want %q", outcome.skipReason.Code, devToolRunningIssueCode)
		}
		// Post running supersedes pre idle on the projected outcome.
		if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateRunning {
			t.Fatalf("idle→running: runningStates = %#v, want post running", outcome.runningStates)
		}
	})

	t.Run("idle to unknown discards measurement", func(t *testing.T) {
		g, _ := newGate(RunningApplicationStateUnknown)
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || outcome.bytes != 0 {
			t.Fatalf("idle→unknown: outcome = %+v, want skip discard bytes", outcome)
		}
		// Dev-cache unknown is skipReason, not Errors diagnostics.
		if len(outcome.diagnostics) != 0 {
			t.Fatalf("idle→unknown: diagnostics = %#v, want empty (skipReason surface)", outcome.diagnostics)
		}
	})

	t.Run("idle to missing discards measurement", func(t *testing.T) {
		// Post detect returns empty states → missing required application state.
		g := runningGate{detect: func(context.Context) []RunningApplicationState { return nil }}
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || outcome.bytes != 0 {
			t.Fatalf("idle→missing: outcome = %+v, want skip discard bytes", outcome)
		}
	})

	t.Run("running at start skips measurement", func(t *testing.T) {
		detectCalled := false
		g := runningGate{detect: func(context.Context) []RunningApplicationState {
			detectCalled = true
			return nil
		}}
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
		measureCalled := false
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			measureCalled = true
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil {
			t.Fatalf("running-at-start: outcome = %+v, want skip", outcome)
		}
		if measureCalled {
			t.Fatal("running-at-start: measure must not run")
		}
		if detectCalled {
			t.Fatal("running-at-start: post detect must not run")
		}
		if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateRunning {
			t.Fatalf("running-at-start: runningStates = %#v, want pre running projected", outcome.runningStates)
		}
	})

	t.Run("unknown at start skips measurement", func(t *testing.T) {
		g := runningGate{}
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateUnknown}}
		measureCalled := false
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			measureCalled = true
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || measureCalled {
			t.Fatalf("unknown-at-start: outcome = %+v measureCalled=%v, want skip without measure", outcome, measureCalled)
		}
	})

	t.Run("missing state at start skips measurement", func(t *testing.T) {
		g := runningGate{}
		measureCalled := false
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, nil, func() (int64, error) {
			measureCalled = true
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || measureCalled {
			t.Fatalf("missing-at-start: outcome = %+v measureCalled=%v, want skip without measure", outcome, measureCalled)
		}
	})

	t.Run("measure failure skips post re-check", func(t *testing.T) {
		detectCalled := false
		g := runningGate{detect: func(context.Context) []RunningApplicationState {
			detectCalled = true
			// Would be idle — must not re-authorize after incomplete measure.
			return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		}}
		pre := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
		measureErr := errors.New("canceled")
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryGo, path, pre, func() (int64, error) {
			return 0, measureErr
		})
		if outcome.proceed || outcome.skipReason != nil || !errors.Is(outcome.measureErr, measureErr) {
			t.Fatalf("measure failure: outcome = %+v, want measureErr only", outcome)
		}
		if detectCalled {
			t.Fatal("measure failure: post detect must not re-authorize")
		}
	})

	t.Run("nuget requires all related apps idle on both checks", func(t *testing.T) {
		// Pre: both idle. Post: dotnet idle, nuget running → fail closed.
		g := runningGate{detect: func(context.Context) []RunningApplicationState {
			return []RunningApplicationState{
				{Application: ApplicationDotNet, State: RunningApplicationStateIdle},
				{Application: ApplicationNuGet, State: RunningApplicationStateRunning},
			}
		}}
		pre := []RunningApplicationState{
			{Application: ApplicationDotNet, State: RunningApplicationStateIdle},
			{Application: ApplicationNuGet, State: RunningApplicationStateIdle},
		}
		outcome := g.gateDevCacheRoot(context.Background(), DevCacheCategoryNuGet, path, pre, func() (int64, error) {
			return measured, nil
		})
		if outcome.proceed || outcome.skipReason == nil || outcome.bytes != 0 {
			t.Fatalf("nuget multi-app post fail: outcome = %+v, want skip", outcome)
		}

		// Pre: nuget running → no measure
		measureCalled := false
		outcome = g.gateDevCacheRoot(context.Background(), DevCacheCategoryNuGet, path, []RunningApplicationState{
			{Application: ApplicationDotNet, State: RunningApplicationStateIdle},
			{Application: ApplicationNuGet, State: RunningApplicationStateRunning},
		}, func() (int64, error) {
			measureCalled = true
			return measured, nil
		})
		if outcome.proceed || measureCalled {
			t.Fatalf("nuget multi-app pre fail: proceed=%v measureCalled=%v", outcome.proceed, measureCalled)
		}
	})
}

func TestGateBrowserPreNotIdleSkipsDiscovery(t *testing.T) {
	g := runningGate{detect: func(context.Context) []RunningApplicationState { return nil }}
	discoverCalled := false
	discover := func() browserCacheDiscoveryResult {
		discoverCalled = true
		return browserCacheDiscoveryResult{}
	}
	for _, pre := range []RunningApplicationState{
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning},
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown},
	} {
		discoverCalled = false
		outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, []RunningApplicationState{pre}, discover)
		if outcome.discoveryRan || outcome.proceed {
			t.Fatalf("pre %v: discoveryRan/proceed = true, want false", pre.State)
		}
		if discoverCalled {
			t.Fatalf("pre %v: discover should not run", pre.State)
		}
		if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != pre.State {
			t.Fatalf("pre %v: runningStates = %#v, want projected pre state", pre.State, outcome.runningStates)
		}
	}
	// absent pre-state -> no discovery, no projected state
	discoverCalled = false
	outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, nil, discover)
	if outcome.discoveryRan || outcome.proceed || discoverCalled {
		t.Fatalf("absent pre-state: discoveryRan=%v proceed=%v discoverCalled=%v, want false", outcome.discoveryRan, outcome.proceed, discoverCalled)
	}
	if len(outcome.runningStates) != 0 {
		t.Fatalf("absent pre-state: runningStates = %#v, want empty", outcome.runningStates)
	}
}

func TestGateBrowserNotCleanDiscoverySkipsPostCheck(t *testing.T) {
	postCalled := false
	g := runningGate{detect: func(context.Context) []RunningApplicationState {
		postCalled = true
		return nil
	}}
	pre := []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle}}

	cases := []struct {
		name      string
		discovery browserCacheDiscoveryResult
	}{
		{"suppressed", browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: []string{`C:\protected`}}},
		{"diagnostic", browserCacheDiscoveryResult{diagnostic: &StructuredIssue{Code: "browser_profile_catalog_unknown"}}},
		{"incomplete", browserCacheDiscoveryResult{incomplete: &IncompleteOpportunityInspection{Path: `C:\Cache`}}},
	}
	for _, tc := range cases {
		postCalled = false
		discovery := tc.discovery
		outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, func() browserCacheDiscoveryResult { return discovery })
		if !outcome.discoveryRan {
			t.Fatalf("%s: discoveryRan = false, want true", tc.name)
		}
		if outcome.proceed {
			t.Errorf("%s: proceed = true, want false (not-clean discovery is not reclaimable)", tc.name)
		}
		if postCalled {
			t.Errorf("%s: post detect should not run", tc.name)
		}
	}
}

func TestGateBrowserCleanPostIdle(t *testing.T) {
	g := runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle}}
	}}
	opp := &Opportunity{Path: `C:\User Data`, Bytes: 100}
	discover := func() browserCacheDiscoveryResult { return browserCacheDiscoveryResult{opportunity: opp} }
	outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, []RunningApplicationState{
		{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle},
	}, discover)
	if !outcome.discoveryRan || !outcome.proceed {
		t.Fatalf("clean+post-idle: discoveryRan=%v proceed=%v, want true/true", outcome.discoveryRan, outcome.proceed)
	}
	if outcome.discovery.opportunity != opp {
		t.Error("discovery not passed through")
	}
	if len(outcome.diagnostics) != 0 {
		t.Errorf("diagnostics should be empty when proceed: %#v", outcome.diagnostics)
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateIdle {
		t.Fatalf("runningStates = %#v, want idle chrome", outcome.runningStates)
	}
}

func TestGateBrowserPostNotIdle(t *testing.T) {
	opp := &Opportunity{Path: `C:\User Data`, Bytes: 100}
	discover := func() browserCacheDiscoveryResult { return browserCacheDiscoveryResult{opportunity: opp} }
	pre := []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle}}

	// post running -> proceed false, projected running state, no diagnostic
	g := runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning}}
	}}
	outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.proceed {
		t.Fatal("post running: proceed = true, want false")
	}
	if !outcome.discoveryRan {
		t.Fatal("post running: discoveryRan = false, want true")
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateRunning {
		t.Fatalf("post running: runningStates = %#v, want running", outcome.runningStates)
	}
	if len(outcome.diagnostics) != 0 {
		t.Errorf("post running: diagnostics should be empty: %#v", outcome.diagnostics)
	}

	// post unknown -> proceed false, projected unknown, diagnostic set (fail-closed)
	g = runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: "snap failed"}}
	}}
	outcome = g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.proceed {
		t.Fatal("post unknown: proceed = true, want false")
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateUnknown {
		t.Fatalf("post unknown: runningStates = %#v, want unknown", outcome.runningStates)
	}
	if len(outcome.diagnostics) != 1 || outcome.diagnostics[0].Code != runningApplicationDetectionIssueCode {
		t.Fatalf("post unknown: diagnostics = %#v, want code %q", outcome.diagnostics, runningApplicationDetectionIssueCode)
	}

	// post absent -> proceed false, keep pre idle projection, no diagnostic
	g = runningGate{detect: func(context.Context) []RunningApplicationState { return nil }}
	outcome = g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.proceed {
		t.Fatal("post absent: proceed = true, want false")
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateIdle {
		t.Fatalf("post absent: runningStates = %#v, want pre idle retained", outcome.runningStates)
	}
	if len(outcome.diagnostics) != 0 {
		t.Fatalf("post absent: diagnostics = %#v, want empty", outcome.diagnostics)
	}
}

func TestRunningGateOutcomeApplyMergesStatesAndDiagnostics(t *testing.T) {
	var states []RunningApplicationState
	var diags []StructuredIssue
	outcome := runningGateOutcome{
		runningStates: []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning},
		},
		diagnostics: []StructuredIssue{{Code: runningApplicationDetectionIssueCode, Message: "x"}},
	}
	outcome.apply(&states, &diags)
	if len(states) != 1 || states[0].Application != ApplicationGoogleChrome {
		t.Fatalf("states = %#v", states)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %#v", diags)
	}
	// Later post observation supersedes without reordering.
	runningGateOutcome{
		runningStates: []RunningApplicationState{
			{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle},
			{Application: ApplicationMicrosoftEdge, State: RunningApplicationStateIdle},
		},
	}.apply(&states, nil)
	if len(states) != 2 || states[0].State != RunningApplicationStateIdle || states[1].Application != ApplicationMicrosoftEdge {
		t.Fatalf("merged states = %#v", states)
	}
}
