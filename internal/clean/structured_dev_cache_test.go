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

// structuredDiscoverer forces structured mode for npm-cache under root, returning
// the given relative child names joined under root. Other categories stay whole-root.
func structuredDiscoverer(relativeChildren ...string) clean.DevCacheChildDiscoverer {
	return func(_ context.Context, category, root string) ([]string, bool) {
		if category != clean.DevCacheCategoryNPM {
			return nil, false
		}
		children := make([]string, 0, len(relativeChildren))
		for _, name := range relativeChildren {
			children = append(children, filepath.Join(root, name))
		}
		return children, true
	}
}

func writeStructuredChildDir(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(path, "data.bin"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestStructuredDevCacheDiscovery_IndependentChildrenNotRoot(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "ms-tool-cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	// Root payload must never become reclaimable when structured mode is on.
	if err := os.WriteFile(filepath.Join(cacheRoot, "root-only.bin"), []byte("root-payload"), 0600); err != nil {
		t.Fatal(err)
	}
	childA := writeStructuredChildDir(t, cacheRoot, "rev-a", "aaaa")
	childB := writeStructuredChildDir(t, cacheRoot, "rev-b", "bbbbbb")
	// Unknown sibling is not returned by the policy and must not appear.
	_ = writeStructuredChildDir(t, cacheRoot, "unknown-meta", "meta")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer:   structuredDiscoverer("rev-a", "rev-b"),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})

	if len(result.OptInCandidates) != 2 {
		t.Fatalf("opt-in candidates = %#v, want 2 structured children", result.OptInCandidates)
	}
	seen := map[string]int64{}
	for _, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryNPM {
			t.Errorf("category = %q", c.Category)
		}
		if c.Path == cacheRoot {
			t.Fatalf("root itself must not be a structured candidate: %q", c.Path)
		}
		seen[c.Path] = c.Bytes
	}
	if seen[childA] != 4 || seen[childB] != 6 {
		t.Fatalf("candidate bytes = %#v, want rev-a=4 rev-b=6", seen)
	}
	if result.Totals.OptInReclaimableBytes != 10 {
		t.Fatalf("opt-in reclaimable = %d, want 10", result.Totals.OptInReclaimableBytes)
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidate bytes must stay frozen, got %d", result.Totals.CandidateBytes)
	}
}

func TestStructuredDevCacheDiscovery_RejectsRootOutsideFileAndReparse(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	good := writeStructuredChildDir(t, cacheRoot, "good", "ok")
	filePath := filepath.Join(cacheRoot, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := writeStructuredChildDir(t, root, "outside", "xx")
	linkPath := filepath.Join(cacheRoot, "linked")
	if err := os.Symlink(good, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver: func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer: func(_ context.Context, category, r string) ([]string, bool) {
			if category != clean.DevCacheCategoryNPM {
				return nil, false
			}
			return []string{
				r,                                 // root itself
				outside,                           // outside root
				filePath,                          // regular file
				linkPath,                          // symlink/reparse
				good,                              // valid child
				filepath.Join(r, "..", "outside"), // traversal escape form
			}, true
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v, want only good child", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != good {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, good)
	}
}

func TestStructuredDevCacheDiscovery_ProtectionPerChild(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	protected := writeStructuredChildDir(t, cacheRoot, "protected-rev", "prot")
	allowed := writeStructuredChildDir(t, cacheRoot, "allowed-rev", "allow")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryNPM},
		Validator:                 pathsafe.NewValidator([]string{protected}),
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer:   structuredDiscoverer("protected-rev", "allowed-rev"),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v, want only unprotected sibling", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != allowed {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, allowed)
	}
	if result.Totals.OptInReclaimableBytes != int64(len("allow")) {
		t.Fatalf("opt-in reclaimable = %d, want %d", result.Totals.OptInReclaimableBytes, len("allow"))
	}
}

func TestStructuredDevCacheDiscovery_ProtectedRootSkipsAllChildren(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	_ = writeStructuredChildDir(t, cacheRoot, "rev-a", "data")

	discoverCalls := 0
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                []string{clean.DevCacheCategoryNPM},
		Validator:            pathsafe.NewValidator([]string{cacheRoot}),
		DevCachePathResolver: func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer: func(_ context.Context, category, r string) ([]string, bool) {
			discoverCalls++
			return structuredDiscoverer("rev-a")(context.Background(), category, r)
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if discoverCalls != 0 {
		t.Fatalf("child discovery called %d times for protected root, want 0", discoverCalls)
	}
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("opt-in candidates = %#v, want none", result.OptInCandidates)
	}
}

