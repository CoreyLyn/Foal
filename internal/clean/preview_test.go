package clean_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

func noUserTempOpportunities(context.Context) clean.UserTempDiscoveryResult {
	return clean.UserTempDiscoveryResult{
		Opportunities: []clean.UserTempOpportunity{},
		Incomplete:    []clean.IncompleteOpportunityInspection{},
	}
}

func noReviewSuggestions(context.Context) []clean.ReviewSuggestion {
	return []clean.ReviewSuggestion{}
}

func TestDryRunProjectsReviewSuggestionsThroughJSONAndHumanReportWithoutBytes(t *testing.T) {
	suggestions := []clean.ReviewSuggestion{
		{
			Tool:      "pnpm",
			Label:     "pnpm cache",
			Command:   "pnpm store prune",
			CachePath: `C:\Users\corey\AppData\Local\pnpm\store\v10`,
		},
		{
			Tool:      "yarn",
			Label:     "yarn cache",
			Command:   "yarn cache clean",
			CachePath: `C:\Users\corey\AppData\Local\Yarn\Cache\v6`,
		},
		{
			Tool:      "bun",
			Label:     "bun cache",
			Command:   "bun pm cache rm",
			CachePath: `C:\Users\corey\.bun\install\cache`,
		},
		{
			Tool:      "corepack",
			Label:     "Corepack cache",
			Command:   "corepack cache clean",
			CachePath: `C:\Users\corey\AppData\Local\node\corepack\v1`,
		},
		{
			Tool:      "mise",
			Label:     "mise cache",
			Command:   "mise cache clear",
			CachePath: `C:\Users\corey\AppData\Local\mise`,
		},
	}
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return suggestions
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
		}},
	})

	if len(result.ReviewSuggestions) != len(suggestions) {
		t.Fatalf("review suggestions = %#v, want JavaScript suggestions", result.ReviewSuggestions)
	}
	if result.Totals.CandidateBytes != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("totals = %#v, want suggestions excluded from byte totals", result.Totals)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, want := range []string{
		`"review_suggestions":[`,
		`"tool":"pnpm"`,
		`"command":"pnpm store prune"`,
		`"tool":"yarn"`,
		`"command":"yarn cache clean"`,
		`"tool":"bun"`,
		`"command":"bun pm cache rm"`,
		`"tool":"corepack"`,
		`"command":"corepack cache clean"`,
		`"tool":"mise"`,
		`"command":"mise cache clear"`,
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("JSON missing %q: %s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), `"bytes"`) {
		t.Fatalf("review suggestion JSON must not carry bytes: %s", encoded)
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{
		"Review suggestions",
		"pnpm cache",
		"pnpm store prune",
		"yarn cache",
		"yarn cache clean",
		"bun cache",
		"bun pm cache rm",
		"Corepack cache",
		"corepack cache clean",
		"mise cache",
		"mise cache clear",
		"Potential space: 0 bytes",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestDryRunExcludedRootsContributeOnlyDeveloperToolReviewSuggestion(t *testing.T) {
	tempRoot := t.TempDir()
	localAppData := t.TempDir()
	detailedListDir := t.TempDir()
	excludedRoots := []string{
		filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default", "Cache"),
		filepath.Join(localAppData, "Microsoft", "Edge", "User Data", "Default", "Cache"),
		filepath.Join(localAppData, "Mozilla", "Firefox", "Profiles", "profile", "cache2"),
		filepath.Join(localAppData, "$Recycle.Bin"),
		filepath.Join(localAppData, "SoftwareDistribution", "Download"),
		filepath.Join(localAppData, "DeliveryOptimization", "Cache"),
	}
	developerCache := filepath.Join(localAppData, "npm-cache")
	for _, root := range append(excludedRoots, developerCache) {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "payload.bin"), []byte("excluded bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	recorder := &recordingHistoryRecorder{}
	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "disabled_test_rule",
			Description:    "disabled test rule",
			DefaultEnabled: false,
		}},
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         tempRoot,
			LocalAppDataDir: localAppData,
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{{
				Tool:      "npm",
				Label:     "npm cache",
				Command:   "npm cache clean --force",
				CachePath: developerCache,
			}}
		},
		DetailedListDir: detailedListDir,
		HistoryRecorder: recorder,
	})

	if len(result.Opportunities) != 0 ||
		result.Totals.OpportunityCount != 0 ||
		result.Totals.OpportunityObservedBytes != 0 ||
		result.Totals.CandidateCount != 0 ||
		result.Totals.CandidateBytes != 0 ||
		len(result.Candidates) != 0 {
		t.Fatalf("excluded roots affected dry-run cleanup data: %#v", result)
	}
	if len(result.ReviewSuggestions) != 1 || result.ReviewSuggestions[0].CachePath != developerCache {
		t.Fatalf("review suggestions = %#v, want developer cache exactly once", result.ReviewSuggestions)
	}

	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	if strings.Count(report, developerCache) != 1 {
		t.Fatalf("developer cache report occurrences = %d, want one Review suggestion:\n%s", strings.Count(report, developerCache), report)
	}
	for _, excluded := range excludedRoots {
		if strings.Contains(report, excluded) {
			t.Fatalf("report emitted excluded root %q:\n%s", excluded, report)
		}
	}

	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range append(excludedRoots, developerCache) {
		if strings.Contains(string(detailed), excluded) {
			t.Fatalf("detailed list emitted excluded or suggestion-only root %q:\n%s", excluded, detailed)
		}
		if strings.Contains(recorder.encoded, excluded) {
			t.Fatalf("history emitted excluded or suggestion-only root %q:\n%s", excluded, recorder.encoded)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 0 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 0 ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want empty excluded-root contribution", recorder.sessions, recorder.items)
	}
}

