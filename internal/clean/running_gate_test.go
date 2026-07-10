package clean

import (
	"context"
	"testing"
)

func TestDevCacheGateTier(t *testing.T) {
	for _, c := range []string{DevCacheCategoryNPM, DevCacheCategoryPip, DevCacheCategoryCorepack} {
		if got := devCacheGateTier(c); got != runningGateTierNone {
			t.Errorf("devCacheGateTier(%q) = %v, want none", c, got)
		}
	}
	for _, c := range []string{DevCacheCategoryGo, DevCacheCategoryCargo, DevCacheCategoryNuGet} {
		if got := devCacheGateTier(c); got != runningGateTierSingle {
			t.Errorf("devCacheGateTier(%q) = %v, want single", c, got)
		}
	}
}

func TestGateDevCacheNoneTierAlwaysProceeds(t *testing.T) {
	g := runningGate{}
	// None-tier proceeds even when an unrelated tool is running.
	states := []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
	for _, category := range []string{DevCacheCategoryNPM, DevCacheCategoryPip, DevCacheCategoryCorepack} {
		outcome := g.gateDevCache(category, `C:\cache`, states)
		if !outcome.proceed || outcome.skipReason != nil {
			t.Fatalf("none-tier %q: outcome = %+v, want proceed", category, outcome)
		}
	}
}

func TestGateDevCacheSingleTier(t *testing.T) {
	g := runningGate{}
	const path = `C:\go-cache`

	// idle -> proceed
	outcome := g.gateDevCache(DevCacheCategoryGo, path, []RunningApplicationState{
		{Application: ApplicationGo, State: RunningApplicationStateIdle},
	})
	if !outcome.proceed || outcome.skipReason != nil {
		t.Fatalf("idle go: outcome = %+v, want proceed", outcome)
	}

	// running -> skip with reason carrying path + category
	outcome = g.gateDevCache(DevCacheCategoryGo, path, []RunningApplicationState{
		{Application: ApplicationGo, State: RunningApplicationStateRunning},
	})
	if outcome.proceed || outcome.skipReason == nil {
		t.Fatalf("running go: outcome = %+v, want skip", outcome)
	}
	if outcome.skipReason.Code != devToolRunningIssueCode {
		t.Errorf("running go reason code = %q, want %q", outcome.skipReason.Code, devToolRunningIssueCode)
	}
	if outcome.skipReason.Path != path {
		t.Errorf("running go reason path = %q, want %q", outcome.skipReason.Path, path)
	}

	// unknown -> skip (fail-closed: unknown never means idle)
	outcome = g.gateDevCache(DevCacheCategoryGo, path, []RunningApplicationState{
		{Application: ApplicationGo, State: RunningApplicationStateUnknown},
	})
	if outcome.proceed || outcome.skipReason == nil {
		t.Fatalf("unknown go: outcome = %+v, want skip (fail-closed)", outcome)
	}

	// absent state -> skip (fail-closed: !ok)
	outcome = g.gateDevCache(DevCacheCategoryGo, path, nil)
	if outcome.proceed || outcome.skipReason == nil {
		t.Fatalf("absent go state: outcome = %+v, want skip (fail-closed)", outcome)
	}

	// nuget: both dotnet and nuget checked; either running skips
	outcome = g.gateDevCache(DevCacheCategoryNuGet, path, []RunningApplicationState{
		{Application: ApplicationDotNet, State: RunningApplicationStateIdle},
		{Application: ApplicationNuGet, State: RunningApplicationStateRunning},
	})
	if outcome.proceed || outcome.skipReason == nil {
		t.Fatalf("nuget with nuget running: outcome = %+v, want skip", outcome)
	}
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
