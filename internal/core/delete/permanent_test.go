package delete_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func TestExecutePermanentRemovesNestedTreeWithOrdinaryFilesystem(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "cache-tree")
	if err := os.MkdirAll(filepath.Join(tree, "nested", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"a.bin", "nested/b.bin", "nested/deep/c.bin"} {
		if err := os.WriteFile(filepath.Join(tree, rel), []byte(rel), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result := delete.ExecutePermanent(context.Background(), []delete.Candidate{
		{Path: tree, Bytes: 99},
	}, delete.FilesystemPermanentRemover{})

	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeDeleted {
		t.Fatalf("result = %#v, want one deleted item", result.Items)
	}
	if result.Items[0].Bytes != 99 {
		t.Fatalf("bytes = %d, want original measured bytes", result.Items[0].Bytes)
	}
	if _, err := os.Lstat(tree); !os.IsNotExist(err) {
		t.Fatalf("tree still exists after permanent delete: %v", err)
	}
}

func TestExecutePermanentSkipsProtectedBeforeRemover(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "protected.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	remover := &countingPermanentRemover{}
	result := delete.ExecutePermanentWithValidator(
		context.Background(),
		[]delete.Candidate{{Path: path, Bytes: 4}},
		remover,
		pathsafe.NewValidator([]string{root}),
	)
	if remover.calls.Load() != 0 {
		t.Fatalf("remover called %d times, want 0", remover.calls.Load())
	}
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeSkipped {
		t.Fatalf("result = %#v, want skipped", result.Items)
	}
	if result.Items[0].Reason.Code != "protected_path" {
		t.Fatalf("reason = %#v", result.Items[0].Reason)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("protected path must remain: %v", err)
	}
}

func TestExecutePermanentRejectsIntroducedReparsePoint(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real-dir")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "file.bin"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link-dir")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}

	result := delete.ExecutePermanent(context.Background(), []delete.Candidate{
		{Path: link, Bytes: 1},
	}, delete.FilesystemPermanentRemover{})

	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeSkipped {
		t.Fatalf("result = %#v, want reparse skip before remover", result.Items)
	}
	if result.Items[0].Reason.Code != "reparse_point" {
		t.Fatalf("reason = %#v, want reparse_point", result.Items[0].Reason)
	}
	if _, err := os.Lstat(filepath.Join(target, "file.bin")); err != nil {
		t.Fatalf("real target must not be deleted through reparse: %v", err)
	}
}

func TestExecutePermanentCancelBeforeCandidateDoesNotCallRemover(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	remover := &countingPermanentRemover{}
	result := delete.ExecutePermanent(ctx, []delete.Candidate{{Path: path, Bytes: 4}}, remover)
	if remover.calls.Load() != 0 {
		t.Fatalf("remover calls = %d, want 0", remover.calls.Load())
	}
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeCanceled {
		t.Fatalf("result = %#v", result.Items)
	}
	if result.Items[0].PartialRisk {
		t.Fatal("untouched cancel must not claim partial risk")
	}
}

func TestExecutePermanentPartialFailureZerosSuccessAndWarns(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	remover := permanentRemoverFunc(func(context.Context, string) error {
		return errors.New("disk fault")
	})
	result := delete.ExecutePermanent(context.Background(), []delete.Candidate{
		{Path: path, Bytes: 4},
	}, remover)
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeFailed {
		t.Fatalf("result = %#v", result.Items)
	}
	item := result.Items[0]
	if item.Reason.Code != "permanent_delete_failed" || !item.PartialRisk {
		t.Fatalf("item = %#v", item)
	}
	if !strings.Contains(item.Reason.Message, "may already be permanently deleted") {
		t.Fatalf("message missing partial-risk warning: %q", item.Reason.Message)
	}
	if item.Bytes != 4 {
		t.Fatalf("bytes = %d, want original measured bytes retained", item.Bytes)
	}
}

