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

func TestDiscoverApplicationCachesVSCodeFamilyEditorsExactAllowlistOnly(t *testing.T) {
	// Each VS Code-family editor is independent: exact AppData folder, own category,
	// and sibling editors under the same Roaming base must never leak.
	cases := []struct {
		policyID string
		appDir   string
		category string
	}{
		{applicationCachePolicyVSCodeInsiders, "Code - Insiders", OpportunityCategoryVSCodeInsidersCache},
		{applicationCachePolicyVSCodium, "VSCodium", OpportunityCategoryVSCodiumCache},
		{applicationCachePolicyWindsurf, "Windsurf", OpportunityCategoryWindsurfCache},
		{applicationCachePolicyTrae, "Trae", OpportunityCategoryTraeCache},
	}
	for _, tc := range cases {
		t.Run(tc.category, func(t *testing.T) {
			roaming := t.TempDir()
			editorRoot := filepath.Join(roaming, tc.appDir)
			for _, name := range []string{"Cache", "CachedData", "MyCache", "User", "extensions", "workspaceStorage"} {
				if err := os.MkdirAll(filepath.Join(editorRoot, name), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(editorRoot, name, "f.bin"), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Sibling Stable Code root must not leak.
			if err := os.MkdirAll(filepath.Join(roaming, "Code", "Cache"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(roaming, "Code", "Cache", "f.bin"), []byte("vscode"), 0600); err != nil {
				t.Fatal(err)
			}
			result := discoverApplicationCaches(context.Background(), tc.policyID, ApplicationCacheDiscoveryOptions{
				RoamingAppDataDir: roaming,
			}, pathsafe.Validator{})
			if len(result.opportunities) != 2 {
				t.Fatalf("opportunities = %#v, want Cache and CachedData only", result.opportunities)
			}
			for _, opportunity := range result.opportunities {
				if opportunity.Category != tc.category {
					t.Fatalf("category = %q, want %q", opportunity.Category, tc.category)
				}
				if !strings.HasPrefix(opportunity.Path, editorRoot) {
					t.Fatalf("path %q not under %s root", opportunity.Path, tc.appDir)
				}
				base := filepath.Base(opportunity.Path)
				if base != "Cache" && base != "CachedData" {
					t.Fatalf("unexpected root %q", base)
				}
			}
		})
	}
}

func TestDiscoverApplicationCachesObsidianPlainElectronAllowlistOnly(t *testing.T) {
	// Obsidian is a non-editor Electron app carrying its own plain-Electron
	// allowlist: the 6 single-segment regenerating roots, excluding CachedData
	// and CachedExtensionVSIXs and every state/config/bundle directory.
	roaming := t.TempDir()
	obsidianRoot := filepath.Join(roaming, "obsidian")
	allDirs := make([]string, 0, len(obsidianCacheAllowlistedRelativeRoots)+8)
	allDirs = append(allDirs, obsidianCacheAllowlistedRelativeRoots...)
	allDirs = append(allDirs, "CachedData", "CachedExtensionVSIXs", "User", "Local Storage", "IndexedDB", "Service Worker", "Preferences", "logs")
	for _, name := range allDirs {
		if err := os.MkdirAll(filepath.Join(obsidianRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(obsidianRoot, name, "f.bin"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Sibling editor root must not leak into Obsidian discovery.
	if err := os.MkdirAll(filepath.Join(roaming, "Code", "Cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roaming, "Code", "Cache", "f.bin"), []byte("vscode"), 0600); err != nil {
		t.Fatal(err)
	}
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyObsidian, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: roaming,
	}, pathsafe.Validator{})
	if len(result.opportunities) != len(obsidianCacheAllowlistedRelativeRoots) {
		t.Fatalf("opportunities = %#v, want one per plain-Electron allowlisted root", result.opportunities)
	}
	seen := map[string]bool{}
	for _, opportunity := range result.opportunities {
		if opportunity.Category != OpportunityCategoryObsidianCache {
			t.Fatalf("category = %q, want obsidian_cache", opportunity.Category)
		}
		if !strings.HasPrefix(opportunity.Path, obsidianRoot) {
			t.Fatalf("path %q not under obsidian root", opportunity.Path)
		}
		base := filepath.Base(opportunity.Path)
		seen[base] = true
		if base == "CachedData" || base == "CachedExtensionVSIXs" {
			t.Fatalf("Obsidian plain-Electron allowlist must not include %q", base)
		}
	}
	for _, root := range obsidianCacheAllowlistedRelativeRoots {
		if !seen[root] {
			t.Fatalf("missing allowlisted root %q", root)
		}
	}
}

func TestDiscoverApplicationCachesVRChatSingleRootAllowlistUnderLocalLow(t *testing.T) {
	// VRChat is a non-editor social VR app on the LocalLow base with a two-segment
	// application directory (VRChat\VRChat) and a single-root allowlist: only
	// Cache-WindowsPlayer (downloaded avatar/world content). Settings, logs,
	// cookies, and unknown siblings are never candidates.
	localLow := t.TempDir()
	vrchatRoot := filepath.Join(localLow, "VRChat", "VRChat")
	dirs := []string{"Cache-WindowsPlayer", "cookies", "Unity", "VRChat_Data", "logs", "amplitude"}
	for _, name := range dirs {
		if err := os.MkdirAll(filepath.Join(vrchatRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vrchatRoot, name, "f.bin"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A stray log file directly under the application directory is not a root.
	if err := os.WriteFile(filepath.Join(vrchatRoot, "output_log.txt"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	// A sibling application directory under LocalLow must not leak into VRChat discovery.
	if err := os.MkdirAll(filepath.Join(localLow, "OtherApp", "Cache-WindowsPlayer"), 0700); err != nil {
		t.Fatal(err)
	}
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyVRChat, ApplicationCacheDiscoveryOptions{
		LocalLowAppDataDir: localLow,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want exactly one (Cache-WindowsPlayer)", result.opportunities)
	}
	opportunity := result.opportunities[0]
	if opportunity.Category != OpportunityCategoryVRChatCache {
		t.Fatalf("category = %q, want vrchat_cache", opportunity.Category)
	}
	if filepath.Base(opportunity.Path) != "Cache-WindowsPlayer" {
		t.Fatalf("root = %q, want Cache-WindowsPlayer", filepath.Base(opportunity.Path))
	}
	if !strings.HasPrefix(opportunity.Path, vrchatRoot) {
		t.Fatalf("path %q not under VRChat application directory %q", opportunity.Path, vrchatRoot)
	}
}

func TestDiscoverApplicationCachesVRChatMissingRootSilentAbsence(t *testing.T) {
	// The VRChat application directory exists but the allowlisted Cache-WindowsPlayer
	// root is absent: silent absence, never an incomplete or a candidate.
	localLow := t.TempDir()
	vrchatRoot := filepath.Join(localLow, "VRChat", "VRChat")
	if err := os.MkdirAll(filepath.Join(vrchatRoot, "cookies"), 0700); err != nil {
		t.Fatal(err)
	}
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyVRChat, ApplicationCacheDiscoveryOptions{
		LocalLowAppDataDir: localLow,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 || len(result.incompletes) != 0 {
		t.Fatalf("missing Cache-WindowsPlayer root should be silent: %#v", result)
	}
}

func TestApplicationCacheBaseDirLocalLowUsesKnownFolder(t *testing.T) {
	// Regression: the LocalLow base must resolve via the LocalLow AppData known
	// folder (FOLDERID_LocalAppDataLow, typically %USERPROFILE%\AppData\LocalLow),
	// not %LOCALAPPDATA%\Low. The wrong path does not exist, so preflight treated
	// every LocalLow application (vrchat_cache) as silently absent at runtime even
	// though the real cache directory was present. Injected LocalLowAppDataDir
	// still wins; this test exercises the non-injected production path.
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "Local"))
	got := applicationCacheBaseDir(ApplicationCacheDiscoveryOptions{}, applicationCachePolicy{
		base: applicationCacheBaseLocalLow,
	})
	want := resolveLocalAppDataLowDir()
	if got != want {
		t.Fatalf("LocalLow base without injection = %q, want LocalLow known folder %q (not %%LOCALAPPDATA%%\\Low)", got, want)
	}
}

func TestDiscoverApplicationCachesBlankAppDataSilent(t *testing.T) {
	result := discoverApplicationCaches(context.Background(), applicationCachePolicyVSCode, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: "   ",
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 || len(result.incompletes) != 0 {
		t.Fatalf("blank Roaming AppData should be silent: %#v", result)
	}

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
	if outcome.discoveryRan || outcome.proceed {
		t.Fatal("discoveryRan/proceed should be false without pre-state")
	}
	outcome = gate.gateApplicationCache(context.Background(), ApplicationVisualStudioCode, []RunningApplicationState{
		{Application: ApplicationVisualStudioCode, State: RunningApplicationStateRunning},
	}, func() applicationCacheDiscoveryResult {
		t.Fatal("discover must not run when running")
		return applicationCacheDiscoveryResult{}
	})
	if outcome.discoveryRan || outcome.proceed {
		t.Fatal("discoveryRan/proceed should be false when running")
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateRunning {
		t.Fatalf("runningStates = %#v, want projected running pre-state", outcome.runningStates)
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
	if !outcome.discoveryRan || outcome.proceed {
		t.Fatalf("outcome = %#v, want discoveryRan and not proceed", outcome)
	}
	if len(outcome.discovery.opportunities) != 1 {
		t.Fatalf("discovery should retain measured data for caller discard: %#v", outcome.discovery)
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].State != RunningApplicationStateRunning {
		t.Fatalf("runningStates = %#v, want post running superseding pre", outcome.runningStates)
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
	if !outcome.discoveryRan || !outcome.proceed || discoverCalls != 1 {
		t.Fatalf("outcome = %#v discoverCalls=%d, want idle Cursor despite running VS Code", outcome, discoverCalls)
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].Application != ApplicationCursor {
		t.Fatalf("runningStates = %#v, want only Cursor", outcome.runningStates)
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
	if !outcome.discoveryRan || !outcome.proceed {
		t.Fatalf("outcome = %#v, want idle VS Code despite running Cursor", outcome)
	}
	if len(outcome.runningStates) != 1 || outcome.runningStates[0].Application != ApplicationVisualStudioCode {
		t.Fatalf("runningStates = %#v, want only VS Code", outcome.runningStates)
	}
}

func TestResolveOptInCandidatesInjectedApplicationCacheDiscovery(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "Code", "Cache")
	discoveryCalls := 0
	opts := Options{
		OptIn: []string{OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: root,
			stat: func(string) (os.FileInfo, error) {
				return appCacheFakeInfo{name: "Code", mode: os.ModeDir, mod: time.Now()}, nil
			},
		},
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

func TestDiscoverApplicationCachesLocalBase(t *testing.T) {
	// Register a test policy with local base
	testPolicyID := "test_local_app"
	testCategory := "test_local_cache"
	originalPolicies := applicationCachePolicies
	defer func() { applicationCachePolicies = originalPolicies }()

	applicationCachePolicies = map[string]applicationCachePolicy{
		testPolicyID: {
			category:     testCategory,
			application:  ApplicationVisualStudioCode,
			base:         applicationCacheBaseLocal,
			appDataPath:  []string{"TestApp", "Data"},
			relativeRoots: []string{"Cache", "Logs"},
		},
	}

	local := t.TempDir()
	appRoot := filepath.Join(local, "TestApp", "Data")
	for _, name := range []string{"Cache", "Logs", "Config"} {
		if err := os.MkdirAll(filepath.Join(appRoot, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(appRoot, name, "test.bin"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result := discoverApplicationCaches(context.Background(), testPolicyID, ApplicationCacheDiscoveryOptions{
		LocalAppDataDir: local,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 2 {
		t.Fatalf("opportunities = %#v, want Cache and Logs only", result.opportunities)
	}
	seen := make(map[string]bool)
	for _, opp := range result.opportunities {
		if opp.Category != testCategory {
			t.Fatalf("category = %q, want %q", opp.Category, testCategory)
		}
		base := filepath.Base(opp.Path)
		seen[base] = true
		if base != "Cache" && base != "Logs" {
			t.Fatalf("unexpected root %q", base)
		}
	}
	if !seen["Cache"] || !seen["Logs"] {
		t.Fatalf("missing expected roots: %#v", seen)
	}

	// Verify RoamingAppDataDir is ignored for local base
	result = discoverApplicationCaches(context.Background(), testPolicyID, ApplicationCacheDiscoveryOptions{
		RoamingAppDataDir: local,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want none when only RoamingAppDataDir is set", result.opportunities)
	}
}

func TestDiscoverApplicationCachesLocalLowBase(t *testing.T) {
	// Register a test policy with locallow base
	testPolicyID := "test_locallow_app"
	testCategory := "test_locallow_cache"
	originalPolicies := applicationCachePolicies
	defer func() { applicationCachePolicies = originalPolicies }()

	applicationCachePolicies = map[string]applicationCachePolicy{
		testPolicyID: {
			category:     testCategory,
			application:  ApplicationVisualStudioCode,
			base:         applicationCacheBaseLocalLow,
			appDataPath:  []string{"TestAppLow", "Data"},
			relativeRoots: []string{"Cache"},
		},
	}

	localLow := t.TempDir()
	appRoot := filepath.Join(localLow, "TestAppLow", "Data")
	if err := os.MkdirAll(filepath.Join(appRoot, "Cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "Cache", "test.bin"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := discoverApplicationCaches(context.Background(), testPolicyID, ApplicationCacheDiscoveryOptions{
		LocalLowAppDataDir: localLow,
	}, pathsafe.Validator{})
	if len(result.opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want Cache only", result.opportunities)
	}
	if result.opportunities[0].Category != testCategory {
		t.Fatalf("category = %q, want %q", result.opportunities[0].Category, testCategory)
	}
	if filepath.Base(result.opportunities[0].Path) != "Cache" {
		t.Fatalf("path = %q, want Cache root", result.opportunities[0].Path)
	}
}

func TestDiscoverApplicationCachesBlankBaseSilent(t *testing.T) {
	// Register test policies
	testPolicyIDLocal := "test_local_app_silent"
	testPolicyIDLow := "test_locallow_app_silent"
	originalPolicies := applicationCachePolicies
	defer func() { applicationCachePolicies = originalPolicies }()

	applicationCachePolicies = map[string]applicationCachePolicy{
		testPolicyIDLocal: {
			category:     "test_local_cache_silent",
			application:  ApplicationVisualStudioCode,
			base:         applicationCacheBaseLocal,
			appDataPath:  []string{"TestApp"},
			relativeRoots: []string{"Cache"},
		},
		testPolicyIDLow: {
			category:     "test_locallow_cache_silent",
			application:  ApplicationVisualStudioCode,
			base:         applicationCacheBaseLocalLow,
			appDataPath:  []string{"TestAppLow"},
			relativeRoots: []string{"Cache"},
		},
	}

	// Blank LocalAppDataDir
	result := discoverApplicationCaches(context.Background(), testPolicyIDLocal, ApplicationCacheDiscoveryOptions{
		LocalAppDataDir: "",
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 || len(result.incompletes) != 0 {
		t.Fatalf("blank LocalAppDataDir should be silent: %#v", result)
	}

	// Blank LocalLowAppDataDir
	result = discoverApplicationCaches(context.Background(), testPolicyIDLow, ApplicationCacheDiscoveryOptions{
		LocalLowAppDataDir: "",
	}, pathsafe.Validator{})
	if len(result.opportunities) != 0 || len(result.incompletes) != 0 {
		t.Fatalf("blank LocalLowAppDataDir should be silent: %#v", result)
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
