package delete_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

type recordingAdapter struct {
	paths []string
}

func (a *recordingAdapter) MoveToRecycleBin(path string) error {
	a.paths = append(a.paths, path)
	return nil
}

func TestExecuteSkipsProtectedPathBeforeAdapter(t *testing.T) {
	adapter := &recordingAdapter{}

	result := delete.Execute(context.Background(), []delete.Candidate{
		{Path: `C:\Windows`, Bytes: 4096},
	}, adapter)

	if len(adapter.paths) != 0 {
		t.Fatalf("adapter received paths %v, want none", adapter.paths)
	}
	if result.AffectedBytes != 0 {
		t.Fatalf("AffectedBytes = %d, want 0", result.AffectedBytes)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("Skipped length = %d, want 1: %#v", len(result.Skipped), result)
	}
	if result.Skipped[0].Path != `C:\Windows` {
		t.Fatalf("Skipped[0].Path = %q, want C:\\Windows", result.Skipped[0].Path)
	}
	if result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("Skipped[0].Reason.Code = %q, want protected_path", result.Skipped[0].Reason.Code)
	}
	if result.Skipped[0].Reason.Message == "" {
		t.Fatal("Skipped[0].Reason.Message is empty")
	}
}

func TestExecuteWithValidatorRevalidatesUserProtectedPathBeforeAdapter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "protected.tmp")
	if err := os.WriteFile(path, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{}

	result := delete.ExecuteWithValidator(
		context.Background(),
		[]delete.Candidate{{Path: path, Bytes: 5}},
		adapter,
		pathsafe.NewValidator([]string{dir}),
	)

	if len(adapter.paths) != 0 {
		t.Fatalf("adapter received paths %v, want none", adapter.paths)
	}
	if len(result.Deleted) != 0 || result.AffectedBytes != 0 {
		t.Fatalf("result = %#v, want no deleted path or affected bytes", result)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("skipped = %#v, want user protected path", result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason.Message, "user-defined Protection rule") {
		t.Fatalf("message = %q, want user-defined rule identity", result.Skipped[0].Reason.Message)
	}
}

func TestExecuteRevalidatesCandidatesAndCountsOnlyDeletedBytes(t *testing.T) {
	dir := t.TempDir()
	safe := filepath.Join(dir, "safe.tmp")
	if err := os.WriteFile(safe, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{}

	result := delete.Execute(context.Background(), []delete.Candidate{
		{Path: safe, Bytes: 4},
		{Path: `\\?\C:\Windows\System32`, Bytes: 8192},
	}, adapter)

	if len(adapter.paths) != 1 || adapter.paths[0] != safe {
		t.Fatalf("adapter paths = %v, want [%q]", adapter.paths, safe)
	}
	if result.AffectedBytes != 4 {
		t.Fatalf("AffectedBytes = %d, want 4", result.AffectedBytes)
	}
	if len(result.Deleted) != 1 {
		t.Fatalf("Deleted length = %d, want 1", len(result.Deleted))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("Skipped length = %d, want 1", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "protected_path" {
		t.Fatalf("Skipped[0].Reason.Code = %q, want protected_path", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteModelsPermissionFailureAsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.tmp")
	if err := os.WriteFile(path, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := failingAdapter{err: fs.ErrPermission}

	result := delete.Execute(context.Background(), []delete.Candidate{
		{Path: path, Bytes: 6},
	}, adapter)

	if result.AffectedBytes != 0 {
		t.Fatalf("AffectedBytes = %d, want 0", result.AffectedBytes)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("Deleted length = %d, want 0", len(result.Deleted))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("Skipped length = %d, want 1", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "permission_denied" {
		t.Fatalf("Skipped[0].Reason.Code = %q, want permission_denied", result.Skipped[0].Reason.Code)
	}
}

func TestExecuteModelsUnsupportedTargetAsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unsupported.tmp")
	if err := os.WriteFile(path, []byte("unsupported"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := failingAdapter{err: fs.ErrInvalid}

	result := delete.Execute(context.Background(), []delete.Candidate{
		{Path: path, Bytes: 11},
	}, adapter)

	if result.AffectedBytes != 0 {
		t.Fatalf("AffectedBytes = %d, want 0", result.AffectedBytes)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("Deleted length = %d, want 0", len(result.Deleted))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("Skipped length = %d, want 1", len(result.Skipped))
	}
	if result.Skipped[0].Reason.Code != "unsupported_target" {
		t.Fatalf("Skipped[0].Reason.Code = %q, want unsupported_target", result.Skipped[0].Reason.Code)
	}
	if result.Skipped[0].Reason.Message == "" {
		t.Fatal("Skipped[0].Reason.Message is empty")
	}
}

type failingAdapter struct {
	err error
}

func (a failingAdapter) MoveToRecycleBin(string) error {
	if a.err == nil {
		return errors.New("unexpected adapter call")
	}
	return a.err
}
