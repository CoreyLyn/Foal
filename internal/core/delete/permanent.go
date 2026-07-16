package delete

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// PermanentRemover permanently removes a filesystem path, bypassing the Recycle Bin.
// Implementations use ordinary filesystem removal only: no overwrite, shred, free-space
// wipe, or forensic non-recoverability claims. Cooperative cancellation must be checked
// during recursive traversal.
type PermanentRemover interface {
	Remove(ctx context.Context, path string) error
}

// FilesystemPermanentRemover removes files and directory trees with ordinary OS calls.
type FilesystemPermanentRemover struct{}

// Remove permanently deletes path. Directories are walked depth-first. Cancellation
// stops further traversal; already-removed descendants are not restored.
func (FilesystemPermanentRemover) Remove(ctx context.Context, path string) error {
	return removePermanently(ctx, path)
}

// PermanentOutcome is the per-candidate result of permanent removal.
type PermanentOutcomeKind string

const (
	PermanentOutcomeDeleted  PermanentOutcomeKind = "deleted"
	PermanentOutcomeSkipped  PermanentOutcomeKind = "skipped"
	PermanentOutcomeFailed   PermanentOutcomeKind = "failed"
	PermanentOutcomeCanceled PermanentOutcomeKind = "canceled"
)

// PermanentItem is one candidate outcome from ExecutePermanent.
type PermanentItem struct {
	Path        string
	Bytes       int64
	Kind        PermanentOutcomeKind
	Reason      pathsafe.Reason
	// PartialRisk is true when mutation may have begun before failure or cancel.
	PartialRisk bool
}

// PermanentResult aggregates permanent-removal outcomes for a candidate batch.
type PermanentResult struct {
	Items []PermanentItem
}

// ExecutePermanent permanently removes candidates with immediate path validation and
// cooperative cancellation. It never moves items to the Recycle Bin.
func ExecutePermanent(ctx context.Context, candidates []Candidate, remover PermanentRemover) PermanentResult {
	return ExecutePermanentWithValidator(ctx, candidates, remover, pathsafe.Validator{})
}

// ExecutePermanentWithValidator is ExecutePermanent with an explicit pathsafe.Validator.
func ExecutePermanentWithValidator(ctx context.Context, candidates []Candidate, remover PermanentRemover, validator pathsafe.Validator) PermanentResult {
	if remover == nil {
		remover = FilesystemPermanentRemover{}
	}
	var result PermanentResult
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			result.Items = append(result.Items, PermanentItem{
				Path:  candidate.Path,
				Bytes: candidate.Bytes,
				Kind:  PermanentOutcomeCanceled,
				Reason: pathsafe.Reason{
					Code:    "context_canceled",
					Message: ctx.Err().Error(),
				},
			})
			continue
		default:
		}

		if reason, ok := validator.ValidateDeletePath(candidate.Path); !ok {
			result.Items = append(result.Items, PermanentItem{
				Path:   candidate.Path,
				Bytes:  candidate.Bytes,
				Kind:   PermanentOutcomeSkipped,
				Reason: reason,
			})
			continue
		}

		err := remover.Remove(ctx, candidate.Path)
		if err == nil {
			result.Items = append(result.Items, PermanentItem{
				Path:  candidate.Path,
				Bytes: candidate.Bytes,
				Kind:  PermanentOutcomeDeleted,
			})
			continue
		}

		// Mutation may have begun once Remove was invoked.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.Items = append(result.Items, PermanentItem{
				Path:        candidate.Path,
				Bytes:       candidate.Bytes,
				Kind:        PermanentOutcomeCanceled,
				PartialRisk: true,
				Reason: pathsafe.Reason{
					Code:    "context_canceled",
					Message: permanentPartialCancelMessage(err),
				},
			})
			continue
		}

		result.Items = append(result.Items, PermanentItem{
			Path:        candidate.Path,
			Bytes:       candidate.Bytes,
			Kind:        PermanentOutcomeFailed,
			PartialRisk: true,
			Reason: pathsafe.Reason{
				Code:    "permanent_delete_failed",
				Message: permanentPartialFailureMessage(err),
			},
		})
	}
	return result
}

func permanentPartialFailureMessage(err error) string {
	return fmt.Sprintf("permanent deletion failed after mutation may have begun; some content may already be permanently deleted: %v", err)
}

func permanentPartialCancelMessage(err error) string {
	return fmt.Sprintf("permanent deletion canceled after mutation may have begun; some content may already be permanently deleted: %v", err)
}

func removePermanently(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Defensive: pathsafe should reject reparse points before Remove.
		return fmt.Errorf("refusing to permanently remove reparse point")
	}
	if !info.IsDir() {
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.Remove(path)
	}
	return removeDirectoryTree(ctx, path)
}

func removeDirectoryTree(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := filepath.Join(dir, entry.Name())
		// Re-check reparse / symlink children fail-closed before descending.
		info, err := os.Lstat(child)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to permanently remove reparse point child %s", child)
		}
		if info.IsDir() {
			if err := removeDirectoryTree(ctx, child); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(child); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Remove(dir)
}
