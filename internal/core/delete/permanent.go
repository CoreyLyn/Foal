package delete

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
	// PermanentOutcomePartial means a directory candidate was partially removed:
	// some files were deleted and some were skipped (e.g. locked by another
	// process). DeletedBytes is the reliable sum of files actually removed; the
	// remainder is carried as a failed portion. PartialRisk is true because some
	// content was already permanently deleted.
	PermanentOutcomePartial PermanentOutcomeKind = "partial"
)

// SkippedFile is one file that could not be permanently removed during a
// continue-on-error directory traversal. It carries a stable reason code and the
// raw OS message for history/diagnostics; it never authorizes fallback.
type SkippedFile struct {
	Path   string
	Reason pathsafe.Reason
}

// PermanentItem is one candidate outcome from ExecutePermanent.
type PermanentItem struct {
	Path        string
	Bytes       int64
	Kind        PermanentOutcomeKind
	Reason      pathsafe.Reason
	// PartialRisk is true when mutation may have begun before failure or cancel.
	PartialRisk bool
	// SkippedFiles lists per-file skips for PermanentOutcomePartial and
	// PermanentOutcomeFailed produced by the detailed continue-on-error path. It
	// is empty for the legacy fail-fast path.
	SkippedFiles []SkippedFile
}

// PermanentResult aggregates permanent-removal outcomes for a candidate batch.
type PermanentResult struct {
	Items []PermanentItem
}

// PreMutationValidator optionally re-checks candidate identity after PathSafe
// validation and immediately before permanent removal. Returning ok=false is a
// recoverable pre-mutation skip: the remover is not called and PartialRisk stays
// false. Validators must not mutate the filesystem or own deletion.
// A nil validator is a no-op (PathSafe-only composition).
type PreMutationValidator func(candidate Candidate) (pathsafe.Reason, bool)

// ExecutePermanent permanently removes candidates with immediate path validation and
// cooperative cancellation. It never moves items to the Recycle Bin.
func ExecutePermanent(ctx context.Context, candidates []Candidate, remover PermanentRemover) PermanentResult {
	return ExecutePermanentWithValidator(ctx, candidates, remover, pathsafe.Validator{})
}

// ExecutePermanentWithValidator is ExecutePermanent with an explicit pathsafe.Validator.
// Categories without extra identity checks use this PathSafe-only path.
func ExecutePermanentWithValidator(ctx context.Context, candidates []Candidate, remover PermanentRemover, validator pathsafe.Validator) PermanentResult {
	return ExecutePermanentWithHooks(ctx, candidates, remover, validator, nil)
}

