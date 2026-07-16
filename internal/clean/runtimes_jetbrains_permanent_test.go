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

// #222 activates permanent deletion for playwright-browsers, puppeteer-browsers,
// electron-cache, and jetbrains-ide-caches. These tests exercise the live catalog
// action end-to-end (no CategoryPlannedActions injection).

func TestRuntimesJetBrainsDryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	cases := []struct {
		name     string
		category string
		setup    func(t *testing.T) (path string, opts clean.Options)
	}{
		{
			name:     "playwright-browsers",
			category: clean.DevCacheCategoryPlaywright,
			setup: func(t *testing.T) (string, clean.Options) {
				root := t.TempDir()
				browsersRoot := filepath.Join(root, "ms-playwright")
				if err := os.Mkdir(browsersRoot, 0700); err != nil {
					t.Fatal(err)
				}
				child := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "pw")
				return child, clean.Options{
					AllowPermanentDeletion:    false,
					OptIn:                     []string{clean.DevCacheCategoryPlaywright},
					DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				}
			},
		},
		{
			name:     "puppeteer-browsers",
			category: clean.DevCacheCategoryPuppeteerBrowsers,
			setup: func(t *testing.T) (string, clean.Options) {
				root := t.TempDir()
				cacheRoot := filepath.Join(root, "puppeteer")
				child := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "pp")
				return child, clean.Options{
					AllowPermanentDeletion:    false,
					OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
					DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				}
			},
		},
		{
			name:     "electron-cache",
			category: clean.DevCacheCategoryElectron,
			setup: func(t *testing.T) (string, clean.Options) {
				root := t.TempDir()
				cachePath := filepath.Join(root, "electron-cache")
				if err := os.Mkdir(cachePath, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cachePath, "bin.zip"), []byte("el"), 0600); err != nil {
					t.Fatal(err)
				}
				return cachePath, clean.Options{
					AllowPermanentDeletion:    false,
					OptIn:                     []string{clean.DevCacheCategoryElectron},
					DevCachePathResolver:      func(string) []string { return []string{cachePath} },
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				}
			},
		},
		{
			name:     "jetbrains-ide-caches",
			category: clean.DevCacheCategoryJetBrainsIDECaches,
			setup: func(t *testing.T) (string, clean.Options) {
				_, parent := jetbrainsLocalAppData(t)
				product := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "jb"})
				child := filepath.Join(product, "caches")
				return child, clean.Options{
					AllowPermanentDeletion:    false,
					OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
					DetectRunningApplications: idleJetBrainsDetector(),
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, opts := tc.setup(t)
			result := clean.DryRun(context.Background(), opts)
			if len(result.OptInCandidates) == 0 {
				t.Fatalf("opt-in candidates empty for %s", tc.category)
			}
			found := false
			for _, c := range result.OptInCandidates {
				if c.Category != tc.category {
					t.Fatalf("category = %q", c.Category)
				}
				if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
					t.Fatalf("planned_action = %q, want delete_permanently", c.PlannedAction)
				}
				if c.Path == path {
					found = true
				}
			}
			if !found {
				t.Fatalf("candidates = %#v, want path %q", result.OptInCandidates, path)
			}
			if len(result.Deleted) != 0 {
				t.Fatalf("dry-run must not delete: %#v", result.Deleted)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("dry-run must leave candidate intact: %v", err)
			}

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			body := string(encoded)
			if !strings.Contains(body, `"planned_action":"delete_permanently"`) {
				t.Fatalf("JSON missing permanent planned_action: %s", body)
			}
			if strings.Contains(strings.ToLower(body), "secure erase") || strings.Contains(strings.ToLower(body), "shred") {
				t.Fatalf("must not claim secure erase: %s", body)
			}
		})
	}
}

func TestRuntimesJetBrainsExecuteWithoutAllowPermanentSkips(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	child := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "shader!")

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.DevCacheCategoryPlaywright},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		HistoryRecorder:        recorder,
		DevCachePathResolver:   func(string) []string { return []string{browsersRoot} },
		DiscoverOpportunities:  noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})

	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v, want default only", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called without auth: %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.DeletionActionMoveToRecycleBin) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skip code = %q", skipped.Reason.Code)
	}
	if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned action changed: %q", skipped.PlannedAction)
	}
	if skipped.Rule != clean.DevCacheCategoryPlaywright {
		t.Fatalf("skip rule = %q", skipped.Rule)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(child); err != nil {
		t.Fatalf("unauthorized permanent path must remain: %v", err)
	}
	foundAuthSkip := false
	for _, item := range recorder.items {
		if item.Result == "skipped" && item.SkippedReason != nil && item.SkippedReason.Code == "permanent_deletion_not_authorized" {
			foundAuthSkip = true
			if item.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history planned action = %q", item.PlannedAction)
			}
			if item.Rule != clean.DevCacheCategoryPlaywright {
				t.Fatalf("history rule = %q", item.Rule)
			}
		}
	}
	if !foundAuthSkip {
		t.Fatalf("history missing auth skip: %#v", recorder.items)
	}
}

