package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

func writePlaywrightCompleteRevision(t *testing.T, root, name, payload string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "INSTALLATION_COMPLETE"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if payload != "" {
		if err := os.WriteFile(filepath.Join(dir, "payload.bin"), []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writePlaywrightIncompleteRevision(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Matching name without INSTALLATION_COMPLETE is incomplete.
	if err := os.WriteFile(filepath.Join(dir, "partial.bin"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPlaywrightBrowsersOptInDiscoversAllowedComponents(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	// Root-only payload must never be reclaimable.
	if err := os.WriteFile(filepath.Join(browsersRoot, "root-only.bin"), []byte("root"), 0600); err != nil {
		t.Fatal(err)
	}

	allowed := []struct {
		name    string
		payload string
	}{
		{"chromium-1161", "aaaa"},
		{"chromium_headless_shell-1161", "bbbbb"},
		{"firefox-1475", "cccccc"},
		{"webkit-2140", "ddddddd"},
		{"ffmpeg-1011", "eeeeeeee"},
		{"winldd-1007", "fffffffff"},
	}
	wantBytes := map[string]int64{}
	for _, item := range allowed {
		path := writePlaywrightCompleteRevision(t, browsersRoot, item.name, item.payload)
		// INSTALLATION_COMPLETE + payload
		wantBytes[path] = int64(len("ok") + len(item.payload))
	}
	// Second revision of chromium stays independent.
	multi := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-999", "zz")
	wantBytes[multi] = int64(len("ok") + len("zz"))

	// Exclusions: MCP profiles, metadata, unknown, incomplete, file decoy.
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "mcp-chrome-abc", "profile-state")
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "mcp-something-1", "other-mcp")
	if err := os.Mkdir(filepath.Join(browsersRoot, ".links"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(browsersRoot, "b"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "unknown-tool-1", "nope")
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "chromium-headless-shell-1", "hyphen-form")
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1a2", "non-numeric")
	_ = writePlaywrightIncompleteRevision(t, browsersRoot, "chromium-42")
	if err := os.WriteFile(filepath.Join(browsersRoot, "chromium-777"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	// Decoy profile/state directory names that must never match.
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "Default", "profile")
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "User Data", "state")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})

	if len(result.OptInCandidates) != len(wantBytes) {
		t.Fatalf("opt-in candidates = %#v, want %d allowed revisions", result.OptInCandidates, len(wantBytes))
	}
	var total int64
	for _, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryPlaywright {
			t.Errorf("category = %q", c.Category)
		}
		if c.Path == browsersRoot {
			t.Fatalf("root must never be a candidate: %q", c.Path)
		}
		want, ok := wantBytes[c.Path]
		if !ok {
			t.Fatalf("unexpected candidate path %q", c.Path)
		}
		if c.Bytes != want {
			t.Fatalf("bytes for %q = %d, want %d", c.Path, c.Bytes, want)
		}
		total += c.Bytes
	}
	if result.Totals.OptInReclaimableBytes != total {
		t.Fatalf("opt-in reclaimable = %d, want %d", result.Totals.OptInReclaimableBytes, total)
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidates must stay frozen, got %d", result.Totals.CandidateBytes)
	}

	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "Playwright") &&
			strings.Contains(notice.Message, "re-download") {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want Playwright re-download/active-automation impact", model.Notices)
	}
}

func TestPlaywrightBrowsersExcludesSymlinkRevision(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	good := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "ok")
	target := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-2", "link-target")
	link := filepath.Join(browsersRoot, "firefox-3")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 2 {
		// good + target; link rejected
		t.Fatalf("candidates = %#v, want good and target only", result.OptInCandidates)
	}
	for _, c := range result.OptInCandidates {
		if c.Path == link {
			t.Fatalf("symlink revision must not be a candidate: %q", link)
		}
		if c.Path != good && c.Path != target {
			t.Fatalf("unexpected path %q", c.Path)
		}
	}
}

func TestPlaywrightBrowsersProtectionPerChild(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	protected := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "prot")
	allowed := writePlaywrightCompleteRevision(t, browsersRoot, "firefox-2", "allow")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		Validator:                 pathsafe.NewValidator([]string{protected}),
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != allowed {
		t.Fatalf("candidates = %#v, want only unprotected sibling", result.OptInCandidates)
	}
}

