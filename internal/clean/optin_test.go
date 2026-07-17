package clean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveOptInCandidatesScansOnlyOptedInOpportunityCategories covers Q6: the
// resolver scans only opted-in opportunity categories. A discovery function
// returning multiple categories is filtered to the opted-in set, so execute
// never scans non-opted-in categories (ADR-0008).
func TestResolveOptInCandidatesScansOnlyOptedInOpportunityCategories(t *testing.T) {
	discover := func(ctx context.Context) OpportunityDiscoveryResult {
		return OpportunityDiscoveryResult{
			Opportunities: []Opportunity{
				{Category: OpportunityCategoryUserTemp, Path: `C:\temp\old`, Bytes: 10},
				{Category: OpportunityCategoryCrashDumps, Path: `C:\CrashDumps`, Bytes: 20},
			},
		}
	}
	opts := Options{
		DiscoverOpportunities: discover,
		OptIn:                 []string{OpportunityCategoryUserTemp},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)

	if len(res.candidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (only user_temp opted in)", len(res.candidates))
	}
	if res.candidates[0].Category != OpportunityCategoryUserTemp {
		t.Errorf("candidate category = %q, want %q", res.candidates[0].Category, OpportunityCategoryUserTemp)
	}
	if res.candidates[0].IsUserTemp != true {
		t.Errorf("user_temp candidate not flagged IsUserTemp")
	}
}

// TestResolveOptInCandidatesBrowserYieldsIndividualCacheDirs covers Q2: a
// Browser cache opt-in candidate is an individual regenerating cache directory,
// not the User Data root. Only non-empty cache directories become candidates.
func TestResolveOptInCandidatesBrowserYieldsIndividualCacheDirs(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	defaultCache := filepath.Join(chromeUserData, "Default", "Cache")
	if err := os.MkdirAll(defaultCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultCache, "data.bin"), []byte("cache data"), 0600); err != nil {
		t.Fatal(err)
	}
	detector := func(ctx context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateIdle}}
	}
	opts := Options{
		DetectRunningApplications: detector,
		BrowserCacheDiscoveryOptions: BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		OptIn: []string{OpportunityCategoryBrowserCache},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)

	if len(res.candidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (Default/Cache only; other allowlisted kinds empty)", len(res.candidates))
	}
	if res.candidates[0].Path != defaultCache {
		t.Errorf("candidate path = %q, want individual cache dir %q (not User Data root)", res.candidates[0].Path, defaultCache)
	}
	if res.candidates[0].Category != OpportunityCategoryBrowserCache {
		t.Errorf("candidate category = %q, want %q", res.candidates[0].Category, OpportunityCategoryBrowserCache)
	}
}

// TestResolveOptInCandidatesDevCacheRunningRecordsSkip covers Q4: a dev cache
// gated out by a running tool produces a Skipped item in the resolution, which
// both modes surface. (dry-run previously dropped this.)
func TestResolveOptInCandidatesDevCacheRunningRecordsSkip(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-build")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "f"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	detector := func(ctx context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
	}
	opts := Options{
		DetectRunningApplications: detector,
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		OptIn:                     []string{DevCacheCategoryGo},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)

	if len(res.candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 (go running)", len(res.candidates))
	}
	if len(res.skipped) != 1 {
		t.Fatalf("skipped = %d, want 1 (dev_tool_running)", len(res.skipped))
	}
	if res.skipped[0].Reason.Code != devToolRunningIssueCode {
		t.Errorf("skip reason code = %q, want %q", res.skipped[0].Reason.Code, devToolRunningIssueCode)
	}
	if res.skipped[0].Path != cachePath {
		t.Errorf("skip path = %q, want %q", res.skipped[0].Path, cachePath)
	}
	if res.skipped[0].Bytes != 0 {
		t.Errorf("skip bytes = %d, want 0 (pre-gate must not measure)", res.skipped[0].Bytes)
	}
}

// sequenceGoDetector returns successive Go application states across detector
// calls. Pre-measurement and each post-measurement re-check consume one call.
func sequenceGoDetector(states ...RunningApplicationStatus) func(context.Context) []RunningApplicationState {
	call := 0
	return func(context.Context) []RunningApplicationState {
		state := states[len(states)-1]
		if call < len(states) {
			state = states[call]
		}
		call++
		return []RunningApplicationState{{Application: ApplicationGo, State: state}}
	}
}

