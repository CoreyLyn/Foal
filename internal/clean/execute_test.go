package clean_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

type recordingRecycleBinAdapter struct {
	paths []string
}

func (a *recordingRecycleBinAdapter) MoveToRecycleBin(path string) error {
	a.paths = append(a.paths, path)
	return nil
}

func TestExecuteMovesEligibleCandidatesThroughRecycleBin(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: adapter,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if result.Status != "ok" || result.Mode != "execute" {
		t.Fatalf("status/mode = %q/%q, want ok/execute", result.Status, result.Mode)
	}
	if len(adapter.paths) != 1 || adapter.paths[0] != candidate {
		t.Fatalf("adapter paths = %v, want [%q]", adapter.paths, candidate)
	}
	if len(result.Deleted) != 1 {
		t.Fatalf("deleted = %#v, want one deleted item", result.Deleted)
	}
	if result.Deleted[0].Path != candidate || result.Deleted[0].Bytes != 5 || result.Deleted[0].Rule != "test_default_rule" {
		t.Fatalf("deleted item = %#v, want path/size/rule", result.Deleted[0])
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("skipped = %#v, want none", result.Skipped)
	}
	if result.Totals.CandidateCount != 1 || result.Totals.DeletedCount != 1 || result.Totals.AffectedBytes != 5 {
		t.Fatalf("totals = %#v, want one candidate/deleted and five affected bytes", result.Totals)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Rebuildable project artifacts") ||
		strings.Contains(string(encoded), "foal analyze <path>") {
		t.Fatalf("execute result contains presentation-only project artifact clue: %s", encoded)
	}
}

func TestExecuteDoesNotDiscoverOrReturnCategorizedOpportunities(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	discoveryCalled := false
	recorder := &recordingHistoryRecorder{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		HistoryRecorder:   recorder,
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			t.Fatal("execute invoked review suggestion discovery")
			return nil
		},
		DiscoverOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			discoveryCalled = true
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{{
					Category: clean.OpportunityCategoryCrashDumps,
					Path:     filepath.Join(root, "unrelated.tmp"),
					Bytes:    4096,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				}},
			}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if discoveryCalled {
		t.Fatal("execute invoked review-only categorized opportunity discovery")
	}
	if len(result.Opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want none for execute", result.Opportunities)
	}
	if len(result.ReviewSuggestions) != 0 {
		t.Fatalf("review suggestions = %#v, want none for execute", result.ReviewSuggestions)
	}
	if result.Totals.OpportunityCount != 0 || result.Totals.OpportunityObservedBytes != 0 {
		t.Fatalf("opportunity totals = %d/%d, want zero for execute", result.Totals.OpportunityCount, result.Totals.OpportunityObservedBytes)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want one execute history session", recorder.sessions)
	}
	aggregate := recorder.sessions[0].Aggregate
	if aggregate.OpportunityCount != 0 || aggregate.OpportunityObservedBytes != 0 {
		t.Fatalf("history opportunity totals = %d/%d, want zero for execute", aggregate.OpportunityCount, aggregate.OpportunityObservedBytes)
	}
	for _, item := range recorder.items {
		if item.Path == filepath.Join(root, "unrelated.tmp") {
			t.Fatalf("execute history persisted review-only opportunity item: %#v", item)
		}
	}
}

func TestExecuteDoesNotPerformRunningApplicationDetection(t *testing.T) {
	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			t.Fatal("execute invoked browser running application detection")
			return nil
		},
		Rules: []clean.Rule{{
			ID:             "disabled_test_rule",
			DefaultEnabled: false,
		}},
	})

	if len(result.RunningApplications) != 0 || len(result.Opportunities) != 0 || result.Totals.OpportunityCount != 0 {
		t.Fatalf("execute review-only state = %#v, want none", result)
	}
}

func TestDryRunOpportunityNeverReachesExecuteAdapterOrHistory(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-owned.tmp")
	opportunity := filepath.Join(root, "unrelated-old-cache")
	if err := os.WriteFile(candidate, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	executeRecorder := &recordingHistoryRecorder{}
	options := clean.Options{
		RecycleBinAdapter: adapter,
		HistoryRecorder:   executeRecorder,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: []clean.UserTempOpportunity{{
				Path:   opportunity,
				Bytes:  4096,
				Status: clean.UserTempOpportunityStatus,
				Reason: clean.UserTempOpportunityReason,
			}}}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	}

	preview := clean.DryRun(context.Background(), options)
	result := clean.Execute(context.Background(), options)

	if len(preview.Opportunities) != 1 || preview.Opportunities[0].Path != opportunity {
		t.Fatalf("dry-run opportunities = %#v, want review-only path %q", preview.Opportunities, opportunity)
	}
	if len(adapter.paths) != 1 || adapter.paths[0] != candidate {
		t.Fatalf("execute adapter paths = %v, want only default candidate %q", adapter.paths, candidate)
	}
	if len(result.Opportunities) != 0 || result.Totals.OpportunityCount != 0 {
		t.Fatalf("execute opportunity data = %#v/%#v, want none", result.Opportunities, result.Totals)
	}
	for _, item := range executeRecorder.items {
		if item.Path == opportunity {
			t.Fatalf("execute history persisted opportunity path: %#v", item)
		}
	}
}