// ExecutePermanentWithHooks composes PathSafe validation with an optional
// category-owned pre-mutation check immediately before each removal.
// Ordering per candidate: cancel check → PathSafe → PreMutation (if non-nil) → Remove.
// A pre-mutation rejection never falls back to the Recycle Bin.
func ExecutePermanentWithHooks(ctx context.Context, candidates []Candidate, remover PermanentRemover, validator pathsafe.Validator, preMutation PreMutationValidator) PermanentResult {
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

		if preMutation != nil {
			if reason, ok := preMutation(candidate); !ok {
				result.Items = append(result.Items, PermanentItem{
					Path:   candidate.Path,
					Bytes:  candidate.Bytes,
					Kind:   PermanentOutcomeSkipped,
					Reason: reason,
				})
				continue
			}
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

// PermanentRemovalOutcome is the continue-on-error result of one detailed
// permanent removal. It distinguishes a fully-deleted tree from a partial one
// (some files skipped) so the caller can account for actually-removed bytes.
type PermanentRemovalOutcome string

const (
	PermanentRemovalFullyDeleted PermanentRemovalOutcome = "deleted"
	PermanentRemovalPartial      PermanentRemovalOutcome = "partial"
	PermanentRemovalFailed       PermanentRemovalOutcome = "failed"
	PermanentRemovalCanceled     PermanentRemovalOutcome = "canceled"
)

// PermanentRemoval is the structured result of DetailedRemove. DeletedBytes is
// the reliable sum of file sizes actually removed during traversal; Skipped
// lists files that could not be removed (locked, permission denied, reparse).
type PermanentRemoval struct {
	Outcome      PermanentRemovalOutcome
	DeletedBytes int64
	Skipped      []SkippedFile
}

// DetailedPermanentRemover is an optional PermanentRemover that reports
// per-file outcomes during recursive removal. When the remover implements it,
// ExecutePermanentDetailed prefers DetailedRemove over Remove so a locked file
// inside a directory tree skips only that file and deletes the rest, with
// accurate deleted-byte accounting. Implementations keep Remove semantics
// unchanged for callers that do not opt into the detailed path.
type DetailedPermanentRemover interface {
	PermanentRemover
	DetailedRemove(ctx context.Context, path string) PermanentRemoval
}

// ExecutePermanentDetailed is ExecutePermanentWithHooks for removers that also
// implement DetailedPermanentRemover. It uses continue-on-error recursive
// removal: a locked file inside a candidate tree is skipped, the rest is
// deleted, and the candidate becomes PermanentOutcomePartial with reliable
// deleted-byte accounting. Removers that do not implement the detailed
// interface fall back to ExecutePermanentWithHooks (legacy fail-fast behavior),
// so existing test fakes are unaffected. Purge does not call this entry point.
func ExecutePermanentDetailed(ctx context.Context, candidates []Candidate, remover PermanentRemover, validator pathsafe.Validator, preMutation PreMutationValidator) PermanentResult {
	if remover == nil {
		remover = FilesystemPermanentRemover{}
	}
	detailed, ok := remover.(DetailedPermanentRemover)
	if !ok {
		return ExecutePermanentWithHooks(ctx, candidates, remover, validator, preMutation)
	}
	var result PermanentResult
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			result.Items = append(result.Items, PermanentItem{
				Path:   candidate.Path,
				Bytes:  candidate.Bytes,
				Kind:   PermanentOutcomeCanceled,
				Reason: pathsafe.Reason{Code: "context_canceled", Message: ctx.Err().Error()},
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

		if preMutation != nil {
			if reason, ok := preMutation(candidate); !ok {
				result.Items = append(result.Items, PermanentItem{
					Path:   candidate.Path,
					Bytes:  candidate.Bytes,
					Kind:   PermanentOutcomeSkipped,
					Reason: reason,
				})
				continue
			}
		}

		removal := detailed.DetailedRemove(ctx, candidate.Path)
		switch removal.Outcome {
		case PermanentRemovalFullyDeleted:
			result.Items = append(result.Items, PermanentItem{
				Path:  candidate.Path,
				Bytes: candidate.Bytes,
				Kind:  PermanentOutcomeDeleted,
			})
		case PermanentRemovalPartial:
			result.Items = append(result.Items, PermanentItem{
				Path:         candidate.Path,
				Bytes:        removal.DeletedBytes,
				Kind:         PermanentOutcomePartial,
				PartialRisk:  true,
				SkippedFiles: removal.Skipped,
				Reason: pathsafe.Reason{
					Code:    "permanent_delete_partial",
					Message: permanentPartialSkipMessage(removal),
				},
			})
		case PermanentRemovalCanceled:
			msg := ctx.Err().Error()
			result.Items = append(result.Items, PermanentItem{
				Path:        candidate.Path,
				Bytes:       candidate.Bytes,
				Kind:        PermanentOutcomeCanceled,
				PartialRisk: true,
				Reason: pathsafe.Reason{
					Code:    "context_canceled",
					Message: permanentPartialCancelMessage(errors.New(msg)),
				},
			})
		default: // PermanentRemovalFailed: nothing was confirmed removed.
			result.Items = append(result.Items, PermanentItem{
				Path:         candidate.Path,
				Bytes:        candidate.Bytes,
				Kind:         PermanentOutcomeFailed,
				PartialRisk:  true,
				SkippedFiles: removal.Skipped,
				Reason: pathsafe.Reason{
					Code:    "permanent_delete_failed",
					Message: permanentDetailedFailureMessage(removal),
				},
			})
		}
	}
	return result
}

// permanentPartialSkipMessage summarizes the per-file skips of a partial
// permanent removal without forwarding raw paths into a path-free surface
// (callers project path-free; this message is retained on the path-bearing
// Result/History item).
func permanentPartialSkipMessage(removal PermanentRemoval) string {
	if len(removal.Skipped) == 0 {
		return fmt.Sprintf("permanent deletion partially completed: %d bytes removed, some content could not be deleted", removal.DeletedBytes)
	}
	first := removal.Skipped[0]
	return fmt.Sprintf("permanent deletion partially completed: %d bytes removed; %d file(s) skipped (first: %s: %s)",
		removal.DeletedBytes, len(removal.Skipped), first.Path, first.Reason.Message)
}

func permanentDetailedFailureMessage(removal PermanentRemoval) string {
	if len(removal.Skipped) == 0 {
		return "permanent deletion failed; no content was confirmed removed"
	}
	first := removal.Skipped[0]
	return fmt.Sprintf("permanent deletion failed; no content was confirmed removed; %d file(s) skipped (first: %s: %s)",
		len(removal.Skipped), first.Path, first.Reason.Message)
}

// classifyRemoveError maps a per-file os.Remove error onto a stable pathsafe
// reason code. "in use"/sharing violations and permission denials are distinct
// from generic delete failures so history can distinguish lock conflicts.
func classifyRemoveError(err error) pathsafe.Reason {
	if errors.Is(err, fs.ErrPermission) {
		return pathsafe.Reason{Code: "permission_denied", Message: "permission denied while permanently deleting file"}
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "being used by another process") || strings.Contains(msg, "used by another process") || strings.Contains(msg, "sharing violation") {
		return pathsafe.Reason{Code: "file_in_use", Message: err.Error()}
	}
	return pathsafe.Reason{Code: "delete_failed", Message: err.Error()}
}

// DetailedRemove permanently removes path with continue-on-error semantics.
// Directories are walked depth-first; a file that cannot be removed (locked,
// permission denied, reparse) is recorded as skipped and traversal continues.
// DeletedBytes is the reliable sum of files actually removed. Single-file
// candidates that cannot be removed yield PermanentRemovalFailed.
func (FilesystemPermanentRemover) DetailedRemove(ctx context.Context, path string) PermanentRemoval {
	return removeDirectoryTreeDetailed(ctx, path)
}

// removeDirectoryTreeDetailed is the continue-on-error counterpart of
// removeDirectoryTree. It never aborts the whole tree on a single file error;
// instead it records the skip and deletes the remaining files. The top-level
// directory is removed at the end when possible; if skipped children remain it
// is left in place and the outcome is PermanentRemovalPartial.
func removeDirectoryTreeDetailed(ctx context.Context, dir string) PermanentRemoval {
	removal := PermanentRemoval{}
	if err := ctx.Err(); err != nil {
		removal.Outcome = PermanentRemovalCanceled
		return removal
	}
	info, err := os.Lstat(dir)
	if err != nil {
		removal.Outcome = permanentRemovalOutcomeForError(ctx)
		return removal
	}
	if info.Mode()&os.ModeSymlink != 0 {
		removal.Outcome = PermanentRemovalFailed
		removal.Skipped = append(removal.Skipped, SkippedFile{
			Path:   dir,
			Reason: pathsafe.Reason{Code: "reparse_point", Message: "refusing to permanently remove reparse point"},
		})
		return removal
	}
	if !info.IsDir() {
		if err := ctx.Err(); err != nil {
			removal.Outcome = PermanentRemovalCanceled
			return removal
		}
		if err := os.Remove(dir); err != nil {
			removal.Outcome = PermanentRemovalFailed
			removal.Skipped = append(removal.Skipped, SkippedFile{Path: dir, Reason: classifyRemoveError(err)})
			return removal
		}
		removal.Outcome = PermanentRemovalFullyDeleted
		removal.DeletedBytes = info.Size()
		return removal
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		removal.Outcome = permanentRemovalOutcomeForError(ctx)
		return removal
	}
	anySkipped := false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			removal.Outcome = PermanentRemovalCanceled
			return removal
		}
		child := filepath.Join(dir, entry.Name())
		cinfo, err := os.Lstat(child)
		if err != nil {
			anySkipped = true
			removal.Skipped = append(removal.Skipped, SkippedFile{Path: child, Reason: pathsafe.Reason{Code: "delete_failed", Message: err.Error()}})
			continue
		}
		if cinfo.Mode()&os.ModeSymlink != 0 {
			anySkipped = true
			removal.Skipped = append(removal.Skipped, SkippedFile{
				Path:   child,
				Reason: pathsafe.Reason{Code: "reparse_point", Message: "refusing to permanently remove reparse point child"},
			})
			continue
		}
		if cinfo.IsDir() {
			sub := removeDirectoryTreeDetailed(ctx, child)
			removal.DeletedBytes += sub.DeletedBytes
			removal.Skipped = append(removal.Skipped, sub.Skipped...)
			if sub.Outcome == PermanentRemovalCanceled {
				removal.Outcome = PermanentRemovalCanceled
				return removal
			}
			if sub.Outcome != PermanentRemovalFullyDeleted {
				anySkipped = true
			}
			continue
		}
		if err := os.Remove(child); err != nil {
			anySkipped = true
			removal.Skipped = append(removal.Skipped, SkippedFile{Path: child, Reason: classifyRemoveError(err)})
			continue
		}
		removal.DeletedBytes += cinfo.Size()
	}
	if err := ctx.Err(); err != nil {
		removal.Outcome = PermanentRemovalCanceled
		return removal
	}
	if rmErr := os.Remove(dir); rmErr != nil {
		if !anySkipped {
			anySkipped = true
			removal.Skipped = append(removal.Skipped, SkippedFile{Path: dir, Reason: classifyRemoveError(rmErr)})
		}
		// When skipped children remain, the non-empty directory error is the
		// expected consequence of those skips; do not double-record the dir.
	}
	if anySkipped {
		removal.Outcome = PermanentRemovalPartial
	} else {
		removal.Outcome = PermanentRemovalFullyDeleted
	}
	return removal
}

func permanentRemovalOutcomeForError(ctx context.Context) PermanentRemovalOutcome {
	if ctx.Err() != nil {
		return PermanentRemovalCanceled
	}
	return PermanentRemovalFailed
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