func TestDryRunProjectsRunningBrowsersAsRunningApplicationSkipsWithoutBrowserCacheData(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if len(result.RunningApplications) != 2 {
		t.Fatalf("running applications = %#v, want Chrome and Edge", result.RunningApplications)
	}
	if len(result.Opportunities) != 0 || result.Totals.OpportunityCount != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("browser running state affected opportunities: %#v", result)
	}
	if result.Totals.CandidateCount != 1 || result.Totals.CandidateBytes != 5 || result.Totals.DeletedCount != 0 {
		t.Fatalf("totals = %#v, want candidate-only accounting", result.Totals)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"running_applications":[`,
		`"application":"google_chrome"`,
		`"state":"running"`,
		`"application":"microsoft_edge"`,
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("JSON missing %q: %s", want, encoded)
		}
	}
	for _, forbidden := range []string{"browser_cache", "Cache", "GPUCache", "Code Cache"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("JSON introduced browser cache detail %q: %s", forbidden, encoded)
		}
		if strings.Contains(recorder.encoded, forbidden) {
			t.Fatalf("history introduced browser cache detail %q: %s", forbidden, recorder.encoded)
		}
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{
		"Running application skips",
		"Google Chrome",
		"Microsoft Edge",
		"Potential space: 5 bytes",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.CandidateCount != 1 ||
		recorder.sessions[0].Aggregate.ErrorCount != 0 ||
		len(recorder.items) != 1 {
		t.Fatalf("history = %#v / %#v, want candidate-only dry-run history", recorder.sessions, recorder.items)
	}
}

func TestDryRunProjectsUnknownBrowserStateAsRecoverableDiagnosticsWithoutBrowserCacheData(t *testing.T) {
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateUnknown, Message: "process snapshot failed"},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Errors) != 1 || result.Errors[0].Code != "running_application_detection_unknown" || !result.Errors[0].Recoverable {
		t.Fatalf("errors = %#v, want recoverable unknown browser diagnostic", result.Errors)
	}
	if len(result.RunningApplications) != 2 || result.RunningApplications[1].State != clean.RunningApplicationStateIdle {
		t.Fatalf("running applications = %#v, want unknown Chrome and idle Edge", result.RunningApplications)
	}
	if result.Totals.CandidateCount != 0 || result.Totals.OpportunityCount != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("totals = %#v, want unknown state outside cleanup accounting", result.Totals)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"browser_cache", "Cache", "GPUCache", "Code Cache"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("JSON introduced browser cache detail %q: %s", forbidden, encoded)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.ErrorCount != 0 ||
		len(recorder.items) != 0 ||
		strings.Contains(recorder.encoded, "running_application_detection_unknown") {
		t.Fatalf("history = %#v / %#v / %s, want no browser diagnostic history", recorder.sessions, recorder.items, recorder.encoded)
	}
}

func TestDryRunReportsChromeBrowserCacheOpportunityThroughReviewSurfaces(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	writeChromeLocalState(t, userDataRoot, map[string]string{
		"Default":        "Corey",
		"Profile 1":      "Work",
		"Guest Profile":  "Guest",
		"System Profile": "System",
	})
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cache", "cache.bin"), "cache")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Code Cache", "code.bin"), "code")
	writeFile(t, filepath.Join(userDataRoot, "Default", "History"), "history must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cookies"), "cookies must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Extensions", "extension.bin"), "extension must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "worker.bin"), "worker must not count")
	writeFile(t, filepath.Join(userDataRoot, "Profile 1", "GPUCache", "gpu.bin"), "gpu")
	writeFile(t, filepath.Join(userDataRoot, "Guest Profile", "Cache", "guest.bin"), "guest must not count")
	writeFile(t, filepath.Join(userDataRoot, "System Profile", "Cache", "system.bin"), "system must not count")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleChromeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one Chrome browser cache opportunity", result.Opportunities)
	}
	opportunity := result.Opportunities[0]
	if opportunity.Category != clean.OpportunityCategoryBrowserCache ||
		opportunity.Bytes != 12 ||
		opportunity.Status != clean.OpportunityStatus ||
		opportunity.Reason != clean.OpportunityReason ||
		opportunity.BrowserCache == nil ||
		opportunity.BrowserCache.Browser != clean.ApplicationGoogleChrome ||
		opportunity.BrowserCache.ProfileCount != 2 {
		t.Fatalf("Chrome opportunity = %#v, want complete browser cache summary", opportunity)
	}
	if result.Totals.OpportunityCount != 1 ||
		result.Totals.OpportunityObservedBytes != 12 ||
		result.Totals.CandidateBytes != 0 ||
		result.Totals.CandidateCount != 0 {
		t.Fatalf("totals = %#v, want observed-only Chrome bytes", result.Totals)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, want := range []string{
		`"category":"browser_cache"`,
		`"browser":"google_chrome"`,
		`"profile_count":2`,
		`"id":"Default"`,
		`"id":"Profile 1"`,
		`"kind":"Cache"`,
		`"kind":"Code Cache"`,
		`"kind":"GPUCache"`,
		`"opportunity_observed_bytes":12`,
		`"candidate_bytes":0`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("JSON missing %q: %s", want, jsonText)
		}
	}
	for _, forbidden := range []string{"Guest Profile", "System Profile", "History", "Cookies", "Extensions", "Service Worker", "move_to_recycle_bin"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded Chrome data %q: %s", forbidden, jsonText)
		}
	}

	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	for _, want := range []string{
		"Google Chrome browser cache",
		"category: browser_cache",
		"profiles: 2",
		"observed bytes: 12 bytes",
		"Potential space: 0 bytes",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}
	for _, noisy := range []string{"Profile 1", "Code Cache", "GPUCache", "History", "Cookies", "Extensions", "Service Worker"} {
		if strings.Contains(report, noisy) {
			t.Fatalf("human report contains noisy Chrome detail %q:\n%s", noisy, report)
		}
	}

	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	detailedText := string(detailed)
	for _, want := range []string{
		"browser: Google Chrome",
		"profiles: 2",
		"profile: Default",
		"profile: Profile 1",
		filepath.Join(userDataRoot, "Default", "Cache"),
		filepath.Join(userDataRoot, "Default", "Code Cache"),
		filepath.Join(userDataRoot, "Default", "GPUCache"),
	} {
		if !strings.Contains(detailedText, want) {
			t.Fatalf("detailed review data missing %q:\n%s", want, detailedText)
		}
	}
	for _, forbidden := range []string{"Guest Profile", "System Profile", "History", "Cookies", "Extensions", "Service Worker"} {
		if strings.Contains(detailedText, forbidden) {
			t.Fatalf("detailed review data contains excluded Chrome path %q:\n%s", forbidden, detailedText)
		}
		if strings.Contains(recorder.encoded, forbidden) || strings.Contains(recorder.encoded, userDataRoot) {
			t.Fatalf("history persisted Chrome path/detail %q: %s", forbidden, recorder.encoded)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 1 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 12 ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want privacy-preserving Chrome aggregate only", recorder.sessions, recorder.items)
	}
}

func TestDryRunReportsEdgeBrowserCacheOpportunityThroughSharedReviewSurfaces(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
	writeBrowserLocalState(t, userDataRoot, map[string]string{
		"Default":        "Corey",
		"Profile 1":      "Work",
		"Guest Profile":  "Guest",
		"System Profile": "System",
	})
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cache", "cache.bin"), "cache")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Code Cache", "code.bin"), "code")
	writeFile(t, filepath.Join(userDataRoot, "Default", "History"), "history must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cookies"), "cookies must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Extensions", "extension.bin"), "extension must not count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "worker.bin"), "worker must not count")
	writeFile(t, filepath.Join(userDataRoot, "Profile 1", "GPUCache", "gpu.bin"), "gpu")
	writeFile(t, filepath.Join(userDataRoot, "Guest Profile", "Cache", "guest.bin"), "guest must not count")
	writeFile(t, filepath.Join(userDataRoot, "System Profile", "Cache", "system.bin"), "system must not count")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleBrowserDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one Edge browser cache opportunity", result.Opportunities)
	}
	opportunity := result.Opportunities[0]
	if opportunity.Category != clean.OpportunityCategoryBrowserCache ||
		opportunity.Bytes != 12 ||
		opportunity.Status != clean.OpportunityStatus ||
		opportunity.Reason != clean.OpportunityReason ||
		opportunity.BrowserCache == nil ||
		opportunity.BrowserCache.Browser != clean.ApplicationMicrosoftEdge ||
		opportunity.BrowserCache.ProfileCount != 2 {
		t.Fatalf("Edge opportunity = %#v, want complete browser cache summary", opportunity)
	}
	if result.Totals.OpportunityCount != 1 ||
		result.Totals.OpportunityObservedBytes != 12 ||
		result.Totals.CandidateBytes != 0 ||
		result.Totals.CandidateCount != 0 {
		t.Fatalf("totals = %#v, want observed-only Edge bytes", result.Totals)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, want := range []string{
		`"category":"browser_cache"`,
		`"browser":"microsoft_edge"`,
		`"profile_count":2`,
		`"id":"Default"`,
		`"id":"Profile 1"`,
		`"kind":"Cache"`,
		`"kind":"Code Cache"`,
		`"kind":"GPUCache"`,
		`"opportunity_observed_bytes":12`,
		`"candidate_bytes":0`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("JSON missing %q: %s", want, jsonText)
		}
	}
	for _, forbidden := range []string{"Guest Profile", "System Profile", "History", "Cookies", "Extensions", "Service Worker", "move_to_recycle_bin"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded Edge data %q: %s", forbidden, jsonText)
		}
	}

	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	for _, want := range []string{
		"Microsoft Edge browser cache",
		"category: browser_cache",
		"profiles: 2",
		"observed bytes: 12 bytes",
		"Potential space: 0 bytes",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}
	for _, noisy := range []string{"Profile 1", "Code Cache", "GPUCache", "History", "Cookies", "Extensions", "Service Worker"} {
		if strings.Contains(report, noisy) {
			t.Fatalf("human report contains noisy Edge detail %q:\n%s", noisy, report)
		}
	}

	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	detailedText := string(detailed)
	for _, want := range []string{
		"browser: Microsoft Edge",
		"profiles: 2",
		"profile: Default",
		"profile: Profile 1",
		filepath.Join(userDataRoot, "Default", "Cache"),
		filepath.Join(userDataRoot, "Default", "Code Cache"),
		filepath.Join(userDataRoot, "Default", "GPUCache"),
	} {
		if !strings.Contains(detailedText, want) {
			t.Fatalf("detailed review data missing %q:\n%s", want, detailedText)
		}
	}
	for _, forbidden := range []string{"Guest Profile", "System Profile", "History", "Cookies", "Extensions", "Service Worker"} {
		if strings.Contains(detailedText, forbidden) {
			t.Fatalf("detailed review data contains excluded Edge path %q:\n%s", forbidden, detailedText)
		}
		if strings.Contains(recorder.encoded, forbidden) || strings.Contains(recorder.encoded, userDataRoot) {
			t.Fatalf("history persisted Edge path/detail %q: %s", forbidden, recorder.encoded)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 1 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 12 ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want privacy-preserving Edge aggregate only", recorder.sessions, recorder.items)
	}
}

func TestDryRunReportsChromeAndEdgeBrowserCachesWithoutSharedStateReuse(t *testing.T) {
	localAppData := t.TempDir()
	chromeRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	edgeRoot := filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
	writeBrowserLocalState(t, chromeRoot, map[string]string{"Default": "Chrome"})
	writeBrowserLocalState(t, edgeRoot, map[string]string{"Profile 1": "Edge"})
	writeFile(t, filepath.Join(chromeRoot, "Default", "Cache", "chrome.bin"), "chrome")
	writeFile(t, filepath.Join(edgeRoot, "Profile 1", "Code Cache", "edge.bin"), "edge")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications:    idleBrowserDetector(),
		DiscoverOpportunities:        noUserTempOpportunities,
		DiscoverReviewSuggestions:    noReviewSuggestions,
		Rules:                        []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 2 {
		t.Fatalf("opportunities = %#v, want Chrome and Edge summaries", result.Opportunities)
	}
	seen := map[string]int64{}
	for _, opportunity := range result.Opportunities {
		if opportunity.BrowserCache == nil {
			t.Fatalf("opportunity missing browser cache detail: %#v", opportunity)
		}
		seen[opportunity.BrowserCache.Browser] = opportunity.Bytes
	}
	if seen[clean.ApplicationGoogleChrome] != 6 || seen[clean.ApplicationMicrosoftEdge] != 4 {
		t.Fatalf("browser bytes = %#v, want independent Chrome/Edge totals", seen)
	}
	if result.Totals.OpportunityCount != 2 ||
		result.Totals.OpportunityObservedBytes != 10 ||
		result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v, want observed-only combined browser bytes", result.Totals)
	}
}

func TestDryRunDiscardsChromeBrowserCacheWhenPostInspectionStateIsUnsafe(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	writeChromeLocalState(t, userDataRoot, map[string]string{"Default": "Corey"})
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cache", "cache.bin"), "cache")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications: sequenceChromeDetector(
			clean.RunningApplicationStateIdle,
			clean.RunningApplicationStateRunning,
		),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("Chrome data survived unsafe post-check: %#v", result)
	}
	model := clean.NewPreviewReadModel(result)
	if len(model.RunningApplicationSkips) != 1 || !strings.Contains(model.RunningApplicationSkips[0].Name, "Google Chrome") {
		t.Fatalf("running skips = %#v, want Chrome safe skip", model.RunningApplicationSkips)
	}
}

func TestDryRunDiscardsEdgeBrowserCacheWhenPostInspectionStateIsUnsafe(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
	writeBrowserLocalState(t, userDataRoot, map[string]string{"Default": "Corey"})
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cache", "cache.bin"), "cache")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications: sequenceBrowserDetector(
			clean.RunningApplicationStateIdle,
			clean.RunningApplicationStateIdle,
			clean.RunningApplicationStateIdle,
			clean.RunningApplicationStateRunning,
		),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("Edge data survived unsafe post-check: %#v", result)
	}
	model := clean.NewPreviewReadModel(result)
	if len(model.RunningApplicationSkips) != 1 || !strings.Contains(model.RunningApplicationSkips[0].Name, "Microsoft Edge") {
		t.Fatalf("running skips = %#v, want Edge safe skip", model.RunningApplicationSkips)
	}
}

func TestDryRunSuppressesProtectedChromeBrowserCacheWithoutLeakingPaths(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	protectedCache := filepath.Join(userDataRoot, "Default", "Cache")
	writeChromeLocalState(t, userDataRoot, map[string]string{"Default": "Corey"})
	writeFile(t, filepath.Join(protectedCache, "cache.bin"), "cache")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:       pathsafe.NewValidator([]string{protectedCache}),
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleChromeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"JSON":          string(encoded),
		"human output":  report,
		"detailed list": string(detailed),
		"history":       recorder.encoded,
	} {
		for _, forbidden := range []string{protectedCache, userDataRoot, "browser_cache", "Google Chrome browser cache"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked protected Chrome data %q:\n%s", name, forbidden, text)
			}
		}
	}
	if len(result.Opportunities) != 0 ||
		len(result.IncompleteOpportunityInspections) != 0 ||
		result.Totals.OpportunityCount != 0 ||
		result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("protected Chrome cache affected result: %#v", result)
	}
}

func TestDryRunSuppressesProtectedEdgeBrowserCacheWithoutLeakingPaths(t *testing.T) {
	for _, test := range []struct {
		name           string
		protectionPath func(string, string, string) string
	}{
		{
			name: "edge root",
			protectionPath: func(localAppData, userDataRoot, protectedCache string) string {
				return userDataRoot
			},
		},
		{
			name: "profile",
			protectionPath: func(localAppData, userDataRoot, protectedCache string) string {
				return filepath.Join(userDataRoot, "Default")
			},
		},
		{
			name: "cache",
			protectionPath: func(localAppData, userDataRoot, protectedCache string) string {
				return protectedCache
			},
		},
		{
			name: "ancestor",
			protectionPath: func(localAppData, userDataRoot, protectedCache string) string {
				return localAppData
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			localAppData := t.TempDir()
			userDataRoot := filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
			protectedCache := filepath.Join(userDataRoot, "Default", "Cache")
			writeBrowserLocalState(t, userDataRoot, map[string]string{"Default": "Corey"})
			writeFile(t, filepath.Join(protectedCache, "cache.bin"), "cache")
			recorder := &recordingHistoryRecorder{}

			result := clean.DryRun(context.Background(), clean.Options{
				Validator:       pathsafe.NewValidator([]string{test.protectionPath(localAppData, userDataRoot, protectedCache)}),
				HistoryRecorder: recorder,
				DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
				BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
					LocalAppDataDir: localAppData,
				},
				DetectRunningApplications: idleBrowserDetector(),
				DiscoverOpportunities:     noUserTempOpportunities,
				DiscoverReviewSuggestions: noReviewSuggestions,
				Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
			})

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
			detailed, err := os.ReadFile(result.DetailedListPath)
			if err != nil {
				t.Fatal(err)
			}
			for name, text := range map[string]string{
				"JSON":          string(encoded),
				"human output":  report,
				"detailed list": string(detailed),
				"history":       recorder.encoded,
			} {
				for _, forbidden := range []string{protectedCache, userDataRoot, "browser_cache", "Microsoft Edge browser cache"} {
					if strings.Contains(text, forbidden) {
						t.Fatalf("%s leaked protected Edge data %q:\n%s", name, forbidden, text)
					}
				}
			}
			if len(result.Opportunities) != 0 ||
				len(result.IncompleteOpportunityInspections) != 0 ||
				result.Totals.OpportunityCount != 0 ||
				result.Totals.OpportunityObservedBytes != 0 {
				t.Fatalf("protected Edge cache affected result: %#v", result)
			}
		})
	}
}

func TestDryRunReportsChromeCatalogProblemsAsDiagnosticsWithoutScanningCaches(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	cachePath := filepath.Join(userDataRoot, "Default", "Cache")
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications:    idleChromeDetector(),
		DiscoverOpportunities:        noUserTempOpportunities,
		DiscoverReviewSuggestions:    noReviewSuggestions,
		Rules:                        []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 0 || len(result.Errors) != 1 || result.Errors[0].Code != "browser_profile_catalog_unknown" {
		t.Fatalf("result = %#v, want catalog diagnostic without cache scan", result)
	}
	if strings.Contains(clean.RenderPreviewReport(clean.NewPreviewReadModel(result)), cachePath) {
		t.Fatalf("catalog diagnostic leaked cache path")
	}
}

func TestDryRunReportsEdgeCatalogProblemsAsDiagnosticsWithoutScanningCaches(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
	cachePath := filepath.Join(userDataRoot, "Default", "Cache")
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications:    idleBrowserDetector(),
		DiscoverOpportunities:        noUserTempOpportunities,
		DiscoverReviewSuggestions:    noReviewSuggestions,
		Rules:                        []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 0 || len(result.Errors) != 1 || result.Errors[0].Code != "browser_profile_catalog_unknown" {
		t.Fatalf("result = %#v, want catalog diagnostic without cache scan", result)
	}
	if strings.Contains(clean.RenderPreviewReport(clean.NewPreviewReadModel(result)), cachePath) {
		t.Fatalf("catalog diagnostic leaked cache path")
	}
}

func TestDryRunSuppressesOnlyReviewSuggestionsWithProtectedResolvedPaths(t *testing.T) {
	protected := `C:\Users\Corey\AppData\Local\App`
	descendant := `c:\users\corey\appdata\local\app\cache`
	sibling := `C:\Users\Corey\AppData\Local\Application\cache`
	unresolvedCommand := `tool clean C:\Users\Corey\AppData\Local\App\cache`

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:                     pathsafe.NewValidator([]string{protected}),
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{
				{Tool: "exact", Label: "exact cache", Command: "exact clean", CachePath: protected},
				{Tool: "descendant", Label: "descendant cache", Command: "descendant clean", CachePath: descendant},
				{Tool: "sibling", Label: "sibling cache", Command: "sibling clean", CachePath: sibling},
				{Tool: "unresolved", Label: "unresolved cache", Command: unresolvedCommand},
			}
		},
		Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.ReviewSuggestions) != 2 {
		t.Fatalf("review suggestions = %#v, want sibling and unresolved suggestions", result.ReviewSuggestions)
	}
	if result.ReviewSuggestions[0].CachePath != sibling ||
		result.ReviewSuggestions[1].CachePath != "" ||
		result.ReviewSuggestions[1].Command != unresolvedCommand {
		t.Fatalf("review suggestions = %#v, want path-aware filtering without command inference", result.ReviewSuggestions)
	}
}

func TestDryRunReportsCandidateContractWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			Roots:          []string{root},
		}},
	})

	if result.Status != "preview" || result.Mode != "dry_run" {
		t.Fatalf("status/mode = %q/%q, want preview/dry_run", result.Status, result.Mode)
	}
	if len(result.DefaultRuleCatalog) != 1 || !result.DefaultRuleCatalog[0].DefaultEnabled {
		t.Fatalf("default rule catalog = %#v, want one default-enabled rule", result.DefaultRuleCatalog)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want one candidate", result.Candidates)
	}
	got := result.Candidates[0]
	if got.Path != candidate || got.Bytes != 5 || got.Rule != "test_default_rule" || got.PlannedAction != "move_to_recycle_bin" {
		t.Fatalf("candidate = %#v, want path/size/rule/planned action", got)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("dry-run deleted or changed candidate: %v", err)
	}
	if result.Totals.CandidateCount != 1 || result.Totals.CandidateBytes != 5 || result.Totals.SkippedCount != 0 {
		t.Fatalf("totals = %#v, want one candidate and no skipped", result.Totals)
	}
}

func TestDryRunSuppressesProtectedChromeRootBeforeCatalogDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name           string
		protectionPath func(string, string) string
	}{
		{
			name: "browser root",
			protectionPath: func(localAppData, userDataRoot string) string {
				return userDataRoot
			},
		},
		{
			name: "ancestor",
			protectionPath: func(localAppData, userDataRoot string) string {
				return localAppData
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			localAppData := t.TempDir()
			userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
			if err := os.MkdirAll(userDataRoot, 0700); err != nil {
				t.Fatal(err)
			}

			result := clean.DryRun(context.Background(), clean.Options{
				Validator:                    pathsafe.NewValidator([]string{test.protectionPath(localAppData, userDataRoot)}),
				BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
				Rules: []clean.Rule{{
					ID:             "disabled_test_rule",
					DefaultEnabled: false,
				}},
				DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
					return nil
				},
				DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
					return clean.OpportunityDiscoveryResult{}
				},
				DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
					return []clean.RunningApplicationState{{
						Application: clean.ApplicationGoogleChrome,
						State:       clean.RunningApplicationStateIdle,
					}}
				},
			})

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), userDataRoot) || len(result.Errors) != 0 {
				t.Fatalf("protected Chrome root leaked through diagnostics: result=%#v json=%s", result, encoded)
			}
		})
	}
}

func TestDryRunProjectsUserProtectionRuleAndExcludesProtectedCandidate(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-protected.tmp")
	if err := os.WriteFile(candidate, []byte("protected cache"), 0600); err != nil {
		t.Fatal(err)
	}
	detailedListDir := t.TempDir()

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:                     pathsafe.NewValidator([]string{candidate}),
		DetailedListDir:               detailedListDir,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want protected path excluded", result.Candidates)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("skipped = %#v, want protected_path", result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason.Message, "user-defined Protection rule") {
		t.Fatalf("message = %q, want user-defined rule identity", result.Skipped[0].Reason.Message)
	}
	if result.Totals.CandidateCount != 0 || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v, want protected path excluded from candidates and Potential space", result.Totals)
	}
	if len(result.ProtectionRules) != 1 || result.ProtectionRules[0].Path != candidate {
		t.Fatalf("protection rules = %#v, want active user rule", result.ProtectionRules)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"protection_rules":[{"path":`) {
		t.Fatalf("JSON missing active protection rule: %s", encoded)
	}

	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(detailed)
	defaultSection := text[strings.Index(text, "Default candidates"):strings.Index(text, "Skipped-by-default opportunities")]
	if strings.Contains(defaultSection, candidate) {
		t.Fatalf("protected path leaked into executable candidate section:\n%s", text)
	}
	if !strings.Contains(text, "Skipped items") || !strings.Contains(text, candidate) {
		t.Fatalf("detailed list missing protected skipped item:\n%s", text)
	}

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	if !strings.Contains(report, candidate) || !strings.Contains(report, "user-defined Protection rule") {
		t.Fatalf("human report missing active user protection rule:\n%s", report)
	}
	if model.PotentialSpaceBytes != 0 || model.CandidateCount != 0 {
		t.Fatalf("read model totals = %#v, want no protected candidate bytes", model)
	}
}