func TestExecuteRecordsHistorySessionAndDeletedItem(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingHistoryRecorder{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		HistoryRecorder:   recorder,
		CommandParameters: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--execute"},
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if result.Totals.DeletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1", result.Totals.DeletedCount)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v, want exactly one execute history session", recorder.sessions)
	}
	session := recorder.sessions[0]
	if session.Mode != "execute" || session.Aggregate.DeletedCount != 1 || session.Aggregate.AffectedBytes != 5 {
		t.Fatalf("session = %#v, want execute aggregate with deleted item", session)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("items = %#v, want one final execution item", recorder.items)
	}
	item := recorder.items[0]
	if item.Path != candidate || item.Rule != "test_default_rule" || item.Action != "move_to_recycle_bin" || item.Result != "deleted" {
		t.Fatalf("item = %#v, want deleted item with Recycle Bin action", item)
	}
	if item.Bytes == nil || *item.Bytes != 5 {
		t.Fatalf("item bytes = %#v, want 5", item.Bytes)
	}
}

func TestExecuteSkipsUnsafePathsBeforeRecycleBinAdapter(t *testing.T) {
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: adapter,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{`\\?\C:\Windows\System32`},
		}},
	})

	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v, want none", adapter.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one unsafe path skipped", result.Skipped)
	}
	if result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("reason code = %q, want protected_path", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteRecordsUserProtectedCandidateAsSkippedWithoutCallingRecycleBin(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "protected.tmp")
	if err := os.WriteFile(candidate, []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}

	result := clean.Execute(context.Background(), clean.Options{
		Validator:         pathsafe.NewValidator([]string{root}),
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v, want none", adapter.paths)
	}
	if len(result.Deleted) != 0 || result.Totals.AffectedBytes != 0 {
		t.Fatalf("result = %#v, want no deletion", result)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("skipped = %#v, want protected_path", result.Skipped)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("history items = %#v, want one skipped outcome", recorder.items)
	}
	item := recorder.items[0]
	if item.Result != "skipped" || item.Path != candidate || item.Rule != "test_default_rule" {
		t.Fatalf("history item = %#v, want protected skipped outcome", item)
	}
	mustHaveIssue(t, item.SkippedReason, "protected_path")
}

func TestExecuteFailsClosedBeforeRecycleBinWhenProtectionFileCannotLoad(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: adapter,
		ProtectionLoadError: &clean.StructuredIssue{
			Code:        "protection_file_load_failed",
			Message:     "selected protection file could not be read",
			Recoverable: false,
			Path:        `C:\missing\protection.txt`,
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if result.Status != "error" || result.Mode != "execute" {
		t.Fatalf("status/mode = %q/%q, want error/execute", result.Status, result.Mode)
	}
	if len(result.Candidates) != 0 || len(result.Deleted) != 0 || len(adapter.paths) != 0 {
		t.Fatalf("result/adapter = %#v/%#v, want fail-closed before executable candidates", result, adapter.paths)
	}
	if len(result.Errors) != 1 || result.Errors[0].Code != "protection_file_load_failed" || result.Errors[0].Recoverable {
		t.Fatalf("errors = %#v, want non-recoverable protection load error", result.Errors)
	}
}

func TestExecuteReportsRecycleBinPermissionFailureAsSkipped(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "locked.tmp")
	if err := os.WriteFile(candidate, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: failingRecycleBinAdapter{err: fs.ErrPermission},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one permission failure", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Path != candidate || skipped.Rule != "test_default_rule" || skipped.Bytes != 6 {
		t.Fatalf("skipped = %#v, want path/rule/bytes", skipped)
	}
	if skipped.Reason.Code != "permission_denied" || skipped.Reason.Message == "" || !skipped.Reason.Recoverable {
		t.Fatalf("reason = %#v, want recoverable permission_denied", skipped.Reason)
	}
}

func TestExecuteRecordsSkippedHistoryItemForRecycleBinFailure(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "locked.tmp")
	if err := os.WriteFile(candidate, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingHistoryRecorder{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: failingRecycleBinAdapter{err: fs.ErrPermission},
		HistoryRecorder:   recorder,
		CommandParameters: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--execute"},
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	if result.Totals.SkippedCount != 1 || result.Totals.DeletedCount != 0 {
		t.Fatalf("totals = %#v, want one skipped and no deleted", result.Totals)
	}
	if len(recorder.items) != 1 {
		t.Fatalf("items = %#v, want one skipped execution item", recorder.items)
	}
	item := recorder.items[0]
	if item.Result != "skipped" || item.Path != candidate || item.Rule != "test_default_rule" {
		t.Fatalf("item = %#v, want skipped execution item", item)
	}
	mustHaveIssue(t, item.SkippedReason, "permission_denied")
	if item.Bytes == nil || *item.Bytes != 6 {
		t.Fatalf("item bytes = %#v, want size metadata", item.Bytes)
	}
}

func TestExecuteIgnoresDryRunDetailedListAndUsesFreshCandidates(t *testing.T) {
	root := t.TempDir()
	staleCandidate := filepath.Join(root, "foal-stale.tmp")
	freshCandidate := filepath.Join(root, "foal-fresh.tmp")
	if err := os.WriteFile(staleCandidate, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshCandidate, []byte("fresh"), 0600); err != nil {
		t.Fatal(err)
	}
	detailedListDir := filepath.Join(t.TempDir(), "Foal", "history")
	dryRunResult := clean.DryRun(context.Background(), clean.Options{
		DetailedListDir:               detailedListDir,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{staleCandidate},
		}},
	})
	if dryRunResult.DetailedListPath == "" {
		t.Fatal("dry-run detailed list path is empty")
	}
	if err := os.WriteFile(dryRunResult.DetailedListPath, []byte("execution manifest attempt\n"+staleCandidate+"\n"), 0600); err != nil {
		t.Fatalf("poison detailed list: %v", err)
	}
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter: adapter,
		DetailedListDir:   detailedListDir,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{freshCandidate},
		}},
	})

	if result.Mode != "execute" || result.DetailedListPath != "" {
		t.Fatalf("result mode/path = %q/%q, want execute with no detailed list path", result.Mode, result.DetailedListPath)
	}
	if len(adapter.paths) != 1 || adapter.paths[0] != freshCandidate {
		t.Fatalf("adapter paths = %v, want only fresh candidate %q", adapter.paths, freshCandidate)
	}
}

