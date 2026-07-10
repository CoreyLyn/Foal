package clean_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestExecuteSkipsVolumeWhenCandidatesCollectivelyExceedRemainingRecycleBinCapacity(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.tmp")
	second := filepath.Join(root, "second.tmp")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("four"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	adapter := &recordingRecycleBinAdapter{}
	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{first, second},
		}},
		RecycleBinAdapter: adapter,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       "test-volume",
				MaxCapacity:  7,
				CurrentUsage: 0,
			}, nil
		},
	})

	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v, want none for unsafe volume", adapter.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none for unsafe volume", result.Deleted)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %#v, want both volume candidates", result.Skipped)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code != "recycle_bin_capacity" {
			t.Fatalf("skip reason = %q, want recycle_bin_capacity", skipped.Reason.Code)
		}
	}
}

func TestExecuteAccountsForCurrentUsageAcrossDefaultAndOptInCandidates(t *testing.T) {
	root := t.TempDir()
	defaultCandidate := writeCapacityTestFile(t, root, "default.tmp", "four")
	optInCandidate := filepath.Join(root, "old-temp")
	if err := os.Mkdir(optInCandidate, 0700); err != nil {
		t.Fatal(err)
	}
	writeCapacityTestFile(t, optInCandidate, "data.tmp", "four")
	old := time.Now().AddDate(0, 0, -8)

	adapter := &recordingRecycleBinAdapter{}
	result := clean.Execute(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{defaultCandidate},
		}},
		OptIn:             []string{clean.OpportunityCategoryUserTemp},
		RecycleBinAdapter: adapter,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: []clean.UserTempOpportunity{{
				Category:         clean.OpportunityCategoryUserTemp,
				Path:             optInCandidate,
				Bytes:            4,
				LatestModifiedAt: old,
				IdleDays:         8,
				Status:           clean.UserTempOpportunityStatus,
				Reason:           clean.UserTempOpportunityReason,
			}}}
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "test-volume", MaxCapacity: 10, CurrentUsage: 3}, nil
		},
	})

	if len(adapter.paths) != 0 || len(result.Deleted) != 0 {
		t.Fatalf("adapter/deleted = %v/%#v, want no deletion when 8 selected bytes exceed 7 remaining", adapter.paths, result.Deleted)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %#v, want default and opt-in candidates skipped together", result.Skipped)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code != "recycle_bin_capacity" {
			t.Fatalf("skip reason = %q, want recycle_bin_capacity", skipped.Reason.Code)
		}
	}
}

func TestExecuteSkipsAllCandidatesWhenRecycleBinIsDisabledForVolume(t *testing.T) {
	root := t.TempDir()
	first := writeCapacityTestFile(t, root, "first.tmp", "one")
	second := writeCapacityTestFile(t, root, "second.tmp", "two")
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{first, second}}},
		RecycleBinAdapter: adapter,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "disabled-volume", NukeOnDelete: true, MaxCapacity: 100}, nil
		},
	})

	if len(adapter.paths) != 0 || len(result.Deleted) != 0 || len(result.Skipped) != 2 {
		t.Fatalf("adapter/deleted/skipped = %v/%#v/%#v, want two skips and no deletion", adapter.paths, result.Deleted, result.Skipped)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code != "recycle_bin_disabled" {
			t.Fatalf("skip reason = %q, want recycle_bin_disabled", skipped.Reason.Code)
		}
	}
}

func TestExecuteSkipsWholeVolumeWhenOneCapacityProbeIsUnknown(t *testing.T) {
	root := t.TempDir()
	first := writeCapacityTestFile(t, root, "first.tmp", "one")
	second := writeCapacityTestFile(t, root, "second.tmp", "two")
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{first, second}}},
		RecycleBinAdapter: adapter,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			config := clean.RecycleBinVolumeConfig{Volume: "test-volume", MaxCapacity: 100}
			if path == second {
				return config, errors.New("usage unavailable")
			}
			return config, nil
		},
	})

	if len(adapter.paths) != 0 || len(result.Deleted) != 0 || len(result.Skipped) != 2 {
		t.Fatalf("adapter/deleted/skipped = %v/%#v/%#v, want whole unknown volume skipped", adapter.paths, result.Deleted, result.Skipped)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code != "recycle_bin_capacity_probe_failed" {
			t.Fatalf("skip reason = %q, want recycle_bin_capacity_probe_failed", skipped.Reason.Code)
		}
	}
}

