package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// wtStableTime is a fixed clock so stability-window math is deterministic.
// (daysAgo and chtimesTree are shared with thunder_update_download_cache_test.go.)
var wtStableTime = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

// wtOpts assembles the shared DryRun options with the injected %SystemRoot%\Temp
// root override and clock. windows-temp has no running-application gate.
func wtOpts(root string, now time.Time) clean.Options {
	return clean.Options{
		OptIn: []string{clean.CategoryWindowsTemp},
		WindowsTempDiscoveryOptions: clean.WindowsTempDiscoveryOptions{
			Root: root,
			Now:  now,
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
}

// TestWindowsTemp_StaleChildBecomesCandidate: a direct child whose latest
// observed modification is exactly 14 days old (inclusive boundary) becomes an
// opt-in candidate carrying aggregated descendant bytes.
func TestWindowsTemp_StaleChildBecomesCandidate(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()
	child := filepath.Join(root, "DiagOutputDir")
	if err := os.MkdirAll(filepath.Join(child, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "a.log"), []byte("aaa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "sub", "b.log"), []byte("bbbbb"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 14))

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want exactly DiagOutputDir at the 14-day boundary", result.OptInCandidates)
	}
	c := result.OptInCandidates[0]
	if c.Category != clean.CategoryWindowsTemp {
		t.Fatalf("category = %q", c.Category)
	}
	if filepath.Base(c.Path) != "DiagOutputDir" {
		t.Fatalf("candidate path = %q", c.Path)
	}
	if c.Bytes != 8 {
		t.Fatalf("aggregated bytes = %d, want 8", c.Bytes)
	}
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryWindowsTemp)
	if !ok {
		t.Fatal("windows-temp missing from catalog")
	}
	if c.PlannedAction != string(summary.PlannedAction) {
		t.Fatalf("planned_action = %q, want %q", c.PlannedAction, summary.PlannedAction)
	}
}

// TestWindowsTemp_FreshChildExcluded: a child modified only 13 days before Now is
// inside the stability window and never becomes a candidate.
func TestWindowsTemp_FreshChildExcluded(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()
	child := filepath.Join(root, "fresh")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.tmp"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 13))

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("child under the 14-day window must be excluded: %#v", result.OptInCandidates)
	}
}

// TestWindowsTemp_DeepFreshDescendantExcludesChild: a child directory whose own
// mtime is old but that contains a descendant modified 3 days before Now is
// excluded, because the window spans all safely inspected descendants
// (user_temp semantics).
func TestWindowsTemp_DeepFreshDescendantExcludesChild(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()
	child := filepath.Join(root, "mixed")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(child, "fresh.tmp")
	if err := os.WriteFile(fresh, []byte("recent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, daysAgo(now, 3), daysAgo(now, 3)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(child, daysAgo(now, 40), daysAgo(now, 40)); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("fresh descendant must exclude the child: %#v", result.OptInCandidates)
	}
}

// TestWindowsTemp_FutureTimestampExcluded: an unknown/future latest-write fails
// closed and never becomes a candidate.
func TestWindowsTemp_FutureTimestampExcluded(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()
	child := filepath.Join(root, "future")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.tmp"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Five days in the future.
	chtimesTree(t, child, daysAgo(now, -5))

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("future timestamp must fail closed: %#v", result.OptInCandidates)
	}
}

// TestWindowsTemp_SymlinkChildNeverCandidate: a symlink direct child is never a
// candidate even when it targets a tree that would otherwise qualify. Skips on
// symlink-privilege-restricted environments.
func TestWindowsTemp_SymlinkChildNeverCandidate(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()

	realOld := filepath.Join(root, "real-old")
	if err := os.MkdirAll(realOld, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realOld, "f.tmp"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, realOld, daysAgo(now, 40))

	link := filepath.Join(root, "link")
	if err := os.Symlink(realOld, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only real-old", result.OptInCandidates)
	}
	for _, c := range result.OptInCandidates {
		if filepath.Base(c.Path) == "link" {
			t.Fatalf("symlink child became a candidate: %q", c.Path)
		}
	}
}

// TestWindowsTemp_PerItemInspectionFailureIsolated: an incompletely inspected
// child (a reparse descendant aborts its walk) is disqualified while a clean
// sibling still qualifies — per-item isolation, never a whole-category failure.
func TestWindowsTemp_PerItemInspectionFailureIsolated(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()

	cleanOld := filepath.Join(root, "clean-old")
	if err := os.MkdirAll(cleanOld, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cleanOld, "f.tmp"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, cleanOld, daysAgo(now, 40))

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

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only clean-old", result.OptInCandidates)
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "clean-old" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
}

// TestWindowsTemp_ProtectedChildSuppressed: a protected direct child is
// suppressed and never surfaces as a candidate.
func TestWindowsTemp_ProtectedChildSuppressed(t *testing.T) {
	now := wtStableTime
	root := t.TempDir()

	protectedChild := filepath.Join(root, "protected")
	if err := os.MkdirAll(protectedChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protectedChild, "f.tmp"), []byte("pp"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, protectedChild, daysAgo(now, 40))

	openChild := filepath.Join(root, "open")
	if err := os.MkdirAll(openChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(openChild, "f.tmp"), []byte("ooo"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, openChild, daysAgo(now, 40))

	opts := wtOpts(root, now)
	opts.Validator = pathsafe.NewValidator([]string{protectedChild})

	result := clean.DryRun(context.Background(), opts)

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only open", result.OptInCandidates)
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "open" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
}

// TestWindowsTemp_MissingRootSilentAbsence: a missing root yields no candidates
// and emits no windows-temp diagnostics or skips.
func TestWindowsTemp_MissingRootSilentAbsence(t *testing.T) {
	now := wtStableTime
	root := filepath.Join(t.TempDir(), "does-not-exist")

	result := clean.DryRun(context.Background(), wtOpts(root, now))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("missing root must yield no candidates: %#v", result.OptInCandidates)
	}
	for _, d := range result.Errors {
		if strings.Contains(d.Code, "windows_temp") || d.Rule == clean.CategoryWindowsTemp {
			t.Fatalf("missing root must be silent, got diagnostic %#v", d)
		}
	}
	for _, s := range result.Skipped {
		if s.Rule == clean.CategoryWindowsTemp {
			t.Fatalf("missing root must not skip loudly, got %#v", s)
		}
	}
}

// TestWindowsTemp_UnresolvableSystemRootSilentAbsence: blank/relative/UNC
// SystemRoot values are silent absence. Root override is left empty so the
// SystemRoot override drives resolution.
func TestWindowsTemp_UnresolvableSystemRootSilentAbsence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		systemRoot string
	}{
		{"relative", `Windows`},
		{"unc", `\\server\share`},
		{"dotted-relative", `.\Windows`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := clean.Options{
				OptIn: []string{clean.CategoryWindowsTemp},
				WindowsTempDiscoveryOptions: clean.WindowsTempDiscoveryOptions{
					SystemRoot: tc.systemRoot,
					Now:        wtStableTime,
				},
				DiscoverOpportunities:     noOpportunities,
				DiscoverReviewSuggestions: noReviewSuggestions,
				Rules:                     []clean.Rule{{ID: "test", DefaultEnabled: false}},
			}
			result := clean.DryRun(context.Background(), opts)
			if len(result.OptInCandidates) != 0 {
				t.Fatalf("%s SystemRoot must be silent absence: %#v", tc.name, result.OptInCandidates)
			}
			for _, d := range result.Errors {
				if strings.Contains(d.Code, "windows_temp") || d.Rule == clean.CategoryWindowsTemp {
					t.Fatalf("%s SystemRoot must be silent, got %#v", tc.name, d)
				}
			}
		})
	}
}