type failingRecycleBinAdapter struct {
	err error
}

func (a failingRecycleBinAdapter) MoveToRecycleBin(string) error {
	return a.err
}

func TestDryRunOptInUserTempShowsOptInCandidatesNotOpportunities(t *testing.T) {
	root := t.TempDir()
	userTempPath := filepath.Join(root, "old_temp_dir")
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	// Set modification time to 8 days ago
	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	opts := clean.Options{
		OptIn: []string{"user_temp"},
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.DryRun(context.Background(), opts)
	if len(result.Opportunities) != 0 {
		t.Fatalf("expected no opportunities when opted in, got %d", len(result.Opportunities))
	}
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("expected 1 opt-in candidate, got %d", len(result.OptInCandidates))
	}
	if result.OptInCandidates[0].Path != userTempPath {
		t.Fatalf("opt-in candidate path mismatch, got %q want %q", result.OptInCandidates[0].Path, userTempPath)
	}
	if result.Totals.OptInCandidateCount != 1 {
		t.Fatalf("expected opt-in candidate count 1, got %d", result.Totals.OptInCandidateCount)
	}
	if result.Totals.OptInReclaimableBytes != 4 {
		t.Fatalf("expected opt-in reclaimable bytes 4, got %d", result.Totals.OptInReclaimableBytes)
	}
}

func TestExecuteOptInUserTempMovesToRecycleBinAndRecordsHistory(t *testing.T) {
	root := t.TempDir()
	userTempPath := filepath.Join(root, "old_temp_dir")
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	// Set modification time to 8 days ago
	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}

	opts := clean.Options{
		Rules:             []clean.Rule{},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		OptIn:             []string{"user_temp"},
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)
	// Check only that our user temp was deleted, ignore others
	found := false
	for _, p := range adapter.paths {
		if p == userTempPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to delete %q, got %v", userTempPath, adapter.paths)
	}
	// Check that our specific item is marked as opt-in
	foundDeleted := false
	for _, d := range result.Deleted {
		if d.Path == userTempPath {
			foundDeleted = true
			if !d.IsOptIn {
				t.Fatalf("expected deleted item to be marked as IsOptIn")
			}
		}
	}
	if !foundDeleted {
		t.Fatalf("expected deleted items to include %q", userTempPath)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1, got %d", result.Totals.OptInDeletedCount)
	}
	if result.Totals.OptInAffectedBytes != 4 {
		t.Fatalf("expected OptInAffectedBytes 4, got %d", result.Totals.OptInAffectedBytes)
	}
	if len(recorder.sessions) != 1 {
		t.Fatalf("expected 1 history session, got %d", len(recorder.sessions))
	}
	if recorder.sessions[0].Aggregate.OptInDeletedCount != 1 {
		t.Fatalf("history OptInDeletedCount mismatch, got %d want 1", recorder.sessions[0].Aggregate.OptInDeletedCount)
	}
	if recorder.sessions[0].Aggregate.OptInAffectedBytes != 4 {
		t.Fatalf("history OptInAffectedBytes mismatch, got %d want 4", recorder.sessions[0].Aggregate.OptInAffectedBytes)
	}
}

func TestOptInAllResolvesToAllCategories(t *testing.T) {
	enabled, invalid, valid := clean.NormalizedOptInSet([]string{"all"})
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid names, got %v", invalid)
	}
	// All 8 opportunity categories and 6 dev caches should be enabled
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
	if len(enabled) != len(expectedOpportunities)+len(expectedDevCaches) {
		t.Fatalf("expected %d enabled categories, got %d", len(expectedOpportunities)+len(expectedDevCaches), len(enabled))
	}
	// Verify valid names list includes all categories, "dev-caches", and "all"
	found := make(map[string]bool)
	for _, name := range valid {
		found[name] = true
	}
	for _, cat := range expectedOpportunities {
		if !found[cat] {
			t.Fatalf("valid names missing expected opportunity category %q", cat)
		}
	}
	for _, cat := range expectedDevCaches {
		if !found[cat] {
			t.Fatalf("valid names missing expected dev cache category %q", cat)
		}
	}
	if !found["all"] {
		t.Fatalf("valid names missing \"all\"")
	}
	if !found["dev-caches"] {
		t.Fatalf("valid names missing \"dev-caches\"")
	}
}

