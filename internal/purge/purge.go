// Package purge implements the independent Project artifact purge flow.
// This slice is dry-run / preview-only for a single explicit root (issue #241).
package purge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// ModeDryRun is the only supported mode in this slice.
	ModeDryRun = "dry_run"

	StatusPreview  = "preview"
	StatusError    = "error"
	StatusCanceled = "canceled"

	// defaultDescendantLimit matches Clean opportunity inspection ceilings.
	defaultDescendantLimit = 100_000
)

// v1 high-confidence Project artifact directory names (exact final component).
// Aligned with analyze's project artifact clue set; deliberately excludes bin/obj.
var artifactDirectoryNames = map[string]struct{}{
	"node_modules": {},
	"target":       {},
	"dist":         {},
	"build":        {},
	".build":       {},
	".next":        {},
	"__pycache__":  {},
}

var (
	errInspectionLimit = errors.New("purge inspection descendant limit exceeded")
	errReparsePoint    = errors.New("purge inspection encountered a reparse point")
)

// Options configures a dry-run discovery under one explicit root.
type Options struct {
	// Root is required. Empty root is an error; Foal never invents a system-drive scan.
	Root string
	// DescendantLimit caps descendants inspected while measuring one candidate.
	// Zero selects the default (100_000).
	DescendantLimit int
	// WalkDir is injectable for tests; nil uses filepath.WalkDir.
	WalkDir func(string, fs.WalkDirFunc) error
}

// Result is the JSON-contract read model for purge dry-run.
type Result struct {
	Status     string      `json:"status"`
	Mode       string      `json:"mode"`
	Root       string      `json:"root,omitempty"`
	Candidates []Candidate `json:"candidates"`
	Totals     Totals      `json:"totals"`
	Skipped    []Skipped   `json:"skipped"`
	Message    string      `json:"message,omitempty"`
	ElapsedMS  int64       `json:"elapsed_ms"`
}

// Candidate is one discovered allowlisted project artifact directory.
type Candidate struct {
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Bytes        int64  `json:"bytes"`
}

// Totals aggregates successful candidate measurements only.
type Totals struct {
	CandidateCount int   `json:"candidate_count"`
	Bytes          int64 `json:"bytes"`
}

