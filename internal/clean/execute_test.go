package clean_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