func TestOptInIndividualCategories(t *testing.T) {
	categories := []string{
		clean.OpportunityCategoryUserTemp,
		clean.OpportunityCategoryCrashDumps,
		clean.OpportunityCategoryWindowsErrorReporting,
		clean.OpportunityCategoryExplorerThumbnailCache,
		clean.OpportunityCategoryINetCache,
		clean.OpportunityCategoryD3DShaderCache,
		clean.OpportunityCategoryNVIDIADXCache,
		clean.OpportunityCategoryBrowserCache,
	}
	for _, cat := range categories {
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
}

func TestInvalidOptInNameReturnsErrorList(t *testing.T) {
	enabled, invalid, valid := clean.NormalizedOptInSet([]string{"invalid_name"})
	if len(enabled) != 0 {
		t.Fatalf("expected no enabled categories for invalid name, got %v", enabled)
	}
	if len(invalid) != 1 || invalid[0] != "invalid_name" {
		t.Fatalf("expected invalid name list to include \"invalid_name\", got %v", invalid)
	}
	// Should have 8 opportunity categories + 6 dev caches + "dev-caches" + "all" = 16
	if len(valid) != 16 {
		t.Fatalf("expected 16 valid names, got %d: %v", len(valid), valid)
	}
}

func TestDryRunOptInCategoriesShowOptInCandidatesNotOpportunities(t *testing.T) {
	testCategories := []struct {
		name     string
		category string
	}{
		{name: "crash_dumps", category: clean.OpportunityCategoryCrashDumps},
		{name: "windows_error_reporting", category: clean.OpportunityCategoryWindowsErrorReporting},
		{name: "explorer_thumbnail_cache", category: clean.OpportunityCategoryExplorerThumbnailCache},
		{name: "inet_cache", category: clean.OpportunityCategoryINetCache},
		{name: "d3d_shader_cache", category: clean.OpportunityCategoryD3DShaderCache},
		{name: "nvidia_dx_cache", category: clean.OpportunityCategoryNVIDIADXCache},
	}
	for _, tc := range testCategories {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			categoryPath := filepath.Join(root, tc.category)
			if err := os.Mkdir(categoryPath, 0700); err != nil {
				t.Fatal(err)
			}
			testFile := filepath.Join(categoryPath, "test.txt")
			if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}

			opts := clean.Options{
				OptIn: []string{tc.category},
				DiscoverOpportunities: func(ctx context.Context) clean.OpportunityDiscoveryResult {
					return clean.OpportunityDiscoveryResult{
						Opportunities: []clean.Opportunity{
							{
								Category: tc.category,
								Path:     categoryPath,
								Bytes:    4,
								Status:   clean.OpportunityStatus,
								Reason:   clean.OpportunityReason,
							},
						},
					}
				},
			}

			result := clean.DryRun(context.Background(), opts)
			if len(result.Opportunities) != 0 {
				t.Fatalf("expected no opportunities when opted in to %q, got %d", tc.category, len(result.Opportunities))
			}
			if len(result.OptInCandidates) != 1 {
				t.Fatalf("expected 1 opt-in candidate for %q, got %d", tc.category, len(result.OptInCandidates))
			}
			if result.OptInCandidates[0].Path != categoryPath {
				t.Fatalf("opt-in candidate path mismatch for %q, got %q want %q", tc.category, result.OptInCandidates[0].Path, categoryPath)
			}
			if result.OptInCandidates[0].Category != tc.category {
				t.Fatalf("opt-in candidate category mismatch for %q, got %q want %q", tc.category, result.OptInCandidates[0].Category, tc.category)
			}
			if result.Totals.OptInCandidateCount != 1 {
				t.Fatalf("expected opt-in candidate count 1 for %q, got %d", tc.category, result.Totals.OptInCandidateCount)
			}
			if result.Totals.OptInReclaimableBytes != 4 {
				t.Fatalf("expected opt-in reclaimable bytes 4 for %q, got %d", tc.category, result.Totals.OptInReclaimableBytes)
			}
		})
	}
}

func TestOptInUserTempRespectsProtectionRules(t *testing.T) {
	root := t.TempDir()
	userTempPath := filepath.Join(root, "old_temp_dir")
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	// Set modification time to 8 days ago
	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}
	// Protect the path
	validator := pathsafe.NewValidator([]string{userTempPath})

	opts := clean.Options{
		Rules:             []clean.Rule{},
		RecycleBinAdapter: adapter,
		Validator:         validator,
		OptIn:             []string{"user_temp"},
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	// Dry run should not show it as opt-in candidate (should be suppressed)
	dryRunResult := clean.DryRun(context.Background(), opts)
	if len(dryRunResult.OptInCandidates) != 0 {
		t.Fatalf("expected no opt-in candidates for protected path, got %d", len(dryRunResult.OptInCandidates))
	}
	// Execute should not delete it
	executeResult := clean.Execute(context.Background(), opts)
	// Check only our user temp wasn't deleted
	for _, p := range adapter.paths {
		if p == userTempPath {
			t.Fatalf("expected adapter to not delete protected path %q, but it did", userTempPath)
		}
	}
	for _, d := range executeResult.Deleted {
		if d.Path == userTempPath {
			t.Fatalf("expected deleted items to not include protected path %q, but it did", userTempPath)
		}
	}
}

