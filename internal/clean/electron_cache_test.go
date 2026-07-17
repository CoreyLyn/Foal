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

func TestElectronCache_CatalogAndGroupTokens(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.DevCacheCategoryElectron)
	if !ok {
		t.Fatal("electron-cache missing from catalog")
	}
	if summary.Label != "Electron cache" {
		t.Fatalf("label = %q", summary.Label)
	}
	if summary.ReportCategory != clean.ReportCategoryDeveloperTools {
		t.Fatalf("report category = %q", summary.ReportCategory)
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q", summary.Eligibility)
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicySharedRuntime {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}

	for _, token := range []string{
		clean.DevCacheCategoryElectron,
		"dev-caches",
		"all",
	} {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", token, invalid)
		}
		if !enabled[clean.DevCacheCategoryElectron] {
			t.Fatalf("%s did not enable electron-cache", token)
		}
	}

	// Selecting electron alone must not enable other developer caches.
	enabled, _, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryElectron})
	if len(enabled) != 1 || !enabled[clean.DevCacheCategoryElectron] {
		t.Fatalf("solo opt-in = %#v", enabled)
	}
}

func TestElectronCache_DefaultAndOverrideResolution(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom-electron-cache")
	if err := os.MkdirAll(custom, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "electron.zip"), []byte("aa"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("non-blank electron_config_cache", func(t *testing.T) {
		t.Setenv("electron_config_cache", custom)
		t.Setenv("LOCALAPPDATA", filepath.Join(root, "unused-local"))
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryElectron)
		if len(paths) != 1 || paths[0] != custom {
			t.Fatalf("paths = %#v, want [%q]", paths, custom)
		}
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryElectron},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != custom {
			t.Fatalf("candidates = %#v, want root %q", result.OptInCandidates, custom)
		}
		if result.OptInCandidates[0].Category != clean.DevCacheCategoryElectron {
			t.Fatalf("category = %q", result.OptInCandidates[0].Category)
		}
		if result.OptInCandidates[0].Bytes != 2 {
			t.Fatalf("bytes = %d, want 2", result.OptInCandidates[0].Bytes)
		}
	})

	t.Run("blank override falls back to LocalAppData default", func(t *testing.T) {
		local := filepath.Join(root, "local-blank")
		defaultRoot := filepath.Join(local, "electron", "Cache")
		if err := os.MkdirAll(defaultRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(defaultRoot, "bin.zip"), []byte("bb"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("electron_config_cache", "   ")
		t.Setenv("LOCALAPPDATA", local)
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryElectron)
		if len(paths) != 1 || paths[0] != defaultRoot {
			t.Fatalf("paths = %#v, want default %q", paths, defaultRoot)
		}
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryElectron},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != defaultRoot {
			t.Fatalf("candidates = %#v, want %q", result.OptInCandidates, defaultRoot)
		}
	})

	t.Run("missing env uses LOCALAPPDATA default", func(t *testing.T) {
		local := filepath.Join(root, "local-default")
		defaultRoot := filepath.Join(local, "electron", "Cache")
		if err := os.MkdirAll(defaultRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(defaultRoot, "bin.zip"), []byte("cc"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("LOCALAPPDATA", local)
		if err := os.Unsetenv("electron_config_cache"); err != nil {
			t.Fatal(err)
		}
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryElectron)
		if len(paths) != 1 || paths[0] != defaultRoot {
			t.Fatalf("paths = %#v, want %q", paths, defaultRoot)
		}
	})
}

func TestElectronCache_WholeRootOnlyNoSiblings(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "electron", "Cache")
	sibling := filepath.Join(root, "electron", "other")
	legacy := filepath.Join(root, ".electron")
	if err := os.MkdirAll(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "electron-v.zip"), []byte("root"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "noise.bin"), []byte("sibling"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.bin"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		// Isolate from host TEMP foal-* pollution when asserting frozen defaults.
		Rules: []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only resolved root", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != cacheRoot {
		t.Fatalf("path = %q, want root %q", result.OptInCandidates[0].Path, cacheRoot)
	}
	if result.Totals.OptInReclaimableBytes != int64(len("root")) {
		t.Fatalf("opt-in reclaimable = %d", result.Totals.OptInReclaimableBytes)
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidates must stay frozen, got %d", result.Totals.CandidateBytes)
	}
}

func TestElectronCache_MissingAndEmptyRoots(t *testing.T) {
	t.Run("missing root silent", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryElectron},
			DevCachePathResolver:      func(string) []string { return []string{missing} },
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 {
			t.Fatalf("candidates = %#v, want silent absence", result.OptInCandidates)
		}
		if result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("reclaimable = %d, want 0", result.Totals.OptInReclaimableBytes)
		}
	})

	t.Run("empty root no reclaimable bytes", func(t *testing.T) {
		empty := t.TempDir()
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryElectron},
			DevCachePathResolver:      func(string) []string { return []string{empty} },
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("reclaimable = %d, want 0 for empty root", result.Totals.OptInReclaimableBytes)
		}
	})
}

func TestElectronCache_SharedRuntimeDoesNotGateOnNode(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "f"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// Shared-runtime policy must not attribute Node (or similar hosts) to this
	// cache: candidates remain even when a detector reports Node running.
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver: func(string) []string {
			return []string{cachePath}
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationNode,
				State:       clean.RunningApplicationStateRunning,
			}}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want 1 despite node running", result.OptInCandidates)
	}
	for _, skipped := range result.Skipped {
		if skipped.Rule == clean.DevCacheCategoryElectron {
			t.Fatalf("electron-cache must not be gated by node: %#v", skipped)
		}
	}
}

