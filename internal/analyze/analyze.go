package analyze

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const (
	StatusOK             = "ok"
	StatusIncomplete     = "incomplete"
	defaultTopChildLimit = 10
	// defaultDescendantLimit matches Clean opportunity inspection ceilings.
	defaultDescendantLimit = 100_000
)

var projectArtifactDirectoryNames = map[string]struct{}{
	"node_modules": {},
	"target":       {},
	"dist":         {},
	"build":        {},
	".build":       {},
	".next":        {},
	"__pycache__":  {},
}

var (
	errInspectionLimit = errors.New("analyze inspection descendant limit exceeded")
)

type Result struct {
	Status      string        `json:"status"`
	Root        string        `json:"root"`
	Totals      Totals        `json:"totals"`
	TopChildren []ChildResult `json:"top_children"`
	Skipped     []SkippedItem `json:"skipped"`
	ElapsedMS   int64         `json:"elapsed_ms"`
}

type Totals struct {
	Bytes          int64 `json:"bytes"`
	FileCount      int64 `json:"file_count"`
	DirectoryCount int64 `json:"directory_count"`
}

type ChildResult struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Classification string `json:"classification,omitempty"`
	Bytes          int64  `json:"bytes"`
	FileCount      int64  `json:"file_count"`
	DirectoryCount int64  `json:"directory_count"`
}

type SkippedItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Options configures analyze.Run behavior (zero values select defaults).
type Options struct {
	// DescendantLimit caps inspected descendants (zero selects default 100_000).
	DescendantLimit int
}

// Run performs directory insight on the supplied root (or current working
// directory when empty). Returns (Result, Reason, ok) where ok is false when
// the root was invalid (Reason contains the failure details).
// Complete scans return StatusOK; scans halted by limits/cancellation return
// StatusIncomplete with partial totals describing only inspected content.
//
// Root policy uses pathsafe.ValidateAnalyzeReadRoot (read-only). Explicit local
// fixed/removable volume roots and Windows-managed trees are allowed. This never
// authorizes Clean, Purge, or other mutation paths.
func Run(ctx context.Context, root string, opts Options) (Result, pathsafe.Reason, bool) {
	start := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	if root == "" {
		root = "."
	}
	// Whitespace-only explicit roots fail closed (empty string alone means CWD).
	if strings.TrimSpace(root) == "" {
		return Result{}, pathsafe.Reason{Code: "empty_path", Message: "analyze root cannot be empty"}, false
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, pathsafe.Reason{Code: "invalid_root", Message: "invalid analyze root: " + err.Error()}, false
	}

	// Fail-closed: Analyze-specific read-root policy (not mutation/purge policy).
	if reason, ok := pathsafe.ValidateAnalyzeReadRoot(cleanRoot); !ok {
		return Result{}, reason, false
	}

	limit := opts.DescendantLimit
	if limit <= 0 {
		limit = defaultDescendantLimit
	}

	scanner := scanner{
		root:       cleanRoot,
		limit:      limit,
		children:   map[string]ChildResult{},
		skipped:    []SkippedItem{},
		incomplete: false,
	}
	scanner.scan(ctx, cleanRoot)
	topChildren := scanner.topChildren(defaultTopChildLimit)

	status := StatusOK
	if scanner.incomplete {
		status = StatusIncomplete
	}

	return Result{
		Status:      status,
		Root:        cleanRoot,
		Totals:      scanner.totals,
		TopChildren: topChildren,
		Skipped:     scanner.skipped,
		ElapsedMS:   time.Since(start).Milliseconds(),
	}, pathsafe.Reason{}, true
}

// RunCompat is a compatibility wrapper for the old Run signature (no context,
// no options). Used by existing tests and callers that haven't migrated yet.
func RunCompat(root string) Result {
	result, _, _ := Run(context.Background(), root, Options{})
	return result
}

type scanner struct {
	root        string
	limit       int
	totals      Totals
	children    map[string]ChildResult
	skipped     []SkippedItem
	incomplete  bool
	descendants int
}

func (s *scanner) scan(ctx context.Context, path string) Totals {
	// Check for cancellation first.
	select {
	case <-ctx.Done():
		s.incomplete = true
		return Totals{}
	default:
	}

	info, err := os.Lstat(path)
	if err != nil {
		s.skip(path, classifyError(err), err.Error())
		return Totals{}
	}
	if isReparsePoint(info) {
		s.skip(path, "reparse_point", "not traversed")
		return Totals{}
	}
	if !info.IsDir() {
		totals := Totals{Bytes: info.Size(), FileCount: 1}
		s.totals.add(totals)
		return totals
	}

	totals := Totals{DirectoryCount: 1}
	entries, err := os.ReadDir(path)
	if err != nil {
		s.skip(path, classifyError(err), err.Error())
		s.totals.add(totals)
		return totals
	}

	for _, entry := range entries {
		// Stop if we've already hit limits or been canceled.
		if s.incomplete {
			break
		}
		select {
		case <-ctx.Done():
			s.incomplete = true
			break
		default:
		}

		// Count descendants (children of the root are first counted here).
		if path != s.root {
			s.descendants++
			if s.descendants > s.limit {
				s.incomplete = true
				break
			}
		}

		childPath := filepath.Join(path, entry.Name())
		childTotals := s.scan(ctx, childPath)
		if path == s.root {
			s.addTopChild(childPath, childKind(entry), childTotals)
		}
		totals.add(childTotals)
	}

	// Always add what we found for this directory, even if incomplete.
	// This maintains original counting behavior.
	s.totals.add(Totals{DirectoryCount: 1})
	return totals
}

func (s *scanner) addTopChild(path, kind string, totals Totals) {
	s.children[path] = ChildResult{
		Name:           filepath.Base(path),
		Path:           path,
		Kind:           kind,
		Classification: childClassification(path, kind),
		Bytes:          totals.Bytes,
		FileCount:      totals.FileCount,
		DirectoryCount: totals.DirectoryCount,
	}
}

func childClassification(path, kind string) string {
	_, isProjectArtifact := projectArtifactDirectoryNames[filepath.Base(path)]
	if kind == "directory" && isProjectArtifact {
		return ClassificationProjectArtifactClue
	}
	return ""
}

func (s *scanner) skip(path, reason, detail string) {
	s.skipped = append(s.skipped, SkippedItem{
		Path:   path,
		Reason: reason,
		Detail: detail,
	})
}

func (s *scanner) topChildren(limit int) []ChildResult {
	children := make([]ChildResult, 0, len(s.children))
	for _, child := range s.children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Bytes == children[j].Bytes {
			return children[i].Name < children[j].Name
		}
		return children[i].Bytes > children[j].Bytes
	})
	if len(children) > limit {
		children = children[:limit]
	}
	return children
}

func (t *Totals) add(other Totals) {
	t.Bytes += other.Bytes
	t.FileCount += other.FileCount
	t.DirectoryCount += other.DirectoryCount
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

func isReparsePoint(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func childKind(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "reparse_point"
	}
	if entry.IsDir() {
		return "directory"
	}
	return "file"
}
