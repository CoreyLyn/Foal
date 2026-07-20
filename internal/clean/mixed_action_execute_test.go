package clean_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

const (
	testRecycleRule   = "test_recycle_rule"
	testPermanentRule = "test_permanent_rule"
)

type recordingPermanentRemover struct {
	paths []string
}

func (r *recordingPermanentRemover) Remove(_ context.Context, path string) error {
	r.paths = append(r.paths, path)
	return delete.FilesystemPermanentRemover{}.Remove(context.Background(), path)
}

type orderedCall struct {
	kind string
	path string
}

type orderedCollaborators struct {
	calls []orderedCall
}

func (o *orderedCollaborators) MoveToRecycleBin(path string) error {
	o.calls = append(o.calls, orderedCall{kind: "recycle", path: path})
	return nil
}

func (o *orderedCollaborators) Remove(_ context.Context, path string) error {
	o.calls = append(o.calls, orderedCall{kind: "permanent", path: path})
	return delete.FilesystemPermanentRemover{}.Remove(context.Background(), path)
}

type permanentRemoverFunc func(context.Context, string) error

func (f permanentRemoverFunc) Remove(ctx context.Context, path string) error {
	return f(ctx, path)
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mixedActionOpts(t *testing.T, root string, recyclePath, permanentPath string, allowPermanent bool) clean.Options {
	t.Helper()
	return clean.Options{
		AllowPermanentDeletion: allowPermanent,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
			testRecycleRule:   clean.PlannedActionMoveToRecycleBin,
		},
		Rules: []clean.Rule{
			{ID: testRecycleRule, DefaultEnabled: true, CandidatePaths: []string{recyclePath}},
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{permanentPath}},
		},
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:      filepath.VolumeName(path),
				MaxCapacity: 1 << 60,
			}, nil
		},
	}
}

func TestExecuteMixedActionDispatchesCollaboratorsByPlannedAction(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "recycle.tmp", "rbin")
	permanentPath := writeTestFile(t, root, "permanent.tmp", "perm")
	collab := &orderedCollaborators{}
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, true)
	opts.RecycleBinAdapter = collab
	opts.PermanentRemover = collab

	result := clean.Execute(context.Background(), opts)

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want recycle then permanent", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[0].path != recyclePath {
		t.Fatalf("first call = %#v, want recycle %q", collab.calls[0], recyclePath)
	}
	if collab.calls[1].kind != "permanent" || collab.calls[1].path != permanentPath {
		t.Fatalf("second call = %#v, want permanent %q", collab.calls[1], permanentPath)
	}
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	byRule := map[string]clean.DeletedItem{}
	for _, item := range result.Deleted {
		byRule[item.Rule] = item
	}
	if byRule[testRecycleRule].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("recycle action = %#v", byRule[testRecycleRule])
	}
	if byRule[testPermanentRule].Action != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("permanent action = %#v", byRule[testPermanentRule])
	}
	if result.Totals.RecycleBinMovedBytes != 4 || result.Totals.PermanentlyDeletedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if result.Totals.AffectedBytes != 8 {
		t.Fatalf("affected = %d, want 8", result.Totals.AffectedBytes)
	}
	if _, err := os.Lstat(permanentPath); !os.IsNotExist(err) {
		t.Fatalf("permanent path still exists: %v", err)
	}
}

func TestExecuteMixedActionOrderRecycleBinBeforePermanent(t *testing.T) {
	root := t.TempDir()
	// Register permanent first in Rules; execution must still do Recycle Bin first.
	recyclePath := writeTestFile(t, root, "r.tmp", "rb")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd")
	collab := &orderedCollaborators{}
	opts := clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
			testRecycleRule:   clean.PlannedActionMoveToRecycleBin,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{permanentPath}},
			{ID: testRecycleRule, DefaultEnabled: true, CandidatePaths: []string{recyclePath}},
		},
		RecycleBinAdapter: collab,
		PermanentRemover:  collab,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	}
	_ = clean.Execute(context.Background(), opts)
	if len(collab.calls) != 2 || collab.calls[0].kind != "recycle" || collab.calls[1].kind != "permanent" {
		t.Fatalf("order = %#v, want recycle then permanent", collab.calls)
	}
}

