package purge_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/purge"
)

func TestDryRunRequiresExplicitRoot(t *testing.T) {
	result := purge.DryRun(context.Background(), purge.Options{Root: ""})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q, want %s", result.Status, purge.StatusError)
	}
	if result.Mode != purge.ModeDryRun {
		t.Fatalf("mode = %q, want %s", result.Mode, purge.ModeDryRun)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none", result.Candidates)
	}
	if !strings.Contains(result.Message, "explicit root") {
		t.Fatalf("message = %q, want explicit root error", result.Message)
	}
}

func TestDryRunRejectsMissingRootWithoutInventingScan(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	result := purge.DryRun(context.Background(), purge.Options{Root: missing})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q, want %s", result.Status, purge.StatusError)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none on invalid root", result.Candidates)
	}
	if !strings.Contains(strings.ToLower(result.Message), "not found") &&
		!strings.Contains(strings.ToLower(result.Message), "inaccessible") {
		t.Fatalf("message = %q, want not-found style error", result.Message)
	}
}

func TestDryRunRejectsFileRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	result := purge.DryRun(context.Background(), purge.Options{Root: file})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q, want %s", result.Status, purge.StatusError)
	}
	if !strings.Contains(result.Message, "directory") {
		t.Fatalf("message = %q, want directory error", result.Message)
	}
}

func TestDryRunDiscoversNestedAllowlistedArtifactsOnly(t *testing.T) {
	root := t.TempDir()
	// monorepo-style layout
	paths := map[string][]byte{
		filepath.Join("app1", "node_modules", "pkg", "index.js"): []byte("aaa"),
		filepath.Join("app2", "target", "debug", "app.exe"):      []byte("bbbb"),
		filepath.Join("app2", "src", "main.rs"):                  []byte("src"),
		filepath.Join("web", "dist", "bundle.js"):                []byte("ccccc"),
		filepath.Join("web", "build", "out.js"):                  []byte("d"),
		filepath.Join("tooling", ".build", "cache"):              []byte("ee"),
		filepath.Join("next", ".next", "static", "x"):            []byte("fff"),
		filepath.Join("py", "pkg", "__pycache__", "m.pyc"):       []byte("gggg"),
		// non-matches
		filepath.Join("app1", "node_modules_backup", "x"): []byte("nope"),
		filepath.Join("legacy", "bin", "tool.exe"):        []byte("bin"),
		filepath.Join("legacy", "obj", "a.o"):             []byte("obj"),
		filepath.Join("src", "code.go"):                   []byte("code"),
	}
	for rel, body := range paths {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, body, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := purge.DryRun(context.Background(), purge.Options{Root: root})
	if result.Status != purge.StatusPreview {
		t.Fatalf("status = %q message=%q, want preview", result.Status, result.Message)
	}
	if result.Mode != purge.ModeDryRun {
		t.Fatalf("mode = %q", result.Mode)
	}

	wantKinds := map[string]int64{
		"node_modules": 3,
		"target":       4,
		"dist":         5,
		"build":        1,
		".build":       2,
		".next":        3,
		"__pycache__":  4,
	}
	if len(result.Candidates) != len(wantKinds) {
		t.Fatalf("candidates = %#v, want %d kinds", result.Candidates, len(wantKinds))
	}
	got := map[string]purge.Candidate{}
	for _, c := range result.Candidates {
		got[c.Kind] = c
		if c.Path == "" || c.RelativePath == "" {
			t.Fatalf("candidate missing path fields: %#v", c)
		}
		if !strings.HasPrefix(c.Path, root) {
			t.Fatalf("candidate path %q escaped root %q", c.Path, root)
		}
		if strings.Contains(c.RelativePath, "node_modules_backup") ||
			c.Kind == "bin" || c.Kind == "obj" {
			t.Fatalf("non-allowlisted candidate: %#v", c)
		}
	}
	for kind, bytes := range wantKinds {
		c, ok := got[kind]
		if !ok {
			t.Fatalf("missing kind %q in %#v", kind, result.Candidates)
		}
		if c.Bytes != bytes {
			t.Fatalf("%s bytes = %d, want %d", kind, c.Bytes, bytes)
		}
	}
	if result.Totals.CandidateCount != len(wantKinds) {
		t.Fatalf("totals.candidate_count = %d", result.Totals.CandidateCount)
	}
	var sum int64
	for _, b := range wantKinds {
		sum += b
	}
	if result.Totals.Bytes != sum {
		t.Fatalf("totals.bytes = %d, want %d", result.Totals.Bytes, sum)
	}
}

func TestDryRunExactNameMatchOnly(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"node_modules_backup", "dist-cache", "my-build", "bin", "obj", "TargetExtra"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// real match nested
	nm := filepath.Join(root, "proj", "node_modules")
	if err := os.MkdirAll(nm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "p"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	result := purge.DryRun(context.Background(), purge.Options{Root: root})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only node_modules", result.Candidates)
	}
	if result.Candidates[0].Kind != "node_modules" {
		t.Fatalf("kind = %q", result.Candidates[0].Kind)
	}
}

func TestDryRunDoesNotRecurseIntoMatchedArtifacts(t *testing.T) {
	root := t.TempDir()
	// Outer node_modules containing a nested node_modules directory.
	outer := filepath.Join(root, "node_modules")
	inner := filepath.Join(outer, "dep", "node_modules")
	if err := os.MkdirAll(inner, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "a"), []byte("aa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "b"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	result := purge.DryRun(context.Background(), purge.Options{Root: root})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want single outer node_modules (no nested double-list)", result.Candidates)
	}
	if result.Candidates[0].Bytes != 3 { // "aa" + "b"
		t.Fatalf("bytes = %d, want 3 (outer measurement includes nested content)", result.Candidates[0].Bytes)
	}
}

