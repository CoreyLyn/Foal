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

// #220 activates nvidia_dx_cache, browser_cache, vscode_cache, and cursor_cache
// as catalog-owned permanent deletion alongside the existing D3D tracer.

func issue220PermanentCategories() []string {
	return []string{
		clean.OpportunityCategoryNVIDIADXCache,
		clean.OpportunityCategoryBrowserCache,
		clean.OpportunityCategoryVSCodeCache,
		clean.OpportunityCategoryCursorCache,
	}
}

func TestIssue220CategoriesDeclareDeletePermanently(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	for _, id := range issue220PermanentCategories() {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%s missing from catalog", id)
		}
		if summary.PlannedAction != clean.DeletionActionDeletePermanently {
			t.Fatalf("%s planned_action = %q", id, summary.PlannedAction)
		}
		if !clean.InitiallySelectedCategory(summary) {
			t.Fatalf("%s must start selected when measurable", id)
		}
	}
	// D3D tracer remains permanent and unchanged.
	d3d, ok := catalog.Summary(clean.OpportunityCategoryD3DShaderCache)
	if !ok || d3d.PlannedAction != clean.DeletionActionDeletePermanently {
		t.Fatalf("d3d tracer broken: %#v", d3d)
	}
}

func TestNVIDIADXCacheDryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	root := t.TempDir()
	nvidiaRoot := filepath.Join(root, "DXCache")
	if err := os.Mkdir(nvidiaRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvidiaRoot, "shader.bin"), []byte("nv-dx"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryNVIDIADXCache},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryNVIDIADXCache,
					Path:     nvidiaRoot,
					Bytes:    5,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}
	if _, err := os.Lstat(nvidiaRoot); err != nil {
		t.Fatalf("dry-run must leave path: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"planned_action":"delete_permanently"`) {
		t.Fatalf("JSON missing permanent planned_action: %s", encoded)
	}
}

func TestNVIDIADXCacheExecuteWithoutAllowPermanentSkipsAndContinuesRecycleBin(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	nvidiaRoot := filepath.Join(root, "DXCache")
	if err := os.Mkdir(nvidiaRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvidiaRoot, "shader.bin"), []byte("nv"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryNVIDIADXCache},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		HistoryRecorder:        recorder,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryNVIDIADXCache,
					Path:     nvidiaRoot,
					Bytes:    2,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})

	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called without auth: %v", permanent.paths)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if result.Skipped[0].Rule != clean.OpportunityCategoryNVIDIADXCache {
		t.Fatalf("skip rule = %q", result.Skipped[0].Rule)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(nvidiaRoot); err != nil {
		t.Fatalf("unauthorized permanent path must remain: %v", err)
	}
}

func TestNVIDIADXCacheExecuteWithAllowPermanentDispatchesPermanentRemover(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	nvidiaRoot := filepath.Join(root, "DXCache")
	if err := os.Mkdir(nvidiaRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvidiaRoot, "shader.bin"), []byte("nvdx"), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryNVIDIADXCache},
		RecycleBinAdapter:      collab,
		PermanentRemover:       collab,
		HistoryRecorder:        recorder,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryNVIDIADXCache,
					Path:     nvidiaRoot,
					Bytes:    4,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[1].kind != "permanent" {
		t.Fatalf("order = %#v, want recycle then permanent", collab.calls)
	}
	if collab.calls[1].path != nvidiaRoot {
		t.Fatalf("permanent path = %q", collab.calls[1].path)
	}
	byRule := map[string]clean.DeletedItem{}
	for _, item := range result.Deleted {
		byRule[item.Rule] = item
	}
	if byRule[clean.OpportunityCategoryNVIDIADXCache].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("nvidia deleted = %#v", byRule[clean.OpportunityCategoryNVIDIADXCache])
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0")
	}
	if _, err := os.Lstat(nvidiaRoot); !os.IsNotExist(err) {
		t.Fatalf("nvidia root still exists: %v", err)
	}
}

