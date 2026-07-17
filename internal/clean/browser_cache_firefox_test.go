package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestDryRunReportsFirefoxBrowserCacheOpportunityThroughReviewSurfaces(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/abcd1234.default-release"
	workRel := "Profiles/efgh5678.work"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "default-release", Path: profileRel, IsRelative: true},
		{Name: "work", Path: workRel, IsRelative: true},
	})
	// Regenerable cache under Local mirror.
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "entries", "a"), "cache")
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(workRel), "cache2", "entries", "b"), "work")
	// Privacy / non-allowlisted state under Roaming must never become candidates.
	roamingProfile := filepath.Join(roamingAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel))
	writeFile(t, filepath.Join(roamingProfile, "cookies.sqlite"), "cookies must not count")
	writeFile(t, filepath.Join(roamingProfile, "places.sqlite"), "places must not count")
	writeFile(t, filepath.Join(roamingProfile, "logins.json"), "logins must not count")
	writeFile(t, filepath.Join(roamingProfile, "sessionstore.jsonlz4"), "session must not count")
	writeFile(t, filepath.Join(roamingProfile, "extensions", "ext.xpi"), "extension must not count")
	// Non-allowlisted Local siblings must not count either.
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "startupCache", "startup.bin"), "startup must not count")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one Firefox browser cache opportunity", result.Opportunities)
	}
	opportunity := result.Opportunities[0]
	if opportunity.Category != clean.OpportunityCategoryBrowserCache ||
		opportunity.Bytes != 9 ||
		opportunity.Status != clean.OpportunityStatus ||
		opportunity.Reason != clean.OpportunityReason ||
		opportunity.BrowserCache == nil ||
		opportunity.BrowserCache.Browser != clean.ApplicationMozillaFirefox ||
		opportunity.BrowserCache.ProfileCount != 2 {
		t.Fatalf("Firefox opportunity = %#v, want complete browser cache summary", opportunity)
	}
	if result.Totals.OpportunityCount != 1 ||
		result.Totals.OpportunityObservedBytes != 9 ||
		result.Totals.CandidateBytes != 0 ||
		result.Totals.CandidateCount != 0 {
		t.Fatalf("totals = %#v, want observed-only Firefox bytes", result.Totals)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, want := range []string{
		`"category":"browser_cache"`,
		`"browser":"mozilla_firefox"`,
		`"profile_count":2`,
		`"kind":"cache2"`,
		`"opportunity_observed_bytes":9`,
		`"candidate_bytes":0`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("JSON missing %q: %s", want, jsonText)
		}
	}
	for _, forbidden := range []string{
		"cookies.sqlite", "places.sqlite", "logins.json", "sessionstore", "extensions", "startupCache",
		"move_to_recycle_bin", "firefox_cache",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded Firefox data %q: %s", forbidden, jsonText)
		}
	}

	report := clean.RenderPreviewReport(clean.NewPreviewReadModel(result))
	for _, want := range []string{
		"Mozilla Firefox browser cache",
		"category: browser_cache",
		"profiles: 2",
		"Observed opportunity bytes: 9 bytes",
		"Potential space: 0 bytes",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("human report missing %q:\n%s", want, report)
		}
	}

	detailed, err := os.ReadFile(result.DetailedListPath)
	if err != nil {
		t.Fatal(err)
	}
	detailedText := string(detailed)
	cachePath := filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2")
	for _, want := range []string{
		"browser: Mozilla Firefox",
		"profiles: 2",
		"cache: cache2",
		cachePath,
	} {
		if !strings.Contains(detailedText, want) {
			t.Fatalf("detailed review data missing %q:\n%s", want, detailedText)
		}
	}
	for _, forbidden := range []string{"cookies.sqlite", "places.sqlite", "logins.json", "sessionstore", "startupCache"} {
		if strings.Contains(detailedText, forbidden) {
			t.Fatalf("detailed review data contains excluded Firefox path %q:\n%s", forbidden, detailedText)
		}
	}
	if len(recorder.sessions) != 1 ||
		recorder.sessions[0].Aggregate.OpportunityCount != 1 ||
		recorder.sessions[0].Aggregate.OpportunityObservedBytes != 9 ||
		len(recorder.items) != 0 {
		t.Fatalf("history = %#v / %#v, want privacy-preserving Firefox aggregate only", recorder.sessions, recorder.items)
	}
}

func TestDryRunReportsFirefoxOnlyWithoutChromeEdge(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/only.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "only", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "x"), "fx")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 1 || result.Opportunities[0].BrowserCache == nil ||
		result.Opportunities[0].BrowserCache.Browser != clean.ApplicationMozillaFirefox {
		t.Fatalf("opportunities = %#v, want Firefox-only", result.Opportunities)
	}
	if result.Totals.OpportunityObservedBytes != 2 {
		t.Fatalf("bytes = %d, want 2", result.Totals.OpportunityObservedBytes)
	}
}