func TestDryRunSkipsReparsePointsFailClosed(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "x"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Allowlisted name as a symlink should not become a candidate.
	link := filepath.Join(root, "node_modules")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	// Nested real artifact still found.
	nested := filepath.Join(root, "app", "target")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "y"), []byte("yy"), 0644); err != nil {
		t.Fatal(err)
	}

	result := purge.DryRun(context.Background(), purge.Options{Root: root})
	for _, c := range result.Candidates {
		if c.Kind == "node_modules" {
			t.Fatalf("symlink node_modules must not be a candidate: %#v", c)
		}
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Kind != "target" {
		t.Fatalf("candidates = %#v, want only target", result.Candidates)
	}
	foundSkip := false
	for _, s := range result.Skipped {
		if s.Path == link && s.Reason == "reparse_point" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("skipped = %#v, want reparse_point for symlink", result.Skipped)
	}
}

func TestDryRunInspectionCeilingFailClosed(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nm, 0755); err != nil {
		t.Fatal(err)
	}
	// Real sibling still measurable.
	dist := filepath.Join(root, "dist")
	if err := os.Mkdir(dist, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	walkDir := func(path string, fn fs.WalkDirFunc) error {
		if path != nm {
			return filepath.WalkDir(path, fn)
		}
		// Synthetic over-limit walk for node_modules only.
		if err := fn(path, fakeDirEntry{name: filepath.Base(path), mode: os.ModeDir}, nil); err != nil {
			return err
		}
		for i := 0; i < 3; i++ {
			name := "f" + string(rune('0'+i))
			child := filepath.Join(path, name)
			if err := fn(child, fakeDirEntry{name: name, mode: 0}, nil); err != nil {
				return err
			}
		}
		return nil
	}

	result := purge.DryRun(context.Background(), purge.Options{
		Root:            root,
		DescendantLimit: 2,
		WalkDir:         walkDir,
	})

	if result.Status != purge.StatusPreview {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Kind != "dist" {
		t.Fatalf("candidates = %#v, want only dist after fail-closed measure", result.Candidates)
	}
	found := false
	for _, s := range result.Skipped {
		if s.Path == nm && s.Reason == "inspection_limit_exceeded" {
			found = true
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want inspection_limit_exceeded for node_modules", result.Skipped)
	}
}

func TestDryRunCancelDiscardsPartialSuccess(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := purge.DryRun(ctx, purge.Options{Root: root})
	if result.Status != purge.StatusCanceled {
		t.Fatalf("status = %q, want %s", result.Status, purge.StatusCanceled)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, cancel must not claim partial success", result.Candidates)
	}
	if result.Totals.CandidateCount != 0 || result.Totals.Bytes != 0 {
		t.Fatalf("totals = %#v, want zero on cancel", result.Totals)
	}
}

func TestDryRunJSONContractShape(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "a"), []byte("ab"), 0644); err != nil {
		t.Fatal(err)
	}

	result := purge.DryRun(context.Background(), purge.Options{Root: root})
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"status", "mode", "root", "candidates", "totals", "skipped", "elapsed_ms"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("JSON missing %q: %s", key, raw)
		}
	}
	if decoded["status"] != purge.StatusPreview || decoded["mode"] != purge.ModeDryRun {
		t.Fatalf("status/mode = %#v %#v", decoded["status"], decoded["mode"])
	}
	cands := decoded["candidates"].([]interface{})
	if len(cands) != 1 {
		t.Fatalf("candidates = %#v", cands)
	}
	c := cands[0].(map[string]interface{})
	for _, key := range []string{"kind", "path", "relative_path", "bytes"} {
		if _, ok := c[key]; !ok {
			t.Fatalf("candidate missing %q: %#v", key, c)
		}
	}
	if c["kind"] != "node_modules" || c["bytes"] != float64(2) {
		t.Fatalf("candidate = %#v", c)
	}
	if c["planned_action"] != purge.PlannedActionDeletePermanently {
		t.Fatalf("planned_action = %#v", c["planned_action"])
	}
	totals := decoded["totals"].(map[string]interface{})
	if totals["candidate_count"] != float64(1) || totals["bytes"] != float64(2) {
		t.Fatalf("totals = %#v", totals)
	}
	// Dry-run never mutates and always discloses high-impact rebuild cost.
	if len(result.Deleted) != 0 || result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("dry-run must not claim deletion: %#v", result)
	}
	if len(result.Notices) == 0 || !strings.Contains(result.Notices[0], "reinstalling") {
		t.Fatalf("notices = %#v, want high-impact rebuild notice", result.Notices)
	}
}