func TestElectronCache_DefaultExecuteDoesNotResolve(t *testing.T) {
	resolverCalls := 0
	detectionCalled := false
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: []string{},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryElectron {
				resolverCalls++
			}
			return []string{`C:\would-not-run`}
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
		t.Fatalf("electron resolver called %d times without opt-in", resolverCalls)
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

func TestElectronCache_ExecuteFreshResolveAndHistory(t *testing.T) {
	root := t.TempDir()
	previewRoot := filepath.Join(root, "preview-cache")
	executeRoot := filepath.Join(root, "execute-cache")
	if err := os.MkdirAll(previewRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(executeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previewRoot, "old.zip"), []byte("old!"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(executeRoot, "new.zip"), []byte("new!"), 0600); err != nil {
		t.Fatal(err)
	}

	// Dry-run resolves preview path only.
	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{previewRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(dry.OptInCandidates) != 1 || dry.OptInCandidates[0].Path != previewRoot {
		t.Fatalf("dry-run candidates = %#v, want preview root", dry.OptInCandidates)
	}

	// Execute independently resolves execute path; never trusts preview paths.
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{executeRoot} },
		PermanentRemover:          permanent,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != executeRoot {
		t.Fatalf("permanent paths = %v, want only %q", permanent.paths, executeRoot)
	}
	for _, p := range permanent.paths {
		if p == previewRoot {
			t.Fatal("execute trusted dry-run path")
		}
	}
	if execResult.Totals.OptInDeletedCount != 1 {
		t.Fatalf("opt-in deleted = %d", execResult.Totals.OptInDeletedCount)
	}
	if len(execResult.Deleted) != 1 || execResult.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v, want delete_permanently", execResult.Deleted)
	}
	if len(recorder.items) == 0 {
		t.Fatal("history items empty, want path-bearing opt-in records")
	}
	found := false
	for _, item := range recorder.items {
		if item.Path == executeRoot {
			found = true
			if item.Rule != clean.DevCacheCategoryElectron {
				t.Fatalf("history rule = %q", item.Rule)
			}
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
		if item.Path == previewRoot {
			t.Fatal("history recorded preview path")
		}
	}
	if !found {
		t.Fatalf("history items = %#v, want execute root", recorder.items)
	}
}

func TestElectronCache_Protection(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("prot"), 0600); err != nil {
		t.Fatal(err)
	}
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		Validator:                 pathsafe.NewValidator([]string{cachePath}),
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
		t.Fatalf("protected electron root leaked: candidates=%#v totals=%#v", result.OptInCandidates, result.Totals)
	}
}

func TestElectronCache_CapacityDoesNotBlockPermanent(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "large.bin"), []byte("12345678"), 0600); err != nil {
		t.Fatal(err)
	}

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
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
	if len(permanent.paths) != 1 || permanent.paths[0] != cachePath {
		t.Fatalf("permanent paths = %v, want %q (capacity must not block permanent)", permanent.paths, cachePath)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "recycle_bin_capacity" {
			t.Fatalf("permanent candidate skipped for recycle capacity: %#v", skipped)
		}
	}
}

func TestElectronCache_Cancellation(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(ctx, clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
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

func TestElectronCache_ImpactNoticeAndFrozenDefaults(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "Electron") &&
			strings.Contains(notice.Message, "re-download") {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want electron re-download/offline impact", model.Notices)
	}
	for _, c := range result.Candidates {
		if c.Rule == clean.DevCacheCategoryElectron || strings.Contains(c.Path, "electron") {
			t.Fatalf("electron path leaked into default candidates: %#v", c)
		}
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable for electron cache")
	}

	// Without opt-in, no electron candidates even if a resolver is injected.
	defaultResult := clean.DryRun(context.Background(), clean.Options{
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(defaultResult.OptInCandidates) != 0 {
		t.Fatalf("default dry-run opt-in candidates = %#v", defaultResult.OptInCandidates)
	}
}

func TestElectronCache_TUICategoryIdentifierOnly(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, cat := range model.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryElectron {
			found = true
			if cat.Selected {
				t.Fatal("electron-cache must start unselected")
			}
		}
	}
	if !found {
		t.Fatalf("OptInCategories missing electron-cache: %#v", model.OptInCategories)
	}

	selected := clean.NewPreviewReadModelForSelection(result, []string{clean.DevCacheCategoryElectron})
	for _, cat := range selected.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryElectron && !cat.Selected {
			t.Fatal("expected selected after identifier selection")
		}
	}
}

func TestElectronCache_NoReviewSuggestionCommand(t *testing.T) {
	// Electron must not invent an unsupported cleanup command on the Review surface.
	// Selecting electron-cache leaves unrelated review suggestions alone and never
	// synthesizes an Electron tool command.
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		DevCachePathResolver:      func(string) []string { return nil },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	for _, suggestion := range result.ReviewSuggestions {
		if strings.Contains(strings.ToLower(suggestion.Tool), "electron") ||
			strings.Contains(strings.ToLower(suggestion.Command), "electron") {
			t.Fatalf("unexpected electron review suggestion: %#v", suggestion)
		}
	}
}

func TestElectronCache_PublicCatalogPathFree(t *testing.T) {
	summaries := clean.CanonicalCleanupCategoryCatalog().Summaries()
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		"electron_config_cache", "resolvePaths", "discoverChildren",
		`electron\Cache`, ".electron",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("catalog exposes private detail %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "electron-cache") {
		t.Fatalf("catalog missing public identifier: %s", encoded)
	}
}

func TestElectronCache_ImmediateValidationOnExecute(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "electron-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	// Protect the path so immediate pre-delete validation rejects it after scan
	// would otherwise have measured it (validator applies on execute path).
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryElectron},
		Validator:                 pathsafe.NewValidator([]string{cachePath}),
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover must not delete protected path: %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}
