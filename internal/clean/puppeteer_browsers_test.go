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

// writePuppeteerInstall creates <root>/<product>/<platformVersion>/payload.bin
// and returns the platform-version installation path.
func writePuppeteerInstall(t *testing.T, root, product, platformVersion, content string) string {
	t.Helper()
	install := filepath.Join(root, product, platformVersion)
	if err := os.MkdirAll(install, 0700); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(install, "payload.bin"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return install
}

func TestPuppeteerBrowsers_CatalogAndGroupTokens(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.DevCacheCategoryPuppeteerBrowsers)
	if !ok {
		t.Fatal("puppeteer-browsers missing from catalog")
	}
	if summary.Label != "Puppeteer browsers" {
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
		clean.DevCacheCategoryPuppeteerBrowsers,
		"dev-caches",
		"all",
	} {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", token, invalid)
		}
		if !enabled[clean.DevCacheCategoryPuppeteerBrowsers] {
			t.Fatalf("%s did not enable puppeteer-browsers", token)
		}
	}

	// Selecting puppeteer alone must not enable other frameworks.
	enabled, _, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryPuppeteerBrowsers})
	if len(enabled) != 1 || !enabled[clean.DevCacheCategoryPuppeteerBrowsers] {
		t.Fatalf("solo opt-in = %#v", enabled)
	}
}

func TestPuppeteerBrowsers_DefaultAndOverrideResolution(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom-puppeteer")
	install := writePuppeteerInstall(t, custom, "chrome", "win64-120.0.0.0", "aa")

	t.Run("non-blank PUPPETEER_CACHE_DIR", func(t *testing.T) {
		t.Setenv("PUPPETEER_CACHE_DIR", custom)
		t.Setenv("USERPROFILE", filepath.Join(root, "unused-home"))
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryPuppeteerBrowsers)
		if len(paths) != 1 || paths[0] != custom {
			t.Fatalf("paths = %#v, want [%q]", paths, custom)
		}
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != install {
			t.Fatalf("candidates = %#v, want install %q", result.OptInCandidates, install)
		}
	})

	t.Run("blank override falls back to home default", func(t *testing.T) {
		home := filepath.Join(root, "home-blank")
		defaultRoot := filepath.Join(home, ".cache", "puppeteer")
		defaultInstall := writePuppeteerInstall(t, defaultRoot, "firefox", "win32-1.0.0", "bb")
		t.Setenv("PUPPETEER_CACHE_DIR", "   ")
		t.Setenv("USERPROFILE", home)
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryPuppeteerBrowsers)
		if len(paths) != 1 || paths[0] != defaultRoot {
			t.Fatalf("paths = %#v, want default %q", paths, defaultRoot)
		}
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != defaultInstall {
			t.Fatalf("candidates = %#v, want %q", result.OptInCandidates, defaultInstall)
		}
	})

	t.Run("missing env uses USERPROFILE default", func(t *testing.T) {
		home := filepath.Join(root, "home-default")
		defaultRoot := filepath.Join(home, ".cache", "puppeteer")
		_ = writePuppeteerInstall(t, defaultRoot, "chrome-headless-shell", "win64-131.0.6778.204", "cc")
		t.Setenv("USERPROFILE", home)
		// Unset override so default applies.
		if err := os.Unsetenv("PUPPETEER_CACHE_DIR"); err != nil {
			t.Fatal(err)
		}
		paths := clean.ResolveDevCachePaths(clean.DevCacheCategoryPuppeteerBrowsers)
		if len(paths) != 1 || paths[0] != defaultRoot {
			t.Fatalf("paths = %#v, want %q", paths, defaultRoot)
		}
	})
}

func TestPuppeteerBrowsers_AllAllowedProductsAndWindowsPlatforms(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	// Root payload and product parents must never become candidates.
	if err := os.MkdirAll(cacheRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "root-only.bin"), []byte("root"), 0600); err != nil {
		t.Fatal(err)
	}

	want := []string{
		writePuppeteerInstall(t, cacheRoot, "chrome", "win64-120.0.6099.109", "chrome64"),
		writePuppeteerInstall(t, cacheRoot, "chrome", "win32-120.0.6099.109", "chrome32"),
		writePuppeteerInstall(t, cacheRoot, "chrome-headless-shell", "win64-131.0.6778.204", "shell"),
		writePuppeteerInstall(t, cacheRoot, "firefox", "win64-125.0.1", "ff"),
		writePuppeteerInstall(t, cacheRoot, "firefox", "win32-125.0.1", "ff32"),
	}
	// Multiple versions under one product.
	secondChrome := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-121.0.6167.85", "chrome2")
	want = append(want, secondChrome)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})

	if len(result.OptInCandidates) != len(want) {
		t.Fatalf("candidates = %#v, want %d installs", result.OptInCandidates, len(want))
	}
	seen := map[string]bool{}
	var total int64
	for _, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryPuppeteerBrowsers {
			t.Fatalf("category = %q", c.Category)
		}
		if c.Path == cacheRoot {
			t.Fatal("root must not be a candidate")
		}
		if filepath.Base(filepath.Dir(c.Path)) == filepath.Base(cacheRoot) {
			t.Fatalf("product parent must not be a candidate: %q", c.Path)
		}
		seen[c.Path] = true
		total += c.Bytes
	}
	for _, path := range want {
		if !seen[path] {
			t.Fatalf("missing candidate %q in %#v", path, result.OptInCandidates)
		}
	}
	if result.Totals.OptInReclaimableBytes != total || total == 0 {
		t.Fatalf("opt-in reclaimable = %d total=%d", result.Totals.OptInReclaimableBytes, total)
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidates must stay frozen, got %d", result.Totals.CandidateBytes)
	}
}

