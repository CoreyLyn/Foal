package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// noisyDetector returns browser, editor, and unrelated developer-tool states so
// scoping can be asserted against a shared DetectSupportedApplications-shaped
// snapshot without depending on live process enumeration.
func noisyDetector(state clean.RunningApplicationStatus) func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationGoogleChrome, State: state},
			{Application: clean.ApplicationMicrosoftEdge, State: state},
			{Application: clean.ApplicationGo, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationNode, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationPython, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationBun, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationUV, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationVisualStudioCode, State: state},
			{Application: clean.ApplicationCursor, State: state},
		}
	}
}

func applicationIDs(states []clean.RunningApplicationState) []string {
	ids := make([]string, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.Application)
	}
	return ids
}

func countApplication(states []clean.RunningApplicationState, application string) int {
	n := 0
	for _, state := range states {
		if state.Application == application {
			n++
		}
	}
	return n
}

func writeChromeEdgeFixtures(t *testing.T, localAppData string) {
	t.Helper()
	for _, parts := range [][]string{
		{"Google", "Chrome", "User Data"},
		{"Microsoft", "Edge", "User Data"},
	} {
		userData := filepath.Join(append([]string{localAppData}, parts...)...)
		defaultCache := filepath.Join(userData, "Default", "Cache")
		if err := os.MkdirAll(defaultCache, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(userData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(defaultCache, "data.bin"), []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDryRunCombinedOptInDedupesAndScopesRunningApplications(t *testing.T) {
	localAppData := t.TempDir()
	writeChromeEdgeFixtures(t, localAppData)
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cursor"})

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications:        noisyDetector(clean.RunningApplicationStateIdle),
		DiscoverOpportunities:            noUserTempOpportunities,
		DiscoverReviewSuggestions:        noReviewSuggestions,
		Rules:                            []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		OptIn: []string{
			clean.OpportunityCategoryBrowserCache,
			clean.OpportunityCategoryVSCodeCache,
			clean.OpportunityCategoryCursorCache,
		},
	})

	wantApps := []string{
		clean.ApplicationGoogleChrome,
		clean.ApplicationMicrosoftEdge,
		clean.ApplicationVisualStudioCode,
		clean.ApplicationCursor,
	}
	if got := applicationIDs(result.RunningApplications); len(got) != len(wantApps) {
		t.Fatalf("running applications = %#v, want exactly %v", result.RunningApplications, wantApps)
	}
	for i, want := range wantApps {
		if result.RunningApplications[i].Application != want {
			t.Fatalf("running applications[%d] = %#v, want %q first-seen order among %v", i, result.RunningApplications[i], want, wantApps)
		}
		if countApplication(result.RunningApplications, want) != 1 {
			t.Fatalf("application %q appeared %d times: %#v", want, countApplication(result.RunningApplications, want), result.RunningApplications)
		}
		if result.RunningApplications[i].State != clean.RunningApplicationStateIdle {
			t.Fatalf("state for %q = %q, want idle", want, result.RunningApplications[i].State)
		}
	}
	for _, forbidden := range []string{
		clean.ApplicationGo,
		clean.ApplicationCargo,
		clean.ApplicationNode,
		clean.ApplicationPython,
		clean.ApplicationBun,
		clean.ApplicationUV,
	} {
		if countApplication(result.RunningApplications, forbidden) != 0 {
			t.Fatalf("unrelated application %q leaked into running_applications: %#v", forbidden, result.RunningApplications)
		}
	}

	model := clean.NewPreviewReadModel(result)
	for _, state := range model.RunningApplicationSkips {
		// Idle apps are not skips; model should not invent duplicates either.
		_ = state
	}
	// Ensure read-model projection has no duplicate running skip rows either.
	seenSkip := map[string]bool{}
	for _, skip := range model.RunningApplicationSkips {
		if seenSkip[skip.Application] {
			t.Fatalf("duplicate read-model running skip for %q", skip.Application)
		}
		seenSkip[skip.Application] = true
	}
}

func TestDryRunCombinedOptInPostStateReplacesEarlierWithoutMoving(t *testing.T) {
	localAppData := t.TempDir()
	writeChromeEdgeFixtures(t, localAppData)
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})

	// Exact plan: no additive review fan-out for unselected editors.
	plan, err := clean.CompileExactCategoryPlan([]string{
		clean.OpportunityCategoryBrowserCache,
		clean.OpportunityCategoryVSCodeCache,
	})
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			calls++
			vscodeState := clean.RunningApplicationStateIdle
			if calls > 1 {
				// Post-measurement supersedes pre-measurement for VS Code.
				vscodeState = clean.RunningApplicationStateRunning
			}
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationBun, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationVisualStudioCode, State: vscodeState},
				{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateIdle},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		Plan:                      &plan,
	})

	wantOrder := []string{
		clean.ApplicationGoogleChrome,
		clean.ApplicationMicrosoftEdge,
		clean.ApplicationVisualStudioCode,
	}
	if got := applicationIDs(result.RunningApplications); len(got) != len(wantOrder) {
		t.Fatalf("running applications = %#v, want %v", result.RunningApplications, wantOrder)
	}
	for i, want := range wantOrder {
		if result.RunningApplications[i].Application != want {
			t.Fatalf("order[%d] = %q, want %q (%#v)", i, result.RunningApplications[i].Application, want, result.RunningApplications)
		}
	}
	vscode := result.RunningApplications[2]
	if vscode.Application != clean.ApplicationVisualStudioCode || vscode.State != clean.RunningApplicationStateRunning {
		t.Fatalf("VS Code state = %#v, want running at original index", vscode)
	}
	if countApplication(result.RunningApplications, clean.ApplicationVisualStudioCode) != 1 {
		t.Fatalf("VS Code duplicated: %#v", result.RunningApplications)
	}
	if countApplication(result.RunningApplications, clean.ApplicationBun) != 0 {
		t.Fatalf("unrelated Bun leaked: %#v", result.RunningApplications)
	}
}

