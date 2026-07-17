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

func writeVisualStudioParent(t *testing.T) (localAppData, parent string) {
	t.Helper()
	localAppData = t.TempDir()
	parent = filepath.Join(localAppData, "Microsoft", "VisualStudio")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", localAppData)
	// Isolate default foal-owned temp sandbox discovery from the host TEMP.
	tempRoot := t.TempDir()
	t.Setenv("TEMP", tempRoot)
	t.Setenv("TMP", tempRoot)
	return localAppData, parent
}

func writeVisualStudioCacheChild(t *testing.T, parent string, relParts ...string) string {
	t.Helper()
	parts := append([]string{parent}, relParts...)
	path := filepath.Join(parts...)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	payload := strings.Join(relParts, "-")
	if err := os.WriteFile(filepath.Join(path, "data.bin"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeVisualStudioDecoyDir(t *testing.T, parent string, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "decoy.bin"), []byte("nope"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func idleVisualStudioDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationVisualStudio,
			State:       clean.RunningApplicationStateIdle,
		}}
	}
}

func TestVisualStudioCaches_CatalogAndGroupTokens(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.DevCacheCategoryVisualStudioCaches)
	if !ok {
		t.Fatal("visual-studio-caches missing from catalog")
	}
	if summary.Label != "Visual Studio caches" {
		t.Fatalf("label = %q", summary.Label)
	}
	if summary.ReportCategory != clean.ReportCategoryDeveloperTools {
		t.Fatalf("report category = %q", summary.ReportCategory)
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q", summary.Eligibility)
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicyDistinctiveProcessIdle {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}
	if summary.PlannedAction != clean.DeletionActionDeletePermanently {
		t.Fatalf("planned action = %q, want delete_permanently", summary.PlannedAction)
	}
	if !clean.InitiallySelectedCategory(summary) {
		t.Fatal("permanent visual-studio-caches must start selected when measurable")
	}

	for _, token := range []string{
		clean.DevCacheCategoryVisualStudioCaches,
		"dev-caches",
		"all",
	} {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", token, invalid)
		}
		if !enabled[clean.DevCacheCategoryVisualStudioCaches] {
			t.Fatalf("%s did not enable visual-studio-caches", token)
		}
	}

	enabled, _, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryVisualStudioCaches})
	if len(enabled) != 1 || !enabled[clean.DevCacheCategoryVisualStudioCaches] {
		t.Fatalf("solo opt-in = %#v", enabled)
	}
	if enabled[clean.OpportunityCategoryVSCodeCache] || enabled[clean.OpportunityCategoryCursorCache] {
		t.Fatal("visual-studio-caches must not enable vscode_cache or cursor_cache")
	}
	if enabled[clean.DevCacheCategoryJetBrainsIDECaches] {
		t.Fatal("visual-studio-caches must not enable jetbrains-ide-caches")
	}
}