func TestExecuteMissingPermanentAuthorizationSkipsWithoutChangingPlannedAction(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "r.tmp", "rb12")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd34")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, false)
	opts.RecycleBinAdapter = recycle
	opts.PermanentRemover = permanent
	opts.HistoryRecorder = recorder

	result := clean.Execute(context.Background(), opts)

	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called: %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skip code = %q", skipped.Reason.Code)
	}
	if skipped.PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("planned action changed: %q", skipped.PlannedAction)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(permanentPath); err != nil {
		t.Fatalf("unauthorized permanent path must remain: %v", err)
	}
	foundAuthSkip := false
	for _, item := range recorder.items {
		if item.Result == "skipped" && item.SkippedReason != nil && item.SkippedReason.Code == "permanent_deletion_not_authorized" {
			foundAuthSkip = true
			if item.PlannedAction != string(clean.PlannedActionDeletePermanently) {
				t.Fatalf("history planned action = %q", item.PlannedAction)
			}
		}
	}
	if !foundAuthSkip {
		t.Fatalf("history missing auth skip: %#v", recorder.items)
	}
}

func TestExecuteCapacityChecksExcludePermanentCandidates(t *testing.T) {
	root := t.TempDir()
	// Recycle candidate is 4 bytes; permanent is 100. Capacity of 10 must accept
	// recycle-only (4) and must not sum permanent into the volume budget.
	recyclePath := writeTestFile(t, root, "r.tmp", "rb12")
	permanentDir := filepath.Join(root, "permanent-tree")
	if err := os.Mkdir(permanentDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(permanentDir, "big.bin"), make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	collab := &orderedCollaborators{}
	opts := clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
			testRecycleRule:   clean.PlannedActionMoveToRecycleBin,
		},
		Rules: []clean.Rule{
			{ID: testRecycleRule, DefaultEnabled: true, CandidatePaths: []string{recyclePath}},
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{permanentDir}},
		},
		RecycleBinAdapter: collab,
		PermanentRemover:  collab,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 10, CurrentUsage: 0}, nil
		},
	}
	result := clean.Execute(context.Background(), opts)
	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want both actions (permanent excluded from capacity)", collab.calls)
	}
	if result.Totals.RecycleBinMovedBytes != 4 || result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestExecuteRecycleBinFailureNeverFallsBackToPermanent(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "r.tmp", "rb")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd")
	permanent := &recordingPermanentRemover{}
	// Only one recycle rule — capacity disable must not authorize permanent
	// deletion for recycle-planned candidates.
	opts := clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testRecycleRule: clean.PlannedActionMoveToRecycleBin,
		},
		Rules: []clean.Rule{
			{ID: testRecycleRule, DefaultEnabled: true, CandidatePaths: []string{recyclePath, permanentPath}},
		},
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		PermanentRemover:  permanent,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", NukeOnDelete: true, MaxCapacity: 1 << 60}, nil
		},
	}
	result := clean.Execute(context.Background(), opts)
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent fallback must not run: %v", permanent.paths)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code != "recycle_bin_disabled" {
			t.Fatalf("skip = %#v", skipped)
		}
		if skipped.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
			t.Fatalf("planned action changed under recycle failure: %q", skipped.PlannedAction)
		}
	}
}

func TestExecuteUnsafeRecycleVolumeDoesNotBlockPermanentSiblings(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "r.tmp", "rb12")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd34")
	collab := &orderedCollaborators{}
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, true)
	opts.RecycleBinAdapter = collab
	opts.PermanentRemover = collab
	opts.RecycleBinCapacityProbe = func(string) (clean.RecycleBinVolumeConfig, error) {
		return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1, CurrentUsage: 0}, nil
	}
	result := clean.Execute(context.Background(), opts)
	if len(collab.calls) != 1 || collab.calls[0].kind != "permanent" {
		t.Fatalf("calls = %#v, want only permanent after recycle capacity skip", collab.calls)
	}
	if result.Totals.PermanentlyDeletedBytes != 4 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "recycle_bin_capacity" {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
}

func TestExecuteGlobalProtectionFailureCallsNeitherCollaborator(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "r.tmp", "rb")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd")
	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, true)
	opts.RecycleBinAdapter = recycle
	opts.PermanentRemover = permanent
	opts.ProtectionLoadError = &clean.StructuredIssue{
		Code:        "protection_file_load_failed",
		Message:     "cannot load protection",
		Recoverable: false,
	}
	result := clean.Execute(context.Background(), opts)
	if result.Status != "error" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(recycle.paths) != 0 || len(permanent.paths) != 0 {
		t.Fatalf("collaborators called under global failure: recycle=%v permanent=%v", recycle.paths, permanent.paths)
	}
}