func TestPreviewReadModelAndHumanReportExposeProtectionDiagnostics(t *testing.T) {
	source := `C:\Users\corey\AppData\Roaming\Foal\protection.txt`
	result := clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		ProtectionDiagnostics: []clean.ProtectionDiagnostic{{
			Code:        "unc_path",
			Message:     "UNC paths cannot be used as Protection rules",
			Recoverable: true,
			Source:      source,
			Line:        7,
			Path:        `\\server\share`,
		}},
	}

	model := clean.NewPreviewReadModel(result)
	if len(model.ProtectionDiagnostics) != 1 || model.ProtectionDiagnostics[0].Code != "unc_path" {
		t.Fatalf("protection diagnostics = %#v, want shared diagnostic", model.ProtectionDiagnostics)
	}
	report := clean.RenderPreviewReport(model)
	for _, want := range []string{"Protection diagnostics", "unc_path", source, "line 7"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestDryRunProjectsUserTempOpportunitiesSeparatelyFromCandidates(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-cache.tmp")
	opportunityPath := filepath.Join(root, "old-tool-cache")
	incompletePath := filepath.Join(root, "unreadable-cache")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	latestModifiedAt := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverReviewSuggestions: noReviewSuggestions,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{{
					Path:             opportunityPath,
					Bytes:            4096,
					LatestModifiedAt: latestModifiedAt,
					IdleDays:         9,
					Status:           clean.UserTempOpportunityStatus,
					Reason:           clean.UserTempOpportunityReason,
				}},
				Incomplete: []clean.IncompleteOpportunityInspection{{
					Path: incompletePath,
					Reason: clean.StructuredIssue{
						Code:        "permission_denied",
						Message:     "access denied",
						Recoverable: true,
						Path:        incompletePath,
					},
				}},
			}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one", result.Opportunities)
	}
	if result.Opportunities[0].Path != opportunityPath || result.Opportunities[0].LatestModifiedAt != latestModifiedAt {
		t.Fatalf("opportunity = %#v, want complete review data", result.Opportunities[0])
	}
	if len(result.IncompleteOpportunityInspections) != 1 || result.IncompleteOpportunityInspections[0].Reason.Code != "permission_denied" {
		t.Fatalf("incomplete inspections = %#v, want structured permission reason", result.IncompleteOpportunityInspections)
	}
	if len(result.Errors) != 1 || result.Errors[0].Path != incompletePath {
		t.Fatalf("errors = %#v, want incomplete inspection through existing review boundary", result.Errors)
	}
	if result.Totals.CandidateCount != 1 || result.Totals.CandidateBytes != 5 {
		t.Fatalf("candidate totals = %#v, want candidate-only count and bytes", result.Totals)
	}
	if result.Totals.OpportunityCount != 1 || result.Totals.OpportunityObservedBytes != 4096 {
		t.Fatalf("opportunity totals = %#v, want separate count and observed bytes", result.Totals)
	}
}