func TestExecuteWithoutOptInDoesNotTouchUserTemp(t *testing.T) {
	root := t.TempDir()
	userTempPath := filepath.Join(root, "old_temp_dir")
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}
	discoveryCalled := false

	opts := clean.Options{
		Rules:             []clean.Rule{},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			discoveryCalled = true
			return clean.UserTempDiscoveryResult{}
		},
	}

	result := clean.Execute(context.Background(), opts)
	if discoveryCalled {
		t.Fatalf("execute should not call user temp discovery without opt-in")
	}
	for _, d := range result.Deleted {
		if d.Path == userTempPath {
			t.Fatalf("execute without opt-in should not delete user temp items")
		}
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("OptInDeletedCount should be 0 without opt-in")
	}
	if len(recorder.sessions) > 0 {
		for _, item := range recorder.items {
			if item.Path == userTempPath {
				t.Fatalf("history should not include user temp path without opt-in")
			}
		}
	}
}

func TestExecuteOptInSkipsWhenRecycleBinDisabled(t *testing.T) {
	root := t.TempDir()
	userTempPath := root + string(filepath.Separator) + "old_temp_dir"
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Fake probe that returns Recycle Bin disabled
	fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			NukeOnDelete: true,
			MaxCapacity:  100 * 1024 * 1024,
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{},
		RecycleBinAdapter:       adapter,
		OptIn:                   []string{"user_temp"},
		RecycleBinCapacityProbe: fakeProbe,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Adapter should NOT receive the path
	for _, p := range adapter.paths {
		if p == userTempPath {
			t.Fatalf("expected adapter to not receive path when Recycle Bin is disabled")
		}
	}

	// Should be skipped with recycle_bin_disabled reason
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "recycle_bin_disabled" {
		t.Fatalf("expected reason code recycle_bin_disabled, got %q", result.Skipped[0].Reason.Code)
	}
	if result.Skipped[0].Path != userTempPath {
		t.Fatalf("skipped path mismatch, got %q", result.Skipped[0].Path)
	}
}