func TestVisualStudioCaches_AllowlistOnlyAndSilentAbsence(t *testing.T) {
	_, parent := writeVisualStudioParent(t)

	roslyn := writeVisualStudioCacheChild(t, parent, "Roslyn")
	cmc170 := writeVisualStudioCacheChild(t, parent, "17.0_abc123ef", "ComponentModelCache")
	cmc180 := writeVisualStudioCacheChild(t, parent, "18.0_a4d9e95d", "ComponentModelCache")
	// Bare major.minor instance without hive id.
	cmc160 := writeVisualStudioCacheChild(t, parent, "16.0", "ComponentModelCache")

	// Non-allowlisted siblings and decoys must never become candidates.
	for _, decoy := range []string{
		"Settings", "Extensions", "Packages", "MEFCacheBackup", "WebView2Cache",
		"PackageCache", "BackupFiles", "CacheService", "Copilot", "TextMateCache",
		"ItemTemplatesCache_{00000000-0000-0000-0000-000000000000}",
		"ProjectTemplatesCache_{00000000-0000-0000-0000-000000000000}",
		// Pre-14 / malformed instance names.
		"13.0", "13.0_deadbeef", "14", "14.0-backup", "My17.0",
		"17.0_not-hex!", "17.0_", "017.0", "17.01",
	} {
		_ = writeVisualStudioDecoyDir(t, parent, decoy)
	}
	// Instance-level Roslyn is not the documented shared Roslyn root.
	_ = writeVisualStudioCacheChild(t, parent, "18.0_a4d9e95d", "Roslyn")
	// Non-allowlisted children under a valid instance.
	_ = writeVisualStudioCacheChild(t, parent, "18.0_a4d9e95d", "Settings")
	_ = writeVisualStudioCacheChild(t, parent, "18.0_a4d9e95d", "Extensions")
	_ = writeVisualStudioCacheChild(t, parent, "18.0_a4d9e95d", "MEFCacheBackup")
	// Regular file decoy named like an instance.
	if err := os.WriteFile(filepath.Join(parent, "15.0"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}

	wantPaths := []string{roslyn, cmc160, cmc170, cmc180}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != len(wantPaths) {
		t.Fatalf("candidates = %#v, want %d paths", result.OptInCandidates, len(wantPaths))
	}
	got := make(map[string]clean.OptInCandidate, len(result.OptInCandidates))
	for i, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryVisualStudioCaches {
			t.Fatalf("category = %q", c.Category)
		}
		if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned action = %q", c.PlannedAction)
		}
		base := filepath.Base(c.Path)
		if base != "Roslyn" && base != "ComponentModelCache" {
			t.Fatalf("unexpected child candidate %q", c.Path)
		}
		// Parent and instance hives must never be candidates.
		if strings.EqualFold(filepath.Base(c.Path), "VisualStudio") {
			t.Fatalf("VisualStudio parent leaked as candidate: %q", c.Path)
		}
		if isVisualStudioInstanceBase(filepath.Base(c.Path)) {
			t.Fatalf("instance hive leaked as candidate: %q", c.Path)
		}
		got[c.Path] = c
		if result.OptInCandidates[i].Path != wantPaths[i] {
			t.Fatalf("candidates[%d] = %q, want %q (deterministic order)", i, result.OptInCandidates[i].Path, wantPaths[i])
		}
	}
	for _, want := range wantPaths {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing candidate %q among %#v", want, result.OptInCandidates)
		}
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidates must stay frozen, got %d", result.Totals.CandidateBytes)
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable")
	}

	// Missing parent ⇒ silent absence.
	emptyLocal := t.TempDir()
	t.Setenv("LOCALAPPDATA", emptyLocal)
	result = clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("missing root candidates = %#v, want silent absence", result.OptInCandidates)
	}

	t.Setenv("LOCALAPPDATA", "   ")
	result = clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("blank LOCALAPPDATA candidates = %#v", result.OptInCandidates)
	}
}

// isVisualStudioInstanceBase is a test-only name shape check (not production policy).
func isVisualStudioInstanceBase(name string) bool {
	if strings.Contains(name, ".") && !strings.EqualFold(name, "ComponentModelCache") && !strings.EqualFold(name, "Roslyn") {
		// Rough: 17.0 or 17.0_hex look like instance dirs.
		return true
	}
	return false
}

func TestVisualStudioCaches_IdleGateFailClosedIndependentOfEditors(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	roslyn := writeVisualStudioCacheChild(t, parent, "Roslyn")
	cmc := writeVisualStudioCacheChild(t, parent, "17.0_deadbeef", "ComponentModelCache")

	t.Run("running devenv skips all", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryVisualStudioCaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{{
					Application: clean.ApplicationVisualStudio,
					State:       clean.RunningApplicationStateRunning,
				}}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 {
			t.Fatalf("running VS leaked candidates: %#v", result.OptInCandidates)
		}
		foundSkip := false
		for _, s := range result.Skipped {
			if s.Path == parent && s.Rule == clean.DevCacheCategoryVisualStudioCaches {
				foundSkip = true
			}
		}
		if !foundSkip {
			t.Fatalf("skipped = %#v, want VisualStudio parent skip", result.Skipped)
		}
	})

	t.Run("unknown state fails closed", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryVisualStudioCaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{{
					Application: clean.ApplicationVisualStudio,
					State:       clean.RunningApplicationStateUnknown,
					Message:     "snapshot failed",
				}}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 {
			t.Fatalf("unknown state leaked candidates: %#v", result.OptInCandidates)
		}
	})

	t.Run("running VS Code or Cursor does not block Visual Studio", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryVisualStudioCaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationVisualStudio, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationVisualStudioCode, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationCursor, State: clean.RunningApplicationStateRunning},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		foundRoslyn, foundCMC := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == roslyn {
				foundRoslyn = true
			}
			if c.Path == cmc {
				foundCMC = true
			}
		}
		if !foundRoslyn || !foundCMC {
			t.Fatalf("candidates = %#v, want Roslyn and ComponentModelCache despite editor running", result.OptInCandidates)
		}
	})

	t.Run("post-measurement unsafe discards measured children", func(t *testing.T) {
		call := 0
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryVisualStudioCaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				call++
				if call == 1 {
					return []clean.RunningApplicationState{{
						Application: clean.ApplicationVisualStudio,
						State:       clean.RunningApplicationStateIdle,
					}}
				}
				return []clean.RunningApplicationState{{
					Application: clean.ApplicationVisualStudio,
					State:       clean.RunningApplicationStateRunning,
				}}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 {
			t.Fatalf("post-gate candidates = %#v, want empty", result.OptInCandidates)
		}
	})
}

