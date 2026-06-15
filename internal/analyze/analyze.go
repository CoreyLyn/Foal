package analyze

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const defaultTopChildLimit = 10

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

func Run(root string) Result {
	start := time.Now()
	if root == "" {
		root = "."
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		cleanRoot = root
	}

	scanner := scanner{
		root:     cleanRoot,
		children: map[string]ChildResult{},
		skipped:  []SkippedItem{},
	}
	scanner.scan(cleanRoot)
	topChildren := scanner.topChildren(defaultTopChildLimit)

	return Result{
		Status:      "ok",
		Root:        cleanRoot,
		Totals:      scanner.totals,
		TopChildren: topChildren,
		Skipped:     scanner.skipped,
		ElapsedMS:   time.Since(start).Milliseconds(),
	}
}

type scanner struct {
	root     string
	totals   Totals
	children map[string]ChildResult
	skipped  []SkippedItem
}

func (s *scanner) scan(path string) Totals {
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
		childPath := filepath.Join(path, entry.Name())
		childTotals := s.scan(childPath)
		if path == s.root {
			s.addTopChild(childPath, childKind(entry), childTotals)
		}
		totals.add(childTotals)
	}

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
	if kind == "directory" && filepath.Base(path) == "node_modules" {
		return "project_artifact_clue"
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