func TestDryRunCombinedOptInPreservesRelevantUnknownStatesWithoutDuplicates(t *testing.T) {
	localAppData := t.TempDir()
	writeChromeEdgeFixtures(t, localAppData)
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateUnknown, Message: "chrome snapshot failed"},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationNode, State: clean.RunningApplicationStateUnknown, Message: "node noise"},
				{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateUnknown, Message: "vscode snapshot failed"},
				{Application: clean.ApplicationBun, State: clean.RunningApplicationStateUnknown, Message: "bun noise"},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		OptIn: []string{
			clean.OpportunityCategoryBrowserCache,
			clean.OpportunityCategoryVSCodeCache,
		},
	})

	if countApplication(result.RunningApplications, clean.ApplicationGoogleChrome) != 1 {
		t.Fatalf("Chrome rows = %#v", result.RunningApplications)
	}
	if countApplication(result.RunningApplications, clean.ApplicationVisualStudioCode) != 1 {
		t.Fatalf("VS Code rows = %#v", result.RunningApplications)
	}
	if countApplication(result.RunningApplications, clean.ApplicationNode) != 0 || countApplication(result.RunningApplications, clean.ApplicationBun) != 0 {
		t.Fatalf("unrelated unknown apps leaked: %#v", result.RunningApplications)
	}
	for _, state := range result.RunningApplications {
		switch state.Application {
		case clean.ApplicationGoogleChrome:
			if state.State != clean.RunningApplicationStateUnknown || state.Message != "chrome snapshot failed" {
				t.Fatalf("Chrome state = %#v, want preserved unknown", state)
			}
		case clean.ApplicationVisualStudioCode:
			if state.State != clean.RunningApplicationStateUnknown || state.Message != "vscode snapshot failed" {
				t.Fatalf("VS Code state = %#v, want preserved unknown", state)
			}
		}
	}
	// Unrelated applications must never invent diagnostics on this path.
	for _, err := range result.Errors {
		if err.Code == "running_application_detection_unknown" {
			if err.Message == "node noise" || err.Message == "bun noise" {
				t.Fatalf("unrelated app created diagnostic: %#v", err)
			}
		}
	}
}