func TestExecutePermanentImmediateValidationAndIntroducedReparse(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "f.bin"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	permanent := &recordingPermanentRemover{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{link}},
		},
		PermanentRemover: permanent,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("remover called for reparse: %v", permanent.paths)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "reparse_point" {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
}

func TestExecutePermanentProtectionSkipBeforeRemover(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "p.tmp", "data")
	permanent := &recordingPermanentRemover{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		Validator:              pathsafe.NewValidator([]string{root}),
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		PermanentRemover: permanent,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("remover called: %v", permanent.paths)
	}
	// Protected paths are skipped at preview time, so execute has no candidate.
	if result.Totals.DeletedCount != 0 || result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecutePermanentPartialFailureSiblingContinuationAndHistory(t *testing.T) {
	root := t.TempDir()
	first := writeTestFile(t, root, "first.tmp", "aaaa")
	second := writeTestFile(t, root, "second.tmp", "bbbb")
	var calls atomic.Int32
	remover := permanentRemoverFunc(func(_ context.Context, path string) error {
		n := calls.Add(1)
		if n == 1 {
			return errors.New("io fault after start")
		}
		return delete.FilesystemPermanentRemover{}.Remove(context.Background(), path)
	})
	recorder := &recordingHistoryRecorder{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{first, second}},
		},
		PermanentRemover:  remover,
		HistoryRecorder:   recorder,
		CommandParameters: history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute"}},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})

	if len(result.Deleted) != 1 || result.Deleted[0].Path != second {
		t.Fatalf("deleted = %#v, want sibling success", result.Deleted)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("failed = %#v", result.Failed)
	}
	failed := result.Failed[0]
	if failed.Reason.Code != "permanent_delete_failed" || failed.Bytes != 4 {
		t.Fatalf("failed item = %#v", failed)
	}
	if failed.Action != string(clean.PlannedActionDeletePermanently) || failed.PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("failed actions = %#v", failed)
	}
	if !strings.Contains(failed.Reason.Message, "may already be permanently deleted") {
		t.Fatalf("missing partial-risk warning: %q", failed.Reason.Message)
	}
	if result.Totals.PermanentlyDeletedBytes != 4 || result.Totals.AffectedBytes != 4 {
		t.Fatalf("totals must exclude failed permanent bytes: %#v", result.Totals)
	}
	// No Recycle Bin fallback on failure.
	if result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("recycle fallback: %#v", result.Totals)
	}

	foundFailed := false
	for _, item := range recorder.items {
		if item.Result == "failed" {
			foundFailed = true
			if item.Error == nil || item.Error.Code != "permanent_delete_failed" {
				t.Fatalf("history failed item = %#v", item)
			}
			if item.Action != string(clean.PlannedActionDeletePermanently) || item.Bytes == nil || *item.Bytes != 4 {
				t.Fatalf("history failed action/bytes = %#v", item)
			}
		}
	}
	if !foundFailed {
		t.Fatalf("history missing failed item: %#v", recorder.items)
	}
	if recorder.sessions[0].Aggregate.PermanentlyDeletedBytes != 4 || recorder.sessions[0].Aggregate.ErrorCount < 1 {
		t.Fatalf("history aggregate = %#v", recorder.sessions[0].Aggregate)
	}

	outcomes := clean.ProjectCategoryExecutionOutcomes([]string{testPermanentRule}, result)
	// testPermanentRule is not a catalog id — projection still attributes by Rule.
	if len(outcomes) != 1 {
		// non-catalog identifiers still get rows with empty label fallback
		t.Fatalf("outcomes = %#v", outcomes)
	}
}