// TestWindowsTemp_ImpactNoticePresent: a category with a candidate carries the
// path-free machine-wide impact notice in its preview projection.
func TestWindowsTemp_ImpactNoticePresent(t *testing.T) {
	obs := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.CategoryWindowsTemp,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:     `C:\Windows\Temp\DiagOutputDir`,
			Bytes:    4096,
			Category: clean.CategoryWindowsTemp,
		}},
	})
	if obs.SafetyNote == "" {
		t.Fatal("windows-temp preview must carry an impact notice")
	}
	if !strings.Contains(obs.SafetyNote, "all users of the machine") {
		t.Fatalf("impact notice must disclose machine-wide scope, got %q", obs.SafetyNote)
	}
}

// TestWindowsTemp_ExactSelectionOnly: the category is exact-selection-only and
// never starts selected (excluded from `all`, group tokens, and TUI Select All).
func TestWindowsTemp_ExactSelectionOnly(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryWindowsTemp)
	if !ok {
		t.Fatal("windows-temp missing from catalog")
	}
	if !clean.ExactSelectionOnlyCategory(summary) {
		t.Fatalf("windows-temp must be exact-selection-only: %#v", summary)
	}
	if clean.InitiallySelectedCategory(summary) {
		t.Fatal("windows-temp must never start selected")
	}
	if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
		t.Fatalf("planned_action = %q, want move_to_recycle_bin", summary.PlannedAction)
	}
	if summary.ReportCategory != clean.ReportCategorySystem {
		t.Fatalf("report category = %q, want System", summary.ReportCategory)
	}
}