// TestResolveOptInCandidatesDevCachePostMeasurementReCheck covers issue #166:
// idle→running after measurement discards the root; idle→idle preserves it.
func TestResolveOptInCandidatesDevCachePostMeasurementReCheck(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-build")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "f"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("idle to idle yields candidate", func(t *testing.T) {
		opts := Options{
			DetectRunningApplications: sequenceGoDetector(
				RunningApplicationStateIdle,
				RunningApplicationStateIdle,
			),
			DevCachePathResolver: func(string) []string { return []string{cachePath} },
			OptIn:                []string{DevCacheCategoryGo},
		}
		plan, _, _ := NormalizedOptInSet(opts.OptIn)
		res := resolveOptInCandidates(context.Background(), opts, plan)
		if len(res.candidates) != 1 {
			t.Fatalf("candidates = %d, want 1", len(res.candidates))
		}
		if res.candidates[0].Bytes != 7 {
			t.Errorf("candidate bytes = %d, want 7", res.candidates[0].Bytes)
		}
		if len(res.skipped) != 0 {
			t.Fatalf("skipped = %d, want 0", len(res.skipped))
		}
	})

	t.Run("idle to running discards candidate and bytes", func(t *testing.T) {
		opts := Options{
			DetectRunningApplications: sequenceGoDetector(
				RunningApplicationStateIdle,
				RunningApplicationStateRunning,
			),
			DevCachePathResolver: func(string) []string { return []string{cachePath} },
			OptIn:                []string{DevCacheCategoryGo},
		}
		plan, _, _ := NormalizedOptInSet(opts.OptIn)
		res := resolveOptInCandidates(context.Background(), opts, plan)
		if len(res.candidates) != 0 {
			t.Fatalf("candidates = %d, want 0 after post running", len(res.candidates))
		}
		if len(res.skipped) != 1 {
			t.Fatalf("skipped = %d, want 1", len(res.skipped))
		}
		if res.skipped[0].Reason.Code != devToolRunningIssueCode || res.skipped[0].Bytes != 0 {
			t.Fatalf("skip = %+v, want dev_tool_running with 0 bytes", res.skipped[0])
		}
	})

	t.Run("idle to unknown discards candidate", func(t *testing.T) {
		opts := Options{
			DetectRunningApplications: sequenceGoDetector(
				RunningApplicationStateIdle,
				RunningApplicationStateUnknown,
			),
			DevCachePathResolver: func(string) []string { return []string{cachePath} },
			OptIn:                []string{DevCacheCategoryGo},
		}
		plan, _, _ := NormalizedOptInSet(opts.OptIn)
		res := resolveOptInCandidates(context.Background(), opts, plan)
		if len(res.candidates) != 0 || len(res.skipped) != 1 {
			t.Fatalf("candidates=%d skipped=%d, want 0/1", len(res.candidates), len(res.skipped))
		}
	})

	t.Run("idle to missing discards candidate", func(t *testing.T) {
		call := 0
		opts := Options{
			DetectRunningApplications: func(context.Context) []RunningApplicationState {
				call++
				if call == 1 {
					return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
				}
				return nil // post missing required state
			},
			DevCachePathResolver: func(string) []string { return []string{cachePath} },
			OptIn:                []string{DevCacheCategoryGo},
		}
		plan, _, _ := NormalizedOptInSet(opts.OptIn)
		res := resolveOptInCandidates(context.Background(), opts, plan)
		if len(res.candidates) != 0 || len(res.skipped) != 1 {
			t.Fatalf("candidates=%d skipped=%d, want 0/1", len(res.candidates), len(res.skipped))
		}
	})
}

// TestResolveOptInCandidatesDevCacheMultiRootIndependentEvidence ensures one
// root discarded by the post-gate does not authorize or double-count another.
func TestResolveOptInCandidatesDevCacheMultiRootIndependentEvidence(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "a")
	pathB := filepath.Join(root, "b")
	for _, p := range []string{pathA, pathB} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "f"), []byte("xx"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Call sequence: pre (idle), post for A (running → discard A), post for B (idle → keep B).
	call := 0
	opts := Options{
		DetectRunningApplications: func(context.Context) []RunningApplicationState {
			call++
			switch call {
			case 1:
				return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
			case 2:
				return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateRunning}}
			default:
				return []RunningApplicationState{{Application: ApplicationGo, State: RunningApplicationStateIdle}}
			}
		},
		DevCachePathResolver: func(category string) []string {
			if category == DevCacheCategoryGo {
				return []string{pathA, pathB}
			}
			return nil
		},
		OptIn: []string{DevCacheCategoryGo},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)

	if len(res.candidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (only root B)", len(res.candidates))
	}
	if res.candidates[0].Path != pathB {
		t.Errorf("candidate path = %q, want %q", res.candidates[0].Path, pathB)
	}
	if res.candidates[0].Bytes != 2 {
		t.Errorf("candidate bytes = %d, want 2 (no double-count of A)", res.candidates[0].Bytes)
	}
	if len(res.skipped) != 1 || res.skipped[0].Path != pathA {
		t.Fatalf("skipped = %+v, want only path A", res.skipped)
	}
}

// TestResolveOptInCandidatesSharedRuntimeDoesNotDetect ensures npm/pip/corepack
// opt-in alone does not run developer-tool process detection.
func TestResolveOptInCandidatesSharedRuntimeDoesNotDetect(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "f"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	detectCalled := false
	opts := Options{
		DetectRunningApplications: func(context.Context) []RunningApplicationState {
			detectCalled = true
			return nil
		},
		DevCachePathResolver: func(string) []string { return []string{cachePath} },
		OptIn:                []string{DevCacheCategoryNPM},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)
	if detectCalled {
		t.Fatal("shared-runtime npm-cache must not trigger developer-tool detection")
	}
	if len(res.candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(res.candidates))
	}
}