func TestDryRunProjectsExistenceObservedCategoriesThroughReadModelReportAndDetailedList(t *testing.T) {
	root := t.TempDir()
	opportunities := []clean.UserTempOpportunity{
		{Category: clean.OpportunityCategoryWindowsErrorReporting, Path: filepath.Join(root, "WER"), Bytes: 1024, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
		{Category: clean.OpportunityCategoryExplorerThumbnailCache, Path: filepath.Join(root, "Explorer"), Bytes: 2048, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
		{Category: clean.OpportunityCategoryINetCache, Path: filepath.Join(root, "INetCache"), Bytes: 4096, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
		{Category: clean.OpportunityCategoryD3DShaderCache, Path: filepath.Join(root, "D3DSCache"), Bytes: 8192, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
		{Category: clean.OpportunityCategoryNVIDIADXCache, Path: filepath.Join(root, "NVIDIA", "DXCache"), Bytes: 16384, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
	}
	recorder := &recordingHistoryRecorder{}
	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: t.TempDir(),
		DiscoverOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: opportunities}
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	model := clean.NewPreviewReadModel(result)
	report := clean.RenderPreviewReport(model)
	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"human report":  report,
		"detailed list": string(detailed),
	} {
		for _, want := range []string{
			opportunities[0].Path,
			"category: windows_error_reporting",
			opportunities[1].Path,
			"category: explorer_thumbnail_cache",
			opportunities[2].Path,
			"category: inet_cache",
			opportunities[3].Path,
			"category: d3d_shader_cache",
			opportunities[4].Path,
			"category: nvidia_dx_cache",
			"not counted as Potential space",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing %q:\n%s", name, want, content)
			}
		}
		for _, forbidden := range []string{"0001-01-01", "idle days: 0"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains misleading age %q:\n%s", name, forbidden, content)
			}
		}
	}
	if model.PotentialSpaceBytes != 0 || model.OpportunityCount != 5 || model.OpportunityObservedBytes != 31744 {
		t.Fatalf("model totals = %#v, want review-only 5/31744 outside Potential space", model)
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 5 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 31744 {
		t.Fatalf("history = %#v / %s, want aggregate-only category totals", recorder.sessions, recorder.encoded)
	}
	for _, opportunity := range opportunities {
		if strings.Contains(recorder.encoded, opportunity.Path) {
			t.Fatalf("history persisted review-only category path %q: %s", opportunity.Path, recorder.encoded)
		}
	}
}

func TestDryRunSuppressesProtectedWindowsCacheOpportunitiesBeforeTotals(t *testing.T) {
	root := t.TempDir()
	wer := filepath.Join(root, "WER")
	explorer := filepath.Join(root, "Explorer")
	inetCache := filepath.Join(root, "INetCache")
	d3d := filepath.Join(root, "D3DSCache")
	nvidia := filepath.Join(root, "NVIDIA", "DXCache")
	visible := filepath.Join(root, "CrashDumps")

	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{
			strings.ToUpper(`\\?\` + wer),
			explorer,
			inetCache,
			d3d,
			nvidia,
		}),
		DiscoverReviewSuggestions: noReviewSuggestions,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: []clean.UserTempOpportunity{
				{Category: clean.OpportunityCategoryWindowsErrorReporting, Path: wer, Bytes: 10, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				{Category: clean.OpportunityCategoryExplorerThumbnailCache, Path: explorer, Bytes: 20, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				{Category: clean.OpportunityCategoryINetCache, Path: inetCache, Bytes: 30, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				{Category: clean.OpportunityCategoryD3DShaderCache, Path: d3d, Bytes: 35, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				{Category: clean.OpportunityCategoryNVIDIADXCache, Path: nvidia, Bytes: 37, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				{Category: clean.OpportunityCategoryCrashDumps, Path: visible, Bytes: 40, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
			}}
		},
		Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 || result.Opportunities[0].Path != visible {
		t.Fatalf("opportunities = %#v, want only unprotected category", result.Opportunities)
	}
	if result.Opportunities[0].Category != clean.OpportunityCategoryCrashDumps {
		t.Fatalf("opportunity category = %q, want crash_dumps preserved", result.Opportunities[0].Category)
	}
	if result.Totals.OpportunityCount != 1 || result.Totals.OpportunityObservedBytes != 40 {
		t.Fatalf("opportunity totals = %d/%d, want 1/40", result.Totals.OpportunityCount, result.Totals.OpportunityObservedBytes)
	}
}

func TestDryRunSuppressedReviewPathsDoNotReachDownstreamSurfacesOrHistory(t *testing.T) {
	root := t.TempDir()
	protectedRoot := filepath.Join(root, "protected")
	protectedOpportunity := filepath.Join(protectedRoot, "private-opportunity")
	protectedSuggestion := filepath.Join(protectedRoot, "private-suggestion")
	protectedIncomplete := filepath.Join(protectedRoot, "private-incomplete")
	visibleOpportunity := filepath.Join(root, "visible-opportunity")
	visibleSuggestion := filepath.Join(root, "visible-suggestion")
	visibleIncomplete := filepath.Join(root, "protected-sibling-incomplete")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:       pathsafe.NewValidator([]string{protectedRoot}),
		HistoryRecorder: recorder,
		DetailedListDir: t.TempDir(),
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{Path: protectedOpportunity, Bytes: 40, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
					{Path: visibleOpportunity, Bytes: 50, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				},
				Incomplete: []clean.IncompleteOpportunityInspection{
					{
						Path: protectedIncomplete,
						Reason: clean.StructuredIssue{
							Code:        "inspection_failed",
							Message:     "protected inspection failed",
							Recoverable: true,
							Path:        protectedIncomplete,
						},
					},
					{
						Path: visibleIncomplete,
						Reason: clean.StructuredIssue{
							Code:        "inspection_failed",
							Message:     "visible inspection failed",
							Recoverable: true,
							Path:        visibleIncomplete,
						},
					},
				},
			}
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{
				{Tool: "private", Label: "private cache", Command: "private clean", CachePath: protectedSuggestion},
				{Tool: "visible", Label: "visible cache", Command: "visible clean", CachePath: visibleSuggestion},
			}
		},
		Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.IncompleteOpportunityInspections) != 1 ||
		result.IncompleteOpportunityInspections[0].Path != visibleIncomplete {
		t.Fatalf("incomplete inspections = %#v, want only unprotected prefix sibling", result.IncompleteOpportunityInspections)
	}
	if len(result.Errors) != 1 || result.Errors[0].Path != visibleIncomplete {
		t.Fatalf("errors = %#v, want only unprotected incomplete inspection error", result.Errors)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, suppressed := range []string{protectedOpportunity, protectedSuggestion, protectedIncomplete} {
		token, err := json.Marshal(suppressed)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), strings.Trim(string(token), `"`)) {
			t.Fatalf("JSON leaked suppressed review-only path %q:\n%s", suppressed, encoded)
		}
	}
	for name, content := range map[string]string{
		"human report":  report,
		"detailed list": string(detailed),
		"raw history":   recorder.encoded,
	} {
		for _, suppressed := range []string{protectedOpportunity, protectedSuggestion, protectedIncomplete} {
			if strings.Contains(content, suppressed) {
				t.Fatalf("%s leaked suppressed review-only path %q:\n%s", name, suppressed, content)
			}
		}
	}
	for _, visible := range []string{visibleOpportunity, visibleSuggestion, visibleIncomplete} {
		token, err := json.Marshal(visible)
		if err != nil {
			t.Fatal(err)
		}
		inJSON := strings.Contains(string(encoded), strings.Trim(string(token), `"`))
		inReport := strings.Contains(report, visible)
		if !inJSON || !inReport {
			t.Fatalf("visible review path %q presence JSON/report = %t/%t\nJSON: %s\nreport:\n%s", visible, inJSON, inReport, encoded, report)
		}
	}
	if !strings.Contains(string(detailed), visibleOpportunity) {
		t.Fatalf("detailed list missing visible opportunity:\n%s", detailed)
	}
	if !strings.Contains(string(detailed), visibleIncomplete) {
		t.Fatalf("detailed list missing visible incomplete inspection:\n%s", detailed)
	}
	if strings.Contains(recorder.encoded, visibleIncomplete) {
		t.Fatalf("raw history persisted review-only incomplete inspection path: %s", recorder.encoded)
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 1 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 50 {
		t.Fatalf("history sessions = %#v, want filtered opportunity aggregate 1/50", recorder.sessions)
	}
}

func TestDryRunRecordsHistorySessionAndCandidateWithoutFileContents(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("SECRET-CACHE-CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder:               recorder,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		CommandParameters: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--dry-run"},
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if result.Totals.CandidateCount != 1 {
		t.Fatalf("candidate count = %d, want 1", result.Totals.CandidateCount)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want one history session", recorder.sessions)
	}
	session := recorder.sessions[0]
	if session.Command.Command != "clean" || len(session.Command.Args) != 2 || session.Command.Args[1] != "--dry-run" {
		t.Fatalf("command parameters = %#v, want clean --dry-run", session.Command)
	}
	if session.Mode != "dry_run" || session.StartedAt.IsZero() || session.EndedAt.IsZero() {
		t.Fatalf("session timing/mode = %#v, want dry_run with start/end", session)
	}
	if session.Aggregate.CandidateCount != 1 || session.Aggregate.CandidateBytes != int64(len("SECRET-CACHE-CONTENT")) {
		t.Fatalf("aggregate = %#v, want one candidate and byte total", session.Aggregate)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("items = %#v, want one history item", recorder.items)
	}
	item := recorder.items[0]
	if item.Path != candidate || item.Rule != "test_default_rule" || item.PlannedAction != "move_to_recycle_bin" || item.Result != "candidate" {
		t.Fatalf("item = %#v, want candidate path/rule/action/result", item)
	}
	if item.Bytes == nil || *item.Bytes != int64(len("SECRET-CACHE-CONTENT")) {
		t.Fatalf("item bytes = %#v, want file size metadata", item.Bytes)
	}
	if strings.Contains(recorder.encoded, "SECRET-CACHE-CONTENT") {
		t.Fatalf("history captured file contents: %s", recorder.encoded)
	}
}

func TestDryRunDoesNotPersistOpportunityPathsInExistingHistoryItems(t *testing.T) {
	root := t.TempDir()
	opportunityPath := filepath.Join(root, "old-tool-cache")
	incompletePath := filepath.Join(root, "unreadable-cache")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder:           recorder,
		DiscoverReviewSuggestions: noReviewSuggestions,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{{
					Path:   opportunityPath,
					Bytes:  4096,
					Status: clean.UserTempOpportunityStatus,
					Reason: clean.UserTempOpportunityReason,
				}},
				Incomplete: []clean.IncompleteOpportunityInspection{{
					Path: incompletePath,
					Reason: clean.StructuredIssue{
						Code:        "permission_denied",
						Message:     "access denied",
						Recoverable: true,
						Path:        incompletePath,
					},
				}},
			}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
		}},
	})

	if len(result.Opportunities) != 1 || len(result.Errors) != 1 {
		t.Fatalf("dry-run review result = %#v, want opportunity and incomplete error", result)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want one history session", recorder.sessions)
	}
	aggregate := recorder.sessions[0].Aggregate
	if aggregate.OpportunityCount != 1 || aggregate.OpportunityObservedBytes != 4096 {
		t.Fatalf("aggregate = %#v, want privacy-preserving opportunity totals", aggregate)
	}
	if aggregate.ErrorCount != 0 {
		t.Fatalf("aggregate error count = %d, want incomplete review error excluded", aggregate.ErrorCount)
	}
	for _, item := range recorder.items {
		if item.Path == opportunityPath || item.Path == incompletePath {
			t.Fatalf("history item persisted review-only opportunity path: %#v", item)
		}
	}
	if strings.Contains(recorder.encoded, opportunityPath) || strings.Contains(recorder.encoded, incompletePath) {
		t.Fatalf("history encoding persisted review-only opportunity path: %s", recorder.encoded)
	}
}

