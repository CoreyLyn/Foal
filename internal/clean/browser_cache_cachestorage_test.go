package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// #264: Chromium Service Worker\CacheStorage is an allowlisted browser_cache
// root; ScriptCache, Database, and the parent Service Worker tree are not.

func TestDryRunMeasuresChromiumCacheStorageAndExcludesSWSiblings(t *testing.T) {
	localAppData := t.TempDir()
	userDataRoot := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	writeBrowserLocalState(t, userDataRoot, map[string]string{
		"Default":   "Person 1",
		"Profile 1": "Work",
	})

	// Allowlisted roots with content.
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cache", "cache.bin"), "cache")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "CacheStorage", "origin", "guid", "data"), "swcs")
	writeFile(t, filepath.Join(userDataRoot, "Profile 1", "Service Worker", "CacheStorage", "o", "g", "p1"), "p1cs")

	// Excluded Service Worker siblings and parent-level noise (must not count).
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "ScriptCache", "script.bin"), "script-must-not-count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "Database", "db.bin"), "db-must-not-count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Service Worker", "worker.bin"), "parent-must-not-count")
	writeFile(t, filepath.Join(userDataRoot, "Default", "History"), "history")
	writeFile(t, filepath.Join(userDataRoot, "Default", "Cookies"), "cookies")
	writeFile(t, filepath.Join(userDataRoot, "Default", "IndexedDB", "idb.bin"), "idb")

	// cache (5) + swcs (4) + p1cs (4) = 13
	const wantBytes int64 = 13

	result := clean.DryRun(context.Background(), clean.Options{
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData},
		DetectRunningApplications:    idleChromeDetector(),
		DiscoverOpportunities:        noUserTempOpportunities,
		DiscoverReviewSuggestions:    noReviewSuggestions,
		Rules:                        []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one Chrome browser_cache opportunity", result.Opportunities)
	}
	opp := result.Opportunities[0]
	if opp.Category != clean.OpportunityCategoryBrowserCache || opp.Bytes != wantBytes {
		t.Fatalf("opportunity = %#v, want browser_cache bytes %d", opp, wantBytes)
	}
	if result.Totals.OpportunityObservedBytes != wantBytes || result.Totals.CandidateBytes != 0 {
		t.Fatalf("totals = %#v, want observed-only %d", result.Totals, wantBytes)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	if !strings.Contains(jsonText, "CacheStorage") {
		t.Fatalf("JSON missing CacheStorage: %s", jsonText)
	}
	for _, forbidden := range []string{
		"ScriptCache",
		`"Database"`,
		filepath.Join("Service Worker", "worker.bin"),
		"History",
		"Cookies",
		"IndexedDB",
		"move_to_recycle_bin",
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains excluded path/kind %q: %s", forbidden, jsonText)
		}
	}

	// Confirm measured CacheStorage paths exist under both profiles.
	cacheStoragePaths := []string{
		filepath.Join(userDataRoot, "Default", "Service Worker", "CacheStorage"),
		filepath.Join(userDataRoot, "Profile 1", "Service Worker", "CacheStorage"),
	}
	for _, profile := range opp.BrowserCache.Profiles {
		for _, cache := range profile.Caches {
			if !strings.Contains(cache.Kind, "CacheStorage") {
				continue
			}
			if cache.Bytes <= 0 {
				t.Fatalf("profile %s CacheStorage bytes = %d, want measured", profile.ID, cache.Bytes)
			}
		}
	}
	for _, wantPath := range cacheStoragePaths {
		found := false
		for _, profile := range opp.BrowserCache.Profiles {
			for _, cache := range profile.Caches {
				if cache.Path == wantPath && cache.Bytes > 0 {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("missing measured CacheStorage path %q in %#v", wantPath, opp.BrowserCache)
		}
	}
}

func TestOptInBrowserCacheYieldsCacheStorageCandidate(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	cacheStorage := filepath.Join(chromeUserData, "Default", "Service Worker", "CacheStorage")
	if err := os.MkdirAll(cacheStorage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheStorage, "blob"), []byte("cache-storage"), 0600); err != nil {
		t.Fatal(err)
	}
	// Excluded siblings must not become candidates.
	if err := os.MkdirAll(filepath.Join(chromeUserData, "Default", "Service Worker", "ScriptCache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Default", "Service Worker", "ScriptCache", "s"), []byte("script"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryBrowserCache},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleChromeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v, want only CacheStorage", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != cacheStorage {
		t.Fatalf("candidate path = %q, want %q", result.OptInCandidates[0].Path, cacheStorage)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}

	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" &&
			strings.Contains(notice.Message, "Cache Storage") &&
			strings.Contains(notice.Message, "Progressive Web Apps") {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want browser Cache Storage / PWA impact", model.Notices)
	}
}

func TestExecuteOptInBrowserCacheDeletesCacheStorageWhenIdleAndAuthorized(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	cacheStorage := filepath.Join(chromeUserData, "Default", "Service Worker", "CacheStorage")
	scriptCache := filepath.Join(chromeUserData, "Default", "Service Worker", "ScriptCache")
	if err := os.MkdirAll(cacheStorage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(scriptCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheStorage, "blob"), []byte("cache-storage"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptCache, "script"), []byte("keep-script"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:                  []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateIdle},
			}
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
	})

	if len(recycle.paths) != 0 {
		t.Fatalf("browser permanent must not use Recycle Bin: %v", recycle.paths)
	}
	found := false
	for _, p := range permanent.paths {
		if p == cacheStorage {
			found = true
		}
		if p == scriptCache || strings.HasSuffix(p, "ScriptCache") {
			t.Fatalf("must not delete ScriptCache: %v", permanent.paths)
		}
	}
	if !found {
		t.Fatalf("expected permanent remover to receive CacheStorage path, got %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
	}
	// ScriptCache must remain on disk (recording remover does not delete; assert path never selected).
	if _, err := os.Lstat(scriptCache); err != nil {
		t.Fatalf("ScriptCache should remain selectable exclusion, lstat: %v", err)
	}
}
