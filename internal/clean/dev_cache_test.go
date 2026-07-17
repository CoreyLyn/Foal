package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestNormalizedOptInSet_DevCaches(t *testing.T) {
	t.Run("dev-caches enables all developer-tools opt-in categories", func(t *testing.T) {
		enabled, invalid, valid := clean.NormalizedOptInSet([]string{"dev-caches"})
		if len(invalid) != 0 {
			t.Fatalf("expected no invalid names, got %v", invalid)
		}
		expectedDevCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryPNPM,
			clean.DevCacheCategoryYarn,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryGoModCache,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryNuGetGlobalPackages,
			clean.DevCacheCategoryCorepack,
			clean.DevCacheCategoryUV,
			clean.DevCacheCategoryBun,
			clean.DevCacheCategoryPlaywright,
			clean.DevCacheCategoryPuppeteerBrowsers,
			clean.DevCacheCategoryElectron,
			clean.DevCacheCategoryJetBrainsIDECaches,
			clean.DevCacheCategoryVisualStudioCaches,
			clean.OpportunityCategoryVSCodeCache,
			clean.OpportunityCategoryCursorCache,
		}
		for _, cat := range expectedDevCaches {
			if !enabled[cat] {
				t.Fatalf("expected %q to be enabled by \"dev-caches\"", cat)
			}
		}
		if len(enabled) != 19 {
			t.Fatalf("expected 19 enabled developer-tools categories, got %d", len(enabled))
		}
		// Verify valid names include dev categories and dev-caches
		found := make(map[string]bool)
		for _, name := range valid {
			found[name] = true
		}
		for _, cat := range expectedDevCaches {
			if !found[cat] {
				t.Fatalf("valid names missing expected dev cache category %q", cat)
			}
		}
		if !found["dev-caches"] {
			t.Fatalf("valid names missing \"dev-caches\"")
		}
	})

	t.Run("all enables both opportunity categories and dev caches", func(t *testing.T) {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{"all"})
		if len(invalid) != 0 {
			t.Fatalf("expected no invalid names, got %v", invalid)
		}
		expectedOpportunities := []string{
			clean.OpportunityCategoryUserTemp,
			clean.OpportunityCategoryCrashDumps,
			clean.OpportunityCategoryWindowsErrorReporting,
			clean.OpportunityCategoryExplorerThumbnailCache,
			clean.OpportunityCategoryINetCache,
			clean.OpportunityCategoryD3DShaderCache,
			clean.OpportunityCategoryNVIDIADXCache,
			clean.OpportunityCategoryAMDGPUShaderCaches,
			clean.OpportunityCategoryIntelGPUShaderCache,
			clean.OpportunityCategoryBrowserCache,
			clean.OpportunityCategoryVSCodeCache,
			clean.OpportunityCategoryCursorCache,
		}
		expectedDevCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryPNPM,
			clean.DevCacheCategoryYarn,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryGoModCache,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryNuGetGlobalPackages,
			clean.DevCacheCategoryCorepack,
			clean.DevCacheCategoryUV,
			clean.DevCacheCategoryBun,
			clean.DevCacheCategoryPlaywright,
			clean.DevCacheCategoryPuppeteerBrowsers,
			clean.DevCacheCategoryElectron,
			clean.DevCacheCategoryJetBrainsIDECaches,
			clean.DevCacheCategoryVisualStudioCaches,
		}
		for _, cat := range expectedOpportunities {
			if !enabled[cat] {
				t.Fatalf("expected %q to be enabled by \"all\"", cat)
			}
		}
		for _, cat := range expectedDevCaches {
			if !enabled[cat] {
				t.Fatalf("expected %q to be enabled by \"all\"", cat)
			}
		}
		if len(enabled) != 12+17 {
			t.Fatalf("expected 29 enabled categories (12+17), got %d", len(enabled))
		}
	})

	t.Run("individual dev cache categories work", func(t *testing.T) {
		devCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryPNPM,
			clean.DevCacheCategoryYarn,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryGoModCache,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryNuGetGlobalPackages,
			clean.DevCacheCategoryCorepack,
			clean.DevCacheCategoryUV,
			clean.DevCacheCategoryBun,
			clean.DevCacheCategoryPlaywright,
			clean.DevCacheCategoryPuppeteerBrowsers,
			clean.DevCacheCategoryElectron,
			clean.DevCacheCategoryJetBrainsIDECaches,
			clean.DevCacheCategoryVisualStudioCaches,
		}
		for _, cat := range devCaches {
			t.Run(cat, func(t *testing.T) {
				enabled, invalid, _ := clean.NormalizedOptInSet([]string{cat})
				if len(invalid) != 0 {
					t.Fatalf("expected no invalid names for %q, got %v", cat, invalid)
				}
				if len(enabled) != 1 {
					t.Fatalf("expected exactly 1 enabled category for %q, got %d", cat, len(enabled))
				}
				if !enabled[cat] {
					t.Fatalf("expected %q to be enabled", cat)
				}
			})
		}
	})
}