func TestVisualStudioCaches_DefaultExecuteDoesNotResolve(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	_ = writeVisualStudioCacheChild(t, parent, "Roslyn")

	resolverCalls := 0
	detectionCalled := false
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: []string{},
		DevCacheRootScopeResolver: func(category string) []clean.DevCacheRootScope {
			if category == clean.DevCacheCategoryVisualStudioCaches {
				resolverCalls++
			}
			return nil
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			detectionCalled = true
			return nil
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			t.Fatal("execute without opt-in must not run review suggestion probes")
			return nil
		},
		RecycleBinAdapter:     adapter,
		DiscoverOpportunities: noOpportunities,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if resolverCalls != 0 {
		t.Fatalf("visual studio root resolver called %d times without opt-in", resolverCalls)
	}
	if detectionCalled {
		t.Fatal("detection must not run without opt-in")
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestVisualStudioCaches_ExecuteFreshResolvePermanentAuthAndHistory(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	previewCMC := writeVisualStudioCacheChild(t, parent, "17.0_aaaa1111", "ComponentModelCache")

	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(dry.OptInCandidates) != 1 || dry.OptInCandidates[0].Path != previewCMC {
		t.Fatalf("dry-run candidates = %#v", dry.OptInCandidates)
	}
	if dry.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("dry-run planned action = %q", dry.OptInCandidates[0].PlannedAction)
	}

	// Change layout for execute: remove preview, add Roslyn + new instance.
	if err := os.RemoveAll(filepath.Dir(previewCMC)); err != nil {
		t.Fatal(err)
	}
	executeRoslyn := writeVisualStudioCacheChild(t, parent, "Roslyn")
	executeCMC := writeVisualStudioCacheChild(t, parent, "18.0_bbbb2222", "ComponentModelCache")

	// Without permanent authorization, permanent candidates are skipped.
	unauthorized := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    false,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if unauthorized.Totals.OptInDeletedCount != 0 {
		t.Fatalf("unauthorized deleted = %d", unauthorized.Totals.OptInDeletedCount)
	}
	foundAuthSkip := false
	for _, s := range unauthorized.Skipped {
		if s.Reason.Code == "permanent_deletion_not_authorized" {
			foundAuthSkip = true
		}
	}
	if !foundAuthSkip {
		t.Fatalf("skipped = %#v, want permanent_deletion_not_authorized", unauthorized.Skipped)
	}

	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		PermanentRemover:          permanent,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 2 {
		t.Fatalf("permanent paths = %v, want Roslyn and new CMC", permanent.paths)
	}
	got := map[string]bool{}
	for _, p := range permanent.paths {
		got[p] = true
		if p == previewCMC {
			t.Fatal("execute trusted dry-run path")
		}
	}
	if !got[executeRoslyn] || !got[executeCMC] {
		t.Fatalf("permanent paths = %v, want %q and %q", permanent.paths, executeRoslyn, executeCMC)
	}
	if execResult.Totals.OptInDeletedCount != 2 {
		t.Fatalf("opt-in deleted = %d", execResult.Totals.OptInDeletedCount)
	}
	if len(execResult.Deleted) != 2 {
		t.Fatalf("deleted = %#v", execResult.Deleted)
	}
	for _, d := range execResult.Deleted {
		if d.Action != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("deleted action = %q", d.Action)
		}
	}
	foundHistory := 0
	for _, item := range recorder.items {
		if item.Path == executeRoslyn || item.Path == executeCMC {
			foundHistory++
			if item.Rule != clean.DevCacheCategoryVisualStudioCaches {
				t.Fatalf("history rule = %q", item.Rule)
			}
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
		if item.Path == previewCMC {
			t.Fatal("history recorded preview path")
		}
	}
	if foundHistory != 2 {
		t.Fatalf("history items = %#v, want both execute paths", recorder.items)
	}
}

func TestVisualStudioCaches_Protection(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	roslyn := writeVisualStudioCacheChild(t, parent, "Roslyn")
	cmcA := writeVisualStudioCacheChild(t, parent, "17.0_aaa", "ComponentModelCache")
	cmcB := writeVisualStudioCacheChild(t, parent, "18.0_bbb", "ComponentModelCache")

	t.Run("protect VisualStudio parent suppresses all", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
			Validator:                 pathsafe.NewValidator([]string{parent}),
			DetectRunningApplications: idleVisualStudioDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("protected parent leaked: %#v", result.OptInCandidates)
		}
	})

	t.Run("protect one ComponentModelCache keeps siblings", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
			Validator:                 pathsafe.NewValidator([]string{cmcA}),
			DetectRunningApplications: idleVisualStudioDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		foundRoslyn, foundB := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == cmcA {
				t.Fatalf("protected CMC leaked: %#v", result.OptInCandidates)
			}
			if c.Path == roslyn {
				foundRoslyn = true
			}
			if c.Path == cmcB {
				foundB = true
			}
		}
		if !foundRoslyn || !foundB {
			t.Fatalf("siblings suppressed: %#v", result.OptInCandidates)
		}
	})
}

