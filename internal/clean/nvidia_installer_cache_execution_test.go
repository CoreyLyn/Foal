package clean_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

// nvidiaExecuteOptions builds Execute options that opt into nvidia_installer_cache
// with a safe aggregate Recycle Bin capacity probe. Callers override the adapter,
// capacity probe, or discovery seams per case.
func nvidiaExecuteOptions(fx nvidiaFixture) clean.Options {
	opts := nvidiaResolveOptions(fx)
	opts.OptIn = []string{clean.CategoryNVIDIAInstallerCache}
	opts.RecycleBinCapacityProbe = func(path string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{
			Volume:      filepath.VolumeName(path),
			MaxCapacity: 1 << 60,
		}, nil
	}
	return opts
}

// TestNVIDIAInstallerCache_ExecuteAllowPermanentKeepsRecycleBin proves
// --allow-permanent neither changes nor authorizes a different action: the
// category stays a Recycle Bin move and never becomes permanent deletion.
func TestNVIDIAInstallerCache_ExecuteAllowPermanentKeepsRecycleBin(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	opts.AllowPermanentDeletion = true
	adapter := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	opts.RecycleBinAdapter = adapter
	opts.PermanentRemover = permanent

	result := clean.Execute(context.Background(), opts)

	if len(adapter.paths) != 1 {
		t.Fatalf("candidate must move through Recycle Bin, adapter = %v", adapter.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("--allow-permanent must not route NVIDIA to permanent deletion: %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("deleted = %#v, want move_to_recycle_bin", result.Deleted)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("permanent totals must stay zero: %#v", result.Totals)
	}
}

// TestNVIDIAInstallerCache_ExecuteCapacityFailureSkipsWithoutPermanentFallback
// proves a Recycle Bin capacity/disabled state is a fail-closed skip that keeps
// the move_to_recycle_bin planned action and never falls back to permanent deletion.
func TestNVIDIAInstallerCache_ExecuteCapacityFailureSkipsWithoutPermanentFallback(t *testing.T) {
	cases := []struct {
		name string
		cfg  clean.RecycleBinVolumeConfig
		code string
	}{
		{"disabled volume", clean.RecycleBinVolumeConfig{Volume: "v", NukeOnDelete: true, MaxCapacity: 1 << 60}, "recycle_bin_disabled"},
		{"over capacity", clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1, CurrentUsage: 0}, "recycle_bin_capacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildValidNVIDIAFixture(t)
			opts := nvidiaExecuteOptions(fx)
			opts.AllowPermanentDeletion = true // must not enable a fallback
			adapter := &recordingRecycleBinAdapter{}
			permanent := &recordingPermanentRemover{}
			opts.RecycleBinAdapter = adapter
			opts.PermanentRemover = permanent
			cfg := tc.cfg
			opts.RecycleBinCapacityProbe = func(string) (clean.RecycleBinVolumeConfig, error) {
				return cfg, nil
			}

			result := clean.Execute(context.Background(), opts)

			if len(adapter.paths) != 0 || len(permanent.paths) != 0 {
				t.Fatalf("no mutation expected: recycle=%v permanent=%v", adapter.paths, permanent.paths)
			}
			if len(result.Deleted) != 0 {
				t.Fatalf("deleted = %#v, want none", result.Deleted)
			}
			found := false
			for _, item := range result.Skipped {
				if item.Rule != clean.CategoryNVIDIAInstallerCache {
					continue
				}
				found = true
				if item.Reason.Code != tc.code {
					t.Fatalf("skip code = %q, want %q", item.Reason.Code, tc.code)
				}
				if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
					t.Fatalf("planned action changed under capacity failure: %q", item.PlannedAction)
				}
			}
			if !found {
				t.Fatalf("expected fail-closed capacity skip, skipped = %#v", result.Skipped)
			}
		})
	}
}

// TestNVIDIAInstallerCache_ExecuteProtectedCandidateNotMoved proves a protected
// task directory is suppressed and never moved.
func TestNVIDIAInstallerCache_ExecuteProtectedCandidateNotMoved(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	opts.Validator = pathsafe.NewValidator([]string{fx.taskDir})
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter

	result := clean.Execute(context.Background(), opts)

	if len(adapter.paths) != 0 {
		t.Fatalf("protected candidate must not move: %v", adapter.paths)
	}
	for _, item := range result.Deleted {
		if item.Rule == clean.CategoryNVIDIAInstallerCache {
			t.Fatalf("protected candidate was deleted: %#v", item)
		}
	}
}