func TestExecutePermanentCancellationZerosInterruptedBytes(t *testing.T) {
	root := t.TempDir()
	first := writeTestFile(t, root, "first.tmp", "aaaa")
	second := writeTestFile(t, root, "second.tmp", "bbbb")
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	remover := permanentRemoverFunc(func(_ context.Context, path string) error {
		n := calls.Add(1)
		if n == 1 {
			cancel()
			return context.Canceled
		}
		t.Fatalf("later candidate started: %s", path)
		return nil
	})
	recorder := &recordingHistoryRecorder{}
	result := clean.Execute(ctx, clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{first, second}},
		},
		PermanentRemover: remover,
		HistoryRecorder:  recorder,
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})

	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.AffectedBytes != 0 {
		t.Fatalf("canceled permanent must contribute zero success bytes: %#v", result.Totals)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if result.Skipped[0].Reason.Code != "context_canceled" || !strings.Contains(result.Skipped[0].Reason.Message, "may already be permanently deleted") {
		t.Fatalf("first cancel = %#v", result.Skipped[0])
	}
	if result.Skipped[1].Reason.Code != "context_canceled" {
		t.Fatalf("second cancel = %#v", result.Skipped[1])
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestExecuteMixedActionProgressPhasesIncludePermanent(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "r.tmp", "rb")
	permanentPath := writeTestFile(t, root, "p.tmp", "pd")
	var events []clean.ExecutionProgress
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, true)
	opts.RecycleBinAdapter = &recordingRecycleBinAdapter{}
	opts.PermanentRemover = &recordingPermanentRemover{}
	opts.ProgressReporter = func(p clean.ExecutionProgress) {
		events = append(events, p)
	}
	_ = clean.Execute(context.Background(), opts)
	phases := make([]clean.ExecutionPhase, 0, len(events))
	for _, e := range events {
		phases = append(phases, e.Phase)
	}
	// Category-boundary duplicates of Scanning / Ops phases are allowed.
	wantOrder := []clean.ExecutionPhase{
		clean.ExecutionPhaseScanning,
		clean.ExecutionPhaseRecycleBinSafety,
		clean.ExecutionPhaseRecycleBinOperations,
		clean.ExecutionPhasePermanentOperations,
		clean.ExecutionPhaseComplete,
	}
	if err := assertPhaseOrderAllowsExtras(phases, wantOrder); err != nil {
		t.Fatalf("phases = %v: %v", phases, err)
	}
	var sawRecycleCat, sawPermanentCat bool
	for _, e := range events {
		if e.Phase == clean.ExecutionPhaseRecycleBinOperations && e.ActiveCategory == testRecycleRule {
			sawRecycleCat = true
		}
		if e.Phase == clean.ExecutionPhasePermanentOperations && e.ActiveCategory == testPermanentRule {
			sawPermanentCat = true
		}
	}
	if !sawRecycleCat || !sawPermanentCat {
		t.Fatalf("missing mutate category progress: recycle=%v permanent=%v events=%#v",
			sawRecycleCat, sawPermanentCat, events)
	}
}

func TestExecuteReportsActiveCategoryAroundResolveAndMutateBoundaries(t *testing.T) {
	root := t.TempDir()
	recycleA := writeTestFile(t, root, "a.tmp", "aa")
	recycleB := writeTestFile(t, root, "b.tmp", "bb")
	permanentC := writeTestFile(t, root, "c.tmp", "cc")
	const (
		ruleA = "test_rule_a"
		ruleB = "test_rule_b"
		ruleC = "test_rule_c"
	)
	var events []clean.ExecutionProgress
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			ruleA: clean.PlannedActionMoveToRecycleBin,
			ruleB: clean.PlannedActionMoveToRecycleBin,
			ruleC: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: ruleA, DefaultEnabled: true, CandidatePaths: []string{recycleA}},
			{ID: ruleB, DefaultEnabled: true, CandidatePaths: []string{recycleB}},
			{ID: ruleC, DefaultEnabled: true, CandidatePaths: []string{permanentC}},
		},
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		PermanentRemover:  &recordingPermanentRemover{},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
		ProgressReporter: func(p clean.ExecutionProgress) {
			events = append(events, p)
		},
	})
	if result.Totals.DeletedCount != 3 {
		t.Fatalf("deleted = %d, want 3; result=%#v", result.Totals.DeletedCount, result)
	}

	// Collect first ActiveCategory per phase for resolve/mutate boundaries.
	scanCats := uniqueActiveCategories(events, clean.ExecutionPhaseScanning)
	rbCats := uniqueActiveCategories(events, clean.ExecutionPhaseRecycleBinOperations)
	permCats := uniqueActiveCategories(events, clean.ExecutionPhasePermanentOperations)
	if !containsAllStrings(scanCats, ruleA, ruleB, ruleC) {
		t.Fatalf("scan ActiveCategory = %v, want %s/%s/%s", scanCats, ruleA, ruleB, ruleC)
	}
	if !containsAllStrings(rbCats, ruleA, ruleB) {
		t.Fatalf("recycle ops ActiveCategory = %v, want %s/%s", rbCats, ruleA, ruleB)
	}
	if !containsAllStrings(permCats, ruleC) {
		t.Fatalf("permanent ops ActiveCategory = %v, want %s", permCats, ruleC)
	}
	// Safety and completion stay non-category-scoped.
	for _, e := range events {
		if e.Phase == clean.ExecutionPhaseRecycleBinSafety && e.ActiveCategory != "" {
			t.Fatalf("RecycleBinSafety should have empty ActiveCategory: %#v", e)
		}
		if e.Phase == clean.ExecutionPhaseComplete && e.ActiveCategory != "" {
			t.Fatalf("Complete should have empty ActiveCategory: %#v", e)
		}
	}
}