// TestResolveOptInCandidatesBrowserRunningRecordsState covers Q4: a browser
// gated out by a running process surfaces the running state, so execute (not
// just dry-run) reports it. (execute previously dropped this.)
func TestResolveOptInCandidatesBrowserRunningRecordsState(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	defaultCache := filepath.Join(chromeUserData, "Default", "Cache")
	if err := os.MkdirAll(defaultCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultCache, "data.bin"), []byte("cache data"), 0600); err != nil {
		t.Fatal(err)
	}
	detector := func(ctx context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationGoogleChrome, State: RunningApplicationStateRunning}}
	}
	opts := Options{
		DetectRunningApplications: detector,
		BrowserCacheDiscoveryOptions: BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		OptIn: []string{OpportunityCategoryBrowserCache},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)

	if len(res.candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 (browser running)", len(res.candidates))
	}
	found := false
	for _, s := range res.runningStates {
		if s.Application == ApplicationGoogleChrome && s.State == RunningApplicationStateRunning {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("running state not surfaced in resolution: %+v", res.runningStates)
	}
}

// TestResolveOptInCandidatesEmptyPlanProducesNothing covers the no-op fast
// path: an empty plan resolves nothing without touching discovery.
func TestResolveOptInCandidatesEmptyPlanProducesNothing(t *testing.T) {
	discoverCalled := false
	opts := Options{
		DiscoverOpportunities: func(context.Context) OpportunityDiscoveryResult {
			discoverCalled = true
			return OpportunityDiscoveryResult{}
		},
	}
	res := resolveOptInCandidates(context.Background(), opts, map[string]bool{})
	if len(res.candidates) != 0 || len(res.skipped) != 0 || len(res.runningStates) != 0 || len(res.diagnostics) != 0 {
		t.Fatalf("empty plan resolution = %+v, want all empty", res)
	}
	if discoverCalled {
		t.Fatal("discovery should not run for an empty plan")
	}
}

func TestResolveOptInCandidatesCancellationAddsRecoverableDiagnostic(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	// Add multiple files to measure
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(cachePath, fmt.Sprintf("file-%d.dat", i)), []byte("cachedata"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	opts := Options{
		DevCachePathResolver: func(category string) []string {
			if category == DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		},
		DiscoverOpportunities: func(context.Context) OpportunityDiscoveryResult {
			return OpportunityDiscoveryResult{}
		},
	}
	plan := map[string]bool{DevCacheCategoryNPM: true}

	res := resolveOptInCandidates(ctx, opts, plan)

	if len(res.candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 after cancellation", len(res.candidates))
	}
	if len(res.diagnostics) != 1 {
		t.Fatalf("diagnostics count = %d, want 1 context_canceled diagnostic", len(res.diagnostics))
	}
	if res.diagnostics[0].Code != "context_canceled" || !res.diagnostics[0].Recoverable {
		t.Fatalf("diagnostic = %+v, want recoverable context_canceled", res.diagnostics[0])
	}
	if res.diagnostics[0].Path != cachePath {
		t.Errorf("diagnostic path = %q, want %q", res.diagnostics[0].Path, cachePath)
	}
	if res.diagnostics[0].Rule != DevCacheCategoryNPM {
		t.Errorf("diagnostic rule = %q, want %q", res.diagnostics[0].Rule, DevCacheCategoryNPM)
	}
}

func TestResolveOptInCandidatesCancellationSkipsPartialBytes(t *testing.T) {
	root := t.TempDir()
	// Create two dev cache paths
	npmPath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(npmPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(npmPath, "small.dat"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}

	goPath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(goPath, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err := os.WriteFile(filepath.Join(goPath, fmt.Sprintf("file-%d.dat", i)), []byte("godata"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Test with uncancelled context first for baseline
	opts := Options{
		DevCachePathResolver: func(category string) []string {
			switch category {
			case DevCacheCategoryNPM:
				return []string{npmPath}
			case DevCacheCategoryGo:
				return []string{goPath}
			}
			return nil
		},
		DiscoverOpportunities: func(context.Context) OpportunityDiscoveryResult {
			return OpportunityDiscoveryResult{}
		},
	}
	plan := map[string]bool{DevCacheCategoryNPM: true, DevCacheCategoryGo: true}

	res := resolveOptInCandidates(context.Background(), opts, plan)
	if len(res.candidates) != 2 {
		t.Fatalf("uncancelled candidates = %d, want 2", len(res.candidates))
	}
	baselineTotal := int64(0)
	for _, c := range res.candidates {
		baselineTotal += c.Bytes
	}
	expectedTotal := int64(4) + 50*6 // small.dat (4) + 50 files * 6 bytes each
	if baselineTotal != expectedTotal {
		t.Fatalf("uncancelled total bytes = %d, want %d", baselineTotal, expectedTotal)
	}
}