func TestDryRunChromeIdleFirefoxRunningKeepsChromeIndependent(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	chromeRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	writeBrowserLocalState(t, chromeRoot, map[string]string{"Default": "Chrome"})
	writeFile(t, filepath.Join(chromeRoot, "Default", "Cache", "chrome.bin"), "chrome")
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "firefox")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationMozillaFirefox, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 || result.Opportunities[0].BrowserCache == nil ||
		result.Opportunities[0].BrowserCache.Browser != clean.ApplicationGoogleChrome {
		t.Fatalf("opportunities = %#v, want Chrome only while Firefox running", result.Opportunities)
	}
	model := clean.NewPreviewReadModel(result)
	foundFirefoxSkip := false
	for _, skip := range model.RunningApplicationSkips {
		if strings.Contains(skip.Name, "Mozilla Firefox") {
			foundFirefoxSkip = true
		}
	}
	if !foundFirefoxSkip {
		t.Fatalf("running skips = %#v, want Firefox running skip", model.RunningApplicationSkips)
	}
}

func TestDryRunSkipsFirefoxWhenRunning(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "firefox")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationMozillaFirefox, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("Firefox running still measured: %#v", result)
	}
	model := clean.NewPreviewReadModel(result)
	if len(model.RunningApplicationSkips) != 1 || !strings.Contains(model.RunningApplicationSkips[0].Name, "Mozilla Firefox") {
		t.Fatalf("running skips = %#v, want Firefox", model.RunningApplicationSkips)
	}
}

func TestDryRunFirefoxUnknownIsFailClosedDiagnostic(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "firefox")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationMozillaFirefox, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("unknown Firefox still measured: %#v", result)
	}
	if len(result.Errors) != 1 || result.Errors[0].Code != "running_application_detection_unknown" || !result.Errors[0].Recoverable {
		t.Fatalf("errors = %#v, want recoverable unknown diagnostic", result.Errors)
	}
}

func TestDryRunDiscardsFirefoxWhenPostInspectionStateIsUnsafe(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "firefox")

	call := 0
	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			state := clean.RunningApplicationStateIdle
			if call > 0 {
				state = clean.RunningApplicationStateRunning
			}
			call++
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationMozillaFirefox, State: state},
			}
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("Firefox data survived unsafe post-check: %#v", result)
	}
}

func TestDryRunMissingFirefoxRootsIsSilent(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   t.TempDir(),
			RoamingAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || len(result.Errors) != 0 || result.Totals.OpportunityCount != 0 {
		t.Fatalf("missing Firefox roots produced noise: %#v", result)
	}
}

func TestDryRunInvalidFirefoxCatalogIsDiagnosticWithoutGuess(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	catalogRoot := filepath.Join(roamingAppData, "Mozilla", "Firefox")
	if err := os.MkdirAll(catalogRoot, 0700); err != nil {
		t.Fatal(err)
	}
	// Root exists but catalog file is missing → unknown, no AppData name guessing.
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", "Profiles", "guessed.default", "cache2", "x"), "guess")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || len(result.Errors) != 1 || result.Errors[0].Code != "browser_profile_catalog_unknown" {
		t.Fatalf("result = %#v, want catalog diagnostic without guess", result)
	}
	if strings.Contains(clean.RenderPreviewReport(clean.NewPreviewReadModel(result)), "guessed.default") {
		t.Fatalf("catalog diagnostic leaked guessed profile path")
	}
}

func TestDryRunInvalidFirefoxProfilesINIContentIsDiagnostic(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	catalogRoot := filepath.Join(roamingAppData, "Mozilla", "Firefox")
	if err := os.MkdirAll(catalogRoot, 0700); err != nil {
		t.Fatal(err)
	}
	// Empty profiles.ini is invalid catalog evidence.
	if err := os.WriteFile(filepath.Join(catalogRoot, "profiles.ini"), []byte("   \n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", "Profiles", "x.default", "cache2", "x"), "x")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || len(result.Errors) != 1 || result.Errors[0].Code != "browser_profile_catalog_unknown" {
		t.Fatalf("result = %#v, want invalid catalog diagnostic", result)
	}
}

func TestDryRunSuppressesProtectedFirefoxBrowserCacheWithoutLeakingPaths(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	protectedCache := filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2")
	writeFile(t, filepath.Join(protectedCache, "f"), "firefox")
	localRoot := filepath.Join(localAppData, "Mozilla", "Firefox")
	recorder := &recordingHistoryRecorder{}

	result := clean.DryRun(context.Background(), clean.Options{
		Validator:       pathsafe.NewValidator([]string{protectedCache}),
		HistoryRecorder: recorder,
		DetailedListDir: filepath.Join(t.TempDir(), "Foal", "history"),
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
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
		for _, forbidden := range []string{protectedCache, localRoot, "browser_cache", "Mozilla Firefox browser cache"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked protected Firefox data %q:\n%s", name, forbidden, text)
			}
		}
	}
	if len(result.Opportunities) != 0 ||
		len(result.IncompleteOpportunityInspections) != 0 ||
		result.Totals.OpportunityCount != 0 ||
		result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("protected Firefox cache affected result: %#v", result)
	}
}