func uniqueActiveCategories(events []clean.ExecutionProgress, phase clean.ExecutionPhase) []string {
	seen := make(map[string]bool)
	var out []string
	for _, e := range events {
		if e.Phase != phase || e.ActiveCategory == "" || seen[e.ActiveCategory] {
			continue
		}
		seen[e.ActiveCategory] = true
		out = append(out, e.ActiveCategory)
	}
	return out
}

func TestExecuteReportsCategoryCompletionBeforeFinalResult(t *testing.T) {
	root := t.TempDir()
	recycleA := writeTestFile(t, root, "a.tmp", "aa")
	recycleB := writeTestFile(t, root, "b.tmp", "bb")
	permanentC := writeTestFile(t, root, "c.tmp", "cc")
	const (
		ruleA = "test_rule_a"
		ruleB = "test_rule_b"
		ruleC = "test_rule_c"
	)
	var events []clean.ExecutionProgress
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			ruleA: clean.PlannedActionMoveToRecycleBin,
			ruleB: clean.PlannedActionMoveToRecycleBin,
			ruleC: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: ruleA, DefaultEnabled: true, CandidatePaths: []string{recycleA}},
			{ID: ruleB, DefaultEnabled: true, CandidatePaths: []string{recycleB}},
			{ID: ruleC, DefaultEnabled: true, CandidatePaths: []string{permanentC}},
		},
		RecycleBinAdapter: &recordingRecycleBinAdapter{},
		PermanentRemover:  &recordingPermanentRemover{},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
		ProgressReporter: func(p clean.ExecutionProgress) {
			events = append(events, p)
		},
	})
	if result.Totals.DeletedCount != 3 {
		t.Fatalf("deleted = %d, want 3; result=%#v", result.Totals.DeletedCount, result)
	}

	// Completions must arrive before phase Complete; ActiveCategory still works.
	var (
		completions        []clean.ExecutionProgress
		sawActiveRecycle   bool
		sawActivePermanent bool
		completePhaseIndex = -1
	)
	for i, e := range events {
		if e.Phase == clean.ExecutionPhaseComplete && completePhaseIndex < 0 {
			completePhaseIndex = i
		}
		if e.ActiveCategory == ruleA && e.Phase == clean.ExecutionPhaseRecycleBinOperations {
			sawActiveRecycle = true
		}
		if e.ActiveCategory == ruleC && e.Phase == clean.ExecutionPhasePermanentOperations {
			sawActivePermanent = true
		}
		if clean.HasCategoryCompletion(e) {
			completions = append(completions, e)
			if strings.Contains(e.CompletedCategory, `\`) {
				t.Fatalf("CompletedCategory must be path-free: %#v", e)
			}
			if e.ActiveCategory != "" && strings.Contains(e.ActiveCategory, `\`) {
				t.Fatalf("ActiveCategory must be path-free: %#v", e)
			}
		}
	}
	if !sawActiveRecycle || !sawActivePermanent {
		t.Fatalf("ActiveCategory missing: recycle=%v permanent=%v", sawActiveRecycle, sawActivePermanent)
	}
	if completePhaseIndex < 0 {
		t.Fatal("missing completion phase")
	}

	byID := map[string]clean.ExecutionProgress{}
	for _, c := range completions {
		byID[c.CompletedCategory] = c
	}
	for _, id := range []string{ruleA, ruleB, ruleC} {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("missing completion for %s; completions=%#v events=%#v", id, completions, events)
		}
		if c.CompletedState != clean.CategoryExecutionCleaned {
			t.Fatalf("%s state = %q, want cleaned", id, c.CompletedState)
		}
		if c.CompletedDeletedCount != 1 || c.CompletedAffectedBytes <= 0 {
			t.Fatalf("%s metrics = %#v", id, c)
		}
	}
	// Completions for A/B/C must precede the Complete phase marker in the stream.
	for _, id := range []string{ruleA, ruleB, ruleC} {
		idx := -1
		for i, e := range events {
			if e.CompletedCategory == id {
				idx = i
				break
			}
		}
		if idx < 0 || idx >= completePhaseIndex {
			t.Fatalf("completion for %q must precede Complete (idx=%d complete=%d)", id, idx, completePhaseIndex)
		}
	}
	// Progress must not leak into JSON Result.
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "CompletedCategory") || strings.Contains(string(encoded), "completed_category") {
		t.Fatalf("completion progress leaked into Result JSON: %s", encoded)
	}
}

func TestExecuteReportsEmptyAfterResolveWhenSelectedCategoryHasNoCandidates(t *testing.T) {
	// Exact plan with a catalog permanent category that resolves to nothing.
	// Mid-flight empty completion must fire during Scanning, before mutation.
	id := clean.DevCacheCategoryGo
	var events []clean.ExecutionProgress
	plan, err := clean.CompileExactCategoryPlan([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		Plan:                   &plan,
		// Force empty resolution for this category.
		DevCachePathResolver: func(category string) []string {
			if category == id {
				return nil
			}
			return nil
		},
		ProgressReporter: func(p clean.ExecutionProgress) {
			events = append(events, p)
		},
	})
	_ = result

	var emptyCompletion *clean.ExecutionProgress
	completeIdx := -1
	for i, e := range events {
		if e.Phase == clean.ExecutionPhaseComplete && completeIdx < 0 {
			completeIdx = i
		}
		if e.CompletedCategory == id {
			cp := e
			emptyCompletion = &cp
			if e.CompletedState != clean.CategoryExecutionEmpty {
				t.Fatalf("empty completion state = %q", e.CompletedState)
			}
			if e.CompletedAffectedBytes != 0 || e.CompletedDeletedCount != 0 {
				t.Fatalf("empty must not invent success: %#v", e)
			}
			if completeIdx >= 0 && i >= completeIdx {
				t.Fatalf("empty completion must precede Complete")
			}
			if e.Phase != clean.ExecutionPhaseScanning {
				t.Fatalf("empty-after-resolve phase = %q, want scanning", e.Phase)
			}
		}
	}
	if emptyCompletion == nil {
		t.Fatalf("missing empty completion for %s; events=%#v", id, events)
	}
}

func containsAllStrings(have []string, want ...string) bool {
	set := make(map[string]bool, len(have))
	for _, s := range have {
		set[s] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestExecutePermanentOnlyPopulatesSplitTotalsAndJSON(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "p.tmp", "hello")
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		PermanentRemover: &recordingPermanentRemover{},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if result.Totals.PermanentlyDeletedBytes != 5 || result.Totals.RecycleBinMovedBytes != 0 || result.Totals.AffectedBytes != 5 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"action":"delete_permanently"`) {
		t.Fatalf("missing permanent action: %s", body)
	}
	if !strings.Contains(body, `"permanently_deleted_bytes":5`) {
		t.Fatalf("missing permanent totals: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "shred") || strings.Contains(strings.ToLower(body), "secure erase") {
		t.Fatalf("must not claim shred/secure erase: %s", body)
	}
}

func TestDryRunInjectedPermanentPlannedActionNeedsNoAuthorization(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "p.tmp", "data")
	result := clean.DryRun(context.Background(), clean.Options{
		CategoryPlannedActions: map[string]clean.PlannedAction{
			testPermanentRule: clean.PlannedActionDeletePermanently,
		},
		DiscoverOpportunities:     noUserTempOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
	})
	if len(result.Candidates) != 1 || result.Candidates[0].PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("dry-run must not delete: %#v", result.Deleted)
	}
}

func TestProductionDefaultCategoryStaysRecycleBinWithoutInjection(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "foal-owned.tmp", "cache")
	permanent := &recordingPermanentRemover{}
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true, // auth alone must not change non-permanent catalog actions
		RecycleBinAdapter:      adapter,
		PermanentRemover:       permanent,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{path},
		}},
	})
	if len(adapter.paths) != 1 || len(permanent.paths) != 0 {
		t.Fatalf("default path must stay recycle-bin: recycle=%v permanent=%v", adapter.paths, permanent.paths)
	}
	if result.Deleted[0].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("action = %q", result.Deleted[0].Action)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}