func TestDryRunDoesNotPersistReviewSuggestionsInHistory(t *testing.T) {
	recorder := &recordingHistoryRecorder{}
	cachePath := `C:\Users\corey\AppData\Local\npm-cache`
	command := "npm cache clean --force"

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder:               recorder,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{{
				Tool:      "npm",
				Label:     "npm cache",
				Command:   command,
				CachePath: cachePath,
			}}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
		}},
	})

	if len(result.ReviewSuggestions) != 1 {
		t.Fatalf("review suggestions = %#v, want one in preview result", result.ReviewSuggestions)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want one dry-run history session", recorder.sessions)
	}
	if strings.Contains(recorder.encoded, cachePath) || strings.Contains(recorder.encoded, command) || strings.Contains(recorder.encoded, "npm cache") {
		t.Fatalf("history persisted review suggestion: %s", recorder.encoded)
	}
}

func TestProjectArtifactReviewClueStaysOutOfResultDetailedListAndHistory(t *testing.T) {
	recorder := &recordingHistoryRecorder{}
	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder:               recorder,
		DetailedListDir:               t.TempDir(),
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
		}},
	})
	model := clean.NewPreviewReadModel(result)
	if len(model.ReviewClues) != 1 {
		t.Fatalf("review clues = %#v, want presentation-only project artifact clue", model.ReviewClues)
	}

	encodedResult, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	for surface, data := range map[string]string{
		"clean.Result JSON": string(encodedResult),
		"detailed list":     string(detailed),
		"history":           recorder.encoded,
	} {
		if strings.Contains(data, "Rebuildable project artifacts") ||
			strings.Contains(data, "foal analyze <path>") {
			t.Fatalf("%s contains presentation-only project artifact clue:\n%s", surface, data)
		}
	}
	if len(result.Deleted) != 0 || result.Totals.DeletedCount != 0 || result.Totals.AffectedBytes != 0 {
		t.Fatalf("dry-run gained deletion semantics: %#v", result)
	}
}

func TestDryRunSkipsUnsafePathsThroughPathSafetyValidation(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{`\\?\C:\Windows\System32`},
		}},
	})

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none", result.Candidates)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one skipped unsafe path", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Path != `\\?\C:\Windows\System32` || skipped.Rule != "test_default_rule" {
		t.Fatalf("skipped = %#v, want original path and rule", skipped)
	}
	if skipped.Reason.Code != "protected_path" || skipped.Reason.Message == "" || !skipped.Reason.Recoverable {
		t.Fatalf("reason = %#v, want recoverable protected_path", skipped.Reason)
	}
}

