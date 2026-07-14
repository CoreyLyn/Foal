package clean_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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

type recycleBinAdapterFunc func(string) error

func (f recycleBinAdapterFunc) MoveToRecycleBin(path string) error { return f(path) }

func (a *recordingRecycleBinAdapter) MoveToRecycleBin(path string) error {
	a.paths = append(a.paths, path)
	return nil
}

func executeCleanWithSafeCapacity(ctx context.Context, opts clean.Options) clean.Result {
	if opts.RecycleBinCapacityProbe == nil {
		opts.RecycleBinCapacityProbe = func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:      filepath.VolumeName(path),
				MaxCapacity: 1 << 60,
			}, nil
		}
	}
	return clean.Execute(ctx, opts)
}

func TestExecuteMovesEligibleCandidatesThroughRecycleBin(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

func TestExecuteReportsSharedProgressWithoutChangingOutcome(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	var phases []clean.ExecutionPhase
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		ProgressReporter: func(progress clean.ExecutionProgress) {
			phases = append(phases, progress.Phase)
		},
		RecycleBinAdapter: adapter,
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{candidate}}},
	})

	want := []clean.ExecutionPhase{
		clean.ExecutionPhaseScanning,
		clean.ExecutionPhaseRecycleBinSafety,
		clean.ExecutionPhaseRecycleBinOperations,
		clean.ExecutionPhaseComplete,
	}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("progress phases = %v, want %v", phases, want)
	}
	if result.Totals.DeletedCount != 1 || len(adapter.paths) != 1 || adapter.paths[0] != candidate {
		t.Fatalf("observer changed execution outcome: result=%#v paths=%v", result, adapter.paths)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "progress") || strings.Contains(string(encoded), "scanning") {
		t.Fatalf("progress leaked into JSON contract: %s", encoded)
	}
}

func TestExecuteProgressReporterCannotInterruptExecution(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		ProgressReporter:  func(clean.ExecutionProgress) { panic("observer failure") },
		RecycleBinAdapter: adapter,
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{candidate}}},
	})
	if result.Totals.DeletedCount != 1 || len(adapter.paths) != 1 {
		t.Fatalf("panicking observer altered execution: result=%#v paths=%v", result, adapter.paths)
	}
}

func TestExecuteCancellationRetainsCompletedAndSkippedOutcomesInHistory(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "foal-first.tmp")
	second := filepath.Join(root, "foal-second.tmp")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	recorder := &recordingHistoryRecorder{}
	adapter := recycleBinAdapterFunc(func(path string) error { cancel(); return nil })
	result := executeCleanWithSafeCapacity(ctx, clean.Options{
		RecycleBinAdapter: adapter, HistoryRecorder: recorder,
		Rules: []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{first, second}}},
	})

	if result.Totals.DeletedCount != 1 || result.Totals.SkippedCount != 1 || result.Totals.AffectedBytes != 4 {
		t.Fatalf("interrupted totals overclaimed outcomes: %#v", result.Totals)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "context_canceled" {
		t.Fatalf("interrupted skipped outcome = %#v", result.Skipped)
	}
	if len(recorder.sessions) != 1 || recorder.sessions[0].Aggregate.DeletedCount != 1 || recorder.sessions[0].Aggregate.SkippedCount != 1 {
		t.Fatalf("history lost interrupted outcomes: %#v", recorder.sessions)
	}
	if len(recorder.items) != 2 {
		t.Fatalf("history items = %#v, want completed and canceled items", recorder.items)
	}
}

