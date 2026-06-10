package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	UserTempOpportunityStatus = "skipped_by_default"
	UserTempOpportunityReason = "requires_explicit_opt_in"
	userTempDescendantLimit   = 100_000
)

var (
	errOpportunityInspectionLimit = errors.New("opportunity inspection descendant limit exceeded")
	errOpportunityReparsePoint    = errors.New("opportunity inspection encountered a reparse point")
)

type UserTempDiscoveryOptions struct {
	TempDir string
	Now     time.Time
}

type UserTempDiscoveryResult struct {
	Opportunities []UserTempOpportunity             `json:"opportunities"`
	Incomplete    []IncompleteOpportunityInspection `json:"incomplete"`
	ElapsedMS     int64                             `json:"elapsed_ms"`
}

type UserTempOpportunity struct {
	Path             string    `json:"path"`
	Bytes            int64     `json:"bytes"`
	LatestModifiedAt time.Time `json:"latest_modified_at"`
	IdleDays         int       `json:"idle_days"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason"`
}

type IncompleteOpportunityInspection struct {
	Path   string          `json:"path"`
	Reason StructuredIssue `json:"reason"`
}

func DiscoverUserTempOpportunities(ctx context.Context, opts UserTempDiscoveryOptions) UserTempDiscoveryResult {
	return discoverUserTempOpportunities(ctx, opts, opportunityDiscoveryDependencies{
		readDir:  os.ReadDir,
		joinPath: joinOpportunityPath,
		walkDir:  filepath.WalkDir,
	})
}

type opportunityDiscoveryDependencies struct {
	readDir  func(string) ([]os.DirEntry, error)
	joinPath func(string, string) string
	walkDir  func(string, fs.WalkDirFunc) error
}

func discoverUserTempOpportunities(ctx context.Context, opts UserTempDiscoveryOptions, deps opportunityDiscoveryDependencies) UserTempDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	root := opts.TempDir
	if root == "" {
		root = os.TempDir()
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	if deps.joinPath == nil {
		deps.joinPath = joinOpportunityPath
	}
	result := UserTempDiscoveryResult{
		Opportunities: []UserTempOpportunity{},
		Incomplete:    []IncompleteOpportunityInspection{},
	}

	select {
	case <-ctx.Done():
		result.Incomplete = append(result.Incomplete, incompleteInspection(root, "context_canceled", ctx.Err().Error()))
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	default:
	}

	entries, err := deps.readDir(root)
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(root, classifyError(err), err.Error()))
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	}
	for _, entry := range entries {
		path := deps.joinPath(root, entry.Name())
		if !isDirectChildPath(root, path) {
			result.Incomplete = append(result.Incomplete, incompleteInspection(path, "unsafe_path", "resolved entry path is not a direct child of the user temp directory"))
			continue
		}
		if isFoalOwnedTempEntry(entry.Name()) {
			continue
		}
		inspection, err := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
		if err != nil {
			result.Incomplete = append(result.Incomplete, incompleteInspection(path, classifyOpportunityInspectionError(err), err.Error()))
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			continue
		}
		idleDays := int(now.Sub(inspection.latestModifiedAt) / (24 * time.Hour))
		if idleDays < 7 {
			continue
		}
		result.Opportunities = append(result.Opportunities, UserTempOpportunity{
			Path:             path,
			Bytes:            inspection.bytes,
			LatestModifiedAt: inspection.latestModifiedAt,
			IdleDays:         idleDays,
			Status:           UserTempOpportunityStatus,
			Reason:           UserTempOpportunityReason,
		})
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

func joinOpportunityPath(root, name string) string {
	return filepath.Join(root, name)
}

func isDirectChildPath(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanPath) {
		return false
	}
	if strings.HasPrefix(cleanRoot, `\\`) || strings.HasPrefix(cleanPath, `\\`) {
		return false
	}
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || relative == "." || relative == "" || filepath.IsAbs(relative) {
		return false
	}
	return filepath.Dir(relative) == "."
}

type opportunityInspection struct {
	bytes            int64
	latestModifiedAt time.Time
}

func inspectOpportunity(ctx context.Context, path string, descendantLimit int, walkDir func(string, fs.WalkDirFunc) error) (opportunityInspection, error) {
	var inspection opportunityInspection
	descendants := 0
	err := walkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil {
			return walkErr
		}
		if current != path {
			descendants++
			if descendants > descendantLimit {
				return errOpportunityInspectionLimit
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errOpportunityReparsePoint
		}
		if info.ModTime().After(inspection.latestModifiedAt) {
			inspection.latestModifiedAt = info.ModTime()
		}
		if !info.IsDir() {
			inspection.bytes += info.Size()
		}
		return nil
	})
	return inspection, err
}

func classifyOpportunityInspectionError(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "context_canceled"
	case errors.Is(err, errOpportunityInspectionLimit):
		return "inspection_limit_exceeded"
	case errors.Is(err, errOpportunityReparsePoint):
		return "reparse_point"
	default:
		return classifyError(err)
	}
}

func isFoalOwnedTempEntry(name string) bool {
	return strings.HasPrefix(name, "foal-") || strings.HasPrefix(name, "Foal-")
}

func incompleteInspection(path, code, message string) IncompleteOpportunityInspection {
	return IncompleteOpportunityInspection{
		Path:   path,
		Reason: issue(code, message, true, path, ""),
	}
}