func TestDryRunRecordsSkippedAndErrorHistoryItems(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder:               recorder,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		CommandParameters: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--dry-run"},
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{`\\?\C:\Windows\System32`},
			Roots:          []string{missingRoot},
		}},
	})

	if result.Totals.SkippedCount != 1 || len(result.Errors) != 1 {
		t.Fatalf("result skipped/errors = %d/%d, want 1/1", result.Totals.SkippedCount, len(result.Errors))
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want one history session", recorder.sessions)
	}
	if recorder.sessions[0].Aggregate.SkippedCount != 1 || recorder.sessions[0].Aggregate.ErrorCount != 1 {
		t.Fatalf("aggregate = %#v, want skipped and error counts", recorder.sessions[0].Aggregate)
	}
	if len(recorder.items) != 2 {
		t.Fatalf("items = %#v, want skipped and error records", recorder.items)
	}
	skipped := recorder.items[0]
	if skipped.Result != "skipped" || skipped.Path != `\\?\C:\Windows\System32` || skipped.Rule != "test_default_rule" {
		t.Fatalf("skipped item = %#v, want unsafe path skipped", skipped)
	}
	mustHaveIssue(t, skipped.SkippedReason, "protected_path")
	errorItem := recorder.items[1]
	if errorItem.Result != "error" || errorItem.Path != missingRoot || errorItem.Rule != "test_default_rule" {
		t.Fatalf("error item = %#v, want missing root error", errorItem)
	}
	mustHaveIssue(t, errorItem.Error, "not_found")
}

func TestDryRunWritesDetailedCandidateListUnderConfiguredHistoryArea(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-cache.tmp")
	if err := os.WriteFile(candidate, []byte("SECRET-CACHE-CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	missingRoot := filepath.Join(root, "missing")
	detailedListDir := filepath.Join(t.TempDir(), "Foal", "history")

	result := clean.DryRun(context.Background(), clean.Options{
		DetailedListDir:               detailedListDir,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate, `\\?\C:\Windows\System32`},
			Roots:          []string{missingRoot},
		}},
	})

	if result.DetailedListPath == "" {
		t.Fatal("detailed list path is empty")
	}
	if gotDir := filepath.Dir(result.DetailedListPath); gotDir != detailedListDir {
		t.Fatalf("detailed list dir = %q, want %q", gotDir, detailedListDir)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.Base(result.DetailedListPath))); !os.IsNotExist(err) {
		t.Fatalf("detailed list was written near working candidate root; stat err = %v", err)
	}

	data, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatalf("read detailed list: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"Foal clean detailed candidate list",
		"Companion file only",
		candidate,
		"bytes: 20",
		"planned action: Recycle Bin",
		`\\?\C:\Windows\System32`,
		"reason: protected_path",
		missingRoot,
		"error: not_found",
		"recoverable: true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("detailed list missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "SECRET-CACHE-CONTENT") {
		t.Fatalf("detailed list leaked file contents:\n%s", text)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal dry-run result: %v", err)
	}
	if strings.Contains(string(encoded), "detailed_list_path") || strings.Contains(string(encoded), result.DetailedListPath) {
		t.Fatalf("dry-run JSON leaked detailed list path:\n%s", encoded)
	}
}

func TestDryRunUsesOnlyDefaultEnabledRules(t *testing.T) {
	root := t.TempDir()
	defaultPath := filepath.Join(root, "default.tmp")
	optInPath := filepath.Join(root, "opt-in.tmp")
	if err := os.WriteFile(defaultPath, []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(optInPath, []byte("opt-in"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{
			{
				ID:             "approved_default_rule",
				Description:    "approved default rule",
				DefaultEnabled: true,
				CandidatePaths: []string{defaultPath},
			},
			{
				ID:             "future_opt_in_rule",
				Description:    "future opt-in rule",
				DefaultEnabled: false,
				CandidatePaths: []string{optInPath},
			},
		},
	})

	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only the default-enabled rule", result.Candidates)
	}
	if result.Candidates[0].Path != defaultPath || result.Candidates[0].Rule != "approved_default_rule" {
		t.Fatalf("candidate = %#v, want default-enabled path only", result.Candidates[0])
	}
	if _, err := os.Stat(optInPath); err != nil {
		t.Fatalf("dry-run touched opt-in path: %v", err)
	}
}

func TestUserProtectionRulesCannotEnableOrExpandDefaultCandidates(t *testing.T) {
	root := t.TempDir()
	defaultPath := filepath.Join(root, "default.tmp")
	disabledPath := filepath.Join(root, "disabled.tmp")
	unruledPath := filepath.Join(root, "unruled.tmp")
	for _, path := range []string{defaultPath, disabledPath, unruledPath} {
		if err := os.WriteFile(path, []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:                     pathsafe.NewValidator([]string{unruledPath}),
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{
			{
				ID:             "default_rule",
				DefaultEnabled: true,
				CandidatePaths: []string{defaultPath},
			},
			{
				ID:             "disabled_rule",
				DefaultEnabled: false,
				CandidatePaths: []string{disabledPath},
			},
		},
	})

	if len(result.Candidates) != 1 || result.Candidates[0].Path != defaultPath {
		t.Fatalf("candidates = %#v, want unchanged default candidate only", result.Candidates)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("skipped = %#v, want unruled protection path absent rather than added", result.Skipped)
	}
}

func TestDryRunRootRuleHonorsCandidateNamePrefixes(t *testing.T) {
	root := t.TempDir()
	foalOwned := filepath.Join(root, "foal-owned.tmp")
	unrelated := filepath.Join(root, "vscode-cache.tmp")
	if err := os.WriteFile(foalOwned, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:                    "foal_owned_temp_sandboxes",
			Description:           "Foal-owned temporary sandbox entries",
			DefaultEnabled:        true,
			Roots:                 []string{root},
			CandidateNamePrefixes: []string{"foal-"},
		}},
	})

	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only Foal-owned temp candidate", result.Candidates)
	}
	if result.Candidates[0].Path != foalOwned {
		t.Fatalf("candidate path = %q, want %q", result.Candidates[0].Path, foalOwned)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("dry-run touched unrelated temp path: %v", err)
	}
}

func TestPreviewReadModelUsesExistingDefaultCandidatesForPotentialSpace(t *testing.T) {
	result := clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		DefaultRuleCatalog: []clean.RuleSummary{
			{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			},
			{
				ID:             "future_opt_in_rule",
				Description:    "future opt-in rule",
				DefaultEnabled: false,
			},
		},
		Candidates: []clean.CandidatePreview{
			{
				Path:          filepath.Join(t.TempDir(), "foal-a.tmp"),
				Bytes:         7,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			},
			{
				Path:          filepath.Join(t.TempDir(), "foal-b.tmp"),
				Bytes:         11,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			},
		},
		Skipped: []clean.SkippedItem{{
			Path:  filepath.Join(t.TempDir(), "skipped.tmp"),
			Bytes: 4096,
			Rule:  "future_opt_in_rule",
			Reason: clean.StructuredIssue{
				Code:        "permission_denied",
				Message:     "access denied",
				Recoverable: true,
			},
		}},
	}

	model := clean.NewPreviewReadModel(result)

	if model.Status != "preview_only" {
		t.Fatalf("status = %q, want preview_only", model.Status)
	}
	if model.PotentialSpaceBytes != 18 {
		t.Fatalf("potential space = %d, want candidate bytes only", model.PotentialSpaceBytes)
	}
	if model.CandidateCount != 2 || model.SkippedCount != 1 {
		t.Fatalf("counts = candidates %d skipped %d, want 2/1", model.CandidateCount, model.SkippedCount)
	}
	if len(model.ProtectionRules) != 1 {
		t.Fatalf("protection rules = %#v, want one default-enabled rule", model.ProtectionRules)
	}
	if model.ProtectionRules[0].ID != "foal_owned_temp_sandboxes" {
		t.Fatalf("protection rule = %#v, want default Foal rule", model.ProtectionRules[0])
	}
	if strings.Contains(model.Summary, "Whitelist") || !strings.Contains(model.Summary, "No changes were made") {
		t.Fatalf("summary = %q, want dry-run no-changes language without Whitelist", model.Summary)
	}
}

