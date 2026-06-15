package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestDiscoverOpportunitiesCategorizesUserTempAndCurrentUserWindowsCaches(t *testing.T) {
	tempRoot := t.TempDir()
	localAppData := t.TempDir()
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	userTempPath := fileWithModification(t, tempRoot, "idle.tmp", "idle", now.Add(-8*24*time.Hour))
	cacheRoots := []struct {
		category string
		path     string
		file     string
		contents string
	}{
		{clean.OpportunityCategoryCrashDumps, filepath.Join(localAppData, "CrashDumps"), "app.dmp", "crash"},
		{clean.OpportunityCategoryWindowsErrorReporting, filepath.Join(localAppData, "Microsoft", "Windows", "WER"), "report.wer", "wer"},
		{clean.OpportunityCategoryExplorerThumbnailCache, filepath.Join(localAppData, "Microsoft", "Windows", "Explorer"), "thumbcache.db", "thumb"},
		{clean.OpportunityCategoryINetCache, filepath.Join(localAppData, "Microsoft", "Windows", "INetCache"), "cache.dat", "inet"},
		{clean.OpportunityCategoryD3DShaderCache, filepath.Join(localAppData, "D3DSCache"), "shader.bin", "d3d"},
		{clean.OpportunityCategoryNVIDIADXCache, filepath.Join(localAppData, "NVIDIA", "DXCache"), "shader.bin", "nvidia"},
	}
	cacheFiles := make([]string, 0, len(cacheRoots))
	for _, cache := range cacheRoots {
		if err := os.MkdirAll(cache.path, 0700); err != nil {
			t.Fatal(err)
		}
		cacheFiles = append(cacheFiles, fileWithModification(t, cache.path, cache.file, cache.contents, now))
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         tempRoot,
		LocalAppDataDir: localAppData,
		Now:             now,
	})

	if len(result.Opportunities) != 1+len(cacheRoots) {
		t.Fatalf("opportunities = %#v, want user temp and six current-user Windows cache roots", result.Opportunities)
	}
	userTemp := result.Opportunities[0]
	if userTemp.Category != clean.OpportunityCategoryUserTemp || userTemp.Path != userTempPath ||
		!userTemp.LatestModifiedAt.Equal(now.Add(-8*24*time.Hour)) || userTemp.IdleDays != 8 {
		t.Fatalf("user temp opportunity = %#v, want categorized age-observed result", userTemp)
	}
	for index, cache := range cacheRoots {
		opportunity := result.Opportunities[index+1]
		if opportunity.Category != cache.category || opportunity.Path != cache.path ||
			opportunity.Bytes != int64(len(cache.contents)) || !opportunity.LatestModifiedAt.IsZero() || opportunity.IdleDays != 0 {
			t.Fatalf("%s opportunity = %#v, want whole-root existence-observed result", cache.category, opportunity)
		}
		encoded, err := json.Marshal(opportunity)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "latest_modified_at") || strings.Contains(string(encoded), "idle_days") {
			t.Fatalf("%s JSON emits misleading age fields: %s", cache.category, encoded)
		}
		if _, err := os.Stat(cacheFiles[index]); err != nil {
			t.Fatalf("discovery changed %s file %q: %v", cache.category, cacheFiles[index], err)
		}
	}
}

func TestDiscoverOpportunitiesOmitsMissingCurrentUserWindowsCacheRootsWithoutIncompleteResult(t *testing.T) {
	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: t.TempDir(),
	})

	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want missing current-user Windows cache roots omitted without inspection errors", result)
	}
}

func TestDiscoverUserTempOpportunitiesIncludesFileAtSevenDayBoundary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "idle.tmp")
	if err := os.WriteFile(path, []byte("idle"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	modified := now.Add(-7 * 24 * time.Hour)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverUserTempOpportunities(context.Background(), clean.UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one", result.Opportunities)
	}
	got := result.Opportunities[0]
	if got.Category != clean.OpportunityCategoryUserTemp || got.Path != path || got.Bytes != 4 ||
		!got.LatestModifiedAt.Equal(modified) || got.IdleDays != 7 {
		t.Fatalf("opportunity = %#v, want path/bytes/latest modification/idle days", got)
	}
	if got.Status != "skipped_by_default" || got.Reason != "requires_explicit_opt_in" {
		t.Fatalf("status/reason = %q/%q, want fixed skipped-by-default values", got.Status, got.Reason)
	}
	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v, want none", result.Incomplete)
	}
}

func TestDiscoverUserTempOpportunitiesMeasuresDirectoryAndNestedLatestModification(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "idle-cache")
	nested := filepath.Join(directory, "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(directory, "first.bin")
	latest := filepath.Join(nested, "latest.bin")
	if err := os.WriteFile(first, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(latest, []byte("latest"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-10 * 24 * time.Hour)
	latestModified := now.Add(-8 * 24 * time.Hour)
	for _, path := range []string{directory, nested, first} {
		if err := os.Chtimes(path, oldest, oldest); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(latest, latestModified, latestModified); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverUserTempOpportunities(context.Background(), clean.UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	})

	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want one directory", result.Opportunities)
	}
	got := result.Opportunities[0]
	if got.Path != directory || got.Bytes != int64(len("first")+len("latest")) {
		t.Fatalf("opportunity path/bytes = %q/%d, want directory and descendant bytes", got.Path, got.Bytes)
	}
	if !got.LatestModifiedAt.Equal(latestModified) || got.IdleDays != 8 {
		t.Fatalf("latest modification/idle days = %v/%d, want nested latest modification and 8 days", got.LatestModifiedAt, got.IdleDays)
	}
}

func TestDiscoverUserTempOpportunitiesExcludesRecentAndFoalOwnedEntries(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	paths := []string{
		fileWithModification(t, root, "recent.tmp", "recent", now.Add(-7*24*time.Hour+time.Second)),
		fileWithModification(t, root, "foal-owned.tmp", "owned", now.Add(-30*24*time.Hour)),
		fileWithModification(t, root, "Foal-Owned.tmp", "owned", now.Add(-30*24*time.Hour)),
	}

	result := clean.DiscoverUserTempOpportunities(context.Background(), clean.UserTempDiscoveryOptions{
		TempDir: root,
		Now:     now,
	})

	if len(result.Opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want recent and Foal-owned entries excluded", result.Opportunities)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("discovery changed %q: %v", path, err)
		}
	}
}

func TestDiscoverUserTempOpportunitiesReportsCancellationAsIncomplete(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := clean.DiscoverUserTempOpportunities(ctx, clean.UserTempDiscoveryOptions{
		TempDir: root,
	})

	if len(result.Opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want none", result.Opportunities)
	}
	if len(result.Incomplete) != 1 {
		t.Fatalf("incomplete = %#v, want cancellation result", result.Incomplete)
	}
	incomplete := result.Incomplete[0]
	if incomplete.Path != root || incomplete.Reason.Code != "context_canceled" || !incomplete.Reason.Recoverable {
		t.Fatalf("incomplete = %#v, want recoverable context_canceled", incomplete)
	}
}

func fileWithModification(t *testing.T, root, name, contents string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return path
}
