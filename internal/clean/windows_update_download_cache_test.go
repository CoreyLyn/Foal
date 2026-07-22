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

// wudcStableTime is a fixed clock so stability-window math is deterministic.
// (daysAgo and chtimesTree are shared with thunder_update_download_cache_test.go.)
var wudcStableTime = time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

// wudcIdle is an injected read-only service detector that always reports idle.
func wudcIdle(context.Context) clean.WindowsUpdateServicesState {
	return clean.WindowsUpdateServicesState{Status: clean.WindowsUpdateServicesIdle}
}

// wudcOpts assembles the shared DryRun options with the injected root override,
// clock, and an idle service gate so discovery proceeds.
func wudcOpts(root string, now time.Time, detect func(context.Context) clean.WindowsUpdateServicesState) clean.Options {
	return clean.Options{
		OptIn: []string{clean.CategoryWindowsUpdateDownloadCache},
		WindowsUpdateDownloadCacheDiscoveryOptions: clean.WindowsUpdateDownloadCacheDiscoveryOptions{
			Root:           root,
			Now:            now,
			DetectServices: detect,
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
}

// TestWindowsUpdateDownloadCache_StaleChildBecomesCandidate: a direct child whose
// latest observed modification is exactly 30 days old (inclusive boundary)
// becomes an opt-in candidate carrying aggregated descendant bytes.
func TestWindowsUpdateDownloadCache_StaleChildBecomesCandidate(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "abc123-package")
	if err := os.MkdirAll(filepath.Join(child, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "a.cab"), []byte("aaa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "sub", "b.cab"), []byte("bbbbb"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 30))

	result := clean.DryRun(context.Background(), wudcOpts(root, now, wudcIdle))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want exactly the package at the 30-day boundary", result.OptInCandidates)
	}
	c := result.OptInCandidates[0]
	if c.Category != clean.CategoryWindowsUpdateDownloadCache {
		t.Fatalf("category = %q", c.Category)
	}
	if filepath.Base(c.Path) != "abc123-package" {
		t.Fatalf("candidate path = %q", c.Path)
	}
	if c.Bytes != 8 {
		t.Fatalf("aggregated bytes = %d, want 8", c.Bytes)
	}
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryWindowsUpdateDownloadCache)
	if !ok {
		t.Fatal("windows-update-download-cache missing from catalog")
	}
	if c.PlannedAction != string(summary.PlannedAction) {
		t.Fatalf("planned_action = %q, want %q", c.PlannedAction, summary.PlannedAction)
	}
}

// TestWindowsUpdateDownloadCache_FreshChildExcluded: a child modified only 29
// days before Now is inside the stability window and never becomes a candidate.
func TestWindowsUpdateDownloadCache_FreshChildExcluded(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "fresh")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 29))

	result := clean.DryRun(context.Background(), wudcOpts(root, now, wudcIdle))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("child under the 30-day window must be excluded: %#v", result.OptInCandidates)
	}
}

// TestWindowsUpdateDownloadCache_FutureTimestampExcluded: an unknown/future
// latest-write fails closed and never becomes a candidate.
func TestWindowsUpdateDownloadCache_FutureTimestampExcluded(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "future")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, -5)) // five days in the future

	result := clean.DryRun(context.Background(), wudcOpts(root, now, wudcIdle))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("future timestamp must fail closed: %#v", result.OptInCandidates)
	}
}

// TestWindowsUpdateDownloadCache_SymlinkChildNeverCandidate: a symlink direct
// child is never a candidate even when it targets a tree that would otherwise
// qualify. Skips on symlink-privilege-restricted environments.
func TestWindowsUpdateDownloadCache_SymlinkChildNeverCandidate(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()

	realOld := filepath.Join(root, "real-old")
	if err := os.MkdirAll(realOld, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realOld, "f.cab"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, realOld, daysAgo(now, 40))

	link := filepath.Join(root, "link")
	if err := os.Symlink(realOld, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}

	result := clean.DryRun(context.Background(), wudcOpts(root, now, wudcIdle))

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only real-old", result.OptInCandidates)
	}
	for _, c := range result.OptInCandidates {
		if filepath.Base(c.Path) == "link" {
			t.Fatalf("symlink child became a candidate: %q", c.Path)
		}
	}
}

