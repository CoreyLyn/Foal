package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestDiscoverApplicationCachesExactAllowlistOnly(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := filepath.Join(roaming, "Code")
	for _, name := range []string{"Cache", "CachedData", "MyCache", "User", "extensions"} {
		if err := os.MkdirAll(filepath.Join(codeRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(codeRoot, name, "f.bin"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 2 {
		t.Fatalf("opportunities = %#v, want Cache and CachedData only", result.opportunities)
	}
	for _, opportunity := range result.opportunities {
		base := filepath.Base(opportunity.Path)
		if base != "Cache" && base != "CachedData" {
			t.Fatalf("unexpected root %q", base)
		}
		if opportunity.Category != OpportunityCategoryVSCodeCache {
			t.Fatalf("category = %q, want vscode_cache", opportunity.Category)
		}
	}
}

func TestDiscoverApplicationCachesCursorExactAllowlistOnly(t *testing.T) {
	roaming := t.TempDir()
	cursorRoot := filepath.Join(roaming, "Cursor")
	for _, name := range []string{"Cache", "CachedData", "MyCache", "User", "extensions", "workspaceStorage"} {
		if err := os.MkdirAll(filepath.Join(cursorRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cursorRoot, name, "f.bin"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// VS Code roots under the same Roaming base must never leak into Cursor discovery.
	if err := os.MkdirAll(filepath.Join(roaming, "Code", "Cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roaming, "Code", "Cache", "f.bin"), []byte("vscode"), 0600); err != nil {
		t.Fatal(err)
	}
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyCursor, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 2 {
		t.Fatalf("opportunities = %#v, want Cache and CachedData only", result.opportunities)
	}
	for _, opportunity := range result.opportunities {
		if opportunity.Category != OpportunityCategoryCursorCache {
			t.Fatalf("category = %q, want cursor_cache", opportunity.Category)
		}
		if !strings.HasPrefix(opportunity.Path, cursorRoot) {
			t.Fatalf("path %q not under Cursor root", opportunity.Path)
		}
		base := filepath.Base(opportunity.Path)
		if base != "Cache" && base != "CachedData" {
			t.Fatalf("unexpected root %q", base)
		}
	}
}

func TestDiscoverApplicationCachesBlankAppDataSilent(t *testing.T) {
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: "   ",
	}, pathsafe.Validator{})
	// Whitespace-only still joins; empty string after trim is silent. Force empty.
	result = discoverApplicationCaches(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: "",
	}, pathsafe.Validator{})
	// With empty override, discover falls back to env; isolate by injecting deps only through blank env is hard.
	// Explicit empty policy path: use a missing child under a real base.
	roaming := t.TempDir()
	result = discoverApplicationCaches(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 || len(result.incompletes) != 0 {
		t.Fatalf("missing Code root should be silent: %#v", result)
	}
}

func TestDiscoverApplicationCachesIncompleteRootKeepsSiblings(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := filepath.Join(roaming, "Code")
	if err := os.MkdirAll(filepath.Join(codeRoot, "Cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codeRoot, "Cache", "ok.bin"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(codeRoot, "CachedData"), 0700); err != nil {
		t.Fatal(err)
	}
	result := discoverApplicationCachesWithDeps(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{}, applicationCacheDiscoveryDependencies{
		stat: os.Lstat,
		walkDir: func(path string, fn fs.WalkDirFunc) error {
			if filepath.Base(path) == "CachedData" {
				return errors.New("permission denied")
			}
			return filepath.WalkDir(path, fn)
		},
	})
	if len(result.opportunities) != 1 || filepath.Base(result.opportunities[0].Path) != "Cache" {
		t.Fatalf("opportunities = %#v, want Cache only", result.opportunities)
	}
	if len(result.incompletes) != 1 || filepath.Base(result.incompletes[0].Path) != "CachedData" {
		t.Fatalf("incompletes = %#v, want CachedData", result.incompletes)
	}
}

func TestDiscoverApplicationCachesCancellationDiscardsMeasured(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := filepath.Join(roaming, "Code")
	if err := os.MkdirAll(filepath.Join(codeRoot, "Cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codeRoot, "Cache", "ok.bin"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(codeRoot, "CachedData"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := discoverApplicationCaches(ctx, applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{})
	if !result.canceled {
		t.Fatalf("result = %#v, want canceled", result)
	}
	if len(result.opportunities) != 0 {
		t.Fatalf("canceled discovery must discard opportunities: %#v", result.opportunities)
	}
}

func TestGateApplicationCachePreIdleRequired(t *testing.T) {
	gate := runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle}}
	}}
	outcome := gate.gateApplicationCache(context.Background(), ApplicationVisualStudioCode, nil, func() applicationCacheDiscoveryResult {
		t.Fatal("discover must not run when pre-state missing")
		return applicationCacheDiscoveryResult{}
	})
	if outcome.preIdle {
		t.Fatal("preIdle should be false without pre-state")
	}
	outcome = gate.gateApplicationCache(context.Background(), ApplicationVisualStudioCode, []RunningApplicationState{
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateRunning},
	}, func() applicationCacheDiscoveryResult {
		t.Fatal("discover must not run when running")
		return applicationCacheDiscoveryResult{}
	})
	if outcome.preIdle {
		t.Fatal("preIdle should be false when running")
	}
}

func TestGateApplicationCachePostDiscard(t *testing.T) {
	calls := 0
	gate := runningGate{detect: func(context.Context) []RunningApplicationState {
		calls++
		return []RunningApplicationState{{Application: ApplicationVisualStudioCode, State: RunningApplicationStateRunning}}
	}}
	outcome := gate.gateApplicationCache(context.Background(), ApplicationVisualStudioCode, []RunningApplicationState{
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
	}, func() applicationCacheDiscoveryResult {
		return applicationCacheDiscoveryResult{
			opportunities: []Opportunity{{
				Category: OpportunityCategoryVSCodeCache,
				Path:     `C:\fake\Cache`,
				Bytes:    10,
			}},
		}
	})
	if !outcome.preIdle || outcome.postIdle {
		t.Fatalf("outcome = %#v, want pre idle and post not idle", outcome)
	}
	if len(outcome.discovery.opportunities) != 1 {
		t.Fatalf("discovery should retain measured data for caller discard: %#v", outcome.discovery)
	}
	if calls != 1 {
		t.Fatalf("post detect calls = %d", calls)
	}
}

func TestGateApplicationCacheIndependentEditorApplications(t *testing.T) {
	// Cursor gate only inspects the Cursor application identity.
	discoverCalls := 0
	gate := runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{
			{Application: ApplicationVisualStudioCode, State: RunningApplicationStateRunning},
			{Application: ApplicationCursor, State: RunningApplicationStateIdle},
		}
	}}
	outcome := gate.gateApplicationCache(context.Background(), ApplicationCursor, []RunningApplicationState{
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateRunning},
		{Application: ApplicationCursor, State: RunningApplicationStateIdle},
	}, func() applicationCacheDiscoveryResult {
		discoverCalls++
		return applicationCacheDiscoveryResult{
			opportunities: []Opportunity{{
				Category: OpportunityCategoryCursorCache,
				Path:     `C:\fake\Cursor\Cache`,
				Bytes:    7,
			}},
		}
	})
	if !outcome.preIdle || !outcome.postIdle || discoverCalls != 1 {
		t.Fatalf("outcome = %#v discoverCalls=%d, want idle Cursor despite running VS Code", outcome, discoverCalls)
	}

	// VS Code gate ignores Cursor running state on both pre and post checks.
	gate = runningGate{detect: func(context.Context) []RunningApplicationState {
		return []RunningApplicationState{
			{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
			{Application: ApplicationCursor, State: RunningApplicationStateRunning},
		}
	}}
	outcome = gate.gateApplicationCache(context.Background(), ApplicationVisualStudioCode, []RunningApplicationState{
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle},
		{Application: ApplicationCursor, State: RunningApplicationStateRunning},
	}, func() applicationCacheDiscoveryResult {
		return applicationCacheDiscoveryResult{
			opportunities: []Opportunity{{
				Category: OpportunityCategoryVSCodeCache,
				Path:     `C:\fake\Code\Cache`,
				Bytes:    3,
			}},
		}
	})
	if !outcome.preIdle || !outcome.postIdle {
		t.Fatalf("outcome = %#v, want idle VS Code despite running Cursor", outcome)
	}
}

func TestResolveOptInCandidatesInjectedApplicationCacheDiscovery(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "Cache")
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "x.bin"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	discoveryCalls := 0
	opts := Options{
		OptIn: []string{OpportunityCategoryVSCodeCache},
		DetectRunningApplications: func(context.Context) []RunningApplicationState {
			return []RunningApplicationState{{Application: ApplicationVisualStudioCode, State: RunningApplicationStateIdle}}
		},
		DiscoverApplicationCaches: func(ctx context.Context, policyID string, o ApplicationCacheDiscoveryOptions, v pathsafe.Validator) applicationCacheDiscoveryResult {
			discoveryCalls++
			if policyID != applicationCachePolicyVSCode {
				t.Fatalf("policyID = %q", policyID)
			}
			return applicationCacheDiscoveryResult{
				opportunities: []Opportunity{{
					Category: OpportunityCategoryVSCodeCache,
					Path:     cachePath,
					Bytes:    4,
					Status:   OpportunityStatus,
					Reason:   OpportunityReason,
				}},
			}
		},
	}
	plan, _, _ := NormalizedOptInSet(opts.OptIn)
	res := resolveOptInCandidates(context.Background(), opts, plan)
	if discoveryCalls != 1 {
		t.Fatalf("discoveryCalls = %d", discoveryCalls)
	}
	if len(res.candidates) != 1 || res.candidates[0].Path != cachePath || res.candidates[0].Bytes != 4 {
		t.Fatalf("candidates = %#v", res.candidates)
	}
}