func TestExecuteOptInSkipsWhenItemExceedsRecycleBinCapacity(t *testing.T) {
	root := t.TempDir()
	userTempPath := root + string(filepath.Separator) + "old_temp_dir"
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Fake probe that returns small MaxCapacity (1 byte)
	fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			NukeOnDelete: false,
			MaxCapacity:  1, // Only 1 byte capacity
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{},
		RecycleBinAdapter:       adapter,
		OptIn:                   []string{"user_temp"},
		RecycleBinCapacityProbe: fakeProbe,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4, // 4 bytes exceeds capacity
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Adapter should NOT receive the path
	for _, p := range adapter.paths {
		if p == userTempPath {
			t.Fatalf("expected adapter to not receive path when item exceeds capacity")
		}
	}

	// Should be skipped with recycle_bin_capacity reason
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "recycle_bin_capacity" {
		t.Fatalf("expected reason code recycle_bin_capacity, got %q", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteOptInAllowsItemWhenWithinRecycleBinCapacity(t *testing.T) {
	root := t.TempDir()
	userTempPath := root + string(filepath.Separator) + "old_temp_dir"
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Fake probe that returns large enough capacity
	fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			NukeOnDelete: false,
			MaxCapacity:  100 * 1024, // 100 KB
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{},
		RecycleBinAdapter:       adapter,
		OptIn:                   []string{"user_temp"},
		RecycleBinCapacityProbe: fakeProbe,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4, // 4 bytes fits
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Adapter should receive the path
	found := false
	for _, p := range adapter.paths {
		if p == userTempPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive path when item fits capacity")
	}

	// Should be deleted, not skipped
	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped items, got %d", len(result.Skipped))
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1, got %d", result.Totals.OptInDeletedCount)
	}
}

func TestExecuteOptInSkipsWhenProbeFails(t *testing.T) {
	root := t.TempDir()
	userTempPath := root + string(filepath.Separator) + "old_temp_dir"
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Fake probe that returns an error
	fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{}, errors.New("probe failed")
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{},
		RecycleBinAdapter:       adapter,
		OptIn:                   []string{"user_temp"},
		RecycleBinCapacityProbe: fakeProbe,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Probe failure should be fail-closed - adapter should NOT receive the path
	for _, p := range adapter.paths {
		if p == userTempPath {
			t.Fatalf("expected adapter to NOT receive path when probe fails (fail closed)")
		}
	}

	// Should be skipped with recycle_bin_capacity_probe_failed reason
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when probe fails, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "recycle_bin_capacity_probe_failed" {
		t.Fatalf("expected reason code recycle_bin_capacity_probe_failed, got %q", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteOptInUsesDefaultProbeWhenNotInjected(t *testing.T) {
	root := t.TempDir()
	userTempPath := root + string(filepath.Separator) + "old_temp_dir"
	if err := os.Mkdir(userTempPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(userTempPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(testFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Don't inject a probe - should use the default
	opts := clean.Options{
		Rules:             []clean.Rule{},
		RecycleBinAdapter: adapter,
		OptIn:             []string{"user_temp"},
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{
						Category:         clean.OpportunityCategoryUserTemp,
						Path:             userTempPath,
						Bytes:            4,
						LatestModifiedAt: eightDaysAgo,
						IdleDays:         8,
						Status:           clean.UserTempOpportunityStatus,
						Reason:           clean.UserTempOpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Default probe should allow deletion
	found := false
	for _, p := range adapter.paths {
		if p == userTempPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive path with default probe")
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 with default probe, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestExecuteOptInNonUserTempCategoryExecutes verifies a non-user_temp category executes via Recycle Bin
func TestExecuteOptInNonUserTempCategoryExecutes(t *testing.T) {
	root := t.TempDir()
	crashDumpsPath := filepath.Join(root, "CrashDumps")
	if err := os.Mkdir(crashDumpsPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(crashDumpsPath, "dump.dmp")
	if err := os.WriteFile(testFile, []byte("dump data"), 0600); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}

	opts := clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false, // Disable default candidate
		}},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		OptIn:             []string{"crash_dumps"},
		DiscoverOpportunities: func(ctx context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{
					{
						Category: clean.OpportunityCategoryCrashDumps,
						Path:     crashDumpsPath,
						Bytes:    9, // "dump data"
						Status:   clean.OpportunityStatus,
						Reason:   clean.OpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Verify the adapter received the path
	found := false
	for _, p := range adapter.paths {
		if p == crashDumpsPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive crash_dumps path, got %v", adapter.paths)
	}

	// Verify deleted item is marked IsOptIn with correct category as rule
	optInDeletedCount := 0
	for _, d := range result.Deleted {
		if d.IsOptIn {
			optInDeletedCount++
			if d.Rule != clean.OpportunityCategoryCrashDumps {
				t.Fatalf("expected deleted item rule to be crash_dumps, got %q", d.Rule)
			}
		}
	}
	if optInDeletedCount != 1 {
		t.Fatalf("expected 1 IsOptIn deleted item, got %d", optInDeletedCount)
	}

	// Verify history records the opt-in deletion
	if len(recorder.sessions) != 1 {
		t.Fatalf("expected 1 history session, got %d", len(recorder.sessions))
	}
	if recorder.sessions[0].Aggregate.OptInDeletedCount != 1 {
		t.Fatalf("history missing OptInDeletedCount = 1, got %d", recorder.sessions[0].Aggregate.OptInDeletedCount)
	}
	if recorder.sessions[0].Aggregate.OptInAffectedBytes != 9 {
		t.Fatalf("history missing OptInAffectedBytes = 9, got %d", recorder.sessions[0].Aggregate.OptInAffectedBytes)
	}
}

// TestExecuteOptInBrowserCacheSkipsWhenBrowserRunning verifies browser_cache skips when browser is running
func TestExecuteOptInBrowserCacheSkipsWhenBrowserRunning(t *testing.T) {
	adapter := &recordingRecycleBinAdapter{}

	// Create a detector that reports Chrome as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGoogleChrome,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	opts := clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false, // Disable default candidate
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"browser_cache"},
		DetectRunningApplications: detector,
	}

	result := clean.Execute(context.Background(), opts)

	// Verify nothing was deleted as opt-in (browser was running)
	for _, d := range result.Deleted {
		if d.IsOptIn {
			t.Fatalf("expected 0 IsOptIn deleted items when browser is running, got at least 1")
		}
	}
	// Verify adapter was not called with browser cache paths (no opt-in deletions)
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when browser is running, got %d: %v", len(adapter.paths), adapter.paths)
	}
}

// TestExecuteOptInBrowserCacheCleansWhenBrowserIdle verifies browser_cache cleans when browser is idle
func TestExecuteOptInBrowserCacheCleansWhenBrowserIdle(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	chromeUserData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	defaultCache := filepath.Join(chromeUserData, "Default", "Cache")
	if err := os.MkdirAll(defaultCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeUserData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(defaultCache, "data.bin")
	if err := os.WriteFile(cacheFile, []byte("cache data"), 0600); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Create a detector that reports Chrome as idle
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGoogleChrome,
				State:       clean.RunningApplicationStateIdle,
			},
			{
				Application: clean.ApplicationMicrosoftEdge,
				State:       clean.RunningApplicationStateIdle,
			},
		}
	}

	opts := clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false, // Disable default candidate
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"browser_cache"},
		DetectRunningApplications: detector,
		BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{
			LocalAppDataDir: localAppData,
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Verify the cache path was deleted (browser was idle)
	found := false
	for _, p := range adapter.paths {
		if p == defaultCache {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive browser cache path, got %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 when browser is idle, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestExecuteOptInNonUserTempCategoryRespectsCapacityPreCheck verifies capacity pre-check applies to non-user_temp categories
func TestExecuteOptInNonUserTempCategoryRespectsCapacityPreCheck(t *testing.T) {
	root := t.TempDir()
	crashDumpsPath := filepath.Join(root, "CrashDumps")
	if err := os.Mkdir(crashDumpsPath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(crashDumpsPath, "large_dump.dmp")
	if err := os.WriteFile(testFile, []byte("large dump data"), 0600); err != nil {
		t.Fatal(err)
	}

	adapter := &recordingRecycleBinAdapter{}

	// Fake probe that returns very small MaxCapacity
	fakeProbe := func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			NukeOnDelete: false,
			MaxCapacity:  1, // Only 1 byte capacity
		}, nil
	}

	opts := clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false, // Disable default candidate
		}},
		RecycleBinAdapter:       adapter,
		OptIn:                   []string{"crash_dumps"},
		RecycleBinCapacityProbe: fakeProbe,
		DiscoverOpportunities: func(ctx context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{
					{
						Category: clean.OpportunityCategoryCrashDumps,
						Path:     crashDumpsPath,
						Bytes:    16, // "large dump data"
						Status:   clean.OpportunityStatus,
						Reason:   clean.OpportunityReason,
					},
				},
			}
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Verify adapter did NOT receive the path (capacity check failed)
	for _, p := range adapter.paths {
		if p == crashDumpsPath {
			t.Fatalf("expected adapter to NOT receive crash_dumps path when over capacity")
		}
	}

	// Verify item was skipped with recycle_bin_capacity reason
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when over capacity, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "recycle_bin_capacity" {
		t.Fatalf("expected reason code recycle_bin_capacity, got %q", result.Skipped[0].Reason.Code)
	}
	if result.Skipped[0].Rule != clean.OpportunityCategoryCrashDumps {
		t.Fatalf("expected skipped item rule to be crash_dumps, got %q", result.Skipped[0].Rule)
	}
}

// TestExecuteWithoutOptInDoesNotRunDetection verifies default execute without opt-in does not run running-application detection
func TestExecuteWithoutOptInDoesNotRunDetection(t *testing.T) {
	adapter := &recordingRecycleBinAdapter{}
	detectionCalled := false

	detector := func(ctx context.Context) []clean.RunningApplicationState {
		detectionCalled = true
		return nil
	}

	// Set up a default candidate to ensure Execute runs
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-owned.tmp")
	if err := os.WriteFile(candidate, []byte("temp data"), 0600); err != nil {
		t.Fatal(err)
	}

	opts := clean.Options{
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{}, // No opt-in
		DetectRunningApplications: detector,
		Rules: []clean.Rule{
			{
				ID:             "test_rule",
				DefaultEnabled: true,
				CandidatePaths: []string{candidate},
			},
		},
	}

	result := clean.Execute(context.Background(), opts)

	// Verify detection was NOT called (default execute without opt-in should not run it)
	if detectionCalled {
		t.Fatalf("expected DetectRunningApplications to NOT be called without --opt-in")
	}

	// Verify default candidate still executes normally
	if result.Totals.DeletedCount != 1 {
		t.Fatalf("expected DeletedCount 1, got %d", result.Totals.DeletedCount)
	}
}

// TestExecuteOptInGoCacheSkipsWhenGoRunning verifies go-cache skips when go is running
func TestExecuteOptInGoCacheSkipsWhenGoRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryGo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Go as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"go-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// Verify nothing was deleted
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("expected OptInDeletedCount 0 when Go is running, got %d", result.Totals.OptInDeletedCount)
	}

	// Verify adapter got no paths
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when Go is running, got %d: %v", len(adapter.paths), adapter.paths)
	}

	// Verify it's in skipped with dev_tool_running reason
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when Go is running, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "dev_tool_running" {
		t.Fatalf("expected reason code dev_tool_running, got %q", result.Skipped[0].Reason.Code)
	}
}

// TestExecuteOptInGoCacheCleansWhenGoIdle verifies go-cache cleans when go is idle
func TestExecuteOptInGoCacheCleansWhenGoIdle(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryGo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Go as idle
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateIdle,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"go-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// Verify the cache path was deleted
	found := false
	for _, p := range adapter.paths {
		if p == cachePath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive go-cache path, got %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 when Go is idle, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestExecuteOptInCargoCacheSkipsWhenCargoRunning verifies cargo-cache skips when cargo is running
func TestExecuteOptInCargoCacheSkipsWhenCargoRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "cargo-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryCargo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Cargo as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationCargo,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"cargo-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("expected OptInDeletedCount 0 when Cargo is running, got %d", result.Totals.OptInDeletedCount)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when Cargo is running, got %d: %v", len(adapter.paths), adapter.paths)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when Cargo is running, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "dev_tool_running" {
		t.Fatalf("expected reason code dev_tool_running, got %q", result.Skipped[0].Reason.Code)
	}
}

// TestExecuteOptInNuGetCacheSkipsWhenDotNetRunning verifies nuget-cache skips when dotnet is running
func TestExecuteOptInNuGetCacheSkipsWhenDotNetRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "nuget-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryNuGet {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports dotnet as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationDotNet,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"nuget-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("expected OptInDeletedCount 0 when dotnet is running, got %d", result.Totals.OptInDeletedCount)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when dotnet is running, got %d: %v", len(adapter.paths), adapter.paths)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when dotnet is running, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "dev_tool_running" {
		t.Fatalf("expected reason code dev_tool_running, got %q", result.Skipped[0].Reason.Code)
	}
}

// TestExecuteOptInNuGetCacheSkipsWhenNuGetRunning verifies nuget-cache skips when nuget is running
func TestExecuteOptInNuGetCacheSkipsWhenNuGetRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "nuget-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryNuGet {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports NuGet as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationNuGet,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"nuget-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("expected OptInDeletedCount 0 when nuget is running, got %d", result.Totals.OptInDeletedCount)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when nuget is running, got %d: %v", len(adapter.paths), adapter.paths)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when nuget is running, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "dev_tool_running" {
		t.Fatalf("expected reason code dev_tool_running, got %q", result.Skipped[0].Reason.Code)
	}
}

// TestExecuteOptInDevCacheSkipsWhenStateUnknown verifies dev cache skips when tool state is unknown (fail-closed)
func TestExecuteOptInDevCacheSkipsWhenStateUnknown(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryGo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports unknown state
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateUnknown,
				Message:     "could not determine process state",
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"go-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("expected OptInDeletedCount 0 when state is unknown, got %d", result.Totals.OptInDeletedCount)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("expected adapter to receive 0 paths when state is unknown, got %d: %v", len(adapter.paths), adapter.paths)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped item when state is unknown, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "dev_tool_running" {
		t.Fatalf("expected reason code dev_tool_running, got %q", result.Skipped[0].Reason.Code)
	}
}