func TestDryRun_OptInDevCaches(t *testing.T) {
	t.Run("dev cache appears as opt-in candidate when opted in", func(t *testing.T) {
		root := t.TempDir()
		cachePath := filepath.Join(root, "npm-cache")
		if err := os.Mkdir(cachePath, 0700); err != nil {
			t.Fatal(err)
		}
		testFile := filepath.Join(cachePath, "data.bin")
		if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                 []string{"npm-cache"},
			DevCachePathResolver:  fakeResolver,
			DiscoverOpportunities: noOpportunities,
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				return []clean.ReviewSuggestion{{
					Tool:      "npm",
					Label:     "npm cache",
					Command:   "npm cache clean --force",
					CachePath: cachePath,
				}}
			},
		})

		if len(result.OptInCandidates) != 1 {
			t.Fatalf("expected 1 opt-in candidate, got %d", len(result.OptInCandidates))
		}
		candidate := result.OptInCandidates[0]
		if candidate.Path != cachePath {
			t.Fatalf("expected path %q, got %q", cachePath, candidate.Path)
		}
		if candidate.Category != clean.DevCacheCategoryNPM {
			t.Fatalf("expected category %q, got %q", clean.DevCacheCategoryNPM, candidate.Category)
		}
		if candidate.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("expected planned action delete_permanently, got %q", candidate.PlannedAction)
		}
		if candidate.Bytes != 4 {
			t.Fatalf("expected 4 bytes, got %d", candidate.Bytes)
		}
	})

	t.Run("dev-caches enables all developer-cache categories", func(t *testing.T) {
		root := t.TempDir()
		// Isolate JetBrains product-scoped resolution from the real user profile.
		t.Setenv("LOCALAPPDATA", t.TempDir())
		cachePaths := make(map[string]string)
		wholeRootCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryPNPM,
			clean.DevCacheCategoryYarn,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryGoModCache,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryNuGetGlobalPackages,
			clean.DevCacheCategoryCorepack,
			clean.DevCacheCategoryUV,
			clean.DevCacheCategoryBun,
			clean.DevCacheCategoryElectron,
		}
		for _, cat := range wholeRootCaches {
			cachePath := filepath.Join(root, cat)
			if err := os.Mkdir(cachePath, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(cachePath, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			cachePaths[cat] = cachePath
		}
		// Playwright needs complete allowlisted revision children with INSTALLATION_COMPLETE.
		playwrightRoot := filepath.Join(root, "ms-playwright")
		pwInstall := filepath.Join(playwrightRoot, "chromium-1")
		if err := os.MkdirAll(pwInstall, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pwInstall, "INSTALLATION_COMPLETE"), []byte("ok"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pwInstall, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		cachePaths[clean.DevCacheCategoryPlaywright] = playwrightRoot
		// Puppeteer needs structured product/platform-version children.
		puppeteerRoot := filepath.Join(root, "puppeteer")
		install := filepath.Join(puppeteerRoot, "chrome", "win64-1.0.0")
		if err := os.MkdirAll(install, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(install, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		cachePaths[clean.DevCacheCategoryPuppeteerBrowsers] = puppeteerRoot

		fakeResolver := func(category string) []string {
			if path, ok := cachePaths[category]; ok {
				return []string{path}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"dev-caches"},
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		// 13 whole-root + 1 playwright install child + 1 puppeteer install child
		// (vscode/cursor need detector).
		if len(result.OptInCandidates) != 15 {
			t.Fatalf("expected 15 opt-in candidates, got %d: %#v", len(result.OptInCandidates), result.OptInCandidates)
		}
	})

	t.Run("empty path from resolver skips the dev cache", func(t *testing.T) {
		// Isolate product-scoped JetBrains roots (catalog resolveRootScopes).
		t.Setenv("LOCALAPPDATA", t.TempDir())
		fakeResolver := func(category string) []string {
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"dev-caches"},
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 0 {
			t.Fatalf("expected 0 opt-in candidates with empty paths, got %d", len(result.OptInCandidates))
		}
	})

	t.Run("protected path skips the dev cache", func(t *testing.T) {
		root := t.TempDir()
		protectedPath := filepath.Join(root, "protected-cache")
		if err := os.Mkdir(protectedPath, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(protectedPath, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{protectedPath}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			Validator:                 pathsafe.NewValidator([]string{protectedPath}),
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 0 {
			t.Fatalf("expected 0 opt-in candidates for protected path, got %d", len(result.OptInCandidates))
		}
	})

	t.Run("multiple roots for single category", func(t *testing.T) {
		root := t.TempDir()
		cachePath1 := filepath.Join(root, "npm-cache-1")
		cachePath2 := filepath.Join(root, "npm-cache-2")
		for _, path := range []string{cachePath1, cachePath2} {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(path, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath1, cachePath2}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 2 {
			t.Fatalf("expected 2 opt-in candidates, got %d", len(result.OptInCandidates))
		}
		seen := make(map[string]bool)
		for _, c := range result.OptInCandidates {
			if c.Category != clean.DevCacheCategoryNPM {
				t.Errorf("unexpected category: %q", c.Category)
			}
			seen[c.Path] = true
		}
		if !seen[cachePath1] || !seen[cachePath2] {
			t.Errorf("expected both paths to be present, got: %v", seen)
		}
	})

	t.Run("duplicate roots are deduplicated", func(t *testing.T) {
		root := t.TempDir()
		cachePath := filepath.Join(root, "npm-cache")
		if err := os.Mkdir(cachePath, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				// Return same path twice with different casing/slashes
				return []string{
					cachePath,
					filepath.Join(root, "npm-cache"),
				}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 {
			t.Fatalf("expected 1 opt-in candidate (deduplicated), got %d", len(result.OptInCandidates))
		}
		if result.OptInCandidates[0].Path != cachePath {
			t.Errorf("unexpected path: %q", result.OptInCandidates[0].Path)
		}
	})

	t.Run("one protected root doesn't block other roots", func(t *testing.T) {
		root := t.TempDir()
		protectedPath := filepath.Join(root, "protected")
		allowedPath := filepath.Join(root, "allowed")
		for _, path := range []string{protectedPath, allowedPath} {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(path, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{protectedPath, allowedPath}
			}
			return nil
		}

		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			Validator:                 pathsafe.NewValidator([]string{protectedPath}),
			DevCachePathResolver:      fakeResolver,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 {
			t.Fatalf("expected 1 opt-in candidate (protected path skipped), got %d", len(result.OptInCandidates))
		}
		if result.OptInCandidates[0].Path != allowedPath {
			t.Errorf("expected allowed path %q, got %q", allowedPath, result.OptInCandidates[0].Path)
		}
	})
}

func TestExecute_OptInDevCaches(t *testing.T) {
	t.Run("dev cache executes via permanent removal when opted in and authorized", func(t *testing.T) {
		root := t.TempDir()
		cachePath := filepath.Join(root, "npm-cache")
		if err := os.Mkdir(cachePath, 0700); err != nil {
			t.Fatal(err)
		}
		testFile := filepath.Join(cachePath, "data.bin")
		if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		}

		recycle := &recordingRecycleBinAdapter{}
		permanent := &recordingPermanentRemover{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion: true,
			OptIn:                  []string{"npm-cache"},
			DevCachePathResolver:   fakeResolver,
			RecycleBinAdapter:      recycle,
			PermanentRemover:       permanent,
			DiscoverOpportunities:  noOpportunities,
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				return nil
			},
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		if len(recycle.paths) != 0 {
			t.Fatalf("npm-cache must not use Recycle Bin: %v", recycle.paths)
		}
		if len(permanent.paths) != 1 || permanent.paths[0] != cachePath {
			t.Fatalf("permanent paths = %v, want [%q]", permanent.paths, cachePath)
		}

		// Verify deleted item is marked as opt-in permanent
		if result.Totals.OptInDeletedCount != 1 {
			t.Fatalf("expected OptInDeletedCount 1, got %d", result.Totals.OptInDeletedCount)
		}
		foundDeleted := false
		for _, d := range result.Deleted {
			if d.Path == cachePath {
				foundDeleted = true
				if !d.IsOptIn {
					t.Fatalf("expected deleted item to be marked IsOptIn")
				}
				if d.Rule != clean.DevCacheCategoryNPM {
					t.Fatalf("expected deleted item rule to be %q, got %q", clean.DevCacheCategoryNPM, d.Rule)
				}
				if d.Action != string(clean.DeletionActionDeletePermanently) {
					t.Fatalf("expected permanent action, got %q", d.Action)
				}
			}
		}
		if !foundDeleted {
			t.Fatalf("expected deleted items to include %q", cachePath)
		}
	})

	t.Run("dev-caches enables all developer-cache categories for execute", func(t *testing.T) {
		root := t.TempDir()
		// Isolate JetBrains product-scoped resolution from the real user profile.
		t.Setenv("LOCALAPPDATA", t.TempDir())
		cachePaths := make(map[string]string)
		// #221+#222: all registered developer-cache whole-root categories are permanent.
		wholeRootCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryPNPM,
			clean.DevCacheCategoryYarn,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryGoModCache,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryNuGetGlobalPackages,
			clean.DevCacheCategoryCorepack,
			clean.DevCacheCategoryUV,
			clean.DevCacheCategoryBun,
			clean.DevCacheCategoryElectron,
		}
		for _, cat := range wholeRootCaches {
			cachePath := filepath.Join(root, cat)
			if err := os.Mkdir(cachePath, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(cachePath, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			cachePaths[cat] = cachePath
		}

		playwrightRoot := filepath.Join(root, "ms-playwright")
		pwInstall := filepath.Join(playwrightRoot, "chromium-1")
		if err := os.MkdirAll(pwInstall, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pwInstall, "INSTALLATION_COMPLETE"), []byte("ok"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pwInstall, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		cachePaths[clean.DevCacheCategoryPlaywright] = playwrightRoot
		puppeteerRoot := filepath.Join(root, "puppeteer")
		install := filepath.Join(puppeteerRoot, "chrome", "win64-1.0.0")
		if err := os.MkdirAll(install, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(install, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		cachePaths[clean.DevCacheCategoryPuppeteerBrowsers] = puppeteerRoot

		fakeResolver := func(category string) []string {
			if path, ok := cachePaths[category]; ok {
				return []string{path}
			}
			return nil
		}

		recycle := &recordingRecycleBinAdapter{}
		permanent := &recordingPermanentRemover{}

		// Distinctive-process package caches need idle snapshots; shared-runtime
		// (npm/pip/corepack/electron/playwright/puppeteer) do not.
		idleDetector := func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationDotNet, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationNuGet, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationUV, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationBun, State: clean.RunningApplicationStateIdle},
			}
		}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion:    true,
			OptIn:                     []string{"dev-caches"},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			RecycleBinAdapter:         recycle,
			PermanentRemover:          permanent,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// All 15 discovered developer-cache candidates are permanent
		// (12 package/build whole-roots + electron + playwright install + puppeteer install).
		if len(recycle.paths) != 0 {
			t.Fatalf("recycle paths = %d: %v, want none (all dev caches permanent)", len(recycle.paths), recycle.paths)
		}
		if len(permanent.paths) != 15 {
			t.Fatalf("permanent paths = %d: %v, want 15", len(permanent.paths), permanent.paths)
		}
		if result.Totals.OptInDeletedCount != 15 {
			t.Fatalf("expected OptInDeletedCount 15, got %d", result.Totals.OptInDeletedCount)
		}
		if result.Totals.PermanentlyDeletedBytes == 0 {
			t.Fatal("expected non-zero permanently_deleted_bytes for permanent dev caches")
		}
		if result.Totals.RecycleBinMovedBytes != 0 {
			t.Fatalf("recycle_bin_moved_bytes = %d, want 0", result.Totals.RecycleBinMovedBytes)
		}
	})

	t.Run("empty path from resolver skips execute", func(t *testing.T) {
		// Isolate product-scoped JetBrains roots (catalog resolveRootScopes).
		t.Setenv("LOCALAPPDATA", t.TempDir())

		fakeResolver := func(category string) []string {
			return nil
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"dev-caches"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		if len(adapter.paths) != 0 {
			t.Fatalf("expected adapter to receive 0 paths, got %v", adapter.paths)
		}
		if result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("expected OptInDeletedCount 0, got %d", result.Totals.OptInDeletedCount)
		}
	})

	t.Run("no opt-in means no dev cache execution", func(t *testing.T) {
		root := t.TempDir()
		cachePath := filepath.Join(root, "npm-cache")
		if err := os.Mkdir(cachePath, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			return []string{cachePath}
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{}, // No opt-in
			DevCachePathResolver:      fakeResolver,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		if len(adapter.paths) != 0 {
			t.Fatalf("expected adapter to receive 0 paths without opt-in, got %v", adapter.paths)
		}
		if result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("expected OptInDeletedCount 0, got %d", result.Totals.OptInDeletedCount)
		}
	})

	t.Run("multiple roots for single category in execute", func(t *testing.T) {
		root := t.TempDir()
		cachePath1 := filepath.Join(root, "npm-cache-1")
		cachePath2 := filepath.Join(root, "npm-cache-2")
		for _, path := range []string{cachePath1, cachePath2} {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(path, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath1, cachePath2}
			}
			return nil
		}

		recycle := &recordingRecycleBinAdapter{}
		permanent := &recordingPermanentRemover{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion:    true,
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
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
			t.Fatalf("npm multi-root must not use Recycle Bin: %v", recycle.paths)
		}
		if len(permanent.paths) != 2 {
			t.Fatalf("expected 2 permanent paths, got %d: %v", len(permanent.paths), permanent.paths)
		}
		if result.Totals.OptInDeletedCount != 2 {
			t.Fatalf("expected OptInDeletedCount 2, got %d", result.Totals.OptInDeletedCount)
		}
	})

	t.Run("one protected root doesn't block other roots", func(t *testing.T) {
		root := t.TempDir()
		skippedPath := filepath.Join(root, "skipped")
		allowedPath := filepath.Join(root, "allowed")
		for _, path := range []string{skippedPath, allowedPath} {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(path, "data.bin")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{skippedPath, allowedPath}
			}
			return nil
		}

		// Protection suppresses one root; permanent capacity is not used for package caches.
		recycle := &recordingRecycleBinAdapter{}
		permanent := &recordingPermanentRemover{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion:    true,
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			Validator:                 pathsafe.NewValidator([]string{skippedPath}),
			RecycleBinAdapter:         recycle,
			PermanentRemover:          permanent,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify only allowed path was executed permanently
		if len(recycle.paths) != 0 {
			t.Fatalf("npm must not use Recycle Bin: %v", recycle.paths)
		}
		if len(permanent.paths) != 1 || permanent.paths[0] != allowedPath {
			t.Fatalf("expected only allowed path to be executed, got %v", permanent.paths)
		}
		// Verify one deleted; protected root does not become a candidate
		if result.Totals.OptInDeletedCount != 1 {
			t.Fatalf("expected OptInDeletedCount 1, got %d", result.Totals.OptInDeletedCount)
		}
	})
}

func noOpportunities(context.Context) clean.OpportunityDiscoveryResult {
	return clean.OpportunityDiscoveryResult{}
}
