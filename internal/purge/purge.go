// Package purge implements the independent Project artifact purge flow.
// Dry-run previews allowlisted rebuildable directories under one or more
// explicit user-supplied roots; execute rediscovers, authorizes permanent
// deletion per run, and mutates. Roots are never implied.
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
	IssueDangerousRoot                  = "dangerous_root"
	IssueProtectedPath                  = "protected_path"
	IssueProtectionFileLoadFailed       = "protection_file_load_failed"

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

// Options configures dry-run discovery or execute mutation under explicit roots.
type Options struct {
	// Roots are required user-supplied scan roots (one or more). When empty,
	// Root is used as a single-root convenience. Foal never invents defaults.
	Roots []string
	// Root is the single-root convenience field. Prefer Roots for multi-root.
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
	// Validator carries user Protection rules and is applied during discovery
	// (omit protected artifacts) and immediately before each permanent delete.
	Validator pathsafe.Validator
	// ProtectionLoadError fail-closes before any scan when Protection config
	// could not be loaded (same policy as Clean).
	ProtectionLoadError *Issue
	// HistoryRecorder optionally records purge sessions (distinct from Clean).
	HistoryRecorder history.Recorder
	// CommandParameters identify the purge invocation in History.
	CommandParameters history.CommandParameters
}

// Result is the JSON-contract read model for purge dry-run and execute.
type Result struct {
	Status string `json:"status"`
	Mode   string `json:"mode"`
	// Root is set when exactly one scan root was used (single-root back-compat).
	Root string `json:"root,omitempty"`
	// Roots lists every validated scan root (always set on successful discovery).
	Roots      []string      `json:"roots,omitempty"`
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
		Roots:      append([]string(nil), preview.roots...),
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
	// root is the sole root when len(roots)==1 (back-compat for Result.Root).
	root       string
	roots      []string
	candidates []Candidate
	skipped    []Skipped
}

func requestedRoots(opts Options) []string {
	roots := make([]string, 0, len(opts.Roots)+1)
	for _, r := range opts.Roots {
		if trimmed := strings.TrimSpace(r); trimmed != "" {
			roots = append(roots, trimmed)
		}
	}
	if len(roots) == 0 {
		if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
			roots = append(roots, trimmed)
		}
	}
	return roots
}

func errorResult(mode string, start time.Time, roots []string, message string) Result {
	result := Result{
		Status:     StatusError,
		Mode:       mode,
		Candidates: []Candidate{},
		Deleted:    []DeletedItem{},
		Failed:     []FailedItem{},
		Skipped:    []Skipped{},
		Message:    message,
		ElapsedMS:  time.Since(start).Milliseconds(),
	}
	if len(roots) == 1 {
		result.Root = roots[0]
		result.Roots = []string{roots[0]}
	} else if len(roots) > 1 {
		result.Roots = append([]string(nil), roots...)
	}
	return result
}

// discover validates all roots first (fail-closed; no partial scan when any root
// is invalid/dangerous), then finds candidates under each. On hard error or
// cancel, ok is false and errResult is ready to return.
func discover(ctx context.Context, opts Options, mode string, start time.Time) (discoveryPreview, Result, bool) {
	if opts.ProtectionLoadError != nil {
		msg := opts.ProtectionLoadError.Message
		if msg == "" {
			msg = "protection configuration could not be loaded"
		}
		code := opts.ProtectionLoadError.Code
		if code == "" {
			code = IssueProtectionFileLoadFailed
		}
		return discoveryPreview{}, Result{
			Status:     StatusError,
			Mode:       mode,
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Skipped:    []Skipped{},
			Message:    code + ": " + msg,
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}

	rawRoots := requestedRoots(opts)
	if len(rawRoots) == 0 {
		return discoveryPreview{}, errorResult(mode, start, nil,
			"purge requires an explicit root path; refusing to scan without a user-supplied root"), false
	}

	// Phase 1: resolve + policy-validate every root before any tree walk.
	// Any failure rejects the whole run so system trees are never partially scanned.
	absRoots := make([]string, 0, len(rawRoots))
	seen := make(map[string]struct{}, len(rawRoots))
	for _, root := range rawRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return discoveryPreview{}, errorResult(mode, start, []string{root},
				"invalid purge root: "+err.Error()), false
		}
		if reason, ok := pathsafe.ValidateUserScanRoot(absRoot); !ok {
			code := reason.Code
			if code == "" {
				code = IssueDangerousRoot
			}
			return discoveryPreview{}, errorResult(mode, start, []string{absRoot},
				code+": "+reason.Message), false
		}
		info, err := os.Lstat(absRoot)
		if err != nil {
			return discoveryPreview{}, errorResult(mode, start, []string{absRoot},
				"purge root not found or inaccessible: "+err.Error()), false
		}
		if isReparsePoint(info) {
			return discoveryPreview{}, errorResult(mode, start, []string{absRoot},
				"purge root is a reparse point or link and cannot be scanned"), false
		}
		if !info.IsDir() {
			return discoveryPreview{}, errorResult(mode, start, []string{absRoot},
				"purge root must be a directory"), false
		}
		identity := pathsafe.NormalizePathForIdentity(absRoot)
		if _, dup := seen[identity]; dup {
			// Same root twice is not an error; ignore duplicates for discovery.
			continue
		}
		seen[identity] = struct{}{}
		absRoots = append(absRoots, absRoot)
	}

	limit := opts.DescendantLimit
	if limit <= 0 {
		limit = defaultDescendantLimit
	}
	walkDir := opts.WalkDir
	if walkDir == nil {
		walkDir = filepath.WalkDir
	}

	var allCandidates []Candidate
	var allSkipped []Skipped
	for _, absRoot := range absRoots {
		if ctx.Err() != nil {
			break
		}
		scanner := &discovery{
			root:      absRoot,
			limit:     limit,
			walkDir:   walkDir,
			validator: opts.Validator,
		}
		scanner.discover(ctx, absRoot)
		allCandidates = append(allCandidates, scanner.candidates...)
		allSkipped = append(allSkipped, scanner.skipped...)
		if scanner.canceled {
			return discoveryPreview{}, Result{
				Status:     StatusCanceled,
				Mode:       mode,
				Root:       singleRootField(absRoots),
				Roots:      append([]string(nil), absRoots...),
				Candidates: []Candidate{},
				Deleted:    []DeletedItem{},
				Failed:     []FailedItem{},
				Totals:     Totals{},
				Skipped:    allSkipped,
				Message:    "purge discovery canceled; no partial preview claimed",
				ElapsedMS:  time.Since(start).Milliseconds(),
			}, false
		}
	}

	if ctx.Err() != nil {
		return discoveryPreview{}, Result{
			Status:     StatusCanceled,
			Mode:       mode,
			Root:       singleRootField(absRoots),
			Roots:      append([]string(nil), absRoots...),
			Candidates: []Candidate{},
			Deleted:    []DeletedItem{},
			Failed:     []FailedItem{},
			Totals:     Totals{},
			Skipped:    allSkipped,
			Message:    "purge discovery canceled; no partial preview claimed",
			ElapsedMS:  time.Since(start).Milliseconds(),
		}, false
	}

	sort.SliceStable(allCandidates, func(i, j int) bool {
		if allCandidates[i].Path == allCandidates[j].Path {
			return allCandidates[i].RelativePath < allCandidates[j].RelativePath
		}
		return allCandidates[i].Path < allCandidates[j].Path
	})

	return discoveryPreview{
		root:       singleRootField(absRoots),
		roots:      absRoots,
		candidates: allCandidates,
		skipped:    allSkipped,
	}, Result{}, true
}

