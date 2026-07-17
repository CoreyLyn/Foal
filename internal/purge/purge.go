// Package purge implements the independent Project artifact purge flow.
// Dry-run previews allowlisted rebuildable directories under one explicit root;
// execute rediscovers, authorizes permanent deletion per run, and mutates.
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

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

const (
	ModeDryRun  = "dry_run"
	ModeExecute = "execute"

	StatusPreview  = "preview"
	StatusOK       = "ok"
	StatusError    = "error"
	StatusCanceled = "canceled"

	// PlannedActionDeletePermanently is the only planned action for purge candidates.
	PlannedActionDeletePermanently = "delete_permanently"

	// Issue codes (stable contracts).
	IssuePermanentDeletionNotAuthorized = "permanent_deletion_not_authorized"
	IssuePermanentDeleteFailed          = "permanent_delete_failed"
	IssueContextCanceled                = "context_canceled"

	// defaultDescendantLimit matches Clean opportunity inspection ceilings.
	defaultDescendantLimit = 100_000
)

// HighImpactNotice discloses reinstall/rebuild cost and permanent-deletion semantics.
const HighImpactNotice = "High impact: removing project artifacts requires reinstalling dependencies and rebuilding projects. Permanent deletion is ordinary filesystem removal (not secure erasure) and is irreversible."

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

// Options configures dry-run discovery or execute mutation under one explicit root.
type Options struct {
	// Root is required. Empty root is an error; Foal never invents a system-drive scan.
	Root string
	// DescendantLimit caps descendants inspected while measuring one candidate.
	// Zero selects the default (100_000).
	DescendantLimit int
	// WalkDir is injectable for tests; nil uses filepath.WalkDir.
	WalkDir func(string, fs.WalkDirFunc) error
	// AllowPermanentDeletion is the per-run permanent-deletion authorization.
	// Dry-run never requires it. Execute without it skips every candidate with
	// permanent_deletion_not_authorized and deletes nothing.
	AllowPermanentDeletion bool
	// PermanentRemover is injectable for tests; nil uses ordinary filesystem removal.
	PermanentRemover delete.PermanentRemover
	// Validator is applied immediately before each permanent delete.
	Validator pathsafe.Validator
	// HistoryRecorder optionally records purge sessions (distinct from Clean).
	HistoryRecorder history.Recorder
	// CommandParameters identify the purge invocation in History.
	CommandParameters history.CommandParameters
}

// Result is the JSON-contract read model for purge dry-run and execute.
type Result struct {
	Status     string        `json:"status"`
	Mode       string        `json:"mode"`
	Root       string        `json:"root,omitempty"`
	Candidates []Candidate   `json:"candidates"`
	Deleted    []DeletedItem `json:"deleted"`
	Failed     []FailedItem  `json:"failed"`
	Skipped    []Skipped     `json:"skipped"`
	Totals     Totals        `json:"totals"`
	Notices    []string      `json:"notices,omitempty"`
	Message    string        `json:"message,omitempty"`
	ElapsedMS  int64         `json:"elapsed_ms"`
}

// Candidate is one discovered allowlisted project artifact directory.
type Candidate struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	RelativePath  string `json:"relative_path"`
	Bytes         int64  `json:"bytes"`
	PlannedAction string `json:"planned_action"`
}

// DeletedItem is one successfully permanently deleted artifact.
type DeletedItem struct {
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path,omitempty"`
	Bytes        int64  `json:"bytes"`
	Action       string `json:"action"`
}

// FailedItem is a permanent deletion that failed after mutation may have begun.
type FailedItem struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	RelativePath  string `json:"relative_path,omitempty"`
	Bytes         int64  `json:"bytes"`
	PlannedAction string `json:"planned_action"`
	Action        string `json:"action"`
	Reason        Issue  `json:"reason"`
}

// Totals aggregates discovery and mutation outcomes.
type Totals struct {
	CandidateCount          int   `json:"candidate_count"`
	Bytes                   int64 `json:"bytes"`
	DeletedCount            int   `json:"deleted_count"`
	SkippedCount            int   `json:"skipped_count"`
	FailedCount             int   `json:"failed_count"`
	PermanentlyDeletedBytes int64 `json:"permanently_deleted_bytes"`
	AffectedBytes           int64 `json:"affected_bytes"`
}