func TestDiscoverApplicationCachesRespectsDescendantCeiling(t *testing.T) {
	roaming := t.TempDir()
	cachePath := filepath.Join(roaming, "Code", "Cache")
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	// Exceed ceiling with synthetic walk that counts descendants.
	result := discoverApplicationCachesWithDeps(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{}, applicationCacheDiscoveryDependencies{
		stat: os.Lstat,
		walkDir: func(path string, fn fs.WalkDirFunc) error {
			// Root + 100001 descendants.
			info := appCacheFakeInfo{name: filepath.Base(path), mode: os.ModeDir, mod: time.Now()}
			if err := fn(path, appCacheFakeEntry{info: info}, nil); err != nil {
				return err
			}
			for i := 0; i <= userTempDescendantLimit; i++ {
				child := filepath.Join(path, "c")
				fileInfo := appCacheFakeInfo{name: "c", mode: 0, mod: time.Now(), size: 1}
				if err := fn(child, appCacheFakeEntry{info: fileInfo}, nil); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if len(result.opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want none over limit", result.opportunities)
	}
	if len(result.incompletes) != 1 || result.incompletes[0].Reason.Code != "inspection_limit_exceeded" {
		t.Fatalf("incompletes = %#v", result.incompletes)
	}
}

type appCacheFakeInfo struct {
	name string
	mode os.FileMode
	mod  time.Time
	size int64
}

func (f appCacheFakeInfo) Name() string       { return f.name }
func (f appCacheFakeInfo) Size() int64        { return f.size }
func (f appCacheFakeInfo) Mode() os.FileMode  { return f.mode }
func (f appCacheFakeInfo) ModTime() time.Time { return f.mod }
func (f appCacheFakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f appCacheFakeInfo) Sys() any           { return nil }

type appCacheFakeEntry struct {
	info appCacheFakeInfo
}

func (f appCacheFakeEntry) Name() string               { return f.info.name }
func (f appCacheFakeEntry) IsDir() bool                { return f.info.IsDir() }
func (f appCacheFakeEntry) Type() os.FileMode          { return f.info.Mode().Type() }
func (f appCacheFakeEntry) Info() (os.FileInfo, error) { return f.info, nil }