// TestWindowsUpdateDownloadCache_ProtectedChildSuppressed: a protected direct
// child is suppressed and never surfaces as a candidate.
func TestWindowsUpdateDownloadCache_ProtectedChildSuppressed(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()

	protectedChild := filepath.Join(root, "protected")
	if err := os.MkdirAll(protectedChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protectedChild, "f.cab"), []byte("pp"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, protectedChild, daysAgo(now, 40))

	openChild := filepath.Join(root, "open")
	if err := os.MkdirAll(openChild, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(openChild, "f.cab"), []byte("ooo"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, openChild, daysAgo(now, 40))

	opts := wudcOpts(root, now, wudcIdle)
	opts.Validator = pathsafe.NewValidator([]string{protectedChild})

	result := clean.DryRun(context.Background(), opts)

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only open", result.OptInCandidates)
	}
	if filepath.Base(result.OptInCandidates[0].Path) != "open" {
		t.Fatalf("candidate = %#v", result.OptInCandidates[0])
	}
}

// TestWindowsUpdateDownloadCache_MissingRootSilentAbsence: a missing root yields
// no candidates and emits no diagnostics or skips.
func TestWindowsUpdateDownloadCache_MissingRootSilentAbsence(t *testing.T) {
	now := wudcStableTime
	root := filepath.Join(t.TempDir(), "does-not-exist")

	result := clean.DryRun(context.Background(), wudcOpts(root, now, wudcIdle))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("missing root must yield no candidates: %#v", result.OptInCandidates)
	}
	for _, d := range result.Errors {
		if strings.Contains(d.Code, "windows_update_download_cache") || d.Rule == clean.CategoryWindowsUpdateDownloadCache {
			t.Fatalf("missing root must be silent, got diagnostic %#v", d)
		}
	}
	for _, s := range result.Skipped {
		if s.Rule == clean.CategoryWindowsUpdateDownloadCache {
			t.Fatalf("missing root must not skip loudly, got %#v", s)
		}
	}
}

// TestWindowsUpdateDownloadCache_UnresolvableSystemRootSilentAbsence:
// blank/relative/UNC SystemRoot values are silent absence.
func TestWindowsUpdateDownloadCache_UnresolvableSystemRootSilentAbsence(t *testing.T) {
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
				OptIn: []string{clean.CategoryWindowsUpdateDownloadCache},
				WindowsUpdateDownloadCacheDiscoveryOptions: clean.WindowsUpdateDownloadCacheDiscoveryOptions{
					SystemRoot:     tc.systemRoot,
					Now:            wudcStableTime,
					DetectServices: wudcIdle,
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
				if strings.Contains(d.Code, "windows_update_download_cache") || d.Rule == clean.CategoryWindowsUpdateDownloadCache {
					t.Fatalf("%s SystemRoot must be silent, got %#v", tc.name, d)
				}
			}
		})
	}
}

// TestWindowsUpdateDownloadCache_ServicesRunningSkipsWholeCategory: the pre-gate
// running observation skips the whole category with the stable path-free reason
// windows_update_services_active and yields no candidates, even when a stale
// child exists.
func TestWindowsUpdateDownloadCache_ServicesRunningSkipsWholeCategory(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "stale")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 40))

	running := func(context.Context) clean.WindowsUpdateServicesState {
		return clean.WindowsUpdateServicesState{Status: clean.WindowsUpdateServicesRunning, Message: "a Windows Update service is active"}
	}
	result := clean.DryRun(context.Background(), wudcOpts(root, now, running))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("running services must skip the whole category: %#v", result.OptInCandidates)
	}
	if !hasWUDCSkip(result.Skipped) {
		t.Fatalf("expected a windows_update_services_active skip, got %#v", result.Skipped)
	}
}