func TestPlaywrightBrowsersProtectedRootSkipsDiscovery(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "data")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		Validator:                 pathsafe.NewValidator([]string{browsersRoot}),
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
		t.Fatalf("protected root leaked: %#v", result.OptInCandidates)
	}
}

func TestPlaywrightBrowsersDefaultExecuteDoesNotResolve(t *testing.T) {
	resolverCalls := 0
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: nil,
		DevCachePathResolver: func(category string) []string {
			resolverCalls++
			return []string{`C:\would-not-run`}
		},
		RecycleBinAdapter:         adapter,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if resolverCalls != 0 {
		t.Fatalf("resolver called %d times without opt-in", resolverCalls)
	}
	if len(adapter.paths) != 0 || result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("unexpected execute activity: adapter=%v result=%#v", adapter.paths, result)
	}
}

func TestPlaywrightBrowsersExecuteFreshDiscoveryAndCapacity(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	previewOnly := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "old")
	executeOnly := writePlaywrightCompleteRevision(t, browsersRoot, "firefox-2", "new!")

	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver: func(string) []string {
			// Preview only sees chromium-1 by using a temporary root with one child.
			previewRoot := filepath.Join(root, "preview-root")
			_ = os.RemoveAll(previewRoot)
			if err := os.Mkdir(previewRoot, 0700); err != nil {
				t.Fatal(err)
			}
			// Mirror only preview child name into a separate root so dry-run path differs.
			dst := writePlaywrightCompleteRevision(t, previewRoot, "chromium-1", "old")
			_ = dst
			return []string{previewRoot}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(dry.OptInCandidates) != 1 {
		t.Fatalf("dry-run candidates = %#v", dry.OptInCandidates)
	}
	if dry.OptInCandidates[0].Path == executeOnly {
		t.Fatal("dry-run unexpectedly resolved execute-only child")
	}
	_ = previewOnly

	permanent := &recordingPermanentRemover{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(permanent.paths) != 2 {
		t.Fatalf("permanent paths = %v, want both fresh children", permanent.paths)
	}
	for _, p := range permanent.paths {
		if p == browsersRoot {
			t.Fatalf("root passed to permanent remover: %q", p)
		}
		if strings.Contains(p, "preview-root") {
			t.Fatalf("execute trusted dry-run path %q", p)
		}
	}
	if execResult.Totals.OptInDeletedCount != 2 {
		t.Fatalf("OptInDeletedCount = %d, want 2", execResult.Totals.OptInDeletedCount)
	}
	if execResult.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0, want playwright content")
	}

	// Permanent candidates are excluded from Recycle Bin capacity; low capacity
	// must not block authorized permanent deletion. Recreate children because the
	// previous permanent execute actually removed them.
	browsersRoot2 := filepath.Join(root, "ms-playwright-cap")
	if err := os.Mkdir(browsersRoot2, 0700); err != nil {
		t.Fatal(err)
	}
	_ = writePlaywrightCompleteRevision(t, browsersRoot2, "chromium-1", "old")
	_ = writePlaywrightCompleteRevision(t, browsersRoot2, "firefox-2", "new!")
	permanent = &recordingPermanentRemover{}
	capResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:   func(string) []string { return []string{browsersRoot2} },
		PermanentRemover:       permanent,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       "C:",
				NukeOnDelete: false,
				MaxCapacity:  1,
				CurrentUsage: 0,
			}, nil
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(permanent.paths) != 2 {
		t.Fatalf("low Recycle Bin capacity blocked permanent playwright: %v", permanent.paths)
	}
	if capResult.Totals.OptInDeletedCount != 2 {
		t.Fatalf("deleted = %d after capacity probe on permanent-only run", capResult.Totals.OptInDeletedCount)
	}
	for _, skipped := range capResult.Skipped {
		if skipped.Reason.Code == "recycle_bin_capacity" {
			t.Fatalf("permanent candidate skipped for recycle capacity: %#v", skipped)
		}
	}
}