// TestNVIDIAInstallerCache_ExecuteStaleStateSkipsBeforeMove drives the real
// immediate validator: the payload signature verifies during fresh resolution
// but fails on the immediate revalidation call, so the candidate is skipped
// fail-closed and never moved.
func TestNVIDIAInstallerCache_ExecuteStaleStateSkipsBeforeMove(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	var calls atomic.Int32
	opts.NVIDIAInstallerCacheDiscoveryOptions.VerifyPayloadSignature = func(string) error {
		if calls.Add(1) == 1 {
			return nil // fresh resolution accepts
		}
		return errors.New("signature no longer verifies") // immediate revalidation rejects
	}
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter

	result := clean.Execute(context.Background(), opts)

	if len(adapter.paths) != 0 {
		t.Fatalf("stale candidate must not move: %v", adapter.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	found := false
	for _, item := range result.Skipped {
		if item.Rule != clean.CategoryNVIDIAInstallerCache {
			continue
		}
		found = true
		if item.Reason.Code != "nvidia_installer_cache_revalidation_failed" {
			t.Fatalf("skip code = %q, want nvidia_installer_cache_revalidation_failed", item.Reason.Code)
		}
		if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
			t.Fatalf("planned action changed: %q", item.PlannedAction)
		}
	}
	if !found {
		t.Fatalf("expected immediate revalidation skip, skipped = %#v", result.Skipped)
	}
	if calls.Load() < 2 {
		t.Fatalf("immediate validator did not repeat the signature proof: calls = %d", calls.Load())
	}
}

// TestNVIDIAInstallerCache_ExecuteInjectedIdentityRejectionSkips proves the
// Recycle Bin pre-mutation hook is wired: an injected action-neutral validator
// rejection is a fail-closed skip that preserves the planned action and never
// falls back to permanent deletion.
func TestNVIDIAInstallerCache_ExecuteInjectedIdentityRejectionSkips(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	opts.AllowPermanentDeletion = true
	adapter := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	opts.RecycleBinAdapter = adapter
	opts.PermanentRemover = permanent
	opts.CategoryIdentityValidators = map[string]clean.CategoryIdentityValidator{
		clean.CategoryNVIDIAInstallerCache: func(clean.CategoryIdentityCandidate) (pathsafe.Reason, bool) {
			return pathsafe.Reason{Code: "nvidia_installer_cache_revalidation_failed", Message: "identity changed"}, false
		},
	}

	result := clean.Execute(context.Background(), opts)

	if len(adapter.paths) != 0 || len(permanent.paths) != 0 {
		t.Fatalf("rejected candidate must not mutate: recycle=%v permanent=%v", adapter.paths, permanent.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	found := false
	for _, item := range result.Skipped {
		if item.Rule == clean.CategoryNVIDIAInstallerCache && item.Reason.Code == "nvidia_installer_cache_revalidation_failed" {
			found = true
			if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("planned action changed: %q", item.PlannedAction)
			}
		}
	}
	if !found {
		t.Fatalf("expected identity-rejection skip, skipped = %#v", result.Skipped)
	}
}

// TestNVIDIAInstallerCache_ExecuteLocalFailureIsolatedPreservesAction proves an
// adapter (Recycle Bin) failure is an isolated skip that keeps the actual
// move_to_recycle_bin action and never permanently deletes.
func TestNVIDIAInstallerCache_ExecuteLocalFailureIsolatedPreservesAction(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	opts.AllowPermanentDeletion = true
	permanent := &recordingPermanentRemover{}
	opts.PermanentRemover = permanent
	opts.RecycleBinAdapter = recycleBinAdapterFunc(func(string) error {
		return errors.New("recycle bin move failed")
	})
	recorder := &recordingHistoryRecorder{}
	opts.HistoryRecorder = recorder

	result := clean.Execute(context.Background(), opts)

	if len(permanent.paths) != 0 {
		t.Fatalf("adapter failure must not fall back to permanent deletion: %v", permanent.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v, want none", result.Deleted)
	}
	found := false
	for _, item := range result.Skipped {
		if item.Rule == clean.CategoryNVIDIAInstallerCache {
			found = true
			if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("planned action changed on failure: %q", item.PlannedAction)
			}
		}
	}
	if !found {
		t.Fatalf("expected isolated local-failure skip, skipped = %#v", result.Skipped)
	}
	// History preserves the actual action for the skipped move.
	sawSkip := false
	for _, item := range recorder.items {
		if item.Rule == clean.CategoryNVIDIAInstallerCache && item.Result == "skipped" {
			sawSkip = true
			if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("history planned action = %q", item.PlannedAction)
			}
		}
	}
	if !sawSkip {
		t.Fatalf("history missing NVIDIA skip: %#v", recorder.items)
	}
}

// TestNVIDIAInstallerCache_ExecuteCancellationDoesNotMove proves a canceled run
// performs no Recycle Bin move.
func TestNVIDIAInstallerCache_ExecuteCancellationDoesNotMove(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := clean.Execute(ctx, opts)

	if len(adapter.paths) != 0 {
		t.Fatalf("canceled run must not move: %v", adapter.paths)
	}
	for _, item := range result.Deleted {
		if item.Rule == clean.CategoryNVIDIAInstallerCache {
			t.Fatalf("canceled run deleted a candidate: %#v", item)
		}
	}
}

// TestNVIDIAInstallerCache_ExecuteHistoryRecordsRecycleBinAction proves a
// successful move is stored in Result and History with the actual Recycle Bin
// action (never permanent).
func TestNVIDIAInstallerCache_ExecuteHistoryRecordsRecycleBinAction(t *testing.T) {
	fx := buildValidNVIDIAFixture(t)
	opts := nvidiaExecuteOptions(fx)
	adapter := &recordingRecycleBinAdapter{}
	opts.RecycleBinAdapter = adapter
	recorder := &recordingHistoryRecorder{}
	opts.HistoryRecorder = recorder
	opts.CommandParameters = history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute", "--opt-in", clean.CategoryNVIDIAInstallerCache}}

	result := clean.Execute(context.Background(), opts)

	if len(result.Deleted) != 1 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	found := false
	for _, item := range recorder.items {
		if item.Rule == clean.CategoryNVIDIAInstallerCache && item.Result == "deleted" {
			found = true
			if item.Action != string(clean.PlannedActionMoveToRecycleBin) {
				t.Fatalf("history action = %q, want move_to_recycle_bin", item.Action)
			}
			if strings.Contains(strings.ToLower(item.Action), "permanent") {
				t.Fatalf("history must not record permanent deletion: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("history missing NVIDIA deleted move: %#v", recorder.items)
	}
	if recorder.sessions[0].Aggregate.RecycleBinMovedBytes <= 0 || recorder.sessions[0].Aggregate.PermanentlyDeletedBytes != 0 {
		t.Fatalf("history aggregate = %#v", recorder.sessions[0].Aggregate)
	}
}
