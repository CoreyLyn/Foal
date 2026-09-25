package clean_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func quietRule(root string) clean.Rule {
	return clean.Rule{
		ID:                    clean.DefaultCategoryFoalOwnedTempSandboxes,
		Description:           "Foal-owned temporary sandbox entries",
		DefaultEnabled:        true,
		Roots:                 []string{root},
		CandidateNamePrefixes: []string{"foal-"},
		MinimumQuietPeriod:    24 * time.Hour,
	}
}

// setTreeTimes stamps every entry under path (deepest first, then path itself)
// so directory timestamps are not refreshed by later child writes.
func setTreeTimes(t *testing.T, path string, when time.Time) {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(path, func(current string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, current)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, current := range paths {
		if err := os.Chtimes(current, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

func makeSandbox(t *testing.T, root, name string, when time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "data.tmp"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	setTreeTimes(t, dir, when)
	return dir
}

func TestDefaultRuleCatalogGatesFoalOwnedTempSandboxesOnQuietPeriod(t *testing.T) {
	rules := clean.DefaultRuleCatalog()
	if len(rules) != 1 || rules[0].ID != clean.DefaultCategoryFoalOwnedTempSandboxes {
		t.Fatalf("default rules = %#v, want only foal_owned_temp_sandboxes", rules)
	}
	if rules[0].MinimumQuietPeriod != 24*time.Hour {
		t.Fatalf("quiet period = %s, want 24h", rules[0].MinimumQuietPeriod)
	}
}

func TestDryRunQuietPeriodSkipsRecentFoalOwnedEntries(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	quiet := makeSandbox(t, root, "foal-quiet", now.Add(-48*time.Hour))
	recent := makeSandbox(t, root, "foal-recent", now.Add(-time.Hour))

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules:                         []clean.Rule{quietRule(root)},
		DefaultRuleNow:                func() time.Time { return now },
	})

	if len(result.Candidates) != 1 || result.Candidates[0].Path != quiet {
		t.Fatalf("candidates = %#v, want only the quiet sandbox", result.Candidates)
	}
	if result.Candidates[0].Bytes != 4 {
		t.Fatalf("candidate bytes = %d, want 4", result.Candidates[0].Bytes)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Path != recent {
		t.Fatalf("skipped = %#v, want the recent sandbox", result.Skipped)
	}
	reason := result.Skipped[0].Reason
	if reason.Code != clean.PreviewReasonFoalOwnedTempRecent || !reason.Recoverable {
		t.Fatalf("skip reason = %#v, want recoverable %s", reason, clean.PreviewReasonFoalOwnedTempRecent)
	}
	if result.Skipped[0].PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("skip planned action = %q, want move_to_recycle_bin", result.Skipped[0].PlannedAction)
	}
}

func TestDryRunQuietPeriodUsesDeepestModification(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	dir := makeSandbox(t, root, "foal-deep", now.Add(-48*time.Hour))
	// Only a nested file is fresh; the top-level directory stays old.
	nested := filepath.Join(dir, "nested", "data.tmp")
	if err := os.Chtimes(nested, now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules:                         []clean.Rule{quietRule(root)},
		DefaultRuleNow:                func() time.Time { return now },
	})

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none while a nested file is recent", result.Candidates)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != clean.PreviewReasonFoalOwnedTempRecent {
		t.Fatalf("skipped = %#v, want one foal_owned_temp_recent skip", result.Skipped)
	}
}

func TestDryRunQuietPeriodSkipsFutureTimestamps(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	makeSandbox(t, root, "foal-future", now.Add(time.Hour))

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules:                         []clean.Rule{quietRule(root)},
		DefaultRuleNow:                func() time.Time { return now },
	})

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none for a future timestamp", result.Candidates)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != clean.PreviewReasonFoalOwnedTempRecent {
		t.Fatalf("skipped = %#v, want one foal_owned_temp_recent skip", result.Skipped)
	}
}

func TestDryRunRuleWithoutQuietPeriodStaysUngated(t *testing.T) {
	root := t.TempDir()
	fresh := makeSandbox(t, root, "foal-fresh", time.Now())
	rule := quietRule(root)
	rule.MinimumQuietPeriod = 0

	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules:                         []clean.Rule{rule},
	})

	if len(result.Candidates) != 1 || result.Candidates[0].Path != fresh {
		t.Fatalf("candidates = %#v, want the fresh entry when no quiet period is set", result.Candidates)
	}
}

func TestExecuteQuietPeriodMovesQuietEntries(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	quiet := makeSandbox(t, root, "foal-quiet", now.Add(-48*time.Hour))

	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		Rules:             []clean.Rule{quietRule(root)},
		RecycleBinAdapter: adapter,
		DefaultRuleNow:    func() time.Time { return now },
	})

	if len(adapter.paths) != 1 || adapter.paths[0] != quiet {
		t.Fatalf("moved = %v, want the quiet sandbox", adapter.paths)
	}
	if len(result.Deleted) != 1 {
		t.Fatalf("deleted = %#v, want one moved sandbox", result.Deleted)
	}
}

func TestExecuteQuietPeriodRechecksImmediatelyBeforeMove(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	sandbox := makeSandbox(t, root, "foal-touched", now.Add(-48*time.Hour))

	adapter := &recordingRecycleBinAdapter{}
	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{quietRule(root)},
		RecycleBinAdapter: adapter,
		DefaultRuleNow:    func() time.Time { return now },
		// The capacity probe runs after fresh resolution and before the move, so
		// a write here models activity between preview-quality resolution and
		// mutation.
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			if err := os.Chtimes(filepath.Join(sandbox, "nested", "data.tmp"), now, now); err != nil {
				t.Fatal(err)
			}
			return clean.RecycleBinVolumeConfig{Volume: filepath.VolumeName(path), MaxCapacity: 1 << 60}, nil
		},
	})

	if len(adapter.paths) != 0 {
		t.Fatalf("moved = %v, want no move after a fresh write", adapter.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != clean.PreviewReasonFoalOwnedTempRecent {
		t.Fatalf("skipped = %#v, want one foal_owned_temp_recent pre-mutation skip", result.Skipped)
	}
	if _, err := os.Stat(sandbox); err != nil {
		t.Fatalf("sandbox should remain on disk: %v", err)
	}
}