func TestPlaywrightBrowsersExecuteHistoryAndCanceled(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	child := writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "data")

	recorder := &recordingHistoryRecorder{}
	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		PermanentRemover:          permanent,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if result.Totals.OptInDeletedCount != 1 || len(permanent.paths) != 1 || permanent.paths[0] != child {
		t.Fatalf("execute result/permanent = %#v / %v", result, permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("deleted = %#v, want delete_permanently", result.Deleted)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("history sessions = %d", len(recorder.sessions))
	}
	found := false
	for _, item := range recorder.items {
		if item.Path == child {
			found = true
			if item.Action != string(clean.PlannedActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
	}
	if !found {
		t.Fatalf("history items missing executed path: %#v", recorder.items)
	}

	// Non-opted-in: no history items for Playwright paths.
	recorder = &recordingHistoryRecorder{}
	_ = executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     nil,
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		PermanentRemover:          &recordingPermanentRemover{},
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	for _, item := range recorder.items {
		if item.Path == child || item.Path == browsersRoot {
			t.Fatalf("non-opted-in Playwright path persisted in history: %#v", item)
		}
	}

	// Cancellation during execute scanning yields no permanent remover calls.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	permanent = &recordingPermanentRemover{}
	canceled := executeCleanWithSafeCapacity(ctx, clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver:      func(string) []string { return []string{browsersRoot} },
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "disabled", DefaultEnabled: false}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("canceled execute still deleted: %v", permanent.paths)
	}
	_ = canceled
}

func TestPlaywrightBrowsersNormalizedOptInGroups(t *testing.T) {
	enabled, invalid, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryPlaywright})
	if len(invalid) != 0 || !enabled[clean.DevCacheCategoryPlaywright] || len(enabled) != 1 {
		t.Fatalf("playwright-browsers opt-in = %#v %#v", enabled, invalid)
	}
	enabled, invalid, _ = clean.NormalizedOptInSet([]string{"dev-caches"})
	if len(invalid) != 0 || !enabled[clean.DevCacheCategoryPlaywright] {
		t.Fatalf("dev-caches missing playwright: %#v %#v", enabled, invalid)
	}
	enabled, invalid, _ = clean.NormalizedOptInSet([]string{"all"})
	if len(invalid) != 0 || !enabled[clean.DevCacheCategoryPlaywright] {
		t.Fatalf("all missing playwright: %#v %#v", enabled, invalid)
	}
	// Selecting playwright alone must not enable other frameworks.
	if enabled, _, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryPlaywright}); enabled[clean.DevCacheCategoryNPM] {
		t.Fatal("playwright-browsers must not enable npm-cache")
	}
}

func TestPlaywrightBrowsersSharedRuntimeDoesNotGateOnNodeChrome(t *testing.T) {
	root := t.TempDir()
	browsersRoot := filepath.Join(root, "ms-playwright")
	if err := os.Mkdir(browsersRoot, 0700); err != nil {
		t.Fatal(err)
	}
	_ = writePlaywrightCompleteRevision(t, browsersRoot, "chromium-1", "data")

	// Running shared runtimes must not suppress Playwright candidates (shared-runtime
	// policy). Dry-run may still call detection for browser/application review.
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                []string{clean.DevCacheCategoryPlaywright},
		DevCachePathResolver: func(string) []string { return []string{browsersRoot} },
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationNode, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationPython, State: clean.RunningApplicationStateRunning},
				{Application: clean.ApplicationGoogleChrome, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want 1 despite node/python/chrome running", result.OptInCandidates)
	}
	for _, skipped := range result.Skipped {
		if skipped.Rule == clean.DevCacheCategoryPlaywright {
			t.Fatalf("playwright must not be skipped via shared runtime attribution: %#v", skipped)
		}
	}
}

func TestPlaywrightBrowsersTUICategoryPresentAndUnselected(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summary, ok := catalog.Summary(clean.DevCacheCategoryPlaywright)
	if !ok {
		t.Fatal("playwright-browsers missing from catalog")
	}
	if summary.Label != "Playwright browsers" {
		t.Fatalf("label = %q", summary.Label)
	}
	// Selection model starts empty; identifier is selectable via NormalizedOptInSet.
	selected, _, _ := clean.NormalizedOptInSet(nil)
	if selected[clean.DevCacheCategoryPlaywright] {
		t.Fatal("empty selection must not enable playwright-browsers")
	}
	model := clean.NewPreviewReadModelForSelection(clean.Result{}, nil)
	found := false
	for _, cat := range model.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryPlaywright {
			found = true
			if cat.Selected {
				t.Fatal("playwright-browsers must start unselected in TUI selection projection")
			}
		}
	}
	if !found {
		t.Fatal("playwright-browsers missing from Clean TUI opt-in category projection")
	}
}

// Ensure history.Recorder type is referenced if package import is needed by helpers.
var _ history.Recorder
