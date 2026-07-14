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
	t.Run("dev-caches enables all 6 dev cache categories", func(t *testing.T) {
		enabled, invalid, valid := clean.NormalizedOptInSet([]string{"dev-caches"})
		if len(invalid) != 0 {
			t.Fatalf("expected no invalid names, got %v", invalid)
		}
		expectedDevCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryCorepack,
		}
		for _, cat := range expectedDevCaches {
			if !enabled[cat] {
				t.Fatalf("expected %q to be enabled by \"dev-caches\"", cat)
			}
		}
		if len(enabled) != 6 {
			t.Fatalf("expected 6 enabled dev cache categories, got %d", len(enabled))
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
			clean.OpportunityCategoryBrowserCache,
		}
		expectedDevCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryCorepack,
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
		if len(enabled) != 8+6 {
			t.Fatalf("expected 14 enabled categories (8+6), got %d", len(enabled))
		}
	})

	t.Run("individual dev cache categories work", func(t *testing.T) {
		devCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryCorepack,
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
		if candidate.PlannedAction != "move_to_recycle_bin" {
			t.Fatalf("expected planned action move_to_recycle_bin, got %q", candidate.PlannedAction)
		}
		if candidate.Bytes != 4 {
			t.Fatalf("expected 4 bytes, got %d", candidate.Bytes)
		}
	})

	t.Run("dev-caches enables all 6 dev caches", func(t *testing.T) {
		root := t.TempDir()
		cachePaths := make(map[string]string)
		devCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryCorepack,
		}
		for _, cat := range devCaches {
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

		if len(result.OptInCandidates) != 6 {
			t.Fatalf("expected 6 opt-in candidates, got %d", len(result.OptInCandidates))
		}
	})

	t.Run("empty path from resolver skips the dev cache", func(t *testing.T) {
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
	t.Run("dev cache executes via Recycle Bin when opted in", func(t *testing.T) {
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

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                 []string{"npm-cache"},
			DevCachePathResolver:  fakeResolver,
			RecycleBinAdapter:     adapter,
			DiscoverOpportunities: noOpportunities,
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				return nil
			},
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify the path was sent to Recycle Bin
		found := false
		for _, p := range adapter.paths {
			if p == cachePath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected adapter to receive %q, got %v", cachePath, adapter.paths)
		}

		// Verify deleted item is marked as opt-in
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
			}
		}
		if !foundDeleted {
			t.Fatalf("expected deleted items to include %q", cachePath)
		}
	})

	t.Run("dev-caches enables all 6 dev caches for execute", func(t *testing.T) {
		root := t.TempDir()
		cachePaths := make(map[string]string)
		devCaches := []string{
			clean.DevCacheCategoryNPM,
			clean.DevCacheCategoryGo,
			clean.DevCacheCategoryPip,
			clean.DevCacheCategoryCargo,
			clean.DevCacheCategoryNuGet,
			clean.DevCacheCategoryCorepack,
		}
		for _, cat := range devCaches {
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

		fakeResolver := func(category string) []string {
			if path, ok := cachePaths[category]; ok {
				return []string{path}
			}
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

		// Verify all 6 paths were sent to Recycle Bin
		if len(adapter.paths) != 6 {
			t.Fatalf("expected 6 paths in adapter, got %d: %v", len(adapter.paths), adapter.paths)
		}

		if result.Totals.OptInDeletedCount != 6 {
			t.Fatalf("expected OptInDeletedCount 6, got %d", result.Totals.OptInDeletedCount)
		}
	})

	t.Run("capacity pre-check applies to dev caches", func(t *testing.T) {
		root := t.TempDir()
		cachePath := filepath.Join(root, "npm-cache")
		if err := os.Mkdir(cachePath, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cachePath, "large.bin"), make([]byte, 100), 0600); err != nil {
			t.Fatal(err)
		}

		fakeResolver := func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		}

		// Fake probe that returns very low capacity
		fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       filepath.VolumeName(path),
				NukeOnDelete: false,
				MaxCapacity:  50, // Too small for our 100-byte file
			}, nil
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinCapacityProbe:   fakeProbe,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify adapter didn't get the path (capacity check failed)
		if len(adapter.paths) != 0 {
			t.Fatalf("expected adapter to receive 0 paths, got %v", adapter.paths)
		}

		// Verify it's in skipped with the right reason
		if len(result.Skipped) != 1 {
			t.Fatalf("expected 1 skipped item, got %d", len(result.Skipped))
		}
		if result.Skipped[0].Reason.Code != "recycle_bin_capacity" {
			t.Fatalf("expected reason code recycle_bin_capacity, got %q", result.Skipped[0].Reason.Code)
		}
	})

	t.Run("probe error fail-closed for dev caches", func(t *testing.T) {
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
				return []string{cachePath}
			}
			return nil
		}

		// Fake probe that returns error
		fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: filepath.VolumeName(path)}, os.ErrNotExist
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinCapacityProbe:   fakeProbe,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify adapter didn't get the path (fail-closed)
		if len(adapter.paths) != 0 {
			t.Fatalf("expected adapter to receive 0 paths, got %v", adapter.paths)
		}

		// Verify it's in skipped with the right reason
		if len(result.Skipped) != 1 {
			t.Fatalf("expected 1 skipped item, got %d", len(result.Skipped))
		}
		if result.Skipped[0].Reason.Code != "recycle_bin_capacity_probe_failed" {
			t.Fatalf("expected reason code recycle_bin_capacity_probe_failed, got %q", result.Skipped[0].Reason.Code)
		}
	})

	t.Run("recycle bin disabled skips dev caches", func(t *testing.T) {
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
				return []string{cachePath}
			}
			return nil
		}

		// Fake probe that returns NukeOnDelete true
		fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       filepath.VolumeName(path),
				NukeOnDelete: true,
				MaxCapacity:  1000,
			}, nil
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinCapacityProbe:   fakeProbe,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify adapter didn't get the path
		if len(adapter.paths) != 0 {
			t.Fatalf("expected adapter to receive 0 paths, got %v", adapter.paths)
		}

		if len(result.Skipped) != 1 {
			t.Fatalf("expected 1 skipped item, got %d", len(result.Skipped))
		}
		if result.Skipped[0].Reason.Code != "recycle_bin_disabled" {
			t.Fatalf("expected reason code recycle_bin_disabled, got %q", result.Skipped[0].Reason.Code)
		}
	})

	t.Run("empty path from resolver skips execute", func(t *testing.T) {
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

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		if len(adapter.paths) != 2 {
			t.Fatalf("expected 2 paths in adapter, got %d: %v", len(adapter.paths), adapter.paths)
		}
		if result.Totals.OptInDeletedCount != 2 {
			t.Fatalf("expected OptInDeletedCount 2, got %d", result.Totals.OptInDeletedCount)
		}
	})

	t.Run("one skipped root doesn't block other roots", func(t *testing.T) {
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

		// Fake probe that fails only for skippedPath
		fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
			if path == skippedPath {
				return clean.RecycleBinVolumeConfig{}, os.ErrNotExist
			}
			return clean.RecycleBinVolumeConfig{
				Volume:       filepath.VolumeName(path),
				NukeOnDelete: false,
				MaxCapacity:  1000,
			}, nil
		}

		adapter := &recordingRecycleBinAdapter{}

		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{"npm-cache"},
			DevCachePathResolver:      fakeResolver,
			RecycleBinCapacityProbe:   fakeProbe,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "test_rule",
				DefaultEnabled: false,
			}},
		})

		// Verify only allowed path was executed
		if len(adapter.paths) != 1 || adapter.paths[0] != allowedPath {
			t.Fatalf("expected only allowed path to be executed, got %v", adapter.paths)
		}
		// Verify one deleted, one skipped
		if result.Totals.OptInDeletedCount != 1 {
			t.Fatalf("expected OptInDeletedCount 1, got %d", result.Totals.OptInDeletedCount)
		}
		if len(result.Skipped) != 1 || result.Skipped[0].Path != skippedPath {
			t.Fatalf("expected skipped path to be present")
		}
	})
}


func noOpportunities(context.Context) clean.OpportunityDiscoveryResult {
	return clean.OpportunityDiscoveryResult{}
}