// Skipped records fail-closed discovery, authorization, or pre-mutation outcomes.
type Skipped struct {
	Path          string `json:"path"`
	Reason        string `json:"reason"`
	Detail        string `json:"detail,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Bytes         int64  `json:"bytes,omitempty"`
	PlannedAction string `json:"planned_action,omitempty"`
}

// Issue is a structured diagnostic attached to failed items.
type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Path        string `json:"path,omitempty"`
}

// DryRun discovers allowlisted project artifacts under one explicit root without mutation.
func DryRun(ctx context.Context, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	preview, errResult, ok := discover(ctx, opts, ModeDryRun, start)
	if !ok {
		return errResult
	}
	result := Result{
		Status:     StatusPreview,
		Mode:       ModeDryRun,
		Root:       preview.root,
		Candidates: preview.candidates,
		Deleted:    []DeletedItem{},
		Failed:     []FailedItem{},
		Skipped:    preview.skipped,
		Totals:     totalsFromPreview(preview.candidates, preview.skipped),
		Notices:    highImpactNotices(len(preview.candidates) > 0),
		ElapsedMS:  time.Since(start).Milliseconds(),
	}
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

type discoveryPreview struct {
	root       string
	candidates []Candidate
	skipped    []Skipped
}

// discover validates the root and finds candidates. On hard error or cancel,
// ok is false and errResult is ready to return.
func discover(ctx context.Context, opts Options, mode string, start time.Time) (discoveryPreview, Result, bool) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    "purge requires an explicit root path; refusing to scan without a user-supplied root",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Root:       root,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    "invalid purge root: " + err.Error(),
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}

	info, err := os.Lstat(absRoot)
	if err != nil {
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Root:       absRoot,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    "purge root not found or inaccessible: " + err.Error(),
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}
	if isReparsePoint(info) {
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Root:       absRoot,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    "purge root is a reparse point or link and cannot be scanned",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}
	if !info.IsDir() {
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Root:       absRoot,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    "purge root must be a directory",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
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
		// Cancellation during discovery: no partial success claim.
		return discoveryPreview{}, Result{
			Status:     StatusCanceled,
			Mode:       mode,
			Root:       absRoot,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Totals:     Totals{},
			Skipped:    scanner.skipped,
			Message:    "purge discovery canceled; no partial preview claimed",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}

	sort.SliceStable(scanner.candidates, func(i, j int) bool {
		if scanner.candidates[i].RelativePath == scanner.candidates[j].RelativePath {
			return scanner.candidates[i].Path < scanner.candidates[j].Path
		}
		return scanner.candidates[i].RelativePath < scanner.candidates[j].RelativePath
	})

	return discoveryPreview{
		root:       absRoot,
		candidates: scanner.candidates,
		skipped:    scanner.skipped,
	}, Result{}, true
}

func totalsFromPreview(candidates []Candidate, skipped []Skipped) Totals {
	var totalBytes int64
	for _, c := range candidates {
		totalBytes += c.Bytes
	}
	return Totals{
		CandidateCount: len(candidates),
		Bytes:          totalBytes,
		SkippedCount:   len(skipped),
	}
}

func highImpactNotices(include bool) []string {
	if !include {
		return nil
	}
	return []string{HighImpactNotice}
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
	b.WriteString("Planned action: permanent deletion (requires --execute --allow-permanent)\n")
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
	for _, notice := range result.Notices {
		b.WriteString("\n")
		b.WriteString(notice)
		b.WriteString("\n")
	}
	if len(result.Notices) == 0 && result.Totals.CandidateCount > 0 {
		b.WriteString("\n")
		b.WriteString(HighImpactNotice)
		b.WriteString("\n")
	}
	b.WriteString("\nNo changes were made. Use --execute --allow-permanent to permanently delete matching artifacts after a fresh rediscovery.\n")
	return b.String()
}

// RenderExecuteReport formats a human execute summary without free-space claims.
func RenderExecuteReport(result Result) string {
	var b strings.Builder
	b.WriteString("Foal purge\n")
	if result.Status == StatusError {
		b.WriteString("Status: error\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
		return b.String()
	}
	if result.Status == StatusCanceled {
		b.WriteString("Status: canceled\n")
		if result.Message != "" {
			b.WriteString(result.Message)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("Execution complete.\n")
	}
	b.WriteString(fmt.Sprintf("Root: %s\n", result.Root))
	b.WriteString(fmt.Sprintf("Deleted: %d, skipped: %d, failed: %d.\n",
		result.Totals.DeletedCount, result.Totals.SkippedCount, result.Totals.FailedCount))
	b.WriteString(fmt.Sprintf("Permanently deleted: %d bytes. Affected: %d bytes.\n",
		result.Totals.PermanentlyDeletedBytes, result.Totals.AffectedBytes))
	if result.Totals.PermanentlyDeletedBytes > 0 || result.Totals.FailedCount > 0 {
		b.WriteString("Permanent deletion is ordinary filesystem removal; it is irreversible and is not a secure-erasure wipe. Completed work is not rolled back.\n")
	}
	for _, notice := range result.Notices {
		b.WriteString(notice)
		b.WriteString("\n")
	}
	if len(result.Notices) == 0 {
		b.WriteString(HighImpactNotice)
		b.WriteString("\n")
	}
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
		Kind:          kind,
		Path:          path,
		RelativePath:  rel,
		Bytes:         bytes,
		PlannedAction: PlannedActionDeletePermanently,
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