// TestWindowsUpdateDownloadCache_ServicesUnknownSkipsWholeCategory: an unknown
// service observation (query failure) fails closed to a whole-category skip with
// the same stable reason.
func TestWindowsUpdateDownloadCache_ServicesUnknownSkipsWholeCategory(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "stale")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 40))

	unknown := func(context.Context) clean.WindowsUpdateServicesState {
		return clean.WindowsUpdateServicesState{Status: clean.WindowsUpdateServicesUnknown, Message: "service state could not be determined"}
	}
	result := clean.DryRun(context.Background(), wudcOpts(root, now, unknown))

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("unknown service state must fail closed to a skip: %#v", result.OptInCandidates)
	}
	if !hasWUDCSkip(result.Skipped) {
		t.Fatalf("expected a windows_update_services_active skip, got %#v", result.Skipped)
	}
}

// TestWindowsUpdateDownloadCache_ServicesWakeAfterMeasurementDiscards: the
// services are idle at the pre-gate (so discovery proceeds and measures a
// candidate) but running at the post re-check; the whole measured set is
// discarded and the category is skipped.
func TestWindowsUpdateDownloadCache_ServicesWakeAfterMeasurementDiscards(t *testing.T) {
	now := wudcStableTime
	root := t.TempDir()
	child := filepath.Join(root, "stale")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f.cab"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	chtimesTree(t, child, daysAgo(now, 40))

	calls := 0
	wakeAfter := func(context.Context) clean.WindowsUpdateServicesState {
		calls++
		if calls == 1 {
			return clean.WindowsUpdateServicesState{Status: clean.WindowsUpdateServicesIdle}
		}
		return clean.WindowsUpdateServicesState{Status: clean.WindowsUpdateServicesRunning, Message: "update stack woke mid-run"}
	}
	result := clean.DryRun(context.Background(), wudcOpts(root, now, wakeAfter))

	if calls < 2 {
		t.Fatalf("post-measurement re-check must run, service detector called %d times", calls)
	}
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("post-gate wake must discard all measured candidates: %#v", result.OptInCandidates)
	}
	if !hasWUDCSkip(result.Skipped) {
		t.Fatalf("expected a windows_update_services_active skip, got %#v", result.Skipped)
	}
}

// TestWindowsUpdateDownloadCache_ImpactNoticePresent: a category with a candidate
// carries the path-free machine-wide impact notice in its preview projection.
func TestWindowsUpdateDownloadCache_ImpactNoticePresent(t *testing.T) {
	obs := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.CategoryWindowsUpdateDownloadCache,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:     `C:\Windows\SoftwareDistribution\Download\abc123`,
			Bytes:    4096,
			Category: clean.CategoryWindowsUpdateDownloadCache,
		}},
	})
	if obs.SafetyNote == "" {
		t.Fatal("windows-update-download-cache preview must carry an impact notice")
	}
	if !strings.Contains(obs.SafetyNote, "all users of the machine") {
		t.Fatalf("impact notice must disclose machine-wide scope, got %q", obs.SafetyNote)
	}
	if !strings.Contains(obs.SafetyNote, "re-downloads") {
		t.Fatalf("impact notice must disclose re-download recovery, got %q", obs.SafetyNote)
	}
}

// TestWindowsUpdateDownloadCache_ExactSelectionOnly: the category is
// exact-selection-only and never starts selected (excluded from `all`, group
// tokens, and TUI Select All).
func TestWindowsUpdateDownloadCache_ExactSelectionOnly(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.CategoryWindowsUpdateDownloadCache)
	if !ok {
		t.Fatal("windows-update-download-cache missing from catalog")
	}
	if !clean.ExactSelectionOnlyCategory(summary) {
		t.Fatalf("windows-update-download-cache must be exact-selection-only: %#v", summary)
	}
	if clean.InitiallySelectedCategory(summary) {
		t.Fatal("windows-update-download-cache must never start selected")
	}
	if summary.PlannedAction != clean.PlannedActionMoveToRecycleBin {
		t.Fatalf("planned_action = %q, want move_to_recycle_bin", summary.PlannedAction)
	}
	if summary.ReportCategory != clean.ReportCategorySystem {
		t.Fatalf("report category = %q, want System", summary.ReportCategory)
	}
}

// hasWUDCSkip reports whether the skipped set contains the stable
// windows_update_services_active whole-category skip.
func hasWUDCSkip(skipped []clean.SkippedItem) bool {
	for _, s := range skipped {
		if s.Rule == clean.CategoryWindowsUpdateDownloadCache && s.Reason.Code == "windows_update_services_active" {
			return true
		}
	}
	return false
}
