package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

// Highest shared execution seam for category-owned permanent pre-mutation
// validation is clean.Execute (ADR-0018 mixed-action permanent path).

func TestExecuteCategoryOwnedPreMutationReceivesFreshCandidateContext(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "identity.tmp", "abcd")
	var seen clean.PermanentIdentityCandidate
	var calls atomic.Int32
	permanent := &recordingPermanentRemover{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		PermanentRemover: permanent,
		PermanentIdentityValidators: map[string]clean.PermanentIdentityValidator{
			testPermanentRule: func(candidate clean.PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				calls.Add(1)
				seen = candidate
				return pathsafe.Reason{}, true
			},
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if calls.Load() != 1 {
		t.Fatalf("validator calls = %d, want 1", calls.Load())
	}
	if seen.Path != path || seen.Bytes != 4 || seen.Category != testPermanentRule {
		t.Fatalf("validator context = %#v, want path/bytes/category", seen)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if len(permanent.paths) != 1 {
		t.Fatalf("remover paths = %v", permanent.paths)
	}
}

func TestExecuteCategoryOwnedRejectionSkipsWithoutRemoverOrRecycleFallback(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "reject.tmp", "data")
	permanent := &recordingPermanentRemover{}
	recycle := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		RecycleBinAdapter:  recycle,
		PermanentRemover:   permanent,
		HistoryRecorder:    recorder,
		CommandParameters:  history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute"}},
		PermanentIdentityValidators: map[string]clean.PermanentIdentityValidator{
			testPermanentRule: func(clean.PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				return pathsafe.Reason{
					Code:    "identity_mismatch",
					Message: "exact basename or parent no longer matches category identity",
				}, false
			},
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})

	if len(permanent.paths) != 0 {
		t.Fatalf("remover called on category rejection: %v", permanent.paths)
	}
	if len(recycle.paths) != 0 {
		t.Fatalf("recycle fallback on permanent rejection: %v", recycle.paths)
	}
	if len(result.Deleted) != 0 || result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("mutation outcomes = deleted=%#v totals=%#v", result.Deleted, result.Totals)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Rule != testPermanentRule || skipped.Bytes != 4 {
		t.Fatalf("skip identity = %#v", skipped)
	}
	if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned action = %q, want delete_permanently", skipped.PlannedAction)
	}
	if skipped.Reason.Code != "identity_mismatch" {
		t.Fatalf("skip reason = %#v", skipped.Reason)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("rejected path must remain: %v", err)
	}

	found := false
	for _, item := range recorder.items {
		if item.Result == "skipped" && item.SkippedReason != nil && item.SkippedReason.Code == "identity_mismatch" {
			found = true
			if item.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history planned action = %q", item.PlannedAction)
			}
			if item.Rule != testPermanentRule || item.Bytes == nil || *item.Bytes != 4 {
				t.Fatalf("history skip item = %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("history missing identity skip: %#v", recorder.items)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"planned_action":"delete_permanently"`) {
		t.Fatalf("result JSON missing planned permanent action: %s", body)
	}
	if !strings.Contains(body, `"identity_mismatch"`) {
		t.Fatalf("result JSON missing identity skip: %s", body)
	}
}

func TestExecuteCategoryOwnedRejectionIsolatesValidSiblings(t *testing.T) {
	root := t.TempDir()
	reject := writeTestFile(t, root, "reject.tmp", "aaaa")
	keep := writeTestFile(t, root, "keep.tmp", "bbbb")
	permanent := &recordingPermanentRemover{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{reject, keep}},
		},
		PermanentRemover: permanent,
		PermanentIdentityValidators: map[string]clean.PermanentIdentityValidator{
			testPermanentRule: func(candidate clean.PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				if filepath.Base(candidate.Path) == "reject.tmp" {
					return pathsafe.Reason{Code: "identity_mismatch", Message: "reject"}, false
				}
				return pathsafe.Reason{}, true
			},
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})

	if len(permanent.paths) != 1 || permanent.paths[0] != keep {
		t.Fatalf("remover paths = %v, want only valid sibling", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Path != keep {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if result.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted action = %q", result.Deleted[0].Action)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Path != reject {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if result.Skipped[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("skipped planned action = %q", result.Skipped[0].PlannedAction)
	}
	if result.Totals.PermanentlyDeletedBytes != 4 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(reject); err != nil {
		t.Fatalf("rejected sibling must remain: %v", err)
	}
	if _, err := os.Lstat(keep); !os.IsNotExist(err) {
		t.Fatalf("valid sibling should be deleted: %v", err)
	}
}

func TestExecuteWithoutCategoryValidatorKeepsSharedPermanentRemover(t *testing.T) {
	// Existing categories register no identity validator; PathSafe + shared remover only.
	root := t.TempDir()
	path := writeTestFile(t, root, "plain.tmp", "hello")
	permanent := &recordingPermanentRemover{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		PermanentRemover: permanent,
		// PermanentIdentityValidators intentionally nil / empty.
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != path {
		t.Fatalf("shared remover paths = %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if result.Totals.PermanentlyDeletedBytes != 5 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestExecutePathSafeRejectionPrecedesCategoryOwnedValidator(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "protected.tmp", "data")
	var categoryCalls atomic.Int32
	permanent := &recordingPermanentRemover{}
	// CandidatePaths still supplies the path; Protection at execute uses Validator.
	// Preview may suppress protected candidates, so inject via Rules that already
	// passed discovery-style listing — Force the permanent path with a validator
	// that would accept, and PathSafe that rejects.
	//
	// Protection filtering during resolve drops protected paths before permanent
	// mutation. Use a PathSafe-invalid form that still survives candidate listing:
	// an introduced reparse point after listing is covered elsewhere. Here we
	// prove ordering when the composed permanent hook runs PathSafe first by
	// calling the shared path with a protected path that remains in Rules because
	// the validator is applied at mutation time only when the path is still a
	// candidate. For Rules CandidatePaths, plan resolution still PathSafe-checks
	// and may skip early — accept either early skip or permanent skip, but
	// category validator must never run and remover must never run.
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		Validator:              pathsafe.NewValidator([]string{root}),
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{path}},
		},
		PermanentRemover: permanent,
		PermanentIdentityValidators: map[string]clean.PermanentIdentityValidator{
			testPermanentRule: func(clean.PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				categoryCalls.Add(1)
				return pathsafe.Reason{}, true
			},
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if categoryCalls.Load() != 0 {
		t.Fatalf("category validator ran despite PathSafe/protection: %d", categoryCalls.Load())
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("remover called: %v", permanent.paths)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || len(result.Deleted) != 0 {
		t.Fatalf("must not delete protected path: %#v", result)
	}
}

func TestExecuteCategoryOwnedValidationCancelSemanticsUnchanged(t *testing.T) {
	root := t.TempDir()
	first := writeTestFile(t, root, "first.tmp", "aaaa")
	second := writeTestFile(t, root, "second.tmp", "bbbb")
	ctx, cancel := context.WithCancel(context.Background())
	var removerCalls atomic.Int32
	var validatorCalls atomic.Int32
	remover := permanentRemoverFunc(func(_ context.Context, path string) error {
		n := removerCalls.Add(1)
		if n == 1 {
			cancel()
			return context.Canceled
		}
		t.Fatalf("later candidate started: %s", path)
		return nil
	})
	result := clean.Execute(ctx, clean.Options{
		AllowPermanentDeletion: true,
		CategoryPlannedActions: map[string]clean.DeletionAction{
			testPermanentRule: clean.DeletionActionDeletePermanently,
		},
		Rules: []clean.Rule{
			{ID: testPermanentRule, DefaultEnabled: true, CandidatePaths: []string{first, second}},
		},
		PermanentRemover: remover,
		PermanentIdentityValidators: map[string]clean.PermanentIdentityValidator{
			testPermanentRule: func(clean.PermanentIdentityCandidate) (pathsafe.Reason, bool) {
				validatorCalls.Add(1)
				return pathsafe.Reason{}, true
			},
		},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 1 << 60}, nil
		},
	})
	if removerCalls.Load() != 1 {
		t.Fatalf("remover calls = %d, want 1", removerCalls.Load())
	}
	// Second candidate is canceled before validation/removal.
	if validatorCalls.Load() != 1 {
		t.Fatalf("validator calls = %d, want 1 (second canceled before validation)", validatorCalls.Load())
	}
	if result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("canceled permanent must contribute zero success bytes: %#v", result.Totals)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	for _, skipped := range result.Skipped {
		if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("cancel skip planned action = %#v", skipped)
		}
		if skipped.Reason.Code != "context_canceled" {
			t.Fatalf("cancel skip = %#v", skipped)
		}
	}
}