func TestDryRunBrowserReviewScopesStatesAndKeepsRelevantUnknownDiagnostics(t *testing.T) {
	// Additive dry-run without browser opt-in uses the review projection: only
	// Chrome/Edge states are reported, and only their unknown diagnostics land.
	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: t.TempDir()},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateUnknown, Message: "chrome snapshot failed"},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationBun, State: clean.RunningApplicationStateUnknown, Message: "bun noise"},
				{Application: clean.ApplicationNode, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})

	if got := applicationIDs(result.RunningApplications); len(got) != 2 {
		t.Fatalf("running applications = %#v, want Chrome and Edge only", result.RunningApplications)
	}
	if result.RunningApplications[0].Application != clean.ApplicationGoogleChrome ||
		result.RunningApplications[0].State != clean.RunningApplicationStateUnknown {
		t.Fatalf("Chrome = %#v, want unknown", result.RunningApplications[0])
	}
	if result.RunningApplications[1].Application != clean.ApplicationMicrosoftEdge ||
		result.RunningApplications[1].State != clean.RunningApplicationStateIdle {
		t.Fatalf("Edge = %#v, want idle", result.RunningApplications[1])
	}

	foundChrome := false
	for _, err := range result.Errors {
		if err.Code != "running_application_detection_unknown" {
			continue
		}
		if err.Message == "bun noise" {
			t.Fatalf("unrelated unknown diagnostic: %#v", err)
		}
		if err.Message == "chrome snapshot failed" || strings.Contains(err.Message, "Chrome") {
			foundChrome = true
		}
	}
	if !foundChrome {
		t.Fatalf("errors = %#v, want recoverable Chrome unknown diagnostic", result.Errors)
	}
}

func TestExecuteCombinedOptInUsesSameRunningApplicationAggregation(t *testing.T) {
	localAppData := t.TempDir()
	writeChromeEdgeFixtures(t, localAppData)
	roaming := t.TempDir()
	writeVSCodeRoot(t, roaming, map[string]string{"Cache": "vscode"})
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cursor"})
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		RecycleBinAdapter:                recycle,
		PermanentRemover:                 permanent,
		AllowPermanentDeletion:           true,
		BrowserCacheDiscoveryOptions:     clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{RoamingAppDataDir: roaming},
		DetectRunningApplications:        noisyDetector(clean.RunningApplicationStateIdle),
		DiscoverOpportunities:            noUserTempOpportunities,
		DiscoverReviewSuggestions:        noReviewSuggestions,
		Rules:                            []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
		OptIn: []string{
			clean.OpportunityCategoryBrowserCache,
			clean.OpportunityCategoryVSCodeCache,
			clean.OpportunityCategoryCursorCache,
		},
	})

	wantApps := []string{
		clean.ApplicationGoogleChrome,
		clean.ApplicationMicrosoftEdge,
		clean.ApplicationVisualStudioCode,
		clean.ApplicationCursor,
	}
	if got := applicationIDs(result.RunningApplications); len(got) != len(wantApps) {
		t.Fatalf("execute running applications = %#v, want %v", result.RunningApplications, wantApps)
	}
	for i, want := range wantApps {
		if result.RunningApplications[i].Application != want {
			t.Fatalf("execute order[%d] = %q, want %q", i, result.RunningApplications[i].Application, want)
		}
		if countApplication(result.RunningApplications, want) != 1 {
			t.Fatalf("execute duplicated %q: %#v", want, result.RunningApplications)
		}
	}
	for _, forbidden := range []string{clean.ApplicationBun, clean.ApplicationNode, clean.ApplicationGo} {
		if countApplication(result.RunningApplications, forbidden) != 0 {
			t.Fatalf("execute leaked %q: %#v", forbidden, result.RunningApplications)
		}
	}
	// Browser/editor caches are permanent: permanent remover receives candidates.
	if len(permanent.paths) == 0 {
		t.Fatal("expected permanent remover to receive opt-in candidates")
	}
	if len(recycle.paths) != 0 {
		t.Fatalf("permanent categories must not use Recycle Bin: %v", recycle.paths)
	}
	if result.Mode != "execute" {
		t.Fatalf("mode = %q, want execute", result.Mode)
	}
}