func TestVisualStudioCaches_CapacityDoesNotBlockPermanent(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	child := writeVisualStudioCacheChild(t, parent, "Roslyn")

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       "C:",
				NukeOnDelete: false,
				MaxCapacity:  1,
				CurrentUsage: 0,
			}, nil
		},
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != child {
		t.Fatalf("permanent paths = %v, want %q (capacity must not block permanent)", permanent.paths, child)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "recycle_bin_capacity" {
			t.Fatalf("permanent candidate skipped for recycle capacity: %#v", skipped)
		}
	}
}

func TestVisualStudioCaches_Cancellation(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	_ = writeVisualStudioCacheChild(t, parent, "Roslyn")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(ctx, clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("canceled execute permanent paths = %v", permanent.paths)
	}
}

func TestVisualStudioCaches_ImpactNoticeAndFrozenDefaults(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	_ = writeVisualStudioCacheChild(t, parent, "Roslyn")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "Visual Studio") &&
			(strings.Contains(notice.Message, "slower") || strings.Contains(notice.Message, "rebuild")) {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want Visual Studio rebuild impact", model.Notices)
	}
	for _, c := range result.Candidates {
		if c.Rule == clean.DevCacheCategoryVisualStudioCaches || strings.Contains(c.Path, "VisualStudio") {
			t.Fatalf("visual studio path leaked into default candidates: %#v", c)
		}
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable")
	}

	defaultResult := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(defaultResult.OptInCandidates) != 0 {
		t.Fatalf("default dry-run opt-in candidates = %#v", defaultResult.OptInCandidates)
	}
}

func TestVisualStudioCaches_TUICategoryIdentifierOnly(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, cat := range model.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryVisualStudioCaches {
			found = true
			// CLI preview read-model without explicit selection leaves opt-ins unselected.
			if cat.Selected {
				t.Fatal("visual-studio-caches must start unselected in default preview model")
			}
		}
	}
	if !found {
		t.Fatalf("OptInCategories missing visual-studio-caches: %#v", model.OptInCategories)
	}
	selected := clean.NewPreviewReadModelForSelection(result, []string{clean.DevCacheCategoryVisualStudioCaches})
	for _, cat := range selected.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryVisualStudioCaches && !cat.Selected {
			t.Fatal("expected selected after identifier selection")
		}
	}
	// Permanent eligibility still drives Clean TUI initial selection policy.
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.DevCacheCategoryVisualStudioCaches)
	if !ok || !clean.InitiallySelectedCategory(summary) {
		t.Fatal("visual-studio-caches must be InitiallySelectedCategory for TUI")
	}
}

