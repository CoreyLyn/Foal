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

		if reason, ok := pathsafe.ValidateDeletePath(candidate.Path); !ok {
			result.Skipped = append(result.Skipped, SkippedItem{
				Path:   candidate.Path,
				Bytes:  candidate.Bytes,
				Reason: reason,
			})
			continue
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