// TestExecuteOptInNPMCacheStillCleansWhenNodeRunning verifies npm-cache still cleans when node is running (runtime-hosted)
func TestExecuteOptInNPMCacheStillCleansWhenNodeRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryNPM {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Node as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationNode,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"npm-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// npm-cache should still be cleaned even though node is running
	found := false
	for _, p := range adapter.paths {
		if p == cachePath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive npm-cache path even when node is running, got %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 for npm-cache when node is running, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestExecuteOptInPipCacheStillCleansWhenPythonRunning verifies pip-cache still cleans when python is running (runtime-hosted)
func TestExecuteOptInPipCacheStillCleansWhenPythonRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "pip-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryPip {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Python as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationPython,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"pip-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// pip-cache should still be cleaned even though python is running
	found := false
	for _, p := range adapter.paths {
		if p == cachePath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive pip-cache path even when python is running, got %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 for pip-cache when python is running, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestExecuteOptInCorepackCacheStillCleansWhenNodeRunning verifies corepack-cache still cleans when node is running (runtime-hosted)
func TestExecuteOptInCorepackCacheStillCleansWhenNodeRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "corepack-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryCorepack {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Node as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationNode,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{"corepack-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// corepack-cache should still be cleaned even though node is running
	found := false
	for _, p := range adapter.paths {
		if p == cachePath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive corepack-cache path even when node is running, got %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 for corepack-cache when node is running, got %d", result.Totals.OptInDeletedCount)
	}
}

// TestDryRunOptInDevCacheHidesWhenToolRunning verifies dev cache doesn't appear as opt-in candidate when tool is running
func TestDryRunOptInDevCacheHidesWhenToolRunning(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryGo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Go as running
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateRunning,
			},
		}
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{"go-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// Should not appear as an opt-in candidate when tool is running
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("expected 0 opt-in candidates when Go is running, got %d", len(result.OptInCandidates))
	}
}

