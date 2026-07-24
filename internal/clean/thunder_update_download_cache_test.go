package clean_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// thunderStableTime is a fixed clock so stability-window math is deterministic.
var thunderStableTime = time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)

func daysAgo(now time.Time, days int) time.Time {
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

// chtimesTree stamps every entry under root with the same mtime. Chtimes never
// re-touches a parent's mtime, so call order does not matter once the tree is
// fully materialized.
func chtimesTree(t *testing.T, root string, mtime time.Time) {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

// thunderActivity builds a fixed DetectActivity stub returning the given status.
func thunderActivity(status clean.FixedRootActivityStatus) func(context.Context) clean.FixedRootActivityState {
	return func(context.Context) clean.FixedRootActivityState {
		return clean.FixedRootActivityState{Status: status}
	}
}

// thunderOpts assembles the shared DryRun options with the injected root, clock,
// and Thunder activity detector. DetectRunningApplications is intentionally left
// nil: the thunder-update-download resolver self-gates on DetectActivity only.
func thunderOpts(root string, now time.Time, detect func(context.Context) clean.FixedRootActivityState) clean.Options {
	return clean.Options{
		OptIn: []string{clean.CategoryThunderUpdateDownload},
		FixedRootDiscoveryOptions: clean.FixedRootDiscoveryOptions{
			Now:                   now,
			Roots:                 map[string]string{clean.CategoryThunderUpdateDownload: root},
			DetectThunderActivity: detect,
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
}

// TestThunderUpdateDownload_OldChildBecomesCandidate covers acceptance case 1:
// a direct child whose latest observed modification is 40 days before Now
// becomes an opt-in candidate carrying aggregated descendant bytes.
func TestThunderUpdateDownload_OldChildBecomesCandidate(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()
	child := filepath.Join(root, "old-pkg")
	if err := os.MkdirAll(filepath.Join(child, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "a.bin"), []byte("aaa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "sub", "b.bin"), []byte("bbbbb"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 40))

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want exactly old-pkg", result.OptInCandidates)
	}
	c := result.OptInCandidates[0]
	if c.Category != clean.CategoryThunderUpdateDownload {
		t.Fatalf("category = %q", c.Category)
	}
	if filepath.Base(c.Path) != "old-pkg" {
		t.Fatalf("candidate path = %q", c.Path)
	}
	if c.Bytes != 8 {
		t.Fatalf("aggregated bytes = %d, want 8", c.Bytes)
	}
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryThunderUpdateDownload)
	if !ok {
		t.Fatal("thunder-update-download missing from catalog")
	}
	if c.PlannedAction != string(summary.PlannedAction) {
		t.Fatalf("planned_action = %q, want %q", c.PlannedAction, summary.PlannedAction)
	}
}

// TestThunderUpdateDownload_FreshChildExcluded covers acceptance case 2: a
// direct child modified only 5 days before Now is inside the stability window
// and never becomes a candidate.
func TestThunderUpdateDownload_FreshChildExcluded(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()
	child := filepath.Join(root, "fresh-pkg")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.bin"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 5))

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("fresh child must be excluded: %#v", result.OptInCandidates)
	}
}

// TestThunderUpdateDownload_DeepFreshDescendantExcluded covers acceptance case
// 3: a child directory whose own mtime is old but that contains a descendant
// file modified 5 days before Now is excluded, because the window spans all
// safely inspected descendants.
func TestThunderUpdateDownload_DeepFreshDescendantExcluded(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()
	child := filepath.Join(root, "mixed-pkg")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(child, "fresh.bin")
	if err := os.WriteFile(fresh, []byte("recent"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Fresh descendant, then stamp the parent dir old AFTER writing the file.
	if err := os.Chtimes(fresh, daysAgo(now, 5), daysAgo(now, 5)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(child, daysAgo(now, 40), daysAgo(now, 40)); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("fresh descendant must exclude the child: %#v", result.OptInCandidates)
	}
}

// TestThunderUpdateDownload_SymlinkChildNeverCandidate covers acceptance case 4:
// a symlink direct child is never a candidate, even when it targets a tree that
// would otherwise qualify. Skips on symlink-privilege-restricted environments.
func TestThunderUpdateDownload_SymlinkChildNeverCandidate(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()

	// A real, old, qualifying sibling proves the symlink is skipped specifically.
	realOld := filepath.Join(root, "real-old")
	if err := os.MkdirAll(realOld, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realOld, "f.bin"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, realOld, daysAgo(now, 40))

	link := filepath.Join(root, "link-pkg")
	if err := os.Symlink(realOld, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only real-old", result.OptInCandidates)
	}
	for _, c := range result.OptInCandidates {
		if filepath.Base(c.Path) == "link-pkg" {
			t.Fatalf("symlink child became a candidate: %q", c.Path)
		}
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "real-old" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
}

// TestThunderUpdateDownload_MissingRootSilentAbsence covers acceptance case 5: a
// missing root yields no candidates and emits no thunder-update-download
// diagnostics, skips, or running states.
func TestThunderUpdateDownload_MissingRootSilentAbsence(t *testing.T) {
	now := thunderStableTime
	root := filepath.Join(t.TempDir(), "does-not-exist")

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("missing root must yield no candidates: %#v", result.OptInCandidates)
	}
	for _, d := range result.Errors {
		if strings.Contains(d.Code, "thunder") || d.Rule == clean.CategoryThunderUpdateDownload {
			t.Fatalf("missing root must be silent, got diagnostic %#v", d)
		}
	}
	for _, s := range result.Skipped {
		if s.Rule == clean.CategoryThunderUpdateDownload {
			t.Fatalf("missing root must not skip loudly, got %#v", s)
		}
	}
	for _, rs := range result.RunningApplications {
		if rs.Application == clean.ApplicationThunder {
			t.Fatalf("missing root must not project Thunder state, got %#v", rs)
		}
	}
}

// TestThunderUpdateDownload_ActivityGateSkipsCategory covers acceptance case 6:
// running or unknown Thunder state skips the whole category with a SkippedItem
// and no candidates.
func TestThunderUpdateDownload_ActivityGateSkipsCategory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status clean.FixedRootActivityStatus
	}{
		{"running", clean.FixedRootActivityRunning},
		{"unknown", clean.FixedRootActivityUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := thunderStableTime
			root := t.TempDir()
			child := filepath.Join(root, "old-pkg")
			if err := os.MkdirAll(child, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(child, "f.bin"), []byte("data"), 0o600); err != nil {
				t.Fatal(err)
			}
			chtimesTree(t, child, daysAgo(now, 40))

			result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(tc.status)))

			if len(result.OptInCandidates) != 0 {
				t.Fatalf("active/unknown must skip whole category: %#v", result.OptInCandidates)
			}
			found := false
			for _, s := range result.Skipped {
				if s.Rule == clean.CategoryThunderUpdateDownload {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected whole-category SkippedItem, skipped = %#v", result.Skipped)
			}
		})
	}
}

// TestThunderUpdateDownload_IdleThenRunningDiscardsCandidates covers acceptance
// case 7: idle before discovery then running on the post re-check discards every
// measured candidate for the whole category.
func TestThunderUpdateDownload_IdleThenRunningDiscardsCandidates(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()
	child := filepath.Join(root, "old-pkg")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.bin"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 40))

	var calls atomic.Int32
	detect := func(context.Context) clean.FixedRootActivityState {
		status := clean.FixedRootActivityIdle
		if calls.Add(1) >= 2 {
			status = clean.FixedRootActivityRunning
		}
		return clean.FixedRootActivityState{Status: status}
	}

	result := clean.DryRun(context.Background(), thunderOpts(root, now, detect))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("post-running re-check must discard candidates: %#v", result.OptInCandidates)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected pre and post activity checks, calls = %d", calls.Load())
	}
}