func TestVisualStudioCaches_EagerPreviewSafetyNote(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	_ = writeVisualStudioCacheChild(t, parent, "Roslyn")

	var terminal *clean.CategoryPreviewObservation
	err := clean.RunEagerPreview(context.Background(), clean.Options{
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	}, func(obs clean.CategoryPreviewObservation) {
		if obs.Identifier == clean.DevCacheCategoryVisualStudioCaches && obs.State != clean.CategoryPreviewScanning {
			cp := obs
			terminal = &cp
		}
	})
	if err != nil {
		t.Fatalf("RunEagerPreview: %v", err)
	}
	if terminal == nil {
		t.Fatal("missing visual-studio-caches eager preview observation")
	}
	if terminal.State != clean.CategoryPreviewComplete && terminal.State != clean.CategoryPreviewPartial {
		t.Fatalf("state = %q, want complete/partial; obs=%#v", terminal.State, terminal)
	}
	if terminal.Bytes == 0 || terminal.CandidateCount == 0 {
		t.Fatalf("observation = %#v, want measured candidates", terminal)
	}
	if !strings.Contains(terminal.SafetyNote, "Visual Studio") ||
		!(strings.Contains(terminal.SafetyNote, "slower") || strings.Contains(terminal.SafetyNote, "rebuild")) {
		t.Fatalf("safety note = %q, want Visual Studio rebuild impact", terminal.SafetyNote)
	}
	_ = parent
}

func TestVisualStudioCaches_PublicCatalogPathFree(t *testing.T) {
	summaries := clean.CanonicalCleanupCategoryCatalog().Summaries()
	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"ComponentModelCache", "Roslyn", "devenv.exe", "LOCALAPPDATA",
		"Microsoft\\\\VisualStudio", `Microsoft\VisualStudio`,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public catalog exposes private path/process data %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(string(encoded), "visual-studio-caches") {
		t.Fatal("public catalog missing visual-studio-caches identifier")
	}
}

func TestVisualStudioCaches_NoReviewSuggestionCommand(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	_ = writeVisualStudioCacheChild(t, parent, "Roslyn")
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	for _, s := range result.ReviewSuggestions {
		if strings.Contains(strings.ToLower(s.Tool+s.Label+s.Command), "visual studio") ||
			strings.Contains(s.CachePath, "VisualStudio") {
			t.Fatalf("unexpected review suggestion: %#v", s)
		}
	}
}

func TestVisualStudioCaches_ImmediateValidationOnExecute(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	child := writeVisualStudioCacheChild(t, parent, "Roslyn")

	// Remove between resolve and delete via custom permanent remover that
	// still records the path; production validate-before-delete is exercised
	// by shared execute (path missing becomes skip/fail without crash).
	if err := os.RemoveAll(child); err != nil {
		t.Fatal(err)
	}
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	// Missing root after remove-all of Roslyn: silent absence, no delete.
	if len(permanent.paths) != 0 {
		t.Fatalf("missing path should not permanently delete: %v", permanent.paths)
	}
}

func TestVisualStudioCaches_ParentNeverDeleted(t *testing.T) {
	_, parent := writeVisualStudioParent(t)
	child := writeVisualStudioCacheChild(t, parent, "17.0_cccc", "ComponentModelCache")

	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryVisualStudioCaches},
		DetectRunningApplications: idleVisualStudioDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != child {
		t.Fatalf("permanent paths = %v, want only %q", permanent.paths, child)
	}
	for _, p := range permanent.paths {
		if p == parent || strings.EqualFold(filepath.Base(p), "VisualStudio") {
			t.Fatalf("parent deleted: %v", permanent.paths)
		}
		if strings.EqualFold(filepath.Base(p), "17.0_cccc") {
			t.Fatalf("instance hive deleted: %v", permanent.paths)
		}
	}
}