func TestExecutePermanentCancelDuringTraversalStopsLaterCandidates(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte("xx"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	remover := permanentRemoverFunc(func(ctx context.Context, path string) error {
		n := calls.Add(1)
		if n == 1 {
			cancel()
			return context.Canceled
		}
		return delete.FilesystemPermanentRemover{}.Remove(ctx, path)
	})
	result := delete.ExecutePermanent(ctx, []delete.Candidate{
		{Path: first, Bytes: 2},
		{Path: second, Bytes: 2},
	}, remover)
	if calls.Load() != 1 {
		t.Fatalf("remover calls = %d, want 1 (later candidate must not start)", calls.Load())
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %#v", result.Items)
	}
	if result.Items[0].Kind != delete.PermanentOutcomeCanceled || !result.Items[0].PartialRisk {
		t.Fatalf("first = %#v, want canceled with partial risk", result.Items[0])
	}
	if result.Items[1].Kind != delete.PermanentOutcomeCanceled || result.Items[1].PartialRisk {
		t.Fatalf("second = %#v, want cancel before start without partial risk", result.Items[1])
	}
}

func TestFilesystemPermanentRemoverHonorsCancelDuringWalk(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "a"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a", "1.bin"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a", "2.bin"), []byte("2"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before entry so walk aborts cooperatively.
	cancel()
	err := delete.FilesystemPermanentRemover{}.Remove(ctx, tree)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestExecutePermanentPathSafeRunsBeforePreMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "protected.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	var preMutationCalls atomic.Int32
	remover := &countingPermanentRemover{}
	result := delete.ExecutePermanentWithHooks(
		context.Background(),
		[]delete.Candidate{{Path: path, Bytes: 4}},
		remover,
		pathsafe.NewValidator([]string{root}),
		func(candidate delete.Candidate) (pathsafe.Reason, bool) {
			preMutationCalls.Add(1)
			return pathsafe.Reason{}, true
		},
	)
	if preMutationCalls.Load() != 0 {
		t.Fatalf("pre-mutation ran after PathSafe rejection: calls=%d", preMutationCalls.Load())
	}
	if remover.calls.Load() != 0 {
		t.Fatalf("remover called after PathSafe rejection: %d", remover.calls.Load())
	}
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeSkipped {
		t.Fatalf("result = %#v, want pathsafe skip", result.Items)
	}
	if result.Items[0].Reason.Code != "protected_path" {
		t.Fatalf("reason = %#v", result.Items[0].Reason)
	}
}

func TestExecutePermanentPreMutationRejectionDoesNotCallRemover(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	remover := &countingPermanentRemover{}
	result := delete.ExecutePermanentWithHooks(
		context.Background(),
		[]delete.Candidate{{Path: path, Bytes: 4}},
		remover,
		pathsafe.Validator{},
		func(candidate delete.Candidate) (pathsafe.Reason, bool) {
			if candidate.Path != path || candidate.Bytes != 4 {
				t.Fatalf("pre-mutation candidate = %#v", candidate)
			}
			return pathsafe.Reason{
				Code:    "identity_mismatch",
				Message: "candidate no longer matches category identity",
			}, false
		},
	)
	if remover.calls.Load() != 0 {
		t.Fatalf("remover called on pre-mutation rejection: %d", remover.calls.Load())
	}
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeSkipped {
		t.Fatalf("result = %#v, want skipped", result.Items)
	}
	if result.Items[0].Reason.Code != "identity_mismatch" || result.Items[0].Bytes != 4 {
		t.Fatalf("item = %#v", result.Items[0])
	}
	if result.Items[0].PartialRisk {
		t.Fatal("pre-mutation skip must not claim partial risk")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("rejected path must remain: %v", err)
	}
}

func TestExecutePermanentPreMutationRejectionIsolatesSiblings(t *testing.T) {
	root := t.TempDir()
	reject := filepath.Join(root, "reject.tmp")
	keep := filepath.Join(root, "keep.tmp")
	for _, path := range []string{reject, keep} {
		if err := os.WriteFile(path, []byte("xx"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	remover := &countingPermanentRemover{}
	result := delete.ExecutePermanentWithHooks(
		context.Background(),
		[]delete.Candidate{
			{Path: reject, Bytes: 2},
			{Path: keep, Bytes: 2},
		},
		remover,
		pathsafe.Validator{},
		func(candidate delete.Candidate) (pathsafe.Reason, bool) {
			if candidate.Path == reject {
				return pathsafe.Reason{Code: "identity_mismatch", Message: "reject"}, false
			}
			return pathsafe.Reason{}, true
		},
	)
	if remover.calls.Load() != 1 {
		t.Fatalf("remover calls = %d, want only valid sibling", remover.calls.Load())
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %#v", result.Items)
	}
	if result.Items[0].Kind != delete.PermanentOutcomeSkipped || result.Items[0].Reason.Code != "identity_mismatch" {
		t.Fatalf("first = %#v", result.Items[0])
	}
	if result.Items[1].Kind != delete.PermanentOutcomeDeleted || result.Items[1].Path != keep {
		t.Fatalf("second = %#v", result.Items[1])
	}
}

func TestExecutePermanentNilPreMutationKeepsExistingBehavior(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.tmp")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	result := delete.ExecutePermanentWithHooks(
		context.Background(),
		[]delete.Candidate{{Path: path, Bytes: 4}},
		delete.FilesystemPermanentRemover{},
		pathsafe.Validator{},
		nil,
	)
	if len(result.Items) != 1 || result.Items[0].Kind != delete.PermanentOutcomeDeleted {
		t.Fatalf("result = %#v, want deleted with nil pre-mutation", result.Items)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("path should be deleted: %v", err)
	}
}

type countingPermanentRemover struct {
	calls atomic.Int32
}

func (r *countingPermanentRemover) Remove(context.Context, string) error {
	r.calls.Add(1)
	return nil
}

type permanentRemoverFunc func(context.Context, string) error

func (f permanentRemoverFunc) Remove(ctx context.Context, path string) error {
	return f(ctx, path)
}