func TestRuntimesJetBrainsExecuteWithAllowPermanentDispatchesPermanentRemover(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	electronRoot := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(electronRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(electronRoot, "bin.zip"), []byte("electron"), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryElectron},
		RecycleBinAdapter:      collab,
		PermanentRemover:       collab,
		HistoryRecorder:        recorder,
		DevCachePathResolver:   func(string) []string { return []string{electronRoot} },
		DiscoverOpportunities:  noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want recycle then permanent", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[0].path != recyclePath {
		t.Fatalf("first call = %#v", collab.calls[0])
	}
	if collab.calls[1].kind != "permanent" || collab.calls[1].path != electronRoot {
		t.Fatalf("second call = %#v, want permanent electron root", collab.calls[1])
	}

	byRule := map[string]clean.DeletedItem{}
	for _, item := range result.Deleted {
		byRule[item.Rule] = item
	}
	if byRule[clean.DefaultCategoryFoalOwnedTempSandboxes].Action != string(clean.DeletionActionMoveToRecycleBin) {
		t.Fatalf("recycle deleted = %#v", byRule[clean.DefaultCategoryFoalOwnedTempSandboxes])
	}
	electronDeleted, ok := byRule[clean.DevCacheCategoryElectron]
	if !ok || electronDeleted.Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("electron deleted = %#v", byRule[clean.DevCacheCategoryElectron])
	}
	if result.Totals.RecycleBinMovedBytes != 2 {
		t.Fatalf("recycle_bin_moved_bytes = %d", result.Totals.RecycleBinMovedBytes)
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0, want electron content")
	}
	if result.Totals.AffectedBytes != result.Totals.RecycleBinMovedBytes+result.Totals.PermanentlyDeletedBytes {
		t.Fatalf("affected_bytes = %d, want sum of action totals %#v", result.Totals.AffectedBytes, result.Totals)
	}
	if _, err := os.Lstat(electronRoot); !os.IsNotExist(err) {
		t.Fatalf("electron root still exists after permanent delete: %v", err)
	}

	foundPermanentHistory := false
	for _, item := range recorder.items {
		if item.Rule == clean.DevCacheCategoryElectron && item.Result == "deleted" {
			foundPermanentHistory = true
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
	}
	if !foundPermanentHistory {
		t.Fatalf("history missing electron permanent success: %#v", recorder.items)
	}
	if recorder.sessions[0].Aggregate.PermanentlyDeletedBytes == 0 {
		t.Fatalf("history aggregate permanent bytes = %#v", recorder.sessions[0].Aggregate)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"action":"delete_permanently"`) {
		t.Fatalf("JSON missing actual permanent action: %s", body)
	}
	if !strings.Contains(body, `"permanently_deleted_bytes"`) {
		t.Fatalf("JSON missing permanently_deleted_bytes: %s", body)
	}
}

func TestRuntimesJetBrainsPermanentFailureNeverFallsBackToRecycleBin(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "bin.zip"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:   func(string) []string { return []string{cachePath} },
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: false,
		}},
		RecycleBinAdapter: recycle,
		PermanentRemover: permanentRemoverFunc(func(context.Context, string) error {
			return os.ErrPermission
		}),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(recycle.paths) != 0 {
		t.Fatalf("permanent failure must never fall back to Recycle Bin: %v", recycle.paths)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("failed permanent must contribute zero action bytes: %#v", result.Totals)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("failed = %#v", result.Failed)
	}
	if result.Failed[0].Rule != clean.DevCacheCategoryElectron {
		t.Fatalf("failed rule = %q", result.Failed[0].Rule)
	}
}

func TestRuntimesJetBrainsTUIInitiallySelectsPermanentCategories(t *testing.T) {
	for _, id := range []string{
		clean.DevCacheCategoryPlaywright,
		clean.DevCacheCategoryPuppeteerBrowsers,
		clean.DevCacheCategoryElectron,
		clean.DevCacheCategoryJetBrainsIDECaches,
	} {
		summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(id)
		if !ok {
			t.Fatalf("%s missing from catalog", id)
		}
		if summary.PlannedAction != clean.DeletionActionDeletePermanently {
			t.Fatalf("%s planned_action = %q", id, summary.PlannedAction)
		}
		if !clean.InitiallySelectedCategory(summary) {
			t.Fatalf("%s must start selected in TUI when permanently eligible", id)
		}
	}
}
