package delete

import (
	"context"
	"errors"
	"io/fs"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

type Candidate struct {
	Path  string
	Bytes int64
}

type Adapter interface {
	MoveToRecycleBin(path string) error
}

type Result struct {
	AffectedBytes int64
	Deleted       []DeletedItem
	Skipped       []SkippedItem
}

type DeletedItem struct {
	Path  string
	Bytes int64
}

type SkippedItem struct {
	Path   string
	Bytes  int64
	Reason pathsafe.Reason
}

func Execute(ctx context.Context, candidates []Candidate, adapter Adapter) Result {
	return ExecuteWithValidator(ctx, candidates, adapter, pathsafe.Validator{})
}

func ExecuteWithValidator(ctx context.Context, candidates []Candidate, adapter Adapter, validator pathsafe.Validator) Result {
	return ExecuteWithHooks(ctx, candidates, adapter, validator, nil)
}

// ExecuteWithHooks moves candidates to the Recycle Bin with PathSafe validation
// and an optional category-owned pre-mutation check immediately before each move.
// Ordering per candidate: cancel check → PathSafe → PreMutation (if non-nil) →
// MoveToRecycleBin. A pre-mutation rejection is a recoverable skip: the adapter
// is not called and the move is never rerouted to permanent deletion. A nil
// preMutation is a no-op (PathSafe-only composition), so existing callers keep
// identical behavior.
func ExecuteWithHooks(ctx context.Context, candidates []Candidate, adapter Adapter, validator pathsafe.Validator, preMutation PreMutationValidator) Result {
	var result Result
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			result.Skipped = append(result.Skipped, SkippedItem{
				Path:  candidate.Path,
				Bytes: candidate.Bytes,
				Reason: pathsafe.Reason{
					Code:    "context_canceled",
					Message: ctx.Err().Error(),
				},
			})
			continue
		default:
		}

		if reason, ok := validator.ValidateDeletePath(candidate.Path); !ok {
			result.Skipped = append(result.Skipped, SkippedItem{
				Path:   candidate.Path,
				Bytes:  candidate.Bytes,
				Reason: reason,
			})
			continue
		}

		if preMutation != nil {
			if reason, ok := preMutation(candidate); !ok {
				result.Skipped = append(result.Skipped, SkippedItem{
					Path:   candidate.Path,
					Bytes:  candidate.Bytes,
					Reason: reason,
				})
				continue
			}
		}

		if err := adapter.MoveToRecycleBin(candidate.Path); err != nil {
			reason := pathsafe.Reason{
				Code:    "delete_failed",
				Message: err.Error(),
			}
			if errors.Is(err, fs.ErrPermission) {
				reason = pathsafe.Reason{
					Code:    "permission_denied",
					Message: "permission denied while moving item to the Recycle Bin",
				}
			} else if errors.Is(err, fs.ErrInvalid) {
				reason = pathsafe.Reason{
					Code:    "unsupported_target",
					Message: "target cannot be moved to the Recycle Bin",
				}
			}
			result.Skipped = append(result.Skipped, SkippedItem{
				Path:   candidate.Path,
				Bytes:  candidate.Bytes,
				Reason: reason,
			})
			continue
		}

		result.Deleted = append(result.Deleted, DeletedItem{Path: candidate.Path, Bytes: candidate.Bytes})
		result.AffectedBytes += candidate.Bytes
	}
	return result
}