func singleRootField(roots []string) string {
	if len(roots) == 1 {
		return roots[0]
	}
	return ""
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

	b.WriteString(formatRootsLine(result))
	b.WriteString(fmt.Sprintf("Candidates: %d\n", result.Totals.CandidateCount))
	b.WriteString(fmt.Sprintf("Measured bytes: %d\n", result.Totals.Bytes))
	b.WriteString("Planned action: permanent deletion (requires --execute --allow-permanent)\n")
	if len(result.Candidates) == 0 {
		b.WriteString("No allowlisted project artifacts found under the supplied root(s).\n")
	} else {
		b.WriteString("\n")
		for _, c := range result.Candidates {
			path := c.Path
			if c.RelativePath != "" && len(result.Roots) <= 1 {
				path = c.RelativePath
			}
			b.WriteString(fmt.Sprintf("  %-12s %10d  %s\n", c.Kind, c.Bytes, path))
		}
	}
	if len(result.Skipped) > 0 {
		b.WriteString("\nSkipped:\n")
		for _, s := range result.Skipped {
			// Protected paths: reason only (avoid advertising protected locations
			// as if they were selectable). Other skips may include path context.
			if s.Reason == IssueProtectedPath {
				b.WriteString(fmt.Sprintf("  (%s) %s\n", s.Reason, s.Detail))
				continue
			}
			label := s.Path
			if label == "" {
				label = s.Kind
			}
			b.WriteString(fmt.Sprintf("  %s (%s)\n", label, s.Reason))
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
	b.WriteString(formatRootsLine(result))
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

func formatRootsLine(result Result) string {
	roots := result.Roots
	if len(roots) == 0 && result.Root != "" {
		roots = []string{result.Root}
	}
	if len(roots) == 0 {
		return "Root: (none)\n"
	}
	if len(roots) == 1 {
		return fmt.Sprintf("Root: %s\n", roots[0])
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Roots (%d):\n", len(roots)))
	for _, r := range roots {
		b.WriteString(fmt.Sprintf("  %s\n", r))
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
	validator  pathsafe.Validator
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
		d.skip(path, "", classifyError(err), err.Error())
		return
	}

	for _, entry := range entries {
		if d.checkCancel(ctx) {
			return
		}
		childPath := filepath.Join(path, entry.Name())
		info, err := entry.Info()
		if err != nil {
			d.skip(childPath, "", classifyError(err), err.Error())
			continue
		}
		if isReparsePoint(info) {
			d.skip(childPath, "", "reparse_point", "not traversed")
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
	// Protection is deny-only: omit before measurement/totals so protected
	// artifacts never enter selectable or deletable candidate sets.
	if d.validator.IsUserProtected(path) {
		// Path omitted from skipped.Path to avoid leaking protected locations as
		// selectable-looking entries; kind + stable reason remain for contracts.
		d.skip("", kind, IssueProtectedPath, "omitted by user-defined Protection rule")
		return
	}
	bytes, err := measureTree(ctx, path, d.limit, d.walkDir)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			d.canceled = true
			return
		}
		d.skip(path, kind, classifyMeasureError(err), err.Error())
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

func (d *discovery) skip(path, kind, reason, detail string) {
	s := Skipped{
		Path:   path,
		Kind:   kind,
		Reason: reason,
		Detail: detail,
	}
	// Planned action only applies to allowlisted artifact outcomes (protection
	// omit / measure fail), not incidental traversal skips (reparse, read errors).
	if kind != "" {
		s.PlannedAction = PlannedActionDeletePermanently
	}
	d.skipped = append(d.skipped, s)
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