func TestNewPreviewReadModelAddsOneProjectArtifactReviewClueWithoutChangingCleanupAccounting(t *testing.T) {
	result := clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		Candidates: []clean.CandidatePreview{{
			Path:          `C:\Temp\foal-owned.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Skipped: []clean.SkippedItem{{
			Path: `C:\Temp\skipped.tmp`,
			Reason: clean.StructuredIssue{
				Code:        "protected_path",
				Recoverable: true,
			},
		}},
		Opportunities: []clean.Opportunity{{
			Path:     `C:\Temp\old-cache`,
			Bytes:    4096,
			Category: clean.OpportunityCategoryUserTemp,
		}},
		Errors: []clean.StructuredIssue{{
			Code:        "inspection_failed",
			Recoverable: true,
		}},
		Totals: clean.Totals{
			CandidateCount:           1,
			SkippedCount:             1,
			OpportunityCount:         1,
			CandidateBytes:           12,
			OpportunityObservedBytes: 4096,
		},
	}

	model := clean.NewPreviewReadModel(result)

	if len(model.ReviewClues) != 1 {
		t.Fatalf("review clues = %#v, want exactly one project artifact clue", model.ReviewClues)
	}
	clue := model.ReviewClues[0]
	if clue.Name != "Rebuildable project artifacts" || clue.Path != "" || !strings.Contains(clue.Details, "foal analyze <path>") {
		t.Fatalf("review clue = %#v, want empty-path project analysis guidance", clue)
	}
	if model.PotentialSpaceBytes != 12 ||
		model.CandidateCount != 1 ||
		model.SkippedCount != 1 ||
		model.OpportunityCount != 1 ||
		model.OpportunityObservedBytes != 4096 ||
		len(model.Candidates) != 1 ||
		len(model.Skipped) != 1 ||
		len(model.Opportunities) != 1 ||
		len(model.Errors) != 1 {
		t.Fatalf("project artifact clue changed cleanup accounting: %#v", model)
	}
}

func TestPreviewReadModelProjectsOpportunitiesWithoutChangingPotentialSpace(t *testing.T) {
	latestModifiedAt := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	result := clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		Candidates: []clean.CandidatePreview{{
			Path:          `C:\Temp\foal-owned.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Opportunities: []clean.UserTempOpportunity{{
			Path:             `C:\Temp\old-tool-cache`,
			Bytes:            4096,
			LatestModifiedAt: latestModifiedAt,
			IdleDays:         9,
			Status:           clean.UserTempOpportunityStatus,
			Reason:           clean.UserTempOpportunityReason,
		}},
		IncompleteOpportunityInspections: []clean.IncompleteOpportunityInspection{{
			Path: `C:\Temp\unreadable-cache`,
			Reason: clean.StructuredIssue{
				Code:        "permission_denied",
				Message:     "access denied",
				Recoverable: true,
				Path:        `C:\Temp\unreadable-cache`,
			},
		}},
		Totals: clean.Totals{
			CandidateCount:           1,
			CandidateBytes:           12,
			OpportunityCount:         1,
			OpportunityObservedBytes: 4096,
		},
	}

	model := clean.NewPreviewReadModel(result)

	if model.PotentialSpaceBytes != 12 {
		t.Fatalf("potential space = %d, want candidate bytes only", model.PotentialSpaceBytes)
	}
	if model.OpportunityCount != 1 || model.OpportunityObservedBytes != 4096 {
		t.Fatalf("opportunity totals = %d/%d, want 1/4096", model.OpportunityCount, model.OpportunityObservedBytes)
	}
	if len(model.Opportunities) != 1 || model.Opportunities[0].LatestModifiedAt != latestModifiedAt {
		t.Fatalf("opportunities = %#v, want complete review projection", model.Opportunities)
	}
	if len(model.IncompleteOpportunityInspections) != 1 || model.IncompleteOpportunityInspections[0].Reason.Code != "permission_denied" {
		t.Fatalf("incomplete inspections = %#v, want structured reason", model.IncompleteOpportunityInspections)
	}
	if len(model.SkippedByDefault) != 0 {
		t.Fatalf("legacy skipped-by-default items = %#v, want opportunity projection kept separate for later TUI work", model.SkippedByDefault)
	}
}

func TestPreviewReadModelRepresentsSkippedUnsafePathsAndRecoverableErrors(t *testing.T) {
	result := clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		DefaultRuleCatalog: []clean.RuleSummary{{
			ID:             "foal_owned_temp_sandboxes",
			Description:    "Foal-owned temporary sandbox entries",
			DefaultEnabled: true,
		}},
		Candidates: []clean.CandidatePreview{{
			Path:          filepath.Join(t.TempDir(), "foal-candidate.tmp"),
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Skipped: []clean.SkippedItem{{
			Path:  `\\?\C:\Windows\System32`,
			Bytes: 4096,
			Rule:  "foal_owned_temp_sandboxes",
			Reason: clean.StructuredIssue{
				Code:        "protected_path",
				Message:     "protected Windows location",
				Recoverable: true,
				Path:        `\\?\C:\Windows\System32`,
				Rule:        "foal_owned_temp_sandboxes",
			},
		}},
		Errors: []clean.StructuredIssue{{
			Code:        "inspection_failed",
			Message:     "could not inspect root",
			Recoverable: true,
			Path:        filepath.Join(t.TempDir(), "missing-root"),
			Rule:        "foal_owned_temp_sandboxes",
		}},
	}

	model := clean.NewPreviewReadModel(result)

	if model.PotentialSpaceBytes != 12 {
		t.Fatalf("potential space = %d, want candidate bytes only", model.PotentialSpaceBytes)
	}
	if len(model.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one skipped unsafe path", model.Skipped)
	}
	skipped := model.Skipped[0]
	if skipped.Path != `\\?\C:\Windows\System32` || skipped.Rule != "foal_owned_temp_sandboxes" || skipped.Reason.Code != "protected_path" {
		t.Fatalf("skipped = %#v, want protected path skip with rule and reason", skipped)
	}
	if len(model.Errors) != 1 {
		t.Fatalf("errors = %#v, want one recoverable inspection error", model.Errors)
	}
	err := model.Errors[0]
	if err.Code != "inspection_failed" || !err.Recoverable || err.Path == "" {
		t.Fatalf("error = %#v, want recoverable inspection error metadata", err)
	}
	if len(model.Notices) != 2 ||
		model.Notices[0].Kind != "permission_boundary" ||
		model.Notices[1].Kind != "permission_boundary" {
		t.Fatalf("notices = %#v, want catalog and observed permission boundary notices", model.Notices)
	}
}

func TestNewPreviewReadModelCommunicatesAdministratorOnlyCacheExclusionsWithoutElevationAdvice(t *testing.T) {
	model := clean.NewPreviewReadModel(clean.Result{
		Status: "preview",
		Mode:   "dry_run",
		Totals: clean.Totals{},
	})

	output := clean.RenderPreviewReport(model)
	for _, expected := range []string{
		"SoftwareDistribution",
		"Delivery Optimization",
		"administrator-only",
		"excluded from Opportunity discovery",
		"will not request elevation automatically",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("report missing permission-boundary wording %q:\n%s", expected, output)
		}
	}
	for _, forbidden := range []string{
		"Run as Administrator",
		"run as administrator",
		"enable automatic elevation",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("report recommends elevation with %q:\n%s", forbidden, output)
		}
	}
}

func TestPreviewReportRendersProjectArtifactClueAsAnalysisOnlyGuidance(t *testing.T) {
	output := clean.RenderPreviewReport(clean.NewPreviewReadModel(clean.Result{
		Status: "preview",
		Mode:   "dry_run",
	}))

	start := strings.Index(output, "\nReview clues\n")
	end := strings.Index(output[start+1:], "\nInspection errors\n")
	if start == -1 || end == -1 {
		t.Fatalf("output missing Review clues section:\n%s", output)
	}
	reviewClues := output[start : start+1+end]
	for _, want := range []string{
		"Rebuildable project artifacts",
		"foal analyze <path>",
	} {
		if !strings.Contains(reviewClues, want) {
			t.Fatalf("Review clues missing %q:\n%s", want, reviewClues)
		}
	}
	for _, forbidden := range []string{
		"delete",
		"Delete",
		"execute",
		"Execute",
		"default candidate",
		"Recycle Bin",
	} {
		if strings.Contains(reviewClues, forbidden) {
			t.Fatalf("Review clues contain cleanup semantics %q:\n%s", forbidden, reviewClues)
		}
	}
}

