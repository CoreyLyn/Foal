package analyze

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRunClassifiesDirectNodeModulesDirectoryAsProjectArtifactClue(t *testing.T) {
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "package.js"), []byte("module"), 0644); err != nil {
		t.Fatal(err)
	}

	result := Run(root)

	if len(result.TopChildren) != 1 {
		t.Fatalf("len(TopChildren) = %d, want 1", len(result.TopChildren))
	}
	child := result.TopChildren[0]
	if child.Name != "node_modules" {
		t.Fatalf("Name = %q, want node_modules", child.Name)
	}
	if child.Path != nodeModules {
		t.Fatalf("Path = %q, want %q", child.Path, nodeModules)
	}
	if child.Kind != "directory" {
		t.Fatalf("Kind = %q, want directory", child.Kind)
	}
	if child.Classification != "project_artifact_clue" {
		t.Fatalf("Classification = %q, want project_artifact_clue", child.Classification)
	}
}

func TestRunDoesNotClassifyNonMatchingProjectArtifactClues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "node_modules"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node_modules_backup", "source"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	nested := filepath.Join(root, "project", "node_modules")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	result := Run(root)

	for _, child := range result.TopChildren {
		if child.Classification != "" {
			t.Fatalf("%s Classification = %q, want empty", child.Name, child.Classification)
		}
		if child.Name == "node_modules" && child.Kind != "file" {
			t.Fatalf("node_modules Kind = %q, want file", child.Kind)
		}
	}
}

func TestRunClassificationDoesNotChangeTopChildSelectionOrOrdering(t *testing.T) {
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "small.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < defaultTopChildLimit; i++ {
		name := "child-" + strconv.Itoa(i)
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data.bin"), make([]byte, 20-i), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := Run(root)

	if len(result.TopChildren) != defaultTopChildLimit {
		t.Fatalf("len(TopChildren) = %d, want %d", len(result.TopChildren), defaultTopChildLimit)
	}
	for i, child := range result.TopChildren {
		wantName := "child-" + strconv.Itoa(i)
		if child.Name != wantName {
			t.Fatalf("TopChildren[%d].Name = %q, want %q", i, child.Name, wantName)
		}
		if child.Path != filepath.Join(root, wantName) {
			t.Fatalf("TopChildren[%d].Path = %q, want %q", i, child.Path, filepath.Join(root, wantName))
		}
		if child.Kind != "directory" || child.Bytes != int64(20-i) || child.FileCount != 1 || child.DirectoryCount != 1 {
			t.Fatalf("TopChildren[%d] changed existing fields: %#v", i, child)
		}
		if child.Classification != "" {
			t.Fatalf("TopChildren[%d].Classification = %q, want empty", i, child.Classification)
		}
	}
	if result.Totals.Bytes != 156 || result.Totals.FileCount != 11 || result.Totals.DirectoryCount != 12 {
		t.Fatalf("Totals = %#v, want bytes=156 files=11 directories=12", result.Totals)
	}
}