func TestStructuredDevCacheDiscovery_IncompleteSiblingDoesNotBlockComplete(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	complete := writeStructuredChildDir(t, cacheRoot, "complete", "done")
	missing := filepath.Join(cacheRoot, "missing-rev")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver: func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer: func(_ context.Context, category, r string) ([]string, bool) {
			if category != clean.DevCacheCategoryNPM {
				return nil, false
			}
			return []string{missing, complete}, true
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != complete {
		t.Fatalf("opt-in candidates = %#v, want complete sibling only", result.OptInCandidates)
	}
}

func TestStructuredDevCacheDiscovery_WholeRootUnchangedWithoutPolicy(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver:      func(string) []string { return []string{cachePath} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v, want whole-root candidate", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != cachePath {
		t.Fatalf("path = %q, want whole root %q", result.OptInCandidates[0].Path, cachePath)
	}
}

func TestStructuredDevCacheDiscovery_DefaultExecuteDoesNotDiscover(t *testing.T) {
	resolverCalls := 0
	discoverCalls := 0
	adapter := &recordingRecycleBinAdapter{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: []string{},
		DevCachePathResolver: func(category string) []string {
			resolverCalls++
			return []string{`C:\would-not-run`}
		},
		DevCacheChildDiscoverer: func(_ context.Context, category, root string) ([]string, bool) {
			discoverCalls++
			return []string{filepath.Join(root, "child")}, true
		},
		RecycleBinAdapter:         adapter,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})

	if resolverCalls != 0 {
		t.Fatalf("resolver called %d times without opt-in, want 0", resolverCalls)
	}
	if discoverCalls != 0 {
		t.Fatalf("child discovery called %d times without opt-in, want 0", discoverCalls)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v, want empty", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d, want 0", result.Totals.OptInDeletedCount)
	}
}

func TestStructuredDevCacheDiscovery_ExecuteFreshResolvesChildren(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	previewChild := writeStructuredChildDir(t, cacheRoot, "preview-only", "old")
	executeChild := writeStructuredChildDir(t, cacheRoot, "execute-only", "new!")

	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer:   structuredDiscoverer("preview-only"),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(dry.OptInCandidates) != 1 || dry.OptInCandidates[0].Path != previewChild {
		t.Fatalf("dry-run candidates = %#v", dry.OptInCandidates)
	}

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer:   structuredDiscoverer("execute-only"),
		RecycleBinAdapter:         recycle,
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})

	if len(recycle.paths) != 0 {
		t.Fatalf("structured npm children must not use Recycle Bin: %v", recycle.paths)
	}
	if len(permanent.paths) != 1 || permanent.paths[0] != executeChild {
		t.Fatalf("permanent paths = %v, want only fresh execute child %q", permanent.paths, executeChild)
	}
	for _, p := range permanent.paths {
		if p == previewChild {
			t.Fatalf("execute trusted dry-run path %q", previewChild)
		}
		if p == cacheRoot {
			t.Fatalf("execute sent root to permanent remover: %q", cacheRoot)
		}
	}
	if execResult.Totals.OptInDeletedCount != 1 {
		t.Fatalf("opt-in deleted = %d, want 1", execResult.Totals.OptInDeletedCount)
	}
	if execResult.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("action = %q", execResult.Deleted[0].Action)
	}
}

func TestStructuredDevCacheDiscovery_DeduplicatesNormalizedChildren(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	child := writeStructuredChildDir(t, cacheRoot, "rev", "data")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver: func(string) []string { return []string{cacheRoot} },
		DevCacheChildDiscoverer: func(_ context.Context, category, r string) ([]string, bool) {
			if category != clean.DevCacheCategoryNPM {
				return nil, false
			}
			return []string{child, filepath.Join(r, "rev")}, true
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v, want 1 after dedupe", result.OptInCandidates)
	}
}

func TestPublicCatalogRemainsPathFreeWithStructuredSeam(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summary, ok := catalog.Summary(clean.DevCacheCategoryPlaywright)
	if !ok || summary.Eligibility != clean.CategoryEligibilityOptIn ||
		summary.ReportCategory != clean.ReportCategoryDeveloperTools ||
		summary.RunningApplicationPolicy != clean.RunningApplicationPolicySharedRuntime {
		t.Fatalf("playwright-browsers summary = %#v", summary)
	}
	puppeteerSummary, ok := catalog.Summary(clean.DevCacheCategoryPuppeteerBrowsers)
	if !ok || puppeteerSummary.Eligibility != clean.CategoryEligibilityOptIn ||
		puppeteerSummary.ReportCategory != clean.ReportCategoryDeveloperTools ||
		puppeteerSummary.RunningApplicationPolicy != clean.RunningApplicationPolicySharedRuntime {
		t.Fatalf("puppeteer-browsers summary = %#v", puppeteerSummary)
	}
	encoded, err := json.Marshal(catalog.Summaries())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"discoverChildren", "resolvePaths", "ms-playwright",
		"PLAYWRIGHT_BROWSERS_PATH", "INSTALLATION_COMPLETE", "PUPPETEER_CACHE_DIR",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("catalog exposes private discovery detail %q: %s", forbidden, encoded)
		}
	}
}