func TestOptInFirefoxBrowserCacheCandidatesAreCache2Only(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	cache2 := filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2")
	writeFile(t, filepath.Join(cache2, "f"), "firefox")
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "startupCache", "s"), "nope")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryBrowserCache},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want one cache2 candidate", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != cache2 {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, cache2)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}
	if result.OptInCandidates[0].Category != clean.OpportunityCategoryBrowserCache {
		t.Fatalf("category = %q", result.OptInCandidates[0].Category)
	}
}

func TestExecuteOptInFirefoxBrowserCacheCleansWhenIdleAndAuthorized(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	cache2 := filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2")
	writeFile(t, filepath.Join(cache2, "data.bin"), "cache data")
	// Non-allowlisted sibling must remain after permanent cache delete.
	sibling := filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "startupCache", "keep.bin")
	writeFile(t, sibling, "keep")

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})

	if len(recycle.paths) != 0 {
		t.Fatalf("Firefox permanent must not use Recycle Bin: %v", recycle.paths)
	}
	found := false
	for _, p := range permanent.paths {
		if p == cache2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("permanent paths = %v, want cache2", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 1 || result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("totals = %#v, want permanent Firefox delete", result.Totals)
	}
	if _, err := os.Lstat(sibling); err != nil {
		t.Fatalf("non-allowlisted sibling removed: %v", err)
	}
	for _, d := range result.Deleted {
		if d.IsOptIn && d.Action != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("deleted action = %#v", d)
		}
	}
}

func TestExecuteOptInFirefoxWithoutPermanentAuthorizationSkips(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	localAppData := filepath.Join(root, "Local")
	roamingAppData := filepath.Join(root, "Roaming")
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "fx")

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})
	if len(recycle.paths) != 1 {
		t.Fatalf("recycle work must continue: %v", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent without auth: %v", permanent.paths)
	}
	foundAuthSkip := false
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "permanent_deletion_not_authorized" &&
			skipped.Rule == clean.OpportunityCategoryBrowserCache {
			foundAuthSkip = true
		}
	}
	if !foundAuthSkip {
		t.Fatalf("skipped = %#v, want permanent_deletion_not_authorized", result.Skipped)
	}
}

func TestExecuteOptInFirefoxSkipsWhenRunning(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	profileRel := "Profiles/ff.default-release"
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "ff", Path: profileRel, IsRelative: true},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", filepath.FromSlash(profileRel), "cache2", "f"), "fx")

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		PermanentRemover:       permanent,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationMozillaFirefox, State: clean.RunningApplicationStateRunning},
			}
		},
		Rules: []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("deleted while Firefox running: %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestFirefoxAbsoluteProfilePathsAreIgnoredNotGuessed(t *testing.T) {
	localAppData := t.TempDir()
	roamingAppData := t.TempDir()
	// Absolute custom profile path is out of scope even when listed in the catalog.
	writeFirefoxProfilesINI(t, roamingAppData, []firefoxProfileSpec{
		{Name: "portable", Path: `D:\Portable\FirefoxProfile`, IsRelative: false},
	})
	writeFile(t, filepath.Join(localAppData, "Mozilla", "Firefox", "Profiles", "ignored.default", "cache2", "x"), "x")

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir:   localAppData,
			RoamingAppDataDir: roamingAppData,
		},
		DetectRunningApplications: idleFirefoxDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	if len(result.Opportunities) != 0 || len(result.Errors) != 0 {
		t.Fatalf("absolute profile produced opportunity or noise: %#v", result)
	}
}

type firefoxProfileSpec struct {
	Name       string
	Path       string
	IsRelative bool
}

func writeFirefoxProfilesINI(t *testing.T, roamingAppData string, profiles []firefoxProfileSpec) {
	t.Helper()
	catalogRoot := filepath.Join(roamingAppData, "Mozilla", "Firefox")
	if err := os.MkdirAll(catalogRoot, 0700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("[General]\nStartWithLastProfile=1\nVersion=2\n\n")
	for i, profile := range profiles {
		b.WriteString("[Profile")
		b.WriteString(itoa(i))
		b.WriteString("]\nName=")
		b.WriteString(profile.Name)
		b.WriteString("\n")
		if profile.IsRelative {
			b.WriteString("IsRelative=1\n")
		} else {
			b.WriteString("IsRelative=0\n")
		}
		b.WriteString("Path=")
		b.WriteString(profile.Path)
		b.WriteString("\n")
		if i == 0 {
			b.WriteString("Default=1\n")
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(catalogRoot, "profiles.ini"), []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func idleFirefoxDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationMozillaFirefox, State: clean.RunningApplicationStateIdle},
		}
	}
}
