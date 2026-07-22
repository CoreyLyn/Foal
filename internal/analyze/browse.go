package analyze

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// Browse child kinds and terminal measurement states for on-demand disk browse.
const (
	BrowseKindFile      = "file"
	BrowseKindDirectory = "directory"
	BrowseKindReparse   = "reparse_point"

	BrowseStateComplete   = "complete"
	BrowseStateIncomplete = "incomplete"
	BrowseStateSkipped    = "skipped"
)

// BrowseOptions configures BrowseLocation (zero values select defaults).
type BrowseOptions struct {
	// DescendantLimit caps inspected descendants per direct directory child
	// (zero selects default 100_000). Each directory child is measured
	// independently with its own ceiling.
	DescendantLimit int
}

// BrowseChild is one direct child of a browse location.
type BrowseChild struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Bytes          int64  `json:"bytes"`
	FileCount      int64  `json:"file_count"`
	DirectoryCount int64  `json:"directory_count"`
	// Classification is set only for direct children (project_artifact_clue).
	// Recursive measurement never classifies nested artifacts.
	Classification string `json:"classification,omitempty"`
	// State is complete, incomplete, or skipped for this slice.
	State string `json:"state"`
	// SkipReason is set when State is skipped (e.g. reparse_point, permission_denied).
	SkipReason string `json:"skip_reason,omitempty"`
	// Hidden and System are presentation-only Windows attribute flags.
	Hidden bool `json:"hidden,omitempty"`
	System bool `json:"system,omitempty"`
	// Navigable is true only for ordinary directories (not files, not reparse).
	Navigable bool `json:"navigable"`
}

// BrowseResult is the complete direct-child inventory for one location.
type BrowseResult struct {
	Root      string        `json:"root"`
	Children  []BrowseChild `json:"children"`
	ElapsedMS int64         `json:"elapsed_ms"`
	// Reason is set when the location itself cannot be browsed.
	Reason pathsafe.Reason `json:"reason,omitempty"`
	OK     bool            `json:"ok"`
}

// BrowseLocation enumerates every direct child of root after entry and measures
// each directory child recursively with an independent descendant limit.
//
// Contract:
//   - Files expose logical size immediately and are never navigable.
//   - Directories are measured independently (serial in this slice).
//   - Reparse children are visible, not traversed, and not navigable.
//   - Hidden/system children remain visible with presentation-only flags.
//   - Nested project artifacts are not classified during recursive walks.
//   - No sibling locations are prefetched; only root is read.
//   - Read-only: no mutation, elevation, process action, or History write.
func BrowseLocation(ctx context.Context, root string, opts BrowseOptions) BrowseResult {
	start := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(root) == "" {
		return BrowseResult{
			OK:     false,
			Reason: pathsafe.Reason{Code: "empty_path", Message: "analyze browse root cannot be empty"},
		}
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return BrowseResult{
			OK:     false,
			Reason: pathsafe.Reason{Code: "invalid_root", Message: "invalid analyze browse root: " + err.Error()},
		}
	}
	if reason, ok := pathsafe.ValidateAnalyzeReadRoot(cleanRoot); !ok {
		return BrowseResult{Root: cleanRoot, OK: false, Reason: reason, ElapsedMS: time.Since(start).Milliseconds()}
	}

	limit := opts.DescendantLimit
	if limit <= 0 {
		limit = defaultDescendantLimit
	}

	entries, err := os.ReadDir(cleanRoot)
	if err != nil {
		return BrowseResult{
			Root:      cleanRoot,
			OK:        false,
			Reason:    pathsafe.Reason{Code: classifyError(err), Message: err.Error()},
			ElapsedMS: time.Since(start).Milliseconds(),
		}
	}

	// Stable name order for serial measurement (two-worker scheduler is #348).
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	children := make([]BrowseChild, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			// Cooperative cancel: return what we have so far.
			return BrowseResult{
				Root:      cleanRoot,
				Children:  children,
				OK:        true,
				ElapsedMS: time.Since(start).Milliseconds(),
			}
		default:
		}
		children = append(children, inspectBrowseChild(ctx, cleanRoot, entry, limit))
	}

	return BrowseResult{
		Root:      cleanRoot,
		Children:  children,
		OK:        true,
		ElapsedMS: time.Since(start).Milliseconds(),
	}
}