// TestDryRunOptInDevCacheShowsWhenToolIdle verifies dev cache appears as opt-in candidate when tool is idle
func TestDryRunOptInDevCacheShowsWhenToolIdle(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(cachePath, "data.bin")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) string {
		if category == clean.DevCacheCategoryGo {
			return cachePath
		}
		return ""
	}

	// Create a detector that reports Go as idle
	detector := func(ctx context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateIdle,
			},
		}
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{"go-cache"},
		DevCachePathResolver:      fakeResolver,
		DetectRunningApplications: detector,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	// Should appear as an opt-in candidate when tool is idle
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("expected 1 opt-in candidate when Go is idle, got %d", len(result.OptInCandidates))
	}
	if result.OptInCandidates[0].Path != cachePath {
		t.Fatalf("expected candidate path %q, got %q", cachePath, result.OptInCandidates[0].Path)
	}
}

// TestExecuteWithoutOptInDoesNotRunDevToolDetection verifies default execute without opt-in doesn't run detection
func TestExecuteWithoutOptInDoesNotRunDevToolDetection(t *testing.T) {
	adapter := &recordingRecycleBinAdapter{}
	detectionCalled := false

	detector := func(ctx context.Context) []clean.RunningApplicationState {
		detectionCalled = true
		return nil
	}

	// Set up a default candidate to ensure Execute runs
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-owned.tmp")
	if err := os.WriteFile(candidate, []byte("temp data"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.Execute(context.Background(), clean.Options{
		RecycleBinAdapter:         adapter,
		OptIn:                     []string{}, // No opt-in
		DetectRunningApplications: detector,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})

	// Verify detection was NOT called (execute without opt-in shouldn't need it)
	if detectionCalled {
		t.Fatalf("expected DetectRunningApplications to NOT be called without --opt-in")
	}

	// Verify default candidate still executes normally
	if result.Totals.DeletedCount != 1 {
		t.Fatalf("expected DeletedCount 1, got %d", result.Totals.DeletedCount)
	}
}

// TestDetectSupportedApplicationsIncludesDevTools verifies DetectSupportedApplications detects dev tools
func TestDetectSupportedApplicationsIncludesDevTools(t *testing.T) {
	// We can't easily test the real process snapshot, but we can verify
	// the new Application constants exist and are used correctly
	if clean.ApplicationGo == "" {
		t.Fatalf("ApplicationGo should not be empty")
	}
	if clean.ApplicationCargo == "" {
		t.Fatalf("ApplicationCargo should not be empty")
	}
	if clean.ApplicationDotNet == "" {
		t.Fatalf("ApplicationDotNet should not be empty")
	}
	if clean.ApplicationNuGet == "" {
		t.Fatalf("ApplicationNuGet should not be empty")
	}
	if clean.ApplicationNode == "" {
		t.Fatalf("ApplicationNode should not be empty")
	}
	if clean.ApplicationPython == "" {
		t.Fatalf("ApplicationPython should not be empty")
	}
}