func TestPreviewReportRendersReviewOnlySectionsWithoutExecutionSemantics(t *testing.T) {
	model := clean.PreviewReadModel{
		Title:  "Foal clean",
		Status: "preview_only",
		ProtectionRules: []clean.PreviewProtectionRule{{
			ID:          "foal_owned_temp_sandboxes",
			Description: "Foal-owned temporary sandbox entries",
		}},
		Candidates: []clean.PreviewCandidate{{
			Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		SkippedByDefault: []clean.PreviewSkippedByDefaultItem{{
			Name:   "Browser cache family",
			Path:   `C:\Users\corey\AppData\Local\Browser\Cache`,
			Bytes:  4096,
			Reason: "requires explicit future opt-in",
		}},
		ReviewClues: []clean.PreviewReviewClue{{
			Name:    "Project artifact clue",
			Path:    `D:\Code\Personal\Foal\node_modules`,
			Details: "Rebuildable project output; review manually before deleting.",
		}},
		ReviewSuggestions: []clean.PreviewReviewSuggestion{{
			Label:    "Open Windows Storage settings",
			NextStep: "Use Windows Settings to review large app storage.",
		}},
		RunningApplicationSkips: []clean.PreviewRunningApplicationSkip{{
			Name:        "Sync client cache",
			Path:        `C:\Users\corey\Sync\Cache`,
			Application: "SyncClient.exe",
			Reason:      "application is running",
		}},
		PotentialSpaceBytes: 12,
		CandidateCount:      1,
		SkippedCount:        0,
		Summary:             "Dry-run summary: No changes were made.",
	}

	output := clean.RenderPreviewReport(model)

	for _, want := range []string{
		"Potential space: 12 bytes",
		"Default candidates",
		`C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
		"Skipped by default",
		"Browser cache family",
		"Review clues",
		"Project artifact clue",
		"Review suggestions",
		"Use Windows Settings to review large app storage.",
		"Running application skips",
		"SyncClient.exe",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"Potential space: 4108 bytes",
		"Potential space: 8204 bytes",
		"planned action: Recycle Bin)\n  Browser cache family",
		"close",
		"Close",
		"execute",
		"Execute",
		"move_to_recycle_bin",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden execution semantics %q:\n%s", forbidden, output)
		}
	}
}

func TestPreviewReportRendersReviewSuggestionSafetyNoteOnceAboveSuggestions(t *testing.T) {
	model := clean.PreviewReadModel{
		Title: "Foal clean",
		ReviewSuggestions: []clean.PreviewReviewSuggestion{
			{Label: "Review npm cache"},
			{Label: "Review Go build cache"},
		},
	}

	output := clean.RenderPreviewReport(model)
	note := "Clearing a tool cache while the tool is installing or building can disrupt that operation. Confirm the tool is idle first."

	if strings.Count(output, note) != 1 {
		t.Fatalf("safety note count = %d, want 1:\n%s", strings.Count(output, note), output)
	}
	headingIndex := strings.Index(output, "Review suggestions\n")
	noteIndex := strings.Index(output, note)
	firstSuggestionIndex := strings.Index(output, "Review npm cache")
	if headingIndex == -1 || noteIndex <= headingIndex || firstSuggestionIndex <= noteIndex {
		t.Fatalf("safety note must render above the suggestions list:\n%s", output)
	}
}

func TestPreviewReportOmitsReviewSuggestionSafetyNoteWithoutSuggestions(t *testing.T) {
	output := clean.RenderPreviewReport(clean.PreviewReadModel{Title: "Foal clean"})

	if strings.Contains(output, "Clearing a tool cache while the tool is installing or building") {
		t.Fatalf("safety note must not render without suggestions:\n%s", output)
	}
}

func TestPreviewReportCapsHighVolumePathSectionsAtTenEntries(t *testing.T) {
	model := clean.PreviewReadModel{
		Title:               "Foal clean",
		Status:              "preview_only",
		PotentialSpaceBytes: 66,
		CandidateCount:      11,
		SkippedCount:        11,
		DetailedListPath:    `C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
		Summary:             "Dry-run summary: No changes were made.",
	}
	for i := 0; i < 11; i++ {
		model.Candidates = append(model.Candidates, clean.PreviewCandidate{
			Path:          fmt.Sprintf(`C:\Users\corey\AppData\Local\Temp\foal-candidate-%02d.tmp`, i),
			Bytes:         int64(i + 1),
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		})
		model.Skipped = append(model.Skipped, clean.PreviewSkippedItem{
			Path:  fmt.Sprintf(`\\?\C:\Windows\System32\foal-skip-%02d.tmp`, i),
			Bytes: int64(i + 1),
			Rule:  "foal_owned_temp_sandboxes",
			Reason: clean.StructuredIssue{
				Code:        "protected_path",
				Message:     "protected Windows location",
				Recoverable: true,
			},
		})
		model.Errors = append(model.Errors, clean.StructuredIssue{
			Code:        "inspection_failed",
			Message:     "could not inspect root",
			Recoverable: true,
			Path:        fmt.Sprintf(`C:\Users\corey\AppData\Local\Temp\foal-error-%02d`, i),
			Rule:        "foal_owned_temp_sandboxes",
		})
	}

	output := clean.RenderPreviewReport(model)

	for _, want := range []string{
		"Potential space: 66 bytes",
		"Candidates: 11, skipped: 11, errors: 11.",
		`C:\Users\corey\AppData\Local\Temp\foal-candidate-09.tmp`,
		`\\?\C:\Windows\System32\foal-skip-09.tmp`,
		`C:\Users\corey\AppData\Local\Temp\foal-error-09`,
		`1 omitted. See detailed candidate list for full path detail: C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		`C:\Users\corey\AppData\Local\Temp\foal-candidate-10.tmp`,
		`\\?\C:\Windows\System32\foal-skip-10.tmp`,
		`C:\Users\corey\AppData\Local\Temp\foal-error-10`,
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains truncated path %q:\n%s", forbidden, output)
		}
	}
	if got := strings.Count(output, "omitted. See detailed candidate list for full path detail:"); got != 3 {
		t.Fatalf("omitted detail lines = %d, want 3:\n%s", got, output)
	}
}

func TestPreviewReportCapsSkippedByDefaultOpportunities(t *testing.T) {
	model := clean.PreviewReadModel{
		Title:                    "Foal clean",
		Status:                   "preview_only",
		PotentialSpaceBytes:      12,
		CandidateCount:           1,
		OpportunityCount:         11,
		OpportunityObservedBytes: 66,
		DetailedListPath:         `C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
		Summary:                  "Dry-run summary: No changes were made.",
	}
	for i := 0; i < 11; i++ {
		model.Opportunities = append(model.Opportunities, clean.UserTempOpportunity{
			Path:             fmt.Sprintf(`C:\Users\corey\AppData\Local\Temp\old-cache-%02d`, i),
			Bytes:            int64(i + 1),
			LatestModifiedAt: time.Date(2026, time.May, i+1, 12, 0, 0, 0, time.UTC),
			IdleDays:         10 + i,
			Status:           clean.UserTempOpportunityStatus,
			Reason:           clean.UserTempOpportunityReason,
		})
	}

	output := clean.RenderPreviewReport(model)

	for _, want := range []string{
		"Potential space: 12 bytes",
		"Skipped by default",
		"Opportunities: 11, observed bytes: 66 bytes (not counted as Potential space)",
		`C:\Users\corey\AppData\Local\Temp\old-cache-09`,
		"latest modified: 2026-05-10T12:00:00Z",
		"idle days: 19",
		"status: skipped_by_default",
		"reason: requires_explicit_opt_in",
		`1 omitted. See detailed candidate list for full path detail: C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"Potential space: 78 bytes",
		`C:\Users\corey\AppData\Local\Temp\old-cache-10`,
		"planned action",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden opportunity semantics %q:\n%s", forbidden, output)
		}
	}
}

func TestDryRunDetailedCandidateListStaysCompleteWhenTerminalReportWouldBeCapped(t *testing.T) {
	root := t.TempDir()
	missingRoot := filepath.Join(root, "missing")
	detailedListDir := filepath.Join(t.TempDir(), "Foal", "history")
	var candidatePaths []string
	var skippedPaths []string
	var errorRoots []string
	for i := 0; i < 11; i++ {
		candidate := filepath.Join(root, fmt.Sprintf("foal-candidate-%02d.tmp", i))
		if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
		candidatePaths = append(candidatePaths, candidate)
		skippedPaths = append(skippedPaths, fmt.Sprintf(`\\?\C:\Windows\System32\foal-skip-%02d.tmp`, i))
		errorRoots = append(errorRoots, filepath.Join(missingRoot, fmt.Sprintf("foal-error-%02d", i)))
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DetailedListDir:               detailedListDir,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: append(candidatePaths, skippedPaths...),
			Roots:          errorRoots,
		}},
	})

	if result.Totals.CandidateCount != 11 || result.Totals.SkippedCount != 11 || len(result.Errors) != 11 {
		t.Fatalf("result counts = candidates %d skipped %d errors %d, want 11/11/11", result.Totals.CandidateCount, result.Totals.SkippedCount, len(result.Errors))
	}
	data, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatalf("read detailed list: %v", err)
	}
	text := string(data)
	for _, path := range append(append(candidatePaths, skippedPaths...), errorRoots...) {
		if !strings.Contains(text, path) {
			t.Fatalf("detailed list missing path %q:\n%s", path, text)
		}
	}
}

func TestDryRunDetailedListContainsCompleteSkippedByDefaultOpportunities(t *testing.T) {
	root := t.TempDir()
	detailedListDir := filepath.Join(t.TempDir(), "Foal", "history")
	var opportunities []clean.UserTempOpportunity
	for i := 0; i < 11; i++ {
		opportunities = append(opportunities, clean.UserTempOpportunity{
			Path:             filepath.Join(root, fmt.Sprintf("old-cache-%02d", i)),
			Bytes:            int64(i + 1),
			LatestModifiedAt: time.Date(2026, time.May, i+1, 12, 0, 0, 0, time.UTC),
			IdleDays:         10 + i,
			Status:           clean.UserTempOpportunityStatus,
			Reason:           clean.UserTempOpportunityReason,
		})
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DetailedListDir:           detailedListDir,
		DiscoverReviewSuggestions: noReviewSuggestions,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: opportunities}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
		}},
	})

	data, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatalf("read detailed list: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"Skipped-by-default opportunities",
		"This section is review-only and is not an execution manifest.",
		opportunities[0].Path,
		opportunities[10].Path,
		"bytes: 11",
		"latest modified: 2026-05-11T12:00:00Z",
		"idle days: 20",
		"status: skipped_by_default",
		"reason: requires_explicit_opt_in",
		"not counted as Potential space",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("detailed list missing %q:\n%s", want, text)
		}
	}
}

func writeChromeLocalState(t *testing.T, userDataRoot string, profiles map[string]string) {
	t.Helper()
	writeBrowserLocalState(t, userDataRoot, profiles)
}

func writeBrowserLocalState(t *testing.T, userDataRoot string, profiles map[string]string) {
	t.Helper()
	if err := os.MkdirAll(userDataRoot, 0700); err != nil {
		t.Fatal(err)
	}
	info := make(map[string]map[string]string, len(profiles))
	for id, name := range profiles {
		info[id] = map[string]string{"name": name}
	}
	payload := map[string]any{
		"profile": map[string]any{
			"info_cache": info,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDataRoot, "Local State"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func idleChromeDetector() func(context.Context) []clean.RunningApplicationState {
	return sequenceChromeDetector(clean.RunningApplicationStateIdle, clean.RunningApplicationStateIdle)
}

func idleBrowserDetector() func(context.Context) []clean.RunningApplicationState {
	return sequenceBrowserDetector(
		clean.RunningApplicationStateIdle,
		clean.RunningApplicationStateIdle,
		clean.RunningApplicationStateIdle,
		clean.RunningApplicationStateIdle,
	)
}

func sequenceChromeDetector(states ...clean.RunningApplicationStatus) func(context.Context) []clean.RunningApplicationState {
	call := 0
	return func(context.Context) []clean.RunningApplicationState {
		state := states[len(states)-1]
		if call < len(states) {
			state = states[call]
		}
		call++
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationGoogleChrome, State: state},
			{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
		}
	}
}

func sequenceBrowserDetector(states ...clean.RunningApplicationStatus) func(context.Context) []clean.RunningApplicationState {
	call := 0
	return func(context.Context) []clean.RunningApplicationState {
		chromeState := states[len(states)-2]
		edgeState := states[len(states)-1]
		stateIndex := call * 2
		if stateIndex+1 < len(states) {
			chromeState = states[stateIndex]
			edgeState = states[stateIndex+1]
		}
		call++
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationGoogleChrome, State: chromeState},
			{Application: clean.ApplicationMicrosoftEdge, State: edgeState},
		}
	}
}