func inspectBrowseChild(ctx context.Context, parent string, entry os.DirEntry, limit int) BrowseChild {
	name := entry.Name()
	childPath := filepath.Join(parent, name)
	child := BrowseChild{
		Name: name,
		Path: childPath,
	}

	info, err := os.Lstat(childPath)
	if err != nil {
		child.Kind = kindFromDirEntry(entry)
		child.State = BrowseStateSkipped
		child.SkipReason = classifyError(err)
		return child
	}

	attrs := filePresentationAttributes(childPath, info)
	child.Hidden = attrs.Hidden
	child.System = attrs.System

	if attrs.Reparse || info.Mode()&os.ModeSymlink != 0 {
		// Visible but neither traversed nor navigable.
		child.Kind = BrowseKindReparse
		child.State = BrowseStateSkipped
		child.SkipReason = "reparse_point"
		child.Navigable = false
		return child
	}

	if !info.IsDir() {
		child.Kind = BrowseKindFile
		child.Bytes = info.Size()
		child.FileCount = 1
		child.State = BrowseStateComplete
		child.Navigable = false
		return child
	}

	child.Kind = BrowseKindDirectory
	child.Navigable = true
	child.Classification = childClassification(childPath, BrowseKindDirectory)

	// Independent recursive measurement for this directory only.
	// Nested artifact classification is intentionally not performed.
	totals, incomplete := measureDirectoryTree(ctx, childPath, limit)
	child.Bytes = totals.Bytes
	child.FileCount = totals.FileCount
	child.DirectoryCount = totals.DirectoryCount
	if incomplete {
		child.State = BrowseStateIncomplete
	} else {
		child.State = BrowseStateComplete
	}
	return child
}

func kindFromDirEntry(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return BrowseKindReparse
	}
	if entry.IsDir() {
		return BrowseKindDirectory
	}
	return BrowseKindFile
}

// measureDirectoryTree measures path as an independent tree with its own
// descendant ceiling. It does not classify nested project artifacts.
func measureDirectoryTree(ctx context.Context, path string, limit int) (Totals, bool) {
	s := &treeMeasurer{
		root:  path,
		limit: limit,
	}
	totals := s.measure(ctx, path)
	return totals, s.incomplete
}

type treeMeasurer struct {
	root        string
	limit       int
	incomplete  bool
	descendants int
}

func (s *treeMeasurer) measure(ctx context.Context, path string) Totals {
	select {
	case <-ctx.Done():
		s.incomplete = true
		return Totals{}
	default:
	}

	info, err := os.Lstat(path)
	if err != nil {
		return Totals{}
	}
	if isReparsePoint(info) || hasReparseAttr(path) {
		// Nested reparse: do not traverse; omit from observed totals.
		return Totals{}
	}
	if !info.IsDir() {
		return Totals{Bytes: info.Size(), FileCount: 1}
	}

	totals := Totals{DirectoryCount: 1}
	entries, err := os.ReadDir(path)
	if err != nil {
		// Unreadable directory still counts as a directory shell (partial-like
		// omission of descendants). Full Partial state streaming is #347.
		return totals
	}

	for _, entry := range entries {
		if s.incomplete {
			break
		}
		select {
		case <-ctx.Done():
			s.incomplete = true
		default:
		}
		if s.incomplete {
			break
		}

		// Count every inspected descendant under this measured directory toward
		// the independent per-child ceiling (default 100_000).
		s.descendants++
		if s.descendants > s.limit {
			s.incomplete = true
			break
		}

		childPath := filepath.Join(path, entry.Name())
		totals.add(s.measure(ctx, childPath))
	}
	return totals
}

// presentationAttributes holds Windows presentation-only flags.
type presentationAttributes struct {
	Hidden  bool
	System  bool
	Reparse bool
}

// filePresentationAttributes is implemented per-platform.
// Windows reads FILE_ATTRIBUTE_*; other platforms return zeros.
func filePresentationAttributes(path string, info os.FileInfo) presentationAttributes {
	return platformPresentationAttributes(path, info)
}

func hasReparseAttr(path string) bool {
	attrs := platformPresentationAttributes(path, nil)
	return attrs.Reparse
}
