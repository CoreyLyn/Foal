package analyze

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const (
	StatusOK             = "ok"
	StatusIncomplete     = "incomplete"
	defaultTopChildLimit = 10
	// defaultDescendantLimit matches Clean opportunity inspection ceilings. It
	// applies independently to each direct directory child of the root.
	defaultDescendantLimit = 100_000
	// defaultChildMeasurementWorkers bounds concurrent direct-child directory
	// measurements for the CLI/JSON path.
	defaultChildMeasurementWorkers = 4
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
	// State is the child's own measurement state: complete, partial (readable
	// but some descendants omitted), incomplete (its descendant ceiling or
	// cancellation stopped traversal), or skipped (unreadable or a reparse
	// point). Partial and incomplete bytes are observed lower bounds.
	State string `json:"state"`
}

type SkippedItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Options configures analyze.Run behavior (zero values select defaults).
type Options struct {
	// DescendantLimit caps inspected descendants per direct directory child
	// (zero selects default 100_000).
	DescendantLimit int
}

// Run performs directory insight on the supplied root (or current working
// directory when empty). Returns (Result, Reason, ok) where ok is false when
// the root was invalid (Reason contains the failure details).
//
// Every direct child of the root is enumerated and appears in the ranking
// input; each directory child is measured independently with its own
// descendant ceiling (the shared Analyze measurement engine used by the TUI
// browser), so one huge child can never hide its siblings. The root status is
// StatusIncomplete when any child's traversal was stopped by its ceiling or by
// cancellation; totals then describe only inspected content.
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

	result := Result{
		Status:      StatusOK,
		Root:        cleanRoot,
		Totals:      Totals{DirectoryCount: 1},
		TopChildren: []ChildResult{},
		Skipped:     []SkippedItem{},
	}

	entries, err := browseReadDir(cleanRoot)
	if err != nil {
		result.Skipped = append(result.Skipped, SkippedItem{Path: cleanRoot, Reason: classifyError(err), Detail: err.Error()})
		result.ElapsedMS = time.Since(start).Milliseconds()
		return result, pathsafe.Reason{}, true
	}

	outcomes, jobs := classifyRootChildren(cleanRoot, entries)
	measureRootChildren(ctx, outcomes, jobs, limit)

	children := make([]ChildResult, 0, len(outcomes))
	for _, outcome := range outcomes {
		result.Totals.add(outcome.totals)
		result.Skipped = append(result.Skipped, outcome.skipped...)
		children = append(children, outcome.child)
		if outcome.child.State == BrowseStateIncomplete {
			result.Status = StatusIncomplete
		}
	}
	if ctx.Err() != nil {
		result.Status = StatusIncomplete
	}
	result.TopChildren = rankTopChildren(children, defaultTopChildLimit)
	result.ElapsedMS = time.Since(start).Milliseconds()
	return result, pathsafe.Reason{}, true
}

// RunCompat is a compatibility wrapper for the old Run signature (no context,
// no options). Used by existing tests and callers that haven't migrated yet.
func RunCompat(root string) Result {
	result, _, _ := Run(context.Background(), root, Options{})
	return result
}

// rootChildOutcome is one direct child's measurement plus its path-bearing
// omissions, kept in directory order for deterministic merging.
type rootChildOutcome struct {
	child   ChildResult
	totals  Totals
	skipped []SkippedItem
}

// classifyRootChildren resolves every direct child from its directory-entry
// data. Files are complete immediately; unreadable entries and reparse points
// are skipped and never traversed; directories are returned as measurement jobs.
func classifyRootChildren(root string, entries []os.DirEntry) ([]rootChildOutcome, []int) {
	outcomes := make([]rootChildOutcome, len(entries))
	var jobs []int
	for i, entry := range entries {
		childPath := filepath.Join(root, entry.Name())
		child := ChildResult{Name: entry.Name(), Path: childPath}
		info, err := entry.Info()
		switch {
		case err != nil:
			child.Kind = childKind(entry)
			child.State = BrowseStateSkipped
			outcomes[i] = rootChildOutcome{
				child:   child,
				skipped: []SkippedItem{{Path: childPath, Reason: classifyError(err), Detail: err.Error()}},
			}
		case isReparseInfo(childPath, info):
			child.Kind = BrowseKindReparse
			child.State = BrowseStateSkipped
			outcomes[i] = rootChildOutcome{
				child:   child,
				skipped: []SkippedItem{{Path: childPath, Reason: SkipReasonReparsePoint, Detail: "not traversed"}},
			}
		case !info.IsDir():
			child.Kind = BrowseKindFile
			child.Bytes = info.Size()
			child.FileCount = 1
			child.State = BrowseStateComplete
			outcomes[i] = rootChildOutcome{child: child, totals: Totals{Bytes: info.Size(), FileCount: 1}}
		default:
			child.Kind = BrowseKindDirectory
			child.Classification = childClassification(childPath, BrowseKindDirectory)
			outcomes[i] = rootChildOutcome{child: child}
			jobs = append(jobs, i)
		}
	}
	return outcomes, jobs
}

// measureRootChildren measures directory children with bounded concurrency.
// Each job writes only its own outcome slot, so merging stays deterministic.
func measureRootChildren(ctx context.Context, outcomes []rootChildOutcome, jobs []int, limit int) {
	if len(jobs) == 0 {
		return
	}
	workers := defaultChildMeasurementWorkers
	if workers > len(jobs) {
		workers = len(jobs)
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range next {
				outcome := &outcomes[index]
				var skipped []SkippedItem
				measured := measureDirectoryTreeWithSkips(ctx, outcome.child.Path, limit, nil, func(path, reason, detail string) {
					skipped = append(skipped, SkippedItem{Path: path, Reason: reason, Detail: detail})
				})
				outcome.totals = measured.Totals
				outcome.skipped = skipped
				outcome.child.Bytes = measured.Totals.Bytes
				outcome.child.FileCount = measured.Totals.FileCount
				outcome.child.DirectoryCount = measured.Totals.DirectoryCount
				outcome.child.State = measured.State
			}
		}()
	}
	for _, index := range jobs {
		next <- index
	}
	close(next)
	wg.Wait()
}

// rankTopChildren orders children by observed bytes (name tie-break) and keeps
// the first limit entries.
func rankTopChildren(children []ChildResult, limit int) []ChildResult {
	ranked := append([]ChildResult(nil), children...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Bytes == ranked[j].Bytes {
			return ranked[i].Name < ranked[j].Name
		}
		return ranked[i].Bytes > ranked[j].Bytes
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func childClassification(path, kind string) string {
	_, isProjectArtifact := projectArtifactDirectoryNames[filepath.Base(path)]
	if kind == "directory" && isProjectArtifact {
		return ClassificationProjectArtifactClue
	}
	return ""
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