func TestExecuteFailsClosedWhenCapacityProbeOmitsVolumeIdentity(t *testing.T) {
	root := t.TempDir()
	candidate := writeCapacityTestFile(t, root, "candidate.tmp", "data")
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{candidate}}},
		RecycleBinAdapter: adapter,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{MaxCapacity: 100, CurrentUsage: 0}, nil
		},
	})

	if len(adapter.paths) != 0 || len(result.Deleted) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("adapter/deleted/skipped = %v/%#v/%#v, want missing-volume fail closed", adapter.paths, result.Deleted, result.Skipped)
	}
	if result.Skipped[0].Reason.Code != "recycle_bin_capacity_probe_failed" {
		t.Fatalf("skip reason = %q, want recycle_bin_capacity_probe_failed", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteContinuesSafeVolumeAndRecordsMixedHistory(t *testing.T) {
	root := t.TempDir()
	safeCandidate := writeCapacityTestFile(t, root, "safe.tmp", "safe")
	unsafeCandidate := writeCapacityTestFile(t, root, "unsafe.tmp", "unsafe")
	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{safeCandidate, unsafeCandidate}}},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			if path == safeCandidate {
				return clean.RecycleBinVolumeConfig{Volume: "safe-volume", MaxCapacity: 100, CurrentUsage: 10}, nil
			}
			return clean.RecycleBinVolumeConfig{Volume: "unsafe-volume", MaxCapacity: 5, CurrentUsage: 1}, nil
		},
	})

	if len(adapter.paths) != 1 || adapter.paths[0] != safeCandidate {
		t.Fatalf("adapter paths = %v, want only safe volume candidate", adapter.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Path != safeCandidate || len(result.Skipped) != 1 || result.Skipped[0].Path != unsafeCandidate {
		t.Fatalf("deleted/skipped = %#v/%#v, want one safe deletion and one unsafe skip", result.Deleted, result.Skipped)
	}
	if result.Totals.DeletedCount != 1 || result.Totals.SkippedCount != 1 || result.Totals.AffectedBytes != 4 {
		t.Fatalf("totals = %#v, want mixed one-deleted/one-skipped result", result.Totals)
	}
	if len(recorder.sessions) != 1 || recorder.sessions[0].Aggregate.DeletedCount != 1 || recorder.sessions[0].Aggregate.SkippedCount != 1 || len(recorder.items) != 2 {
		t.Fatalf("history sessions/items = %#v/%#v, want accurate mixed result", recorder.sessions, recorder.items)
	}
}

func TestExecuteDeletesDefaultAndOptInCandidatesWhenAggregateCapacityIsSafe(t *testing.T) {
	root := t.TempDir()
	defaultCandidate := writeCapacityTestFile(t, root, "default.tmp", "four")
	optInCandidate := filepath.Join(root, "old-temp")
	if err := os.Mkdir(optInCandidate, 0700); err != nil {
		t.Fatal(err)
	}
	writeCapacityTestFile(t, optInCandidate, "data.tmp", "four")
	old := time.Now().AddDate(0, 0, -8)
	adapter := &recordingRecycleBinAdapter{}

	result := clean.Execute(context.Background(), clean.Options{
		Rules:             []clean.Rule{{ID: "test_default_rule", DefaultEnabled: true, CandidatePaths: []string{defaultCandidate}}},
		OptIn:             []string{clean.OpportunityCategoryUserTemp},
		RecycleBinAdapter: adapter,
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{Opportunities: []clean.UserTempOpportunity{{
				Category: clean.OpportunityCategoryUserTemp, Path: optInCandidate, Bytes: 4,
				LatestModifiedAt: old, IdleDays: 8, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason,
			}}}
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "test-volume", MaxCapacity: 20, CurrentUsage: 2}, nil
		},
	})

	if len(adapter.paths) != 2 || len(result.Deleted) != 2 || len(result.Skipped) != 0 {
		t.Fatalf("adapter/deleted/skipped = %v/%#v/%#v, want both candidates deleted", adapter.paths, result.Deleted, result.Skipped)
	}
	if result.Totals.DeletedCount != 2 || result.Totals.OptInDeletedCount != 1 || result.Totals.AffectedBytes != 8 || result.Totals.OptInAffectedBytes != 4 {
		t.Fatalf("totals = %#v, want default plus opt-in success", result.Totals)
	}
}

func writeCapacityTestFile(t *testing.T, root, name, contents string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