// TestThunderUpdateDownload_IncompleteInspectionDisqualifies covers acceptance
// case 8: an incompletely inspected child is disqualified. Descendant-ceiling
// truncation is surfaced only as an inspection error (opportunityInspection has
// no partial-result flag), so the hermetic proxy is a child holding a reparse
// descendant, which aborts the walk through the identical error branch. Skips on
// symlink-privilege-restricted environments.
func TestThunderUpdateDownload_IncompleteInspectionDisqualifies(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()

	// Clean old child that fully inspects and qualifies.
	cleanOld := filepath.Join(root, "clean-old")
	if err := os.MkdirAll(cleanOld, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cleanOld, "f.bin"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, cleanOld, daysAgo(now, 40))

	// Poisoned old child: its own mtime is old, but a reparse descendant makes
	// inspection incomplete (returns an error), disqualifying the child.
	poison := filepath.Join(root, "poison-old")
	if err := os.MkdirAll(poison, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.bin")
	if err := os.WriteFile(target, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(poison, "inner-link")); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if err := os.Chtimes(poison, daysAgo(now, 40), daysAgo(now, 40)); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle)))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only clean-old", result.OptInCandidates)
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "clean-old" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
	for _, c := range result.OptInCandidates {
		if filepath.Base(c.Path) == "poison-old" {
			t.Fatalf("incompletely inspected child must be disqualified: %q", c.Path)
		}
	}
}

// TestThunderUpdateDownload_ProtectedChildSuppressed covers acceptance case 9: a
// protected direct child is suppressed and never surfaces as a candidate.
func TestThunderUpdateDownload_ProtectedChildSuppressed(t *testing.T) {
	now := thunderStableTime
	root := t.TempDir()

	protectedChild := filepath.Join(root, "protected-pkg")
	if err := os.MkdirAll(protectedChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protectedChild, "f.bin"), []byte("pp"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, protectedChild, daysAgo(now, 40))

	openChild := filepath.Join(root, "open-pkg")
	if err := os.MkdirAll(openChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(openChild, "f.bin"), []byte("ooo"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, openChild, daysAgo(now, 40))

	opts := thunderOpts(root, now, thunderActivity(clean.FixedRootActivityIdle))
	opts.Validator = pathsafe.NewValidator([]string{protectedChild})

	result := clean.DryRun(context.Background(), opts)

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only open-pkg", result.OptInCandidates)
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "open-pkg" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
	for _, c := range result.OptInCandidates {
		if filepath.Base(c.Path) == "protected-pkg" {
			t.Fatalf("protected child must be suppressed: %q", c.Path)
		}
	}
}
