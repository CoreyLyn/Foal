package clean

import (
	"context"
	"errors"
	"testing"
)

func TestDevCacheGateTier(t *testing.T) {
	for _, c := range []string{DevCacheCategoryNPM, DevCacheCategoryPip, DevCacheCategoryCorepack} {
		if got := devCacheGateTier(c); got != runningGateTierNone {
			t.Errorf("devCacheGateTier(%q) = %v, want none", c, got)
		}
	}
	for _, c := range []string{DevCacheCategoryGo, DevCacheCategoryCargo, DevCacheCategoryNuGet, DevCacheCategoryNuGetGlobalPackages} {
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
	if !planNeedsDistinctiveProcessDetection(map[string]bool{DevCacheCategoryGo: true}) {
		t.Fatal("go-cache should need distinctive-process detection")
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
		if outcome.preIdle {
			t.Fatalf("pre %v: preIdle = true, want false", pre.State)
		}
		if discoverCalled {
			t.Fatalf("pre %v: discover should not run", pre.State)
		}
	}
	// absent pre-state -> preIdle false, no discover
	discoverCalled = false
	outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, nil, discover)
	if outcome.preIdle || discoverCalled {
		t.Fatalf("absent pre-state: preIdle=%v discoverCalled=%v, want false/false", outcome.preIdle, discoverCalled)
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
		if !outcome.preIdle {
			t.Fatalf("%s: preIdle = false, want true", tc.name)
		}
		if !outcome.postIdle {
			t.Errorf("%s: postIdle = false, want true (no post-check on not-clean discovery)", tc.name)
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
	if !outcome.preIdle || !outcome.postIdle {
		t.Fatalf("clean+post-idle: preIdle=%v postIdle=%v, want true/true", outcome.preIdle, outcome.postIdle)
	}
	if outcome.discovery.opportunity != opp {
		t.Error("discovery not passed through")
	}
	if outcome.postState != nil || outcome.postDiagnostic != nil {
		t.Error("postState/diagnostic should be nil when postIdle")
	}
}

func TestGateBrowserPostNotIdle(t *testing.T) {
	opp := &Opportunity{Path: `C:\User Data`, Bytes: 100}
	discover := func() browserCacheDiscoveryResult { return browserCacheDiscoveryResult{opportunity: opp} }
	pre := []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle}}

	// post running -> postIdle false, postState set, no diagnostic
	g := runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning}}
	}}
	outcome := g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.postIdle {
		t.Fatal("post running: postIdle = true, want false")
	}
	if outcome.postState == nil || outcome.postState.State != RunningApplicationStateRunning {
		t.Fatalf("post running: postState = %+v, want running", outcome.postState)
	}
	if outcome.postDiagnostic != nil {
		t.Error("post running: diagnostic should be nil")
	}

	// post unknown -> postIdle false, postState set, diagnostic set (fail-closed)
	g = runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateUnknown, Message: "snap failed"}}
	}}
	outcome = g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.postIdle {
		t.Fatal("post unknown: postIdle = true, want false")
	}
	if outcome.postState == nil || outcome.postState.State != RunningApplicationStateUnknown {
		t.Fatalf("post unknown: postState = %+v, want unknown", outcome.postState)
	}
	if outcome.postDiagnostic == nil || outcome.postDiagnostic.Code != runningApplicationDetectionIssueCode {
		t.Fatalf("post unknown: diagnostic = %+v, want code %q", outcome.postDiagnostic, runningApplicationDetectionIssueCode)
	}

	// post absent -> postIdle false, no postState, no diagnostic
	g = runningGate{detect: func(context.Context) []RunningApplicationState { return nil }}
	outcome = g.gateBrowser(context.Background(), ApplicationGoogleChrome, pre, discover)
	if outcome.postIdle {
		t.Fatal("post absent: postIdle = true, want false")
	}
	if outcome.postState != nil || outcome.postDiagnostic != nil {
		t.Fatalf("post absent: postState=%+v diagnostic=%+v, want nil/nil", outcome.postState, outcome.postDiagnostic)
	}
}
