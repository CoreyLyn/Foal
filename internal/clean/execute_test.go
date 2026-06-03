package clean_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
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

type failingRecycleBinAdapter struct {
	err error
}

func (a failingRecycleBinAdapter) MoveToRecycleBin(string) error {
	return a.err
}