func writeChromeDefaultCache(t *testing.T, localAppData string, contents string) string {
	t.Helper()
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	defaultCache := filepath.Join(chromeUserData, "Default", "Cache")
	if err := os.MkdirAll(defaultCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultCache, "data.bin"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return defaultCache
}

func idleBrowsersDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle},
		}
	}
}

func TestBrowserCacheDryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	localAppData := t.TempDir()
	defaultCache := writeChromeDefaultCache(t, localAppData, "browser")

	result := clean.DryRun(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleBrowsersDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != defaultCache {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, defaultCache)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}
}

func TestBrowserCacheUnauthorizedSkipsPermanentOnly(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	localAppData := filepath.Join(root, "Local")
	_ = writeChromeDefaultCache(t, localAppData, "b")

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryBrowserCache},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
		DetectRunningApplications: idleBrowsersDetector(),
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
			if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("planned action = %q", skipped.PlannedAction)
			}
		}
	}
	if !foundAuthSkip {
		t.Fatalf("skipped = %#v, want permanent_deletion_not_authorized", result.Skipped)
	}
}

func TestVSCodeCacheDryRunReportsPermanentAndVSIXImpact(t *testing.T) {
	roaming := t.TempDir()
	codeRoot := writeVSCodeRoot(t, roaming, map[string]string{
		"Cache":                "cache",
		"CachedExtensionVSIXs": "vsix-pkg",
	})
	result := clean.DryRun(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryVSCodeCache},
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: t.TempDir(),
		},
		DetectRunningApplications: idleVSCodeDetector(),
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 2 {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	for _, c := range result.OptInCandidates {
		if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned_action = %q for %q", c.PlannedAction, c.Path)
		}
	}
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "CachedExtensionVSIXs") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notices = %#v, want VSIX impact", model.Notices)
	}
	// Eager projection surfaces the same impact vocabulary for TUI confirmation.
	res := clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryVSCodeCache,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:     filepath.Join(codeRoot, "CachedExtensionVSIXs"),
			Bytes:    8,
			Category: clean.OpportunityCategoryVSCodeCache,
		}},
	}
	obs := clean.ProjectCategoryPreview(res)
	if !strings.Contains(obs.SafetyNote, "CachedExtensionVSIXs") {
		t.Fatalf("safety note = %q, want VSIX impact", obs.SafetyNote)
	}
}

func TestCursorCacheUnauthorizedSkipsPermanent(t *testing.T) {
	roaming := t.TempDir()
	writeCursorRoot(t, roaming, map[string]string{"Cache": "cursor"})
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryCursorCache},
		PermanentRemover:       permanent,
		ApplicationCacheDiscoveryOptions: clean.ApplicationCacheDiscoveryOptions{
			RoamingAppDataDir: roaming,
		},
		DetectRunningApplications: idleCursorDetector(),
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent without auth: %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("deleted count = %d", result.Totals.OptInDeletedCount)
	}
	found := false
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "permanent_deletion_not_authorized" &&
			skipped.Rule == clean.OpportunityCategoryCursorCache {
			found = true
			if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("planned action = %q", skipped.PlannedAction)
			}
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want permanent_deletion_not_authorized", result.Skipped)
	}
}

func TestIssue220RecycleBinCategoriesUnchanged(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	for _, id := range []string{
		clean.OpportunityCategoryExplorerThumbnailCache,
		clean.OpportunityCategoryINetCache,
		clean.OpportunityCategoryUserTemp,
		clean.OpportunityCategoryCrashDumps,
		clean.OpportunityCategoryWindowsErrorReporting,
		clean.DefaultCategoryFoalOwnedTempSandboxes,
		clean.DevCacheCategoryNPM,
		clean.DevCacheCategoryGo,
	} {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if summary.PlannedAction != clean.DeletionActionMoveToRecycleBin {
			t.Fatalf("%s planned_action = %q, want recycle bin", id, summary.PlannedAction)
		}
	}
}