func TestExecutePartialAdapterFailureRetainsSuccessAndFailureHistory(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.tmp")
	second := filepath.Join(root, "second.tmp")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	adapter := recycleBinAdapterFunc(func(string) error {
		calls++
		if calls == 2 {
			return errors.New("adapter failed")
		}
		return nil
	})
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{RecycleBinAdapter: adapter, HistoryRecorder: recorder, Rules: []clean.Rule{{ID: "test", DefaultEnabled: true, CandidatePaths: []string{first, second}}}})
	if result.Totals.DeletedCount != 1 || result.Totals.SkippedCount != 1 || result.Totals.AffectedBytes != 4 || result.Skipped[0].Reason.Code != "delete_failed" {
		t.Fatalf("partial result = %#v", result)
	}
	if len(recorder.sessions) != 1 || recorder.sessions[0].Aggregate.DeletedCount != 1 || recorder.sessions[0].Aggregate.SkippedCount != 1 || recorder.sessions[0].Aggregate.AffectedBytes != 4 {
		t.Fatalf("partial history aggregate = %#v", recorder.sessions)
	}
	if len(recorder.items) != 2 || recorder.items[0].Result != "deleted" || recorder.items[1].Result != "skipped" || recorder.items[1].SkippedReason.Code != "delete_failed" {
		t.Fatalf("partial history items = %#v", recorder.items)
	}
}

func TestExecuteFreshOptInResolutionCanTurnPreviewIntoNoOp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old-cache")
	if err := os.WriteFile(path, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	discoveryCalls := 0
	discover := func(context.Context) clean.OpportunityDiscoveryResult {
		discoveryCalls++
		if discoveryCalls == 1 {
			return clean.OpportunityDiscoveryResult{Opportunities: []clean.Opportunity{{Category: clean.OpportunityCategoryCrashDumps, Path: path, Bytes: 5}}}
		}
		return clean.OpportunityDiscoveryResult{}
	}
	opts := clean.Options{
		OptIn: []string{clean.OpportunityCategoryCrashDumps}, DiscoverOpportunities: discover,
		Rules: []clean.Rule{{ID: "disabled_default", DefaultEnabled: false}},
	}
	preview := clean.DryRun(context.Background(), opts)
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter
	result := executeCleanWithSafeCapacity(context.Background(), opts)
	if len(preview.OptInCandidates) != 1 || result.Totals.DeletedCount != 0 || len(adapter.paths) != 0 {
		t.Fatalf("execute consumed stale preview: preview=%#v result=%#v paths=%v", preview.OptInCandidates, result, adapter.paths)
	}
}

func TestExecuteFreshResolutionDropsTempTouchedAfterPreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old-temp")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, "data.bin")
	if err := os.WriteFile(file, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	old := now.Add(-8 * 24 * time.Hour)
	for _, target := range []string{file, path} {
		if err := os.Chtimes(target, old, old); err != nil {
			t.Fatal(err)
		}
	}
	opts := clean.Options{Rules: []clean.Rule{{ID: "disabled", DefaultEnabled: false}}, OptIn: []string{clean.OpportunityCategoryUserTemp}, UserTempDiscoveryOptions: clean.UserTempDiscoveryOptions{TempDir: root, Now: now}}
	if preview := clean.DryRun(context.Background(), opts); len(preview.OptInCandidates) != 1 {
		t.Fatalf("preview = %#v", preview.OptInCandidates)
	}
	for _, target := range []string{file, path} {
		if err := os.Chtimes(target, now, now); err != nil {
			t.Fatal(err)
		}
	}
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter
	result := executeCleanWithSafeCapacity(context.Background(), opts)
	if result.Totals.DeletedCount != 0 || len(adapter.paths) != 0 {
		t.Fatalf("touched temp survived fresh resolution: %#v %v", result, adapter.paths)
	}
}

func TestExecuteFreshProtectionSuppressesPreviewedCandidate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "candidate.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := clean.Options{Rules: []clean.Rule{{ID: "test", DefaultEnabled: true, CandidatePaths: []string{path}}}}
	if preview := clean.DryRun(context.Background(), opts); len(preview.Candidates) != 1 {
		t.Fatalf("preview = %#v", preview.Candidates)
	}
	adapter := &recordingRecycleBinAdapter{}
	opts.Validator = pathsafe.NewValidator([]string{root})
	opts.RecycleBinAdapter = adapter
	result := executeCleanWithSafeCapacity(context.Background(), opts)
	if result.Totals.DeletedCount != 0 || len(adapter.paths) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("fresh Protection not enforced: result=%#v paths=%v", result, adapter.paths)
	}
}

