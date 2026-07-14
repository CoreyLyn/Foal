package clean

import (
	"context"
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
		t.Fatalf("candidates = %d, want 1 (Default/Cache only; Code Cache/GPUCache are empty)", len(res.candidates))
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