func TestPuppeteerBrowsers_ExcludesUnknownAndUnsafe(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	good := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "ok")

	// Unknown product
	_ = writePuppeteerInstall(t, cacheRoot, "chromedriver", "win64-1.0.0", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chromium", "win64-1.0.0", "no")
	// Non-Windows platforms
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "linux-1.0.0", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "mac_arm-1.0.0", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "linux_arm-1.0.0", "no")
	// Malformed version directories
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "win64", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "win64-", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0-extra", "no")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "WIN64-1.0.0", "no")
	// Metadata and regular files under product
	productChrome := filepath.Join(cacheRoot, "chrome")
	if err := os.WriteFile(filepath.Join(productChrome, ".metadata"), []byte(`{"aliases":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(productChrome, "137.0.0-chrome-win64.zip"), []byte("zip"), 0600); err != nil {
		t.Fatal(err)
	}
	// Regular file at root
	if err := os.WriteFile(filepath.Join(cacheRoot, "stray.bin"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// Symlink install (when available)
	linkPath := filepath.Join(productChrome, "win64-link")
	if err := os.Symlink(good, linkPath); err != nil {
		t.Logf("symlink skip: %v", err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only good install", result.OptInCandidates)
	}
	if result.OptInCandidates[0].Path != good {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, good)
	}
	// No excluded path may contribute bytes via totals.
	if result.Totals.OptInReclaimableBytes != int64(len("ok")) {
		t.Fatalf("opt-in reclaimable = %d", result.Totals.OptInReclaimableBytes)
	}
}

func TestPuppeteerBrowsers_ProtectionPerChildAndRoot(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	protected := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "prot")
	allowed := writePuppeteerInstall(t, cacheRoot, "firefox", "win64-2.0.0", "allow")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		Validator:                 pathsafe.NewValidator([]string{protected}),
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != allowed {
		t.Fatalf("candidates = %#v, want unprotected %q", result.OptInCandidates, allowed)
	}

	// Protected root skips all children without discovery measurement noise.
	resultRoot := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		Validator:                 pathsafe.NewValidator([]string{cacheRoot}),
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(resultRoot.OptInCandidates) != 0 {
		t.Fatalf("protected root candidates = %#v", resultRoot.OptInCandidates)
	}
}

func TestPuppeteerBrowsers_DefaultExecuteDoesNotResolve(t *testing.T) {
	resolverCalls := 0
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: []string{},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryPuppeteerBrowsers {
				resolverCalls++
			}
			return []string{`C:\would-not-run`}
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
		t.Fatalf("puppeteer resolver called %d times without opt-in", resolverCalls)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestPuppeteerBrowsers_ExecuteFreshDiscoveryAndNotRoot(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	previewOnly := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-preview", "old")
	executeOnly := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-execute", "new!")

	// Dry-run sees preview-only by temporarily removing execute-only path name match...
	// Use resolver that always returns root; discovery sees both. Force execute-only
	// freshness by deleting preview-only before execute.
	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	seenPreview := false
	for _, c := range dry.OptInCandidates {
		if c.Path == previewOnly {
			seenPreview = true
		}
	}
	if !seenPreview {
		t.Fatalf("dry-run missing preview install: %#v", dry.OptInCandidates)
	}

	if err := os.RemoveAll(previewOnly); err != nil {
		t.Fatal(err)
	}
	// Leave only execute install.
	_ = executeOnly

	permanent := &recordingPermanentRemover{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})

	if len(permanent.paths) != 1 || permanent.paths[0] != executeOnly {
		t.Fatalf("permanent paths = %v, want only %q", permanent.paths, executeOnly)
	}
	for _, p := range permanent.paths {
		if p == previewOnly {
			t.Fatal("execute trusted dry-run path")
		}
		if p == cacheRoot || filepath.Base(p) == "chrome" {
			t.Fatalf("permanent remover received root or product parent: %q", p)
		}
	}
	if execResult.Totals.OptInDeletedCount != 1 {
		t.Fatalf("opt-in deleted = %d", execResult.Totals.OptInDeletedCount)
	}
	if len(execResult.Deleted) != 1 || execResult.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v, want delete_permanently", execResult.Deleted)
	}
	// History records opted-in path through normal item outcomes when recorder present;
	// non-opted-in paths are never an execution manifest (default execute test above).
}

func TestPuppeteerBrowsers_CapacityDoesNotBlockPermanent(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	install := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "12345678")

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
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
	if len(permanent.paths) != 1 || permanent.paths[0] != install {
		t.Fatalf("permanent paths = %v, want %q (capacity must not block permanent)", permanent.paths, install)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "recycle_bin_capacity" {
			t.Fatalf("permanent candidate skipped for recycle capacity: %#v", skipped)
		}
	}
}

func TestPuppeteerBrowsers_ImpactNoticeAndFrozenDefaults(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	_ = writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "data")

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "Puppeteer") &&
			strings.Contains(notice.Message, "re-download") {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want puppeteer re-download/automation impact", model.Notices)
	}
	// Puppeteer must not expand Default candidates; only Opt-in reclaimable.
	for _, c := range result.Candidates {
		if c.Rule == clean.DevCacheCategoryPuppeteerBrowsers || strings.Contains(c.Path, "puppeteer") {
			t.Fatalf("puppeteer path leaked into default candidates: %#v", c)
		}
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable for puppeteer installs")
	}

	// Without opt-in, no puppeteer candidates even if a resolver is injected.
	defaultResult := clean.DryRun(context.Background(), clean.Options{
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(defaultResult.OptInCandidates) != 0 {
		t.Fatalf("default dry-run opt-in candidates = %#v", defaultResult.OptInCandidates)
	}
}

func TestPuppeteerBrowsers_MissingRootSilent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{missing} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("candidates = %#v, want silent absence", result.OptInCandidates)
	}
}

func TestPuppeteerBrowsers_EmptyRootNoCandidate(t *testing.T) {
	cacheRoot := t.TempDir()
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("candidates = %#v, want none for empty root", result.OptInCandidates)
	}
}

func TestPuppeteerBrowsers_CancellationDuringMeasurement(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "puppeteer")
	first := writePuppeteerInstall(t, cacheRoot, "chrome", "win64-1.0.0", "ab")
	second := writePuppeteerInstall(t, cacheRoot, "firefox", "win64-2.0.0", "cd")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel after first candidate is observed by wrapping through DryRun is hard;
	// use Execute with a canceling context once progress starts is covered by
	// structured seam tests. Here we ensure a pre-canceled context yields no
	// partial adapter calls for puppeteer.
	cancel()
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(ctx, clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryPuppeteerBrowsers},
		DevCachePathResolver:      func(string) []string { return []string{cacheRoot} },
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
	_ = first
	_ = second
}

func TestPuppeteerBrowsers_TUICategoryIdentifierOnly(t *testing.T) {
	// Preview read model lists puppeteer as an unselected opt-in category.
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, cat := range model.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryPuppeteerBrowsers {
			found = true
			if cat.Selected {
				t.Fatal("puppeteer-browsers must start unselected")
			}
		}
	}
	if !found {
		t.Fatalf("OptInCategories missing puppeteer-browsers: %#v", model.OptInCategories)
	}

	// Selection by identifier only surfaces category, not a path manifest.
	selected := clean.NewPreviewReadModelForSelection(result, []string{clean.DevCacheCategoryPuppeteerBrowsers})
	for _, cat := range selected.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryPuppeteerBrowsers && !cat.Selected {
			t.Fatal("expected selected after identifier selection")
		}
	}
}

func TestPuppeteerBrowsers_PublicCatalogPathFree(t *testing.T) {
	summaries := clean.CanonicalCleanupCategoryCatalog().Summaries()
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		"PUPPETEER_CACHE_DIR", "discoverChildren", "resolvePaths",
		`.cache\puppeteer`, "chrome-headless-shell", "win64-",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("catalog exposes private detail %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "puppeteer-browsers") {
		t.Fatalf("catalog missing public identifier: %s", encoded)
	}
	// playwright-browsers is a sibling structured category on main; path-free
	// public identifiers for both are expected.
	if !strings.Contains(encoded, "playwright-browsers") {
		t.Fatalf("catalog missing sibling playwright-browsers identifier: %s", encoded)
	}
}