func TestRenderPreviewReportListsKindPathBytes(t *testing.T) {
	report := purge.RenderPreviewReport(purge.Result{
		Status: purge.StatusPreview,
		Mode:   purge.ModeDryRun,
		Root:   `D:\work\proj`,
		Candidates: []purge.Candidate{{
			Kind:          "node_modules",
			Path:          `D:\work\proj\app\node_modules`,
			RelativePath:  `app\node_modules`,
			Bytes:         42,
			PlannedAction: purge.PlannedActionDeletePermanently,
		}},
		Totals:  purge.Totals{CandidateCount: 1, Bytes: 42},
		Notices: []string{purge.HighImpactNotice},
	})
	for _, want := range []string{
		"Foal purge",
		"dry-run",
		"node_modules",
		"42",
		`app\node_modules`,
		"No changes were made",
		"reinstalling dependencies",
		"--execute --allow-permanent",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestIsArtifactDirectoryNameAllowlist(t *testing.T) {
	for _, name := range []string{"node_modules", "target", "dist", "build", ".build", ".next", "__pycache__"} {
		if !purge.IsArtifactDirectoryName(name) {
			t.Fatalf("%q should be allowlisted", name)
		}
	}
	for _, name := range []string{"bin", "obj", "node_modules_backup", "dist-cache", "Build", "NODE_MODULES"} {
		if purge.IsArtifactDirectoryName(name) {
			t.Fatalf("%q must not be allowlisted", name)
		}
	}
}

// --- test helpers that reach package-private Options.walkDir via purge_export_test.go ---

type fakeDirEntry struct {
	name string
	mode os.FileMode
}

func (e fakeDirEntry) Name() string      { return e.name }
func (e fakeDirEntry) IsDir() bool       { return e.mode.IsDir() }
func (e fakeDirEntry) Type() fs.FileMode { return e.mode.Type() }
func (e fakeDirEntry) Info() (fs.FileInfo, error) {
	return fakeFileInfo{name: e.name, mode: e.mode}, nil
}

type fakeFileInfo struct {
	name string
	mode os.FileMode
}

func (i fakeFileInfo) Name() string       { return i.name }
func (i fakeFileInfo) Size() int64        { return 1 }
func (i fakeFileInfo) Mode() os.FileMode  { return i.mode }
func (i fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (i fakeFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeFileInfo) Sys() interface{}   { return nil }
