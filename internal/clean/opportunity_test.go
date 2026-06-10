package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

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
	if got.Path != path || got.Bytes != 4 || !got.LatestModifiedAt.Equal(modified) || got.IdleDays != 7 {
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