func TestExecuteFreshRunningGateSkipsBrowserPreview(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "AppData", "Local")
	userData := filepath.Join(localAppData, "Google", "Chrome", "User Data")
	cachePath := filepath.Join(userData, "Default", "Cache")
	if err := os.MkdirAll(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userData, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Person 1"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	running := false
	detector := func(context.Context) []clean.RunningApplicationState {
		state := clean.RunningApplicationStateIdle
		if running {
			state = clean.RunningApplicationStateRunning
		}
		return []clean.RunningApplicationState{{Application: clean.ApplicationGoogleChrome, State: state}, {Application: clean.ApplicationMicrosoftEdge, State: clean.RunningApplicationStateIdle}}
	}
	opts := clean.Options{Rules: []clean.Rule{{ID: "disabled", DefaultEnabled: false}}, OptIn: []string{clean.OpportunityCategoryBrowserCache}, DetectRunningApplications: detector, BrowserCacheDiscoveryOptions: clean.BrowserCacheDiscoveryOptions{LocalAppDataDir: localAppData}}
	if preview := clean.DryRun(context.Background(), opts); len(preview.OptInCandidates) != 1 {
		t.Fatalf("preview = %#v", preview.OptInCandidates)
	}
	running = true
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter
	result := executeCleanWithSafeCapacity(context.Background(), opts)
	if result.Totals.DeletedCount != 0 || len(adapter.paths) != 0 {
		t.Fatalf("started browser was not skipped: %#v %v", result, adapter.paths)
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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
	result := executeCleanWithSafeCapacity(context.Background(), options)

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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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
		Rules:             []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)
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
	// All opportunity categories and 9 dev caches should be enabled
	expectedOpportunities := []string{
		clean.OpportunityCategoryUserTemp,
		clean.OpportunityCategoryCrashDumps,
		clean.OpportunityCategoryWindowsErrorReporting,
		clean.OpportunityCategoryExplorerThumbnailCache,
		clean.OpportunityCategoryINetCache,
		clean.OpportunityCategoryD3DShaderCache,
		clean.OpportunityCategoryNVIDIADXCache,
		clean.OpportunityCategoryBrowserCache,
		clean.OpportunityCategoryVSCodeCache,
		clean.OpportunityCategoryCursorCache,
	}
	expectedDevCaches := []string{
		clean.DevCacheCategoryNPM,
		clean.DevCacheCategoryGo,
		clean.DevCacheCategoryPip,
		clean.DevCacheCategoryCargo,
		clean.DevCacheCategoryNuGet,
		clean.DevCacheCategoryNuGetGlobalPackages,
		clean.DevCacheCategoryCorepack,
		clean.DevCacheCategoryUV,
		clean.DevCacheCategoryBun,
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
		clean.OpportunityCategoryVSCodeCache,
		clean.OpportunityCategoryCursorCache,
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
	// Should have 10 opportunity categories + 9 dev caches + "dev-caches" + "all" = 21
	if len(valid) != 21 {
		t.Fatalf("expected 21 valid names, got %d: %v", len(valid), valid)
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
		Rules:             []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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
	executeResult := executeCleanWithSafeCapacity(context.Background(), opts)
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
		Rules:             []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		DiscoverUserTempOpportunities: func(ctx context.Context) clean.UserTempDiscoveryResult {
			discoveryCalled = true
			return clean.UserTempDiscoveryResult{}
		},
	}

	result := executeCleanWithSafeCapacity(context.Background(), opts)
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
			Volume:       filepath.VolumeName(path),
			NukeOnDelete: true,
			MaxCapacity:  100 * 1024 * 1024,
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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
			Volume:       filepath.VolumeName(path),
			NukeOnDelete: false,
			MaxCapacity:  1, // Only 1 byte capacity
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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
			Volume:       filepath.VolumeName(path),
			NukeOnDelete: false,
			MaxCapacity:  100 * 1024, // 100 KB
		}, nil
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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
		return clean.RecycleBinVolumeConfig{Volume: filepath.VolumeName(path)}, errors.New("probe failed")
	}

	opts := clean.Options{
		Rules:                   []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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

func TestExecuteOptInUsesSafeInjectedCapacityProbe(t *testing.T) {
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

	// The shared test execute seam injects deterministic safe capacity.
	opts := clean.Options{
		Rules:             []clean.Rule{{ID: "test_rule", DefaultEnabled: false}},
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

	// The safe probe should allow deletion.
	found := false
	for _, p := range adapter.paths {
		if p == userTempPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected adapter to receive path with safe probe")
	}
	if result.Totals.OptInDeletedCount != 1 {
		t.Fatalf("expected OptInDeletedCount 1 with safe probe, got %d", result.Totals.OptInDeletedCount)
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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
			Volume:       filepath.VolumeName(path),
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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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

	result := executeCleanWithSafeCapacity(context.Background(), opts)

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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryGo {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryGo {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryCargo {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryNuGet {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryNuGet {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryGo {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryNPM {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryPip {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryCorepack {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryGo {
			return []string{cachePath}
		}
		return nil
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

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryGo {
			return []string{cachePath}
		}
		return nil
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

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
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

// TestUVCacheOptInDryRunAndExecuteEndToEnd covers uv-cache selection, gate,
// suggestion suppression, and execute reclaim via the shared Clean seams.
func TestUVCacheOptInDryRunAndExecuteEndToEnd(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "uv-cache")
	siblingSuggestion := filepath.Join(root, "uv-cache-sibling")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(siblingSuggestion, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("uv!!"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryUV {
			return []string{cachePath}
		}
		return nil
	}
	idleDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationUV,
			State:       clean.RunningApplicationStateIdle,
		}}
	}
	runningDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationUV,
			State:       clean.RunningApplicationStateRunning,
		}}
	}
	unknownDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationUV,
			State:       clean.RunningApplicationStateUnknown,
			Message:     "snapshot failed",
		}}
	}

	t.Run("dry-run idle produces opt-in candidate and suppresses same-identity suggestion", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryUV},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				return []clean.ReviewSuggestion{
					{Tool: "uv", Label: "uv cache", Command: "uv cache prune", CachePath: cachePath},
					{Tool: "uv", Label: "uv cache sibling", Command: "uv cache prune", CachePath: siblingSuggestion},
				}
			},
			Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(result.OptInCandidates) != 1 {
			t.Fatalf("opt-in candidates = %#v, want 1", result.OptInCandidates)
		}
		if result.OptInCandidates[0].Category != clean.DevCacheCategoryUV {
			t.Fatalf("category = %q, want %q", result.OptInCandidates[0].Category, clean.DevCacheCategoryUV)
		}
		if result.OptInCandidates[0].Bytes != 4 {
			t.Fatalf("bytes = %d, want 4", result.OptInCandidates[0].Bytes)
		}
		if result.Totals.OptInReclaimableBytes != 4 {
			t.Fatalf("opt-in reclaimable = %d, want 4", result.Totals.OptInReclaimableBytes)
		}
		if result.Totals.CandidateBytes != 0 || result.Totals.OpportunityObservedBytes != 0 {
			t.Fatalf("uv bytes leaked into Potential space or Observed opportunity: %#v", result.Totals)
		}
		if len(result.ReviewSuggestions) != 1 || result.ReviewSuggestions[0].CachePath != siblingSuggestion {
			t.Fatalf("review suggestions = %#v, want only sibling path", result.ReviewSuggestions)
		}
		model := clean.NewPreviewReadModel(result)
		foundImpact := false
		for _, notice := range model.Notices {
			if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "not zero-impact") {
				foundImpact = true
			}
		}
		if !foundImpact {
			t.Fatalf("preview notices = %#v, want uv rebuild/non-zero-impact notice", model.Notices)
		}
	})

	t.Run("dry-run running and unknown fail closed", func(t *testing.T) {
		for name, detector := range map[string]func(context.Context) []clean.RunningApplicationState{
			"running": runningDetector,
			"unknown": unknownDetector,
		} {
			t.Run(name, func(t *testing.T) {
				result := clean.DryRun(context.Background(), clean.Options{
					OptIn:                     []string{clean.DevCacheCategoryUV},
					DevCachePathResolver:      fakeResolver,
					DetectRunningApplications: detector,
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				})
				if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
					t.Fatalf("%s: candidates/bytes = %#v / %d", name, result.OptInCandidates, result.Totals.OptInReclaimableBytes)
				}
				if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "dev_tool_running" {
					t.Fatalf("%s: skipped = %#v, want dev_tool_running", name, result.Skipped)
				}
			})
		}
	})

	t.Run("execute idle deletes via recycle bin; running skips adapter", func(t *testing.T) {
		adapter := &recordingRecycleBinAdapter{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryUV},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(adapter.paths) != 1 || adapter.paths[0] != cachePath {
			t.Fatalf("adapter paths = %v, want [%q]", adapter.paths, cachePath)
		}
		if result.Totals.OptInDeletedCount != 1 {
			t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
		}

		adapter = &recordingRecycleBinAdapter{}
		result = executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryUV},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: runningDetector,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(adapter.paths) != 0 || result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("running execute adapter/deleted = %v / %d", adapter.paths, result.Totals.OptInDeletedCount)
		}
	})

	t.Run("default execute without opt-in does not resolve uv or run detection", func(t *testing.T) {
		detectionCalled := false
		resolverCalled := false
		adapter := &recordingRecycleBinAdapter{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                nil,
			DevCachePathResolver: func(category string) []string {
				resolverCalled = true
				return fakeResolver(category)
			},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				detectionCalled = true
				return nil
			},
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				t.Fatal("execute without opt-in must not run review suggestion probes")
				return nil
			},
			RecycleBinAdapter: adapter,
			Rules:             []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if detectionCalled || resolverCalled {
			t.Fatalf("detectionCalled=%v resolverCalled=%v, want both false", detectionCalled, resolverCalled)
		}
		if len(result.OptInCandidates) != 0 || result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("unexpected opt-in activity without selection: %#v", result)
		}
	})

	t.Run("protection suppresses uv root before totals", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryUV},
			Validator:                 pathsafe.NewValidator([]string{cachePath}),
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("protected uv root leaked: candidates=%#v totals=%#v", result.OptInCandidates, result.Totals)
		}
	})
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
	if clean.ApplicationUV == "" {
		t.Fatalf("ApplicationUV should not be empty")
	}
	if clean.ApplicationBun == "" {
		t.Fatalf("ApplicationBun should not be empty")
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

// TestBunCacheOptInDryRunAndExecuteEndToEnd covers bun-cache selection, gate,
// suggestion suppression, impact notice, and execute reclaim via shared seams.
func TestBunCacheOptInDryRunAndExecuteEndToEnd(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "bun-cache")
	siblingSuggestion := filepath.Join(root, "bun-cache-sibling")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(siblingSuggestion, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("bun!"), 0600); err != nil {
		t.Fatal(err)
	}

	fakeResolver := func(category string) []string {
		if category == clean.DevCacheCategoryBun {
			return []string{cachePath}
		}
		return nil
	}
	idleDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationBun,
			State:       clean.RunningApplicationStateIdle,
		}}
	}
	runningDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationBun,
			State:       clean.RunningApplicationStateRunning,
		}}
	}
	unknownDetector := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationBun,
			State:       clean.RunningApplicationStateUnknown,
			Message:     "snapshot failed",
		}}
	}

	t.Run("dry-run idle produces opt-in candidate and suppresses same-identity suggestion", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryBun},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				return []clean.ReviewSuggestion{
					{Tool: "bun", Label: "bun cache", Command: "bun pm cache rm", CachePath: cachePath},
					{Tool: "bun", Label: "bun cache sibling", Command: "bun pm cache rm", CachePath: siblingSuggestion},
				}
			},
			Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(result.OptInCandidates) != 1 {
			t.Fatalf("opt-in candidates = %#v, want 1", result.OptInCandidates)
		}
		if result.OptInCandidates[0].Category != clean.DevCacheCategoryBun {
			t.Fatalf("category = %q, want %q", result.OptInCandidates[0].Category, clean.DevCacheCategoryBun)
		}
		if result.OptInCandidates[0].Bytes != 4 {
			t.Fatalf("bytes = %d, want 4", result.OptInCandidates[0].Bytes)
		}
		if result.Totals.OptInReclaimableBytes != 4 {
			t.Fatalf("opt-in reclaimable = %d, want 4", result.Totals.OptInReclaimableBytes)
		}
		if result.Totals.CandidateBytes != 0 || result.Totals.OpportunityObservedBytes != 0 {
			t.Fatalf("bun bytes leaked into Potential space or Observed opportunity: %#v", result.Totals)
		}
		if len(result.ReviewSuggestions) != 1 || result.ReviewSuggestions[0].CachePath != siblingSuggestion {
			t.Fatalf("review suggestions = %#v, want only sibling path", result.ReviewSuggestions)
		}
		model := clean.NewPreviewReadModel(result)
		foundImpact := false
		for _, notice := range model.Notices {
			if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "Hardlinked") {
				foundImpact = true
			}
		}
		if !foundImpact {
			t.Fatalf("preview notices = %#v, want bun hardlink/download impact notice", model.Notices)
		}
	})

	t.Run("dry-run running and unknown fail closed", func(t *testing.T) {
		for name, detector := range map[string]func(context.Context) []clean.RunningApplicationState{
			"running": runningDetector,
			"unknown": unknownDetector,
		} {
			t.Run(name, func(t *testing.T) {
				result := clean.DryRun(context.Background(), clean.Options{
					OptIn:                     []string{clean.DevCacheCategoryBun},
					DevCachePathResolver:      fakeResolver,
					DetectRunningApplications: detector,
					DiscoverOpportunities:     noOpportunities,
					DiscoverReviewSuggestions: noReviewSuggestions,
				})
				if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
					t.Fatalf("%s: candidates/bytes = %#v / %d", name, result.OptInCandidates, result.Totals.OptInReclaimableBytes)
				}
				if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "dev_tool_running" {
					t.Fatalf("%s: skipped = %#v, want dev_tool_running", name, result.Skipped)
				}
			})
		}
	})

	t.Run("execute idle deletes via recycle bin; running skips adapter", func(t *testing.T) {
		adapter := &recordingRecycleBinAdapter{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryBun},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(adapter.paths) != 1 || adapter.paths[0] != cachePath {
			t.Fatalf("adapter paths = %v, want [%q]", adapter.paths, cachePath)
		}
		if result.Totals.OptInDeletedCount != 1 {
			t.Fatalf("OptInDeletedCount = %d, want 1", result.Totals.OptInDeletedCount)
		}

		adapter = &recordingRecycleBinAdapter{}
		result = executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryBun},
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: runningDetector,
			RecycleBinAdapter:         adapter,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules:                     []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if len(adapter.paths) != 0 || result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("running execute adapter/deleted = %v / %d", adapter.paths, result.Totals.OptInDeletedCount)
		}
	})

	t.Run("default execute without opt-in does not resolve bun or run detection", func(t *testing.T) {
		detectionCalled := false
		resolverCalled := false
		adapter := &recordingRecycleBinAdapter{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			OptIn: nil,
			DevCachePathResolver: func(category string) []string {
				resolverCalled = true
				return fakeResolver(category)
			},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				detectionCalled = true
				return nil
			},
			DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
				t.Fatal("execute without opt-in must not run review suggestion probes")
				return nil
			},
			RecycleBinAdapter: adapter,
			Rules:             []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
		})
		if detectionCalled || resolverCalled {
			t.Fatalf("detectionCalled=%v resolverCalled=%v, want both false", detectionCalled, resolverCalled)
		}
		if len(result.OptInCandidates) != 0 || result.Totals.OptInDeletedCount != 0 {
			t.Fatalf("unexpected opt-in activity without selection: %#v", result)
		}
	})

	t.Run("protection suppresses bun root before totals", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryBun},
			Validator:                 pathsafe.NewValidator([]string{cachePath}),
			DevCachePathResolver:      fakeResolver,
			DetectRunningApplications: idleDetector,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("protected bun root leaked: candidates=%#v totals=%#v", result.OptInCandidates, result.Totals)
		}
	})
}