// Skipped records fail-closed discovery or measurement outcomes.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// DryRun discovers allowlisted project artifacts under one explicit root without mutation.
func DryRun(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return Result{
			Status:     StatusError,
			Mode:       ModeDryRun,
			Candidates: []Candidate{},
			Skipped:    []Skipped{},
			Message:    "purge requires an explicit root path; refusing to scan without a user-supplied root",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{
			Status:     StatusError,
			Mode:       ModeDryRun,
			Root:       root,
			Candidates: []Candidate{},
			Skipped:    []Skipped{},
			Message:    "invalid purge root: " + err.Error(),
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}

	info, err := os.Lstat(absRoot)
	if err != nil {
		return Result{
			Status:     StatusError,
			Mode:       ModeDryRun,
			Root:       absRoot,
			Candidates: []Candidate{},
			Skipped:    []Skipped{},
			Message:    "purge root not found or inaccessible: " + err.Error(),
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}
	if isReparsePoint(info) {
		return Result{
			Status:     StatusError,
			Mode:       ModeDryRun,
			Root:       absRoot,
			Candidates: []Candidate{},
			Skipped:    []Skipped{},
			Message:    "purge root is a reparse point or link and cannot be scanned",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}
	if !info.IsDir() {
		return Result{
			Status:     StatusError,
			Mode:       ModeDryRun,
			Root:       absRoot,
			Candidates: []Candidate{},
			Skipped:    []Skipped{},
			Message:    "purge root must be a directory",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}

	limit := opts.DescendantLimit
	if limit <= 0 {
		limit = defaultDescendantLimit
	}
	walkDir := opts.WalkDir
	if walkDir == nil {
		walkDir = filepath.WalkDir
	}

	scanner := &discovery{
		root:       absRoot,
		limit:      limit,
		walkDir:    walkDir,
		candidates: nil,
		skipped:    nil,
	}
	scanner.discover(ctx, absRoot)

	if ctx.Err() != nil || scanner.canceled {
		// Cancellation: no partial success claim.
		return Result{
			Status:     StatusCanceled,
			Mode:       ModeDryRun,
			Root:       absRoot,
			Candidates: []Candidate{},
			Totals:     Totals{},
			Skipped:    scanner.skipped,
			Message:    "purge discovery canceled; no partial preview claimed",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}
	}

	sort.SliceStable(scanner.candidates, func(i, j int) bool {
		if scanner.candidates[i].RelativePath == scanner.candidates[j].RelativePath {
			return scanner.candidates[i].Path < scanner.candidates[j].Path
		}
		return scanner.candidates[i].RelativePath < scanner.candidates[j].RelativePath
	})

	var totalBytes int64
	for _, c := range scanner.candidates {
		totalBytes += c.Bytes
	}

	return Result{
		Status:     StatusPreview,
		Mode:       ModeDryRun,
		Root:       absRoot,
		Candidates: scanner.candidates,
		Totals: Totals{
			CandidateCount: len(scanner.candidates),
			Bytes:          totalBytes,
		},
		Skipped:   scanner.skipped,
		ElapsedMS: time.Since(start).Milliseconds(),
	}
}

// RenderPreviewReport formats a human dry-run preview (no mutation language).
func RenderPreviewReport(result Result) string {
	var b strings.Builder
	b.WriteString("Foal purge (dry-run / preview only)\n")
	if result.Status == StatusError {
		b.WriteString("Status: error\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
		b.WriteString("No changes were made.\n")
		return b.String()
	}
	if result.Status == StatusCanceled {
		b.WriteString("Status: canceled\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
		b.WriteString("No changes were made.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Root: %s\n", result.Root))
	b.WriteString(fmt.Sprintf("Candidates: %d\n", result.Totals.CandidateCount))
	b.WriteString(fmt.Sprintf("Measured bytes: %d\n", result.Totals.Bytes))
	if len(result.Candidates) == 0 {
		b.WriteString("No allowlisted project artifacts found under this root.\n")
	} else {
		b.WriteString("\n")
		for _, c := range result.Candidates {
			path := c.RelativePath
			if path == "" {
				path = c.Path
			}
			b.WriteString(fmt.Sprintf("  %-12s %10d  %s\n", c.Kind, c.Bytes, path))
		}
	}
	if len(result.Skipped) > 0 {
		b.WriteString("\nSkipped:\n")
		for _, s := range result.Skipped {
			b.WriteString(fmt.Sprintf("  %s (%s)\n", s.Path, s.Reason))
		}
	}
	b.WriteString("\nNo changes were made. Execute is not available in this slice.\n")
	return b.String()
}

// IsArtifactDirectoryName reports whether name is an exact v1 allowlisted final component.
func IsArtifactDirectoryName(name string) bool {
	_, ok := artifactDirectoryNames[name]
	return ok
}

type discovery struct {
	root       string
	limit      int
	walkDir    func(string, fs.WalkDirFunc) error
	candidates []Candidate
	skipped    []Skipped
	canceled   bool
}

func (d *discovery) discover(ctx context.Context, path string) {
	if d.checkCancel(ctx) {
		return
	}

	// If the supplied root itself is an allowlisted artifact, measure only it.
	if path == d.root {
		base := filepath.Base(path)
		if IsArtifactDirectoryName(base) {
			d.measureCandidate(ctx, path, base)
			return
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		d.skip(path, classifyError(err), err.Error())
		return
	}

	for _, entry := range entries {
		if d.checkCancel(ctx) {
			return
		}
		childPath := filepath.Join(path, entry.Name())
		info, err := entry.Info()
		if err != nil {
			d.skip(childPath, classifyError(err), err.Error())
			continue
		}
		if isReparsePoint(info) {
			d.skip(childPath, "reparse_point", "not traversed")
			continue
		}
		if !info.IsDir() {
			continue
		}
		name := entry.Name()
		if IsArtifactDirectoryName(name) {
			d.measureCandidate(ctx, childPath, name)
			// Do not walk into matched artifacts for further discovery (avoid nested double-count).
			continue
		}
		d.discover(ctx, childPath)
	}
}

func (d *discovery) measureCandidate(ctx context.Context, path, kind string) {
	if d.checkCancel(ctx) {
		return
	}
	bytes, err := measureTree(ctx, path, d.limit, d.walkDir)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			d.canceled = true
			return
		}
		d.skip(path, classifyMeasureError(err), err.Error())
		return
	}
	rel, err := filepath.Rel(d.root, path)
	if err != nil {
		rel = path
	}
	d.candidates = append(d.candidates, Candidate{
		Kind:         kind,
		Path:         path,
		RelativePath: rel,
		Bytes:        bytes,
	})
}

func (d *discovery) skip(path, reason, detail string) {
	d.skipped = append(d.skipped, Skipped{
		Path:   path,
		Reason: reason,
		Detail: detail,
	})
}

func (d *discovery) checkCancel(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		d.canceled = true
		return true
	default:
		return false
	}
}

func measureTree(ctx context.Context, path string, descendantLimit int, walkDir func(string, fs.WalkDirFunc) error) (int64, error) {
	var total int64
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
				return errInspectionLimit
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if isReparsePoint(info) {
			return errReparsePoint
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func classifyError(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "not_found"
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	return "read_error"
}

func classifyMeasureError(err error) string {
	switch {
	case errors.Is(err, errInspectionLimit):
		return "inspection_limit_exceeded"
	case errors.Is(err, errReparsePoint):
		return "reparse_point"
	default:
		return classifyError(err)
	}
}

func isReparsePoint(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
