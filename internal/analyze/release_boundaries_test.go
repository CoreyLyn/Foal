package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Release-boundary contract tests for issue #351: artifact clues stay direct-child
// only, purge handoff is guarded, and Analyze remains measurement-only.

func TestRunDoesNotClassifyNestedArtifactsDuringRecursiveTraversal(t *testing.T) {
	root := t.TempDir()
	// Direct non-artifact parent containing a nested allowlisted name.
	parent := filepath.Join(root, "apps")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(parent, "node_modules")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "pkg.js"), []byte("nested-only"), 0644); err != nil {
		t.Fatal(err)
	}
	// Ordinary sibling so apps is not the only child.
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	result, reason, ok := Run(context.Background(), root, Options{})
	if !ok {
		t.Fatalf("Run failed: %#v", reason)
	}
	for _, child := range result.TopChildren {
		if child.Name == "apps" && child.Classification != "" {
			t.Fatalf("parent of nested artifact must not be classified: %#v", child)
		}
		if child.Name == "node_modules" {
			t.Fatalf("nested artifact must not appear as a top child of root: %#v", child)
		}
		if child.Classification != "" && child.Name != "node_modules" {
			// Only exact direct-child allowlist names may carry the clue.
			t.Fatalf("unexpected classification on %q: %q", child.Name, child.Classification)
		}
	}
}

func TestBrowseLocationRecursiveMeasurementDoesNotClassifyNestedArtifacts(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(parent, "target"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "target", "out.o"), []byte("obj"), 0644); err != nil {
		t.Fatal(err)
	}
	direct := filepath.Join(root, "dist")
	if err := os.Mkdir(direct, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(direct, "app.js"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BrowseLocation(context.Background(), root, BrowseOptions{})
	if !result.OK {
		t.Fatalf("browse failed: %#v", result.Reason)
	}
	byName := map[string]BrowseChild{}
	for _, c := range result.Children {
		byName[c.Name] = c
	}
	if byName["dist"].Classification != ClassificationProjectArtifactClue {
		t.Fatalf("direct dist classification = %q", byName["dist"].Classification)
	}
	if byName["workspace"].Classification != "" {
		t.Fatalf("workspace must not inherit nested target classification; got %q", byName["workspace"].Classification)
	}
}

func TestAnalyzePackageDoesNotImportHistoryElevationOrMutation(t *testing.T) {
	// Static contract: analyze package source must not wire History, elevation,
	// process control, or delete packages. If this fails, release boundaries regressed.
	// We inspect the package directory relative to this test file.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// When go test runs, cwd is the package directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenImport := []string{
		`"github.com/CoreyLyn/Foal/internal/history"`,
		`"github.com/CoreyLyn/Foal/internal/core/delete"`,
		`"github.com/CoreyLyn/Foal/internal/purge"`,
		`"github.com/CoreyLyn/Foal/internal/clean"`,
		`"golang.org/x/sys/windows"`, // elevation / process helpers live elsewhere
	}
	// Note: golang.org/x/sys/windows may be used for reparse attrs on Windows —
	// allow that only in build-tagged files; still ban history/delete/purge/clean.
	bannedAlways := []string{
		`"github.com/CoreyLyn/Foal/internal/history"`,
		`"github.com/CoreyLyn/Foal/internal/core/delete"`,
		`"github.com/CoreyLyn/Foal/internal/purge"`,
		`"github.com/CoreyLyn/Foal/internal/clean"`,
	}
	_ = forbiddenImport
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, ban := range bannedAlways {
			if strings.Contains(src, ban) {
				t.Fatalf("%s imports %s (Analyze must stay measurement-only)", e.Name(), ban)
			}
		}
		// Negative capability language in production sources is fine as comments;
		// ban actual elevation request helpers if any appear as imports.
		if strings.Contains(src, "RequestElevation") || strings.Contains(src, "ShellExecute") {
			t.Fatalf("%s must not request elevation", e.Name())
		}
	}
}

func TestProtectionNonInterventionStillMeasuresProtectedSubtree(t *testing.T) {
	// Stronger than the soft comment test: set a protection file that would hide
	// a path from Clean, and prove Analyze still measures it. Protection is loaded
	// only by Clean/purge validators, not by Analyze — so setting the env must be
	// a no-op for Run.
	root := t.TempDir()
	child := filepath.Join(root, "keep-measuring")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "blob.bin"), make([]byte, 64), 0644); err != nil {
		t.Fatal(err)
	}
	prot := filepath.Join(t.TempDir(), "protection.txt")
	if err := os.WriteFile(prot, []byte(child+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOAL_PROTECTION_FILE", prot)

	result, reason, ok := Run(context.Background(), root, Options{})
	if !ok {
		t.Fatalf("Run failed under protection env: %#v", reason)
	}
	found := false
	for _, c := range result.TopChildren {
		if c.Name == "keep-measuring" {
			found = true
			if c.Bytes < 64 {
				t.Fatalf("protected child must still be measured; bytes=%d", c.Bytes)
			}
		}
	}
	if !found {
		t.Fatalf("protection must not hide Analyze measurements: %#v", result.TopChildren)
	}
}
